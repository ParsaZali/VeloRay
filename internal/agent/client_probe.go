package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"time"
)

// Client probes run the generated client-side outbound, never the production
// core's direct route. A successful response therefore crosses the real inbound.
func (m *Manager) ProbeClient(ctx context.Context, raw []byte, target string) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	u, host, err := prepareProbeURL(ctx, target)
	if err != nil {
		return nil, err
	}
	return m.probeClientAt(ctx, raw, u, host)
}
func (m *Manager) probeClientAt(ctx context.Context, raw []byte, target *url.URL, hostname string) (map[string]any, error) {
	if m.Core == "native" {
		return nil, errors.New("native client requires a protocol-specific probe")
	}
	if !m.probeMu.TryLock() {
		return nil, errors.New("a connection test is already running on this core")
	}
	defer m.probeMu.Unlock()
	if len(raw) > 65536 {
		return nil, errors.New("client test configuration exceeds 64 KiB")
	}
	var config map[string]json.RawMessage
	if json.Unmarshal(raw, &config) != nil {
		return nil, errors.New("invalid client test configuration")
	}
	for key := range config {
		if key != "outbounds" && key != "dns" {
			return nil, errors.New("client test accepts only client outbounds and DNS")
		}
	}
	entries, dns, err := probeOutbounds(raw, m.Core, "proxy")
	if err != nil {
		return nil, err
	}
	if len(entries) > 16 {
		return nil, errors.New("too many client test dependencies")
	}
	allowed := map[string]bool{"vless": true, "vmess": true, "trojan": true, "shadowsocks": true, "http": true, "socks": true, "hysteria2": true, "tuic": true, "snell": true, "shadowtls": true}
	for _, value := range entries {
		entry, _ := value.(map[string]any)
		protocol, _ := entry["protocol"].(string)
		if m.Core == "singbox" {
			protocol, _ = entry["type"].(string)
		}
		if !allowed[protocol] {
			return nil, errors.New("this client protocol is unavailable in the isolated probe core")
		}
	}
	result, err := m.probeGraphAt(ctx, "proxy", entries, dns, target, hostname)
	if result != nil {
		result["network_tested"] = true
		result["vantage"] = "node"
		result["traffic_counts"] = true
		if result["ok"] == false {
			result["error"] = "client connection or destination request failed"
		}
	}
	return result, err
}
