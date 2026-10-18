package panel

import (
	"bytes"
	"fmt"
	"net/http"
	"time"
)

// Aggregates only: no customer names, IPs, credentials or unbounded labels.
func (s *Server) prometheus(w http.ResponseWriter, r *http.Request, a Actor) {
	if r.Method != "GET" || !flag(a.User, "is_superuser") {
		http.Error(w, "administrator read access required", 403)
		return
	}
	overview, err := s.overview(r.Context())
	if err != nil {
		s.respond(w, r, nil, 0, err)
		return
	}
	var out bytes.Buffer
	values := []struct{ name, key, help string }{
		{"nodes", "nodes_total", "Managed nodes."}, {"nodes_online", "nodes_online", "Nodes reporting healthy status."},
		{"client_connections", "clients_total", "Managed client connections; shared accounts may have multiple connections."},
		{"client_connections_active", "clients_active", "Enabled, unexpired connections within quota."},
		{"inbounds", "inbounds_total", "Managed inbounds."},
		{"quota_used_bytes", "traffic_used_bytes", "Current charged usage; may reset."},
		{"transfer_upload_bytes", "traffic_upload_bytes", "Current actual uploaded bytes; may reset."},
		{"transfer_download_bytes", "traffic_download_bytes", "Current actual downloaded bytes; may reset."},
	}
	for _, v := range values {
		fmt.Fprintf(&out, "# HELP veloray_%s %s\n# TYPE veloray_%s gauge\nveloray_%s %d\n", v.name, v.help, v.name, v.name, num(overview, v.key))
	}
	started := s.StartedAt
	if started.IsZero() {
		started = panelProcessStartedAt
	}
	fmt.Fprintf(&out, "# HELP veloray_panel_uptime_seconds Panel process uptime.\n# TYPE veloray_panel_uptime_seconds gauge\nveloray_panel_uptime_seconds %.0f\n", max(0, time.Since(started).Seconds()))
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write(out.Bytes())
}
