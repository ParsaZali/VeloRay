package panel

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestIntegrationBalancingAtomicDeploymentAndDependencies(t *testing.T) {
	s, ctx := integration(t)
	m := &nodeMock{running: true, stats: []any{}}
	n, _, customer := fixture(t, s, ctx, m)
	a := Actor{User: Data{"is_superuser": true}, Scope: "admin"}
	path := fmt.Sprintf("nodes/%d/balancing", num(n, "id"))
	getView := func() Data {
		t.Helper()
		out, code, err := managementRequest(t, s, ctx, a, "GET", path, nil)
		if err != nil || code != 200 {
			t.Fatal(out, code, err)
		}
		return out.(Data)
	}
	initial := getView()
	for _, actor := range []Actor{{User: Data{"role": "reseller"}, Scope: "admin"}, {User: Data{"is_superuser": true}, Scope: "client"}} {
		if _, _, err := managementRequest(t, s, ctx, actor, "GET", path, nil); err == nil {
			t.Fatal("unauthorized balancing read")
		}
	}
	readOnly := a
	readOnly.Scope = "read"
	if _, _, err := managementRequest(t, s, ctx, readOnly, "PUT", path, Data{"revision": str(initial, "revision"), "balancers": []any{}}); err == nil {
		t.Fatal("read key wrote balancing")
	}
	// Profile-owned outbounds and policy must participate in final references.
	profile := Data{"name": "shared", "body": Data{"xray": Data{"outbounds": []any{Data{"tag": "direct", "protocol": "freedom"}, Data{"tag": "transit-a", "protocol": "freedom"}}, "dns": Data{"servers": []any{"1.1.1.1"}}}, "singbox": Data{}, "snippet_ids": []any{}}}
	out, _, err := managementRequest(t, s, ctx, a, "POST", "profiles", profile)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = managementRequest(t, s, ctx, a, "PATCH", fmt.Sprintf("nodes/%d", num(n, "id")), Data{"profile_id": num(out.(Data), "id"), "config_patch": Data{"log": Data{"loglevel": "error"}}}); err != nil {
		t.Fatal(err)
	}
	view := getView()
	groups := []any{Data{"tag": "pool", "selector": []any{"transit-"}, "fallbackTag": "direct", "strategy": Data{"type": "roundRobin"}}}
	if _, _, err = managementRequest(t, s, ctx, a, "PUT", path, Data{"revision": str(initial, "revision"), "balancers": groups}); err == nil {
		t.Fatal("accepted stale revision")
	}
	if _, _, err = managementRequest(t, s, ctx, a, "PUT", path, Data{"revision": str(view, "revision"), "balancers": groups}); err != nil {
		t.Fatal(err)
	}
	fresh, _ := get(ctx, s.Store.Pool, "nodes", num(n, "id"))
	if str(obj(obj(fresh, "config_patch"), "log"), "loglevel") != "error" {
		t.Fatal("unrelated node settings overwritten")
	}
	if len(array(obj(m.config, "routing"), "balancers")) != 1 || len(array(obj(m.config, "dns"), "servers")) != 1 {
		t.Fatal("profile/deployment merge lost", m.config)
	}
	rule := Data{"name": "pool-rule", "node": num(n, "id"), "core": "xray", "priority": 10, "enabled": true, "config": Data{"network": "tcp", "balancerTag": "pool"}}
	out, _, err = managementRequest(t, s, ctx, a, "POST", "routing_rules", rule)
	if err != nil {
		t.Fatal(err)
	}
	ruleID := num(out.(Data), "id")
	view = getView()
	if _, _, err = managementRequest(t, s, ctx, a, "PUT", path, Data{"revision": str(view, "revision"), "balancers": []any{}}); err == nil {
		t.Fatal("removed referenced balancer")
	}
	if str(getView(), "revision") != str(view, "revision") {
		t.Fatal("failed mutation committed")
	}
	if _, _, err = managementRequest(t, s, ctx, a, "PATCH", fmt.Sprintf("nodes/%d", num(n, "id")), Data{"config_patch": Data{"outbounds": []any{Data{"tag": "direct", "protocol": "freedom"}}, "routing": Data{"balancers": groups}}}); err == nil {
		t.Fatal("removed selected profile outbound")
	}
	if _, _, err = managementRequest(t, s, ctx, a, "DELETE", fmt.Sprintf("routing_rules/%d", ruleID), nil); err != nil {
		t.Fatal(err)
	}
	m.failApply = true
	if _, _, err = managementRequest(t, s, ctx, a, "PUT", path, Data{"revision": str(view, "revision"), "balancers": []any{}}); err == nil {
		t.Fatal("failed deployment accepted")
	}
	m.failApply = false
	if str(getView(), "revision") != str(view, "revision") {
		t.Fatal("deployment failure committed settings")
	}
	c, _ := get(ctx, s.Store.Pool, "clients", num(customer, "id"))
	before, _ := json.Marshal(customer)
	after, _ := json.Marshal(c)
	if string(before) != string(after) {
		t.Fatal("balancing changed customer accounting")
	}
}

