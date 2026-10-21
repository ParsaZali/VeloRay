package panel

import (
	"bytes"
	"crypto/tls"
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

func TestShadowTLSValidationAndExport(t *testing.T) {
	i := Data{"id": 0, "name": "ST", "core": "singbox", "protocol": "shadowtls", "transport": "raw", "security": "none", "listen": "0.0.0.0", "port": 443, "protocol_settings": Data{"handshake_server": "tls.example"}}
	if err := validateShadowTLS(i); err != nil || !validShadowTLSKey(str(obj(i, "protocol_settings"), "psk")) {
		t.Fatal(i, err)
	}
	for _, patch := range []Data{{"core": "xray"}, {"transport": "ws"}, {"security": "tls"}, {"protocol_settings": Data{"handshake_server": "https://tls.example"}}, {"protocol_settings": Data{"handshake_server": "tls.example", "handshake_port": 65536}}, {"protocol_settings": Data{"handshake_server": "tls.example", "psk": "bad"}}} {
		bad := clone(i)
		for k, v := range patch {
			bad[k] = v
		}
		if validateShadowTLS(bad) == nil {
			t.Fatal("invalid ShadowTLS accepted", bad)
		}
	}
	c := Data{"id": 21, "name": "alice", "credential": shadowTLSKey(), "protocol_settings": Data{"shadowtls_password": strings.Repeat("s", 32)}}
	n := Data{"public_host": "vpn.example"}
	graph, err := singClientOutbounds(c, i, n)
	if err != nil || len(graph) != 2 {
		t.Fatal(graph, err)
	}
	inner, outer := graph[0].(Data), graph[1].(Data)
	if str(inner, "detour") != str(outer, "tag") || str(inner, "password") != str(obj(i, "protocol_settings"), "psk")+":"+str(c, "credential") || flag(obj(outer, "tls"), "insecure") || str(obj(outer, "tls"), "server_name") != "tls.example" {
		t.Fatal(graph)
	}
	clash, err := clashConfig(c, i, n)
	if err != nil {
		t.Fatal(err)
	}
	proxy := array(clash, "proxies")[0].(Data)
	if flag(proxy, "udp") || str(proxy, "plugin") != "shadow-tls" || num(obj(proxy, "plugin-opts"), "version") != 3 {
		t.Fatal(proxy)
	}
	if shareLink(c, i, n) != "" {
		t.Fatal("unverified portable URI advertised")
	}
	accounts := []any{Data{"name": "veloray:21:alice", "password": str(c, "credential")}}
	inbounds := shadowTLSInbounds(i, accounts, []Data{c})
	if len(inbounds) != 2 || str(inbounds[0].(Data), "listen") != "127.0.0.1" || num(inbounds[0].(Data), "listen_port") != 0 || str(inbounds[1].(Data), "detour") != str(inbounds[0].(Data), "tag") || !flag(inbounds[1].(Data), "strict_mode") || len(shadowTLSInbounds(i, nil, nil)) != 0 {
		t.Fatal(inbounds)
	}
	// Validate the full graph with the exact runtime that ships to operators.
	if binary := os.Getenv("VELORAY_TEST_SINGBOX_BINARY"); binary != "" {
		for _, cfg := range []Data{{"inbounds": inbounds, "outbounds": []any{Data{"type": "direct", "tag": "direct"}}}, {"outbounds": graph, "route": Data{"final": str(inner, "tag")}}} {
			if out, e := exec.Command(binary, "check", "-c", nativeConfigFile(t, cfg)).CombinedOutput(); e != nil {
				t.Fatal(e, string(out))
			}
		}
	}
}

func TestIntegrationShadowTLSWithPinnedSingBox(t *testing.T) {
	binary := os.Getenv("VELORAY_TEST_SINGBOX_BINARY")
	if binary == "" {
		t.Skip("set VELORAY_TEST_SINGBOX_BINARY")
	}
	s, ctx := integration(t)
	n, i, c := fixture(t, s, ctx, &nodeMock{running: true, stats: []any{}})
	cert, key := testTLS(t)
	pair, err := tls.LoadX509KeyPair(cert, key)
	if err != nil {
		t.Fatal(err)
	}
	mask := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("TLS handshake fixture")) }))
	mask.TLS = &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13}
	mask.StartTLS()
	defer mask.Close()
	i["core"], i["protocol"], i["transport"], i["security"], i["listen"], i["port"] = "singbox", "shadowtls", "raw", "none", "127.0.0.1", nativeTestPort(t)
	i["protocol_settings"] = Data{"handshake_server": "127.0.0.1", "handshake_port": mask.Listener.Addr().(*net.TCPAddr).Port, "psk": shadowTLSKey()}
	if err = validateInbound(ctx, s.Store.Pool, s.Config, i); err != nil {
		t.Fatal(err)
	}
	if err = save(ctx, s.Store.Pool, "inbounds", i); err != nil {
		t.Fatal(err)
	}
	c["credential"], c["protocol_settings"], c["traffic_limit_bytes"], c["device_limit"] = "", Data{}, 1<<30, 2
	if err = validateClient(ctx, s.Store.Pool, c); err != nil {
		t.Fatal(err)
	}
	if !validShadowTLSKey(str(c, "credential")) || str(obj(c, "protocol_settings"), "shadowtls_password") == "" {
		t.Fatal("credentials not generated", c)
	}
	if err = save(ctx, s.Store.Pool, "clients", c); err != nil {
		t.Fatal(err)
	}
	second := defaults("clients")
	second["name"], second["inbound"], second["enabled"] = "bob", num(i, "id"), true
	if err = validateClient(ctx, s.Store.Pool, second); err != nil {
		t.Fatal(err)
	}
	if err = save(ctx, s.Store.Pool, "clients", second); err != nil {
		t.Fatal(err)
	}
	feed := httptest.NewRecorder()
	s.subscription(feed, httptest.NewRequest("GET", "/sub/"+str(c, "subscription_token")+"?format=singbox", nil))
	var exported Data
	if feed.Code != 200 || json.Unmarshal(feed.Body.Bytes(), &exported) != nil || len(array(exported, "outbounds")) != 4 {
		t.Fatal("subscription omitted the helper graph", feed.Code, feed.Body.String())
	}
	server, err := buildSingBoxConfig(ctx, s.Store.Pool, n)
	if err != nil {
		t.Fatal(err)
	}
	if len(array(server, "inbounds")) != 2 || len(array(asData(array(server, "inbounds")[0]), "users")) != 2 {
		t.Fatal(server)
	}
	controller, stats := nativeTestPort(t), fmt.Sprintf("127.0.0.1:%d", nativeTestPort(t))
	obj(obj(server, "experimental"), "clash_api")["external_controller"] = fmt.Sprintf("127.0.0.1:%d", controller)
	obj(obj(server, "experimental"), "v2ray_api")["listen"] = stats
	path := nativeConfigFile(t, server)
	if out, e := exec.Command(binary, "check", "-c", path).CombinedOutput(); e != nil {
		t.Fatal(e, string(out))
	}
	if os.Getenv("VELORAY_TEST_NO_NETLINK") != "1" {
		nativeRun(t, binary, path)
		for _, account := range []Data{c, second} {
			shadowTLSRoundTrip(t, binary, cert, account, i, controller, stats)
		}
	}
	// Expiry/quota/disable all pass through active(), removing both layers.
	c["enabled"] = false
	second["used_traffic_bytes"], second["traffic_limit_bytes"] = 1024, 1024
	for _, account := range []Data{c, second} {
		if err = save(ctx, s.Store.Pool, "clients", account); err != nil {
			t.Fatal(err)
		}
	}
	empty, err := buildSingBoxConfig(ctx, s.Store.Pool, n)
	if err != nil || len(array(empty, "inbounds")) != 0 {
		t.Fatal("inactive accounts left listeners", empty, err)
	}
}

