package panel

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"golang.org/x/crypto/pbkdf2"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ber "github.com/go-asn1-ber/asn1-ber"
)

const ldapTestDN = "uid=alice,ou=people,dc=example,dc=com"

type ldapFixture struct {
	endpoint, ca string
	requests     atomic.Int32
	mu           sync.Mutex
	beforeReply  func(string, string) int64
	rawReply     []byte
}

// Speak the actual LDAPv3 simple-bind protocol over verified TLS; this fixture
// does not bypass the production dialer, certificate checks or LDAP client.
func ldapTestDirectory(t *testing.T) *ldapFixture {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "LDAP test"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}})
	if err != nil {
		t.Fatal(err)
	}
	f := &ldapFixture{endpoint: "ldaps://" + listener.Addr().String(), ca: filepath.Join(t.TempDir(), "ca.pem")}
	if err := os.WriteFile(f.ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(3 * time.Second))
				request, err := ber.ReadPacket(conn)
				if err != nil {
					return
				}
				f.requests.Add(1)
				if len(request.Children) != 2 || len(request.Children[1].Children) != 3 || request.Children[1].Tag != 0 || request.Children[1].ClassType != ber.ClassApplication {
					t.Error("not an LDAP bind request")
					return
				}
				op := request.Children[1]
				if op.Children[0].Value != int64(3) || op.Children[2].Tag != 0 || op.Children[2].ClassType != ber.ClassContext {
					t.Error("not LDAPv3 simple bind")
					return
				}
				dn, password := op.Children[1].Value.(string), string(op.Children[2].Data.Bytes())
				code := int64(49)
				if dn == ldapTestDN && password == "Directory-password-42" {
					code = 0
				}
				f.mu.Lock()
				callback, raw := f.beforeReply, append([]byte(nil), f.rawReply...)
				f.mu.Unlock()
				if callback != nil {
					code = callback(dn, password)
				}
				if len(raw) != 0 {
					conn.Write(raw)
					return
				}
				response := ber.Encode(ber.ClassUniversal, ber.TypeConstructed, ber.TagSequence, nil, "LDAP message")
				response.AppendChild(ber.NewInteger(ber.ClassUniversal, ber.TypePrimitive, ber.TagInteger, request.Children[0].Value, "message ID"))
				result := ber.Encode(ber.ClassApplication, ber.TypeConstructed, 1, nil, "BindResponse")
				result.AppendChild(ber.NewInteger(ber.ClassUniversal, ber.TypePrimitive, ber.TagEnumerated, code, "result"))
				result.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, "", "matched DN"))
				result.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, "", "diagnostic"))
				response.AppendChild(result)
				conn.Write(response.Bytes())
			}()
		}
	}()
	t.Cleanup(func() { listener.Close(); workers.Wait() })
	return f
}
func (f *ldapFixture) configure(t *testing.T, s *Server) {
	t.Helper()
	s.Config.SecureCookies = true
	s.Config.PublicURL = "https://panel.example"
	if err := s.Config.configureLDAP(f.endpoint, f.ca); err != nil {
		t.Fatal(err)
	}
}

func TestLDAPVerifiedTLSBindAndBounds(t *testing.T) {
	f := ldapTestDirectory(t)
	s := &Server{}
	f.configure(t, s)
	if err := s.ldapBind(context.Background(), ldapTestDN, "Directory-password-42"); err != nil {
		t.Fatal("valid bind", err)
	}
	if err := s.ldapBind(context.Background(), ldapTestDN, "wrong"); err == nil {
		t.Fatal("invalid password accepted")
	}
	before := f.requests.Load()
	for _, password := range []string{"", "bad\x00password", strings.Repeat("x", 1025)} {
		if err := s.ldapBind(context.Background(), ldapTestDN, password); err == nil {
			t.Fatal("unsafe password accepted")
		}
	}
	if f.requests.Load() != before {
		t.Fatal("empty/invalid bind reached directory")
	}
	trusted := s.Config.LDAPTLS
	s.Config.LDAPTLS = trusted.Clone()
	s.Config.LDAPTLS.RootCAs = x509.NewCertPool()
	if err := s.ldapBind(context.Background(), ldapTestDN, "Directory-password-42"); err == nil {
		t.Fatal("untrusted certificate accepted")
	}
	s.Config.LDAPTLS = trusted.Clone()
	s.Config.LDAPTLS.ServerName = "wrong.example"
	if err := s.ldapBind(context.Background(), ldapTestDN, "Directory-password-42"); err == nil {
		t.Fatal("wrong TLS hostname accepted")
	}
	s.Config.LDAPTLS = trusted
	f.mu.Lock()
	f.rawReply = append([]byte{0x30, 0x84, 0x7f, 0xff, 0xff, 0xff, 0x04, 0x84, 0x7f, 0xff, 0xff, 0xf0}, make([]byte, 70<<10)...)
	f.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.ldapBind(ctx, ldapTestDN, "Directory-password-42"); err == nil {
		t.Fatal("oversized BER accepted")
	}
	f.mu.Lock()
	f.rawReply = nil
	f.beforeReply = func(string, string) int64 { time.Sleep(300 * time.Millisecond); return 0 }
	f.mu.Unlock()
	ctx, cancel = context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	started := time.Now()
	if err := s.ldapBind(ctx, ldapTestDN, "Directory-password-42"); err == nil || time.Since(started) > 250*time.Millisecond {
		t.Fatal("cancelled directory request did not stop promptly", err)
	}
}

