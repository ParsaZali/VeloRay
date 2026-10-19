package panel

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIntegrationLiveOverviewIsScopedAndDisconnectReleasesSlot(t *testing.T) {
	s, ctx := integration(t)
	_, i, _ := fixture(t, s, ctx, &nodeMock{running: true, stats: []any{}})
	owner := defaults("users")
	owner["username"], owner["role"], owner["allowed_inbounds"] = "stream-seller", "reseller", []any{num(i, "id")}
	if e := save(ctx, s.Store.Pool, "users", owner); e != nil {
		t.Fatal(e)
	}
	_, _, e := managementRequest(t, s, ctx, Actor{User: owner}, "POST", "customers", Data{"name": "Only my account", "inbound_ids": []any{num(i, "id")}, "traffic_limit_bytes": 12345})
	if e != nil {
		t.Fatal(e)
	}
	key := defaults("api_keys")
	key["name"], key["user"], key["_token_hash"] = "stream-test", num(owner, "id"), digest("stream-read-token")
	if e = save(ctx, s.Store.Pool, "api_keys", key); e != nil {
		t.Fatal(e)
	}
	host := httptest.NewServer(s.Handler())
	defer host.Close()
	requestCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, _ := http.NewRequestWithContext(requestCtx, "GET", host.URL+"/api/events", nil)
	req.Header.Set("Authorization", "Bearer stream-read-token")
	response, e := host.Client().Do(req)
	if e != nil {
		t.Fatal(e)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 || response.Header.Get("Content-Type") != "text/event-stream" || response.Header.Get("X-Accel-Buffering") != "no" {
		t.Fatal(response.StatusCode, response.Header)
	}
	scanner := bufio.NewScanner(response.Body)
	found := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			payload, e := decode([]byte(strings.TrimPrefix(line, "data: ")))
			if e != nil {
				t.Fatal(e)
			}
			overview := obj(payload, "overview")
			if num(overview, "clients_total") != 1 || num(overview, "traffic_limit_bytes") != 12345 || num(overview, "nodes_total") != 0 {
				t.Fatal("cross-owner stream", overview)
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatal("stream event absent", scanner.Err())
	}
	cancel()
	response.Body.Close()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		s.streamMu.Lock()
		count := s.streamCount
		s.streamMu.Unlock()
		if count == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("stream slot leaked after disconnect")
}
