package panel

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

// Fallbacks are operator-owned core destinations, never fetched by the panel.
func validateInboundFallbacks(in Data) error {
	ps := obj(in, "protocol_settings")
	raw, exists := ps["fallbacks"]
	if !exists {
		return nil
	}
	items, ok := raw.([]any)
	if !ok || len(items) > 32 {
		return errors.New("fallbacks must be an array of at most 32 entries")
	}
	if len(items) == 0 {
		return nil
	}
	if fallback(str(in, "core"), "xray") != "xray" || !oneOf(str(in, "protocol"), "vless", "trojan") || str(in, "transport") != "raw" || str(in, "security") == "reality" {
		return errors.New("fallbacks require Xray VLESS/Trojan over RAW with TLS or no security")
	}
	seen := map[string]bool{}
	groups := map[string]bool{}
	for _, item := range items {
		b, err := json.Marshal(item)
		v, decErr := decode(b)
		if err != nil || decErr != nil || v == nil {
			return errors.New("each fallback must be an object")
		}
		if err = allowed(v, "name alpn path dest xver"); err != nil {
			return err
		}
		for _, key := range []string{"name", "alpn", "path"} {
			if value, exists := v[key]; exists {
				if _, ok := value.(string); !ok {
					return fmt.Errorf("fallback %s must be a string", key)
				}
			}
			if len(str(v, key)) > 255 || strings.ContainsAny(str(v, key), "\x00\r\n") {
				return fmt.Errorf("invalid fallback %s", key)
			}
		}
		path := str(v, "path")
		if path != "" && (!strings.HasPrefix(path, "/") || str(v, "alpn") == "h2") {
			return errors.New("fallback path must start with / and cannot be combined with h2")
		}
		match, _ := json.Marshal([]string{str(v, "name"), str(v, "alpn"), path})
		group, _ := json.Marshal([]string{str(v, "name"), str(v, "alpn")})
		if _, exists := groups[string(group)]; !exists {
			groups[string(group)] = false
		}
		if path == "" {
			groups[string(group)] = true
		}
		if seen[string(match)] {
			return errors.New("duplicate fallback match")
		}
		seen[string(match)] = true
		if value, exists := v["xver"]; exists {
			encoded, _ := json.Marshal(value)
			n, err := strconv.Atoi(string(encoded))
			if err != nil || n < 0 || n > 2 {
				return errors.New("fallback xver must be 0, 1 or 2")
			}
		}
		dest, ok := v["dest"].(string)
		if !ok {
			encoded, _ := json.Marshal(v["dest"])
			n, err := strconv.Atoi(string(encoded))
			if err != nil || n < 1 || n > 65535 {
				return errors.New("fallback destination must be a port, host:port or Unix socket")
			}
			continue
		}
		if len(dest) == 0 || len(dest) > 255 || strings.ContainsAny(dest, "\x00\r\n\t ") {
			return errors.New("invalid fallback destination")
		}
		if strings.HasPrefix(dest, "/") || strings.HasPrefix(dest, "@") {
			if len(dest) > 107 || dest == "/" || dest == "@" {
				return errors.New("invalid fallback Unix socket")
			}
			continue
		}
		if n, err := strconv.Atoi(dest); err == nil && n > 0 && n <= 65535 {
			continue
		}
		host, port, err := net.SplitHostPort(dest)
		n, portErr := strconv.Atoi(port)
		if err != nil || !validHost(host) || portErr != nil || n < 1 || n > 65535 {
			return errors.New("invalid fallback host:port")
		}
	}
	root, _ := json.Marshal([]string{"", "", ""})
	if !seen[string(root)] {
		return errors.New("fallbacks require a default destination with empty SNI, ALPN and path")
	}
	for _, hasDefault := range groups {
		if !hasDefault {
			return errors.New("each fallback SNI/ALPN group requires a default destination with an empty path")
		}
	}
	return nil
}
