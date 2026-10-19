package panel

import (
	"fmt"
	"testing"
)

func TestIntegrationBulkCustomersAtomicPolicyAccessAndDelete(t *testing.T) {
	s, ctx := integration(t)
	m1 := &nodeMock{running: true, stats: []any{}}
	m2 := &nodeMock{running: true, stats: []any{}}
	_, i1, _ := fixture(t, s, ctx, m1)
	_, i2, _ := fixture(t, s, ctx, m2)
	ids := []any{}
	credentials := map[int64]string{}
	for n := 0; n < 2; n++ {
		out, _, e := managementRequest(t, s, ctx, Actor{}, "POST", "customers", Data{"name": fmt.Sprintf("bulk%d", n), "inbound_ids": []any{num(i1, "id"), num(i2, "id")}, "traffic_limit_bytes": 1000})
		if e != nil {
			t.Fatal(e)
		}
		d := out.(Data)
		ids = append(ids, num(d, "id"))
		members, _ := customerMembers(ctx, s.Store.Pool, num(d, "id"))
		for _, c := range members {
			credentials[num(c, "id")] = str(c, "credential")
		}
		d["used_traffic_bytes"], d["lifetime_traffic_bytes"] = 300, 900
		if e = save(ctx, s.Store.Pool, "customers", d); e != nil {
			t.Fatal(e)
		}
	}
	request := Data{"ids": ids, "action": "patch", "patch": Data{"traffic_limit_bytes": 2000}}
	m2.failApply = true
	if _, _, e := managementRequest(t, s, ctx, Actor{}, "POST", "customers/bulk", request); e == nil {
		t.Fatal("failed deployment accepted")
	}
	for _, id := range ids {
		d, _ := get(ctx, s.Store.Pool, "customers", num(Data{"id": id}, "id"))
		if num(d, "traffic_limit_bytes") != 1000 {
			t.Fatal("partial policy committed", d)
		}
	}
	m2.failApply = false
	before1, before2 := m1.applies, m2.applies
	if _, _, e := managementRequest(t, s, ctx, Actor{}, "POST", "customers/bulk", request); e != nil {
		t.Fatal(e)
	}
	if m1.applies != before1+1 || m2.applies != before2+1 {
		t.Fatal("nodes deployed more than once")
	}
	for _, id := range ids {
		d, _ := get(ctx, s.Store.Pool, "customers", num(Data{"id": id}, "id"))
		if num(d, "used_traffic_bytes") != 300 || num(d, "traffic_limit_bytes") != 2000 {
			t.Fatal("edit reset usage", d)
		}
		members, _ := customerMembers(ctx, s.Store.Pool, num(d, "id"))
		for _, c := range members {
			if str(c, "credential") != credentials[num(c, "id")] {
				t.Fatal("credential replaced")
			}
		}
	}
	for _, action := range []string{"disable", "enable", "reset"} {
		if _, _, e := managementRequest(t, s, ctx, Actor{}, "POST", "customers/bulk", Data{"ids": ids, "action": action}); e != nil {
			t.Fatal(action, e)
		}
	}
	d, _ := get(ctx, s.Store.Pool, "customers", num(Data{"id": ids[0]}, "id"))
	if num(d, "used_traffic_bytes") != 0 || num(d, "lifetime_traffic_bytes") != 900 {
		t.Fatal("reset affected lifetime", d)
	}
	admin := Actor{User: Data{"is_superuser": true}, Scope: "admin"}
	service, _, e := managementRequest(t, s, ctx, admin, "POST", "services", Data{"name": "South", "inbound_ids": []any{num(i2, "id")}, "defaults": Data{"traffic_limit_bytes": 10}})
	if e != nil {
		t.Fatal(e)
	}
	request["patch"] = Data{"service_ids": []any{num(service.(Data), "id")}, "inbound_ids": []any{}}
	if _, _, e = managementRequest(t, s, ctx, Actor{}, "POST", "customers/bulk", request); e != nil {
		t.Fatal(e)
	}
	for _, id := range ids {
		d, _ := get(ctx, s.Store.Pool, "customers", num(Data{"id": id}, "id"))
		members, _ := customerMembers(ctx, s.Store.Pool, num(d, "id"))
		if len(members) != 1 || num(members[0], "inbound") != num(i2, "id") || num(d, "traffic_limit_bytes") != 2000 {
			t.Fatal("access reapplied defaults", d, members)
		}
	}
	if _, _, e = managementRequest(t, s, ctx, admin, "POST", "customers/bulk", Data{"ids": ids, "action": "delete"}); e != nil {
		t.Fatal(e)
	}
	for _, id := range ids {
		if _, e = get(ctx, s.Store.Pool, "customers", num(Data{"id": id}, "id")); e == nil {
			t.Fatal("account survived deletion")
		}
	}
}

func TestIntegrationBulkCustomersRejectsCrossOwnerAndBudgetOverflow(t *testing.T) {
	s, ctx := integration(t)
	_, i, _ := fixture(t, s, ctx, &nodeMock{running: true, stats: []any{}})
	owner := defaults("users")
	owner["username"], owner["role"], owner["traffic_budget_bytes"], owner["allowed_inbounds"] = "bulk-seller", "reseller", 1000, []any{num(i, "id")}
	if e := save(ctx, s.Store.Pool, "users", owner); e != nil {
		t.Fatal(e)
	}
	seller := Actor{User: owner}
	ids := []any{}
	for n := 0; n < 2; n++ {
		out, _, e := managementRequest(t, s, ctx, seller, "POST", "customers", Data{"name": fmt.Sprintf("buyer%d", n), "inbound_ids": []any{num(i, "id")}, "traffic_limit_bytes": 400})
		if e != nil {
			t.Fatal(e)
		}
		ids = append(ids, num(out.(Data), "id"))
	}
	request := Data{"ids": ids, "action": "patch", "patch": Data{"traffic_limit_bytes": 600}}
	if _, _, e := managementRequest(t, s, ctx, seller, "POST", "customers/bulk", request); e == nil {
		t.Fatal("combined budget bypass")
	}
	for _, id := range ids {
		d, _ := get(ctx, s.Store.Pool, "customers", num(Data{"id": id}, "id"))
		if num(d, "traffic_limit_bytes") != 400 {
			t.Fatal("partial budget change committed")
		}
	}
	outsider := Actor{User: Data{"id": num(owner, "id") + 1, "role": "reseller"}}
	if _, _, e := managementRequest(t, s, ctx, outsider, "POST", "customers/bulk", Data{"ids": ids, "action": "disable"}); e == nil {
		t.Fatal("cross-owner bulk accepted")
	}
	denied := Actor{User: Data{"id": num(owner, "id"), "role": "custom", "own_clients_only": true, "permissions": []any{"clients.write"}}}
	if _, _, e := managementRequest(t, s, ctx, denied, "POST", "customers/bulk", Data{"ids": ids, "action": "delete"}); e == nil {
		t.Fatal("delete permission bypass")
	}
	for _, patch := range []Data{{"used_traffic_bytes": 0}, {"owner_id": 0}, {"credential": "changed"}} {
		if _, _, e := managementRequest(t, s, ctx, seller, "POST", "customers/bulk", Data{"ids": ids, "action": "patch", "patch": patch}); e == nil {
			t.Fatal("managed fields accepted", patch)
		}
	}
}
