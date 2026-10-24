package panel

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const receiptPNG = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y9Zl1sAAAAASUVORK5CYII="

func enableManual(t *testing.T, s *Server, ctx context.Context, a Actor, patch Data) {
	t.Helper()
	b := salesDefaults()
	b = merge(b, patch)
	if _, _, e := managementRequest(t, s, ctx, a, "PATCH", "commerce/store", Data{"bot": b}); e != nil {
		t.Fatal(e)
	}
}
func TestIntegrationSalesConfigOptionalAndStrict(t *testing.T) {
	s, ctx, a, _, _ := commerceFixture(t)
	for _, input := range []Data{{"welcome_image": "javascript:alert(1)"}, {"usdt_enabled": true, "usdt_address": "invalid"}, {"card_enabled": true, "card_number": "123", "card_holder": "Test"}, {"buttons": []any{Data{"action": "exec", "label": "Unsafe", "visible": true, "row": 0}}}, {"trial_enabled": true, "trial_hours": 0}} {
		b := merge(salesDefaults(), input)
		if _, _, e := managementRequest(t, s, ctx, a, "PATCH", "commerce/store", Data{"bot": b}); e == nil {
			t.Fatal("accepted invalid bot settings", input)
		}
	}
	d, e := getStore(ctx, s.Store.Pool, num(a.User, "id"))
	if e != nil || flag(d, "telegram_sales") {
		t.Fatal(d, e)
	}
	if e = validateProofImage("data:image/svg+xml;base64,PHN2Zz4="); e == nil {
		t.Fatal("active receipt content accepted")
	}
	if e = validateProofImage("data:image/png;base64,SGVsbG8="); e == nil {
		t.Fatal("forged image MIME accepted")
	}
}
func TestIntegrationManualReceiptNeverAutoPaysAndReviewIsIdempotent(t *testing.T) {
	s, ctx, a, o, _ := commerceFixture(t)
	enableManual(t, s, ctx, a, Data{"card_enabled": true, "card_number": "6037991234567890", "card_holder": "Test Seller"})
	order, e := s.createOrder(ctx, Data{"offer_id": num(o, "id"), "kind": "purchase", "name": "Buyer"}, 0, true)
	if e != nil {
		t.Fatal(e)
	}
	proof, e := s.recordPaymentProof(ctx, str(order, "id"), Data{"method": "card", "image": receiptPNG, "note": "private-bank-proof"})
	if e != nil {
		t.Fatal(e)
	}
	current, _ := getOrder(ctx, s.Store.Pool, str(order, "id"), false)
	if str(current, "state") != "pending" || num(current, "customer_id") != 0 {
		t.Fatal("upload delivered unpaid subscription", current)
	}
	if _, e = s.recordPaymentProof(ctx, str(order, "id"), Data{"method": "card", "image": receiptPNG}); e == nil {
		t.Fatal("duplicate pending proof accepted")
	}
	id := num(proof, "id")
	path := fmt.Sprintf("commerce/receipts/%d/approve", id)
	if _, _, e = managementRequest(t, s, ctx, a, "POST", path, Data{"expected_amount": 999, "expected_currency": "IRT", "reference": "bank-123"}); e == nil {
		t.Fatal("wrong review amount accepted")
	}
	for i := 0; i < 2; i++ {
		if _, _, e = managementRequest(t, s, ctx, a, "POST", path, Data{"expected_amount": 10000, "expected_currency": "IRT", "reference": "bank-123"}); e != nil {
			t.Fatal(e)
		}
	}
	current, _ = getOrder(ctx, s.Store.Pool, str(order, "id"), false)
	if str(current, "state") != "fulfilled" {
		t.Fatal(current)
	}
	var count int
	s.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM vr_accounts WHERE data->>'name' LIKE 'Buyer-%'`).Scan(&count)
	if count != 1 {
		t.Fatal("duplicate fulfillment", count)
	}
	var sealed string
	s.Store.Pool.QueryRow(ctx, `SELECT evidence_ciphertext FROM vr_payment_proofs WHERE id=$1`, id).Scan(&sealed)
	if strings.Contains(sealed, "private-bank-proof") || strings.Contains(sealed, "iVBOR") {
		t.Fatal("proof stored unencrypted")
	}
	req := httptest.NewRequest("GET", "/?owner=999", nil).WithContext(ctx)
	if _, _, e = s.receiptsAPI(httptest.NewRecorder(), req, fmt.Sprintf("receipts/%d", id), a, 999); e == nil {
		t.Fatal("other store proof disclosed")
	}
}
func TestIntegrationUSDTReceiptRejectsReusedHashAndKeepsExactUnits(t *testing.T) {
	s, ctx, a, o, _, _, _ := starsFixture(t)
	enableManual(t, s, ctx, a, Data{"usdt_enabled": true, "usdt_network": "TRC20", "usdt_address": "T" + strings.Repeat("1", 33)})
	if _, _, e := managementRequest(t, s, ctx, a, "PATCH", fmt.Sprintf("commerce/offers/%d", num(o, "id")), Data{"currency": "USDT", "amount": 1250000}); e != nil {
		t.Fatal(e)
	}
	buyer := context.WithValue(ctx, telegramBuyerKey{}, int64(777))
	input := Data{"offer_id": num(o, "id"), "kind": "purchase", "name": "USDT buyer", "accept_terms": true}
	first, e := s.createOrder(buyer, input, 0, true)
	if e != nil {
		t.Fatal(e)
	}
	second, e := s.createOrder(buyer, input, 0, true)
	if e != nil {
		t.Fatal(e)
	}
	hash := strings.Repeat("A", 64)
	proof, e := s.recordPaymentProof(ctx, str(first, "id"), Data{"method": "usdt", "transaction_id": "0x" + hash})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.recordPaymentProof(ctx, str(second, "id"), Data{"method": "usdt", "transaction_id": strings.ToLower(hash)}); e == nil {
		t.Fatal("reused transaction accepted")
	}
	if _, _, e := managementRequest(t, s, ctx, a, "POST", fmt.Sprintf("commerce/receipts/%d/approve", num(proof, "id")), Data{"expected_amount": 1250000, "expected_currency": "USDT"}); e != nil {
		t.Fatal(e)
	}
	current, _ := getOrder(ctx, s.Store.Pool, str(first, "id"), false)
	if str(current, "state") != "fulfilled" || num(current, "amount") != 1250000 || botAmount(current) != "1.250000 USDT" {
		t.Fatal(current)
	}
}
func TestIntegrationTrialOnceAndBuyerIsolation(t *testing.T) {
	s, ctx, a, o, _, _, token := starsFixture(t)
	enableManual(t, s, ctx, a, Data{"trial_enabled": true, "trial_service_id": num(o, "service_id"), "trial_traffic_bytes": 1048576, "trial_hours": 2, "allow_toggle": true, "allow_rotate": true})
	first, e := s.claimSalesTrial(ctx, num(a.User, "id"), 777)
	if e != nil {
		t.Fatal(e)
	}
	second, e := s.claimSalesTrial(ctx, num(a.User, "id"), 777)
	if e != nil || num(first, "id") != num(second, "id") {
		t.Fatal(first, second, e)
	}
	expiry, _ := timestamp(first, "expires_at")
	if time.Until(expiry) > 2*time.Hour || time.Until(expiry) < time.Hour {
		t.Fatal(first)
	}
	if _, e = s.salesCustomerView(ctx, num(a.User, "id"), 778, num(first, "id")); e == nil {
		t.Fatal("another buyer read trial")
	}
	if _, e = s.salesServiceChange(ctx, num(a.User, "id"), 778, num(first, "id"), "rotate"); e == nil {
		t.Fatal("another buyer rotated trial")
	}
	changed, e := s.salesServiceChange(ctx, num(a.User, "id"), 777, num(first, "id"), "rotate")
	if e != nil || str(changed, "subscription_url") == str(first, "subscription_url") {
		t.Fatal(changed, e)
	}
	r := httptest.NewRequest("GET", fmt.Sprintf("/api/shop/me?store=%d", num(a.User, "id")), nil)
	r.Header.Set("X-Telegram-Init-Data", signedMiniApp(token, 777, time.Now()))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "services") {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, _, e = managementRequest(t, s, ctx, a, "PATCH", "commerce/store", Data{"telegram_sales": false}); e != nil {
		t.Fatal(e)
	}
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 404 {
		t.Fatal("disabled sales still accessible", w.Code)
	}
}
func TestIntegrationBotPurchaseConfirmationReplayAndPrivateMenu(t *testing.T) {
	s, ctx, a, o, cfg, _, _ := starsFixture(t)
	reply := s.salesBotReply(ctx, cfg, fmt.Sprintf("vs:offer:%d", num(o, "id")), 777, Data{})
	buttons := array(obj(clone(reply), "reply_markup"), "inline_keyboard")
	if len(buttons) < 2 {
		t.Fatal(reply)
	}
	row, _ := buttons[1].([]any)
	callback := str(asData(row[0]), "callback_data")
	first := s.salesBotReply(ctx, cfg, callback, 777, Data{})
	second := s.salesBotReply(ctx, cfg, callback, 777, Data{})
	if str(first, "text") != str(second, "text") {
		t.Fatal(first, second)
	}
	var count int
	s.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM vr_orders WHERE owner_id=$1`, num(a.User, "id")).Scan(&count)
	if count != 1 {
		t.Fatal("duplicate bot invoice", count)
	}
	other := s.salesBotReply(ctx, cfg, fmt.Sprintf("/start shop_%d", num(a.User, "id")), 778, Data{})
	if str(other, "text") == "" {
		t.Fatal(other)
	}
	forged := s.salesBotReply(ctx, cfg, callback, 778, Data{})
	if !strings.Contains(str(forged, "text"), "expired") {
		t.Fatal("other buyer reused confirmation", forged)
	}
}
func TestIntegrationSupportTicketOwnershipAndEncryptedMessages(t *testing.T) {
	s, ctx, a, _, _, _, _ := starsFixture(t)
	owner := num(a.User, "id")
	ticket, e := s.writeTicket(ctx, owner, 777, 0, "customer", "", "sensitive support text", false)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.writeTicket(ctx, owner, 778, num(ticket, "id"), "customer", "", "hijack", false); e == nil {
		t.Fatal("other buyer wrote to ticket")
	}
	if _, e = s.writeTicket(ctx, owner, 777, num(ticket, "id"), "staff", "", "reply", false); e != nil {
		t.Fatal(e)
	}
	messages, e := s.ticketMessages(ctx, num(ticket, "id"))
	if e != nil || len(messages) != 2 || str(messages[0], "text") != "sensitive support text" {
		t.Fatal(messages, e)
	}
	var subject string
	s.Store.Pool.QueryRow(ctx, `SELECT subject FROM vr_support_tickets WHERE id=$1`, num(ticket, "id")).Scan(&subject)
	if strings.Contains(subject, "sensitive") {
		t.Fatal("support body copied into plaintext subject")
	}
	var ciphertext string
	s.Store.Pool.QueryRow(ctx, `SELECT body_ciphertext FROM vr_ticket_messages WHERE ticket_id=$1 ORDER BY id LIMIT 1`, num(ticket, "id")).Scan(&ciphertext)
	if strings.Contains(ciphertext, "sensitive") {
		t.Fatal("plaintext support message")
	}
	if _, e = s.writeTicket(ctx, owner, 777, num(ticket, "id"), "staff", "", "", true); e != nil {
		t.Fatal(e)
	}
	if _, e = s.writeTicket(ctx, owner, 777, num(ticket, "id"), "customer", "", "reopen", false); e == nil {
		t.Fatal("closed ticket accepted reply")
	}
}
func TestIntegrationUsageAPIExactBytesAndUnknownTelemetry(t *testing.T) {
	s, ctx := integration(t)
	_, _, c := fixture(t, s, ctx, &nodeMock{running: true})
	c["traffic_limit_bytes"] = int64(9007199254740999)
	c["used_traffic_bytes"] = int64(9007199254740993)
	if e := save(ctx, s.Store.Pool, "clients", c); e != nil {
		t.Fatal(e)
	}
	out, e := s.clientUsage(ctx, num(c, "id"))
	if e != nil || num(out, "remaining_bytes") != 6 || str(out, "used_bytes_exact") != "9007199254740993" || flag(out, "telemetry_fresh") {
		t.Fatal(out, e)
	}
	c["traffic_limit_bytes"] = 0
	if e = save(ctx, s.Store.Pool, "clients", c); e != nil {
		t.Fatal(e)
	}
	out, e = s.clientUsage(ctx, num(c, "id"))
	if e != nil || out["remaining_bytes"] != nil || !flag(out, "unlimited") {
		t.Fatal(out, e)
	}
}
func TestIntegrationBotReceiptManagementRequiresConfirmationAndEnabledChanges(t *testing.T) {
	s, ctx, a, o, cfg, _, _ := starsFixture(t)
	enableManual(t, s, ctx, a, Data{"usdt_enabled": true, "usdt_network": "TRC20", "usdt_address": "T" + strings.Repeat("1", 33)})
	if _, _, e := managementRequest(t, s, ctx, a, "PATCH", fmt.Sprintf("commerce/offers/%d", num(o, "id")), Data{"currency": "USDT", "amount": 1000000}); e != nil {
		t.Fatal(e)
	}
	order, e := s.createOrder(context.WithValue(ctx, telegramBuyerKey{}, int64(777)), Data{"offer_id": num(o, "id"), "kind": "purchase", "name": "Bot receipt", "accept_terms": true}, 0, true)
	if e != nil {
		t.Fatal(e)
	}
	proof, e := s.recordPaymentProof(ctx, str(order, "id"), Data{"method": "usdt", "transaction_id": strings.Repeat("a", 64)})
	if e != nil {
		t.Fatal(e)
	}
	cmd := fmt.Sprintf("/receipt %d approve", num(proof, "id"))
	cfg["telegram_owner_ids"] = []any{"123"}
	reply, _ := s.advancedTelegramReply(ctx, cfg, cmd, "123")
	if !strings.Contains(str(reply, "text"), "panel") {
		t.Fatal("disabled bot management mutated receipt", reply)
	}
	cfg["telegram_allow_changes"] = true
	reply, _ = s.advancedTelegramReply(ctx, cfg, cmd, "123")
	rows := array(obj(clone(reply), "reply_markup"), "inline_keyboard")
	if len(rows) == 0 {
		t.Fatal(reply)
	}
	row := rows[0].([]any)
	nonce := strings.TrimPrefix(str(asData(row[0]), "callback_data"), "vr:/confirm ")
	reply = s.telegramExecute(ctx, cfg, "123", nonce)
	if str(reply, "text") != "Change applied" {
		t.Fatal(reply)
	}
	current, _ := getOrder(ctx, s.Store.Pool, str(order, "id"), false)
	if str(current, "state") != "fulfilled" {
		t.Fatal(current)
	}
}