func TestLDAPConfigurationAndDNValidation(t *testing.T) {
	for _, endpoint := range []string{"ldap://host", "ldaps://user:pass@host", "ldaps://host/path", "ldaps://host?x=1", "ldaps://host?", "ldaps://host#fragment", "ldaps://host:0", "ldaps://host:65536"} {
		c := Config{SecureCookies: true, PublicURL: "https://panel.example"}
		if err := c.configureLDAP(endpoint, ""); err == nil {
			t.Fatal("unsafe endpoint", endpoint)
		}
	}
	c := Config{SecureCookies: true, PublicURL: "https://panel.example"}
	if err := c.configureLDAP("ldaps://directory.example", ""); err != nil || c.LDAPAddress != "directory.example:636" || c.LDAPTLS.InsecureSkipVerify {
		t.Fatal("valid configuration", err)
	}
	c = Config{PublicURL: "http://localhost"}
	if c.configureLDAP("ldaps://directory.example", "") == nil {
		t.Fatal("insecure panel allowed")
	}
	for _, dn := range []string{"uid=alice\n,dc=example", "not a DN", " ", strings.Repeat("x", 1025)} {
		if validateLDAPBinding(Data{"ldap_dn": dn}) == nil {
			t.Fatal("invalid DN accepted", dn)
		}
	}
	for _, dn := range []string{"", ldapTestDN, `CN=Alice\, Doe,OU=People,DC=example,DC=com`} {
		if err := validateLDAPBinding(Data{"ldap_dn": dn}); err != nil {
			t.Fatal("valid DN", err)
		}
	}
}

