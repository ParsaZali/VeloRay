package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

type nodeMock struct {
	value                int64
	running, failApply   bool
	applies, rollbacks   int
	config               Data
	statsCalls, restarts int
	portConflict         bool
	stats                []any
	statMetadata         Data
}

func (m *nodeMock) handler(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/health":
		writeJSON(w, 200, Data{"status": "ok", "xray": Data{"running": m.running, "version": "test"}})
	case "/xray/status":
		writeJSON(w, 200, Data{"running": m.running})
	case "/xray/stats":
		m.statsCalls++
		if !m.running {
			writeJSON(w, 503, Data{"detail": "Xray is stopped"})
			return
		}
		entries := m.stats
		if entries == nil {
			entries = []any{Data{"name": "user>>>veloray:1:alice>>>traffic>>>uplink", "value": m.value}}
		}
		writeJSON(w, 200, merge(Data{"accounting": "durable-v1", "ledger_id": "stable-ledger", "stat": entries}, m.statMetadata))
	case "/xray/validate":
		if m.portConflict {
			var input Data
			_ = json.NewDecoder(r.Body).Decode(&input)
			for _, v := range array(obj(input, "config"), "inbounds") {
				if in, ok := v.(map[string]any); ok && num(in, "port") == 443 {
					writeJSON(w, 503, Data{"detail": "port conflict: nginx on 443"})
					return
				}
			}
		}
		writeJSON(w, 200, Data{"status": "valid"})
	case "/xray/ports":
		var input Data
		_ = json.NewDecoder(r.Body).Decode(&input)
		checks := []any{}
		for _, v := range array(obj(input, "config"), "inbounds") {
			if in, ok := v.(map[string]any); ok {
				checks = append(checks, Data{"tag": in["tag"], "listen": in["listen"], "port": in["port"], "network": "tcp", "state": "free"})
			}
		}
		writeJSON(w, 200, Data{"ports": checks})
	case "/xray/apply":
		m.applies++
		if m.failApply {
			writeJSON(w, 503, Data{"detail": "simulated failure"})
			return
		}
		var in Data
		_ = json.NewDecoder(r.Body).Decode(&in)
		m.config = obj(in, "config")
		writeJSON(w, 200, Data{"status": "applied"})
	case "/xray/rollback":
		m.rollbacks++
		writeJSON(w, 200, Data{"status": "rolled_back"})
	case "/xray/restart":
		m.restarts++
		m.running = true
		writeJSON(w, 200, Data{"status": "restarted"})
	default:
		http.NotFound(w, r)
	}
}
func TestIntegrationPortRecoveryKeepsLinksAndDataConsistent(t *testing.T) {
	s, ctx := integration(t)
	m := &nodeMock{running: false, portConflict: true}
	n, i, c := fixture(t, s, ctx, m)
	n["is_local"] = true
	if err := save(ctx, s.Store.Pool, "nodes", n); err != nil {
		t.Fatal(err)
	}
	c["used_traffic_bytes"] = int64(1234)
	if err := save(ctx, s.Store.Pool, "clients", c); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ChangeLocalInboundPort(ctx, num(i, "id"), 2053); err != nil {
		t.Fatal(err)
	}
	updated, err := get(ctx, s.Store.Pool, "inbounds", num(i, "id"))
	if err != nil || num(updated, "port") != 2053 {
		t.Fatal(updated, err)
	}
	view, err := s.view(ctx, s.Store.Pool, "clients", c)
	if err != nil || !strings.Contains(str(view, "share_link"), ":2053") {
		t.Fatal("client link stayed on the old port", view, err)
	}
	fresh, err := get(ctx, s.Store.Pool, "clients", num(c, "id"))
	if err != nil || str(fresh, "credential") != str(c, "credential") || num(fresh, "used_traffic_bytes") != 1234 {
		t.Fatal("client data changed", fresh, err)
	}
	if m.applies != 1 {
		t.Fatal("port recovery did not deploy", m.applies)
	}
	if _, err = s.ChangeLocalInboundPort(ctx, num(i, "id"), 443); err == nil {
		t.Fatal("occupied port accepted")
	}
	updated, _ = get(ctx, s.Store.Pool, "inbounds", num(i, "id"))
	if num(updated, "port") != 2053 || m.applies != 1 {
		t.Fatal("failed change modified the database or runtime", updated, m.applies)
	}
}
func TestIntegrationBatchClientsDeploysOnceAndRollsBack(t *testing.T) {
	s, ctx := integration(t)
	m := &nodeMock{running: true}
	_, i, _ := fixture(t, s, ctx, m)
	create := func(prefix string, count int) (any, error) {
		req := httptest.NewRequest("POST", "/", strings.NewReader(fmt.Sprintf(`{"name":%q,"inbound":%d,"count":%d,"traffic_limit_bytes":1048576}`, prefix, num(i, "id"), count))).WithContext(ctx)
		out, _, err := s.api(httptest.NewRecorder(), req, "clients/batch", Actor{})
		return out, err
	}
	if _, err := create("batch", 3); err != nil {
		t.Fatal(err)
	}
	rows, err := list(ctx, s.Store.Pool, "clients", "ORDER BY id")
	if err != nil || len(rows) != 4 || m.applies != 1 {
		t.Fatal("batch did not use one deployment", len(rows), m.applies, err)
	}
	seen := map[string]bool{}
	for _, row := range rows {
		if seen[str(row, "credential")] || seen[str(row, "subscription_token")] {
			t.Fatal("reused credential or token")
		}
		seen[str(row, "credential")] = true
		seen[str(row, "subscription_token")] = true
	}
	if _, err = create("invalid", 101); err == nil {
		t.Fatal("unbounded batch accepted")
	}
	m.failApply = true
	if _, err = create("failed", 2); err == nil {
		t.Fatal("failed deployment reported success")
	}
	rows, _ = list(ctx, s.Store.Pool, "clients", "")
	if len(rows) != 4 {
		t.Fatal("partial batch survived rollback", len(rows))
	}
}
func TestIntegrationBulkActionsAreAtomicAndRespectExpiry(t *testing.T) {
	s, ctx := integration(t)
	m := &nodeMock{running: true}
	_, i, c := fixture(t, s, ctx, m)
	second := defaults("clients")
	second["name"] = "bob"
	second["inbound"] = num(i, "id")
	second["credential"] = uuid()
	second["subscription_token"] = randomToken(32)
	if err := save(ctx, s.Store.Pool, "clients", second); err != nil {
		t.Fatal(err)
	}
	call := func(action string) error {
		req := httptest.NewRequest("POST", "/", strings.NewReader(fmt.Sprintf(`{"ids":[%d,%d],"action":%q}`, num(c, "id"), num(second, "id"), action))).WithContext(ctx)
		_, _, err := s.api(httptest.NewRecorder(), req, "clients/bulk", Actor{})
		return err
	}
	if err := call("disable"); err != nil || m.applies != 1 {
		t.Fatal("bulk disable did not deploy once", err, m.applies)
	}
	m.failApply = true
	if err := call("enable"); err == nil {
		t.Fatal("failed bulk deployment accepted")
	}
	rows, _ := list(ctx, s.Store.Pool, "clients", "")
	for _, row := range rows {
		if flag(row, "enabled") {
			t.Fatal("partial enable committed")
		}
	}
	m.failApply = false
	if err := call("enable"); err != nil {
		t.Fatal(err)
	}
	fresh, _ := get(ctx, s.Store.Pool, "clients", num(c, "id"))
	fresh["enabled"] = false
	fresh["expires_at"] = stamp(time.Now().Add(-time.Hour))
	if err := save(ctx, s.Store.Pool, "clients", fresh); err != nil {
		t.Fatal(err)
	}
	before := m.applies
	if err := call("enable"); err == nil || m.applies != before {
		t.Fatal("expired account was enabled or changed runtime", err, m.applies)
	}
}

