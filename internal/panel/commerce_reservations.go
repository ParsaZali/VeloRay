package panel

import (
	"context"
	"errors"
	"math"
)

type fulfillingOrderKey struct{}

func orderReservations(ctx context.Context, q Query, owner int64) (int64, int64, error) {
	exclude, _ := ctx.Value(fulfillingOrderKey{}).(string)
	var seats, traffic int64
	var unlimited bool
	err := q.QueryRow(ctx, `SELECT count(*) FILTER(WHERE kind='purchase'),LEAST(COALESCE(sum(COALESCE((snapshot->>'traffic_bytes')::bigint,0)),0),9223372036854775807)::bigint,COALESCE(bool_or(COALESCE((snapshot->>'traffic_bytes')::bigint,0)=0),false) FROM vr_orders WHERE owner_id=$1 AND id<>$2 AND kind IN ('purchase','renewal','traffic') AND NOT COALESCE((snapshot->>'_sandbox')::boolean,false) AND (state='paid' OR state='pending' AND expires_at>now())`, owner, exclude).Scan(&seats, &traffic, &unlimited)
	if unlimited {
		traffic = -1
	}
	return seats, traffic, err
}

// Unpaid invoices and paid deliveries reserve resources just like child accounts.
func capacityReservations(ctx context.Context, q Query, owner, excludeChild int64) (int64, int64, error) {
	seats, traffic, e := childReservations(ctx, q, owner, excludeChild)
	if e != nil {
		return 0, 0, e
	}
	pendingSeats, pendingTraffic, e := orderReservations(ctx, q, owner)
	if e != nil {
		return 0, 0, e
	}
	if pendingSeats > math.MaxInt64-seats {
		return 0, 0, fail(409, "client reservations exceed supported range")
	}
	if pendingTraffic < 0 {
		traffic = -1
	} else if pendingTraffic > math.MaxInt64-traffic {
		traffic = math.MaxInt64
	} else {
		traffic += pendingTraffic
	}
	return seats + pendingSeats, traffic, nil
}

func (s *Server) validateOrderCapacity(ctx context.Context, q Query, owner Data, snapshot Data, kind string, customerID int64) error {
	ids, e := idArray(snapshot, "inbound_ids", 200)
	if e != nil {
		return e
	}
	if len(ids) == 0 {
		return fail(409, "offer service has no connections")
	}
	c := defaults("clients")
	c["inbound"] = ids[0]
	c["owner_id"] = num(owner, "id")
	c["traffic_limit_bytes"] = num(snapshot, "traffic_bytes")
	if kind != "purchase" {
		if customerID <= 0 {
			return fail(409, "renewal customer was deleted; review this paid order")
		}
		old, e := get(ctx, q, "customers", customerID)
		if e != nil {
			return e
		}
		c["_account"] = customerID
		grant := num(snapshot, "traffic_bytes")
		limit := num(old, "traffic_limit_bytes")
		if limit > 0 && grant > 0 {
			if limit > (1<<62)-grant {
				return fail(400, "traffic grant exceeds supported range")
			}
			c["traffic_limit_bytes"] = limit + grant
		} else if kind == "traffic" && limit == 0 {
			return fail(409, "an unlimited subscription does not need a traffic top-up")
		}
	}
	return validateClientOwner(ctx, q, Actor{User: owner, Scope: "admin"}, c, kind == "purchase")
}
func supportsClientDevices(protocol string) bool {
	return oneOf(protocol, "vless", "vmess", "trojan", "hysteria", "tuic", "naive", "snell", "shadowtls", "mtproto")
}
func offeredPolicy(protocol string, policy Data, grant int64) error {
	if grant > 0 && !metered(protocol) {
		return errors.New("offer traffic quotas require a metered protocol")
	}
	if num(policy, "device_limit") > 0 && !supportsClientDevices(protocol) {
		return errors.New("offer device limit is unsupported by a service protocol")
	}
	if flag(policy, "start_after_first_use") && !metered(protocol) {
		return errors.New("first-use activation requires per-user telemetry")
	}
	return nil
}

// Reject a changed or overbooked grant before starting an external payment.
func (s *Server) recheckOrderCapacity(ctx context.Context, q Query, o Data) error {
	if str(o, "kind") == "topup" {
		return nil
	}
	u, e := seller(ctx, q, num(o, "owner_id"))
	if e != nil {
		return e
	}
	return s.validateOrderCapacity(context.WithValue(ctx, fulfillingOrderKey{}, str(o, "id")), q, u, obj(o, "snapshot"), str(o, "kind"), num(o, "customer_id"))
}
