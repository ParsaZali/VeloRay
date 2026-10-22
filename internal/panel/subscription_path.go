package panel

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
)

var subscriptionPathPattern = regexp.MustCompile(`^/(?:[A-Za-z0-9_-]+/)*$`)

func subscriptionBasePath(cfg Data) string {
	if _, exists := cfg["subscription_path"]; !exists {
		return "/sub/"
	}
	p := strings.Trim(str(cfg, "subscription_path"), "/")
	if p == "" {
		return "/"
	}
	return "/" + p + "/"
}

func validateSubscriptionPath(value string) error {
	p := subscriptionBasePath(Data{"subscription_path": value})
	if len(p) > 160 || !subscriptionPathPattern.MatchString(p) {
		return errors.New("subscription path must contain letters, digits, underscores, hyphens or slash-separated segments")
	}
	first := strings.Split(strings.Trim(p, "/"), "/")[0]
	if oneOf(first, "api", "assets", "fonts", ".well-known") {
		return errors.New("subscription path conflicts with a reserved application route")
	}
	return nil
}

func subscriptionToken(path string, cfg Data) (string, bool) {
	prefix := subscriptionBasePath(cfg)
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	token := strings.TrimPrefix(path, prefix)
	if head, _, found := strings.Cut(token, "/assets/"); found {
		token = head
	}
	return token, subscriptionTokenPattern.MatchString(token)
}

// Both the panel listener and the independent subscription listener use the
// same route resolver. Root paths never capture API, static files or SPA routes.
func (s *Server) routeSubscription(w http.ResponseWriter, r *http.Request) bool {
	if s.Store == nil || strings.HasPrefix(r.URL.Path, "/api/") {
		return false
	}
	head, _, _ := strings.Cut(r.URL.Path, "/assets/")
	parts := strings.Split(strings.Trim(head, "/"), "/")
	if !subscriptionTokenPattern.MatchString(parts[len(parts)-1]) {
		return false
	}
	cfg, err := settings(r.Context(), s.Store.Pool)
	if err != nil {
		s.respond(w, r, nil, 0, err)
		return true
	}
	if _, ok := subscriptionToken(r.URL.Path, cfg); !ok {
		return false
	}
	if !s.Config.SubscriptionOnly {
		if origin := subscriptionOrigin(s.Config, cfg); origin != strings.TrimRight(s.Config.PublicURL, "/") {
			if r.Method != "GET" && r.Method != "HEAD" {
				writeJSON(w, 405, Data{"detail": "method not allowed"})
			} else {
				http.Redirect(w, r, origin+r.URL.RequestURI(), http.StatusTemporaryRedirect)
			}
			return true
		}
	}
	s.subscription(w, r)
	return true
}
