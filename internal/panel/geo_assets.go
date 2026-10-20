package panel

import (
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) geoAssetsAPI(r *http.Request, id int64, a Actor) (any, int, error) {
	if !flag(a.User, "is_superuser") || !oneOf(a.Scope, "admin", "read") {
		return nil, 0, fail(403, "administrator access required")
	}
	if r.Method != "GET" {
		return nil, 0, fail(405, "method not allowed")
	}
	q := r.URL.Query()
	kind, query := q.Get("kind"), q.Get("q")
	limit := 100
	var err error
	if q.Has("limit") {
		limit, err = strconv.Atoi(q.Get("limit"))
	}
	if !oneOf(kind, "geoip", "geosite") || len(query) > 96 || strings.ContainsAny(query, "\r\n\x00") || err != nil || limit < 1 || limit > 100 {
		return nil, 0, fail(400, "invalid asset kind, search or limit")
	}
	query = strings.TrimSpace(query)
	node, err := get(r.Context(), s.Store.Pool, "nodes", id)
	if err != nil {
		return nil, 0, err
	}
	out, err := s.nodeCall(r.Context(), node, "/xray/geo-assets", Data{"kind": kind, "query": query, "limit": limit})
	if err != nil {
		return nil, 0, fail(502, "geo asset index unavailable; verify installed assets and update the node agent")
	}
	return out, 200, nil
}
