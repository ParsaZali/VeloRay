package panel

import (
	"errors"
	"strings"
)

func validateNativeProtocol(d Data) error {
	p := str(d, "protocol")
	if !oneOf(p, "tuic", "naive", "snell") {
		return nil
	}
	if str(d, "core") != "singbox" || str(d, "transport") != "raw" {
		return errors.New("TUIC, Naive and Snell require sing-box and RAW transport")
	}
	if oneOf(p, "tuic", "naive") && str(d, "security") != "tls" {
		return errors.New("TUIC and Naive require TLS")
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
