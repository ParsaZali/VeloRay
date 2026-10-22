package panel

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/netip"
	"strings"

	"github.com/PaarSaAm/VeloRay/internal/nativecore"
	"golang.org/x/crypto/curve25519"
)

func wireBytes(value string) []byte {
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		raw, _ = base64.RawURLEncoding.DecodeString(value)
	}
	return raw
}
func wireHex(value string) string { return hex.EncodeToString(wireBytes(value)) }
func panelAWGSettings(ps Data) (*nativecore.AWGSettings, error) {
	values := Data{"private_key": wireHex(str(ps, "secret_key")), "mtu": 1280, "jc": 4, "jmin": 40, "jmax": 70, "s1": 32, "s2": 64, "s3": 8, "s4": 16, "h1": "123456789", "h2": "234567890", "h3": "345678901", "h4": "456789012"}
	for key := range values {
		if key != "private_key" {
			if value, ok := ps[key]; ok {
				values[key] = value
			}
		}
	}
	raw, _ := json.Marshal(values)
	var settings nativecore.AWGSettings
	if err := json.Unmarshal(raw, &settings); err != nil {
		return nil, errors.New("invalid AmneziaWG parameter type")
	}
	return &settings, nil
}
func wirePairValid(private, public string) bool {
	if !validKey(private) || !validKey(public) {
		return false
	}
	computed, err := curve25519.X25519(wireBytes(private), curve25519.Basepoint)
	return err == nil && bytes.Equal(computed, wireBytes(public))
}
func validatePanelAWG(in Data) error {
	ps := obj(in, "protocol_settings")
	if err := allowed(ps, "secret_key public_key mtu dns jc jmin jmax s1 s2 s3 s4 h1 h2 h3 h4"); err != nil {
		return err
	}
	if !wirePairValid(str(ps, "secret_key"), str(ps, "public_key")) {
		return errors.New("AmneziaWG server keypair does not match")
	}
	for _, address := range strings.Split(fallback(str(ps, "dns"), "1.1.1.1"), ",") {
		if net.ParseIP(strings.TrimSpace(address)) == nil {
			return errors.New("AmneziaWG DNS must contain IP addresses")
		}
	}
	settings, err := panelAWGSettings(ps)
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(nativecore.Config{Inbounds: []nativecore.Inbound{{Tag: "validate", Listen: str(in, "listen"), Port: int(num(in, "port")), Protocol: "amneziawg", AWG: settings}}})
	_, err = nativecore.Decode(raw)
	return err
}
func validateAWGClient(c Data) error {
	ps := obj(c, "protocol_settings")
	if !wirePairValid(str(ps, "private_key"), str(ps, "public_key")) {
		return errors.New("AmneziaWG client keypair does not match")
	}
	if value := str(ps, "pre_shared_key"); value != "" && !validKey(value) {
		return errors.New("invalid AmneziaWG preshared key")
	}
	prefix, err := netip.ParsePrefix(str(ps, "address"))
	if err != nil || !prefix.Addr().IsPrivate() || prefix.Bits() != prefix.Addr().BitLen() {
		return errors.New("AmneziaWG address must be a private /32 or /128")
	}
	if num(ps, "keepalive") < 0 || num(ps, "keepalive") > 65535 {
		return errors.New("invalid AmneziaWG keepalive")
	}
	return nil
}
