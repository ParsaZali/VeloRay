package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
)

func serviceRevision(ctx context.Context, q Query, service Data) (string, error) {
	customers, e := list(ctx, q, "customers", `WHERE data->'service_ids' @> $1::jsonb ORDER BY id`, fmt.Sprintf("[%d]", num(service, "id")))
	if e != nil {
		return "", e
	}
	refs := []any{}
	for _, c := range customers {
		members, e := customerMembers(ctx, q, num(c, "id"))
		if e != nil {
			return "", e
		}
		connections := []any{}
		for _, m := range members {
			connections = append(connections, Data{"id": num(m, "id"), "inbound": num(m, "inbound")})
		}
		refs = append(refs, Data{"id": num(c, "id"), "inbound_ids": c["inbound_ids"], "service_ids": c["service_ids"], "connections": connections})
	}
	raw, _ := json.Marshal(Data{"service": service, "customers": refs})
	return digest(string(raw)), nil
}
func inboundsWithService(ctx context.Context, q Query, c, replacement Data) ([]int64, error) {
	ids, e := idArray(c, "inbound_ids", 200)
	if e != nil {
		return nil, e
	}
	services, e := idArray(c, "service_ids", 20)
	if e != nil {
		return nil, e
	}
	for _, id := range services {
		svc := replacement
		if id != num(replacement, "id") {
			svc, e = get(ctx, q, "services", id)
			if e != nil {
				return nil, e
			}
		}
		v, e := idArray(svc, "inbound_ids", 200)
		if e != nil {
			return nil, e
		}
		ids = append(ids, v...)
	}
	return nodeIDs(ids...), nil
}
func (s *Server) serviceImpact(ctx context.Context, q Query, old, candidate Data) (Data, error) {
	customers, e := list(ctx, q, "customers", `WHERE data->'service_ids' @> $1::jsonb ORDER BY id`, fmt.Sprintf("[%d]", num(old, "id")))
	if e != nil {
		return nil, e
	}
	ids, e := idArray(candidate, "inbound_ids", 200)
	if e != nil {
		return nil, e
	}
	oldIDs, e := idArray(old, "inbound_ids", 200)
	if e != nil {
		return nil, e
	}
	ids = append(ids, oldIDs...)
	added, removed, unchanged := 0, 0, 0
	affected := []any{}
	for _, c := range customers {
		want, e := inboundsWithService(ctx, q, c, candidate)
		if e != nil {
			return nil, e
		}
		if len(want) == 0 {
			return nil, fail(400, "this change would leave a customer without connections")
		}
		members, e := customerMembers(ctx, q, num(c, "id"))
		if e != nil {
			return nil, e
		}
		existing := map[int64]bool{}
		desired := map[int64]bool{}
		for _, m := range members {
			existing[num(m, "inbound")] = true
			ids = append(ids, num(m, "inbound"))
		}
		for _, id := range want {
			desired[id] = true
			ids = append(ids, id)
			if existing[id] {
				unchanged++
			} else {
				added++
			}
		}
		for id := range existing {
			if !desired[id] {
				removed++
			}
		}
		if len(affected) < 20 {
			affected = append(affected, Data{"id": num(c, "id"), "name": str(c, "name")})
		}
	}
	nodes, e := nodesForInbounds(ctx, q, nodeIDs(ids...))
	if e != nil {
		return nil, e
	}
	revision, e := serviceRevision(ctx, q, old)
	if e != nil {
		return nil, e
	}
	return Data{"revision": revision, "customers": len(customers), "nodes": idsJSON(nodes), "connections_added": added, "connections_removed": removed, "connections_preserved": unchanged, "sample_customers": affected, "credentials_preserved": true, "plan_defaults_reapplied": false}, nil
}
func (s *Server) serviceImpactAPI(w http.ResponseWriter, r *http.Request, id int64, a Actor) (any, int, error) {
	if r.Method != "POST" {
		return nil, 0, fail(405, "method not allowed")
	}
	if !commerceAdmin(a) {
		return nil, 0, fail(403, "administrator access required")
	}
	input, e := readBody(w, r)
	if e != nil {
		return nil, 0, e
	}
	if e = allowed(input, "name note inbound_ids defaults"); e != nil {
		return nil, 0, fail(400, e.Error())
	}
	if e = typed(input, Data{"name": "", "note": "", "inbound_ids": []any{}, "defaults": Data{}}); e != nil {
		return nil, 0, fail(400, e.Error())
	}
	out, e := s.managementChange(r.Context(), nil, a, "service.preview", strconv.FormatInt(id, 10), func(tx pgx.Tx) (any, error) {
		old, e := get(r.Context(), tx, "services", id)
		if e != nil {
			return nil, e
		}
		return s.serviceImpact(r.Context(), tx, old, merge(old, input))
	})
	return out, 200, e
}
