package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func managementRequest(t *testing.T, s *Server, ctx context.Context, a Actor, method, path string, input Data) (any, int, error) {
	t.Helper()
	raw, _ := json.Marshal(input)
	r := httptest.NewRequest(method, "/api/"+path, strings.NewReader(string(raw))).WithContext(ctx)
	return s.api(httptest.NewRecorder(), r, path, a)
}
func TestIntegrationNativeCustomerMultiNodeStableTokenAndQuota(t *testing.T) {
	s, ctx := integration(t)
	m1 := &nodeMock{running: true, stats: []any{}}
	m2 := &nodeMock{running: true, stats: []any{}}
	n1, i1, _ := fixture(t, s, ctx, m1)
	n2, i2, _ := fixture(t, s, ctx, m2)
	out, code, e := managementRequest(t, s, ctx, Actor{}, "POST", "customers", Data{"name": "shared", "inbound_ids": []any{num(i1, "id"), num(i2, "id")}, "traffic_limit_bytes": 1000})
	if e != nil || code != 201 {
		t.Fatal(out, code, e)
	}
	d := out.(Data)
	id := num(d, "id")
	token := str(d, "subscription_token")
	members, e := customerMembers(ctx, s.Store.Pool, id)
	if e != nil || len(members) != 2 || m1.applies != 1 || m2.applies != 1 {
		t.Fatal(members, e, m1.applies, m2.applies)
	}
	c1, c2 := members[0], members[1]
	key2 := str(c2, "credential")
	m1.stats = []any{Data{"name": fmt.Sprintf("user>>>veloray:%d:shared>>>traffic>>>uplink", num(c1, "id")), "value": 300}}
	m2.stats = []any{Data{"name": fmt.Sprintf("user>>>veloray:%d:shared>>>traffic>>>downlink", num(c2, "id")), "value": 450}}
	for _, n := range []Data{n1, n2} {
		tx, e := s.Store.Pool.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		fresh, _ := get(ctx, tx, "nodes", num(n, "id"))
		if e = s.collect(ctx, tx, fresh); e != nil {
			tx.Rollback(ctx)
			t.Fatal(e)
		}
		if e = tx.Commit(ctx); e != nil {
			t.Fatal(e)
		}
	}
	for _, c := range members {
		fresh, e := get(ctx, s.Store.Pool, "clients", num(c, "id"))
		if e != nil || num(fresh, "used_traffic_bytes") != 750 {
			t.Fatal("quota not shared", fresh, e)
		}
	}
	out, _, e = managementRequest(t, s, ctx, Actor{}, "PATCH", fmt.Sprintf("customers/%d", id), Data{"inbound_ids": []any{num(i2, "id")}})
	if e != nil {
		t.Fatal(e)
	}
	d = out.(Data)
	if str(d, "subscription_token") != token || num(d, "used_traffic_bytes") != 750 {
		t.Fatal("membership reset account", d)
	}
	members, _ = customerMembers(ctx, s.Store.Pool, id)
	if len(members) != 1 || str(members[0], "credential") != key2 {
		t.Fatal("credential replaced", members)
	}
	w := httptest.NewRecorder()
	s.subscription(w, httptest.NewRequest("GET", "/sub/"+token+"?format=raw", nil).WithContext(ctx))
	if w.Code != 200 || !strings.Contains(w.Body.String(), key2) {
		t.Fatal("stable account URL failed", w.Code, w.Body.String())
	}
	// A quota action on any connection must deploy every member node.
	_, _, e = managementRequest(t, s, ctx, Actor{}, "PATCH", fmt.Sprintf("customers/%d", id), Data{"inbound_ids": []any{num(i1, "id"), num(i2, "id")}})
	if e != nil {
		t.Fatal(e)
	}
	members, _ = customerMembers(ctx, s.Store.Pool, id)
	before1, before2 := m1.applies, m2.applies
	_, _, e = managementRequest(t, s, ctx, Actor{}, "POST", fmt.Sprintf("clients/%d/toggle", num(members[0], "id")), Data{})
	if e != nil || m1.applies != before1+1 || m2.applies != before2+1 {
		t.Fatal("toggle did not deploy shared policy", e, m1.applies, m2.applies)
	}
}
func TestIntegrationServiceResellerBudgetAndRollback(t *testing.T) {
	s, ctx := integration(t)
	m1 := &nodeMock{running: true, stats: []any{}}
	m2 := &nodeMock{running: true, stats: []any{}}
	_, i1, _ := fixture(t, s, ctx, m1)
	_, i2, _ := fixture(t, s, ctx, m2)
	admin := Actor{User: Data{"is_superuser": true}, Scope: "admin"}
	out, _, e := managementRequest(t, s, ctx, admin, "POST", "services", Data{"name": "basic", "inbound_ids": []any{num(i1, "id")}, "defaults": Data{"traffic_limit_bytes": 500}})
	if e != nil {
		t.Fatal(e)
	}
	serviceID := num(out.(Data), "id")
	owner := defaults("users")
	owner["username"], owner["role"], owner["max_clients"], owner["traffic_budget_bytes"], owner["allowed_inbounds"] = "seller", "reseller", 1, 600, []any{num(i1, "id")}
	if e = save(ctx, s.Store.Pool, "users", owner); e != nil {
		t.Fatal(e)
	}
	seller := Actor{User: owner}
	create := Data{"name": "buyer", "service_ids": []any{serviceID}, "plan_id": serviceID}
	out, _, e = managementRequest(t, s, ctx, seller, "POST", "customers", create)
	if e != nil {
		t.Fatal(e)
	}
	customer := out.(Data)
	if num(customer, "traffic_limit_bytes") != 500 {
		t.Fatal("plan not applied", customer)
	}
	create["name"] = "over-budget"
	if _, _, e = managementRequest(t, s, ctx, seller, "POST", "customers", create); e == nil {
		t.Fatal("account budget bypass")
	}
	outsider := Actor{User: Data{"id": num(owner, "id") + 1, "role": "reseller"}}
	if _, _, e = managementRequest(t, s, ctx, outsider, "GET", fmt.Sprintf("customers/%d", num(customer, "id")), nil); e == nil {
		t.Fatal("customer leaked")
	}
	// Expanding a shared service may never grant the reseller an unassigned inbound.
	_, _, e = managementRequest(t, s, ctx, admin, "PATCH", fmt.Sprintf("services/%d", serviceID), Data{"inbound_ids": []any{num(i1, "id"), num(i2, "id")}})
	if e == nil {
		t.Fatal("service bypassed owner assignment")
	}
	fresh, _ := get(ctx, s.Store.Pool, "services", serviceID)
	if len(array(fresh, "inbound_ids")) != 1 {
		t.Fatal("failed service edit committed", fresh)
	}
	owner["allowed_inbounds"] = []any{num(i1, "id"), num(i2, "id")}
	if e = save(ctx, s.Store.Pool, "users", owner); e != nil {
		t.Fatal(e)
	}
	m2.failApply = true
	if _, _, e = managementRequest(t, s, ctx, admin, "PATCH", fmt.Sprintf("services/%d", serviceID), Data{"inbound_ids": []any{num(i1, "id"), num(i2, "id")}}); e == nil {
		t.Fatal("failed node deployment accepted")
	}
	members, _ := customerMembers(ctx, s.Store.Pool, num(customer, "id"))
	if len(members) != 1 {
		t.Fatal("membership committed despite rollback", members)
	}
	m2.failApply = false
	if _, _, e = managementRequest(t, s, ctx, admin, "PATCH", fmt.Sprintf("services/%d", serviceID), Data{"inbound_ids": []any{num(i1, "id"), num(i2, "id")}}); e != nil {
		t.Fatal(e)
	}
	members, _ = customerMembers(ctx, s.Store.Pool, num(customer, "id"))
	if len(members) != 2 {
		t.Fatal(members)
	}
	// Expiration reconciles once per principal and schedules every other node.
	c := members[0]
	c["expires_at"] = stamp(time.Now().Add(-time.Hour))
	if e = save(ctx, s.Store.Pool, "clients", c); e != nil {
		t.Fatal(e)
	}
	i, _ := get(ctx, s.Store.Pool, "inbounds", num(c, "inbound"))
	if e = s.reconcileNode(ctx, num(i, "node")); e != nil {
		t.Fatal(e)
	}
	for _, member := range members {
		fresh, _ := get(ctx, s.Store.Pool, "clients", num(member, "id"))
		if flag(fresh, "enabled") {
			t.Fatal("shared expiry not persisted", fresh)
		}
	}
	var jobs int
	if e = s.Store.Pool.QueryRow(ctx, "SELECT count(*) FROM vr_jobs").Scan(&jobs); e != nil || jobs != 2 {
		t.Fatal("expiry not queued on both nodes", jobs, e)
	}
}

