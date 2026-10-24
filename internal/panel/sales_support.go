package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) ticketMessages(ctx context.Context, id int64) ([]Data, error) {
	rows, e := commerceRows(ctx, s.Store.Pool, `SELECT jsonb_build_object('id',id,'author',author,'_body',body_ciphertext,'created_at',created_at) FROM vr_ticket_messages WHERE ticket_id=$1 ORDER BY id DESC LIMIT 100`, id)
	if e != nil {
		return nil, e
	}
	out := []Data{}
	for i := len(rows) - 1; i >= 0; i-- {
		d := rows[i]
		plain, e := unseal(s.Config.FieldKey, str(d, "_body"))
		if e != nil {
			return nil, e
		}
		delete(d, "_body")
		d["text"] = plain
		out = append(out, d)
	}
	return out, nil
}
func (s *Server) writeTicket(ctx context.Context, owner, user, id int64, author, subject, text string, closeTicket bool) (Data, error) {
	if len([]rune(text)) > 2000 || strings.TrimSpace(text) == "" && !closeTicket || len([]rune(subject)) > 100 {
		return nil, fail(400, "ticket text requires 1–2000 characters")
	}
	if e := rateLimit(ctx, s.Store.Pool, fmt.Sprintf("ticket:%d:%d", owner, user), 20, time.Hour); e != nil {
		return nil, e
	}
	tx, e := s.Store.Pool.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	if id == 0 {
		if subject == "" {
			subject = "Support request"
		}
		if e = tx.QueryRow(ctx, `INSERT INTO vr_support_tickets(owner_id,user_id,subject) VALUES($1,$2,$3) RETURNING id`, owner, user, subject).Scan(&id); e != nil {
			return nil, e
		}
	}
	var state string
	var ticketUser int64
	e = tx.QueryRow(ctx, `SELECT state,user_id FROM vr_support_tickets WHERE id=$1 AND owner_id=$2 FOR UPDATE`, id, owner).Scan(&state, &ticketUser)
	if e != nil || author == "customer" && ticketUser != user {
		return nil, fail(404, "ticket not found")
	}
	if state == "closed" && !closeTicket {
		return nil, fail(409, "ticket is closed; create a new ticket")
	}
	if strings.TrimSpace(text) != "" {
		sealed, e := seal(s.Config.FieldKey, text)
		if e != nil {
			return nil, e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO vr_ticket_messages(ticket_id,author,body_ciphertext) VALUES($1,$2,$3)`, id, author, sealed); e != nil {
			return nil, e
		}
	}
	if closeTicket {
		state = "closed"
	}
	_, e = tx.Exec(ctx, `UPDATE vr_support_tickets SET state=$2,updated_at=now() WHERE id=$1`, id, state)
	if e != nil {
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	return Data{"id": id, "state": state}, nil
}
func (s *Server) buyerTicketsAPI(w http.ResponseWriter, r *http.Request, path string, owner, user int64) (any, int, error) {
	ctx := r.Context()
	parts := strings.Split(path, "/")
	id := int64(0)
	if len(parts) == 4 {
		var e error
		id, e = strconv.ParseInt(parts[3], 10, 64)
		if e != nil || id < 1 {
			return nil, 0, fail(404, "ticket not found")
		}
	} else if len(parts) != 3 {
		return nil, 0, fail(404, "endpoint not found")
	}
	if r.Method == "GET" {
		if id == 0 {
			out, e := commerceRows(ctx, s.Store.Pool, `SELECT to_jsonb(t) FROM vr_support_tickets t WHERE owner_id=$1 AND user_id=$2 ORDER BY updated_at DESC LIMIT 100`, owner, user)
			return out, 200, e
		}
		var yes bool
		e := s.Store.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vr_support_tickets WHERE id=$1 AND owner_id=$2 AND user_id=$3)`, id, owner, user).Scan(&yes)
		if e != nil {
			return nil, 0, e
		}
		if !yes {
			return nil, 0, fail(404, "ticket not found")
		}
		out, e := s.ticketMessages(ctx, id)
		return out, 200, e
	}
	if r.Method != "POST" {
		return nil, 0, fail(405, "method not allowed")
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, 0, e
	}
	if e = allowed(d, "subject text close"); e != nil {
		return nil, 0, fail(400, e.Error())
	}
	out, e := s.writeTicket(ctx, owner, user, id, "customer", str(d, "subject"), str(d, "text"), flag(d, "close"))
	return out, 201, e
}
func (s *Server) ticketsAPI(w http.ResponseWriter, r *http.Request, path string, a Actor, owner int64) (any, int, error) {
	ctx := r.Context()
	parts := strings.Split(path, "/")
	if len(parts) == 1 && r.Method == "GET" {
		out, e := commerceRows(ctx, s.Store.Pool, `SELECT to_jsonb(t) FROM vr_support_tickets t WHERE owner_id=$1 ORDER BY updated_at DESC LIMIT 200`, owner)
		return out, 200, e
	}
	if len(parts) != 2 {
		return nil, 0, fail(404, "ticket not found")
	}
	id, e := strconv.ParseInt(parts[1], 10, 64)
	if e != nil {
		return nil, 0, fail(404, "ticket not found")
	}
	var user int64
	e = s.Store.Pool.QueryRow(ctx, `SELECT user_id FROM vr_support_tickets WHERE id=$1 AND owner_id=$2`, id, owner).Scan(&user)
	if e != nil {
		return nil, 0, fail(404, "ticket not found")
	}
	if r.Method == "GET" {
		out, e := s.ticketMessages(ctx, id)
		return out, 200, e
	}
	if r.Method != "POST" || !permitted(a.User, "commerce.write") {
		return nil, 0, fail(403, "support write permission required")
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, 0, e
	}
	if e = allowed(d, "text close"); e != nil {
		return nil, 0, fail(400, e.Error())
	}
	out, e := s.writeTicket(ctx, owner, user, id, "staff", "", str(d, "text"), flag(d, "close"))
	if e == nil {
		_ = audit(ctx, s.Store.Pool, num(a.User, "id"), "commerce.ticket_reply", fmt.Sprint(id), Data{"closed": flag(d, "close")})
	}
	return out, 200, e
}
func (s *Server) ticketNotifications(ctx context.Context, cfg Data, token string) error {
	rows, e := commerceRows(ctx, s.Store.Pool, `SELECT jsonb_build_object('id',m.id,'ticket_id',t.id,'owner_id',t.owner_id,'user_id',t.user_id,'author',m.author,'_body',m.body_ciphertext) FROM vr_ticket_messages m JOIN vr_support_tickets t ON t.id=m.ticket_id WHERE m.notified_at IS NULL ORDER BY m.id LIMIT 10`)
	if e != nil {
		return e
	}
	for _, d := range rows {
		plain, e := unseal(s.Config.FieldKey, str(d, "_body"))
		if e != nil {
			return e
		}
		targets := []string{}
		if str(d, "author") == "staff" {
			targets = append(targets, fmt.Sprint(num(d, "user_id")))
		} else {
			for id := range telegramOwners(cfg) {
				targets = append(targets, id)
			}
		}
		for _, id := range targets {
			callback := fmt.Sprintf("vs:ticket:%d", num(d, "ticket_id"))
			localCfg := cfg
			if str(d, "author") == "customer" {
				callback = "vr:/tickets"
			} else {
				store, err := getStore(ctx, s.Store.Pool, num(d, "owner_id"))
				if err != nil {
					return err
				}
				localCfg = salesLocale(cfg, store)
			}
			_, e = s.telegramRequest(ctx, token, "sendMessage", Data{"chat_id": id, "text": fmt.Sprintf("Ticket #%d\n%s", num(d, "ticket_id"), plain), "protect_content": true, "reply_markup": botKeyboard([]any{botButton(botText(localCfg, "View ticket", "مشاهده تیکت"), callback)})})
			if e != nil {
				return e
			}
		}
		_, e = s.Store.Pool.Exec(ctx, `UPDATE vr_ticket_messages SET notified_at=now() WHERE id=$1`, num(d, "id"))
		if e != nil {
			return e
		}
	}
	return nil
}
func salesJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
