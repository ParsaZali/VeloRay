package panel

import (
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPanelRuntimeStartsWithPanelProcess(t *testing.T) {
	now := time.Now().UTC()
	server := &Server{StartedAt: now.Add(-125 * time.Second)}
	info := server.panelRuntime(now)
	if num(info, "uptime_seconds") != 125 || str(info, "started_at") != stamp(server.StartedAt) {
		t.Fatal(info)
	}
	server.StartedAt = now.Add(time.Minute)
	if num(server.panelRuntime(now), "uptime_seconds") != 0 {
		t.Fatal("negative panel uptime")
	}
	server.StartedAt = time.Time{}
	fallback := server.panelRuntime(now)
	want := max(int64(0), int64(now.Sub(panelProcessStartedAt).Seconds()))
	if num(fallback, "uptime_seconds") != want || str(fallback, "started_at") != stamp(panelProcessStartedAt) {
		t.Fatal("panel uptime did not use the process start", fallback)
	}
}
func TestIntegrationHealthReportsDatabaseAndPanelUptime(t *testing.T) {
	server, ctx := integration(t)
	server.StartedAt = time.Now().Add(-90 * time.Second)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/health", nil))
	info, err := decode(w.Body.Bytes())
	if err != nil || w.Code != 200 || str(info, "database") != "ok" || num(info, "uptime_seconds") < 90 || num(info, "uptime_seconds") > 92 {
		t.Fatal(info, err, w.Code)
	}
	system, err := server.systemInfo(ctx)
	if err != nil || num(system, "panel_uptime_seconds") < 90 || str(system, "panel_started_at") == "" {
		t.Fatal(system, err)
	}
	server.Store.Pool.Close()
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/health", nil))
	info, err = decode(w.Body.Bytes())
	if err != nil || w.Code != 503 || str(info, "database") != "unavailable" {
		t.Fatal(info, err, w.Code)
	}
}
func TestCompactPortalPreservesDownloadsAndEscapesNames(t *testing.T) {
	server := &Server{Config: Config{PublicURL: "https://panel.example"}}
	client := defaults("clients")
	client["name"] = "Arman"
	client["credential"] = uuid()
	client["subscription_token"] = strings.Repeat("a", 32)
	client["used_traffic_bytes"] = int64(3 << 30)
	client["traffic_limit_bytes"] = int64(20 << 30)
	client["traffic_multiplier"] = 3
	inbound := defaults("inbounds")
	inbound["name"] = "main"
	node := Data{"public_host": "vpn.example"}
	cfg := defaultSettings()
	cfg["subscription_template"] = "compact"
	cfg["site_name"] = "VeloRay"
	cfg["support_url"] = "https://support.example"
	render := func(method string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		server.portal(w, httptest.NewRequest(method, "/sub/test?format=portal", nil), client, inbound, node, cfg, "vless://example", "")
		return w
	}
	w := render("GET")
	html := w.Body.String()
	for _, needle := range []string{"connection-layout", "<details>", "?format=base64", "?format=clash", "?format=json", "readonly", "3x", "3.0 GB", "/ 20.0 GB", "copy-subscription", "Link copied", "/subscription-portal.js"} {
		if !strings.Contains(html, needle) {
			t.Fatal("portal missing", needle)
		}
	}
	if file := os.Getenv("VELORAY_TEST_PORTAL_HTML"); file != "" {
		if err := os.WriteFile(file, w.Body.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	client["name"] = "<script>alert(1)</script>"
	if strings.Contains(render("GET").Body.String(), "<script>alert") {
		t.Fatal("unescaped client name")
	}
	cfg["default_locale"] = "fa"
	if !strings.Contains(render("GET").Body.String(), "کپی لینک") {
		t.Fatal("Persian portal copy action is missing")
	}
	if render("HEAD").Body.Len() != 0 {
		t.Fatal("HEAD returned a portal body")
	}
}
