package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var permissionCatalog = []string{"resellers.read", "resellers.write", "commerce.read", "commerce.write", "dashboard.read", "clients.read", "clients.write", "clients.delete", "inbounds.read", "inbounds.write", "nodes.read", "nodes.write", "traffic.read", "xray.read", "xray.write", "settings.read", "settings.write", "api.read", "outbounds.read", "outbounds.write", "routing_rules.read", "routing_rules.write"}

func accountRole(u Data) string {
	if flag(u, "is_superuser") {
		return "administrator"
	}
	return fallback(str(u, "role"), "custom")
}
func permissions(u Data) []string {
	if flag(u, "is_superuser") {
		return append([]string{}, permissionCatalog...)
	}
	switch str(u, "role") {
	case "reseller":
		out := []string{"resellers.read", "commerce.read", "commerce.write", "dashboard.read", "clients.read", "clients.write", "clients.delete", "traffic.read", "settings.read"}
		if flag(u, "can_delegate") && num(u, "max_subresellers") > 0 {
			out = append(out, "resellers.write")
		}
		return out
	case "viewer":
		return []string{"dashboard.read", "clients.read", "inbounds.read", "nodes.read", "traffic.read", "xray.read", "settings.read", "api.read", "outbounds.read", "routing_rules.read"}
	case "custom":
		out := []string{}
		for _, p := range array(u, "permissions") {
			if value, ok := p.(string); ok {
				out = append(out, value)
			}
		}
		return out
	default:
		return append([]string{}, permissionCatalog...) // Existing staff retain their access.
	}
}
func permitted(u Data, p string) bool {
	for _, v := range permissions(u) {
		if v == p {
			return true
		}
	}
	return false
}
func ownClients(u Data) bool {
	return !flag(u, "is_superuser") && (str(u, "role") == "reseller" || flag(u, "own_clients_only"))
}
func inboundAllowed(u Data, id int64) bool {
	if flag(u, "is_superuser") {
		return true
	}
	values := array(u, "allowed_inbounds")
	if len(values) == 0 && str(u, "role") != "reseller" {
		return true
	}
	for _, value := range values {
		if num(Data{"id": value}, "id") == id {
			return true
		}
	}
	return false
}

const ownerExpression = "COALESCE((SELECT data->>'owner_id' FROM vr_accounts WHERE id=vr_clients.account_id),vr_clients.data->>'owner_id','0')::bigint"

