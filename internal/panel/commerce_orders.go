package panel

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Server) shopAPI(w http.ResponseWriter, r *http.Request, path string) (any, int, error) {
	ctx := r.Context()
	if path == "shop/catalog" && r.Method == "GET" {
		id, e := strconv.ParseInt(r.URL.Query().Get("store"), 10, 64)
		if e != nil || id < 1 {
			return nil, 0, fail(404, "store not found")
		}
		d, e := getStore(ctx, s.Store.Pool, id)
		if e != nil {
			return nil, 0, e
		}
		if !flag(d, "enabled") {
			return nil, 0, fail(404, "store not found")
		}
		if _, e = seller(ctx, s.Store.Pool, id); e != nil {
			return nil, 0, e
		}
		all, e := offers(ctx, s.Store.Pool, id, true)
		if e != nil {
			return nil, 0, e
		}
		out := []Data{}
		for _, v := range all {
			if s.validateOffer(ctx, s.Store.Pool, v) == nil {
				delete(v, "cost_amount")
				delete(v, "service_id")
				out = append(out, v)
			}
		}
		cfg, e := settings(ctx, s.Store.Pool)
		if e != nil {
			return nil, 0, e
		}
		d["owner_id"] = id
		d["offers"] = out
		d["payments_enabled"] = flag(cfg, "commerce_payments_enabled") && str(cfg, "_commerce_merchant") != ""
		return d, 200, nil
	}
	if path == "shop/orders" && r.Method == "POST" {
		if e := rateLimit(ctx, s.Store.Pool, "shop:create:"+clientIP(r), 12, time.Hour); e != nil {
			return nil, 0, e
		}
		d, e := readBody(w, r)
		if e != nil {
			return nil, 0, e
		}
		if e = allowed(d, "offer_id name kind customer_token"); e != nil {
			return nil, 0, fail(400, e.Error())
		}
		o, e := s.createOrder(ctx, d, 0, true)
		if e != nil {
			return nil, 0, e
		}
		return o, 201, nil
	}
	if strings.HasPrefix(path, "shop/orders/") {
		parts := strings.Split(path, "/")
		if len(parts) < 3 || len(parts) > 4 {
			return nil, 0, fail(404, "order not found")
		}
		id := parts[2]
		d, e := getOrder(ctx, s.Store.Pool, id, false)
		if e != nil {
			return nil, 0, e
		}
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		hash := digest(token)
		if len(token) < 32 || subtle.ConstantTimeCompare([]byte(hash), []byte(str(d, "_token_hash"))) != 1 {
			return nil, 0, fail(404, "order not found")
		}
		if len(parts) == 3 && r.Method == "GET" {
			out := orderView(d)
			if str(d, "state") == "fulfilled" && num(d, "customer_id") > 0 {
				c, e := get(ctx, s.Store.Pool, "customers", num(d, "customer_id"))
				if e != nil {
					return nil, 0, e
				}
				out["subscription_url"], e = s.subscriptionURL(ctx, s.Store.Pool, str(c, "subscription_token"))
				if e != nil {
					return nil, 0, e
				}
			}
			return out, 200, nil
		}
		if len(parts) == 4 && parts[3] == "pay" && r.Method == "POST" {
			return s.requestPayment(ctx, id)
		}
		if len(parts) == 4 && parts[3] == "verify" && r.Method == "POST" {
			if e = rateLimit(ctx, s.Store.Pool, "shop:verify:"+id, 20, time.Hour); e != nil {
				return nil, 0, e
			}
			e = s.verifyPayment(ctx, id, "")
			if e != nil {
				return nil, 0, e
			}
			d, e = getOrder(ctx, s.Store.Pool, id, false)
			return orderView(d), 200, e
		}
	}
	return nil, 0, fail(404, "endpoint not found")
}
func (s *Server) createOrder(ctx context.Context, input Data, payer int64, publicShop bool) (Data, error) {
	if !oneOf(str(input, "kind"), "purchase", "renewal", "traffic") {
		return nil, fail(400, "choose purchase, renewal or traffic")
	}
	if str(input, "kind") == "purchase" {
		if e := require(input, "name", 80); e != nil {
			return nil, fail(400, e.Error())
		}
	}
	tx, e := s.Store.Pool.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(842100,2)`); e != nil {
		return nil, e
	}
	requestRaw, _ := json.Marshal(input)
	requestHash := digest(string(requestRaw))
	if payer > 0 {
		ref := str(input, "reference")
		if len(ref) < 12 || len(ref) > 160 {
			return nil, fail(400, "unique purchase reference required")
		}
		var existing string
		err := tx.QueryRow(ctx, `SELECT id FROM vr_orders WHERE payer_id=$1 AND client_reference=$2`, payer, ref).Scan(&existing)
		if err == nil {
			d, e := getOrder(ctx, tx, existing, false)
			if e != nil {
				return nil, e
			}
			if str(obj(d, "snapshot"), "_request_hash") != requestHash {
				return nil, fail(409, "purchase reference was already used with different details")
			}
			return orderView(d), nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}
	v, e := offer(ctx, tx, num(input, "offer_id"))
	if e != nil {
		return nil, e
	}
	if !flag(v, "enabled") {
		return nil, fail(409, "offer is unavailable")
	}
	if e = s.validateOffer(ctx, tx, v); e != nil {
		return nil, e
	}
	owner := num(v, "owner_id")
	if publicShop {
		store, e := getStore(ctx, tx, owner)
		if e != nil {
			return nil, e
		}
		if !flag(store, "enabled") {
			return nil, fail(404, "store not found")
		}
	}
	service, e := get(ctx, tx, "services", num(v, "service_id"))
	if e != nil {
		return nil, e
	}
	snapshot := clone(v)
	snapshot["defaults"] = clone(obj(service, "defaults"))
	snapshot["inbound_ids"] = service["inbound_ids"]
	snapshot["customer_name"] = str(input, "name")
	snapshot["_request_hash"] = requestHash
	inboundIDs, e := idArray(snapshot, "inbound_ids", 200)
	if e != nil {
		return nil, e
	}
	nodes, e := nodesForInbounds(ctx, tx, inboundIDs)
	if e != nil {
		return nil, e
	}
	allocations := []any{}
	for index, node := range nodes {
		count := int64(len(nodes))
		amount := num(v, "amount") / count
		cost := num(v, "cost_amount") / count
		if int64(index) < num(v, "amount")%count {
			amount++
		}
		if int64(index) < num(v, "cost_amount")%count {
			cost++
		}
		allocations = append(allocations, Data{"node_id": node, "amount": amount, "cost_amount": cost})
	}
	snapshot["node_allocations"] = allocations
	var customer any
	if str(input, "kind") != "purchase" {
		token := str(input, "customer_token")
		if len(token) < 20 || len(token) > 128 {
			return nil, fail(400, "valid customer subscription token required")
		}
		var raw []byte
		e = tx.QueryRow(ctx, `SELECT data||jsonb_build_object('id',id) FROM vr_accounts WHERE data->>'subscription_token'=$1`, token).Scan(&raw)
		if e != nil {
			return nil, fail(404, "customer not found")
		}
		c, e := decode(raw)
		if e != nil {
			return nil, e
		}
		if num(c, "owner_id") != owner {
			return nil, fail(404, "customer not found")
		}
		if payer > 0 && payer != owner {
			return nil, fail(403, "wallet renewal is limited to your customers")
		}
		found := false
		for _, value := range array(c, "service_ids") {
			if num(Data{"id": value}, "id") == num(v, "service_id") {
				found = true
			}
		}
		if !found {
			return nil, fail(409, "renewal offer must use the customer's existing service")
		}
		if str(c, "disabled_reason") == "frozen" {
			return nil, fail(409, "resume the subscription before renewing")
		}
		customer = num(c, "id")
		snapshot["customer_name"] = str(c, "name")
		if str(input, "kind") == "traffic" && num(v, "traffic_bytes") == 0 {
			return nil, fail(400, "traffic top-up requires a finite traffic grant")
		}
	}
	id, token := randomToken(24), randomToken(32)
	raw, _ := json.Marshal(snapshot)
	var payerValue any
	if payer > 0 {
		payerValue = payer
	}
	var reference any
	if payer > 0 {
		reference = str(input, "reference")
	}
	_, e = tx.Exec(ctx, `INSERT INTO vr_orders(id,owner_id,payer_id,offer_id,customer_id,kind,amount,currency,token_hash,snapshot,client_reference) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, id, owner, payerValue, num(v, "id"), customer, str(input, "kind"), num(v, "amount"), str(v, "currency"), digest(token), string(raw), reference)
	if e != nil {
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	return Data{"id": id, "token": token, "state": "pending", "amount": num(v, "amount"), "currency": str(v, "currency"), "owner_id": owner}, nil
}

