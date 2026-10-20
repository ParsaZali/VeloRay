package panel

import (
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/PaarSaAm/VeloRay/internal/envfiles"
	"net"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	LDAPAddress                                                      string
	LDAPTLS                                                          *tls.Config
	OIDCIssuer, OIDCClientID, OIDCClientSecret                       string
	GitHubClientID, GitHubClientSecret                               string
	Listen, DatabaseURL, PublicURL, SecretKey, FieldKey, FrontendDir string
	SecureCookies                                                    bool
	SubscriptionOnly                                                 bool
	SessionAge                                                       time.Duration
	PanelPort                                                        int
	TrustedOrigins                                                   map[string]bool
	TrustedProxies                                                   []netip.Prefix
	TemplateRoot                                                     string
}

func env(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}
func LoadConfig() (Config, error) {
	if err := envfiles.LoadSecrets(); err != nil {
		return Config{}, err
	}
	c := Config{Listen: env("VELORAY_LISTEN", "127.0.0.1:8610"), PublicURL: strings.TrimRight(env("VELORAY_PUBLIC_URL", ""), "/"), SecretKey: os.Getenv("VELORAY_SECRET_KEY"), FieldKey: os.Getenv("VELORAY_FIELD_KEY"), FrontendDir: env("VELORAY_FRONTEND_DIR", "frontend/dist"), SecureCookies: env("VELORAY_COOKIE_SECURE", "true") != "false", TrustedOrigins: map[string]bool{}}
	if len(c.SecretKey) < 32 || len(c.FieldKey) < 32 {
		return c, errors.New("VELORAY_SECRET_KEY and VELORAY_FIELD_KEY must each contain at least 32 characters")
	}
	if _, _, err := net.SplitHostPort(c.Listen); err != nil {
		return c, fmt.Errorf("invalid listen address: %w", err)
	}
	if c.PublicURL == "" {
		host := env("VELORAY_DOMAIN", env("VELORAY_PUBLIC_HOST", "localhost"))
		port := env("VELORAY_PANEL_PORT", "8443")
		c.PublicURL = "https://" + net.JoinHostPort(host, port)
	}
	u, err := url.Parse(c.PublicURL)
	if err != nil || u.Host == "" || u.User != nil || u.Path != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return c, errors.New("VELORAY_PUBLIC_URL must be an HTTP(S) origin without credentials or a path")
	}
	if u.Scheme != "https" && c.SecureCookies {
		return c, errors.New("HTTP development requires VELORAY_COOKIE_SECURE=false")
	}
	if !c.SecureCookies {
		ip := net.ParseIP(u.Hostname())
		if u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return c, errors.New("insecure session cookies are allowed only on a loopback development origin")
		}
	}
	c.OIDCIssuer, c.OIDCClientID, c.OIDCClientSecret = env("VELORAY_OIDC_ISSUER", ""), env("VELORAY_OIDC_CLIENT_ID", ""), env("VELORAY_OIDC_CLIENT_SECRET", "")
	if (c.OIDCIssuer != "" || c.OIDCClientID != "" || c.OIDCClientSecret != "") && (!validOIDCURL(c.OIDCIssuer) || c.OIDCClientID == "" || !c.SecureCookies) {
		return c, errors.New("OIDC requires HTTPS issuer, client ID and secure cookies")
	}
	if err := c.configureLDAP(os.Getenv("VELORAY_LDAP_URL"), os.Getenv("VELORAY_LDAP_CA_FILE")); err != nil {
		return c, err
	}
	c.SubscriptionOnly = env("VELORAY_SUBSCRIPTION_ONLY", "false") == "true"
	c.GitHubClientID, c.GitHubClientSecret = os.Getenv("VELORAY_GITHUB_CLIENT_ID"), os.Getenv("VELORAY_GITHUB_CLIENT_SECRET")
	if (c.GitHubClientID != "" || c.GitHubClientSecret != "") && (c.GitHubClientID == "" || c.GitHubClientSecret == "" || !c.SecureCookies || !strings.HasPrefix(c.PublicURL, "https://")) {
		return c, errors.New("GitHub sign-in requires client ID, client secret and a secure HTTPS origin")
	}
	proxyCIDRs, configured := os.LookupEnv("VELORAY_TRUSTED_PROXIES")
	if !configured {
		proxyCIDRs = "127.0.0.1/32,::1/128"
	}
	c.TrustedProxies, err = parseTrustedProxies(proxyCIDRs)
	if err != nil {
		return c, err
	}
	c.TemplateRoot = env("VELORAY_TEMPLATE_ROOT", "/etc/veloray/templates")
	c.TrustedOrigins[c.PublicURL] = true
	for _, s := range strings.Split(os.Getenv("VELORAY_CSRF_TRUSTED_ORIGINS"), ",") {
		if s = strings.TrimSpace(s); s != "" {
			c.TrustedOrigins[s] = true
		}
	}
	c.PanelPort, _ = strconv.Atoi(env("VELORAY_PANEL_PORT", u.Port()))
	if c.PanelPort == 0 {
		if u.Scheme == "https" {
			c.PanelPort = 443
		} else {
			c.PanelPort = 80
		}
	}
	age, _ := strconv.Atoi(env("VELORAY_SESSION_AGE", "43200"))
	if age < 300 || age > 604800 {
		return c, errors.New("VELORAY_SESSION_AGE must be 300–604800 seconds")
	}
	c.SessionAge = time.Duration(age) * time.Second
	c.DatabaseURL = os.Getenv("DATABASE_URL")
	if c.DatabaseURL == "" {
		db := url.URL{Scheme: "postgres", Host: net.JoinHostPort(env("POSTGRES_HOST", "127.0.0.1"), env("POSTGRES_PORT", "5432")), Path: "/" + env("POSTGRES_DB", "veloray"), User: url.UserPassword(env("POSTGRES_USER", "veloray"), os.Getenv("POSTGRES_PASSWORD"))}
		q := url.Values{}
		q.Set("sslmode", env("POSTGRES_SSLMODE", "disable"))
		db.RawQuery = q.Encode()
		c.DatabaseURL = db.String()
	}
	return c, nil
}
