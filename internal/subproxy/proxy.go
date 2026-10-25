// Package subproxy supervises Docker's rootless HTTPS proxy. Its shared socket
// accepts only a subscription port, never Caddy JSON, filenames or commands.
package subproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const controlPath = "/run/veloray-proxy/control.sock"
const adminPath = "/tmp/veloray-caddy-admin.sock"
const statePath = "/config/veloray-subscriptions.json"

var errProxyUnavailable = errors.New("proxy configuration API is not ready")

var domainLabel = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)

func validDomain(domain string) bool {
	if net.ParseIP(domain) != nil {
		return true
	}
	if len(domain) < 1 || len(domain) > 253 {
		return false
	}
	for _, label := range strings.Split(domain, ".") {
		if !domainLabel.MatchString(label) {
			return false
		}
	}
	return true
}
func validPort(port int) bool {
	if port < 0 || port > 65535 {
		return false
	}
	for _, reserved := range []int{80, 5432, 8610, 8611, 8612, 9191, 10085, 10086, 10087} {
		if port == reserved {
			return false
		}
	}
	return true
}
func configuration(domain string, port int) string {
	return configurationFor(domain, port, 443, "auto")
}
func configurationFor(domain string, port, panelPort int, tlsMode string) string {
	host := domain
	if net.ParseIP(domain) != nil && strings.Contains(domain, ":") {
		host = "[" + domain + "]"
	}
	if panelPort != 443 {
		host = net.JoinHostPort(domain, strconv.Itoa(panelPort))
	}
	tlsDirective, globalDirective := "", ""
	if tlsMode == "local" {
		tlsDirective = "tls internal"
		globalDirective = "auto_https disable_redirects"
	}
	subscription := "reverse_proxy 127.0.0.1:8612"
	if port > 0 {
		subscription = fmt.Sprintf("redir https://%s{uri} 307", net.JoinHostPort(domain, strconv.Itoa(port)))
	}
	raw := fmt.Sprintf(`{
 admin unix/%s
 default_sni %s
 %s
 skip_install_trust
 log default {
  output stderr
  exclude http admin.api
 }
}
https://%s {
 %s
 encode zstd gzip
 header {
  X-Content-Type-Options nosniff
  Referrer-Policy no-referrer
 }
 handle /sub/* {
  %s
 }
 handle {
  reverse_proxy 127.0.0.1:8610
 }
}
`, adminPath, domain, globalDirective, host, tlsDirective, subscription)
	if port > 0 {
		raw += fmt.Sprintf(`https://%s {
 %s
 encode zstd gzip
 header {
  X-Content-Type-Options nosniff
  Referrer-Policy no-referrer
 }
 reverse_proxy 127.0.0.1:8612
}
`, net.JoinHostPort(domain, strconv.Itoa(port)), tlsDirective)
	}
	return raw
}

type controller struct {
	mu        sync.Mutex
	domain    string
	panelPort int
	tlsMode   string
	port      int
	reload    func(context.Context, string) error
	persist   func(int) error
	health    func(context.Context) error
	fatal     func()
}

