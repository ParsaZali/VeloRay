package panel

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	qrcode "github.com/skip2/go-qrcode"
	"html/template"
	"net/http"
	"strings"
	"time"
)

func clashConfig(c, i, n Data) (Data, error) {
	p, t := str(i, "protocol"), str(i, "transport")
	if !oneOf(p, "vless", "vmess", "trojan", "shadowsocks", "hysteria", "tuic", "shadowtls") {
		return nil, fmt.Errorf("Clash output is unavailable for %s", p)
	}
	if !oneOf(t, "raw", "ws", "grpc", "xhttp", "hysteria") {
		return nil, fmt.Errorf("Clash output is unavailable for %s transport", t)
	}
	name := connectionName(c, i)
	v := Data{"name": name, "server": str(n, "public_host"), "port": num(i, "port"), "udp": true}
	switch p {
	case "vless", "vmess":
		v["type"] = p
		v["uuid"] = str(c, "credential")
		if p == "vmess" {
			v["alterId"] = 0
			v["cipher"] = "auto"
		}
		if flow := clientFlow(c, i); p == "vless" && flow != "" {
			v["flow"] = flow
		}
	case "trojan":
		v["type"] = p
		v["password"] = str(c, "credential")
	case "shadowsocks":
		v["type"] = "ss"
		v["cipher"] = fallback(str(obj(i, "protocol_settings"), "method"), "aes-256-gcm")
		v["password"] = str(c, "credential")
	case "shadowtls":
		v["type"], v["cipher"], v["udp"] = "ss", shadowTLSMethod, false
		v["password"] = str(obj(i, "protocol_settings"), "psk") + ":" + str(c, "credential")
		v["plugin"] = "shadow-tls"
		v["client-fingerprint"] = "chrome"
		v["plugin-opts"] = Data{"host": str(obj(i, "protocol_settings"), "handshake_server"), "password": str(obj(c, "protocol_settings"), "shadowtls_password"), "version": 3}
	case "tuic":
		v["type"] = p
		v["uuid"] = str(c, "credential")
		v["password"] = str(obj(c, "protocol_settings"), "password")
		v["congestion-controller"] = fallback(str(obj(i, "protocol_settings"), "congestion_control"), "bbr")
		v["udp-relay-mode"] = "native"
	case "hysteria":
		v["type"] = "hysteria2"
		v["password"] = str(c, "credential")
		v["sni"] = fallback(str(i, "tls_server_name"), str(n, "public_host"))
	}
	switch t {
	case "ws":
		v["network"] = "ws"
		v["ws-opts"] = Data{"path": fallback(str(i, "path"), "/"), "headers": Data{"Host": str(i, "host_header")}}
	case "grpc":
		v["network"] = "grpc"
		v["grpc-opts"] = Data{"grpc-service-name": fallback(str(i, "service_name"), str(i, "name"))}
	case "xhttp":
		v["network"] = "xhttp"
		v["xhttp-opts"] = Data{"path": fallback(str(i, "path"), "/"), "mode": "auto"}
	case "raw":
		v["network"] = "tcp"
	}
	if str(i, "security") == "tls" {
		v["tls"] = true
		v["servername"] = fallback(str(i, "tls_server_name"), str(n, "public_host"))
	}
	if str(i, "security") == "reality" {
		v["tls"] = true
		v["servername"] = str(i, "reality_server_name")
		v["client-fingerprint"] = "chrome"
		v["reality-opts"] = Data{"public-key": str(i, "reality_public_key"), "short-id": str(i, "reality_short_id")}
	}
	return Data{"proxies": []any{v}, "proxy-groups": []any{Data{"name": "VeloRay", "type": "select", "proxies": []any{name}}}, "rules": []any{"MATCH,VeloRay"}}, nil
}
func (s *Server) subscription(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		writeJSON(w, 405, Data{"detail": "method not allowed"})
		return
	}
	token := strings.TrimPrefix(r.URL.Path, "/sub/")
	assetPath := ""
	if before, after, found := strings.Cut(token, "/assets/"); found {
		token, assetPath = before, after
	}
	if len(token) < 16 || len(token) > 96 || strings.Contains(token, "/") {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()
	cfg, e := settings(ctx, s.Store.Pool)
	if e != nil {
		s.respond(w, r, nil, 0, e)
		return
	}
	if !flag(cfg, "subscription_enabled") || flag(cfg, "maintenance_mode") {
		writeJSON(w, 503, Data{"detail": "Subscriptions are temporarily unavailable"})
		return
	}
	rows, e := list(ctx, s.Store.Pool, "clients", "WHERE data->>'subscription_token'=$1 OR account_id IN (SELECT id FROM vr_accounts WHERE data->>'subscription_token'=$1) ORDER BY id LIMIT 1", token)
	if e != nil {
		s.respond(w, r, nil, 0, e)
		return
	}
	if len(rows) != 1 {
		http.NotFound(w, r)
		return
	}
	c := rows[0]
	if assetPath == "" && r.Method == "GET" && !oneOf(r.URL.Query().Get("format"), "stats", "info", "qr") {
		history := &subscriptionHistoryWriter{ResponseWriter: w}
		w = history
		defer s.recordSubscriptionRequest(r, c, history)
	}
	members := rows
	if account := num(c, "_account"); account > 0 {
		members, e = list(ctx, s.Store.Pool, "clients", "WHERE account_id=$1 ORDER BY id", account)
		if e != nil {
			s.respond(w, r, nil, 0, e)
			return
		}
	}
	connections := []subscriptionConnection{}
	onlineKnown, onlineConnected := true, false
	for _, member := range members {
		inbound, err := get(ctx, s.Store.Pool, "inbounds", num(member, "inbound"))
		if err != nil {
			s.respond(w, r, nil, 0, err)
			return
		}
		node, err := get(ctx, s.Store.Pool, "nodes", num(inbound, "node"))
		if err != nil {
			s.respond(w, r, nil, 0, err)
			return
		}
		if active(member, time.Now()) && flag(inbound, "enabled") && flag(node, "enabled") {
			observed, telemetryErr := timestamp(node, "_telemetry_at")
			if telemetryErr != nil || time.Since(observed) < 0 || time.Since(observed) >= 20*time.Second || !flag(node, "_online_available") || !oneOf(str(inbound, "protocol"), "vless", "vmess", "trojan", "hysteria", "tuic", "naive", "snell", "shadowtls") {
				onlineKnown = false
			} else if flag(obj(node, "_online_clients"), fmt.Sprint(num(member, "id"))) {
				onlineConnected = true
			}
			mapped := false
			for _, value := range array(cfg, "subscription_hosts") {
				host := asData(value)
				if num(host, "inbound") != num(inbound, "id") {
					continue
				}
				mapped = true
				if flag(host, "enabled") {
					exportInbound, exportNode := deliveryHost(inbound, node, host)
					connections = append(connections, subscriptionConnection{member, exportInbound, exportNode})
				}
			}
			if !mapped {
				connections = append(connections, subscriptionConnection{member, inbound, node})
			}
		}
	}
	if len(connections) == 0 {
		writeJSON(w, 403, Data{"detail": "This subscription is inactive, expired or out of traffic"})
		return
	}
	if assetPath != "" {
		s.portalAsset(w, r, cfg, assetPath)
		return
	}
	external, exportCfg, err := s.externalSubscription(ctx, c, cfg, r)
	if err != nil {
		s.respond(w, r, nil, 0, err)
		return
	}
	cfg = exportCfg
	connections = append(connections, external...)
	i, n := connections[0].inbound, connections[0].node
	c = clone(connections[0].client)
	c["subscription_token"] = token
	c["_subscription_online"] = nil
	if onlineConnected {
		c["_subscription_online"] = true
	} else if onlineKnown {
		c["_subscription_online"] = false
	}
	exp := int64(0)
	if t, e := timestamp(c, "expires_at"); e == nil {
		exp = t.Unix()
	}
	w.Header().Set("Subscription-Userinfo", fmt.Sprintf("upload=0; download=%d; total=%d; expire=%d", num(c, "used_traffic_bytes"), num(c, "traffic_limit_bytes"), exp))
	w.Header().Set("Profile-Title", "base64:"+base64.StdEncoding.EncodeToString([]byte(str(cfg, "site_name")+" · "+str(c, "name"))))
	w.Header().Set("Profile-Update-Interval", fmt.Sprint(num(cfg, "subscription_update_interval_hours")))
	w.Header().Set("Profile-Web-Page-URL", subscriptionOrigin(s.Config, cfg)+"/sub/"+token+"?format=portal")
	for key, field := range map[string]string{"Support-URL": "support_url", "Announce-URL": "announcement_url"} {
		if str(cfg, field) != "" {
			w.Header().Set(key, str(cfg, field))
		}
	}
	if str(cfg, "announcement") != "" {
		w.Header().Set("Announce", "base64:"+base64.StdEncoding.EncodeToString([]byte(str(cfg, "announcement"))))
	}
	w.Header().Set("Vary", "Accept, User-Agent, X-HWID")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	format := r.URL.Query().Get("format")
	if format == "" {
		if strings.Contains(r.Header.Get("Accept"), "text/html") {
			format = "portal"
		} else if ua := strings.ToLower(r.UserAgent()); strings.Contains(ua, "clash") || strings.Contains(ua, "mihomo") || strings.Contains(ua, "stash") {
			format = "clash"
		} else if strings.Contains(ua, "sing-box") || strings.Contains(ua, "singbox") {
			format = "singbox"
		} else {
			format = str(cfg, "subscription_default_format")
			if format == "" || format == "auto" {
				format = "base64"
			}
		}
	}
	link := shareLink(c, i, n)
	links := []string{}
	details := []Data{}
	for _, item := range connections {
		client := clone(item.client)
		uri := shareLink(client, item.inbound, item.node)
		if uri != "" {
			links = append(links, uri)
		}
		details = append(details, Data{"name": str(item.inbound, "name"), "protocol": str(item.inbound, "protocol"), "link": uri, "node": str(item.node, "name"), "host": str(item.node, "public_host")})
	}
	link = strings.Join(links, "\n")
	profile := profile(c, i, n)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	var body string
	switch format {
	case "base64", "b64":
		if link == "" {
			writeJSON(w, 400, Data{"detail": "Use the WireGuard download or account details for this protocol"})
			return
		}
		body = base64.StdEncoding.EncodeToString([]byte(link + "\n"))
	case "raw", "text":
		body = link + "\n"
		if link == "" {
			writeJSON(w, 400, Data{"detail": "No portable URI is available for this protocol"})
			return
		}
	case "singbox", "sing-box":
		outbounds := []any{}
		names := []any{}
		for _, item := range connections {
			graph, err := singClientOutbounds(item.client, item.inbound, item.node)
			if err != nil {
				writeJSON(w, 400, Data{"detail": err.Error()})
				return
			}
			outbounds = append(outbounds, graph...)
			names = append(names, str(graph[0].(Data), "tag"))
		}
		all := []any{Data{"type": "selector", "tag": "VeloRay", "outbounds": names}, Data{"type": "direct", "tag": "direct"}}
		all = append(all, outbounds...)
		raw, _ := json.MarshalIndent(Data{"outbounds": all, "route": Data{"final": "VeloRay"}}, "", "  ")
		if output, err := applyFeedTemplate(str(cfg, "subscription_singbox_template"), string(raw), "singbox"); err != nil {
			writeJSON(w, 500, Data{"detail": "Subscription template is invalid"})
			return
		} else {
			body = output
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	case "xray":
		profiles := []any{}
		for _, item := range connections {
			generated, err := xrayClientConfig(item.client, item.inbound, item.node)
			if err != nil {
				writeJSON(w, 400, Data{"detail": err.Error()})
				return
			}
			b, _ := json.Marshal(generated)
			output, err := applyFeedTemplate(str(cfg, "subscription_xray_template"), string(b), "xray")
			if err != nil {
				writeJSON(w, 500, Data{"detail": "Subscription template is invalid"})
				return
			}
			parsed, err := decode([]byte(output))
			if err != nil {
				s.respond(w, r, nil, 0, err)
				return
			}
			profiles = append(profiles, parsed)
		}
		b, _ := json.MarshalIndent(profiles, "", "  ")
		body = string(b) + "\n"
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	case "info":
		if r.Method == "HEAD" {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			return
		}
		writeJSON(w, 200, legacySubscriptionInfo(c, cfg))
		return
	case "qr":
		s.subscriptionQR(w, r, c, cfg, links)
		return
	case "stats":
		s.subscriptionStats(w, r, c)
		return
	case "json":
		if err := s.admitSubscriptionDevice(w, r, c, cfg); err != nil {
			s.respond(w, r, nil, 0, err)
			return
		}
		if r.Method == "HEAD" {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			return
		}
		writeJSON(w, 200, Data{"name": str(c, "name"), "protocol": str(i, "protocol"), "link": link, "profile": profile, "used_traffic_bytes": num(c, "used_traffic_bytes"), "traffic_limit_bytes": num(c, "traffic_limit_bytes"), "expires_at": c["expires_at"], "raw_used_traffic_bytes": num(c, "raw_used_traffic_bytes"), "traffic_multiplier": c["traffic_multiplier"], "connections": details})
		return
	case "clash", "yaml", "stash", "mihomo":
		proxies := []any{}
		names := []any{}
		for _, item := range connections {
			client := clone(item.client)
			data, err := clashConfig(client, item.inbound, item.node)
			if err != nil {
				writeJSON(w, 400, Data{"detail": err.Error()})
				return
			}
			for _, proxy := range array(data, "proxies") {
				proxies = append(proxies, proxy)
				names = append(names, str(asData(proxy), "name"))
			}
		}
		data := Data{"proxies": proxies, "proxy-groups": []any{Data{"name": "VeloRay", "type": "select", "proxies": names}}, "rules": []any{"MATCH,VeloRay"}}
		b, _ := json.MarshalIndent(data, "", "  ")
		if output, err := applyFeedTemplate(str(cfg, "subscription_clash_template"), string(b), "clash"); err != nil {
			writeJSON(w, 500, Data{"detail": "Subscription template is invalid"})
			return
		} else {
			body = output
		}
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	case "wireguard", "wg":
		if profile == "" {
			writeJSON(w, 400, Data{"detail": "No WireGuard profile is available"})
			return
		}
		body = profile
		w.Header().Set("Content-Disposition", `attachment; filename="veloray.conf"`)
	case "portal", "html":
		portalCfg := clone(cfg)
		formats := Data{"singbox": true, "xray": true, "clash": true}
		for _, item := range connections {
			if _, err := singClientOutbound(item.client, item.inbound, item.node); err != nil {
				formats["singbox"] = false
			}
			if _, err := xrayClientConfig(item.client, item.inbound, item.node); err != nil {
				formats["xray"] = false
			}
			if _, err := clashConfig(item.client, item.inbound, item.node); err != nil {
				formats["clash"] = false
			}
		}
		portalCfg["_feed_formats"] = formats
		s.portal(w, r, c, i, n, portalCfg, link, profile, details...)
		return
	default:
		writeJSON(w, 400, Data{"detail": "Unknown subscription format"})
		return
	}
	if err := s.admitSubscriptionDevice(w, r, c, cfg); err != nil {
		s.respond(w, r, nil, 0, err)
		return
	}
	if r.URL.Query().Get("download") == "1" {
		filenames := map[string]string{"base64": "veloray-subscription.txt", "b64": "veloray-subscription.txt", "raw": "veloray-links.txt", "text": "veloray-links.txt", "clash": "veloray-clash.yaml", "yaml": "veloray-clash.yaml", "singbox": "veloray-singbox.json", "sing-box": "veloray-singbox.json", "xray": "veloray-xray.json"}
		if filename := filenames[format]; filename != "" {
			w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
		}
	}
	if r.Method != "HEAD" {
		_, _ = w.Write([]byte(body))
	}
}
func fmtBytes(value int64) string {
	v := float64(max(0, value))
	unit := "B"
	for _, u := range []string{"B", "KB", "MB", "GB", "TB", "PB"} {
		unit = u
		if v < 1024 {
			break
		}
		v /= 1024
	}
	return fmt.Sprintf("%.1f %s", v, unit)
}

var portalTemplate = template.Must(template.New("portal").Parse(`<!doctype html>
<html lang="{{.Locale}}" dir="{{.Direction}}">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="robots" content="noindex,nofollow"><title>{{.Site}} · {{.Name}}</title><script src="/subscription-portal.js" defer></script>
<style>
:root{color-scheme:dark;font-family:system-ui,sans-serif;--bg:#000;--card:#0b0b0b;--surface:#141414;--border:#292929;--fg:#f0f0f0;--muted:#9b9b9b;--accent:#72ddb0;--accent-fg:#10271c;background:var(--bg);color:var(--fg)}*{box-sizing:border-box}body{margin:0;min-height:100dvh;display:grid;place-items:center;padding:20px 16px}main{width:100%;max-width:680px;min-width:0}header{display:flex;align-items:center;justify-content:space-between;gap:12px;margin-bottom:16px}.identity{min-width:0}.site{font-size:12px;color:var(--muted)}h1{font-size:22px;line-height:1.3;margin:5px 0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.host{font-size:11px;color:var(--muted);margin:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.badge{flex-shrink:0;border:1px solid color-mix(in srgb,var(--accent) 30%,var(--border));background:color-mix(in srgb,var(--accent) 8%,var(--card));color:var(--accent);padding:5px 10px;border-radius:999px;font-size:11px}section{border:1px solid var(--border);border-radius:14px;background:var(--card);padding:18px;margin-top:12px}.usage{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr);gap:18px}.usage small{display:block;color:var(--muted);font-size:11px;margin-bottom:6px}.usage strong{font-size:17px;overflow-wrap:anywhere}.quota{font-size:12px;color:var(--muted);font-weight:400}.factor{font-size:11px;color:var(--muted);margin-inline-start:6px}.quota-track{height:4px;background:var(--surface);border-radius:4px;overflow:hidden;margin-top:14px}.quota-track svg{display:block;width:100%;height:4px;color:var(--accent)}h2{font-size:13px;margin:0 0 12px}.connection-layout{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:18px;align-items:center}.connection-tools{min-width:0}.field-label{display:block;color:var(--muted);font-size:11px;margin-bottom:5px}input,textarea{width:100%;border:1px solid var(--border);border-radius:8px;background:var(--surface);color:var(--fg);padding:10px;direction:ltr;font:11px ui-monospace,monospace}input{height:36px}textarea{resize:vertical;min-height:74px;max-height:180px;margin-top:8px;line-height:1.6}.copy-row{display:flex;align-items:stretch;gap:8px;min-width:0}.copy-row input{flex:1;min-width:0}.copy-button{flex-shrink:0;border:1px solid var(--border);border-radius:8px;background:var(--surface);color:var(--fg);font:600 12px system-ui,sans-serif;padding:0 12px;cursor:pointer;min-height:36px}.copy-button:hover{border-color:var(--accent);color:var(--accent)}.copy-button:focus-visible{outline:2px solid var(--accent);outline-offset:2px}.copy-feedback{position:absolute;width:1px;height:1px;padding:0;margin:-1px;overflow:hidden;clip:rect(0,0,0,0);white-space:nowrap;border:0}.links{display:flex;flex-wrap:wrap;gap:6px;margin-top:10px}a{color:var(--accent);text-decoration:none}.links a{display:inline-flex;align-items:center;justify-content:center;border:1px solid var(--border);border-radius:8px;padding:7px 10px;font-size:11px;color:var(--fg)}.links .primary{background:var(--accent);color:var(--accent-fg);border-color:var(--accent)}.qr{margin:0;padding:8px;border-radius:10px;background:white;line-height:0}.qr img{width:128px;height:128px;max-width:100%}details{margin-top:14px;border-top:1px solid var(--border);padding-top:10px;font-size:12px}summary{display:flex;align-items:center;justify-content:space-between;gap:8px;cursor:pointer;color:var(--muted);list-style:none}summary::-webkit-details-marker{display:none}summary::after{content:"";width:6px;height:6px;border-right:1px solid currentColor;border-bottom:1px solid currentColor;transform:rotate(45deg);margin-inline-end:3px;transition:transform .15s}details[open] summary::after{transform:rotate(225deg)}.apps{font-size:11px;color:var(--muted);line-height:1.8;margin:8px 0 0}.announcement{font-size:12px;line-height:1.6;max-height:90px;overflow:auto;overflow-wrap:anywhere}pre{font:11px ui-monospace,monospace;white-space:pre-wrap;overflow-wrap:anywhere;line-height:1.7}footer{display:flex;justify-content:space-between;flex-wrap:wrap;gap:8px;color:var(--muted);font-size:11px;margin-top:14px;overflow-wrap:anywhere}:focus-visible{outline:2px solid var(--muted);outline-offset:3px}*{scrollbar-width:thin;scrollbar-color:var(--border) transparent}.compact main{max-width:540px}@media(prefers-color-scheme:light){:root{color-scheme:light;--bg:#f3f6f8;--card:#fff;--surface:#f5f7f8;--border:#dce2e7;--fg:#17191b;--muted:#66727e;--accent:#167a52;--accent-fg:#fff}}@media(max-width:420px){body{padding:16px 12px}section{padding:14px}.copy-row{gap:6px}.copy-button{padding:0 8px;font-size:11px}.connection-layout{gap:12px}.qr img{width:96px;height:96px}h1{font-size:20px}.usage strong{font-size:15px}.links a{padding:7px 8px}.usage{gap:12px}}@media(max-width:420px){.compact .connection-tools{display:contents}.compact .connection-layout{grid-template-columns:minmax(0,1fr) auto;gap:8px 12px;align-items:start}.compact .field-label{grid-column:1;grid-row:1;align-self:end;margin:0}.compact .copy-row{grid-column:1;grid-row:2;align-self:start}.compact .links{grid-column:1/-1;grid-row:3;margin-top:0}.compact .qr{grid-column:2;grid-row:1/3}.compact .qr img{width:80px;height:80px}}@media(prefers-reduced-motion:reduce){summary::after{transition:none}}
</style></head>
<body class="{{if .Compact}}compact{{end}}"><main>
<header><div class="identity"><span class="site">{{.Site}}</span><h1 title="{{.Name}}">{{.Name}}</h1><p class="host">{{.Protocol}} · {{.Host}}</p></div><span class="badge">{{.Active}}</span></header>
{{if .Announcement}}<section class="announcement">{{.Announcement}}{{if .AnnouncementURL}} <a href="{{.AnnouncementURL}}" aria-label="{{.Announcement}}">↗</a>{{end}}</section>{{end}}
<section><div class="usage"><div><small>{{.UsageLabel}}</small><strong>{{.Used}} <span class="quota">/ {{.Quota}}</span></strong><span class="factor">{{.Factor}}x</span></div><div><small>{{.ExpiryLabel}}</small><strong>{{.Expiry}}</strong></div></div>{{if .Limited}}<div class="quota-track" role="progressbar" aria-label="{{.UsageLabel}}" aria-valuemin="0" aria-valuemax="100" aria-valuenow="{{.Percent}}"><svg viewBox="0 0 100 4" preserveAspectRatio="none" aria-hidden="true"><rect width="{{.Percent}}" height="4" fill="currentColor"/></svg></div>{{end}}</section>
<section><h2>{{.Connect}}</h2><div class="connection-layout"><div class="connection-tools"><label class="field-label" for="subscription">{{.URLLabel}}</label><div class="copy-row"><input id="subscription" readonly value="{{.URL}}" aria-label="{{.URLLabel}}"><button id="copy-subscription" type="button" class="copy-button" data-success="{{.CopyDoneLabel}}" data-error="{{.CopyFailedLabel}}">{{.CopyLabel}}</button></div><span id="copy-feedback" class="copy-feedback" role="status" aria-live="polite"></span><div class="links">{{if .Link}}<a class="primary" href="{{.URL}}?format=base64">Base64</a>{{end}}{{if .Clash}}<a href="{{.URL}}?format=clash">Clash / Mihomo</a>{{end}}{{if .SingBox}}<a href="{{.URL}}?format=singbox">sing-box</a>{{end}}{{if .Xray}}<a href="{{.URL}}?format=xray">Xray JSON</a>{{end}}{{if .Profile}}<a class="primary" href="{{.URL}}?format=wireguard">WireGuard</a>{{end}}<a href="{{.URL}}?format=json">JSON</a></div></div>{{if .ShowQR}}<figure class="qr"><img width="128" height="128" src="{{.QR}}" alt="{{.QRLabel}}"></figure>{{end}}</div>
{{if .ShowURI}}{{if .Link}}<details><summary>{{.URILabel}}</summary><textarea readonly rows="3" aria-label="{{.URILabel}}">{{.Link}}</textarea></details>{{end}}{{end}}
{{if .Proxy}}<details><summary>{{.ProxyLabel}}</summary><pre dir="ltr">{{.Proxy}}</pre></details>{{end}}
{{if .ShowApps}}<details><summary>{{.AppsLabel}}</summary><p class="apps" dir="ltr">V2Box · v2rayNG · Hiddify · Streisand · Mihomo</p></details>{{end}}
</section>
{{if or .Support .Footer}}<footer>{{if .Support}}<a href="{{.Support}}">{{.SupportLabel}}</a>{{end}}{{if .Footer}}<span>{{.Footer}}</span>{{end}}</footer>{{end}}
</main></body></html>
`))

func (s *Server) portal(w http.ResponseWriter, r *http.Request, c, i, n, cfg Data, link, profile string, details ...Data) {
	// A browser page must not become an alternate credential-download path
	// around HWID admission. Keep account metadata and the subscription URL,
	// but remove every raw URI/profile before any built-in or custom template.
	_, hwidRequired := hwidPolicy(c, cfg)
	if hwidRequired {
		cfg = clone(cfg)
		cfg["subscription_show_connection_uri"] = false
		link, profile = "", ""
		safeDetails := make([]Data, 0, len(details))
		for _, entry := range details {
			safe := clone(entry)
			delete(safe, "link")
			safeDetails = append(safeDetails, safe)
		}
		details = safeDetails
	}
	url := subscriptionOrigin(s.Config, cfg) + "/sub/" + str(c, "subscription_token")
	used, limit := num(c, "used_traffic_bytes"), num(c, "traffic_limit_bytes")
	expiry := "No expiry"
	if t, e := timestamp(c, "expires_at"); e == nil {
		expiry = t.UTC().Format("2006-01-02")
	}
	quota := "Unlimited"
	if limit > 0 {
		quota = fmtBytes(limit)
	}
	_, clashErr := clashConfig(c, i, n)
	d := map[string]any{"Locale": str(cfg, "default_locale"), "Direction": "ltr", "Compact": str(cfg, "subscription_template") == "compact", "Site": str(cfg, "site_name"), "Name": str(c, "name"), "Protocol": strings.ToUpper(str(i, "protocol")), "Host": str(n, "public_host"), "Active": "Active", "UsageLabel": "Traffic used", "ExpiryLabel": "Expires", "Used": fmtBytes(used), "Quota": quota, "Expiry": expiry, "Limited": limit > 0, "Percent": min(100, percent(used, limit)), "URL": url, "URLLabel": "Subscription URL", "CopyLabel": "Copy link", "CopyDoneLabel": "Link copied", "CopyFailedLabel": "Select and copy the link", "Connect": "Connection", "QRLabel": "Subscription QR code", "URILabel": "Connection URI", "AppsLabel": "Compatible apps", "SupportLabel": "Contact support", "ShowQR": flag(cfg, "subscription_show_qr"), "ShowApps": flag(cfg, "subscription_show_apps"), "ShowURI": flag(cfg, "subscription_show_connection_uri"), "Link": link, "Profile": profile, "Clash": clashErr == nil, "Support": str(cfg, "support_url"), "Footer": str(cfg, "subscription_footer"), "Announcement": str(cfg, "announcement"), "AnnouncementURL": str(cfg, "announcement_url"), "ProxyLabel": "Proxy account"}
	_, singErr := singClientOutbound(c, i, n)
	_, xrayErr := xrayClientConfig(c, i, n)
	d["SingBox"], d["Xray"] = singErr == nil, xrayErr == nil
	if formats := obj(cfg, "_feed_formats"); len(formats) > 0 {
		d["Clash"], d["SingBox"], d["Xray"] = flag(formats, "clash"), flag(formats, "singbox"), flag(formats, "xray")
	}
	rate := effectiveTrafficMultiplier(c)
	d["ActualLabel"], d["Actual"], d["FactorLabel"], d["Factor"] = "Actual traffic", fmtBytes(num(c, "raw_used_traffic_bytes")), "Billing rate", fmt.Sprintf("%g", rate)
	if str(cfg, "default_locale") == "fa" {
		d["ActualLabel"], d["FactorLabel"] = "مصرف واقعی", "ضریب محاسبه"
		d["Direction"] = "rtl"
		d["Active"] = "فعال"
		d["UsageLabel"] = "مصرف ترافیک"
		d["ExpiryLabel"] = "تاریخ پایان"
		d["Connect"] = "اتصال"
		d["URLLabel"] = "آدرس اشتراک"
		d["CopyLabel"], d["CopyDoneLabel"], d["CopyFailedLabel"] = "کپی لینک", "لینک کپی شد", "لینک را انتخاب و کپی کنید"
		d["QRLabel"] = "کد اشتراک"
		d["URILabel"] = "لینک اتصال"
		d["AppsLabel"] = "برنامه‌های سازگار"
		d["SupportLabel"] = "تماس با پشتیبانی"
		d["ProxyLabel"] = "اطلاعات پروکسی"
		if limit == 0 {
			d["Quota"] = "نامحدود"
		}
		if expiry == "No expiry" {
			d["Expiry"] = "بدون انقضا"
		}
	}
	if !hwidRequired && oneOf(str(i, "protocol"), "http", "socks") {
		d["Proxy"] = fmt.Sprintf("Host: %s\nPort: %d\nUsername: %s\nPassword: %s", str(n, "public_host"), num(i, "port"), str(obj(c, "protocol_settings"), "username"), str(c, "credential"))
	}
	if flag(cfg, "subscription_show_qr") {
		png, e := qrcode.Encode(url, qrcode.Medium, 192)
		if e == nil {
			d["QR"] = template.URL("data:image/png;base64," + base64.StdEncoding.EncodeToString(png))
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	for key, value := range portalSampleData() {
		if _, ok := d[key]; !ok {
			d[key] = value
		}
	}
	modernPortalData(d, c, cfg, details)
	d["AssetsURL"] = url + "/assets/"
	for key, value := range legacyPortalData(c, cfg, url, details) {
		d[key] = value
	}
	d["ThemeLabel"], d["DarkLabel"], d["LightLabel"], d["SystemLabel"] = "Appearance", "Dark", "Light", "System"
	d["CloseLabel"] = "Close"
	d["DetailsLabel"], d["RemainingLabel"] = "Connection profiles", "Remaining"
	d["ClientLabel"], d["AutoLabel"], d["SetupLabel"], d["SetupHelp"], d["DownloadsLabel"], d["SearchLabel"] = "Application", "Auto detect / other", "Connect in a few steps", "Choose your application, copy the subscription link, then import it using Add subscription in the app.", "Download configuration", "Search profiles"
	if str(cfg, "default_locale") == "fa" {
		d["ThemeLabel"], d["DarkLabel"], d["LightLabel"], d["SystemLabel"] = "ظاهر", "تیره", "روشن", "دستگاه"
		d["CloseLabel"] = "بستن"
		d["DetailsLabel"], d["RemainingLabel"] = "کانفیگ‌های اتصال", "باقی‌مانده"
		d["ClientLabel"], d["AutoLabel"], d["SetupLabel"], d["SetupHelp"], d["DownloadsLabel"], d["SearchLabel"] = "برنامه اتصال", "تشخیص خودکار / سایر", "اتصال در چند قدم", "برنامه را انتخاب کنید، لینک اشتراک را کپی کنید و در بخش افزودن اشتراک برنامه وارد کنید.", "دانلود فایل کانفیگ", "جستجوی کانفیگ"
		if limit == 0 {
			d["Remaining"] = "نامحدود"
		}
	}
	tmpl := simplePortalTemplate
	switch str(cfg, "subscription_template") {
	case "compact":
		tmpl = portalTemplate
	case "dashboard":
		tmpl = modernPortalTemplate
	case "custom":
		custom, err := s.loadPortalTemplate(str(cfg, "subscription_template_path"))
		if err == nil {
			tmpl = custom
		} else {
			s.portalFallback(w, err)
		}
	}
	var rendered bytes.Buffer
	if err := tmpl.Execute(&limitedTemplateWriter{Writer: &rendered, Remaining: 2 << 20}, d); err != nil {
		s.portalFallback(w, err)
		rendered.Reset()
		if err = simplePortalTemplate.Execute(&rendered, d); err != nil {
			http.Error(w, "Subscription page unavailable", 500)
			return
		}
	}
	if str(cfg, "subscription_template") == "custom" {
		portalScriptPolicy(w, rendered.String())
	}
	if r.Method != "HEAD" {
		_, _ = w.Write(rendered.Bytes())
	}
}
