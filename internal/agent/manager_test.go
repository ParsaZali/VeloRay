package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type fakeRuntime struct {
	mu                  sync.Mutex
	value               int64
	epoch               int
	running             bool
	reject, restartFail bool
	restarts            int
	sockets             string
	onlineUsers         []string
	onlineError         bool
}

func (f *fakeRuntime) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if name == "ss" {
		return []byte(f.sockets), nil
	}
	if name == "xray" {
		switch args[0] {
		case "version":
			return []byte("Xray 26.10.1"), nil
		case "api":
			if args[1] == "statsgetallonlineusers" {
				if f.onlineError {
					return nil, errors.New("unsupported API")
				}
				b, _ := json.Marshal(map[string]any{"users": f.onlineUsers})
				return b, nil
			}
			b, _ := json.Marshal(map[string]any{"stat": []Counter{{Name: "user>>>veloray:1:alice>>>traffic>>>uplink", Value: f.value}}})
			return b, nil
		case "run":
			if f.reject {
				return []byte("invalid settings"), errors.New("reject")
			}
			return nil, nil
		}
	}
	if name == "systemctl" {
		switch args[0] {
		case "show":
			if !f.running {
				return []byte("MainPID=0"), nil
			}
			return []byte(fmt.Sprintf("MainPID=123\nExecMainStartTimestampMonotonic=%d", f.epoch)), nil
		case "is-active":
			if f.running {
				return []byte("active"), nil
			}
			return []byte("inactive"), errors.New("inactive")
		case "restart":
			f.restarts++
			if f.restartFail {
				f.restartFail = false
				return []byte("failed"), errors.New("restart")
			}
			f.epoch++
			f.value = 0
			f.running = true
			return nil, nil
		case "stop":
			f.running = false
			return nil, nil
		}
	}
	return nil, nil
}
func TestPortConflictPreservesConfigAndRunningService(t *testing.T) {
	m, f := testManager(t)
	f.sockets = `tcp LISTEN 0 511 0.0.0.0:443 0.0.0.0:* users:(("nginx",pid=456,fd=8))`
	raw := []byte(`{"inbounds":[{"tag":"vpn","listen":"0.0.0.0","port":443,"protocol":"vless"}]}`)
	for _, check := range []func() error{func() error { return m.Validate(context.Background(), raw) }, func() error { return m.Apply(context.Background(), "occupied-port-operation", raw) }} {
		err := check()
		if err == nil || !strings.Contains(err.Error(), "nginx") || !strings.Contains(err.Error(), "443") {
			t.Fatal("foreign listener not identified", err)
		}
	}
	unchanged, _ := os.ReadFile(m.ConfigPath)
	if string(unchanged) != `{"old":true}` || f.restarts != 0 || !f.running || len(m.state.Operations) != 0 {
		t.Fatal("preflight changed the runtime", string(unchanged), f.restarts)
	}
}
func TestPortInspectionRecognizesXrayUDPAndAddressScope(t *testing.T) {
	f := &fakeRuntime{running: true, sockets: `tcp LISTEN 0 511 0.0.0.0:443 0.0.0.0:* users:(("xray",pid=123,fd=8))
udp UNCONN 0 0 127.0.0.1:8443 0.0.0.0:* users:(("foreign",pid=456,fd=9))
tcp LISTEN 0 511 [::]:2083 [::]:* users:(("nginx",pid=456,fd=10))`}
	raw := []byte(`{"inbounds":[{"tag":"own","listen":"0.0.0.0","port":443,"protocol":"vless"},{"tag":"udp","listen":"127.0.0.1","port":8443,"protocol":"vless","streamSettings":{"network":"kcp"}},{"tag":"v6","listen":"0.0.0.0","port":2083,"protocol":"vless"},{"tag":"tcp-free","listen":"127.0.0.1","port":8443,"protocol":"vless"}]}`)
	checks, err := InspectConfigPorts(context.Background(), f, "xray", raw)
	if err != nil || len(checks) != 4 {
		t.Fatal(checks, err)
	}
	for i, want := range []string{"xray", "conflict", "conflict", "free"} {
		if checks[i].State != want {
			t.Fatal(i, checks)
		}
	}
	if listenOverlap("0.0.0.0", "::1") || listenOverlap("127.0.0.1", "127.0.0.2") {
		t.Fatal("distinct addresses overlap")
	}
	if !listenOverlap("0.0.0.0", "127.0.0.2") {
		t.Fatal("wildcard missed")
	}
}
func TestPortInspectionDetectsActualSocket(t *testing.T) {
	if _, err := exec.LookPath("ss"); err != nil {
		t.Skip("iproute2 is required")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	raw := []byte(fmt.Sprintf(`{"inbounds":[{"tag":"occupied","listen":"127.0.0.1","port":%d,"protocol":"vless"}]}`, listener.Addr().(*net.TCPAddr).Port))
	checks, err := InspectConfigPorts(context.Background(), Commands{}, "veloray-test-nonexistent.service", raw)
	if err != nil || len(checks) != 1 || checks[0].State != "conflict" {
		t.Fatal("real socket missed", checks, err)
	}
	if RequireFreePorts(checks) == nil {
		t.Fatal("occupied port accepted")
	}
}
func testManager(t *testing.T) (*Manager, *fakeRuntime) {
	t.Helper()
	d := t.TempDir()
	f := &fakeRuntime{running: true, epoch: 1}
	m, e := NewManager("xray", filepath.Join(d, "config.json"), "xray", filepath.Join(d, "state.json"), f)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(m.ConfigPath, []byte(`{"old":true}`), 0600); e != nil {
		t.Fatal(e)
	}
	return m, f
}
func TestDurableAccountingAcrossRestartAndAgentReload(t *testing.T) {
	m, f := testManager(t)
	ctx := context.Background()
	f.value = 100
	s, e := m.Stats(ctx)
	if e != nil || s.Stat[0].Value != 100 || s.Stat[0].Core != "xray" || s.OutboundAccounting != "core-scoped-v1" {
		t.Fatal(s, e)
	}
	s, _ = m.Stats(ctx)
	if s.Stat[0].Value != 100 {
		t.Fatal("double count")
	}
	if e = m.Restart(ctx); e != nil {
		t.Fatal(e)
	}
	f.value = 25
	s, e = m.Stats(ctx)
	if e != nil || s.Stat[0].Value != 125 {
		t.Fatal(s, e)
	}
	m2, e := NewManager("xray", m.ConfigPath, "xray", m.StatePath, f)
	if e != nil {
		t.Fatal(e)
	}
	s, e = m2.Stats(ctx)
	if e != nil || s.Stat[0].Value != 125 {
		t.Fatal(s, e)
	}
	f.value = 5
	s, _ = m2.Stats(ctx)
	if s.Stat[0].Value != 130 {
		t.Fatal("external reset lost traffic", s)
	}
}
func TestApplyJournalIdempotencyAndActualRollback(t *testing.T) {
	m, f := testManager(t)
	ctx := context.Background()
	id := "operation-123456789"
	cfg := []byte(`{"new":true}`)
	if e := m.Apply(ctx, id, cfg); e != nil {
		t.Fatal(e)
	}
	if e := m.Apply(ctx, id, cfg); e != nil || f.restarts != 1 {
		t.Fatal(e, f.restarts)
	}
	if e := m.Rollback(ctx, id); e != nil {
		t.Fatal(e)
	}
	raw, _ := os.ReadFile(m.ConfigPath)
	if string(raw) != `{"old":true}` {
		t.Fatal(string(raw))
	}
	if e := m.Rollback(ctx, id); e != nil {
		t.Fatal(e)
	}
}
func TestApplyFailureRestoresPriorConfig(t *testing.T) {
	m, f := testManager(t)
	f.value = 100
	f.restartFail = true
	if e := m.Apply(context.Background(), "failure-operation-123", []byte(`{"new":true}`)); e == nil {
		t.Fatal("expected failure")
	}
	raw, _ := os.ReadFile(m.ConfigPath)
	if string(raw) != `{"old":true}` {
		t.Fatal(string(raw))
	}
	s, e := m.Stats(context.Background())
	if e != nil || s.Stat[0].Value != 100 {
		t.Fatal(s, e)
	}
}
func TestRollbackRefusesSupersededConfig(t *testing.T) {
	m, _ := testManager(t)
	ctx := context.Background()
	if e := m.Apply(ctx, "operation-first-12345", []byte(`{"a":1}`)); e != nil {
		t.Fatal(e)
	}
	if e := m.Apply(ctx, "operation-second-1234", []byte(`{"a":2}`)); e != nil {
		t.Fatal(e)
	}
	if e := m.Rollback(ctx, "operation-first-12345"); e == nil {
		t.Fatal("rollback clobbered later update")
	}
}
func TestConcurrentStatsAreSerialized(t *testing.T) {
	m, f := testManager(t)
	f.value = 100
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, e := m.Stats(context.Background())
			if e != nil || s.Stat[0].Value != 100 {
				t.Error(s, e)
			}
		}()
	}
	wg.Wait()
}
func TestCorruptLedgerFailsClosed(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "state.json")
	_ = os.WriteFile(p, []byte("invalid"), 0600)
	if _, e := NewManager("xray", filepath.Join(d, "config"), "xray", p, nil); e == nil {
		t.Fatal("corrupt ledger accepted")
	}
}

