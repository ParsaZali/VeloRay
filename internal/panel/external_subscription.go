package panel

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

type subscriptionConnection struct{ client, inbound, node Data }

func decodeFeed64(raw string) ([]byte, error) {
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, e := enc.DecodeString(strings.TrimSpace(raw)); e == nil {
			return b, nil
		}
	}
	return nil, errors.New("invalid base64")
}

// External URI input is normalized into the same export model as local entries.
// It is never deployed to an agent, executed, or used as a server listener.
func externalConnection(raw string) (subscriptionConnection, error) {
	bad := func() (subscriptionConnection, error) {
		return subscriptionConnection{}, errors.New("unsupported or malformed external connection")
	}
	if len(raw) > 16384 || strings.ContainsAny(raw, "\r\n\x00") {
		return bad()
	}
	u, e := url.Parse(raw)
	if e != nil {
		return bad()
	}
	p := strings.ToLower(u.Scheme)
	if p == "hy2" {
		p = "hysteria2"
	}
	if p == "ss" {
		p = "shadowsocks"
	}
	c := defaults("clients")
	c["name"] = "External"
	c["traffic_multiplier"] = 1
	i := defaults("inbounds")
	i["name"] = "External"
	i["protocol"] = p
	i["transport"] = "raw"
	i["security"] = "none"
	n := Data{"public_host": u.Hostname()}
	if u.Fragment != "" {
		i["name"] = u.Fragment
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		port = 443
	}
	i["port"] = port
	if p == "vmess" {
		b, err := decodeFeed64(strings.TrimPrefix(raw, "vmess://"))
		if err != nil {
			return bad()
		}
		v, err := decode(b)
		if err != nil {
			return bad()
		}
		n["public_host"] = str(v, "add")
		i["port"] = num(v, "port")
		c["credential"] = str(v, "id")
		i["name"] = fallback(str(v, "ps"), "External")
		i["transport"] = fallback(str(v, "net"), "raw")
		i["security"] = fallback(str(v, "tls"), "none")
		i["tls_server_name"] = str(v, "sni")
		i["path"] = str(v, "path")
		i["host_header"] = str(v, "host")
		i["service_name"] = str(v, "path")
	} else {
		if u.User == nil {
			return bad()
		}
		c["credential"] = u.User.Username()
		q := u.Query()
		i["transport"] = fallback(q.Get("type"), "raw")
		i["security"] = fallback(q.Get("security"), "none")
		i["tls_server_name"] = q.Get("sni")
		i["path"] = q.Get("path")
		i["host_header"] = q.Get("host")
		i["service_name"] = q.Get("serviceName")
		i["flow"] = q.Get("flow")
		if str(i, "security") == "reality" {
			i["reality_server_name"] = q.Get("sni")
			i["reality_public_key"] = q.Get("pbk")
			i["reality_short_id"] = q.Get("sid")
		}
		switch p {
		case "trojan":
			if q.Get("security") == "" {
				i["security"] = "tls"
			}
		case "hysteria2":
			i["protocol"] = "hysteria"
			i["transport"] = "hysteria"
			i["security"] = "tls"
		case "tuic":
			password, ok := u.User.Password()
			if !ok || password == "" {
				return bad()
			}
			c["protocol_settings"] = Data{"password": password}
			i["protocol_settings"] = Data{"congestion_control": fallback(q.Get("congestion_control"), "bbr")}
			i["security"] = "tls"
		case "shadowsocks":
			if q.Get("plugin") != "" {
				return bad()
			}
			method, password, ok := strings.Cut(u.User.String(), ":")
			if !ok {
				b, err := decodeFeed64(u.User.Username())
				if err != nil {
					return bad()
				}
				method, password, ok = strings.Cut(string(b), ":")
			} else {
				method = u.User.Username()
				password, _ = u.User.Password()
			}
			if !ok || password == "" || !oneOf(method, "aes-128-gcm", "aes-256-gcm", "chacha20-ietf-poly1305", "2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm", "2022-blake3-chacha20-poly1305") {
				return bad()
			}
			i["protocol_settings"] = Data{"method": method}
			c["credential"] = password
		case "vless":
		default:
			return bad()
		}
	}
	switch str(i, "transport") {
	case "tcp", "":
		i["transport"] = "raw"
	case "kcp":
		i["transport"] = "mkcp"
	}
	if !validHost(str(n, "public_host")) || num(i, "port") < 1 || num(i, "port") > 65535 || str(c, "credential") == "" || len(str(i, "name")) > 256 || !utf8.ValidString(str(i, "name")) {
		return bad()
	}
	if oneOf(p, "vless", "vmess", "tuic") && !uuidPattern.MatchString(str(c, "credential")) {
		return bad()
	}
	if !oneOf(str(i, "security"), "none", "tls", "reality") || !oneOf(str(i, "transport"), "raw", "ws", "grpc", "xhttp", "httpupgrade", "mkcp", "hysteria") {
		return bad()
	}
	if str(i, "security") == "reality" && (str(i, "reality_public_key") == "" || str(i, "reality_server_name") == "") {
		return bad()
	}
	// Disallow silent loss of client security/transport options we cannot represent.
	for _, k := range []string{"allowInsecure", "insecure", "obfs", "obfs-password", "spx", "encryption", "alpn"} {
		if v := u.Query().Get(k); v != "" && v != "0" && v != "false" && !(k == "encryption" && v == "none") {
			return bad()
		}
	}
	return subscriptionConnection{c, i, n}, nil
}

