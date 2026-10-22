package panel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"strings"
	"time"
)

// A domain-separated key allows a user to rotate the ordinary panel credential
// without exposing it in a Telegram link. FakeTLS's hostname is part of ee secrets.
func nativeUsername(c Data) string {
	return strings.Replace(naiveUsername(c), "veloray-naive-", "veloray-native-", 1)
}

func mtprotoSecret(client, inbound Data) string {
	key := sha256.Sum256([]byte("VeloRay/MTProto/v1/" + str(client, "credential")))
	return "ee" + hex.EncodeToString(key[:16]) + hex.EncodeToString([]byte(str(obj(inbound, "protocol_settings"), "fake_tls_host")))
}

func buildNativeConfig(ctx context.Context, q Query, node Data) (Data, error) {
	inbounds, err := list(ctx, q, "inbounds", "WHERE node_id=$1", num(node, "id"))
	if err != nil {
		return nil, err
	}
	result := []any{}
	now := time.Now().UTC()
	for _, in := range inbounds {
		if str(in, "core") != "native" || !flag(in, "enabled") || !flag(node, "enabled") {
			continue
		}
		clients, err := list(ctx, q, "clients", "WHERE inbound_id=$1", num(in, "id"))
		if err != nil {
			return nil, err
		}
		users := []any{}
		for _, c := range clients {
			if !active(c, now) {
				continue
			}
			denied := []any{}
			for ip, at := range obj(c, "_device_denied") {
				if net.ParseIP(ip) != nil && now.Unix()-num(Data{"at": at}, "at") <= 600 {
					denied = append(denied, ip)
				}
			}
			u := Data{"name": nativeUsername(c), "secret": mtprotoSecret(c, in), "ip_limit": num(c, "device_limit"), "denied_ips": denied}
			if str(in, "protocol") == "amneziawg" {
				ps := obj(c, "protocol_settings")
				u = Data{"name": nativeUsername(c), "public_key": wireHex(str(ps, "public_key")), "preshared_key": wireHex(str(ps, "pre_shared_key")), "address": str(ps, "address")}
			}
			if value := str(c, "expires_at"); value != "" {
				u["expires_at"] = value
			}
			users = append(users, u)
		}
		entry := Data{"tag": fmt.Sprintf("in-%d-%s", num(in, "id"), str(in, "name")), "protocol": str(in, "protocol"), "listen": str(in, "listen"), "port": num(in, "port"), "users": users}
		if str(in, "protocol") == "amneziawg" {
			params, err := panelAWGSettings(obj(in, "protocol_settings"))
			if err != nil {
				return nil, err
			}
			entry["awg"] = params
		}
		result = append(result, entry)
	}
	return Data{"inbounds": result}, nil
}
