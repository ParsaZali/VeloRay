package panel

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2"
)

func (s *Server) oidcCookie(w http.ResponseWriter, name, value string, age int) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", Secure: s.Config.SecureCookies, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: age})
}
func validOIDCURL(raw string) bool {
	u, e := url.Parse(raw)
	return e == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.Fragment == "" && u.RawQuery == ""
}
func (s *Server) oidcProvider(ctx context.Context) (context.Context, *oidc.Provider, *oauth2.Config, error) {
	if s.Config.OIDCIssuer == "" || s.Config.OIDCClientID == "" {
		return ctx, nil, nil, fail(404, "SSO is not configured")
	}
	client := s.OIDCHTTP
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	ctx = oidc.ClientContext(ctx, client)
	provider, e := oidc.NewProvider(ctx, s.Config.OIDCIssuer)
	if e != nil {
		return ctx, nil, nil, fail(502, "identity provider discovery unavailable")
	}
	endpoint := provider.Endpoint()
	var discovery struct {
		JWKS string `json:"jwks_uri"`
	}
	if e = provider.Claims(&discovery); e != nil {
		return ctx, nil, nil, e
	}
	for _, raw := range []string{endpoint.AuthURL, endpoint.TokenURL, discovery.JWKS} {
		if !validOIDCURL(raw) {
			return ctx, nil, nil, fail(502, "identity provider endpoints must use HTTPS")
		}
	}
	config := &oauth2.Config{ClientID: s.Config.OIDCClientID, ClientSecret: s.Config.OIDCClientSecret, Endpoint: endpoint, RedirectURL: strings.TrimRight(s.Config.PublicURL, "/") + "/api/auth/oidc/callback", Scopes: []string{oidc.ScopeOpenID}}
	return ctx, provider, config, nil
}
func (s *Server) oidcStart(w http.ResponseWriter, r *http.Request) (any, int, error) {
	if e := rateLimit(r.Context(), s.Store.Pool, "oidc-start:"+clientIP(r), 20, 5*time.Minute); e != nil {
		return nil, 0, e
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	ctx, _, provider, e := s.oidcProvider(ctx)
	if e != nil {
		return nil, 0, e
	}
	state, browser, nonce, verifier := randomToken(32), randomToken(32), randomToken(32), oauth2.GenerateVerifier()
	encrypted, e := seal(s.Config.FieldKey, string(mustJSON(Data{"nonce": nonce, "verifier": verifier})))
	if e != nil {
		return nil, 0, e
	}
	_, e = s.Store.Pool.Exec(ctx, "INSERT INTO vr_oidc_states(state_hash,browser_hash,body,expires_at) VALUES($1,$2,$3,now()+interval '5 minutes')", digest(state), digest(browser), encrypted)
	if e != nil {
		return nil, 0, e
	}
	_, _ = s.Store.Pool.Exec(ctx, "DELETE FROM vr_oidc_states WHERE expires_at<now()")
	s.oidcCookie(w, "veloray_oidc", browser, 300)
	return Data{"authorization_url": provider.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier))}, 200, nil
}
func (s *Server) oidcCallback(w http.ResponseWriter, r *http.Request) {
	failLogin := func() {
		s.oidcCookie(w, "veloray_oidc", "", -1)
		http.Redirect(w, r, "/?oidc=error", http.StatusSeeOther)
	}
	if len(r.URL.Query().Get("state")) > 128 || len(r.URL.Query().Get("code")) > 4096 || r.URL.Query().Get("code") == "" {
		failLogin()
		return
	}
	cookie, e := r.Cookie("veloray_oidc")
	if e != nil {
		failLogin()
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	var encrypted string
	e = s.Store.Pool.QueryRow(ctx, "DELETE FROM vr_oidc_states WHERE state_hash=$1 AND browser_hash=$2 AND expires_at>now() RETURNING body", digest(r.URL.Query().Get("state")), digest(cookie.Value)).Scan(&encrypted)
	if e != nil {
		failLogin()
		return
	}
	s.oidcCookie(w, "veloray_oidc", "", -1)
	raw, e := unseal(s.Config.FieldKey, encrypted)
	if e != nil {
		failLogin()
		return
	}
	state, e := decode([]byte(raw))
	if e != nil {
		failLogin()
		return
	}
	ctx, provider, config, e := s.oidcProvider(ctx)
	if e != nil {
		failLogin()
		return
	}
	token, e := config.Exchange(ctx, r.URL.Query().Get("code"), oauth2.VerifierOption(str(state, "verifier")))
	if e != nil {
		failLogin()
		return
	}
	idTokenRaw, ok := token.Extra("id_token").(string)
	if !ok {
		failLogin()
		return
	}
	idToken, e := provider.VerifierContext(ctx, &oidc.Config{ClientID: s.Config.OIDCClientID}).Verify(ctx, idTokenRaw)
	if e != nil || idToken.IssuedAt.After(time.Now().Add(time.Minute)) || idToken.Subject == "" || len(idToken.Subject) > 512 || subtle.ConstantTimeCompare([]byte(idToken.Nonce), []byte(str(state, "nonce"))) != 1 {
		failLogin()
		return
	}
	var claims struct {
		AuthorizedParty string `json:"azp"`
	}
	if idToken.Claims(&claims) != nil || (len(idToken.Audience) > 1 || claims.AuthorizedParty != "") && claims.AuthorizedParty != s.Config.OIDCClientID {
		failLogin()
		return
	}
	s.finishExternalLogin(w, r, ctx, s.Config.OIDCIssuer, idToken.Subject)
}

func (s *Server) finishExternalLogin(w http.ResponseWriter, r *http.Request, ctx context.Context, issuer, subject string) {
	failLogin := func() { http.Redirect(w, r, "/?oidc=error", http.StatusSeeOther) }
	tx, e := s.Store.Pool.Begin(ctx)
	if e != nil {
		failLogin()
		return
	}
	defer tx.Rollback(ctx)
	var data []byte
	e = tx.QueryRow(ctx, "SELECT data||jsonb_build_object('id',id) FROM vr_users WHERE data->>'oidc_issuer'=$1 AND data->>'oidc_subject'=$2 FOR UPDATE", issuer, subject).Scan(&data)
	user, _ := decode(data)
	if e != nil || !flag(user, "is_active") || !flag(user, "is_staff") {
		failLogin()
		return
	}
	if flag(user, "_totp_enabled") {
		ticket := randomToken(32)
		_, e = tx.Exec(ctx, "INSERT INTO vr_oidc_challenges(token_hash,user_id,auth_version,expires_at) VALUES($1,$2,$3,now()+interval '2 minutes')", digest(ticket), num(user, "id"), num(user, "_auth_version"))
		if e != nil || tx.Commit(ctx) != nil {
			failLogin()
			return
		}
		s.oidcCookie(w, "veloray_oidc_challenge", ticket, 120)
		http.Redirect(w, r, "/?oidc=totp", http.StatusSeeOther)
		return
	}
	session, e := s.createOIDCSession(ctx, tx, user, false)
	if e != nil || tx.Commit(ctx) != nil {
		failLogin()
		return
	}
	s.cookie(w, "veloray_session", session, int(s.Config.SessionAge.Seconds()), true)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
func (s *Server) createOIDCSession(ctx context.Context, tx pgx.Tx, user Data, second bool) (string, error) {
	token := randomToken(32)
	user["last_login"] = stamp()
	if e := save(ctx, tx, "users", user); e != nil {
		return "", e
	}
	if _, e := tx.Exec(ctx, "INSERT INTO vr_sessions(token_hash,user_id,auth_version,second_factor,expires_at) VALUES($1,$2,$3,$4,$5)", digest(token), num(user, "id"), num(user, "_auth_version"), second, time.Now().Add(s.Config.SessionAge)); e != nil {
		return "", e
	}
	if e := audit(ctx, tx, num(user, "id"), "auth.oidc-login", str(user, "username"), Data{}); e != nil {
		return "", e
	}
	return token, nil
}
func (s *Server) oidcComplete(w http.ResponseWriter, r *http.Request) (any, int, error) {
	if e := rateLimit(r.Context(), s.Store.Pool, "oidc-complete:"+clientIP(r), 10, 5*time.Minute); e != nil {
		return nil, 0, e
	}
	input, e := readBody(w, r)
	if e != nil {
		return nil, 0, e
	}
	if len(str(input, "totp_code")) > 128 {
		return nil, 0, fail(400, "invalid authentication code")
	}
	cookie, e := r.Cookie("veloray_oidc_challenge")
	if e != nil {
		return nil, 0, fail(401, "restart SSO sign-in")
	}
	ctx := r.Context()
	tx, e := s.Store.Pool.Begin(ctx)
	if e != nil {
		return nil, 0, e
	}
	defer tx.Rollback(ctx)
	var userID, version int64
	e = tx.QueryRow(ctx, "SELECT user_id,auth_version FROM vr_oidc_challenges WHERE token_hash=$1 AND expires_at>now() FOR UPDATE", digest(cookie.Value)).Scan(&userID, &version)
	if e != nil {
		return nil, 0, fail(401, "restart SSO sign-in")
	}
	var data []byte
	e = tx.QueryRow(ctx, "SELECT data||jsonb_build_object('id',id) FROM vr_users WHERE id=$1 FOR UPDATE", userID).Scan(&data)
	user, _ := decode(data)
	if e != nil || num(user, "_auth_version") != version || !flag(user, "is_active") || !flag(user, "is_staff") || !flag(user, "_totp_enabled") || !verifySecond(s.Config, user, str(input, "totp_code")) {
		return nil, 0, fail(401, "invalid or expired authentication code")
	}
	if _, e = tx.Exec(ctx, "DELETE FROM vr_oidc_challenges WHERE token_hash=$1", digest(cookie.Value)); e != nil {
		return nil, 0, e
	}
	token, e := s.createOIDCSession(ctx, tx, user, true)
	if e != nil {
		return nil, 0, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, 0, e
	}
	s.oidcCookie(w, "veloray_oidc_challenge", "", -1)
	s.cookie(w, "veloray_session", token, int(s.Config.SessionAge.Seconds()), true)
	return s.sessionView(ctx, Actor{User: user, SessionHash: digest(token), SecondFactor: true}), 200, nil
}
func validateOIDCBinding(d Data) error {
	issuer, sub := str(d, "oidc_issuer"), str(d, "oidc_subject")
	if issuer == "" && sub == "" {
		return nil
	}
	if !validOIDCURL(issuer) || sub == "" || len(sub) > 512 {
		return errors.New("OIDC requires an HTTPS issuer and a subject of at most 512 bytes")
	}
	return nil
}
