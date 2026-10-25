package panel

import (
	"net/http"
	"os"
	"runtime"
)

// Deployment metadata is configuration, not Docker-daemon access or a claim
// about the state of another container. Live engine health remains on Nodes.
func (s *Server) deploymentAPI(r *http.Request, a Actor) (any, int, error) {
	if !flag(a.User, "is_superuser") || a.Scope != "admin" {
		return nil, 0, fail(403, "administrator access required")
	}
	if r.Method != http.MethodGet {
		return nil, 0, fail(405, "method not allowed")
	}
	mode, proxy := "external", "external"
	if os.Getenv("VELORAY_DEPLOYMENT") == "docker" {
		mode, proxy = "docker", "Caddy"
	} else if s.Config.SubscriptionDriver == "native" {
		mode, proxy = "native", "nginx"
	}
	return Data{"mode": mode, "database": "PostgreSQL", "https_proxy": proxy, "architecture": runtime.GOARCH, "panel_origin": s.Config.PublicURL, "subscription_port": s.Config.SubscriptionPort, "managed_updates": s.nativeControlAvailable(), "installed_commit": installedCommit()}, 200, nil
}
