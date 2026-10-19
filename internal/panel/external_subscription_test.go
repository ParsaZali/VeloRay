package panel

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type featureTransport func(*http.Request) (*http.Response, error)

func (f featureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func featureResponse(code int, body string) *http.Response {
	return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}
}

const testExternalURI = "vless://955cc471-391e-4f25-bc37-c8336ed2be5b@edge.example:443?type=ws&security=tls&sni=edge.example&path=%2Fws#External"

func TestExternalFeedNormalizationAndRejection(t *testing.T) {
	for _, uri := range []string{testExternalURI, "trojan://secret@edge.example:443?security=tls#Trojan", "ss://" + base64.RawURLEncoding.EncodeToString([]byte("aes-256-gcm:password")) + "@edge.example:8388#SS", "hy2://password@edge.example:443#HY", "tuic://955cc471-391e-4f25-bc37-c8336ed2be5b:password@edge.example:443#TUIC", "vmess://" + base64.StdEncoding.EncodeToString([]byte(`{"add":"edge.example","port":"443","id":"955cc471-391e-4f25-bc37-c8336ed2be5b","net":"ws","tls":"tls","path":"/ws","ps":"VMess"}`))} {
		c, e := externalConnection(uri)
		if e != nil {
			t.Fatal(uri, e)
		}
		if _, e = singClientOutbound(c.client, c.inbound, c.node); e != nil {
			t.Fatal(e)
		}
	}
	for _, uri := range []string{"file:///etc/passwd", "https://example.com/sub", "ss://broken@edge.example:443", "vless://key@edge.example:70000", "vless://not-a-uuid@edge.example:443?security=tls", "vless://key@edge.example:443?security=reality", "trojan://secret@edge.example:443?allowInsecure=1", "trojan://secret@edge.example:443?type=unsupported"} {
		if _, e := externalConnection(uri); e == nil {
			t.Fatal("bad external link", uri)
		}
	}
	links, e := parseExternalLinks(base64.StdEncoding.EncodeToString([]byte(testExternalURI + "\n" + testExternalURI)))
	if e != nil || len(links) != 1 {
		t.Fatal(links, e)
	}
	if _, e = parseExternalLinks(strings.Repeat("X", (1<<20)+1)); e == nil {
		t.Fatal("oversized feed accepted")
	}
}
func TestIntegrationExternalPolicyAssignmentEncryptionCacheAndHWID(t *testing.T) {
	s, ctx := integration(t)
	_, i, _ := fixture(t, s, ctx, &nodeMock{running: true, stats: []any{}})
	admin := Actor{User: Data{"is_superuser": true}, Scope: "admin"}
	out, _, e := managementRequest(t, s, ctx, admin, "POST", "customers", Data{"name": "policy-buyer", "inbound_ids": []any{num(i, "id")}})
	if e != nil {
		t.Fatal(e)
	}
	customer := out.(Data)
	body := Data{"url": "https://feed.example/sub/private-source-token", "links": []any{testExternalURI}, "templates": Data{"clash": `{"mixed-port":7890}`}, "apps": []any{}, "default_format": "auto"}
	out, _, e = managementRequest(t, s, ctx, admin, "POST", "subscription-policies", Data{"name": "External service", "enabled": true, "customer_ids": []any{num(customer, "id")}, "body": body})
	if e != nil {
		t.Fatal(e)
	}
	policy := out.(Data)
	id := num(policy, "id")
	raw, _ := get(ctx, s.Store.Pool, "subscription_policies", id)
	if strings.Contains(string(mustJSON(raw)), "private-source-token") || strings.Contains(string(mustJSON(raw)), testExternalURI) {
		t.Fatal("plaintext subscription secrets")
	}
	out, _, e = managementRequest(t, s, ctx, admin, "GET", "subscription-policies", nil)
	if e != nil || strings.Contains(string(mustJSON(out)), "private-source-token") {
		t.Fatal(out, e)
	}
	request := func(format string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/sub/"+str(customer, "subscription_token")+"?format="+format, nil).WithContext(ctx)
		s.Handler().ServeHTTP(w, r)
		return w
	}
	w := request("raw")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "edge.example") || !strings.Contains(w.Body.String(), "vpn.example") {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request("stash")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"mixed-port": 7890`) {
		t.Fatal(w.Code, w.Body.String())
	}
	s.ExternalHTTP = &http.Client{Transport: featureTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "feed.example" {
			t.Fatal("wrong feed")
		}
		return featureResponse(200, base64.StdEncoding.EncodeToString([]byte(strings.ReplaceAll(testExternalURI, "edge.example", "cache.example")))), nil
	})}
	_, _, e = managementRequest(t, s, ctx, admin, "POST", fmt.Sprintf("subscription-policies/%d/refresh", id), Data{})
	if e != nil {
		t.Fatal(e)
	}
	s.ExternalHTTP = &http.Client{Transport: featureTransport(func(*http.Request) (*http.Response, error) { return featureResponse(500, "error"), nil })}
	if _, _, e = managementRequest(t, s, ctx, admin, "POST", fmt.Sprintf("subscription-policies/%d/refresh", id), Data{}); e == nil {
		t.Fatal("failed refresh accepted")
	}
	w = request("raw")
	if !strings.Contains(w.Body.String(), "cache.example") {
		t.Fatal("cache lost", w.Body.String())
	}
	_, _, e = managementRequest(t, s, ctx, admin, "PATCH", fmt.Sprintf("subscription-policies/%d", id), Data{"body": merge(body, Data{"apps": []any{"clash"}})})
	if e != nil {
		t.Fatal(e)
	}
	w = request("raw")
	if strings.Contains(w.Body.String(), "edge.example") {
		t.Fatal("app rules ignored")
	}
	// External exports must still be protected by the local account HWID policy.
	cfg, _ := settings(ctx, s.Store.Pool)
	cfg["subscription_hwid_required"] = true
	cfg["subscription_hwid_limit"] = 1
	if e = saveSettings(ctx, s.Store.Pool, cfg); e != nil {
		t.Fatal(e)
	}
	w = request("raw")
	if w.Code != 403 || strings.Contains(w.Body.String(), "vless://") {
		t.Fatal("HWID bypass", w.Code, w.Body.String())
	}
	restricted := Actor{User: Data{"role": "reseller"}, Scope: "admin"}
	if e = s.roleAuthorize(restricted, "GET", "subscription-policies"); e == nil {
		t.Fatal("policy credentials permission bypass")
	}
	s.Config.SubscriptionOnly = true
	for _, path := range []string{"/", "/api/settings", "/api/auth/csrf", "/api/customers", "/api/auth/oidc"} {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 404 {
			t.Fatal("subscription service exposed management", path, w.Code)
		}
	}
}
