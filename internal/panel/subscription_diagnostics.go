package panel

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const subscriptionProbeTarget = "https://www.gstatic.com/generate_204"

type subscriptionProbeOption struct {
	item     subscriptionConnection
	id, core string
	config   Data
	public   Data
}

// Only connection configuration contributes to the identifier. Traffic, online
// timestamps and quota changes must not invalidate profile links or samples.
func subscriptionConnectionKey(secret string, item subscriptionConnection) string {
	inbound := Data{}
	for _, field := range strings.Fields("id core protocol transport security port path host_header service_name tls_server_name reality_server_name reality_public_key reality_short_id protocol_settings stream_settings") {
		inbound[field] = item.inbound[field]
	}
	raw, _ := json.Marshal(Data{
		"client":  Data{"id": num(item.client, "id"), "credential": str(item.client, "credential"), "protocol_settings": obj(item.client, "protocol_settings")},
		"inbound": inbound,
		"node":    Data{"id": num(item.node, "id"), "host": str(item.node, "public_host")},
	})
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("veloray-subscription-connection-v1\x00"))
	_, _ = mac.Write(raw)
	return hex.EncodeToString(mac.Sum(nil))
}
func (s *Server) subscriptionProbeOptions(connections []subscriptionConnection) ([]subscriptionProbeOption, string) {
	options := make([]subscriptionProbeOption, 0, min(len(connections), 200))
	identities := []string{}
	for _, item := range connections[:min(200, len(connections))] {
		id := subscriptionConnectionKey(s.Config.SecretKey, item)
		core, config, err := connectionProbeConfig(item.client, item.inbound, item.node)
		protocol := str(item.inbound, "protocol")
		if num(item.client, "id") < 1 || num(item.node, "id") < 1 {
			err = errors.New("External subscriptions are checked in their own application")
		}
		if oneOf(protocol, "wireguard", "naive", "tun", "tunnel") || core == "xray" && protocol == "hysteria" {
			err = errors.New("This protocol requires a compatible client application")
		}
		public := Data{"id": id, "name": connectionName(item.client, item.inbound), "protocol": protocol, "host": str(item.node, "public_host"), "supported": err == nil}
		if err != nil {
			public["reason"] = "client_application_required"
		}
		options = append(options, subscriptionProbeOption{item: item, id: id, core: core, config: config, public: public})
		identities = append(identities, id)
	}
	sort.Strings(identities)
	return options, digest(strings.Join(identities, "\n"))
}
func (s *Server) diagnosticDevice(ctx context.Context, r *http.Request, c, cfg Data) error {
	_, required := hwidPolicy(c, cfg)
	hwid := deviceHeader(r.Header.Get("X-HWID"), 256)
	if hwid == "" {
		if required {
			return failCode(403, "hwid_required", "VPN checks require a registered application's HWID")
		}
		return nil
	}
	field, id := deviceOwner(c)
	var permitted bool
	err := s.Store.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vr_client_devices WHERE `+field+`=$1 AND hwid=$2 AND COALESCE(data->>'revoked','false')<>'true')`, id, hwid).Scan(&permitted)
	if err != nil {
		return err
	}
	if !permitted {
		return failCode(403, "device_required", "Download the subscription in an authorized app before running a VPN check")
	}
	return nil
}
func diagnosticSameOrigin(r *http.Request) bool {
	origin, err := url.Parse(r.Header.Get("Origin"))
	if err != nil || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" || origin.Path != "" || !oneOf(origin.Scheme, "http", "https") || origin.Host == "" {
		return false
	}
	normalize := func(host string) string {
		host = strings.ToLower(host)
		if origin.Scheme == "https" {
			return strings.TrimSuffix(host, ":443")
		}
		return strings.TrimSuffix(host, ":80")
	}
	return normalize(origin.Host) == normalize(r.Host) && r.Header.Get("X-VeloRay-Diagnostic") == "1"
}
func (s *Server) subscriptionDiagnostics(w http.ResponseWriter, r *http.Request, c, cfg Data, connections []subscriptionConnection) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method == http.MethodHead {
		w.WriteHeader(200)
		return
	}
	if !flag(cfg, "subscription_diagnostics_enabled") {
		writeJSON(w, 403, Data{"detail": "Subscriber connection checks are disabled", "code": "diagnostics_disabled"})
		return
	}
	options, revision := s.subscriptionProbeOptions(connections)
	if r.Method == http.MethodGet {
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			writeJSON(w, 403, Data{"detail": "Open checks from your subscription page"})
			return
		}
		if err := s.diagnosticDevice(r.Context(), r, c, cfg); err != nil {
			var detail *apiError
			if errors.As(err, &detail) {
				writeJSON(w, 200, Data{"enabled": true, "browser_only": true, "code": detail.Code, "connections": []any{}, "vantage": "node"})
				return
			}
			s.respond(w, r, nil, 0, err)
			return
		}
		if err := rateLimit(r.Context(), s.Store.Pool, "subscription-probe-ticket:"+accountKey(c), 20, time.Hour); err != nil {
			s.respond(w, r, nil, 0, err)
			return
		}
		ticket := randomToken(32)
		_, err := s.Store.Pool.Exec(r.Context(), `INSERT INTO vr_subscription_probe_tickets(ticket_hash,principal_key,client_id,subscription_hash,device_hash,revision) VALUES($1,$2,$3,$4,$5,$6)`,
			digest(ticket), accountKey(c), num(c, "id"), digest(str(c, "subscription_token")), digest(deviceHeader(r.Header.Get("X-HWID"), 256)), revision)
		if err != nil {
			s.respond(w, r, nil, 0, err)
			return
		}
		items := make([]Data, 0, len(options))
		for _, option := range options {
			items = append(items, option.public)
		}
		writeJSON(w, 200, Data{"enabled": true, "ticket": ticket, "expires_in_seconds": 120, "connections": items, "vantage": "node", "traffic_counts": true, "target_host": "www.gstatic.com", "total_connections": len(connections)})
		return
	}
	if r.Method != http.MethodPost || !diagnosticSameOrigin(r) || !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		writeJSON(w, 403, Data{"detail": "Run checks from your subscription page"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	body, err := readBody(w, r)
	if err != nil {
		s.respond(w, r, nil, 0, err)
		return
	}
	if err = allowed(body, "ticket connection"); err != nil {
		writeJSON(w, 400, Data{"detail": "Only a ticket and connection are accepted"})
		return
	}
	if err = typed(body, Data{"ticket": "", "connection": ""}); err != nil || len(str(body, "ticket")) < 16 || len(str(body, "ticket")) > 96 {
		writeJSON(w, 400, Data{"detail": "Invalid diagnostic ticket"})
		return
	}
	var selected *subscriptionProbeOption
	for index := range options {
		if options[index].id == str(body, "connection") {
			selected = &options[index]
			break
		}
	}
	if selected == nil || !flag(selected.public, "supported") {
		writeJSON(w, 400, Data{"detail": "Select a supported connection from this subscription"})
		return
	}
	if err = s.diagnosticDevice(r.Context(), r, c, cfg); err != nil {
		s.respond(w, r, nil, 0, err)
		return
	}
	if err = rateLimit(r.Context(), s.Store.Pool, "subscription-probe:"+accountKey(c), 10, time.Hour); err != nil {
		s.respond(w, r, nil, 0, err)
		return
	}
	if err = rateLimit(r.Context(), s.Store.Pool, "subscription-probe-ip:"+clientIP(r), 60, time.Hour); err != nil {
		s.respond(w, r, nil, 0, err)
		return
	}
	var expected string
	err = s.Store.Pool.QueryRow(r.Context(), `DELETE FROM vr_subscription_probe_tickets WHERE ticket_hash=$1 AND principal_key=$2 AND subscription_hash=$3 AND device_hash=$4 AND expires_at>now() RETURNING revision`,
		digest(str(body, "ticket")), accountKey(c), digest(str(c, "subscription_token")), digest(deviceHeader(r.Header.Get("X-HWID"), 256))).Scan(&expected)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, 409, Data{"detail": "Diagnostic ticket expired or was already used; refresh the check"})
		return
	}
	if err != nil {
		s.respond(w, r, nil, 0, err)
		return
	}
	if expected != revision {
		writeJSON(w, 409, Data{"detail": "Connection settings changed; refresh the check"})
		return
	}
	node, err := get(r.Context(), s.Store.Pool, "nodes", num(selected.item.node, "id"))
	if err != nil {
		s.respond(w, r, nil, 0, err)
		return
	}
	result, probeErr := s.nodeCall(r.Context(), node, "/clients/probe", Data{"core": selected.core, "config": selected.config, "url": subscriptionProbeTarget})
	out := Data{"connection": selected.id, "ok": false, "network_tested": false, "vantage": "node", "traffic_counts": true, "checked_at": stamp(), "metric": "https"}
	if selected.core == "native" && str(selected.item.inbound, "protocol") == "mtproto" {
		out["metric"] = "handshake"
	}
	if probeErr == nil {
		out["ok"], out["network_tested"] = flag(result, "ok"), flag(result, "network_tested")
		if latency := num(result, "latency_ms"); latency >= 0 && latency <= 60000 {
			out["latency_ms"] = latency
		}
		if code := num(result, "status_code"); code >= 100 && code <= 599 {
			out["status_code"] = code
		}
	}
	if probeErr != nil {
		out["error"] = "node_probe_unavailable"
	} else if !flag(out, "ok") {
		out["error"] = "connection_check_failed"
	}
	if recordErr := s.recordConnectionQuality(r.Context(), c, *selected, out); recordErr != nil {
		out["history_recorded"] = false
		if s.Logger != nil {
			s.Logger.Warn("subscription probe history was not saved")
		}
	} else {
		out["history_recorded"] = true
	}
	_ = audit(r.Context(), s.Store.Pool, 0, "subscription.connection-test", accountKey(c), Data{"connection": selected.id, "ok": flag(out, "ok")})
	writeJSON(w, 200, out)
}
func (s *Server) recordConnectionQuality(ctx context.Context, c Data, option subscriptionProbeOption, result Data) error {
	tx, err := s.Store.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	principal := accountKey(c)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "connection-quality:"+principal+":"+option.id); err != nil {
		return err
	}
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT data FROM vr_connection_quality WHERE principal_key=$1 AND connection_key=$2`, principal, option.id).Scan(&raw)
	data := Data{}
	if err == nil {
		data, err = decode(raw)
	} else if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return err
	}
	samples := append(array(data, "samples"), Data{"ok": flag(result, "ok"), "network_tested": flag(result, "network_tested"), "latency_ms": num(result, "latency_ms"), "metric": str(result, "metric"), "checked_at": result["checked_at"]})
	data["samples"] = samples[max(0, len(samples)-9):]
	raw, err = json.Marshal(data)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO vr_connection_quality(principal_key,connection_key,client_id,node_id,data) VALUES($1,$2,$3,$4,$5) ON CONFLICT(principal_key,connection_key) DO UPDATE SET data=excluded.data,updated_at=now()`,
		principal, option.id, num(option.item.client, "id"), num(option.item.node, "id"), string(raw)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