func TestServeRejectsOccupiedPortBeforeDatabase(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	err = Serve(context.Background(), Config{Listen: listener.Addr().String(), DatabaseURL: "invalid"})
	if err == nil || !strings.Contains(err.Error(), "cannot listen") {
		t.Fatal("duplicate server reached database startup", err)
	}
}

func TestIntegrationValidateAndRecoverStoppedXray(t *testing.T) {
	s, ctx := integration(t)
	m := &nodeMock{running: false}
	n, _, _ := fixture(t, s, ctx, m)
	req := httptest.NewRequest("POST", "/", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	if _, _, err := s.action(w, req, "nodes", num(n, "id"), "config-validate", Actor{}); err != nil {
		t.Fatal(err)
	}
	if m.applies != 0 || m.restarts != 0 {
		t.Fatal("validation changed the running service")
	}
	if _, _, err := s.action(w, req, "nodes", num(n, "id"), "xray-restart", Actor{}); err != nil {
		t.Fatal(err)
	}
	if !m.running || m.restarts != 1 || m.statsCalls != 0 {
		t.Fatal("stopped runtime could not be recovered without live counters", m)
	}
}
func integration(t *testing.T) (*Server, context.Context) {
	t.Helper()
	dsn := os.Getenv("VELORAY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set VELORAY_TEST_DATABASE_URL to an isolated PostgreSQL database")
	}
	ctx := context.Background()
	store, e := OpenStore(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(store.Pool.Close)
	if e = store.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	_, e = store.Pool.Exec(ctx, `TRUNCATE vr_monitor_samples,vr_monitors,vr_node_billing,vr_node_billing_periods,vr_alert_states, vr_oidc_states,vr_oidc_challenges,vr_webhook_endpoints,vr_subscription_policies,vr_subscription_requests,vr_profiles,vr_snippets,vr_config_revisions,vr_services,vr_accounts,vr_nodes,vr_inbounds,vr_clients,vr_users,vr_api_keys,vr_audit,vr_sessions,vr_stats_cursors,vr_traffic_windows,vr_owner_traffic_windows,vr_jobs,vr_operations,vr_rate_limits,vr_telegram_updates,vr_telegram_actions,vr_webhook_deliveries RESTART IDENTITY CASCADE`)
	if e != nil {
		t.Fatal(e)
	}
	if e = saveSettings(ctx, store.Pool, defaultSettings()); e != nil {
		t.Fatal(e)
	}
	s := &Server{Config: Config{PublicURL: "http://127.0.0.1:8610", SecretKey: strings.Repeat("s", 32), FieldKey: strings.Repeat("f", 32), SessionAge: time.Hour, PanelPort: 8443, TrustedOrigins: map[string]bool{"http://127.0.0.1:8610": true}}, Store: store, Logger: slog.Default()}
	return s, ctx
}
func fixture(t *testing.T, s *Server, ctx context.Context, m *nodeMock) (Data, Data, Data) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(m.handler))
	t.Cleanup(server.Close)
	token, e := seal(s.Config.FieldKey, strings.Repeat("a", 32))
	if e != nil {
		t.Fatal(e)
	}
	n := defaults("nodes")
	n["name"] = fmt.Sprintf("node-%d", time.Now().UnixNano())
	n["public_host"] = "vpn.example"
	n["agent_url"] = server.URL
	n["_agent_token"] = token
	if e = save(ctx, s.Store.Pool, "nodes", n); e != nil {
		t.Fatal(e)
	}
	i := defaults("inbounds")
	i["node"] = num(n, "id")
	i["name"] = "main"
	i["port"] = 443
	if e = save(ctx, s.Store.Pool, "inbounds", i); e != nil {
		t.Fatal(e)
	}
	c := defaults("clients")
	c["inbound"] = num(i, "id")
	c["name"] = "alice"
	c["credential"] = uuid()
	c["subscription_token"] = randomToken(32)
	if e = save(ctx, s.Store.Pool, "clients", c); e != nil {
		t.Fatal(e)
	}
	return n, i, c
}
func TestIntegrationAccountingResetFailureAndHistory(t *testing.T) {
	s, ctx := integration(t)
	m := &nodeMock{running: true, value: 100}
	n, _, c := fixture(t, s, ctx, m)
	collect := func() {
		tx, e := s.Store.Pool.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback(ctx)
		fresh, _ := get(ctx, tx, "nodes", num(n, "id"))
		if e = s.collect(ctx, tx, fresh); e != nil {
			t.Fatal(e)
		}
		if e = tx.Commit(ctx); e != nil {
			t.Fatal(e)
		}
	}
	collect()
	collect()
	fresh, _ := get(ctx, s.Store.Pool, "clients", num(c, "id"))
	if num(fresh, "used_traffic_bytes") != 100 {
		t.Fatal("double counted", fresh)
	}
	m.failApply = true
	_, e := s.change(ctx, []int64{num(n, "id")}, Actor{}, "deploy", "node", func(pgx.Tx) (any, error) { return nil, nil })
	if e == nil {
		t.Fatal("expected deploy failure")
	}
	collect()
	fresh, _ = get(ctx, s.Store.Pool, "clients", num(c, "id"))
	if num(fresh, "used_traffic_bytes") != 100 {
		t.Fatal("failed deploy doubled usage", fresh)
	}
	m.failApply = false
	m.value = 125
	_, _, e = s.action(httptest.NewRecorder(), httptest.NewRequest("POST", "/reset", nil), "clients", num(c, "id"), "reset-usage", Actor{})
	if e != nil {
		t.Fatal(e)
	}
	fresh, _ = get(ctx, s.Store.Pool, "clients", num(c, "id"))
	if num(fresh, "used_traffic_bytes") != 0 || num(fresh, "lifetime_traffic_bytes") != 125 {
		t.Fatal("reset harvested old usage after zeroing", fresh)
	}
	collect()
	fresh, _ = get(ctx, s.Store.Pool, "clients", num(c, "id"))
	if num(fresh, "used_traffic_bytes") != 0 {
		t.Fatal("reset cursor not advanced")
	}
	if e = remove(ctx, s.Store.Pool, "clients", num(c, "id")); e != nil {
		t.Fatal(e)
	}
	var history int64
	if e = s.Store.Pool.QueryRow(ctx, `SELECT COALESCE(sum(bytes),0) FROM vr_traffic_windows`).Scan(&history); e != nil || history != 125 {
		t.Fatal("history depends on current quota sum", history, e)
	}
}
func TestIntegrationPolicyRetriesAndRenewal(t *testing.T) {
	s, ctx := integration(t)
	m := &nodeMock{running: true, value: 100, failApply: true}
	n, _, c := fixture(t, s, ctx, m)
	c["traffic_limit_bytes"] = 50
	if e := save(ctx, s.Store.Pool, "clients", c); e != nil {
		t.Fatal(e)
	}
	if e := s.reconcileNode(ctx, num(n, "id")); e != nil {
		t.Fatal(e)
	}
	fresh, _ := get(ctx, s.Store.Pool, "clients", num(c, "id"))
	if flag(fresh, "enabled") {
		t.Fatal("quota not enforced")
	}
	if e := s.runJob(ctx, num(n, "id")); e == nil {
		t.Fatal("expected failed apply")
	}
	m.failApply = false
	_, _ = s.Store.Pool.Exec(ctx, `UPDATE vr_jobs SET next_at=now()`)
	if e := s.runJob(ctx, num(n, "id")); e != nil {
		t.Fatal(e)
	}
	if m.applies != 2 {
		t.Fatal("disabled client never retried", m.applies)
	}
	fresh["renewal_interval_days"] = 1
	fresh["next_renewal_at"] = stamp(time.Now().Add(-time.Hour))
	if e := save(ctx, s.Store.Pool, "clients", fresh); e != nil {
		t.Fatal(e)
	}
	if e := s.reconcileNode(ctx, num(n, "id")); e != nil {
		t.Fatal(e)
	}
	fresh, _ = get(ctx, s.Store.Pool, "clients", num(c, "id"))
	if num(fresh, "used_traffic_bytes") != 0 || !flag(fresh, "enabled") {
		t.Fatal("renewal failed", fresh)
	}
}
func TestIntegrationTwoNodeFailureCompensates(t *testing.T) {
	s, ctx := integration(t)
	first := &nodeMock{running: true}
	second := &nodeMock{running: true, failApply: true}
	n1, _, _ := fixture(t, s, ctx, first)
	n2, _, _ := fixture(t, s, ctx, second)
	_, e := s.change(ctx, []int64{num(n1, "id"), num(n2, "id")}, Actor{}, "move", "test", func(tx pgx.Tx) (any, error) {
		n, _ := get(ctx, tx, "nodes", num(n1, "id"))
		n["name"] = "changed"
		return nil, save(ctx, tx, "nodes", n)
	})
	if e == nil {
		t.Fatal("expected failure")
	}
	n, _ := get(ctx, s.Store.Pool, "nodes", num(n1, "id"))
	if str(n, "name") == "changed" || first.rollbacks != 1 {
		t.Fatal("database/runtime not compensated", n, first.rollbacks)
	}
}
func TestIntegrationMandatory2FAAppliesToAdminAPI(t *testing.T) {
	s, ctx := integration(t)
	u := defaults("users")
	u["username"] = "admin"
	u["is_superuser"] = true
	u["_password"], _ = hashPassword("Very-long-s3cret")
	if e := save(ctx, s.Store.Pool, "users", u); e != nil {
		t.Fatal(e)
	}
	cfg := defaultSettings()
	cfg["require_2fa_for_admins"] = true
	_ = saveSettings(ctx, s.Store.Pool, cfg)
	a := Actor{User: u, SessionHash: "session", Scope: "admin"}
	for _, p := range []string{"admin/users", "admin/api-keys", "settings", "nodes"} {
		if e := s.authorize(ctx, a, "POST", p); e == nil {
			t.Fatal("2FA bypass", p)
		}
	}
	if e := s.authorize(ctx, a, "GET", "auth/2fa/status"); e != nil {
		t.Fatal("cannot enroll", e)
	}
	a.SessionHash = ""
	if s.authorize(ctx, a, "GET", "auth/me") == nil {
		t.Fatal("API key bypass")
	}
}
func TestIntegrationSubscriptionsFormatsAndClosedEmptyProxies(t *testing.T) {
	s, ctx := integration(t)
	m := &nodeMock{running: true}
	n, i, c := fixture(t, s, ctx, m)
	for _, format := range []string{"base64", "raw", "clash", "json", "portal"} {
		r := httptest.NewRequest("GET", "/sub/"+str(c, "subscription_token")+"?format="+format, nil)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", format, w.Code, w.Body.String())
		}
	}
	for _, p := range []string{"http", "socks"} {
		i["protocol"] = p
		settings, e := inboundSettings(i, n, nil)
		if e != nil || settings != nil {
			t.Fatal("empty proxy opens listener", p, settings, e)
		}
	}
	m.running = false
	if _, e := s.probe(ctx, num(n, "id")); e != nil {
		t.Fatal(e)
	}
	fresh, _ := get(ctx, s.Store.Pool, "nodes", num(n, "id"))
	if str(fresh, "status") != "degraded" {
		t.Fatal("stopped runtime healthy", fresh)
	}
}
func TestIntegrationLoginAndSingleUseRecovery(t *testing.T) {
	s, ctx := integration(t)
	u := defaults("users")
	u["username"] = "admin"
	u["is_superuser"] = true
	u["_password"], _ = hashPassword("Very-long-s3cret")
	u["_totp_enabled"] = true
	u["_totp_secret"], _ = seal(s.Config.FieldKey, "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ")
	u["_recovery"] = []any{digest("RECOVERY-CODE")}
	_ = save(ctx, s.Store.Pool, "users", u)
	token := csrfToken(s.Config.SecretKey)
	login := func(code string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(fmt.Sprintf(`{"username":"admin","password":"Very-long-s3cret","totp_code":%q}`, code)))
		r.Header.Set("Origin", s.Config.PublicURL)
		r.Header.Set("X-CSRFToken", token)
		r.AddCookie(&http.Cookie{Name: "csrftoken", Value: token})
		s.Handler().ServeHTTP(w, r)
		return w
	}
	if w := login(""); w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := login("RECOVERY-CODE"); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := login("RECOVERY-CODE"); w.Code != 401 {
		t.Fatal("recovery replay", w.Code, w.Body.String())
	}
}

