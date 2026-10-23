package panel

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5"
	"golang.org/x/net/idna"
)

type passkeyUser struct {
	data        Data
	handle      []byte
	credentials []webauthn.Credential
}

func (u *passkeyUser) WebAuthnID() []byte                         { return u.handle }
func (u *passkeyUser) WebAuthnName() string                       { return str(u.data, "username") }
func (u *passkeyUser) WebAuthnDisplayName() string                { return str(u.data, "username") }
func (u *passkeyUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }
func (s *Server) passkeyHandle(id int64) []byte {
	mac := hmac.New(sha256.New, []byte(s.Config.SecretKey))
	mac.Write([]byte(fmt.Sprintf("veloray-passkey-user\x00%d", id)))
	return mac.Sum(nil)
}
func (s *Server) webAuthn() (*webauthn.WebAuthn, error) {
	u, e := url.Parse(s.Config.PublicURL)
	if e != nil || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return nil, fail(409, "passkeys require a trusted HTTPS domain")
	}
	host, e := idna.Lookup.ToASCII(strings.ToLower(u.Hostname()))
	if e != nil || net.ParseIP(host) != nil || !validHost(host) || u.Scheme != "https" && !(u.Scheme == "http" && host == "localhost") {
		return nil, fail(409, "passkeys require a trusted HTTPS domain")
	}
	timeout := webauthn.TimeoutConfig{Enforce: true, Timeout: 2 * time.Minute, TimeoutUVD: 2 * time.Minute}
	return webauthn.New(&webauthn.Config{RPID: host, RPDisplayName: "VeloRay", RPOrigins: []string{s.Config.PublicURL}, AttestationPreference: protocol.PreferNoAttestation, AuthenticatorSelection: protocol.AuthenticatorSelection{ResidentKey: protocol.ResidentKeyRequirementRequired, UserVerification: protocol.VerificationRequired}, Timeouts: webauthn.TimeoutsConfig{Login: timeout, Registration: timeout}})
}
func (s *Server) passkeyRuntime(ctx context.Context) (*webauthn.WebAuthn, error) {
	cfg, e := settings(ctx, s.Store.Pool)
	if e != nil {
		return nil, e
	}
	if !flag(cfg, "passkeys_enabled") {
		return nil, fail(409, "passkey sign-in is disabled")
	}
	return s.webAuthn()
}
func (s *Server) loadPasskeyUser(ctx context.Context, q Query, u Data) (*passkeyUser, error) {
	rows, e := q.Query(ctx, `SELECT credential FROM vr_passkeys WHERE user_id=$1 ORDER BY created_at`, num(u, "id"))
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	user := &passkeyUser{data: u, handle: s.passkeyHandle(num(u, "id")), credentials: []webauthn.Credential{}}
	for rows.Next() {
		var raw []byte
		var c webauthn.Credential
		if e = rows.Scan(&raw); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(raw, &c); e != nil {
			return nil, e
		}
		user.credentials = append(user.credentials, c)
	}
	return user, rows.Err()
}
func lockedPasskeyAccount(ctx context.Context, tx pgx.Tx, id int64) (Data, error) {
	var raw []byte
	if e := tx.QueryRow(ctx, `SELECT data||jsonb_build_object('id',id) FROM vr_users WHERE id=$1 FOR UPDATE`, id).Scan(&raw); e != nil {
		return nil, e
	}
	u, e := decode(raw)
	if e != nil {
		return nil, e
	}
	if !flag(u, "is_staff") || !flag(u, "is_active") {
		return nil, fail(401, "passkey sign-in failed")
	}
	if e = resellerAncestorsActive(ctx, tx, u); e != nil {
		return nil, fail(401, "passkey sign-in failed")
	}
	return u, nil
}
func passkeyManagementProof(c Config, u Data, input Data, a Actor) error {
	if !flag(u, "_totp_enabled") && num(u, "_passkey_count") > 0 && !a.SecondFactor {
		return fail(403, "sign in with an existing passkey before managing passkeys")
	}
	if !checkPassword(str(u, "_password"), str(input, "password")) {
		return fail(401, "confirm your local password to manage passkeys")
	}
	if flag(u, "_totp_enabled") && !verifySecond(c, u, str(input, "code")) {
		return fail(401, "Authentication code is invalid or already used")
	}
	return nil
}
func (s *Server) savePasskeyChallenge(ctx context.Context, q Query, purpose string, u Data, a Actor, label string, data *webauthn.SessionData) (string, error) {
	token := randomToken(32)
	raw, e := json.Marshal(data)
	if e != nil {
		return "", e
	}
	var uid any
	if num(u, "id") > 0 {
		uid = num(u, "id")
	}
	_, e = q.Exec(ctx, `INSERT INTO vr_passkey_challenges(token_hash,purpose,user_id,auth_version,session_hash,label,data) VALUES($1,$2,$3,$4,$5,$6,$7)`, digest(token), purpose, uid, num(u, "_auth_version"), a.SessionHash, label, string(raw))
	return token, e
}

