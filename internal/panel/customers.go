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

const customerEditable = "renewal_period renewal_every renewal_timezone name note owner_id inbound_ids service_ids plan_id enabled expires_at traffic_limit_bytes traffic_multiplier hwid_limit device_limit start_after_first_use duration_days renewal_interval_days next_renewal_at"
const planEditable = "renewal_period renewal_every renewal_timezone traffic_limit_bytes traffic_multiplier hwid_limit device_limit start_after_first_use duration_days renewal_interval_days"

func customerDefaults() Data {
	d := defaults("clients")
	for _, k := range []string{"inbound", "credential", "protocol_settings", "subscription_token"} {
		delete(d, k)
	}
	d["inbound_ids"], d["service_ids"], d["plan_id"] = []any{}, []any{}, 0
	return d
}
func idArray(d Data, key string, max int) ([]int64, error) {
	values := array(d, key)
	if len(values) > max {
		return nil, fail(400, key+" has too many entries")
	}
	out := []int64{}
	seen := map[int64]bool{}
	for _, v := range values {
		id := num(Data{"id": v}, "id")
		if id < 1 || fmt.Sprint(id) != fmt.Sprint(v) {
			return nil, fail(400, key+" must contain positive integer IDs")
		}
		if !seen[id] {
			out = append(out, id)
			seen[id] = true
		}
	}
	return out, nil
}
func idsJSON(ids []int64) []any {
	out := []any{}
	for _, id := range ids {
		out = append(out, id)
	}
	return out
}
func customerMembers(ctx context.Context, q Query, id int64) ([]Data, error) {
	return list(ctx, q, "clients", "WHERE account_id=$1 ORDER BY id", id)
}
func customerAccess(a Actor, d Data) error {
	if ownClients(a.User) && num(d, "owner_id") != num(a.User, "id") {
		return fail(404, "customer not found")
	}
	return nil
}
func effectiveCustomerInbounds(ctx context.Context, q Query, d Data) ([]int64, error) {
	ids, e := idArray(d, "inbound_ids", 200)
	if e != nil {
		return nil, e
	}
	services, e := idArray(d, "service_ids", 20)
	if e != nil {
		return nil, e
	}
	for _, id := range services {
		v, e := get(ctx, q, "services", id)
		if e != nil {
			return nil, fail(400, "service does not exist")
		}
		values, e := idArray(v, "inbound_ids", 200)
		if e != nil {
			return nil, e
		}
		ids = nodeIDs(append(ids, values...)...)
	}
	if len(ids) > 200 {
		return nil, fail(400, "a customer may have at most 200 connections")
	}
	return ids, nil
}
func nodesForInbounds(ctx context.Context, q Query, ids []int64) ([]int64, error) {
	out := []int64{}
	for _, id := range ids {
		n, e := inboundNode(ctx, q, id)
		if e != nil {
			return nil, fail(400, "inbound does not exist")
		}
		out = nodeIDs(append(out, n)...)
	}
	return out, nil
}
func clientNodes(ctx context.Context, q Query, c Data) ([]int64, error) {
	members := []Data{c}
	var e error
	if id := num(c, "_account"); id > 0 {
		members, e = customerMembers(ctx, q, id)
		if e != nil {
			return nil, e
		}
	}
	ids := []int64{}
	for _, c := range members {
		ids = append(ids, num(c, "inbound"))
	}
	return nodesForInbounds(ctx, q, ids)
}
func (s *Server) customerView(ctx context.Context, q Query, d Data) (Data, error) {
	out := public(d)
	members, e := customerMembers(ctx, q, num(d, "id"))
	if e != nil {
		return nil, e
	}
	connections := []any{}
	for _, c := range members {
		v, e := s.view(ctx, q, "clients", c)
		if e != nil {
			return nil, e
		}
		connections = append(connections, v)
	}
	out["connections"] = connections
	out["subscription_url"], e = s.subscriptionURL(ctx, q, str(d, "subscription_token"))
	if e != nil {
		return nil, e
	}
	// Imported accounts keep every historical token and can be managed here as well.
	if str(d, "subscription_token") == "" && len(members) > 0 {
		out["subscription_url"], e = s.subscriptionURL(ctx, q, str(members[0], "subscription_token"))
		if e != nil {
			return nil, e
		}
	}
	return out, nil
}
func copyAccountFields(dst, src Data) {
	for _, k := range accountFields {
		if v, ok := src[k]; ok {
			dst[k] = v
		}
	}
}