func (s *Server) walletPurchase(w http.ResponseWriter, r *http.Request, a Actor) (any, int, error) {
	if !permitted(a.User, "commerce.write") {
		return nil, 0, fail(403, "commerce write access required")
	}
	input, e := readBody(w, r)
	if e != nil {
		return nil, 0, e
	}
	if e = allowed(input, "offer_id name kind customer_token reference"); e != nil {
		return nil, 0, fail(400, e.Error())
	}
	ctx := r.Context()
	o, e := s.createOrder(ctx, input, num(a.User, "id"), false)
	if e != nil {
		return nil, 0, e
	}
	id := str(o, "id")
	_, e = s.managementChange(ctx, nil, a, "commerce.wallet_purchase", id, func(tx pgx.Tx) (any, error) {
		d, e := getOrder(ctx, tx, id, true)
		if e != nil {
			return nil, e
		}
		if str(d, "state") != "pending" {
			if oneOf(str(d, "state"), "paid", "fulfilled") {
				return nil, nil
			}
			return nil, fail(409, "order is not pending")
		}
		_, e = walletEntry(ctx, tx, num(a.User, "id"), str(d, "currency"), -num(d, "amount"), "order:"+id, "Order "+id, num(a.User, "id"))
		if e != nil {
			return nil, e
		}
		_, e = tx.Exec(ctx, `UPDATE vr_orders SET state='paid',gateway='wallet',payment_reference=$2,paid_at=now() WHERE id=$1`, id, id)
		return nil, e
	})
	if e != nil {
		return nil, 0, e
	}
	_ = s.fulfillOrder(ctx, id)
	d, e := getOrder(ctx, s.Store.Pool, id, false)
	out := orderView(d)
	out["token"] = o["token"]
	return out, 201, e
}
func (s *Server) walletTopup(w http.ResponseWriter, r *http.Request, a Actor) (any, int, error) {
	if !permitted(a.User, "commerce.write") {
		return nil, 0, fail(403, "commerce write access required")
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, 0, e
	}
	amount, e := money(d, "amount", 1)
	if e != nil {
		return nil, 0, e
	}
	currency := str(d, "currency")
	if !oneOf(currency, "IRR", "IRT") {
		return nil, 0, fail(400, "payment gateway supports IRR and IRT")
	}
	id, token := randomToken(24), randomToken(32)
	_, e = s.Store.Pool.Exec(r.Context(), `INSERT INTO vr_orders(id,owner_id,payer_id,kind,amount,currency,token_hash,snapshot) VALUES($1,$2,$2,'topup',$3,$4,$5,$6)`, id, num(a.User, "id"), amount, currency, digest(token), `{"name":"Wallet top-up"}`)
	return Data{"id": id, "token": token, "state": "pending", "amount": amount, "currency": currency}, 201, e
}

