package panel

import (
	"bytes"
	"encoding/json"
	"fmt"
	qrcode "github.com/skip2/go-qrcode"
	"image"
	_ "image/png"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSimplePortalDefaultLegacyCompatibility(t *testing.T) {
	for _, name := range []string{"modernX.html", "sub-dashboard.html", "sub-fish.html"} {
		t.Run(name, func(t *testing.T) {
			s, c, i, n, cfg := portalFixture(t)
			source, e := os.ReadFile(filepath.Join("..", "..", "examples", "subscription", "legacy", name))
			if e != nil {
				t.Fatal(e)
			}
			if e = os.WriteFile(filepath.Join(s.Config.TemplateRoot, "page.html"), source, 0600); e != nil {
				t.Fatal(e)
			}
			cfg["subscription_template"], cfg["subscription_template_path"] = "custom", "page.html"
			c["raw_upload_traffic_bytes"], c["raw_download_traffic_bytes"] = int64(128<<20), int64(256<<20)
			c["expires_at"], c["last_traffic_at"] = stamp(time.Now().Add(30*24*time.Hour)), stamp(time.Now().Add(-time.Hour))
			uri := "vless://" + str(c, "credential") + "@vpn.example:443?security=tls#" + url.PathEscape(`Primary & <Route> "Alpha's"`)
			w := httptest.NewRecorder()
			s.portal(w, httptest.NewRequest("GET", "/sub/"+str(c, "subscription_token")+"?format=portal", nil), c, i, n, cfg, uri, "", Data{"name": "Primary", "protocol": "vless", "link": uri})
			if w.Header().Get("X-VeloRay-Template-Fallback") != "" {
				t.Fatal("template fell back", name)
			}
			if !strings.Contains(w.Body.String(), "subscription-template-compat.js") || strings.Contains(w.Body.String(), "ZgotmplZ") {
				t.Fatal("unsafe or missing template output")
			}
			if strings.Contains(w.Header().Get("Content-Security-Policy"), "unsafe-eval") || strings.Contains(w.Header().Get("Content-Security-Policy"), "https://") {
				t.Fatal("CSP broadened")
			}
			directory := os.Getenv("VELORAY_TEST_LEGACY_PORTAL_DIR")
			if directory == "" && os.Getenv("VELORAY_TEST_SIMPLE_PORTAL_HTML") != "" {
				directory = filepath.Join(filepath.Dir(os.Getenv("VELORAY_TEST_SIMPLE_PORTAL_HTML")), "veloray-legacy-portals")
			}
			if directory != "" {
				if e = os.MkdirAll(directory, 0700); e != nil {
					t.Fatal(e)
				}
				if e = os.WriteFile(filepath.Join(directory, name), w.Body.Bytes(), 0600); e != nil {
					t.Fatal(e)
				}
				png, _ := qrcode.Encode(uri, qrcode.Medium, 280)
				if e = os.WriteFile(filepath.Join(directory, name+".qr.png"), png, 0600); e != nil {
					t.Fatal(e)
				}
				if e = os.WriteFile(filepath.Join(directory, name+".headers.json"), mustJSON(w.Header()), 0600); e != nil {
					t.Fatal(e)
				}
			}
			tmpl, e := s.loadPortalTemplate("page.html")
			if e != nil {
				t.Fatal(e)
			}
			var rendered bytes.Buffer
			if e = tmpl.Execute(&rendered, portalSampleData()); e != nil {
				t.Fatal("settings validation cannot render", e)
			}
		})
	}
}

func TestLegacyPortalDataPreservesQuotaUnitsAndScope(t *testing.T) {
	_, c, _, _, cfg := portalFixture(t)
	expiry := time.Date(2028, 2, 29, 12, 0, 0, 0, time.UTC)
	c["expires_at"] = stamp(expiry)
	c["raw_upload_traffic_bytes"], c["raw_download_traffic_bytes"] = 100, 200
	d := legacyPortalData(c, cfg, "https://panel.example/sub/token", []Data{{"link": "vless://own", "name": "Main"}, {"name": "No URI"}})
	if num(d, "expire") != expiry.UnixMilli() || num(d, "usedByte") != 1<<30 || num(d, "uploadByte") != 100 || num(d, "downloadByte") != 200 {
		t.Fatal(d)
	}
	if len(d["links"].([]string)) != 1 || len(d["emails"].([]string)) != 1 || !strings.Contains(str(d, "subJsonUrl"), "format=xray") {
		t.Fatal(d)
	}
	info := legacySubscriptionInfo(c, cfg)
	raw, _ := json.Marshal(info)
	for _, secret := range []string{"subscription_token", "credential", "vless://", "subUrl", "links"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("info leaked connection credentials", secret)
		}
	}
	if d["isOnline"] != nil {
		t.Fatal("unknown telemetry guessed")
	}
}

