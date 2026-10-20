package panel

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

const githubIdentityIssuer = "https://github.com"

func (s *Server) githubOAuth() *oauth2.Config {
	return &oauth2.Config{ClientID: s.Config.GitHubClientID, ClientSecret: s.Config.GitHubClientSecret, Endpoint: oauth2.Endpoint{AuthURL: "https://github.com/login/oauth/authorize", TokenURL: "https://github.com/login/oauth/access_token", AuthStyle: oauth2.AuthStyleInParams}, RedirectURL: strings.TrimRight(s.Config.PublicURL, "/") + "/api/auth/github/callback"}
}
func (s *Server) githubEnabled() bool {
	return s.Config.GitHubClientID != "" && s.Config.GitHubClientSecret != ""
}
func (s *Server) githubStart(w http.ResponseWriter, r *http.Request) (any, int, error) {
	if !s.githubEnabled() {
		return nil, 0, fail(404, "GitHub sign-in is not configured")
	}
	if err := rateLimit(r.Context(), s.Store.Pool, "github-start:"+clientIP(r), 20, 5*time.Minute); err != nil {
		return nil, 0, err
	}
	state, browser, verifier := randomToken(32), randomToken(32), oauth2.GenerateVerifier()
	encrypted, err := seal(s.Config.FieldKey, string(mustJSON(Data{"provider": "github", "verifier": verifier})))
	if err != nil {
		return nil, 0, err
	}
	_, err = s.Store.Pool.Exec(r.Context(), "INSERT INTO vr_oidc_states(state_hash,browser_hash,body,expires_at) VALUES($1,$2,$3,now()+interval '5 minutes')", digest(state), digest(browser), encrypted)
	if err != nil {
		return nil, 0, err
	}
	_, _ = s.Store.Pool.Exec(r.Context(), "DELETE FROM vr_oidc_states WHERE expires_at<now()")
	s.oidcCookie(w, "veloray_github", browser, 300)
	return Data{"authorization_url": s.githubOAuth().AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oauth2.SetAuthURLParam("allow_signup", "false"), oauth2.SetAuthURLParam("scope", ""))}, 200, nil
}
func (s *Server) githubCallback(w http.ResponseWriter, r *http.Request) {
	failure := func() {
		s.oidcCookie(w, "veloray_github", "", -1)
		http.Redirect(w, r, "/?oidc=error", http.StatusSeeOther)
	}
	state, code := r.URL.Query().Get("state"), r.URL.Query().Get("code")
	cookie, err := r.Cookie("veloray_github")
	if !s.githubEnabled() || err != nil || len(cookie.Value) > 128 || state == "" || len(state) > 128 || code == "" || len(code) > 4096 || r.URL.Query().Get("error") != "" {
		failure()
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	var encrypted string
	err = s.Store.Pool.QueryRow(ctx, "DELETE FROM vr_oidc_states WHERE state_hash=$1 AND browser_hash=$2 AND expires_at>now() RETURNING body", digest(state), digest(cookie.Value)).Scan(&encrypted)
	if err != nil {
		failure()
		return
	}
	s.oidcCookie(w, "veloray_github", "", -1)
	raw, err := unseal(s.Config.FieldKey, encrypted)
	if err != nil {
		failure()
		return
	}
	data, err := decode([]byte(raw))
	if err != nil || str(data, "provider") != "github" || len(str(data, "verifier")) < 43 {
		failure()
		return
	}
	client := s.GitHubHTTP
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	// Also reject redirects on injected clients: an OAuth token must never be
	// forwarded to a different host, including a provider's error redirect.
	bounded := *client
	bounded.Timeout = 8 * time.Second
	bounded.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	ctx = context.WithValue(ctx, oauth2.HTTPClient, &bounded)
	token, err := s.githubOAuth().Exchange(ctx, code, oauth2.VerifierOption(str(data, "verifier")))
	if err != nil || token.AccessToken == "" || len(token.AccessToken) > 4096 || strings.ContainsAny(token.AccessToken, "\r\n\x00") {
		failure()
		return
	}
	subject, err := githubSubject(ctx, &bounded, token.AccessToken)
	if err != nil {
		failure()
		return
	}
	s.finishExternalLogin(w, r, ctx, githubIdentityIssuer, subject)
}
func githubSubject(ctx context.Context, client *http.Client, token string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/user", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	req.Header.Set("User-Agent", "VeloRay/1.0")
	response, err := client.Do(req)
	if err != nil {
		return "", errors.New("identity lookup unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return "", errors.New("identity lookup unavailable")
	}
	var identity struct {
		ID json.Number `json:"id"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 65537))
	if decoder.Decode(&identity) != nil {
		return "", errors.New("invalid identity")
	}
	id, err := strconv.ParseInt(identity.ID.String(), 10, 64)
	if err != nil || id <= 0 {
		return "", errors.New("invalid identity")
	}
	return strconv.FormatInt(id, 10), nil
}
