package agent

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Runtime interface {
	Run(context.Context, string, ...string) ([]byte, error)
}
type Commands struct{}

func (Commands) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

type Status struct {
	Installed bool   `json:"installed"`
	Running   bool   `json:"running"`
	Version   string `json:"version"`
	State     string `json:"state"`
}

// PortCheck is a read-only snapshot of a configured listener and its current owner.
type PortCheck struct {
	Tag     string `json:"tag"`
	Listen  string `json:"listen"`
	Port    int    `json:"port"`
	Network string `json:"network"`
	State   string `json:"state"`
	Owner   string `json:"owner,omitempty"`
}

var socketPID = regexp.MustCompile(`pid=([0-9]+)`)

func InspectConfigPorts(ctx context.Context, run Runtime, service string, raw []byte) ([]PortCheck, error) {
	var cfg struct {
		Inbounds []struct {
			Tag, Listen, Protocol string
			Port                  json.RawMessage
			Settings              struct {
				Network string
				UDP     bool
			}
			StreamSettings struct{ Network string }
		}
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	checks := []PortCheck{}
	for _, in := range cfg.Inbounds {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if in.Protocol == "tun" || strings.HasPrefix(in.Listen, "/") {
			continue
		}
		portText := strings.Trim(string(in.Port), `"`)
		if portText == "" || portText == "null" || portText == "0" {
			continue
		}
		bounds := strings.Split(portText, "-")
		first, err := strconv.Atoi(bounds[0])
		if err != nil {
			return nil, fmt.Errorf("%s: invalid port", in.Tag)
		}
		last := first
		if len(bounds) == 2 {
			last, err = strconv.Atoi(bounds[1])
		}
		if err != nil || len(bounds) > 2 || first < 1 || last > 65535 || last < first {
			return nil, fmt.Errorf("%s: invalid port range", in.Tag)
		}
		listen := in.Listen
		if listen == "" {
			listen = "0.0.0.0"
		}
		if net.ParseIP(listen) == nil {
			return nil, fmt.Errorf("%s: port checks require an IP listen address", in.Tag)
		}
		networks := []string{"tcp"}
		switch {
		case in.Protocol == "tuic" || in.Protocol == "wireguard" || in.Protocol == "hysteria" || in.StreamSettings.Network == "kcp" || in.StreamSettings.Network == "mkcp" || in.StreamSettings.Network == "quic" || in.StreamSettings.Network == "hysteria":
			networks = []string{"udp"}
		case in.Protocol == "shadowsocks":
			networks = []string{"tcp", "udp"}
		case in.Protocol == "dokodemo-door":
			if in.Settings.Network != "" {
				networks = strings.Split(in.Settings.Network, ",")
			}
		case in.Protocol == "socks" && in.Settings.UDP:
			networks = []string{"tcp", "udp"}
		}
		for port := first; port <= last; port++ {
			for _, network := range networks {
				if network != "tcp" && network != "udp" {
					return nil, fmt.Errorf("%s: unsupported listen network", in.Tag)
				}
				checks = append(checks, PortCheck{Tag: in.Tag, Listen: listen, Port: port, Network: network, State: "free"})
			}
		}
	}
	if len(checks) == 0 {
		return checks, nil
	}
	var mainPID string
	if pid, err := run.Run(ctx, "systemctl", "show", service, "--property=MainPID", "--value"); err == nil {
		mainPID = strings.TrimSpace(string(pid))
		if strings.HasPrefix(mainPID, "MainPID=") {
			mainPID = strings.Split(strings.TrimPrefix(mainPID, "MainPID="), "\n")[0]
		}
		if mainPID == "0" {
			mainPID = ""
		}
	}
	output, err := run.Run(ctx, "ss", "-H", "-l", "-n", "-t", "-u", "-p", "-O")
	if err != nil {
		return nil, fmt.Errorf("cannot inspect listening ports; install iproute2: %w", err)
	}
	for _, line := range strings.Split(string(output), "\n") {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		f := strings.Fields(line)
		if len(f) < 6 {
			continue
		}
		host, portText, err := net.SplitHostPort(f[4])
		if err != nil {
			continue
		}
		port, err := strconv.Atoi(portText)
		if err != nil {
			continue
		}
		owner := "owner unavailable"
		if len(f) > 6 {
			owner = strings.Join(f[6:], " ")
		}
		pids := socketPID.FindAllStringSubmatch(owner, -1)
		ours := len(pids) > 0 && mainPID != ""
		for _, pid := range pids {
			if pid[1] != mainPID {
				ours = false
			}
		}
		for i := range checks {
			c := &checks[i]
			if c.Port != port || c.Network != f[0] || !listenOverlap(c.Listen, host) {
				continue
			}
			state := "conflict"
			if ours {
				state = "xray"
			}
			if c.State != "conflict" {
				c.State = state
				c.Owner = owner
			}
		}
	}
	// Detect overlap inside the proposed configuration before touching the runtime.
	seen := map[string][]int{}
	for i := range checks {
		key := checks[i].Network + ":" + strconv.Itoa(checks[i].Port)
		for _, j := range seen[key] {
			if checks[i].Port == checks[j].Port && checks[i].Network == checks[j].Network && listenOverlap(checks[i].Listen, checks[j].Listen) {
				checks[i].State = "conflict"
				checks[i].Owner = "configured listener " + checks[j].Tag
				break
			}
		}
		seen[key] = append(seen[key], i)
	}
	return checks, nil
}

func listenOverlap(a, b string) bool {
	a = strings.Split(a, "%")[0]
	b = strings.Split(b, "%")[0]
	if a == "*" || b == "*" {
		return true
	}
	x, y := net.ParseIP(a), net.ParseIP(b)
	if x == nil || y == nil {
		return false
	}
	if x.Equal(y) {
		return true
	}
	if x.IsUnspecified() || y.IsUnspecified() {
		// IPv6 wildcard listeners may also accept IPv4 connections.
		if x.IsUnspecified() && x.To4() == nil || y.IsUnspecified() && y.To4() == nil {
			return true
		}
		return (x.To4() == nil) == (y.To4() == nil)
	}
	return false
}

func RequireFreePorts(checks []PortCheck) error {
	for _, c := range checks {
		if c.State == "conflict" {
			return fmt.Errorf("port conflict: %s %s:%d (%s), %s; choose another inbound port", c.Network, c.Listen, c.Port, c.Tag, c.Owner)
		}
	}
	return nil
}

func (m *Manager) Ports(ctx context.Context, raw []byte) ([]PortCheck, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(raw) == 0 {
		var err error
		raw, err = os.ReadFile(m.ConfigPath)
		if err != nil {
			return nil, err
		}
	}
	return m.inspectPorts(ctx, raw)
}

type Counter struct {
	Name  string `json:"name"`
	Value int64  `json:"value"`
	Core  string `json:"core,omitempty"`
}
type Snapshot struct {
	OutboundAccounting     string                      `json:"outbound_accounting"`
	CoreLedgers            map[string]string           `json:"core_ledgers"`
	Stat                   []Counter                   `json:"stat"`
	Accounting             string                      `json:"accounting"`
	LedgerID               string                      `json:"ledger_id"`
	OnlineUsers            []string                    `json:"online_users"`
	OnlineIPs              map[string]map[string]int64 `json:"online_ips"`
	OnlineIPsAvailable     bool                        `json:"online_ips_available"`
	OnlineIPCoreAvailable  map[string]bool             `json:"online_ip_cores"`
	OnlineAvailable        bool                        `json:"online_available"`
	OnlineTrackingRequired bool                        `json:"online_tracking_required"`
	ObservedAt             string                      `json:"observed_at"`
}
type counterState struct {
	Raw   int64  `json:"raw"`
	Total int64  `json:"total"`
	Epoch string `json:"epoch"`
}
type operation struct {
	Before []byte    `json:"before"`
	After  string    `json:"after"`
	State  string    `json:"state"`
	At     time.Time `json:"at"`
}
type ledger struct {
	ID         string                  `json:"id"`
	Counters   map[string]counterState `json:"counters"`
	Operations map[string]operation    `json:"operations"`
}
type Manager struct {
	Binary, ConfigPath, ServiceName, StatePath string
	Core                                       string
	Runtime                                    Runtime
	mu                                         sync.Mutex
	state                                      ledger
}

func NewManager(binary, config, service, state string, run Runtime) (*Manager, error) {
	if run == nil {
		run = Commands{}
	}
	m := &Manager{Binary: binary, ConfigPath: config, ServiceName: service, StatePath: state, Runtime: run}
	raw, err := os.ReadFile(state)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err == nil {
		if err = json.Unmarshal(raw, &m.state); err != nil {
			return nil, fmt.Errorf("accounting state is corrupt: %w", err)
		}
	} else {
		b := make([]byte, 16)
		if _, err = rand.Read(b); err != nil {
			return nil, err
		}
		m.state.ID = hex.EncodeToString(b)
	}
	if m.state.ID == "" {
		return nil, errors.New("missing accounting ledger identity")
	}
	if m.state.Counters == nil {
		m.state.Counters = map[string]counterState{}
	}
	if m.state.Operations == nil {
		m.state.Operations = map[string]operation{}
	}
	if err = m.persist(); err != nil {
		return nil, err
	}
	return m, nil
}
func atomicWrite(path string, raw []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".veloray-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(raw)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	d, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
func (m *Manager) persist() error {
	b, err := json.Marshal(m.state)
	if err != nil {
		return err
	}
	return atomicWrite(m.StatePath, b, 0600)
}
func (m *Manager) status(ctx context.Context) Status {
	v, _ := m.Runtime.Run(ctx, m.Binary, "version")
	fields := strings.Fields(string(v))
	version := ""
	if len(fields) >= 2 {
		version = strings.Join(fields[:2], " ")
	}
	a, e := m.Runtime.Run(ctx, "systemctl", "is-active", m.ServiceName)
	state := strings.TrimSpace(string(a))
	if state == "" {
		state = "unknown"
	}
	return Status{Installed: version != "", Running: e == nil && state == "active", Version: version, State: state}
}
func (m *Manager) Status(ctx context.Context) Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status(ctx)
}
func (m *Manager) epoch(ctx context.Context) (string, error) {
	b, e := m.Runtime.Run(ctx, "systemctl", "show", m.ServiceName, "--property=MainPID,ExecMainStartTimestampMonotonic")
	if e != nil {
		return "", e
	}
	s := strings.TrimSpace(string(b))
	if s == "" || strings.Contains(s, "MainPID=0\n") || strings.HasSuffix(s, "MainPID=0") {
		return "", errors.New("Xray is not running")
	}
	return s, nil
}
func (m *Manager) observe(ctx context.Context) error {
	epoch, err := m.epoch(ctx)
	if err != nil {
		return err
	}
	var b []byte
	if m.Core == "singbox" {
		b, err = m.querySingStats(ctx)
	} else {
		b, err = m.Runtime.Run(ctx, m.Binary, "api", "statsquery", "--server=127.0.0.1:10085")
	}
	if err != nil && m.Core != "singbox" {
		b, err = m.Runtime.Run(ctx, m.Binary, "api", "statsquery", "--server=127.0.1.0:10085")
	} // Upgrade bridge for v0.1.0 loopback configuration.
	if err != nil {
		return fmt.Errorf("Xray stats unavailable: %w", err)
	}
	var v struct {
		Stat []Counter `json:"stat"`
	}
	if err = json.Unmarshal(b, &v); err != nil {
		return fmt.Errorf("invalid Xray stats: %w", err)
	}
	old := m.state.Counters
	next := make(map[string]counterState, len(old))
	for k, v := range old {
		next[k] = v
	}
	for _, s := range v.Stat {
		if s.Value < 0 {
			return errors.New("negative Xray counter")
		}
		prev := next[s.Name]
		delta := s.Value
		if prev.Epoch == epoch && s.Value >= prev.Raw {
			delta = s.Value - prev.Raw
		}
		if delta > 0 && prev.Total > int64(^uint64(0)>>1)-delta {
			return errors.New("traffic counter overflow")
		}
		next[s.Name] = counterState{s.Value, prev.Total + delta, epoch}
	}
	m.state.Counters = next
	if err = m.persist(); err != nil {
		m.state.Counters = old
		return err
	}
	return nil
}
func (m *Manager) Stats(ctx context.Context) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	running := m.status(ctx).Running
	if running {
		if err := m.observe(ctx); err != nil {
			return Snapshot{}, err
		}
	}
	s := Snapshot{Accounting: "durable-v1", OutboundAccounting: "core-scoped-v1", LedgerID: m.state.ID, Stat: []Counter{}, OnlineUsers: []string{}, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	s.OnlineIPs = map[string]map[string]int64{}
	if m.Core == "singbox" {
		if running {
			var onlineErr error
			s.OnlineIPs, onlineErr = m.singOnlineIPs(ctx)
			s.OnlineIPsAvailable = onlineErr == nil
			for name := range s.OnlineIPs {
				s.OnlineUsers = append(s.OnlineUsers, name)
			}
		}
		s.OnlineAvailable = s.OnlineIPsAvailable || !running
		s.OnlineIPsAvailable = s.OnlineAvailable
	} else if !running {
		s.OnlineIPsAvailable = true
		s.OnlineAvailable = true
	} else {
		var cfg struct {
			Policy struct {
				Levels map[string]struct {
					StatsUserOnline bool `json:"statsUserOnline"`
				} `json:"levels"`
			} `json:"policy"`
		}
		raw, err := os.ReadFile(m.ConfigPath)
		if err == nil && json.Unmarshal(raw, &cfg) == nil {
			s.OnlineTrackingRequired = !cfg.Policy.Levels["0"].StatsUserOnline
			if !s.OnlineTrackingRequired {
				onlineCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
				defer cancel()
				out, err := m.Runtime.Run(onlineCtx, m.Binary, "api", "statsgetallonlineusers", "--server=127.0.0.1:10085", "--timeout=2")
				var users struct {
					Users []string `json:"users"`
				}
				if err == nil && json.Unmarshal(out, &users) == nil {
					s.OnlineAvailable = true
					for _, name := range users.Users {
						parts := strings.Split(name, ">>>")
						if len(parts) == 3 && parts[0] == "user" && parts[2] == "online" {
							name = parts[1]
						}
						s.OnlineUsers = append(s.OnlineUsers, name)
					}
				}
			}
		}
	}
	if running && m.Core != "singbox" {
		if _, ok := m.Runtime.(Commands); ok {
			if ips, e := xrayOnlineIPs(ctx); e == nil {
				s.OnlineIPs = ips
				s.OnlineIPsAvailable = true
				s.OnlineAvailable = true
				s.OnlineUsers = []string{}
				for name := range ips {
					s.OnlineUsers = append(s.OnlineUsers, name)
				}
			}
		}
	}
	s.OnlineIPCoreAvailable = map[string]bool{fallbackCore(m.Core): s.OnlineIPsAvailable}
	for k, v := range m.state.Counters {
		s.Stat = append(s.Stat, Counter{Name: k, Value: v.Total, Core: fallbackCore(m.Core)})
	}
	return s, nil
}
func (m *Manager) Validate(ctx context.Context, raw []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.validate(ctx, raw)
}
func (m *Manager) validate(ctx context.Context, raw []byte) error {
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil || value == nil {
		return errors.New("config must be a JSON object")
	}
	checks, err := m.inspectPorts(ctx, raw)
	if err != nil {
		return err
	}
	if err = RequireFreePorts(checks); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.ConfigPath), 0700); err != nil {
		return err
	}
	f, e := os.CreateTemp(filepath.Dir(m.ConfigPath), ".validate-*.json")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(raw)
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	args := []string{"run", "-test", "-config", f.Name()}
	if m.Core == "singbox" {
		args = []string{"check", "-c", f.Name()}
	}
	out, e := m.Runtime.Run(ctx, m.Binary, args...)
	if e != nil {
		return fmt.Errorf("Xray rejected config: %.2000s", out)
	}
	return nil
}
func hash(raw []byte) string { v := sha256.Sum256(raw); return hex.EncodeToString(v[:]) }
func (m *Manager) restart(ctx context.Context) error {
	if raw, err := os.ReadFile(m.ConfigPath); err == nil {
		checks, err := m.inspectPorts(ctx, raw)
		if err != nil {
			return err
		}
		if err = RequireFreePorts(checks); err != nil {
			return err
		}
	} else {
		return err
	}
	if _, real := m.Runtime.(Commands); real && os.Geteuid() == 0 {
		name := "nogroup"
		if value, err := m.Runtime.Run(ctx, "systemctl", "show", m.ServiceName, "--property=Group", "--value"); err == nil && strings.TrimSpace(string(value)) != "" {
			name = strings.TrimSpace(string(value))
		}
		group, err := user.LookupGroup(name)
		if err != nil {
			return fmt.Errorf("Xray service group %s: %w", name, err)
		}
		gid, err := strconv.Atoi(group.Gid)
		if err != nil {
			return err
		}
		if err = os.Chown(m.ConfigPath, 0, gid); err != nil {
			return err
		}
	}

	_, _ = m.Runtime.Run(ctx, "systemctl", "reset-failed", m.ServiceName)
	out, e := m.Runtime.Run(ctx, "systemctl", "restart", m.ServiceName)
	if e != nil {
		return fmt.Errorf("Xray restart failed: %.2000s", out)
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if m.status(ctx).Running {
			if e := m.observe(ctx); e == nil {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return errors.New("Xray did not become active")
}
func (m *Manager) Apply(ctx context.Context, id string, raw []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(id) < 16 || len(id) > 128 {
		return errors.New("operation_id must be 16–128 characters")
	}
	prior, known := m.state.Operations[id]
	if known {
		if prior.After != hash(raw) {
			return errors.New("operation_id reused with another config")
		}
		if prior.State == "rolled_back" || prior.State == "rollback_failed" {
			return errors.New("operation has already been rolled back")
		}
		if prior.State == "applied" {
			current, e := os.ReadFile(m.ConfigPath)
			if e == nil && hash(current) == prior.After {
				return nil
			}
			return errors.New("operation superseded by another config")
		}
	}
	if err := m.validate(ctx, raw); err != nil {
		return err
	}
	// A running runtime must be observed before any restart. Failing closed avoids losing quota data.
	if m.status(ctx).Running {
		if e := m.observe(ctx); e != nil {
			return e
		}
	}
	before, e := os.ReadFile(m.ConfigPath)
	if e != nil && !errors.Is(e, os.ErrNotExist) {
		return e
	}
	if known {
		if hash(before) != prior.After && hash(before) != hash(prior.Before) {
			return errors.New("prepared operation was superseded")
		}
		before = prior.Before
	}
	op := operation{Before: before, After: hash(raw), State: "prepared", At: time.Now().UTC()}
	if hash(before) == hash(raw) && m.status(ctx).Running {
		op.State = "applied"
		m.state.Operations[id] = op
		return m.persist()
	}
	for key, item := range m.state.Operations {
		if time.Since(item.At) > 7*24*time.Hour && oneTerminal(item.State) {
			delete(m.state.Operations, key)
		}
	}
	m.state.Operations[id] = op
	if e = m.persist(); e != nil {
		return e
	}
	additions, live := liveUserAdditions(before, raw)
	live = live && m.Core != "singbox" && m.status(ctx).Running
	if e = atomicWrite(m.ConfigPath, raw, 0640); e == nil {
		if live {
			e = m.addUsersLive(ctx, additions)
		} else {
			e = m.restart(ctx)
		}
	}
	if e != nil {
		rollbackErr := m.restore(ctx, before)
		op.State = "rolled_back"
		if rollbackErr != nil {
			op.State = "rollback_failed"
		}
		m.state.Operations[id] = op
		saveErr := m.persist()
		return errors.Join(e, rollbackErr, saveErr)
	}
	op.State = "applied"
	m.state.Operations[id] = op
	return m.persist()
}
func (m *Manager) restore(ctx context.Context, b []byte) error {
	if len(b) == 0 {
		if e := os.Remove(m.ConfigPath); e != nil && !errors.Is(e, os.ErrNotExist) {
			return e
		}
		_, e := m.Runtime.Run(ctx, "systemctl", "stop", m.ServiceName)
		return e
	}
	if e := atomicWrite(m.ConfigPath, b, 0640); e != nil {
		return e
	}
	return m.restart(ctx)
}
func (m *Manager) Rollback(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	op, ok := m.state.Operations[id]
	if !ok {
		return nil
	} // The request never reached this agent; nothing was changed.
	if op.State == "rolled_back" {
		return nil
	}
	current, e := os.ReadFile(m.ConfigPath)
	if e != nil {
		return e
	}
	if hash(current) != op.After {
		if hash(current) == hash(op.Before) && (op.State == "prepared" || op.State == "rollback_failed") {
			op.State = "rolled_back"
			m.state.Operations[id] = op
			return m.persist()
		}
		return errors.New("refusing rollback: config changed since this operation")
	}
	if hash(current) == hash(op.Before) {
		op.State = "rolled_back"
		m.state.Operations[id] = op
		return m.persist()
	}
	if m.status(ctx).Running {
		if e = m.observe(ctx); e != nil {
			return e
		}
	}
	if e = m.restore(ctx, op.Before); e != nil {
		op.State = "rollback_failed"
		m.state.Operations[id] = op
		_ = m.persist()
		return e
	}
	op.State = "rolled_back"
	m.state.Operations[id] = op
	return m.persist()
}
func (m *Manager) Restart(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status(ctx).Running {
		if e := m.observe(ctx); e != nil {
			return e
		}
	}
	return m.restart(ctx)
}
func (m *Manager) Keypair(ctx context.Context, wireguard bool) (map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cmd := "x25519"
	if wireguard {
		cmd = "wg"
	}
	out, e := m.Runtime.Run(ctx, m.Binary, cmd)
	if e != nil {
		return nil, e
	}
	v := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		p := strings.SplitN(line, ":", 2)
		if len(p) != 2 {
			continue
		}
		k := strings.ToLower(p[0])
		if strings.Contains(k, "private") {
			v["private_key"] = strings.TrimSpace(p[1])
		}
		if strings.Contains(k, "public") || strings.Contains(k, "password") {
			v["public_key"] = strings.TrimSpace(p[1])
		}
	}
	if len(v) != 2 {
		return nil, errors.New("unexpected Xray key output")
	}
	return v, nil
}
func (m *Manager) Poll(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.mu.Lock()
			_ = m.observe(ctx)
			m.mu.Unlock()
		}
	}
}

func oneTerminal(state string) bool { return state == "applied" || state == "rolled_back" }

func (m *Manager) hasOperation(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.state.Operations[id]
	return ok
}

func fallbackCore(core string) string {
	if core == "singbox" {
		return core
	}
	return "xray"
}
