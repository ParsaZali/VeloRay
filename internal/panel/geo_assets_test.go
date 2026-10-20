package panel

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIntegrationGeoAssetReadAccessAndAgentContract(t *testing.T) {
	s, ctx := integration(t)
	seen := 0
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen++
		if r.URL.Path != "/xray/geo-assets" || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("incorrect agent request")
		}
		var input Data
		if json.NewDecoder(r.Body).Decode(&input) != nil || str(input, "kind") != "geosite" || str(input, "query") != "ir" || num(input, "limit") != 10 {
			t.Error("incorrect geo search", input)
		}
		writeJSON(w, 200, Data{"kind": "geosite", "sha256": "test-hash", "total": 2, "matched": 1, "entries": []any{Data{"tag": "ir", "reference": "geosite:ir", "count": 42}}})
	}))
	defer agent.Close()
	token, err := seal(s.Config.FieldKey, "test-token")
	if err != nil {
		t.Fatal(err)
	}
	n := merge(defaults("nodes"), Data{"name": "geo-node", "agent_url": agent.URL, "_agent_token": token})
	if err = save(ctx, s.Store.Pool, "nodes", n); err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("nodes/%d/geo-assets?kind=geosite&q=ir&limit=10", num(n, "id"))
	for _, a := range []Actor{{User: Data{"is_superuser": true}, Scope: "admin"}, {User: Data{"is_superuser": true}, Scope: "read"}} {
		out, code, err := managementRequest(t, s, ctx, a, "GET", path, nil)
		if err != nil || code != 200 || str(out.(Data), "sha256") != "test-hash" {
			t.Fatal(out, code, err)
		}
	}
	if seen != 2 {
		t.Fatal("agent not called", seen)
	}
	for _, a := range []Actor{{User: Data{"role": "reseller"}, Scope: "admin"}, {User: Data{"is_superuser": true}, Scope: "client"}} {
		if _, _, err = managementRequest(t, s, ctx, a, "GET", path, nil); err == nil {
			t.Fatal("unauthorized asset read")
		}
	}
	a := Actor{User: Data{"is_superuser": true}, Scope: "admin"}
	for _, bad := range []string{"?kind=../../secrets", "?kind=geosite&limit=101", "?kind=geosite&q=%0A"} {
		if _, _, err = managementRequest(t, s, ctx, a, "GET", fmt.Sprintf("nodes/%d/geo-assets%s", num(n, "id"), bad), nil); err == nil {
			t.Fatal("accepted invalid query")
		}
	}
	if _, _, err = managementRequest(t, s, ctx, a, "POST", path, Data{}); err == nil {
		t.Fatal("accepted asset mutation")
	}
	if seen != 2 {
		t.Fatal("invalid request reached agent", seen)
	}
}
