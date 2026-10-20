package panel

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/go-ldap/ldap/v3"
)

var errDirectoryLogin = errors.New("directory sign-in failed")

// LDAP identities are administrator-bound DNs, never a filter built from login input.
func validateLDAPBinding(user Data) error {
	value := str(user, "ldap_dn")
	if value == "" {
		return nil
	}
	if len(value) > 1024 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return errors.New("LDAP DN must be a valid distinguished name of at most 1024 bytes")
	}
	dn, err := ldap.ParseDN(value)
	if err != nil || len(dn.RDNs) == 0 {
		return errors.New("LDAP DN must be a valid distinguished name")
	}
	user["ldap_dn"] = dn.String()
	return nil
}

func (c *Config) configureLDAP(endpoint, caFile string) error {
	if endpoint == "" {
		if caFile != "" {
			return errors.New("VELORAY_LDAP_CA_FILE requires VELORAY_LDAP_URL")
		}
		return nil
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "ldaps" || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return errors.New("VELORAY_LDAP_URL must be an ldaps://host[:port] origin without credentials or a path")
	}
	port := u.Port()
	if port == "" {
		port = "636"
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return errors.New("invalid LDAPS port")
	}
	if !c.SecureCookies || !strings.HasPrefix(c.PublicURL, "https://") {
		return errors.New("LDAP sign-in requires a secure HTTPS panel origin")
	}
	config := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: u.Hostname()}
	if caFile != "" {
		f, err := os.Open(caFile)
		if err != nil {
			return errors.New("cannot read LDAP CA file")
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
			return errors.New("LDAP CA file must be a regular PEM file of at most 1 MiB")
		}
		pem, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
		if err != nil || len(pem) > 1<<20 {
			return errors.New("cannot read LDAP CA file")
		}
		roots, err := x509.SystemCertPool()
		if err != nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM(pem) {
			return errors.New("LDAP CA file contains no certificates")
		}
		config.RootCAs = roots
	}
	c.LDAPAddress, c.LDAPTLS = net.JoinHostPort(u.Hostname(), port), config
	return nil
}

func (s *Server) ldapEnabled() bool { return s.Config.LDAPAddress != "" && s.Config.LDAPTLS != nil }

// A bind response is tiny. Bound all BER input without changing the library's
// global packet limits (which would race with other connections).
type ldapReadLimitConn struct {
	net.Conn
	remaining int
}

func (c *ldapReadLimitConn) Read(p []byte) (int, error) {
	if c.remaining <= 0 {
		return 0, io.ErrUnexpectedEOF
	}
	if len(p) > c.remaining {
		p = p[:c.remaining]
	}
	n, err := c.Conn.Read(p)
	c.remaining -= n
	return n, err
}

func (s *Server) ldapBind(ctx context.Context, dn, password string) error {
	// Empty passwords can mean anonymous authentication on directory servers.
	if !s.ldapEnabled() || dn == "" || password == "" || len(password) > 1024 || strings.ContainsRune(password, 0) {
		return errDirectoryLogin
	}
	ctx, cancel := context.WithTimeout(ctx, 7*time.Second)
	defer cancel()
	raw, err := (&net.Dialer{}).DialContext(ctx, "tcp", s.Config.LDAPAddress)
	if err != nil {
		return errDirectoryLogin
	}
	defer raw.Close()
	stop := context.AfterFunc(ctx, func() { raw.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	if err = raw.SetDeadline(deadline); err != nil {
		return errDirectoryLogin
	}
	secured := tls.Client(raw, s.Config.LDAPTLS.Clone())
	if err = secured.HandshakeContext(ctx); err != nil {
		return errDirectoryLogin
	}
	conn := ldap.NewConn(&ldapReadLimitConn{Conn: secured, remaining: 64 << 10}, true)
	conn.SetTimeout(7 * time.Second)
	conn.Start()
	defer conn.Close()
	if err = conn.Bind(dn, password); err != nil {
		return errDirectoryLogin
	}
	return nil
}
