package panel

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestIntegrationNotificationRoutingDeliveryRetryAndSecrecy(t *testing.T) {
	s, ctx := integration(t)
	admin := Actor{User: Data{"is_superuser": true}, Scope: "admin"}
	secret := strings.Repeat("K", 40)
	ids := []int64{}
	for _, item := range []Data{{"name": "Billing", "channel": "webhook", "url": "https://events.example/hook/private-token", "signing_secret": secret, "events": []any{"client.*"}}, {"name": "Operations", "channel": "discord", "url": "https://discord.com/api/webhooks/test/secret-discord-token", "events": []any{"node.*"}}} {
		item["enabled"] = true
		out, _, e := managementRequest(t, s, ctx, admin, "POST", "webhooks/endpoints", item)
		if e != nil {
			t.Fatal(e)
		}
		ids = append(ids, num(out.(Data), "id"))
	}
	if _, _, e := managementRequest(t, s, ctx, admin, "PATCH", fmt.Sprintf("webhooks/endpoints/%d", ids[1]), Data{"channel": "webhook"}); e == nil {
		t.Fatal("Discord key cannot become an unknown generic signing key")
	}
	out, _, e := managementRequest(t, s, ctx, admin, "GET", "webhooks/endpoints", nil)
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(mustJSON(out)), "private-token") || strings.Contains(string(mustJSON(out)), secret) {
		t.Fatal("endpoint secret leaked")
	}
	tx, e := s.Store.Pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if e = audit(ctx, tx, 0, "client.quota", "account:1", Data{}); e != nil {
		t.Fatal(e)
	}
	tx.Rollback(ctx)
	var count int
	if e = s.Store.Pool.QueryRow(ctx, "SELECT count(*) FROM vr_webhook_deliveries").Scan(&count); e != nil || count != 0 {
		t.Fatal("rolled back endpoint event queued", count, e)
	}
	if e = audit(ctx, s.Store.Pool, 0, "client.quota", "account:1", Data{"token": "sensitive-do-not-send"}); e != nil {
		t.Fatal(e)
	}
	if e = audit(ctx, s.Store.Pool, 0, "node.offline", "node:1", Data{}); e != nil {
		t.Fatal(e)
	}
	calls := map[string]int{}
	deliveryIDs := map[string]string{}
	// Fixed test transport never contacts the public endpoints.
	s.WebhookHTTP = &http.Client{Transport: featureTransport(func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		host := r.URL.Host
		calls[host]++
		deliveryIDs[host] = r.Header.Get("X-VeloRay-Delivery")
		if strings.Contains(string(raw), "sensitive-do-not-send") {
			t.Fatal("audit details leaked")
		}
		if host == "events.example" {
			if r.Header.Get("X-VeloRay-Signature") != webhookSignature(secret, r.Header.Get("X-VeloRay-Timestamp"), raw) {
				t.Fatal("bad endpoint HMAC")
			}
			if calls[host] == 1 {
				return featureResponse(503, "retry"), nil
			}
		}
		if host == "discord.com" && !strings.Contains(string(raw), `"allowed_mentions":{"parse":[]}`) {
			t.Fatal("Discord mentions not suppressed", string(raw))
		}
		return featureResponse(204, ""), nil
	})}
	// A disabled legacy integration must not block independent destinations,
	// even if an old encrypted secret can no longer be decrypted.
	cfg, e := settings(ctx, s.Store.Pool)
	if e != nil {
		t.Fatal(e)
	}
	cfg["webhook_enabled"], cfg["_webhook_secret"] = false, "invalid-old-ciphertext"
	if e = saveSettings(ctx, s.Store.Pool, cfg); e != nil {
		t.Fatal(e)
	}
	if e = s.deliverWebhookQueue(ctx); e != nil {
		t.Fatal(e)
	}
	if calls["events.example"] != 1 || calls["discord.com"] != 1 {
		t.Fatal(calls)
	}
	if e = s.deliverWebhookQueue(ctx); e != nil {
		t.Fatal(e)
	}
	if calls["events.example"] != 1 {
		t.Fatal("retry backoff ignored")
	}
	prior := deliveryIDs["events.example"]
	if _, e = s.Store.Pool.Exec(ctx, "UPDATE vr_webhook_deliveries SET next_at=now() WHERE endpoint_id=$1 AND delivered_at IS NULL", ids[0]); e != nil {
		t.Fatal(e)
	}
	if e = s.deliverWebhookQueue(ctx); e != nil {
		t.Fatal(e)
	}
	if calls["events.example"] != 2 || deliveryIDs["events.example"] != prior {
		t.Fatal("retry ID changed", calls)
	}
	_, _, e = managementRequest(t, s, ctx, admin, "PATCH", fmt.Sprintf("webhooks/endpoints/%d", ids[0]), Data{"enabled": false})
	if e != nil {
		t.Fatal(e)
	}
	if e = audit(ctx, s.Store.Pool, 0, "client.expired", "account:1", Data{}); e != nil {
		t.Fatal(e)
	}
	if _, _, e = managementRequest(t, s, ctx, admin, "DELETE", fmt.Sprintf("webhooks/endpoints/%d", ids[0]), nil); e != nil {
		t.Fatal(e)
	}
}
func TestWebhookFiltersAndStreamLimits(t *testing.T) {
	for _, tc := range []struct {
		p, e string
		want bool
	}{{"client.*", "client.quota", true}, {"node.*", "client.quota", false}, {"client.*", "clients.create", false}, {"*", "clients.create", true}, {"auth.login", "auth.login", true}} {
		if matchesWebhookEvent(tc.p, tc.e) != tc.want {
			t.Fatal(tc)
		}
	}
	s := &Server{}
	a := Actor{User: Data{"id": 1}}
	if !s.acquireStream(a) || !s.acquireStream(a) || s.acquireStream(a) {
		t.Fatal("per-user stream cap")
	}
	s.releaseStream(a)
	if !s.acquireStream(a) {
		t.Fatal("stream slot leaked")
	}
	s.releaseStream(a)
	s.releaseStream(a)

}
