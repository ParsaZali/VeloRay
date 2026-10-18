package panel

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestIntegrationProfilesSnippetsRevisionEncryptionAndCompensation(t *testing.T) {
	s, ctx := integration(t)
	m1 := &nodeMock{running: true, stats: []any{}}
	m2 := &nodeMock{running: true, stats: []any{}}
	n1, _, _ := fixture(t, s, ctx, m1)
	n2, _, _ := fixture(t, s, ctx, m2)
	a := Actor{User: Data{"is_superuser": true}, Scope: "admin"}
	request := func(method, path string, body Data) (any, error) {
		out, _, e := managementRequest(t, s, ctx, a, method, path, body)
		return out, e
	}
	secret := "provider-secret-do-not-list"
	out, e := request("POST", "snippets", Data{"name": "provider", "body": Data{"core": "xray", "config": Data{"outbounds": []any{Data{"tag": "provider", "protocol": "socks", "settings": Data{"servers": []any{Data{"address": "example.com", "port": 1080, "users": []any{Data{"user": "demo", "pass": secret}}}}}}}}}})
	if e != nil {
		t.Fatal(e)
	}
	snippet := out.(Data)
	sid := num(snippet, "id")
	out, e = request("POST", "profiles", Data{"name": "shared policy", "body": Data{"xray": Data{"routing": Data{"domainStrategy": "IPIfNonMatch"}}, "singbox": Data{}, "snippet_ids": []any{sid}}})
	if e != nil {
		t.Fatal(e)
	}
	pid := num(out.(Data), "id")
	path := fmt.Sprintf("profiles/%d", pid)
	out, e = request("POST", path+"/assign", Data{"node_ids": []any{num(n1, "id"), num(n2, "id")}})
	if e != nil {
		t.Fatal(out, e)
	}
	if m1.applies != 1 || m2.applies != 1 {
		t.Fatal("profile not applied on both nodes", m1.applies, m2.applies)
	}
	for _, n := range []Data{n1, n2} {
		fresh, _ := get(ctx, s.Store.Pool, "nodes", num(n, "id"))
		cfg, e := s.buildConfig(ctx, s.Store.Pool, fresh)
		if e != nil || str(obj(cfg, "routing"), "domainStrategy") != "IPIfNonMatch" {
			t.Fatal(cfg, e)
		}
		raw, _ := json.Marshal(cfg)
		if !strings.Contains(string(raw), secret) {
			t.Fatal("snippet not used")
		}
	}
	// Neither current storage nor revision snapshots persist provider passwords as plaintext.
	var plain bool
	if e = s.Store.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vr_snippets WHERE data::text LIKE $1 UNION ALL SELECT 1 FROM vr_config_revisions WHERE data::text LIKE $1)`, "%"+secret+"%").Scan(&plain); e != nil || plain {
		t.Fatal("plaintext profile secret", plain, e)
	}
	out, e = request("GET", "snippets", nil)
	raw, _ := json.Marshal(out)
	if e != nil || strings.Contains(string(raw), secret) || strings.Contains(string(raw), "_body") {
		t.Fatal("list leaked profile", out, e)
	}
	m2.failApply = true
	if _, e = request("PATCH", path, Data{"revision": 1, "body": Data{"xray": Data{"routing": Data{"domainStrategy": "AsIs"}}, "singbox": Data{}, "snippet_ids": []any{sid}}}); e == nil {
		t.Fatal("deployment failure committed")
	}
	current, _ := get(ctx, s.Store.Pool, "profiles", pid)
	if num(current, "revision") != 1 || m1.rollbacks == 0 {
		t.Fatal("profile update not compensated", current, m1.rollbacks)
	}
	m2.failApply = false
	if _, e = request("PATCH", path, Data{"revision": 1, "body": Data{"xray": Data{"routing": Data{"domainStrategy": "AsIs"}}, "singbox": Data{}, "snippet_ids": []any{sid}}}); e != nil {
		t.Fatal(e)
	}
	if _, e = request("PATCH", path, Data{"revision": 1, "note": "stale editor"}); e == nil {
		t.Fatal("stale editor overwrote profile")
	}
	if _, e = request("POST", path+"/restore", Data{"revision": 1}); e != nil {
		t.Fatal(e)
	}
	current, _ = get(ctx, s.Store.Pool, "profiles", pid)
	if num(current, "revision") != 3 {
		t.Fatal("restore rewound version", current)
	}
	if _, e = request("DELETE", fmt.Sprintf("snippets/%d", sid), nil); e == nil {
		t.Fatal("referenced snippet deleted")
	}
	if _, e = request("PATCH", path, Data{"body": Data{"xray": Data{"inbounds": []any{Data{"listen": "0.0.0.0", "port": 10085}}}}}); e == nil {
		t.Fatal("profile replaced managed listeners")
	}
	if _, _, e = managementRequest(t, s, ctx, Actor{User: Data{"role": "viewer"}}, "GET", path, nil); e == nil {
		t.Fatal("profile leaked to viewer")
	}
	if e = s.Store.Migrate(ctx); e != nil {
		t.Fatal("migration repeatability", e)
	}
}
