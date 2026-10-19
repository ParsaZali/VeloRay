package panel

import (
	"fmt"
	"net/http/httptest"
	"testing"
)

func TestIntegrationOutboundTrafficCoreIsolationIdempotenceAndBilling(t *testing.T) {
	s, ctx := integration(t)
	m := &nodeMock{running: true, stats: []any{}, statMetadata: Data{"outbound_accounting": "core-scoped-v1", "core_ledgers": Data{"singbox": "sing-ledger"}}}
	n, _, c := fixture(t, s, ctx, m)
	userMetric := fmt.Sprintf("user>>>veloray:%d:alice>>>traffic>>>uplink", num(c, "id"))
	m.stats = []any{
		Data{"name": "outbound>>>direct>>>traffic>>>uplink", "value": 100, "core": "xray"},
		Data{"name": "outbound>>>direct>>>traffic>>>downlink", "value": 200, "core": "xray"},
		Data{"name": "outbound>>>direct>>>traffic>>>uplink", "value": 700, "core": "singbox"},
		Data{"name": "outbound>>>direct>>>traffic>>>downlink", "value": 900, "core": "singbox"},
		Data{"name": userMetric, "value": 50, "core": "xray"},
	}
	collect := func() error {
		tx, e := s.Store.Pool.Begin(ctx)
		if e != nil {
			return e
		}
		defer tx.Rollback(ctx)
		node, e := get(ctx, tx, "nodes", num(n, "id"))
		if e != nil {
			return e
		}
		if e = s.collect(ctx, tx, node); e != nil {
			return e
		}
		return tx.Commit(ctx)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if e := collect(); e != nil {
			t.Fatal(e)
		}
	}
	assertTotals := func(core string, upload, download int64) {
		t.Helper()
		var up, down, periodUp, periodDown int64
		e := s.Store.Pool.QueryRow(ctx, `SELECT upload_bytes,download_bytes FROM vr_outbound_traffic_totals WHERE node_id=$1 AND core=$2 AND tag='direct'`, num(n, "id"), core).Scan(&up, &down)
		if e != nil || up != upload || down != download {
			t.Fatal("incorrect outbound totals", core, up, down, e)
		}
		e = s.Store.Pool.QueryRow(ctx, `SELECT sum(upload_bytes)::bigint,sum(download_bytes)::bigint FROM vr_outbound_traffic_windows WHERE node_id=$1 AND core=$2`, num(n, "id"), core).Scan(&periodUp, &periodDown)
		if e != nil || periodUp != upload || periodDown != download {
			t.Fatal("window duplicate/mismatch", core, periodUp, periodDown, e)
		}
	}
	assertTotals("xray", 100, 200)
	assertTotals("singbox", 700, 900)
	fresh, _ := get(ctx, s.Store.Pool, "clients", num(c, "id"))
	if num(fresh, "used_traffic_bytes") != 50 {
		t.Fatal("outbound traffic billed to customer", fresh)
	}
	var billed int64
	if e := s.Store.Pool.QueryRow(ctx, "SELECT sum(bytes)::bigint FROM vr_traffic_windows").Scan(&billed); e != nil || billed != 50 {
		t.Fatal("outbound mixed with node usage", billed, e)
	}
	m.stats[0].(Data)["value"] = 125
	if e := collect(); e != nil {
		t.Fatal(e)
	}
	assertTotals("xray", 125, 200)
	m.stats[0].(Data)["value"] = 99
	if e := collect(); e == nil {
		t.Fatal("decreased durable outbound counter accepted")
	}
	assertTotals("xray", 125, 200)
	m.stats[0].(Data)["value"] = 150
	if e := collect(); e != nil {
		t.Fatal(e)
	}
	assertTotals("xray", 150, 200)
	request := httptest.NewRequest("GET", fmt.Sprintf("/api/outbound-traffic?node=%d&core=singbox&hours=24", num(n, "id")), nil).WithContext(ctx)
	out, e := s.outboundTraffic(request, Actor{})
	if e != nil {
		t.Fatal(e)
	}
	response := out.(Data)
	rows := response["outbounds"].([]Data)
	if !flag(response, "available") || len(rows) != 1 || num(rows[0], "upload_bytes") != 700 {
		t.Fatal(response)
	}
	// Legacy agents cannot distinguish engines. Ignore ambiguous outbound samples,
	// while continuing the established customer cursor without changing its key.
	m.statMetadata = nil
	m.stats = []any{Data{"name": "outbound>>>direct>>>traffic>>>uplink", "value": 99999}, Data{"name": userMetric, "value": 75}}
	if e = collect(); e != nil {
		t.Fatal(e)
	}
	assertTotals("xray", 150, 200)
	fresh, _ = get(ctx, s.Store.Pool, "clients", num(c, "id"))
	if num(fresh, "used_traffic_bytes") != 75 {
		t.Fatal("legacy billing interrupted", fresh)
	}
	out, e = s.outboundTraffic(request, Actor{})
	if e != nil || flag(out.(Data), "available") {
		t.Fatal("legacy capability reported current", out, e)
	}
}

func TestOutboundTrafficAuthorizationAndQueryValidation(t *testing.T) {
	s := &Server{}
	for _, user := range []Data{
		{"role": "reseller"},
		{"role": "viewer", "own_clients_only": true},
		{"role": "custom", "permissions": []any{"traffic.read"}},
		{"role": "custom", "permissions": []any{"nodes.read"}},
	} {
		if _, e := s.outboundTraffic(httptest.NewRequest("GET", "/api/outbound-traffic?node=1&core=xray", nil), Actor{User: user}); e == nil {
			t.Fatal("node-wide telemetry leaked", user)
		}
	}
	for _, query := range []string{"", "node=1&core=other", "node=0&core=xray", "node=1&core=xray&hours=0", "node=1&core=xray&hours=8761", "node=1&core=xray&hours=bad"} {
		if _, e := s.outboundTraffic(httptest.NewRequest("GET", "/api/outbound-traffic?"+query, nil), Actor{}); e == nil {
			t.Fatal("invalid query accepted", query)
		}
	}
}
