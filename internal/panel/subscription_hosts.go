package panel

import (
	"errors"
	"fmt"
	"strings"
)

// Delivery hosts override only exported client settings, never the listener.
// Multiple hosts can publish one listener via distinct CDN addresses and ports.
func validateSubscriptionHosts(entries []any) error {
	if len(entries) > 200 {
		return errors.New("at most 200 subscription hosts")
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		d := asData(entry)
		if err := allowed(d, "name inbound address port sni path host_header enabled"); err != nil {
			return err
		}
		if err := require(d, "name", 96); err != nil {
			return err
		}
		if num(d, "inbound") < 1 || !validHost(str(d, "address")) {
			return errors.New("each host requires an inbound and public address")
		}
		if p := num(d, "port"); p < 0 || p > 65535 {
			return errors.New("invalid delivery port")
		}
		if sni := str(d, "sni"); sni != "" && !validHost(sni) {
			return errors.New("invalid delivery SNI")
		}
		for _, key := range []string{"path", "host_header"} {
			if len(str(d, key)) > 2048 || strings.ContainsAny(str(d, key), "\r\n") {
				return errors.New("invalid delivery path or host header")
			}
		}
		if path := str(d, "path"); path != "" && !strings.HasPrefix(path, "/") {
			return errors.New("delivery path must start with /")
		}
		if _, ok := d["enabled"].(bool); !ok {
			return errors.New("host enabled must be a boolean")
		}
		key := fmt.Sprintf("%d:%s", num(d, "inbound"), str(d, "name"))
		if seen[key] {
			return errors.New("duplicate host name for inbound")
		}
		seen[key] = true
	}
	return nil
}
func deliveryHost(i, n, host Data) (Data, Data) {
	i, n = clone(i), clone(n)
	n["public_host"] = str(host, "address")
	if num(host, "port") > 0 {
		i["port"] = num(host, "port")
	}
	if str(host, "sni") != "" {
		if str(i, "security") == "reality" {
			i["reality_server_name"] = str(host, "sni")
		} else {
			i["tls_server_name"] = str(host, "sni")
		}
	}
	if str(host, "path") != "" {
		i["path"] = str(host, "path")
	}
	if str(host, "host_header") != "" {
		i["host_header"] = str(host, "host_header")
	}
	i["name"] = str(host, "name")
	return i, n
}
