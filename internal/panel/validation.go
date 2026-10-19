package panel

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var subscriptionTokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,96}$`)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func oneOf(s string, values ...string) bool {
	for _, v := range values {
		if s == v {
			return true
		}
	}
	return false
}
func typed(input, template Data) error {
	for k, v := range input {
		if oneOf(k, "expires_at", "next_renewal_at") {
			if v != nil {
				if _, ok := v.(string); !ok {
					return fmt.Errorf("%s must be a timestamp or null", k)
				}
			}
		}
		if oneOf(k, "password", "agent_token", "telegram_bot_token", "webhook_signing_secret") {
			if _, ok := v.(string); !ok {
				return fmt.Errorf("%s must be text", k)
			}
		}
		if oneOf(k, "telegram_clear_bot_token", "webhook_clear_secret") {
			if _, ok := v.(bool); !ok {
				return fmt.Errorf("%s must be boolean", k)
			}
		}
	}

	for k, v := range input {
		base, ok := template[k]
		if !ok {
			continue
		}
		switch base.(type) {
		case bool:
			if _, ok := v.(bool); !ok {
				return fmt.Errorf("%s must be boolean", k)
			}
		case string:
			if _, ok := v.(string); !ok {
				return fmt.Errorf("%s must be text", k)
			}
		case float64:
			if _, ok := v.(json.Number); !ok {
				return fmt.Errorf("%s must be a number", k)
			}
		case int, int64:
			if _, ok := v.(json.Number); !ok {
				return fmt.Errorf("%s must be an integer", k)
			}
			if _, e := v.(json.Number).Int64(); e != nil {
				return fmt.Errorf("%s must be an integer", k)
			}
		case Data, map[string]any:
			if _, ok := v.(map[string]any); !ok {
				return fmt.Errorf("%s must be an object", k)
			}
		case []any:
			if _, ok := v.([]any); !ok {
				return fmt.Errorf("%s must be an array", k)
			}
		}
	}
	return nil
}
func validHost(host string) bool {
	if net.ParseIP(host) != nil {
		return true
	}
	if len(host) == 0 || len(host) > 253 || strings.ContainsAny(host, "/\\?#@:\n\r\t ") {
		return false
	}
	for _, part := range strings.Split(host, ".") {
		if len(part) == 0 || len(part) > 63 || !regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]*[a-zA-Z0-9])?$`).MatchString(part) {
			return false
		}
	}
	return true
}
func validKey(s string) bool {
	b, e := base64.StdEncoding.DecodeString(s)
	if e != nil {
		b, e = base64.RawURLEncoding.DecodeString(s)
	}
	return e == nil && len(b) == 32
}
func validateNode(c Config, d Data) error {
	if e := require(d, "name", 96); e != nil {
		return e
	}
	if !validHost(str(d, "public_host")) {
		return errors.New("public_host must be a hostname or IP address")
	}
	u, e := url.Parse(str(d, "agent_url"))
	if e != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.Trim(u.Path, "/") != "" {
		return errors.New("agent_url must be an HTTP(S) origin")
	}
	ip := net.ParseIP(u.Hostname())
	loop := u.Hostname() == "localhost" || ip != nil && ip.IsLoopback()
	if u.Scheme != "https" && !(u.Scheme == "http" && loop) {
		return errors.New("remote agents require HTTPS")
	}
	if str(d, "_agent_token") == "" {
		return errors.New("agent_token is required")
	}
	if flag(d, "is_local") && !loop {
		return errors.New("local node must use a loopback agent")
	}
	return nil
}
func validateInbound(ctx context.Context, q Query, c Config, d Data) error {
	stream := obj(d, "stream_settings")
	if err := allowed(stream, "rawSettings tcpSettings wsSettings grpcSettings xhttpSettings httpupgradeSettings kcpSettings hysteriaSettings tlsSettings realitySettings sockopt"); err != nil {
		return err
	}
	if raw, _ := json.Marshal(stream); len(raw) > 65536 {
		return errors.New("advanced stream settings exceed 64 KiB")
	}
	if minTLS := str(obj(stream, "tlsSettings"), "minVersion"); minTLS != "" && !oneOf(minTLS, "1.2", "1.3") {
		return errors.New("TLS minimum version must be 1.2 or 1.3")
	}
	for _, key := range []string{"path", "host_header", "service_name", "tls_server_name", "flow"} {
		if strings.ContainsAny(str(d, key), "\r\n") {
			return fmt.Errorf("invalid %s", key)
		}
	}

	if e := require(d, "name", 96); e != nil {
		return e
	}
	node, e := get(ctx, q, "nodes", num(d, "node"))
	if e != nil {
		return errors.New("node does not exist")
	}
	p, t, s := str(d, "protocol"), str(d, "transport"), str(d, "security")
	core := fallback(str(d, "core"), "xray")
	d["core"] = core
	if !oneOf(core, "xray", "singbox") {
		return errors.New("invalid core")
	}
	if core == "singbox" && (!oneOf(p, "vless", "vmess", "trojan", "shadowsocks", "hysteria", "http", "socks", "tuic", "naive", "snell") || !oneOf(t, "raw", "ws", "grpc", "httpupgrade", "hysteria")) {
		return errors.New("unsupported sing-box protocol or transport")
	}
	if !oneOf(p, "vless", "vmess", "trojan", "shadowsocks", "hysteria", "wireguard", "http", "socks", "tunnel", "tun", "tuic", "naive", "snell") || !oneOf(t, "raw", "ws", "grpc", "xhttp", "httpupgrade", "mkcp", "hysteria") || !oneOf(s, "none", "tls", "reality") {
		return errors.New("unsupported protocol, transport or security")
	}
	if core == "xray" && oneOf(p, "tuic", "naive", "snell") {
		return errors.New("protocol requires sing-box")
	}
	if e := validateNativeProtocol(d); e != nil {
		return e
	}
	port := num(d, "port")
	if p == "tun" {
		d["port"] = 0
	} else if port < 1 || port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	if port == 10085 || port == 10086 || port == 10087 || port == 9191 || flag(node, "is_local") && (port == 8610 || port == int64(c.PanelPort)) {
		return errors.New("port is reserved by the panel, agent or Xray API")
	}
	if net.ParseIP(str(d, "listen")) == nil {
		return errors.New("listen must be an IP address")
	}
	if oneOf(p, "wireguard", "shadowsocks", "tunnel", "tun") && (t != "raw" || s != "none") {
		return errors.New("this protocol requires RAW without stream security")
	}
	if p == "socks" && t != "raw" {
		return errors.New("SOCKS requires RAW")
	}
	if p == "hysteria" && (t != "hysteria" || s != "tls") {
		return errors.New("Hysteria2 requires Hysteria transport and TLS")
	}
	if t == "hysteria" && p != "hysteria" {
		return errors.New("Hysteria transport requires Hysteria2")
	}
	if !oneOf(p, "vless", "vmess", "trojan", "http", "hysteria") && t != "raw" {
		return errors.New("unsupported transport for this protocol")
	}
	if s == "reality" && (p != "vless" || !oneOf(t, "raw", "grpc", "xhttp")) {
		return errors.New("REALITY requires VLESS with RAW, gRPC or XHTTP")
	}
	if f := str(d, "flow"); f != "" && (f != "xtls-rprx-vision" || p != "vless" || t != "raw" || s != "reality") {
		return errors.New("Vision requires VLESS / RAW / REALITY")
	}
	if s == "tls" {
		for _, k := range []string{"tls_cert_file", "tls_key_file"} {
			if !filepath.IsAbs(str(d, k)) || strings.ContainsAny(str(d, k), "\r\n") {
				return fmt.Errorf("%s must be an absolute file path", k)
			}
		}
	}
	if s == "reality" {
		if !validKey(str(d, "reality_private_key")) || !validKey(str(d, "reality_public_key")) || !validHost(str(d, "reality_server_name")) {
			return errors.New("REALITY requires valid private/public keys and server name")
		}
		if _, _, e = net.SplitHostPort(str(d, "reality_dest")); e != nil {
			return errors.New("REALITY target requires host:port")
		}
		sid := str(d, "reality_short_id")
		if len(sid) > 16 || len(sid)%2 != 0 || !regexp.MustCompile(`^[a-fA-F0-9]*$`).MatchString(sid) {
			return errors.New("REALITY short ID must be even-length hex, at most 16 characters")
		}
	}
	ps := obj(d, "protocol_settings")
	if p == "wireguard" && (!validKey(str(ps, "secret_key")) || !validKey(str(ps, "public_key"))) {
		return errors.New("WireGuard requires a server keypair")
	}
	if p == "shadowsocks" && !oneOf(fallback(str(ps, "method"), "aes-256-gcm"), "aes-128-gcm", "aes-256-gcm", "chacha20-ietf-poly1305") {
		return errors.New("unsupported Shadowsocks method")
	}
	if p == "tunnel" && !flag(ps, "follow_redirect") && (num(ps, "rewrite_port") < 1 || num(ps, "rewrite_port") > 65535) {
		return errors.New("tunnel requires a target port or follow_redirect")
	}
	return nil
}
func validateClient(ctx context.Context, q Query, d Data) error {
	if n := num(d, "hwid_limit"); n < -1 || n > 1000 {
		return errors.New("HWID limit must be -1–1000")
	}
	if num(d, "device_limit") < 0 || num(d, "device_limit") > 10000 {
		return errors.New("device limit must be 0–10000")
	}
	if flag(d, "start_after_first_use") {
		if num(d, "duration_days") < 1 || num(d, "duration_days") > 36500 {
			return errors.New("duration must be 1–36500 days")
		}
		if str(d, "first_used_at") == "" {
			d["expires_at"] = nil
		}
	}
	if _, err := multiplierMilli(d); err != nil {
		return err
	}
	if e := require(d, "name", 96); e != nil {
		return e
	}
	i, e := get(ctx, q, "inbounds", num(d, "inbound"))
	if e != nil {
		return errors.New("inbound does not exist")
	}
	p := str(i, "protocol")
	if num(d, "device_limit") > 0 && !oneOf(p, "vless", "vmess", "trojan", "hysteria", "tuic", "naive", "snell") {
		return errors.New("device limits require a supported metered protocol")
	}
	if !supportsClients(p) {
		return errors.New("this inbound has no client accounts")
	}
	if num(d, "traffic_limit_bytes") < 0 || num(d, "renewal_interval_days") < 0 || num(d, "renewal_interval_days") > 3650 {
		return errors.New("invalid quota or renewal interval")
	}
	_, renewEvery, _, renewErr := renewalSpec(d)
	if renewErr != nil {
		return renewErr
	}
	if !metered(p) && (num(d, "traffic_limit_bytes") > 0 || renewEvery > 0) {
		return errors.New("per-client traffic quotas are unavailable for this protocol")
	}
	for _, k := range []string{"expires_at", "next_renewal_at"} {
		if d[k] != nil && str(d, k) != "" {
			if _, e := timestamp(d, k); e != nil {
				return fmt.Errorf("%s must be an RFC3339 timestamp", k)
			}
		} else {
			d[k] = nil
		}
	}
	credential := str(d, "credential")
	if credential == "" {
		if oneOf(p, "vless", "vmess", "tuic") {
			d["credential"] = uuid()
		} else {
			d["credential"] = randomToken(24)
		}
	}
	if oneOf(p, "vless", "vmess", "tuic") && !uuidPattern.MatchString(str(d, "credential")) {
		return errors.New("credential must be a UUID")
	}
	if len(str(d, "credential")) > 255 || strings.ContainsAny(str(d, "credential"), "\r\n") {
		return errors.New("invalid credential")
	}
	if str(d, "subscription_token") == "" {
		d["subscription_token"] = randomToken(32)
	}
	if !subscriptionTokenPattern.MatchString(str(d, "subscription_token")) {
		return errors.New("subscription token must contain 16–96 URL-safe characters")
	}
	ps := obj(d, "protocol_settings")
	if p == "tuic" {
		if str(ps, "password") == "" {
			ps["password"] = randomToken(24)
		}
		if len(str(ps, "password")) > 255 || strings.ContainsAny(str(ps, "password"), "\r\n") {
			return errors.New("invalid TUIC password")
		}
		d["protocol_settings"] = ps
	}
	if p == "http" || p == "socks" {
		if str(ps, "username") == "" {
			ps["username"] = "vr-" + randomToken(8)
		}
		if strings.ContainsAny(str(ps, "username"), "\r\n") || len(str(ps, "username")) > 96 {
			return errors.New("invalid proxy username")
		}
		d["protocol_settings"] = ps
	}
	if p == "shadowsocks" {
		var n int
		_ = q.QueryRow(ctx, `SELECT count(*) FROM vr_clients WHERE inbound_id=$1 AND id<>$2`, num(d, "inbound"), num(d, "id")).Scan(&n)
		if n > 0 {
			return errors.New("Shadowsocks supports one client per inbound")
		}
	}
	if p == "wireguard" {
		if !validKey(str(ps, "public_key")) || !validKey(str(ps, "private_key")) {
			return errors.New("WireGuard client requires a keypair")
		}
		if str(ps, "address") == "" {
			if num(d, "id") == 0 {
				return errors.New("WireGuard address is required (unique CIDR)")
			}
		} else if _, _, e := net.ParseCIDR(str(ps, "address")); e != nil {
			return errors.New("invalid WireGuard address")
		}
		others, e := list(ctx, q, "clients", "WHERE inbound_id=$1 AND id<>$2", num(d, "inbound"), num(d, "id"))
		if e != nil {
			return e
		}
		for _, o := range others {
			op := obj(o, "protocol_settings")
			if str(op, "public_key") == str(ps, "public_key") || str(op, "address") == str(ps, "address") {
				return errors.New("WireGuard peer key/address already in use")
			}
		}
	}
	if e := scheduleRenewal(d, time.Now().UTC()); e != nil {
		return e
	}
	return nil
}
func fallback(s, d string) string {
	if s != "" {
		return s
	}
	return d
}
func supportsClients(p string) bool { return p != "tun" && p != "tunnel" }
func metered(p string) bool {
	return oneOf(p, "vless", "vmess", "trojan", "shadowsocks", "hysteria", "wireguard", "tuic", "naive", "snell")
}
func validateSettings(d Data) error {
	if err := validateSubscriptionHosts(array(d, "subscription_hosts")); err != nil {
		return err
	}
	if n := num(d, "subscription_hwid_limit"); n < 0 || n > 1000 {
		return errors.New("HWID limit must be 0–1000")
	}
	if len(str(d, "subscription_template_path")) > 4096 {
		return errors.New("template path is too long")
	}
	for _, key := range []string{"subscription_xray_template", "subscription_clash_template", "subscription_singbox_template"} {
		if err := validateFeedTemplate(str(d, key)); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
	}
	if e := validateWebhookURL(str(d, "webhook_url")); e != nil {
		return e
	}
	if flag(d, "webhook_enabled") && (str(d, "webhook_url") == "" || str(d, "_webhook_secret") == "") {
		return errors.New("enabled webhooks require an HTTPS URL and signing secret")
	}
	if e := require(d, "site_name", 64); e != nil {
		return e
	}
	for k, bounds := range map[string][2]int64{"dashboard_refresh_seconds": {5, 3600}, "traffic_history_days": {1, 365}, "default_client_days": {0, 3650}, "default_traffic_gb": {0, 1000000}, "default_renewal_days": {0, 3650}, "api_key_default_days": {1, 3650}, "audit_retention_days": {1, 3650}, "telegram_alert_cpu_percent": {1, 100}, "telegram_alert_memory_percent": {1, 100}, "telegram_alert_disk_percent": {1, 100}, "telegram_alert_cooldown_minutes": {1, 1440}, "telegram_quota_warning_percent": {1, 100}, "telegram_expiry_warning_days": {1, 30}, "subscription_update_interval_hours": {1, 168}} {
		if num(d, k) < bounds[0] || num(d, k) > bounds[1] {
			return fmt.Errorf("invalid %s", k)
		}
	}
	if !oneOf(str(d, "accent"), "emerald", "blue", "violet", "rose", "amber") || !oneOf(str(d, "default_locale"), "en", "fa") || !oneOf(str(d, "subscription_template"), "default", "compact", "dashboard", "custom") {
		return errors.New("unsupported accent, locale or subscription template")
	}
	if !oneOf(str(d, "subscription_default_format"), "auto", "base64", "raw", "clash", "singbox", "xray") || !oneOf(str(d, "subscription_theme"), "system", "light", "dark") {
		return errors.New("unsupported subscription format or theme")
	}
	for _, k := range []string{"support_url", "announcement_url"} {
		if str(d, k) != "" {
			u, e := url.Parse(str(d, k))
			if e != nil || !oneOf(u.Scheme, "http", "https") || u.Host == "" || u.User != nil {
				return fmt.Errorf("%s must be an HTTP(S) URL", k)
			}
		}
	}
	if len(str(d, "announcement")) > 1120 || len(str(d, "subscription_footer")) > 720 {
		return errors.New("announcement or footer is too long")
	}
	if !oneOf(str(d, "telegram_locale"), "en", "fa") {
		return errors.New("Telegram language must be en or fa")
	}
	ids := array(d, "telegram_owner_ids")
	if len(ids) > 20 {
		return errors.New("at most 20 Telegram owners")
	}
	for _, v := range ids {
		s, ok := v.(string)
		if !ok || !regexp.MustCompile(`^[1-9][0-9]{0,18}$`).MatchString(s) {
			return errors.New("Telegram owner IDs must be positive integer strings")
		}
	}
	return nil
}
