package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProcessRuntimeOwnsOnlyRegisteredChildren(t *testing.T) {
	if os.PathSeparator != '/' {
		t.Skip("Linux process runtime")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "core")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexec /bin/sleep 300\n"), 0700); err != nil {
		t.Fatal(err)
	}
	p := NewProcessRuntime()
	defer p.Close()
	m := &Manager{Binary: binary, ConfigPath: filepath.Join(dir, "core.json"), ServiceName: "xray.service"}
	if err := p.register(m); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := p.Run(ctx, "systemctl", "restart", "other.service"); err == nil {
		t.Fatal("controlled unrelated host service")
	}
	if _, err := p.Run(ctx, "systemctl", "restart", m.ServiceName, "other.service"); err == nil {
		t.Fatal("accepted extra service")
	}
	if _, err := p.Run(ctx, "systemctl", "restart", m.ServiceName); err != nil {
		t.Fatal(err)
	}
	first, err := p.Run(ctx, "systemctl", "show", m.ServiceName, "--property=MainPID,ExecMainStartTimestampMonotonic")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(first), "MainPID=0\n") {
		t.Fatal("missing child identity")
	}
	if _, err := p.Run(ctx, "systemctl", "restart", m.ServiceName); err != nil {
		t.Fatal(err)
	}
	second, _ := p.Run(ctx, "systemctl", "show", m.ServiceName, "--property=MainPID,ExecMainStartTimestampMonotonic")
	if string(first) == string(second) {
		t.Fatal("restart reused counter epoch")
	}
	select {
	case err := <-p.failure:
		t.Fatal("planned restart reported as failure", err)
	default:
	}
	p.mu.Lock()
	child := p.children[m.ServiceName]
	p.mu.Unlock()
	if err := child.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-p.failure:
	case <-time.After(3 * time.Second):
		t.Fatal("unexpected core death not reported")
	}
	p.Close()
	if processAlive(child) {
		t.Fatal("child survived supervisor close")
	}
	if _, err := p.Run(ctx, "systemctl", "restart", m.ServiceName); err == nil {
		t.Fatal("closed supervisor restarted child")
	}
}
func TestProcessRuntimeCoreInitializationPreservesExistingData(t *testing.T) {
	for _, core := range []string{"xray", "singbox"} {
		m := &Manager{Core: core, ConfigPath: filepath.Join(t.TempDir(), "config.json")}
		if err := initializeProcessCore(m); err != nil {
			t.Fatal(err)
		}
		raw, _ := os.ReadFile(m.ConfigPath)
		if !strings.Contains(string(raw), "127.0.0.1") {
			t.Fatal("bootstrap has no private telemetry")
		}
		os.WriteFile(m.ConfigPath, []byte("operator-owned-config"), 0600)
		if err := initializeProcessCore(m); err != nil {
			t.Fatal(err)
		}
		raw, _ = os.ReadFile(m.ConfigPath)
		if string(raw) != "operator-owned-config" {
			t.Fatal("existing configuration replaced")
		}
	}
}
func TestProcessRuntimeRecoversPreparedConfigBeforeStartingListeners(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager("unused", filepath.Join(dir, "config.json"), "xray.service", filepath.Join(dir, "state.json"), NewProcessRuntime())
	if err != nil {
		t.Fatal(err)
	}
	before, after := []byte(`{"old":"confirmed"}`), []byte(`{"new":"unconfirmed"}`)
	if err = os.WriteFile(m.ConfigPath, after, 0640); err != nil {
		t.Fatal(err)
	}
	m.state.Operations["interrupted"] = operation{Before: before, After: hash(after), State: "prepared", At: time.Now()}
	if err = m.persist(); err != nil {
		t.Fatal(err)
	}
	if err = initializeProcessCore(m); err != nil {
		t.Fatal(err)
	}
	current, _ := os.ReadFile(m.ConfigPath)
	if string(current) != string(before) || m.state.Operations["interrupted"].State != "rolled_back" {
		t.Fatal("opened an unconfirmed configuration")
	}
}
