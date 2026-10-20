package panel

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base32"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Actor struct {
	User         Data
	SessionHash  string
	Scope        string
	SecondFactor bool
}

func (s *Server) authenticate(r *http.Request) (Actor, error) {
	ctx := r.Context()
	settings, e := settings(ctx, s.Store.Pool)
	if e != nil {
		return Actor{}, e
	}
	var a Actor
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		if !flag(settings, "api_keys_enabled") {
			return a, fail(401, "API keys are disabled")
		}
		keys, e := list(ctx, s.Store.Pool, "api_keys", "WHERE data->>'_token_hash'=$1", digest(strings.TrimPrefix(auth, "Bearer ")))
		if e != nil {
			return a, e
		}
		if len(keys) != 1 || !flag(keys[0], "enabled") {
			return a, fail(401, "invalid API key")
		}
		k := keys[0]
		if exp, e := timestamp(k, "expires_at"); e == nil && time.Now().After(exp) {
			return a, fail(401, "API key expired")
		}
		a.User, e = get(ctx, s.Store.Pool, "users", num(k, "user"))
		if e != nil {
			return a, fail(401, "invalid API key owner")
		}
		a.Scope = str(k, "scope")
		a.SecondFactor = flag(a.User, "_totp_enabled")
		_, _ = s.Store.Pool.Exec(ctx, `UPDATE vr_api_keys SET data=jsonb_set(data,'{last_used_at}',to_jsonb($1::text)) WHERE id=$2`, stamp(), num(k, "id"))
	} else {
		cookie, e := r.Cookie("veloray_session")
		if e != nil {
			return a, fail(401, "sign in required")
		}
		a.SessionHash = digest(cookie.Value)
		var uid, version int64
		e = s.Store.Pool.QueryRow(ctx, `SELECT user_id,auth_version,second_factor FROM vr_sessions WHERE token_hash=$1 AND expires_at>now()`, a.SessionHash).Scan(&uid, &version, &a.SecondFactor)
		if e != nil {
			return a, fail(401, "session expired")
		}
		a.User, e = get(ctx, s.Store.Pool, "users", uid)
		if e != nil || num(a.User, "_auth_version") != version {
			return a, fail(401, "session expired")
		}
		a.Scope = "admin"
	}
	if !flag(a.User, "is_active") || !flag(a.User, "is_staff") {
		return a, fail(403, "staff access required")
	}
	return a, nil
}
func (s *Server) authorize(ctx context.Context, a Actor, method, path string) error {
	cfg, e := settings(ctx, s.Store.Pool)
	if e != nil {
		return e
	}
	if flag(cfg, "require_2fa_for_admins") && (!flag(a.User, "_totp_enabled") || !a.SecondFactor) {
		if a.SessionHash == "" || !(strings.HasPrefix(path, "auth/2fa/") || path == "auth/me" || path == "auth/logout") {
			return failCode(403, "two_factor_setup_required", "Set up two-factor authentication to continue")
		}
	}
	if strings.HasPrefix(path, "admin/") || strings.HasPrefix(path, "imports/") || strings.HasPrefix(path, "telegram/") || path == "settings" && method != "GET" {
		if !flag(a.User, "is_superuser") || a.Scope != "admin" {
			return fail(403, "administrator access required")
		}
	}
	if method != "GET" && a.Scope == "read" {
		return fail(403, "API key is read-only")
	}
	if a.SessionHash == "" && strings.HasPrefix(path, "auth/2fa/") {
		return fail(403, "use a browser session to manage two-factor authentication")
	}
	return s.roleAuthorize(a, method, path)
}
func (s *Server) sessionView(ctx context.Context, a Actor) Data {
	cfg, _ := settings(ctx, s.Store.Pool)
	return Data{"role": accountRole(a.User), "permissions": permissions(a.User), "own_clients_only": ownClients(a.User), "id": num(a.User, "id"), "username": str(a.User, "username"), "is_staff": flag(a.User, "is_staff"), "is_superuser": flag(a.User, "is_superuser"), "two_factor_setup_required": flag(cfg, "require_2fa_for_admins") && (!flag(a.User, "_totp_enabled") || !a.SecondFactor)}
}
func (s *Server) cookie(w http.ResponseWriter, name, value string, maxAge int, httpOnly bool) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", Secure: s.Config.SecureCookies, HttpOnly: httpOnly, SameSite: http.SameSiteStrictMode, MaxAge: maxAge})
}
func (s *Server) checkCSRF(r *http.Request) error {
	if r.Method == "GET" || r.Method == "HEAD" || r.Method == "OPTIONS" {
		return nil
	}
	if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") && r.URL.Path != "/api/auth/login" {
		return nil
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		if ref, e := url.Parse(r.Header.Get("Referer")); e == nil && ref.Host != "" {
			origin = ref.Scheme + "://" + ref.Host
		}
	}
	if !s.Config.TrustedOrigins[origin] || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return fail(403, "request origin is not trusted")
	}
	c, e := r.Cookie("csrftoken")
	token := r.Header.Get("X-CSRFToken")
	if e != nil || len(c.Value) != len(token) || subtle.ConstantTimeCompare([]byte(c.Value), []byte(token)) != 1 || !validCSRF(s.Config.SecretKey, token) {
		return failCode(403, "csrf_failed", "Refresh the page and try again")
	}
	return nil
}
func clientIP(r *http.Request) string {
	host, _, e := net.SplitHostPort(r.RemoteAddr)
	if e == nil {
		return host
	}
	return r.RemoteAddr
}
func rateLimit(ctx context.Context, q Query, key string, max int, period time.Duration) error {
	var count int
	seconds := int64(period.Seconds())
	e := q.QueryRow(ctx, `INSERT INTO vr_rate_limits(key,start_at,hits) VALUES($1,now(),1) ON CONFLICT(key) DO UPDATE SET hits=CASE WHEN vr_rate_limits.start_at<now()-$2*interval '1 second' THEN 1 ELSE vr_rate_limits.hits+1 END,start_at=CASE WHEN vr_rate_limits.start_at<now()-$2*interval '1 second' THEN now() ELSE vr_rate_limits.start_at END RETURNING hits`, key, seconds).Scan(&count)
	if e != nil {
		return e
	}
	if count > max {
		return fail(429, "Too many attempts. Try again later")
	}
	return nil
}
func verifySecond(c Config, user Data, code string) bool {
	secret, e := unseal(c.FieldKey, str(user, "_totp_secret"))
	if e == nil {
		if step, ok := verifyTOTP(secret, code, time.Now(), num(user, "_totp_step")); ok {
			user["_totp_step"] = step
			return true
		}
	}
	codes := array(user, "_recovery")
	for i, v := range codes {
		stored, ok := v.(string)
		if !ok {
			continue
		}
		got := digest(strings.ToUpper(strings.TrimSpace(code)))
		if len(got) == len(stored) && subtle.ConstantTimeCompare([]byte(got), []byte(stored)) == 1 {
			user["_recovery"] = append(codes[:i:i], codes[i+1:]...)
			return true
		}
	}
	return false
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) (any, int, error) {
	input, e := readBody(w, r)
	if e != nil {
		return nil, 0, e
	}
	name, password := str(input, "username"), str(input, "password")
	if len(name) > 150 || len(password) > 1024 {
		return nil, 0, fail(400, "invalid credentials")
	}
	if e = rateLimit(r.Context(), s.Store.Pool, "login-ip:"+clientIP(r), 30, 5*time.Minute); e != nil {
		return nil, 0, e
	}
	if e = rateLimit(r.Context(), s.Store.Pool, "login-user:"+digest(name), 10, 5*time.Minute); e != nil {
		return nil, 0, e
	}
	tx, e := s.Store.Pool.Begin(r.Context())
	if e != nil {
		return nil, 0, e
	}
	defer tx.Rollback(r.Context())
	var raw []byte
	e = tx.QueryRow(r.Context(), `SELECT data||jsonb_build_object('id',id) FROM vr_users WHERE data->>'username'=$1 FOR UPDATE`, name).Scan(&raw)
	user, _ := decode(raw)
	if e != nil || !checkPassword(str(user, "_password"), password) || !flag(user, "is_active") || !flag(user, "is_staff") {
		return nil, 0, fail(401, "Username or password is incorrect")
	}
	if flag(user, "_totp_enabled") {
		if str(input, "totp_code") == "" {
			return Data{"two_factor_required": true}, 202, nil
		}
		if !verifySecond(s.Config, user, str(input, "totp_code")) {
			return nil, 0, fail(401, "Authentication code is invalid or already used")
		}
	}
	if strings.HasPrefix(str(user, "_password"), "pbkdf2_sha256$") {
		user["_password"], e = hashPassword(password)
		if e != nil {
			return nil, 0, e
		}
	}
	user["last_login"] = stamp()
	if e = save(r.Context(), tx, "users", user); e != nil {
		return nil, 0, e
	}
	token := randomToken(32)
	hashed := digest(token)
	_, e = tx.Exec(r.Context(), `INSERT INTO vr_sessions(token_hash,user_id,auth_version,second_factor,expires_at) VALUES($1,$2,$3,$4,$5)`, hashed, num(user, "id"), num(user, "_auth_version"), flag(user, "_totp_enabled"), time.Now().Add(s.Config.SessionAge))
	if e != nil {
		return nil, 0, e
	}
	if e = audit(r.Context(), tx, num(user, "id"), "auth.login", name, Data{"ip": clientIP(r)}); e != nil {
		return nil, 0, e
	}
	if e = tx.Commit(r.Context()); e != nil {
		return nil, 0, e
	}
	s.cookie(w, "veloray_session", token, int(s.Config.SessionAge.Seconds()), true)
	return s.sessionView(r.Context(), Actor{User: user, SessionHash: hashed, SecondFactor: flag(user, "_totp_enabled")}), 200, nil
}
func (s *Server) authAPI(w http.ResponseWriter, r *http.Request, path string, a Actor) (any, int, error) {
	ctx := r.Context()
	if path == "auth/me" && r.Method == "GET" {
		return s.sessionView(ctx, a), 200, nil
	}
	if path == "auth/logout" && r.Method == "POST" {
		_, e := s.Store.Pool.Exec(ctx, `DELETE FROM vr_sessions WHERE token_hash=$1`, a.SessionHash)
		s.cookie(w, "veloray_session", "", -1, true)
		return nil, 204, e
	}
	if path == "auth/2fa/status" && r.Method == "GET" {
		return Data{"enabled": flag(a.User, "_totp_enabled"), "recovery_codes_remaining": len(array(a.User, "_recovery"))}, 200, nil
	}
	if r.Method != "POST" {
		return nil, 0, fail(405, "method not allowed")
	}
	input, e := readBody(w, r)
	if e != nil {
		return nil, 0, e
	}
	if e = rateLimit(ctx, s.Store.Pool, fmt.Sprintf("2fa:%d", num(a.User, "id")), 12, 5*time.Minute); e != nil {
		return nil, 0, e
	}
	tx, e := s.Store.Pool.Begin(ctx)
	if e != nil {
		return nil, 0, e
	}
	defer tx.Rollback(ctx)
	var raw []byte
	e = tx.QueryRow(ctx, `SELECT data||jsonb_build_object('id',id) FROM vr_users WHERE id=$1 FOR UPDATE`, num(a.User, "id")).Scan(&raw)
	if e != nil {
		return nil, 0, e
	}
	u, e := decode(raw)
	if e != nil {
		return nil, 0, e
	}
	var out Data
	switch path {
	case "auth/2fa/setup":
		if flag(u, "_totp_enabled") {
			return nil, 0, fail(409, "Two-factor authentication is already enabled")
		}
		b := make([]byte, 20)
		if _, e = rand.Read(b); e != nil {
			return nil, 0, e
		}
		secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
		u["_totp_pending"], e = seal(s.Config.FieldKey, secret)
		u["_totp_pending_at"] = stamp()
		if e != nil {
			return nil, 0, e
		}
		out = Data{"secret": secret, "otpauth_uri": "otpauth://totp/" + url.PathEscape("VeloRay:"+str(u, "username")) + "?secret=" + secret + "&issuer=VeloRay&digits=6&period=30"}
	case "auth/2fa/confirm":
		secret, e := unseal(s.Config.FieldKey, str(u, "_totp_pending"))
		at, te := timestamp(u, "_totp_pending_at")
		if flag(u, "_totp_enabled") || e != nil || te != nil || time.Since(at) > 10*time.Minute {
			return nil, 0, fail(400, "Start two-factor setup again")
		}
		step, ok := verifyTOTP(secret, str(input, "code"), time.Now(), -1)
		if !ok {
			return nil, 0, fail(400, "Authentication code is invalid")
		}
		u["_totp_secret"] = u["_totp_pending"]
		delete(u, "_totp_pending")
		delete(u, "_totp_pending_at")
		u["_totp_enabled"] = true
		u["_totp_step"] = step
		codes := []any{}
		hashes := []any{}
		for i := 0; i < 10; i++ {
			code := strings.ToUpper(randomToken(9))
			codes = append(codes, code)
			hashes = append(hashes, digest(code))
		}
		u["_recovery"] = hashes
		out = Data{"enabled": true, "recovery_codes": codes}
		u["_auth_version"] = num(u, "_auth_version") + 1
		_, e = tx.Exec(ctx, `DELETE FROM vr_sessions WHERE user_id=$1 AND token_hash<>$2`, num(u, "id"), a.SessionHash)
		if e != nil {
			return nil, 0, e
		}
		_, e = tx.Exec(ctx, `UPDATE vr_sessions SET auth_version=$1,second_factor=true WHERE token_hash=$2`, num(u, "_auth_version"), a.SessionHash)
		if e != nil {
			return nil, 0, e
		}
	case "auth/2fa/disable":
		cfg, e := settings(ctx, tx)
		if e != nil {
			return nil, 0, e
		}
		if flag(cfg, "require_2fa_for_admins") {
			return nil, 0, fail(403, "Two-factor authentication is required by panel settings")
		}
		if !checkPassword(str(u, "_password"), str(input, "password")) || !verifySecond(s.Config, u, str(input, "code")) {
			return nil, 0, fail(400, "Password or authentication code is invalid")
		}
		u["_totp_enabled"] = false
		u["_totp_secret"] = ""
		u["_recovery"] = []any{}
		u["_auth_version"] = num(u, "_auth_version") + 1
		_, e = tx.Exec(ctx, `DELETE FROM vr_sessions WHERE user_id=$1 AND token_hash<>$2`, num(u, "id"), a.SessionHash)
		if e != nil {
			return nil, 0, e
		}
		_, e = tx.Exec(ctx, `UPDATE vr_sessions SET auth_version=$1,second_factor=false WHERE token_hash=$2`, num(u, "_auth_version"), a.SessionHash)
		if e != nil {
			return nil, 0, e
		}
		out = Data{"enabled": false}
	default:
		return nil, 0, fail(404, "endpoint not found")
	}
	if e = save(ctx, tx, "users", u); e != nil {
		return nil, 0, e
	}
	if e = audit(ctx, tx, num(u, "id"), path, str(u, "username"), Data{}); e != nil {
		return nil, 0, e
	}
	return out, 200, tx.Commit(ctx)
}
func (s *Store) Bootstrap(ctx context.Context, c Config, name, password string) error {
	return s.bootstrap(ctx, c, name, password, false)
}
func (s *Store) BootstrapIfEmpty(ctx context.Context, c Config, name, password string) error {
	return s.bootstrap(ctx, c, name, password, true)
}
func (s *Store) bootstrap(ctx context.Context, c Config, name, password string, onlyEmpty bool) error {
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	_, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(842100,1)`)
	if e != nil {
		return e
	}
	if onlyEmpty {
		var exists bool
		if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM vr_users)").Scan(&exists); e != nil {
			return e
		}
		if exists {
			return nil
		}
	}
	if e := passwordPolicy(name, password); e != nil {
		return e
	}
	users, e := list(ctx, tx, "users", "WHERE data->>'username'=$1", name)
	if e != nil {
		return e
	}
	if len(users) > 0 {
		return errors.New("user already exists; use admin-reset")
	}
	hash, e := hashPassword(password)
	if e != nil {
		return e
	}
	u := defaults("users")
	u["username"] = name
	u["_password"] = hash
	u["is_superuser"] = true
	if e = save(ctx, tx, "users", u); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (s *Store) AdminReset(ctx context.Context, name, password string) error {
	if e := passwordPolicy(name, password); e != nil {
		return e
	}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var raw []byte
	e = tx.QueryRow(ctx, `SELECT data||jsonb_build_object('id',id) FROM vr_users WHERE data->>'username'=$1 FOR UPDATE`, name).Scan(&raw)
	if errors.Is(e, pgx.ErrNoRows) {
		return errors.New("administrator not found")
	}
	if e != nil {
		return e
	}
	u, e := decode(raw)
	if e != nil {
		return e
	}
	u["_password"], e = hashPassword(password)
	if e != nil {
		return e
	}
	u["_auth_version"] = num(u, "_auth_version") + 1
	if e = save(ctx, tx, "users", u); e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `DELETE FROM vr_sessions WHERE user_id=$1`, num(u, "id"))
	if e != nil {
		return e
	}
	if e = audit(ctx, tx, num(u, "id"), "admin.password_reset", name, Data{"source": "CLI"}); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
