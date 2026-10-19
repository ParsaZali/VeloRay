package panel

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func billingDefaults() Data {
	return Data{"enabled": true, "provider": "", "region": "", "reference": "", "currency": "USD", "price_minor": 0, "budget_bytes": 0, "warning_percent": 80, "reset_day": 1, "timezone": "UTC"}
}
func validateBilling(d Data) error {
	if e := allowed(d, "enabled provider region reference currency price_minor budget_bytes warning_percent reset_day timezone"); e != nil {
		return e
	}
	if e := typed(d, billingDefaults()); e != nil {
		return e
	}
	for _, k := range []string{"provider", "region", "reference"} {
		if len(str(d, k)) > 160 || strings.ContainsAny(str(d, k), "\r\n\x00") {
			return errors.New("invalid provider metadata")
		}
	}
	if !regexp.MustCompile(`^[A-Z]{3}$`).MatchString(str(d, "currency")) || num(d, "price_minor") < 0 || num(d, "price_minor") > 1000000000000 || num(d, "budget_bytes") < 0 || num(d, "warning_percent") < 1 || num(d, "warning_percent") > 100 || num(d, "reset_day") < 1 || num(d, "reset_day") > 31 {
		return errors.New("invalid billing currency, amount, budget or reset day")
	}
	_, e := time.LoadLocation(str(d, "timezone"))
	return e
}
func billingBounds(now time.Time, day int, zone string) (time.Time, time.Time) {
	loc, _ := time.LoadLocation(zone)
	if loc == nil {
		loc = time.UTC
	}
	local := now.In(loc)
	date := func(year int, month time.Month) time.Time {
		last := time.Date(year, month+1, 0, 0, 0, 0, 0, loc).Day()
		d := day
		if d > last {
			d = last
		}
		return time.Date(year, month, d, 0, 0, 0, 0, loc)
	}
	current := date(local.Year(), local.Month())
	if current.After(local) {
		previous := date(local.Year(), local.Month()-1)
		return previous.UTC(), current.UTC()
	}
	return current.UTC(), date(local.Year(), local.Month()+1).UTC()
}
func (s *Server) billingConfig(node Data) (Data, error) {
	if str(node, "_billing") == "" {
		return billingDefaults(), nil
	}
	raw, e := unseal(s.Config.FieldKey, str(node, "_billing"))
	if e != nil {
		return nil, e
	}
	d, e := decode([]byte(raw))
	return merge(billingDefaults(), d), e
}
func (s *Server) recordNodeBilling(ctx context.Context, q Query, node Data, delta int64, now time.Time) error {
	if str(node, "_billing") == "" {
		return nil
	}
	cfg, e := s.billingConfig(node)
	if e != nil {
		return e
	}
	if !flag(cfg, "enabled") {
		delta = 0
	}
	start, end := billingBounds(now, int(num(cfg, "reset_day")), str(cfg, "timezone"))
	id := num(node, "id")
	if _, e = q.Exec(ctx, `INSERT INTO vr_node_billing(node_id,cycle_start,cycle_end) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, id, now, end); e != nil {
		return e
	}
	var oldStart, oldEnd time.Time
	var used, lifetime int64
	var warned bool
	if e = q.QueryRow(ctx, "SELECT cycle_start,cycle_end,used_bytes,lifetime_bytes,warned FROM vr_node_billing WHERE node_id=$1 FOR UPDATE", id).Scan(&oldStart, &oldEnd, &used, &lifetime, &warned); e != nil {
		return e
	}
	if !oldEnd.After(now) {
		if _, e = q.Exec(ctx, "INSERT INTO vr_node_billing_periods(node_id,cycle_start,cycle_end,used_bytes) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING", id, oldStart, oldEnd, used); e != nil {
			return e
		}
		oldStart, oldEnd, used, warned = start, end, 0, false
	}
	if delta < 0 || delta > int64(^uint64(0)>>1)-used || delta > int64(^uint64(0)>>1)-lifetime {
		return errors.New("node billing overflow")
	}
	used += delta
	lifetime += delta
	above := flag(cfg, "enabled") && num(cfg, "budget_bytes") > 0 && float64(used)/float64(num(cfg, "budget_bytes"))*100 >= float64(num(cfg, "warning_percent"))
	if above && !warned {
		if e = audit(ctx, q, 0, "node.budget_warning", fmt.Sprintf("node:%d", id), Data{}); e != nil {
			return e
		}
	}
	if !above {
		warned = false
	} else {
		warned = true
	}
	_, e = q.Exec(ctx, "UPDATE vr_node_billing SET cycle_start=$2,cycle_end=$3,used_bytes=$4,lifetime_bytes=$5,warned=$6 WHERE node_id=$1", id, oldStart, oldEnd, used, lifetime, warned)
	return e
}
func (s *Server) nodeOperationsAPI(w http.ResponseWriter, r *http.Request, path string, a Actor) (any, int, error) {
	parts := strings.Split(path, "/")
	if len(parts) != 4 || parts[1] != "nodes" {
		return nil, 0, fail(404, "operation not found")
	}
	id, _ := strconv.ParseInt(parts[2], 10, 64)
	if id < 1 {
		return nil, 0, fail(404, "node not found")
	}
	ctx := r.Context()
	node, e := get(ctx, s.Store.Pool, "nodes", id)
	if e != nil {
		return nil, 0, e
	}
	if parts[3] == "diagnostics" {
		if r.Method != "GET" {
			return nil, 0, fail(405, "method not allowed")
		}
		result, e := s.nodeCall(ctx, node, "/diagnostics", nil)
		if e != nil {
			return nil, 0, fail(502, "node diagnostics unavailable; update the node agent")
		}
		return result, 200, nil
	}
	if parts[3] != "billing" {
		return nil, 0, fail(404, "operation not found")
	}
	if r.Method == "GET" {
		cfg, e := s.billingConfig(node)
		if e != nil {
			return nil, 0, e
		}
		usage := Data{}
		var start, end time.Time
		var used, lifetime int64
		e = s.Store.Pool.QueryRow(ctx, "SELECT cycle_start,cycle_end,used_bytes,lifetime_bytes FROM vr_node_billing WHERE node_id=$1", id).Scan(&start, &end, &used, &lifetime)
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return nil, 0, e
		}
		if e == nil {
			usage = Data{"cycle_start": stamp(start), "cycle_end": stamp(end), "used_bytes": used, "lifetime_bytes": lifetime}
		}
		rows, e := s.Store.Pool.Query(ctx, "SELECT cycle_start,cycle_end,used_bytes FROM vr_node_billing_periods WHERE node_id=$1 ORDER BY cycle_start DESC LIMIT 24", id)
		if e != nil {
			return nil, 0, e
		}
		defer rows.Close()
		history := []Data{}
		for rows.Next() {
			var start, end time.Time
			var used int64
			if e = rows.Scan(&start, &end, &used); e != nil {
				return nil, 0, e
			}
			history = append(history, Data{"cycle_start": stamp(start), "cycle_end": stamp(end), "used_bytes": used})
		}
		return Data{"config": cfg, "usage": usage, "history": history, "configured": str(node, "_billing") != ""}, 200, rows.Err()
	}
	if r.Method != "PATCH" {
		return nil, 0, fail(405, "method not allowed")
	}
	input, e := readBody(w, r)
	if e != nil {
		return nil, 0, e
	}
	out, e := s.managementChange(ctx, nil, a, "node.billing.update", fmt.Sprint(id), func(tx pgx.Tx) (any, error) {
		n, e := get(ctx, tx, "nodes", id)
		if e != nil {
			return nil, e
		}
		cfg, e := s.billingConfig(n)
		if e != nil {
			return nil, e
		}
		cfg = merge(cfg, input)
		if e = validateBilling(cfg); e != nil {
			return nil, fail(400, e.Error())
		}
		encrypted, e := seal(s.Config.FieldKey, string(mustJSON(cfg)))
		if e != nil {
			return nil, e
		}
		n["_billing"] = encrypted
		if e = save(ctx, tx, "nodes", n); e != nil {
			return nil, e
		}
		if e = s.recordNodeBilling(ctx, tx, n, 0, time.Now().UTC()); e != nil {
			return nil, e
		}
		return cfg, nil
	})
	return out, 200, e
}

func (s *Server) rollNodeBilling(ctx context.Context) error {
	rows, e := s.Store.Pool.Query(ctx, "SELECT node_id FROM vr_node_billing WHERE cycle_end<=now() ORDER BY cycle_end LIMIT 250")
	if e != nil {
		return e
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, id := range ids {
		tx, e := s.Store.Pool.Begin(ctx)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(842100,2)"); e != nil {
			tx.Rollback(ctx)
			return e
		}
		node, e := get(ctx, tx, "nodes", id)
		if e == nil {
			e = s.recordNodeBilling(ctx, tx, node, 0, time.Now().UTC())
		}
		if e != nil {
			tx.Rollback(ctx)
			return e
		}
		if e = tx.Commit(ctx); e != nil {
			return e
		}
	}
	_, e = s.Store.Pool.Exec(ctx, "DELETE FROM vr_node_billing_periods WHERE cycle_end<now()-interval '2 years'")
	return e
}
