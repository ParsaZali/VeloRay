package panel

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func (s *Server) nodeCall(ctx context.Context, node Data, path string, payload any) (Data, error) {
	token, e := unseal(s.Config.FieldKey, str(node, "_agent_token"))
	if e != nil {
		return nil, e
	}
	var body io.Reader
	method := "GET"
	if payload != nil {
		b, e := json.Marshal(payload)
		if e != nil {
			return nil, e
		}
		body = bytes.NewReader(b)
		method = "POST"
	}
	ctx, cancel := context.WithTimeout(ctx, 55*time.Second)
	defer cancel()
	req, e := http.NewRequestWithContext(ctx, method, strings.TrimRight(str(node, "agent_url"), "/")+path, body)
	if e != nil {
		return nil, e
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	client := s.HTTP
	if client == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: !flag(node, "verify_tls")}
		if ca := os.Getenv("VELORAY_AGENT_CA_FILE"); ca != "" {
			raw, e := os.ReadFile(ca)
			if e != nil {
				return nil, e
			}
			pool, e := x509.SystemCertPool()
			if e != nil {
				pool = x509.NewCertPool()
			}
			if !pool.AppendCertsFromPEM(raw) {
				return nil, errors.New("invalid agent CA bundle")
			}
			transport.TLSClientConfig.RootCAs = pool
		}
		if cert, key := os.Getenv("VELORAY_AGENT_CLIENT_CERT"), os.Getenv("VELORAY_AGENT_CLIENT_KEY"); cert != "" || key != "" {
			pair, e := tls.LoadX509KeyPair(cert, key)
			if e != nil {
				return nil, e
			}
			transport.TLSClientConfig.Certificates = []tls.Certificate{pair}
		}
		transport.Proxy = nil
		defer transport.CloseIdleConnections()
		client = &http.Client{Transport: transport, Timeout: 55 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	res, e := client.Do(req)
	if e != nil {
		return nil, fmt.Errorf("%s agent is unavailable: %w", str(node, "name"), e)
	}
	defer res.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if e != nil {
		return nil, e
	}
	out, e := decode(raw)
	if e != nil {
		return nil, errors.New("agent returned invalid JSON")
	}
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("%s: %s", str(node, "name"), fallback(str(out, "detail"), "agent request failed"))
	}
	return out, nil
}
func (s *Server) collect(ctx context.Context, q Query, node Data) error {
	stats, e := s.nodeCall(ctx, node, "/xray/stats", nil)
	if e != nil {
		return e
	}
	if str(stats, "accounting") != "durable-v1" || str(stats, "ledger_id") == "" {
		return errors.New("upgrade the node agent before collecting traffic")
	}
	id := num(node, "id")
	ledgerID := str(stats, "ledger_id")
	if prev := str(node, "_ledger_id"); prev != "" && prev != ledgerID {
		return errors.New("node accounting ledger changed; restore its state before continuing")
	}
	for core, value := range obj(stats, "core_ledgers") {
		if prev := str(obj(node, "_core_ledgers"), core); prev != "" && prev != fmt.Sprint(value) {
			return errors.New("secondary core accounting ledger changed; restore its state before continuing")
		}
	}
	node["_core_ledgers"] = obj(stats, "core_ledgers")
	node["_ledger_id"] = ledgerID
	node["_outbound_accounting"] = str(stats, "outbound_accounting")
	node["_online_ips"] = obj(stats, "online_ips")
	node["_online_ips_available"] = flag(stats, "online_ips_available")
	node["_online_ip_cores"] = obj(stats, "online_ip_cores")
	node["_online_available"] = flag(stats, "online_available")
	node["_online_tracking_required"] = flag(stats, "online_tracking_required")
	node["_telemetry_at"] = stamp()
	online := Data{}
	for _, name := range array(stats, "online_users") {
		email := fmt.Sprint(name)
		metric := strings.Split(email, ">>>")
		if len(metric) == 3 && metric[0] == "user" && metric[2] == "online" {
			email = metric[1]
		}
		parts := strings.SplitN(email, ":", 3)
		if len(parts) == 3 && parts[0] == "veloray" {
			online[parts[1]] = true
		}
	}
	node["_online_clients"] = online
	inbounds, e := list(ctx, q, "inbounds", "WHERE node_id=$1", id)
	if e != nil {
		return e
	}
	bound := map[int64]Data{}
	ss := map[int64]Data{}
	clients := map[int64]Data{}
	for _, i := range inbounds {
		bound[num(i, "id")] = i
		cs, e := list(ctx, q, "clients", "WHERE inbound_id=$1", num(i, "id"))
		if e != nil {
			return e
		}
		for _, c := range cs {
			clients[num(c, "id")] = c
			if str(i, "protocol") == "shadowsocks" {
				ss[num(i, "id")] = c
			}
		}
	}
	var totalDelta, uploadDelta, downloadDelta int64
	ownerDeltas := map[int64][3]int64{}
	chargedClients := map[string]Data{}
	principalDeltas := map[string][3]int64{}
	for _, entry := range array(stats, "stat") {
		b, _ := json.Marshal(entry)
		v, e := decode(b)
		if e != nil {
			return e
		}
		name, value := str(v, "name"), num(v, "value")
		parts := strings.Split(name, ">>>")
		metric := name
		isOutbound := len(parts) == 4 && parts[0] == "outbound" && parts[2] == "traffic" && oneOf(parts[3], "uplink", "downlink")
		if isOutbound {
			// Older agents omit the core, making identical Xray/sing-box tags
			// ambiguous. Ignore those counters without disturbing client billing.
			if !oneOf(str(v, "core"), "xray", "singbox") || len(parts[1]) == 0 || len(parts[1]) > 256 {
				continue
			}
			metric = str(v, "core") + ">>>" + name
		}
		if value < 0 {
			return errors.New("negative agent counter")
		}
		var previous int64
		e = q.QueryRow(ctx, `SELECT total FROM vr_stats_cursors WHERE node_id=$1 AND metric=$2`, id, metric).Scan(&previous)
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if value < previous {
			return errors.New("durable counter decreased")
		}
		delta := value - previous
		_, e = q.Exec(ctx, `INSERT INTO vr_stats_cursors(node_id,metric,total) VALUES($1,$2,$3) ON CONFLICT(node_id,metric) DO UPDATE SET total=excluded.total`, id, metric, value)
		if e != nil {
			return e
		}
		if delta == 0 {
			continue
		}
		if isOutbound {
			if e = recordOutboundDelta(ctx, q, id, str(v, "core"), parts[1], parts[3], delta); e != nil {
				return e
			}
			continue
		}
		if len(parts) != 4 || parts[2] != "traffic" || !oneOf(parts[3], "uplink", "downlink") {
			continue
		}
		var client Data
		if parts[0] == "user" && strings.HasPrefix(parts[1], "veloray:") {
			p := strings.SplitN(parts[1], ":", 3)
			if len(p) >= 2 {
				cid, _ := strconv.ParseInt(p[1], 10, 64)
				client = clients[cid]
			}
			if client != nil && str(bound[num(client, "inbound")], "protocol") == "shadowsocks" {
				client = nil
			}
		} else if parts[0] == "inbound" && strings.HasPrefix(parts[1], "in-") {
			p := strings.SplitN(parts[1], "-", 3)
			if len(p) >= 2 {
				iid, _ := strconv.ParseInt(p[1], 10, 64)
				client = ss[iid]
			}
		}
		if client == nil {
			continue
		}
		key := accountKey(client)
		series := principalDeltas[key]
		if delta > int64(^uint64(0)>>1)-series[0] {
			return errors.New("principal traffic overflow")
		}
		series[0] += delta
		if parts[3] == "uplink" {
			series[1] += delta
		} else {
			series[2] += delta
		}
		principalDeltas[key] = series
		if chargedClients[key] == nil {
			chargedClients[key] = client
		}
		firstConnection := str(chargedClients[key], "last_traffic_at") == "" && num(chargedClients[key], "raw_lifetime_traffic_bytes") == 0
		if e = applyDirectionalTrafficDelta(chargedClients[key], parts[3], delta); e != nil {
			return e
		}
		if firstConnection {
			if e = audit(ctx, q, 0, "client.first_connection", key, Data{}); e != nil {
				return e
			}
		}
		if delta > int64(^uint64(0)>>1)-totalDelta {
			return errors.New("node traffic overflow")
		}
		totalDelta += delta
		owner := num(client, "owner_id")
		ownerDelta := ownerDeltas[owner]
		ownerDelta[0] += delta
		if parts[3] == "uplink" {
			uploadDelta += delta
			ownerDelta[1] += delta
		} else {
			downloadDelta += delta
			ownerDelta[2] += delta
		}
		ownerDeltas[owner] = ownerDelta
	}
	for _, c := range chargedClients {
		if e = save(ctx, q, "clients", c); e != nil {
			return e
		}
	}
	if e = s.recordNodeBilling(ctx, q, node, totalDelta, time.Now().UTC()); e != nil {
		return e
	}
	if totalDelta > 0 {
		at := time.Now().UTC().Truncate(5 * time.Minute)
		_, e = q.Exec(ctx, `INSERT INTO vr_traffic_windows(at,node_id,bytes,upload_bytes,download_bytes) VALUES($1,$2,$3,$4,$5) ON CONFLICT(at,node_id) DO UPDATE SET bytes=vr_traffic_windows.bytes+excluded.bytes,upload_bytes=vr_traffic_windows.upload_bytes+excluded.upload_bytes,download_bytes=vr_traffic_windows.download_bytes+excluded.download_bytes`, at, id, totalDelta, uploadDelta, downloadDelta)
		if e != nil {
			return e
		}
	}
	for key, delta := range principalDeltas {
		if delta[0] <= 0 {
			continue
		}
		_, err := q.Exec(ctx, `INSERT INTO vr_client_traffic_windows(at,principal_key,bytes,upload_bytes,download_bytes) VALUES($1,$2,$3,$4,$5) ON CONFLICT(principal_key,at) DO UPDATE SET bytes=vr_client_traffic_windows.bytes+excluded.bytes,upload_bytes=vr_client_traffic_windows.upload_bytes+excluded.upload_bytes,download_bytes=vr_client_traffic_windows.download_bytes+excluded.download_bytes`, time.Now().UTC().Truncate(5*time.Minute), key, delta[0], delta[1], delta[2])
		if err != nil {
			return err
		}
	}
	for owner, delta := range ownerDeltas {
		if owner <= 0 || delta[0] <= 0 {
			continue
		}
		_, err := q.Exec(ctx, `INSERT INTO vr_owner_traffic_windows(at,owner_id,bytes,upload_bytes,download_bytes) VALUES($1,$2,$3,$4,$5) ON CONFLICT(at,owner_id) DO UPDATE SET bytes=vr_owner_traffic_windows.bytes+excluded.bytes,upload_bytes=vr_owner_traffic_windows.upload_bytes+excluded.upload_bytes,download_bytes=vr_owner_traffic_windows.download_bytes+excluded.download_bytes`, time.Now().UTC().Truncate(5*time.Minute), owner, delta[0], delta[1], delta[2])
		if err != nil {
			return err
		}
	}
	return save(ctx, q, "nodes", node)
}
func queueJob(ctx context.Context, q Query, node int64, message string, ops []any) error {
	if ops == nil {
		ops = []any{}
	}
	raw, _ := json.Marshal(ops)
	_, e := q.Exec(ctx, `INSERT INTO vr_jobs(node_id,last_error,rollback_ops) VALUES($1,$2,$3) ON CONFLICT(node_id) DO UPDATE SET last_error=excluded.last_error,rollback_ops=vr_jobs.rollback_ops||excluded.rollback_ops,next_at=now(),updated_at=now()`, node, message, string(raw))
	return e
}
func (s *Server) compensate(ctx context.Context, applied []Data, operation string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 100*time.Second)
	defer cancel()
	for j := len(applied) - 1; j >= 0; j-- {
		n := applied[j]
		if _, e := s.nodeCall(ctx, n, "/xray/rollback", Data{"operation_id": operation}); e != nil {
			_ = queueJob(ctx, s.Store.Pool, num(n, "id"), e.Error(), []any{operation})
			_, _ = s.Store.Pool.Exec(ctx, `UPDATE vr_nodes SET data=jsonb_set(jsonb_set(data,'{deploy_state}','"rollback_pending"'),'{last_deploy_error}',to_jsonb($1::text)) WHERE id=$2`, e.Error(), num(n, "id"))
		}
	}
}

