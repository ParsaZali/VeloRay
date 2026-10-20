package panel

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestIntegrationGitHubPKCEBindingReplayAndSecondFactor(t *testing.T) {
	s, ctx := integration(t)
	s.Config.PublicURL = "https://panel.example"
	s.Config.SecureCookies = true
	s.Config.GitHubClientID = "github-client"
	s.Config.GitHubClientSecret = "github-secret"
	user := defaults("users")
	user["username"], user["oidc_issuer"], user["oidc_subject"] = "github-admin", githubIdentityIssuer, "42"
	if err := save(ctx, s.Store.Pool, "users", user); err != nil {
		t.Fatal(err)
	}
	challenge := ""
	subject := "42"
	requests := 0
	s.GitHubHTTP = &http.Client{Transport: featureTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		var body string
		switch r.URL.String() {
		case "https://github.com/login/oauth/access_token":
			if r.Method != "POST" || r.ParseForm() != nil || r.PostForm.Get("client_id") != "github-client" || r.PostForm.Get("client_secret") != "github-secret" || r.PostForm.Get("redirect_uri") != "https://panel.example/api/auth/github/callback" {
				t.Error("incorrect token exchange")
			}
			hash := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
			if base64.RawURLEncoding.EncodeToString(hash[:]) != challenge {
				t.Error("PKCE mismatch")
			}
			if r.URL.RawQuery != "" {
				t.Error("OAuth secrets in URL")
			}
			body = `{"access_token":"test-github-access","token_type":"Bearer"}`
		case "https://api.github.com/user":
			if r.Method != "GET" || r.Header.Get("Authorization") != "Bearer test-github-access" {
				t.Error("incorrect identity request")
			}
			body = fmt.Sprintf(`{"id":%s,"login":"username-may-change","email":"ignored@example.com"}`, subject)
		default:
			t.Error("unexpected OAuth endpoint", r.URL)
			return nil, fmt.Errorf("unexpected endpoint")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	start := func() (string, *http.Cookie) {
		t.Helper()
		w := httptest.NewRecorder()
		out, code, err := s.githubStart(w, httptest.NewRequest("POST", "/api/auth/github/start", nil).WithContext(ctx))
		if err != nil || code != 200 {
			t.Fatal(out, code, err)
		}
		u, err := url.Parse(str(out.(Data), "authorization_url"))
		if err != nil {
			t.Fatal(err)
		}
		q := u.Query()
		challenge = q.Get("code_challenge")
		if u.Host != "github.com" || u.Path != "/login/oauth/authorize" || q.Get("code_challenge_method") != "S256" || q.Get("scope") != "" || q.Get("allow_signup") != "false" {
			t.Fatal("invalid authorization URL", u)
		}
		cookies := w.Result().Cookies()
		if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
			t.Fatal("invalid browser cookie")
		}
		return q.Get("state"), cookies[0]
	}
	callback := func(state string, cookie *http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest("GET", "https://panel.example/api/auth/github/callback?state="+url.QueryEscape(state)+"&code=test-code", nil).WithContext(ctx)
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		s.githubCallback(w, r)
		return w
	}
	state, cookie := start()
	wrong := *cookie
	wrong.Value = "wrong-browser"
	if callback(state, &wrong).Header().Get("Location") != "/?oidc=error" || requests != 0 {
		t.Fatal("browser binding bypass")
	}
	w := callback(state, cookie)
	if w.Header().Get("Location") != "/" || requests != 2 {
		t.Fatal("valid GitHub login failed", w.Header(), requests)
	}
	var session *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "veloray_session" {
			session = c
		}
	}
	if session == nil {
		t.Fatal("no session")
	}
	r := httptest.NewRequest("GET", "/api/auth/me", nil).WithContext(ctx)
	r.AddCookie(session)
	a, err := s.authenticate(r)
	if err != nil || num(a.User, "id") != num(user, "id") {
		t.Fatal("wrong account", err)
	}
	if callback(state, cookie).Header().Get("Location") != "/?oidc=error" || requests != 2 {
		t.Fatal("state replay accepted")
	}
	for _, invalid := range []string{"43", "0", "-1", "1.5"} {
		subject = invalid
		state, cookie = start()
		if callback(state, cookie).Header().Get("Location") != "/?oidc=error" {
			t.Fatal("unbound or invalid subject accepted")
		}
	}
	subject = "42"
	user["_totp_enabled"] = true
	user["_totp_secret"], _ = seal(s.Config.FieldKey, "JBSWY3DPEHPK3PXP")
	if err = save(ctx, s.Store.Pool, "users", user); err != nil {
		t.Fatal(err)
	}
	state, cookie = start()
	w = callback(state, cookie)
	if w.Header().Get("Location") != "/?oidc=totp" {
		t.Fatal("second factor bypass")
	}
	var second *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "veloray_session" {
			t.Fatal("session before second factor")
		}
		if c.Name == "veloray_oidc_challenge" {
			second = c
		}
	}
	if second == nil {
		t.Fatal("missing challenge")
	}
	code, _ := totpCode("JBSWY3DPEHPK3PXP", time.Now().Unix()/30)
	req := httptest.NewRequest("POST", "/api/auth/oidc/complete", strings.NewReader(string(mustJSON(Data{"totp_code": code})))).WithContext(ctx)
	req.AddCookie(second)
	if _, _, err = s.oidcComplete(httptest.NewRecorder(), req); err != nil {
		t.Fatal("second factor failed", err)
	}
}
