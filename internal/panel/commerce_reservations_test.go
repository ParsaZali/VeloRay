package panel

import (
	"fmt"
	"strings"
	"testing"
)

func TestIntegrationOrderCapacityReservedUntilExpiryOrDelivery(t *testing.T) {
	s, ctx, admin, offer, _ := commerceFixture(t)
	svc, _ := get(ctx, s.Store.Pool, "services", num(offer, "service_id"))
	inbound := num(Data{"id": array(svc, "inbound_ids")[0]}, "id")
	parent := newReseller(t, s, ctx, admin, "inventory-seller", inbound, 1, 1000, true)
	owner := num(parent.User, "id")
	out, _, e := managementRequest(t, s, ctx, admin, "POST", fmt.Sprintf("commerce/offers?owner=%d", owner), Data{"name": "Reserved", "enabled": true, "amount": 10000, "currency": "IRT", "service_id": num(svc, "id"), "duration_days": 30, "traffic_bytes": 1000})
	if e != nil {
		t.Fatal(e)
	}
	input := Data{"offer_id": num(out.(Data), "id"), "kind": "purchase", "name": "Buyer"}
	order, e := s.createOrder(ctx, input, 0, false)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.createOrder(ctx, input, 0, false); e == nil {
		t.Fatal("overbooked invoice capacity")
	}
	if _, _, e = managementRequest(t, s, ctx, parent, "POST", "customers", Data{"name": "Manual overbooking", "inbound_ids": []any{inbound}, "traffic_limit_bytes": 1}); e == nil {
		t.Fatal("manual customer consumed reserved inventory")
	}
	if _, _, e = managementRequest(t, s, ctx, parent, "POST", "resellers", Data{"username": "Child overbooking", "allowed_inbounds": []any{inbound}, "max_clients": 1, "traffic_budget_bytes": 1}); e == nil {
		t.Fatal("child allocation consumed invoice reservation")
	}
	if _, e = s.Store.Pool.Exec(ctx, `UPDATE vr_orders SET expires_at=now()-interval '1 second' WHERE id=$1`, str(order, "id")); e != nil {
		t.Fatal(e)
	}
	if _, e = s.createOrder(ctx, input, 0, false); e != nil {
		t.Fatal("expired invoice kept inventory", e)
	}
	// Release the second hold, then purchase the entire capacity using a wallet.
	if _, e = s.Store.Pool.Exec(ctx, `UPDATE vr_orders SET state='cancelled' WHERE owner_id=$1 AND state='pending'`, owner); e != nil {
		t.Fatal(e)
	}
	if _, _, e = managementRequest(t, s, ctx, admin, "POST", fmt.Sprintf("commerce/adjust?owner=%d", owner), Data{"amount": 10000, "currency": "IRT", "reference": randomToken(18), "note": "Deposit"}); e != nil {
		t.Fatal(e)
	}
	input["reference"] = randomToken(18)
	out, _, e = managementRequest(t, s, ctx, parent, "POST", "commerce/purchase", input)
	if e != nil || str(out.(Data), "state") != "fulfilled" {
		t.Fatal("delivery counted its own reservation", out, e)
	}
}

func TestIntegrationFailedWalletDebitReleasesReservation(t *testing.T) {
	s, ctx, a, o, _ := commerceFixture(t)
	_, _, e := managementRequest(t, s, ctx, a, "POST", "commerce/purchase", Data{"offer_id": num(o, "id"), "kind": "purchase", "name": "No credit", "reference": randomToken(18)})
	if e == nil || !strings.Contains(e.Error(), "insufficient wallet credit") {
		t.Fatal(e)
	}
	var holds int
	if e = s.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM vr_orders WHERE state IN ('pending','paid')`).Scan(&holds); e != nil || holds != 0 {
		t.Fatal(holds, e)
	}
}

func TestOfferRejectsUnenforceablePolicies(t *testing.T) {
	if offeredPolicy("shadowsocks", Data{"device_limit": 2}, 1000) == nil {
		t.Fatal("unsupported device policy advertised")
	}
	if offeredPolicy("socks", Data{}, 1000) == nil {
		t.Fatal("unmetered quota advertised")
	}
	if e := offeredPolicy("vless", Data{"device_limit": 2}, 1000); e != nil {
		t.Fatal(e)
	}
}
