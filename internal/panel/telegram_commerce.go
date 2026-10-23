package panel

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type telegramBuyerKey struct{}

func telegramBotID(token string) string {
	if parts := strings.SplitN(token, ":", 2); len(parts) == 2 {
		return parts[0]
	}
	return digest(token)
}

// User IDs are accepted only from the raw query signed by Telegram.
func validateMiniApp(raw, token string, now time.Time) (int64, error) {
	invalid := func() (int64, error) { return 0, fail(401, "reopen the store from its Telegram bot") }
	if len(raw) == 0 || len(raw) > 16384 || token == "" {
		return invalid()
	}
	v, e := url.ParseQuery(raw)
	if e != nil {
		return invalid()
	}
	lines := []string{}
	for k, values := range v {
		if len(values) != 1 {
			return invalid()
		}
		if k != "hash" {
			lines = append(lines, k+"="+values[0])
		}
	}
	sort.Strings(lines)
	supplied, e := hex.DecodeString(v.Get("hash"))
	if e != nil || len(supplied) != 32 {
		return invalid()
	}
	key := hmac.New(sha256.New, []byte("WebAppData"))
	key.Write([]byte(token))
	mac := hmac.New(sha256.New, key.Sum(nil))
	mac.Write([]byte(strings.Join(lines, "\n")))
	if !hmac.Equal(supplied, mac.Sum(nil)) {
		return invalid()
	}
	seconds, e := strconv.ParseInt(v.Get("auth_date"), 10, 64)
	if e != nil || seconds > now.Add(30*time.Second).Unix() || seconds < now.Add(-time.Hour).Unix() {
		return invalid()
	}
	user, e := decode([]byte(v.Get("user")))
	if e != nil || num(user, "id") <= 0 || num(user, "id") > 1<<52 || flag(user, "is_bot") {
		return invalid()
	}
	return num(user, "id"), nil
}
func (s *Server) miniAppBuyer(ctx context.Context, q Query, raw string) (int64, error) {
	if raw == "" {
		return 0, nil
	}
	cfg, e := settings(ctx, q)
	if e != nil {
		return 0, e
	}
	if !flag(cfg, "telegram_enabled") {
		return 0, fail(409, "Telegram sales are unavailable")
	}
	token, e := unseal(s.Config.FieldKey, str(cfg, "_telegram_token"))
	if e != nil {
		return 0, fail(409, "Telegram sales are unavailable")
	}
	return validateMiniApp(raw, token, time.Now())
}
func telegramStoreURL(cfg Data, owner int64) string {
	name := str(cfg, "_telegram_username")
	if name == "" || !flag(cfg, "telegram_enabled") {
		return ""
	}
	return "https://t.me/" + url.PathEscape(name) + "?start=shop_" + strconv.FormatInt(owner, 10)
}
func (s *Server) telegramSalesRoute(ctx context.Context, text string, message, from Data) bool {
	if str(obj(message, "chat"), "type") != "private" || num(obj(message, "chat"), "id") != num(from, "id") || num(from, "id") <= 0 {
		return false
	}
	if strings.HasPrefix(text, "/start shop_") || strings.HasPrefix(text, "vs:shop_") {
		return true
	}
	if !oneOf(text, "/shop", "/orders", "/paysupport", "/terms") {
		return false
	}
	var bound bool
	e := s.Store.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vr_telegram_customers WHERE user_id=$1)`, num(from, "id")).Scan(&bound)
	return e == nil && bound
}
func (s *Server) telegramSalesReply(ctx context.Context, cfg Data, text string, user int64) Data {
	unavailable := Data{"text": botText(cfg, "This store is unavailable.", "این فروشگاه در دسترس نیست.")}
	var owner int64
	if strings.HasPrefix(text, "/start shop_") || strings.HasPrefix(text, "vs:shop_") {
		id := strings.TrimPrefix(strings.TrimPrefix(text, "/start shop_"), "vs:shop_")
		owner, _ = strconv.ParseInt(id, 10, 64)
	} else {
		_ = s.Store.Pool.QueryRow(ctx, `SELECT owner_id FROM vr_telegram_customers WHERE user_id=$1 ORDER BY last_seen_at DESC LIMIT 1`, user).Scan(&owner)
	}
	d, e := getStore(ctx, s.Store.Pool, owner)
	if e != nil || !flag(d, "enabled") || !flag(d, "telegram_sales") {
		return unavailable
	}
	if _, e = seller(ctx, s.Store.Pool, owner); e != nil {
		return unavailable
	}
	if !strings.HasPrefix(s.Config.PublicURL, "https://") {
		return unavailable
	}
	if _, e = s.Store.Pool.Exec(ctx, `INSERT INTO vr_telegram_customers(user_id,owner_id) VALUES($1,$2) ON CONFLICT(user_id,owner_id) DO UPDATE SET last_seen_at=now()`, user, owner); e != nil {
		return unavailable
	}
	shop := strings.TrimRight(s.Config.PublicURL, "/") + "/shop?store=" + fmt.Sprint(owner)
	reply := Data{"text": str(d, "name") + "\n" + botText(cfg, "Buy, renew or add traffic in the store.\n/shop · /orders · /paysupport · /terms", "خرید، تمدید و افزایش حجم از فروشگاه\n/shop · /orders · /paysupport · /terms"), "reply_markup": botKeyboard([]any{Data{"text": botText(cfg, "Open store", "باز کردن فروشگاه"), "web_app": Data{"url": shop}}})}
	if text == "/paysupport" || text == "/terms" {
		key := "support_url"
		if text == "/terms" {
			key = "terms_url"
		}
		reply["text"] = str(d, "name") + "\n" + str(d, key)
	}
	if text == "/orders" {
		rows, e := s.Store.Pool.Query(ctx, `SELECT t.order_id,t.token_ciphertext,o.state FROM vr_telegram_orders t JOIN vr_orders o ON o.id=t.order_id WHERE t.user_id=$1 AND o.owner_id=$2 ORDER BY t.created_at DESC LIMIT 8`, user, owner)
		if e != nil {
			return unavailable
		}
		defer rows.Close()
		buttons := [][]any{}
		for rows.Next() {
			var id, sealed, state string
			if rows.Scan(&id, &sealed, &state) != nil {
				return unavailable
			}
			token, e := unseal(s.Config.FieldKey, sealed)
			if e != nil {
				return unavailable
			}
			buttons = append(buttons, []any{Data{"text": state + " · " + id[:8], "url": privateOrderURL(s.Config.PublicURL, owner, id, token)}})
		}
		if rows.Err() != nil {
			return unavailable
		}
		reply["text"] = botText(cfg, "Your recent orders", "آخرین سفارش‌های شما")
		reply["reply_markup"] = Data{"inline_keyboard": buttons}
	}
	return reply
}
func privateOrderURL(origin string, owner int64, id, token string) string {
	return strings.TrimRight(origin, "/") + "/shop?store=" + fmt.Sprint(owner) + "&order=" + id + "#token=" + url.QueryEscape(token)
}

func (s *Server) requestStarsPayment(ctx context.Context, id string) (any, int, error) {
	tx, e := s.Store.Pool.Begin(ctx)
	if e != nil {
		return nil, 0, e
	}
	defer tx.Rollback(ctx)
	o, e := getOrder(ctx, tx, id, true)
	if e != nil {
		return nil, 0, e
	}
	if str(o, "state") != "pending" || str(o, "currency") != "XTR" || num(obj(o, "snapshot"), "_telegram_user") <= 0 {
		return nil, 0, fail(409, "open this Stars offer through its Telegram store")
	}
	if expiry, e := timestamp(o, "expires_at"); e != nil || !expiry.After(time.Now()) {
		return nil, 0, fail(409, "checkout expired; create a new order")
	}
	if e = s.recheckOrderCapacity(ctx, tx, o); e != nil {
		return nil, 0, e
	}
	cfg, e := settings(ctx, tx)
	if e != nil {
		return nil, 0, e
	}
	if !flag(cfg, "telegram_enabled") {
		return nil, 0, fail(409, "Telegram sales are unavailable")
	}
	store, e := getStore(ctx, tx, num(o, "owner_id"))
	if e != nil {
		return nil, 0, e
	}
	if !flag(store, "telegram_sales") || !flag(store, "enabled") {
		return nil, 0, fail(409, "store is unavailable")
	}
	snap := obj(o, "snapshot")
	token, e := unseal(s.Config.FieldKey, str(cfg, "_telegram_token"))
	if e != nil || token == "" {
		return nil, 0, fail(409, "Telegram sales are unavailable")
	}
	if str(snap, "_telegram_bot_id") != telegramBotID(token) {
		return nil, 0, fail(409, "the store bot changed; create a new invoice")
	}
	if link := str(snap, "_invoice_url"); link != "" {
		return Data{"url": link, "gateway": "telegram-stars"}, 200, nil
	}
	nonce := randomToken(18)
	payload := "vr:" + id + ":" + nonce
	title := []rune(str(snap, "name"))
	if len(title) > 32 {
		title = title[:32]
	}
	res, e := s.telegramRequest(ctx, token, "createInvoiceLink", Data{"title": string(title), "description": "VeloRay · " + str(snap, "name"), "payload": payload, "provider_token": "", "currency": "XTR", "prices": []any{Data{"label": string(title), "amount": num(o, "amount")}}})
	if e != nil {
		return nil, 0, fail(502, "Telegram invoice could not be created")
	}
	link := str(res, "result")
	u, e := url.Parse(link)
	if e != nil || u.Scheme != "https" || u.Host != "t.me" || u.User != nil || !strings.HasPrefix(u.Path, "/$") {
		return nil, 0, fail(502, "invalid Telegram invoice link")
	}
	snap["_invoice_nonce"], snap["_invoice_url"] = nonce, link
	raw, _ := json.Marshal(snap)
	if _, e = tx.Exec(ctx, `UPDATE vr_orders SET gateway='telegram-stars',authority=$2,snapshot=$3 WHERE id=$1`, id, payload, string(raw)); e != nil {
		return nil, 0, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, 0, e
	}
	return Data{"url": link, "gateway": "telegram-stars"}, 200, nil
}

func (s *Server) acceptStarsPayment(ctx context.Context, payment Data, user int64, paid bool, botID string) error {
	payload := str(payment, "invoice_payload")
	parts := strings.Split(payload, ":")
	if len(parts) != 3 || parts[0] != "vr" {
		return fail(409, "unknown invoice")
	}
	tx, e := s.Store.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if !paid {
		var locked bool
		if e = tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(842100,2)`).Scan(&locked); e != nil {
			return e
		}
		if !locked {
			return fail(409, "store is busy; please retry")
		}
	}
	o, e := getOrder(ctx, tx, parts[1], true)
	if e != nil {
		return e
	}
	if str(obj(o, "snapshot"), "_telegram_bot_id") != botID || str(o, "gateway") != "telegram-stars" || str(o, "_authority") != payload || num(obj(o, "snapshot"), "_telegram_user") != user || str(payment, "currency") != "XTR" || str(o, "currency") != "XTR" || num(payment, "total_amount") != num(o, "amount") {
		return fail(409, "invoice details do not match")
	}
	if !paid {
		if str(o, "state") != "pending" {
			return fail(409, "invoice is no longer pending")
		}
		if expiry, e := timestamp(o, "expires_at"); e != nil || !expiry.After(time.Now()) {
			return fail(409, "invoice expired; open the store again")
		}
		if e = s.recheckOrderCapacity(ctx, tx, o); e != nil {
			return e
		}
		return tx.Commit(ctx)
	}
	charge := str(payment, "telegram_payment_charge_id")
	if len(charge) < 1 || len(charge) > 256 || flag(payment, "is_recurring") {
		return fail(409, "unsupported payment receipt")
	}
	if oneOf(str(o, "state"), "paid", "fulfilled") {
		if str(o, "payment_reference") != charge {
			return fail(409, "invoice was already paid")
		}
		return nil
	}
	if str(o, "state") != "pending" {
		return fail(409, "invoice was cancelled")
	}
	if _, e = tx.Exec(ctx, `UPDATE vr_orders SET state='paid',payment_reference=$2,paid_at=now(),next_attempt_at=now(),last_error='' WHERE id=$1`, parts[1], charge); e != nil {
		return e
	}
	if e = audit(ctx, tx, 0, "commerce.stars_verified", parts[1], Data{}); e != nil {
		return e
	}
	return tx.Commit(ctx)
}

