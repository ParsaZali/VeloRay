package panel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const Version = "1.0"

// Data is a stored entity. Private fields use a leading underscore and are never
// included in API responses. All mutations validate a per-entity allowlist.
type Data map[string]any

func decode(raw []byte) (Data, error) {
	var d Data
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	err := dec.Decode(&d)
	return d, err
}
func clone(d Data) Data { raw, _ := json.Marshal(d); out, _ := decode(raw); return out }
func public(d Data) Data {
	out := clone(d)
	for k := range out {
		if strings.HasPrefix(k, "_") {
			delete(out, k)
		}
	}
	return out
}
func str(d Data, k string) string { v, _ := d[k].(string); return v }
func num(d Data, k string) int64 {
	switch v := d[k].(type) {
	case json.Number:
		n, _ := v.Int64()
		return n
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	case string:
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	}
	return 0
}
func flag(d Data, k string) bool { v, _ := d[k].(bool); return v }
func obj(d Data, k string) Data {
	switch v := d[k].(type) {
	case Data:
		return v
	case map[string]any:
		return Data(v)
	}
	return Data{}
}
func array(d Data, k string) []any                  { v, _ := d[k].([]any); return v }
func timestamp(d Data, k string) (time.Time, error) { return time.Parse(time.RFC3339Nano, str(d, k)) }
func stamp(values ...time.Time) string {
	t := time.Now()
	if len(values) > 0 {
		t = values[0]
	}
	return t.UTC().Format(time.RFC3339Nano)
}
func active(c Data, now time.Time) bool {
	if !flag(c, "enabled") {
		return false
	}
	if t, e := timestamp(c, "expires_at"); e == nil && !t.After(now) {
		return false
	}
	return num(c, "traffic_limit_bytes") <= 0 || num(c, "used_traffic_bytes") < num(c, "traffic_limit_bytes")
}
func require(d Data, k string, max int) error {
	s := str(d, k)
	if strings.TrimSpace(s) == "" || len([]rune(s)) > max {
		return fmt.Errorf("%s must contain 1–%d characters", k, max)
	}
	return nil
}
func allowed(d Data, keys string) error {
	set := map[string]bool{}
	for _, k := range strings.Fields(keys) {
		set[k] = true
	}
	for k := range d {
		if !set[k] {
			return fmt.Errorf("field %q is not editable", k)
		}
	}
	return nil
}
func merge(a, b Data) Data {
	out := clone(a)
	for k, v := range b {
		out[k] = v
	}
	return out
}
func deepMerge(a, b Data) Data {
	out := clone(a)
	b = clone(b)
	for k, v := range b {
		if nested, ok := v.(map[string]any); ok {
			if old, ok := out[k].(map[string]any); ok {
				out[k] = deepMerge(Data(old), Data(nested))
				continue
			}
		}
		out[k] = v
	}
	return out
}
func defaults(kind string) Data {
	switch kind {
	case "nodes":
		return Data{"profile_id": 0, "name": "", "public_host": "", "agent_url": "", "enabled": true, "verify_tls": true, "is_local": false, "status": "unknown", "xray_version": "", "last_seen_at": nil, "config_patch": Data{}, "deploy_state": "pending", "last_deploy_error": ""}
	case "inbounds":
		return Data{"name": "", "node": 0, "listen": "0.0.0.0", "port": 0, "core": "xray", "protocol": "vless", "transport": "raw", "security": "none", "flow": "", "path": "/", "host_header": "", "service_name": "", "tls_server_name": "", "tls_cert_file": "", "tls_key_file": "", "reality_dest": "", "reality_server_name": "", "reality_private_key": "", "reality_public_key": "", "reality_short_id": "", "protocol_settings": Data{}, "stream_settings": Data{}, "enabled": true}
	case "clients":
		return Data{"renewal_period": "none", "renewal_every": 1, "renewal_timezone": "UTC", "hwid_limit": 0, "device_limit": 0, "start_after_first_use": false, "duration_days": 30, "first_used_at": nil, "owner_id": 0, "inbound": 0, "name": "", "credential": "", "protocol_settings": Data{}, "subscription_token": "", "enabled": true, "expires_at": nil, "traffic_limit_bytes": 0, "used_traffic_bytes": 0, "lifetime_traffic_bytes": 0, "raw_used_traffic_bytes": 0, "raw_lifetime_traffic_bytes": 0, "raw_upload_traffic_bytes": 0, "raw_download_traffic_bytes": 0, "traffic_multiplier": 1.0, "last_traffic_at": nil, "disabled_reason": "", "renewal_interval_days": 0, "next_renewal_at": nil, "note": ""}
	case "outbounds":
		return Data{"name": "", "node": 0, "core": "xray", "tag": "", "protocol": "freedom", "config": Data{}, "enabled": true}
	case "routing_rules":
		return Data{"name": "", "node": 0, "core": "xray", "priority": 100, "config": Data{}, "enabled": true}
	case "users":
		return Data{"ldap_dn": "", "oidc_issuer": "", "oidc_subject": "", "role": "", "permissions": []any{}, "allowed_inbounds": []any{}, "own_clients_only": false, "max_clients": 0, "traffic_budget_bytes": 0, "username": "", "email": "", "is_active": true, "is_staff": true, "is_superuser": false, "last_login": nil, "date_joined": stamp(time.Now()), "_password": "", "_totp_secret": "", "_totp_enabled": false, "_totp_step": int64(-1), "_recovery": []any{}, "_auth_version": 1}
	case "api_keys":
		return Data{"name": "", "prefix": "", "scope": "read", "enabled": true, "expires_at": nil, "last_used_at": nil, "user": 0}
	}
	return Data{}
}
func defaultSettings() Data {
	return Data{
		"site_name": "VeloRay", "support_url": "", "announcement": "", "announcement_url": "", "default_locale": "en", "default_client_days": 30, "default_traffic_gb": 0, "default_renewal_days": 0, "traffic_history_days": 30, "dashboard_refresh_seconds": 5, "subscription_enabled": true, "subscription_show_apps": true, "subscription_show_qr": true, "subscription_show_connection_uri": true, "subscription_footer": "", "subscription_template": "default", "subscription_path": "/sub/", "subscription_update_interval_hours": 12, "subscription_default_format": "auto", "subscription_show_analytics": true, "subscription_show_downloads": true, "subscription_theme": "system", "maintenance_mode": false, "require_2fa_for_admins": false, "api_keys_enabled": true, "api_key_default_days": 90, "audit_retention_days": 90, "accent": "emerald", "telegram_enabled": false, "telegram_locale": "en", "telegram_allow_changes": false, "telegram_client_alerts": true, "telegram_quota_warning_percent": 80, "telegram_expiry_warning_days": 3, "_telegram_token": "", "telegram_owner_ids": []any{}, "telegram_alert_cpu_percent": 90, "telegram_alert_memory_percent": 90, "telegram_alert_disk_percent": 90, "telegram_alert_cooldown_minutes": 30, "_telegram_last_alerts": Data{}, "_telegram_last_update_id": 0,
		"subscription_hosts": []any{}, "subscription_template_path": "", "subscription_hwid_required": false, "subscription_hwid_limit": 0,
		"subscription_xray_template": "", "subscription_clash_template": "", "subscription_singbox_template": "",
		"smtp_enabled": false, "smtp_host": "", "smtp_port": 465, "smtp_tls_mode": "tls", "smtp_username": "", "smtp_from": "", "_smtp_password": "",
		"alert_quota_percent": 0, "alert_expiry_days": 0, "alert_inactive_days": 0,
		"webhook_enabled": false, "webhook_url": "", "_webhook_secret": "",
	}
}