func TestOnlineSnapshotDoesNotChangeDurableAccounting(t *testing.T) {
	m, f := testManager(t)
	f.value = 100
	f.onlineUsers = []string{"user>>>veloray:1:alice>>>online"}
	if err := os.WriteFile(m.ConfigPath, []byte(`{"policy":{"levels":{"0":{"statsUserOnline":true}}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := m.Stats(context.Background())
	if err != nil || !snapshot.OnlineAvailable || len(snapshot.OnlineUsers) != 1 || snapshot.OnlineUsers[0] != "veloray:1:alice" || snapshot.OnlineTrackingRequired || snapshot.Stat[0].Value != 100 {
		t.Fatal(snapshot, err)
	}
	f.onlineError = true
	snapshot, err = m.Stats(context.Background())
	if err != nil || snapshot.OnlineAvailable || snapshot.Stat[0].Value != 100 {
		t.Fatal("online failure broke billing", snapshot, err)
	}
	f.running = false
	snapshot, err = m.Stats(context.Background())
	if err != nil || !snapshot.OnlineAvailable || len(snapshot.OnlineUsers) != 0 {
		t.Fatal(snapshot, err)
	}
}
func TestOnlineTrackingUpgradeDetection(t *testing.T) {
	m, _ := testManager(t)
	snapshot, err := m.Stats(context.Background())
	if err != nil || !snapshot.OnlineTrackingRequired || snapshot.OnlineAvailable {
		t.Fatal(snapshot, err)
	}
}
