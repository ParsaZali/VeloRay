package agent

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestClientProbeCannotUseDirectFallbackOrHostEndpoints(t *testing.T) {
	for _, core := range []string{"xray", "singbox"} {
		m := &Manager{Core: core}
		if _, err := m.probeClientAt(context.Background(), []byte(`{"outbounds":[{"tag":"proxy","protocol":"freedom","type":"direct"}]}`), &url.URL{}, ""); err == nil {
			t.Fatal("direct route accepted as a client test")
		}
	}
	for _, endpoint := range []string{"127.0.0.1:443", "[::1]:443", "169.254.169.254:80", "192.168.1.1:443", "100.64.0.1:443"} {
		if _, err := resolveNativeEndpoint(context.Background(), endpoint); err == nil {
			t.Fatal("local diagnostic endpoint accepted", endpoint)
		}
	}
}
func TestClientProbeAuthenticatesThroughPinnedInbound(t *testing.T) {
	for _, core := range []string{"xray", "singbox"} {
		t.Run(core, func(t *testing.T) {
			binary := os.Getenv("VELORAY_TEST_XRAY_BINARY")
			if core == "singbox" {
				binary = os.Getenv("VELORAY_TEST_SINGBOX_BINARY")
			}
			if binary == "" {
				t.Skip("pinned core not provided")
			}
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
			defer target.Close()
			u, _ := url.Parse(target.URL)
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			port := listener.Addr().(*net.TCPAddr).Port
			listener.Close()
			const credential = "218fca39-8dc9-4c31-8947-d849f006bfb7"
			inbound := map[string]any{"tag": "real-inbound", "protocol": "vless", "listen": "127.0.0.1", "port": port, "settings": map[string]any{"decryption": "none", "clients": []any{map[string]any{"id": credential}}}}
			config := map[string]any{"log": map[string]any{"loglevel": "none"}, "inbounds": []any{inbound}, "outbounds": []any{map[string]any{"protocol": "freedom", "tag": "direct"}}}
			if core == "singbox" {
				config = map[string]any{"log": map[string]any{"disabled": true}, "inbounds": []any{map[string]any{"tag": "real-inbound", "type": "vless", "listen": "127.0.0.1", "listen_port": port, "users": []any{map[string]any{"name": "fixture", "uuid": credential}}}}, "outbounds": []any{map[string]any{"type": "direct", "tag": "direct"}}, "route": map[string]any{"final": "direct"}}
			}
			directory := t.TempDir()
			file := filepath.Join(directory, "server.json")
			raw, _ := json.Marshal(config)
			if err = os.WriteFile(file, raw, 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			args := []string{"run", "-config", file}
			if core == "singbox" {
				args = []string{"run", "-c", file}
			}
			command := exec.CommandContext(ctx, binary, args...)
			if err = command.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { cancel(); command.Wait() }()
			deadline := time.Now().Add(3 * time.Second)
			ready := false
			for time.Now().Before(deadline) {
				connection, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 50*time.Millisecond)
				if err == nil {
					connection.Close()
					ready = true
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			if !ready {
				if core == "singbox" && os.Getenv("VELORAY_TEST_NO_NETLINK") == "1" {
					t.Skip("sandbox denies sing-box networking; tested in CI")
				}
				t.Fatal("inbound did not start")
			}
			manager := &Manager{Core: core, Binary: binary, ConfigPath: filepath.Join(directory, "production-unchanged.json")}
			for _, valid := range []bool{true, false} {
				key := credential
				if !valid {
					key = "1dd69f00-3ff6-4478-90f6-111a711bfe23"
				}
				outbound := map[string]any{"protocol": "vless", "tag": "proxy", "settings": map[string]any{"vnext": []any{map[string]any{"address": "127.0.0.1", "port": port, "users": []any{map[string]any{"id": key, "encryption": "none"}}}}}}
				if core == "singbox" {
					outbound = map[string]any{"type": "vless", "tag": "proxy", "server": "127.0.0.1", "server_port": port, "uuid": key}
				}
				body, _ := json.Marshal(map[string]any{"outbounds": []any{outbound}})
				probeCtx, stop := context.WithTimeout(context.Background(), 8*time.Second)
				result, err := manager.probeClientAt(probeCtx, body, u, u.Hostname())
				stop()
				if err != nil {
					t.Fatal(err)
				}
				if result["ok"] != valid {
					t.Fatal("client credential bypass or valid connection failure", result, valid)
				}
			}
			if _, err = os.Stat(manager.ConfigPath); !os.IsNotExist(err) {
				t.Fatal("client test touched production config")
			}
		})
	}
}
