package panel

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

func signedMiniApp(token string, user int64, at time.Time) string {
	v := url.Values{"auth_date": {strconv.FormatInt(at.Unix(), 10)}, "query_id": {"fixture-query"}, "user": {fmt.Sprintf(`{"id":%d,"first_name":"Buyer"}`, user)}}
	lines := []string{}
	for k := range v {
		lines = append(lines, k+"="+v.Get(k))
	}
	sort.Strings(lines)
	key := hmac.New(sha256.New, []byte("WebAppData"))
	key.Write([]byte(token))
	mac := hmac.New(sha256.New, key.Sum(nil))
	mac.Write([]byte(strings.Join(lines, "\n")))
	v.Set("hash", hex.EncodeToString(mac.Sum(nil)))
	return v.Encode()
}
func TestTelegramMiniAppRejectsTamperingExpiryAndDuplicateFields(t *testing.T) {
	now := time.Now()
	raw := signedMiniApp("token", 777, now)
	if id, e := validateMiniApp(raw, "token", now); e != nil || id != 777 {
		t.Fatal(id, e)
	}
	for _, bad := range []string{raw + "&auth_date=1", strings.Replace(raw, "Buyer", "Other", 1), signedMiniApp("token", 777, now.Add(-2*time.Hour)), signedMiniApp("token", 777, now.Add(time.Hour)), signedMiniApp("other-token", 777, now)} {
		if _, e := validateMiniApp(bad, "token", now); e == nil {
			t.Fatal("accepted untrusted Mini App data")
		}
	}
}

type starsTransport struct {
	telegramTransport
	invoices []Data
	answers  []Data
}

