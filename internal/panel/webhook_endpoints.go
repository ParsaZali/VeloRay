package panel

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

var eventPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,94}(\.\*)?$`)

func matchesWebhookEvent(pattern, event string) bool {
	return pattern == "*" || pattern == event || strings.HasSuffix(pattern, ".*") && strings.HasPrefix(event, strings.TrimSuffix(pattern, "*"))
}
func validateEndpoint(d Data) error {
	if e := require(d, "name", 96); e != nil {
		return e
	}
	if !oneOf(str(d, "channel"), "webhook", "discord") {
		return fail(400, "choose webhook or discord")
	}
	if len(array(d, "events")) > 64 {
		return fail(400, "at most 64 event filters")
	}
	for _, raw := range array(d, "events") {
		p, ok := raw.(string)
		if !ok || p != "*" && !eventPattern.MatchString(p) {
			return fail(400, "invalid event filter")
		}
	}
	return nil
}
func (s *Server) webhookEndpointsAPI(w http.ResponseWriter, r *http.Request, path string, a Actor) (any, int, error) {
	ctx := r.Context()
	parts := strings.Split(path, "/")
	id := int64(0)
	if len(parts) > 4 {
		return nil, 0, fail(404, "endpoint not found")
	}
	if len(parts) > 2 {
		id, _ = strconv.ParseInt(parts[2], 10, 64)
		if id < 1 {
			return nil, 0, fail(404, "endpoint not found")
		}
	}
	view := func(d Data, detail bool) (Data, error) {
		v := public(d)
		v["secret_configured"] = str(d, "_secret") != ""
		if detail {
			u, e := unseal(s.Config.FieldKey, str(d, "_url"))
			if e != nil {
				return nil, e
			}
			v["url"] = u
		}
		return v, nil
	}
	if len(parts) == 4 {
		if r.Method != "POST" || !oneOf(parts[3], "test", "retry") {
			return nil, 0, fail(405, "method not allowed")
		}
		d, e := get(ctx, s.Store.Pool, "webhook_endpoints", id)
		if e != nil {
			return nil, 0, e
		}
		if parts[3] == "retry" {
			tag, e := s.Store.Pool.Exec(ctx, "UPDATE vr_webhook_deliveries SET attempts=0,next_at=now(),last_error='' WHERE endpoint_id=$1 AND delivered_at IS NULL AND attempts >= $2", id, webhookMaxAttempts)
			return Data{"retried": tag.RowsAffected()}, 200, e
		}
		if !flag(d, "enabled") {
			return nil, 0, fail(400, "enable this endpoint first")
		}
		payload := Data{"id": randomToken(12), "event": "webhook.test", "target": "endpoint", "actor": str(a.User, "username"), "occurred_at": stamp()}
		_, e = s.Store.Pool.Exec(ctx, "INSERT INTO vr_webhook_deliveries(endpoint_id,payload) VALUES($1,$2::jsonb)", id, string(mustJSON(payload)))
		return Data{"queued": e == nil}, 202, e
	}
	if r.Method == "GET" {
		if id > 0 {
			d, e := get(ctx, s.Store.Pool, "webhook_endpoints", id)
			if e != nil {
				return nil, 0, e
			}
			v, e := view(d, true)
			return v, 200, e
		}
		rows, e := list(ctx, s.Store.Pool, "webhook_endpoints", "ORDER BY id")
		if e != nil {
			return nil, 0, e
		}
		out := []Data{}
		for _, d := range rows {
			v, e := view(d, false)
			if e != nil {
				return nil, 0, e
			}
			out = append(out, v)
		}
		return out, 200, nil
	}
	if !(r.Method == "POST" && id == 0 || oneOf(r.Method, "PATCH", "DELETE") && id > 0) {
		return nil, 0, fail(405, "method not allowed")
	}
	input := Data{}
	var e error
	if r.Method != "DELETE" {
		input, e = readBody(w, r)
		if e != nil {
			return nil, 0, e
		}
		if e = allowed(input, "name url enabled channel events signing_secret"); e != nil {
			return nil, 0, fail(400, e.Error())
		}
		if e = typed(input, Data{"name": "", "url": "", "enabled": false, "channel": "webhook", "events": []any{}, "signing_secret": ""}); e != nil {
			return nil, 0, fail(400, e.Error())
		}
	}
	out, e := s.managementChange(ctx, nil, a, "webhook-endpoint."+strings.ToLower(r.Method), fmt.Sprint(id), func(tx pgx.Tx) (any, error) {
		d := Data{"name": "", "enabled": false, "channel": "webhook", "events": []any{}}
		if id > 0 {
			var e error
			d, e = get(ctx, tx, "webhook_endpoints", id)
			if e != nil {
				return nil, e
			}
		}
		if r.Method == "DELETE" {
			return nil, remove(ctx, tx, "webhook_endpoints", id)
		}
		previousChannel := str(d, "channel")
		d = merge(d, input)
		delete(d, "url")
		delete(d, "signing_secret")
		if e := validateEndpoint(d); e != nil {
			return nil, fail(400, e.Error())
		}
		target := ""
		if id > 0 {
			var e error
			target, e = unseal(s.Config.FieldKey, str(d, "_url"))
			if e != nil {
				return nil, e
			}
		}
		if _, ok := input["url"]; ok {
			target = str(input, "url")
		}
		if target == "" {
			return nil, fail(400, "endpoint URL is required")
		}
		if e := validateWebhookURL(target); e != nil {
			return nil, fail(400, e.Error())
		}
		if id == 0 {
			var count int
			if e := tx.QueryRow(ctx, "SELECT count(*) FROM vr_webhook_endpoints").Scan(&count); e != nil {
				return nil, e
			}
			if count >= 16 {
				return nil, fail(400, "at most 16 notification endpoints")
			}
		}
		if previousChannel != "webhook" && str(d, "channel") == "webhook" && str(input, "signing_secret") == "" {
			return nil, fail(400, "set a signing secret when switching to a generic webhook")
		}
		if secret := str(input, "signing_secret"); secret != "" {
			if len(secret) < 32 || len(secret) > 1024 {
				return nil, fail(400, "signing secret must contain 32–1024 bytes")
			}
			sealed, e := seal(s.Config.FieldKey, secret)
			if e != nil {
				return nil, e
			}
			d["_secret"] = sealed
		}
		if str(d, "_secret") == "" {
			if str(d, "channel") == "webhook" {
				return nil, fail(400, "a signing secret is required")
			}
			sealed, e := seal(s.Config.FieldKey, randomToken(32))
			if e != nil {
				return nil, e
			}
			d["_secret"] = sealed
		}
		sealed, e := seal(s.Config.FieldKey, target)
		if e != nil {
			return nil, e
		}
		d["_url"] = sealed
		if e = save(ctx, tx, "webhook_endpoints", d); e != nil {
			return nil, e
		}
		return view(d, true)
	})
	status := 200
	if r.Method == "POST" {
		status = 201
	}
	if r.Method == "DELETE" {
		status = 204
	}
	return out, status, e
}
func enqueueEndpointWebhooks(ctx context.Context, q Query, payload Data) error {
	rows, e := list(ctx, q, "webhook_endpoints", "WHERE data->>'enabled'='true' ORDER BY id LIMIT 16")
	if e != nil {
		return e
	}
	for _, d := range rows {
		match := len(array(d, "events")) == 0
		for _, p := range array(d, "events") {
			v, _ := p.(string)
			match = match || matchesWebhookEvent(v, str(payload, "event"))
		}
		if !match {
			continue
		}
		if _, e = q.Exec(ctx, "INSERT INTO vr_webhook_deliveries(endpoint_id,payload) VALUES($1,$2::jsonb)", num(d, "id"), string(mustJSON(payload))); e != nil {
			return e
		}
	}
	return nil
}
func endpointPayload(channel string, payload []byte) []byte {
	if channel != "discord" {
		return payload
	}
	d, _ := decode(payload)
	content := fmt.Sprintf("VeloRay · %s\nTarget: %s\nActor: %s\nAt: %s\nEvent: %v", str(d, "event"), str(d, "target"), str(d, "actor"), str(d, "occurred_at"), d["id"])
	if len(content) > 1800 {
		content = content[:1800]
	}
	return mustJSON(Data{"content": content, "allowed_mentions": Data{"parse": []any{}}})
}
