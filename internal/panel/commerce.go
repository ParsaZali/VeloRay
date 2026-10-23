package panel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

const moneyLimit int64 = 1_000_000_000_000

var commerceID = regexp.MustCompile(`^[A-Za-z0-9_-]{20,80}$`)

func money(d Data, key string, minimum int64) (int64, error) {
	v := d[key]
	n, err := strconv.ParseInt(fmt.Sprint(v), 10, 64)
	if err != nil || n < minimum || n > moneyLimit {
		return 0, fail(400, key+" must be a whole amount within the supported range")
	}
	return n, nil
}
func validCurrency(s string) bool { return oneOf(s, "IRR", "IRT", "USD", "EUR") }
func commerceAdmin(a Actor) bool  { return flag(a.User, "is_superuser") && a.Scope == "admin" }
func commerceOwner(r *http.Request, a Actor) (int64, error) {
	id := num(a.User, "id")
	if raw := r.URL.Query().Get("owner"); raw != "" {
		selected, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || selected < 1 {
			return 0, fail(400, "invalid seller")
		}
		if selected != id && !commerceAdmin(a) {
			return 0, fail(403, "seller access denied")
		}
		id = selected
	}
	return id, nil
}
func storeDefaults() Data {
	return Data{"enabled": false, "name": "VeloRay", "support_url": "", "logo_url": "", "description": ""}
}
func getStore(ctx context.Context, q Query, owner int64) (Data, error) {
	d := storeDefaults()
	var raw []byte
	err := q.QueryRow(ctx, `SELECT data FROM vr_stores WHERE owner_id=$1`, owner).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, nil
	}
	if err != nil {
		return nil, err
	}
	v, err := decode(raw)
	return merge(d, v), err
}
func seller(ctx context.Context, q Query, id int64) (Data, error) {
	u, err := get(ctx, q, "users", id)
	if err != nil || !flag(u, "is_active") || !flag(u, "is_staff") || !permitted(u, "clients.write") {
		return nil, fail(409, "seller is unavailable")
	}
	return u, nil
}
func commerceURL(raw string) bool {
	if raw == "" {
		return true
	}
	u, e := url.Parse(raw)
	return e == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.Fragment == ""
}
func offer(ctx context.Context, q Query, id int64) (Data, error) {
	var raw []byte
	err := q.QueryRow(ctx, `SELECT data||jsonb_build_object('id',id,'owner_id',owner_id) FROM vr_offers WHERE id=$1`, id).Scan(&raw)
	if err != nil {
		return nil, err
	}
	return decode(raw)
}
func offers(ctx context.Context, q Query, owner int64, published bool) ([]Data, error) {
	rows, err := q.Query(ctx, `SELECT data||jsonb_build_object('id',id,'owner_id',owner_id) FROM vr_offers WHERE owner_id=$1 ORDER BY id DESC LIMIT 500`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Data{}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		d, e := decode(raw)
		if e != nil {
			return nil, e
		}
		if !published || flag(d, "enabled") {
			out = append(out, public(d))
		}
	}
	return out, rows.Err()
}
func (s *Server) validateOffer(ctx context.Context, q Query, d Data) error {
	if e := require(d, "name", 96); e != nil {
		return fail(400, e.Error())
	}
	if len(str(d, "description")) > 1200 {
		return fail(400, "description is too long")
	}
	if _, e := money(d, "amount", 1); e != nil {
		return e
	}
	if !validCurrency(str(d, "currency")) {
		return fail(400, "unsupported currency")
	}
	if num(d, "duration_days") < 1 || num(d, "duration_days") > 3650 {
		return fail(400, "duration_days must be 1–3650")
	}
	if num(d, "traffic_bytes") < 0 || num(d, "traffic_bytes") > 1<<60 {
		return fail(400, "invalid traffic grant")
	}
	if num(d, "cost_amount") < 0 || num(d, "cost_amount") > moneyLimit {
		return fail(400, "invalid unit cost")
	}
	service, e := get(ctx, q, "services", num(d, "service_id"))
	if e != nil {
		return fail(400, "service does not exist")
	}
	u, e := seller(ctx, q, num(d, "owner_id"))
	if e != nil {
		return e
	}
	if !visibleService(Actor{User: u}, service) {
		return fail(403, "service is not assigned to this seller")
	}
	ids, e := idArray(service, "inbound_ids", 200)
	if e != nil {
		return e
	}
	if len(ids) == 0 {
		return fail(400, "service must contain an inbound")
	}
	for _, id := range ids {
		v, e := get(ctx, q, "inbounds", id)
		if e != nil {
			return e
		}
		if !supportsClients(str(v, "protocol")) {
			return fail(400, "service contains an inbound without managed users")
		}
	}
	return nil
}

