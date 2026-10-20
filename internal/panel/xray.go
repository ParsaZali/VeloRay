package panel

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func streamSettings(i, node Data) Data {
	p, t, s := str(i, "protocol"), str(i, "transport"), str(i, "security")
	if oneOf(p, "wireguard", "tunnel", "tun") {
		return nil
	}
	method := map[string]string{"raw": "raw", "ws": "websocket", "grpc": "grpc", "xhttp": "xhttp", "httpupgrade": "httpupgrade", "mkcp": "mkcp", "hysteria": "hysteria"}[t]
	v := clone(obj(i, "stream_settings"))
	v["method"], v["security"] = method, s
	switch t {
	case "ws":
		v["wsSettings"] = merge(obj(v, "wsSettings"), Data{"path": fallback(str(i, "path"), "/"), "host": str(i, "host_header")})
	case "grpc":
		v["grpcSettings"] = merge(obj(v, "grpcSettings"), Data{"serviceName": fallback(str(i, "service_name"), str(i, "name"))})
	case "xhttp":
		v["xhttpSettings"] = merge(Data{"mode": "auto"}, obj(v, "xhttpSettings"))
		obj(v, "xhttpSettings")["path"] = fallback(str(i, "path"), "/")
	case "httpupgrade":
		v["httpupgradeSettings"] = merge(obj(v, "httpupgradeSettings"), Data{"path": fallback(str(i, "path"), "/"), "host": str(i, "host_header")})
	case "mkcp":
		if len(obj(v, "kcpSettings")) == 0 {
			v["kcpSettings"] = Data{}
		}
	case "hysteria":
		h := Data{"version": 2, "udpIdleTimeout": 60}
		ps := obj(i, "protocol_settings")
		if num(ps, "udp_idle_timeout") > 0 {
			h["udpIdleTimeout"] = num(ps, "udp_idle_timeout")
		}
		if str(ps, "masquerade_url") != "" {
			h["masquerade"] = Data{"type": "proxy", "url": str(ps, "masquerade_url"), "rewriteHost": true, "insecure": false}
		}
		v["hysteriaSettings"] = h
	}
	if s == "tls" {
		v["tlsSettings"] = merge(obj(v, "tlsSettings"), Data{"serverName": fallback(str(i, "tls_server_name"), str(node, "public_host")), "certificates": []any{Data{"certificateFile": str(i, "tls_cert_file"), "keyFile": str(i, "tls_key_file")}}})
	}
	if s == "reality" {
		v["realitySettings"] = merge(obj(v, "realitySettings"), Data{"show": false, "target": str(i, "reality_dest"), "serverNames": []any{str(i, "reality_server_name")}, "privateKey": str(i, "reality_private_key"), "shortIds": []any{str(i, "reality_short_id")}})
	}
	return v
}
func clientFlow(c, i Data) string {
	settings := obj(c, "protocol_settings")
	if _, exists := settings["flow"]; exists {
		return str(settings, "flow")
	}
	return str(i, "flow")
}

