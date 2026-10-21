package panel

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
)

const shadowTLSMethod = "2022-blake3-aes-128-gcm"

func shadowTLSKey() string {
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(key)
}

func validShadowTLSKey(key string) bool {
	raw, err := base64.StdEncoding.Strict().DecodeString(key)
	return err == nil && len(raw) == 16 && base64.StdEncoding.EncodeToString(raw) == key
}

func validateShadowTLS(d Data) error {
	if str(d, "protocol") != "shadowtls" {
		return nil
	}
	if str(d, "core") != "singbox" || str(d, "transport") != "raw" || str(d, "security") != "none" {
		return fmt.Errorf("ShadowTLS v3 requires sing-box, RAW and no additional stream security")
	}
	ps := obj(d, "protocol_settings")
	if !validHost(str(ps, "handshake_server")) || strings.ContainsAny(str(ps, "handshake_server"), ":/\\") {
		return fmt.Errorf("ShadowTLS requires a TLS 1.3 handshake server hostname")
	}
	if ps["handshake_port"] == nil {
		ps["handshake_port"] = 443
	}
	if num(ps, "handshake_port") < 1 || num(ps, "handshake_port") > 65535 {
		return fmt.Errorf("invalid ShadowTLS handshake port")
	}
	if str(ps, "psk") == "" && num(d, "id") == 0 {
		ps["psk"] = shadowTLSKey()
	}
	if !validShadowTLSKey(str(ps, "psk")) {
		return fmt.Errorf("ShadowTLS inner server key must be a base64-encoded 16-byte key")
	}
	d["protocol_settings"] = ps
	return nil
}

// Keep the managed inbound tag on the final authenticated layer, so existing
// routing references and per-user accounting see the original panel identity.
func shadowTLSInbounds(i Data, accounts []any, clients []Data) []any {
	if len(accounts) == 0 {
		return nil
	}
	tag := fmt.Sprintf("in-%d-%s", num(i, "id"), str(i, "name"))
	outerUsers := []any{}
	for _, c := range clients {
		outerUsers = append(outerUsers, Data{"name": fmt.Sprintf("veloray:%d:%s", num(c, "id"), str(c, "name")), "password": str(obj(c, "protocol_settings"), "shadowtls_password")})
	}
	ps := obj(i, "protocol_settings")
	return []any{
		Data{"type": "shadowsocks", "tag": tag, "listen": "127.0.0.1", "listen_port": 0, "network": "tcp", "method": shadowTLSMethod, "password": str(ps, "psk"), "users": accounts},
		Data{"type": "shadowtls", "tag": fmt.Sprintf("veloray-shadowtls-%d", num(i, "id")), "listen": str(i, "listen"), "listen_port": num(i, "port"), "version": 3, "users": outerUsers, "handshake": Data{"server": str(ps, "handshake_server"), "server_port": num(ps, "handshake_port")}, "strict_mode": true, "wildcard_sni": "off", "detour": tag},
	}
}

func shadowTLSOutbound(c, i, n Data) (Data, Data) {
	name := connectionName(c, i)
	// The helper namespace cannot collide with display names (which start with
	// "VeloRay · "). Include the delivery host, port and name for host variants.
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%s:%d:%s", num(c, "id"), num(i, "id"), str(n, "public_host"), num(i, "port"), name)))
	helperTag := fmt.Sprintf("veloray-shadowtls-%x", digest[:16])
	ps := obj(i, "protocol_settings")
	inner := Data{"type": "shadowsocks", "tag": name, "server": str(n, "public_host"), "server_port": num(i, "port"), "method": shadowTLSMethod, "password": str(ps, "psk") + ":" + str(c, "credential"), "network": "tcp", "detour": helperTag}
	outer := Data{"type": "shadowtls", "tag": helperTag, "server": str(n, "public_host"), "server_port": num(i, "port"), "version": 3, "password": str(obj(c, "protocol_settings"), "shadowtls_password"), "tls": Data{"enabled": true, "server_name": str(ps, "handshake_server"), "min_version": "1.3"}}
	return inner, outer
}

// Some protocols need a graph rather than a single outbound. Compatibility
// checks still use singClientOutbound; delivery must include every helper.
func singClientOutbounds(c, i, n Data) ([]any, error) {
	out, err := singClientOutbound(c, i, n)
	if err != nil {
		return nil, err
	}
	if str(i, "protocol") == "shadowtls" {
		_, helper := shadowTLSOutbound(c, i, n)
		return []any{out, helper}, nil
	}
	return []any{out}, nil
}
