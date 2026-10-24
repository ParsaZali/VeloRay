package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"
)

func commerceFixture(t *testing.T) (*Server, context.Context, Actor, Data, *nodeMock) {
	t.Helper()
	s, ctx := integration(t)
	s.Config.PublicURL = "https://panel.example"
	m := &nodeMock{running: true, stats: []any{}}
	_, inbound, _ := fixture(t, s, ctx, m)
	u := defaults("users")
	u["username"] = "seller"
	u["is_superuser"] = true
	u["role"] = "administrator"
	if e := save(ctx, s.Store.Pool, "users", u); e != nil {
		t.Fatal(e)
	}
	a := Actor{User: u, Scope: "admin"}
	svc := Data{"name": "Primary", "inbound_ids": []any{num(inbound, "id")}, "defaults": Data{"device_limit": 2}}
	if e := save(ctx, s.Store.Pool, "services", svc); e != nil {
		t.Fatal(e)
	}
	out, _, e := managementRequest(t, s, ctx, a, "POST", "commerce/offers", Data{"name": "Monthly", "enabled": true, "amount": 10000, "currency": "IRT", "service_id": num(svc, "id"), "duration_days": 30, "traffic_bytes": int64(1000), "cost_amount": 2000})
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = managementRequest(t, s, ctx, a, "PATCH", "commerce/store", Data{"enabled": true}); e != nil {
		t.Fatal(e)
	}
	return s, ctx, a, out.(Data), m
}
func creditWallet(t *testing.T, s *Server, ctx context.Context, a Actor, amount int64) {
	t.Helper()
	_, _, e := managementRequest(t, s, ctx, a, "POST", "commerce/adjust", Data{"amount": amount, "currency": "IRT", "reference": randomToken(18), "note": "Initial deposit"})
	if e != nil {
		t.Fatal(e)
	}
}
func TestMoneyRejectsFractionalOverflowAndAmbiguousValues(t *testing.T) {
	for _, v := range []any{json.Number("1.5"), json.Number("1e3"), json.Number("9223372036854775808"), nil, true, "", int64(moneyLimit + 1)} {
		if _, e := money(Data{"amount": v}, "amount", 1); e == nil {
			t.Fatalf("accepted %v", v)
		}
	}
	if n, e := money(Data{"amount": json.Number("10000")}, "amount", 1); e != nil || n != 10000 {
		t.Fatal(n, e)
	}
}
func TestIntegrationCommerceWalletConcurrentIdempotentAndImmutable(t *testing.T) {
	s, ctx, a, _, _ := commerceFixture(t)
	const count = 8
	errs := make(chan error, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, e := s.Store.Pool.Begin(ctx)
			if e != nil {
				errs <- e
				return
			}
			defer tx.Rollback(ctx)
			_, e = walletEntry(ctx, tx, num(a.User, "id"), "IRT", 100, "manual:identical-reference", "Deposit", num(a.User, "id"))
			if e == nil {
				e = tx.Commit(ctx)
			}
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var balance, countEntries int64
	if e := s.Store.Pool.QueryRow(ctx, `SELECT balance FROM vr_wallets WHERE user_id=$1 AND currency='IRT'`, num(a.User, "id")).Scan(&balance); e != nil || balance != 100 {
		t.Fatal(balance, e)
	}
	if e := s.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM vr_ledger`).Scan(&countEntries); e != nil || countEntries != 1 {
		t.Fatal(countEntries, e)
	}
	if _, e := s.Store.Pool.Exec(ctx, `UPDATE vr_ledger SET amount=999`); e == nil {
		t.Fatal("ledger update allowed")
	}
	if _, e := s.Store.Pool.Exec(ctx, `DELETE FROM vr_ledger`); e == nil {
		t.Fatal("ledger delete allowed")
	}
	tx, e := s.Store.Pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	if _, e = walletEntry(ctx, tx, num(a.User, "id"), "IRT", 101, "manual:identical-reference", "Deposit", num(a.User, "id")); e == nil {
		t.Fatal("reference accepted with changed amount")
	}
}
func TestIntegrationCommerceCreditAndTenantIsolation(t *testing.T) {
	s, ctx, a, _, _ := commerceFixture(t)
	u := defaults("users")
	u["username"] = "reseller"
	u["role"] = "reseller"
	if e := save(ctx, s.Store.Pool, "users", u); e != nil {
		t.Fatal(e)
	}
	r := Actor{User: u, Scope: "admin"}
	if _, _, e := managementRequest(t, s, ctx, r, "GET", fmt.Sprintf("commerce/ledger?owner=%d", num(a.User, "id")), nil); e == nil {
		t.Fatal("reseller read another wallet")
	}
	if _, _, e := managementRequest(t, s, ctx, r, "POST", "commerce/adjust", Data{"amount": 100, "currency": "IRT", "note": "forged", "reference": randomToken(16)}); e == nil {
		t.Fatal("reseller forged credit")
	}
	_, _, e := managementRequest(t, s, ctx, a, "POST", "commerce/credit-limit", Data{"currency": "IRT", "credit_limit": 50})
	if e != nil {
		t.Fatal(e)
	}
	tx, e := s.Store.Pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	_, e = walletEntry(ctx, tx, num(a.User, "id"), "IRT", -50, "manual:credit-withdraw", "Debit", num(a.User, "id"))
	if e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if _, _, e = managementRequest(t, s, ctx, a, "POST", "commerce/credit-limit", Data{"currency": "IRT", "credit_limit": 49}); e == nil {
		t.Fatal("credit reduced below existing debt")
	}
	tx, e = s.Store.Pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	if _, e = walletEntry(ctx, tx, num(a.User, "id"), "IRT", -1, "manual:overdraw-reference", "Overdraw", 0); e == nil {
		t.Fatal("credit overdraw allowed")
	}
}
func TestIntegrationCommercePurchaseRenewAndReplayPreserveCredentials(t *testing.T) {
	s, ctx, a, o, _ := commerceFixture(t)
	creditWallet(t, s, ctx, a, 40000)
	input := Data{"offer_id": num(o, "id"), "kind": "purchase", "name": "Alice", "reference": randomToken(18)}
	out, status, e := managementRequest(t, s, ctx, a, "POST", "commerce/purchase", input)
	if e != nil || status != 201 {
		t.Fatal(out, status, e)
	}
	order := out.(Data)
	if str(order, "state") != "fulfilled" {
		t.Fatal(order)
	}
	cid := num(order, "customer_id")
	before, e := get(ctx, s.Store.Pool, "customers", cid)
	if e != nil {
		t.Fatal(e)
	}
	token := str(before, "subscription_token")
	if token == "" || num(before, "device_limit") != 2 {
		t.Fatal(before)
	}
	replay, _, e := managementRequest(t, s, ctx, a, "POST", "commerce/purchase", input)
	if e != nil || str(replay.(Data), "id") != str(order, "id") {
		t.Fatal(replay, e)
	}
	var balance int64
	s.Store.Pool.QueryRow(ctx, `SELECT balance FROM vr_wallets WHERE user_id=$1 AND currency='IRT'`, num(a.User, "id")).Scan(&balance)
	if balance != 30000 {
		t.Fatal("duplicate charge", balance)
	}
	before["used_traffic_bytes"] = int64(400)
	before["raw_used_traffic_bytes"] = int64(400)
	before["lifetime_traffic_bytes"] = int64(400)
	if e = save(ctx, s.Store.Pool, "customers", before); e != nil {
		t.Fatal(e)
	}
	expiry, _ := timestamp(before, "expires_at")
	members, _ := customerMembers(ctx, s.Store.Pool, cid)
	credential := str(members[0], "credential")
	out, _, e = managementRequest(t, s, ctx, a, "POST", "commerce/purchase", Data{"offer_id": num(o, "id"), "kind": "renewal", "customer_token": token, "reference": randomToken(18)})
	if e != nil {
		t.Fatal(e)
	}
	if str(out.(Data), "state") != "fulfilled" {
		t.Fatal(out)
	}
	after, e := get(ctx, s.Store.Pool, "customers", cid)
	if e != nil {
		t.Fatal(e)
	}
	newExpiry, _ := timestamp(after, "expires_at")
	if num(after, "used_traffic_bytes") != 400 || num(after, "traffic_limit_bytes") != 2000 || str(after, "subscription_token") != token || newExpiry.Before(expiry.AddDate(0, 0, 30)) {
		t.Fatal(after)
	}
	members, _ = customerMembers(ctx, s.Store.Pool, cid)
	if str(members[0], "credential") != credential {
		t.Fatal("renewal changed VPN credential")
	}
}
func TestIntegrationCommerceFailedDeliveryRetainsPaymentAndRetries(t *testing.T) {
	s, ctx, a, o, m := commerceFixture(t)
	creditWallet(t, s, ctx, a, 10000)
	m.failApply = true
	out, _, e := managementRequest(t, s, ctx, a, "POST", "commerce/purchase", Data{"offer_id": num(o, "id"), "kind": "purchase", "name": "Retry", "reference": randomToken(18)})
	if e != nil {
		t.Fatal(e)
	}
	order := out.(Data)
	if str(order, "state") != "paid" || str(order, "last_error") == "" {
		t.Fatal(order)
	}
	var n int
	s.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM vr_accounts`).Scan(&n)
	if n != 0 {
		t.Fatal("failed delivery committed a customer", n)
	}
	m.failApply = false
	// The failed deployment journal is compensated before a retry may proceed.
	if e = s.reconcileNodes(ctx); e != nil {
		t.Fatal(e)
	}
	if e = s.fulfillOrder(ctx, str(order, "id")); e != nil {
		t.Fatal(e)
	}
	if e = s.fulfillOrder(ctx, str(order, "id")); e != nil {
		t.Fatal(e)
	}
	s.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM vr_accounts`).Scan(&n)
	if n != 1 {
		t.Fatal("duplicate customer", n)
	}
	var balance int64
	s.Store.Pool.QueryRow(ctx, `SELECT balance FROM vr_wallets WHERE user_id=$1 AND currency='IRT'`, num(a.User, "id")).Scan(&balance)
	if balance != 0 {
		t.Fatal(balance)
	}
}

func TestIntegrationCommerceGatewayIsUnavailable(t *testing.T) {
	s, ctx, a, _, _ := commerceFixture(t)
	out, _, e := managementRequest(t, s, ctx, a, "GET", "commerce/settings", nil)
	if e != nil || str(out.(Data), "gateway") != "soon" {
		t.Fatal(out, e)
	}
	if _, _, e = managementRequest(t, s, ctx, a, "PATCH", "commerce/settings", Data{"payments_enabled": true}); e == nil {
		t.Fatal("retired gateway could be enabled")
	}
	if e = s.verifyPayment(ctx, randomToken(24), "forged-authority"); e == nil {
		t.Fatal("removed gateway accepted payment")
	}
}
func TestIntegrationCommerceSnapshotAndReportExcludeWalletTopups(t *testing.T) {
	s, ctx, a, o, _ := commerceFixture(t)
	creditWallet(t, s, ctx, a, 20000)
	_, _, e := managementRequest(t, s, ctx, a, "POST", "commerce/purchase", Data{"offer_id": num(o, "id"), "kind": "purchase", "name": "Revenue", "reference": randomToken(18)})
	if e != nil {
		t.Fatal(e)
	}
	_, _, e = managementRequest(t, s, ctx, a, "POST", "commerce/costs", Data{"amount": 1000, "currency": "IRT", "note": "Hosting", "reference": randomToken(18)})
	if e != nil {
		t.Fatal(e)
	}
	report, _, e := managementRequest(t, s, ctx, a, "GET", "commerce/reports", nil)
	if e != nil {
		t.Fatal(e)
	}
	summary := report.(Data)["summary"].([]Data)
	if len(summary) != 1 || num(summary[0], "revenue") != 10000 || num(summary[0], "profit") != 7000 {
		t.Fatal(report)
	}
	// Expiry is checked before leaving the panel for the payment provider.
	d, e := s.createOrder(ctx, Data{"offer_id": num(o, "id"), "kind": "purchase", "name": "Expired"}, 0, true)
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Store.Pool.Exec(ctx, `UPDATE vr_orders SET expires_at=$2 WHERE id=$1`, str(d, "id"), time.Now().Add(-time.Hour))
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = s.requestPayment(ctx, str(d, "id")); e == nil {
		t.Fatal("expired checkout allowed")
	}
}
