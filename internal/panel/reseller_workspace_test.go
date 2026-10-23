package panel

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestResellerCannotReadOrChangeInfrastructure(t *testing.T) {
	s := &Server{}
	user := Data{"role": "reseller", "permissions": []any{"inbounds.read", "nodes.write", "routing_rules.read"}}
	actor := Actor{User: user, Scope: "admin"}
	for _, path := range []string{"inbounds", "inbounds/1", "inbounds/1/reset", "nodes", "nodes/1/geo-assets", "outbounds", "routing_rules", "system-info", "outbound-traffic", "xray", "operations/nodes/1/diagnostics", "subscription-policies", "openapi"} {
		for _, method := range []string{"GET", "POST", "PATCH", "DELETE", "HEAD"} {
			if err := s.roleAuthorize(actor, method, path); err == nil {
				t.Errorf("reseller may %s %s", method, path)
			}
		}
	}
	for _, path := range []string{"clients", "clients/inbound-options", "services", "overview", "traffic-history", "settings", "resellers/policy"} {
		if err := s.roleAuthorize(actor, "GET", path); err != nil {
			t.Errorf("reseller cannot read %s: %v", path, err)
		}
	}
	if err := s.roleAuthorize(Actor{User: Data{"role": "custom", "permissions": []any{"clients.read"}}}, "GET", "inbounds"); err == nil {
		t.Fatal("clients.read implicitly granted infrastructure access")
	}
}

func TestIntegrationResellerOptionsAreAssignedAndPublic(t *testing.T) {
	s, ctx := integration(t)
	_, assigned, _ := fixture(t, s, ctx, &nodeMock{running: true})
	_, _, _ = fixture(t, s, ctx, &nodeMock{running: true})
	actor := Actor{User: Data{"role": "reseller", "allowed_inbounds": []any{num(assigned, "id")}}, Scope: "admin"}
	r := httptest.NewRequest("GET", "/api/clients/inbound-options", nil).WithContext(ctx)
	if err := s.authorize(ctx, actor, "GET", "clients/inbound-options"); err != nil {
		t.Fatal(err)
	}
	out, code, err := s.api(httptest.NewRecorder(), r, "clients/inbound-options", actor)
	if err != nil || code != 200 {
		t.Fatal(out, code, err)
	}
	rows := out.([]Data)
	if len(rows) != 1 || num(rows[0], "id") != num(assigned, "id") || !flag(rows[0], "supports_clients") {
		t.Fatal("assigned selector is incorrect", rows)
	}
	for key := range rows[0] {
		if !oneOf(key, "id", "name", "node_name", "protocol", "core", "enabled", "supports_clients", "usage_metered", "supports_batch") {
			t.Fatalf("infrastructure field %s leaked into selector", key)
		}
	}
	actor.User["allowed_inbounds"] = []any{}
	out, _, err = s.api(httptest.NewRecorder(), r, "clients/inbound-options", actor)
	if err != nil || len(out.([]Data)) != 0 {
		t.Fatal("unassigned reseller received connections", out, err)
	}
	actor.Scope = "read"
	if err = s.authorize(ctx, actor, "POST", "clients"); err == nil {
		t.Fatal("read-only reseller API key could create users")
	}
}

func TestIntegrationUnifiedUsersCountOnceAndDeleteAllMembersAtomically(t *testing.T) {
	s, ctx := integration(t)
	m1, m2 := &nodeMock{running: true, stats: []any{}}, &nodeMock{running: true, stats: []any{}}
	_, i1, _ := fixture(t, s, ctx, m1)
	_, i2, _ := fixture(t, s, ctx, m2)
	owner := defaults("users")
	owner["username"], owner["role"], owner["allowed_inbounds"] = "workspace-seller", "reseller", []any{num(i1, "id"), num(i2, "id")}
	owner["max_clients"], owner["traffic_budget_bytes"] = 10, 10000
	if err := save(ctx, s.Store.Pool, "users", owner); err != nil {
		t.Fatal(err)
	}
	a := Actor{User: owner, Scope: "admin"}
	out, _, err := managementRequest(t, s, ctx, a, "POST", "customers", Data{"name": "One buyer", "inbound_ids": []any{num(i1, "id"), num(i2, "id")}, "traffic_limit_bytes": 1000, "expires_at": stamp(time.Now().Add(24 * time.Hour))})
	if err != nil {
		t.Fatal(err)
	}
	account := out.(Data)
	members, err := customerMembers(ctx, s.Store.Pool, num(account, "id"))
	if err != nil || len(members) != 2 {
		t.Fatal(members, err)
	}
	view, err := s.overviewForActor(ctx, a)
	if err != nil || num(view, "clients_total") != 1 || num(view, "clients_active") != 1 || num(view, "clients_expiring_7d") != 1 || num(view, "allocated_traffic_bytes") != 1000 || num(view, "max_clients") != 10 {
		t.Fatal("subscription counted once per connection", view, err)
	}
	ids := []any{num(members[0], "id"), num(members[1], "id")}
	request := Data{"ids": ids, "action": "delete"}
	if _, _, err = managementRequest(t, s, ctx, a, "POST", "clients/bulk", request); err == nil {
		t.Fatal("legacy connection-only delete silently deleted an account")
	}
	foreign := Actor{User: Data{"id": num(owner, "id") + 99, "role": "reseller"}}
	request["whole_accounts"] = true
	if _, _, err = managementRequest(t, s, ctx, foreign, "POST", "clients/bulk", request); err == nil {
		t.Fatal("another seller deleted the subscription")
	}
	m2.failApply = true
	if _, _, err = managementRequest(t, s, ctx, a, "POST", "clients/bulk", request); err == nil {
		t.Fatal("failed deployment committed deletion")
	}
	remaining, err := customerMembers(ctx, s.Store.Pool, num(account, "id"))
	if err != nil || len(remaining) != 2 {
		t.Fatal("partial deletion", remaining, err)
	}
	m2.failApply = false
	out, _, err = managementRequest(t, s, ctx, a, "POST", "clients/bulk", request)
	if err != nil || num(out.(Data), "account_count") != 1 {
		t.Fatal(out, err)
	}
	if _, err = get(ctx, s.Store.Pool, "customers", num(account, "id")); err == nil {
		t.Fatal("subscription remained after deletion")
	}
	for _, member := range members {
		if _, err = get(ctx, s.Store.Pool, "clients", num(member, "id")); err == nil {
			t.Fatalf("connection %d survived whole-account deletion", num(member, "id"))
		}
	}
	// The unrelated legacy clients created by fixture remain intact.
	rows, err := list(ctx, s.Store.Pool, "clients", "ORDER BY id")
	if err != nil || len(rows) != 2 {
		t.Fatal("unrelated clients were removed", rows, err)
	}
}
