package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/PaarSaAm/VeloRay/internal/nativecore"
)

// The native runtime is part of the agent and can only operate its own typed
// protocol listeners. It implements Manager's durable operation/counter contract
// without a privileged service socket or additional public control API.
type nativeRuntime struct {
	mu      sync.Mutex
	engine  *nativecore.Engine
	config  string
	started int64
	running bool
}

func newNativeManager(directory string) (*Manager, error) {
	runtime := &nativeRuntime{engine: nativecore.New(), started: time.Now().UnixNano(), config: filepath.Join(directory, "native-config.json")}
	manager, err := NewManager("veloray-native", runtime.config, "veloray-native.service", filepath.Join(directory, "native-state.json"), runtime)
	if err != nil {
		return nil, err
	}
	manager.Core = "native"
	manager.native = runtime
	// Restore an unacknowledged configuration before opening any native listener.
	if err = initializeNative(manager); err != nil {
		runtime.engine.Close()
		return nil, err
	}
	return manager, nil
}
func initializeNative(m *Manager) error {
	if _, err := os.Stat(m.ConfigPath); errors.Is(err, os.ErrNotExist) {
		if err = atomicWrite(m.ConfigPath, []byte(`{"inbounds":[]}`), 0600); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	raw, err := os.ReadFile(m.ConfigPath)
	if err != nil {
		return err
	}
	var latest operation
	found := false
	for _, op := range m.state.Operations {
		if (op.State == "prepared" || op.State == "rollback_failed") && hash(raw) == op.After && (!found || op.At.After(latest.At)) {
			latest = op
			found = true
		}
	}
	if found {
		raw = latest.Before
		if len(raw) == 0 {
			raw = []byte(`{"inbounds":[]}`)
		}
		if err = atomicWrite(m.ConfigPath, raw, 0600); err != nil {
			return err
		}
	}
	if _, err = nativecore.Decode(raw); err != nil {
		return err
	}
	return m.restart(context.Background())
}
func (n *nativeRuntime) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if name == "veloray-native" {
		if len(args) == 1 && args[0] == "version" {
			return []byte("VeloRay-native 1.0\n"), nil
		}
		if len(args) > 1 && args[0] == "api" {
			counters, _ := n.engine.Snapshot()
			return json.Marshal(map[string]any{"stat": counters})
		}
		return nil, errors.New("unsupported native command")
	}
	if name != "systemctl" || len(args) < 2 || args[1] != "veloray-native.service" {
		return nil, errors.New("native runtime only controls its own listeners")
	}
	switch args[0] {
	case "is-active":
		if n.running {
			return []byte("active\n"), nil
		}
		return []byte("inactive\n"), errors.New("native runtime is inactive")
	case "show":
		return []byte(fmt.Sprintf("MainPID=%d\nExecMainStartTimestampMonotonic=%d\n", os.Getpid(), n.started)), nil
	case "reset-failed":
		return nil, nil
	case "restart":
		raw, err := os.ReadFile(n.config)
		if err != nil {
			return nil, err
		}
		if err = n.engine.Apply(raw); err != nil {
			return nil, err
		}
		if n.started == 0 {
			n.started = time.Now().UnixNano()
		}
		n.running = true
		return nil, nil
	case "stop":
		if err := n.engine.Apply([]byte(`{"inbounds":[]}`)); err != nil {
			return nil, err
		}
		n.running = false
		return nil, nil
	}
	return nil, errors.New("unsupported native operation")
}
func (n *nativeRuntime) ports(ctx context.Context, raw []byte) ([]PortCheck, error) {
	cfg, err := nativecore.Decode(raw)
	if err != nil {
		return nil, err
	}
	checks := []PortCheck{}
	for _, in := range cfg.Inbounds {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		address := net.JoinHostPort(in.Listen, strconv.Itoa(in.Port))
		check := PortCheck{Tag: in.Tag, Listen: in.Listen, Port: in.Port, Network: "tcp", State: "free"}
		if in.Protocol == "amneziawg" {
			check.Network = "udp"
			if n.engine.OwnsUDP(address) {
				check.State = "owned"
				check.Owner = "native"
			} else {
				socket, err := net.ListenPacket("udp", address)
				if err != nil {
					check.State = "conflict"
					check.Owner = "another listener"
				} else {
					socket.Close()
				}
			}
			checks = append(checks, check)
			continue
		}

		if n.engine.Owns(address) {
			check.State = "owned"
			check.Owner = "native"
		} else {
			listener, err := net.Listen("tcp", address)
			if err != nil {
				check.State = "conflict"
				check.Owner = "another listener"
			} else {
				listener.Close()
			}
		}
		checks = append(checks, check)
	}
	return checks, nil
}
func (n *nativeRuntime) close() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.engine.Close()
	n.running = false
}

func nativePart(raw []byte) ([]byte, error) {
	var cfg map[string]json.RawMessage
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	return cfg["_native"], nil
}
