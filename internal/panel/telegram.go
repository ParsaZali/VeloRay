package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) telegramRequest(ctx context.Context, token, method string, in any) (Data, error) {
	if oneOf(method, "getUpdates", "answerPreCheckoutQuery") {
		limited, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		ctx = limited
	}
	b, e := json.Marshal(in)
	if e != nil {
		return nil, e
	}
	req, e := http.NewRequestWithContext(ctx, "POST", "https://api.telegram.org/bot"+token+"/"+method, bytes.NewReader(b))
	if e != nil {
		return nil, e
	}
	req.Header.Set("Content-Type", "application/json")
	client := s.TelegramHTTP
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	res, e := client.Do(req)
	if e != nil {
		return nil, errors.New("Telegram connection failed")
	}
	defer res.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if e != nil {
		return nil, e
	}
	v, e := decode(raw)
	if e != nil || res.StatusCode != 200 || !flag(v, "ok") {
		return nil, errors.New("Telegram request failed")
	}
	return v, nil
}

func telegramOwners(cfg Data) map[string]bool {
	owners := map[string]bool{}
	for _, v := range array(cfg, "telegram_owner_ids") {
		if id, ok := v.(string); ok {
			owners[id] = true
		}
	}
	return owners
}
func telegramPermitted(cfg, message, from Data) bool {
	chat := obj(message, "chat")
	return telegramOwners(cfg)[strconv.FormatInt(num(from, "id"), 10)] && str(chat, "type") == "private" && num(chat, "id") == num(from, "id")
}
func botText(cfg Data, en, fa string) string {
	if str(cfg, "telegram_locale") == "fa" {
		return fa
	}
	return en
}
func botButton(label, callback string) Data { return Data{"text": label, "callback_data": callback} }
func botKeyboard(rows ...[]any) Data        { return Data{"inline_keyboard": rows} }
func botHome(cfg Data) Data {
	return botKeyboard([]any{botButton(botText(cfg, "Status", "وضعیت"), "vr:/status"), botButton(botText(cfg, "Clients", "کلاینت‌ها"), "vr:/clients 1")}, []any{botButton(botText(cfg, "Nodes", "نودها"), "vr:/nodes"), botButton(botText(cfg, "Help", "راهنما"), "vr:/help")})
}

