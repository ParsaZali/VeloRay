package panel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"sync"
	"time"
)

func (s *Server) probe(ctx context.Context, id int64) (Data, error) {
	tx, e := s.Store.Pool.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	if e = lockNodes(ctx, tx, id); e != nil {
		return nil, e
	}
	n, e := get(ctx, tx, "nodes", id)
	if e != nil {
		return nil, e
	}
	v, e := s.nodeCall(ctx, n, "/health", nil)
	n["status"] = "offline"
	if e == nil {
		n["status"] = "degraded"
		if flag(obj(v, "xray"), "running") {
			n["status"] = "ok"
		}
		n["last_seen_at"] = stamp()
		n["xray_version"] = str(obj(v, "xray"), "version")
	}
	if se := save(ctx, tx, "nodes", n); se != nil {
		return nil, se
	}
	if se := tx.Commit(ctx); se != nil {
		return nil, se
	}
	return v, e
}
func (s *Server) reconcileNode(ctx context.Context, id int64) error {
	tx, e := s.Store.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(842100,2)`); e != nil {
		return e
	}
	if e = lockNodes(ctx, tx, id); e != nil {
		return e
	}
	n, e := get(ctx, tx, "nodes", id)
	if e != nil {
		return e
	}
	if !flag(n, "enabled") {
		return nil
	}
	previousStatus := str(n, "status")
	health, e := s.nodeCall(ctx, n, "/health", nil)
	if e != nil {
		original := e
		n["status"] = "offline"
		if e = auditNodeTransition(ctx, tx, n, previousStatus); e != nil {
			return e
		}
		if e = save(ctx, tx, "nodes", n); e != nil {
			return e
		}
		if e = tx.Commit(ctx); e != nil {
			return e
		}
		return original
	}

	n["status"] = "degraded"
	n["_online_available"] = false
	n["last_seen_at"] = stamp()
	n["xray_version"] = str(obj(health, "xray"), "version")
	if flag(obj(health, "xray"), "running") {
		n["status"] = "ok"
		if _, e = tx.Exec(ctx, `SAVEPOINT accounting`); e != nil {
			return e
		}
		if e = s.collect(ctx, tx, n); e != nil {
			original := e
			if _, e = tx.Exec(ctx, `ROLLBACK TO SAVEPOINT accounting`); e != nil {
				return e
			}
			n["status"] = "degraded"
			n["accounting_error"] = original.Error()
			if e = auditNodeTransition(ctx, tx, n, previousStatus); e != nil {
				return e
			}
			if e = save(ctx, tx, "nodes", n); e != nil {
				return e
			}
			if e = tx.Commit(ctx); e != nil {
				return e
			}
			return original
		}
		n["accounting_error"] = ""
	}
	if e = save(ctx, tx, "nodes", n); e != nil {
		return e
	}
	if e = auditNodeTransition(ctx, tx, n, previousStatus); e != nil {
		return e
	}
	cs, e := list(ctx, tx, "clients", `WHERE inbound_id IN(SELECT id FROM vr_inbounds WHERE node_id=$1)`, id)
	if e != nil {
		return e
	}
	dirty := false
	if flag(n, "_online_tracking_required") {
		var pending bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vr_jobs WHERE node_id=$1)`, id).Scan(&pending); e != nil {
			return e
		}
		dirty = !pending
	}
	seenPrincipals := map[string]bool{}
	for _, c := range cs {
		if seenPrincipals[accountKey(c)] {
			continue
		}
		seenPrincipals[accountKey(c)] = true
		changed := false
		now := time.Now().UTC()
		_, renewEvery, _, renewErr := renewalSpec(c)
		if renewErr != nil {
			return renewErr
		}
		if next, te := timestamp(c, "next_renewal_at"); te == nil && !next.After(now) && renewEvery > 0 {
			upcoming, err := calendarRenewal(c, next, now)
			if err != nil {
				return err
			}
			c["next_renewal_at"] = stamp(upcoming)
			if e = audit(ctx, tx, 0, "client.renewed", accountKey(c), Data{}); e != nil {
				return e
			}
			resetTraffic(c)
			if str(c, "disabled_reason") == "quota" {
				c["enabled"] = true
				c["disabled_reason"] = ""
			}
			changed = true
		}
		if activateFirstUse(c, now, flag(obj(n, "_online_clients"), fmt.Sprint(num(c, "id")))) {
			if e = audit(ctx, tx, 0, "client.first_use", accountKey(c), Data{}); e != nil {
				return e
			}
			changed = true
		}
		if num(c, "device_limit") > 0 || len(obj(c, "_device_denied")) > 0 {
			online := Data{}
			members := []Data{c}
			if account := num(c, "_account"); account > 0 {
				members, e = list(ctx, tx, "clients", "WHERE account_id=$1", account)
				if e != nil {
					return e
				}
			}
			memberNodes := map[int64]bool{}
			for _, member := range members {
				inbound, err := get(ctx, tx, "inbounds", num(member, "inbound"))
				if err != nil {
					return err
				}
				nodeID := num(inbound, "node")
				memberNodes[nodeID] = true
				other := n
				if nodeID != id {
					other, err = get(ctx, tx, "nodes", nodeID)
					if err != nil {
						return err
					}
					if observed, err := timestamp(other, "_telemetry_at"); err != nil || now.Sub(observed) > 90*time.Second {
						continue
					}
				}
				for ip, at := range obj(obj(other, "_online_ips"), fmt.Sprintf("veloray:%d:%s", num(member, "id"), str(member, "name"))) {
					online[ip] = at
				}
			}
			policyChanged := updateDevicePolicy(c, online, now)
			if policyChanged {
				changed = true
				for nodeID := range memberNodes {
					if nodeID != id {
						if e = queueJob(ctx, tx, nodeID, "device policy changed", nil); e != nil {
							return e
						}
					}
				}
			}
			if e = save(ctx, tx, "clients", c); e != nil {
				return e
			}
		}
		reason := ""
		if exp, te := timestamp(c, "expires_at"); te == nil && !exp.After(now) {
			reason = "expired"
		} else if num(c, "traffic_limit_bytes") > 0 && num(c, "used_traffic_bytes") >= num(c, "traffic_limit_bytes") {
			reason = "quota"
		}
		if reason != "" && flag(c, "enabled") {
			c["enabled"] = false
			c["disabled_reason"] = reason
			if e = audit(ctx, tx, 0, "client."+reason, accountKey(c), Data{}); e != nil {
				return e
			}
			changed = true
		}
		if changed {
			related, err := clientNodes(ctx, tx, c)
			if err != nil {
				return err
			}
			for _, otherID := range related {
				if otherID != id {
					if e = queueJob(ctx, tx, otherID, "shared account policy changed", nil); e != nil {
						return e
					}
				}
			}
			if e = save(ctx, tx, "clients", c); e != nil {
				return e
			}
			dirty = true
		}
	}
	if dirty {
		n["deploy_state"] = "pending"
		if e = save(ctx, tx, "nodes", n); e != nil {
			return e
		}
		if e = queueJob(ctx, tx, id, "policy changed", nil); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}
