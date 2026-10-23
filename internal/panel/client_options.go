package panel

import (
	"net/http"
)

// Client creation needs a selectable label, never infrastructure configuration.
// This allowlist deliberately excludes ports, addresses, keys, TLS paths, stream
// settings and other users' counts. The full /inbounds API has its own permission.
func (s *Server) clientInboundOptions(r *http.Request, a Actor) (any, int, error) {
	if r.Method != http.MethodGet {
		return nil, 0, fail(405, "method not allowed")
	}
	where := "ORDER BY id"
	args := []any{}
	if !flag(a.User, "is_superuser") && (str(a.User, "role") == "reseller" || len(array(a.User, "allowed_inbounds")) > 0) {
		assigned := []int64{}
		for _, value := range array(a.User, "allowed_inbounds") {
			assigned = append(assigned, num(Data{"id": value}, "id"))
		}
		where = "WHERE id=ANY($1::bigint[]) ORDER BY id"
		args = append(args, assigned)
	}
	rows, err := list(r.Context(), s.Store.Pool, "inbounds", where, args...)
	if err != nil {
		return nil, 0, err
	}
	out := []Data{}
	for _, inbound := range rows {
		if !inboundAllowed(a.User, num(inbound, "id")) || !supportsClients(str(inbound, "protocol")) {
			continue
		}
		node, err := get(r.Context(), s.Store.Pool, "nodes", num(inbound, "node"))
		if err != nil {
			return nil, 0, err
		}
		protocol := str(inbound, "protocol")
		out = append(out, Data{
			"id": num(inbound, "id"), "name": str(inbound, "name"), "node_name": str(node, "name"),
			"protocol": protocol, "core": str(inbound, "core"), "enabled": flag(inbound, "enabled") && flag(node, "enabled"),
			"supports_clients": true, "usage_metered": metered(protocol),
			"supports_batch": protocol != "shadowsocks" || oneOf(str(obj(inbound, "protocol_settings"), "method"), "2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm"),
		})
	}
	return out, 200, nil
}