func (s *Server) telegram(ctx context.Context, cfg Data) error {
	if e := s.telegramPoll(ctx, cfg); e != nil {
		return e
	}
	return s.telegramReplies(ctx, cfg)
}
func (s *Server) telegramReplies(ctx context.Context, cfg Data) error {
	conn, e := s.Store.Pool.Acquire(ctx)
	if e != nil {
		return e
	}
	defer conn.Release()
	var locked bool
	if e = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(842100,32)`).Scan(&locked); e != nil {
		return e
	}
	if !locked {
		return nil
	}
	defer conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock(842100,32)`)
	token, e := unseal(s.Config.FieldKey, str(cfg, "_telegram_token"))
	if e != nil || token == "" {
		return errors.New("bot token is missing")
	}
	updates, e := s.telegramInbox(ctx, "message", telegramBotID(token))
	if e != nil {
		return e
	}
	for _, u := range updates {
		id := num(u, "update_id")
		m := obj(u, "message")
		from := obj(m, "from")
		text := str(m, "text")
		callback := obj(u, "callback_query")
		if len(callback) > 0 {
			m = obj(callback, "message")
			from = obj(callback, "from")
			text = str(callback, "data")
			if strings.HasPrefix(text, "vr:") {
				text = strings.TrimPrefix(text, "vr:")
			} else if !strings.HasPrefix(text, "vs:") {
				text = ""
			}
		}
		sales := s.telegramSalesRoute(ctx, text, m, from)
		permitted := sales || telegramPermitted(cfg, m, from)
		owner := strconv.FormatInt(num(from, "id"), 10)
		var cached []byte
		var sent *time.Time
		e = s.Store.Pool.QueryRow(ctx, `SELECT reply,sent_at FROM vr_telegram_updates WHERE id=$1 AND bot_id=$2`, id, telegramBotID(token)).Scan(&cached, &sent)
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if permitted && sent == nil {
			if len(callback) > 0 {
				_, _ = s.telegramRequest(ctx, token, "answerCallbackQuery", Data{"callback_query_id": str(callback, "id")})
			}
			if len(cached) == 0 {
				reply := Data{"text": botText(cfg, "Too many commands; try again shortly", "تعداد درخواست‌ها زیاد است؛ کمی بعد دوباره تلاش کنید"), "reply_markup": botHome(cfg)}
				if e = rateLimit(ctx, s.Store.Pool, "telegram:"+owner, 30, time.Minute); e == nil {
					if sales {
						reply = s.telegramSalesReply(ctx, cfg, text, num(from, "id"))
					} else {
						reply = s.telegramReply(ctx, cfg, text, owner)
					}
				}
				reply["chat_id"], reply["disable_web_page_preview"] = num(obj(m, "chat"), "id"), true
				cached, _ = json.Marshal(reply)
				if sales {
					sealed, err := seal(s.Config.FieldKey, string(cached))
					if err != nil {
						return err
					}
					cached, _ = json.Marshal(Data{"_sealed_sales": sealed})
				}
				if _, e = s.Store.Pool.Exec(ctx, `INSERT INTO vr_telegram_updates(id,reply,bot_id) VALUES($1,$2,$3) ON CONFLICT(bot_id,id) DO UPDATE SET reply=excluded.reply`, id, string(cached), telegramBotID(token)); e != nil {
					return e
				}
			}
			reply, e := decode(cached)
			if e != nil {
				return e
			}
			if sealed := str(reply, "_sealed_sales"); sealed != "" {
				plain, err := unseal(s.Config.FieldKey, sealed)
				if err != nil {
					return err
				}
				reply, e = decode([]byte(plain))
				if e != nil {
					return e
				}
			}
			if _, e = s.telegramRequest(ctx, token, "sendMessage", reply); e != nil {
				return e
			}
		}
		if _, e = s.Store.Pool.Exec(ctx, `INSERT INTO vr_telegram_updates(id,sent_at,bot_id) VALUES($1,now(),$2) ON CONFLICT(bot_id,id) DO UPDATE SET sent_at=now()`, id, telegramBotID(token)); e != nil {
			return e
		}
		if _, e = s.Store.Pool.Exec(ctx, `UPDATE vr_telegram_inbox SET processed_at=now() WHERE id=$1 AND bot_id=$2`, id, telegramBotID(token)); e != nil {
			return e
		}
	}
	if e = s.telegramOrderNotifications(ctx, cfg, token); e != nil {
		return e
	}
	if e = s.telegramAlerts(ctx, cfg, token); e != nil {
		return e
	}
	_, e = s.Store.Pool.Exec(ctx, `UPDATE vr_settings SET data=jsonb_set(jsonb_set(data,'{_telegram_success}',to_jsonb($1::text)),'{_telegram_error}','""') WHERE id=1`, stamp())
	return e
}

