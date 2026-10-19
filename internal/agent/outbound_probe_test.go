package agent

import (
	"context"
	"encoding/json"
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