func TestIntegrationCustomerRotationRevokesEveryAliasAndHistoryIsPrivate(t *testing.T) {
	s, ctx := integration(t)
	_, i, _ := fixture(t, s, ctx, &nodeMock{running: true, stats: []any{}})
	out, _, e := managementRequest(t, s, ctx, Actor{}, "POST", "customers", Data{"name": "rotate", "inbound_ids": []any{num(i, "id")}})
	if e != nil {
		t.Fatal(e)
	}
	customer := out.(Data)
	id := num(customer, "id")
	old := []string{str(customer, "subscription_token")}
	members, _ := customerMembers(ctx, s.Store.Pool, id)
	old = append(old, str(members[0], "subscription_token"))
	request := httptest.NewRequest("GET", "/sub/"+old[0]+"?format=raw", nil).WithContext(ctx)
	request.Header.Set("User-Agent", "secret-raw-agent-clash-token-should-not-persist")
	w := httptest.NewRecorder()
	s.subscription(w, request)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	history, e := s.subscriptionHistory(ctx, members[0])
	raw, _ := json.Marshal(history)
	if e != nil || len(history) != 1 || str(history[0], "client_app") != "clash" || strings.Contains(string(raw), old[0]) || strings.Contains(string(raw), "secret-raw-agent") {
		t.Fatal("history leaked identifying fields", history, e)
	}
	_, _, e = managementRequest(t, s, ctx, Actor{User: Data{"role": "reseller", "id": 42}}, "GET", fmt.Sprintf("clients/%d/subscription-history", num(members[0], "id")), nil)
	if e == nil {
		t.Fatal("cross-owner history leak")
	}
	out, _, e = managementRequest(t, s, ctx, Actor{}, "POST", fmt.Sprintf("customers/%d/rotate-token", id), Data{})
	if e != nil {
		t.Fatal(e)
	}
	next := out.(Data)
	for _, token := range old {
		w = httptest.NewRecorder()
		s.subscription(w, httptest.NewRequest("GET", "/sub/"+token+"?format=raw", nil).WithContext(ctx))
		if w.Code != 404 {
			t.Fatal("old subscription alias still active", w.Code)
		}
	}
	w = httptest.NewRecorder()
	s.subscription(w, httptest.NewRequest("GET", "/sub/"+str(next, "subscription_token")+"?format=raw", nil).WithContext(ctx))
	if w.Code != 200 {
		t.Fatal("rotated URL failed", w.Code, w.Body.String())
	}
}

