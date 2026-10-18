package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Only network policy is reusable. Managed listeners, API endpoints and accounting
// are owned by the panel and cannot be replaced through a shared profile.
func validateProfilePatch(core string, d Data) error {
	keys := "dns routing outbounds policy log"
	if core == "singbox" {
		keys = "dns route outbounds log"
	}
	if e := allowed(d, keys); e != nil {
		return fail(400, e.Error())
	}
	raw, _ := json.Marshal(d)
	if len(raw) > 128<<10 {
		return fail(400, "profile patch exceeds 128 KiB")
	}
	for k, v := range d {
		if k == "outbounds" {
			if _, ok := v.([]any); !ok {
				return fail(400, "outbounds must be an array")
			}
		} else if _, ok := v.(map[string]any); !ok {
			if _, ok := v.(Data); !ok {
				return fail(400, k+" must be an object")
			}
		}
	}
	return nil
}
func (s *Server) managedConfigBody(d Data) (Data, error) {
	raw, e := unseal(s.Config.FieldKey, str(d, "_body"))
	if e != nil {
		return nil, e
	}
	return decode([]byte(raw))
}
func (s *Server) profilePatch(ctx context.Context, q Query, id int64, core string) (Data, error) {
	if id == 0 {
		return Data{}, nil
	}
	d, e := get(ctx, q, "profiles", id)
	if e != nil {
		return nil, e
	}
	body, e := s.managedConfigBody(d)
	if e != nil {
		return nil, e
	}
	out := Data{}
	for _, v := range array(body, "snippet_ids") {
		snippet, e := get(ctx, q, "snippets", num(Data{"id": v}, "id"))
		if e != nil {
			return nil, e
		}
		b, e := s.managedConfigBody(snippet)
		if e != nil {
			return nil, e
		}
		if str(b, "core") == core {
			out = deepMerge(out, obj(b, "config"))
		}
	}
	return deepMerge(out, obj(body, core)), nil
}
func (s *Server) affectedProfileNodes(ctx context.Context, q Query, kind string, id int64) ([]int64, error) {
	if id < 1 {
		return []int64{}, nil
	}
	profiles := []int64{id}
	if kind == "snippets" {
		profiles = []int64{}
		rows, e := list(ctx, q, "profiles", "")
		if e != nil {
			return nil, e
		}
		for _, p := range rows {
			body, e := s.managedConfigBody(p)
			if e != nil {
				return nil, e
			}
			for _, v := range array(body, "snippet_ids") {
				if num(Data{"id": v}, "id") == id {
					profiles = append(profiles, num(p, "id"))
					break
				}
			}
		}
	}
	rows, e := list(ctx, q, "nodes", "WHERE COALESCE((data->>'profile_id')::bigint,0)=ANY($1::bigint[])", profiles)
	if e != nil {
		return nil, e
	}
	ids := []int64{}
	for _, n := range rows {
		ids = append(ids, num(n, "id"))
	}
	return nodeIDs(ids...), nil
}
func ensureCovered(actual, locked []int64) error {
	set := map[int64]bool{}
	for _, id := range locked {
		set[id] = true
	}
	for _, id := range actual {
		if !set[id] {
			return fail(409, "node assignments changed; retry")
		}
	}
	return nil
}
func (s *Server) saveManagedConfig(ctx context.Context, tx pgx.Tx, kind string, d, body Data) error {
	raw, e := json.Marshal(body)
	if e != nil {
		return e
	}
	sealed, e := seal(s.Config.FieldKey, string(raw))
	if e != nil {
		return e
	}
	d["_body"], d["revision"] = sealed, num(d, "revision")+1
	if e = save(ctx, tx, kind, d); e != nil {
		return e
	}
	raw, e = json.Marshal(d)
	if e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO vr_config_revisions(kind,entity_id,revision,data) VALUES($1,$2,$3,$4)`, kind, num(d, "id"), num(d, "revision"), string(raw)); e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `DELETE FROM vr_config_revisions WHERE kind=$1 AND entity_id=$2 AND revision <= $3-50`, kind, num(d, "id"), num(d, "revision"))
	return e
}
func (s *Server) validateManagedBody(ctx context.Context, q Query, kind string, body Data) error {
	template := Data{"xray": Data{}, "singbox": Data{}, "snippet_ids": []any{}}
	if kind == "snippets" {
		template = Data{"core": "", "config": Data{}}
	}
	if e := typed(body, template); e != nil {
		return fail(400, e.Error())
	}
	if kind == "snippets" {
		if e := allowed(body, "core config"); e != nil {
			return fail(400, e.Error())
		}
		if !oneOf(str(body, "core"), "xray", "singbox") {
			return fail(400, "invalid snippet core")
		}
		return validateProfilePatch(str(body, "core"), obj(body, "config"))
	}
	if e := allowed(body, "xray singbox snippet_ids"); e != nil {
		return fail(400, e.Error())
	}
	for _, core := range []string{"xray", "singbox"} {
		if e := validateProfilePatch(core, obj(body, core)); e != nil {
			return e
		}
	}
	ids, e := idArray(body, "snippet_ids", 32)
	if e != nil {
		return e
	}
	for _, id := range ids {
		if _, e = get(ctx, q, "snippets", id); e != nil {
			return fail(400, "snippet does not exist")
		}
	}
	return nil
}
func (s *Server) profilesAPI(w http.ResponseWriter, r *http.Request, path string, a Actor) (any, int, error) {
	if !flag(a.User, "is_superuser") || a.Scope != "admin" {
		return nil, 0, fail(403, "administrator access required")
	}
	ctx := r.Context()
	parts := strings.Split(strings.Trim(path, "/"), "/")
	kind := parts[0]
	id := int64(0)
	action := ""
	if len(parts) > 3 {
		return nil, 0, fail(404, "endpoint not found")
	}
	if len(parts) > 1 {
		var e error
		id, e = strconv.ParseInt(parts[1], 10, 64)
		if e != nil || id < 1 {
			return nil, 0, fail(404, "configuration not found")
		}
	}
	if len(parts) == 3 {
		action = parts[2]
	}
	if r.Method == "GET" {
		if id == 0 {
			rows, e := list(ctx, s.Store.Pool, kind, "ORDER BY id")
			out := []Data{}
			for _, d := range rows {
				out = append(out, public(d))
			}
			return out, 200, e
		}
		d, e := get(ctx, s.Store.Pool, kind, id)
		if e != nil {
			return nil, 0, e
		}
		if action == "revisions" {
			rows, e := s.Store.Pool.Query(ctx, `SELECT revision,created_at FROM vr_config_revisions WHERE kind=$1 AND entity_id=$2 ORDER BY revision DESC`, kind, id)
			if e != nil {
				return nil, 0, e
			}
			defer rows.Close()
			out := []Data{}
			for rows.Next() {
				var revision int64
				var at any
				if e = rows.Scan(&revision, &at); e != nil {
					return nil, 0, e
				}
				out = append(out, Data{"revision": revision, "created_at": at})
			}
			return out, 200, rows.Err()
		}
		if action != "" {
			return nil, 0, fail(404, "action not found")
		}
		body, e := s.managedConfigBody(d)
		if e != nil {
			return nil, 0, e
		}
		out := public(d)
		out["body"] = body
		if kind == "profiles" {
			ids, e := s.affectedProfileNodes(ctx, s.Store.Pool, kind, id)
			if e != nil {
				return nil, 0, e
			}
			out["node_ids"] = idsJSON(ids)
		}
		return out, 200, nil
	}
	if !((r.Method == "POST" && id == 0 && action == "") || (oneOf(r.Method, "PATCH", "DELETE") && id > 0 && action == "") || (r.Method == "POST" && id > 0 && oneOf(action, "assign", "restore"))) {
		return nil, 0, fail(405, "method not allowed")
	}
	input := Data{}
	var e error
	if r.Method != "DELETE" {
		input, e = readBody(w, r)
		if e != nil {
			return nil, 0, e
		}
		keys := "name note body revision"
		if action == "assign" {
			keys = "node_ids"
		}
		if action == "restore" {
			keys = "revision"
		}
		if e = allowed(input, keys); e != nil {
			return nil, 0, fail(400, e.Error())
		}
		if e = typed(input, Data{"name": "", "note": "", "body": Data{}, "node_ids": []any{}, "revision": 0}); e != nil {
			return nil, 0, fail(400, e.Error())
		}
	}
	nodes, e := s.affectedProfileNodes(ctx, s.Store.Pool, kind, id)
	if e != nil {
		return nil, 0, e
	}
	if action == "assign" {
		if kind != "profiles" {
			return nil, 0, fail(400, "only profiles can be assigned")
		}
		ids, e := idArray(input, "node_ids", 1000)
		if e != nil {
			return nil, 0, e
		}
		nodes = nodeIDs(append(nodes, ids...)...)
	}
	out, e := s.managementChange(ctx, nodes, a, kind+"."+fallback(action, strings.ToLower(r.Method)), fmt.Sprint(id), func(tx pgx.Tx) (any, error) {
		d := Data{"name": "", "note": "", "revision": 0}
		if id > 0 {
			var e error
			d, e = get(ctx, tx, kind, id)
			if e != nil {
				return nil, e
			}
		}
		actual, e := s.affectedProfileNodes(ctx, tx, kind, id)
		if e != nil {
			return nil, e
		}
		if e = ensureCovered(actual, nodes); e != nil {
			return nil, e
		}
		if action == "assign" {
			ids, e := idArray(input, "node_ids", 1000)
			if e != nil {
				return nil, e
			}
			selected := map[int64]bool{}
			for _, id := range ids {
				selected[id] = true
			}
			for _, nodeID := range nodes {
				n, e := get(ctx, tx, "nodes", nodeID)
				if e != nil {
					return nil, fail(400, "node does not exist")
				}
				if selected[nodeID] {
					n["profile_id"] = id
				} else if num(n, "profile_id") == id {
					n["profile_id"] = 0
				}
				if e = save(ctx, tx, "nodes", n); e != nil {
					return nil, e
				}
			}
			return Data{"node_ids": idsJSON(ids)}, nil
		}
		if r.Method == "DELETE" {
			if len(actual) > 0 {
				return nil, fail(409, "configuration is assigned to nodes")
			}
			if kind == "snippets" {
				profiles, e := list(ctx, tx, "profiles", "")
				if e != nil {
					return nil, e
				}
				for _, p := range profiles {
					body, e := s.managedConfigBody(p)
					if e != nil {
						return nil, e
					}
					for _, v := range array(body, "snippet_ids") {
						if num(Data{"id": v}, "id") == id {
							return nil, fail(409, "snippet is referenced by a profile")
						}
					}
				}
			}
			if _, e = tx.Exec(ctx, `DELETE FROM vr_config_revisions WHERE kind=$1 AND entity_id=$2`, kind, id); e != nil {
				return nil, e
			}
			return nil, remove(ctx, tx, kind, id)
		}
		body := Data{"xray": Data{}, "singbox": Data{}, "snippet_ids": []any{}}
		if kind == "snippets" {
			body = Data{"core": "xray", "config": Data{}}
		}
		if id > 0 {
			body, e = s.managedConfigBody(d)
			if e != nil {
				return nil, e
			}
		}
		if input["revision"] != nil && action != "restore" && num(input, "revision") != num(d, "revision") {
			return nil, fail(409, "configuration changed; reload before saving")
		}
		if action == "restore" {
			var raw []byte
			if e = tx.QueryRow(ctx, `SELECT data FROM vr_config_revisions WHERE kind=$1 AND entity_id=$2 AND revision=$3`, kind, id, num(input, "revision")).Scan(&raw); e != nil {
				return nil, e
			}
			snapshot, e := decode(raw)
			if e != nil {
				return nil, e
			}
			body, e = s.managedConfigBody(snapshot)
			if e != nil {
				return nil, e
			}
		}
		if input["body"] != nil {
			body = obj(input, "body")
		}
		for _, k := range []string{"name", "note"} {
			if v, ok := input[k]; ok {
				d[k] = v
			}
		}
		if e = require(d, "name", 96); e != nil {
			return nil, fail(400, e.Error())
		}
		if e = s.validateManagedBody(ctx, tx, kind, body); e != nil {
			return nil, e
		}
		if e = s.saveManagedConfig(ctx, tx, kind, d, body); e != nil {
			return nil, e
		}
		return public(d), nil
	})
	status := 200
	if r.Method == "POST" && id == 0 {
		status = 201
	}
	if r.Method == "DELETE" {
		status = 204
	}
	return out, status, e
}

func profileConfigPatch(ctx context.Context, q Query, node Data, core string, keys ...string) (Data, error) {
	key := ""
	if len(keys) > 0 {
		key = keys[0]
	}
	s := &Server{Config: Config{FieldKey: key}}
	return s.profilePatch(ctx, q, num(node, "profile_id"), core)
}
func (s *Server) buildConfig(ctx context.Context, q Query, node Data) (Data, error) {
	return buildConfig(ctx, q, node, s.Config.FieldKey)
}