// syncCustomer preserves existing credentials/counters, mutating membership only.
// A changed service is checked against both the caller and the account's owner.
func (s *Server) syncCustomer(ctx context.Context, tx pgx.Tx, a Actor, d Data, locked []int64) error {
	ids, e := effectiveCustomerInbounds(ctx, tx, d)
	if e != nil {
		return e
	}
	if len(ids) == 0 {
		return fail(400, "select at least one inbound or a nonempty service")
	}
	nodes, e := nodesForInbounds(ctx, tx, ids)
	if e != nil {
		return e
	}
	allowedNodes := map[int64]bool{}
	for _, id := range locked {
		allowedNodes[id] = true
	}
	for _, id := range nodes {
		if !allowedNodes[id] {
			return fail(409, "service membership changed; retry")
		}
	}
	existing, e := customerMembers(ctx, tx, num(d, "id"))
	if e != nil {
		return e
	}
	existingNodes, e := clientNodesForMembers(ctx, tx, existing)
	if e != nil {
		return e
	}
	if e = ensureCovered(existingNodes, locked); e != nil {
		return e
	}
	byInbound := map[int64]Data{}
	for _, c := range existing {
		byInbound[num(c, "inbound")] = c
	}
	keep := map[int64]bool{}
	for _, id := range ids {
		c := byInbound[id]
		isNew := c == nil
		if isNew {
			c = defaults("clients")
			c["_account"] = num(d, "id")
			c["inbound"] = id
		}
		c["name"], c["note"] = str(d, "name"), str(d, "note")
		copyAccountFields(c, d)
		// The account is already in the budget set: count it once, including new children.
		if e = validateClientOwner(ctx, tx, a, c, false); e != nil {
			return e
		}
		inbound, e := get(ctx, tx, "inbounds", id)
		if e != nil {
			return e
		}
		if str(inbound, "protocol") == "wireguard" && isNew {
			node, e := get(ctx, tx, "nodes", num(inbound, "node"))
			if e != nil {
				return e
			}
			pair, e := s.nodeCall(ctx, node, "/xray/wg", Data{})
			if e != nil {
				return e
			}
			ps := Data{"private_key": pair["private_key"], "public_key": pair["public_key"]}
			peers, e := list(ctx, tx, "clients", "WHERE inbound_id=$1", id)
			if e != nil {
				return e
			}
			used := map[string]bool{}
			for _, p := range peers {
				used[fallback(str(obj(p, "protocol_settings"), "address"), wireguardAddress(num(p, "id")))] = true
			}
			for slot := int64(1); slot <= 64009; slot++ {
				address := wireguardAddress(slot)
				if !used[address] {
					ps["address"] = address
					break
				}
			}
			c["protocol_settings"] = ps
		}
		if e = validateClient(ctx, tx, c); e != nil {
			return fail(400, e.Error())
		}
		// validateClient may schedule the first renewal or clear deferred expiry.
		copyAccountFields(d, c)
		if e = save(ctx, tx, "clients", c); e != nil {
			return fail(400, databaseError(e).Error())
		}
		keep[id] = true
	}
	for _, c := range existing {
		if !keep[num(c, "inbound")] {
			if e = remove(ctx, tx, "clients", num(c, "id")); e != nil {
				return e
			}
		}
	}
	return save(ctx, tx, "customers", d)
}
func (s *Server) customersAPI(w http.ResponseWriter, r *http.Request, path string, a Actor) (any, int, error) {
	ctx := r.Context()
	parts := strings.Split(strings.Trim(path, "/"), "/")
	id := int64(0)
	if len(parts) == 2 && parts[1] == "bulk" {
		return s.bulkCustomers(w, r, a)
	}
	if len(parts) == 3 && parts[2] == "rotate-token" {
		id, e := strconv.ParseInt(parts[1], 10, 64)
		if e != nil || id < 1 {
			return nil, 0, fail(404, "customer not found")
		}
		return s.rotateCustomerToken(w, r, id, a)
	}
	if len(parts) > 2 {
		return nil, 0, fail(404, "endpoint not found")
	}
	if len(parts) == 2 {
		var e error
		id, e = strconv.ParseInt(parts[1], 10, 64)
		if e != nil || id < 1 {
			return nil, 0, fail(404, "customer not found")
		}
	}
	if r.Method == "GET" {
		if id > 0 {
			d, e := get(ctx, s.Store.Pool, "customers", id)
			if e != nil {
				return nil, 0, e
			}
			if e = customerAccess(a, d); e != nil {
				return nil, 0, e
			}
			v, e := s.customerView(ctx, s.Store.Pool, d)
			return v, 200, e
		}
		where := "ORDER BY id DESC LIMIT 1000"
		args := []any{}
		if ownClients(a.User) {
			where = "WHERE COALESCE((data->>'owner_id')::bigint,0)=$1 " + where
			args = append(args, num(a.User, "id"))
		}
		rows, e := list(ctx, s.Store.Pool, "customers", where, args...)
		if e != nil {
			return nil, 0, e
		}
		out := []Data{}
		for _, d := range rows {
			v, e := s.customerView(ctx, s.Store.Pool, d)
			if e != nil {
				return nil, 0, e
			}
			out = append(out, v)
		}
		return out, 200, nil
	}
	if !((r.Method == "POST" && id == 0) || (oneOf(r.Method, "PATCH", "DELETE") && id > 0)) {
		return nil, 0, fail(405, "method not allowed")
	}
	input := Data{}
	var e error
	if r.Method != "DELETE" {
		input, e = readBody(w, r)
		if e != nil {
			return nil, 0, e
		}
		if e = allowed(input, customerEditable); e != nil {
			return nil, 0, fail(400, e.Error())
		}
		if e = typed(input, customerDefaults()); e != nil {
			return nil, 0, fail(400, e.Error())
		}
	}
	old := customerDefaults()
	if id > 0 {
		old, e = get(ctx, s.Store.Pool, "customers", id)
		if e != nil {
			return nil, 0, e
		}
		if e = customerAccess(a, old); e != nil {
			return nil, 0, e
		}
	}
	candidate := merge(clone(old), input)
	inboundIDs, e := effectiveCustomerInbounds(ctx, s.Store.Pool, candidate)
	if e != nil && r.Method != "DELETE" {
		return nil, 0, e
	}
	members, e := customerMembers(ctx, s.Store.Pool, id)
	if e != nil {
		return nil, 0, e
	}
	for _, c := range members {
		inboundIDs = append(inboundIDs, num(c, "inbound"))
	}
	nodes, e := nodesForInbounds(ctx, s.Store.Pool, nodeIDs(inboundIDs...))
	if e != nil {
		return nil, 0, e
	}
	mutation := func(tx pgx.Tx) (any, error) {
		d := customerDefaults()
		if id > 0 {
			var e error
			d, e = get(ctx, tx, "customers", id)
			if e != nil {
				return nil, e
			}
			if e = customerAccess(a, d); e != nil {
				return nil, e
			}
		}
		if r.Method == "DELETE" {
			members, e := customerMembers(ctx, tx, id)
			if e != nil {
				return nil, e
			}
			actual, e := clientNodesForMembers(ctx, tx, members)
			if e != nil {
				return nil, e
			}
			if e = ensureCovered(actual, nodes); e != nil {
				return nil, e
			}
			if _, e := tx.Exec(ctx, "DELETE FROM vr_clients WHERE account_id=$1", id); e != nil {
				return nil, e
			}
			return nil, remove(ctx, tx, "customers", id)
		}
		prior := clone(d)
		d = merge(d, input)
		if !flag(a.User, "is_superuser") && input["owner_id"] != nil && num(input, "owner_id") != num(prior, "owner_id") {
			return nil, fail(403, "only administrators can assign customer owners")
		}
		if ownClients(a.User) || id == 0 && num(d, "owner_id") == 0 {
			d["owner_id"] = num(a.User, "id")
		}
		if e := require(d, "name", 96); e != nil {
			return nil, fail(400, e.Error())
		}

		if flag(d, "enabled") {
			d["disabled_reason"] = ""
		}
		if _, provided := input["plan_id"]; provided && num(d, "plan_id") > 0 {
			found := false
			for _, v := range array(d, "service_ids") {
				if num(Data{"id": v}, "id") == num(d, "plan_id") {
					found = true
				}
			}
			if !found {
				return nil, fail(400, "plan must be among the selected services")
			}
			plan, e := get(ctx, tx, "services", num(d, "plan_id"))
			if e != nil {
				return nil, e
			}
			for k, v := range obj(plan, "defaults") {
				if _, explicit := input[k]; !explicit {
					d[k] = v
				}
			}
		}
		reanchorRenewal(d, prior, input)
		if id == 0 {
			d["subscription_token"] = randomToken(32)
		}
		if e := save(ctx, tx, "customers", d); e != nil {
			return nil, e
		}
		if e := s.syncCustomer(ctx, tx, a, d, nodes); e != nil {
			return nil, e
		}
		return s.customerView(ctx, tx, d)
	}
	out, e := s.managementChange(ctx, nodes, a, "customer."+strings.ToLower(r.Method), fmt.Sprint(id), mutation)
	status := 200
	if r.Method == "POST" {
		status = 201
	}
	if r.Method == "DELETE" {
		status = 204
	}
	return out, status, e
}

