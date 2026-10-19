package panel

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

func testOIDCToken(key *rsa.PrivateKey, claims Data) string {
	head := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","kid":"test"}`))
	body := base64.RawURLEncoding.EncodeToString(mustJSON(claims))
	raw := head + "." + body
	hash := sha256.Sum256([]byte(raw))
	sig, _ := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
	return raw + "." + base64.RawURLEncoding.EncodeToString(sig)
}
func TestIntegrationOIDCNonceAudienceReplayBindingAndSecondFactor(t *testing.T) {
	s, ctx := integration(t)
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	var mu sync.Mutex
	tokens := map[string]Data{}
	challenges := map[string]string{}
	issuer := ""
	idp := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			json.NewEncoder(w).Encode(Data{"issuer": issuer, "authorization_endpoint": issuer + "/auth", "token_endpoint": issuer + "/token", "jwks_uri": issuer + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}})
		case "/keys":
			json.NewEncoder(w).Encode(Data{"keys": []any{Data{"kty": "RSA", "kid": "test", "alg": "RS256", "use": "sig", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": "AQAB"}}})
		case "/token":
			r.ParseForm()
			mu.Lock()
			claims := tokens[r.Form.Get("code")]
			challenge := challenges[r.Form.Get("code")]
			mu.Unlock()
			h := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if claims == nil || base64.RawURLEncoding.EncodeToString(h[:]) != challenge {
				w.WriteHeader(400)
				return
			}
			json.NewEncoder(w).Encode(Data{"access_token": "mock-access-token", "token_type": "Bearer", "id_token": testOIDCToken(key, claims), "expires_in": 3600})
		default:
			w.WriteHeader(404)
		}
	}))
	defer idp.Close()
	issuer = idp.URL
	s.OIDCHTTP = idp.Client()
	s.Config.OIDCIssuer = issuer
	s.Config.OIDCClientID = "panel-client"
	s.Config.OIDCClientSecret = "server-secret"
	s.Config.PublicURL = "https://panel.example"
	s.Config.SecureCookies = true
	user := defaults("users")
	user["username"], user["oidc_issuer"], user["oidc_subject"], user["role"] = "sso-admin", issuer, "linked-subject", "viewer"
	if e = save(ctx, s.Store.Pool, "users", user); e != nil {
		t.Fatal(e)
	}
	start := func(code string, patch Data) (string, *http.Cookie) {
		t.Helper()
		w := httptest.NewRecorder()
		out, _, e := s.oidcStart(w, httptest.NewRequest("POST", "/api/auth/oidc/start", nil).WithContext(ctx))
		if e != nil {
			t.Fatal(e)
		}
		u, e := url.Parse(str(out.(Data), "authorization_url"))
		if e != nil {
			t.Fatal(e)
		}
		q := u.Query()
		if q.Get("code_challenge_method") != "S256" || q.Get("redirect_uri") != "https://panel.example/api/auth/oidc/callback" {
			t.Fatal(q)
		}
		claims := merge(Data{"iss": issuer, "aud": "panel-client", "sub": "linked-subject", "nonce": q.Get("nonce"), "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix()}, patch)
		mu.Lock()
		tokens[code] = claims
		challenges[code] = q.Get("code_challenge")
		mu.Unlock()
		cookies := w.Result().Cookies()
		if len(cookies) != 1 || cookies[0].SameSite != http.SameSiteLaxMode || !cookies[0].Secure || !cookies[0].HttpOnly {
			t.Fatal("OIDC cookie binding", cookies)
		}
		return q.Get("state"), cookies[0]
	}
	callback := func(state, code string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "https://panel.example/api/auth/oidc/callback?state="+url.QueryEscape(state)+"&code="+url.QueryEscape(code), nil).WithContext(ctx)
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		s.oidcCallback(w, r)
		return w
	}
	for n, patch := range []Data{{"nonce": "wrong"}, {"aud": "other-client"}, {"sub": "unlinked-subject"}, {"exp": time.Now().Add(-time.Hour).Unix()}, {"iss": "https://other-issuer.example"}, {"aud": []string{"panel-client", "other"}, "azp": "other"}} {
		code := fmt.Sprintf("invalid%d", n)
		state, cookie := start(code, patch)
		w := callback(state, code, cookie)
		if w.Header().Get("Location") != "/?oidc=error" {
			t.Fatal("invalid identity accepted", patch, w.Header())
		}
	}
	state, cookie := start("valid", Data{})
	w := callback(state, "valid", cookie)
	if w.Header().Get("Location") != "/" {
		t.Fatal(w.Header())
	}
	if callback(state, "valid", cookie).Header().Get("Location") != "/?oidc=error" {
		t.Fatal("OIDC state replay accepted")
	}
	var session *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "veloray_session" {
			session = c
		}
	}
	if session == nil {
		t.Fatal("session not issued")
	}
	r := httptest.NewRequest("GET", "https://panel.example/api/auth/me", nil).WithContext(ctx)
	r.AddCookie(session)
	a, e := s.authenticate(r)
	if e != nil || num(a.User, "id") != num(user, "id") || a.SecondFactor {
		t.Fatal(a, e)
	}
	// A mismatched browser cannot consume the authorization state.
	state, cookie = start("bound-browser", Data{})
	if callback(state, "bound-browser", &http.Cookie{Name: "veloray_oidc", Value: "other-browser"}).Header().Get("Location") != "/?oidc=error" {
		t.Fatal("browser binding bypass")
	}
	if callback(state, "bound-browser", cookie).Header().Get("Location") != "/" {
		t.Fatal("state consumed by wrong browser")
	}
	// Preconfigured local 2FA remains mandatory after a valid provider response.
	secret := "JBSWY3DPEHPK3PXP"
	sealed, _ := seal(s.Config.FieldKey, secret)
	user["_totp_secret"], user["_totp_enabled"] = sealed, true
	if e = save(ctx, s.Store.Pool, "users", user); e != nil {
		t.Fatal(e)
	}
	state, cookie = start("with-totp", Data{})
	w = callback(state, "with-totp", cookie)
	if w.Header().Get("Location") != "/?oidc=totp" {
		t.Fatal("2FA bypass", w.Header())
	}
	var challengeCookie *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "veloray_session" {
			t.Fatal("session issued before 2FA")
		}
		if c.Name == "veloray_oidc_challenge" {
			challengeCookie = c
		}
	}
	if challengeCookie == nil {
		t.Fatal("2FA challenge absent")
	}
	code, _ := totpCode(secret, time.Now().Unix()/30)
	request := func() *http.Request {
		r := httptest.NewRequest("POST", "/api/auth/oidc/complete", strings.NewReader(string(mustJSON(Data{"totp_code": code})))).WithContext(ctx)
		r.AddCookie(challengeCookie)
		return r
	}
	out, _, e := s.oidcComplete(httptest.NewRecorder(), request())
	if e != nil || !flag(out.(Data), "two_factor_setup_required") && num(out.(Data), "id") != num(user, "id") {
		t.Fatal(out, e)
	}
	if _, _, e = s.oidcComplete(httptest.NewRecorder(), request()); e == nil {
		t.Fatal("SSO second factor challenge replay")
	}
}
