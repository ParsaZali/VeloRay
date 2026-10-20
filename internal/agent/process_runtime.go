package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type processSpec struct {
	binary, config string
	core           string
}
type managedProcess struct {
	cmd      *exec.Cmd
	done     chan struct{}
	started  int64
	expected bool
}

// ProcessRuntime owns only explicitly registered core children. It emulates the
// small service-control contract used by Manager, without a host service socket,
// shell, privileged container or permission to control unrelated host processes.
type ProcessRuntime struct {
	mu       sync.Mutex
	specs    map[string]processSpec
	children map[string]*managedProcess
	closed   bool
	failure  chan error
}

func NewProcessRuntime() *ProcessRuntime {
	return &ProcessRuntime{specs: map[string]processSpec{}, children: map[string]*managedProcess{}, failure: make(chan error, 1)}
}
func (p *ProcessRuntime) register(m *Manager) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.specs[m.ServiceName].binary != "" {
		return errors.New("duplicate or closed managed core")
	}
	p.specs[m.ServiceName] = processSpec{m.Binary, m.ConfigPath, m.Core}
	return nil
}
func processAlive(child *managedProcess) bool {
	if child == nil {
		return false
	}
	select {
	case <-child.done:
		return false
	default:
		return true
	}
}
func (p *ProcessRuntime) stopLocked(service string) {
	child := p.children[service]
	if child == nil {
		return
	}
	child.expected = true
	if processAlive(child) {
		_ = child.cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-child.done:
		case <-time.After(2 * time.Second):
			_ = child.cmd.Process.Kill()
			<-child.done
		}
	}
	delete(p.children, service)
}
func (p *ProcessRuntime) startLocked(service string) error {
	spec := p.specs[service]
	args := []string{"run", "-c", spec.config}
	cmd := exec.Command(spec.binary, args...)
	// Runtime output can contain customer metadata. HTTP diagnostics deliberately
	// expose structured state only; do not copy raw core output into Docker logs.
	cmd.Stdout, cmd.Stderr, cmd.Stdin = io.Discard, io.Discard, nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", service, err)
	}
	child := &managedProcess{cmd: cmd, done: make(chan struct{}), started: time.Now().UnixNano()}
	p.children[service] = child
	go func() {
		_ = cmd.Wait()
		close(child.done)
		p.mu.Lock()
		defer p.mu.Unlock()
		if !child.expected && !p.closed && p.children[service] == child {
			select {
			case p.failure <- fmt.Errorf("managed core %s exited unexpectedly", service):
			default:
			}
		}
	}()
	return nil
}
func (p *ProcessRuntime) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if name != "systemctl" {
		return (Commands{}).Run(ctx, name, args...)
	}
	if len(args) < 2 {
		return nil, errors.New("invalid managed service command")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	action, service := args[0], args[1]
	if p.closed || p.specs[service].binary == "" {
		return nil, errors.New("unregistered managed service")
	}
	child := p.children[service]
	active := processAlive(child)
	pid, epoch := 0, int64(0)
	if active {
		pid, epoch = child.cmd.Process.Pid, child.started
	}
	switch action {
	case "is-active":
		if active {
			return []byte("active\n"), nil
		}
		return []byte("inactive\n"), errors.New("core is inactive")
	case "show":
		if len(args) == 4 && args[2] == "--property=MainPID" && args[3] == "--value" {
			return []byte(strconv.Itoa(pid) + "\n"), nil
		}
		if len(args) == 4 && args[2] == "--property=Group" && args[3] == "--value" {
			return []byte("\n"), nil
		}
		if len(args) == 3 && args[2] == "--property=MainPID,ExecMainStartTimestampMonotonic" {
			return []byte(fmt.Sprintf("MainPID=%d\nExecMainStartTimestampMonotonic=%d\n", pid, epoch)), nil
		}
	case "reset-failed":
		return nil, nil
	case "restart":
		if len(args) != 2 {
			break
		}
		p.stopLocked(service)
		return nil, p.startLocked(service)
	case "stop":
		if len(args) != 2 {
			break
		}
		p.stopLocked(service)
		return nil, nil
	}
	return nil, errors.New("unsupported managed service command")
}
func (p *ProcessRuntime) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	for service := range p.children {
		p.stopLocked(service)
	}
}

func initializeProcessCore(m *Manager) error {
	// A prepared operation never received a durable success acknowledgment. On
	// process-mode boot, restore its saved configuration before opening listeners.
	if current, err := os.ReadFile(m.ConfigPath); err == nil {
		id := ""
		var latest operation
		for key, op := range m.state.Operations {
			if (op.State == "prepared" || op.State == "rollback_failed") && hash(current) == op.After && (id == "" || op.At.After(latest.At) || op.At.Equal(latest.At) && key > id) {
				id, latest = key, op
			}
		}
		if id != "" {
			if len(latest.Before) == 0 {
				if err = os.Remove(m.ConfigPath); err != nil {
					return err
				}
			} else if err = atomicWrite(m.ConfigPath, latest.Before, 0640); err != nil {
				return err
			}
			latest.State = "rolled_back"
			m.state.Operations[id] = latest
			if err = m.persist(); err != nil {
				return err
			}
		}
	}
	if _, err := os.Stat(m.ConfigPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var raw string
	if m.Core == "singbox" {
		raw = `{"log":{"level":"warn"},"inbounds":[],"outbounds":[{"type":"direct","tag":"direct"}],"route":{"final":"direct"},"experimental":{"v2ray_api":{"listen":"127.0.0.1:10086","stats":{"enabled":true,"users":[],"inbounds":[],"outbounds":["direct"]}},"clash_api":{"external_controller":"127.0.0.1:10087","secret":""}}}`
	} else {
		raw = `{"log":{"loglevel":"warning"},"api":{"tag":"api","services":["StatsService","HandlerService"]},"stats":{},"policy":{"levels":{"0":{"statsUserUplink":true,"statsUserDownlink":true,"statsUserOnline":true}},"system":{"statsInboundUplink":true,"statsInboundDownlink":true,"statsOutboundUplink":true,"statsOutboundDownlink":true}},"inbounds":[{"tag":"api","listen":"127.0.0.1","port":10085,"protocol":"dokodemo-door","settings":{"address":"127.0.0.1"}}],"outbounds":[{"protocol":"freedom","tag":"direct"},{"protocol":"blackhole","tag":"blocked"}],"routing":{"rules":[{"type":"field","inboundTag":["api"],"outboundTag":"api"}]}}`
	}
	return atomicWrite(m.ConfigPath, []byte(strings.TrimSpace(raw)), 0640)
}
