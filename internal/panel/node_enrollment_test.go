package panel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestIntegrationNodeEnrollmentPinsCertificateAndConsumesTokenOnce(t *testing.T) {
	s, ctx, a, _, _ := commerceFixture(t)
	var probes atomic.Int32
	agent := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("b", 40) {
			w.WriteHeader(401)
			return
		}
		probes.Add(1)
		writeJSON(w, 200, Data{"status": "ok", "node_id": "127.0.0.1", "xray": Data{"running": true, "version": "v26.10"}})
	}))
	defer agent.Close()
	sum := sha256.Sum256(agent.Certificate().Raw)
	pin := hex.EncodeToString(sum[:])
	issued, _, e := managementRequest(t, s, ctx, a, "POST", "node-enrollment", Data{"name": "New pinned node", "public_host": "127.0.0.1"})
	if e != nil {
		t.Fatal(e)
	}
	token := str(issued.(Data), "token")
	invoke := func(fingerprint string) *httptest.ResponseRecorder {
		d := Data{"agent_url": agent.URL, "agent_token": strings.Repeat("b", 40), "certificate_sha256": fingerprint}
		raw, _ := json.Marshal(d)
		r := httptest.NewRequest("POST", "/api/node-enrollment/complete", strings.NewReader(string(raw)))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w
	}
	if w := invoke(strings.Repeat("0", 64)); w.Code != 502 {
		t.Fatal(w.Code, w.Body.String())
	}
	codes := make(chan int, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); codes <- invoke(pin).Code }()
	}
	wg.Wait()
	close(codes)
	created, used := 0, 0
	for code := range codes {
		if code == 201 {
			created++
		} else if code == 404 {
			used++
		} else {
			t.Fatal(code)
		}
	}
	if created != 1 || used != 1 || probes.Load() != 1 {
		t.Fatal(created, used, probes.Load())
	}
	var stored []byte
	if e = s.Store.Pool.QueryRow(ctx, `SELECT data FROM vr_nodes WHERE data->>'name'='New pinned node'`).Scan(&stored); e != nil {
		t.Fatal(e)
	}
	node, e := decode(stored)
	if e != nil {
		t.Fatal(e)
	}
	if str(node, "agent_cert_sha256") != pin || str(node, "_agent_token") == strings.Repeat("b", 40) {
		t.Fatal("pin or token encryption missing")
	}
	list, _, e := managementRequest(t, s, ctx, a, "GET", "node-enrollment", nil)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(list)
	if strings.Contains(string(raw), token) || strings.Contains(string(raw), "token_hash") {
		t.Fatal("enrollment secret leaked")
	}
}
func TestIntegrationNodeEnrollmentRevocationAndExpiration(t *testing.T) {
	s, ctx, a, _, _ := commerceFixture(t)
	for _, action := range []string{"revoke", "expire"} {
		out, _, e := managementRequest(t, s, ctx, a, "POST", "node-enrollment", Data{"name": "Denied " + action, "public_host": "node.example"})
		if e != nil {
			t.Fatal(e)
		}
		d := out.(Data)
		if action == "revoke" {
			_, _, e = managementRequest(t, s, ctx, a, "DELETE", "node-enrollment/"+str(d, "id"), nil)
		} else {
			_, e = s.Store.Pool.Exec(ctx, `UPDATE vr_node_enrollments SET expires_at=$2 WHERE id=$1`, str(d, "id"), time.Now().Add(-time.Hour))
		}
		if e != nil {
			t.Fatal(e)
		}
		r := httptest.NewRequest("POST", "/api/node-enrollment/complete", strings.NewReader(`{"agent_url":"https://node.example:12345","agent_token":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","certificate_sha256":"0000000000000000000000000000000000000000000000000000000000000000"}`))
		r.Header.Set("Authorization", "Bearer "+str(d, "token"))
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != 404 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
