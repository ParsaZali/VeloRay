package panel

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-webauthn/webauthn/protocol/webauthncbor"
)

const passkeyTestPassword = "Local-passkey-test-strong-123"

type passkeyBrowser struct {
	s       *Server
	t       *testing.T
	cookies map[string]*http.Cookie
}

func (b *passkeyBrowser) request(method, path string, input Data) (Data, int) {
	b.t.Helper()
	raw, _ := json.Marshal(input)
	r := httptest.NewRequest(method, b.s.Config.PublicURL+path, strings.NewReader(string(raw)))
	csrf := csrfToken(b.s.Config.SecretKey)
	r.Header.Set("Origin", b.s.Config.PublicURL)
	r.Header.Set("X-CSRFToken", csrf)
	r.AddCookie(&http.Cookie{Name: "csrftoken", Value: csrf})
	for _, cookie := range b.cookies {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	b.s.Handler().ServeHTTP(w, r)
	for _, cookie := range w.Result().Cookies() {
		if cookie.MaxAge < 0 {
			delete(b.cookies, cookie.Name)
		} else {
			b.cookies[cookie.Name] = cookie
		}
	}
	data, _ := decode(w.Body.Bytes())
	return data, w.Code
}
func passkeyFixture(t *testing.T) (*Server, context.Context, Data, *passkeyBrowser) {
	t.Helper()
	s, ctx := integration(t)
	s.Config.PublicURL = "https://panel.example"
	s.Config.TrustedOrigins[s.Config.PublicURL] = true
	s.Config.SecureCookies = true
	u := defaults("users")
	u["username"] = "passkey-admin"
	u["is_superuser"] = true
	u["_password"], _ = hashPassword(passkeyTestPassword)
	if e := save(ctx, s.Store.Pool, "users", u); e != nil {
		t.Fatal(e)
	}
	browser := &passkeyBrowser{s: s, t: t, cookies: map[string]*http.Cookie{}}
	if out, status := browser.request("POST", "/api/auth/login", Data{"username": str(u, "username"), "password": passkeyTestPassword}); status != 200 {
		t.Fatal(out, status)
	}
	return s, ctx, u, browser
}

type softPasskey struct {
	key *ecdsa.PrivateKey
	id  []byte
}

func newSoftPasskey(t *testing.T) *softPasskey {
	t.Helper()
	k, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	id := make([]byte, 32)
	rand.Read(id)
	return &softPasskey{key: k, id: id}
}
func passkeyBase64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
func (k *softPasskey) register(challenge, origin, rp string, verified bool) Data {
	client, _ := json.Marshal(Data{"type": "webauthn.create", "challenge": challenge, "origin": origin, "crossOrigin": false})
	hash := sha256.Sum256([]byte(rp))
	auth := append([]byte{}, hash[:]...)
	flags := byte(0x41)
	if verified {
		flags |= 4
	}
	auth = append(auth, flags)
	auth = append(auth, make([]byte, 4+16)...)
	auth = binary.BigEndian.AppendUint16(auth, uint16(len(k.id)))
	auth = append(auth, k.id...)
	key, _ := webauthncbor.Marshal(map[int]any{1: 2, 3: -7, -1: 1, -2: k.key.X.FillBytes(make([]byte, 32)), -3: k.key.Y.FillBytes(make([]byte, 32))})
	auth = append(auth, key...)
	att, _ := webauthncbor.Marshal(map[string]any{"fmt": "none", "authData": auth, "attStmt": map[string]any{}})
	return Data{"id": passkeyBase64(k.id), "rawId": passkeyBase64(k.id), "type": "public-key", "authenticatorAttachment": "platform", "response": Data{"clientDataJSON": passkeyBase64(client), "attestationObject": passkeyBase64(att), "transports": []any{"internal"}}, "clientExtensionResults": Data{}}
}
func (k *softPasskey) assertion(challenge, origin, rp string, handle []byte, count uint32, verified bool) Data {
	client, _ := json.Marshal(Data{"type": "webauthn.get", "challenge": challenge, "origin": origin, "crossOrigin": false})
	hash := sha256.Sum256([]byte(rp))
	auth := append([]byte{}, hash[:]...)
	flags := byte(1)
	if verified {
		flags |= 4
	}
	auth = append(auth, flags)
	auth = binary.BigEndian.AppendUint32(auth, count)
	clientHash := sha256.Sum256(client)
	signed := append(append([]byte{}, auth...), clientHash[:]...)
	digest := sha256.Sum256(signed)
	signature, _ := ecdsa.SignASN1(rand.Reader, k.key, digest[:])
	return Data{"id": passkeyBase64(k.id), "rawId": passkeyBase64(k.id), "type": "public-key", "authenticatorAttachment": "platform", "response": Data{"clientDataJSON": passkeyBase64(client), "authenticatorData": passkeyBase64(auth), "signature": passkeyBase64(signature), "userHandle": passkeyBase64(handle)}, "clientExtensionResults": Data{}}
}
func enrollSoftPasskey(t *testing.T, s *Server, b *passkeyBrowser, k *softPasskey) {
	t.Helper()
	options, status := b.request("POST", "/api/auth/passkeys/register/begin", Data{"name": "My device", "password": passkeyTestPassword})
	if status != 200 {
		t.Fatal(options, status)
	}
	public := obj(options, "publicKey")
	if str(obj(public, "authenticatorSelection"), "userVerification") != "required" {
		t.Fatal(options)
	}
	body := k.register(str(public, "challenge"), s.Config.PublicURL, "panel.example", true)
	out, status := b.request("POST", "/api/auth/passkeys/register/finish", body)
	if status != 201 {
		t.Fatal(out, status)
	}
}
func TestPasskeyConfigurationRequiresTrustedDomain(t *testing.T) {
	for _, origin := range []string{"https://127.0.0.1:8443", "http://panel.example", "https://user:password@panel.example", "https://panel.example/path"} {
		s := &Server{Config: Config{PublicURL: origin}}
		if _, e := s.webAuthn(); e == nil {
			t.Fatal(origin)
		}
	}
	for _, origin := range []string{"https://panel.example:1000", "http://localhost:8863"} {
		s := &Server{Config: Config{PublicURL: origin}}
		if _, e := s.webAuthn(); e != nil {
			t.Fatal(origin, e)
		}
	}
}
func TestIntegrationPasskeysCryptographicRegistrationLoginReplayAndRevocation(t *testing.T) {
	s, ctx, u, b := passkeyFixture(t)
	other := &passkeyBrowser{s: s, t: t, cookies: map[string]*http.Cookie{}}
	if _, status := other.request("POST", "/api/auth/login", Data{"username": str(u, "username"), "password": passkeyTestPassword}); status != 200 {
		t.Fatal(status)
	}
	key := newSoftPasskey(t)
	enrollSoftPasskey(t, s, b, key)
	if out, status := other.request("GET", "/api/auth/me", nil); status != 401 {
		t.Fatal("other sessions survived enrollment", out, status)
	}
	cfg, _ := settings(ctx, s.Store.Pool)
	cfg["require_2fa_for_admins"] = true
	if e := saveSettings(ctx, s.Store.Pool, cfg); e != nil {
		t.Fatal(e)
	}
	if out, status := b.request("PATCH", "/api/settings", Data{"passkeys_enabled": false}); status != 400 {
		t.Fatal("disabled the last sign-in factor for accounts under required MFA", out, status)
	}
	fresh := &passkeyBrowser{s: s, t: t, cookies: map[string]*http.Cookie{}}
	options, status := fresh.request("POST", "/api/auth/passkey/login/begin", Data{})
	if status != 200 {
		t.Fatal(options, status)
	}
	cookie := fresh.cookies["veloray_passkey_login"]
	assertion := key.assertion(str(obj(options, "publicKey"), "challenge"), s.Config.PublicURL, "panel.example", s.passkeyHandle(num(u, "id")), 1, true)
	out, status := fresh.request("POST", "/api/auth/passkey/login/finish", assertion)
	if status != 200 || flag(out, "two_factor_setup_required") {
		t.Fatal(out, status)
	}
	if out, status = fresh.request("GET", "/api/overview", nil); status != 200 {
		t.Fatal("verified passkey did not satisfy MFA policy", out, status)
	}
	fresh.cookies["veloray_passkey_login"] = cookie
	if out, status = fresh.request("POST", "/api/auth/passkey/login/finish", assertion); status != 401 {
		t.Fatal("replayed consumed challenge", out, status)
	}
	list, status := fresh.request("GET", "/api/auth/passkeys", nil)
	if status != 200 || len(array(list, "credentials")) != 1 {
		t.Fatal(list, status)
	}
	if out, status = fresh.request("DELETE", "/api/auth/passkeys/"+passkeyBase64(key.id), Data{"password": passkeyTestPassword}); status != 403 {
		t.Fatal("removed the last required MFA factor", out, status)
	}
	cfg["require_2fa_for_admins"] = false
	if e := saveSettings(ctx, s.Store.Pool, cfg); e != nil {
		t.Fatal(e)
	}
	if out, status = fresh.request("DELETE", "/api/auth/passkeys/"+passkeyBase64(key.id), Data{"password": passkeyTestPassword}); status != 204 {
		t.Fatal(out, status)
	}
	options, status = fresh.request("POST", "/api/auth/passkey/login/begin", Data{})
	if status != 200 {
		t.Fatal(options, status)
	}
	assertion = key.assertion(str(obj(options, "publicKey"), "challenge"), s.Config.PublicURL, "panel.example", s.passkeyHandle(num(u, "id")), 2, true)
	if out, status = fresh.request("POST", "/api/auth/passkey/login/finish", assertion); status != 401 {
		t.Fatal("revoked key still signed in", out, status)
	}
}
func TestIntegrationPasskeysRejectWrongOriginRPChallengeSignatureAndMissingUV(t *testing.T) {
	s, ctx, u, b := passkeyFixture(t)
	key := newSoftPasskey(t)
	enrollSoftPasskey(t, s, b, key)
	for _, kind := range []string{"origin", "rp", "challenge", "signature", "uv", "handle"} {
		t.Run(kind, func(t *testing.T) {
			options, status := b.request("POST", "/api/auth/passkey/login/begin", Data{})
			if status != 200 {
				t.Fatal(options, status)
			}
			challenge, origin, rp, handle, uv := str(obj(options, "publicKey"), "challenge"), s.Config.PublicURL, "panel.example", s.passkeyHandle(num(u, "id")), true
			switch kind {
			case "origin":
				origin = "https://evil.example"
			case "rp":
				rp = "evil.example"
			case "challenge":
				challenge = "another-challenge-abcdefghijklmnopqrstuvwxyz"
			case "uv":
				uv = false
			case "handle":
				handle = []byte("another-user")
			}
			response := key.assertion(challenge, origin, rp, handle, 1, uv)
			if kind == "signature" {
				obj(response, "response")["signature"] = "invalid-signature"
			}
			if out, status := b.request("POST", "/api/auth/passkey/login/finish", response); status != 401 {
				t.Fatal("accepted invalid assertion", kind, out, status)
			}
		})
	}
	// A password-only session cannot replace an existing passkey as an MFA bypass.
	weak := &passkeyBrowser{s: s, t: t, cookies: map[string]*http.Cookie{}}
	if _, status := weak.request("POST", "/api/auth/login", Data{"username": str(u, "username"), "password": passkeyTestPassword}); status != 200 {
		t.Fatal(status)
	}
	if out, status := weak.request("POST", "/api/auth/passkeys/register/begin", Data{"name": "MFA bypass", "password": passkeyTestPassword}); status != 403 {
		t.Fatal(out, status)
	}
	if _, e := s.Store.Pool.Exec(ctx, `UPDATE vr_passkeys SET credential=jsonb_set(credential,'{authenticator,signCount}','5') WHERE user_id=$1`, num(u, "id")); e != nil {
		t.Fatal(e)
	}
	options, status := b.request("POST", "/api/auth/passkey/login/begin", Data{})
	if status != 200 {
		t.Fatal(options, status)
	}
	response := key.assertion(str(obj(options, "publicKey"), "challenge"), s.Config.PublicURL, "panel.example", s.passkeyHandle(num(u, "id")), 4, true)
	if out, status := b.request("POST", "/api/auth/passkey/login/finish", response); status != 401 {
		t.Fatal("cloned counter accepted", out, status)
	}
}
func TestIntegrationPasskeysChallengeBindingAndRegistrationUV(t *testing.T) {
	s, ctx, _, b := passkeyFixture(t)
	key := newSoftPasskey(t)
	options, status := b.request("POST", "/api/auth/passkeys/register/begin", Data{"name": "No UV", "password": passkeyTestPassword})
	if status != 200 {
		t.Fatal(options, status)
	}
	if out, status := b.request("POST", "/api/auth/passkeys/register/finish", key.register(str(obj(options, "publicKey"), "challenge"), s.Config.PublicURL, "panel.example", false)); status != 400 {
		t.Fatal("registered key without user verification", out, status)
	}
	options, status = b.request("POST", "/api/auth/passkeys/register/begin", Data{"name": "Expired", "password": passkeyTestPassword})
	if status != 200 {
		t.Fatal(options, status)
	}
	if _, e := s.Store.Pool.Exec(ctx, `UPDATE vr_passkey_challenges SET expires_at=now()-interval '1 second'`); e != nil {
		t.Fatal(e)
	}
	if out, status := b.request("POST", "/api/auth/passkeys/register/finish", key.register(str(obj(options, "publicKey"), "challenge"), s.Config.PublicURL, "panel.example", true)); status != 401 {
		t.Fatal("expired challenge accepted", out, status)
	}
	cfg, _ := settings(ctx, s.Store.Pool)
	cfg["passkeys_enabled"] = false
	if e := saveSettings(ctx, s.Store.Pool, cfg); e != nil {
		t.Fatal(e)
	}
	if out, status := b.request("POST", "/api/auth/passkey/login/begin", Data{}); status != 409 {
		t.Fatal("disabled sign-in allowed", out, status)
	}
	r := httptest.NewRequest("POST", s.Config.PublicURL+"/api/auth/passkey/login/begin", strings.NewReader("{}"))
	r.Header.Set("Authorization", "Bearer fake-csrf-bypass")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("bearer header bypassed authentication CSRF", w.Code, w.Body.String())
	}
}

