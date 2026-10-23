package panel

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func mockStoreOrigin(s *Server, secret string) {
	s.ExternalHTTP = &http.Client{Transport: featureTransport(func(r *http.Request) (*http.Response, error) {
		body, _ := json.Marshal(Data{"status": "ok", "domain_instance": domainInstanceProof(secret, r.URL.Query().Get("domain_probe"))})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{}}, nil
	})}
}

func TestStoreOriginNormalization(t *testing.T) {
	for _, raw := range []string{"http://brand.example", "https://brand.example/path", "https://u:p@brand.example", "https://127.0.0.1:100", "https://brand.example/?x=1", "https://brand.example:99999"} {
		if _, e := normalizeStoreOrigin(raw); e == nil {
			t.Fatal("invalid store origin", raw)
		}
	}
	if value, e := normalizeStoreOrigin("https://Sub.Example:100/"); e != nil || value != "https://sub.example:100" {
		t.Fatal(value, e)
	}
}
func TestIntegrationStoreBrandingVerifiesDNSAndKeepsTenantScope(t *testing.T) {
	s, ctx, a, _, _ := commerceFixture(t)
	customers, e := list(ctx, s.Store.Pool, "clients", "ORDER BY id")
	if e != nil {
		t.Fatal(e)
	}
	c := customers[0]
	c["owner_id"] = num(a.User, "id")
	if e = save(ctx, s.Store.Pool, "clients", c); e != nil {
		t.Fatal(e)
	}
	token := str(c, "subscription_token")
	cfg, _ := settings(ctx, s.Store.Pool)
	cfg["_subscription_endpoint"] = Data{"applied_port": 100}
	if e = saveSettings(ctx, s.Store.Pool, cfg); e != nil {
		t.Fatal(e)
	}
	out, _, e := managementRequest(t, s, ctx, a, "PATCH", "commerce/store", Data{"name": "Green Brand", "subscription_branding": true, "subscription_origin": "https://sub.brand.example:100", "logo_url": "https://images.brand.example/logo.svg", "support_url": "https://support.brand.example"})
	if e != nil {
		t.Fatal(e)
	}
	brand := out.(Data)
	record := obj(brand, "domain_record")
	if str(record, "value") == "" {
		t.Fatal(brand)
	}
	before, e := s.subscriptionURL(ctx, s.Store.Pool, token)
	if e != nil || before != "https://panel.example:100/sub/"+token {
		t.Fatal("unverified domain entered a live subscription link", before, e)
	}
	s.LookupTXT = func(context.Context, string) ([]string, error) { return []string{"wrong-store-proof"}, nil }
	if _, _, e = managementRequest(t, s, ctx, a, "POST", "commerce/store/verify-domain", Data{}); e == nil {
		t.Fatal("unverified DNS accepted")
	}
	s.LookupTXT = func(_ context.Context, name string) ([]string, error) {
		if name != "_veloray.sub.brand.example" {
			t.Fatal(name)
		}
		return []string{str(record, "value")}, nil
	}
	mockStoreOrigin(s, "another-instance-key")
	if _, _, e = managementRequest(t, s, ctx, a, "POST", "commerce/store/verify-domain", Data{}); e == nil {
		t.Fatal("domain targeting another panel was accepted")
	}
	mockStoreOrigin(s, s.Config.SecretKey)
	if _, _, e = managementRequest(t, s, ctx, a, "POST", "commerce/store/verify-domain", Data{"origin": "https://changed.brand.example"}); e == nil {
		t.Fatal("unsaved origin was verified")
	}
	if _, _, e = managementRequest(t, s, ctx, a, "POST", "commerce/store/verify-domain", Data{}); e != nil {
		t.Fatal(e)
	}
	after, e := s.subscriptionURL(ctx, s.Store.Pool, token)
	if e != nil || after != "https://sub.brand.example:100/sub/"+token {
		t.Fatal(after, e)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/sub/"+token+"?format=portal", nil)
	s.subscription(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Green Brand") || !strings.Contains(w.Body.String(), "https://images.brand.example/logo.svg") || w.Header().Get("Support-URL") != "https://support.brand.example" {
		t.Fatal(w.Code, w.Body.String())
	}
	title, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(w.Header().Get("Profile-Title"), "base64:"))
	if !strings.Contains(string(title), "Green Brand") {
		t.Fatal(string(title))
	}
	if !strings.Contains(w.Header().Get("Content-Security-Policy"), "https://images.brand.example") || strings.Contains(w.Header().Get("Content-Security-Policy"), "script-src https:") {
		t.Fatal("incorrect logo CSP")
	}
	// Public storefront metadata never contains its DNS verification proof.
	raw, _ := json.Marshal(brand)
	if strings.Contains(string(raw), "_domain_proof") {
		t.Fatal("private field leaked in store view")
	}
	public, _, e := s.shopAPI(httptest.NewRecorder(), httptest.NewRequest("GET", fmt.Sprintf("/api/shop/catalog?store=%d", num(a.User, "id")), nil), "shop/catalog")
	if e != nil {
		t.Fatal(e)
	}
	raw, _ = json.Marshal(public)
	if strings.Contains(string(raw), str(record, "value")) || strings.Contains(string(raw), "domain_record") {
		t.Fatal("DNS proof leaked to shoppers")
	}
	if _, _, e = managementRequest(t, s, ctx, a, "PATCH", "commerce/store", Data{"subscription_origin": ""}); e != nil {
		t.Fatal(e)
	}
	restored, e := s.subscriptionURL(ctx, s.Store.Pool, token)
	if e != nil || restored != before {
		t.Fatal(restored, e)
	}
}
func TestIntegrationStoreDomainCannotBeClaimedByAnotherSeller(t *testing.T) {
	s, ctx, a, _, _ := commerceFixture(t)
	mockStoreOrigin(s, s.Config.SecretKey)
	first, _, e := managementRequest(t, s, ctx, a, "PATCH", "commerce/store", Data{"subscription_origin": "https://unique.brand.example:100"})
	if e != nil {
		t.Fatal(e)
	}
	proof := str(obj(first.(Data), "domain_record"), "value")
	s.LookupTXT = func(context.Context, string) ([]string, error) { return []string{proof}, nil }
	if _, _, e = managementRequest(t, s, ctx, a, "POST", "commerce/store/verify-domain", Data{}); e != nil {
		t.Fatal(e)
	}
	u := defaults("users")
	u["username"] = "other-owner"
	u["is_superuser"] = true
	if e = save(ctx, s.Store.Pool, "users", u); e != nil {
		t.Fatal(e)
	}
	other := Actor{User: u, Scope: "admin"}
	second, _, e := managementRequest(t, s, ctx, other, "PATCH", "commerce/store", Data{"subscription_origin": "https://unique.brand.example:100"})
	if e != nil {
		t.Fatal(e)
	}
	proof = str(obj(second.(Data), "domain_record"), "value")
	if _, _, e = managementRequest(t, s, ctx, other, "POST", "commerce/store/verify-domain", Data{}); e == nil {
		t.Fatal("duplicate store domain reassigned")
	}
}