func (s *Server) managementChange(ctx context.Context, nodes []int64, a Actor, event, target string, fn func(pgx.Tx) (any, error)) (any, error) {
	if len(nodes) > 0 {
		return s.change(ctx, nodes, a, event, target, fn)
	}
	tx, e := s.Store.Pool.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(842100,2)"); e != nil {
		return nil, e
	}
	out, e := fn(tx)
	if e != nil {
		return nil, e
	}
	if e = audit(ctx, tx, num(a.User, "id"), event, target, Data{}); e != nil {
		return nil, e
	}
	return out, tx.Commit(ctx)
}
func visibleService(a Actor, d Data) bool {
	for _, v := range array(d, "inbound_ids") {
		if !inboundAllowed(a.User, num(Data{"id": v}, "id")) {
			return false
		}
	}
	return true
}
func (s *Server) servicesAPI(w http.ResponseWriter, r *http.Request, path string, a Actor) (any, int, error) {
	ctx := r.Context()
	parts := strings.Split(strings.Trim(path, "/"), "/")
	id := int64(0)
	if len(parts) > 2 {
		return nil, 0, fail(404, "endpoint not found")
	}
	if len(parts) == 2 {
		var e error
		id, e = strconv.ParseInt(parts[1], 10, 64)
		if e != nil || id < 1 {
			return nil, 0, fail(404, "service not found")
		}
	}
	if r.Method == "GET" {
		if id > 0 {
			d, e := get(ctx, s.Store.Pool, "services", id)
			if e != nil {
				return nil, 0, e
			}
			if !visibleService(a, d) {
				return nil, 0, fail(404, "service not found")
			}
			return public(d), 200, nil
		}
		rows, e := list(ctx, s.Store.Pool, "services", "ORDER BY id")
		out := []Data{}
		for _, d := range rows {
			if visibleService(a, d) {
				out = append(out, public(d))
			}
		}
		return out, 200, e
	}
	if !flag(a.User, "is_superuser") {
		return nil, 0, fail(403, "administrator access required")
	}
	if !((r.Method == "POST" && id == 0) || (oneOf(r.Method, "PATCH", "DELETE") && id > 0)) {
		return nil, 0, fail(405, "method not allowed")
	}
	input := Data{}
	var e error
	if r.Method != "DELETE" {
		input, e = readBody(w, r)
		if e != nil {
			return nil, 0, e
		}
		if e = allowed(input, "name note inbound_ids defaults"); e != nil {
			return nil, 0, fail(400, e.Error())
		}
		if e = typed(input, Data{"name": "", "note": "", "inbound_ids": []any{}, "defaults": Data{}}); e != nil {
			return nil, 0, fail(400, e.Error())
		}
	}
	old := Data{"name": "", "note": "", "inbound_ids": []any{}, "defaults": Data{}}
	if id > 0 {
		old, e = get(ctx, s.Store.Pool, "services", id)
		if e != nil {
			return nil, 0, e
		}
	}
	candidate := merge(clone(old), input)
	ids, e := idArray(candidate, "inbound_ids", 200)
	if e != nil {
		return nil, 0, e
	}
	oldIDs, _ := idArray(old, "inbound_ids", 200)
	ids = nodeIDs(append(ids, oldIDs...)...)
	customers, e := list(ctx, s.Store.Pool, "customers", "WHERE data->'service_ids' @> $1::jsonb", fmt.Sprintf("[%d]", id))
	if e != nil {
		return nil, 0, e
	}
	for _, c := range customers {
		members, e := customerMembers(ctx, s.Store.Pool, num(c, "id"))
		if e != nil {
			return nil, 0, e
		}
		for _, m := range members {
			ids = append(ids, num(m, "inbound"))
		}
	}
	nodes, e := nodesForInbounds(ctx, s.Store.Pool, nodeIDs(ids...))
	if e != nil {
		return nil, 0, e
	}
	out, e := s.managementChange(ctx, nodes, a, "service."+strings.ToLower(r.Method), fmt.Sprint(id), func(tx pgx.Tx) (any, error) {
		d := Data{"name": "", "note": "", "inbound_ids": []any{}, "defaults": Data{}}
		if id > 0 {
			var e error
			d, e = get(ctx, tx, "services", id)
			if e != nil {
				return nil, e
			}
		}
		users, e := list(ctx, tx, "customers", "WHERE data->'service_ids' @> $1::jsonb", fmt.Sprintf("[%d]", id))
		if e != nil {
			return nil, e
		}
		if r.Method == "DELETE" {
			if len(users) > 0 {
				return nil, fail(409, "remove this service from customers before deleting it")
			}
			return nil, remove(ctx, tx, "services", id)
		}
		d = merge(d, input)
		if e = require(d, "name", 96); e != nil {
			return nil, fail(400, e.Error())
		}
		if _, e = idArray(d, "inbound_ids", 200); e != nil {
			return nil, e
		}
		plan := obj(d, "defaults")
		if e = allowed(plan, planEditable); e != nil {
			return nil, fail(400, e.Error())
		}
		if e = typed(plan, defaults("clients")); e != nil {
			return nil, fail(400, e.Error())
		}
		raw, _ := json.Marshal(plan)
		if len(raw) > 4096 {
			return nil, fail(400, "plan defaults exceed 4 KiB")
		}
		check := merge(defaults("clients"), plan)
		if _, _, _, e = renewalSpec(check); e != nil {
			return nil, fail(400, e.Error())
		}
		if _, e = multiplierMilli(check); e != nil {
			return nil, fail(400, e.Error())
		}
		if num(check, "traffic_limit_bytes") < 0 || num(check, "hwid_limit") < -1 || num(check, "hwid_limit") > 1000 || num(check, "device_limit") < 0 || num(check, "device_limit") > 10000 || num(check, "renewal_interval_days") < 0 || num(check, "renewal_interval_days") > 3650 || (flag(check, "start_after_first_use") && (num(check, "duration_days") < 1 || num(check, "duration_days") > 36500)) {
			return nil, fail(400, "invalid plan limits")
		}
		for _, v := range array(d, "inbound_ids") {
			if _, e = get(ctx, tx, "inbounds", num(Data{"id": v}, "id")); e != nil {
				return nil, fail(400, "inbound does not exist")
			}
		}

		if e = save(ctx, tx, "services", d); e != nil {
			return nil, e
		}
		for _, c := range users {
			if e = s.syncCustomer(ctx, tx, a, c, nodes); e != nil {
				return nil, e
			}
		}
		return public(d), nil
	})
	status := 200
	if r.Method == "POST" {
		status = 201
	}
	if r.Method == "DELETE" {
		status = 204
	}
	return out, status, e
}

