package panel

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

const maxFreezeSeconds int64 = 30 * 24 * 60 * 60

func frozen(c Data) bool { return str(c, "_freeze_until") != "" }
func validateFrozenEdit(c, input Data) error {
	if !frozen(c) {
		return nil
	}
	for k := range input {
		if oneOf(k, "enabled", "expires_at", "duration_days", "start_after_first_use", "renewal_interval_days", "renewal_period", "renewal_every", "renewal_timezone", "next_renewal_at", "plan_id") {
			return fail(409, "resume the frozen subscription before changing its duration or renewal policy")
		}
	}
	return nil
}
func freezeState(d Data, days int64, now time.Time) error {
	if frozen(d) {
		return fail(409, "subscription is already frozen")
	}
	expiry, e := timestamp(d, "expires_at")
	if e != nil || !active(d, now) {
		return fail(409, "only an active subscription with an expiry can be frozen")
	}
	if days < 1 || days > 30 {
		return fail(400, "freeze duration must be 1–30 days")
	}
	reset, e := timestamp(d, "_freeze_budget_reset_at")
	if e != nil || !reset.After(now) {
		d["_freeze_used_seconds"] = int64(0)
		d["_freeze_budget_reset_at"] = stamp(now.AddDate(1, 0, 0))
	}
	if days*86400 > maxFreezeSeconds-num(d, "_freeze_used_seconds") {
		return fail(409, "annual freeze allowance is 30 days")
	}
	d["_freeze_started_at"] = stamp(now)
	d["_freeze_until"] = stamp(now.Add(time.Duration(days) * 24 * time.Hour))
	d["_freeze_remaining_seconds"] = int64(expiry.Sub(now).Seconds())
	d["_freeze_renewal_at"] = d["next_renewal_at"]
	d["expires_at"] = nil
	d["next_renewal_at"] = nil
	d["enabled"] = false
	d["disabled_reason"] = "frozen"
	return nil
}
func resumeState(d Data, now time.Time) error {
	until, e := timestamp(d, "_freeze_until")
	if e != nil {
		return fail(409, "subscription is not frozen")
	}
	started, e := timestamp(d, "_freeze_started_at")
	if e != nil {
		return fail(409, "freeze metadata is invalid")
	}
	anchor := now
	if until.Before(anchor) {
		anchor = until
	}
	elapsed := int64(anchor.Sub(started).Seconds())
	if elapsed < 0 {
		elapsed = 0
	}
	elapsed = min(elapsed, maxFreezeSeconds)
	expiry := anchor.Add(time.Duration(num(d, "_freeze_remaining_seconds")) * time.Second)
	d["expires_at"] = stamp(expiry)
	d["enabled"] = expiry.After(now)
	d["disabled_reason"] = ""
	if !flag(d, "enabled") {
		d["disabled_reason"] = "expired"
	}
	if renewal, e := timestamp(d, "_freeze_renewal_at"); e == nil {
		d["next_renewal_at"] = stamp(renewal.Add(time.Duration(elapsed) * time.Second))
	}
	d["_freeze_used_seconds"] = num(d, "_freeze_used_seconds") + elapsed
	d["_freeze_until"] = ""
	d["_freeze_started_at"] = ""
	d["_freeze_remaining_seconds"] = int64(0)
	d["_freeze_renewal_at"] = nil
	if num(d, "traffic_limit_bytes") > 0 && num(d, "used_traffic_bytes") >= num(d, "traffic_limit_bytes") {
		d["enabled"] = false
		d["disabled_reason"] = "quota"
	}
	return nil
}
func (s *Server) customerFreezeAPI(w http.ResponseWriter, r *http.Request, id int64, action string, a Actor) (any, int, error) {
	if r.Method != "POST" {
		return nil, 0, fail(405, "method not allowed")
	}
	ctx := r.Context()
	d, e := get(ctx, s.Store.Pool, "customers", id)
	if e != nil {
		return nil, 0, e
	}
	if e = customerAccess(a, d); e != nil {
		return nil, 0, e
	}
	input, e := readBody(w, r)
	if e != nil {
		return nil, 0, e
	}
	if e = allowed(input, "days"); e != nil {
		return nil, 0, fail(400, e.Error())
	}
	if action == "freeze" {
		if e = typed(input, Data{"days": 7}); e != nil {
			return nil, 0, fail(400, e.Error())
		}
	}
	members, e := customerMembers(ctx, s.Store.Pool, id)
	if e != nil {
		return nil, 0, e
	}
	nodes, e := clientNodesForMembers(ctx, s.Store.Pool, members)
	if e != nil {
		return nil, 0, e
	}
	mutation := func(tx pgx.Tx) (any, error) {
		fresh, e := get(ctx, tx, "customers", id)
		if e != nil {
			return nil, e
		}
		if e = customerAccess(a, fresh); e != nil {
			return nil, e
		}
		currentMembers, e := customerMembers(ctx, tx, id)
		if e != nil {
			return nil, e
		}
		currentNodes, e := clientNodesForMembers(ctx, tx, currentMembers)
		if e != nil {
			return nil, e
		}
		if action == "freeze" {
			if e = ensureCovered(currentNodes, nodes); e != nil {
				return nil, e
			}
		}
		if action == "freeze" {
			e = freezeState(fresh, num(input, "days"), time.Now().UTC())
		} else {
			e = resumeState(fresh, time.Now().UTC())
		}
		if e != nil {
			return nil, e
		}
		if e = save(ctx, tx, "customers", fresh); e != nil {
			return nil, e
		}
		if action == "resume" {
			for _, node := range currentNodes {
				if e = queueJob(ctx, tx, node, "subscription resumed", nil); e != nil {
					return nil, e
				}
			}
		}
		return s.customerView(ctx, tx, fresh)
	}
	// A freeze is effective only after agents have disabled all connections. A
	// resume persists its bounded deadline even when a node is temporarily offline.
	changeNodes := nodes
	if action == "resume" {
		changeNodes = nil
	}
	out, e := s.managementChange(ctx, changeNodes, a, "customer."+action, fmt.Sprint(id), mutation)
	return out, 200, e
}
func (s *Server) resumeFrozenCustomers(ctx context.Context) error {
	rows, e := list(ctx, s.Store.Pool, "customers", `WHERE COALESCE(data->>'_freeze_until','')<>'' AND (data->>'_freeze_until')::timestamptz<=now() ORDER BY id LIMIT 100`)
	if e != nil {
		return e
	}
	for _, d := range rows {
		_, e = s.managementChange(ctx, nil, Actor{}, "customer.auto_resume", fmt.Sprint(num(d, "id")), func(tx pgx.Tx) (any, error) {
			fresh, e := get(ctx, tx, "customers", num(d, "id"))
			if e != nil {
				return nil, e
			}
			if !frozen(fresh) {
				return nil, nil
			}
			until, e := timestamp(fresh, "_freeze_until")
			if e != nil || until.After(time.Now()) {
				return nil, e
			}
			if e = resumeState(fresh, time.Now().UTC()); e != nil {
				return nil, e
			}
			if e = save(ctx, tx, "customers", fresh); e != nil {
				return nil, e
			}
			members, e := customerMembers(ctx, tx, num(d, "id"))
			if e != nil {
				return nil, e
			}
			nodes, e := clientNodesForMembers(ctx, tx, members)
			if e != nil {
				return nil, e
			}
			for _, node := range nodes {
				if e = queueJob(ctx, tx, node, "subscription auto-resumed", nil); e != nil {
					return nil, e
				}
			}
			return nil, nil
		})
		if e != nil {
			return e
		}
	}
	return nil
}
func addFreezeView(out, d Data) {
	out["frozen"] = frozen(d)
	out["freeze_until"] = d["_freeze_until"]
	out["freeze_remaining_seconds"] = num(d, "_freeze_remaining_seconds")
	out["freeze_used_seconds"] = num(d, "_freeze_used_seconds")
}
