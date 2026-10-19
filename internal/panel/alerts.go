package panel

import (
	"context"
	"fmt"
	"time"
)

// One state per principal/event, not per node or connection. Keyset scanning keeps
// maintenance work bounded without starving customers beyond the first page.
func alertConditions(c, cfg Data, now time.Time) map[string]string {
	out := map[string]string{}
	if !flag(c, "enabled") {
		return out
	}
	quota, used, percent := num(c, "traffic_limit_bytes"), num(c, "used_traffic_bytes"), num(cfg, "alert_quota_percent")
	if percent > 0 && quota > 0 && used < quota && float64(used)/float64(quota)*100 >= float64(percent) {
		out["client.quota_warning"] = fmt.Sprintf("%d:%d:%d", quota, percent, num(c, "lifetime_traffic_bytes")-used)
	}
	if exp, e := timestamp(c, "expires_at"); e == nil && exp.After(now) && num(cfg, "alert_expiry_days") > 0 && exp.Sub(now) <= time.Duration(num(cfg, "alert_expiry_days"))*24*time.Hour {
		out["client.expiry_warning"] = str(c, "expires_at")
	}
	if active(c, now) && num(cfg, "alert_inactive_days") > 0 {
		last, e := timestamp(c, "last_traffic_at")
		if e != nil {
			last, e = timestamp(c, "created_at")
		}
		if e == nil && now.Sub(last) >= time.Duration(num(cfg, "alert_inactive_days"))*24*time.Hour {
			out["client.inactive"] = stamp(last)
		}
	}
	return out
}
func (s *Server) scanLifecycleAlerts(ctx context.Context) error {
	s.alertMu.Lock()
	defer s.alertMu.Unlock()
	tx, e := s.Store.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(842100,9)"); e != nil {
		return e
	}
	cfg, e := settings(ctx, tx)
	if e != nil {
		return e
	}
	rows, e := tx.Query(ctx, `SELECT principal,data FROM (SELECT 'account:'||id AS principal,data FROM vr_accounts UNION ALL SELECT 'client:'||id,data FROM vr_clients WHERE account_id IS NULL) p WHERE principal>$1 ORDER BY principal LIMIT 250`, s.alertCursor)
	if e != nil {
		return e
	}
	type item struct {
		key  string
		data []byte
	}
	items := []item{}
	for rows.Next() {
		var v item
		if e = rows.Scan(&v.key, &v.data); e != nil {
			rows.Close()
			return e
		}
		items = append(items, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	cursor := ""
	for _, v := range items {
		cursor = v.key
		c, e := decode(v.data)
		if e != nil {
			return e
		}
		conditions := alertConditions(c, cfg, time.Now().UTC())
		for _, event := range []string{"client.quota_warning", "client.expiry_warning", "client.inactive"} {
			fingerprint, active := conditions[event]
			if !active {
				if _, e = tx.Exec(ctx, "DELETE FROM vr_alert_states WHERE principal=$1 AND event=$2", v.key, event); e != nil {
					return e
				}
				continue
			}
			tag, e := tx.Exec(ctx, `INSERT INTO vr_alert_states(principal,event,fingerprint) VALUES($1,$2,$3) ON CONFLICT(principal,event) DO UPDATE SET fingerprint=EXCLUDED.fingerprint,updated_at=now() WHERE vr_alert_states.fingerprint<>EXCLUDED.fingerprint`, v.key, event, fingerprint)
			if e != nil {
				return e
			}
			if tag.RowsAffected() > 0 {
				if e = audit(ctx, tx, 0, event, v.key, Data{}); e != nil {
					return e
				}
			}
		}
	}
	if len(items) < 250 {
		cursor = ""
	}
	if cursor == "" {
		if _, e = tx.Exec(ctx, `DELETE FROM vr_alert_states a WHERE (a.principal LIKE 'account:%' AND NOT EXISTS(SELECT 1 FROM vr_accounts WHERE id=split_part(a.principal,':',2)::bigint)) OR (a.principal LIKE 'client:%' AND NOT EXISTS(SELECT 1 FROM vr_clients WHERE id=split_part(a.principal,':',2)::bigint AND account_id IS NULL))`); e != nil {
			return e
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return e
	}
	s.alertCursor = cursor
	return nil
}