// hydrateCustomer supplies management fields for imported legacy accounts without
// changing their tokens or accounting records during reads.
func hydrateCustomer(ctx context.Context, q Query, d Data) error {
	if _, exists := d["inbound_ids"]; exists {
		return nil
	}
	members, e := customerMembers(ctx, q, num(d, "id"))
	if e != nil {
		return e
	}
	ids := []int64{}
	for _, c := range members {
		ids = append(ids, num(c, "inbound"))
	}
	d["inbound_ids"], d["service_ids"], d["plan_id"] = idsJSON(ids), []any{}, 0
	if len(members) > 0 {
		d["name"] = str(members[0], "name")
		d["note"] = str(members[0], "note")
	}
	return nil
}

func clientNodesForMembers(ctx context.Context, q Query, members []Data) ([]int64, error) {
	ids := []int64{}
	for _, c := range members {
		ids = append(ids, num(c, "inbound"))
	}
	return nodesForInbounds(ctx, q, nodeIDs(ids...))
}

func (s *Server) rotateCustomerToken(w http.ResponseWriter, r *http.Request, id int64, a Actor) (any, int, error) {
	if r.Method != "POST" {
		return nil, 0, fail(405, "method not allowed")
	}
	input, e := readBody(w, r)
	if e != nil {
		return nil, 0, e
	}
	if len(input) > 0 {
		return nil, 0, fail(400, "no fields are accepted")
	}
	ctx := r.Context()
	out, e := s.managementChange(ctx, nil, a, "customer.rotate-token", fmt.Sprint(id), func(tx pgx.Tx) (any, error) {
		d, e := get(ctx, tx, "customers", id)
		if e != nil {
			return nil, e
		}
		if e = customerAccess(a, d); e != nil {
			return nil, e
		}
		members, e := customerMembers(ctx, tx, id)
		if e != nil {
			return nil, e
		}
		d["subscription_token"] = randomToken(32)
		if e = save(ctx, tx, "customers", d); e != nil {
			return nil, e
		}
		for _, c := range members {
			c["subscription_token"] = randomToken(32)
			if e = save(ctx, tx, "clients", c); e != nil {
				return nil, e
			}
		}
		return s.customerView(ctx, tx, d)
	})
	return out, 200, e
}
