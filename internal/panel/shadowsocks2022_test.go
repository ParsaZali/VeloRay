package panel

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestShadowsocks2022SIP002AndClientFormats(t *testing.T) {
	for _, method := range []string{"2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm"} {
		i := defaults("inbounds")
		i["protocol"] = "shadowsocks"
		i["port"] = 8388
		i["protocol_settings"] = Data{"method": method}
		if e := validateShadowsocks2022(i); e != nil {
			t.Fatal(e)
		}
		c := defaults("clients")
		c["credential"] = ssKey(ssKeyBytes(i))
		c["name"] = "Alice"
		n := Data{"public_host": "vpn.example"}
		uri := shareLink(c, i, n)
		parsed, e := url.Parse(uri)
		if e != nil {
			t.Fatal(e)
		}
		password, _ := parsed.User.Password()
		if parsed.User.Username() != method || password != ssClientPassword(c, i) {
			t.Fatal("invalid SIP002 AEAD-2022 URI")
		}
		clash, e := clashConfig(c, i, n)
		if e != nil {
			t.Fatal(e)
		}
		proxy := asData(array(clash, "proxies")[0])
		if str(proxy, "password") != password {
			t.Fatal(proxy)
		}
		sing, e := singClientOutbound(c, i, n)
		if e != nil || str(sing, "password") != password {
			t.Fatal(sing, e)
		}
		xray, e := xrayClientConfig(c, i, n)
		if e != nil {
			t.Fatal(e)
		}
		server := asData(array(obj(asData(array(xray, "outbounds")[0]), "settings"), "servers")[0])
		if str(server, "password") != password {
			t.Fatal(server)
		}
	}
}
func TestIntegrationShadowsocks2022CountsEachUserOnce(t *testing.T) {
	s, ctx := integration(t)
	m := &nodeMock{running: true}
	n, i, c := fixture(t, s, ctx, m)
	i["protocol"] = "shadowsocks"
	i["protocol_settings"] = Data{"method": "2022-blake3-aes-128-gcm"}
	if e := validateInbound(ctx, s.Store.Pool, s.Config, i); e == nil {
		t.Fatal("changing an existing UUID user to SS2022 without rotation should fail")
	}
	c["credential"] = ssKey(16)
	if e := save(ctx, s.Store.Pool, "clients", c); e != nil {
		t.Fatal(e)
	}
	if e := validateInbound(ctx, s.Store.Pool, s.Config, i); e != nil {
		t.Fatal(e)
	}
	if e := save(ctx, s.Store.Pool, "inbounds", i); e != nil {
		t.Fatal(e)
	}
	c2 := defaults("clients")
	c2["inbound"] = num(i, "id")
	c2["name"] = "Bob"
	if e := validateClient(ctx, s.Store.Pool, c2); e != nil {
		t.Fatal(e)
	}
	if !validSSKey(str(c2, "credential"), 16) {
		t.Fatal("user key not generated")
	}
	if e := save(ctx, s.Store.Pool, "clients", c2); e != nil {
		t.Fatal(e)
	}
	m.stats = []any{Data{"name": fmt.Sprintf("user>>>veloray:%d:%s>>>traffic>>>uplink", num(c, "id"), str(c, "name")), "value": 100}, Data{"name": fmt.Sprintf("user>>>veloray:%d:%s>>>traffic>>>downlink", num(c2, "id"), str(c2, "name")), "value": 250}, Data{"name": fmt.Sprintf("inbound>>>in-%d-%s>>>traffic>>>uplink", num(i, "id"), str(i, "name")), "value": 350}}
	tx, e := s.Store.Pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	if e = s.collect(ctx, tx, n); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	one, _ := get(ctx, s.Store.Pool, "clients", num(c, "id"))
	two, _ := get(ctx, s.Store.Pool, "clients", num(c2, "id"))
	if num(one, "used_traffic_bytes") != 100 || num(two, "used_traffic_bytes") != 250 {
		t.Fatal(one, two)
	}
}
func TestIntegrationShadowsocks2022RealCoresAndSeparateCredentials(t *testing.T) {
	xray := os.Getenv("VELORAY_TEST_XRAY_BINARY")
	sing := os.Getenv("VELORAY_TEST_SINGBOX_BINARY")
	if xray == "" || sing == "" {
		t.Skip("set pinned core binaries for SS2022 interoperability tests")
	}
	for _, core := range []string{"xray", "singbox"} {
		for _, method := range []string{"2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm"} {
			t.Run(core+"/"+method, func(t *testing.T) {
				s, ctx := integration(t)
				n, i, c := fixture(t, s, ctx, &nodeMock{running: true})
				n["public_host"] = "127.0.0.1"
				i["protocol"] = "shadowsocks"
				i["core"] = core
				i["listen"] = "127.0.0.1"
				i["port"] = nativeTestPort(t)
				i["protocol_settings"] = Data{"method": method}
				c["credential"] = ssKey(ssKeyBytes(i))
				if e := save(ctx, s.Store.Pool, "clients", c); e != nil {
					t.Fatal(e)
				}
				if e := validateInbound(ctx, s.Store.Pool, s.Config, i); e != nil {
					t.Fatal(e)
				}
				if e := save(ctx, s.Store.Pool, "inbounds", i); e != nil {
					t.Fatal(e)
				}
				c2 := defaults("clients")
				c2["name"] = "second"
				c2["inbound"] = num(i, "id")
				if e := validateClient(ctx, s.Store.Pool, c2); e != nil {
					t.Fatal(e)
				}
				if e := save(ctx, s.Store.Pool, "clients", c2); e != nil {
					t.Fatal(e)
				}
				var cfg Data
				var e error
				binary := xray
				apiPort := nativeTestPort(t)
				if core == "xray" {
					cfg, e = buildConfig(ctx, s.Store.Pool, n)
					if e == nil {
						for _, value := range array(cfg, "inbounds") {
							in := asData(value)
							if str(in, "tag") == "api" {
								in["port"] = apiPort
							}
						}
					}
				} else {
					binary = sing
					cfg, e = buildSingBoxConfig(ctx, s.Store.Pool, n)
					if e == nil {
						obj(obj(cfg, "experimental"), "clash_api")["external_controller"] = fmt.Sprintf("127.0.0.1:%d", apiPort)
						obj(obj(cfg, "experimental"), "v2ray_api")["listen"] = fmt.Sprintf("127.0.0.1:%d", nativeTestPort(t))
					}
				}
				if e != nil {
					t.Fatal(e)
				}
				path := nativeConfigFile(t, cfg)
				args := []string{"run", "-test", "-c", path}
				if core == "singbox" {
					args = []string{"check", "-c", path}
				}
				if output, e := exec.Command(binary, args...).CombinedOutput(); e != nil {
					t.Fatal(e, string(output))
				}
				if core == "singbox" && os.Getenv("VELORAY_TEST_NO_NETLINK") == "1" {
					return
				}
				nativeRun(t, binary, path)
				waitNetworkListener(t, apiPort)
				for _, account := range []Data{c, c2} {
					t.Run(str(account, "name"), func(t *testing.T) { ss2022RoundTrip(t, xray, account, i, n, false) })
				}
				invalid := clone(c2)
				invalid["credential"] = ssKey(ssKeyBytes(i))
				t.Run("wrong-key", func(t *testing.T) { ss2022RoundTrip(t, xray, invalid, i, n, true) })
			})
		}
	}
}
func ss2022RoundTrip(t *testing.T, binary string, c, i, n Data, deny bool) {
	t.Helper()
	config, e := xrayClientConfig(c, i, n)
	if e != nil {
		t.Fatal(e)
	}
	port := nativeTestPort(t)
	config["inbounds"] = []any{Data{"listen": "127.0.0.1", "port": port, "protocol": "http", "settings": Data{}}}
	nativeRun(t, binary, nativeConfigFile(t, config))
	waitNetworkListener(t, port)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "ss2022-authenticated") }))
	defer upstream.Close()
	proxy, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", port))
	transport := &http.Transport{Proxy: http.ProxyURL(proxy)}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
	res, e := client.Get(upstream.URL)
	if deny {
		if e == nil {
			defer res.Body.Close()
			body, _ := io.ReadAll(res.Body)
			if strings.Contains(string(body), "ss2022-authenticated") {
				t.Fatal("invalid key authenticated")
			}
		}
		return
	}
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if string(raw) != "ss2022-authenticated" {
		t.Fatal(res.StatusCode, string(raw))
	}
}
