package agent

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PaarSaAm/VeloRay/internal/nativecore"
)

func nativeFixture(t *testing.T) (*Manager, []byte) {
	t.Helper()
	m, err := newNativeManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.native.close)
	socket, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := socket.Addr().(*net.TCPAddr).Port
	socket.Close()
	raw, _ := json.Marshal(nativecore.Config{Inbounds: []nativecore.Inbound{{Tag: "telegram", Listen: "127.0.0.1", Port: port, Protocol: "mtproto", Users: []nativecore.User{{Name: "veloray:1:test", Secret: "ee" + strings.Repeat("01", 16) + hex.EncodeToString([]byte("www.example.com"))}}}}})
	return m, raw
}
func TestNativeManagerIdempotentApplyRollbackAndStableCounterEpoch(t *testing.T) {
	m, raw := nativeFixture(t)
	ctx := context.Background()
	epoch, err := m.epoch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := m.Apply(ctx, "native-operation", raw); err != nil {
			t.Fatal(err)
		}
	}
	if checks, err := m.Ports(ctx, nil); err != nil || len(checks) != 1 || checks[0].State != "owned" {
		t.Fatal(checks, err)
	}
	if _, err := m.native.Run(ctx, "systemctl", "stop", "veloray-native.service"); err != nil {
		t.Fatal(err)
	}
	if err := m.Restart(ctx); err != nil {
		t.Fatal(err)
	}
	after, err := m.epoch(ctx)
	if err != nil || after != epoch {
		t.Fatalf("preserved raw counters require a stable epoch: %q %q %v", epoch, after, err)
	}
	if err := m.Rollback(ctx, "native-operation"); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(m.ConfigPath)
	cfg, err := nativecore.Decode(body)
	if err != nil || len(cfg.Inbounds) != 0 {
		t.Fatal(string(body), err)
	}
}
func TestNativeCrashRecoveryRestoresUnacknowledgedConfig(t *testing.T) {
	m, raw := nativeFixture(t)
	before, _ := os.ReadFile(m.ConfigPath)
	m.state.Operations["unacknowledged-native-operation"] = operation{Before: before, After: hash(raw), State: "prepared"}
	if err := m.persist(); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(m.ConfigPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Dir(m.ConfigPath)
	m.native.close()
	next, err := newNativeManager(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer next.native.close()
	restored, _ := os.ReadFile(next.ConfigPath)
	if string(restored) != string(before) {
		t.Fatal("unacknowledged listener restored instead of previous config")
	}
}
