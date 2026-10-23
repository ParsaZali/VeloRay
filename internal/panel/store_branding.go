package panel

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/net/idna"
)

func normalizeStoreOrigin(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" {
		return "", fail(400, "subscription origin must be an HTTPS domain origin")
	}
	host, e := idna.Lookup.ToASCII(strings.ToLower(u.Hostname()))
	if e != nil || !validHost(host) || net.ParseIP(host) != nil || !strings.Contains(host, ".") {
		return "", fail(400, "subscription origin requires a DNS domain")
	}
	port := u.Port()
	if port != "" {
		n, e := strconv.Atoi(port)
		if e != nil || n < 1 || n > 65535 {
			return "", fail(400, "invalid subscription origin port")
		}
		host = net.JoinHostPort(host, strconv.Itoa(n))
	}
	return "https://" + host, nil
}
func storeBrandView(d Data) Data {
	out := public(d)
	origin, _ := url.Parse(str(d, "subscription_origin"))
	if origin != nil && origin.Host != "" && str(d, "_domain_proof") != "" {
		out["domain_record"] = Data{"name": "_veloray." + origin.Hostname(), "value": "veloray-site-verification=" + str(d, "_domain_proof"), "verified": flag(d, "subscription_verified")}
	}
	return out
}
func (s *Server) verifyStoreDomain(w http.ResponseWriter, r *http.Request, a Actor, owner int64) (any, int, error) {
	if r.Method != "POST" {
		return nil, 0, fail(405, "method not allowed")
	}
	if !permitted(a.User, "commerce.write") {
		return nil, 0, fail(403, "commerce write access required")
	}
	ctx := r.Context()
	input, e := readBody(w, r)
	if e != nil {
		return nil, 0, e
	}
	if e = allowed(input, "origin"); e != nil {
		return nil, 0, fail(400, e.Error())
	}
	if e := rateLimit(ctx, s.Store.Pool, "store:dns:"+fmt.Sprint(owner), 20, time.Hour); e != nil {
		return nil, 0, e
	}
	d, e := getStore(ctx, s.Store.Pool, owner)
	if e != nil {
		return nil, 0, e
	}
	if proposed, ok := input["origin"]; ok {
		value, yes := proposed.(string)
		if !yes {
			return nil, 0, fail(400, "origin must be text")
		}
		normalized, e := normalizeStoreOrigin(value)
		if e != nil {
			return nil, 0, e
		}
		if normalized != str(d, "subscription_origin") {
			return nil, 0, fail(409, "save changed store settings before verifying DNS")
		}
	}
	origin, e := normalizeStoreOrigin(str(d, "subscription_origin"))
	if e != nil {
		return nil, 0, e
	}
	if origin == "" || str(d, "_domain_proof") == "" {
		return nil, 0, fail(409, "save a subscription origin before verifying DNS")
	}
	parsed, _ := url.Parse(origin)
	lookup := s.LookupTXT
	if lookup == nil {
		lookup = net.DefaultResolver.LookupTXT
	}
	query, cancel := context.WithTimeout(ctx, 5*time.Second)
	records, e := lookup(query, "_veloray."+parsed.Hostname())
	cancel()
	if e != nil {
		return nil, 0, fail(409, "DNS verification record is unavailable")
	}
	verified := false
	for _, record := range records {
		if record == "veloray-site-verification="+str(d, "_domain_proof") {
			verified = true
		}
	}
	if !verified {
		return nil, 0, fail(409, "DNS verification record does not match this store")
	}
	if e = s.checkStoreOrigin(ctx, origin); e != nil {
		return nil, 0, e
	}
	out, e := s.managementChange(ctx, nil, a, "store.domain_verified", fmt.Sprint(owner), func(tx pgx.Tx) (any, error) {
		fresh, e := getStore(ctx, tx, owner)
		if e != nil {
			return nil, e
		}
		if str(fresh, "subscription_origin") != origin || str(fresh, "_domain_proof") != str(d, "_domain_proof") {
			return nil, fail(409, "store domain changed; verify the new record")
		}
		var existing int64
		e = tx.QueryRow(ctx, `SELECT owner_id FROM vr_store_domains WHERE origin=$1`, origin).Scan(&existing)
		if e == nil && existing != owner {
			return nil, fail(409, "this subscription origin belongs to another store")
		}
		if e != nil && e != pgx.ErrNoRows {
			return nil, e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO vr_store_domains(origin,owner_id) VALUES($1,$2) ON CONFLICT(origin) DO UPDATE SET verified_at=now() WHERE vr_store_domains.owner_id=excluded.owner_id`, origin, owner); e != nil {
			return nil, e
		}
		fresh["subscription_verified"] = true
		fresh["domain_verified_at"] = stamp()
		raw, _ := json.Marshal(fresh)
		if _, e = tx.Exec(ctx, `UPDATE vr_stores SET data=$2 WHERE owner_id=$1`, owner, string(raw)); e != nil {
			return nil, e
		}
		return storeBrandView(fresh), nil
	})
	return out, 200, e
}
func domainInstanceProof(secret, nonce string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("veloray-domain-instance\x00" + nonce))
	return hex.EncodeToString(mac.Sum(nil))
}
func (s *Server) checkStoreOrigin(ctx context.Context, origin string) error {
	nonce := randomToken(24)
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	req, e := http.NewRequestWithContext(ctx, "GET", origin+"/api/health?domain_probe="+nonce, nil)
	if e != nil {
		return fail(400, "invalid subscription origin")
	}
	req.Header.Set("Accept", "application/json")
	client := s.ExternalHTTP
	if client == nil {
		client = webhookHTTPClient()
	}
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, e := copyClient.Do(req)
	if e != nil {
		return fail(409, "subscription domain HTTPS is not ready; configure its certificate and proxy first")
	}
	defer res.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(res.Body, 16385))
	if e != nil || len(raw) > 16384 || res.StatusCode != 200 {
		return fail(409, "subscription domain health check failed")
	}
	d, e := decode(raw)
	if e != nil || str(d, "status") != "ok" || subtle.ConstantTimeCompare([]byte(str(d, "domain_instance")), []byte(domainInstanceProof(s.Config.SecretKey, nonce))) != 1 {
		return fail(409, "subscription domain does not reach this VeloRay instance")
	}
	return nil
}
func (s *Server) brandedSubscriptionSettings(ctx context.Context, q Query, cfg Data, owner int64) (Data, error) {
	if owner <= 0 {
		return cfg, nil
	}
	d, e := getStore(ctx, q, owner)
	if e != nil {
		return nil, e
	}
	if !flag(d, "subscription_branding") {
		return cfg, nil
	}
	out := clone(cfg)
	out["site_name"] = str(d, "name")
	out["support_url"] = str(d, "support_url")
	out["subscription_logo_url"] = str(d, "logo_url")
	if str(d, "subscription_origin") != "" && flag(d, "subscription_verified") {
		var origin string
		e = q.QueryRow(ctx, `SELECT origin FROM vr_store_domains WHERE owner_id=$1 AND origin=$2`, owner, str(d, "subscription_origin")).Scan(&origin)
		if e == nil {
			out["_store_origin"] = origin
		} else if e != pgx.ErrNoRows {
			return nil, e
		}
	}
	return out, nil
}
func subscriptionTokenOwner(ctx context.Context, q Query, token string) (int64, error) {
	var owner int64
	e := q.QueryRow(ctx, `SELECT COALESCE((SELECT (data->>'owner_id')::bigint FROM vr_accounts WHERE data->>'subscription_token'=$1),(SELECT COALESCE((a.data->>'owner_id')::bigint,(c.data->>'owner_id')::bigint,0) FROM vr_clients c LEFT JOIN vr_accounts a ON a.id=c.account_id WHERE c.data->>'subscription_token'=$1 LIMIT 1),0)`, token).Scan(&owner)
	return owner, e
}
