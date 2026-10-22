package panel

import (
	"context"
	"net/http"
)

func nodeNeedsSingIPs(ctx context.Context, q Query, nodeID int64) (bool, error) {
	var needed bool
	err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vr_clients c JOIN vr_inbounds i ON i.id=c.inbound_id WHERE i.node_id=$1 AND i.data->>'core'='singbox' AND COALESCE((SELECT data->>'device_limit' FROM vr_accounts WHERE id=c.account_id),c.data->>'device_limit','0')::bigint>0)`, nodeID).Scan(&needed)
	return needed, err
}
func (s *Server) coreManagerAPI(w http.ResponseWriter, r *http.Request, node Data, action string, a Actor) (any, int, error) {
	if !flag(a.User, "is_superuser") || a.Scope != "admin" {
		return nil, 0, fail(403, "administrator access required")
	}
	ctx := r.Context()
	if action == "core-manager" && r.Method == "GET" {
		out, err := s.nodeCall(ctx, node, "/cores/status", nil)
		return out, 200, err
	}
	if action == "core-releases" && r.Method == "GET" {
		core := r.URL.Query().Get("core")
		if !oneOf(core, "xray", "singbox") {
			return nil, 0, fail(400, "unsupported core")
		}
		out, err := s.nodeCall(ctx, node, "/cores/catalog", Data{"core": core})
		return out, 200, err
	}
	if action != "core-install" && action != "core-policy" {
		return nil, 0, fail(405, "method not allowed")
	}
	if r.Method != "POST" {
		return nil, 0, fail(405, "method not allowed")
	}
	in, err := readBody(w, r)
	if err != nil {
		return nil, 0, err
	}
	if action == "core-policy" {
		if err = allowed(in, "enabled"); err != nil {
			return nil, 0, fail(400, err.Error())
		}
		if _, ok := in["enabled"].(bool); !ok {
			return nil, 0, fail(400, "enabled must be boolean")
		}
		if err = audit(ctx, s.Store.Pool, num(a.User, "id"), "core.policy.request", "nodes", Data{"node": num(node, "id"), "enabled": flag(in, "enabled")}); err != nil {
			return nil, 0, err
		}
		out, err := s.nodeCall(ctx, node, "/cores/policy", in)
		return out, 200, err
	}
	if err = allowed(in, "core tag rollback"); err != nil {
		return nil, 0, fail(400, err.Error())
	}
	if err = typed(in, Data{"core": "", "tag": "", "rollback": false}); err != nil {
		return nil, 0, fail(400, err.Error())
	}
	if !oneOf(str(in, "core"), "xray", "singbox") {
		return nil, 0, fail(400, "unsupported core")
	}
	tx, err := s.Store.Pool.Begin(ctx)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback(ctx)
	if err = lockNodes(ctx, tx, num(node, "id")); err != nil {
		return nil, 0, err
	}
	needed, err := nodeNeedsSingIPs(ctx, tx, num(node, "id"))
	if err != nil {
		return nil, 0, err
	}
	in["require_user_ips"] = needed
	if err = audit(ctx, s.Store.Pool, num(a.User, "id"), "core.install.request", "nodes", Data{"node": num(node, "id"), "core": str(in, "core"), "tag": str(in, "tag"), "rollback": flag(in, "rollback")}); err != nil {
		return nil, 0, err
	}
	out, err := s.nodeCall(ctx, node, "/cores/install", in)
	return out, 202, err
}
