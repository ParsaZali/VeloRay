package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func nativeTestPort(t *testing.T) int {
	t.Helper()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
func nativeConfigFile(t *testing.T, cfg Data) string {
	t.Helper()
	raw, e := json.Marshal(cfg)
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if e = os.WriteFile(path, raw, 0600); e != nil {
		t.Fatal(e)
	}
	return path
}
func nativeRun(t *testing.T, binary, path string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	log, e := os.Create(filepath.Join(t.TempDir(), "core.log"))
	if e != nil {
		t.Fatal(e)
	}
	cmd := exec.CommandContext(ctx, binary, "run", "-c", path)
	cmd.Stdout, cmd.Stderr = log, log
	if e = cmd.Start(); e != nil {
		cancel()
		log.Close()
		t.Fatal(e)
	}
	t.Cleanup(func() { cancel(); _ = cmd.Wait(); _ = log.Close() })
}
func TestIntegrationNativeProtocolsWithPinnedSingBox(t *testing.T) {
	binary := os.Getenv("VELORAY_TEST_SINGBOX_BINARY")
	if binary == "" {
		t.Skip("set VELORAY_TEST_SINGBOX_BINARY")
	}
	for _, p := range []string{"tuic", "naive", "snell"} {
		t.Run(p, func(t *testing.T) {
			s, ctx := integration(t)
			n, i, c := fixture(t, s, ctx, &nodeMock{running: true, stats: []any{}})
			cert, key := testTLS(t)
			i["core"], i["protocol"], i["transport"], i["security"], i["listen"], i["port"] = "singbox", p, "raw", "tls", "127.0.0.1", nativeTestPort(t)
			if p == "snell" {
				i["security"] = "none"
				i["protocol_settings"] = Data{"psk": randomToken(32)}
			}
			i["tls_cert_file"], i["tls_key_file"], i["tls_server_name"] = cert, key, "vpn.example"
			if e := validateInbound(ctx, s.Store.Pool, s.Config, i); e != nil {
				t.Fatal(e)
			}
			if e := save(ctx, s.Store.Pool, "inbounds", i); e != nil {
				t.Fatal(e)
			}
			c["credential"] = ""
			c["protocol_settings"] = Data{}
			c["traffic_limit_bytes"], c["device_limit"] = 1024, 2
			if e := validateClient(ctx, s.Store.Pool, c); e != nil {
				t.Fatal(e)
			}
			if e := save(ctx, s.Store.Pool, "clients", c); e != nil {
				t.Fatal(e)
			}
			server, e := buildSingBoxConfig(ctx, s.Store.Pool, n)
			if e != nil {
				t.Fatal(e)
			}
			controller := nativeTestPort(t)
			obj(obj(server, "experimental"), "clash_api")["external_controller"] = fmt.Sprintf("127.0.0.1:%d", controller)
			obj(obj(server, "experimental"), "v2ray_api")["listen"] = fmt.Sprintf("127.0.0.1:%d", nativeTestPort(t))
			serverFile := nativeConfigFile(t, server)
			if out, e := exec.Command(binary, "check", "-c", serverFile).CombinedOutput(); e != nil {
				t.Fatal(e, string(out))
			}
			outbound, e := singClientOutbound(c, i, Data{"public_host": "127.0.0.1"})
			if e != nil {
				t.Fatal(e)
			}
			if p == "tuic" {
				clash, e := clashConfig(c, i, n)
				if e != nil || len(array(clash, "proxies")) != 1 || !strings.HasPrefix(shareLink(c, i, n), "tuic://") {
					t.Fatal(clash, e)
				}
			}
			// Naive outbound requires a client build with Chromium/cronet support. The
			// bundled CGO-free server supports Naive inbound; verify that boundary honestly.
			if p != "naive" && os.Getenv("VELORAY_TEST_NO_NETLINK") != "1" {
				if tls := obj(outbound, "tls"); len(tls) > 0 {
					tls["insecure"] = true
					outbound["tls"] = tls
				}
				clientPort := nativeTestPort(t)
				client := Data{"inbounds": []any{Data{"type": "http", "tag": "local", "listen": "127.0.0.1", "listen_port": clientPort}}, "outbounds": []any{outbound}, "route": Data{"final": str(outbound, "tag")}}
				clientFile := nativeConfigFile(t, client)
				if out, e := exec.Command(binary, "check", "-c", clientFile).CombinedOutput(); e != nil {
					t.Fatal(e, string(out))
				}
				nativeRun(t, binary, serverFile)
				nativeRun(t, binary, clientFile)
				proxy, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", clientPort))
				transport := &http.Transport{Proxy: http.ProxyURL(proxy)}
				defer transport.CloseIdleConnections()
				httpClient := &http.Client{Transport: transport, Timeout: time.Second}
				hold := make(chan struct{})
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(200)
					_, _ = w.Write([]byte("veloray-native-ok"))
					w.(http.Flusher).Flush()
					<-hold
				}))
				defer upstream.Close()
				defer close(hold)
				var response *http.Response
				for attempt := 0; attempt < 50; attempt++ {
					response, e = httpClient.Get(upstream.URL)
					if e == nil {
						break
					}
					time.Sleep(40 * time.Millisecond)
				}
				if e != nil {
					t.Fatal("native proxy round trip", e)
				}
				defer response.Body.Close()
				body := make([]byte, len("veloray-native-ok"))
				if _, e = io.ReadFull(response.Body, body); e != nil || string(body) != "veloray-native-ok" {
					t.Fatal(string(body), e)
				}
				// The engine must attribute the live connection to the panel principal used
				// by traffic accounting and concurrent-IP rules, not merely open a port.
				telemetry, e := http.Get(fmt.Sprintf("http://127.0.0.1:%d/connections", controller))
				if e != nil {
					t.Fatal(e)
				}
				raw, _ := io.ReadAll(telemetry.Body)
				telemetry.Body.Close()
				if !strings.Contains(string(raw), fmt.Sprintf("veloray:%d:alice", num(c, "id"))) {
					t.Fatal("principal missing from live telemetry", string(raw))
				}
			}
			c["enabled"] = false
			if e = save(ctx, s.Store.Pool, "clients", c); e != nil {
				t.Fatal(e)
			}
			empty, e := buildSingBoxConfig(ctx, s.Store.Pool, n)
			if e != nil || len(array(empty, "inbounds")) != 0 {
				t.Fatal("disabled protocol left a shared-password listener", empty, e)
			}
		})
	}
}
