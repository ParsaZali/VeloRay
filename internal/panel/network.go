package panel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"time"
)

var tagPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,95}$`)

func validateNetworkEntity(ctx context.Context, q Query, kind string, d Data) error {
	if e := require(d, "name", 96); e != nil {
		return e
	}
	if _, e := get(ctx, q, "nodes", num(d, "node")); e != nil {
		return errors.New("node does not exist")
	}
	if !oneOf(str(d, "core"), "xray", "singbox") {
		return errors.New("invalid core")
	}
	raw, _ := json.Marshal(obj(d, "config"))
	if len(raw) > 65536 {
		return errors.New("configuration exceeds 64 KiB")
	}
	if kind == "outbounds" {
		if !tagPattern.MatchString(str(d, "tag")) || oneOf(str(d, "tag"), "api", "direct", "blocked") {
			return errors.New("invalid or reserved outbound tag")
		}
		if e := require(d, "protocol", 64); e != nil {
			return e
		}
		others, e := list(ctx, q, kind, "WHERE node_id=$1 AND id<>$2", num(d, "node"), num(d, "id"))
		if e != nil {
			return e
		}
		for _, x := range others {
			if str(x, "core") == str(d, "core") && str(x, "tag") == str(d, "tag") {
				return errors.New("outbound tag already exists")
			}
		}
	} else {
		if num(d, "priority") < 0 || num(d, "priority") > 100000 {
			return errors.New("priority must be 0–100000")
		}
		config := obj(d, "config")
		key := "outboundTag"
		if str(d, "core") == "singbox" {
			key = "outbound"
		}
		tag := str(config, key)
		if str(d, "core") == "singbox" && tag == "blocked" {
			return errors.New("sing-box blocking rules must use the reject action")
		}
		if tag != "" && !oneOf(tag, "direct", "blocked") {
			outs, e := list(ctx, q, "outbounds", "WHERE node_id=$1", num(d, "node"))
			if e != nil {
				return e
			}
			valid := false
			for _, o := range outs {
				if flag(o, "enabled") && str(o, "core") == str(d, "core") && str(o, "tag") == tag {
					valid = true
				}
			}
			if !valid {
				return errors.New("routing outbound does not exist")
			}
		}
	}
	return nil
}
func applyNetworkEntities(ctx context.Context, q Query, node, cfg Data, core string) error {
	outs, e := list(ctx, q, "outbounds", "WHERE node_id=$1 ORDER BY id", num(node, "id"))
	if e != nil {
		return e
	}
	entries := array(cfg, "outbounds")
	for _, o := range outs {
		if !flag(o, "enabled") || str(o, "core") != core {
			continue
		}
		v := clone(obj(o, "config"))
		v["tag"] = str(o, "tag")
		if core == "xray" {
			v["protocol"] = str(o, "protocol")
		} else {
			v["type"] = str(o, "protocol")
		}
		entries = append(entries, v)
	}
	cfg["outbounds"] = entries
	rules, e := list(ctx, q, "routing_rules", "WHERE node_id=$1 ORDER BY (data->>'priority')::bigint,id", num(node, "id"))
	if e != nil {
		return e
	}
	key := "routing"
	if core == "singbox" {
		key = "route"
	}
	route := obj(cfg, key)
	entries = array(route, "rules")
	needsSniff := false
	for _, rule := range rules {
		if flag(rule, "enabled") && str(rule, "core") == core {
			if e := validateNetworkEntity(ctx, q, "routing_rules", rule); e != nil {
				return e
			}
			v := clone(obj(rule, "config"))
			if core == "singbox" && needsProtocolSniff(v) {
				needsSniff = true
			}
			if core == "xray" {
				v["type"] = "field"
			}
			entries = append(entries, v)
		}
	}
	if needsSniff {
		entries = append([]any{Data{"action": "sniff"}}, entries...)
	}
	route["rules"] = entries
	cfg[key] = route
	return nil
}
func needsProtocolSniff(config Data) bool {
	for _, key := range []string{"protocol", "domain", "domain_suffix", "domain_keyword", "domain_regex"} {
		if len(array(config, key)) > 0 || str(config, key) != "" {
			return true
		}
	}
	for _, raw := range array(config, "rules") {
		b, _ := json.Marshal(raw)
		child, err := decode(b)
		if err == nil && needsProtocolSniff(child) {
			return true
		}
	}
	return false
}

func singTLS(i, n Data) Data {
	security := str(i, "security")
	if security == "none" {
		return nil
	}
	v := Data{"enabled": true, "server_name": fallback(str(i, "tls_server_name"), str(n, "public_host"))}
	if security == "reality" {
		v["server_name"] = str(i, "reality_server_name")
		host, port, _ := net.SplitHostPort(str(i, "reality_dest"))
		v["reality"] = Data{"enabled": true, "handshake": Data{"server": host, "server_port": num(Data{"p": port}, "p")}, "private_key": str(i, "reality_private_key"), "short_id": []any{str(i, "reality_short_id")}}
	} else {
		v["certificate_path"] = str(i, "tls_cert_file")
		v["key_path"] = str(i, "tls_key_file")
	}
	tls := obj(obj(i, "stream_settings"), "tlsSettings")
	if alpn := array(tls, "alpn"); len(alpn) > 0 {
		v["alpn"] = alpn
	}
	if min := str(tls, "minVersion"); min != "" {
		v["min_version"] = min
	}
	return v
}
func singTransport(i Data) Data {
	switch str(i, "transport") {
	case "ws":
		return Data{"type": "ws", "path": fallback(str(i, "path"), "/"), "headers": Data{"Host": str(i, "host_header")}}
	case "grpc":
		return Data{"type": "grpc", "service_name": fallback(str(i, "service_name"), str(i, "name"))}
	case "httpupgrade":
		return Data{"type": "httpupgrade", "path": fallback(str(i, "path"), "/"), "host": str(i, "host_header")}
	}
	return nil
}
func buildSingBoxConfig(ctx context.Context, q Query, node Data, fieldKey ...string) (Data, error) {
	ins, e := list(ctx, q, "inbounds", "WHERE node_id=$1", num(node, "id"))
	if e != nil {
		return nil, e
	}
	entries, users, meteredInbounds := []any{}, []any{}, []any{}
	has := false
	for _, i := range ins {
		if str(i, "core") != "singbox" {
			continue
		}
		has = true
		if !flag(i, "enabled") || !flag(node, "enabled") {
			continue
		}
		p := str(i, "protocol")
		kind := p
		if p == "hysteria" {
			kind = "hysteria2"
		}
		v := Data{"type": kind, "tag": fmt.Sprintf("in-%d-%s", num(i, "id"), str(i, "name")), "listen": str(i, "listen"), "listen_port": num(i, "port")}
		cs, e := list(ctx, q, "clients", "WHERE inbound_id=$1", num(i, "id"))
		if e != nil {
			return nil, e
		}
		accounts := []any{}
		for _, c := range cs {
			if !active(c, time.Now()) {
				continue
			}
			name := fmt.Sprintf("veloray:%d:%s", num(c, "id"), str(c, "name"))
			users = append(users, name)
			u := Data{"name": name}
			switch p {
			case "vless", "vmess":
				u["uuid"] = str(c, "credential")
				if p == "vless" {
					u["flow"] = clientFlow(c, i)
				} else {
					u["alterId"] = 0
				}
			case "tuic":
				u["uuid"], u["password"] = str(c, "credential"), str(obj(c, "protocol_settings"), "password")
			case "snell":
				u["userkey"] = str(c, "credential")
			case "naive":
				delete(u, "name")
				u["username"], u["password"] = name, str(c, "credential")
			case "http", "socks":
				delete(u, "name")
				u["username"] = fallback(str(obj(c, "protocol_settings"), "username"), str(c, "name"))
				u["password"] = str(c, "credential")
			default:
				u["password"] = str(c, "credential")
			}
			accounts = append(accounts, u)
		}
		if p == "shadowsocks" {
			meteredInbounds = append(meteredInbounds, v["tag"])
			if len(accounts) == 0 {
				continue
			}
			u := accounts[0].(Data)
			v["method"] = fallback(str(obj(i, "protocol_settings"), "method"), "aes-256-gcm")
			v["password"] = str(u, "password")
		} else {
			v["users"] = accounts
		}
		if !nativeSingInbound(v, i, accounts) {
			continue
		}
		if tls := singTLS(i, node); tls != nil {
			v["tls"] = tls
		}
		if t := singTransport(i); t != nil {
			v["transport"] = t
		}
		entries = append(entries, v)
	}
	if !has && !flag(node, "_singbox_deployed") {
		return nil, nil
	}
	cfg := Data{"log": Data{"level": "info", "timestamp": true}, "inbounds": entries, "outbounds": []any{Data{"type": "direct", "tag": "direct"}}, "route": Data{"final": "direct", "rules": []any{}}, "experimental": Data{"v2ray_api": Data{"listen": "127.0.0.1:10086", "stats": Data{"enabled": true, "users": users, "inbounds": meteredInbounds, "outbounds": []any{}}}, "clash_api": Data{"external_controller": "127.0.0.1:10087", "secret": ""}}}
	sharedPatch, err := profileConfigPatch(ctx, q, node, "singbox", fieldKey...)
	if err != nil {
		return nil, err
	}
	cfg = deepMerge(cfg, sharedPatch)
	if e = applyNetworkEntities(ctx, q, node, cfg, "singbox"); e != nil {
		return nil, e
	}
	device, e := deviceRulesForNode(ctx, q, node, "singbox")
	if e != nil {
		return nil, e
	}
	route := obj(cfg, "route")
	route["rules"] = append(device, array(route, "rules")...)
	cfg["route"] = route
	return cfg, nil
}
func singClientOutbound(c, i, n Data) (Data, error) {
	p, t := str(i, "protocol"), str(i, "transport")
	if !oneOf(p, "vless", "vmess", "trojan", "shadowsocks", "hysteria", "tuic", "naive", "snell") || !oneOf(t, "raw", "ws", "grpc", "httpupgrade", "hysteria") {
		return nil, fmt.Errorf("sing-box output unavailable for %s/%s", p, t)
	}
	kind := p
	if kind == "hysteria" {
		kind = "hysteria2"
	}
	v := Data{"type": kind, "tag": connectionName(c, i), "server": str(n, "public_host"), "server_port": num(i, "port")}
	if oneOf(p, "vless", "vmess") {
		v["uuid"] = str(c, "credential")
		if p == "vless" {
			v["flow"] = clientFlow(c, i)
		} else {
			v["security"] = "auto"
			v["alter_id"] = 0
		}
	} else {
		v["password"] = str(c, "credential")
	}
	switch p {
	case "tuic":
		v["uuid"] = str(c, "credential")
		v["password"] = str(obj(c, "protocol_settings"), "password")
		v["congestion_control"] = fallback(str(obj(i, "protocol_settings"), "congestion_control"), "bbr")
		v["udp_relay_mode"] = "native"
	case "naive":
		v["username"] = fmt.Sprintf("veloray:%d:%s", num(c, "id"), str(c, "name"))
	case "snell":
		delete(v, "password")
		v["version"] = 6
		v["psk"] = str(obj(i, "protocol_settings"), "psk")
		v["userkey"] = str(c, "credential")
		v["mode"] = fallback(str(obj(i, "protocol_settings"), "mode"), "default")
	}
	if p == "shadowsocks" {
		v["method"] = fallback(str(obj(i, "protocol_settings"), "method"), "aes-256-gcm")
	}
	if str(i, "security") != "none" {
		tls := Data{"enabled": true, "server_name": fallback(str(i, "tls_server_name"), str(n, "public_host"))}
		if str(i, "security") == "reality" {
			tls["server_name"] = str(i, "reality_server_name")
			tls["utls"] = Data{"enabled": true, "fingerprint": "chrome"}
			tls["reality"] = Data{"enabled": true, "public_key": str(i, "reality_public_key"), "short_id": str(i, "reality_short_id")}
		}
		v["tls"] = tls
	}
	if tr := singTransport(i); tr != nil {
		v["transport"] = tr
	}
	return v, nil
}