func TestIntegrationLDAPLocalPermissionsSecondFactorAndPasswordIsolation(t *testing.T) {
	s, ctx := integration(t)
	f := ldapTestDirectory(t)
	f.configure(t, s)
	u := defaults("users")
	u["username"], u["ldap_dn"], u["role"] = "directory-admin", ldapTestDN, "viewer"
	// A legacy local hash must survive LDAP login without being replaced with the directory password.
	u["_password"] = "pbkdf2_sha256$1000$salt$" + base64.StdEncoding.EncodeToString(pbkdf2.Key([]byte("Local-password-42"), []byte("salt"), 1000, 32, sha256.New))
	u["_totp_enabled"] = true
	u["_totp_secret"], _ = seal(s.Config.FieldKey, "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ")
	u["_recovery"] = []any{digest("RECOVERY-LDAP")}
	if err := save(ctx, s.Store.Pool, "users", u); err != nil {
		t.Fatal(err)
	}
	initialHash := str(u, "_password")
	attempt := func(name, password, provider, code string) (any, int, error, *httptest.ResponseRecorder) {
		t.Helper()
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(string(mustJSON(Data{"username": name, "password": password, "provider": provider, "totp_code": code})))).WithContext(ctx)
		out, status, err := s.login(w, r)
		return out, status, err, w
	}
	reject := func(name, password, provider, code string) {
		t.Helper()
		_, _, err, w := attempt(name, password, provider, code)
		var api *apiError
		if !errors.As(err, &api) || api.Status != 401 || len(w.Result().Cookies()) != 0 {
			t.Fatal("unauthorized bind accepted", err)
		}
	}
	reject("missing", "Directory-password-42", "ldap", "")
	reject("directory-admin", "Directory-password-42", "local", "")
	reject("directory-admin", "wrong", "ldap", "")
	out, status, err, _ := attempt("directory-admin", "Directory-password-42", "ldap", "")
	if err != nil || status != 202 || !flag(out.(Data), "two_factor_required") {
		t.Fatal("second factor skipped", out, status, err)
	}
	_, status, err, w := attempt("directory-admin", "Directory-password-42", "ldap", "RECOVERY-LDAP")
	if err != nil || status != 200 {
		t.Fatal("bound directory login", status, err)
	}
	r := httptest.NewRequest("GET", "/api/auth/me", nil).WithContext(ctx)
	for _, cookie := range w.Result().Cookies() {
		r.AddCookie(cookie)
	}
	actor, err := s.authenticate(r)
	if err != nil || str(actor.User, "role") != "viewer" || flag(actor.User, "is_superuser") || !actor.SecondFactor {
		t.Fatal("directory elevated local permissions", actor, err)
	}
	fresh, err := get(ctx, s.Store.Pool, "users", num(u, "id"))
	if err != nil || str(fresh, "_password") != initialHash || len(array(fresh, "_recovery")) != 0 {
		t.Fatal("directory password persisted or recovery not consumed", err)
	}
	reject("directory-admin", "Directory-password-42", "ldap", "RECOVERY-LDAP")
	// LDAP binding changes must revoke sessions through the existing auth version.
	fresh["_auth_version"] = num(fresh, "_auth_version") + 1
	if err := save(ctx, s.Store.Pool, "users", fresh); err != nil {
		t.Fatal(err)
	}
	if _, err := s.authenticate(r); err == nil {
		t.Fatal("revoked directory session survived")
	}
	// Disabled/non-staff accounts are rejected before sending credentials to LDAP.
	for _, key := range []string{"is_active", "is_staff"} {
		fresh[key] = false
		save(ctx, s.Store.Pool, "users", fresh)
		before := f.requests.Load()
		reject("directory-admin", "Directory-password-42", "ldap", "")
		if f.requests.Load() != before {
			t.Fatal("disabled account contacted directory")
		}
		fresh[key] = true
	}
	fresh["ldap_dn"] = ""
	save(ctx, s.Store.Pool, "users", fresh)
	before := f.requests.Load()
	reject("directory-admin", "Directory-password-42", "ldap", "")
	if f.requests.Load() != before {
		t.Fatal("unbound account contacted directory")
	}
	fresh["_totp_enabled"] = false
	if err := save(ctx, s.Store.Pool, "users", fresh); err != nil {
		t.Fatal(err)
	}
	_, status, err, _ = attempt("directory-admin", "Local-password-42", "", "")
	if err != nil || status != 200 || f.requests.Load() != before {
		t.Fatal("local fallback failed", status, err)
	}
	local, _ := get(ctx, s.Store.Pool, "users", num(u, "id"))
	if !checkPassword(str(local, "_password"), "Local-password-42") || checkPassword(str(local, "_password"), "Directory-password-42") {
		t.Fatal("LDAP replaced local password")
	}

}

