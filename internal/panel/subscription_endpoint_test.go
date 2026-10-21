package panel

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestSubscriptionOriginUsesOnlyAppliedPort(t *testing.T) {
	c := Config{PublicURL: "https://panel.example:8443"}
	cfg := Data{"_subscription_endpoint": Data{"port": 9400, "status": "pending"}}
	if got := subscriptionOrigin(c, cfg); got != c.PublicURL {
		t.Fatal(got)
	}
	obj(cfg, "_subscription_endpoint")["applied_port"] = 9444
	if got := subscriptionOrigin(c, cfg); got != "https://panel.example:9444" {
		t.Fatal(got)
	}
	c.PublicURL = "https://[2001:db8::1]:8443"
	if got := subscriptionOrigin(c, cfg); got != "https://[2001:db8::1]:9444" {
		t.Fatal(got)
	}
	for _, port := range []int64{0, -1, 65536, 80, 5432, 8610, 8611, 8612, 9191, 10085, 10086, 10087, 8443} {
		if err := validateSubscriptionPort(Config{PanelPort: 8443}, port); err == nil {
			t.Fatalf("reserved port accepted: %d", port)
		}
	}
}
func TestIntegrationSubscriptionEndpointMovesOnlyAfterSuccessfulApply(t *testing.T) {
	s, ctx := integration(t)
	s.Config.SubscriptionDriver = "caddy"
	s.Config.SubscriptionPort = 8444
	n, _, client := fixture(t, s, ctx, &nodeMock{running: true})
	n["is_local"] = true
	if err := save(ctx, s.Store.Pool, "nodes", n); err != nil {
		t.Fatal(err)
	}
	initial, err := s.subscriptionURL(ctx, s.Store.Pool, str(client, "subscription_token"))
	if err != nil || !strings.HasPrefix(initial, s.Config.PublicURL+"/sub/") {
		t.Fatal(initial, err)
	}
	actor := Actor{User: Data{"is_superuser": true}, Scope: "admin"}
	getStatus := func() Data {
		t.Helper()
		v, status, err := s.subscriptionEndpointAPI(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/subscription-endpoint", nil), actor)
		if err != nil || status != 200 {
			t.Fatal(v, status, err)
		}
		return v.(Data)
	}
	request := func(port int64, revision string) (int, error) {
		t.Helper()
		raw := []byte(`{"port":` + strconv.FormatInt(port, 10) + `,"revision":"` + revision + `"}`)
		_, status, err := s.subscriptionEndpointAPI(httptest.NewRecorder(), httptest.NewRequest("PUT", "/api/subscription-endpoint", bytes.NewReader(raw)), actor)
		return status, err
	}
	if _, _, err := s.subscriptionEndpointAPI(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/subscription-endpoint", nil), Actor{User: Data{"is_superuser": true}, Scope: "subscription"}); err == nil {
		t.Fatal("subscription scope gained admin access")
	}
	if _, err := request(443, ""); err == nil {
		t.Fatal("local inbound port accepted")
	}
	if status, err := request(9444, ""); err != nil || status != 202 {
		t.Fatal(status, err)
	}
	if _, err := request(9555, ""); err == nil {
		t.Fatal("stale revision accepted")
	}
	if url, err := s.subscriptionURL(ctx, s.Store.Pool, str(client, "subscription_token")); err != nil || url != initial {
		t.Fatal("pending request moved URL", url, err)
	}
	failed := func(context.Context, int, int) error { return errors.New("occupied") }
	if err := s.syncSubscriptionEndpoint(ctx, false, failed); err == nil {
		t.Fatal("failed listener accepted")
	}
	if status := getStatus(); str(status, "status") != "error" || num(status, "applied_port") != 0 {
		t.Fatal(status)
	}
	if err := s.syncSubscriptionEndpoint(ctx, true, func(_ context.Context, newPort, oldPort int) error {
		if newPort != 9444 || oldPort != 0 {
			t.Errorf("wrong transition %d <- %d", newPort, oldPort)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	status := getStatus()
	if str(status, "status") != "ready" || num(status, "applied_port") != 9444 {
		t.Fatal(status)
	}
	url, err := s.subscriptionURL(ctx, s.Store.Pool, str(client, "subscription_token"))
	if err != nil || !strings.HasPrefix(url, "http://127.0.0.1:9444/sub/") {
		t.Fatal(url, err)
	}
	redirect := httptest.NewRecorder()
	s.Handler().ServeHTTP(redirect, httptest.NewRequest("GET", "/sub/"+str(client, "subscription_token")+"?format=raw", nil))
	if redirect.Code != http.StatusTemporaryRedirect || redirect.Header().Get("Location") != url+"?format=raw" {
		t.Fatal(redirect.Code, redirect.Header())
	}
	if _, err := request(9555, str(status, "revision")); err != nil {
		t.Fatal(err)
	}
	if err := s.syncSubscriptionEndpoint(ctx, false, failed); err == nil {
		t.Fatal("failed replacement accepted")
	}
	if current, err := s.subscriptionURL(ctx, s.Store.Pool, str(client, "subscription_token")); err != nil || current != url {
		t.Fatal("failed replacement moved live URL", current, err)
	}
	s.Config.SubscriptionOnly = true
	restricted := httptest.NewRecorder()
	s.Handler().ServeHTTP(restricted, httptest.NewRequest("GET", "/api/settings", nil))
	if restricted.Code != 404 {
		t.Fatal("subscription listener exposed management", restricted.Code)
	}
	feed := httptest.NewRecorder()
	s.Handler().ServeHTTP(feed, httptest.NewRequest("GET", "/sub/"+str(client, "subscription_token")+"?format=raw", nil))
	if feed.Code != 200 || !strings.Contains(feed.Body.String(), "vless://") || !strings.HasPrefix(feed.Header().Get("Profile-Web-Page-URL"), "http://127.0.0.1:9444/sub/") {
		t.Fatal(feed.Code, feed.Header(), feed.Body.String())
	}
}
func TestIntegrationSubscriptionEndpointCompensatesDatabaseFailure(t *testing.T) {
	s, ctx := integration(t)
	s.Config.SubscriptionDriver = "caddy"
	s.Config.SubscriptionPort = 9444
	_, err := s.Store.Pool.Exec(ctx, `CREATE FUNCTION vr_test_endpoint_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.data->>'action'='subscription.endpoint.ready' THEN RAISE EXCEPTION 'synthetic audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER vr_test_endpoint_audit BEFORE INSERT ON vr_audit FOR EACH ROW EXECUTE FUNCTION vr_test_endpoint_audit()`)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = s.Store.Pool.Exec(ctx, `DROP TRIGGER IF EXISTS vr_test_endpoint_audit ON vr_audit; DROP FUNCTION IF EXISTS vr_test_endpoint_audit()`)
	})
	current := 0
	calls := 0
	err = s.syncSubscriptionEndpoint(ctx, false, func(_ context.Context, port, previous int) error { calls++; current = port; return nil })
	if err == nil || current != 0 || calls != 2 {
		t.Fatal("database error was not compensated", err, current, calls)
	}
	cfg, err := settings(ctx, s.Store.Pool)
	if err != nil || num(obj(cfg, "_subscription_endpoint"), "applied_port") != 0 {
		t.Fatal(cfg, err)
	}
}
