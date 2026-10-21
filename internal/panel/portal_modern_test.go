package panel

import (
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestModernSubscriptionPortalHasNativeStatisticsAndSafeMarkup(t *testing.T) {
	server := &Server{Config: Config{PublicURL: "https://panel.example"}}
	client := defaults("clients")
	client["name"] = "Arman"
	client["subscription_token"] = strings.Repeat("x", 32)
	client["credential"] = uuid()
	client["used_traffic_bytes"] = int64(2 << 30)
	client["raw_used_traffic_bytes"] = int64(1 << 30)
	client["traffic_limit_bytes"] = int64(10 << 30)
	client["device_limit"] = int64(3)
	client["_device_seen"] = Data{"device1": true, "device2": true}
	client["last_traffic_at"] = "2026-10-16T16:30:00Z"
	inbound := defaults("inbounds")
	inbound["name"] = "Main"
	inbound["protocol"] = "vless"
	node := Data{"public_host": "example.com"}
	cfg := defaultSettings()
	cfg["default_locale"] = "fa"
	cfg["subscription_template"] = "dashboard"
	uri := "vless://" + str(client, "credential") + "@example.com:443#Main"
	details := []Data{{"name": "Primary", "protocol": "vless", "node": "Local", "host": "example.com", "link": uri}}
	render := func(method string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		server.portal(w, httptest.NewRequest(method, "/sub/test?format=portal", nil), client, inbound, node, cfg, uri, "", details...)
		return w
	}
	w := render("GET")
	if w.Code != 200 {
		t.Fatalf("status %d", w.Code)
	}
	html := w.Body.String()
	for _, needle := range []string{
		"data-portal-version=\"2\"", "subscription-modern.css", "subscription-modern.js",
		"data-chart-url=\"/sub/", "format=stats", "traffic-chart",
		"profile-list", "profile-search", "protocol-filter", "copy-all", "copy-subscription",
		"data-range=\"90d\"", "id=\"qr-dialog\"", "portal-language", "toggle-theme",
		"2.0 GB", "8.0 GB", "1.0 GB", "Arman", "Primary", "2 / 3", "id=\"last-activity\"",
	} {
		if !strings.Contains(html, needle) {
			t.Fatalf("modern portal missing %q", needle)
		}
	}
	if file := os.Getenv("VELORAY_TEST_MODERN_PORTAL_HTML"); file != "" {
		if err := os.WriteFile(file, w.Body.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cfg["subscription_show_connection_uri"] = false
	if strings.Contains(render("GET").Body.String(), "profile-list") {
		t.Fatal("hidden URI surfaced in modern portal")
	}
	cfg["subscription_show_connection_uri"] = true
	cfg["subscription_show_qr"] = false
	if strings.Contains(render("GET").Body.String(), "qr-trigger") {
		t.Fatal("disabled QR surfaced")
	}
	cfg["subscription_show_qr"] = true
	client["name"] = "<script>alert(1)</script>"
	if strings.Contains(render("GET").Body.String(), "<script>alert") {
		t.Fatal("unescaped customer name")
	}
	if render("HEAD").Body.Len() != 0 {
		t.Fatal("HEAD must have no body")
	}
}

func TestSubscriptionRangeValidation(t *testing.T) {
	for _, v := range []string{"1h", "12h", "24h", "7d", "30d", "90d", ""} {
		d, b, ok := subscriptionRange(v)
		if !ok || d == "" || b == "" {
			t.Fatalf("range %q incorrectly rejected", v)
		}
	}
	for _, v := range []string{"100y", "' OR 1=1--", "day", "-1h"} {
		if _, _, ok := subscriptionRange(v); ok {
			t.Fatalf("unsafe range accepted %q", v)
		}
	}
}

func TestModernConnectionScoping(t *testing.T) {
	d := map[string]any{"URL": "https://example/sub/abc"}
	c := Data{"traffic_limit_bytes": int64(2), "used_traffic_bytes": int64(10)}
	cfg := defaultSettings()
	modernPortalData(d, c, cfg, []Data{
		{"name": "Valid", "protocol": "vmess", "node": "Local", "host": "example.com", "link": "vmess://valid"},
		{"name": "Missing", "protocol": "tun", "node": "Local", "host": "example.com", "link": ""},
	})
	if d["ConnectionCount"] != 1 || d["Remaining"] != "0 B" {
		t.Fatal("unexpected shared accounting", d)
	}
	if len(d["Connections"].([]Data)) != 1 {
		t.Fatal("non-exportable URI surfaced")
	}
	if d["ChartURL"] != "/sub/abc?format=stats" {
		t.Fatal("stats must stay on the serving origin", d["ChartURL"])
	}
}

func TestSubscriptionStatsReadOnlyTokenScopedIntegration(t *testing.T) {
	s, ctx := integration(t)
	principal := "client:9900042"
	_, err := s.Store.Pool.Exec(ctx, `INSERT INTO vr_client_traffic_windows(at,principal_key,bytes,upload_bytes,download_bytes) VALUES($1,$2,$3,$4,$5) ON CONFLICT(principal_key,at) DO UPDATE SET bytes=excluded.bytes,upload_bytes=excluded.upload_bytes,download_bytes=excluded.download_bytes`, time.Now().UTC().Truncate(5*time.Minute), principal, int64(1024), int64(256), int64(768))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Store.Pool.Exec(ctx, `DELETE FROM vr_client_traffic_windows WHERE principal_key=$1`, principal)
	req := httptest.NewRequest("GET", "/sub/secret?format=stats&range=7d", nil)
	w := httptest.NewRecorder()
	s.subscriptionStats(w, req, Data{"id": int64(9900042)})
	if w.Code != 200 {
		t.Fatalf("stats http status %d: %s", w.Code, w.Body.String())
	}
	d, e := decode(w.Body.Bytes())
	if e != nil || !flag(d, "history_available") || len(array(d, "points")) == 0 {
		t.Fatalf("stats missing %v %v", d, e)
	}
	point := asData(array(d, "points")[0])
	if num(point, "bytes") != 1024 || num(point, "upload_bytes") != 256 || num(point, "download_bytes") != 768 {
		t.Fatalf("unexpected history: %v", point)
	}
	w = httptest.NewRecorder()
	s.subscriptionStats(w, httptest.NewRequest("HEAD", "/sub/secret?format=stats&range=7d", nil), Data{"id": int64(9900042)})
	if w.Code != 200 || w.Body.Len() != 0 {
		t.Fatalf("HEAD returned body / bad status %d %q", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	s.subscriptionStats(w, httptest.NewRequest("GET", "/sub/secret?format=stats&range=DROP", nil), Data{"id": int64(9900042)})
	if w.Code != 400 {
		t.Fatalf("unsafe range accepted %d", w.Code)
	}
	w = httptest.NewRecorder()
	s.subscriptionStats(w, httptest.NewRequest("GET", "/sub/secret?format=stats&range=7d&export=csv", nil), Data{"id": int64(9900042)})
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Type"), "text/csv") || !strings.Contains(w.Body.String(), "timestamp_utc,total_bytes,upload_bytes,download_bytes") || !strings.Contains(w.Body.String(), ",1024,256,768") {
		t.Fatalf("csv export invalid: status=%d content=%q", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	s.subscriptionStats(w, httptest.NewRequest("GET", "/sub/secret?format=stats&range=7d&export=invalid", nil), Data{"id": int64(9900042)})
	if w.Code != 400 {
		t.Fatal("unknown export accepted")
	}
	w = httptest.NewRecorder()
	s.subscriptionStats(w, httptest.NewRequest("HEAD", "/sub/secret?format=stats&range=7d&export=csv", nil), Data{"id": int64(9900042)})
	if w.Code != 200 || w.Body.Len() != 0 || !strings.Contains(w.Header().Get("Content-Type"), "text/csv") {
		t.Fatal("CSV HEAD is invalid")
	}
}
