package agent

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"time"

	"github.com/PaarSaAm/VeloRay/internal/nativecore"
)

type nativeClientProbe struct {
	Protocol string                      `json:"protocol"`
	Endpoint string                      `json:"endpoint,omitempty"`
	Secret   string                      `json:"secret,omitempty"`
	AWG      *nativecore.AWGClientConfig `json:"awg,omitempty"`
}

func resolveNativeEndpoint(ctx context.Context, endpoint string) (netip.AddrPort, error) {
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil {
		return netip.AddrPort{}, errors.New("invalid native diagnostic endpoint")
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return netip.AddrPort{}, errors.New("native server DNS lookup failed")
	}
	for _, address := range addresses {
		if publicProbeIP(address.IP) {
			value, err := netip.ParseAddrPort(net.JoinHostPort(address.IP.String(), port))
			if err == nil && value.Port() > 0 {
				return value, nil
			}
		}
	}
	return netip.AddrPort{}, errors.New("native diagnostic server must have a public IP address")
}
func (m *Manager) ProbeNativeClient(ctx context.Context, raw []byte, target string) (map[string]any, error) {
	if m.native == nil {
		return nil, errors.New("native runtime unavailable")
	}
	if !m.probeMu.TryLock() {
		return nil, errors.New("a native connection test is already running")
	}
	defer m.probeMu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if len(raw) > 65536 {
		return nil, errors.New("native diagnostic config exceeds 64 KiB")
	}
	var config nativeClientProbe
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&config) != nil || decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("invalid native diagnostic config")
	}
	start := time.Now()
	result := map[string]any{"ok": false, "network_tested": true, "vantage": "node", "traffic_counts": true, "status_code": 0, "checked_at": start.UTC().Format(time.RFC3339Nano)}
	if config.Protocol == "mtproto" {
		if config.AWG != nil {
			return nil, errors.New("unexpected AmneziaWG configuration")
		}
		endpoint, err := resolveNativeEndpoint(ctx, config.Endpoint)
		if err != nil {
			return nil, err
		}
		err = nativecore.ProbeMTProto(ctx, endpoint.String(), config.Secret)
		result["latency_ms"] = time.Since(start).Milliseconds()
		result["stage"] = "telegram_res_pq"
		result["ok"] = err == nil
		if err != nil {
			result["error"] = err.Error()
		}
		return result, nil
	}
	if config.Protocol != "amneziawg" || config.AWG == nil || config.Endpoint != "" || config.Secret != "" {
		return nil, errors.New("invalid native probe protocol")
	}
	endpoint, err := resolveNativeEndpoint(ctx, config.AWG.Endpoint)
	if err != nil {
		return nil, err
	}
	u, hostname, err := prepareProbeURL(ctx, target)
	if err != nil {
		return nil, err
	}
	// The current managed peer has one address family. Resolve the test target
	// in that family so an unrelated AAAA preference cannot create a false failure.
	prefix, err := netip.ParsePrefix(config.AWG.Address)
	if err != nil {
		return nil, errors.New("invalid AmneziaWG client address")
	}
	selected := net.ParseIP(u.Hostname())
	if (selected.To4() != nil) != prefix.Addr().Is4() {
		addresses, lookupErr := net.DefaultResolver.LookupIPAddr(ctx, hostname)
		if lookupErr != nil {
			return nil, errors.New("test destination DNS failed")
		}
		found := false
		for _, value := range addresses {
			if publicProbeIP(value.IP) && (value.IP.To4() != nil) == prefix.Addr().Is4() {
				u.Host = net.JoinHostPort(value.IP.String(), u.Port())
				found = true
				break
			}
		}
		if !found {
			return nil, errors.New("test destination does not support this peer address family")
		}
	}
	client, err := nativecore.NewAWGClient(*config.AWG, endpoint)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	transport := &http.Transport{Proxy: nil, DialContext: client.DialContext, TLSClientConfig: &tls.Config{ServerName: hostname, MinVersion: tls.VersionTLS12}, DisableKeepAlives: true}
	result, err = performProbeRequest(ctx, u, hostname, transport)
	if result != nil {
		result["network_tested"] = true
		result["vantage"] = "node"
		result["traffic_counts"] = true
		if result["ok"] == false {
			result["error"] = "AmneziaWG tunnel or destination request failed"
		}
	}
	return result, err
}
