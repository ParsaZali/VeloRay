package panel

import (
	"fmt"
	"testing"
	"time"
)

func TestFreezeBoundedDurationAndRenewalClock(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	c := defaults("clients")
	c["expires_at"] = stamp(now.Add(10 * 24 * time.Hour))
	c["next_renewal_at"] = stamp(now.Add(5 * 24 * time.Hour))
	if e := freezeState(c, 7, now); e != nil {
		t.Fatal(e)
	}
	if flag(c, "enabled") || str(c, "disabled_reason") != "frozen" || num(c, "_freeze_remaining_seconds") != 10*86400 {
		t.Fatal(c)
	}
	if e := resumeState(c, now.Add(10*24*time.Hour)); e != nil {
		t.Fatal(e)
	}
	expiry, _ := timestamp(c, "expires_at")
	renewal, _ := timestamp(c, "next_renewal_at")
	if !expiry.Equal(now.Add(17*24*time.Hour)) || !renewal.Equal(now.Add(12*24*time.Hour)) || num(c, "_freeze_used_seconds") != 7*86400 {
		t.Fatal(c)
	}
	if e := freezeState(c, 24, now.Add(10*24*time.Hour)); e == nil {
		t.Fatal("annual freeze allowance exceeded")
	}
}
func TestFreezeEarlyResumeAndInvalidStates(t *testing.T) {
	now := time.Now().UTC()
	c := defaults("clients")
	c["expires_at"] = stamp(now.Add(5 * 24 * time.Hour))
	if e := freezeState(c, 0, now); e == nil {
		t.Fatal("zero freeze allowed")
	}
	if e := freezeState(c, 5, now); e != nil {
		t.Fatal(e)
	}
	if e := freezeState(c, 1, now); e == nil {
		t.Fatal("double freeze allowed")
	}
	if e := resumeState(c, now.Add(2*24*time.Hour)); e != nil {
		t.Fatal(e)
	}
	exp, _ := timestamp(c, "expires_at")
	if exp.Before(now.Add(7*24*time.Hour - time.Second)) {
		t.Fatal(c)
	}
	unlimited := defaults("clients")
	if e := freezeState(unlimited, 1, now); e == nil {
		t.Fatal("unbounded subscription frozen")
	}
}
func TestIntegrationFreezePreservesTokenAndResumesWithOfflineAgent(t *testing.T) {
	s, ctx, a, o, m := commerceFixture(t)
	out, _, e := managementRequest(t, s, ctx, a, "POST", "customers", Data{"name": "Holiday", "service_ids": []any{num(o, "service_id")}, "expires_at": stamp(time.Now().Add(10 * 24 * time.Hour))})
	if e != nil {
		t.Fatal(e)
	}
	d := out.(Data)
	id := num(d, "id")
	token := str(d, "subscription_token")
	out, _, e = managementRequest(t, s, ctx, a, "POST", fmt.Sprintf("customers/%d/freeze", id), Data{"days": 7})
	if e != nil {
		t.Fatal(e)
	}
	if !flag(out.(Data), "frozen") {
		t.Fatal(out)
	}
	if _, _, e = managementRequest(t, s, ctx, a, "PATCH", fmt.Sprintf("customers/%d", id), Data{"enabled": true}); e == nil {
		t.Fatal("generic edit bypassed freeze")
	}
	if _, _, e = managementRequest(t, s, ctx, a, "POST", "customers/bulk", Data{"ids": []any{id}, "action": "enable"}); e == nil {
		t.Fatal("bulk enable bypassed freeze")
	}
	members, _ := customerMembers(ctx, s.Store.Pool, id)
	if _, _, e = managementRequest(t, s, ctx, a, "PATCH", fmt.Sprintf("clients/%d", num(members[0], "id")), Data{"expires_at": stamp(time.Now().Add(99 * 24 * time.Hour))}); e == nil {
		t.Fatal("client edit bypassed saved duration")
	}
	m.failApply = true
	out, _, e = managementRequest(t, s, ctx, a, "POST", fmt.Sprintf("customers/%d/resume", id), Data{})
	if e != nil {
		t.Fatal(e)
	}
	fresh := out.(Data)
	if flag(fresh, "frozen") || str(fresh, "subscription_token") != token || !flag(fresh, "enabled") {
		t.Fatal(fresh)
	}
	var jobs int
	if e = s.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM vr_jobs`).Scan(&jobs); e != nil || jobs != 1 {
		t.Fatal(jobs, e)
	}
}
func TestIntegrationServiceImpactIsReadOnlyAndRejectsStaleMembership(t *testing.T) {
	s, ctx, a, o, m := commerceFixture(t)
	out, _, e := managementRequest(t, s, ctx, a, "POST", "customers", Data{"name": "Impact", "service_ids": []any{num(o, "service_id")}})
	if e != nil {
		t.Fatal(e)
	}
	c := out.(Data)
	m2 := &nodeMock{running: true, stats: []any{}}
	_, i2, _ := fixture(t, s, ctx, m2)
	before := m.applies
	service := num(o, "service_id")
	path := fmt.Sprintf("services/%d", service)
	out, _, e = managementRequest(t, s, ctx, a, "POST", path+"/impact", Data{"inbound_ids": []any{num(i2, "id")}})
	if e != nil {
		t.Fatal(e)
	}
	impact := out.(Data)
	if num(impact, "customers") != 1 || num(impact, "connections_added") != 1 || num(impact, "connections_removed") != 1 || m.applies != before || m2.applies != 0 {
		t.Fatal(impact, m.applies, m2.applies)
	}
	if _, _, e = managementRequest(t, s, ctx, a, "PATCH", fmt.Sprintf("customers/%d", num(c, "id")), Data{"service_ids": []any{}, "inbound_ids": []any{num(i2, "id")}}); e != nil {
		t.Fatal(e)
	}
	if _, _, e = managementRequest(t, s, ctx, a, "PATCH", path, Data{"inbound_ids": []any{num(i2, "id")}, "expected_revision": str(impact, "revision")}); e == nil {
		t.Fatal("stale impact accepted")
	}
	out, _, e = managementRequest(t, s, ctx, a, "POST", path+"/impact", Data{"inbound_ids": []any{num(i2, "id")}})
	if e != nil {
		t.Fatal(e)
	}
	impact = out.(Data)
	if _, _, e = managementRequest(t, s, ctx, a, "PATCH", path, Data{"inbound_ids": []any{num(i2, "id")}, "expected_revision": str(impact, "revision")}); e != nil {
		t.Fatal(e)
	}
}