func TestIntegrationAdministratorPasskeyRecoveryKeepsPasswordAndTOTP(t *testing.T) {
	s, ctx, u, b := passkeyFixture(t)
	key := newSoftPasskey(t)
	enrollSoftPasskey(t, s, b, key)
	before, e := get(ctx, s.Store.Pool, "users", num(u, "id"))
	if e != nil {
		t.Fatal(e)
	}
	before["_totp_enabled"] = true
	before["_totp_secret"], _ = seal(s.Config.FieldKey, "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ")
	if e = save(ctx, s.Store.Pool, "users", before); e != nil {
		t.Fatal(e)
	}
	if e = s.Store.ResetAdministratorPasskeys(ctx, str(u, "username")); e != nil {
		t.Fatal(e)
	}
	after, e := get(ctx, s.Store.Pool, "users", num(u, "id"))
	if e != nil {
		t.Fatal(e)
	}
	if num(after, "_passkey_count") != 0 || num(after, "_auth_version") != num(before, "_auth_version")+1 || str(after, "_password") != str(before, "_password") || flag(after, "_totp_enabled") != flag(before, "_totp_enabled") {
		t.Fatal("recovery changed unrelated factors", after)
	}
	var count int
	if e = s.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM vr_passkeys`).Scan(&count); e != nil || count != 0 {
		t.Fatal(count, e)
	}
	if out, status := b.request("GET", "/api/auth/me", nil); status != 401 {
		t.Fatal("recovery left a browser session active", out, status)
	}
	staff := defaults("users")
	staff["username"] = "ordinary-staff"
	if e = save(ctx, s.Store.Pool, "users", staff); e != nil {
		t.Fatal(e)
	}
	if e = s.Store.ResetAdministratorPasskeys(ctx, "ordinary-staff"); e == nil {
		t.Fatal("administrator recovery accepted ordinary staff")
	}
}
