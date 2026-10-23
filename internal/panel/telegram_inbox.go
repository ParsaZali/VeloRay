package panel

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Polling and checkout acknowledgement have a separate queue from slow admin
// operations and message delivery, which may involve unavailable nodes.
func (s *Server) telegramPoll(ctx context.Context, cfg Data) error {
	conn, e := s.Store.Pool.Acquire(ctx)
	if e != nil {
		return e
	}
	defer conn.Release()
	var locked bool
	if e = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(842100,31)`).Scan(&locked); e != nil {
		return e
	}
	if !locked {
		return nil
	}
	defer conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock(842100,31)`)
	token, e := unseal(s.Config.FieldKey, str(cfg, "_telegram_token"))
	if e != nil || token == "" {
		return errors.New("bot token is missing")
	}
	updates, e := s.telegramRequest(ctx, token, "getUpdates", Data{"offset": num(cfg, "_telegram_last_update_id") + 1, "limit": 50, "timeout": 0, "allowed_updates": []any{"message", "callback_query", "pre_checkout_query"}})
	if e != nil {
		return e
	}
	tx, e := conn.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var activeToken string
	if e = tx.QueryRow(ctx, `SELECT COALESCE(data->>'_telegram_token','') FROM vr_settings WHERE id=1 FOR UPDATE`).Scan(&activeToken); e != nil {
		return e
	}
	if activeToken != str(cfg, "_telegram_token") {
		return nil
	}
	for _, raw := range array(updates, "result") {
		u := asData(raw)
		id := num(u, "update_id")
		kind := "message"
		if id < 0 {
			return errors.New("invalid Telegram update")
		}
		if len(obj(u, "pre_checkout_query")) > 0 {
			kind = "checkout"
		} else if len(obj(obj(u, "message"), "successful_payment")) > 0 {
			kind = "receipt"
		}
		body, _ := json.Marshal(u)
		sealed, e := seal(s.Config.FieldKey, string(body))
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO vr_telegram_inbox(bot_id,id,kind,input_ciphertext) VALUES($1,$2,$3,$4) ON CONFLICT(bot_id,id) DO NOTHING`, telegramBotID(token), id, kind, sealed); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE vr_settings SET data=jsonb_set(data,'{_telegram_last_update_id}',to_jsonb(GREATEST(COALESCE((data->>'_telegram_last_update_id')::bigint,0),$1::bigint))) WHERE id=1`, id); e != nil {
			return e
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return e
	}
	for _, kind := range []string{"checkout", "receipt"} {
		items, e := s.telegramInbox(ctx, kind, telegramBotID(token))
		if e != nil {
			return e
		}
		for _, u := range items {
			// A pre-checkout query is unusable after Telegram's short deadline.
			if kind == "checkout" && flag(u, "_expired_checkout") {
				e = nil
			} else {
				_, e = s.telegramPaymentUpdate(ctx, cfg, token, u)
			}
			if e != nil {
				return e
			}
			if _, e = conn.Exec(ctx, `UPDATE vr_telegram_inbox SET processed_at=now() WHERE id=$1 AND bot_id=$2`, num(u, "update_id"), telegramBotID(token)); e != nil {
				return e
			}
		}
	}
	return nil
}
func (s *Server) telegramInbox(ctx context.Context, kind, botID string) ([]Data, error) {
	rows, e := s.Store.Pool.Query(ctx, `SELECT input_ciphertext,received_at FROM vr_telegram_inbox WHERE kind=$1 AND bot_id=$2 AND processed_at IS NULL ORDER BY id LIMIT 50`, kind, botID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Data{}
	for rows.Next() {
		var sealed string
		var received time.Time
		if e = rows.Scan(&sealed, &received); e != nil {
			return nil, e
		}
		plain, e := unseal(s.Config.FieldKey, sealed)
		if e != nil {
			return nil, e
		}
		u, e := decode([]byte(plain))
		if e != nil {
			return nil, e
		}
		u["_expired_checkout"] = time.Since(received) > 8*time.Second
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Server) telegramWorker(ctx context.Context, poll bool) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		cfg, e := settings(ctx, s.Store.Pool)
		if e == nil && flag(cfg, "telegram_enabled") {
			if poll {
				e = s.telegramPoll(ctx, cfg)
			} else {
				e = s.telegramReplies(ctx, cfg)
			}
			if e != nil && ctx.Err() == nil {
				s.Logger.Warn("Telegram processing pending", "poll", poll)
				_, _ = s.Store.Pool.Exec(ctx, `UPDATE vr_settings SET data=jsonb_set(data,'{_telegram_error}','"Telegram polling or delivery pending; check bot status"') WHERE id=1`)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
