package panel

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Webhook events contain audit metadata only. Credentials, client configuration,
// subscription URLs and the potentially sensitive audit detail are never sent.
const webhookMaxAttempts = 8

func validateWebhookURL(raw string) error {
	if raw == "" {
		return nil
	}
	if len(raw) > 2048 {
		return errors.New("webhook URL is too long")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return errors.New("webhook URL must be an HTTPS URL without credentials or a fragment")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if !validHost(host) || host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return errors.New("webhook URL must use a public hostname")
	}
	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return errors.New("invalid webhook port")
		}
	}
	if ip := net.ParseIP(host); ip != nil && !publicWebhookIP(ip) {
		return errors.New("webhook URL cannot target a private or reserved IP")
	}
	return nil
}

func publicWebhookIP(ip net.IP) bool {
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	// CGNAT and benchmarking ranges are not valid outbound webhook targets.
	for _, cidr := range []string{"100.64.0.0/10", "198.18.0.0/15"} {
		_, block, _ := net.ParseCIDR(cidr)
		if block.Contains(ip) {
			return false
		}
	}
	return true
}

// Resolve and connect to the checked address directly. Disabling environment
// proxies and redirects prevents bypassing the private-network restriction.
func webhookHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 3 * time.Second, KeepAlive: -1}
	transport := &http.Transport{
		Proxy:             nil,
		TLSClientConfig:   &tls.Config{MinVersion: tls.VersionTLS12},
		DisableKeepAlives: true,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			for _, candidate := range ips {
				if publicWebhookIP(candidate.IP) {
					return dialer.DialContext(ctx, network, net.JoinHostPort(candidate.IP.String(), port))
				}
			}
			return nil, errors.New("webhook destination has no public IP address")
		},
	}
	return &http.Client{
		Timeout:   5 * time.Second,
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func webhookSignature(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = io.WriteString(mac, timestamp+".")
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func webhookBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > webhookMaxAttempts {
		attempt = webhookMaxAttempts
	}
	seconds := 15 << (attempt - 1)
	if seconds > 3600 {
		seconds = 3600
	}
	return time.Duration(seconds) * time.Second
}

// enqueueAuditWebhook runs in the same transaction as the audit log, ensuring
// that rolled-back mutations never emit a webhook event.
func enqueueAuditWebhook(ctx context.Context, q Query, auditRecord Data) error {
	payload := Data{
		"id":          auditRecord["id"],
		"event":       auditRecord["action"],
		"target":      auditRecord["target"],
		"actor":       auditRecord["actor_name"],
		"occurred_at": auditRecord["created_at"],
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `INSERT INTO vr_webhook_deliveries(payload)
		SELECT $1::jsonb FROM vr_settings
		WHERE id=1 AND data->>'webhook_enabled'='true'
		  AND COALESCE(data->>'webhook_url','') <> ''
		  AND COALESCE(data->>'_webhook_secret','') <> ''`, string(body))
	if err != nil {
		return err
	}
	return enqueueEndpointWebhooks(ctx, q, payload)
}

func (s *Server) webhooksAPI(w http.ResponseWriter, r *http.Request, path string, actor Actor) (any, int, error) {
	if path == "webhooks/endpoints" || strings.HasPrefix(path, "webhooks/endpoints/") {
		return s.webhookEndpointsAPI(w, r, path, actor)
	}
	ctx := r.Context()
	switch {
	case path == "webhooks/status" && r.Method == http.MethodGet:
		var pending, failed, delivered int64
		err := s.Store.Pool.QueryRow(ctx, `SELECT
			count(*) FILTER (WHERE delivered_at IS NULL AND attempts < $1),
			count(*) FILTER (WHERE delivered_at IS NULL AND attempts >= $1),
			count(*) FILTER (WHERE delivered_at IS NOT NULL)
			FROM vr_webhook_deliveries`, webhookMaxAttempts).Scan(&pending, &failed, &delivered)
		return Data{"pending": pending, "failed": failed, "delivered": delivered}, 200, err
	case path == "webhooks/test" && r.Method == http.MethodPost:
		d, err := settings(ctx, s.Store.Pool)
		if err != nil {
			return nil, 0, err
		}
		if !flag(d, "webhook_enabled") || str(d, "_webhook_secret") == "" {
			return nil, 0, fail(400, "Configure and enable a signed webhook first")
		}
		err = audit(ctx, s.Store.Pool, num(actor.User, "id"), "webhook.test", "webhook", Data{})
		return Data{"queued": err == nil}, 202, err
	case path == "webhooks/retry" && r.Method == http.MethodPost:
		tag, err := s.Store.Pool.Exec(ctx, `UPDATE vr_webhook_deliveries SET attempts=0, next_at=now(), last_error=''
			WHERE delivered_at IS NULL AND attempts >= $1`, webhookMaxAttempts)
		return Data{"retried": tag.RowsAffected()}, 200, err
	default:
		return nil, 0, fail(405, "method not allowed")
	}
}

func (s *Server) deliverWebhookQueue(ctx context.Context) error {
	d, err := settings(ctx, s.Store.Pool)
	if err != nil {
		return err
	}
	target, secret := "", ""
	legacy := flag(d, "webhook_enabled")
	if legacy {
		target = str(d, "webhook_url")
		if err = validateWebhookURL(target); err != nil {
			return err
		}
		secret, err = unseal(s.Config.FieldKey, str(d, "_webhook_secret"))
		if err != nil {
			return err
		}
		legacy = secret != "" && target != ""
	}
	rows, err := s.Store.Pool.Query(ctx, `WITH ready AS (
		SELECT id FROM vr_webhook_deliveries
		WHERE delivered_at IS NULL AND attempts < $1 AND next_at <= now()
 AND ((endpoint_id IS NULL AND $2) OR endpoint_id IN (SELECT id FROM vr_webhook_endpoints WHERE data->>'enabled'='true' AND ($3 OR data->>'channel'<>'email')))
		ORDER BY id LIMIT 8 FOR UPDATE SKIP LOCKED
	)
	UPDATE vr_webhook_deliveries AS d
	SET attempts=d.attempts+1, next_at=now()+interval '2 minutes'
	FROM ready WHERE d.id=ready.id
	RETURNING d.id, d.payload, d.attempts,COALESCE(d.endpoint_id,0)`, webhookMaxAttempts, legacy, flag(d, "smtp_enabled"))
	if err != nil {
		return err
	}
	type delivery struct {
		id       int64
		payload  []byte
		attempts int
		endpoint int64
	}
	var ready []delivery
	for rows.Next() {
		var item delivery
		if err = rows.Scan(&item.id, &item.payload, &item.attempts, &item.endpoint); err != nil {
			break
		}
		ready = append(ready, item)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	client := s.WebhookHTTP
	if client == nil {
		client = webhookHTTPClient()
	}
	for _, item := range ready {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		deliveryTarget, deliverySecret, channel := target, secret, "webhook"
		if item.endpoint > 0 {
			endpoint, e := get(ctx, s.Store.Pool, "webhook_endpoints", item.endpoint)
			if errors.Is(e, pgx.ErrNoRows) {
				continue
			}
			if e != nil {
				return e
			}
			if !flag(endpoint, "enabled") {
				_, e = s.Store.Pool.Exec(ctx, "UPDATE vr_webhook_deliveries SET attempts=GREATEST(attempts-1,0),next_at=now()+interval '1 minute' WHERE id=$1", item.id)
				if e != nil {
					return e
				}
				continue
			}
			deliveryTarget, e = unseal(s.Config.FieldKey, str(endpoint, "_url"))
			if e != nil {
				return e
			}
			deliverySecret, e = unseal(s.Config.FieldKey, str(endpoint, "_secret"))
			if e != nil {
				return e
			}
			channel = str(endpoint, "channel")
		}
		if channel == "email" {
			err = s.deliverEmail(ctx, deliveryTarget, item.payload, strconv.FormatInt(item.id, 10))
		} else {
			if e := validateWebhookURL(deliveryTarget); e != nil {
				return e
			}
			item.payload = endpointPayload(channel, item.payload)
			timestamp := strconv.FormatInt(time.Now().Unix(), 10)
			req, requestErr := http.NewRequestWithContext(ctx, http.MethodPost, deliveryTarget, bytes.NewReader(item.payload))
			err = requestErr
			if err == nil {
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("User-Agent", "VeloRay-Webhooks/1.0")
				req.Header.Set("X-VeloRay-Timestamp", timestamp)
				req.Header.Set("X-VeloRay-Delivery", strconv.FormatInt(item.id, 10))
				req.Header.Set("X-VeloRay-Signature", webhookSignature(deliverySecret, timestamp, item.payload))
				var response *http.Response
				response, err = client.Do(req)
				if err == nil {
					_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
					_ = response.Body.Close()
					if response.StatusCode < 200 || response.StatusCode >= 300 {
						err = fmt.Errorf("remote status %d", response.StatusCode)
					}
				}
			}
		}
		if err == nil {
			_, err = s.Store.Pool.Exec(ctx, `UPDATE vr_webhook_deliveries
				SET delivered_at=now(), last_error='' WHERE id=$1`, item.id)
		} else {
			s.Logger.Warn("webhook delivery failed", "delivery_id", item.id, "attempts", item.attempts, "error", "request failed")
			message := "request failed"
			if responseStatus := func() string {
				if strings.HasPrefix(err.Error(), "remote status ") {
					return err.Error()
				}
				return ""
			}(); responseStatus != "" {
				message = responseStatus
			}
			if len(message) > 300 {
				message = message[:300]
			}
			_, err = s.Store.Pool.Exec(ctx, `UPDATE vr_webhook_deliveries
				SET next_at=now()+$2::interval, last_error=$3 WHERE id=$1`,
				item.id, webhookBackoff(item.attempts).String(), message)
		}
		if err != nil {
			return err
		}
	}
	_, err = s.Store.Pool.Exec(ctx, `DELETE FROM vr_webhook_deliveries WHERE delivered_at < now()-interval '7 days'`)
	return err
}