func actorClientWhere(a Actor) (string, []any) {
	if ownClients(a.User) {
		return "WHERE " + ownerExpression + "=$1", []any{num(a.User, "id")}
	}
	return "", nil
}
func clientAccess(a Actor, c Data) error {
	if ownClients(a.User) && num(c, "owner_id") != num(a.User, "id") {
		return fail(404, "client not found")
	}
	return nil
}
func (s *Server) entityAccess(ctx context.Context, a Actor, kind string, id int64) error {
	if id == 0 {
		return nil
	}
	if kind == "clients" {
		c, e := get(ctx, s.Store.Pool, kind, id)
		if e != nil {
			return e
		}
		return clientAccess(a, c)
	}
	if kind == "inbounds" && !inboundAllowed(a.User, id) {
		return fail(404, "inbound not found")
	}
	return nil
}
func validateAccount(ctx context.Context, q Query, d, old Data, input Data) error {
	if _, present := input["role"]; present {
		if !oneOf(str(d, "role"), "administrator", "reseller", "viewer", "custom") {
			return fail(400, "invalid account role")
		}
		d["is_superuser"] = str(d, "role") == "administrator"
		d["is_staff"] = true
	} else if flag(d, "is_superuser") {
		d["role"] = "administrator"
	}
	if str(d, "role") == "reseller" {
		d["own_clients_only"] = true
	}
	seen := map[string]bool{}
	clean := []any{}
	for _, value := range array(d, "permissions") {
		p, ok := value.(string)
		valid := false
		for _, v := range permissionCatalog {
			if p == v {
				valid = true
			}
		}
		if !ok || !valid {
			return fail(400, "invalid account permission")
		}
		if !seen[p] {
			clean = append(clean, p)
			seen[p] = true
		}
	}
	d["permissions"] = clean
	for _, value := range array(d, "allowed_inbounds") {
		id := num(Data{"id": value}, "id")
		if id < 1 {
			return fail(400, "invalid assigned inbound")
		}
		if _, e := get(ctx, q, "inbounds", id); e != nil {
			return fail(400, "assigned inbound does not exist")
		}
	}
	if num(d, "max_clients") < 0 || num(d, "traffic_budget_bytes") < 0 {
		return fail(400, "account limits cannot be negative")
	}
	if e := validateHierarchyAccount(ctx, q, d, old); e != nil {
		return e
	}
	for _, key := range []string{"can_delegate", "max_subresellers", "role", "permissions", "own_clients_only", "allowed_inbounds", "is_active", "is_staff", "is_superuser"} {
		before, _ := json.Marshal(old[key])
		after, _ := json.Marshal(d[key])
		if string(before) != string(after) {
			d["_auth_version"] = num(d, "_auth_version") + 1
			break
		}
	}
	if num(old, "id") > 0 && num(d, "_auth_version") != num(old, "_auth_version") {
		if _, e := q.Exec(ctx, `WITH RECURSIVE children(id,depth) AS (SELECT id,1 FROM vr_users WHERE COALESCE((data->>'parent_id')::bigint,0)=$1 UNION ALL SELECT u.id,c.depth+1 FROM vr_users u JOIN children c ON COALESCE((u.data->>'parent_id')::bigint,0)=c.id WHERE c.depth<8) UPDATE vr_users SET data=jsonb_set(data,'{_auth_version}',to_jsonb(COALESCE((data->>'_auth_version')::bigint,1)+1)) WHERE id IN(SELECT id FROM children)`, num(old, "id")); e != nil {
			return e
		}
	}
	return nil
}
func validateClientOwner(ctx context.Context, q Query, a Actor, c Data, isNew bool) error {
	if ownClients(a.User) {
		c["owner_id"] = num(a.User, "id")
	} else if isNew && num(c, "owner_id") == 0 {
		c["owner_id"] = num(a.User, "id")
	}
	if !inboundAllowed(a.User, num(c, "inbound")) {
		return fail(403, "inbound is not assigned to your account")
	}
	ownerID := num(c, "owner_id")
	if ownerID == 0 {
		return nil
	}
	owner, e := get(ctx, q, "users", ownerID)
	if e != nil || !flag(owner, "is_staff") {
		return fail(400, "client owner must be a panel account")
	}
	if !inboundAllowed(owner, num(c, "inbound")) {
		return fail(400, "inbound is not assigned to the client owner")
	}
	if num(owner, "max_clients") == 0 && num(owner, "traffic_budget_bytes") == 0 {
		return nil
	}
	reservedClients, reservedTraffic, e := capacityReservations(ctx, q, ownerID, 0)
	if e != nil {
		return e
	}
	rows, e := list(ctx, q, "clients", "WHERE "+ownerExpression+"=$1", ownerID)
	if e != nil {
		return e
	}
	count := int64(0)
	allocated := int64(0)
	seen := map[string]bool{}
	for _, existing := range rows {
		if accountKey(existing) == accountKey(c) && !isNew {
			continue
		}
		key := accountKey(existing)
		if seen[key] {
			continue
		}
		seen[key] = true
		count++
		quota := num(existing, "traffic_limit_bytes")
		if num(owner, "traffic_budget_bytes") > 0 && quota <= 0 {
			return fail(400, "a limited reseller account requires finite client quotas")
		}
		if quota > 0 {
			if quota > int64(^uint64(0)>>1)-allocated {
				return fail(400, "allocated traffic overflow")
			}
			allocated += quota
		}
	}
	if max := num(owner, "max_clients"); max > 0 && (reservedClients > max || count+1 > max-reservedClients) {
		return fail(400, "account client limit reached")
	}
	if budget := num(owner, "traffic_budget_bytes"); budget > 0 && (num(c, "traffic_limit_bytes") <= 0 || reservedTraffic < 0 || reservedTraffic > budget || allocated > budget-reservedTraffic || num(c, "traffic_limit_bytes") > budget-reservedTraffic-allocated) {
		return fail(400, "account traffic budget exceeded")
	}
	return nil
}
func (s *Server) roleAuthorize(a Actor, method, path string) error {
	if strings.HasPrefix(path, "auth/") {
		return nil
	}
	if path == "settings" && method == http.MethodGet {
		return nil
	} // Only safe defaults are exposed to restricted accounts.
	resource := strings.Split(path, "/")[0]
	switch resource {
	case "cluster":
		if !commerceAdmin(a) {
			return fail(403, "administrator access required")
		}
		return nil
	case "node-enrollment":
		if !commerceAdmin(a) {
			return fail(403, "administrator access required")
		}
		return nil
	case "commerce":
		if method == http.MethodGet && permitted(a.User, "commerce.read") {
			return nil
		}
		if method != http.MethodGet && permitted(a.User, "commerce.write") {
			return nil
		}
		return fail(403, "commerce access is not assigned to this account")
	case "customers":
		resource = "clients"
	case "services":
		if method == http.MethodGet {
			resource = "clients"
		} else if !flag(a.User, "is_superuser") || a.Scope != "admin" {
			return fail(403, "administrator access required")
		} else {
			return nil
		}
	case "metrics":
		if method != http.MethodGet || !flag(a.User, "is_superuser") {
			return fail(403, "administrator read access required")
		}
		return nil
	case "project":
		if method == http.MethodGet {
			return nil
		}
		return fail(405, "method not allowed")
	case "openapi":
		resource = "api"
	case "overview", "events":
		resource = "dashboard"
	case "traffic-history":
		resource = "traffic"
	case "outbound-traffic":
		if !permitted(a.User, "nodes.read") {
			return fail(403, "node access is required for outbound traffic")
		}
		resource = "traffic"
	case "system-info":
		resource = "nodes"
	case "operations", "monitors", "profiles", "snippets", "audit", "admin", "imports", "telegram", "webhooks", "updates", "subscription-endpoint", "subscription-template", "subscription-policies":
		if !flag(a.User, "is_superuser") || a.Scope != "admin" {
			return fail(403, "administrator access required")
		}
		return nil
	}
	operation := "read"
	if method != http.MethodGet {
		operation = "write"
	}
	if method == http.MethodDelete {
		operation = "delete"
		if resource != "clients" {
			operation = "write"
		}
	}
	if resource == "nodes" && (strings.Contains(path, "/xray-") || strings.Contains(path, "/singbox-")) {
		resource = "xray"
	}
	if !permitted(a.User, resource+"."+operation) {
		return fail(403, fmt.Sprintf("permission required: %s.%s", resource, operation))
	}
	return nil
}