func TestIntegrationCrashRecoveryAndProtectedPatch(t *testing.T) {
	s, ctx := integration(t)
	m := &nodeMock{running: true}
	n, _, _ := fixture(t, s, ctx, m)
	ids, _ := json.Marshal([]int64{num(n, "id")})
	op := "crashed-operation-12345"
	if _, e := s.Store.Pool.Exec(ctx, `INSERT INTO vr_operations(id,nodes,created_at) VALUES($1,$2,now()-interval '3 minutes')`, op, string(ids)); e != nil {
		t.Fatal(e)
	}
	if e := s.RecoverOperations(ctx); e != nil {
		t.Fatal(e)
	}
	if m.rollbacks != 1 {
		t.Fatal("orphaned deployment was not compensated")
	}
	n["config_patch"] = Data{"api": Data{"tag": "public"}, "policy": Data{"levels": Data{"0": Data{"statsUserUplink": false}}}, "inbounds": []any{Data{"tag": "api", "listen": "0.0.0.0", "port": 10085}}, "routing": Data{"rules": []any{}}}
	cfg, e := buildConfig(ctx, s.Store.Pool, n)
	if e != nil {
		t.Fatal(e)
	}
	if str(obj(cfg, "api"), "tag") != "api" || !flag(obj(obj(obj(cfg, "policy"), "levels"), "0"), "statsUserUplink") {
		t.Fatal("advanced patch disabled accounting", cfg)
	}
	b, _ := json.Marshal(array(cfg, "inbounds")[0])
	api, _ := decode(b)
	if str(api, "listen") != "127.0.0.1" {
		t.Fatal("API exposed by patch", api)
	}
}

