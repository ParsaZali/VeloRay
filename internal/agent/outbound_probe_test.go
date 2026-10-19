package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProbeTargetPolicyAndDependencies(t *testing.T) {
	for _, raw := range []string{"http://example.com", "https://localhost", "https://127.0.0.1", "https://169.254.169.254/metadata", "https://[::1]", "https://user:password@example.com", "https://example.com/#fragment"} {
		if _, _, e := prepareProbeURL(context.Background(), raw); e == nil {
			t.Fatal("unsafe probe", raw)
		}
	}
	raw := []byte(`{"outbounds":[{"tag":"chain","protocol":"http","proxySettings":{"tag":"direct"}},{"tag":"direct","protocol":"freedom"},{"tag":"ignored","protocol":"wireguard"}]}`)
	entries, _, e := probeOutbounds(raw, "xray", "chain")
	if e != nil || len(entries) != 2 {
		t.Fatal(entries, e)
	}
	for _, raw := range []string{`{"outbounds":[{"tag":"x","type":"selector"}]}`, `{"outbounds":[{"tag":"x","type":"direct","detour":"x"}]}`, `{"outbounds":[{"tag":"x","protocol":"wireguard"}]}`} {
		if _, _, e := probeOutbounds([]byte(raw), "singbox", "x"); e == nil {
			t.Fatal("unsafe outbound accepted")
		}
	}
}
func TestOutboundProbeUsesSelectedPinnedCoreAndCleansTemporaryFiles(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer target.Close()
	u, _ := url.Parse(target.URL)
	for _, core := range []string{"xray", "singbox"} {
		t.Run(core, func(t *testing.T) {
			binary := os.Getenv("VELORAY_TEST_XRAY_BINARY")
			if core == "singbox" {
				binary = os.Getenv("VELORAY_TEST_SINGBOX_BINARY")
			}
			if binary == "" {
				t.Skip("pinned runtime not provided")
			}
			directory := t.TempDir()
			config := filepath.Join(directory, "config.json")
			entries := []any{map[string]any{"tag": "working", "protocol": "freedom"}, map[string]any{"tag": "broken", "protocol": "blackhole"}}
			if core == "singbox" {
				entries = []any{map[string]any{"tag": "working", "type": "direct"}, map[string]any{"tag": "broken", "type": "http", "server": "127.0.0.1", "server_port": 1}}
			}
			raw, _ := json.Marshal(map[string]any{"outbounds": entries})
			if e := os.WriteFile(config, raw, 0600); e != nil {
				t.Fatal(e)
			}
			m := &Manager{Core: core, Binary: binary, ConfigPath: config}
			for _, tag := range []string{"working", "broken"} {
				ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
				result, e := m.probeOutboundAt(ctx, tag, u, u.Hostname())
				cancel()
				if e != nil {
					if core == "singbox" && os.Getenv("VELORAY_TEST_NO_NETLINK") == "1" && e.Error() == "probe core rejected its isolated config" {
						t.Skip("sandbox denies sing-box netlink; real round trip runs in CI")
					}
					t.Fatal(e)
				}
				if tag == "working" && result["ok"] != true {
					if os.Getenv("VELORAY_TEST_NO_NETLINK") == "1" {
						t.Skip("sandbox denies core networking; real round trip runs in CI")
					}
					t.Fatal("direct route failed", result)
				}
				if tag == "broken" && result["ok"] != false {
					t.Fatal("selected failed route was bypassed", result)
				}
			}
			fresh, _ := os.ReadFile(config)
			if string(fresh) != string(raw) {
				t.Fatal("probe modified running config")
			}
			files, _ := os.ReadDir(directory)
			for _, file := range files {
				if strings.HasPrefix(file.Name(), ".probe-") {
					t.Fatal("probe files leaked")
				}
			}
		})
	}
}

func TestIsolatedProbeRejectsUnauthenticatedLocalProxyUse(t *testing.T) {
	for _, core := range []string{"xray", "singbox"} {
		t.Run(core, func(t *testing.T) {
			binary := os.Getenv("VELORAY_TEST_XRAY_BINARY")
			if core == "singbox" {
				binary = os.Getenv("VELORAY_TEST_SINGBOX_BINARY")
			}
			if binary == "" {
				t.Skip("pinned runtime not provided")
			}
			started, release := make(chan struct{}, 1), make(chan struct{})
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				select {
				case started <- struct{}{}:
				default:
				}
				<-release
				w.WriteHeader(204)
			}))
			defer target.Close()
			defer close(release)
			directory := t.TempDir()
			config := filepath.Join(directory, "config.json")
			outbound := map[string]any{"tag": "direct", "protocol": "freedom"}
			if core == "singbox" {
				outbound = map[string]any{"tag": "direct", "type": "direct"}
			}
			raw, _ := json.Marshal(map[string]any{"outbounds": []any{outbound}})
			if e := os.WriteFile(config, raw, 0600); e != nil {
				t.Fatal(e)
			}
			m := &Manager{Core: core, Binary: binary, ConfigPath: config}
			u, _ := url.Parse(target.URL)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				result, e := m.probeOutboundAt(ctx, "direct", u, u.Hostname())
				if e == nil && result["ok"] != true {
					e = fmt.Errorf("authenticated probe failed")
				}
				done <- e
			}()
			select {
			case <-started:
			case e := <-done:
				if core == "singbox" && os.Getenv("VELORAY_TEST_NO_NETLINK") == "1" && e != nil {
					t.Skip("sandbox denies sing-box netlink; real round trip runs in CI")
				}
				t.Fatal("probe did not reach target", e)
			case <-ctx.Done():
				t.Fatal("probe did not start")
			}
			files, e := filepath.Glob(filepath.Join(directory, ".probe-*", "config.json"))
			if e != nil || len(files) != 1 {
				t.Fatal(files, e)
			}
			body, e := os.ReadFile(files[0])
			if e != nil {
				t.Fatal(e)
			}
			var transient struct {
				Inbounds []struct {
					Port       int `json:"port"`
					ListenPort int `json:"listen_port"`
				} `json:"inbounds"`
			}
			if e = json.Unmarshal(body, &transient); e != nil || len(transient.Inbounds) != 1 {
				t.Fatal(e)
			}
			port := transient.Inbounds[0].Port
			if core == "singbox" {
				port = transient.Inbounds[0].ListenPort
			}
			proxy, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
			transport := &http.Transport{Proxy: http.ProxyURL(proxy)}
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport, Timeout: time.Second}
			response, e := client.Get(target.URL)
			if e != nil {
				t.Fatal("unauthenticated request was not rejected", e)
			}
			response.Body.Close()
			if response.StatusCode != 407 {
				t.Fatal("temporary proxy allowed local unauthenticated use", response.StatusCode)
			}
			// Release the authenticated check only after verifying the independent
			// local request cannot borrow its outbound credentials.
			release <- struct{}{}
			if e = <-done; e != nil {
				t.Fatal(e)
			}
		})
	}
}
