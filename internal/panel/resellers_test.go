package panel

import (
	"context"
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func newReseller(t *testing.T, s *Server, ctx context.Context, parent Actor, name string, inbound int64, clients, traffic int64, delegate bool) Actor {
	t.Helper()
	out, _, e := managementRequest(t, s, ctx, parent, "POST", "resellers", Data{"username": name, "allowed_inbounds": []any{inbound}, "max_clients": clients, "traffic_budget_bytes": traffic, "can_delegate": delegate, "max_subresellers": 5})
	if e != nil {
		t.Fatal(e)
	}
	view := out.(Data)
	if str(view, "generated_password") == "" {
		t.Fatal("generated password was not shown once")
	}
	u, e := get(ctx, s.Store.Pool, "users", num(view, "id"))
	if e != nil {
		t.Fatal(e)
	}
	if flag(u, "is_superuser") || str(u, "role") != "reseller" || num(u, "parent_id") != num(parent.User, "id") {
		t.Fatal(u)
	}
	return Actor{User: u, Scope: "admin"}
}
func TestIntegrationResellerHierarchyReservationsAndAccess(t *testing.T) {
	s, ctx, admin, o, _ := commerceFixture(t)
	svc, _ := get(ctx, s.Store.Pool, "services", num(o, "service_id"))
	inbound := num(Data{"id": array(svc, "inbound_ids")[0]}, "id")
	parent := newReseller(t, s, ctx, admin, "parent", inbound, 10, 5000, true)
	child := newReseller(t, s, ctx, parent, "child", inbound, 4, 2000, true)
	grand := newReseller(t, s, ctx, child, "grand", inbound, 1, 500, false)
	if _, _, e := managementRequest(t, s, ctx, parent, "POST", "resellers", Data{"username": "too-large", "allowed_inbounds": []any{inbound}, "max_clients": 7, "traffic_budget_bytes": 4000}); e == nil {
		t.Fatal("overallocated child reservation")
	}
	if _, _, e := managementRequest(t, s, ctx, child, "POST", "resellers", Data{"username": "unlimited-child", "allowed_inbounds": []any{inbound}}); e == nil {
		t.Fatal("unlimited child bypassed finite parent")
	}
	if yes, e := descendant(ctx, s.Store.Pool, num(parent.User, "id"), num(grand.User, "id")); e != nil || !yes {
		t.Fatal(yes, e)
	}
	if _, _, e := managementRequest(t, s, ctx, parent, "GET", fmt.Sprintf("commerce/ledger?owner=%d", num(grand.User, "id")), nil); e != nil {
		t.Fatal("parent cannot inspect descendant ledger", e)
	}
	if _, _, e := managementRequest(t, s, ctx, grand, "GET", fmt.Sprintf("commerce/ledger?owner=%d", num(parent.User, "id")), nil); e == nil {
		t.Fatal("child read parent ledger")
	}
	if _, _, e := managementRequest(t, s, ctx, grand, "POST", "resellers", Data{"username": "forbidden", "allowed_inbounds": []any{inbound}}); e == nil {
		t.Fatal("delegation was granted without permission")
	}
	if _, _, e := managementRequest(t, s, ctx, parent, "POST", "customers", Data{"name": "parent-own", "inbound_ids": []any{inbound}, "traffic_limit_bytes": 3000}); e != nil {
		t.Fatal(e)
	}
	if _, _, e := managementRequest(t, s, ctx, parent, "POST", "customers", Data{"name": "steal-child-reserve", "inbound_ids": []any{inbound}, "traffic_limit_bytes": 1}); e == nil {
		t.Fatal("parent consumed child's reserved traffic")
	}
	if _, _, e := managementRequest(t, s, ctx, admin, "PATCH", fmt.Sprintf("admin/users/%d", num(parent.User, "id")), Data{"max_clients": 3}); e == nil {
		t.Fatal("parent limit reduced below allocated children")
	}
	denied := parent
	denied.Scope = "write"
	if _, _, e := managementRequest(t, s, ctx, denied, "POST", "resellers", Data{"username": "scope-bypass"}); e == nil {
		t.Fatal("write-only API scope created a staff identity")
	}
}
func TestIntegrationResellerInheritedInboundAndInactiveAncestor(t *testing.T) {
	s, ctx, admin, o, _ := commerceFixture(t)
	svc, _ := get(ctx, s.Store.Pool, "services", num(o, "service_id"))
	inbound := num(Data{"id": array(svc, "inbound_ids")[0]}, "id")
	parent := newReseller(t, s, ctx, admin, "limited-parent", inbound, 10, 5000, true)
	child := newReseller(t, s, ctx, parent, "limited-child", inbound, 4, 2000, false)
	_, extra, _ := fixture(t, s, ctx, &nodeMock{running: true})
	if _, _, e := managementRequest(t, s, ctx, parent, "PATCH", fmt.Sprintf("resellers/%d", num(child.User, "id")), Data{"allowed_inbounds": []any{num(extra, "id")}}); e == nil {
		t.Fatal("child gained an unassigned inbound")
	}
	if _, _, e := managementRequest(t, s, ctx, admin, "PATCH", fmt.Sprintf("admin/users/%d", num(parent.User, "id")), Data{"allowed_inbounds": []any{}}); e == nil {
		t.Fatal("parent removed an inbound still assigned to children")
	}
	token := randomToken(32)
	_, e := s.Store.Pool.Exec(ctx, `INSERT INTO vr_sessions(token_hash,user_id,auth_version,expires_at) VALUES($1,$2,$3,$4)`, digest(token), num(child.User, "id"), num(child.User, "_auth_version"), time.Now().Add(time.Hour))
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = managementRequest(t, s, ctx, admin, "PATCH", fmt.Sprintf("admin/users/%d", num(parent.User, "id")), Data{"is_active": false}); e != nil {
		t.Fatal(e)
	}
	r := httptest.NewRequest("GET", "/api/commerce/wallets", nil)
	r.Header.Set("Cookie", "veloray_session="+token)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("inactive ancestor left session enabled", w.Code, w.Body.String())
	}
}
func TestIntegrationResellerCreditTransferAtomicAndReplaySafe(t *testing.T) {
	s, ctx, admin, o, _ := commerceFixture(t)
	svc, _ := get(ctx, s.Store.Pool, "services", num(o, "service_id"))
	inbound := num(Data{"id": array(svc, "inbound_ids")[0]}, "id")
	parent := newReseller(t, s, ctx, admin, "wallet-parent", inbound, 0, 0, true)
	one := newReseller(t, s, ctx, parent, "wallet-one", inbound, 0, 0, false)
	two := newReseller(t, s, ctx, parent, "wallet-two", inbound, 0, 0, false)
	_, _, e := managementRequest(t, s, ctx, admin, "POST", fmt.Sprintf("commerce/adjust?owner=%d", num(parent.User, "id")), Data{"amount": 10000, "currency": "IRT", "reference": randomToken(18), "note": "Parent deposit"})
	if e != nil {
		t.Fatal(e)
	}
	input := Data{"user_id": num(one.User, "id"), "amount": 3000, "currency": "IRT", "reference": randomToken(18), "note": "Child credit"}
	for i := 0; i < 2; i++ {
		if _, _, e = managementRequest(t, s, ctx, parent, "POST", "resellers/transfer", input); e != nil {
			t.Fatal(e)
		}
	}
	var parentBalance, childBalance int64
	s.Store.Pool.QueryRow(ctx, `SELECT balance FROM vr_wallets WHERE user_id=$1 AND currency='IRT'`, num(parent.User, "id")).Scan(&parentBalance)
	s.Store.Pool.QueryRow(ctx, `SELECT balance FROM vr_wallets WHERE user_id=$1 AND currency='IRT'`, num(one.User, "id")).Scan(&childBalance)
	if parentBalance != 7000 || childBalance != 3000 {
		t.Fatal(parentBalance, childBalance)
	}
	// Two concurrent transfers cannot spend the same remaining balance.
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, target := range []Actor{one, two} {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			_, _, err := managementRequest(t, s, ctx, parent, "POST", "resellers/transfer", Data{"user_id": id, "amount": 6000, "currency": "IRT", "reference": randomToken(18)})
			errs <- err
		}(num(target.User, "id"))
	}
	wg.Wait()
	close(errs)
	success := 0
	for err := range errs {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatal("double-spent parent credit", success)
	}
	var total int64
	if e = s.Store.Pool.QueryRow(ctx, `SELECT sum(balance) FROM vr_wallets WHERE currency='IRT'`).Scan(&total); e != nil || total != 10000 {
		t.Fatal("transfer created or lost funds", total, e)
	}
	if _, _, e = managementRequest(t, s, ctx, one, "POST", "resellers/transfer", Data{"user_id": num(two.User, "id"), "amount": 1, "currency": "IRT", "reference": randomToken(18)}); e == nil {
		t.Fatal("sibling transfer forged")
	}
}