func (m *starsTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if strings.HasSuffix(r.URL.Path, "/createInvoiceLink") || strings.HasSuffix(r.URL.Path, "/answerPreCheckoutQuery") {
		raw, _ := io.ReadAll(r.Body)
		body, _ := decode(raw)
		result := Data{"ok": true, "result": true}
		if strings.HasSuffix(r.URL.Path, "/createInvoiceLink") {
			m.invoices = append(m.invoices, body)
			result["result"] = "https://t.me/$fixture-invoice"
		} else {
			m.answers = append(m.answers, body)
		}
		b, _ := json.Marshal(result)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b))), Header: http.Header{}}, nil
	}
	return m.telegramTransport.RoundTrip(r)
}
func starsFixture(t *testing.T) (*Server, context.Context, Actor, Data, Data, *starsTransport, string) {
	t.Helper()
	s, ctx, a, o, _ := commerceFixture(t)
	if _, _, e := managementRequest(t, s, ctx, a, "PATCH", "commerce/store", Data{"telegram_sales": true, "terms_url": "https://store.example/terms", "support_url": "https://store.example/support"}); e != nil {
		t.Fatal(e)
	}
	out, _, e := managementRequest(t, s, ctx, a, "PATCH", fmt.Sprintf("commerce/offers/%d", num(o, "id")), Data{"currency": "XTR", "amount": 75, "cost_amount": 5})
	if e != nil {
		t.Fatal(e)
	}
	o = out.(Data)
	cfg, _ := settings(ctx, s.Store.Pool)
	token := "123456:official-test-token"
	sealed, e := seal(s.Config.FieldKey, token)
	if e != nil {
		t.Fatal(e)
	}
	cfg["telegram_enabled"], cfg["_telegram_token"], cfg["_telegram_username"] = true, sealed, "fixturebot"
	for _, k := range []string{"telegram_alert_cpu_percent", "telegram_alert_memory_percent", "telegram_alert_disk_percent"} {
		cfg[k] = 101
	}
	if e = saveSettings(ctx, s.Store.Pool, cfg); e != nil {
		t.Fatal(e)
	}
	m := &starsTransport{}
	s.TelegramHTTP = &http.Client{Transport: m}
	reply := s.telegramSalesReply(ctx, cfg, fmt.Sprintf("/start shop_%d", num(a.User, "id")), 777)
	if str(reply, "text") == "This store is unavailable." {
		t.Fatal(reply)
	}
	return s, ctx, a, o, cfg, m, token
}
func TestIntegrationStarsCheckoutReceiptReplayAndDelivery(t *testing.T) {
	s, ctx, a, o, cfg, m, token := starsFixture(t)
	input := Data{"offer_id": num(o, "id"), "kind": "purchase", "name": "Buyer", "telegram_init_data": signedMiniApp(token, 777, time.Now()), "accept_terms": true}
	body, _ := json.Marshal(input)
	req := httptest.NewRequest("POST", "/api/shop/orders", strings.NewReader(string(body))).WithContext(ctx)
	out, _, e := s.shopAPI(httptest.NewRecorder(), req, "shop/orders")
	if e != nil {
		t.Fatal(e)
	}
	order := out.(Data)
	id := str(order, "id")
	for i := 0; i < 2; i++ {
		pay, _, e := s.requestPayment(ctx, id)
		if e != nil || str(pay.(Data), "gateway") != "telegram-stars" {
			t.Fatal(pay, e)
		}
	}
	if len(m.invoices) != 1 || str(m.invoices[0], "currency") != "XTR" || str(m.invoices[0], "provider_token") != "" {
		t.Fatal(m.invoices)
	}
	d, _ := getOrder(ctx, s.Store.Pool, id, false)
	payload := str(d, "_authority")
	p := Data{"invoice_payload": payload, "total_amount": 75, "currency": "XTR", "telegram_payment_charge_id": "telegram-unique-charge"}
	if e = s.acceptStarsPayment(ctx, p, 778, false, telegramBotID(token)); e == nil {
		t.Fatal("different payer accepted")
	}
	if e = s.acceptStarsPayment(ctx, p, 777, true, "another-bot"); e == nil {
		t.Fatal("another bot's receipt accepted")
	}
	bad := clone(p)
	bad["total_amount"] = 74
	if e = s.acceptStarsPayment(ctx, bad, 777, true, telegramBotID(token)); e == nil {
		t.Fatal("wrong paid amount accepted")
	}
	p["id"] = "precheckout"
	p["from"] = Data{"id": 777}
	m.updates = []any{Data{"update_id": 100, "pre_checkout_query": p}}
	if e = s.telegramPoll(ctx, cfg); e != nil {
		t.Fatal(e)
	}
	if len(m.answers) != 1 || !flag(m.answers[0], "ok") {
		t.Fatal(m.answers)
	}
	d, _ = getOrder(ctx, s.Store.Pool, id, false)
	if str(d, "state") != "pending" {
		t.Fatal("precheckout credited money", d)
	}
	if e = s.verifyPayment(ctx, id, ""); e == nil {
		t.Fatal("public verification accepted an unpolled Stars payment")
	}
	m.updates = []any{Data{"update_id": 101, "message": Data{"chat": Data{"id": 777, "type": "private"}, "from": Data{"id": 777}, "successful_payment": p}}}
	for i := 0; i < 2; i++ {
		if e = s.telegramPoll(ctx, cfg); e != nil {
			t.Fatal(e)
		}
		if e = s.fulfillOrder(ctx, id); e != nil {
			t.Fatal(e)
		}
	}
	d, _ = getOrder(ctx, s.Store.Pool, id, false)
	if str(d, "state") != "fulfilled" || num(d, "customer_id") == 0 {
		t.Fatal(d)
	}
	if _, _, e = managementRequest(t, s, ctx, a, "PATCH", "commerce/store", Data{"bot": Data{"delivery_text": "Ready at {store}; buyer {user_id}"}}); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		if e = s.telegramOrderNotifications(ctx, cfg, token); e != nil {
			t.Fatal(e)
		}
	}
	if len(m.sent) != 1 {
		t.Fatal("delivery notification replayed", m.sent)
	}
	if !strings.Contains(str(m.sent[0], "text"), "buyer 777") {
		t.Fatal("custom delivery text was not used", m.sent)
	}
	raw, _ := json.Marshal(m.sent[0])
	if !strings.Contains(string(raw), "#token=") {
		t.Fatal("missing private order delivery", string(raw))
	}
	var ciphertext string
	if e = s.Store.Pool.QueryRow(ctx, `SELECT token_ciphertext FROM vr_telegram_orders WHERE order_id=$1`, id).Scan(&ciphertext); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(ciphertext, str(order, "token")) {
		t.Fatal("private order bearer stored as plaintext")
	}
	if str(obj(d, "snapshot"), "_terms_url") != "https://store.example/terms" {
		t.Fatal("purchase terms were not snapshotted")
	}
	report, _, e := managementRequest(t, s, ctx, a, "GET", "commerce/reports", nil)
	if e != nil {
		t.Fatal(e)
	}
	summary := report.(Data)["summary"].([]Data)
	if len(summary) != 1 || str(summary[0], "currency") != "XTR" || num(summary[0], "revenue") != 75 {
		t.Fatal(report)
	}
}

