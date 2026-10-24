package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) salesCustomer(ctx context.Context, owner, user, id int64) (Data, error) {
	var yes bool
	e := s.Store.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vr_telegram_orders t JOIN vr_orders o ON o.id=t.order_id WHERE t.user_id=$1 AND o.owner_id=$2 AND o.customer_id=$3 AND o.state='fulfilled' UNION ALL SELECT 1 FROM vr_bot_trials WHERE user_id=$1 AND owner_id=$2 AND customer_id=$3)`, user, owner, id).Scan(&yes)
	if e != nil {
		return nil, e
	}
	if !yes {
		return nil, fail(404, "subscription not found")
	}
	d, e := get(ctx, s.Store.Pool, "customers", id)
	if e != nil || num(d, "owner_id") != owner {
		return nil, fail(404, "subscription not found")
	}
	return d, nil
}
func (s *Server) salesCustomers(ctx context.Context, owner, user int64) ([]Data, error) {
	rows, e := s.Store.Pool.Query(ctx, `SELECT DISTINCT a.id FROM vr_accounts a WHERE COALESCE((a.data->>'owner_id')::bigint,0)=$1 AND (EXISTS(SELECT 1 FROM vr_telegram_orders t JOIN vr_orders o ON o.id=t.order_id WHERE t.user_id=$2 AND o.owner_id=$1 AND o.customer_id=a.id AND o.state='fulfilled') OR EXISTS(SELECT 1 FROM vr_bot_trials t WHERE t.user_id=$2 AND t.owner_id=$1 AND t.customer_id=a.id)) ORDER BY a.id DESC LIMIT 100`, owner, user)
	if e != nil {
		return nil, e
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return nil, e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	out := []Data{}
	for _, id := range ids {
		c, e := s.salesCustomerView(ctx, owner, user, id)
		if e != nil {
			return nil, e
		}
		out = append(out, c)
	}
	return out, nil
}
func (s *Server) salesCustomerView(ctx context.Context, owner, user, id int64) (Data, error) {
	c, e := s.salesCustomer(ctx, owner, user, id)
	if e != nil {
		return nil, e
	}
	out := Data{}
	for _, k := range strings.Fields("id name service_ids enabled expires_at first_used_at used_traffic_bytes raw_used_traffic_bytes raw_upload_traffic_bytes raw_download_traffic_bytes traffic_limit_bytes disabled_reason") {
		out[k] = c[k]
	}
	out["remaining_bytes"] = nil
	out["unlimited"] = num(c, "traffic_limit_bytes") == 0
	if num(c, "traffic_limit_bytes") > 0 {
		out["remaining_bytes"] = max(int64(0), num(c, "traffic_limit_bytes")-num(c, "used_traffic_bytes"))
	}
	out["subscription_url"], e = s.subscriptionURL(ctx, s.Store.Pool, str(c, "subscription_token"))
	return out, e
}
func (s *Server) salesServiceChange(ctx context.Context, owner, user, id int64, action string) (Data, error) {
	c, e := s.salesCustomer(ctx, owner, user, id)
	if e != nil {
		return nil, e
	}
	store, e := getStore(ctx, s.Store.Pool, owner)
	if e != nil {
		return nil, e
	}
	b := salesConfig(store)
	if oneOf(action, "enable", "disable") && !flag(b, "allow_toggle") || action == "rotate" && !flag(b, "allow_rotate") || !oneOf(action, "enable", "disable", "rotate") {
		return nil, fail(403, "this service action is disabled by the seller")
	}
	u, e := seller(ctx, s.Store.Pool, owner)
	if e != nil {
		return nil, e
	}
	actor := Actor{User: u, Scope: "admin"}
	body := "{}"
	if action != "rotate" {
		raw, _ := json.Marshal(Data{"ids": []any{id}, "action": action})
		body = string(raw)
	}
	req, _ := http.NewRequestWithContext(ctx, "POST", "http://localhost/", strings.NewReader(body))
	if action == "rotate" {
		_, _, e = s.rotateCustomerToken(discardResponse{}, req, num(c, "id"), actor)
	} else {
		_, _, e = s.bulkCustomers(discardResponse{}, req, actor)
	}
	if e != nil {
		return nil, e
	}
	_ = audit(ctx, s.Store.Pool, 0, "shop.service_"+action, fmt.Sprint(id), Data{"telegram_user": user, "owner_id": owner})
	return s.salesCustomerView(ctx, owner, user, id)
}
func (s *Server) claimSalesTrial(ctx context.Context, owner, user int64) (Data, error) {
	store, e := getStore(ctx, s.Store.Pool, owner)
	if e != nil {
		return nil, e
	}
	b := salesConfig(store)
	if !flag(store, "enabled") || !flag(store, "telegram_sales") || !flag(b, "trial_enabled") {
		return nil, fail(409, "free trials are disabled")
	}
	if str(b, "trial_channel") != "" {
		cfg, e := settings(ctx, s.Store.Pool)
		if e != nil {
			return nil, e
		}
		token, e := unseal(s.Config.FieldKey, str(cfg, "_telegram_token"))
		if e != nil {
			return nil, e
		}
		member, e := s.telegramRequest(ctx, token, "getChatMember", Data{"chat_id": str(b, "trial_channel"), "user_id": user})
		if e != nil {
			return nil, fail(409, "channel membership could not be verified; contact support")
		}
		m := obj(member, "result")
		if !oneOf(str(m, "status"), "creator", "administrator", "member") && !(str(m, "status") == "restricted" && flag(m, "is_member")) {
			return nil, fail(403, "join the trial channel and try again")
		}
	}
	if str(b, "trial_mode") == "paid_only" || flag(b, "trial_paid_only") {
		var paid bool
		e = s.Store.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vr_orders o JOIN vr_telegram_orders t ON t.order_id=o.id WHERE o.owner_id=$1 AND t.user_id=$2 AND o.state='fulfilled')`, owner, user).Scan(&paid)
		if e != nil {
			return nil, e
		}
		if !paid {
			return nil, fail(403, "this trial is available to existing customers only")
		}
	}
	svc, e := get(ctx, s.Store.Pool, "services", num(b, "trial_service_id"))
	if e != nil {
		return nil, e
	}
	ids, e := idArray(svc, "inbound_ids", 200)
	if e != nil {
		return nil, e
	}
	nodes, e := nodesForInbounds(ctx, s.Store.Pool, ids)
	if e != nil {
		return nil, e
	}
	var customer int64
	_, e = s.managementChange(ctx, nodes, Actor{}, "shop.trial", fmt.Sprint(user), func(tx pgx.Tx) (any, error) {
		// The shared management lock and unique claim protect simultaneous retries.
		fresh, err := getStore(ctx, tx, owner)
		if err != nil {
			return nil, err
		}
		if !flag(fresh, "enabled") || !flag(fresh, "telegram_sales") || !flag(salesConfig(fresh), "trial_enabled") {
			return nil, fail(409, "trials are disabled")
		}
		raw1, _ := json.Marshal(salesConfig(fresh))
		raw2, _ := json.Marshal(b)
		if string(raw1) != string(raw2) {
			return nil, fail(409, "trial settings changed; retry")
		}
		var previous *int64
		err = tx.QueryRow(ctx, `SELECT customer_id FROM vr_bot_trials WHERE owner_id=$1 AND user_id=$2`, owner, user).Scan(&previous)
		if err == nil {
			if previous == nil {
				return nil, fail(409, "your free trial was already used")
			}
			customer = *previous
			return nil, nil
		}
		if err != pgx.ErrNoRows {
			return nil, err
		}
		u, err := seller(ctx, tx, owner)
		if err != nil {
			return nil, err
		}
		if !visibleService(Actor{User: u}, svc) {
			return nil, fail(403, "trial service unavailable")
		}
		c := merge(customerDefaults(), obj(svc, "defaults"))
		c["name"] = fmt.Sprintf("trial-%d-%s", user, randomToken(4))
		c["owner_id"] = owner
		c["service_ids"] = []any{num(svc, "id")}
		c["inbound_ids"] = []any{}
		c["traffic_limit_bytes"] = num(b, "trial_traffic_bytes")
		c["subscription_token"] = randomToken(32)
		c["start_after_first_use"] = false
		c["expires_at"] = stamp(time.Now().Add(time.Duration(num(b, "trial_hours")) * time.Hour))
		c["enabled"] = true
		c["renewal_period"] = "none"
		c["renewal_interval_days"] = 0
		c["next_renewal_at"] = nil
		if err = save(ctx, tx, "customers", c); err != nil {
			return nil, err
		}
		if err = s.syncCustomer(ctx, tx, Actor{User: u, Scope: "admin"}, c, nodes); err != nil {
			return nil, err
		}
		customer = num(c, "id")
		_, err = tx.Exec(ctx, `INSERT INTO vr_bot_trials(owner_id,user_id,customer_id) VALUES($1,$2,$3)`, owner, user, customer)
		return nil, err
	})
	if e != nil {
		return nil, e
	}
	return s.salesCustomerView(ctx, owner, user, customer)
}
func (s *Server) shopCustomerAPI(w http.ResponseWriter, r *http.Request, path string) (any, int, error) {
	ctx := r.Context()
	owner, e := strconv.ParseInt(r.URL.Query().Get("store"), 10, 64)
	if e != nil || owner <= 0 {
		return nil, 0, fail(404, "store not found")
	}
	user, e := s.miniAppBuyer(ctx, s.Store.Pool, r.Header.Get("X-Telegram-Init-Data"))
	if e != nil {
		return nil, 0, e
	}
	if user <= 0 {
		return nil, 0, fail(401, "open the Mini App through Telegram")
	}
	store, e := getStore(ctx, s.Store.Pool, owner)
	if e != nil {
		return nil, 0, e
	}
	if !flag(store, "enabled") || !flag(store, "telegram_sales") {
		return nil, 0, fail(404, "store unavailable")
	}
	if _, e = seller(ctx, s.Store.Pool, owner); e != nil {
		return nil, 0, e
	}
	var bound bool
	e = s.Store.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vr_telegram_customers WHERE owner_id=$1 AND user_id=$2)`, owner, user).Scan(&bound)
	if e != nil {
		return nil, 0, e
	}
	if !bound {
		return nil, 0, fail(403, "start this seller's bot first")
	}
	if e = rateLimit(ctx, s.Store.Pool, fmt.Sprintf("shop:me:%d:%d", owner, user), 120, time.Minute); e != nil {
		return nil, 0, e
	}
	if path == "shop/me" && r.Method == "GET" {
		cs, e := s.salesCustomers(ctx, owner, user)
		if e != nil {
			return nil, 0, e
		}
		return Data{"user_id": user, "services": cs, "bot": publicSalesConfig(store)}, 200, nil
	}
	if path == "shop/me/trial" && r.Method == "POST" {
		d, e := readBody(w, r)
		if e != nil {
			return nil, 0, e
		}
		if !flag(d, "accept_terms") {
			return nil, 0, fail(400, "accept the store terms first")
		}
		out, e := s.claimSalesTrial(ctx, owner, user)
		return out, 200, e
	}
	if strings.HasPrefix(path, "shop/me/tickets") {
		return s.buyerTicketsAPI(w, r, path, owner, user)
	}
	if path == "shop/me/orders" && r.Method == "GET" {
		rows, e := commerceRows(ctx, s.Store.Pool, orderSelect+`WHERE owner_id=$1 AND id IN(SELECT order_id FROM vr_telegram_orders WHERE user_id=$2) ORDER BY created_at DESC LIMIT 100`, owner, user)
		out := []Data{}
		for _, d := range rows {
			v := orderView(d)
			var sealed string
			if e = s.Store.Pool.QueryRow(ctx, `SELECT token_ciphertext FROM vr_telegram_orders WHERE order_id=$1 AND user_id=$2`, str(d, "id"), user).Scan(&sealed); e != nil {
				return nil, 0, e
			}
			token, err := unseal(s.Config.FieldKey, sealed)
			if err != nil {
				return nil, 0, err
			}
			v["token"] = token
			out = append(out, v)
		}
		return out, 200, e
	}
	parts := strings.Split(path, "/")
	if len(parts) == 5 && parts[2] == "services" && r.Method == "POST" {
		id, e := strconv.ParseInt(parts[3], 10, 64)
		if e != nil {
			return nil, 0, fail(404, "subscription not found")
		}
		d, e := readBody(w, r)
		if e != nil {
			return nil, 0, e
		}
		if !flag(d, "confirm") {
			return nil, 0, fail(400, "confirm this service change")
		}
		out, e := s.salesServiceChange(ctx, owner, user, id, parts[4])
		return out, 200, e
	}
	return nil, 0, fail(404, "endpoint not found")
}
