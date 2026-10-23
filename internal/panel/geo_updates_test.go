package panel

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestIntegrationGeoUpdaterRoutesAccessValidationAndAudit(t *testing.T) {
	s, ctx := integration(t)
	var seen atomic.Int32
	inputs := make(chan Data, 8)
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Add(1)
		if r.Header.Get("Authorization") != "Bearer geo-token" {
			t.Error("agent credential was not applied")
		}
		input := Data{"path": r.URL.Path}
		if r.Method == "POST" {
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Error(err)
			}
		}
		input["path"] = r.URL.Path
		inputs <- input
		writeJSON(w, 200, Data{"managed": true, "enabled": true, "job": Data{"state": "running"}})
	}))
	defer agent.Close()
	token, err := seal(s.Config.FieldKey, "geo-token")
	if err != nil {
		t.Fatal(err)
	}
	node := merge(defaults("nodes"), Data{"name": "geo-updater", "agent_url": agent.URL, "_agent_token": token})
	if err = save(ctx, s.Store.Pool, "nodes", node); err != nil {
		t.Fatal(err)
	}
	admin := importActor(t, s, ctx)
	prefix := fmt.Sprintf("nodes/%d/", num(node, "id"))
	for _, item := range []struct{ action, path string }{
		{"geo-manager", "/geo/status"},
		{"geo-releases?kind=geosite", "/geo/catalog"},
	} {
		if _, code, err := managementRequest(t, s, ctx, admin, "GET", prefix+item.action, nil); err != nil || code != 200 {
			t.Fatal(code, err)
		}
		input := <-inputs
		if str(input, "path") != item.path {
			t.Fatal("incorrect agent endpoint", input)
		}
		if item.path == "/geo/catalog" && str(input, "kind") != "geosite" {
			t.Fatal("missing geo kind", input)
		}
	}
	for _, body := range []Data{{"kind": "geosite", "tag": "20261023064708", "rollback": false}, {"kind": "geosite", "rollback": true}} {
		if _, code, err := managementRequest(t, s, ctx, admin, "POST", prefix+"geo-install", body); err != nil || code != 202 {
			t.Fatal(code, err)
		}
		input := <-inputs
		if str(input, "path") != "/geo/install" || str(input, "kind") != "geosite" || flag(input, "rollback") != flag(body, "rollback") {
			t.Fatal(input)
		}
	}
	var audits int
	if err = s.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM vr_audit WHERE data->>'action'='geo.install.request'`).Scan(&audits); err != nil || audits != 2 {
		t.Fatal("mutation audit missing", audits, err)
	}
	before := seen.Load()
	for _, body := range []Data{
		{"kind": "../../etc", "tag": "20261023"},
		{"kind": "geosite", "tag": "../../x"},
		{"kind": "geoip", "tag": "20261023", "rollback": "true"},
		{"kind": "geoip", "tag": "20261023", "url": "https://attacker.example/data"},
		{"kind": "geoip", "tag": "20261023", "path": "/etc/passwd"},
	} {
		if _, _, err := managementRequest(t, s, ctx, admin, "POST", prefix+"geo-install", body); err == nil {
			t.Fatal("unsafe input accepted", body)
		}
	}
	if _, _, err := managementRequest(t, s, ctx, admin, "GET", prefix+"geo-releases?kind=bad", nil); err == nil {
		t.Fatal("invalid kind accepted")
	}
	if _, _, err := managementRequest(t, s, ctx, admin, "GET", prefix+"geo-install", nil); err == nil {
		t.Fatal("GET queued an update")
	}
	for _, actor := range []Actor{{User: admin.User, Scope: "read"}, {User: Data{"role": "reseller", "is_staff": true}, Scope: "admin"}, {User: admin.User, Scope: "client"}} {
		for _, action := range []string{"geo-manager", "geo-releases?kind=geosite"} {
			if _, _, err := managementRequest(t, s, ctx, actor, "GET", prefix+action, nil); err == nil {
				t.Fatal("unauthorized read accepted", actor.Scope)
			}
		}
		if _, _, err := managementRequest(t, s, ctx, actor, "POST", prefix+"geo-install", Data{"kind": "geosite", "rollback": true}); err == nil {
			t.Fatal("unauthorized update accepted", actor.Scope)
		}
	}
	if seen.Load() != before {
		t.Fatal("rejected request reached the agent")
	}
}
