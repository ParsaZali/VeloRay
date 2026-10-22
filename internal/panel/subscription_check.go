package panel

import (
	"context"
	"time"
)

// Explain compatibility without exposing config contents or claiming to probe a
// subscriber's network. This intentionally performs no external network I/O.
func (s *Server) deliveryCheck(ctx context.Context, id int64) (Data, error) {
	c, err := get(ctx, s.Store.Pool, "clients", id)
	if err != nil {
		return nil, err
	}
	cfg, err := settings(ctx, s.Store.Pool)
	if err != nil {
		return nil, err
	}
	members := []Data{c}
	if account := num(c, "_account"); account > 0 {
		members, err = list(ctx, s.Store.Pool, "clients", "WHERE account_id=$1 ORDER BY id", account)
		if err != nil {
			return nil, err
		}
	}
	results := []any{}
	for _, member := range members {
		inbound, err := get(ctx, s.Store.Pool, "inbounds", num(member, "inbound"))
		if err != nil {
			return nil, err
		}
		node, err := get(ctx, s.Store.Pool, "nodes", num(inbound, "node"))
		if err != nil {
			return nil, err
		}
		hosts := []Data{}
		mapped := false
		for _, value := range array(cfg, "subscription_hosts") {
			host := asData(value)
			if num(host, "inbound") == num(inbound, "id") {
				mapped = true
				hosts = append(hosts, host)
			}
		}
		if !mapped {
			hosts = append(hosts, Data{"name": str(inbound, "name"), "address": str(node, "public_host"), "enabled": true})
		}
		for hostIndex, host := range hosts {
			i, n := deliveryHost(inbound, node, host)
			reasons := []string{}
			if !active(member, time.Now()) {
				reasons = append(reasons, "account_inactive")
			}
			if !flag(inbound, "enabled") {
				reasons = append(reasons, "inbound_disabled")
			}
			if !flag(node, "enabled") {
				reasons = append(reasons, "node_disabled")
			}
			if !flag(host, "enabled") {
				reasons = append(reasons, "host_hidden")
			}
			if !flag(cfg, "subscription_enabled") {
				reasons = append(reasons, "subscriptions_disabled")
			}
			if flag(cfg, "maintenance_mode") {
				reasons = append(reasons, "maintenance")
			}
			formats := Data{}
			formats["raw"] = Data{"available": shareLink(member, i, n) != ""}
			formats["wireguard"] = Data{"available": profile(member, i, n) != ""}
			if _, err = clashConfig(member, i, n); err != nil {
				formats["clash"] = Data{"available": false, "reason": err.Error()}
			} else {
				formats["clash"] = Data{"available": true}
			}
			if _, err = singClientOutbound(member, i, n); err != nil {
				formats["singbox"] = Data{"available": false, "reason": err.Error()}
			} else {
				formats["singbox"] = Data{"available": true}
			}
			_, err = xrayClientConfig(member, i, n)
			formats["xray"] = Data{"available": err == nil}
			if err != nil {
				asData(formats["xray"])["reason"] = err.Error()
			}
			_, _, probeErr := connectionProbeConfig(member, i, n)
			results = append(results, Data{"inbound": num(inbound, "id"), "name": str(host, "name"), "protocol": str(i, "protocol"), "eligible": len(reasons) == 0, "reasons": reasons, "formats": formats, "host_index": hostIndex, "probe_available": probeErr == nil})
		}
	}
	limit, required := hwidPolicy(c, cfg)
	templateValid := true
	if str(cfg, "subscription_template") == "custom" {
		_, err := s.loadPortalTemplate(str(cfg, "subscription_template_path"))
		templateValid = err == nil
	}
	return Data{"checks": results, "hwid_required": required, "hwid_limit": limit, "template_available": templateValid, "network_tested": false}, nil
}