func TestIntegrationDirectionalTrafficHistoryAndOwnership(t *testing.T) {
	s, ctx := integration(t)
	m := &nodeMock{running: true}
	n, _, c := fixture(t, s, ctx, m)
	u := defaults("users")
	u["username"], u["own_clients_only"] = "traffic-owner", true
	if err := save(ctx, s.Store.Pool, "users", u); err != nil {
		t.Fatal(err)
	}
	c["owner_id"], c["traffic_multiplier"] = num(u, "id"), 3
	if err := save(ctx, s.Store.Pool, "clients", c); err != nil {
		t.Fatal(err)
	}
	m.stats = []any{
		Data{"name": fmt.Sprintf("user>>>veloray:%d:alice>>>traffic>>>uplink", num(c, "id")), "value": 100},
		Data{"name": fmt.Sprintf("user>>>veloray:%d:alice>>>traffic>>>downlink", num(c, "id")), "value": 200},
	}
	for attempt := 0; attempt < 2; attempt++ {
		tx, err := s.Store.Pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		fresh, err := get(ctx, tx, "nodes", num(n, "id"))
		if err == nil {
			err = s.collect(ctx, tx, fresh)
		}
		if err != nil {
			tx.Rollback(ctx)
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	for _, actor := range []Actor{{}, {User: u}} {
		o, err := s.overviewForActor(ctx, actor)
		if err != nil || num(o, "traffic_used_bytes") != 900 || num(o, "traffic_raw_bytes") != 300 || num(o, "traffic_upload_bytes") != 100 || num(o, "traffic_download_bytes") != 200 {
			t.Fatal("overview multiplied or duplicated real traffic", o, err)
		}
		h, err := s.trafficHistoryForActor(ctx, httptest.NewRequest("GET", "/traffic-history?hours=24", nil), actor)
		if err != nil || len(h) != 1 || num(h[0], "upload_bytes") != 100 || num(h[0], "download_bytes") != 200 || num(h[0], "used_traffic_bytes") != 300 {
			t.Fatal("history lost directions", h, err)
		}
	}
	other := Actor{User: Data{"id": num(u, "id") + 1, "own_clients_only": true}}
	if h, err := s.trafficHistoryForActor(ctx, httptest.NewRequest("GET", "/traffic-history", nil), other); err != nil || len(h) != 0 {
		t.Fatal("another owner saw traffic", h, err)
	}
	if err := s.Store.Migrate(ctx); err != nil {
		t.Fatal("migration is not repeatable", err)
	}
}
