package panel

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAlertAndBillingBoundaries(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	cfg := merge(defaultSettings(), Data{"alert_quota_percent": 80, "alert_expiry_days": 3, "alert_inactive_days": 7})
	c := merge(defaults("clients"), Data{"traffic_limit_bytes": 100, "used_traffic_bytes": 85, "lifetime_traffic_bytes": 185, "expires_at": stamp(now.Add(24 * time.Hour)), "last_traffic_at": stamp(now.Add(-8 * 24 * time.Hour))})
	conditions := alertConditions(c, cfg, now)
	if len(conditions) != 3 {
		t.Fatal(conditions)
	}
	old := conditions["client.quota_warning"]
	c["lifetime_traffic_bytes"] = 285
	if alertConditions(c, cfg, now)["client.quota_warning"] == old {
		t.Fatal("quota reset cycle not distinguished")
	}
	c["enabled"] = false
	if len(alertConditions(c, cfg, now)) != 0 {
		t.Fatal("disabled customer alerts")
	}
	for _, at := range []time.Time{time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC), time.Date(2025, 2, 28, 0, 0, 0, 0, time.UTC)} {
		start, end := billingBounds(at, 31, "UTC")
		if start.After(at) || !end.After(at) || end.Day() != 31 || end.Month() != 3 {
			t.Fatal(start, end)
		}
	}
	for _, invalid := range []string{"a@example.com\r\nBcc: victim@example.com", "invalid", ""} {
		if _, e := emailRecipients(invalid); e == nil {
			t.Fatal("bad recipient", invalid)
		}
	}
}
func TestIntegrationEmailQueueAlertsAndEncryptedSettings(t *testing.T) {
	s, ctx := integration(t)
	_, i, _ := fixture(t, s, ctx, &nodeMock{running: true, stats: []any{}})
	admin := Actor{User: Data{"is_superuser": true}, Scope: "admin"}
	body := Data{"smtp_enabled": true, "smtp_host": "smtp.example.com", "smtp_port": 465, "smtp_tls_mode": "tls", "smtp_from": "notify@example.com", "smtp_username": "smtp-user", "smtp_password": "private-smtp-password", "alert_quota_percent": 80, "alert_expiry_days": 3, "alert_inactive_days": 7}
	out, _, e := managementRequest(t, s, ctx, admin, "PATCH", "settings", body)
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(mustJSON(out)), "private-smtp-password") {
		t.Fatal("SMTP password response leak")
	}
	stored, _ := settings(ctx, s.Store.Pool)
	if strings.Contains(str(stored, "_smtp_password"), "private-smtp-password") {
		t.Fatal("SMTP password unencrypted")
	}
	out, _, e = managementRequest(t, s, ctx, admin, "POST", "webhooks/endpoints", Data{"name": "Mail", "channel": "email", "url": "ops@example.com", "enabled": true, "events": []any{"client.*"}})
	if e != nil {
		t.Fatal(e)
	}
	_, _, e = managementRequest(t, s, ctx, admin, "POST", "customers", Data{"name": "Alert buyer", "inbound_ids": []any{num(i, "id")}, "traffic_limit_bytes": 100, "expires_at": stamp(time.Now().Add(24 * time.Hour))})
	if e != nil {
		t.Fatal(e)
	}
	customer, e := get(ctx, s.Store.Pool, "customers", 1)
	if e != nil {
		t.Fatal(e)
	}
	customer["used_traffic_bytes"], customer["lifetime_traffic_bytes"] = 85, 85
	customer["last_traffic_at"] = stamp(time.Now().Add(-8 * 24 * time.Hour))
	if e = save(ctx, s.Store.Pool, "customers", customer); e != nil {
		t.Fatal(e)
	}
	for n := 0; n < 2; n++ {
		if e = s.scanLifecycleAlerts(ctx); e != nil {
			t.Fatal(e)
		}
	}
	var count int
	if e = s.Store.Pool.QueryRow(ctx, "SELECT count(*) FROM vr_webhook_deliveries").Scan(&count); e != nil || count != 3 {
		t.Fatal("shared alert duplicate or missing", count, e)
	}
	sent := 0
	s.EmailSend = func(ctx context.Context, from string, to []string, message []byte) error {
		sent++
		if from != "notify@example.com" || len(to) != 1 || to[0] != "ops@example.com" || strings.Contains(string(message), "private-smtp-password") || !strings.Contains(string(message), "Message-ID: <veloray-") {
			t.Fatal("mail privacy or envelope", string(message))
		}
		return nil
	}
	if e = s.deliverWebhookQueue(ctx); e != nil || sent != 3 {
		t.Fatal(sent, e)
	}
	customer["used_traffic_bytes"] = 0
	if e = save(ctx, s.Store.Pool, "customers", customer); e != nil {
		t.Fatal(e)
	}
	if e = s.scanLifecycleAlerts(ctx); e != nil {
		t.Fatal(e)
	}
	customer["used_traffic_bytes"], customer["lifetime_traffic_bytes"] = 85, 170
	if e = save(ctx, s.Store.Pool, "customers", customer); e != nil {
		t.Fatal(e)
	}
	if e = s.scanLifecycleAlerts(ctx); e != nil {
		t.Fatal(e)
	}
	if e = s.Store.Pool.QueryRow(ctx, "SELECT count(*) FROM vr_webhook_deliveries").Scan(&count); e != nil || count != 4 {
		t.Fatal("new quota cycle alert missing", count, e)
	}
	// Global SMTP disable pauses queued mail without consuming retry attempts.
	_, _, e = managementRequest(t, s, ctx, admin, "PATCH", "settings", Data{"smtp_enabled": false})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.deliverWebhookQueue(ctx); e != nil || sent != 3 {
		t.Fatal(sent, e)
	}
}
func TestIntegrationNodeBillingPreservesRawAccountingAndPrivacy(t *testing.T) {
	s, ctx := integration(t)
	n, _, _ := fixture(t, s, ctx, &nodeMock{running: true, stats: []any{}})
	id := num(n, "id")
	admin := Actor{User: Data{"is_superuser": true}, Scope: "admin"}
	path := fmt.Sprintf("operations/nodes/%d/billing", id)
	_, _, e := managementRequest(t, s, ctx, admin, "PATCH", path, Data{"provider": "Private hosting", "reference": "private-contract", "budget_bytes": 100, "warning_percent": 80, "reset_day": 31})
	if e != nil {
		t.Fatal(e)
	}
	n, _ = get(ctx, s.Store.Pool, "nodes", id)
	if strings.Contains(string(mustJSON(public(n))), "private-contract") {
		t.Fatal("billing metadata leaked")
	}
	tx, e := s.Store.Pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	now := time.Now().UTC()
	if e = s.recordNodeBilling(ctx, tx, n, 85, now); e != nil {
		t.Fatal(e)
	}
	if e = s.recordNodeBilling(ctx, tx, n, 5, now); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	out, _, e := managementRequest(t, s, ctx, admin, "GET", path, nil)
	if e != nil || num(obj(out.(Data), "usage"), "used_bytes") != 90 {
		t.Fatal(out, e)
	}
	var count int
	s.Store.Pool.QueryRow(ctx, "SELECT count(*) FROM vr_audit WHERE data->>'action'='node.budget_warning'").Scan(&count)
	if count != 1 {
		t.Fatal("budget warning duplicate", count)
	}
	if _, e = s.Store.Pool.Exec(ctx, "UPDATE vr_node_billing SET cycle_end=now()-interval '1 minute' WHERE node_id=$1", id); e != nil {
		t.Fatal(e)
	}
	tx, _ = s.Store.Pool.Begin(ctx)
	if e = s.recordNodeBilling(ctx, tx, n, 2, now); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	out, _, e = managementRequest(t, s, ctx, admin, "GET", path, nil)
	if e != nil || num(obj(out.(Data), "usage"), "used_bytes") != 2 || num(obj(out.(Data), "usage"), "lifetime_bytes") != 92 || len(out.(Data)["history"].([]Data)) != 1 {
		t.Fatal(out, e)
	}
	for _, resource := range []string{"operations/nodes/1/billing", "monitors"} {
		if e = s.roleAuthorize(Actor{User: Data{"role": "viewer"}, Scope: "admin"}, "GET", resource); e == nil {
			t.Fatal("operations permission bypass")
		}
	}
}
func TestIntegrationMonitorFailuresRecoveryHistoryAndPrivacy(t *testing.T) {
	s, ctx := integration(t)
	n, _, _ := fixture(t, s, ctx, &nodeMock{running: true, stats: []any{}})
	ok := false
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, Data{"ok": ok, "latency_ms": 7, "status_code": 204, "error": "secret remote echo", "url": "private-feed-token"})
	}))
	defer agent.Close()
	n["agent_url"] = agent.URL
	if e := save(ctx, s.Store.Pool, "nodes", n); e != nil {
		t.Fatal(e)
	}
	admin := Actor{User: Data{"is_superuser": true}, Scope: "admin"}
	out, _, e := managementRequest(t, s, ctx, admin, "POST", "monitors", Data{"name": "Core path", "node": num(n, "id"), "url": "https://probe.example/secret-target-token", "failure_threshold": 2})
	if e != nil {
		t.Fatal(e)
	}
	id := num(out.(Data), "id")
	path := fmt.Sprintf("monitors/%d", id)
	out, _, e = managementRequest(t, s, ctx, admin, "GET", "monitors", nil)
	if e != nil || strings.Contains(string(mustJSON(out)), "secret-target-token") {
		t.Fatal("monitor source exposed", out, e)
	}
	for j := 0; j < 3; j++ {
		out, _, e = managementRequest(t, s, ctx, admin, "POST", path+"/probe", Data{})
		if e != nil || strings.Contains(string(mustJSON(out)), "secret remote") {
			t.Fatal(out, e)
		}
	}
	ok = true
	_, _, e = managementRequest(t, s, ctx, admin, "POST", path+"/probe", Data{})
	if e != nil {
		t.Fatal(e)
	}
	var count int
	s.Store.Pool.QueryRow(ctx, "SELECT count(*) FROM vr_audit WHERE data->>'action' IN('monitor.down','monitor.recovered')").Scan(&count)
	if count != 2 {
		t.Fatal("monitor transition duplicate", count)
	}
	out, _, e = managementRequest(t, s, ctx, admin, "GET", path+"/history", nil)
	if e != nil || len(out.([]Data)) != 4 || strings.Contains(string(mustJSON(out)), "private-feed-token") {
		t.Fatal(out, e)
	}
	_, _, e = managementRequest(t, s, ctx, admin, "DELETE", path, nil)
	if e != nil {
		t.Fatal(e)
	}
	s.Store.Pool.QueryRow(ctx, "SELECT count(*) FROM vr_monitor_samples").Scan(&count)
	if count != 0 {
		t.Fatal("monitor history did not cascade")
	}
}