func TestInboundFallbackValidationAndSettings(t *testing.T) {
	base := Data{"core": "xray", "protocol": "vless", "transport": "raw", "security": "tls"}
	for _, dest := range []any{8080, "8080", "127.0.0.1:8080", "[::1]:8080", "web.example:80", "/run/web.sock", "@web"} {
		in := clone(base)
		in["protocol_settings"] = Data{"fallbacks": []any{Data{"dest": dest}}}
		if err := validateInboundFallbacks(in); err != nil {
			t.Fatal(dest, err)
		}
		for _, protocol := range []string{"vless", "trojan"} {
			in["protocol"] = protocol
			settings, err := inboundSettings(in, Data{}, []Data{})
			if err != nil || len(array(settings, "fallbacks")) != 1 {
				t.Fatal(settings, err)
			}
		}
	}
	for _, bad := range []any{nil, "bad", []any{1}, []any{Data{"dest": 0}}, []any{Data{"dest": 1.5}}, []any{Data{"dest": "https://example.com"}}, []any{Data{"dest": "localhost:99999"}}, []any{Data{"dest": "localhost:80\r\n"}}, []any{Data{"dest": 80, "xver": "1"}}, []any{Data{"dest": 80, "xver": 3}}, []any{Data{"dest": 80, "path": "bad"}}, []any{Data{"dest": 80, "path": "/", "alpn": "h2"}}, []any{Data{"dest": 80}, Data{"dest": 81}}} {
		in := clone(base)
		in["protocol_settings"] = Data{"fallbacks": bad}
		if err := validateInboundFallbacks(in); err == nil {
			t.Fatal("accepted invalid fallback", bad)
		}
	}
	for _, patch := range []Data{{"core": "singbox"}, {"protocol": "vmess"}, {"transport": "ws"}, {"security": "reality"}} {
		in := merge(clone(base), patch)
		in["protocol_settings"] = Data{"fallbacks": []any{Data{"dest": 80}}}
		if err := validateInboundFallbacks(in); err == nil {
			t.Fatal("accepted unsupported fallback", patch)
		}
	}
}

func TestXrayBalancingReferences(t *testing.T) {
	base := Data{"outbounds": []any{Data{"tag": "direct"}, Data{"tag": "transit-a"}, Data{"tag": "transit-b"}}, "routing": Data{"balancers": []any{Data{"tag": "pool", "selector": []any{"transit-"}, "strategy": Data{"type": "roundRobin"}, "fallbackTag": "direct"}}, "rules": []any{Data{"balancerTag": "pool", "network": "tcp"}}}}
	if err := validateXrayRouting(base); err != nil {
		t.Fatal(err)
	}
	for _, edit := range []func(Data){
		func(c Data) { obj(c, "routing")["balancers"] = []any{} },
		func(c Data) { c["outbounds"] = []any{Data{"tag": "direct"}} },
		func(c Data) {
			asData(array(obj(c, "routing"), "balancers")[0])["selector"] = []any{"transit-", "missing"}
		},
		func(c Data) {
			asData(array(obj(c, "routing"), "balancers")[0])["selector"] = []any{"transit-", "transit-"}
		},
		func(c Data) { asData(array(obj(c, "routing"), "balancers")[0])["fallbackTag"] = "missing" },
		func(c Data) { asData(array(obj(c, "routing"), "balancers")[0])["strategy"] = Data{"type": "leastPing"} },
		func(c Data) { asData(array(obj(c, "routing"), "rules")[0])["outboundTag"] = "direct" },
	} {
		c := clone(base)
		edit(c)
		if err := validateXrayRouting(c); err == nil {
			t.Fatal("accepted broken balancing", c)
		}
	}
	changed := clone(base)
	obj(changed, "routing")["balancers"] = []any{}
	if str(balancingView(base), "revision") == str(balancingView(changed), "revision") {
		t.Fatal("revision did not change")
	}
}

func waitNetworkListener(t *testing.T, port int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("core did not open listener")
}

