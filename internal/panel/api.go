package panel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var editable = map[string]string{
	"nodes":         "profile_id name public_host agent_url agent_token enabled verify_tls config_patch",
	"inbounds":      "name node core listen port protocol transport security flow path host_header service_name tls_server_name tls_cert_file tls_key_file reality_dest reality_server_name reality_private_key reality_public_key reality_short_id protocol_settings stream_settings enabled",
	"clients":       "renewal_period renewal_every renewal_timezone hwid_limit device_limit start_after_first_use duration_days owner_id inbound name credential protocol_settings subscription_token enabled expires_at traffic_limit_bytes renewal_interval_days next_renewal_at traffic_multiplier note",
	"outbounds":     "name node core tag protocol config enabled",
	"routing_rules": "name node core priority config enabled",
	"users":         "ldap_dn oidc_issuer oidc_subject username email is_active is_staff is_superuser password role permissions allowed_inbounds own_clients_only max_clients traffic_budget_bytes",
	"api_keys":      "name scope enabled expires_at user",
}

func (s *Server) view(ctx context.Context, q Query, kind string, d Data) (Data, error) {
	out := public(d)
	if kind == "clients" {
		out["device_count"] = len(obj(d, "_device_seen"))
		out["device_limit"] = num(d, "device_limit")
		out["start_after_first_use"] = flag(d, "start_after_first_use")
		out["duration_days"] = num(d, "duration_days")
		out["first_used_at"] = d["first_used_at"]
	}
	switch kind {
	case "users":
		out["two_factor_enabled"] = flag(d, "_totp_enabled")
		out["role"] = accountRole(d)
		out["permissions"] = permissions(d)
		var count, allocated int64
		if e := q.QueryRow(ctx, `SELECT count(*), COALESCE(sum(quota),0)::bigint FROM (SELECT DISTINCT COALESCE(account_id,-id) AS account, COALESCE((SELECT data->>'traffic_limit_bytes' FROM vr_accounts WHERE id=vr_clients.account_id),vr_clients.data->>'traffic_limit_bytes','0')::bigint AS quota FROM vr_clients WHERE `+ownerExpression+`=$1) accounts`, num(d, "id")).Scan(&count, &allocated); e != nil {
			return nil, e
		}
		out["clients_count"], out["allocated_traffic_bytes"] = count, allocated
	case "api_keys":
		u, e := get(ctx, q, "users", num(d, "user"))
		if e != nil {
			return nil, e
		}
		out["user_name"] = str(u, "username")
	case "inbounds":
		n, e := get(ctx, q, "nodes", num(d, "node"))
		if e != nil {
			return nil, e
		}
		out["node_name"] = str(n, "name")
		var count int
		if e = q.QueryRow(ctx, `SELECT count(*) FROM vr_clients WHERE inbound_id=$1`, num(d, "id")).Scan(&count); e != nil {
			return nil, e
		}
		out["client_count"] = count
		out["supports_clients"] = supportsClients(str(d, "protocol"))
	case "clients":
		i, e := get(ctx, q, "inbounds", num(d, "inbound"))
		if e != nil {
			return nil, e
		}
		n, e := get(ctx, q, "nodes", num(i, "node"))
		if e != nil {
			return nil, e
		}
		for _, k := range []string{"protocol", "transport", "security"} {
			out[k] = i[k]
		}
		out["inbound_name"] = str(i, "name")
		out["node_name"] = str(n, "name")
		out["usage_metered"] = metered(str(i, "protocol"))
		out["online"] = nil
		out["connection_status"] = "unknown"
		out["telemetry_at"] = n["_telemetry_at"]
		if !flag(d, "enabled") {
			out["online"] = false
			out["connection_status"] = "disabled"
		} else if observed, err := timestamp(n, "_telemetry_at"); err == nil && time.Since(observed) < 20*time.Second && flag(n, "_online_available") && oneOf(str(i, "protocol"), "vless", "vmess", "trojan", "hysteria", "tuic", "naive", "snell", "shadowtls") {
			connected := flag(obj(n, "_online_clients"), strconv.FormatInt(num(d, "id"), 10))
			out["online"] = connected
			out["connection_status"] = "offline"
			if connected {
				out["connection_status"] = "online"
			}
		}
		out["account_id"] = num(d, "_account")
		if num(d, "_account") > 0 {
			var token string
			var count int
			if e = q.QueryRow(ctx, `SELECT COALESCE((SELECT data->>'subscription_token' FROM vr_accounts WHERE id=$1), data->>'subscription_token') FROM vr_clients WHERE account_id=$1 ORDER BY id LIMIT 1`, num(d, "_account")).Scan(&token); e != nil {
				return nil, e
			}
			if e = q.QueryRow(ctx, `SELECT count(*) FROM vr_clients WHERE account_id=$1`, num(d, "_account")).Scan(&count); e != nil {
				return nil, e
			}
			out["subscription_token"], out["account_connections"] = token, count
		}
		rate := effectiveTrafficMultiplier(d)
		out["effective_traffic_multiplier"] = rate
		out["share_link"] = shareLink(d, i, n)
		out["profile_text"] = profile(d, i, n)
		out["subscription_url"], e = s.subscriptionURL(ctx, q, str(out, "subscription_token"))
		if e != nil {
			return nil, e
		}
		out["subscription_portal_url"] = out["subscription_url"]
	}
	return out, nil
}
func (s *Server) api(w http.ResponseWriter, r *http.Request, path string, a Actor) (any, int, error) {
	ctx := r.Context()
	if path == "monitors" || strings.HasPrefix(path, "monitors/") {
		return s.monitorsAPI(w, r, path, a)
	}
	if strings.HasPrefix(path, "operations/") {
		return s.nodeOperationsAPI(w, r, path, a)
	}
	if path == "subscription-policies" || strings.HasPrefix(path, "subscription-policies/") {
		return s.subscriptionPoliciesAPI(w, r, path, a)
	}
	if path == "profiles" || path == "snippets" || strings.HasPrefix(path, "profiles/") || strings.HasPrefix(path, "snippets/") {
		return s.profilesAPI(w, r, path, a)
	}
	if path == "customers" || strings.HasPrefix(path, "customers/") {
		return s.customersAPI(w, r, path, a)
	}
	if path == "services" || strings.HasPrefix(path, "services/") {
		return s.servicesAPI(w, r, path, a)
	}
	if strings.HasPrefix(path, "imports/") {
		return s.importsAPI(w, r, path, a)
	}
	if strings.HasPrefix(path, "telegram/") {
		return s.telegramAPI(w, r, path, a)
	}
	if path == "subscription-template/validate" {
		return s.validatePortalAPI(w, r, a)
	}
	if strings.HasPrefix(path, "webhooks/") {
		return s.webhooksAPI(w, r, path, a)
	}
	if strings.HasPrefix(path, "updates/") {
		return s.updatesAPI(r, path)
	}
	parts := strings.Split(path, "/")
	if path == "openapi" && r.Method == "GET" {
		return openAPISpec(s.Config.PublicURL), 200, nil
	}
	if path == "clients/batch" {
		return s.batchClients(w, r, a)
	}
	if path == "clients/bulk" {
		return s.bulkClients(w, r, a)
	}
	if path == "subscription-endpoint" {
		return s.subscriptionEndpointAPI(w, r, a)
	}
	if path == "settings" {
		return s.settingsAPI(w, r, a)
	}
	switch path {
	case "overview":
		if r.Method != "GET" {
			return nil, 0, fail(405, "method not allowed")
		}
		out, e := s.overviewForActor(ctx, a)
		return out, 200, e
	case "traffic/clients":
		if r.Method != "GET" {
			return nil, 0, fail(405, "method not allowed")
		}
		where, args := actorClientWhere(a)
		clients, err := list(ctx, s.Store.Pool, "clients", where, args...)
		if err != nil {
			return nil, 0, err
		}
		output := []Data{}
		for _, client := range clients {
			v, err := s.view(ctx, s.Store.Pool, "clients", client)
			if err != nil {
				return nil, 0, err
			}
			safe := Data{}
			for _, key := range []string{"id", "name", "inbound", "inbound_name", "node_name", "enabled", "expires_at", "traffic_limit_bytes", "used_traffic_bytes", "raw_used_traffic_bytes", "raw_lifetime_traffic_bytes", "traffic_multiplier", "effective_traffic_multiplier", "renewal_interval_days", "next_renewal_at", "disabled_reason", "account_id"} {
				safe[key] = v[key]
			}
			output = append(output, safe)
		}
		return output, 200, nil
	case "system-info":
		out, e := s.systemInfo(ctx)
		return out, 200, e
	case "traffic-history":
		out, e := s.trafficHistoryForActor(ctx, r, a)
		return out, 200, e
	case "outbound-traffic":
		out, e := s.outboundTraffic(r, a)
		return out, 200, e
	}
	kind := parts[0]
	if kind == "admin" && len(parts) >= 2 {
		kind = map[string]string{"users": "users", "api-keys": "api_keys"}[parts[1]]
		parts = parts[1:]
	}
	if _, ok := tables[kind]; !ok {
		return nil, 0, fail(404, "endpoint not found")
	}
	var id int64
	var e error
	if len(parts) > 1 {
		id, e = strconv.ParseInt(parts[1], 10, 64)
		if e != nil || id < 1 {
			return nil, 0, fail(404, "not found")
		}
	}
	if e = s.entityAccess(ctx, a, kind, id); e != nil {
		return nil, 0, e
	}
	if kind == "nodes" && len(parts) == 3 && parts[2] == "balancing" {
		return s.balancingAPI(w, r, id, a)
	}
	if kind == "nodes" && len(parts) == 3 && parts[2] == "geo-assets" {
		return s.geoAssetsAPI(r, id, a)
	}
	if kind == "api_keys" && len(parts) > 2 && parts[2] == "reset" {
		return s.resetAPIKey(w, r, id, a)
	}
	if kind == "clients" && len(parts) == 3 && parts[2] == "subscription-history" {
		if r.Method != "GET" {
			return nil, 0, fail(405, "method not allowed")
		}
		c, e := get(ctx, s.Store.Pool, "clients", id)
		if e != nil {
			return nil, 0, e
		}
		out, e := s.subscriptionHistory(ctx, c)
		return out, 200, e
	}
	if kind == "clients" && len(parts) == 3 && parts[2] == "delivery-check" {
		if r.Method != "GET" {
			return nil, 0, fail(405, "method not allowed")
		}
		out, err := s.deliveryCheck(ctx, id)
		return out, 200, err
	}
	if kind == "clients" && len(parts) == 3 && parts[2] == "device-action" {
		return s.deviceAction(w, r, id, a)
	}
	if kind == "clients" && len(parts) == 3 && oneOf(parts[2], "ips", "devices") {
		if r.Method != http.MethodGet {
			return nil, 0, fail(405, "method not allowed")
		}
		out, err := s.clientConnections(ctx, id, parts[2])
		return out, 200, err
	}
	if len(parts) > 2 {
		return s.action(w, r, kind, id, parts[2], a)
	}
	if r.Method == "GET" {
		if id > 0 {
			d, e := get(ctx, s.Store.Pool, kind, id)
			if e != nil {
				return nil, 0, e
			}
			v, e := s.view(ctx, s.Store.Pool, kind, d)
			if kind == "inbounds" && !permitted(a.User, "inbounds.write") {
				for _, key := range []string{"reality_private_key", "tls_key_file", "protocol_settings", "stream_settings"} {
					delete(v, key)
				}
			}
			return v, 200, e
		}
		limit := 250
		offset := 0
		if v := r.URL.Query().Get("limit"); v != "" {
			limit, e = strconv.Atoi(v)
			if e != nil || limit < 1 || limit > 1000 {
				return nil, 0, fail(400, "limit must be 1–1000")
			}
		}
		if v := r.URL.Query().Get("offset"); v != "" {
			offset, e = strconv.Atoi(v)
			if e != nil || offset < 0 {
				return nil, 0, fail(400, "offset must be nonnegative")
			}
		}
		filter := ""
		args := []any{limit, offset}
		if kind == "clients" && ownClients(a.User) {
			filter = "WHERE " + ownerExpression + "=$3 "
			args = append(args, num(a.User, "id"))
		}
		if kind == "inbounds" && !flag(a.User, "is_superuser") && (str(a.User, "role") == "reseller" || len(array(a.User, "allowed_inbounds")) > 0) {
			assigned := []int64{}
			for _, v := range array(a.User, "allowed_inbounds") {
				assigned = append(assigned, num(Data{"id": v}, "id"))
			}
			filter = "WHERE id=ANY($3::bigint[]) "
			args = append(args, assigned)
		}
		where := filter + "ORDER BY id"
		if kind == "audit" {
			where = "ORDER BY id DESC"
		}
		rows, e := list(ctx, s.Store.Pool, kind, where+" LIMIT $1 OFFSET $2", args...)
		if e != nil {
			return nil, 0, e
		}
		out := []Data{}
		for _, d := range rows {
			v, e := s.view(ctx, s.Store.Pool, kind, d)
			if e != nil {
				return nil, 0, e
			}
			if kind == "inbounds" && !flag(a.User, "is_superuser") && !permitted(a.User, "inbounds.write") {
				for _, key := range []string{"reality_private_key", "tls_key_file", "protocol_settings", "stream_settings"} {
					delete(v, key)
				}
			}
			out = append(out, v)
		}
		var total int64
		countFilter := strings.ReplaceAll(filter, "$3", "$1")
		e = s.Store.Pool.QueryRow(ctx, "SELECT count(*) FROM "+tables[kind]+" "+countFilter, args[2:]...).Scan(&total)
		if e != nil {
			return nil, 0, e
		}
		w.Header().Set("X-Total-Count", strconv.FormatInt(total, 10))
		w.Header().Set("X-Page-Limit", strconv.Itoa(limit))
		return out, 200, nil
	}
	if kind == "audit" {
		return nil, 0, fail(405, "audit events are read-only")
	}
	if !oneOf(r.Method, "POST", "PATCH", "PUT", "DELETE") || r.Method == "POST" && id > 0 || r.Method != "POST" && id == 0 {
		return nil, 0, fail(405, "method not allowed")
	}
	input := Data{}
	if r.Method != "DELETE" {
		input, e = readBody(w, r)
		if e != nil {
			return nil, 0, e
		}
		if e = allowed(input, editable[kind]); e != nil {
			return nil, 0, fail(400, e.Error())
		}
		if e = typed(input, defaults(kind)); e != nil {
			return nil, 0, fail(400, e.Error())
		}
	}
	old := defaults(kind)
	if id > 0 {
		old, e = get(ctx, s.Store.Pool, kind, id)
		if e != nil {
			return nil, 0, e
		}
	}
	ids := []int64{}
	if oneOf(kind, "inbounds", "outbounds", "routing_rules") {
		ids = nodeIDs(num(old, "node"), num(input, "node"))
	}
	if kind == "clients" {
		if num(old, "_account") > 0 && input["subscription_token"] != nil {
			return nil, 0, fail(409, "rotate shared subscription tokens through customers")
		}
		if num(old, "_account") > 0 && (r.Method == "DELETE" || (input["inbound"] != nil && num(input, "inbound") != num(old, "inbound"))) {
			return nil, 0, fail(409, "manage shared account membership through customers")
		}
		if num(old, "_account") > 0 {
			ids, e = clientNodes(ctx, s.Store.Pool, old)
			if e != nil {
				return nil, 0, e
			}
		}
		for _, in := range nodeIDs(num(old, "inbound"), num(input, "inbound")) {
			n, e := inboundNode(ctx, s.Store.Pool, in)
			if e != nil {
				return nil, 0, fail(400, "inbound does not exist")
			}
			ids = nodeIDs(append(ids, n)...)
		}
	}
	if kind == "nodes" && id > 0 && r.Method != "DELETE" {
		ids = nodeIDs(id)
	}
	mutation := func(tx pgx.Tx) (any, error) {
		d := defaults(kind)
		if id > 0 {
			var e error
			d, e = get(ctx, tx, kind, id)
			if e != nil {
				return nil, e
			}
		}
		if kind == "clients" && id > 0 {
			if err := clientAccess(a, d); err != nil {
				return nil, err
			}
		}
		if r.Method == "DELETE" {
			if kind == "inbounds" {
				var referenced bool
				if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vr_clients WHERE inbound_id=$1 AND account_id IS NOT NULL UNION ALL SELECT 1 FROM vr_services WHERE data->'inbound_ids' @> $2::jsonb UNION ALL SELECT 1 FROM vr_accounts WHERE data->'inbound_ids' @> $2::jsonb)`, id, fmt.Sprintf("[%d]", id)).Scan(&referenced); e != nil {
					return nil, e
				}
				if referenced {
					return nil, fail(409, "remove this inbound from services and shared customers before deleting it")
				}
			}
			if kind == "nodes" {
				if flag(d, "is_local") {
					return nil, fail(400, "The local node cannot be deleted")
				}
				var count int
				if e := tx.QueryRow(ctx, `SELECT count(*) FROM vr_inbounds WHERE node_id=$1`, id).Scan(&count); e != nil {
					return nil, e
				}
				if count > 0 {
					return nil, fail(409, "Remove the node's inbounds before deleting it")
				}
			}
			if kind == "users" {
				if id == num(a.User, "id") {
					return nil, fail(400, "You cannot delete your own account")
				}
				if e := ensureAdmin(ctx, tx, id, nil); e != nil {
					return nil, e
				}
			}
			return nil, remove(ctx, tx, kind, id)
		}
		prior := clone(d)
		d = merge(d, input)
		if kind == "users" {
			if str(d, "ldap_dn") != str(prior, "ldap_dn") || str(d, "oidc_issuer") != str(prior, "oidc_issuer") || str(d, "oidc_subject") != str(prior, "oidc_subject") {
				d["_auth_version"] = num(d, "_auth_version") + 1
			}
			if e := validateLDAPBinding(d); e != nil {
				return nil, fail(400, e.Error())
			}
			if e := validateOIDCBinding(d); e != nil {
				return nil, fail(400, e.Error())
			}
		}
		if kind == "clients" {
			reanchorRenewal(d, prior, input)
			if flag(d, "enabled") {
				d["disabled_reason"] = ""
			}
		}
		switch kind {
		case "nodes":
			if num(d, "profile_id") < 0 {
				return nil, fail(400, "invalid profile")
			}
			if num(d, "profile_id") > 0 {
				if _, e := get(ctx, tx, "profiles", num(d, "profile_id")); e != nil {
					return nil, fail(400, "profile does not exist")
				}
			}
			if token := str(input, "agent_token"); token != "" {
				if len(token) < 32 {
					return nil, fail(400, "agent_token must have at least 32 characters")
				}
				v, e := seal(s.Config.FieldKey, token)
				if e != nil {
					return nil, e
				}
				d["_agent_token"] = v
			}
			delete(d, "agent_token")
			if e := validateNode(s.Config, d); e != nil {
				return nil, fail(400, e.Error())
			}
			if flag(prior, "is_local") && (str(d, "agent_url") != str(prior, "agent_url") || !flag(d, "enabled")) {
				return nil, fail(400, "The local agent address and enabled state are managed by the installer")
			}
		case "inbounds":
			if num(d, "id") > 0 && str(d, "protocol") != str(prior, "protocol") {
				var n int
				if e := tx.QueryRow(ctx, `SELECT count(*) FROM vr_clients WHERE inbound_id=$1`, id).Scan(&n); e != nil {
					return nil, e
				}
				if n > 0 {
					return nil, fail(400, "Remove clients before changing the inbound protocol")
				}
			}
			if e := validateInbound(ctx, tx, s.Config, d); e != nil {
				return nil, fail(400, e.Error())
			}
		case "outbounds", "routing_rules":
			if e := validateNetworkEntity(ctx, tx, kind, d); e != nil {
				return nil, fail(400, e.Error())
			}
		case "clients":
			if !flag(a.User, "is_superuser") && input["owner_id"] != nil && num(input, "owner_id") != num(prior, "owner_id") {
				return nil, fail(403, "only administrators can assign client owners")
			}
			if err := validateClientOwner(ctx, tx, a, d, id == 0); err != nil {
				return nil, err
			}
			inbound, err := get(ctx, tx, "inbounds", num(d, "inbound"))
			if err != nil {
				return nil, fail(400, "inbound does not exist")
			}
			if oneOf(str(inbound, "protocol"), "wireguard", "amneziawg") {
				ps := obj(d, "protocol_settings")
				if str(ps, "private_key") == "" || str(ps, "public_key") == "" {
					node, err := get(ctx, tx, "nodes", num(inbound, "node"))
					if err != nil {
						return nil, err
					}
					pair, err := s.nodeCall(ctx, node, "/xray/wg", Data{})
					if err != nil {
						return nil, fail(502, err.Error())
					}
					ps["private_key"] = pair["private_key"]
					ps["public_key"] = pair["public_key"]
				}
				if str(ps, "address") == "" {
					peers, err := list(ctx, tx, "clients", "WHERE inbound_id=$1", num(inbound, "id"))
					if err != nil {
						return nil, err
					}
					used := map[string]bool{}
					for _, peer := range peers {
						used[fallback(str(obj(peer, "protocol_settings"), "address"), wireguardAddress(num(peer, "id")))] = true
					}
					for slot := int64(1); slot <= 64009; slot++ {
						address := wireguardAddress(slot)
						if !used[address] {
							ps["address"] = address
							break
						}
					}
					if str(ps, "address") == "" {
						return nil, fail(400, "WireGuard address pool is full")
					}
				}
				d["protocol_settings"] = ps
			}
			if e := validateClient(ctx, tx, d); e != nil {
				return nil, fail(400, e.Error())
			}
			if num(d, "_account") > 0 {
				related, e := clientNodes(ctx, tx, d)
				if e != nil {
					return nil, e
				}
				if e = ensureCovered(related, ids); e != nil {
					return nil, e
				}
				members, e := customerMembers(ctx, tx, num(d, "_account"))
				if e != nil {
					return nil, e
				}
				for _, member := range members {
					copyAccountFields(member, d)
					if e = validateClientOwner(ctx, tx, a, member, false); e != nil {
						return nil, e
					}
					if e = validateClient(ctx, tx, member); e != nil {
						return nil, fail(400, e.Error())
					}
				}
			}

		case "users":
			if err := validateAccount(ctx, tx, d, prior, input); err != nil {
				return nil, err
			}
			if e := require(d, "username", 150); e != nil {
				return nil, fail(400, e.Error())
			}
			if !flag(d, "is_staff") && flag(d, "is_superuser") {
				return nil, fail(400, "Administrators must have staff access")
			}
			password := str(input, "password")
			if id == 0 && password == "" {
				return nil, fail(400, "password is required")
			}
			if password != "" {
				if e := passwordPolicy(str(d, "username"), password); e != nil {
					return nil, fail(400, e.Error())
				}
				h, e := hashPassword(password)
				if e != nil {
					return nil, e
				}
				d["_password"] = h
				d["_auth_version"] = num(d, "_auth_version") + 1
			}
			delete(d, "password")
			if id == num(a.User, "id") && (!flag(d, "is_active") || !flag(d, "is_superuser") || !flag(d, "is_staff")) {
				return nil, fail(400, "You cannot remove your own administrator access")
			}
			if e := ensureAdmin(ctx, tx, id, d); e != nil {
				return nil, e
			}
		case "api_keys":
			if e := require(d, "name", 96); e != nil {
				return nil, fail(400, e.Error())
			}
			if !oneOf(str(d, "scope"), "read", "write", "admin") {
				return nil, fail(400, "invalid API key scope")
			}
			if id == 0 {
				token := "vr_" + randomToken(32)
				d["_token_hash"] = digest(token)
				d["prefix"] = token[:15]
				d["_new_token"] = token
				if num(d, "user") == 0 {
					d["user"] = num(a.User, "id")
				}
				if d["expires_at"] == nil {
					cfg, e := settings(ctx, tx)
					if e != nil {
						return nil, e
					}
					d["expires_at"] = stamp(time.Now().Add(time.Duration(num(cfg, "api_key_default_days")) * 24 * time.Hour))
				}
			}
			u, e := get(ctx, tx, "users", num(d, "user"))
			if e != nil || !flag(u, "is_active") || !flag(u, "is_staff") {
				return nil, fail(400, "API key owner must be an active staff user")
			}
			if d["expires_at"] != nil {
				if _, e := timestamp(d, "expires_at"); e != nil {
					return nil, fail(400, "invalid expiry")
				}
			}
		}
		token := str(d, "_new_token")
		delete(d, "_new_token")
		if e := save(ctx, tx, kind, d); e != nil {
			mapped := databaseError(e)
			var api *apiError
			if errors.As(mapped, &api) {
				return nil, mapped
			}
			return nil, fail(400, mapped.Error())
		}
		out, e := s.view(ctx, tx, kind, d)
		if e != nil {
			return nil, e
		}
		if token != "" {
			out["token"] = token
		}
		return out, nil
	}
	var out any
	if len(ids) > 0 {
		out, e = s.change(ctx, ids, a, kind+"."+strings.ToLower(r.Method), fmt.Sprint(id), mutation)
	} else {
		tx, te := s.Store.Pool.Begin(ctx)
		if te != nil {
			return nil, 0, te
		}
		defer tx.Rollback(ctx)
		if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(842100,2)`); e == nil {
			out, e = mutation(tx)
		}
		if e == nil {
			e = audit(ctx, tx, num(a.User, "id"), kind+"."+strings.ToLower(r.Method), fmt.Sprint(id), Data{})
		}
		if e == nil {
			e = tx.Commit(ctx)
		}
	}
	status := 200
	if r.Method == "POST" {
		status = 201
	}
	if r.Method == "DELETE" {
		status = 204
	}
	return out, status, e
}
func ensureAdmin(ctx context.Context, q Query, id int64, d Data) error {
	if d != nil && flag(d, "is_active") && flag(d, "is_staff") && flag(d, "is_superuser") {
		return nil
	}
	var count int
	e := q.QueryRow(ctx, `SELECT count(*) FROM vr_users WHERE id<>$1 AND data->>'is_active'='true' AND data->>'is_staff'='true' AND data->>'is_superuser'='true'`, id).Scan(&count)
	if e != nil {
		return e
	}
	if count == 0 {
		return fail(400, "At least one active administrator must remain")
	}
	return nil
}
func (s *Server) settingsAPI(w http.ResponseWriter, r *http.Request, a Actor) (any, int, error) {
	ctx := r.Context()
	if r.Method == "GET" {
		d, e := settings(ctx, s.Store.Pool)
		out := public(d)
		if !flag(a.User, "is_superuser") {
			out = Data{}
			for _, key := range []string{"site_name", "accent", "default_locale", "default_client_days", "default_traffic_gb", "default_renewal_days", "dashboard_refresh_seconds"} {
				out[key] = d[key]
			}
		}
		out["telegram_configured"] = str(d, "_telegram_token") != ""
		if flag(a.User, "is_superuser") {
			out["webhook_configured"] = str(d, "_webhook_secret") != ""
			out["smtp_configured"] = str(d, "_smtp_password") != ""
		}
		return out, 200, e
	}
	if r.Method != "PATCH" && r.Method != "PUT" {
		return nil, 0, fail(405, "method not allowed")
	}
	in, e := readBody(w, r)
	if e != nil {
		return nil, 0, e
	}
	keys := []string{"smtp_password", "smtp_clear_password", "telegram_bot_token", "telegram_clear_bot_token", "webhook_signing_secret", "webhook_clear_secret"}
	for k := range defaultSettings() {
		if !strings.HasPrefix(k, "_") {
			keys = append(keys, k)
		}
	}
	if e = allowed(in, strings.Join(keys, " ")); e != nil {
		return nil, 0, fail(400, e.Error())
	}
	if e = typed(in, defaultSettings()); e != nil {
		return nil, 0, fail(400, e.Error())
	}
	tx, e := s.Store.Pool.Begin(ctx)
	if e != nil {
		return nil, 0, e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT id FROM vr_settings WHERE id=1 FOR UPDATE`); e != nil {
		return nil, 0, e
	}
	d, e := settings(ctx, tx)
	if e != nil {
		return nil, 0, e
	}
	d = merge(d, in)
	if in["subscription_hosts"] != nil {
		for _, value := range array(d, "subscription_hosts") {
			if _, err := get(ctx, tx, "inbounds", num(asData(value), "inbound")); err != nil {
				return nil, 0, fail(400, "subscription host inbound does not exist")
			}
		}
	}
	if token := str(in, "telegram_bot_token"); token != "" {
		if len(token) > 256 || strings.ContainsAny(token, "\r\n /?") {
			return nil, 0, fail(400, "invalid bot token")
		}
		d["_telegram_token"], e = seal(s.Config.FieldKey, token)
		if e != nil {
			return nil, 0, e
		}
	}
	if flag(in, "telegram_clear_bot_token") {
		d["_telegram_token"] = ""
	}
	delete(d, "telegram_bot_token")
	delete(d, "telegram_clear_bot_token")
	if secret := str(in, "webhook_signing_secret"); secret != "" {
		if len(secret) < 32 || len(secret) > 1024 {
			return nil, 0, fail(400, "webhook signing secret must contain 32–1024 characters")
		}
		d["_webhook_secret"], e = seal(s.Config.FieldKey, secret)
		if e != nil {
			return nil, 0, e
		}
	}
	if flag(in, "webhook_clear_secret") {
		d["_webhook_secret"] = ""
	}
	delete(d, "webhook_signing_secret")
	delete(d, "webhook_clear_secret")
	if password := str(in, "smtp_password"); password != "" {
		if len(password) > 4096 {
			return nil, 0, fail(400, "SMTP password is too long")
		}
		d["_smtp_password"], e = seal(s.Config.FieldKey, password)
		if e != nil {
			return nil, 0, e
		}
	}
	if flag(in, "smtp_clear_password") {
		d["_smtp_password"] = ""
	}
	delete(d, "smtp_password")
	delete(d, "smtp_clear_password")
	if e = validateSettings(d); e != nil {
		return nil, 0, fail(400, e.Error())
	}
	if _, changed := in["subscription_template_path"]; changed || in["subscription_template"] != nil {
		if str(d, "subscription_template") == "custom" {
			if _, err := s.loadPortalTemplate(str(d, "subscription_template_path")); err != nil {
				return nil, 0, fail(400, err.Error())
			}
		}
	}
	if e = saveSettings(ctx, tx, d); e != nil {
		return nil, 0, e
	}
	if e = audit(ctx, tx, num(a.User, "id"), "settings.update", "settings", Data{}); e != nil {
		return nil, 0, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, 0, e
	}
	out := public(d)
	out["telegram_configured"] = str(d, "_telegram_token") != ""
	out["webhook_configured"] = str(d, "_webhook_secret") != ""
	out["smtp_configured"] = str(d, "_smtp_password") != ""
	return out, 200, nil
}
func (s *Server) action(w http.ResponseWriter, r *http.Request, kind string, id int64, action string, a Actor) (any, int, error) {
	ctx := r.Context()
	d, e := get(ctx, s.Store.Pool, kind, id)
	if e != nil {
		return nil, 0, e
	}
	if kind == "nodes" {
		switch action {
		case "ports":
			if r.Method != "GET" {
				return nil, 0, fail(405, "method not allowed")
			}
			cfg, err := s.buildConfig(ctx, s.Store.Pool, d)
			if err != nil {
				return nil, 0, err
			}

			out, err := s.nodeCall(ctx, d, "/xray/ports", Data{"config": cfg})
			if err != nil {
				return nil, 0, fail(502, err.Error())
			}
			return out, 200, nil
		case "config-preview":
			if r.Method != "GET" {
				return nil, 0, fail(405, "method not allowed")
			}
			cfg, e := s.buildConfig(ctx, s.Store.Pool, d)
			return Data{"node": str(d, "name"), "config": cfg}, 200, e
		case "singbox-status":
			if r.Method != "GET" {
				return nil, 0, fail(405, "method not allowed")
			}
			out, err := s.nodeCall(ctx, d, "/singbox/status", nil)
			return out, 200, err
		case "xray-status":
			if r.Method != "GET" {
				return nil, 0, fail(405, "method not allowed")
			}
			out, e := s.nodeCall(ctx, d, "/xray/status", nil)
			if e != nil {
				return nil, 0, fail(502, e.Error())
			}
			return out, 200, nil
		}
		if r.Method != "POST" {
			return nil, 0, fail(405, "method not allowed")
		}
		switch action {
		case "sni-scan":
			input, err := readBody(w, r)
			if err != nil {
				return nil, 0, err
			}
			if err = allowed(input, "domains"); err != nil {
				return nil, 0, fail(400, err.Error())
			}
			out, err := s.nodeCall(ctx, d, "/sni/scan", input)
			if err != nil {
				return nil, 0, fail(502, err.Error())
			}
			return out, 200, nil
		case "singbox-restart":
			out, err := s.nodeCall(ctx, d, "/singbox/restart", Data{})
			return out, 200, err
		case "check-listener":
			input, err := readBody(w, r)
			if err != nil {
				return nil, 0, err
			}
			if err = allowed(input, editable["inbounds"]+" id"); err != nil {
				return nil, 0, fail(400, err.Error())
			}
			if err = typed(input, defaults("inbounds")); err != nil {
				return nil, 0, fail(400, err.Error())
			}
			candidate := defaults("inbounds")
			for k, v := range input {
				candidate[k] = v
			}
			candidate["node"] = id
			port := num(candidate, "port")
			if port < 1 || port > 65535 || net.ParseIP(str(candidate, "listen")) == nil || !oneOf(str(candidate, "protocol"), "vless", "vmess", "trojan", "shadowsocks", "hysteria", "wireguard", "http", "socks", "tunnel", "tun", "tuic", "naive", "snell", "shadowtls", "mtproto", "amneziawg") || !oneOf(str(candidate, "transport"), "raw", "ws", "grpc", "xhttp", "httpupgrade", "mkcp", "hysteria") {
				return nil, 0, fail(400, "valid protocol, transport, IP listen address and port are required")
			}
			if port == 10085 || port == 9191 || flag(d, "is_local") && (port == 8610 || port == int64(s.Config.PanelPort)) {
				return nil, 0, fail(400, "port is reserved by VeloRay")
			}
			stream := streamSettings(candidate, d)
			protocol := str(candidate, "protocol")
			if protocol == "tunnel" {
				protocol = "dokodemo-door"
			}
			cfg := Data{"inbounds": []any{Data{"tag": "candidate", "listen": candidate["listen"], "port": candidate["port"], "protocol": protocol, "settings": obj(candidate, "protocol_settings"), "streamSettings": stream}}}
			if oneOf(protocol, "mtproto", "amneziawg") {
				entry := Data{"tag": "candidate", "listen": candidate["listen"], "port": candidate["port"], "protocol": protocol, "users": []any{}}
				if protocol == "amneziawg" {
					params, err := panelAWGSettings(obj(candidate, "protocol_settings"))
					if err != nil {
						return nil, 0, fail(400, err.Error())
					}
					entry["awg"] = params
				}
				cfg = Data{"inbounds": []any{}, "_native": Data{"inbounds": []any{entry}}}
			}
			out, err := s.nodeCall(ctx, d, "/xray/ports", Data{"config": cfg})
			if err != nil {
				return nil, 0, fail(502, err.Error())
			}
			// A running Xray may own the port, but another saved inbound still reserves it.
			others, err := list(ctx, s.Store.Pool, "inbounds", "WHERE node_id=$1", id)
			if err != nil {
				return nil, 0, err
			}
			for _, entry := range array(out, "ports") {
				check, ok := entry.(map[string]any)
				if !ok {
					continue
				}
				for _, other := range others {
					if num(other, "id") != num(input, "id") && flag(other, "enabled") && num(other, "port") == num(candidate, "port") {
						check["state"] = "conflict"
						check["owner"] = "saved inbound " + str(other, "name")
					}
				}
			}
			return out, 200, nil
		case "probe":
			out, e := s.probe(ctx, id)
			if e != nil {
				return nil, 0, fail(502, e.Error())
			}
			_ = audit(ctx, s.Store.Pool, num(a.User, "id"), "node.probe", str(d, "name"), Data{})
			return out, 200, nil
		case "deploy":
			out, e := s.change(ctx, []int64{id}, a, "node.deploy", str(d, "name"), func(pgx.Tx) (any, error) { return Data{"status": "applied"}, nil })
			return out, 200, e
		case "config-validate":
			cfg, err := s.buildConfig(ctx, s.Store.Pool, d)
			if err != nil {
				return nil, 0, err
			}
			out, err := s.nodeCall(ctx, d, "/xray/validate", Data{"config": cfg})
			if err != nil {
				return nil, 0, fail(502, err.Error())
			}
			return out, 200, nil
		case "xray-restart":
			tx, e := s.Store.Pool.Begin(ctx)
			if e != nil {
				return nil, 0, e
			}
			defer tx.Rollback(ctx)
			if e = lockNodes(ctx, tx, id); e != nil {
				return nil, 0, e
			}
			d, e = get(ctx, tx, "nodes", id)
			if e != nil {
				return nil, 0, e
			}
			st, err := s.nodeCall(ctx, d, "/xray/status", nil)
			if err != nil {
				return nil, 0, fail(502, err.Error())
			}
			if flag(st, "running") {
				if e = s.collect(ctx, tx, d); e != nil {
					return nil, 0, fail(502, e.Error())
				}
			}
			out, e := s.nodeCall(ctx, d, "/xray/restart", Data{})
			if e != nil {
				return nil, 0, fail(502, e.Error())
			}
			if e = audit(ctx, tx, num(a.User, "id"), "node.restart", str(d, "name"), Data{}); e != nil {
				return nil, 0, e
			}
			return out, 200, tx.Commit(ctx)
		case "reality-keypair", "wireguard-keypair":
			path := "/xray/x25519"
			if action == "wireguard-keypair" {
				path = "/xray/wg"
			}
			out, e := s.nodeCall(ctx, d, path, Data{})
			if e != nil {
				return nil, 0, fail(502, e.Error())
			}
			return out, 200, nil
		}
	}
	if kind == "clients" && oneOf(action, "reset-usage", "toggle") && r.Method == "POST" {
		ids, e := clientNodes(ctx, s.Store.Pool, d)
		if e != nil {
			return nil, 0, e
		}
		out, e := s.change(ctx, ids, a, "client."+action, str(d, "name"), func(tx pgx.Tx) (any, error) {
			c, e := get(ctx, tx, "clients", id)
			if e == nil {
				e = clientAccess(a, c)
			}
			if e != nil {
				return nil, e
			}
			related, err := clientNodes(ctx, tx, c)
			if err != nil {
				return nil, err
			}
			if err = ensureCovered(related, ids); err != nil {
				return nil, err
			}
			operation := action
			if action == "toggle" {
				operation = "enable"
				if flag(c, "enabled") {
					operation = "disable"
				}
			}
			if e = applyClientAction(c, operation); e != nil {
				return nil, e
			}
			if e = save(ctx, tx, "clients", c); e != nil {
				return nil, e
			}
			return s.view(ctx, tx, "clients", c)
		})
		return out, 200, e
	}
	if kind == "inbounds" && action == "clone" && r.Method == "POST" {
		in, e := readBody(w, r)
		if e != nil {
			return nil, 0, e
		}
		copy := clone(d)
		for _, k := range []string{"id", "created_at", "updated_at"} {
			delete(copy, k)
		}
		for _, k := range []string{"node", "port", "name"} {
			if v, ok := in[k]; ok {
				copy[k] = v
			}
		}
		if e = validateInbound(ctx, s.Store.Pool, s.Config, copy); e != nil {
			return nil, 0, fail(400, e.Error())
		}
		out, e := s.change(ctx, []int64{num(copy, "node")}, a, "inbound.clone", str(copy, "name"), func(tx pgx.Tx) (any, error) {
			if e := save(ctx, tx, "inbounds", copy); e != nil {
				return nil, e
			}
			return s.view(ctx, tx, "inbounds", copy)
		})
		return out, 201, e
	}
	if kind == "users" && action == "reset-password" && r.Method == "POST" {
		in, e := readBody(w, r)
		if e != nil {
			return nil, 0, e
		}
		e = s.Store.AdminReset(ctx, str(d, "username"), str(in, "password"))
		if e != nil {
			return nil, 0, fail(400, e.Error())
		}
		return Data{"status": "updated"}, 200, nil
	}
	return nil, 0, fail(404, "action not found")
}
func (s *Server) overview(ctx context.Context) (Data, error) {
	nodes, e := list(ctx, s.Store.Pool, "nodes", "")
	if e != nil {
		return nil, e
	}
	clients, e := list(ctx, s.Store.Pool, "clients", "")
	if e != nil {
		return nil, e
	}
	inbounds, e := list(ctx, s.Store.Pool, "inbounds", "")
	if e != nil {
		return nil, e
	}
	out := Data{"nodes_total": len(nodes), "nodes_online": 0, "clients_total": len(clients), "clients_active": 0, "clients_expiring_7d": 0, "traffic_used_bytes": int64(0), "traffic_upload_bytes": int64(0), "traffic_download_bytes": int64(0), "traffic_limit_bytes": int64(0), "inbounds_total": len(inbounds)}
	for _, n := range nodes {
		if flag(n, "enabled") && str(n, "status") == "ok" {
			out["nodes_online"] = num(out, "nodes_online") + 1
		}
	}
	seenAccounts := map[string]bool{}
	for _, c := range clients {
		if active(c, time.Now()) {
			out["clients_active"] = num(out, "clients_active") + 1
			if t, e := timestamp(c, "expires_at"); e == nil && time.Until(t) < 7*24*time.Hour {
				out["clients_expiring_7d"] = num(out, "clients_expiring_7d") + 1
			}
		}
		if seenAccounts[accountKey(c)] {
			continue
		}
		seenAccounts[accountKey(c)] = true
		out["traffic_used_bytes"] = num(out, "traffic_used_bytes") + num(c, "used_traffic_bytes")
		out["traffic_raw_bytes"] = num(out, "traffic_raw_bytes") + num(c, "raw_used_traffic_bytes")
		out["traffic_upload_bytes"] = num(out, "traffic_upload_bytes") + num(c, "raw_upload_traffic_bytes")
		out["traffic_download_bytes"] = num(out, "traffic_download_bytes") + num(c, "raw_download_traffic_bytes")
		out["traffic_limit_bytes"] = num(out, "traffic_limit_bytes") + num(c, "traffic_limit_bytes")
	}
	counts := map[string]int{}
	for _, i := range inbounds {
		counts[str(i, "protocol")]++
	}
	protocols := []any{}
	for p, n := range counts {
		protocols = append(protocols, Data{"protocol": p, "count": n})
	}
	out["protocols"] = protocols
	return out, nil
}
func (s *Server) trafficHistory(ctx context.Context, r *http.Request) ([]Data, error) {
	hours := 24
	if v := r.URL.Query().Get("hours"); v != "" {
		n, e := strconv.Atoi(v)
		if e != nil || n < 1 || n > 8760 {
			return nil, fail(400, "hours must be 1–8760")
		}
		hours = n
	}
	rows, e := s.Store.Pool.Query(ctx, `SELECT at,sum(bytes)::bigint,sum(upload_bytes)::bigint,sum(download_bytes)::bigint FROM vr_traffic_windows WHERE at>now()-$1*interval '1 hour' GROUP BY at ORDER BY at`, hours)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Data{}
	var cumulative, upload, download int64
	for rows.Next() {
		var at time.Time
		var b, up, down int64
		if e = rows.Scan(&at, &b, &up, &down); e != nil {
			return nil, e
		}
		cumulative += b
		upload += up
		download += down
		out = append(out, Data{"at": stamp(at), "bytes": b, "used_traffic_bytes": cumulative, "upload_bytes": up, "download_bytes": down, "raw_upload_traffic_bytes": upload, "raw_download_traffic_bytes": download, "interval_seconds": 300, "active_clients": 0, "online_nodes": 0})
	}
	return out, rows.Err()
}

func (s *Server) batchClients(w http.ResponseWriter, r *http.Request, a Actor) (any, int, error) {
	if r.Method != "POST" {
		return nil, 0, fail(405, "method not allowed")
	}
	input, err := readBody(w, r)
	if err != nil {
		return nil, 0, err
	}
	if err = allowed(input, "owner_id name inbound count expires_at traffic_limit_bytes traffic_multiplier renewal_interval_days note hwid_limit device_limit start_after_first_use duration_days"); err != nil {
		return nil, 0, fail(400, err.Error())
	}
	template := defaults("clients")
	template["count"] = 0
	if err = typed(input, template); err != nil {
		return nil, 0, fail(400, err.Error())
	}
	count := num(input, "count")
	if count < 1 || count > 100 || len(strings.TrimSpace(str(input, "name"))) == 0 || len(str(input, "name")) > 72 {
		return nil, 0, fail(400, "provide a name prefix (1–72 bytes) and a count between 1 and 100")
	}
	ctx := r.Context()
	in, err := get(ctx, s.Store.Pool, "inbounds", num(input, "inbound"))
	if err != nil {
		return nil, 0, err
	}
	if !supportsClients(str(in, "protocol")) || str(in, "protocol") == "shadowsocks" {
		return nil, 0, fail(400, "this protocol does not support batch client creation")
	}
	out, err := s.change(ctx, []int64{num(in, "node")}, a, "clients.batch-create", str(input, "name"), func(tx pgx.Tx) (any, error) {
		created := []Data{}
		for index := int64(1); index <= count; index++ {
			d := defaults("clients")
			for k, v := range input {
				if k != "count" {
					d[k] = v
				}
			}
			if !flag(a.User, "is_superuser") && num(input, "owner_id") != 0 && num(input, "owner_id") != num(a.User, "id") {
				return nil, fail(403, "only administrators can assign client owners")
			}
			if err := validateClientOwner(ctx, tx, a, d, true); err != nil {
				return nil, err
			}
			d["name"] = fmt.Sprintf("%s-%03d", strings.TrimSpace(str(input, "name")), index)
			if err := validateClient(ctx, tx, d); err != nil {
				return nil, fail(400, err.Error())
			}
			if err := save(ctx, tx, "clients", d); err != nil {
				return nil, err
			}
			v, err := s.view(ctx, tx, "clients", d)
			if err != nil {
				return nil, err
			}
			created = append(created, v)
		}
		return Data{"clients": created, "count": count}, nil
	})
	return out, 201, err
}

func applyClientAction(c Data, action string) error {
	switch action {
	case "enable":
		c["enabled"] = true
		if !active(c, time.Now()) {
			return fail(400, "renew expiry and reset exhausted usage before enabling "+str(c, "name"))
		}
		c["disabled_reason"] = ""
	case "disable":
		c["enabled"] = false
		c["disabled_reason"] = "manual"
	case "reset", "reset-usage":
		resetTraffic(c)
		if str(c, "disabled_reason") == "quota" {
			c["enabled"] = true
			c["disabled_reason"] = ""
			if !active(c, time.Now()) {
				c["enabled"] = false
				c["disabled_reason"] = "expired"
			}
		}
	}
	return nil
}

func (s *Server) bulkClients(w http.ResponseWriter, r *http.Request, a Actor) (any, int, error) {
	if r.Method != "POST" {
		return nil, 0, fail(405, "method not allowed")
	}
	input, err := readBody(w, r)
	if err != nil {
		return nil, 0, err
	}
	if err = allowed(input, "ids action"); err != nil {
		return nil, 0, fail(400, err.Error())
	}
	action := str(input, "action")
	if action == "delete" && !permitted(a.User, "clients.delete") {
		return nil, 0, fail(403, "permission required: clients.delete")
	}
	values := array(input, "ids")
	if !oneOf(action, "enable", "disable", "reset", "delete") || len(values) < 1 || len(values) > 200 {
		return nil, 0, fail(400, "choose enable, disable, reset or delete and 1–200 client IDs")
	}
	ctx := r.Context()
	ids := []int64{}
	nodes := []int64{}
	seen := map[int64]bool{}
	for _, value := range values {
		number, ok := value.(json.Number)
		if !ok {
			return nil, 0, fail(400, "client IDs must be integers")
		}
		id, err := number.Int64()
		if err != nil || id < 1 || seen[id] {
			return nil, 0, fail(400, "client IDs must be positive and unique")
		}
		seen[id] = true
		client, err := get(ctx, s.Store.Pool, "clients", id)
		if err == nil {
			err = clientAccess(a, client)
		}
		if err != nil {
			return nil, 0, err
		}
		related, err := clientNodes(ctx, s.Store.Pool, client)
		if err != nil {
			return nil, 0, err
		}
		if action == "delete" && num(client, "_account") > 0 {
			return nil, 0, fail(409, "delete shared accounts through customers")
		}
		ids = append(ids, id)
		nodes = append(nodes, related...)
	}
	nodes = nodeIDs(nodes...)
	out, err := s.change(ctx, nodes, a, "clients.bulk-"+action, fmt.Sprintf("%d accounts", len(ids)), func(tx pgx.Tx) (any, error) {
		principals := map[string]bool{}
		for _, id := range ids {
			client, err := get(ctx, tx, "clients", id)
			if err == nil {
				err = clientAccess(a, client)
			}
			if err != nil {
				return nil, err
			}
			related, err := clientNodes(ctx, tx, client)
			if err != nil {
				return nil, err
			}
			if err = ensureCovered(related, nodes); err != nil {
				return nil, err
			}
			if principals[accountKey(client)] {
				continue
			}
			principals[accountKey(client)] = true
			if action == "delete" && num(client, "_account") > 0 {
				return nil, fail(409, "delete shared accounts through customers")
			}

			if action == "delete" {
				if _, err = tx.Exec(ctx, `DELETE FROM vr_clients WHERE id=$1`, id); err != nil {
					return nil, err
				}
			} else {
				if err = applyClientAction(client, action); err != nil {
					return nil, err
				}
				if err = save(ctx, tx, "clients", client); err != nil {
					return nil, err
				}
			}
		}
		return Data{"count": len(ids), "status": "applied"}, nil
	})
	return out, 200, err
}

// LocalInbounds lists only operational fields for the root management console.
func (s *Server) LocalInbounds(ctx context.Context) ([]Data, error) {
	rows, err := list(ctx, s.Store.Pool, "inbounds", "ORDER BY id")
	if err != nil {
		return nil, err
	}
	out := []Data{}
	for _, row := range rows {
		node, err := get(ctx, s.Store.Pool, "nodes", num(row, "node"))
		if err != nil {
			return nil, err
		}
		if !flag(node, "is_local") {
			continue
		}
		view := Data{}
		for _, key := range []string{"id", "name", "listen", "port", "protocol", "enabled"} {
			view[key] = row[key]
		}
		out = append(out, view)
	}
	return out, nil
}

// ChangeLocalInboundPort keeps the database, runtime and subscription output in one deployment.
func (s *Server) ChangeLocalInboundPort(ctx context.Context, id int64, port int) (any, error) {
	row, err := get(ctx, s.Store.Pool, "inbounds", id)
	if err != nil {
		return nil, err
	}
	node, err := get(ctx, s.Store.Pool, "nodes", num(row, "node"))
	if err != nil {
		return nil, err
	}
	if !flag(node, "is_local") {
		return nil, errors.New("use the web panel to change a remote inbound")
	}
	return s.change(ctx, []int64{num(node, "id")}, Actor{}, "host.inbound-port", str(row, "name"), func(tx pgx.Tx) (any, error) {
		current, err := get(ctx, tx, "inbounds", id)
		if err != nil {
			return nil, err
		}
		if num(current, "node") != num(node, "id") {
			return nil, errors.New("inbound moved; retry")
		}
		current["port"] = port
		if err = validateInbound(ctx, tx, s.Config, current); err != nil {
			return nil, err
		}
		if err = save(ctx, tx, "inbounds", current); err != nil {
			return nil, err
		}
		return Data{"id": id, "name": current["name"], "port": port, "status": "applied"}, nil
	})
}

var _ = json.Marshal
var _ = errors.Is
