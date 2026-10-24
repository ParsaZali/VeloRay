package panel

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestUpdateStateNeverClaimsUnknownOrUnverifiedVersionsAreCurrent(t *testing.T) {
	for _, c := range []struct{ installed, latest, ci, problem, want string }{
		{"", "b", "success", "", "unknown"}, {"a", "", "success", "", "unknown"}, {"a", "a", "", "outage", "unknown"},
		{"a", "a", "", "", "up_to_date"}, {"a", "b", "success", "", "available"}, {"a", "b", "failure", "", "verifying"}, {"a", "b", "in_progress", "", "verifying"},
	} {
		if got := updateState(c.installed, c.latest, c.ci, c.problem); got != c.want {
			t.Fatalf("%+v: %s", c, got)
		}
	}
}

type updateTransport func(*http.Request) (*http.Response, error)

func (f updateTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestUpdateGitHubTransportCannotFollowRedirects(t *testing.T) {
	var calls atomic.Int64
	s := &Server{GitHubHTTP: &http.Client{Transport: updateTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if r.URL.Scheme != "https" || r.URL.Host != "api.github.com" || !strings.HasPrefix(r.URL.Path, "/repos/ParsaZali/VeloRay/") {
			t.Fatal("nonofficial source", r.URL)
		}
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://untrusted.example/"}}, Body: io.NopCloser(strings.NewReader("{}"))}, nil
	})}}
	if err := s.githubUpdateJSON(context.Background(), "git/ref/heads/main", &Data{}); err == nil || calls.Load() != 1 {
		t.Fatal("redirect accepted", calls.Load(), err)
	}
}
func TestHostControlRejectsOtherScopesAndArbitraryCommands(t *testing.T) {
	s := &Server{}
	for _, a := range []Actor{{User: Data{"role": "reseller"}, Scope: "admin"}, {User: Data{"is_superuser": true}, Scope: "read"}} {
		for _, path := range []string{"updates/status", "updates/check", "updates/install"} {
			if _, _, err := s.updatesAPI(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/"+path, strings.NewReader(`{}`)), path, a); err == nil {
				t.Fatal("scope accepted", a, path)
			}
		}
		if _, _, err := s.panelEndpointAPI(httptest.NewRecorder(), httptest.NewRequest("PUT", "/api/panel-endpoint", strings.NewReader(`{"port":12345}`)), a); err == nil {
			t.Fatal("port access accepted")
		}
	}
	for _, c := range [][3]string{{"exec", "sh", ""}, {"panel_update", "$(id)", strings.Repeat("a", 40)}, {"panel_update", strings.Repeat("a", 40), "bad"}, {"panel_port", "443;id", "old"}} {
		if err := s.executeHostJob(context.Background(), c[0], c[1], c[2]); err == nil {
			t.Fatal("untyped operation accepted", c)
		}
	}
	if err := s.RunNativeHostJob(context.Background()); err == nil {
		t.Fatal("non-native host control accepted")
	}
}
func TestIntegrationHostJobsSerializeAndRecordTypedRequests(t *testing.T) {
	s, ctx := integration(t)
	a := Actor{User: Data{"id": 0, "is_superuser": true}, Scope: "admin"}
	first, err := s.queueHostJob(ctx, a, "panel_update", strings.Repeat("a", 40), strings.Repeat("b", 40))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.queueHostJob(ctx, a, "panel_port", "9443", s.Config.PublicURL); err == nil {
		t.Fatal("concurrent root operations queued")
	}
	last, err := s.lastHostJob(ctx, "panel_update")
	if err != nil || num(last, "id") != num(first, "id") || str(last, "state") != "queued" {
		t.Fatal(last, err)
	}
	raw, _ := json.Marshal(last)
	if strings.Contains(string(raw), "previous") || strings.Contains(string(raw), "log") {
		t.Fatal("private job data exposed", string(raw))
	}
	if _, err = s.Store.Pool.Exec(ctx, `UPDATE vr_host_jobs SET state='failed' WHERE id=$1`, num(first, "id")); err != nil {
		t.Fatal(err)
	}
	if _, err = s.queueHostJob(ctx, a, "panel_port", "9443", s.Config.PublicURL); err != nil {
		t.Fatal("terminal job blocked retry", err)
	}
	if _, err = s.queueHostJob(ctx, a, "shell", "id", ""); err == nil {
		t.Fatal("DB accepted unknown operation")
	}
}
func TestIntegrationRandomSubscriptionPortDoesNotApplyOrChangeURLs(t *testing.T) {
	s, ctx := integration(t)
	s.Config.SubscriptionDriver = "native"
	cfg := defaultSettings()
	cfg["_subscription_endpoint"] = Data{"port": 55000, "applied_port": 55001, "status": "ready", "revision": "keep"}
	if err := saveSettings(ctx, s.Store.Pool, cfg); err != nil {
		t.Fatal(err)
	}
	a := Actor{User: Data{"is_superuser": true}, Scope: "admin"}
	for i := 0; i < 10; i++ {
		r := httptest.NewRequest("GET", "/api/subscription-endpoint/random-port", nil).WithContext(ctx)
		out, code, err := s.subscriptionEndpointAPI(httptest.NewRecorder(), r, a)
		if err != nil || code != 200 {
			t.Fatal(out, code, err)
		}
		p := num(out.(Data), "port")
		if p < 49152 || p > 65535 || p == 55000 || p == 55001 || validateSubscriptionPort(s.Config, p) != nil {
			t.Fatal("bad random candidate", p)
		}
	}
	after, err := settings(ctx, s.Store.Pool)
	if err != nil || str(obj(after, "_subscription_endpoint"), "revision") != "keep" || num(obj(after, "_subscription_endpoint"), "applied_port") != 55001 {
		t.Fatal("random suggestion applied a listener", after, err)
	}
}
func TestIntegrationPanelPortRejectsInboundsSubscriptionsAndServicePorts(t *testing.T) {
	s, ctx := integration(t)
	n, _, _ := fixture(t, s, ctx, &nodeMock{running: true})
	n["is_local"] = true
	if err := save(ctx, s.Store.Pool, "nodes", n); err != nil {
		t.Fatal(err)
	}
	cfg := defaultSettings()
	cfg["_subscription_endpoint"] = Data{"port": 12345, "applied_port": 12346}
	if err := saveSettings(ctx, s.Store.Pool, cfg); err != nil {
		t.Fatal(err)
	}
	for _, p := range []int64{0, 65536, 80, 443, 5432, 8610, 8611, 8612, 9191, 10085, 10086, 10087, 8443, 12345, 12346} {
		if err := s.validatePanelPort(ctx, p); err == nil {
			t.Fatal("occupied or reserved panel port accepted", p)
		}
	}
	if err := s.validatePanelPort(ctx, 54321); err != nil {
		t.Fatal(err)
	}
}