type passkeyChallenge struct {
	session       webauthn.SessionData
	user, version int64
	label         string
}

func (s *Server) takePasskeyChallenge(w http.ResponseWriter, r *http.Request, purpose string, a Actor) (passkeyChallenge, error) {
	c, e := r.Cookie("veloray_passkey_" + purpose)
	s.cookie(w, "veloray_passkey_"+purpose, "", -1, true)
	invalid := func() (passkeyChallenge, error) {
		return passkeyChallenge{}, fail(401, "passkey request expired; try again")
	}
	if e != nil || len(c.Value) < 32 {
		return invalid()
	}
	var raw []byte
	out := passkeyChallenge{}
	e = s.Store.Pool.QueryRow(r.Context(), `DELETE FROM vr_passkey_challenges WHERE token_hash=$1 AND purpose=$2 AND expires_at>now() AND ($2='login' OR user_id=$3 AND session_hash=$4) RETURNING data,COALESCE(user_id,0),auth_version,label`, digest(c.Value), purpose, num(a.User, "id"), a.SessionHash).Scan(&raw, &out.user, &out.version, &out.label)
	if e != nil {
		return invalid()
	}
	if e = json.Unmarshal(raw, &out.session); e != nil {
		return invalid()
	}
	return out, nil
}
func (s *Server) passkeyPublic(w http.ResponseWriter, r *http.Request, path string) (any, int, error) {
	ctx := r.Context()
	wa, e := s.passkeyRuntime(ctx)
	if path == "auth/passkey/status" && r.Method == "GET" {
		if e != nil {
			return Data{"enabled": false}, 200, nil
		}
		return Data{"enabled": true, "rp_id": wa.Config.RPID}, 200, nil
	}
	if e != nil {
		return nil, 0, e
	}
	if r.Method != "POST" {
		return nil, 0, fail(405, "method not allowed")
	}
	if e = rateLimit(ctx, s.Store.Pool, "passkey-login:"+clientIP(r), 30, 5*time.Minute); e != nil {
		return nil, 0, e
	}
	if path == "auth/passkey/login/begin" {
		input, e := readBody(w, r)
		if e != nil {
			return nil, 0, e
		}
		if e = allowed(input, ""); e != nil {
			return nil, 0, fail(400, e.Error())
		}
		options, session, e := wa.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired), webauthn.WithLoginOrigin(s.Config.PublicURL))
		if e != nil {
			return nil, 0, e
		}
		token, e := s.savePasskeyChallenge(ctx, s.Store.Pool, "login", nil, Actor{}, "", session)
		if e != nil {
			return nil, 0, e
		}
		s.cookie(w, "veloray_passkey_login", token, 120, true)
		return options, 200, nil
	}
	if path != "auth/passkey/login/finish" {
		return nil, 0, fail(404, "endpoint not found")
	}
	challenge, e := s.takePasskeyChallenge(w, r, "login", Actor{})
	if e != nil {
		return nil, 0, e
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	tx, e := s.Store.Pool.Begin(ctx)
	if e != nil {
		return nil, 0, e
	}
	defer tx.Rollback(ctx)
	var u Data
	credential, e := wa.FinishDiscoverableLogin(func(rawID, handle []byte) (webauthn.User, error) {
		var id int64
		if err := tx.QueryRow(ctx, `SELECT user_id FROM vr_passkeys WHERE id=$1`, rawID).Scan(&id); err != nil {
			return nil, err
		}
		var err error
		u, err = lockedPasskeyAccount(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(s.passkeyHandle(id), handle) {
			return nil, fmt.Errorf("passkey handle mismatch")
		}
		return s.loadPasskeyUser(ctx, tx, u)
	}, challenge.session, r)
	if e != nil || credential == nil || !credential.Flags.UserVerified || credential.Authenticator.CloneWarning && !credential.Flags.BackupEligible {
		return nil, 0, fail(401, "passkey sign-in failed")
	}
	raw, _ := json.Marshal(credential)
	if _, e = tx.Exec(ctx, `UPDATE vr_passkeys SET credential=$2,last_used_at=now() WHERE id=$1 AND user_id=$3`, credential.ID, string(raw), num(u, "id")); e != nil {
		return nil, 0, e
	}
	u["last_login"] = stamp()
	if e = save(ctx, tx, "users", u); e != nil {
		return nil, 0, e
	}
	token := randomToken(32)
	hashed := digest(token)
	if _, e = tx.Exec(ctx, `INSERT INTO vr_sessions(token_hash,user_id,auth_version,second_factor,expires_at) VALUES($1,$2,$3,true,$4)`, hashed, num(u, "id"), num(u, "_auth_version"), time.Now().Add(s.Config.SessionAge)); e != nil {
		return nil, 0, e
	}
	if e = audit(ctx, tx, num(u, "id"), "auth.passkey_login", str(u, "username"), Data{"ip": clientIP(r), "backup_eligible": credential.Flags.BackupEligible, "counter_warning": credential.Authenticator.CloneWarning}); e != nil {
		return nil, 0, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, 0, e
	}
	s.cookie(w, "veloray_session", token, int(s.Config.SessionAge.Seconds()), true)
	return s.sessionView(ctx, Actor{User: u, SessionHash: hashed, Scope: "admin", SecondFactor: true}), 200, nil
}

func (s *Server) passkeysAPI(w http.ResponseWriter, r *http.Request, path string, a Actor) (any, int, error) {
	if a.SessionHash == "" || a.Scope != "admin" {
		return nil, 0, fail(403, "use a browser session to manage passkeys")
	}
	ctx := r.Context()
	if path == "auth/passkeys" && r.Method == "GET" {
		all, e := commerceRows(ctx, s.Store.Pool, `SELECT jsonb_build_object('id',translate(rtrim(encode(id,'base64'),'='),E'+/\n','-_'),'name',label,'created_at',created_at,'last_used_at',last_used_at) FROM vr_passkeys WHERE user_id=$1 ORDER BY created_at`, num(a.User, "id"))
		if e != nil {
			return nil, 0, e
		}
		_, reason := s.passkeyRuntime(ctx)
		return Data{"enabled": reason == nil, "credentials": all}, 200, nil
	}
	wa, e := s.passkeyRuntime(ctx)
	// Revocation remains available while new passkey sign-in is disabled.
	deleting := strings.HasPrefix(path, "auth/passkeys/") && r.Method == "DELETE"
	if e != nil && !deleting {
		return nil, 0, e
	}
	if e = rateLimit(ctx, s.Store.Pool, fmt.Sprintf("passkey-manage:%d", num(a.User, "id")), 12, 5*time.Minute); e != nil {
		return nil, 0, e
	}
	if path == "auth/passkeys/register/finish" && r.Method == "POST" {
		challenge, e := s.takePasskeyChallenge(w, r, "register", a)
		if e != nil {
			return nil, 0, e
		}
		tx, e := s.Store.Pool.Begin(ctx)
		if e != nil {
			return nil, 0, e
		}
		defer tx.Rollback(ctx)
		u, e := lockedPasskeyAccount(ctx, tx, num(a.User, "id"))
		if e != nil {
			return nil, 0, e
		}
		if num(u, "_auth_version") != challenge.version || challenge.user != num(u, "id") {
			return nil, 0, fail(401, "passkey request expired; try again")
		}
		user, e := s.loadPasskeyUser(ctx, tx, u)
		if e != nil {
			return nil, 0, e
		}
		if len(user.credentials) >= 20 {
			return nil, 0, fail(409, "passkey limit reached")
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		credential, e := wa.FinishRegistration(user, challenge.session, r)
		if e != nil || credential == nil || !credential.Flags.UserVerified {
			return nil, 0, fail(400, "passkey registration could not be verified")
		}
		raw, _ := json.Marshal(credential)
		tag, e := tx.Exec(ctx, `INSERT INTO vr_passkeys(id,user_id,label,credential) VALUES($1,$2,$3,$4) ON CONFLICT(id) DO NOTHING`, credential.ID, num(u, "id"), challenge.label, string(raw))
		if e != nil {
			return nil, 0, e
		}
		if tag.RowsAffected() != 1 {
			return nil, 0, fail(409, "passkey is already registered")
		}
		if e = s.passkeyAccountChanged(ctx, tx, u, a, true); e != nil {
			return nil, 0, e
		}
		if e = audit(ctx, tx, num(u, "id"), "auth.passkey_added", challenge.label, Data{}); e != nil {
			return nil, 0, e
		}
		if e = tx.Commit(ctx); e != nil {
			return nil, 0, e
		}
		return Data{"id": base64.RawURLEncoding.EncodeToString(credential.ID), "name": challenge.label}, 201, nil
	}
	if !(path == "auth/passkeys/register/begin" && r.Method == "POST" || deleting) {
		return nil, 0, fail(405, "method not allowed")
	}
	input, e := readBody(w, r)
	if e != nil {
		return nil, 0, e
	}
	if e = allowed(input, "password code name"); e != nil {
		return nil, 0, fail(400, e.Error())
	}
	tx, e := s.Store.Pool.Begin(ctx)
	if e != nil {
		return nil, 0, e
	}
	defer tx.Rollback(ctx)
	u, e := lockedPasskeyAccount(ctx, tx, num(a.User, "id"))
	if e != nil {
		return nil, 0, e
	}
	if num(u, "_auth_version") != num(a.User, "_auth_version") {
		return nil, 0, fail(401, "session expired")
	}
	if e = passkeyManagementProof(s.Config, u, input, a); e != nil {
		return nil, 0, e
	}
	user, e := s.loadPasskeyUser(ctx, tx, u)
	if e != nil {
		return nil, 0, e
	}
	if deleting {
		id, e := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(path, "auth/passkeys/"))
		if e != nil || len(id) == 0 || len(id) > 2048 {
			return nil, 0, fail(404, "passkey not found")
		}
		cfg, e := settings(ctx, tx)
		if e != nil {
			return nil, 0, e
		}
		if flag(cfg, "require_2fa_for_admins") && !flag(u, "_totp_enabled") && len(user.credentials) <= 1 {
			return nil, 0, fail(403, "enroll another passkey or TOTP before removing your last security factor")
		}
		tag, e := tx.Exec(ctx, `DELETE FROM vr_passkeys WHERE id=$1 AND user_id=$2`, id, num(u, "id"))
		if e != nil {
			return nil, 0, e
		}
		if tag.RowsAffected() != 1 {
			return nil, 0, fail(404, "passkey not found")
		}
		second := flag(u, "_totp_enabled") || a.SecondFactor && len(user.credentials) > 1
		if e = s.passkeyAccountChanged(ctx, tx, u, a, second); e != nil {
			return nil, 0, e
		}
		if e = audit(ctx, tx, num(u, "id"), "auth.passkey_removed", base64.RawURLEncoding.EncodeToString(id), Data{}); e != nil {
			return nil, 0, e
		}
		if e = tx.Commit(ctx); e != nil {
			return nil, 0, e
		}
		return nil, 204, nil
	}
	if len(user.credentials) >= 20 {
		return nil, 0, fail(409, "passkey limit reached")
	}
	if e = require(input, "name", 80); e != nil {
		return nil, 0, fail(400, e.Error())
	}
	exclusions := []protocol.CredentialDescriptor{}
	for _, c := range user.credentials {
		exclusions = append(exclusions, c.Descriptor())
	}
	options, session, e := wa.BeginRegistration(user, webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired), webauthn.WithExclusions(exclusions), webauthn.WithRegistrationOrigin(s.Config.PublicURL))
	if e != nil {
		return nil, 0, e
	}
	token, e := s.savePasskeyChallenge(ctx, tx, "register", u, a, str(input, "name"), session)
	if e != nil {
		return nil, 0, e
	}
	if e = save(ctx, tx, "users", u); e != nil {
		return nil, 0, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, 0, e
	}
	s.cookie(w, "veloray_passkey_register", token, 120, true)
	return options, 200, nil
}
func (s *Server) passkeyAccountChanged(ctx context.Context, tx pgx.Tx, u Data, a Actor, second bool) error {
	var count int64
	if e := tx.QueryRow(ctx, `SELECT count(*) FROM vr_passkeys WHERE user_id=$1`, num(u, "id")).Scan(&count); e != nil {
		return e
	}
	u["_passkey_count"] = count
	u["_auth_version"] = num(u, "_auth_version") + 1
	if _, e := tx.Exec(ctx, `DELETE FROM vr_sessions WHERE user_id=$1 AND token_hash<>$2`, num(u, "id"), a.SessionHash); e != nil {
		return e
	}
	if _, e := tx.Exec(ctx, `UPDATE vr_sessions SET auth_version=$1,second_factor=$2 WHERE token_hash=$3`, num(u, "_auth_version"), second, a.SessionHash); e != nil {
		return e
	}
	return save(ctx, tx, "users", u)
}
