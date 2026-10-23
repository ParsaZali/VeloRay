package panel

import (
	"net/http"
	"regexp"
)

var geoReleaseTag = regexp.MustCompile(`^v?[0-9][0-9A-Za-z.-]{0,63}$`)

func (s *Server) geoManagerAPI(w http.ResponseWriter, r *http.Request, node Data, action string, a Actor) (any, int, error) {
	if !flag(a.User, "is_superuser") || a.Scope != "admin" {
		return nil, 0, fail(403, "administrator access required")
	}
	ctx := r.Context()
	if action == "geo-manager" && r.Method == "GET" {
		out, err := s.nodeCall(ctx, node, "/geo/status", nil)
		return out, 200, err
	}
	if action == "geo-releases" && r.Method == "GET" {
		kind := r.URL.Query().Get("kind")
		if !oneOf(kind, "geoip", "geosite") {
			return nil, 0, fail(400, "choose geoip or geosite")
		}
		out, err := s.nodeCall(ctx, node, "/geo/catalog", Data{"kind": kind})
		return out, 200, err
	}
	if action != "geo-install" || r.Method != "POST" {
		return nil, 0, fail(405, "method not allowed")
	}
	in, err := readBody(w, r)
	if err != nil {
		return nil, 0, err
	}
	if err = allowed(in, "kind tag rollback"); err != nil {
		return nil, 0, fail(400, err.Error())
	}
	if err = typed(in, Data{"kind": "", "tag": "", "rollback": false}); err != nil {
		return nil, 0, fail(400, err.Error())
	}
	if !oneOf(str(in, "kind"), "geoip", "geosite") || (!flag(in, "rollback") && !geoReleaseTag.MatchString(str(in, "tag"))) {
		return nil, 0, fail(400, "choose a valid geo asset and release")
	}
	tx, err := s.Store.Pool.Begin(ctx)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback(ctx)
	if err = lockNodes(ctx, tx, num(node, "id")); err != nil {
		return nil, 0, err
	}
	if err = audit(ctx, tx, num(a.User, "id"), "geo.install.request", "nodes", Data{"node": num(node, "id"), "kind": str(in, "kind"), "tag": str(in, "tag"), "rollback": flag(in, "rollback")}); err != nil {
		return nil, 0, err
	}
	out, err := s.nodeCall(ctx, node, "/geo/install", in)
	if err != nil {
		return nil, 0, err
	}
	return out, 202, tx.Commit(ctx)
}
