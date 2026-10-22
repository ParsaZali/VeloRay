package nativecore

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

type AWGSettings struct {
	PrivateKey string `json:"private_key"`
	MTU        int    `json:"mtu"`
	Jc         int    `json:"jc"`
	Jmin       int    `json:"jmin"`
	Jmax       int    `json:"jmax"`
	S1         int    `json:"s1"`
	S2         int    `json:"s2"`
	S3         int    `json:"s3"`
	S4         int    `json:"s4"`
	H1         string `json:"h1"`
	H2         string `json:"h2"`
	H3         string `json:"h3"`
	H4         string `json:"h4"`
}

func validHexKey(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value) && len(raw) == 32 && value != strings.Repeat("0", 64)
}
func validateAWG(in Inbound) error {
	s := in.AWG
	if s == nil || !validHexKey(s.PrivateKey) || s.MTU < 1280 || s.MTU > 1420 {
		return errors.New("AmneziaWG requires a valid private key and MTU between 1280 and 1420")
	}
	if s.Jc < 0 || s.Jc > 128 || s.Jmin < 0 || s.Jmax < s.Jmin || s.Jmax > 1280 {
		return errors.New("invalid AmneziaWG junk packet settings")
	}
	if s.S1 < 0 || s.S1 > 1024 || s.S2 < 0 || s.S2 > 1024 || s.S3 < 0 || s.S3 > 1024 || s.S4 < 0 || s.S4 > 64 || s.S1+148 == s.S2+92 {
		return errors.New("invalid or overlapping AmneziaWG padding sizes")
	}
	seen := map[uint64]bool{}
	for _, value := range []string{s.H1, s.H2, s.H3, s.H4} {
		v, err := strconv.ParseUint(value, 10, 32)
		if err != nil || v < 5 || seen[v] {
			return errors.New("AmneziaWG headers must be four distinct integers between 5 and 4294967295")
		}
		seen[v] = true
	}
	keys, addresses := map[string]bool{}, map[netip.Addr]bool{}
	for _, u := range in.Users {
		address, err := netip.ParsePrefix(u.Address)
		if err != nil || address.Bits() != address.Addr().BitLen() || !address.Addr().IsPrivate() || addresses[address.Addr()] {
			return errors.New("AmneziaWG peers require unique private /32 or /128 addresses")
		}
		addresses[address.Addr()] = true
		if !validHexKey(u.PublicKey) || keys[u.PublicKey] || u.PresharedKey != "" && !validHexKey(u.PresharedKey) {
			return errors.New("invalid or duplicate AmneziaWG peer key")
		}
		keys[u.PublicKey] = true
		if u.Secret != "" || u.IPLimit != 0 || len(u.DeniedIPs) > 0 {
			return errors.New("AmneziaWG uses peer keys and single-endpoint roaming, not MTProto secrets or IP caps")
		}
	}
	return nil
}
func awgUAPI(in Inbound, old *Inbound, withPort bool) string {
	s := in.AWG
	lines := []string{"private_key=" + s.PrivateKey}
	if withPort {
		lines = append(lines, fmt.Sprint("listen_port=", in.Port))
	}
	for _, entry := range []struct {
		k string
		v int
	}{{"jc", s.Jc}, {"jmin", s.Jmin}, {"jmax", s.Jmax}, {"s1", s.S1}, {"s2", s.S2}, {"s3", s.S3}, {"s4", s.S4}} {
		lines = append(lines, fmt.Sprintf("%s=%d", entry.k, entry.v))
	}
	lines = append(lines, "h1="+s.H1, "h2="+s.H2, "h3="+s.H3, "h4="+s.H4)
	users := map[string]User{}
	for _, u := range in.Users {
		users[u.PublicKey] = u
	}
	if old != nil {
		for _, u := range old.Users {
			if _, ok := users[u.PublicKey]; !ok {
				lines = append(lines, "public_key="+u.PublicKey, "remove=true")
			}
		}
	}
	for _, u := range in.Users {
		psk := u.PresharedKey
		if psk == "" {
			psk = strings.Repeat("0", 64)
		}
		lines = append(lines, "public_key="+u.PublicKey, "preshared_key="+psk, "replace_allowed_ips=true", "allowed_ip="+u.Address)
	}
	return strings.Join(lines, "\n") + "\n"
}
