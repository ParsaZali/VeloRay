package panel

import (
	"context"
	"net/http"
	"strconv"
	"time"
)

// Outbound telemetry is independent of billed customer usage. A connection may
// traverse several outbounds; adding these totals to customer quotas double bills.
func recordOutboundDelta(ctx context.Context, q Query, node int64, core, tag, direction string, delta int64) error {
	var upload, download int64
	if direction == "uplink" {
		upload = delta
	} else {
		download = delta
	}
	_, e := q.Exec(ctx, `INSERT INTO vr_outbound_traffic_totals(node_id,core,tag,upload_bytes,download_bytes) VALUES($1,$2,$3,$4,$5)
 ON CONFLICT(node_id,core,tag) DO UPDATE SET upload_bytes=vr_outbound_traffic_totals.upload_bytes+excluded.upload_bytes,download_bytes=vr_outbound_traffic_totals.download_bytes+excluded.download_bytes,updated_at=now()`, node, core, tag, upload, download)
	if e != nil {
		return e
	}
	_, e = q.Exec(ctx, `INSERT INTO vr_outbound_traffic_windows(at,node_id,core,tag,upload_bytes,download_bytes) VALUES($1,$2,$3,$4,$5,$6)
 ON CONFLICT(at,node_id,core,tag) DO UPDATE SET upload_bytes=vr_outbound_traffic_windows.upload_bytes+excluded.upload_bytes,download_bytes=vr_outbound_traffic_windows.download_bytes+excluded.download_bytes`, time.Now().UTC().Truncate(5*time.Minute), node, core, tag, upload, download)
	return e
}

func (s *Server) outboundTraffic(r *http.Request, a Actor) (any, error) {
	if r.Method != http.MethodGet {
		return nil, fail(405, "method not allowed")
	}
	if ownClients(a.User) || !permitted(a.User, "nodes.read") || !permitted(a.User, "traffic.read") {
		return nil, fail(403, "node-wide traffic requires nodes.read and traffic.read without owner restrictions")
	}
	node, e := strconv.ParseInt(r.URL.Query().Get("node"), 10, 64)
	if e != nil || node < 1 {
		return nil, fail(400, "select a node")
	}
	core := r.URL.Query().Get("core")
	if !oneOf(core, "xray", "singbox") {
		return nil, fail(400, "select xray or singbox")
	}
	hours := int64(24)
	if raw := r.URL.Query().Get("hours"); raw != "" {
		hours, e = strconv.ParseInt(raw, 10, 64)
		if e != nil || hours < 1 || hours > 8760 {
			return nil, fail(400, "hours must be 1–8760")
		}
	}
	ctx := r.Context()
	n, e := get(ctx, s.Store.Pool, "nodes", node)
	if e != nil {
		return nil, e
	}
	rows, e := s.Store.Pool.Query(ctx, `SELECT t.tag,t.upload_bytes,t.download_bytes,t.updated_at,COALESCE(w.up,0)::bigint,COALESCE(w.down,0)::bigint
 FROM vr_outbound_traffic_totals t LEFT JOIN (
 SELECT tag,sum(upload_bytes) AS up,sum(download_bytes) AS down FROM vr_outbound_traffic_windows
 WHERE node_id=$1 AND core=$2 AND at>now()-$3*interval '1 hour' GROUP BY tag
 ) w ON w.tag=t.tag WHERE t.node_id=$1 AND t.core=$2 ORDER BY t.tag LIMIT 1000`, node, core, hours)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Data{}
	for rows.Next() {
		var tag string
		var up, down, recentUp, recentDown int64
		var at time.Time
		if e = rows.Scan(&tag, &up, &down, &at, &recentUp, &recentDown); e != nil {
			return nil, e
		}
		out = append(out, Data{"tag": tag, "upload_bytes": up, "download_bytes": down, "period_upload_bytes": recentUp, "period_download_bytes": recentDown, "updated_at": stamp(at)})
	}
	return Data{"node": node, "core": core, "hours": hours, "available": str(n, "_outbound_accounting") == "core-scoped-v1" && (core == "xray" || str(obj(n, "_core_ledgers"), "singbox") != ""), "collected_at": n["_telemetry_at"], "outbounds": out}, rows.Err()
}