func (s *Server) overviewForActor(ctx context.Context, a Actor) (Data, error) {
	if !ownClients(a.User) {
		return s.overview(ctx)
	}
	where, args := actorClientWhere(a)
	clients, e := list(ctx, s.Store.Pool, "clients", where, args...)
	if e != nil {
		return nil, e
	}
	out := Data{"nodes_total": 0, "nodes_online": 0, "clients_total": 0, "clients_active": 0, "clients_expiring_7d": 0, "traffic_used_bytes": int64(0), "traffic_upload_bytes": int64(0), "traffic_download_bytes": int64(0), "traffic_raw_bytes": int64(0), "traffic_limit_bytes": int64(0), "inbounds_total": len(array(a.User, "allowed_inbounds")), "protocols": []any{}}
	seen := map[string]bool{}
	for _, c := range clients {
		key := accountKey(c)
		if seen[key] {
			continue
		}
		seen[key] = true
		out["clients_total"] = num(out, "clients_total") + 1
		if active(c, time.Now()) {
			out["clients_active"] = num(out, "clients_active") + 1
			if expires, err := timestamp(c, "expires_at"); err == nil && time.Until(expires) < 7*24*time.Hour {
				out["clients_expiring_7d"] = num(out, "clients_expiring_7d") + 1
			}
		}
		for output, field := range map[string]string{"traffic_used_bytes": "used_traffic_bytes", "traffic_raw_bytes": "raw_used_traffic_bytes", "traffic_upload_bytes": "raw_upload_traffic_bytes", "traffic_download_bytes": "raw_download_traffic_bytes", "traffic_limit_bytes": "traffic_limit_bytes"} {
			out[output] = num(out, output) + num(c, field)
		}
	}
	_, allocated, e := ownAllocation(ctx, s.Store.Pool, num(a.User, "id"))
	if e != nil {
		return nil, e
	}
	reservedClients, reservedTraffic, e := capacityReservations(ctx, s.Store.Pool, num(a.User, "id"), 0)
	if e != nil {
		return nil, e
	}
	out["max_clients"], out["traffic_budget_bytes"] = num(a.User, "max_clients"), num(a.User, "traffic_budget_bytes")
	out["allocated_traffic_bytes"], out["reserved_clients"], out["reserved_traffic_bytes"] = allocated, reservedClients, reservedTraffic
	return out, nil
}
func (s *Server) trafficHistoryForActor(ctx context.Context, r *http.Request, a Actor) ([]Data, error) {
	if !ownClients(a.User) {
		return s.trafficHistory(ctx, r)
	}
	hours := 24
	if value := r.URL.Query().Get("hours"); value != "" {
		n, e := strconv.Atoi(value)
		if e != nil || n < 1 || n > 8760 {
			return nil, fail(400, "hours must be 1–8760")
		}
		hours = n
	}
	rows, e := s.Store.Pool.Query(ctx, `SELECT at,bytes,upload_bytes,download_bytes FROM vr_owner_traffic_windows WHERE owner_id=$1 AND at>now()-$2*interval '1 hour' ORDER BY at`, num(a.User, "id"), hours)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Data{}
	var cumulative, upload, download int64
	for rows.Next() {
		var at time.Time
		var bytes, up, down int64
		if e = rows.Scan(&at, &bytes, &up, &down); e != nil {
			return nil, e
		}
		cumulative += bytes
		upload += up
		download += down
		out = append(out, Data{"at": stamp(at), "bytes": bytes, "used_traffic_bytes": cumulative, "upload_bytes": up, "download_bytes": down, "raw_upload_traffic_bytes": upload, "raw_download_traffic_bytes": download, "interval_seconds": 300, "active_clients": 0, "online_nodes": 0})
	}
	return out, rows.Err()
}

func (s *Server) AuditEvents(ctx context.Context, limit int) ([]Data, error) {
	if limit < 1 || limit > 1000 {
		return nil, fmt.Errorf("audit limit must be 1–1000")
	}
	return list(ctx, s.Store.Pool, "audit", "ORDER BY id DESC LIMIT $1", limit)
}