func TestIntegrationTelegramPaymentPollingIsIndependentAndDurable(t *testing.T) {
	s, ctx, _, o, cfg, m, token := starsFixture(t)
	order, e := s.createOrder(context.WithValue(ctx, telegramBuyerKey{}, int64(777)), Data{"offer_id": num(o, "id"), "kind": "purchase", "name": "Queue", "accept_terms": true}, 0, true)
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = s.requestPayment(ctx, str(order, "id")); e != nil {
		t.Fatal(e)
	}
	d, _ := getOrder(ctx, s.Store.Pool, str(order, "id"), false)
	// A pending customer reply stays queued while payment acknowledgement proceeds.
	m.updates = []any{Data{"update_id": 301, "message": Data{"from": Data{"id": 777}, "chat": Data{"id": 777, "type": "private"}, "text": "/orders"}}, Data{"update_id": 302, "pre_checkout_query": Data{"id": "priority", "from": Data{"id": 777}, "invoice_payload": str(d, "_authority"), "currency": "XTR", "total_amount": 75}}}
	if e = s.telegramPoll(ctx, cfg); e != nil {
		t.Fatal(e)
	}
	if len(m.sent) != 0 || len(m.answers) != 1 || !flag(m.answers[0], "ok") {
		t.Fatal("payment waited for message delivery", m.sent, m.answers)
	}
	var processed bool
	if e = s.Store.Pool.QueryRow(ctx, `SELECT processed_at IS NOT NULL FROM vr_telegram_inbox WHERE bot_id=$1 AND id=301`, telegramBotID(token)).Scan(&processed); e != nil || processed {
		t.Fatal(processed, e)
	}
	// A new Server process uses the persisted inbox even with an empty provider batch.
	m.updates = nil
	fresh := &Server{Config: s.Config, Store: s.Store, TelegramHTTP: s.TelegramHTTP, Logger: s.Logger}
	if e = fresh.telegramReplies(ctx, cfg); e != nil {
		t.Fatal(e)
	}
	if len(m.sent) != 1 {
		t.Fatal("saved message was lost across process restart", m.sent)
	}
	// Another bot can reuse the same update ID without replaying the prior bot's data.
	other := clone(cfg)
	otherToken, _ := seal(s.Config.FieldKey, "987654:another-test-token")
	other["_telegram_token"] = otherToken
	other["_telegram_last_update_id"] = 0
	if e = saveSettings(ctx, s.Store.Pool, other); e != nil {
		t.Fatal(e)
	}
	m.updates = []any{Data{"update_id": 301, "message": Data{"from": Data{"id": 777}, "chat": Data{"id": 777, "type": "private"}, "text": "/orders"}}}
	if e = fresh.telegramPoll(ctx, other); e != nil {
		t.Fatal(e)
	}
	var count int
	if e = s.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM vr_telegram_inbox WHERE id=301`).Scan(&count); e != nil || count != 2 {
		t.Fatal("bot update IDs collided", count, e)
	}
}
func TestIntegrationTelegramCustomerCannotReadStaffOrOtherBuyer(t *testing.T) {
	s, ctx, a, o, cfg, m, token := starsFixture(t)
	input := Data{"offer_id": num(o, "id"), "kind": "purchase", "name": "Private", "accept_terms": true}
	order, e := s.createOrder(context.WithValue(ctx, telegramBuyerKey{}, int64(777)), input, 0, true)
	if e != nil {
		t.Fatal(e)
	}
	// The sales bot never grants a Telegram customer a panel identity or admin commands.
	for _, text := range []string{"/status", "/clients", "/node 1 restart", "vr:/confirm fake"} {
		if s.telegramSalesRoute(ctx, text, Data{"chat": Data{"id": 777, "type": "private"}}, Data{"id": 777}) {
			t.Fatal("staff route exposed", text)
		}
	}
	m.updates = []any{Data{"update_id": 200, "message": Data{"from": Data{"id": 777}, "chat": Data{"id": 777, "type": "private"}, "text": "/orders"}}}
	if e = s.telegram(ctx, cfg); e != nil {
		t.Fatal(e)
	}
	if len(m.sent) != 1 {
		t.Fatal(m.sent)
	}
	var cached string
	if e = s.Store.Pool.QueryRow(ctx, `SELECT reply::text FROM vr_telegram_updates WHERE id=200`).Scan(&cached); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(cached, str(order, "token")) || !strings.Contains(cached, "_sealed_sales") {
		t.Fatal("private sales reply was not encrypted")
	}
	// A separate buyer has no access to that order, including through bot history.
	reply := s.telegramSalesReply(ctx, cfg, fmt.Sprintf("/start shop_%d", num(a.User, "id")), 778)
	if str(reply, "text") == "This store is unavailable." {
		t.Fatal(reply)
	}
	reply = s.telegramSalesReply(ctx, cfg, "/orders", 778)
	raw, _ := json.Marshal(reply)
	if strings.Contains(string(raw), str(order, "id")) {
		t.Fatal("other buyer read private order")
	}
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/shop/catalog?store=%d", num(a.User, "id")), nil).WithContext(ctx)
	out, _, e := s.shopAPI(httptest.NewRecorder(), req, "shop/catalog")
	if e != nil || len(out.(Data)["offers"].([]Data)) != 0 {
		t.Fatal("Stars offered to non-Telegram checkout", out, e)
	}
	req.Header.Set("X-Telegram-Init-Data", signedMiniApp(token, 777, time.Now()))
	out, _, e = s.shopAPI(httptest.NewRecorder(), req, "shop/catalog")
	if e != nil || !flag(out.(Data), "telegram_checkout") || len(out.(Data)["offers"].([]Data)) != 1 {
		t.Fatal(out, e)
	}
}