func (s *Server) telegramReply(ctx context.Context, cfg Data, text, owner string) Data {
	reply := Data{"text": botText(cfg, "Use /help", "از /help استفاده کنید"), "reply_markup": botHome(cfg)}
	p := strings.Fields(text)
	if len(p) == 0 {
		return reply
	}
	cmd := strings.ToLower(strings.Split(p[0], "@")[0])
	message := ""
	switch cmd {
	case "/help", "/start":
		message = botText(cfg, "VeloRay "+Version+"\n/status · /nodes · /clients [page] · /search name\n/client ID · /link ID\n/client ID enable|disable|reset\n/node ID deploy|restart\nChanges require confirmation and enabled bot management.", "VeloRay "+Version+"\n/status وضعیت · /nodes نودها\n/clients [page] کلاینت‌ها · /search name جستجو\n/client ID جزئیات · /link ID اشتراک\n/client ID enable|disable|reset\n/node ID deploy|restart\nتغییرات نیازمند فعال‌سازی مدیریت و تأیید جداگانه هستند.")
	case "/status":
		o, e := s.overview(ctx)
		if e != nil {
			message = "Status unavailable"
			break
		}
		message = fmt.Sprintf("VeloRay\nNodes: %d/%d healthy\nConnections: %d/%d active\nBilled: %s\nActual: %s", num(o, "nodes_online"), num(o, "nodes_total"), num(o, "clients_active"), num(o, "clients_total"), fmtBytes(num(o, "traffic_used_bytes")), fmtBytes(num(o, "traffic_raw_bytes")))
	case "/nodes":
		ns, e := list(ctx, s.Store.Pool, "nodes", "ORDER BY id LIMIT 15")
		if e != nil {
			message = "Nodes unavailable"
			break
		}
		rows := []string{}
		for _, n := range ns {
			rows = append(rows, fmt.Sprintf("%d · %s · %s", num(n, "id"), str(n, "name"), str(n, "status")))
		}
		message = fallback(strings.Join(rows, "\n"), "No nodes")
	case "/clients", "/search":
		page := int64(1)
		query := ""
		if cmd == "/search" {
			query = strings.TrimSpace(strings.TrimPrefix(text, p[0]))
			if query == "" {
				message = "Use /search name"
				break
			}
		} else if len(p) > 1 {
			page, _ = strconv.ParseInt(p[1], 10, 64)
		}
		if page < 1 || page > 10000 {
			message = "Invalid page"
			break
		}
		cs, e := list(ctx, s.Store.Pool, "clients", "WHERE data->>'name' ILIKE $1 ORDER BY id LIMIT 11 OFFSET $2", "%"+query+"%", (page-1)*10)
		if e != nil {
			message = "Clients unavailable"
			break
		}
		rows := []string{}
		buttons := []any{}
		for _, c := range cs[:min(10, len(cs))] {
			rows = append(rows, fmt.Sprintf("%d · %s · %s / %s", num(c, "id"), str(c, "name"), fmtBytes(num(c, "used_traffic_bytes")), quotaText(c)))
			buttons = append(buttons, []any{botButton(fmt.Sprintf("%d · %s", num(c, "id"), shortBotName(str(c, "name"))), fmt.Sprintf("vr:/client %d", num(c, "id")))})
		}
		if cmd == "/clients" {
			nav := []any{}
			if page > 1 {
				nav = append(nav, botButton("‹", fmt.Sprintf("vr:/clients %d", page-1)))
			}
			if len(cs) > 10 {
				nav = append(nav, botButton("›", fmt.Sprintf("vr:/clients %d", page+1)))
			}
			if len(nav) > 0 {
				buttons = append(buttons, nav)
			}
		}
		buttons = append(buttons, []any{botButton("VeloRay", "vr:/status")})
		reply["reply_markup"] = Data{"inline_keyboard": buttons}
		message = fallback(strings.Join(rows, "\n"), "No clients found")
	case "/client", "/link", "/node":
		if len(p) < 2 || len(p) > 3 {
			message = "Use /client ID or /node ID deploy|restart"
			break
		}
		id, e := strconv.ParseInt(p[1], 10, 64)
		if e != nil || id < 1 {
			message = "Invalid ID"
			break
		}
		if len(p) == 3 {
			kind := "clients"
			if cmd == "/node" {
				kind = "nodes"
			}
			return s.telegramConfirm(ctx, cfg, owner, kind, id, p[2])
		}
		if cmd == "/node" {
			message = "Use /node ID deploy|restart"
			break
		}
		c, e := get(ctx, s.Store.Pool, "clients", id)
		if e != nil {
			message = "Client not found"
			break
		}
		if cmd == "/link" {
			view, e := s.view(ctx, s.Store.Pool, "clients", c)
			if e != nil {
				message = "Subscription unavailable"
				break
			}
			message = str(view, "subscription_url")
			break
		}
		rate := effectiveTrafficMultiplier(c)
		message = fmt.Sprintf("%d · %s\nEnabled: %t\nBilled: %s / %s\nActual: %s\nMultiplier: %v → %.3fx\nExpiry: %s", id, str(c, "name"), flag(c, "enabled"), fmtBytes(num(c, "used_traffic_bytes")), quotaText(c), fmtBytes(num(c, "raw_used_traffic_bytes")), c["traffic_multiplier"], rate, fallback(str(c, "expires_at"), "Never"))
		reply["reply_markup"] = botKeyboard([]any{botButton("Enable", fmt.Sprintf("vr:/client %d enable", id)), botButton("Disable", fmt.Sprintf("vr:/client %d disable", id))}, []any{botButton("Reset usage", fmt.Sprintf("vr:/client %d reset", id)), botButton("Subscription", fmt.Sprintf("vr:/link %d", id))}, []any{botButton("‹ Clients", "vr:/clients 1")})
	case "/confirm":
		if len(p) != 2 {
			break
		}
		return s.telegramExecute(ctx, cfg, owner, p[1])
	case "/cancel":
		if len(p) != 2 {
			break
		}
		result, err := s.Store.Pool.Exec(ctx, `UPDATE vr_telegram_actions SET used=true WHERE id=$1 AND owner_id=$2 AND NOT used`, p[1], owner)
		if err != nil || result.RowsAffected() != 1 {
			message = botText(cfg, "Confirmation expired or already used", "تأیید منقضی یا قبلاً استفاده شده است")
		} else {
			message = botText(cfg, "Action cancelled", "عملیات لغو شد")
		}
	}
	if message != "" {
		reply["text"] = message
	}
	return reply
}
func shortBotName(name string) string { r := []rune(name); return string(r[:min(len(r), 32)]) }
func quotaText(c Data) string {
	if num(c, "traffic_limit_bytes") == 0 {
		return "Unlimited"
	}
	return fmtBytes(num(c, "traffic_limit_bytes"))
}

