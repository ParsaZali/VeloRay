package panel

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func monitorDefaults() Data {
	return Data{"name": "", "node": 0, "core": "xray", "tag": "direct", "enabled": true, "interval_seconds": 300, "failure_threshold": 3, "failures": 0, "down": false}
}
func (s *Server) monitorView(d Data, detail bool) (Data, error) {
	out := public(d)
	if detail {
		target, e := unseal(s.Config.FieldKey, str(d, "_target"))
		if e != nil {
			return nil, e
		}
		out["url"] = target
	}
	return out, nil
}
func validateMonitor(d Data, target string) error {
	if e := require(d, "name", 96); e != nil {
		return e
	}
	if e := typed(d, monitorDefaults()); e != nil {
		return e
	}
	if !oneOf(str(d, "core"), "xray", "singbox") || !regexp.MustCompile(`^[a-zA-Z0-9._-]{1,96}$`).MatchString(str(d, "tag")) || num(d, "interval_seconds") < 60 || num(d, "interval_seconds") > 86400 || num(d, "failure_threshold") < 1 || num(d, "failure_threshold") > 10 {
		return errors.New("invalid core, tag, interval or failure threshold")
	}
	if target == "" {
		return errors.New("target URL is required")
	}
	return validateWebhookURL(target)
}
func (s *Server) monitorsAPI(w http.ResponseWriter, r *http.Request, path string, a Actor) (any, int, error) {
	ctx := r.Context()
	parts := strings.Split(path, "/")
	if len(parts) > 3 {
		return nil, 0, fail(404, "monitor not found")
	}
	id := int64(0)
	if len(parts) > 1 {
		id, _ = strconv.ParseInt(parts[1], 10, 64)
		if id < 1 {
			return nil, 0, fail(404, "monitor not found")
		}
	}
	if len(parts) == 3 {
		if parts[2] == "probe" && r.Method == "POST" {
			return s.runMonitor(ctx, id, true)
		}
		if parts[2] == "history" && r.Method == "GET" {
			if _, e := get(ctx, s.Store.Pool, "monitors", id); e != nil {
				return nil, 0, e
			}
			rows, e := s.Store.Pool.Query(ctx, "SELECT at,result FROM vr_monitor_samples WHERE monitor_id=$1 ORDER BY at DESC,id DESC LIMIT 100", id)
			if e != nil {
				return nil, 0, e
			}
			defer rows.Close()
			out := []Data{}
			for rows.Next() {
				var at time.Time
				var raw []byte
				if e = rows.Scan(&at, &raw); e != nil {
					return nil, 0, e
				}
				v, _ := decode(raw)
				v["at"] = stamp(at)
				out = append(out, v)
			}
			return out, 200, rows.Err()
		}
		return nil, 0, fail(405, "method not allowed")
	}
	if r.Method == "GET" {
		if id > 0 {
			d, e := get(ctx, s.Store.Pool, "monitors", id)
			if e != nil {
				return nil, 0, e
			}
			v, e := s.monitorView(d, true)
			return v, 200, e
		}
		rows, e := list(ctx, s.Store.Pool, "monitors", "ORDER BY id")
		if e != nil {
			return nil, 0, e
		}
		out := []Data{}
		for _, d := range rows {
			v, e := s.monitorView(d, false)
			if e != nil {
				return nil, 0, e
			}
			out = append(out, v)
		}
		return out, 200, nil
	}
	if !(r.Method == "POST" && id == 0 || oneOf(r.Method, "PATCH", "DELETE") && id > 0) {
		return nil, 0, fail(405, "method not allowed")
	}
	input := Data{}
	var e error
	if r.Method != "DELETE" {
		input, e = readBody(w, r)
		if e != nil {
			return nil, 0, e
		}
		if e = allowed(input, "name node core tag enabled interval_seconds failure_threshold url"); e != nil {
			return nil, 0, fail(400, e.Error())
		}
	}
	out, e := s.managementChange(ctx, nil, a, "monitor."+strings.ToLower(r.Method), fmt.Sprint(id), func(tx pgx.Tx) (any, error) {
		d := monitorDefaults()
		target := ""
		if id > 0 {
			var e error
			d, e = get(ctx, tx, "monitors", id)
			if e != nil {
				return nil, e
			}
			target, e = unseal(s.Config.FieldKey, str(d, "_target"))
			if e != nil {
				return nil, e
			}
		}
		if r.Method == "DELETE" {
			return nil, remove(ctx, tx, "monitors", id)
		}
		prior := clone(d)
		priorTarget := target
		d = merge(d, input)
		if v, ok := input["url"]; ok {
			target, _ = v.(string)
		}
		delete(d, "url")
		if e = validateMonitor(d, target); e != nil {
			return nil, fail(400, e.Error())
		}
		if _, e = get(ctx, tx, "nodes", num(d, "node")); e != nil {
			return nil, fail(400, "node not found")
		}
		if id == 0 {
			var count int
			if e = tx.QueryRow(ctx, "SELECT count(*) FROM vr_monitors").Scan(&count); e != nil {
				return nil, e
			}
			if count >= 100 {
				return nil, fail(400, "at most 100 health monitors")
			}
		}
		sealed, e := seal(s.Config.FieldKey, target)
		if e != nil {
			return nil, e
		}
		d["_target"] = sealed
		u, _ := url.Parse(target)
		d["target_host"] = u.Hostname()
		if str(prior, "_target") == "" || num(prior, "node") != num(d, "node") || str(prior, "core") != str(d, "core") || str(prior, "tag") != str(d, "tag") || priorTarget != target {
			d["failures"], d["down"], d["last_result"] = 0, false, nil
		}
		if e = save(ctx, tx, "monitors", d); e != nil {
			return nil, e
		}
		if _, e = tx.Exec(ctx, "UPDATE vr_monitors SET next_at=now() WHERE id=$1", num(d, "id")); e != nil {
			return nil, e
		}
		return s.monitorView(d, true)
	})
	status := 200
	if r.Method == "POST" {
		status = 201
	}
	if r.Method == "DELETE" {
		status = 204
	}
	return out, status, e
}
func (s *Server) runMonitor(ctx context.Context, id int64, manual bool) (any, int, error) {
	var raw []byte
	query := `UPDATE vr_monitors SET lease_until=now()+interval '30 seconds' WHERE id=(SELECT m.id FROM vr_monitors m JOIN vr_nodes n ON n.id=m.node_id WHERE m.lease_until<now() AND (($1::bigint>0 AND m.id=$1) OR ($1=0 AND m.next_at<=now() AND m.data->>'enabled'='true' AND n.data->>'enabled'='true')) ORDER BY m.next_at,m.id LIMIT 1 FOR UPDATE OF m SKIP LOCKED) RETURNING data||jsonb_build_object('id',id)`
	e := s.Store.Pool.QueryRow(ctx, query, id).Scan(&raw)
	if errors.Is(e, pgx.ErrNoRows) {
		if manual {
			return nil, 0, fail(409, "monitor is busy or no longer exists")
		}
		return nil, 204, nil
	}
	if e != nil {
		return nil, 0, e
	}
	snapshot, e := decode(raw)
	if e != nil {
		return nil, 0, e
	}
	target, e := unseal(s.Config.FieldKey, str(snapshot, "_target"))
	if e != nil {
		return nil, 0, e
	}
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	node, e := get(ctx, s.Store.Pool, "nodes", num(snapshot, "node"))
	var result Data
	if e == nil {
		result, e = s.nodeCall(probeCtx, node, "/outbounds/probe", Data{"core": str(snapshot, "core"), "tag": str(snapshot, "tag"), "url": target})
	}
	if e != nil && strings.Contains(e.Error(), "an outbound probe is already running") {
		_, releaseErr := s.Store.Pool.Exec(ctx, "UPDATE vr_monitors SET lease_until=now(),next_at=now()+interval '10 seconds' WHERE id=$1", num(snapshot, "id"))
		if releaseErr != nil {
			return nil, 0, releaseErr
		}
		if manual {
			return nil, 0, fail(409, "another probe is using this core; try again shortly")
		}
		return nil, 204, nil
	}
	if e != nil {
		result = Data{"ok": false, "error": "node probe unavailable; check connectivity and agent version", "status_code": 0, "latency_ms": 0}
	}
	// Persist only a strict result allowlist; a remote agent cannot echo secrets.
	clean := Data{"ok": flag(result, "ok"), "status_code": num(result, "status_code"), "latency_ms": num(result, "latency_ms"), "checked_at": stamp()}
	if !flag(clean, "ok") {
		clean["error"] = "outbound probe failed"
	}
	if num(clean, "latency_ms") < 0 || num(clean, "latency_ms") > 10000 {
		clean["latency_ms"] = 0
	}
	tx, e := s.Store.Pool.Begin(ctx)
	if e != nil {
		return nil, 0, e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "SELECT id FROM vr_monitors WHERE id=$1 FOR UPDATE", num(snapshot, "id")); e != nil {
		return nil, 0, e
	}
	fresh, e := get(ctx, tx, "monitors", num(snapshot, "id"))
	if e != nil {
		return nil, 0, e
	}
	if str(fresh, "updated_at") != str(snapshot, "updated_at") {
		_, e = tx.Exec(ctx, "UPDATE vr_monitors SET lease_until=now() WHERE id=$1", num(snapshot, "id"))
		if e != nil {
			return nil, 0, e
		}
		if e = tx.Commit(ctx); e != nil {
			return nil, 0, e
		}
		return nil, 0, fail(409, "monitor changed while probing; run it again")
	}
	failures := num(fresh, "failures") + 1
	if flag(clean, "ok") {
		failures = 0
	}
	wasDown := flag(fresh, "down")
	down := wasDown
	if flag(clean, "ok") {
		down = false
	} else if failures >= num(fresh, "failure_threshold") {
		down = true
	}
	if down != wasDown {
		event := "monitor.down"
		if !down {
			event = "monitor.recovered"
		}
		if e = audit(ctx, tx, 0, event, fmt.Sprintf("monitor:%d", num(fresh, "id")), Data{}); e != nil {
			return nil, 0, e
		}
	}
	fresh["failures"], fresh["down"], fresh["last_result"] = failures, down, clean
	if e = save(ctx, tx, "monitors", fresh); e != nil {
		return nil, 0, e
	}
	if _, e = tx.Exec(ctx, "UPDATE vr_monitors SET lease_until=now(),next_at=now()+$2*interval '1 second' WHERE id=$1", num(fresh, "id"), num(fresh, "interval_seconds")); e != nil {
		return nil, 0, e
	}
	if _, e = tx.Exec(ctx, "INSERT INTO vr_monitor_samples(monitor_id,result) VALUES($1,$2::jsonb)", num(fresh, "id"), string(mustJSON(clean))); e != nil {
		return nil, 0, e
	}
	if _, e = tx.Exec(ctx, "DELETE FROM vr_monitor_samples WHERE monitor_id=$1 AND id NOT IN(SELECT id FROM vr_monitor_samples WHERE monitor_id=$1 ORDER BY id DESC LIMIT 500)", num(fresh, "id")); e != nil {
		return nil, 0, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, 0, e
	}
	return clean, 200, nil
}
