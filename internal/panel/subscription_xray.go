package panel

import (
	"fmt"
)

func xrayClientConfig(c, i, n Data) (Data, error) {
	p := str(i, "protocol")
	if !oneOf(p, "vless", "vmess", "trojan", "shadowsocks") || !oneOf(str(i, "transport"), "raw", "ws", "grpc", "xhttp", "httpupgrade", "mkcp") {
		return nil, fmt.Errorf("Xray JSON unavailable for %s/%s", p, str(i, "transport"))
	}
	server := Data{"address": str(n, "public_host"), "port": num(i, "port")}
	settings := Data{}
	switch p {
	case "vless", "vmess":
		user := Data{"id": str(c, "credential")}
		if p == "vless" {
			user["encryption"] = "none"
			if f := clientFlow(c, i); f != "" {
				user["flow"] = f
			}
		} else {
			user["security"] = "auto"
			user["alterId"] = 0
		}
		server["users"] = []any{user}
		settings["vnext"] = []any{server}
	case "trojan":
		server["password"] = str(c, "credential")
		settings["servers"] = []any{server}
	case "shadowsocks":
		server["password"] = ssClientPassword(c, i)
		server["method"] = fallback(str(obj(i, "protocol_settings"), "method"), "aes-256-gcm")
		settings["servers"] = []any{server}
	}
	outbound := Data{"tag": "proxy", "protocol": p, "settings": settings}
	if p != "shadowsocks" {
		stream := streamSettings(i, n)
		// Never export server certificates, private keys, socket options or handshake
		// targets. A client needs only the public TLS/REALITY handshake parameters.
		delete(stream, "tlsSettings")
		delete(stream, "realitySettings")
		delete(stream, "sockopt")
		if str(i, "security") == "tls" {
			stream["tlsSettings"] = Data{"serverName": fallback(str(i, "tls_server_name"), str(n, "public_host")), "fingerprint": "chrome"}
		}
		if str(i, "security") == "reality" {
			stream["realitySettings"] = Data{"serverName": str(i, "reality_server_name"), "password": str(i, "reality_public_key"), "shortId": str(i, "reality_short_id"), "fingerprint": "chrome"}
		}
		outbound["streamSettings"] = stream
	}
	return Data{"remarks": connectionName(c, i), "log": Data{"loglevel": "warning"}, "outbounds": []any{outbound, Data{"tag": "direct", "protocol": "freedom"}, Data{"tag": "blocked", "protocol": "blackhole"}}}, nil
}
