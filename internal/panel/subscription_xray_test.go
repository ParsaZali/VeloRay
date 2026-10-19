package panel

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestXrayClientExportNeverIncludesServerSecrets(t *testing.T) {
	binary := os.Getenv("VELORAY_TEST_XRAY_BINARY")
	for _, p := range []string{"vless", "vmess", "trojan", "shadowsocks"} {
		for _, security := range []string{"none", "tls", "reality"} {
			if security == "reality" && p != "vless" {
				continue
			}
			if p == "shadowsocks" && security != "none" {
				continue
			}
			i := defaults("inbounds")
			i["protocol"], i["security"], i["port"] = p, security, 443
			i["reality_private_key"] = "do-not-export-private-key"
			i["reality_public_key"] = "c1RX0eN6mrmk9lc3WnPwH_K4TeicRnmFokqiU25N8Gg"
			i["reality_server_name"] = "example.com"
			i["tls_key_file"] = "/private/key.pem"
			i["stream_settings"] = Data{"tlsSettings": Data{"certificates": []any{Data{"keyFile": "/private/another.pem"}}}}
			c := defaults("clients")
			c["credential"] = "955cc471-391e-4f25-bc37-c8336ed2be5b"
			cfg, e := xrayClientConfig(c, i, Data{"public_host": "vpn.example"})
			if e != nil {
				t.Fatal(e)
			}
			raw, _ := json.Marshal(cfg)
			for _, secret := range []string{"do-not-export-private-key", "/private/", "privateKey", "certificateFile"} {
				if strings.Contains(string(raw), secret) {
					t.Fatal("server secret in client config", string(raw))
				}
			}
			if binary != "" {
				path := nativeConfigFile(t, cfg)
				if out, e := exec.Command(binary, "run", "-test", "-config", path).CombinedOutput(); e != nil {
					t.Fatal(p, security, e, string(out))
				}
			}
		}
	}
}

func TestPortalOffersJSONForProtocolsWithoutPortableURI(t *testing.T) {
	s := &Server{Config: Config{PublicURL: "https://panel.example"}}
	i := defaults("inbounds")
	i["protocol"], i["security"], i["port"], i["core"] = "snell", "none", 443, "singbox"
	i["protocol_settings"] = Data{"psk": "private-server-psk"}
	c := defaults("clients")
	c["name"], c["credential"], c["subscription_token"] = "snell-user", "private-user-key", strings.Repeat("t", 32)
	for _, style := range []string{"default", "dashboard", "compact"} {
		cfg := defaultSettings()
		cfg["subscription_template"] = style
		w := httptest.NewRecorder()
		s.portal(w, httptest.NewRequest("GET", "/", nil), c, i, Data{"public_host": "vpn.example"}, cfg, "", "")
		html := w.Body.String()
		if w.Code != 200 || !strings.Contains(html, "?format=singbox") || strings.Contains(html, "?format=clash") || strings.Contains(html, "private-server-psk") || strings.Contains(html, "private-user-key") {
			t.Fatal(style, "incorrect or unsafe format links", w.Code, html)
		}
	}
}
