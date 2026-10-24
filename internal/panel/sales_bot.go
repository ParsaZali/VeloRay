package panel

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type telegramCheckoutKey struct{}

func salesLocale(cfg, store Data) Data {
	out := clone(cfg)
	out["telegram_locale"] = str(salesConfig(store), "locale")
	return out
}
func salesHome(store, cfg Data, shop string) Data {
	b := salesConfig(store)
	buttons := append([]any{}, array(b, "buttons")...)
	sort.SliceStable(buttons, func(i, j int) bool { return num(asData(buttons[i]), "row") < num(asData(buttons[j]), "row") })
	rows := [][]any{}
	last := int64(-1)
	for _, v := range buttons {
		d := asData(v)
		a := str(d, "action")
		if !flag(d, "visible") || a == "trial" && !flag(b, "trial_enabled") {
			continue
		}
		row := num(d, "row")
		if row != last || len(rows) == 0 || len(rows[len(rows)-1]) >= 3 {
			rows = append(rows, []any{})
			last = row
		}
		button := botButton(str(d, "label"), "vs:"+a)
		if a == "miniapp" {
			button = Data{"text": str(d, "label"), "web_app": Data{"url": shop}}
		}
		rows[len(rows)-1] = append(rows[len(rows)-1], button)
	}
	for i, v := range array(b, "custom_buttons") {
		d := asData(v)
		button := botButton(str(d, "label"), fmt.Sprintf("vs:custom:%d", i))
		if str(d, "url") != "" {
			button = Data{"text": str(d, "label"), "url": str(d, "url")}
		}
		rows = append(rows, []any{button})
	}
	if str(b, "menu_mode") == "keyboard" {
		keyboard := [][]any{}
		for _, row := range rows {
			r := []any{}
			for _, v := range row {
				d := asData(v)
				button := Data{"text": str(d, "text")}
				if app := obj(d, "web_app"); len(app) > 0 {
					button["web_app"] = app
				}
				r = append(r, button)
			}
			keyboard = append(keyboard, r)
		}
		return Data{"keyboard": keyboard, "resize_keyboard": true, "is_persistent": true}
	}
	return Data{"inline_keyboard": rows}
}
func (s *Server) saveSalesSession(ctx context.Context, owner, user int64, d Data) error {
	sealed, e := seal(s.Config.FieldKey, salesJSON(d))
	if e != nil {
		return e
	}
	_, e = s.Store.Pool.Exec(ctx, `INSERT INTO vr_bot_sessions(owner_id,user_id,state_ciphertext) VALUES($1,$2,$3) ON CONFLICT(owner_id,user_id) DO UPDATE SET state_ciphertext=excluded.state_ciphertext,expires_at=now()+interval '30 minutes'`, owner, user, sealed)
	return e
}
func (s *Server) getSalesSession(ctx context.Context, owner, user int64) Data {
	var sealed string
	if s.Store.Pool.QueryRow(ctx, `SELECT state_ciphertext FROM vr_bot_sessions WHERE owner_id=$1 AND user_id=$2 AND expires_at>now()`, owner, user).Scan(&sealed) != nil {
		return Data{}
	}
	plain, e := unseal(s.Config.FieldKey, sealed)
	if e != nil {
		return Data{}
	}
	d, _ := decode([]byte(plain))
	return d
}
func (s *Server) salesConfirmation(ctx context.Context, owner, user, target int64, kind string) (string, error) {
	id := randomToken(18)
	_, e := s.Store.Pool.Exec(ctx, `INSERT INTO vr_bot_confirmations(id,owner_id,user_id,kind,target_id) VALUES($1,$2,$3,$4,$5)`, id, owner, user, kind, target)
	return id, e
}
func (s *Server) salesBotReply(ctx context.Context, cfg Data, text string, user int64, message Data) Data {
	owner := int64(0)
	if strings.HasPrefix(text, "/start shop_") || strings.HasPrefix(text, "vs:shop_") {
		owner, _ = strconv.ParseInt(strings.TrimPrefix(strings.TrimPrefix(text, "/start shop_"), "vs:shop_"), 10, 64)
	} else {
		_ = s.Store.Pool.QueryRow(ctx, `SELECT owner_id FROM vr_telegram_customers WHERE user_id=$1 ORDER BY last_seen_at DESC LIMIT 1`, user).Scan(&owner)
	}
	if owner <= 0 && text == "/start" {
		stores, e := commerceRows(ctx, s.Store.Pool, `SELECT data||jsonb_build_object('owner_id',owner_id) FROM vr_stores WHERE data->>'enabled'='true' AND data->>'telegram_sales'='true' ORDER BY owner_id LIMIT 20`)
		if e == nil && len(stores) == 1 {
			owner = num(stores[0], "owner_id")
		} else {
			rows := [][]any{}
			for _, d := range stores {
				rows = append(rows, []any{botButton(str(d, "name"), fmt.Sprintf("vs:shop_%d", num(d, "owner_id")))})
			}
			if len(rows) > 0 {
				return Data{"text": botText(cfg, "Choose a store", "فروشگاه را انتخاب کنید"), "reply_markup": Data{"inline_keyboard": rows}}
			}
		}
	}
	unavailable := Data{"text": botText(cfg, "This store is unavailable.", "این فروشگاه در دسترس نیست.")}
	store, e := getStore(ctx, s.Store.Pool, owner)
	if e != nil || !flag(store, "enabled") || !flag(store, "telegram_sales") || !strings.HasPrefix(s.Config.PublicURL, "https://") {
		return unavailable
	}
	if _, e = seller(ctx, s.Store.Pool, owner); e != nil {
		return unavailable
	}
	store["_owner_id"] = owner
	cfg = salesLocale(cfg, store)
	b := salesConfig(store)
	if _, e = s.Store.Pool.Exec(ctx, `INSERT INTO vr_telegram_customers(user_id,owner_id) VALUES($1,$2) ON CONFLICT(user_id,owner_id) DO UPDATE SET last_seen_at=now()`, user, owner); e != nil {
		return unavailable
	}
	shop := strings.TrimRight(s.Config.PublicURL, "/") + "/shop?store=" + fmt.Sprint(owner)
	home := salesHome(store, cfg, shop)
	reply := Data{"text": salesText(store, cfg, "welcome", "Welcome to {store}. Choose an option below.", "به {store} خوش آمدید. از منوی زیر انتخاب کنید.", user), "reply_markup": home}
	if str(b, "welcome_image") != "" {
		reply["photo"] = str(b, "welcome_image")
	}
	failReply := func(err error) Data {
		return Data{"text": err.Error(), "reply_markup": botKeyboard([]any{botButton(botText(cfg, "Back", "بازگشت"), "vs:home")})}
	}
	// Reply-keyboard labels resolve to fixed actions, never executable commands.
	for _, v := range array(b, "buttons") {
		d := asData(v)
		if flag(d, "visible") && text == str(d, "label") {
			text = "vs:" + str(d, "action")
		}
	}
	for i, v := range array(b, "custom_buttons") {
		if text == str(asData(v), "label") {
			text = fmt.Sprintf("vs:custom:%d", i)
		}
	}
	if text == "/cancel" || text == "vs:home" || strings.HasPrefix(text, "/start") || strings.HasPrefix(text, "vs:shop_") {
		_, _ = s.Store.Pool.Exec(ctx, `DELETE FROM vr_bot_sessions WHERE owner_id=$1 AND user_id=$2`, owner, user)
		return reply
	}
	action := strings.TrimPrefix(text, "vs:")
	switch action {
	case "/shop", "/buy", "miniapp":
		action = "buy"
	case "/services":
		action = "services"
	case "/account":
		action = "account"
	case "/orders":
		action = "orders"
	case "/free":
		action = "trial"
	case "/tutorial":
		action = "tutorial"
	case "/support", "/paysupport":
		action = "support"
	case "/terms":
		action = "terms"
	}
	// Order tokens stay server-side; the customer can submit a transfer only for
	// the order selected in this private, expiring conversation.
	session := s.getSalesSession(ctx, owner, user)
	if str(session, "step") == "receipt" && !strings.HasPrefix(action, "/") && !strings.HasPrefix(text, "vs:") {
		var orderOwner int64
		e = s.Store.Pool.QueryRow(ctx, `SELECT o.owner_id FROM vr_orders o JOIN vr_telegram_orders t ON t.order_id=o.id WHERE o.id=$1 AND t.user_id=$2`, str(session, "order_id"), user).Scan(&orderOwner)
		if e != nil || orderOwner != owner {
			return failReply(fail(404, "order not found"))
		}
		input := Data{"method": str(session, "method"), "note": str(message, "caption")}
		if str(session, "method") == "usdt" {
			input["transaction_id"] = strings.TrimSpace(text)
		}
		photos := array(message, "photo")
		if len(photos) > 0 {
			photo := asData(photos[len(photos)-1])
			image, e := s.telegramReceiptImage(ctx, cfg, str(photo, "file_id"))
			if e != nil {
				return failReply(e)
			}
			input["image"] = image
		}
		if _, e = s.recordPaymentProof(ctx, str(session, "order_id"), input); e != nil {
			return failReply(e)
		}
		_, _ = s.Store.Pool.Exec(ctx, `DELETE FROM vr_bot_sessions WHERE owner_id=$1 AND user_id=$2`, owner, user)
		return Data{"text": botText(cfg, "Receipt submitted. Your subscription will be delivered after administrator approval.", "رسید ثبت شد. پس از بررسی و تأیید مدیر، اشتراک تحویل داده می‌شود."), "reply_markup": home}
	}
	delete(reply, "photo")
	switch {
	case action == "buy" || strings.HasPrefix(action, "category:"):
		all, e := offers(ctx, s.Store.Pool, owner, true)
		if e != nil {
			return failReply(e)
		}
		valid := []Data{}
		cats := []string{}
		seen := map[string]bool{}
		for _, d := range all {
			if s.validateOffer(ctx, s.Store.Pool, d) != nil {
				continue
			}
			if str(d, "currency") != "XTR" && len(manualMethods(store, str(d, "currency"))) == 0 {
				continue
			}
			valid = append(valid, d)
			cat := fallback(str(d, "category"), botText(cfg, "Subscriptions", "اشتراک‌ها"))
			if !seen[cat] {
				cats = append(cats, cat)
				seen[cat] = true
			}
		}
		rows := [][]any{}
		category := ""
		if strings.HasPrefix(action, "category:") {
			n, _ := strconv.Atoi(strings.TrimPrefix(action, "category:"))
			if n < 0 || n >= len(cats) {
				return failReply(fail(409, "catalog changed; reopen the shop"))
			}
			category = cats[n]
		}
		if len(cats) > 1 && category == "" {
			for i, c := range cats {
				rows = append(rows, []any{botButton(c, fmt.Sprintf("vs:category:%d", i))})
			}
		} else {
			for _, d := range valid {
				if category != "" && fallback(str(d, "category"), botText(cfg, "Subscriptions", "اشتراک‌ها")) != category {
					continue
				}
				rows = append(rows, []any{botButton(fmt.Sprintf("%s · %s", str(d, "name"), botAmount(d)), fmt.Sprintf("vs:offer:%d", num(d, "id")))})
				if len(rows) >= 30 {
					break
				}
			}
		}
		rows = append(rows, []any{botButton(botText(cfg, "Main menu", "منوی اصلی"), "vs:home")})
		reply["reply_markup"] = Data{"inline_keyboard": rows}
		reply["text"] = salesText(store, cfg, "catalog_text", "Choose a subscription", "اشتراک موردنظرتان را انتخاب کنید", user)
		if len(valid) == 0 {
			reply["text"] = salesText(store, cfg, "empty_text", "No subscriptions are available yet.", "فعلاً اشتراکی برای فروش موجود نیست.", user)
		}
		if str(b, "catalog_image") != "" {
			reply["photo"] = str(b, "catalog_image")
		}
	case strings.HasPrefix(action, "offer:"):
		id, _ := strconv.ParseInt(strings.TrimPrefix(action, "offer:"), 10, 64)
		d, e := offer(ctx, s.Store.Pool, id)
		if e != nil || num(d, "owner_id") != owner || !flag(d, "enabled") {
			return failReply(fail(404, "offer unavailable"))
		}
		nonce, e := s.salesConfirmation(ctx, owner, user, id, "purchase")
		if e != nil {
			return failReply(e)
		}
		reply["text"] = fmt.Sprintf("%s\n%s\n%d %s · %s\n%s\n\n%s", str(d, "name"), botAmount(d), num(d, "duration_days"), botText(cfg, "days", "روز"), quotaText(Data{"traffic_limit_bytes": num(d, "traffic_bytes")}), str(d, "description"), botText(cfg, "Confirm to accept the purchase terms and create this invoice.", "با تأیید، شرایط خرید را می‌پذیرید و فاکتور ساخته می‌شود."))
		reply["reply_markup"] = botKeyboard([]any{Data{"text": botText(cfg, "Purchase terms", "شرایط خرید"), "url": str(store, "terms_url")}}, []any{botButton(botText(cfg, "Confirm purchase", "تأیید خرید"), "vs:confirm:"+nonce)}, []any{botButton(botText(cfg, "Back", "بازگشت"), "vs:buy")})
		if str(d, "image_url") != "" {
			reply["photo"] = str(d, "image_url")
		}
	case strings.HasPrefix(action, "confirm:"):
		nonce := strings.TrimPrefix(action, "confirm:")
		var kind string
		var target int64
		var used bool
		e = s.Store.Pool.QueryRow(ctx, `SELECT kind,target_id,used FROM vr_bot_confirmations WHERE id=$1 AND owner_id=$2 AND user_id=$3 AND expires_at>now()`, nonce, owner, user).Scan(&kind, &target, &used)
		if e != nil {
			return failReply(fail(409, "confirmation expired; try again"))
		}
		if kind == "purchase" {
			v, e := offer(ctx, s.Store.Pool, target)
			if e != nil || num(v, "owner_id") != owner {
				return failReply(fail(404, "offer not found"))
			}
			purchaseCtx := context.WithValue(context.WithValue(ctx, telegramBuyerKey{}, user), telegramCheckoutKey{}, "bot:"+nonce)
			order, e := s.createOrder(purchaseCtx, Data{"offer_id": target, "kind": "purchase", "name": fmt.Sprintf("tg-%d", user), "accept_terms": true}, 0, true)
			if e != nil {
				return failReply(e)
			}
			return s.salesOrderReply(ctx, cfg, store, user, str(order, "id"))
		}
		if used {
			return failReply(fail(409, "confirmation already used"))
		}
		tag, e := s.Store.Pool.Exec(ctx, `UPDATE vr_bot_confirmations SET used=true WHERE id=$1 AND NOT used AND owner_id=$2 AND user_id=$3`, nonce, owner, user)
		if e != nil || tag.RowsAffected() != 1 {
			return failReply(fail(409, "confirmation already used"))
		}
		if _, e = s.salesServiceChange(ctx, owner, user, target, kind); e != nil {
			return failReply(e)
		}
		return s.salesServiceReply(ctx, cfg, store, user, target)
	case action == "services":
		cs, e := s.salesCustomers(ctx, owner, user)
		if e != nil {
			return failReply(e)
		}
		rows := [][]any{}
		for _, c := range cs {
			status := "🔴"
			if flag(c, "enabled") {
				status = "🟢"
			}
			rows = append(rows, []any{botButton(status+" "+str(c, "name"), fmt.Sprintf("vs:service:%d", num(c, "id")))})
		}
		rows = append(rows, []any{botButton(botText(cfg, "Buy subscription", "خرید اشتراک"), "vs:buy"), botButton(botText(cfg, "Main menu", "منوی اصلی"), "vs:home")})
		reply["text"] = botText(cfg, "Your subscriptions", "سرویس‌های شما")
		if len(cs) == 0 {
			reply["text"] = salesText(store, cfg, "empty_text", "You have no subscriptions yet.", "هنوز اشتراکی ندارید.", user)
		}
		reply["reply_markup"] = Data{"inline_keyboard": rows}
	case strings.HasPrefix(action, "service:"):
		id, _ := strconv.ParseInt(strings.TrimPrefix(action, "service:"), 10, 64)
		return s.salesServiceReply(ctx, cfg, store, user, id)
	case strings.HasPrefix(action, "change:"):
		parts := strings.Split(action, ":")
		if len(parts) != 3 {
			return failReply(fail(400, "invalid action"))
		}
		id, _ := strconv.ParseInt(parts[2], 10, 64)
		if _, e = s.salesCustomer(ctx, owner, user, id); e != nil {
			return failReply(e)
		}
		if !oneOf(parts[1], "enable", "disable", "rotate") {
			return failReply(fail(400, "invalid action"))
		}
		nonce, e := s.salesConfirmation(ctx, owner, user, id, parts[1])
		if e != nil {
			return failReply(e)
		}
		reply["text"] = botText(cfg, "Confirm this subscription change? Rotating a link invalidates the old links.", "تغییر سرویس را تأیید می‌کنید؟ تغییر لینک، لینک‌های قبلی را باطل می‌کند.")
		reply["reply_markup"] = botKeyboard([]any{botButton(botText(cfg, "Confirm", "تأیید"), "vs:confirm:"+nonce), botButton(botText(cfg, "Cancel", "انصراف"), fmt.Sprintf("vs:service:%d", id))})
	case action == "trial":
		c, e := s.claimSalesTrial(ctx, owner, user)
		if e != nil {
			if str(b, "trial_channel_url") != "" {
				return Data{"text": e.Error(), "reply_markup": botKeyboard([]any{Data{"text": botText(cfg, "Join channel", "عضویت در کانال"), "url": str(b, "trial_channel_url")}}, []any{botButton(botText(cfg, "Check again", "بررسی مجدد"), "vs:trial")})}
			}
			return failReply(e)
		}
		reply["text"] = salesText(store, cfg, "trial_text", "Your trial subscription is ready.", "اشتراک آزمایشی شما آماده است.", user) + "\n" + str(c, "subscription_url")
		reply["protect_content"] = true
	case action == "account":
		var count int64
		_ = s.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM vr_orders o JOIN vr_telegram_orders t ON t.order_id=o.id WHERE o.owner_id=$1 AND t.user_id=$2 AND o.state='fulfilled'`, owner, user).Scan(&count)
		reply["text"] = fmt.Sprintf("%s\nID: %d\n%s: %d", str(store, "name"), user, botText(cfg, "Completed orders", "سفارش‌های تکمیل‌شده"), count)
		reply["reply_markup"] = botKeyboard([]any{botButton(botText(cfg, "My orders", "سفارش‌های من"), "vs:orders"), botButton(botText(cfg, "My services", "سرویس‌های من"), "vs:services")}, []any{botButton(botText(cfg, "Main menu", "منوی اصلی"), "vs:home")})
	case action == "orders":
		rows, e := commerceRows(ctx, s.Store.Pool, orderSelect+`WHERE owner_id=$1 AND id IN(SELECT order_id FROM vr_telegram_orders WHERE user_id=$2) ORDER BY created_at DESC LIMIT 20`, owner, user)
		if e != nil {
			return failReply(e)
		}
		buttons := [][]any{}
		for _, o := range rows {
			buttons = append(buttons, []any{botButton(str(o, "state")+" · "+str(o, "id")[:8], "vs:order:"+str(o, "id"))})
		}
		buttons = append(buttons, []any{botButton(botText(cfg, "Main menu", "منوی اصلی"), "vs:home")})
		reply["text"] = botText(cfg, "Your recent orders", "آخرین سفارش‌های شما")
		reply["reply_markup"] = Data{"inline_keyboard": buttons}
	case strings.HasPrefix(action, "order:"):
		return s.salesOrderReply(ctx, cfg, store, user, strings.TrimPrefix(action, "order:"))
	case strings.HasPrefix(action, "pay:"):
		p := strings.Split(action, ":")
		if len(p) != 3 || !oneOf(p[1], "card", "usdt") {
			return failReply(fail(400, "invalid payment option"))
		}
		var yes bool
		e = s.Store.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vr_orders o JOIN vr_telegram_orders t ON t.order_id=o.id WHERE o.id=$1 AND o.owner_id=$2 AND t.user_id=$3)`, p[2], owner, user).Scan(&yes)
		if e != nil || !yes {
			return failReply(fail(404, "order not found"))
		}
		o, e := getOrder(ctx, s.Store.Pool, p[2], false)
		if e != nil {
			return failReply(e)
		}
		var method Data
		for _, v := range manualMethods(store, str(o, "currency")) {
			d := asData(v)
			if str(d, "method") == p[1] {
				method = d
			}
		}
		if method == nil {
			return failReply(fail(409, "payment option unavailable"))
		}
		if e = s.saveSalesSession(ctx, owner, user, Data{"step": "receipt", "method": p[1], "order_id": p[2]}); e != nil {
			return failReply(e)
		}
		reply["text"] = botAmount(o) + "\n"
		if p[1] == "card" {
			reply["text"] = str(reply, "text") + str(method, "card_number") + "\n" + str(method, "card_holder") + "\n" + str(method, "instructions") + "\n" + botText(cfg, "Send a photo of your bank transfer receipt. /cancel", "عکس رسید کارت‌به‌کارت را ارسال کنید. /cancel")
		} else {
			reply["text"] = str(reply, "text") + "USDT · " + str(method, "network") + "\n" + str(method, "address") + "\n" + str(method, "instructions") + "\n" + botText(cfg, "Send the transaction hash after transferring on this exact network. Approval is manual. /cancel", "پس از انتقال روی همین شبکه، هش تراکنش را ارسال کنید. تأیید توسط مدیر انجام می‌شود. /cancel")
		}
		reply["protect_content"] = true
		reply["reply_markup"] = botKeyboard([]any{botButton(botText(cfg, "Cancel", "انصراف"), "vs:home")})
	case action == "terms":
		reply["text"] = str(store, "terms_url")
	case action == "support":
		reply["text"] = salesText(store, cfg, "support_text", "Send /ticket followed by your message, or contact support.", "برای ثبت تیکت، /ticket و سپس پیام خود را بفرستید یا با پشتیبانی تماس بگیرید.", user)
		reply["reply_markup"] = botKeyboard([]any{Data{"text": botText(cfg, "Contact support", "تماس با پشتیبانی"), "url": str(store, "support_url")}}, []any{Data{"text": botText(cfg, "Support tickets", "تیکت‌های پشتیبانی"), "web_app": Data{"url": shop + "&view=support"}}}, []any{botButton(botText(cfg, "Main menu", "منوی اصلی"), "vs:home")})
	case strings.HasPrefix(action, "/ticket "):
		d, e := s.writeTicket(ctx, owner, user, 0, "customer", "", strings.TrimSpace(strings.TrimPrefix(action, "/ticket ")), false)
		if e != nil {
			return failReply(e)
		}
		reply["text"] = fmt.Sprintf("%s #%d", botText(cfg, "Ticket created", "تیکت ثبت شد"), num(d, "id"))
	case action == "tutorial":
		rows := [][]any{}
		for i, v := range array(b, "tutorials") {
			d := asData(v)
			rows = append(rows, []any{botButton(str(d, "title"), fmt.Sprintf("vs:tutorial:%d", i))})
		}
		rows = append(rows, []any{botButton(botText(cfg, "Main menu", "منوی اصلی"), "vs:home")})
		reply["reply_markup"] = Data{"inline_keyboard": rows}
		reply["text"] = salesText(store, cfg, "tutorial_text", "Choose your device or app.", "دستگاه یا برنامه خود را انتخاب کنید.", user)
		if str(b, "tutorial_image") != "" {
			reply["photo"] = str(b, "tutorial_image")
		}
	case strings.HasPrefix(action, "tutorial:") || strings.HasPrefix(action, "custom:"):
		p := strings.Split(action, ":")
		i, _ := strconv.Atoi(p[1])
		key := "tutorials"
		if p[0] == "custom" {
			key = "custom_buttons"
		}
		values := array(b, key)
		if i < 0 || i >= len(values) {
			return failReply(fail(404, "content not found"))
		}
		d := asData(values[i])
		reply["text"] = fallback(str(d, "text"), str(d, "url"))
		if str(d, "image") != "" {
			reply["photo"] = str(d, "image")
		}
		if reply["text"] == "" {
			reply["text"] = str(d, "title")
		}
	}
	return reply
}
func botAmount(d Data) string {
	n := num(d, "amount")
	switch str(d, "currency") {
	case "USDT":
		return fmt.Sprintf("%d.%06d USDT", n/1_000_000, n%1_000_000)
	case "USD", "EUR":
		return fmt.Sprintf("%d.%02d %s", n/100, n%100, str(d, "currency"))
	default:
		return fmt.Sprintf("%d %s", n, str(d, "currency"))
	}
}
func (s *Server) salesServiceReply(ctx context.Context, cfg, store Data, user, id int64) Data {
	c, e := s.salesCustomerView(ctx, num(store, "_owner_id"), user, id)
	if e != nil {
		// getStore deliberately contains no synthetic ownership key; call sites set it.
		return Data{"text": e.Error()}
	}
	remaining := botText(cfg, "Unlimited", "نامحدود")
	if !flag(c, "unlimited") {
		remaining = fmtBytes(num(c, "remaining_bytes"))
	}
	rows := [][]any{{botButton(botText(cfg, "Refresh", "بروزرسانی"), fmt.Sprintf("vs:service:%d", id))}, {Data{"text": botText(cfg, "Renew / add traffic", "تمدید / افزایش حجم"), "web_app": Data{"url": strings.TrimRight(s.Config.PublicURL, "/") + fmt.Sprintf("/shop?store=%d&service=%d", num(store, "_owner_id"), id)}}}}
	b := salesConfig(store)
	if flag(b, "allow_toggle") {
		a, label := "enable", botText(cfg, "Enable", "روشن کردن")
		if flag(c, "enabled") {
			a, label = "disable", botText(cfg, "Disable", "خاموش کردن")
		}
		rows = append(rows, []any{botButton(label, fmt.Sprintf("vs:change:%s:%d", a, id))})
	}
	if flag(b, "allow_rotate") {
		rows = append(rows, []any{botButton(botText(cfg, "Change subscription link", "تغییر لینک اشتراک"), fmt.Sprintf("vs:change:rotate:%d", id))})
	}
	rows = append(rows, []any{botButton(botText(cfg, "My services", "سرویس‌های من"), "vs:services")})
	return Data{"text": fmt.Sprintf("%s\n%s: %s\n%s: %s\n%s: %s\n%s", str(c, "name"), botText(cfg, "Used", "مصرف"), fmtBytes(num(c, "used_traffic_bytes")), botText(cfg, "Remaining", "باقی‌مانده"), remaining, botText(cfg, "Expiry", "انقضا"), fallback(str(c, "expires_at"), "—"), str(c, "subscription_url")), "protect_content": true, "reply_markup": Data{"inline_keyboard": rows}}
}
func (s *Server) salesOrderReply(ctx context.Context, cfg, store Data, user int64, id string) Data {
	var owner int64
	e := s.Store.Pool.QueryRow(ctx, `SELECT o.owner_id FROM vr_orders o JOIN vr_telegram_orders t ON t.order_id=o.id WHERE o.id=$1 AND t.user_id=$2`, id, user).Scan(&owner)
	if e != nil || owner != num(store, "_owner_id") {
		return Data{"text": "Order not found"}
	}
	o, e := getOrder(ctx, s.Store.Pool, id, false)
	if e != nil {
		return Data{"text": e.Error()}
	}
	rows := [][]any{}
	reply := Data{"text": fmt.Sprintf("#%s\n%s\n%s", id, botAmount(o), str(o, "state")), "protect_content": true}
	if str(o, "state") == "pending" {
		proof, e := s.paymentProof(ctx, s.Store.Pool, id, false)
		if e != nil {
			return Data{"text": e.Error()}
		}
		if proof != nil {
			reply["text"] = str(reply, "text") + "\n" + proofBotDescription(proof) + "\n" + str(proof, "review_note")
		}
		if str(o, "currency") == "XTR" {
			out, _, e := s.requestStarsPayment(ctx, id)
			if e != nil {
				return Data{"text": e.Error()}
			}
			rows = append(rows, []any{Data{"text": botText(cfg, "Pay with Stars", "پرداخت با استارز"), "url": str(out.(Data), "url")}})
		} else if proof == nil || str(proof, "state") == "rejected" {
			for _, v := range manualMethods(store, str(o, "currency")) {
				m := asData(v)
				label := botText(cfg, "Bank transfer · send receipt", "کارت‌به‌کارت · ارسال رسید")
				if str(m, "method") == "usdt" {
					label = "USDT · " + str(m, "network")
				}
				rows = append(rows, []any{botButton(label, "vs:pay:"+str(m, "method")+":"+id)})
			}
		}
	}
	if str(o, "state") == "fulfilled" && num(o, "customer_id") > 0 {
		c, e := s.salesCustomerView(ctx, owner, user, num(o, "customer_id"))
		if e == nil {
			reply["text"] = salesText(store, cfg, "delivery_text", "Your subscription is ready.", "اشتراک شما آماده است.", user) + "\n" + str(c, "subscription_url")
		}
	}
	rows = append(rows, []any{botButton(botText(cfg, "Refresh", "بروزرسانی"), "vs:order:"+id), botButton(botText(cfg, "Main menu", "منوی اصلی"), "vs:home")})
	reply["reply_markup"] = Data{"inline_keyboard": rows}
	return reply
}
func (s *Server) telegramReceiptImage(ctx context.Context, cfg Data, fileID string) (string, error) {
	token, e := unseal(s.Config.FieldKey, str(cfg, "_telegram_token"))
	if e != nil {
		return "", e
	}
	file, e := s.telegramRequest(ctx, token, "getFile", Data{"file_id": fileID})
	if e != nil {
		return "", e
	}
	d := obj(file, "result")
	path := str(d, "file_path")
	if num(d, "file_size") > 1<<20 || path == "" || strings.Contains(path, "..") || strings.HasPrefix(path, "/") {
		return "", fail(400, "send a receipt photo smaller than 1 MiB")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, e := http.NewRequestWithContext(ctx, "GET", "https://api.telegram.org/file/bot"+token+"/"+strings.Join(func() []string {
		parts := strings.Split(path, "/")
		for i, p := range parts {
			parts[i] = url.PathEscape(p)
		}
		return parts
	}(), "/"), nil)
	if e != nil {
		return "", e
	}
	client := s.TelegramHTTP
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, e := copyClient.Do(req)
	if e != nil {
		return "", fail(502, "receipt download failed")
	}
	defer res.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(res.Body, (1<<20)+1))
	if e != nil || res.StatusCode != 200 || len(raw) > 1<<20 {
		return "", fail(400, "invalid or oversized receipt image")
	}
	value := "data:" + http.DetectContentType(raw) + ";base64," + base64.StdEncoding.EncodeToString(raw)
	if e = validateProofImage(value); e != nil {
		return "", e
	}
	return value, nil
}
