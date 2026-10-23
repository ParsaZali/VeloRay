package panel

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func descendant(ctx context.Context, q Query, parent, child int64) (bool, error) {
	var yes bool
	e := q.QueryRow(ctx, `WITH RECURSIVE tree(id,depth) AS (SELECT id,1 FROM vr_users WHERE COALESCE((data->>'parent_id')::bigint,0)=$1 UNION ALL SELECT u.id,t.depth+1 FROM vr_users u JOIN tree t ON COALESCE((u.data->>'parent_id')::bigint,0)=t.id WHERE t.depth<8) SELECT EXISTS(SELECT 1 FROM tree WHERE id=$2)`, parent, child).Scan(&yes)
	return yes, e
}
func resellerAncestorsActive(ctx context.Context, q Query, u Data) error {
	seen := map[int64]bool{}
	for depth := 0; num(u, "parent_id") > 0; depth++ {
		if depth >= 8 || seen[num(u, "parent_id")] {
			return fail(403, "invalid reseller hierarchy")
		}
		seen[num(u, "parent_id")] = true
		var e error
		u, e = get(ctx, q, "users", num(u, "parent_id"))
		if e != nil || !flag(u, "is_active") || !flag(u, "is_staff") {
			return fail(403, "parent reseller account is inactive")
		}
	}
	return nil
}
func childReservations(ctx context.Context, q Query, owner, exclude int64) (int64, int64, error) {
	children, e := list(ctx, q, "users", `WHERE COALESCE((data->>'parent_id')::bigint,0)=$1 AND id<>$2`, owner, exclude)
	if e != nil {
		return 0, 0, e
	}
	var count, traffic int64
	for _, c := range children {
		if num(c, "max_clients") > math.MaxInt64-count || num(c, "traffic_budget_bytes") > math.MaxInt64-traffic {
			return 0, 0, fail(400, "subreseller reservation overflow")
		}
		count += num(c, "max_clients")
		traffic += num(c, "traffic_budget_bytes")
	}
	return count, traffic, nil
}
func ownAllocation(ctx context.Context, q Query, owner int64) (int64, int64, error) {
	rows, e := list(ctx, q, "clients", "WHERE "+ownerExpression+"=$1", owner)
	if e != nil {
		return 0, 0, e
	}
	seen := map[string]bool{}
	var count, traffic int64
	for _, c := range rows {
		key := accountKey(c)
		if seen[key] {
			continue
		}
		seen[key] = true
		count++
		quota := num(c, "traffic_limit_bytes")
		if quota == 0 {
			traffic = -1
			continue
		}
		if traffic < 0 {
			continue
		}
		if quota > math.MaxInt64-traffic {
			return 0, 0, fail(400, "traffic allocation overflow")
		}
		traffic += quota
	}
	return count, traffic, nil
}
func validateHierarchyAccount(ctx context.Context, q Query, d, old Data) error {
	if num(d, "max_subresellers") < 0 || num(d, "max_subresellers") > 100 {
		return fail(400, "max_subresellers must be 0–100")
	}
	if num(old, "id") > 0 && num(old, "parent_id") != num(d, "parent_id") {
		return fail(400, "reseller parent cannot be reassigned after creation")
	}
	if parentID := num(d, "parent_id"); parentID > 0 {
		if str(d, "role") != "reseller" || flag(d, "is_superuser") {
			return fail(400, "a subreseller cannot have administrator privileges")
		}
		parent, e := get(ctx, q, "users", parentID)
		if e != nil || !flag(parent, "is_active") || !flag(parent, "is_staff") || !permitted(parent, "clients.write") {
			return fail(400, "parent reseller is unavailable")
		}
		depth := 1
		ancestor := parent
		seen := map[int64]bool{num(d, "id"): num(d, "id") > 0}
		for {
			id := num(ancestor, "id")
			if seen[id] || depth > 8 {
				return fail(400, "reseller hierarchy depth or cycle limit reached")
			}
			seen[id] = true
			if num(ancestor, "parent_id") == 0 {
				break
			}
			ancestor, e = get(ctx, q, "users", num(ancestor, "parent_id"))
			if e != nil {
				return e
			}
			depth++
		}
		for _, value := range array(d, "allowed_inbounds") {
			if !inboundAllowed(parent, num(Data{"id": value}, "id")) {
				return fail(400, "subreseller inbounds must be assigned to its parent")
			}
		}
		siblings, traffic, e := capacityReservations(ctx, q, parentID, num(d, "id"))
		if e != nil {
			return e
		}
		ownCount, ownTraffic, e := ownAllocation(ctx, q, parentID)
		if e != nil {
			return e
		}
		if max := num(parent, "max_clients"); max > 0 && (num(d, "max_clients") == 0 || siblings > max || ownCount > max-siblings || num(d, "max_clients") > max-siblings-ownCount) {
			return fail(400, "subreseller client reservation exceeds the parent's available limit")
		}
		if max := num(parent, "traffic_budget_bytes"); max > 0 && (num(d, "traffic_budget_bytes") == 0 || ownTraffic < 0 || traffic < 0 || traffic > max || ownTraffic > max-traffic || num(d, "traffic_budget_bytes") > max-traffic-ownTraffic) {
			return fail(400, "subreseller traffic reservation exceeds the parent's available limit")
		}
	}
	if num(d, "id") > 0 {
		children, e := list(ctx, q, "users", `WHERE COALESCE((data->>'parent_id')::bigint,0)=$1`, num(d, "id"))
		if e != nil {
			return e
		}
		if !flag(d, "is_superuser") && flag(d, "can_delegate") && int64(len(children)) > num(d, "max_subresellers") {
			return fail(400, "subreseller limit is below existing direct subresellers")
		}
		reservedCount, reservedTraffic, e := capacityReservations(ctx, q, num(d, "id"), 0)
		if e != nil {
			return e
		}
		ownCount, ownTraffic, e := ownAllocation(ctx, q, num(d, "id"))
		if e != nil {
			return e
		}
		if max := num(d, "max_clients"); max > 0 && (reservedCount > max || ownCount > max-reservedCount) {
			return fail(400, "account limit is below its existing clients and child reservations")
		}
		if max := num(d, "traffic_budget_bytes"); max > 0 && (ownTraffic < 0 || reservedTraffic < 0 || reservedTraffic > max || ownTraffic > max-reservedTraffic) {
			return fail(400, "traffic budget is below existing allocations and child reservations")
		}
		for _, child := range children {
			if num(d, "max_clients") > 0 && num(child, "max_clients") == 0 || num(d, "traffic_budget_bytes") > 0 && num(child, "traffic_budget_bytes") == 0 {
				return fail(400, "limited parents cannot contain unlimited subresellers")
			}
			for _, id := range array(child, "allowed_inbounds") {
				if !inboundAllowed(d, num(Data{"id": id}, "id")) {
					return fail(400, "remove child inbound assignments before removing them from the parent")
				}
			}
		}
	}
	return nil
}
func resellerView(ctx context.Context, q Query, u Data) (Data, error) {
	count, traffic, e := ownAllocation(ctx, q, num(u, "id"))
	if e != nil {
		return nil, e
	}
	out := Data{}
	for _, key := range strings.Fields("id username email is_active parent_id allowed_inbounds max_clients traffic_budget_bytes can_delegate max_subresellers") {
		out[key] = u[key]
	}
	out["clients_count"] = count
	out["unrestricted_inbounds"] = flag(u, "is_superuser") || (str(u, "role") != "reseller" && len(array(u, "allowed_inbounds")) == 0)
	out["may_delegate"] = flag(u, "is_superuser") || (flag(u, "can_delegate") && num(u, "max_subresellers") > 0)
	out["allocated_traffic_bytes"] = traffic
	reserved, volume, e := childReservations(ctx, q, num(u, "id"), 0)
	out["reserved_clients"], out["reserved_traffic_bytes"] = reserved, volume
	return out, e
}
func (s *Server) resellersAPI(w http.ResponseWriter, r *http.Request, path string, a Actor) (any, int, error) {
	ctx := r.Context()
	parent, e := s.commerceOwner(r, a)
	if e != nil {
		return nil, 0, e
	}
	if path == "resellers/policy" && r.Method == "GET" {
		u, e := get(ctx, s.Store.Pool, "users", parent)
		if e != nil {
			return nil, 0, e
		}
		d, e := resellerView(ctx, s.Store.Pool, u)
		return d, 200, e
	}
	if path == "resellers" && r.Method == "GET" {
		rows, e := list(ctx, s.Store.Pool, "users", `WHERE COALESCE((data->>'parent_id')::bigint,0)=$1 ORDER BY id`, parent)
		if e != nil {
			return nil, 0, e
		}
		out := []Data{}
		for _, u := range rows {
			v, e := resellerView(ctx, s.Store.Pool, u)
			if e != nil {
				return nil, 0, e
			}
			out = append(out, v)
		}
		return out, 200, nil
	}
	if path == "resellers/transfer" && r.Method == "POST" {
		return s.transferResellerCredit(w, r, a)
	}
	if !permitted(a.User, "resellers.write") {
		return nil, 0, fail(403, "reseller delegation is not enabled")
	}
	if a.Scope != "admin" {
		return nil, 0, fail(403, "administrator API scope required to manage reseller identities")
	}
	id := int64(0)
	if path != "resellers" {
		parts := strings.Split(path, "/")
		if len(parts) != 2 {
			return nil, 0, fail(404, "reseller not found")
		}
		id, _ = strconv.ParseInt(parts[1], 10, 64)
		if id < 1 {
			return nil, 0, fail(404, "reseller not found")
		}
	}
	if !(r.Method == "POST" && id == 0 || r.Method == "PATCH" && id > 0) {
		return nil, 0, fail(405, "method not allowed")
	}
	input, e := readBody(w, r)
	if e != nil {
		return nil, 0, e
	}
	if e = allowed(input, "username email password is_active allowed_inbounds max_clients traffic_budget_bytes can_delegate max_subresellers"); e != nil {
		return nil, 0, fail(400, e.Error())
	}
	if e = typed(input, merge(defaults("users"), Data{"password": ""})); e != nil {
		return nil, 0, fail(400, e.Error())
	}
	if e = rateLimit(ctx, s.Store.Pool, "reseller:manage:"+fmt.Sprint(num(a.User, "id")), 60, time.Hour); e != nil {
		return nil, 0, e
	}
	out, e := s.managementChange(ctx, nil, a, "reseller."+strings.ToLower(r.Method), fmt.Sprint(id), func(tx pgx.Tx) (any, error) {
		p, e := get(ctx, tx, "users", parent)
		if e != nil {
			return nil, e
		}
		if !flag(p, "is_active") || !permitted(p, "clients.write") {
			return nil, fail(403, "parent reseller is unavailable")
		}
		if !flag(p, "is_superuser") && (!flag(p, "can_delegate") || num(p, "max_subresellers") == 0) {
			return nil, fail(403, "reseller delegation is not enabled")
		}
		u := defaults("users")
		old := clone(u)
		if id > 0 {
			u, e = get(ctx, tx, "users", id)
			if e != nil {
				return nil, e
			}
			if num(u, "parent_id") != parent {
				return nil, fail(404, "reseller not found")
			}
			old = clone(u)
		} else {
			var n int
			if e = tx.QueryRow(ctx, `SELECT count(*) FROM vr_users WHERE COALESCE((data->>'parent_id')::bigint,0)=$1`, parent).Scan(&n); e != nil {
				return nil, e
			}
			if !flag(p, "is_superuser") && int64(n) >= num(p, "max_subresellers") {
				return nil, fail(409, "subreseller limit reached")
			}
			u["parent_id"] = parent
			u["role"] = "reseller"
			u["own_clients_only"] = true
		}
		u = merge(u, input)
		delete(u, "password")
		if e = require(u, "username", 150); e != nil {
			return nil, fail(400, e.Error())
		}
		password := str(input, "password")
		generated := ""
		if id == 0 && password == "" {
			generated = randomToken(18)
			password = generated
		}
		if password != "" {
			if e = passwordPolicy(str(u, "username"), password); e != nil {
				return nil, fail(400, e.Error())
			}
			u["_password"], e = hashPassword(password)
			if e != nil {
				return nil, e
			}
			u["_auth_version"] = num(u, "_auth_version") + 1
		}
		if e = validateAccount(ctx, tx, u, old, Data{}); e != nil {
			return nil, e
		}
		if e = save(ctx, tx, "users", u); e != nil {
			return nil, fail(400, databaseError(e).Error())
		}
		v, e := resellerView(ctx, tx, u)
		if generated != "" {
			v["generated_password"] = generated
		}
		return v, e
	})
	status := 200
	if r.Method == "POST" {
		status = 201
	}
	return out, status, e
}
func (s *Server) transferResellerCredit(w http.ResponseWriter, r *http.Request, a Actor) (any, int, error) {
	if !permitted(a.User, "resellers.write") {
		return nil, 0, fail(403, "reseller delegation is not enabled")
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, 0, e
	}
	if e = allowed(d, "user_id amount currency reference note"); e != nil {
		return nil, 0, fail(400, e.Error())
	}
	amount, e := money(d, "amount", 1)
	if e != nil {
		return nil, 0, e
	}
	if len(str(d, "reference")) < 12 || len(str(d, "reference")) > 100 {
		return nil, 0, fail(400, "unique transfer reference required")
	}
	if len(str(d, "note")) > 400 {
		return nil, 0, fail(400, "transfer note is too long")
	}
	ctx := r.Context()
	owner := num(a.User, "id")
	target := num(d, "user_id")
	out, e := s.managementChange(ctx, nil, a, "wallet.transfer", fmt.Sprint(target), func(tx pgx.Tx) (any, error) {
		u, e := get(ctx, tx, "users", target)
		if e != nil || num(u, "parent_id") != owner {
			return nil, fail(404, "direct subreseller not found")
		}
		if !flag(u, "is_active") {
			return nil, fail(409, "subreseller is inactive")
		}
		note := "Transfer to #" + fmt.Sprint(target) + ": " + str(d, "note")
		ref := "transfer:" + fmt.Sprint(owner) + ":" + str(d, "reference")
		debit, e := walletEntry(ctx, tx, owner, str(d, "currency"), -amount, ref+":debit", note, owner)
		if e != nil {
			return nil, e
		}
		credit, e := walletEntry(ctx, tx, target, str(d, "currency"), amount, ref+":credit", note, owner)
		if e != nil {
			return nil, e
		}
		return Data{"debit": debit, "credit": credit}, nil
	})
	return out, 200, e
}