func TestXrayFallbacksAndRoundRobinWithPinnedCore(t *testing.T) {
	binary := os.Getenv("VELORAY_TEST_XRAY_BINARY")
	if binary == "" {
		t.Skip("set VELORAY_TEST_XRAY_BINARY")
	}
	t.Run("strategy-configurations", func(t *testing.T) {
		for _, strategy := range []string{"random", "roundRobin", "leastPing", "leastLoad"} {
			cfg := Data{"outbounds": []any{Data{"protocol": "freedom", "tag": "transit-a"}}, "observatory": Data{"subjectSelector": []any{"transit-"}, "probeURL": "https://example.com/", "probeInterval": "1m"}, "routing": Data{"balancers": []any{Data{"tag": "pool", "selector": []any{"transit-"}, "strategy": Data{"type": strategy}}}, "rules": []any{Data{"type": "field", "network": "tcp", "balancerTag": "pool"}}}}
			if err := validateXrayRouting(cfg); err != nil {
				t.Fatal(strategy, err)
			}
			if raw, err := exec.Command(binary, "run", "-test", "-c", nativeConfigFile(t, cfg)).CombinedOutput(); err != nil {
				t.Fatal(strategy, err, string(raw))
			}
		}
	})
	web := func(label string) *httptest.Server {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, label) }))
		t.Cleanup(server.Close)
		return server
	}
	a, b := web("A"), web("B")
	for _, protocol := range []string{"vless", "trojan"} {
		t.Run(protocol, func(t *testing.T) {
			cert, key := testTLS(t)
			port := nativeTestPort(t)
			in := Data{"protocol": protocol, "core": "xray", "transport": "raw", "security": "tls", "tls_cert_file": cert, "tls_key_file": key, "tls_server_name": "vpn.example", "protocol_settings": Data{"fallbacks": []any{Data{"dest": strings.TrimPrefix(a.URL, "http://")}, Data{"name": "vpn.example", "alpn": "http/1.1", "dest": strings.TrimPrefix(a.URL, "http://")}, Data{"name": "vpn.example", "alpn": "http/1.1", "path": "/special", "dest": strings.TrimPrefix(b.URL, "http://")}}}}
			if err := validateInboundFallbacks(in); err != nil {
				t.Fatal(err)
			}
			settings, err := inboundSettings(in, Data{}, []Data{{"credential": "11111111-1111-4111-8111-111111111111", "name": "test", "id": 1}})
			if err != nil {
				t.Fatal(err)
			}
			cfg := Data{"inbounds": []any{Data{"listen": "127.0.0.1", "port": port, "protocol": protocol, "settings": settings, "streamSettings": streamSettings(in, Data{})}}, "outbounds": []any{Data{"protocol": "freedom", "tag": "direct"}}}
			file := nativeConfigFile(t, cfg)
			if raw, err := exec.Command(binary, "run", "-test", "-c", file).CombinedOutput(); err != nil {
				t.Fatal(err, string(raw))
			}
			nativeRun(t, binary, file)
			waitNetworkListener(t, port)
			pem, _ := os.ReadFile(cert)
			roots := x509.NewCertPool()
			roots.AppendCertsFromPEM(pem)
			transport := &http.Transport{Proxy: nil, DisableKeepAlives: true, TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "vpn.example", NextProtos: []string{"http/1.1"}}}
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
			for path, want := range map[string]string{"/normal": "A", "/special": "B"} {
				resp, err := client.Get(fmt.Sprintf("https://127.0.0.1:%d%s", port, path))
				if err != nil {
					t.Fatal(err)
				}
				raw, err := io.ReadAll(resp.Body)
				resp.Body.Close()
				if err != nil || string(raw) != want {
					t.Fatal(path, string(raw), err)
				}
			}
		})
	}
	t.Run("round-robin", func(t *testing.T) {
		port := nativeTestPort(t)
		cfg := Data{"inbounds": []any{Data{"listen": "127.0.0.1", "port": port, "protocol": "http", "settings": Data{}}}, "outbounds": []any{Data{"protocol": "blackhole", "tag": "blocked"}, Data{"protocol": "freedom", "tag": "transit-a", "settings": Data{"redirect": strings.TrimPrefix(a.URL, "http://")}}, Data{"protocol": "freedom", "tag": "transit-b", "settings": Data{"redirect": strings.TrimPrefix(b.URL, "http://")}}}, "routing": Data{"balancers": []any{Data{"tag": "pool", "selector": []any{"transit-"}, "strategy": Data{"type": "roundRobin"}}}, "rules": []any{Data{"type": "field", "network": "tcp", "balancerTag": "pool"}}}}
		if err := validateXrayRouting(cfg); err != nil {
			t.Fatal(err)
		}
		file := nativeConfigFile(t, cfg)
		nativeRun(t, binary, file)
		waitNetworkListener(t, port)
		proxy, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
		transport := &http.Transport{Proxy: http.ProxyURL(proxy), DisableKeepAlives: true}
		defer transport.CloseIdleConnections()
		client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
		counts := map[string]int{}
		for range 8 {
			resp, err := client.Get("http://192.0.2.1/check")
			if err != nil {
				t.Fatal(err)
			}
			raw, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			counts[string(raw)]++
		}
		if counts["A"] != 4 || counts["B"] != 4 {
			t.Fatal("connections did not use both selected outbounds equally", counts)
		}
	})
}
