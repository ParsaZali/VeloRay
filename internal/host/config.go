package host

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

func SetEnv(path, key, value string) error {
	if !regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`).MatchString(key) || strings.ContainsAny(value, "\r\n\x00") {
		return errors.New("invalid environment setting")
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	found := false
	for i, line := range lines {
		if strings.HasPrefix(line, key+"=") {
			lines[i] = key + "=" + value
			found = true
		}
	}
	if !found {
		lines = append(lines, key+"="+value)
	}
	return AtomicFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0640)
}
func RenderNginx(ctx context.Context, origin, path string, subscriptionPort ...int) error {
	u, e := url.Parse(origin)
	if e != nil || u.Scheme != "https" || u.User != nil || u.Path != "" || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("HTTPS origin is required")
	}
	host := u.Hostname()
	if !regexp.MustCompile(`^[a-zA-Z0-9.:-]+$`).MatchString(host) {
		return errors.New("invalid public host")
	}
	port := u.Port()
	if port == "" {
		port = "443"
	}
	n, pe := strconv.Atoi(port)
	if pe != nil || n < 1 || n > 65535 {
		return errors.New("invalid port")
	}
	cert, key := "/etc/veloray/tls/panel.crt", "/etc/veloray/tls/panel.key"
	trusted := fmt.Sprintf("/etc/letsencrypt/live/%s", host)
	if _, e = os.Stat(trusted + "/fullchain.pem"); e == nil {
		cert, key = trusted+"/fullchain.pem", trusted+"/privkey.pem"
	} else {
		if e = os.MkdirAll("/etc/veloray/tls", 0700); e != nil {
			return e
		}
		san, check := "DNS:"+host, "-checkhost"
		if net.ParseIP(host) != nil {
			san, check = "IP:"+host, "-checkip"
		}
		_, keyErr := os.Stat(key)
		matches := exec.CommandContext(ctx, "openssl", "x509", "-in", cert, "-noout", check, host).Run() == nil
		if keyErr != nil || !matches {
			temporary, err := os.MkdirTemp(filepath.Dir(cert), ".certificate-*")
			if err != nil {
				return err
			}
			defer os.RemoveAll(temporary)
			newKey, newCert := filepath.Join(temporary, "panel.key"), filepath.Join(temporary, "panel.crt")
			cmd := exec.CommandContext(ctx, "openssl", "req", "-x509", "-newkey", "rsa:3072", "-sha256", "-nodes", "-days", "365", "-keyout", newKey, "-out", newCert, "-subj", "/CN="+host, "-addext", "subjectAltName="+san)
			if e = cmd.Run(); e != nil {
				return fmt.Errorf("cannot generate panel certificate: %w", e)
			}
			if e = os.Chmod(newKey, 0600); e != nil {
				return e
			}
			if e = os.Rename(newKey, key); e != nil {
				return e
			}
			if e = os.Rename(newCert, cert); e != nil {
				return e
			}
		}
	}
	if e = os.MkdirAll("/var/lib/veloray-acme", 0755); e != nil {
		return e
	}
	raw := nginxConfiguration(host, port, cert, key)
	if len(subscriptionPort) > 0 && subscriptionPort[0] > 0 {
		if subscriptionPort[0] > 65535 || subscriptionPort[0] == n || subscriptionPort[0] == 80 {
			return errors.New("invalid subscription listener port")
		}
		raw = append(raw, nginxSubscriptionConfiguration(host, strconv.Itoa(subscriptionPort[0]), cert, key)...)
	}

	old, e := os.ReadFile(path)
	exists := e == nil
	if e = AtomicFile(path, raw, 0644); e != nil {
		return e
	}
	link := "/etc/nginx/sites-enabled/veloray"
	if _, e = os.Lstat(link); os.IsNotExist(e) {
		if e = os.Symlink(path, link); e != nil {
			return e
		}
	}
	if out, e := exec.CommandContext(ctx, "nginx", "-t").CombinedOutput(); e != nil {
		if exists {
			_ = AtomicFile(path, old, 0644)
		} else {
			_ = os.Remove(path)
			_ = os.Remove(link)
		}
		return fmt.Errorf("Nginx validation failed: %.1500s", out)
	}
	if e = exec.CommandContext(ctx, "systemctl", "reload", "nginx").Run(); e != nil {
		if exists {
			_ = AtomicFile(path, old, 0644)
		} else {
			_ = os.Remove(path)
			_ = os.Remove(link)
		}
		_ = exec.CommandContext(ctx, "systemctl", "reload", "nginx").Run()
		return fmt.Errorf("Nginx reload failed; previous configuration restored: %w", e)
	}
	return nil
}
func nginxConfiguration(host, port, cert, key string) []byte {
	raw := []byte(fmt.Sprintf(`server {
 listen %s ssl;
 listen [::]:%s ssl;
 server_name %s;
 ssl_certificate %s;
 ssl_certificate_key %s;
 ssl_protocols TLSv1.2 TLSv1.3;
 server_tokens off;
 client_max_body_size 65m;
 location / {
  proxy_pass http://127.0.0.1:8610;
  proxy_set_header Host $http_host;
  proxy_set_header X-Real-IP $remote_addr;
  proxy_set_header X-Forwarded-Proto https;
  proxy_set_header X-Forwarded-For $remote_addr;
  proxy_read_timeout 180s;
 }
 access_log off;
}
`, port, port, host, cert, key))
	if port != "80" {
		raw = append(raw, []byte(fmt.Sprintf(`server {
 listen 80;
 listen [::]:80;
 server_name %s;
 server_tokens off;
 location ^~ /.well-known/acme-challenge/ {
  root /var/lib/veloray-acme;
  default_type text/plain;
  try_files $uri =404;
 }
 location / { return 308 https://%s$request_uri; }
 access_log off;
}
`, host, net.JoinHostPort(host, port)))...)
	}
	return raw
}

// The separate HTTPS listener forwards only to the subscription-only process.
func nginxSubscriptionConfiguration(host, port, cert, key string) []byte {
	return []byte(fmt.Sprintf(`server {
 listen %s ssl;
 listen [::]:%s ssl;
 server_name %s;
 ssl_certificate %s;
 ssl_certificate_key %s;
 ssl_protocols TLSv1.2 TLSv1.3;
 server_tokens off;
 client_max_body_size 1m;
 location / {
  proxy_pass http://127.0.0.1:8611;
  proxy_set_header Host $http_host;
  proxy_set_header X-Real-IP $remote_addr;
  proxy_set_header X-Forwarded-Proto https;
  proxy_set_header X-Forwarded-For $remote_addr;
  proxy_read_timeout 180s;
 }
 access_log off;
}
`, port, port, host, cert, key))
}
func NginxCertificate(host string) string {
	path := fmt.Sprintf("/etc/letsencrypt/live/%s/fullchain.pem", host)
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return "/etc/veloray/tls/panel.crt"
}