func TestIntegrationSelectingPlanReanchorsRenewalAndHonorsExplicitNext(t *testing.T) {
	s, ctx := integration(t)
	_, i, _ := fixture(t, s, ctx, &nodeMock{running: true, stats: []any{}})
	admin := Actor{User: Data{"is_superuser": true}, Scope: "admin"}
	out, _, e := managementRequest(t, s, ctx, admin, "POST", "services", Data{"name": "monthly", "inbound_ids": []any{num(i, "id")}, "defaults": Data{"renewal_period": "month", "renewal_every": 1}})
	if e != nil {
		t.Fatal(e)
	}
	service := num(out.(Data), "id")
	explicit := stamp(time.Now().Add(12 * time.Hour))
	out, _, e = managementRequest(t, s, ctx, admin, "POST", "customers", Data{"name": "calendar", "service_ids": []any{service}, "renewal_period": "day", "renewal_every": 1, "next_renewal_at": explicit})
	if e != nil {
		t.Fatal(e)
	}
	customer := out.(Data)
	if str(customer, "next_renewal_at") != explicit {
		t.Fatal("explicit renewal replaced", customer)
	}
	out, _, e = managementRequest(t, s, ctx, admin, "PATCH", fmt.Sprintf("customers/%d", num(customer, "id")), Data{"plan_id": service})
	if e != nil {
		t.Fatal(e)
	}
	customer = out.(Data)
	next, e := timestamp(customer, "next_renewal_at")
	if e != nil || str(customer, "renewal_period") != "month" || time.Until(next) < 20*24*time.Hour {
		t.Fatal("plan did not reanchor calendar", customer, e)
	}
}
