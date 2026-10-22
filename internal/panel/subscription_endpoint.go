package panel

import (
	"bytes"
	"context"
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
)

// A requested port never changes customer links until the listener is applied.
func subscriptionOrigin(c Config, cfg Data) string {
	u, err := url.Parse(c.PublicURL)
	port := num(obj(cfg, "_subscription_endpoint"), "applied_port")
	if err != nil || u.Host == "" || port < 1 || port > 65535 {
		return strings.TrimRight(c.PublicURL, "/")
	}
	u.Host = net.JoinHostPort(u.Hostname(), strconv.FormatInt(port, 10))
	return u.String()
}
func (s *Server) subscriptionURL(ctx context.Context, q Query, token string) (string, error) {
	cfg, err := settings(ctx, q)
	if err != nil {
		return "", err
	}
	return subscriptionOrigin(s.Config, cfg) + subscriptionBasePath(cfg) + token, nil
}
func validateSubscriptionPort(c Config, port int64) error {
	if port < 1 || port > 65535 {
		return errors.New("subscription port must be between 1 and 65535")
	}
	if port == int64(c.PanelPort) || oneOf(strconv.FormatInt(port, 10), "80", "5432", "8610", "8611", "8612", "9191", "10085", "10086", "10087") {
		return errors.New("subscription port is reserved by the panel or another VeloRay service")
	}
	return nil
}
func subscriptionPortAvailable(ctx context.Context, q Query, port int64) error {
	var used bool
	if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vr_inbounds i JOIN vr_nodes n ON n.id=i.node_id WHERE n.data->>'is_local'='true' AND (i.data->>'port')::integer=$1)`, port).Scan(&used); err != nil {
		return err
	}
	if used {
		return errors.New("subscription port is already reserved by a local inbound")
	}
	return nil
}
func (s *Server) endpointView(cfg Data) Data {
	e := obj(cfg, "_subscription_endpoint")
	return Data{"managed": oneOf(s.Config.SubscriptionDriver, "native", "caddy"), "driver": fallback(s.Config.SubscriptionDriver, "manual"), "panel_origin": s.Config.PublicURL, "subscription_origin": subscriptionOrigin(s.Config, cfg), "port": num(e, "port"), "applied_port": num(e, "applied_port"), "revision": str(e, "revision"), "status": fallback(str(e, "status"), "unconfigured"), "error": str(e, "error"), "applied_at": str(e, "applied_at")}
}
func (s *Server) subscriptionEndpointAPI(w http.ResponseWriter, r *http.Request, a Actor) (any, int, error) {
	if !flag(a.User, "is_superuser") || a.Scope != "admin" {
		return nil, 0, fail(403, "administrator access required")
	}
	ctx := r.Context()
	if r.Method == "GET" {
		cfg, err := settings(ctx, s.Store.Pool)
		return s.endpointView(cfg), 200, err
	}
	if r.Method != "PUT" {
		return nil, 0, fail(405, "method not allowed")
	}
	if !oneOf(s.Config.SubscriptionDriver, "native", "caddy") {
		return nil, 0, fail(409, "managed subscription endpoint is not installed on this deployment")
	}
	in, err := readBody(w, r)
	if err != nil {
		return nil, 0, err
	}
	if err = allowed(in, "port revision"); err != nil {
		return nil, 0, fail(400, err.Error())
	}
	if err = typed(in, Data{"port": 0, "revision": ""}); err != nil {
		return nil, 0, fail(400, err.Error())
	}
	if _, ok := in["revision"]; !ok {
		return nil, 0, fail(400, "revision is required")
	}
	port := num(in, "port")
	if err = validateSubscriptionPort(s.Config, port); err != nil {
		return nil, 0, fail(400, err.Error())
	}
	tx, err := s.Store.Pool.Begin(ctx)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT id FROM vr_settings WHERE id=1 FOR UPDATE`); err != nil {
		return nil, 0, err
	}
	cfg, err := settings(ctx, tx)
	if err != nil {
		return nil, 0, err
	}
	e := obj(cfg, "_subscription_endpoint")
	if str(in, "revision") != str(e, "revision") {
		return nil, 0, fail(409, "subscription endpoint changed; refresh before applying")
	}
	if err = subscriptionPortAvailable(ctx, tx, port); err != nil {
		return nil, 0, fail(400, err.Error())
	}
	e["port"], e["revision"], e["status"], e["error"] = port, randomToken(16), "pending", ""
	cfg["_subscription_endpoint"] = e
	if err = saveSettings(ctx, tx, cfg); err != nil {
		return nil, 0, err
	}
	if err = audit(ctx, tx, num(a.User, "id"), "subscription.endpoint.request", "subscriptions", Data{"port": port}); err != nil {
		return nil, 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, 0, err
	}
	return s.endpointView(cfg), 202, nil
}