func TestIntegrationLegacyInfoAndQRRespectSubscriptionScope(t *testing.T) {
	s, ctx := integration(t)
	n, _, c := fixture(t, s, ctx, &nodeMock{running: true})
	n["_telemetry_at"], n["_online_available"], n["_online_clients"] = stamp(), true, Data{fmt.Sprint(num(c, "id")): true}
	if e := save(ctx, s.Store.Pool, "nodes", n); e != nil {
		t.Fatal(e)
	}
	c["used_traffic_bytes"], c["raw_upload_traffic_bytes"], c["raw_download_traffic_bytes"] = 900, 100, 200
	if e := save(ctx, s.Store.Pool, "clients", c); e != nil {
		t.Fatal(e)
	}
	base := "/sub/" + str(c, "subscription_token")
	fetch := func(query string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("GET", base+query, nil))
		return w
	}
	w := fetch("?format=info")
	var d Data
	if e := json.Unmarshal(w.Body.Bytes(), &d); e != nil || w.Code != 200 || num(d, "usedByte") != 900 || num(d, "uploadByte") != 100 || !flag(d, "isOnline") {
		t.Fatal(w.Code, w.Body.String(), e)
	}
	if strings.Contains(w.Body.String(), str(c, "credential")) || strings.Contains(w.Body.String(), "links") {
		t.Fatal("info credential leak")
	}
	var history int
	s.Store.Pool.QueryRow(ctx, "SELECT count(*) FROM vr_subscription_requests").Scan(&history)
	if history != 0 {
		t.Fatal("automatic polling filled request history")
	}
	raw := fetch("?format=raw")
	uri := strings.TrimSpace(raw.Body.String())
	w = fetch("?format=qr&data=" + url.QueryEscape(uri))
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" {
		t.Fatal(w.Code, w.Body.String())
	}
	if image, _, e := image.DecodeConfig(bytes.NewReader(w.Body.Bytes())); e != nil || image.Width != 280 {
		t.Fatal("invalid local QR", e)
	}
	w = fetch("?format=qr&data=" + url.QueryEscape("https://foreign.example/private"))
	if w.Code != 400 {
		t.Fatal("foreign QR target accepted", w.Code)
	}
	cfg, _ := settings(ctx, s.Store.Pool)
	cfg["subscription_hwid_limit"] = 1
	if e := saveSettings(ctx, s.Store.Pool, cfg); e != nil {
		t.Fatal(e)
	}
	if w = fetch("?format=qr&data=" + url.QueryEscape(uri)); w.Code == 200 {
		t.Fatal("QR bypassed HWID admission")
	}
	if w = fetch("?format=info"); w.Code != 200 || strings.Contains(w.Body.String(), str(c, "credential")) {
		t.Fatal("metadata unavailable or unsafe", w.Code)
	}
}

func TestIndependentSubscriptionServiceExposesOnlyPresentationAssets(t *testing.T) {
	directory := t.TempDir()
	for _, path := range []string{"subscription-template-compat.js", "subscription-simple.css", "fonts/test.woff2", "subscription-template-assets/icons/icon.css", "index.html", "private.js"} {
		file := filepath.Join(directory, path)
		if e := os.MkdirAll(filepath.Dir(file), 0700); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(file, []byte("fixture"), 0600); e != nil {
			t.Fatal(e)
		}
	}
	s := &Server{Config: Config{SubscriptionOnly: true, FrontendDir: directory}}
	for _, path := range []string{"/subscription-template-compat.js", "/subscription-simple.css", "/fonts/test.woff2", "/subscription-template-assets/icons/icon.css", "/index.html", "/private.js", "/api/admin", "/fonts/missing.woff2", "/fonts/../private.js"} {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		allowed := oneOf(path, "/subscription-template-compat.js", "/subscription-simple.css", "/fonts/test.woff2", "/subscription-template-assets/icons/icon.css")
		if (w.Code == 200) != allowed {
			t.Fatal(path, w.Code)
		}
	}
}
