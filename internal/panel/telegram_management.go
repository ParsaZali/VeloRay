package panel

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) advancedTelegramReply(ctx context.Context, cfg Data, text, owner string) (Data, bool) {
	p := strings.Fields(text)
	if len(p) == 0 {
		return nil, false
	}
	cmd := strings.ToLower(strings.Split(p[0], "@")[0])
	reply := Data{"reply_markup": botHome(cfg)}
	bad := func(message string) (Data, bool) { reply["text"] = message; return reply, true }
	switch cmd {
	case "/usage", "/ips", "/devices":
		if len(p) != 2 {
			return bad("Use " + cmd + " CLIENT_ID")
		}
		id, e := strconv.ParseInt(p[1], 10, 64)
		if e != nil || id < 1 {
			return bad("Invalid client ID")
		}
		if cmd == "/usage" {
			u, e := s.clientUsage(ctx, id)
			if e != nil {
				return bad(e.Error())
			}
			remaining := "Unlimited"
			if !flag(u, "unlimited") {
				remaining = fmtBytes(num(u, "remaining_bytes"))
			}
			return bad(fmt.Sprintf("Client #%d\nBilled: %s\nRemaining: %s\nRaw upload: %s\nRaw download: %s\nTelemetry: %s\nFresh: %t", id, fmtBytes(num(u, "used_bytes")), remaining, fmtBytes(num(u, "raw_upload_bytes")), fmtBytes(num(u, "raw_download_bytes")), str(u, "telemetry_at"), flag(u, "telemetry_fresh")))
		}
		kind := strings.TrimPrefix(cmd, "/")
		d, e := s.clientConnections(ctx, id, kind)
		if e != nil {
			return bad(e.Error())
		}
		lines := []string{fmt.Sprintf("%s · #%d · %d", kind, id, num(d, "count"))}
		for _, v := range array(d, "items") {
			item := asData(v)
			if kind == "ips" {
				lines = append(lines, str(item, "ip")+" · "+str(item, "last_seen_at"))
			} else {
				lines = append(lines, fmt.Sprintf("#%d · %s · blocked: %t", num(item, "id"), str(item, "hwid"), flag(item, "blocked")))
			}
			if len(lines) >= 25 {
				break
			}
		}
		if kind == "ips" && !flag(d, "available") {
			lines = append(lines, "Some nodes have unavailable or stale IP telemetry.")
		}
		return bad(strings.Join(lines, "\n"))
	case "/sales":
		rows, e := commerceRows(ctx, s.Store.Pool, `SELECT jsonb_build_object('currency',currency,'amount',sum(amount)::bigint,'orders',count(*)) FROM vr_orders WHERE state IN('paid','fulfilled') AND kind<>'topup' AND created_at>now()-interval '30 days' GROUP BY currency`)
		if e != nil {
			return bad(e.Error())
		}
		lines := []string{botText(cfg, "Sales · last 30 days", "فروش · ۳۰ روز گذشته")}
		for _, d := range rows {
			lines = append(lines, fmt.Sprintf("%s · %d orders", botAmount(d), num(d, "orders")))
		}
		return bad(strings.Join(lines, "\n"))
	case "/receipts":
		rows, e := commerceRows(ctx, s.Store.Pool, proofSelect+`WHERE p.state='pending' ORDER BY p.created_at LIMIT 20`)
		if e != nil {
			return bad(e.Error())
		}
		buttons := [][]any{}
		for _, d := range rows {
			buttons = append(buttons, []any{botButton(fmt.Sprintf("#%d · %s", num(d, "id"), str(d, "method")), fmt.Sprintf("vr:/receipt %d", num(d, "id")))})
		}
		buttons = append(buttons, []any{botButton("VeloRay", "vr:/status")})
		reply["text"] = botText(cfg, "Transfers awaiting review", "واریزهای در انتظار بررسی")
		reply["reply_markup"] = Data{"inline_keyboard": buttons}
		return reply, true
	case "/receipt":
		if len(p) < 2 {
			return bad("Use /receipt ID, or /receipt ID approve BANK_REFERENCE")
		}
		id, e := strconv.ParseInt(p[1], 10, 64)
		if e != nil {
			return bad("Invalid receipt ID")
		}
		var order string
		if s.Store.Pool.QueryRow(ctx, `SELECT order_id FROM vr_payment_proofs WHERE id=$1`, id).Scan(&order) != nil {
			return bad("Receipt not found")
		}
		proof, e := s.paymentProof(ctx, s.Store.Pool, order, true)
		if e != nil {
			return bad(e.Error())
		}
		o, e := getOrder(ctx, s.Store.Pool, order, false)
		if e != nil {
			return bad(e.Error())
		}
		if len(p) >= 3 {
			if p[2] != "approve" {
				return bad("Use the panel to reject a receipt with a reason")
			}
			reference := ""
			if len(p) == 4 {
				reference = p[3]
			}
			if str(proof, "method") == "card" && len(reference) < 4 {
				return bad("Provide the bank reference: /receipt ID approve BANK_REFERENCE")
			}
			if !flag(cfg, "telegram_allow_changes") || flag(cfg, "require_2fa_for_admins") {
				return bad("Use the panel to review transfers, or enable bot management without enforced MFA")
			}
			nonce := randomToken(18)
			payload := salesJSON(Data{"reference": reference, "expected_amount": num(o, "amount"), "expected_currency": str(o, "currency")})
			sealed, e := seal(s.Config.FieldKey, payload)
			if e != nil {
				return bad(e.Error())
			}
			if _, e = s.Store.Pool.Exec(ctx, `INSERT INTO vr_telegram_actions(id,owner_id,kind,target_id,action) VALUES($1,$2,'receipts',$3,$4)`, nonce, owner, id, sealed); e != nil {
				return bad(e.Error())
			}
			reply["text"] = fmt.Sprintf("%s\n%s\n%s", proofBotDescription(proof), botAmount(o), botText(cfg, "Confirm only after independently verifying the transfer. This delivers the subscription.", "فقط پس از بررسی واریز تأیید کنید. این کار اشتراک را تحویل می‌دهد."))
			reply["reply_markup"] = botKeyboard([]any{botButton(botText(cfg, "Approve", "تأیید"), "vr:/confirm "+nonce), botButton(botText(cfg, "Cancel", "انصراف"), "vr:/cancel "+nonce)})
			return reply, true
		}
		reply["text"] = proofBotDescription(proof) + "\n" + botAmount(o) + "\n" + str(proof, "transaction_id") + "\n" + str(obj(proof, "evidence"), "note")
		reply["protect_content"] = true
		if image := str(obj(proof, "evidence"), "image"); image != "" {
			reply["_photo_data"] = image
		}
		if str(proof, "method") == "usdt" {
			reply["reply_markup"] = botKeyboard([]any{botButton(botText(cfg, "Review transfer", "بررسی واریز"), fmt.Sprintf("vr:/receipt %d approve", id))})
		} else {
			reply["text"] = str(reply, "text") + fmt.Sprintf("\n/receipt %d approve BANK_REFERENCE", id)
		}
		return reply, true
	case "/tickets":
		rows, e := commerceRows(ctx, s.Store.Pool, `SELECT to_jsonb(t) FROM vr_support_tickets t WHERE state='open' ORDER BY updated_at DESC LIMIT 20`)
		if e != nil {
			return bad(e.Error())
		}
		lines := []string{botText(cfg, "Open support tickets", "تیکت‌های باز")}
		for _, d := range rows {
			lines = append(lines, fmt.Sprintf("#%d · %s · Telegram %d", num(d, "id"), str(d, "subject"), num(d, "user_id")))
		}
		lines = append(lines, "/reply TICKET_ID message")
		return bad(strings.Join(lines, "\n"))
	case "/reply":
		if len(p) < 3 {
			return bad("Use /reply TICKET_ID message")
		}
		if !flag(cfg, "telegram_allow_changes") || flag(cfg, "require_2fa_for_admins") {
			return bad("Use the panel for support replies")
		}
		id, e := strconv.ParseInt(p[1], 10, 64)
		if e != nil {
			return bad("Invalid ticket ID")
		}
		var seller, user int64
		if s.Store.Pool.QueryRow(ctx, `SELECT owner_id,user_id FROM vr_support_tickets WHERE id=$1`, id).Scan(&seller, &user) != nil {
			return bad("Ticket not found")
		}
		body := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(strings.TrimPrefix(text, p[0])), p[1]))
		if _, e = s.writeTicket(ctx, seller, user, id, "staff", "", body, false); e != nil {
			return bad(e.Error())
		}
		_ = audit(ctx, s.Store.Pool, 0, "telegram.ticket_reply", owner, Data{"ticket": id})
		return bad(botText(cfg, "Reply sent", "پاسخ ارسال شد"))
	default:
		return nil, false
	}
}
func (s *Server) executeBotReceipt(ctx context.Context, cfg Data, owner string, id int64, sealed string) error {
	if !telegramOwners(cfg)[owner] || !flag(cfg, "telegram_allow_changes") || flag(cfg, "require_2fa_for_admins") {
		return fail(403, "owner management required")
	}
	raw, e := unseal(s.Config.FieldKey, sealed)
	if e != nil {
		return e
	}
	var sellerID int64
	if e = s.Store.Pool.QueryRow(ctx, `SELECT o.owner_id FROM vr_orders o JOIN vr_payment_proofs p ON p.order_id=o.id WHERE p.id=$1`, id).Scan(&sellerID); e != nil {
		return e
	}
	roots, e := list(ctx, s.Store.Pool, "users", `WHERE data->>'is_superuser'='true' AND data->>'is_active'='true' ORDER BY id LIMIT 1`)
	if e != nil {
		return e
	}
	if len(roots) == 0 {
		return fail(409, "active administrator unavailable")
	}
	req, _ := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("http://localhost/?owner=%d", sellerID), strings.NewReader(raw))
	_, _, e = s.receiptsAPI(discardResponse{}, req, fmt.Sprintf("receipts/%d/approve", id), Actor{User: roots[0], Scope: "admin"}, sellerID)
	_ = audit(ctx, s.Store.Pool, num(roots[0], "id"), "telegram.receipt_review", owner, Data{"receipt": id, "success": e == nil})
	return e
}
