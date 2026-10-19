package agent

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

func publicProbeIP(ip net.IP) bool {
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, cidr := range []string{"100.64.0.0/10", "198.18.0.0/15"} {
		_, block, _ := net.ParseCIDR(cidr)
		if block.Contains(ip) {
			return false
		}
	}
	return true
}
func prepareProbeURL(ctx context.Context, raw string) (*url.URL, string, error) {
	u, e := url.Parse(raw)
	if e != nil || len(raw) > 2048 || u.Scheme != "https" || u.User != nil || u.Hostname() == "" || u.Fragment != "" {
		return nil, "", errors.New("probe requires a public HTTPS URL without credentials or fragment")
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "localhost" || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") || strings.HasSuffix(host, ".localhost") {
		return nil, "", errors.New("private probe targets are forbidden")
	}
	port := u.Port()
	if port == "" {
		port = "443"
	}
	n, e := strconv.Atoi(port)
	if e != nil || n < 1 || n > 65535 {
		return nil, "", errors.New("invalid probe port")
	}
	ips, e := net.DefaultResolver.LookupIPAddr(ctx, host)
	if e != nil {
		return nil, "", errors.New("probe DNS lookup failed")
	}
	for _, v := range ips {
		if publicProbeIP(v.IP) {
			u.Host = net.JoinHostPort(v.IP.String(), port)
			return u, host, nil
		}
	}
	return nil, "", errors.New("probe target has no public address")
}
func probeOutbounds(raw []byte, core, tag string) ([]any, any, error) {
	var cfg map[string]any
	if json.Unmarshal(raw, &cfg) != nil {
		return nil, nil, errors.New("invalid stored core config")
	}
	entries, _ := cfg["outbounds"].([]any)
	byTag := map[string]map[string]any{}
	for _, v := range entries {
		if d, ok := v.(map[string]any); ok {
			key, _ := d["tag"].(string)
			byTag[key] = d
		}
	}
	visited := map[string]bool{}
	visiting := map[string]bool{}
	selected := []any{}
	var visit func(string) error
	visit = func(key string) error {
		if visiting[key] {
			return errors.New("outbound dependency cycle")
		}
		if visited[key] {
			return nil
		}
		d, ok := byTag[key]
		if !ok {
			return errors.New("outbound tag or dependency not found")
		}
		visiting[key] = true
		protocol, _ := d["protocol"].(string)
		kind, _ := d["type"].(string)
		if protocol == "wireguard" || kind == "wireguard" || kind == "tun" {
			return errors.New("kernel tunnel outbounds cannot be probed in an isolated process")
		}
		refs := []string{}
		if v, ok := d["detour"].(string); ok && v != "" {
			refs = append(refs, v)
		}
		if p, ok := d["proxySettings"].(map[string]any); ok {
			if v, ok := p["tag"].(string); ok && v != "" {
				refs = append(refs, v)
			}
		}
		if stream, ok := d["streamSettings"].(map[string]any); ok {
			if opts, ok := stream["sockopt"].(map[string]any); ok {
				if v, ok := opts["dialerProxy"].(string); ok && v != "" {
					refs = append(refs, v)
				}
			}
		}
		if kind == "selector" || kind == "urltest" {
			choices, ok := d["outbounds"].([]any)
			if !ok {
				return errors.New("invalid outbound group")
			}
			for _, v := range choices {
				if ref, ok := v.(string); ok {
					refs = append(refs, ref)
				}
			}
		}
		for _, ref := range refs {
			if e := visit(ref); e != nil {
				return e
			}
		}
		delete(visiting, key)
		visited[key] = true
		selected = append(selected, d)
		return nil
	}
	if e := visit(tag); e != nil {
		return nil, nil, e
	}
	return selected, cfg["dns"], nil
}
func (m *Manager) ProbeOutbound(ctx context.Context, tag, target string) (map[string]any, error) {
	if len(tag) == 0 || len(tag) > 96 {
		return nil, errors.New("invalid outbound tag")
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	u, host, e := prepareProbeURL(ctx, target)
	if e != nil {
		return nil, e
	}
	return m.probeOutboundAt(ctx, tag, u, host)
}
func (m *Manager) probeOutboundAt(ctx context.Context, tag string, target *url.URL, hostname string) (map[string]any, error) {
	if !m.probeMu.TryLock() {
		return nil, errors.New("an outbound probe is already running")
	}
	defer m.probeMu.Unlock()
	m.mu.Lock()
	raw, e := os.ReadFile(m.ConfigPath)
	m.mu.Unlock()
	if e != nil {
		return nil, errors.New("core config unavailable")
	}
	outbounds, dns, e := probeOutbounds(raw, m.Core, tag)
	if e != nil {
		return nil, e
	}
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		return nil, e
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	config := map[string]any{}
	if m.Core == "singbox" {
		config = map[string]any{"log": map[string]any{"disabled": true}, "inbounds": []any{map[string]any{"type": "http", "tag": "vr-probe", "listen": "127.0.0.1", "listen_port": port}}, "outbounds": outbounds, "route": map[string]any{"final": tag}}
	} else {
		config = map[string]any{"log": map[string]any{"loglevel": "none"}, "inbounds": []any{map[string]any{"protocol": "http", "tag": "vr-probe", "listen": "127.0.0.1", "port": port}}, "outbounds": outbounds, "routing": map[string]any{"rules": []any{map[string]any{"type": "field", "inboundTag": []string{"vr-probe"}, "outboundTag": tag}}}}
	}
	if dns != nil {
		config["dns"] = dns
	}
	directory, e := os.MkdirTemp(filepath.Dir(m.ConfigPath), ".probe-")
	if e != nil {
		return nil, e
	}
	defer os.RemoveAll(directory)
	body, _ := json.Marshal(config)
	file := filepath.Join(directory, "config.json")
	if e = os.WriteFile(file, body, 0600); e != nil {
		return nil, e
	}
	args := []string{"run", "-config", file}
	if m.Core == "singbox" {
		args = []string{"run", "-c", file}
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, m.Binary, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if e = cmd.Start(); e != nil {
		return nil, errors.New("probe core could not start")
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	defer func() { _ = cmd.Process.Kill(); <-done }()
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	ready := false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 50*time.Millisecond)
		if err == nil {
			conn.Close()
			ready = true
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-done:
			return nil, errors.New("probe core rejected its isolated config")
		case <-time.After(20 * time.Millisecond):
		}
	}
	if !ready {
		return nil, errors.New("probe core did not become ready")
	}
	proxy, _ := url.Parse("http://" + address)
	transport := &http.Transport{Proxy: http.ProxyURL(proxy), TLSClientConfig: &tls.Config{ServerName: hostname, MinVersion: tls.VersionTLS12}, DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, e := http.NewRequestWithContext(ctx, "GET", target.String(), nil)
	if e != nil {
		return nil, e
	}
	req.Host = hostname
	if port := target.Port(); port != "" && port != "443" {
		req.Host = net.JoinHostPort(hostname, port)
	}
	req.Header.Set("User-Agent", "VeloRay-Probe/1.0")
	start := time.Now()
	response, e := client.Do(req)
	elapsed := time.Since(start).Milliseconds()
	result := map[string]any{"ok": false, "latency_ms": elapsed, "checked_at": time.Now().UTC().Format(time.RFC3339Nano), "status_code": 0}
	if e != nil {
		result["error"] = "outbound request failed"
		return result, nil
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	result["status_code"] = response.StatusCode
	result["ok"] = response.StatusCode >= 200 && response.StatusCode < 300
	if result["ok"] == false {
		result["error"] = fmt.Sprintf("HTTP status %d", response.StatusCode)
	}
	return result, nil
}
func (m *Manager) Diagnostics(ctx context.Context) map[string]any {
	status := m.Status(ctx)
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := []string{}
	for id := range m.state.Operations {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return m.state.Operations[ids[i]].At.After(m.state.Operations[ids[j]].At) })
	if len(ids) > 50 {
		ids = ids[:50]
	}
	operations := []any{}
	for _, id := range ids {
		op := m.state.Operations[id]
		operations = append(operations, map[string]any{"operation_id": id, "state": op.State, "at": op.At.UTC().Format(time.RFC3339Nano), "config_hash": op.After})
	}
	return map[string]any{"status": status, "core": m.Core, "operations": operations, "ledger_id": m.state.ID}
}