func (s *Server) telegramConfirm(ctx context.Context, cfg Data, owner, kind string, id int64, action string) Data {
	reply := Data{"text": "Enable bot management in the panel to make changes", "reply_markup": botHome(cfg)}
	if !flag(cfg, "telegram_allow_changes") {
		return reply
	}
	if flag(cfg, "require_2fa_for_admins") {
		reply["text"] = "Use the panel for changes when two-factor authentication is required"
		return reply
	}
	if !(kind == "clients" && oneOf(action, "enable", "disable", "reset") || kind == "nodes" && oneOf(action, "deploy", "restart")) {
		reply["text"] = "Unsupported action"
		return reply
	}
	d, e := get(ctx, s.Store.Pool, kind, id)
	if e != nil {
		reply["text"] = "Record not found"
		return reply
	}
	nonce := randomToken(18)
	if _, e = s.Store.Pool.Exec(ctx, `INSERT INTO vr_telegram_actions(id,owner_id,kind,target_id,action) VALUES($1,$2,$3,$4,$5)`, nonce, owner, kind, id, action); e != nil {
		reply["text"] = "Confirmation unavailable"
		return reply
	}
	reply["text"] = fmt.Sprintf("Confirm %s for %s?\nExpires in 5 minutes. Shared account changes affect every connection.", action, str(d, "name"))
	reply["reply_markup"] = botKeyboard([]any{botButton(botText(cfg, "Confirm", "تأیید"), "vr:/confirm "+nonce), botButton(botText(cfg, "Cancel", "لغو"), "vr:/cancel "+nonce)})
	return reply
}

func (s *Server) telegramExecute(ctx context.Context, cfg Data, owner, nonce string) Data {
	reply := Data{"text": "Confirmation expired or already used; check the panel", "reply_markup": botHome(cfg)}
	if !flag(cfg, "telegram_allow_changes") || flag(cfg, "require_2fa_for_admins") {
		reply["text"] = "Use the panel for changes"
		return reply
	}
	var kind, action string
	var id int64
	if e := s.Store.Pool.QueryRow(ctx, `UPDATE vr_telegram_actions SET used=true WHERE id=$1 AND owner_id=$2 AND NOT used AND expires_at>now() RETURNING kind,target_id,action`, nonce, owner).Scan(&kind, &id, &action); e != nil {
		return reply
	}
	req, _ := http.NewRequestWithContext(ctx, "POST", "http://localhost/", strings.NewReader("{}"))
	var e error
	if kind == "clients" {
		c, err := get(ctx, s.Store.Pool, "clients", id)
		if err != nil {
			reply["text"] = "Client not found"
			return reply
		}
		node, err := inboundNode(ctx, s.Store.Pool, num(c, "inbound"))
		if err != nil {
			reply["text"] = "Inbound not found"
			return reply
		}
		_, e = s.change(ctx, []int64{node}, Actor{}, "telegram.client-"+action, owner, func(tx pgx.Tx) (any, error) {
			c, err := get(ctx, tx, "clients", id)
			if err != nil {
				return nil, err
			}
			if err = applyClientAction(c, action); err != nil {
				return nil, err
			}
			return nil, save(ctx, tx, "clients", c)
		})
	} else {
		mapped := map[string]string{"deploy": "deploy", "restart": "xray-restart"}[action]
		_, _, e = s.action(discardResponse{}, req, "nodes", id, mapped, Actor{})
	}
	if e != nil {
		reply["text"] = "Change failed; review the node in VeloRay"
	} else {
		reply["text"] = botText(cfg, "Change applied", "تغییر اعمال شد")
	}
	_ = audit(ctx, s.Store.Pool, 0, "telegram.confirmed", owner, Data{"kind": kind, "id": id, "action": action, "success": e == nil})
	return reply
}