type endpointApply func(context.Context, int, int) error

// Use the same management/node lock order as inbound changes.
func (s *Server) syncSubscriptionEndpoint(ctx context.Context, force bool, apply endpointApply) error {
	tx, err := s.Store.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(842100,2)`); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `SELECT id FROM vr_settings WHERE id=1 FOR UPDATE`); err != nil {
		return err
	}
	cfg, err := settings(ctx, tx)
	if err != nil {
		return err
	}
	e := obj(cfg, "_subscription_endpoint")
	initial := num(e, "port") == 0
	if !initial && !force && str(e, "status") != "pending" {
		return nil
	}
	nodes, err := list(ctx, tx, "nodes", "WHERE data->>'is_local'='true' ORDER BY id")
	if err != nil {
		return err
	}
	ids := []int64{}
	for _, n := range nodes {
		ids = append(ids, num(n, "id"))
	}
	if err = lockNodes(ctx, tx, ids...); err != nil {
		return err
	}
	port := num(e, "port")
	if initial {
		port = int64(s.Config.SubscriptionPort)
		if port == 0 {
			port = 8444
		}
		if s.Config.SubscriptionDriver == "native" && port == 8444 {
			for attempts := 0; attempts < 100; attempts++ {
				if validateSubscriptionPort(s.Config, port) == nil && subscriptionPortAvailable(ctx, tx, port) == nil {
					listener, listenErr := net.Listen("tcp", net.JoinHostPort("0.0.0.0", strconv.FormatInt(port, 10)))
					if listenErr == nil {
						listener.Close()
						break
					}
				}
				port++
			}
		}
		e["port"], e["revision"] = port, randomToken(16)
	}
	previousStatus, previousError := str(e, "status"), str(e, "error")
	oldPort := int(num(e, "applied_port"))
	applyErr := validateSubscriptionPort(s.Config, port)
	if applyErr == nil {
		applyErr = subscriptionPortAvailable(ctx, tx, port)
	}
	if applyErr == nil {
		applyErr = apply(ctx, int(port), oldPort)
	}
	committed := false
	if applyErr == nil {
		defer func() {
			if !committed {
				rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
				defer cancel()
				_ = apply(rollbackCtx, oldPort, int(port))
			}
		}()
		e["applied_port"], e["applied_at"], e["status"], e["error"] = port, stamp(), "ready", ""
	} else {
		e["status"], e["error"] = "error", "The subscription listener could not be applied. The previous endpoint remains active. Check that the port is free and the subscription service is running."
		var unavailable *endpointUnavailable
		if errors.As(applyErr, &unavailable) {
			e["status"] = "pending"
		}
		// A missing proxy can be retried without filling the audit table every tick.
		if !initial && str(e, "status") == "pending" && previousStatus == "pending" && previousError == str(e, "error") {
			return applyErr
		}
	}
	cfg["_subscription_endpoint"] = e
	if err = saveSettings(ctx, tx, cfg); err != nil {
		return err
	}
	if err = audit(ctx, tx, 0, "subscription.endpoint."+str(e, "status"), "subscriptions", Data{"port": port}); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	committed = true
	return applyErr
}

type endpointUnavailable struct{ error }

func (s *Server) caddySubscriptionApply(ctx context.Context, port, previous int) error {
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", s.Config.SubscriptionControl)
	}}
	defer transport.CloseIdleConnections()
	raw, _ := json.Marshal(Data{"port": port})
	req, _ := http.NewRequestWithContext(ctx, "PUT", "http://subscription-control/port", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	res, err := (&http.Client{Transport: transport, Timeout: 8 * time.Second}).Do(req)
	if err != nil {
		return &endpointUnavailable{err}
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
	if res.StatusCode == 503 {
		return &endpointUnavailable{errors.New("subscription proxy is not ready")}
	}
	if res.StatusCode != 200 {
		return fmt.Errorf("subscription proxy rejected port %d", port)
	}
	return nil
}
func (s *Server) subscriptionEndpointWorker(ctx context.Context) {
	if s.Config.SubscriptionDriver != "caddy" {
		return
	}
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		applyCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		_ = s.syncSubscriptionEndpoint(applyCtx, false, s.caddySubscriptionApply)
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
