package panel

import (
	"encoding/json"
	"errors"
	"strings"
)

// Client templates are JSON objects. Dynamic proxies/selectors are inserted by
// the server, never interpolated as strings, so credentials cannot break JSON.
func validateFeedTemplate(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	if len(raw) > 128<<10 {
		return errors.New("template exceeds 128 KiB")
	}
	value, err := decode([]byte(raw))
	if err != nil || value == nil || !json.Valid([]byte(raw)) {
		return errors.New("a valid JSON object is required")
	}
	return nil
}
func applyFeedTemplate(raw, generated, format string) (string, error) {
	if err := validateFeedTemplate(raw); err != nil {
		return "", err
	}
	original, err := decode([]byte(generated))
	if err != nil {
		return "", err
	}
	result := original
	if strings.TrimSpace(raw) != "" {
		result, err = decode([]byte(raw))
		if err != nil {
			return "", err
		}
		switch format {
		case "clash":
			result["proxies"] = original["proxies"]
			if len(array(result, "proxy-groups")) == 0 {
				result["proxy-groups"] = original["proxy-groups"]
			}
			expandProxyNames(result, array(asData(array(original, "proxy-groups")[0]), "proxies"))
			if _, ok := result["rules"]; !ok {
				result["rules"] = original["rules"]
			}
		case "xray":
			result["outbounds"] = original["outbounds"]
			result["remarks"] = original["remarks"]
		case "singbox":
			result["outbounds"] = original["outbounds"]
			if _, ok := result["route"]; !ok {
				result["route"] = original["route"]
			}
		}
	}
	data, err := json.MarshalIndent(result, "", "  ")
	return string(data) + "\n", err
}

// A whole array item named $VELO_PROXIES expands to all generated proxy names.
func expandProxyNames(data Data, names []any) {
	for _, item := range array(data, "proxy-groups") {
		group := asData(item)
		out := []any{}
		for _, name := range array(group, "proxies") {
			if value, ok := name.(string); ok && value == "$VELO_PROXIES" {
				out = append(out, names...)
			} else {
				out = append(out, name)
			}
		}
		group["proxies"] = out
	}
}
