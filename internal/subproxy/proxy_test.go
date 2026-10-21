package subproxy

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestControllerValidatesPortAndKeepsPreviousListener(t *testing.T) {
	var configs []string
	persisted := []int{}
	failReload, failPersist := false, false
	c := &controller{domain: "example.com", port: 8444, reload: func(_ context.Context, config string) error {
		configs = append(configs, config)
		if failReload {
			return errors.New("reload failed")
		}
		return nil
	}, persist: func(port int) error {
		if failPersist {
			return errors.New("write failed")
		}
		persisted = append(persisted, port)
		return nil
	}}
	put := func(body, origin string) int {
		r := httptest.NewRequest("PUT", "/port", strings.NewReader(body))
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		c.ServeHTTP(w, r)
		return w.Code
	}
	for _, body := range []string{`{}`, `{"port":"9000"}`, `{"port":80}`, `{"port":443}`, `{"port":8612}`, `{"port":65536}`, `{"port":9000,"config":"evil"}`, `{"port":9000}{"port":9001}`} {
		if got := put(body, ""); got != 400 {
			t.Fatalf("%s: %d", body, got)
		}
	}
	if got := put(`{"port":9000}`, "https://example.com"); got != 403 {
		t.Fatal(got)
	}
	if c.port != 8444 || len(configs) != 0 {
		t.Fatal("invalid request changed listener")
	}
	failReload = true
	if got := put(`{"port":9000}`, ""); got != 409 {
		t.Fatal(got)
	}
	if c.port != 8444 || len(persisted) != 0 {
		t.Fatal("failed reload changed state")
	}
	failReload = false
	failPersist = true
	if got := put(`{"port":9000}`, ""); got != 500 {
		t.Fatal(got)
	}
	if c.port != 8444 || !strings.Contains(configs[len(configs)-1], "https://example.com:8444") {
		t.Fatal("persistence error did not restore old listener")
	}
	failPersist = false
	if got := put(`{"port":9000}`, ""); got != 200 {
		t.Fatal(got)
	}
	if c.port != 9000 || len(persisted) != 1 || persisted[0] != 9000 {
		t.Fatal("successful port was not saved")
	}
	if got := put(`{"port":9000}`, ""); got != 200 || len(persisted) != 2 {
		t.Fatal("same port was not confirmed and persisted")
	}
	w := httptest.NewRecorder()
	c.ServeHTTP(w, httptest.NewRequest("GET", "/port", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"port":9000`) {
		t.Fatal(w.Body.String())
	}
}
func TestConfigurationIsFixedAndDomainValidated(t *testing.T) {
	for _, host := range []string{"example.com; import /etc/passwd", "https://example.com", "bad\nheader", "a..b", ""} {
		if validDomain(host) {
			t.Fatalf("accepted %q", host)
		}
	}
	for _, host := range []string{"example.com", "127.0.0.1", "::1"} {
		if !validDomain(host) {
			t.Fatalf("rejected %q", host)
		}
	}
	if raw := configuration("example.com", 9000); !strings.Contains(raw, "redir https://example.com:9000{uri} 307") || !strings.Contains(raw, "admin unix//tmp/veloray-caddy-admin.sock") || !strings.Contains(raw, "exclude http admin.api") {
		t.Fatal(raw)
	}
}

func TestUnavailableSubscriptionBackendDoesNotMoveListener(t *testing.T) {
	called := false
	c := &controller{domain: "example.com", port: 8444, health: func(context.Context) error { return errors.New("backend offline") }, reload: func(context.Context, string) error { called = true; return nil }, persist: func(int) error { called = true; return nil }}
	w := httptest.NewRecorder()
	c.ServeHTTP(w, httptest.NewRequest("PUT", "/port", strings.NewReader(`{"port":9444}`)))
	if w.Code != 503 || called || c.port != 8444 {
		t.Fatal("unhealthy backend moved the live endpoint", w.Code, called, c.port)
	}
}

func TestInitialPortWaitsForCaddyBeforePersistence(t *testing.T) {
	persisted := false
	c := &controller{domain: "example.com", port: 8444, reload: func(context.Context, string) error { return errProxyUnavailable }, persist: func(int) error { persisted = true; return nil }}
	w := httptest.NewRecorder()
	c.ServeHTTP(w, httptest.NewRequest("PUT", "/port", strings.NewReader(`{"port":8444}`)))
	if w.Code != 503 || persisted {
		t.Fatal("unconfirmed initial port became durable", w.Code, persisted)
	}
}