func parseExternalLinks(raw string) ([]any, error) {
	if len(raw) > 1<<20 {
		return nil, errors.New("feed exceeds 1 MiB")
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []any{}, nil
	}
	if !strings.Contains(raw, "://") {
		b, e := decodeFeed64(raw)
		if e != nil {
			return nil, errors.New("feed must contain portable URIs or base64 URIs")
		}
		raw = string(b)
	}
	if !utf8.ValidString(raw) {
		return nil, errors.New("feed must be UTF-8")
	}
	links := []any{}
	seen := map[string]bool{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if _, e := externalConnection(line); e != nil {
			return nil, e
		}
		if !seen[line] {
			links = append(links, line)
			seen[line] = true
		}
		if len(links) > 200 {
			return nil, errors.New("at most 200 external connections per policy")
		}
	}
	return links, nil
}
func policyBodyDefaults() Data {
	return Data{"url": "", "links": []any{}, "apps": []any{}, "default_format": "auto", "templates": Data{}}
}
func (s *Server) externalPolicyBody(d Data) (Data, error) {
	b, e := unseal(s.Config.FieldKey, str(d, "_body"))
	if e != nil {
		return nil, e
	}
	return decode([]byte(b))
}
func validateExternalPolicyBody(body Data) error {
	if e := allowed(body, "url links apps default_format templates"); e != nil {
		return e
	}
	if e := typed(body, policyBodyDefaults()); e != nil {
		return e
	}
	if e := validateWebhookURL(str(body, "url")); e != nil {
		return errors.New("feed URL must be public HTTPS without credentials or a fragment")
	}
	if !oneOf(str(body, "default_format"), "auto", "base64", "raw", "clash", "singbox", "xray") {
		return errors.New("invalid default format")
	}
	if len(array(body, "links")) > 200 || len(array(body, "apps")) > 12 {
		return errors.New("too many links or application rules")
	}
	for _, v := range array(body, "links") {
		link, ok := v.(string)
		if !ok {
			return errors.New("links must be URI strings")
		}
		if _, e := externalConnection(link); e != nil {
			return e
		}
	}
	for _, v := range array(body, "apps") {
		app, ok := v.(string)
		if !ok || !oneOf(app, "sing-box", "singbox", "mihomo", "clash", "stash", "hiddify", "v2rayng", "v2rayn", "browser", "other") {
			return errors.New("unknown application family")
		}
	}
	if e := allowed(obj(body, "templates"), "clash singbox xray"); e != nil {
		return e
	}
	for _, v := range obj(body, "templates") {
		raw, ok := v.(string)
		if !ok {
			return errors.New("templates must be JSON text")
		}
		if e := validateFeedTemplate(raw); e != nil {
			return e
		}
	}
	b, _ := json.Marshal(body)
	if len(b) > 512<<10 {
		return errors.New("policy exceeds 512 KiB")
	}
	return nil
}
func (s *Server) subscriptionPoliciesAPI(w http.ResponseWriter, r *http.Request, path string, a Actor) (any, int, error) {
	ctx := r.Context()
	parts := strings.Split(path, "/")
	id := int64(0)
	if len(parts) > 3 {
		return nil, 0, fail(404, "endpoint not found")
	}
	if len(parts) > 1 {
		id, _ = strconv.ParseInt(parts[1], 10, 64)
		if id < 1 {
			return nil, 0, fail(404, "policy not found")
		}
	}
	view := func(d Data, detail bool) (Data, error) {
		v := public(d)
		if detail {
			b, e := s.externalPolicyBody(d)
			if e != nil {
				return nil, e
			}
			v["body"] = b
		}
		return v, nil
	}
	if len(parts) == 3 {
		if parts[2] != "refresh" || r.Method != "POST" {
			return nil, 0, fail(405, "method not allowed")
		}
		return s.refreshExternalPolicy(r, id, a)
	}
	if r.Method == "GET" {
		if id > 0 {
			d, e := get(ctx, s.Store.Pool, "subscription_policies", id)
			if e != nil {
				return nil, 0, e
			}
			v, e := view(d, true)
			return v, 200, e
		}
		rows, e := list(ctx, s.Store.Pool, "subscription_policies", "ORDER BY id LIMIT 100")
		if e != nil {
			return nil, 0, e
		}
		out := []Data{}
		for _, d := range rows {
			v, e := view(d, false)
			if e != nil {
				return nil, 0, e
			}
			out = append(out, v)
		}
		return out, 200, nil
	}
	if !(r.Method == "POST" && id == 0 || oneOf(r.Method, "PATCH", "DELETE") && id > 0) {
		return nil, 0, fail(405, "method not allowed")
	}
	input := Data{}
	var e error
	if r.Method != "DELETE" {
		input, e = readBody(w, r)
		if e != nil {
			return nil, 0, e
		}
		if e = allowed(input, "name enabled customer_ids service_ids body"); e != nil {
			return nil, 0, fail(400, e.Error())
		}
		if e = typed(input, Data{"name": "", "enabled": true, "customer_ids": []any{}, "service_ids": []any{}, "body": Data{}}); e != nil {
			return nil, 0, fail(400, e.Error())
		}
	}
	out, e := s.managementChange(ctx, nil, a, "subscription-policy."+strings.ToLower(r.Method), fmt.Sprint(id), func(tx pgx.Tx) (any, error) {
		d := Data{"name": "", "enabled": true, "customer_ids": []any{}, "service_ids": []any{}, "cache_count": 0}
		body := policyBodyDefaults()
		if id > 0 {
			var e error
			d, e = get(ctx, tx, "subscription_policies", id)
			if e != nil {
				return nil, e
			}
			body, e = s.externalPolicyBody(d)
			if e != nil {
				return nil, e
			}
		}
		if r.Method == "DELETE" {
			return nil, remove(ctx, tx, "subscription_policies", id)
		}
		if b, ok := input["body"]; ok {
			body = merge(policyBodyDefaults(), asData(b))
		}
		oldURL := ""
		if id > 0 {
			old, _ := s.externalPolicyBody(d)
			oldURL = str(old, "url")
		}
		d = merge(d, input)
		delete(d, "body")
		if e := require(d, "name", 96); e != nil {
			return nil, fail(400, e.Error())
		}
		if e := validateExternalPolicyBody(body); e != nil {
			return nil, fail(400, e.Error())
		}
		for key, kind := range map[string]string{"customer_ids": "customers", "service_ids": "services"} {
			ids, e := idArray(d, key, 200)
			if e != nil {
				return nil, e
			}
			for _, i := range ids {
				if _, e = get(ctx, tx, kind, i); e != nil {
					return nil, fail(400, "assigned account or service does not exist")
				}
			}
		}
		if id == 0 {
			var count int
			if e := tx.QueryRow(ctx, "SELECT count(*) FROM vr_subscription_policies").Scan(&count); e != nil {
				return nil, e
			}
			if count >= 100 {
				return nil, fail(400, "at most 100 subscription policies")
			}
		}
		sealed, e := seal(s.Config.FieldKey, string(mustJSON(body)))
		if e != nil {
			return nil, e
		}
		d["_body"] = sealed
		if oldURL != str(body, "url") {
			d["_cache"] = ""
			d["cache_count"] = 0
			d["cached_at"] = nil
		}
		if e = save(ctx, tx, "subscription_policies", d); e != nil {
			return nil, e
		}
		return view(d, true)
	})
	status := 200
	if r.Method == "POST" {
		status = 201
	}
	if r.Method == "DELETE" {
		status = 204
	}
	return out, status, e
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
func (s *Server) refreshExternalPolicy(r *http.Request, id int64, a Actor) (any, int, error) {
	ctx := r.Context()
	d, e := get(ctx, s.Store.Pool, "subscription_policies", id)
	if e != nil {
		return nil, 0, e
	}
	body, e := s.externalPolicyBody(d)
	if e != nil {
		return nil, 0, e
	}
	target := str(body, "url")
	if target == "" {
		return nil, 0, fail(400, "policy has no remote URL")
	}
	if e = validateWebhookURL(target); e != nil {
		return nil, 0, fail(400, "invalid remote feed URL")
	}
	req, e := http.NewRequestWithContext(ctx, "GET", target, nil)
	if e != nil {
		return nil, 0, fail(400, "invalid remote feed URL")
	}
	req.Header.Set("User-Agent", "VeloRay-Feed/1.0")
	client := s.ExternalHTTP
	if client == nil {
		client = webhookHTTPClient()
	}
	resp, e := client.Do(req)
	if e != nil {
		return nil, 0, fail(502, "remote feed unavailable; previous cache retained")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, 0, fail(502, "remote feed did not return HTTP 200; previous cache retained")
	}
	raw, e := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if e != nil {
		return nil, 0, fail(502, "remote feed could not be read")
	}
	links, e := parseExternalLinks(string(raw))
	if e != nil {
		return nil, 0, fail(400, e.Error())
	}
	out, e := s.managementChange(ctx, nil, a, "subscription-policy.refresh", fmt.Sprint(id), func(tx pgx.Tx) (any, error) {
		fresh, e := get(ctx, tx, "subscription_policies", id)
		if e != nil {
			return nil, e
		}
		if str(fresh, "updated_at") != str(d, "updated_at") {
			return nil, fail(409, "policy changed; refresh again")
		}
		cache, e := seal(s.Config.FieldKey, string(mustJSON(Data{"links": links})))
		if e != nil {
			return nil, e
		}
		fresh["_cache"], fresh["cache_count"], fresh["cached_at"] = cache, len(links), stamp()
		if e = save(ctx, tx, "subscription_policies", fresh); e != nil {
			return nil, e
		}
		return public(fresh), nil
	})
	return out, 200, e
}
func (s *Server) externalSubscription(ctx context.Context, c, cfg Data, r *http.Request) ([]subscriptionConnection, Data, error) {
	out := []subscriptionConnection{}
	cfg = clone(cfg)
	if num(c, "_account") == 0 {
		return out, cfg, nil
	}
	account, e := get(ctx, s.Store.Pool, "customers", num(c, "_account"))
	if e != nil {
		return nil, nil, e
	}
	policies, e := list(ctx, s.Store.Pool, "subscription_policies", "WHERE data->>'enabled'='true' ORDER BY id LIMIT 100")
	if e != nil {
		return nil, nil, e
	}
	for _, p := range policies {
		matches := false
		for _, v := range array(p, "customer_ids") {
			matches = matches || num(Data{"id": v}, "id") == num(account, "id")
		}
		for _, v := range array(p, "service_ids") {
			for _, id := range array(account, "service_ids") {
				matches = matches || num(Data{"id": v}, "id") == num(Data{"id": id}, "id")
			}
		}
		if !matches {
			continue
		}
		body, e := s.externalPolicyBody(p)
		if e != nil {
			return nil, nil, e
		}
		apps := array(body, "apps")
		matchApp := len(apps) == 0
		for _, app := range apps {
			matchApp = matchApp || app == subscriptionApp(r)
		}
		if !matchApp {
			continue
		}
		if str(body, "default_format") != "auto" {
			cfg["subscription_default_format"] = str(body, "default_format")
		}
		for k, v := range obj(body, "templates") {
			cfg["subscription_"+k+"_template"] = v
		}
		links := append([]any{}, array(body, "links")...)
		if str(p, "_cache") != "" {
			raw, e := unseal(s.Config.FieldKey, str(p, "_cache"))
			if e != nil {
				return nil, nil, e
			}
			cached, e := decode([]byte(raw))
			if e != nil {
				return nil, nil, e
			}
			links = append(links, array(cached, "links")...)
		}
		seen := map[string]bool{}
		for _, v := range links {
			raw, _ := v.(string)
			if seen[raw] {
				continue
			}
			seen[raw] = true
			connection, e := externalConnection(raw)
			if e != nil {
				return nil, nil, e
			}
			connection.inbound["name"] = fmt.Sprintf("%s · %d · %s", str(p, "name"), len(out)+1, str(connection.inbound, "name"))
			out = append(out, connection)
			if len(out) > 300 {
				return nil, nil, errors.New("too many external connections assigned to account")
			}
		}
	}
	return out, cfg, nil
}