// A paid order and its customer mutation commit together, after agents validate
// and apply their journaled configuration. Failed deployment leaves it paid.
func (s *Server) fulfillOrder(ctx context.Context, id string) error {
	o, e := getOrder(ctx, s.Store.Pool, id, false)
	if e != nil {
		return e
	}
	if str(o, "state") == "fulfilled" {
		return nil
	}
	if str(o, "state") != "paid" {
		return fail(409, "order has not been paid")
	}
	if flag(obj(o, "snapshot"), "_sandbox") {
		return fail(409, "test payments never create wallet credit or provision customers")
	}
	if str(o, "kind") == "topup" {
		_, e = s.managementChange(ctx, nil, Actor{}, "commerce.topup", id, func(tx pgx.Tx) (any, error) {
			d, e := getOrder(ctx, tx, id, true)
			if e != nil {
				return nil, e
			}
			if str(d, "state") == "fulfilled" {
				return nil, nil
			}
			if str(d, "state") != "paid" {
				return nil, fail(409, "order has not been paid")
			}
			_, e = walletEntry(ctx, tx, num(d, "payer_id"), str(d, "currency"), num(d, "amount"), "topup:"+id, "Verified payment "+id, 0)
			if e != nil {
				return nil, e
			}
			_, e = tx.Exec(ctx, `UPDATE vr_orders SET state='fulfilled',fulfilled_at=now(),last_error='' WHERE id=$1`, id)
			return nil, e
		})
		return e
	}
	snapshot := obj(o, "snapshot")
	ids, e := idArray(snapshot, "inbound_ids", 200)
	if e != nil {
		return e
	}
	if cid := num(o, "customer_id"); cid > 0 {
		members, e := customerMembers(ctx, s.Store.Pool, cid)
		if e != nil {
			return e
		}
		for _, c := range members {
			ids = append(ids, num(c, "inbound"))
		}
	}
	nodes, e := nodesForInbounds(ctx, s.Store.Pool, nodeIDs(ids...))
	if e != nil {
		return e
	}
	_, e = s.managementChange(ctx, nodes, Actor{}, "commerce.fulfilled", id, func(tx pgx.Tx) (any, error) {
		d, e := getOrder(ctx, tx, id, true)
		if e != nil {
			return nil, e
		}
		if str(d, "state") == "fulfilled" {
			return nil, nil
		}
		if str(d, "state") != "paid" {
			return nil, fail(409, "order has not been paid")
		}
		u, e := seller(ctx, tx, num(d, "owner_id"))
		if e != nil {
			return nil, e
		}
		actor := Actor{User: u, Scope: "admin"}
		snap := obj(d, "snapshot")
		if str(d, "kind") != "purchase" && num(d, "customer_id") <= 0 {
			return nil, fail(409, "renewal customer was deleted; review this paid order")
		}
		c := customerDefaults()
		if cid := num(d, "customer_id"); cid > 0 {
			c, e = get(ctx, tx, "customers", cid)
			if e != nil {
				return nil, e
			}
			if num(c, "owner_id") != num(d, "owner_id") {
				return nil, fail(409, "customer owner changed; review the order")
			}
			if str(c, "disabled_reason") == "frozen" {
				return nil, fail(409, "resume the subscription before renewing")
			}
		}
		now := time.Now().UTC()
		kind := str(d, "kind")
		grant := num(snap, "traffic_bytes")
		if kind == "purchase" {
			c = merge(c, obj(snap, "defaults"))
			c["name"] = str(snap, "customer_name") + "-" + id[:6]
			c["owner_id"] = num(d, "owner_id")
			c["service_ids"] = []any{num(snap, "service_id")}
			c["inbound_ids"] = []any{}
			c["plan_id"] = num(snap, "service_id")
			c["traffic_limit_bytes"] = grant
			c["subscription_token"] = randomToken(32)
			c["duration_days"] = num(snap, "duration_days")
			c["renewal_interval_days"] = 0
			c["renewal_period"] = "none"
			c["next_renewal_at"] = nil
			if !flag(c, "start_after_first_use") {
				c["expires_at"] = stamp(now.AddDate(0, 0, int(num(snap, "duration_days"))))
			}
		} else {
			if grant > 0 && num(c, "traffic_limit_bytes") > 0 {
				if num(c, "traffic_limit_bytes") > (1<<62)-grant {
					return nil, fail(400, "traffic grant exceeds supported range")
				}
				c["traffic_limit_bytes"] = num(c, "traffic_limit_bytes") + grant
			} else if kind == "renewal" && grant == 0 {
				c["traffic_limit_bytes"] = int64(0)
			}
			if kind == "renewal" {
				anchor := now
				if expiry, err := timestamp(c, "expires_at"); err == nil && expiry.After(anchor) {
					anchor = expiry
				}
				if flag(c, "start_after_first_use") && str(c, "first_used_at") == "" {
					days := num(c, "duration_days") + num(snap, "duration_days")
					if days > 36500 {
						return nil, fail(400, "deferred duration exceeds supported range")
					}
					c["duration_days"] = days
				} else {
					c["expires_at"] = stamp(anchor.AddDate(0, 0, int(num(snap, "duration_days"))))
				}
			}
		}
		if kind == "purchase" || oneOf(str(c, "disabled_reason"), "quota", "expired") {
			c["enabled"] = true
			c["disabled_reason"] = ""
		}
		if e = save(ctx, tx, "customers", c); e != nil {
			return nil, e
		}
		if e = s.syncCustomer(ctx, tx, actor, c, nodes); e != nil {
			return nil, e
		}
		_, e = tx.Exec(ctx, `UPDATE vr_orders SET state='fulfilled',customer_id=$2,fulfilled_at=now(),last_error='' WHERE id=$1`, id, num(c, "id"))
		return nil, e
	})
	if e != nil {
		_, _ = s.Store.Pool.Exec(context.WithoutCancel(ctx), `UPDATE vr_orders SET attempts=attempts+1,last_error='Customer deployment pending; retry after checking the service and node health',next_attempt_at=now()+least(3600,30*power(2,least(attempts,6))) * interval '1 second' WHERE id=$1 AND state='paid'`, id)
	}
	return e
}
func (s *Server) fulfillPaidOrders(ctx context.Context) error {
	rows, e := s.Store.Pool.Query(ctx, `SELECT id FROM vr_orders WHERE state='paid' AND NOT COALESCE((snapshot->>'_sandbox')::boolean,false) AND next_attempt_at<=now() ORDER BY paid_at LIMIT 5`)
	if e != nil {
		return e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, id := range ids {
		if err := s.fulfillOrder(ctx, id); err != nil && ctx.Err() != nil {
			return err
		}
	}
	return nil
}

func (s *Server) commerceSettings(w http.ResponseWriter, r *http.Request, a Actor) (any, int, error) {
	if !commerceAdmin(a) {
		return nil, 0, fail(403, "administrator access required")
	}
	ctx := r.Context()
	view := func(d Data) Data {
		return Data{"payments_enabled": flag(d, "commerce_payments_enabled"), "merchant_configured": str(d, "_commerce_merchant") != "", "sandbox": flag(d, "commerce_sandbox"), "gateway": "zarinpal"}
	}
	if r.Method == "GET" {
		d, e := settings(ctx, s.Store.Pool)
		return view(d), 200, e
	}
	if r.Method != "PATCH" {
		return nil, 0, fail(405, "method not allowed")
	}
	input, e := readBody(w, r)
	if e != nil {
		return nil, 0, e
	}
	if e = allowed(input, "payments_enabled merchant_id sandbox"); e != nil {
		return nil, 0, fail(400, e.Error())
	}
	if e = typed(input, Data{"payments_enabled": false, "merchant_id": "", "sandbox": false}); e != nil {
		return nil, 0, fail(400, e.Error())
	}
	out, e := s.managementChange(ctx, nil, a, "commerce.gateway", "settings", func(tx pgx.Tx) (any, error) {
		d, e := settings(ctx, tx)
		if e != nil {
			return nil, e
		}
		if v, ok := input["merchant_id"]; ok {
			value := v.(string)
			if value != "" && !merchantPattern.MatchString(value) {
				return nil, fail(400, "merchant_id must be a UUID")
			}
			sealed := ""
			if value != "" {
				sealed, e = seal(s.Config.FieldKey, value)
				if e != nil {
					return nil, e
				}
			}
			d["_commerce_merchant"] = sealed
		}
		if v, ok := input["payments_enabled"]; ok {
			d["commerce_payments_enabled"] = v
		}
		if v, ok := input["sandbox"]; ok {
			d["commerce_sandbox"] = v
		}
		if flag(d, "commerce_payments_enabled") && str(d, "_commerce_merchant") == "" {
			return nil, fail(400, "configure a merchant before enabling payments")
		}
		if flag(d, "commerce_payments_enabled") && !strings.HasPrefix(s.Config.PublicURL, "https://") {
			return nil, fail(400, "payments require a public HTTPS panel URL")
		}
		return view(d), saveSettings(ctx, tx, d)
	})
	return out, 200, e
}
