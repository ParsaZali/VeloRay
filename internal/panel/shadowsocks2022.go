package panel

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

func shadowsocks2022(i Data) bool {
	return str(i, "protocol") == "shadowsocks" && oneOf(str(obj(i, "protocol_settings"), "method"), "2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm")
}
func ssKeyBytes(i Data) int {
	if str(obj(i, "protocol_settings"), "method") == "2022-blake3-aes-128-gcm" {
		return 16
	}
	return 32
}
func ssKey(size int) string {
	raw := make([]byte, size)
	if _, e := rand.Read(raw); e != nil {
		panic(e)
	}
	return base64.StdEncoding.EncodeToString(raw)
}
func validSSKey(value string, size int) bool {
	raw, e := base64.StdEncoding.Strict().DecodeString(value)
	return e == nil && len(raw) == size && base64.StdEncoding.EncodeToString(raw) == value
}
func ssClientPassword(c, i Data) string {
	if shadowsocks2022(i) {
		return str(obj(i, "protocol_settings"), "password") + ":" + str(c, "credential")
	}
	return str(c, "credential")
}
func validateShadowsocks2022(i Data) error {
	if !shadowsocks2022(i) {
		return nil
	}
	ps := obj(i, "protocol_settings")
	if str(ps, "password") == "" {
		ps["password"] = ssKey(ssKeyBytes(i))
	}
	if !validSSKey(str(ps, "password"), ssKeyBytes(i)) {
		return fmt.Errorf("Shadowsocks 2022 server key must be base64-encoded %d bytes", ssKeyBytes(i))
	}
	i["protocol_settings"] = ps
	return nil
}
