package panel

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSubscriptionPathsAndTokenRedaction(t *testing.T) {
	token := strings.Repeat("s", 32)
	for _, value := range []string{"", "/", "sub", "/sub/", "/customer/access/", "hello-world_1"} {
		if err := validateSubscriptionPath(value); err != nil {
			t.Fatal(value, err)
		}
		cfg := Data{"subscription_path": value}
		path := subscriptionBasePath(cfg) + token
		if got, ok := subscriptionToken(path, cfg); !ok || got != token {
			t.Fatal(value, got, ok)
		}
		if got, ok := subscriptionToken(path+"/assets/site.css", cfg); !ok || got != token {
			t.Fatal("custom assets lost token scope")
		}
		if strings.Contains(requestLogPath(httptest.NewRequest("GET", path+"?format=raw", nil)), token) {
			t.Fatal("custom path exposed token in logs")
		}
		if strings.Contains(requestLogPath(httptest.NewRequest("GET", path+"/assets/private.js", nil)), "private.js") {
			t.Fatal("custom path exposed private asset suffix in logs")
		}
	}
	for _, value := range []string{"/api/", "/assets/", "/fonts/", "/../", "/bad//path/", "/a?b/", "/a%2fb/", "/a b/"} {
		if validateSubscriptionPath(value) == nil {
			t.Fatal("unsafe path accepted", value)
		}
	}
	for _, path := range []string{"/api/health", "/settings", "/subscription-simple.css", "/assets/script.js"} {
		if _, ok := subscriptionToken(path, Data{"subscription_path": ""}); ok {
			t.Fatal("root subscription captured reserved route", path)
		}
	}
}
func TestIntegrationCustomSubscriptionPathOnBothListeners(t *testing.T) {
	s, ctx := integration(t)
	_, _, client := fixture(t, s, ctx, &nodeMock{running: true})
	token := str(client, "subscription_token")
	for _, base := range []string{"/access/", "/nested/custom/", ""} {
		cfg, err := settings(ctx, s.Store.Pool)
		if err != nil {
			t.Fatal(err)
		}
		cfg["subscription_path"] = base
		if err = saveSettings(ctx, s.Store.Pool, cfg); err != nil {
			t.Fatal(err)
		}
		link, err := s.subscriptionURL(ctx, s.Store.Pool, token)
		if err != nil || !strings.HasSuffix(link, subscriptionBasePath(cfg)+token) {
			t.Fatal(link, err)
		}
		for _, independent := range []bool{false, true} {
			s.Config.SubscriptionOnly = independent
			for _, format := range []string{"raw", "info", "portal"} {
				w := httptest.NewRecorder()
				s.Handler().ServeHTTP(w, httptest.NewRequest("GET", subscriptionBasePath(cfg)+token+"?format="+format, nil))
				if w.Code != 200 {
					t.Fatal(base, independent, format, w.Code, w.Body.String())
				}
			}
		}
		s.Config.SubscriptionOnly = false
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/sub/"+token+"?format=raw", nil))
		if w.Code == 200 && base != "/sub/" {
			t.Fatal("old private subscription path remained active")
		}
	}
}