// Receipts arrive exclusively through authenticated Bot API polling.
func (s *Server) telegramPaymentUpdate(ctx context.Context, cfg Data, token string, u Data) (bool, error) {
	if p := obj(u, "pre_checkout_query"); len(p) > 0 {
		check, cancel := context.WithTimeout(ctx, 6*time.Second)
		e := s.acceptStarsPayment(check, p, num(obj(p, "from"), "id"), false, telegramBotID(token))
		cancel()
		answer := Data{"pre_checkout_query_id": str(p, "id"), "ok": e == nil}
		if e != nil {
			answer["error_message"] = botText(cfg, "This invoice is unavailable. Reopen the store and try again.", "این فاکتور در دسترس نیست؛ فروشگاه را دوباره باز کنید.")
		}
		_, e = s.telegramRequest(ctx, token, "answerPreCheckoutQuery", answer)
		return true, e
	}
	m := obj(u, "message")
	p := obj(m, "successful_payment")
	if len(p) == 0 {
		return false, nil
	}
	user := num(obj(m, "from"), "id")
	if str(obj(m, "chat"), "type") != "private" || num(obj(m, "chat"), "id") != user {
		return true, nil
	}
	e := s.acceptStarsPayment(ctx, p, user, true, telegramBotID(token))
	var invalid *apiError
	if errors.As(e, &invalid) || errors.Is(e, pgx.ErrNoRows) {
		return true, nil
	}
	return true, e
}
func (s *Server) telegramOrderNotifications(ctx context.Context, cfg Data, token string) error {
	rows, e := s.Store.Pool.Query(ctx, `SELECT t.order_id,t.user_id,t.token_ciphertext,o.owner_id FROM vr_telegram_orders t JOIN vr_orders o ON o.id=t.order_id WHERE o.state='fulfilled' AND o.snapshot->>'_telegram_bot_id'=$1 AND t.notified_at IS NULL ORDER BY t.created_at LIMIT 10`, telegramBotID(token))
	if e != nil {
		return e
	}
	type notice struct {
		id, sealed  string
		user, owner int64
	}
	all := []notice{}
	for rows.Next() {
		var n notice
		if e = rows.Scan(&n.id, &n.user, &n.sealed, &n.owner); e != nil {
			rows.Close()
			return e
		}
		all = append(all, n)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, n := range all {
		bearer, e := unseal(s.Config.FieldKey, n.sealed)
		if e != nil {
			return e
		}
		link := privateOrderURL(s.Config.PublicURL, n.owner, n.id, bearer)
		if _, e = s.telegramRequest(ctx, token, "sendMessage", Data{"chat_id": n.user, "text": botText(cfg, "Your subscription is ready.", "اشتراک شما آماده است."), "reply_markup": botKeyboard([]any{Data{"text": botText(cfg, "View order and subscription", "مشاهدهٔ سفارش و اشتراک"), "url": link}}), "disable_web_page_preview": true}); e != nil {
			return e
		}
		if _, e = s.Store.Pool.Exec(ctx, `UPDATE vr_telegram_orders SET notified_at=now() WHERE order_id=$1`, n.id); e != nil {
			return e
		}
	}
	return nil
}
