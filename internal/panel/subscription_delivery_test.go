package panel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func portalFixture(t *testing.T) (*Server, Data, Data, Data, Data) {
	t.Helper()
	c := defaults("clients")
	c["name"] = "Arman"
	c["credential"] = uuid()
	c["subscription_token"] = strings.Repeat("s", 32)
	c["used_traffic_bytes"] = int64(1 << 30)
	c["traffic_limit_bytes"] = int64(5 << 30)
	i := defaults("inbounds")
	i["name"] = "Primary"
	i["port"] = 443
	n := Data{"public_host": "vpn.example"}
	return &Server{Config: Config{PublicURL: "https://panel.example", TemplateRoot: t.TempDir()}}, c, i, n, defaultSettings()
}
func TestSimplePortalDefaultAndCustomFallback(t *testing.T) {
	s, c, i, n, cfg := portalFixture(t)
	render := func(method string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		s.portal(w, httptest.NewRequest(method, "/sub/example?format=portal", nil), c, i, n, cfg, "vless://example", "", Data{"name": "Primary", "protocol": "vless", "link": "vless://example"})
		return w
	}
	w := render("GET")
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	for _, value := range []string{"subscription-simple.css", "subscription-simple.js", "appearance", "5.0 GB", "4.0 GB", "Arman", "profile-0"} {
		if !strings.Contains(w.Body.String(), value) {
			t.Fatal("missing", value)
		}
	}
	if strings.Contains(w.Body.String(), "traffic-chart") {
		t.Fatal("default page is still the complex dashboard")
	}
	if file := os.Getenv("VELORAY_TEST_SIMPLE_PORTAL_HTML"); file != "" {
		if err := os.WriteFile(file, w.Body.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if render("HEAD").Body.Len() != 0 {
		t.Fatal("HEAD has body")
	}
	cfg["subscription_template"] = "custom"
	cfg["subscription_template_path"] = "example.html"
	path := filepath.Join(s.Config.TemplateRoot, "example.html")
	write := func(value string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(`<html><h1>{{.Name}}</h1><script>document.title = "custom";</script><link rel="stylesheet" href="{{.AssetsURL}}page.css"></html>`)
	w = render("GET")
	if strings.Contains(w.Body.String(), "subscription-simple.css") || !strings.Contains(w.Header().Get("Content-Security-Policy"), "sha256-") {
		t.Fatal("custom document not rendered / CSP missing")
	}
	c["name"] = `<script>bad()</script>`
	if strings.Contains(render("GET").Body.String(), `<h1><script>`) {
		t.Fatal("customer field is not escaped")
	}
	write(`<html>Reloaded {{.Name}}</html>`)
	if !strings.Contains(render("GET").Body.String(), "Reloaded") {
		t.Fatal("template did not reload")
	}
	for _, broken := range []string{`<html>{{`, `<html>partial {{.UnsupportedVariable}}</html>`} {
		write(broken)
		w = render("GET")
		if w.Header().Get("X-VeloRay-Template-Fallback") != "true" || !strings.Contains(w.Body.String(), "subscription-simple.css") || strings.Contains(w.Body.String(), "partial") {
			t.Fatal("broken template leaked partial output instead of fallback", w.Body.String())
		}
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if render("GET").Header().Get("X-VeloRay-Template-Fallback") != "true" {
		t.Fatal("missing template did not fall back")
	}
}
func TestCustomPortalFileConfinementAndAssets(t *testing.T) {
	s, _, _, _, cfg := portalFixture(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.html"), []byte("<html>secret</html>"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(s.Config.TemplateRoot, "escape")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"../secret.html", filepath.Join(outside, "secret.html"), "escape/secret.html", ".env", ""} {
		if _, err := s.loadPortalTemplate(path); err == nil {
			t.Fatal("unsafe template accepted", path)
		}
	}
	if err := os.MkdirAll(filepath.Join(s.Config.TemplateRoot, "custom/assets"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Config.TemplateRoot, "custom/index.html"), []byte("<html>custom</html>"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Config.TemplateRoot, "custom/assets/style.css"), []byte("body{color:red}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.html"), filepath.Join(s.Config.TemplateRoot, "custom/assets/secret.css")); err != nil {
		t.Fatal(err)
	}
	cfg["subscription_template"] = "custom"
	cfg["subscription_template_path"] = "custom/index.html"
	for _, test := range []struct {
		path   string
		status int
	}{{"style.css", 200}, {"../index.html", 404}, {"secret.css", 404}, {"index.html", 404}, {".env", 404}} {
		w := httptest.NewRecorder()
		s.portalAsset(w, httptest.NewRequest("GET", "/", nil), cfg, test.path)
		if w.Code != test.status {
			t.Fatal(test, w.Code, w.Body.String())
		}
	}
	root, err := os.OpenRoot(s.Config.TemplateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if _, err = readRegular(root, "custom/index.html", 2); err == nil {
		t.Fatal("oversized template accepted")
	}
	var buffer bytes.Buffer
	if _, err = (&limitedTemplateWriter{Writer: &buffer, Remaining: 2}).Write([]byte("abc")); err == nil || buffer.Len() != 0 {
		t.Fatal("unbounded template output")
	}
}
func TestFeedTemplatesPreserveCredentialsAndExpandNames(t *testing.T) {
	generated := `{"proxies":[{"name":"A","password":"a\"b"},{"name":"B"}],"proxy-groups":[{"name":"VeloRay","proxies":["A","B"]}],"rules":["MATCH,VeloRay"]}`
	raw, err := applyFeedTemplate(`{"dns":{"enable":true},"proxy-groups":[{"name":"VeloRay","type":"select","proxies":["$VELO_PROXIES","DIRECT"]}]}`, generated, "clash")
	if err != nil {
		t.Fatal(err)
	}
	d, err := decode([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(array(d, "proxies")) != 2 || str(asData(array(d, "proxies")[0]), "password") != "a\"b" || len(array(asData(array(d, "proxy-groups")[0]), "proxies")) != 3 {
		t.Fatal(raw)
	}
	for _, bad := range []string{`[]`, `{"dns":`, strings.Repeat("a", 130<<10), `{} {}`} {
		if validateFeedTemplate(bad) == nil {
			t.Fatal("bad template accepted")
		}
	}
	// Arbitrary JSON items must not panic during placeholder expansion.
	if _, err = applyFeedTemplate(`{"proxy-groups":[{"proxies":[{}]}]}`, generated, "clash"); err != nil {
		t.Fatal(err)
	}
}
func TestDeliveryHostsOnlyChangeExportedSettings(t *testing.T) {
	i := Data{"id": 1, "name": "Original", "port": 443, "security": "reality", "reality_server_name": "original.example"}
	n := Data{"public_host": "original.example"}
	h := Data{"inbound": 1, "name": "CDN", "address": "edge.example", "port": 8443, "sni": "sni.example", "path": "/cdn", "enabled": true}
	if err := validateSubscriptionHosts([]any{h}); err != nil {
		t.Fatal(err)
	}
	exportI, exportN := deliveryHost(i, n, h)
	if str(exportN, "public_host") != "edge.example" || num(exportI, "port") != 8443 || str(exportI, "reality_server_name") != "sni.example" {
		t.Fatal(exportI, exportN)
	}
	if num(i, "port") != 443 || str(n, "public_host") != "original.example" {
		t.Fatal("listener mutated")
	}
	bad := clone(h)
	bad["address"] = "example\n.com"
	if validateSubscriptionHosts([]any{bad}) == nil {
		t.Fatal("header injection accepted")
	}
	if validateSubscriptionHosts([]any{h, h}) == nil {
		t.Fatal("duplicate host accepted")
	}
}
func TestIntegrationHWIDLimitConcurrentRevocationAndScope(t *testing.T) {
	s, ctx := integration(t)
	_, _, c := fixture(t, s, ctx, &nodeMock{running: true})
	cfg, err := settings(ctx, s.Store.Pool)
	if err != nil {
		t.Fatal(err)
	}
	cfg["subscription_hwid_limit"] = 1
	if err = saveSettings(ctx, s.Store.Pool, cfg); err != nil {
		t.Fatal(err)
	}
	fetch := func(method, hwid, format string) int {
		r := httptest.NewRequest(method, "/sub/"+str(c, "subscription_token")+"?format="+format, nil)
		r.Header.Set("X-HWID", hwid)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w.Code
	}
	if fetch("GET", "", "raw") != 403 {
		t.Fatal("missing HWID bypassed requirement")
	}
	if fetch("HEAD", "head-device", "raw") != 200 {
		t.Fatal("HEAD preflight rejected")
	}
	out, err := s.clientConnections(ctx, num(c, "id"), "devices")
	if err != nil || num(out, "count") != 0 {
		t.Fatal("HEAD registered device", out, err)
	}
	if fetch("GET", "browser", "portal") != 200 {
		t.Fatal("browser portal must remain available")
	}
	var wg sync.WaitGroup
	results := make(chan int, 16)
	for index := 0; index < 16; index++ {
		wg.Add(1)
		go func(index int) { defer wg.Done(); results <- fetch("GET", fmt.Sprintf("device-%d", index), "raw") }(index)
	}
	wg.Wait()
	close(results)
	accepted := 0
	for code := range results {
		if code == 200 {
			accepted++
		} else if code != 403 {
			t.Fatal("unexpected status", code)
		}
	}
	if accepted != 1 {
		t.Fatal("parallel admission bypassed cap", accepted)
	}
	out, err = s.clientConnections(ctx, num(c, "id"), "devices")
	if err != nil || num(out, "count") != 1 {
		t.Fatal(out, err)
	}
	device := out["items"].([]Data)[0]
	action := func(a Actor, what string) error {
		body, _ := json.Marshal(Data{"device_id": num(device, "id"), "action": what})
		path := fmt.Sprintf("clients/%d/device-action", num(c, "id"))
		_, _, err := s.api(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/"+path, bytes.NewReader(body)), path, a)
		return err
	}
	if action(Actor{User: Data{"id": 99, "role": "reseller"}, Scope: "write"}, "revoke") == nil {
		t.Fatal("foreign device mutation allowed")
	}
	admin := Actor{User: Data{"is_superuser": true}, Scope: "admin"}
	if err = action(admin, "revoke"); err != nil {
		t.Fatal(err)
	}
	if fetch("GET", str(device, "hwid"), "raw") != 403 {
		t.Fatal("revoked device admitted")
	}
	if fetch("GET", "replacement", "raw") != 200 {
		t.Fatal("revoked entry did not release slot")
	}
	if fetch("HEAD", "third-device", "raw") != 403 {
		t.Fatal("HEAD bypassed cap")
	}
	if fetch("GET", "invalid-format-device", "invalid") != 400 {
		t.Fatal("unknown format accepted")
	}
}
func TestIntegrationDeliveryHostsDiagnosticsAndMetrics(t *testing.T) {
	s, ctx := integration(t)
	_, i, c := fixture(t, s, ctx, &nodeMock{running: true})
	cfg, _ := settings(ctx, s.Store.Pool)
	cfg["subscription_hosts"] = []any{Data{"inbound": num(i, "id"), "name": "CDN", "address": "edge.example", "port": 8443, "enabled": true}}
	if err := saveSettings(ctx, s.Store.Pool, cfg); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/sub/"+str(c, "subscription_token")+"?format=raw", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "edge.example:8443") {
		t.Fatal(w.Code, w.Body.String())
	}
	check, err := s.deliveryCheck(ctx, num(c, "id"))
	if err != nil || flag(check, "network_tested") || len(array(check, "checks")) != 1 {
		t.Fatal(check, err)
	}
	for _, actor := range []Actor{{User: Data{"role": "reseller"}, Scope: "read"}, {User: Data{"role": "viewer"}, Scope: "read"}} {
		if s.roleAuthorize(actor, "GET", "metrics") == nil {
			t.Fatal("restricted metrics exposed")
		}
	}
	admin := Actor{User: Data{"is_superuser": true}, Scope: "read"}
	if err = s.roleAuthorize(admin, "GET", "metrics"); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	s.prometheus(w, httptest.NewRequest(http.MethodGet, "/api/metrics", nil), admin)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "# TYPE veloray_nodes gauge") || strings.Contains(w.Body.String(), str(c, "credential")) || strings.Contains(w.Body.String(), str(c, "subscription_token")) {
		t.Fatal("metrics invalid or contain secrets", w.Body.String())
	}
}
