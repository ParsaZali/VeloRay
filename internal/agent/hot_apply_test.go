package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func hotConfig(users []any) []byte {
	b, _ := json.Marshal(map[string]any{"api": map[string]any{"tag": "api", "services": []any{"StatsService", "HandlerService"}}, "inbounds": []any{map[string]any{"tag": "main", "listen": "127.0.0.1", "port": 18543, "protocol": "vless", "settings": map[string]any{"clients": users, "decryption": "none"}}}, "outbounds": []any{map[string]any{"protocol": "freedom"}}})
	return b
}
func hotUser(email, id string) map[string]any {
	return map[string]any{"email": email, "id": id, "level": 0}
}
func TestLiveUserAdditionsRejectsRemovalsCredentialChangesAndStructuralChanges(t *testing.T) {
	a := hotUser("first", "9abdb74a-7e0e-4c2b-9634-9040f7af9d06")
	b := hotUser("second", "7abdb74a-7e0e-4c2b-9634-9040f7af9d06")
	before := hotConfig([]any{a})
	after := hotConfig([]any{a, b})
	raw, ok := liveUserAdditions(before, after)
	if !ok || !bytes.Contains(raw, []byte("second")) || bytes.Contains(raw, []byte("first")) {
		t.Fatal(string(raw), ok)
	}
	for _, candidate := range [][]byte{hotConfig([]any{}), hotConfig([]any{hotUser("first", b["id"].(string))}), bytes.Replace(after, []byte("18543"), []byte("18544"), 1), bytes.Replace(after, []byte("HandlerService"), []byte("UnknownService"), 1)} {
		if _, ok = liveUserAdditions(before, candidate); ok {
			t.Fatal("unsafe hot apply accepted", string(candidate))
		}
	}
}
func TestLiveUserAdditionsWithPinnedXray(t *testing.T) {
	binary := os.Getenv("VELORAY_TEST_XRAY_BINARY")
	if binary == "" {
		t.Skip("set VELORAY_TEST_XRAY_BINARY to the pinned runtime")
	}
	// Let the OS select both loopback listener ports to avoid conflicts in CI.
	port := func() int {
		l, e := net.Listen("tcp", "127.0.0.1:0")
		if e != nil {
			t.Fatal(e)
		}
		defer l.Close()
		return l.Addr().(*net.TCPAddr).Port
	}
	apiPort, proxyPort := port(), port()
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := hotUser("first", "9abdb74a-7e0e-4c2b-9634-9040f7af9d06")
	b := hotUser("second", "7abdb74a-7e0e-4c2b-9634-9040f7af9d06")
	var cfg map[string]any
	_ = json.Unmarshal(hotConfig([]any{a}), &cfg)
	ins := cfg["inbounds"].([]any)
	ins[0].(map[string]any)["port"] = proxyPort
	cfg["inbounds"] = append(ins, map[string]any{"tag": "api", "listen": "127.0.0.1", "port": apiPort, "protocol": "dokodemo-door", "settings": map[string]any{"address": "127.0.0.1"}})
	cfg["routing"] = map[string]any{"rules": []any{map[string]any{"type": "field", "inboundTag": []any{"api"}, "outboundTag": "api"}}}
	before, _ := json.Marshal(cfg)
	file := filepath.Join(dir, "config.json")
	if e := os.WriteFile(file, before, 0600); e != nil {
		t.Fatal(e)
	}
	cmd := exec.CommandContext(ctx, binary, "run", "-c", file)
	var logs synchronizedTestBuffer
	cmd.Stdout = &logs
	cmd.Stderr = &logs
	if e := cmd.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() { cancel(); _ = cmd.Wait() }()
	server := "--server=" + net.JoinHostPort("127.0.0.1", strconv.Itoa(apiPort))
	ready := false
	for attempts := 0; attempts < 100; attempts++ {
		if _, e := exec.CommandContext(ctx, binary, "api", "inboundusercount", server, "-tag=main").CombinedOutput(); e == nil {
			ready = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ready {
		t.Fatal("Xray did not start", logs.String())
	}
	ins[0].(map[string]any)["settings"].(map[string]any)["clients"] = []any{a, b}
	after, _ := json.Marshal(cfg)
	addition, ok := liveUserAdditions(before, after)
	if !ok {
		t.Fatal("addition not detected")
	}
	users := filepath.Join(dir, "users.json")
	_ = os.WriteFile(users, addition, 0600)
	if out, e := exec.CommandContext(ctx, binary, "api", "adu", server, users).CombinedOutput(); e != nil {
		t.Fatal(e, string(out))
	}
	out, e := exec.CommandContext(ctx, binary, "api", "inbounduser", server, "-tag=main").CombinedOutput()
	if e != nil || !strings.Contains(string(out), "second") || !strings.Contains(string(out), "first") {
		t.Fatal(e, string(out))
	}
}

type liveFakeRuntime struct {
	*fakeRuntime
	acknowledged int
	adds         int
}

func (f *liveFakeRuntime) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if name == "xray" && len(args) > 1 && args[0] == "api" && args[1] == "adu" {
		f.adds++
		return []byte(fmt.Sprintf("Added %d user(s) in total.\n", f.acknowledged)), nil
	}
	return f.fakeRuntime.Run(ctx, name, args...)
}
func TestManagerLiveAdditionKeepsJournalAndCompensatesPartialAcknowledgment(t *testing.T) {
	for _, ack := range []int{0, 1} {
		t.Run(fmt.Sprint(ack), func(t *testing.T) {
			dir := t.TempDir()
			before := hotConfig([]any{hotUser("first", "9abdb74a-7e0e-4c2b-9634-9040f7af9d06")})
			after := hotConfig([]any{hotUser("first", "9abdb74a-7e0e-4c2b-9634-9040f7af9d06"), hotUser("second", "7abdb74a-7e0e-4c2b-9634-9040f7af9d06")})
			config := filepath.Join(dir, "config.json")
			if e := os.WriteFile(config, before, 0600); e != nil {
				t.Fatal(e)
			}
			f := &liveFakeRuntime{fakeRuntime: &fakeRuntime{running: true, epoch: 1, value: 100}, acknowledged: ack}
			m, e := NewManager("xray", config, "xray", filepath.Join(dir, "state.json"), f)
			if e != nil {
				t.Fatal(e)
			}
			e = m.Apply(context.Background(), "live-add-user-operation", after)
			if ack == 0 {
				if e == nil || f.restarts != 1 || m.state.Operations["live-add-user-operation"].State != "rolled_back" {
					t.Fatal("partial addition was not compensated", e, f.restarts, m.state.Operations)
				}
				raw, _ := os.ReadFile(config)
				if !bytes.Equal(raw, before) {
					t.Fatal("partial config persisted")
				}
			}
			if ack == 1 {
				if e != nil || f.restarts != 0 || f.adds != 1 {
					t.Fatal("user addition restarted runtime", e, f.restarts, f.adds)
				}
				if e = m.Apply(context.Background(), "live-add-user-operation", after); e != nil || f.adds != 1 {
					t.Fatal("operation was not idempotent", e, f.adds)
				}
				if e = m.Rollback(context.Background(), "live-add-user-operation"); e != nil || f.restarts != 1 {
					t.Fatal("live addition did not roll back", e, f.restarts)
				}
			}
		})
	}
}