// change serializes mutations and uses agent-side journals to compensate actual configurations.
func (s *Server) change(ctx context.Context, ids []int64, actor Actor, action, target string, fn func(pgx.Tx) (any, error)) (out any, err error) {
	op := randomToken(24)
	rawIDs, _ := json.Marshal(ids)
	if _, e := s.Store.Pool.Exec(ctx, `INSERT INTO vr_operations(id,nodes) VALUES($1,$2)`, op, string(rawIDs)); e != nil {
		return nil, e
	}
	defer func() {
		if err != nil {
			_, _ = s.Store.Pool.Exec(context.WithoutCancel(ctx), `UPDATE vr_operations SET status='cancelled' WHERE id=$1 AND status='prepared'`, op)
		}
	}()
	tx, e := s.Store.Pool.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(842100,2)`); e != nil {
		return nil, e
	}
	if e = lockNodes(ctx, tx, ids...); e != nil {
		return nil, e
	}
	nodes := []Data{}
	for _, id := range ids {
		n, e := get(ctx, tx, "nodes", id)
		if e != nil {
			return nil, e
		}
		var pending bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vr_jobs WHERE node_id=$1 AND jsonb_array_length(rollback_ops)>0)`, id).Scan(&pending); e != nil {
			return nil, e
		}
		if pending {
			return nil, fail(409, "A previous deployment is being rolled back. Retry after the node recovers")
		}

		if e = s.collect(ctx, tx, n); e != nil {
			st, se := s.nodeCall(ctx, n, "/xray/status", nil)
			if se != nil || flag(st, "running") {
				return nil, fail(502, e.Error())
			}
		}
		nodes = append(nodes, n)
	}
	out, e = fn(tx)
	if e != nil {
		return nil, e
	}
	configs := make([]Data, len(nodes))
	for j, n := range nodes {
		fresh, e := get(ctx, tx, "nodes", num(n, "id"))
		if e != nil {
			return nil, e
		}
		cfg, e := s.buildConfig(ctx, tx, fresh)
		if e != nil {
			return nil, fail(400, e.Error())
		}
		configs[j] = cfg
		if _, e = s.nodeCall(ctx, fresh, "/xray/validate", Data{"config": cfg}); e != nil {
			return nil, failCode(502, "node_deploy_failed", e.Error())
		}
		nodes[j] = fresh
	}
	applied := []Data{}
	defer func() {
		if err != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
			committed := true
			for _, n := range nodes {
				fresh, checkErr := get(context.WithoutCancel(ctx), s.Store.Pool, "nodes", num(n, "id"))
				if checkErr != nil || str(fresh, "_last_operation") != op {
					committed = false
					break
				}
			}
			if !committed {
				s.compensate(ctx, applied, op)
			}
			_, _ = s.Store.Pool.Exec(context.WithoutCancel(ctx), `UPDATE vr_operations SET status=$1 WHERE id=$2`, map[bool]string{true: "committed", false: "compensated"}[committed], op)
		}
	}()
	for j, n := range nodes { // Include uncertain requests: the agent may have committed before a network timeout.
		applied = append(applied, n)
		if _, e = s.nodeCall(ctx, n, "/xray/apply", Data{"config": configs[j], "operation_id": op}); e != nil {
			return nil, failCode(502, "node_deploy_failed", e.Error())
		}
		if configs[j]["_singbox"] != nil {
			n["_singbox_deployed"] = true
		}
		n["_last_operation"] = op
		n["deploy_state"] = "applied"
		n["last_deploy_error"] = ""
		if e = save(ctx, tx, "nodes", n); e != nil {
			return nil, e
		}
		if _, e = tx.Exec(ctx, `DELETE FROM vr_jobs WHERE node_id=$1`, num(n, "id")); e != nil {
			return nil, e
		}
	}
	if e = audit(ctx, tx, num(actor.User, "id"), action, target, Data{}); e != nil {
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	_, _ = s.Store.Pool.Exec(context.WithoutCancel(ctx), `UPDATE vr_operations SET status='committed' WHERE id=$1`, op)
	return out, nil
}
func inboundNode(ctx context.Context, q Query, id int64) (int64, error) {
	i, e := get(ctx, q, "inbounds", id)
	return num(i, "node"), e
}
func nodeIDs(ids ...int64) []int64 {
	seen := map[int64]bool{}
	out := []int64{}
	for _, id := range ids {
		if id > 0 && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