func TestIntegrationLDAPConcurrentBindingChangeAndUniqueIdentity(t *testing.T) {
	s, ctx := integration(t)
	f := ldapTestDirectory(t)
	f.configure(t, s)
	u := defaults("users")
	u["username"], u["ldap_dn"] = "directory-admin", ldapTestDN
	if err := save(ctx, s.Store.Pool, "users", u); err != nil {
		t.Fatal(err)
	}
	duplicate := defaults("users")
	duplicate["username"], duplicate["ldap_dn"] = "other", strings.ToUpper(ldapTestDN)
	if err := save(ctx, s.Store.Pool, "users", duplicate); err == nil {
		t.Fatal("same directory identity linked twice")
	}
	f.mu.Lock()
	f.beforeReply = func(string, string) int64 {
		// This would block if login held FOR UPDATE during directory network I/O.
		timeout, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		_, err := s.Store.Pool.Exec(timeout, `UPDATE vr_users SET data=jsonb_set(data,'{_auth_version}',to_jsonb((data->>'_auth_version')::bigint+1)) WHERE id=$1`, num(u, "id"))
		if err != nil {
			t.Error("user row locked while waiting for directory", err)
		}
		return 0
	}
	f.mu.Unlock()
	r := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(fmt.Sprintf(`{"username":"directory-admin","password":"Directory-password-42","provider":"ldap"}`))).WithContext(ctx)
	_, _, err := s.login(httptest.NewRecorder(), r)
	var api *apiError
	if !errors.As(err, &api) || api.Status != 401 {
		t.Fatal("binding changed during LDAP bind but session created", err)
	}
	var sessions int
	if err := s.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM vr_sessions`).Scan(&sessions); err != nil || sessions != 0 {
		t.Fatal("unexpected session", sessions, err)
	}
}

// Ensure the response budget is strict even if the peer advertises a giant BER length.
func TestLDAPReadBudget(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	go func() { b.Write(make([]byte, 128<<10)) }()
	c := &ldapReadLimitConn{Conn: a, remaining: 64 << 10}
	got, err := io.ReadAll(c)
	if len(got) != 64<<10 || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal("unbounded LDAP input", len(got), err)
	}
}

func TestIntegrationLDAPAdministratorBindingAPI(t *testing.T) {
	s, ctx := integration(t)
	admin := defaults("users")
	admin["username"], admin["is_superuser"] = "owner", true
	if err := save(ctx, s.Store.Pool, "users", admin); err != nil {
		t.Fatal(err)
	}
	user := defaults("users")
	user["username"], user["role"] = "directory-user", "viewer"
	if err := save(ctx, s.Store.Pool, "users", user); err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("admin/users/%d", num(user, "id"))
	token := randomToken(32)
	if _, err := s.Store.Pool.Exec(ctx, `INSERT INTO vr_sessions(token_hash,user_id,auth_version,second_factor,expires_at) VALUES($1,$2,$3,false,now()+interval '1 hour')`, digest(token), num(user, "id"), num(user, "_auth_version")); err != nil {
		t.Fatal(err)
	}
	csrf := csrfToken(s.Config.SecretKey)
	request := httptest.NewRequest("PATCH", "/api/"+path, strings.NewReader(string(mustJSON(Data{"ldap_dn": ldapTestDN})))).WithContext(ctx)
	request.Header.Set("Origin", s.Config.PublicURL)
	request.Header.Set("X-CSRFToken", csrf)
	request.AddCookie(&http.Cookie{Name: "csrftoken", Value: csrf})
	request.AddCookie(&http.Cookie{Name: "veloray_session", Value: token})
	rejected := httptest.NewRecorder()
	s.Handler().ServeHTTP(rejected, request)
	if rejected.Code != 403 {
		t.Fatal("non-admin changed LDAP identity", rejected.Code, rejected.Body.String())
	}
	var api *apiError
	actor := Actor{User: admin}
	for _, input := range []Data{{"ldap_dn": "not a DN"}, {"ldap_dn": "uid=alice\n,dc=example"}} {
		_, _, err := managementRequest(t, s, ctx, actor, "PATCH", path, input)
		if !errors.As(err, &api) || api.Status != 400 {
			t.Fatal("invalid LDAP identity saved", err)
		}
	}
	out, _, err := managementRequest(t, s, ctx, actor, "PATCH", path, Data{"ldap_dn": ldapTestDN})
	if err != nil || str(out.(Data), "ldap_dn") != ldapTestDN {
		t.Fatal("binding API failed", out, err)
	}
	fresh, err := get(ctx, s.Store.Pool, "users", num(user, "id"))
	if err != nil || num(fresh, "_auth_version") != num(user, "_auth_version")+1 {
		t.Fatal("identity update did not revoke sessions", err)
	}
	_, _, err = managementRequest(t, s, ctx, actor, "PATCH", fmt.Sprintf("admin/users/%d", num(admin, "id")), Data{"ldap_dn": strings.ToUpper(ldapTestDN)})
	if !errors.As(err, &api) || api.Status != 409 {
		t.Fatal("duplicate identity must return conflict", err)
	}
	if err := s.Store.Migrate(ctx); err != nil {
		t.Fatal("LDAP migration is not idempotent", err)
	}
	_, _, err = managementRequest(t, s, ctx, actor, "PATCH", path, Data{"ldap_dn": ""})
	if err != nil {
		t.Fatal("cannot remove LDAP binding", err)
	}
	fresh, _ = get(ctx, s.Store.Pool, "users", num(user, "id"))
	if str(fresh, "ldap_dn") != "" || num(fresh, "_auth_version") != num(user, "_auth_version")+2 {
		t.Fatal("removing binding did not revoke sessions")
	}
}
