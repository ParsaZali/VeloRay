package panel

import (
	"errors"
	"net"
	"net/http"
	"strconv"
	"time"
)

func connectionProbeConfig(c, i, n Data) (string, Data, error) {
	protocol := str(i, "protocol")
	if protocol == "mtproto" {
		return "native", Data{"protocol": "mtproto", "endpoint": net.JoinHostPort(str(n, "public_host"), strconv.FormatInt(num(i, "port"), 10)), "secret": mtprotoSecret(c, i)}, nil
	}
	if protocol == "amneziawg" {
		settings, err := panelAWGSettings(obj(i, "protocol_settings"))
		if err != nil {
			return "", nil, err
		}
		settings.PrivateKey = wireHex(str(obj(c, "protocol_settings"), "private_key"))
		return "native", Data{"protocol": "amneziawg", "awg": Data{"settings": settings, "public_key": wireHex(str(obj(i, "protocol_settings"), "public_key")), "preshared_key": wireHex(str(obj(c, "protocol_settings"), "pre_shared_key")), "address": str(obj(c, "protocol_settings"), "address"), "endpoint": net.JoinHostPort(str(n, "public_host"), strconv.FormatInt(num(i, "port"), 10))}}, nil
	}

	if oneOf(protocol, "http", "socks") {
		account := Data{"user": str(obj(c, "protocol_settings"), "username"), "pass": str(c, "credential")}
		server := Data{"address": str(n, "public_host"), "port": num(i, "port"), "users": []any{account}}
		proxy := Data{"tag": "proxy", "protocol": protocol, "settings": Data{"servers": []any{server}}}
		return "xray", Data{"outbounds": []any{proxy}}, nil
	}
	if str(i, "core") == "singbox" {
		if protocol == "naive" {
			return "", nil, errors.New("Naive client probe requires a separate compatible Chromium build")
		}
		outbounds, err := singClientOutbounds(c, i, n)
		if err != nil {
			return "", nil, err
		}
		asData(outbounds[0])["tag"] = "proxy"
		return "singbox", Data{"outbounds": outbounds}, nil
	}
	cfg, err := xrayClientConfig(c, i, n)
	if err != nil {
		return "", nil, err
	}
	return "xray", Data{"outbounds": array(cfg, "outbounds")}, nil
}
func (s *Server) clientProbe(w http.ResponseWriter, r *http.Request, id int64, a Actor) (any, int, error) {
	if r.Method != "POST" {
		return nil, 0, fail(405, "method not allowed")
	}
	body, err := readBody(w, r)
	if err != nil {
		return nil, 0, err
	}
	if err = allowed(body, "inbound host_index url"); err != nil {
		return nil, 0, fail(400, err.Error())
	}
	if err = typed(body, Data{"inbound": int64(0), "host_index": 0, "url": ""}); err != nil {
		return nil, 0, fail(400, err.Error())
	}
	if num(body, "host_index") < 0 || num(body, "host_index") > 200 {
		return nil, 0, fail(400, "invalid delivery host index")
	}
	c, err := get(r.Context(), s.Store.Pool, "clients", id)
	if err != nil {
		return nil, 0, err
	}
	members := []Data{c}
	if account := num(c, "_account"); account > 0 {
		members, err = customerMembers(r.Context(), s.Store.Pool, account)
		if err != nil {
			return nil, 0, err
		}
	}
	selected := Data(nil)
	for _, member := range members {
		if num(member, "inbound") == num(body, "inbound") {
			selected = member
			break
		}
	}
	if selected == nil {
		return nil, 0, fail(404, "connection is not part of this account")
	}
	if !active(selected, time.Now()) {
		return nil, 0, fail(409, "account is inactive, expired or out of traffic")
	}
	in, err := get(r.Context(), s.Store.Pool, "inbounds", num(selected, "inbound"))
	if err != nil {
		return nil, 0, err
	}
	node, err := get(r.Context(), s.Store.Pool, "nodes", num(in, "node"))
	if err != nil {
		return nil, 0, err
	}
	if !flag(in, "enabled") || !flag(node, "enabled") {
		return nil, 0, fail(409, "inbound or node is disabled")
	}
	cfg, err := settings(r.Context(), s.Store.Pool)
	if err != nil {
		return nil, 0, err
	}
	hosts := probeDeliveryHosts(in, node, cfg)
	index := int(num(body, "host_index"))
	if index >= len(hosts) || !flag(hosts[index], "enabled") {
		return nil, 0, fail(404, "delivery host unavailable")
	}
	delivered, public := deliveryHost(in, node, hosts[index])
	core, config, err := connectionProbeConfig(selected, delivered, public)
	if err != nil {
		return nil, 0, fail(400, err.Error())
	}
	result, err := s.nodeCall(r.Context(), node, "/clients/probe", Data{"core": core, "config": config, "url": fallback(str(body, "url"), "https://www.gstatic.com/generate_204")})
	if err != nil {
		return nil, 0, fail(502, err.Error())
	}
	_ = audit(r.Context(), s.Store.Pool, num(a.User, "id"), "client.connection-test", str(c, "name"), Data{"inbound": num(in, "id"), "ok": flag(result, "ok")})
	return result, 200, nil
}
func probeDeliveryHosts(in, node, cfg Data) []Data {
	hosts := []Data{}
	for _, value := range array(cfg, "subscription_hosts") {
		host := asData(value)
		if num(host, "inbound") == num(in, "id") {
			hosts = append(hosts, host)
		}
	}
	if len(hosts) == 0 {
		hosts = append(hosts, Data{"name": str(in, "name"), "address": str(node, "public_host"), "enabled": true})
	}
	return hosts
}
