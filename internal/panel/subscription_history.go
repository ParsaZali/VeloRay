package panel

import (
	"context"
	"net/http"
	"strings"
	"time"
)

type subscriptionHistoryWriter struct {
	http.ResponseWriter
	status int
}

func (w *subscriptionHistoryWriter) WriteHeader(code int) {
	if w.status != 0 {
		return
	}
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
func (w *subscriptionHistoryWriter) Write(raw []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(raw)
}
func subscriptionApp(r *http.Request) string {
	ua := strings.ToLower(r.UserAgent())
	for _, app := range []string{"sing-box", "singbox", "mihomo", "clash", "stash", "hiddify", "v2rayng", "v2rayn"} {
		if strings.Contains(ua, app) {
			return app
		}
	}
	if strings.Contains(r.Header.Get("Accept"), "text/html") {
		return "browser"
	}
	return "other"
}
func (s *Server) recordSubscriptionRequest(r *http.Request, c Data, w *subscriptionHistoryWriter) {
	format := r.URL.Query().Get("format")
	if !oneOf(format, "base64", "b64", "raw", "text", "singbox", "sing-box", "xray", "json", "clash", "yaml", "wireguard", "wg", "portal", "html") {
		format = "auto"
	}
	status := w.status
	if status == 0 {
		status = 200
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), time.Second)
	defer cancel()
	// No source IP, HWID, raw User-Agent, URL, credentials, token or query is stored.
	if _, e := s.Store.Pool.Exec(ctx, `INSERT INTO vr_subscription_requests(principal_key,format,client_app,status) VALUES($1,$2,$3,$4)`, accountKey(c), format, subscriptionApp(r), status); e != nil {
		s.Logger.Warn("subscription request history unavailable")
	}
}
func (s *Server) subscriptionHistory(ctx context.Context, c Data) ([]Data, error) {
	rows, e := s.Store.Pool.Query(ctx, `SELECT at,format,client_app,status FROM vr_subscription_requests WHERE principal_key=$1 ORDER BY id DESC LIMIT 100`, accountKey(c))
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Data{}
	for rows.Next() {
		var at time.Time
		var format, app string
		var status int
		if e = rows.Scan(&at, &format, &app, &status); e != nil {
			return nil, e
		}
		out = append(out, Data{"at": stamp(at), "format": format, "client_app": app, "status": status})
	}
	return out, rows.Err()
}