func TestIntegrationHostWorkerTracksFailureSuccessAndInterruptedJobs(t *testing.T) {
	s, ctx := integration(t)
	a := Actor{User: Data{"is_superuser": true}, Scope: "admin"}
	_, err := s.queueHostJob(ctx, a, "panel_port", "12345", s.Config.PublicURL)
	if err != nil {
		t.Fatal(err)
	}
	called := 0
	execute := func(_ context.Context, kind, target, previous string) error {
		called++
		job, err := s.lastHostJob(ctx, kind)
		if err != nil || str(job, "state") != "running" || kind != "panel_port" || target != "12345" || previous != s.Config.PublicURL {
			t.Fatal(job, err, kind, target, previous)
		}
		return errors.New("simulated install failure")
	}
	if err = s.runNativeHostJob(ctx, execute); err != nil {
		t.Fatal(err)
	}
	job, err := s.lastHostJob(ctx, "panel_port")
	if err != nil || str(job, "state") != "failed" || str(job, "error") != "simulated install failure" || called != 1 {
		t.Fatal(job, err, called)
	}
	_, err = s.queueHostJob(ctx, a, "panel_port", "12346", s.Config.PublicURL)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.runNativeHostJob(ctx, func(context.Context, string, string, string) error { called++; return nil }); err != nil {
		t.Fatal(err)
	}
	job, err = s.lastHostJob(ctx, "panel_port")
	if err != nil || str(job, "state") != "succeeded" || str(job, "error") != "" || called != 2 {
		t.Fatal(job, err, called)
	}
	next, err := s.queueHostJob(ctx, a, "panel_port", "12347", s.Config.PublicURL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Store.Pool.Exec(ctx, `UPDATE vr_host_jobs SET state='running' WHERE id=$1`, num(next, "id")); err != nil {
		t.Fatal(err)
	}
	if err = s.runNativeHostJob(ctx, func(context.Context, string, string, string) error {
		t.Fatal("interrupted job executed twice")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	job, err = s.lastHostJob(ctx, "panel_port")
	if err != nil || str(job, "state") != "failed" || !strings.Contains(str(job, "error"), "interrupted") {
		t.Fatal(job, err)
	}
}
