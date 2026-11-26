package panel

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFirstUsePolicyAndName(t *testing.T) {
	c := defaults("clients")
	c["name"] = "alice"
	c["start_after_first_use"] = true
	c["duration_days"] = 7
	c["traffic_multiplier"] = 2.0
	now := time.Now().UTC()
	if activateFirstUse(c, now, false) || c["expires_at"] != nil {
		t.Fatal("unused account started")
	}
	if !activateFirstUse(c, now, true) {
		t.Fatal("first use was not activated")
	}
	expires, e := timestamp(c, "expires_at")
	if e != nil || !expires.Equal(now.Add(7*24*time.Hour)) {
		t.Fatal(c, e)
	}
	if activateFirstUse(c, now.Add(time.Hour), true) {
		t.Fatal("expiry moved on later traffic")
	}
	if name := connectionName(c, Data{"name": "main"}); !strings.HasSuffix(name, "2.0x") || str(c, "name") != "alice" {
		t.Fatal(name, c)
	}
}
func TestDeviceLimitReplacesPreviousIP(t *testing.T) {
	c := defaults("clients")
	c["device_limit"] = 1
	now := time.Now()
	if updateDevicePolicy(c, Data{"1.1.1.1": now.Unix()}, now) {
		t.Fatal("first IP needs no block")
	}
	if !updateDevicePolicy(c, Data{"1.1.1.1": now.Unix(), "8.8.8.8": now.Unix()}, now.Add(time.Second)) {
		t.Fatal("excess IP was not blocked")
	}
	if len(obj(c, "_device_seen")) != 1 || obj(c, "_device_denied")["1.1.1.1"] == nil {
		t.Fatal(c)
	}
	if updateDevicePolicy(c, Data{"1.1.1.1": now.Unix(), "8.8.8.8": now.Unix()}, now.Add(2*time.Second)) {
		t.Fatal("blocked IP displaced current IP")
	}
}
func TestIntegrationAPIKeyResetRevokesPreviousToken(t *testing.T) {
	s, ctx := integration(t)
	u := defaults("users")
	u["username"] = "admin"
	u["is_active"], u["is_staff"], u["is_superuser"] = true, true, true
	if e := save(ctx, s.Store.Pool, "users", u); e != nil {
		t.Fatal(e)
	}
	k := defaults("api_keys")
	k["name"] = "automation"
	k["scope"] = "admin"
	k["user"] = num(u, "id")
	k["_token_hash"] = digest("vr_previous_key")
	if e := save(ctx, s.Store.Pool, "api_keys", k); e != nil {
		t.Fatal(e)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/api-keys/1/reset", nil)
	a := Actor{User: u, Scope: "admin"}
	out, status, e := s.resetAPIKey(httptest.NewRecorder(), req, num(k, "id"), a)
	if e != nil || status != 200 {
		t.Fatal(status, e)
	}
	key := out.(Data)
	stored, e := get(ctx, s.Store.Pool, "api_keys", num(k, "id"))
	if e != nil || str(stored, "_token_hash") != digest(str(key, "token")) || str(stored, "_token_hash") == digest("vr_previous_key") {
		t.Fatal(stored, e)
	}
	view, e := s.view(ctx, s.Store.Pool, "api_keys", stored)
	if e != nil || view["token"] != nil || view["_token_hash"] != nil {
		t.Fatal("secret leaked in subsequent read", view, e)
	}
}
func TestIntegrationSingBoxConfigurationMatrix(t *testing.T) {
	binary := os.Getenv("VELORAY_TEST_SINGBOX_BINARY")
	if binary == "" {
		t.Skip("set VELORAY_TEST_SINGBOX_BINARY")
	}
	s, ctx := integration(t)
	n, i, c := fixture(t, s, ctx, &nodeMock{running: true})
	cert, key := testTLS(t)
	private, e := ecdh.X25519().GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	matrix := [][3]string{{"vless", "raw", "none"}, {"vless", "raw", "reality"}, {"vless", "ws", "tls"}, {"vless", "grpc", "tls"}, {"vless", "httpupgrade", "tls"}, {"vmess", "raw", "none"}, {"trojan", "raw", "tls"}, {"shadowsocks", "raw", "none"}, {"hysteria", "hysteria", "tls"}, {"http", "raw", "none"}, {"socks", "raw", "none"}}
	for _, v := range matrix {
		t.Run(strings.Join(v[:], "/"), func(t *testing.T) {
			i["core"] = "singbox"
			i["protocol"], i["transport"], i["security"] = v[0], v[1], v[2]
			i["tls_cert_file"], i["tls_key_file"] = cert, key
			i["reality_dest"] = "example.com:443"
			i["reality_server_name"] = "example.com"
			i["reality_private_key"] = base64.RawURLEncoding.EncodeToString(private.Bytes())
			i["reality_public_key"] = base64.RawURLEncoding.EncodeToString(private.PublicKey().Bytes())
			i["reality_short_id"] = "aabbccdd"
			if e := save(ctx, s.Store.Pool, "inbounds", i); e != nil {
				t.Fatal(e)
			}
			if e := save(ctx, s.Store.Pool, "clients", c); e != nil {
				t.Fatal(e)
			}
			cfg, e := buildSingBoxConfig(ctx, s.Store.Pool, n)
			if e != nil {
				t.Fatal(e)
			}
			raw, _ := json.Marshal(cfg)
			path := filepath.Join(t.TempDir(), "config.json")
			if e = os.WriteFile(path, raw, 0600); e != nil {
				t.Fatal(e)
			}
			if out, e := exec.Command(binary, "check", "-c", path).CombinedOutput(); e != nil {
				t.Fatalf("rejected %v: %s", v, out)
			}
		})
	}
}

func TestTrustedSourceIPsExemptFromConcurrentLimit(t *testing.T) {
	now := time.Now()
	c := Data{"device_limit": 1, "trusted_source_ips": []any{"203.0.113.15/24", "2001:db8::42", "203.0.113.0/24"},
		"_device_seen": Data{"203.0.113.9": now.Unix()}, "_device_denied": Data{"2001:db8::42": now.Unix()}}
	if err := normalizeTrustedSourceIPs(c); err != nil {
		t.Fatal(err)
	}
	if values := array(c, "trusted_source_ips"); len(values) != 2 || values[0] != "203.0.113.0/24" {
		t.Fatalf("unexpected prefixes: %v", values)
	}
	if !updateDevicePolicy(c, Data{"::ffff:203.0.113.9": true, "2001:db8::42": true, "198.51.100.8": true}, now) {
		t.Fatal("expected stale policy to be replaced")
	}
	if len(obj(c, "_device_denied")) != 0 || obj(c, "_device_seen")["198.51.100.8"] == nil || len(obj(c, "_device_seen")) != 1 {
		t.Fatalf("unexpected policy: %v", c)
	}
	updateDevicePolicy(c, Data{"198.51.100.9": true}, now.Add(time.Second))
	if obj(c, "_device_denied")["198.51.100.8"] == nil {
		t.Fatal("non-trusted client should still be subject to limit")
	}
	for _, value := range []string{"0.0.0.0/0", "::/0", "host.example", "fe80::1%eth0"} {
		if normalizeTrustedSourceIPs(Data{"trusted_source_ips": []any{value}}) == nil {
			t.Fatalf("accepted %q", value)
		}
	}
}
