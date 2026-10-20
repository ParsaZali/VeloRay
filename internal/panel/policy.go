package panel

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strconv"
	"time"
)

func activateFirstUse(c Data, now time.Time, used bool) bool {
	if !flag(c, "enabled") || !used || !flag(c, "start_after_first_use") || str(c, "first_used_at") != "" || num(c, "duration_days") < 1 {
		return false
	}
	c["first_used_at"] = stamp(now)
	c["expires_at"] = stamp(now.Add(time.Duration(num(c, "duration_days")) * 24 * time.Hour))
	return true
}
func connectionName(c, i Data) string {
	name := "VeloRay · " + str(c, "name") + " · " + str(i, "name")
	if rate := effectiveTrafficMultiplier(c); rate > 1 {
		suffix := strconv.FormatFloat(rate, 'f', 3, 64)
		for len(suffix) > 0 && suffix[len(suffix)-1] == '0' {
			suffix = suffix[:len(suffix)-1]
		}
		if suffix[len(suffix)-1] == '.' {
			suffix += "0"
		}
		name += " · " + suffix + "x"
	}
	return name
}
func updateDevicePolicy(c Data, online Data, now time.Time) bool {
	limit := num(c, "device_limit")
	if limit == 0 {
		if len(obj(c, "_device_denied")) > 0 || len(obj(c, "_device_seen")) > 0 {
			c["_device_denied"] = Data{}
			c["_device_seen"] = Data{}
			return true
		}
		return false
	}
	seen, denied := obj(c, "_device_seen"), obj(c, "_device_denied")
	changed := false
	for ip, v := range denied {
		if now.Unix()-num(Data{"v": v}, "v") > 600 {
			delete(denied, ip)
			changed = true
		}
	}
	for ip, v := range seen {
		if _, present := online[ip]; !present && now.Unix()-num(Data{"v": v}, "v") > 90 {
			delete(seen, ip)
		}
	}
	candidates := []string{}
	for ip := range online {
		if net.ParseIP(ip) == nil {
			continue
		}
		if _, no := denied[ip]; no {
			continue
		}
		candidates = append(candidates, ip)
	}
	sort.Strings(candidates)
	for _, ip := range candidates {
		if _, exists := seen[ip]; exists {
			seen[ip] = now.Unix()
			continue
		}
		if int64(len(seen)) >= limit {
			oldest := ""
			var at int64 = 1<<63 - 1
			for old, v := range seen {
				n := num(Data{"v": v}, "v")
				if n < at || n == at && old < oldest {
					oldest, at = old, n
				}
			}
			if oldest != "" {
				delete(seen, oldest)
				denied[oldest] = now.Unix()
				changed = true
			}
		}
		seen[ip] = now.Unix()
	}
	for int64(len(seen)) > limit {
		oldest := ""
		at := int64(1<<63 - 1)
		for ip, v := range seen {
			n := num(Data{"v": v}, "v")
			if n < at || n == at && ip < oldest {
				oldest, at = ip, n
			}
		}
		delete(seen, oldest)
		denied[oldest] = now.Unix()
		changed = true
	}
	c["_device_seen"], c["_device_denied"] = seen, denied
	return changed
}
func deviceRulesForNode(ctx context.Context, q Query, node Data, core string) ([]any, error) {
	cs, e := list(ctx, q, "clients", "WHERE inbound_id IN(SELECT id FROM vr_inbounds WHERE node_id=$1)", num(node, "id"))
	if e != nil {
		return nil, e
	}
	rules := []any{}
	for _, c := range cs {
		i, e := get(ctx, q, "inbounds", num(c, "inbound"))
		if e != nil {
			return nil, e
		}
		if fallback(str(i, "core"), "xray") != core || num(c, "device_limit") == 0 {
			continue
		}
		ips := []any{}
		for ip, v := range obj(c, "_device_denied") {
			if net.ParseIP(ip) != nil && time.Now().Unix()-num(Data{"v": v}, "v") <= 600 {
				if core == "singbox" {
					if net.ParseIP(ip).To4() != nil {
						ip += "/32"
					} else {
						ip += "/128"
					}
				}
				ips = append(ips, ip)
			}
		}
		if len(ips) == 0 {
			continue
		}
		user := fmt.Sprintf("veloray:%d:%s", num(c, "id"), str(c, "name"))
		if str(i, "protocol") == "naive" {
			user = naiveUsername(c)
		}
		if core == "xray" {
			rules = append(rules, Data{"type": "field", "user": []any{user}, "source": ips, "outboundTag": "blocked"})
		} else {
			rules = append(rules, Data{"auth_user": []any{user}, "source_ip_cidr": ips, "action": "reject"})
		}
	}
	return rules, nil
}
