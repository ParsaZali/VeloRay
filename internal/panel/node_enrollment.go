package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var certificateFingerprint = regexp.MustCompile(`^[a-f0-9]{64}$`)

func shellLiteral(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
func (s *Server) enrollmentAPI(w http.ResponseWriter, r *http.Request, path string, a Actor) (any, int, error) {
	if !commerceAdmin(a) {
		return nil, 0, fail(403, "administrator access required")
	}
	ctx := r.Context()
	if path == "node-enrollment" && r.Method == "GET" {
		out, e := commerceRows(ctx, s.Store.Pool, `SELECT jsonb_build_object('id',id,'name',data->>'name','public_host',data->>'public_host','expires_at',expires_at,'used_at',used_at,'revoked_at',revoked_at,'node_id',node_id) FROM vr_node_enrollments ORDER BY created_at DESC LIMIT 100`)
		return out, 200, e
	}
	if path == "node-enrollment" && r.Method == "POST" {
		d, e := readBody(w, r)
		if e != nil {
			return nil, 0, e
		}
		if e = allowed(d, "name public_host"); e != nil {
			return nil, 0, fail(400, e.Error())
		}
		if e = require(d, "name", 96); e != nil {
			return nil, 0, fail(400, e.Error())
		}
		if !validHost(str(d, "public_host")) {
			return nil, 0, fail(400, "public_host must be a hostname or IP address")
		}
		if !strings.HasPrefix(s.Config.PublicURL, "https://") {
			return nil, 0, fail(400, "node enrollment requires a public HTTPS panel URL")
		}
		id, token := randomToken(18), randomToken(32)
		expiry := time.Now().Add(15 * time.Minute)
		raw, _ := json.Marshal(d)
		_, e = s.managementChange(ctx, nil, a, "node.enrollment_issued", id, func(tx pgx.Tx) (any, error) {
			_, e := tx.Exec(ctx, `INSERT INTO vr_node_enrollments(id,token_hash,actor_id,data,expires_at) VALUES($1,$2,$3,$4,$5)`, id, digest(token), num(a.User, "id"), string(raw), expiry)
			return nil, e
		})
		if e != nil {
			return nil, 0, e
		}
		endpoint := strings.TrimRight(s.Config.PublicURL, "/") + "/api/node-enrollment/complete"
		command := "VELORAY_ENROLL_URL=" + shellLiteral(endpoint) + " VELORAY_ENROLL_TOKEN=" + shellLiteral(token) + " VELORAY_PUBLIC_HOST=" + shellLiteral(str(d, "public_host")) + " bash <(curl -fsSL https://raw.githubusercontent.com/ParsaZali/VeloRay/main/install.sh) node"
		return Data{"id": id, "token": token, "expires_at": stamp(expiry), "install_command": command}, 201, nil
	}
	if strings.HasPrefix(path, "node-enrollment/") && r.Method == "DELETE" {
		id := strings.TrimPrefix(path, "node-enrollment/")
		out, e := s.managementChange(ctx, nil, a, "node.enrollment_revoked", id, func(tx pgx.Tx) (any, error) {
			tag, e := tx.Exec(ctx, `UPDATE vr_node_enrollments SET revoked_at=now() WHERE id=$1 AND used_at IS NULL`, id)
			if e == nil && tag.RowsAffected() != 1 {
				return nil, fail(404, "unused enrollment not found")
			}
			return Data{"revoked": true}, e
		})
		return out, 200, e
	}
	return nil, 0, fail(405, "method not allowed")
}
func (s *Server) completeEnrollment(w http.ResponseWriter, r *http.Request) (any, int, error) {
	if r.Method != "POST" {
		return nil, 0, fail(405, "method not allowed")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	if e := rateLimit(ctx, s.Store.Pool, "node:enroll:"+clientIP(r), 20, time.Hour); e != nil {
		return nil, 0, e
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if len(token) < 40 || len(token) > 100 {
		return nil, 0, fail(404, "enrollment not found")
	}
	input, e := readBody(w, r)
	if e != nil {
		return nil, 0, e
	}
	if e = allowed(input, "agent_url agent_token certificate_sha256"); e != nil {
		return nil, 0, fail(400, e.Error())
	}
	if len(str(input, "agent_token")) < 32 || len(str(input, "agent_token")) > 256 || !certificateFingerprint.MatchString(str(input, "certificate_sha256")) {
		return nil, 0, fail(400, "valid agent credentials and certificate fingerprint required")
	}
	tx, e := s.Store.Pool.Begin(ctx)
	if e != nil {
		return nil, 0, e
	}
	defer tx.Rollback(ctx)
	var id string
	var actor int64
	var raw []byte
	e = tx.QueryRow(ctx, `SELECT id,actor_id,data FROM vr_node_enrollments WHERE token_hash=$1 AND used_at IS NULL AND revoked_at IS NULL AND expires_at>now() FOR UPDATE`, digest(token)).Scan(&id, &actor, &raw)
	if e != nil {
		return nil, 0, fail(404, "enrollment is expired, used or unavailable")
	}
	d, e := decode(raw)
	if e != nil {
		return nil, 0, e
	}
	u, e := get(ctx, tx, "users", actor)
	if e != nil || !flag(u, "is_active") || !flag(u, "is_superuser") {
		return nil, 0, fail(403, "enrollment issuer is no longer an administrator")
	}
	address, e := url.Parse(str(input, "agent_url"))
	if e != nil || address.Scheme != "https" || !strings.EqualFold(address.Hostname(), str(d, "public_host")) {
		return nil, 0, fail(400, "agent URL must use the enrollment's HTTPS public host")
	}
	if address.Port() != "" {
		port, e := strconv.Atoi(address.Port())
		if e != nil || port < 1 || port > 65535 {
			return nil, 0, fail(400, "invalid agent port")
		}
	}
	n := merge(defaults("nodes"), d)
	n["agent_url"] = strings.TrimRight(address.String(), "/")
	n["agent_cert_sha256"] = str(input, "certificate_sha256")
	n["verify_tls"] = true
	n["_agent_token"], e = seal(s.Config.FieldKey, str(input, "agent_token"))
	if e != nil {
		return nil, 0, e
	}
	if e = validateNode(s.Config, n); e != nil {
		return nil, 0, fail(400, e.Error())
	}
	health, e := s.nodeCall(ctx, n, "/health", nil)
	if e != nil {
		return nil, 0, fail(502, "agent verification failed; check HTTPS reachability, credentials and certificate")
	}
	if str(health, "status") != "ok" || !flag(obj(health, "xray"), "running") || str(health, "node_id") != str(d, "public_host") {
		return nil, 0, fail(409, "agent identity or core health does not match the enrollment")
	}
	n["status"] = "ok"
	n["last_seen_at"] = stamp()
	n["xray_version"] = str(obj(health, "xray"), "version")
	if e = save(ctx, tx, "nodes", n); e != nil {
		return nil, 0, e
	}
	if _, e = tx.Exec(ctx, `UPDATE vr_node_enrollments SET used_at=now(),node_id=$2 WHERE id=$1`, id, num(n, "id")); e != nil {
		return nil, 0, e
	}
	if e = audit(ctx, tx, actor, "node.enrolled", fmt.Sprint(num(n, "id")), Data{"enrollment": id}); e != nil {
		return nil, 0, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, 0, e
	}
	return Data{"registered": true, "node_id": num(n, "id"), "name": str(n, "name")}, 201, nil
}

func agentURLHost(host string, port int) string {
	return "https://" + net.JoinHostPort(host, strconv.Itoa(port))
}