// Wallet rows are locked before recording a balance and the append-only ledger.
// The caller's reference is unique and a retry must have the identical payload.
func walletEntry(ctx context.Context, tx pgx.Tx, user int64, currency string, amount int64, reference, note string, actor int64) (Data, error) {
	if !validCurrency(currency) || amount == 0 || amount < -moneyLimit || amount > moneyLimit || len(reference) < 12 || len(reference) > 160 || len(note) > 500 {
		return nil, fail(400, "invalid ledger entry")
	}
	if _, err := tx.Exec(ctx, `INSERT INTO vr_wallets(user_id,currency) VALUES($1,$2) ON CONFLICT DO NOTHING`, user, currency); err != nil {
		return nil, err
	}
	var balance, credit int64
	if err := tx.QueryRow(ctx, `SELECT balance,credit_limit FROM vr_wallets WHERE user_id=$1 AND currency=$2 FOR UPDATE`, user, currency).Scan(&balance, &credit); err != nil {
		return nil, err
	}
	var id, oldUser, oldAmount, oldBalance int64
	var oldCurrency, oldNote string
	err := tx.QueryRow(ctx, `SELECT id,user_id,currency,amount,balance_after,note FROM vr_ledger WHERE reference=$1`, reference).Scan(&id, &oldUser, &oldCurrency, &oldAmount, &oldBalance, &oldNote)
	if err == nil {
		if oldUser != user || oldCurrency != currency || oldAmount != amount || oldNote != note {
			return nil, fail(409, "reference was already used for a different entry")
		}
		return Data{"id": id, "balance": oldBalance, "duplicate": true}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if amount > 0 && balance > moneyLimit-amount {
		return nil, fail(400, "wallet balance would exceed its limit")
	}
	if amount < 0 && balance < -credit-amount {
		return nil, fail(409, "insufficient wallet credit")
	}
	balance += amount
	if _, err = tx.Exec(ctx, `UPDATE vr_wallets SET balance=$3,updated_at=now() WHERE user_id=$1 AND currency=$2`, user, currency, balance); err != nil {
		return nil, err
	}
	var actorValue any
	if actor > 0 {
		actorValue = actor
	}
	if err = tx.QueryRow(ctx, `INSERT INTO vr_ledger(user_id,currency,amount,balance_after,reference,note,actor_id) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`, user, currency, amount, balance, reference, note, actorValue).Scan(&id); err != nil {
		return nil, err
	}
	return Data{"id": id, "balance": balance, "duplicate": false}, nil
}
func commerceRows(ctx context.Context, q Query, query string, args ...any) ([]Data, error) {
	rows, e := q.Query(ctx, query, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Data{}
	for rows.Next() {
		var raw []byte
		if e = rows.Scan(&raw); e != nil {
			return nil, e
		}
		d, e := decode(raw)
		if e != nil {
			return nil, e
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

const orderSelect = `SELECT jsonb_build_object('id',id,'owner_id',owner_id,'payer_id',payer_id,'offer_id',offer_id,'customer_id',customer_id,'kind',kind,'amount',amount,'currency',currency,'state',state,'gateway',gateway,'payment_reference',payment_reference,'snapshot',snapshot,'attempts',attempts,'last_error',last_error,'created_at',created_at,'paid_at',paid_at,'fulfilled_at',fulfilled_at,'expires_at',expires_at,'_authority',authority,'_token_hash',token_hash) FROM vr_orders `

func getOrder(ctx context.Context, q Query, id string, lock bool) (Data, error) {
	if !commerceID.MatchString(id) {
		return nil, fail(404, "order not found")
	}
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	var raw []byte
	err := q.QueryRow(ctx, orderSelect+`WHERE id=$1`+suffix, id).Scan(&raw)
	if err != nil {
		return nil, err
	}
	return decode(raw)
}
func orderView(d Data) Data {
	out := public(d)
	snapshot := obj(d, "snapshot")
	out["test_mode"] = flag(snapshot, "_sandbox")
	out["snapshot"] = Data{"name": snapshot["name"], "duration_days": snapshot["duration_days"], "traffic_bytes": snapshot["traffic_bytes"], "customer_name": snapshot["customer_name"]}
	return out
}
func (s *Server) commerceAPI(w http.ResponseWriter, r *http.Request, path string, a Actor) (any, int, error) {
	owner, e := commerceOwner(r, a)
	if e != nil {
		return nil, 0, e
	}
	ctx := r.Context()
	part := strings.TrimPrefix(path, "commerce/")
	if part == "settings" {
		return s.commerceSettings(w, r, a)
	}
	if part == "store" {
		if r.Method == "GET" {
			d, e := getStore(ctx, s.Store.Pool, owner)
			if d != nil {
				d["url"] = strings.TrimRight(s.Config.PublicURL, "/") + "/shop?store=" + fmt.Sprint(owner)
				d["owner_id"] = owner
			}
			return d, 200, e
		}
		if r.Method != "PATCH" {
			return nil, 0, fail(405, "method not allowed")
		}
		if !permitted(a.User, "commerce.write") {
			return nil, 0, fail(403, "commerce write access required")
		}
		input, e := readBody(w, r)
		if e != nil {
			return nil, 0, e
		}
		if e = allowed(input, "enabled name support_url logo_url description"); e != nil {
			return nil, 0, fail(400, e.Error())
		}
		if e = typed(input, storeDefaults()); e != nil {
			return nil, 0, fail(400, e.Error())
		}
		out, e := s.managementChange(ctx, nil, a, "commerce.store", fmt.Sprint(owner), func(tx pgx.Tx) (any, error) {
			d, e := getStore(ctx, tx, owner)
			if e != nil {
				return nil, e
			}
			d = merge(d, input)
			if e = require(d, "name", 96); e != nil {
				return nil, fail(400, e.Error())
			}
			if !commerceURL(str(d, "support_url")) || !commerceURL(str(d, "logo_url")) || len(str(d, "description")) > 1200 {
				return nil, fail(400, "invalid branding URL or description")
			}
			if _, e = seller(ctx, tx, owner); e != nil {
				return nil, e
			}
			raw, _ := json.Marshal(d)
			_, e = tx.Exec(ctx, `INSERT INTO vr_stores(owner_id,data) VALUES($1,$2) ON CONFLICT(owner_id) DO UPDATE SET data=excluded.data`, owner, string(raw))
			return d, e
		})
		return out, 200, e
	}
	if part == "offers" || strings.HasPrefix(part, "offers/") {
		return s.offersAPI(w, r, part, a, owner)
	}
	if part == "wallets" && r.Method == "GET" {
		out, e := commerceRows(ctx, s.Store.Pool, `SELECT to_jsonb(w) FROM vr_wallets w WHERE user_id=$1 ORDER BY currency`, owner)
		return out, 200, e
	}
	if part == "ledger" && r.Method == "GET" {
		out, e := commerceRows(ctx, s.Store.Pool, `SELECT to_jsonb(l) FROM vr_ledger l WHERE user_id=$1 ORDER BY id DESC LIMIT 500`, owner)
		return out, 200, e
	}
	if part == "adjust" && r.Method == "POST" {
		if !commerceAdmin(a) {
			return nil, 0, fail(403, "administrator access required")
		}
		d, e := readBody(w, r)
		if e != nil {
			return nil, 0, e
		}
		amount, e := money(d, "amount", -moneyLimit)
		if e != nil || amount == 0 {
			return nil, 0, fail(400, "nonzero whole amount required")
		}
		if e = require(d, "note", 500); e != nil {
			return nil, 0, fail(400, e.Error())
		}
		out, e := s.managementChange(ctx, nil, a, "wallet.adjust", fmt.Sprint(owner), func(tx pgx.Tx) (any, error) {
			return walletEntry(ctx, tx, owner, str(d, "currency"), amount, "manual:"+str(d, "reference"), str(d, "note"), num(a.User, "id"))
		})
		return out, 200, e
	}
	if part == "credit-limit" && r.Method == "POST" {
		if !commerceAdmin(a) {
			return nil, 0, fail(403, "administrator access required")
		}
		d, e := readBody(w, r)
		if e != nil {
			return nil, 0, e
		}
		limit, e := money(d, "credit_limit", 0)
		if e != nil {
			return nil, 0, e
		}
		currency := str(d, "currency")
		if !validCurrency(currency) {
			return nil, 0, fail(400, "unsupported currency")
		}
		out, e := s.managementChange(ctx, nil, a, "wallet.credit_limit", fmt.Sprint(owner), func(tx pgx.Tx) (any, error) {
			if _, e := tx.Exec(ctx, `INSERT INTO vr_wallets(user_id,currency) VALUES($1,$2) ON CONFLICT DO NOTHING`, owner, currency); e != nil {
				return nil, e
			}
			tag, e := tx.Exec(ctx, `UPDATE vr_wallets SET credit_limit=$3,updated_at=now() WHERE user_id=$1 AND currency=$2 AND balance >= -($3::bigint)`, owner, currency, limit)
			if e == nil && tag.RowsAffected() != 1 {
				e = fail(409, "credit limit is below the outstanding debt")
			}
			return Data{"credit_limit": limit}, e
		})
		return out, 200, e
	}
	if part == "orders" && r.Method == "GET" {
		rows, e := commerceRows(ctx, s.Store.Pool, orderSelect+`WHERE owner_id=$1 OR payer_id=$1 ORDER BY created_at DESC LIMIT 500`, owner)
		out := []Data{}
		for _, d := range rows {
			out = append(out, orderView(d))
		}
		return out, 200, e
	}
	if strings.HasPrefix(part, "orders/") && r.Method == "POST" {
		parts := strings.Split(part, "/")
		if len(parts) != 3 || parts[2] != "fulfill" {
			return nil, 0, fail(404, "endpoint not found")
		}
		d, e := getOrder(ctx, s.Store.Pool, parts[1], false)
		if e != nil {
			return nil, 0, e
		}
		if num(d, "owner_id") != owner && num(d, "payer_id") != num(a.User, "id") {
			return nil, 0, fail(404, "order not found")
		}
		e = s.fulfillOrder(ctx, parts[1])
		if e != nil {
			return nil, 0, e
		}
		d, e = getOrder(ctx, s.Store.Pool, parts[1], false)
		return orderView(d), 200, e
	}
	if part == "purchase" && r.Method == "POST" {
		return s.walletPurchase(w, r, a)
	}
	if part == "topup" && r.Method == "POST" {
		return s.walletTopup(w, r, a)
	}
	if part == "costs" {
		return s.commerceCosts(w, r, a, owner)
	}
	if part == "reports" && r.Method == "GET" {
		return s.commerceReports(ctx, r, a, owner)
	}
	return nil, 0, fail(405, "method not allowed")
}
func (s *Server) offersAPI(w http.ResponseWriter, r *http.Request, path string, a Actor, owner int64) (any, int, error) {
	ctx := r.Context()
	id := int64(0)
	if path != "offers" {
		parts := strings.Split(path, "/")
		if len(parts) != 2 {
			return nil, 0, fail(404, "offer not found")
		}
		id, _ = strconv.ParseInt(parts[1], 10, 64)
		if id < 1 {
			return nil, 0, fail(404, "offer not found")
		}
	}
	if r.Method == "GET" && id == 0 {
		d, e := offers(ctx, s.Store.Pool, owner, false)
		return d, 200, e
	}
	if !permitted(a.User, "commerce.write") {
		return nil, 0, fail(403, "commerce write access required")
	}
	if !(r.Method == "POST" && id == 0 || r.Method == "PATCH" && id > 0) {
		return nil, 0, fail(405, "method not allowed")
	}
	input, e := readBody(w, r)
	if e != nil {
		return nil, 0, e
	}
	defaults := Data{"name": "", "description": "", "enabled": false, "service_id": 0, "amount": 0, "currency": "IRT", "duration_days": 30, "traffic_bytes": int64(0), "cost_amount": int64(0)}
	if e = allowed(input, "name description enabled service_id amount currency duration_days traffic_bytes cost_amount"); e != nil {
		return nil, 0, fail(400, e.Error())
	}
	if e = typed(input, defaults); e != nil {
		return nil, 0, fail(400, e.Error())
	}
	out, e := s.managementChange(ctx, nil, a, "commerce.offer", fmt.Sprint(id), func(tx pgx.Tx) (any, error) {
		d := defaults
		if id > 0 {
			var e error
			d, e = offer(ctx, tx, id)
			if e != nil {
				return nil, e
			}
			if num(d, "owner_id") != owner {
				return nil, fail(404, "offer not found")
			}
		}
		d = merge(d, input)
		d["owner_id"] = owner
		if e = s.validateOffer(ctx, tx, d); e != nil {
			return nil, e
		}
		d["updated_at"] = stamp()
		raw, _ := json.Marshal(d)
		if id == 0 {
			e = tx.QueryRow(ctx, `INSERT INTO vr_offers(owner_id,data) VALUES($1,$2) RETURNING id`, owner, string(raw)).Scan(&id)
		} else {
			_, e = tx.Exec(ctx, `UPDATE vr_offers SET data=$2 WHERE id=$1`, id, string(raw))
		}
		d["id"] = id
		return public(d), e
	})
	status := 200
	if r.Method == "POST" {
		status = 201
	}
	return out, status, e
}

func (s *Server) commerceCosts(w http.ResponseWriter, r *http.Request, a Actor, owner int64) (any, int, error) {
	ctx := r.Context()
	if r.Method == "GET" {
		out, e := commerceRows(ctx, s.Store.Pool, `SELECT to_jsonb(c) FROM vr_cost_entries c WHERE owner_id=$1 ORDER BY id DESC LIMIT 500`, owner)
		return out, 200, e
	}
	if r.Method != "POST" {
		return nil, 0, fail(405, "method not allowed")
	}
	if !commerceAdmin(a) {
		return nil, 0, fail(403, "administrator access required")
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, 0, e
	}
	amount, e := money(d, "amount", 1)
	if e != nil {
		return nil, 0, e
	}
	if !validCurrency(str(d, "currency")) || len(str(d, "reference")) < 12 || len(str(d, "reference")) > 160 {
		return nil, 0, fail(400, "invalid cost currency or reference")
	}
	if e = require(d, "note", 500); e != nil {
		return nil, 0, fail(400, e.Error())
	}
	var node any
	if num(d, "node_id") > 0 {
		node = num(d, "node_id")
	}
	out, e := s.managementChange(ctx, nil, a, "commerce.cost", fmt.Sprint(owner), func(tx pgx.Tx) (any, error) {
		var id int64
		e := tx.QueryRow(ctx, `INSERT INTO vr_cost_entries(owner_id,node_id,amount,currency,note,reference) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, owner, node, amount, str(d, "currency"), str(d, "note"), str(d, "reference")).Scan(&id)
		return Data{"id": id}, e
	})
	return out, 201, e
}
func (s *Server) commerceReports(ctx context.Context, r *http.Request, a Actor, owner int64) (any, int, error) {
	days := int64(30)
	if r.URL.Query().Get("days") != "" {
		var e error
		days, e = strconv.ParseInt(r.URL.Query().Get("days"), 10, 64)
		if e != nil || days < 1 || days > 3660 {
			return nil, 0, fail(400, "days must be 1–3660")
		}
	}
	revenue, e := commerceRows(ctx, s.Store.Pool, `WITH sales AS (
 SELECT currency,sum(amount) revenue,count(*) orders,sum(COALESCE((snapshot->>'cost_amount')::numeric,0)) unit_cost FROM vr_orders
 WHERE owner_id=$1 AND kind<>'topup' AND NOT COALESCE((snapshot->>'_sandbox')::boolean,false) AND paid_at>=now()-$2*interval '1 day' GROUP BY currency
 ), costs AS (SELECT currency,sum(amount) cost FROM vr_cost_entries WHERE owner_id=$1 AND incurred_at>=now()-$2*interval '1 day' GROUP BY currency)
 SELECT jsonb_build_object('currency',COALESCE(s.currency,c.currency),'revenue',COALESCE(revenue,0)::text,'orders',COALESCE(orders,0),'unit_cost',COALESCE(unit_cost,0)::text,'operating_cost',COALESCE(cost,0)::text,'profit',(COALESCE(revenue,0)-COALESCE(unit_cost,0)-COALESCE(cost,0))::text)
 FROM sales s FULL JOIN costs c ON c.currency=s.currency ORDER BY COALESCE(s.currency,c.currency)`, owner, days)
	if e != nil {
		return nil, 0, e
	}
	costs, e := commerceRows(ctx, s.Store.Pool, `SELECT jsonb_build_object('currency',currency,'cost',sum(amount)::text) FROM vr_cost_entries WHERE owner_id=$1 AND incurred_at>=now()-$2*interval '1 day' GROUP BY currency`, owner, days)
	if e != nil {
		return nil, 0, e
	}
	byOffer, e := commerceRows(ctx, s.Store.Pool, `SELECT jsonb_build_object('offer_id',offer_id,'currency',currency,'orders',count(*),'revenue',sum(amount)::text) FROM vr_orders WHERE owner_id=$1 AND kind<>'topup' AND NOT COALESCE((snapshot->>'_sandbox')::boolean,false) AND paid_at>=now()-$2*interval '1 day' GROUP BY offer_id,currency ORDER BY sum(amount) DESC`, owner, days)
	if e != nil {
		return nil, 0, e
	}
	byNode, e := commerceRows(ctx, s.Store.Pool, `WITH sales AS (
 SELECT (a->>'node_id')::bigint node_id,currency,sum((a->>'amount')::numeric) revenue,sum((a->>'cost_amount')::numeric) unit_cost FROM vr_orders o CROSS JOIN LATERAL jsonb_array_elements(COALESCE(snapshot->'node_allocations','[]')) a
 WHERE owner_id=$1 AND kind<>'topup' AND NOT COALESCE((snapshot->>'_sandbox')::boolean,false) AND paid_at>=now()-$2*interval '1 day' GROUP BY (a->>'node_id')::bigint,currency
 ), costs AS (SELECT node_id,currency,sum(amount) cost FROM vr_cost_entries WHERE owner_id=$1 AND incurred_at>=now()-$2*interval '1 day' GROUP BY node_id,currency)
 SELECT jsonb_build_object('node_id',COALESCE(s.node_id,c.node_id),'currency',COALESCE(s.currency,c.currency),'revenue',COALESCE(revenue,0)::text,'unit_cost',COALESCE(unit_cost,0)::text,'cost',COALESCE(cost,0)::text,'profit',(COALESCE(revenue,0)-COALESCE(unit_cost,0)-COALESCE(cost,0))::text)
 FROM sales s FULL JOIN costs c ON c.currency=s.currency AND c.node_id IS NOT DISTINCT FROM s.node_id`, owner, days)
	if e != nil {
		return nil, 0, e
	}
	bySeller, e := commerceRows(ctx, s.Store.Pool, `SELECT jsonb_build_object('owner_id',owner_id,'username',u.data->>'username','currency',currency,'orders',count(*),'revenue',sum(amount)::text) FROM vr_orders o JOIN vr_users u ON u.id=o.owner_id WHERE (owner_id=$1 OR $3::boolean) AND kind<>'topup' AND NOT COALESCE((snapshot->>'_sandbox')::boolean,false) AND paid_at>=now()-$2*interval '1 day' GROUP BY owner_id,u.data->>'username',currency ORDER BY sum(amount) DESC`, owner, days, commerceAdmin(a))
	if e != nil {
		return nil, 0, e
	}
	return Data{"days": days, "summary": revenue, "costs": costs, "by_offer": byOffer, "by_node": byNode, "by_seller": bySeller, "node_allocation": "equal shares across the offer's nodes at purchase time; not traffic-derived revenue"}, 200, nil
}
