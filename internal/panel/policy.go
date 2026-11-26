package panel

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"strings"
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
func sourcePrefix(value string) (netip.Prefix, error) {
	if address, err := netip.ParseAddr(value); err == nil && address.Zone() == "" {
		address = address.Unmap()
		return netip.PrefixFrom(address, address.BitLen()), nil
	}
	prefix, err := netip.ParsePrefix(value)
	if err != nil || prefix.Addr().Is4In6() || prefix.Bits() == 0 {
		return netip.Prefix{}, fmt.Errorf("invalid trusted source IP or CIDR: %q", value)
	}
	return prefix.Masked(), nil
}

func normalizeTrustedSourceIPs(d Data) error {
	raw, exists := d["trusted_source_ips"]
	if !exists {
		return nil
	}
	entries, ok := raw.([]any)
	if !ok || len(entries) > 32 {
		return fmt.Errorf("trusted source IPs must be a list of at most 32 IPs or ranges")
	}
	canonical := make([]any, 0, len(entries))
	seen := map[netip.Prefix]bool{}
	for _, entry := range entries {
		value, ok := entry.(string)
		if !ok || len(value) > 80 {
			return fmt.Errorf("invalid trusted source IP or CIDR")
		}
		prefix, err := sourcePrefix(strings.TrimSpace(value))
		if err != nil {
			return err
		}
		if !seen[prefix] {
			canonical = append(canonical, prefix.String())
			seen[prefix] = true
		}
	}
	d["trusted_source_ips"] = canonical
	return nil
}

func trustedSourceMatcher(c Data) func(string) bool {
	prefixes := []netip.Prefix{}
	for _, v := range array(c, "trusted_source_ips") {
		if p, e := sourcePrefix(fmt.Sprint(v)); e == nil {
			prefixes = append(prefixes, p)
		}
	}
	return func(value string) bool {
		if len(prefixes) == 0 {
			return false
		}
		if addr, err := netip.ParseAddr(value); err == nil {
			for _, prefix := range prefixes {
				if prefix.Contains(addr.Unmap()) {
					return true
				}
			}
		}
		return false
	}
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
	trusted := trustedSourceMatcher(c)
	for ip, v := range denied {
		if trusted(ip) || now.Unix()-num(Data{"v": v}, "v") > 600 {
			delete(denied, ip)
			changed = true
		}
	}
	for ip, v := range seen {
		if trusted(ip) {
			delete(seen, ip)
			changed = true
			continue
		}
		if _, present := online[ip]; !present && now.Unix()-num(Data{"v": v}, "v") > 90 {
			delete(seen, ip)
		}
	}
	candidates := []string{}
	for ip := range online {
		if net.ParseIP(ip) == nil || trusted(ip) {
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
		trusted := trustedSourceMatcher(c)
		for ip, v := range obj(c, "_device_denied") {
			if net.ParseIP(ip) != nil && !trusted(ip) && time.Now().Unix()-num(Data{"v": v}, "v") <= 600 {
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