func (s *Server) runJob(ctx context.Context, id int64) error {
	tx, e := s.Store.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if e = lockNodes(ctx, tx, id); e != nil {
		return e
	}
	n, e := get(ctx, tx, "nodes", id)
	if e != nil {
		return e
	}
	var raw []byte
	var attempts int
	e = tx.QueryRow(ctx, `SELECT rollback_ops,attempts FROM vr_jobs WHERE node_id=$1 AND next_at<=now() FOR UPDATE`, id).Scan(&raw, &attempts)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	if !flag(n, "enabled") {
		return nil
	}
	var ops []string
	if e = json.Unmarshal(raw, &ops); e != nil {
		return e
	}
	for len(ops) > 0 {
		if _, e = s.nodeCall(ctx, n, "/xray/rollback", Data{"operation_id": ops[0]}); e != nil {
			break
		}
		ops = ops[1:]
	}
	if e == nil {
		var cfg Data
		cfg, e = s.buildConfig(ctx, tx, n)
		if e == nil {
			_, e = s.nodeCall(ctx, n, "/xray/apply", Data{"config": cfg, "operation_id": randomToken(24)})
		}
	}
	if e != nil {
		n["deploy_state"] = "error"
		if len(ops) > 0 {
			n["deploy_state"] = "rollback_pending"
		}
		n["last_deploy_error"] = e.Error()
		b, _ := json.Marshal(ops)
		_, qe := tx.Exec(ctx, `UPDATE vr_jobs SET attempts=attempts+1,last_error=$1,rollback_ops=$2,next_at=now()+$3*interval '1 second',updated_at=now() WHERE node_id=$4`, e.Error(), string(b), min(300, 5*(1<<min(attempts, 6))), id)
		if qe != nil {
			return qe
		}
	} else {
		if cfg, ce := s.buildConfig(ctx, tx, n); ce == nil && cfg["_singbox"] != nil {
			n["_singbox_deployed"] = true
		}
		n["deploy_state"] = "applied"
		n["last_deploy_error"] = ""
		if _, e = tx.Exec(ctx, `DELETE FROM vr_jobs WHERE node_id=$1`, id); e != nil {
			return e
		}
	}
	if qe := save(ctx, tx, "nodes", n); qe != nil {
		return qe
	}
	if qe := tx.Commit(ctx); qe != nil {
		return qe
	}
	return e
}
func (s *Server) workerLoops(ctx context.Context) {
	var monitors sync.WaitGroup
	for _, poll := range []bool{true, false} {
		monitors.Add(1)
		go func(poll bool) { defer monitors.Done(); s.telegramWorker(ctx, poll) }(poll)
	}
	for worker := 0; worker < 4; worker++ {
		monitors.Add(1)
		go func() {
			defer monitors.Done()
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				if _, _, e := s.runMonitor(ctx, 0, false); e != nil && ctx.Err() == nil {
					s.Logger.Warn("monitor check failed", "error", "check failed")
				}
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
	}
	defer monitors.Wait()

	// Network-dependent housekeeping cannot delay live client telemetry.
	monitors.Add(1)
	go func() {
		defer monitors.Done()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			if e := s.maintain(ctx); e != nil && ctx.Err() == nil {
				s.Logger.Warn("housekeeping failed", "error", e)
			}
			if e := s.rollNodeBilling(ctx); e != nil && ctx.Err() == nil {
				s.Logger.Warn("billing period update failed", "error", e)
			}
			if e := s.fulfillPaidOrders(ctx); e != nil && ctx.Err() == nil {
				s.Logger.Warn("paid order fulfillment pending", "error", "check commerce orders")
			}
			if e := s.scanLifecycleAlerts(ctx); e != nil && ctx.Err() == nil {
				s.Logger.Warn("lifecycle alert scan failed", "error", e)
			}
			if e := s.deliverWebhookQueue(ctx); e != nil && ctx.Err() == nil {
				s.Logger.Warn("webhook queue failed", "error", e)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if e := s.resumeFrozenCustomers(ctx); e != nil && ctx.Err() == nil {
			s.Logger.Warn("subscription resume pending", "error", e)
		}
		if e := s.reconcileNodes(ctx); e != nil && ctx.Err() == nil {
			s.Logger.Warn("reconciliation failed", "error", e)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (s *Server) Reconcile(ctx context.Context) error {
	if err := s.reconcileNodes(ctx); err != nil {
		return err
	}
	return s.maintain(ctx)
}
func (s *Server) reconcileNodes(ctx context.Context) error {
	if e := s.RecoverOperations(ctx); e != nil {
		return e
	}
	nodes, e := list(ctx, s.Store.Pool, "nodes", "")
	if e != nil {
		return e
	}
	for _, n := range nodes {
		if !flag(n, "enabled") {
			continue
		}
		if e = s.reconcileNode(ctx, num(n, "id")); e != nil && ctx.Err() == nil {
			s.Logger.Warn("node accounting unavailable", "node", num(n, "id"), "error", e)
		}
		if e = s.runJob(ctx, num(n, "id")); e != nil && ctx.Err() == nil {
			s.Logger.Warn("node deployment pending", "node", num(n, "id"), "error", e)
		}
	}
	return nil
}
func (s *Server) maintain(ctx context.Context) error {
	cfg, e := settings(ctx, s.Store.Pool)
	if e != nil {
		return e
	}
	_, e = s.Store.Pool.Exec(ctx, `DELETE FROM vr_sessions WHERE expires_at<now(); DELETE FROM vr_rate_limits WHERE start_at<now()-interval '1 day'; DELETE FROM vr_telegram_updates WHERE processed_at<now()-interval '7 days'; DELETE FROM vr_telegram_actions WHERE expires_at<now(); DELETE FROM vr_import_jobs WHERE expires_at<now()`)
	if e != nil {
		return e
	}
	_, e = s.Store.Pool.Exec(ctx, `DELETE FROM vr_audit WHERE created_at<now()-$1*interval '1 day'`, num(cfg, "audit_retention_days"))
	if e != nil {
		return e
	}
	_, e = s.Store.Pool.Exec(ctx, `DELETE FROM vr_traffic_windows WHERE at<now()-$1*interval '1 day'`, num(cfg, "traffic_history_days"))
	if e != nil {
		return e
	}
	_, e = s.Store.Pool.Exec(ctx, `DELETE FROM vr_owner_traffic_windows WHERE at<now()-$1*interval '1 day'`, num(cfg, "traffic_history_days"))
	if e != nil {
		return e
	}
	_, e = s.Store.Pool.Exec(ctx, `DELETE FROM vr_client_traffic_windows WHERE at<now()-$1*interval '1 day'`, num(cfg, "traffic_history_days"))
	if e != nil {
		return e
	}
	if _, e = s.Store.Pool.Exec(ctx, `DELETE FROM vr_subscription_requests WHERE at<now()-$1*interval '1 day'`, num(cfg, "traffic_history_days")); e != nil {
		return e
	}
	if _, e = s.Store.Pool.Exec(ctx, `DELETE FROM vr_outbound_traffic_windows WHERE at<now()-$1*interval '1 day'`, num(cfg, "traffic_history_days")); e != nil {
		return e
	}
	if _, e = s.Store.Pool.Exec(ctx, "DELETE FROM vr_oidc_states WHERE expires_at<now()"); e != nil {
		return e
	}
	if _, e = s.Store.Pool.Exec(ctx, "DELETE FROM vr_oidc_challenges WHERE expires_at<now()"); e != nil {
		return e
	}
	_, e = s.Store.Pool.Exec(ctx, `DELETE FROM vr_telegram_inbox WHERE processed_at<now()-interval '7 days'`)
	if e != nil {
		return e
	}
	return nil
}

// RecoverOperations compensates uncommitted API changes after a panel process crash.
func (s *Server) RecoverOperations(ctx context.Context) error {
	rows, e := s.Store.Pool.Query(ctx, `SELECT id FROM vr_operations WHERE status='prepared' AND created_at<now()-interval '2 minutes' ORDER BY created_at LIMIT 10`)
	if e != nil {
		return e
	}
	ids := []string{}
	for rows.Next() {
		var id string
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
	for _, op := range ids {
		if e = s.recoverOperation(ctx, op); e != nil {
			return e
		}
	}
	_, e = s.Store.Pool.Exec(ctx, `DELETE FROM vr_operations WHERE status<>'prepared' AND created_at<now()-interval '7 days'`)
	return e
}
func (s *Server) recoverOperation(ctx context.Context, op string) error {
	tx, e := s.Store.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(842100,2)`); e != nil {
		return e
	}
	var raw []byte
	var status string
	e = tx.QueryRow(ctx, `SELECT nodes,status FROM vr_operations WHERE id=$1 FOR UPDATE`, op).Scan(&raw, &status)
	if e != nil {
		return e
	}
	if status != "prepared" {
		return nil
	}
	var ids []int64
	if e = json.Unmarshal(raw, &ids); e != nil {
		return e
	}
	if e = lockNodes(ctx, tx, ids...); e != nil {
		return e
	}
	nodes := []Data{}
	committed := true
	for _, id := range ids {
		n, e := get(ctx, tx, "nodes", id)
		if errors.Is(e, pgx.ErrNoRows) {
			continue
		}
		if e != nil {
			return e
		}
		nodes = append(nodes, n)
		if str(n, "_last_operation") != op {
			committed = false
		}
	}
	status = "committed"
	if !committed {
		status = "recovered"
		for j := len(nodes) - 1; j >= 0; j-- {
			n := nodes[j]
			if _, e = s.nodeCall(ctx, n, "/xray/rollback", Data{"operation_id": op}); e != nil {
				if e = queueJob(ctx, tx, num(n, "id"), e.Error(), []any{op}); e != nil {
					return e
				}
				n["deploy_state"] = "rollback_pending"
				if e = save(ctx, tx, "nodes", n); e != nil {
					return e
				}
			}
		}
	}
	if _, e = tx.Exec(ctx, `UPDATE vr_operations SET status=$1 WHERE id=$2`, status, op); e != nil {
		return e
	}
	return tx.Commit(ctx)
}

func auditNodeTransition(ctx context.Context, q Query, node Data, previous string) error {
	if previous == str(node, "status") {
		return nil
	}
	return audit(ctx, q, 0, "node."+str(node, "status"), fmt.Sprintf("node:%d", num(node, "id")), Data{})
}