func TestIntegrationWalletFundingIsReviewedByParentNotSelf(t *testing.T) {
	s, ctx, parent, _, _ := commerceFixture(t)
	enableManual(t, s, ctx, parent, Data{"card_enabled": true, "card_number": "6037991234567890", "card_holder": "Parent"})
	if _, _, e := managementRequest(t, s, ctx, parent, "POST", "commerce/topup", Data{"amount": 1000, "currency": "IRT"}); e == nil {
		t.Fatal("self-funded wallet accepted")
	}
	child := defaults("users")
	child["username"] = "funding-child"
	child["role"] = "reseller"
	child["parent_id"] = num(parent.User, "id")
	if e := save(ctx, s.Store.Pool, "users", child); e != nil {
		t.Fatal(e)
	}
	actor := Actor{User: child, Scope: "write"}
	out, _, e := managementRequest(t, s, ctx, actor, "POST", "commerce/topup", Data{"amount": 1000, "currency": "IRT"})
	if e != nil {
		t.Fatal(e)
	}
	o := out.(Data)
	if num(o, "owner_id") != num(parent.User, "id") {
		t.Fatal(o)
	}
	proof, e := s.recordPaymentProof(ctx, str(o, "id"), Data{"method": "card", "image": receiptPNG})
	if e != nil {
		t.Fatal(e)
	}
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{"reference":"bank-456","expected_amount":1000,"expected_currency":"IRT"}`)).WithContext(ctx)
	if _, _, e = s.receiptsAPI(httptest.NewRecorder(), req, fmt.Sprintf("receipts/%d/approve", num(proof, "id")), actor, num(child, "id")); e == nil {
		t.Fatal("child approved own deposit")
	}
	if _, _, e = managementRequest(t, s, ctx, parent, "POST", fmt.Sprintf("commerce/receipts/%d/approve", num(proof, "id")), Data{"reference": "bank-456", "expected_amount": 1000, "expected_currency": "IRT"}); e != nil {
		t.Fatal(e)
	}
	var balance int64
	if e = s.Store.Pool.QueryRow(ctx, `SELECT balance FROM vr_wallets WHERE user_id=$1 AND currency='IRT'`, num(child, "id")).Scan(&balance); e != nil || balance != 1000 {
		t.Fatal(balance, e)
	}
}

func TestIntegrationSalesImagePreviewKeepsStrictScriptPolicy(t *testing.T) {
	s, _ := integration(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/version", nil)
	s.Handler().ServeHTTP(w, r)
	csp := w.Header().Get("Content-Security-Policy")
	if w.Code != 200 || !strings.Contains(csp, "img-src 'self' data: https:") || !strings.Contains(csp, "script-src 'self';") || strings.Contains(csp, "script-src https:") || w.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatal(w.Code, csp)
	}
}

func TestIntegrationBuyerCannotReserveUnlimitedUnpaidInvoices(t *testing.T) {
	s, ctx, _, o, _, _, _ := starsFixture(t)
	buyer := context.WithValue(ctx, telegramBuyerKey{}, int64(777))
	input := Data{"offer_id": num(o, "id"), "kind": "purchase", "name": "Limited pending", "accept_terms": true}
	var first string
	for i := 0; i < 3; i++ {
		created, e := s.createOrder(buyer, input, 0, true)
		if e != nil {
			t.Fatal(e)
		}
		if i == 0 {
			first = str(created, "id")
		}
	}
	if _, e := s.createOrder(buyer, input, 0, true); e == nil {
		t.Fatal("unbounded pending reservations accepted")
	}
	if _, e := s.Store.Pool.Exec(ctx, `UPDATE vr_orders SET expires_at=now()-interval '1 second' WHERE id=$1`, first); e != nil {
		t.Fatal(e)
	}
	if _, e := s.createOrder(buyer, input, 0, true); e != nil {
		t.Fatal("expired reservation did not release buyer limit", e)
	}
}

func TestIntegrationSupportConversationWorksInsideSameBot(t *testing.T) {
	s, ctx, a, _, cfg, _, _ := starsFixture(t)
	reply := s.salesBotReply(ctx, cfg, "vs:ticketnew", 777, Data{})
	if !strings.Contains(str(reply, "text"), "2000") && !strings.Contains(str(reply, "text"), "۲۰۰۰") {
		t.Fatal(reply)
	}
	reply = s.salesBotReply(ctx, cfg, "Please help with my connection", 777, Data{})
	var id int64
	if e := s.Store.Pool.QueryRow(ctx, `SELECT id FROM vr_support_tickets WHERE owner_id=$1 AND user_id=777`, num(a.User, "id")).Scan(&id); e != nil {
		t.Fatal(e, reply)
	}
	if _, e := s.writeTicket(ctx, num(a.User, "id"), 777, id, "staff", "", "Try updating your subscription", false); e != nil {
		t.Fatal(e)
	}
	s.salesBotReply(ctx, cfg, fmt.Sprintf("vs:ticketreply:%d", id), 777, Data{})
	s.salesBotReply(ctx, cfg, "It works now, thank you", 777, Data{})
	messages, e := s.ticketMessages(ctx, id)
	if e != nil || len(messages) != 3 {
		t.Fatal(messages, e)
	}
	s.salesBotReply(ctx, cfg, fmt.Sprintf("/start shop_%d", num(a.User, "id")), 778, Data{})
	bad := s.salesBotReply(ctx, cfg, fmt.Sprintf("vs:ticket:%d", id), 778, Data{})
	if !strings.Contains(str(bad, "text"), "not found") {
		t.Fatal("other buyer read ticket", bad)
	}
}