type discardResponse struct{}

func (discardResponse) Header() http.Header         { return http.Header{} }
func (discardResponse) Write(p []byte) (int, error) { return len(p), nil }
func (discardResponse) WriteHeader(int)             {}

func (s *Server) telegramAPI(w http.ResponseWriter, r *http.Request, path string, a Actor) (any, int, error) {
	if !flag(a.User, "is_superuser") || a.Scope != "admin" {
		return nil, 0, fail(403, "administrator access required")
	}
	if !(path == "telegram/status" && r.Method == "GET" || oneOf(path, "telegram/test", "telegram/setup") && r.Method == "POST") {
		return nil, 0, fail(405, "method not allowed")
	}
	ctx := r.Context()
	cfg, e := settings(ctx, s.Store.Pool)
	if e != nil {
		return nil, 0, e
	}
	token, e := unseal(s.Config.FieldKey, str(cfg, "_telegram_token"))
	if e != nil || token == "" {
		return nil, 0, fail(400, "Save the bot token in Settings first")
	}
	me, e := s.telegramRequest(ctx, token, "getMe", Data{})
	if e != nil {
		return nil, 0, fail(502, e.Error())
	}
	if _, e = s.Store.Pool.Exec(ctx, `UPDATE vr_settings SET data=jsonb_set(data,'{_telegram_username}',to_jsonb($1::text)) WHERE id=1`, str(obj(me, "result"), "username")); e != nil {
		return nil, 0, e
	}
	hook, e := s.telegramRequest(ctx, token, "getWebhookInfo", Data{})
	if e != nil {
		return nil, 0, fail(502, e.Error())
	}
	webhook := str(obj(hook, "result"), "url") != ""
	out := Data{"username": str(obj(me, "result"), "username"), "webhook_configured": webhook, "owner_count": len(telegramOwners(cfg)), "last_success": str(cfg, "_telegram_success"), "last_error": str(cfg, "_telegram_error")}
	if path == "telegram/status" {
		return out, 200, nil
	}
	if len(telegramOwners(cfg)) == 0 {
		return nil, 0, fail(400, "Configure at least one owner ID first")
	}
	if path == "telegram/setup" && webhook {
		return nil, 0, fail(409, "This token has a webhook. Remove it in the existing bot service before using VeloRay polling")
	}
	for owner := range telegramOwners(cfg) {
		if path == "telegram/test" {
			_, e = s.telegramRequest(ctx, token, "sendMessage", Data{"chat_id": owner, "text": botText(cfg, "VeloRay test succeeded. Owner-only bot is ready.", "تست VeloRay موفق بود. ربات ویژهٔ مدیر آماده است."), "reply_markup": botHome(cfg)})
		} else {
			commands := []any{}
			for _, pair := range [][2]string{{"status", "Panel and traffic status"}, {"clients", "List clients"}, {"search", "Find a client by name"}, {"client", "Client details and management"}, {"link", "Subscription address"}, {"nodes", "Node status"}, {"help", "Commands and help"}} {
				commands = append(commands, Data{"command": pair[0], "description": pair[1]})
			}
			_, e = s.telegramRequest(ctx, token, "setMyCommands", Data{"commands": commands, "scope": Data{"type": "chat", "chat_id": owner}})
		}
		if e != nil {
			return nil, 0, fail(502, "Telegram failed. Each owner must start the bot before testing")
		}
	}
	_ = audit(ctx, s.Store.Pool, num(a.User, "id"), path, "telegram", Data{})
	out["status"] = "ok"
	return out, 200, nil
}