func inboundSettings(i, node Data, clients []Data) (Data, error) {
	p := str(i, "protocol")
	ps := obj(i, "protocol_settings")
	entries := []any{}
	for _, c := range clients {
		email := fmt.Sprintf("veloray:%d:%s", num(c, "id"), str(c, "name"))
		v := Data{"email": email, "level": 0}
		switch p {
		case "vless", "vmess":
			v["id"] = str(c, "credential")
			if p == "vmess" {
				v["alterId"] = 0
			}
			if p == "vless" {
				if flow := clientFlow(c, i); flow != "" {
					v["flow"] = flow
				}
			}
		case "trojan":
			v["password"] = str(c, "credential")
		case "hysteria":
			v["auth"] = str(c, "credential")
		case "http", "socks":
			v = Data{"user": fallback(str(obj(c, "protocol_settings"), "username"), str(c, "name")), "pass": str(c, "credential")}
		case "wireguard":
			cs := obj(c, "protocol_settings")
			address := str(cs, "address")
			if address == "" {
				address = wireguardAddress(num(c, "id"))
			}
			v = Data{"publicKey": str(cs, "public_key"), "allowedIPs": []any{address}, "email": email, "level": 0}
			if str(cs, "pre_shared_key") != "" {
				v["preSharedKey"] = str(cs, "pre_shared_key")
			}
			if num(cs, "keepalive") > 0 {
				v["keepAlive"] = num(cs, "keepalive")
			}
		}
		entries = append(entries, v)
	}
	switch p {
	case "vless":
		v := Data{"clients": entries, "decryption": "none"}
		if len(array(ps, "fallbacks")) > 0 {
			v["fallbacks"] = array(ps, "fallbacks")
		}
		return v, nil
	case "trojan":
		v := Data{"clients": entries}
		if len(array(ps, "fallbacks")) > 0 {
			v["fallbacks"] = array(ps, "fallbacks")
		}
		return v, nil
	case "vmess":
		return Data{"clients": entries}, nil
	case "hysteria":
		return Data{"version": 2, "users": entries}, nil
	case "shadowsocks":
		if len(clients) == 0 {
			return nil, nil
		}
		if len(clients) > 1 {
			return nil, fmt.Errorf("Shadowsocks supports one client per inbound")
		}
		return Data{"method": fallback(str(ps, "method"), "aes-256-gcm"), "password": str(clients[0], "credential"), "network": "tcp,udp"}, nil
	case "http":
		if len(clients) == 0 {
			return nil, nil
		}
		return Data{"users": entries, "allowTransparent": false, "userLevel": 0}, nil
	case "socks":
		if len(clients) == 0 {
			return nil, nil
		}
		udp := true
		if _, ok := ps["udp"]; ok {
			udp = flag(ps, "udp")
		}
		return Data{"auth": "password", "users": entries, "udp": udp, "ip": fallback(str(ps, "udp_ip"), str(node, "public_host")), "userLevel": 0}, nil
	case "wireguard":
		mtu := num(ps, "mtu")
		if mtu == 0 {
			mtu = 1420
		}
		return Data{"secretKey": str(ps, "secret_key"), "peers": entries, "mtu": mtu}, nil
	case "tunnel":
		d := Data{"allowedNetwork": fallback(str(ps, "allowed_network"), "tcp"), "rewriteAddress": fallback(str(ps, "rewrite_address"), "localhost"), "rewritePort": num(ps, "rewrite_port"), "followRedirect": flag(ps, "follow_redirect"), "userLevel": 0}
		if len(obj(ps, "port_map")) > 0 {
			d["portMap"] = obj(ps, "port_map")
		}
		return d, nil
	case "tun":
		mtu := num(ps, "mtu")
		if mtu == 0 {
			mtu = 1500
		}
		gateway := array(ps, "gateway")
		if len(gateway) == 0 {
			gateway = []any{"10.66.0.1/16"}
		}
		return Data{"name": fallback(str(ps, "name"), "veloray-tun0"), "mtu": mtu, "gateway": gateway, "dns": array(ps, "dns"), "userLevel": 0, "autoSystemRoutingTable": array(ps, "auto_routes"), "autoOutboundsInterface": fallback(str(ps, "outbound_interface"), "auto")}, nil
	}
	return nil, fmt.Errorf("unsupported protocol %s", p)
}
func buildConfig(ctx context.Context, q Query, node Data, fieldKey ...string) (Data, error) {
	inbounds, e := list(ctx, q, "inbounds", "WHERE node_id=$1", num(node, "id"))
	if e != nil {
		return nil, e
	}
	items := []any{}
	for _, i := range inbounds {
		if !flag(i, "enabled") || !flag(node, "enabled") || str(i, "core") == "singbox" {
			continue
		}
		all, e := list(ctx, q, "clients", "WHERE inbound_id=$1", num(i, "id"))
		if e != nil {
			return nil, e
		}
		clients := []Data{}
		for _, c := range all {
			if active(c, time.Now()) {
				clients = append(clients, c)
			}
		}
		settings, e := inboundSettings(i, node, clients)
		if e != nil {
			return nil, e
		}
		if settings == nil {
			continue
		}
		v := Data{"tag": fmt.Sprintf("in-%d-%s", num(i, "id"), str(i, "name")), "protocol": str(i, "protocol"), "settings": settings}
		if str(i, "protocol") != "tun" {
			v["listen"] = str(i, "listen")
			v["port"] = num(i, "port")
		}
		if !oneOf(str(i, "protocol"), "tun", "tunnel") {
			v["sniffing"] = Data{"enabled": true, "destOverride": []any{"http", "tls", "quic"}, "routeOnly": true}
		}
		if stream := streamSettings(i, node); stream != nil {
			v["streamSettings"] = stream
		}
		items = append(items, v)
	}
	cfg := Data{"log": Data{"loglevel": "warning"}, "inbounds": items, "outbounds": []any{Data{"protocol": "freedom", "tag": "direct"}, Data{"protocol": "blackhole", "tag": "blocked"}}, "routing": Data{"domainStrategy": "AsIs", "rules": []any{}}}
	sharedPatch, err := profileConfigPatch(ctx, q, node, "xray", fieldKey...)
	if err != nil {
		return nil, err
	}
	cfg = deepMerge(cfg, sharedPatch)
	cfg = deepMerge(cfg, obj(node, "config_patch"))
	if e = applyNetworkEntities(ctx, q, node, cfg, "xray"); e != nil {
		return nil, e
	}
	cfg["api"] = Data{"tag": "api", "services": []any{"StatsService", "HandlerService"}}
	cfg["stats"] = Data{}
	policy := obj(cfg, "policy")
	levels := obj(policy, "levels")
	l0 := obj(levels, "0")
	l0["statsUserUplink"] = true
	l0["statsUserDownlink"] = true
	l0["statsUserOnline"] = true
	levels["0"] = l0
	policy["levels"] = levels
	system := obj(policy, "system")
	for _, k := range []string{"statsInboundUplink", "statsInboundDownlink", "statsOutboundUplink", "statsOutboundDownlink"} {
		system[k] = true
	}
	policy["system"] = system
	cfg["policy"] = policy
	final := []any{Data{"tag": "api", "listen": "127.0.0.1", "port": 10085, "protocol": "dokodemo-door", "settings": Data{"address": "127.0.0.1"}}}
	for _, item := range array(cfg, "inbounds") {
		b, _ := json.Marshal(item)
		d, _ := decode(b)
		if str(d, "tag") != "api" {
			final = append(final, item)
		}
	}
	cfg["inbounds"] = final
	routing := obj(cfg, "routing")
	rules := []any{Data{"type": "field", "inboundTag": []any{"api"}, "outboundTag": "api"}}
	for _, r := range array(routing, "rules") {
		b, _ := json.Marshal(r)
		d, _ := decode(b)
		if str(d, "outboundTag") != "api" {
			rules = append(rules, r)
		}
	}
	deviceRules, e := deviceRulesForNode(ctx, q, node, "xray")
	if e != nil {
		return nil, e
	}
	rules = append(deviceRules, rules...)
	routing["rules"] = rules
	cfg["routing"] = routing
	sing, e := buildSingBoxConfig(ctx, q, node, fieldKey...)
	if e != nil {
		return nil, e
	}
	if sing != nil {
		cfg["_singbox"] = sing
	}
	return cfg, nil
}
func shareLink(c, i, node Data) string {
	p := str(i, "protocol")
	host := str(node, "public_host")
	addr := net.JoinHostPort(host, strconv.FormatInt(num(i, "port"), 10))
	name := connectionName(c, i)
	t := str(i, "transport")
	if t == "raw" {
		t = "tcp"
	}
	params := url.Values{"type": {t}, "security": {str(i, "security")}}
	if oneOf(t, "ws", "xhttp", "httpupgrade") {
		params.Set("path", fallback(str(i, "path"), "/"))
	}
	if str(i, "host_header") != "" {
		params.Set("host", str(i, "host_header"))
	}
	if t == "grpc" {
		params.Set("serviceName", fallback(str(i, "service_name"), str(i, "name")))
	}
	if str(i, "security") == "tls" {
		params.Set("sni", fallback(str(i, "tls_server_name"), host))
	}
	if str(i, "security") == "reality" {
		params.Set("sni", str(i, "reality_server_name"))
		params.Set("fp", "chrome")
		params.Set("pbk", str(i, "reality_public_key"))
		params.Set("sid", str(i, "reality_short_id"))
	}
	credential := str(c, "credential")
	fragment := url.QueryEscape(name)
	fragment = strings.ReplaceAll(fragment, "+", "%20")
	switch p {
	case "vless", "trojan":
		if p == "vless" {
			params.Set("encryption", "none")
			if flow := clientFlow(c, i); flow != "" {
				params.Set("flow", flow)
			}
		}
		return p + "://" + url.PathEscape(credential) + "@" + addr + "?" + params.Encode() + "#" + fragment
	case "vmess":
		v := Data{"v": "2", "ps": name, "add": host, "port": strconv.FormatInt(num(i, "port"), 10), "id": credential, "aid": "0", "scy": "auto", "net": t, "type": "none", "host": str(i, "host_header"), "path": str(i, "path"), "tls": "", "sni": ""}
		if str(i, "security") == "tls" {
			v["tls"] = "tls"
			v["sni"] = fallback(str(i, "tls_server_name"), host)
		}
		b, _ := json.Marshal(v)
		return "vmess://" + base64.StdEncoding.EncodeToString(b)
	case "shadowsocks":
		return "ss://" + base64.RawURLEncoding.EncodeToString([]byte(fallback(str(obj(i, "protocol_settings"), "method"), "aes-256-gcm")+":"+credential)) + "@" + addr + "#" + fragment
	case "tuic":
		q := url.Values{"sni": {fallback(str(i, "tls_server_name"), host)}, "congestion_control": {fallback(str(obj(i, "protocol_settings"), "congestion_control"), "bbr")}, "udp_relay_mode": {"native"}}
		return "tuic://" + url.UserPassword(credential, str(obj(c, "protocol_settings"), "password")).String() + "@" + addr + "?" + q.Encode() + "#" + fragment
	case "naive":
		return "naive+https://" + url.UserPassword(fmt.Sprintf("veloray:%d:%s", num(c, "id"), str(c, "name")), credential).String() + "@" + addr + "#" + fragment
	case "hysteria":
		return "hysteria2://" + url.PathEscape(credential) + "@" + addr + "/?sni=" + url.QueryEscape(fallback(str(i, "tls_server_name"), host)) + "#" + fragment
	}
	return ""
}
func wireguardAddress(id int64) string {
	if id < 1 || id > 64009 {
		return ""
	}
	return fmt.Sprintf("10.66.%d.%d/32", (id-1)/253+1, (id-1)%253+2)
}
func profile(c, i, node Data) string {
	if str(i, "protocol") != "wireguard" {
		return ""
	}
	cs, ps := obj(c, "protocol_settings"), obj(i, "protocol_settings")
	address := fallback(str(cs, "address"), wireguardAddress(num(c, "id")))
	if str(cs, "private_key") == "" || str(ps, "public_key") == "" || address == "" {
		return ""
	}
	v := fmt.Sprintf("[Interface]\nPrivateKey = %s\nAddress = %s\nDNS = %s\n\n[Peer]\nPublicKey = %s\nEndpoint = %s\nAllowedIPs = 0.0.0.0/0, ::/0\nPersistentKeepalive = %d\n", str(cs, "private_key"), address, fallback(str(ps, "dns"), "1.1.1.1"), str(ps, "public_key"), net.JoinHostPort(str(node, "public_host"), strconv.FormatInt(num(i, "port"), 10)), max(25, num(cs, "keepalive")))
	if str(cs, "pre_shared_key") != "" {
		v += "PresharedKey = " + str(cs, "pre_shared_key") + "\n"
	}
	return v
}