func (c *controller) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Path != "/port" {
		http.NotFound(w, r)
		return
	}
	if r.Header.Get("Origin") != "" {
		http.Error(w, "origin forbidden", 403)
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if r.Method == "GET" {
		_ = json.NewEncoder(w).Encode(map[string]int{"port": c.port})
		return
	}
	if r.Method != "PUT" {
		w.WriteHeader(405)
		return
	}
	var in struct {
		Port *int `json:"port"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil || in.Port == nil || (!validPort(*in.Port) || *in.Port == c.effectivePanelPort()) {
		http.Error(w, "invalid port", 400)
		return
	}
	var trailing any
	if d.Decode(&trailing) != io.EOF {
		http.Error(w, "invalid body", 400)
		return
	}
	port := *in.Port
	if port > 0 && c.health != nil {
		if err := c.health(r.Context()); err != nil {
			http.Error(w, "subscription service unavailable", 503)
			return
		}
	}

	// Even an unchanged initial port must be confirmed by the running Caddy
	// configuration API before it becomes durable or customer-visible.
	if err := c.reload(r.Context(), c.configuration(port)); err != nil {
		status := 409
		if errors.Is(err, errProxyUnavailable) {
			status = 503
		}
		http.Error(w, "port could not be applied", status)
		return
	}
	if err := c.persist(port); err != nil {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
		rollbackErr := c.reload(ctx, c.configuration(c.port))
		cancel()
		if rollbackErr != nil && c.fatal != nil {
			c.fatal()
		}
		http.Error(w, "configuration could not be saved", 500)
		return
	}
	c.port = port

	_ = json.NewEncoder(w).Encode(map[string]int{"port": c.port})
}
func (c *controller) effectivePanelPort() int {
	if c.panelPort == 0 {
		return 443
	}
	return c.panelPort
}
func (c *controller) configuration(port int) string {
	return configurationFor(c.domain, port, c.effectivePanelPort(), c.tlsMode)
}
func persistPort(port int) error {
	raw, _ := json.Marshal(map[string]int{"port": port})
	f, err := os.CreateTemp(filepath.Dir(statePath), ".subscription-port-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), statePath)
}
func Run(parent context.Context) error {
	domain := os.Getenv("VELORAY_DOMAIN")
	if !validDomain(domain) {
		return errors.New("invalid proxy domain")
	}
	panelPort := 443
	if text := os.Getenv("VELORAY_PANEL_PORT"); text != "" {
		var err error
		panelPort, err = strconv.Atoi(text)
		if err != nil || panelPort < 1 || panelPort > 65535 {
			return errors.New("invalid panel port")
		}
	}
	for _, reserved := range []int{80, 5432, 8610, 8611, 8612, 9191, 10085, 10086, 10087} {
		if panelPort == reserved {
			return errors.New("reserved panel port")
		}
	}
	tlsMode := os.Getenv("VELORAY_TLS_MODE")
	if tlsMode == "" {
		tlsMode = "auto"
	}
	if tlsMode != "auto" && tlsMode != "local" {
		return errors.New("invalid TLS mode")
	}
	port := 8444
	if text := os.Getenv("VELORAY_SUBSCRIPTION_PORT"); text != "" {
		var err error
		port, err = strconv.Atoi(text)
		if err != nil {
			return errors.New("invalid subscription port")
		}
	}
	if raw, err := os.ReadFile(statePath); err == nil {
		var saved struct {
			Port *int `json:"port"`
		}
		if json.Unmarshal(raw, &saved) != nil || saved.Port == nil || !validPort(*saved.Port) {
			return errors.New("invalid saved subscription port")
		}
		port = *saved.Port
	} else if !os.IsNotExist(err) {
		return err
	}
	if !validPort(port) || port == panelPort {
		return errors.New("reserved subscription port")
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	const configPath = "/tmp/veloray-Caddyfile"
	if err := os.WriteFile(configPath, []byte(configurationFor(domain, port, panelPort, tlsMode)), 0600); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "caddy", "run", "--config", configPath, "--adapter", "caddyfile")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait(); cancel() }()
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", adminPath)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 6 * time.Second}
	reload := func(ctx context.Context, config string) error {
		req, _ := http.NewRequestWithContext(ctx, "POST", "http://caddy/load", bytes.NewBufferString(config))
		req.Header.Set("Content-Type", "text/caddyfile")
		res, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("%w: %v", errProxyUnavailable, err)
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		if res.StatusCode != 200 {
			// This response concerns only our generated domain/port configuration; no
			// public request, subscription token or application credential is included.
			fmt.Fprintf(os.Stderr, "VeloRay proxy configuration rejected (HTTP %d): %q\n", res.StatusCode, string(body))
			return errors.New("Caddy rejected configuration")
		}
		return nil
	}
	if info, err := os.Lstat(controlPath); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return errors.New("unexpected proxy control file")
		}
		if err = os.Remove(controlPath); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	listener, err := net.Listen("unix", controlPath)
	if err != nil {
		return err
	}
	defer listener.Close()
	defer os.Remove(controlPath)
	if err = os.Chmod(controlPath, 0660); err != nil {
		return err
	}
	healthClient := &http.Client{Transport: &http.Transport{}, Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	health := func(ctx context.Context) error {
		req, _ := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:8612/api/health", nil)
		res, err := healthClient.Do(req)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
		if res.StatusCode != 200 {
			return errors.New("subscription service health failed")
		}
		return nil
	}
	server := &http.Server{Handler: &controller{domain: domain, port: port, panelPort: panelPort, tlsMode: tlsMode, reload: reload, persist: persistPort, health: health, fatal: cancel}, ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 8 * time.Second, WriteTimeout: 10 * time.Second, MaxHeaderBytes: 4096}
	go func() {
		<-ctx.Done()
		shutdown, stop := context.WithTimeout(context.Background(), 2*time.Second)
		defer stop()
		_ = server.Shutdown(shutdown)
	}()
	err = server.Serve(listener)
	cancel()
	processErr := <-done
	if errors.Is(err, http.ErrServerClosed) && parent.Err() != nil {
		return nil
	}
	if processErr != nil {
		return processErr
	}
	return err
}