func shadowTLSRoundTrip(t *testing.T, binary, certificate string, c, i Data, controller int, stats string) {
	t.Helper()
	graph, err := singClientOutbounds(c, i, Data{"public_host": "127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	// Trust only the fixture CA; production exports retain normal verification.
	tlsOptions := obj(graph[1].(Data), "tls")
	tlsOptions["server_name"], tlsOptions["certificate_path"] = "vpn.example", certificate
	port := nativeTestPort(t)
	client := Data{"inbounds": []any{Data{"type": "http", "listen": "127.0.0.1", "listen_port": port}}, "outbounds": graph, "route": Data{"final": str(graph[0].(Data), "tag")}}
	nativeRun(t, binary, nativeConfigFile(t, client))
	proxy, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	transport := &http.Transport{Proxy: http.ProxyURL(proxy)}
	defer transport.CloseIdleConnections()
	httpClient := &http.Client{Transport: transport, Timeout: 4 * time.Second}
	payload := strings.Repeat("p", 20*1024)
	hold := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(io.LimitReader(r.Body, 32*1024))
		if string(raw) != payload {
			w.WriteHeader(400)
			return
		}
		_, _ = w.Write([]byte(payload))
		w.(http.Flusher).Flush()
		<-hold
	}))
	defer upstream.Close()
	defer close(hold)
	var response *http.Response
	for attempt := 0; attempt < 30; attempt++ {
		response, err = httpClient.Post(upstream.URL, "text/plain", strings.NewReader(payload))
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatal("ShadowTLS round trip", err)
	}
	defer response.Body.Close()
	raw := make([]byte, len(payload))
	if _, err = io.ReadFull(response.Body, raw); err != nil || !bytes.Equal(raw, []byte(payload)) {
		t.Fatal("payload mismatch", err)
	}
	telemetry, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/connections", controller))
	if err != nil {
		t.Fatal(err)
	}
	defer telemetry.Body.Close()
	var connections Data
	if err = json.NewDecoder(telemetry.Body).Decode(&connections); err != nil {
		t.Fatal(err)
	}
	principal := fmt.Sprintf("veloray:%d:%s", num(c, "id"), str(c, "name"))
	found := false
	for _, entry := range array(connections, "connections") {
		metadata := obj(entry.(map[string]any), "metadata")
		if str(metadata, "user") == principal {
			found = true
			if str(metadata, "sourceIP") != "127.0.0.1" {
				t.Fatal("original source lost", metadata)
			}
		}
	}
	if !found {
		t.Fatal("authenticated principal missing", connections)
	}
	counters := nativeTrafficStats(t, stats, principal)
	for direction, count := range counters {
		if count >= 40*1024 {
			t.Fatal("traffic counted twice", direction, count)
		}
	}
	// Either credential being wrong must deny the tunnel, independently.
	for _, layer := range []string{"inner", "wrapper"} {
		bad, e := singClientOutbounds(c, i, Data{"public_host": "127.0.0.1"})
		if e != nil {
			t.Fatal(e)
		}
		obj(bad[1].(Data), "tls")["server_name"] = "vpn.example"
		obj(bad[1].(Data), "tls")["certificate_path"] = certificate
		if layer == "inner" {
			bad[0].(Data)["password"] = str(obj(i, "protocol_settings"), "psk") + ":" + shadowTLSKey()
		} else {
			bad[1].(Data)["password"] = randomToken(32)
		}
		badPort := nativeTestPort(t)
		cfg := Data{"inbounds": []any{Data{"type": "http", "listen": "127.0.0.1", "listen_port": badPort}}, "outbounds": bad, "route": Data{"final": str(bad[0].(Data), "tag")}}
		nativeRun(t, binary, nativeConfigFile(t, cfg))
		address := fmt.Sprintf("127.0.0.1:%d", badPort)
		for attempt := 0; attempt < 50; attempt++ {
			conn, dialErr := net.DialTimeout("tcp", address, 100*time.Millisecond)
			if dialErr == nil {
				conn.Close()
				break
			}
			time.Sleep(40 * time.Millisecond)
		}
		badProxy, _ := url.Parse("http://" + address)
		badTransport := &http.Transport{Proxy: http.ProxyURL(badProxy)}
		badClient := &http.Client{Transport: badTransport, Timeout: 2 * time.Second}
		res, requestErr := badClient.Post(upstream.URL, "text/plain", strings.NewReader(payload))
		if res != nil {
			res.Body.Close()
		}
		badTransport.CloseIdleConnections()
		if requestErr == nil && res.StatusCode == 200 {
			t.Fatal("invalid credential accepted", layer)
		}
	}
}
