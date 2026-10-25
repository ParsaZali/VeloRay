package panel

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// HTTP Basic usernames cannot contain colons. Encode the canonical principal
// reversibly so subscriptions, telemetry and per-user policy share one identity.
func naiveUsername(c Data) string {
	return "veloray-naive-" + base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("veloray:%d:%s", num(c, "id"), str(c, "name"))))
}
func canonicalNaiveUsername(name string) string {
	prefix := "veloray-naive-"
	if strings.HasPrefix(name, "veloray-native-") {
		prefix = "veloray-native-"
	}
	if !strings.HasPrefix(name, prefix) || len(name) > 1024 {
		return name
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(name, prefix))
	if err != nil {
		return name
	}
	parts := strings.SplitN(string(raw), ":", 3)
	if len(parts) != 3 || parts[0] != "veloray" || parts[2] == "" {
		return name
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || id < 1 || strconv.FormatInt(id, 10) != parts[1] {
		return name
	}
	return string(raw)
}
func normalizeNaiveTelemetry(stats Data) {
	// Counter names remain exactly as supplied by the durable agent ledger.
	// Decode their identity only after splitting the metric envelope in collect;
	// display names may themselves contain the metric delimiter.
	for index, name := range array(stats, "online_users") {
		if text, ok := name.(string); ok {
			parts := strings.Split(text, ">>>")
			if len(parts) == 3 && parts[0] == "user" && parts[2] == "online" {
				text = canonicalNaiveUsername(parts[1])
			} else {
				text = canonicalNaiveUsername(text)
			}
			array(stats, "online_users")[index] = text
		}
	}
	ips := obj(stats, "online_ips")
	for name, value := range ips {
		canonical := canonicalNaiveUsername(name)
		if canonical != name {
			ips[canonical] = value
			delete(ips, name)
		}
	}
}

func validateNativeProtocol(d Data) error {
	p := str(d, "protocol")
	if !oneOf(p, "tuic", "anytls", "naive", "snell") {
		return nil
	}
	if str(d, "core") != "singbox" || str(d, "transport") != "raw" {
		return errors.New("TUIC, AnyTLS, Naive and Snell require sing-box and RAW transport")
	}
	if oneOf(p, "tuic", "anytls", "naive") && str(d, "security") != "tls" {
		return errors.New("TUIC, AnyTLS and Naive require TLS")
	}
	if p == "snell" && str(d, "security") != "none" {
		return errors.New("Snell v6 uses its own encryption; stream security must be none")
	}
	ps := obj(d, "protocol_settings")
	if p == "tuic" {
		if !oneOf(fallback(str(ps, "congestion_control"), "bbr"), "bbr", "cubic", "new_reno") {
			return errors.New("invalid TUIC congestion control")
		}
		if ps["zero_rtt_handshake"] != nil {
			if _, ok := ps["zero_rtt_handshake"].(bool); !ok {
				return errors.New("zero_rtt_handshake must be boolean")
			}
		}
	}
	if p == "snell" {
		if str(ps, "psk") == "" && num(d, "id") == 0 {
			ps["psk"] = randomToken(32)
		}
		if len(str(ps, "psk")) < 16 || len(str(ps, "psk")) > 255 || strings.ContainsAny(str(ps, "psk"), "\r\n") {
			return errors.New("Snell requires a 16–255 character PSK")
		}
		if !oneOf(fallback(str(ps, "mode"), "default"), "default", "unshaped", "unsafe-raw") {
			return errors.New("invalid Snell v6 mode")
		}
	}
	d["protocol_settings"] = ps
	return nil
}
func nativeSingInbound(v, i Data, accounts []any) bool {
	p := str(i, "protocol")
	ps := obj(i, "protocol_settings")
	switch p {
	case "tuic":
		v["congestion_control"] = fallback(str(ps, "congestion_control"), "bbr")
		v["zero_rtt_handshake"] = flag(ps, "zero_rtt_handshake")
		v["auth_timeout"] = "3s"
		v["heartbeat"] = "10s"
	case "anytls":
		// The core supplies its safe default padding scheme.
	case "naive":
		v["network"] = "tcp"
	case "snell":
		v["version"] = 6
		v["psk"] = str(ps, "psk")
		v["mode"] = fallback(str(ps, "mode"), "default")
	default:
		return true
	}
	// Some protocols treat an empty user list as shared or unauthenticated access.
	return len(accounts) > 0
}
