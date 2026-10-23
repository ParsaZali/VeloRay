package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var merchantPattern = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)
var authorityPattern = regexp.MustCompile(`^[AS][A-Za-z0-9]{20,60}$`)

func gatewayBase(sandbox bool) string {
	if sandbox {
		return "https://sandbox.zarinpal.com"
	}
	return "https://payment.zarinpal.com"
}
func (s *Server) gatewayCall(ctx context.Context, sandbox bool, action string, input Data) (Data, error) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	raw, e := json.Marshal(input)
	if e != nil {
		return nil, e
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, gatewayBase(sandbox)+"/pg/v4/payment/"+action+".json", bytes.NewReader(raw))
	if e != nil {
		return nil, e
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	client := s.PaymentHTTP
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, e := copyClient.Do(req)
	if e != nil {
		return nil, fail(502, "payment provider is unavailable; try verification again before creating another order")
	}
	defer res.Body.Close()
	b, e := io.ReadAll(io.LimitReader(res.Body, (64<<10)+1))
	if e != nil || len(b) > 64<<10 {
		return nil, fail(502, "invalid payment provider response")
	}
	data, e := decode(b)
	if e != nil || res.StatusCode != 200 {
		return nil, fail(502, "payment provider rejected the request")
	}
	out := obj(data, "data")
	if len(out) == 0 {
		return nil, fail(502, "payment provider rejected the request")
	}
	return out, nil
}
func (s *Server) requestPayment(ctx context.Context, id string) (any, int, error) {
	order, e := getOrder(ctx, s.Store.Pool, id, false)
	if e != nil {
		return nil, 0, e
	}
	if str(order, "currency") == "XTR" {
		return s.requestStarsPayment(ctx, id)
	}
	tx, e := s.Store.Pool.Begin(ctx)
	if e != nil {
		return nil, 0, e
	}
	defer tx.Rollback(ctx)
	o, e := getOrder(ctx, tx, id, true)
	if e != nil {
		return nil, 0, e
	}
	if str(o, "state") != "pending" {
		return nil, 0, fail(409, "order is not awaiting payment")
	}
	expiry, err := timestamp(o, "expires_at")
	if err != nil || !expiry.After(time.Now()) {
		return nil, 0, fail(409, "checkout expired; create a new order")
	}
	if e = s.recheckOrderCapacity(ctx, tx, o); e != nil {
		return nil, 0, e
	}
	snap := obj(o, "snapshot")
	if authority := str(o, "_authority"); authority != "" {
		return Data{"url": gatewayBase(flag(snap, "_sandbox")) + "/pg/StartPay/" + authority, "sandbox": flag(snap, "_sandbox")}, 200, nil
	}
	cfg, e := settings(ctx, tx)
	if e != nil {
		return nil, 0, e
	}
	if !flag(cfg, "commerce_payments_enabled") {
		return nil, 0, fail(409, "online payments are not enabled")
	}
	if !strings.HasPrefix(s.Config.PublicURL, "https://") {
		return nil, 0, fail(409, "payments require HTTPS")
	}
	merchant, e := unseal(s.Config.FieldKey, str(cfg, "_commerce_merchant"))
	if e != nil || !merchantPattern.MatchString(merchant) {
		return nil, 0, fail(409, "payment merchant is not configured")
	}
	currency := str(o, "currency")
	if !oneOf(currency, "IRR", "IRT") {
		return nil, 0, fail(400, "gateway supports IRR and IRT only")
	}
	// Gateway request and verification both use whole rials; invoices retain their unit.
	amount := num(o, "amount")
	if currency == "IRT" {
		amount *= 10
	}
	sandbox := flag(cfg, "commerce_sandbox")
	callback := strings.TrimRight(s.Config.PublicURL, "/") + "/api/shop/payment/" + id
	result, e := s.gatewayCall(ctx, sandbox, "request", Data{"merchant_id": merchant, "currency": "IRR", "amount": amount, "description": "VeloRay order " + id, "callback_url": callback, "metadata": Data{"order_id": id, "auto_verify": false}})
	if e != nil {
		return nil, 0, e
	}
	authority := str(result, "authority")
	if num(result, "code") != 100 || !authorityPattern.MatchString(authority) || (sandbox && !strings.HasPrefix(authority, "S")) || (!sandbox && !strings.HasPrefix(authority, "A")) {
		return nil, 0, fail(502, "invalid payment authority")
	}
	snap["_merchant"] = str(cfg, "_commerce_merchant")
	snap["_sandbox"] = sandbox
	snap["_gateway_amount"] = amount
	raw, _ := json.Marshal(snap)
	gateway := "zarinpal"
	if sandbox {
		gateway = "zarinpal-sandbox"
	}
	if _, e = tx.Exec(ctx, `UPDATE vr_orders SET gateway=$4,authority=$2,snapshot=$3 WHERE id=$1`, id, authority, string(raw), gateway); e != nil {
		return nil, 0, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, 0, e
	}
	return Data{"url": gatewayBase(sandbox) + "/pg/StartPay/" + authority, "sandbox": sandbox}, 200, nil
}
func (s *Server) verifyPayment(ctx context.Context, id, authority string) error {
	tx, e := s.Store.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	o, e := getOrder(ctx, tx, id, true)
	if e != nil {
		return e
	}
	if authority != "" && authority != str(o, "_authority") {
		return fail(404, "payment not found")
	}
	if oneOf(str(o, "state"), "paid", "fulfilled") {
		return nil
	}
	if str(o, "state") != "pending" || !oneOf(str(o, "gateway"), "zarinpal", "zarinpal-sandbox") || !authorityPattern.MatchString(str(o, "_authority")) {
		return fail(409, "order has no pending payment")
	}
	snap := obj(o, "snapshot")
	merchant, e := unseal(s.Config.FieldKey, str(snap, "_merchant"))
	if e != nil {
		return fail(409, "payment credentials are unavailable")
	}
	result, e := s.gatewayCall(ctx, flag(snap, "_sandbox"), "verify", Data{"merchant_id": merchant, "authority": str(o, "_authority"), "amount": num(snap, "_gateway_amount")})
	if e != nil {
		return e
	}
	if !oneOf(fmt.Sprint(num(result, "code")), "100", "101") || num(result, "ref_id") <= 0 {
		return fail(409, "payment has not been verified")
	}
	// Code 101 recovers an acknowledgement lost before our commit, using only
	// the persisted authority, merchant and amount. Reference uniqueness is enforced.
	if _, e = tx.Exec(ctx, `UPDATE vr_orders SET state='paid',payment_reference=$2,paid_at=now(),last_error='',next_attempt_at=now() WHERE id=$1`, id, fmt.Sprint(num(result, "ref_id"))); e != nil {
		return e
	}
	if e = audit(ctx, tx, 0, "commerce.payment_verified", id, Data{"sandbox": flag(snap, "_sandbox")}); e != nil {
		return e
	}
	if e = tx.Commit(ctx); e != nil {
		return e
	}
	if !flag(snap, "_sandbox") {
		_ = s.fulfillOrder(ctx, id)
	}
	return nil
}
func (s *Server) paymentCallback(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", 405)
		return
	}
	if e := rateLimit(r.Context(), s.Store.Pool, "shop:callback:"+clientIP(r), 60, time.Hour); e != nil {
		s.respond(w, r, nil, 0, e)
		return
	}
	o, e := getOrder(r.Context(), s.Store.Pool, id, false)
	if e != nil {
		s.respond(w, r, nil, 0, e)
		return
	}
	if r.URL.Query().Get("Status") == "OK" && r.URL.Query().Get("Authority") != "" {
		_ = s.verifyPayment(r.Context(), id, r.URL.Query().Get("Authority"))
	}
	// No bearer credentials or customer details are disclosed in the redirect.
	target := "/shop?store=" + fmt.Sprint(num(o, "owner_id")) + "&order=" + url.QueryEscape(id)
	http.Redirect(w, r, target, http.StatusSeeOther)
}
