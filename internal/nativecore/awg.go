package nativecore

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"net/netip"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/amnezia-vpn/amneziawg-go/device"
	"github.com/sagernet/gvisor/pkg/tcpip/adapters/gonet"
	"github.com/sagernet/gvisor/pkg/tcpip/stack"
	"github.com/sagernet/gvisor/pkg/tcpip/transport/tcp"
	"github.com/sagernet/gvisor/pkg/tcpip/transport/udp"
	"github.com/sagernet/gvisor/pkg/waiter"
)

type awgFlow struct {
	user          string
	local, remote net.Conn
	cancel        context.CancelFunc
}
type awgRuntime struct {
	mu     sync.Mutex
	config Inbound
	engine *Engine
	dev    *device.Device
	tun    *packetTun
	bind   *udpBind
	users  map[netip.Addr]User
	last   map[string]time.Time
	flows  map[*awgFlow]bool
	slots  chan struct{}
	timers []*time.Timer
	active bool
}

func newAWG(e *Engine, in Inbound) (*awgRuntime, error) {
	b, err := reserveBind(in.Listen, in.Port)
	if err != nil {
		return nil, errors.New("cannot reserve AmneziaWG UDP port")
	}
	n := &awgRuntime{engine: e, bind: b, users: map[netip.Addr]User{}, last: map[string]time.Time{}, flows: map[*awgFlow]bool{}, slots: make(chan struct{}, 1024)}
	n.tun, err = newPacketTun(in.AWG.MTU, n.packet)
	if err != nil {
		b.dispose()
		return nil, err
	}
	n.dev = device.NewDevice(n.tun, b, &device.Logger{Verbosef: func(string, ...any) {}, Errorf: func(string, ...any) {}})
	if err = n.dev.IpcSet(awgUAPI(in, nil, true)); err != nil {
		n.close()
		return nil, errors.New("AmneziaWG configuration rejected")
	}
	if err = n.dev.Up(); err != nil {
		n.close()
		return nil, errors.New("cannot start AmneziaWG listener")
	}
	n.tun.stack.SetTransportProtocolHandler(tcp.ProtocolNumber, tcp.NewForwarder(n.tun.stack, 0, 128, n.forwardTCP).HandlePacket)
	n.tun.stack.SetTransportProtocolHandler(udp.ProtocolNumber, n.forwardUDP)
	n.setUsers(in)
	return n, nil
}
func (n *awgRuntime) setUsers(in Inbound) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, timer := range n.timers {
		timer.Stop()
	}
	n.timers = nil
	next := map[netip.Addr]User{}
	byName := map[string]User{}
	for _, u := range in.Users {
		prefix, _ := netip.ParsePrefix(u.Address)
		next[prefix.Addr()] = u
		byName[u.Name] = u
	}
	oldNames := map[string]User{}
	for _, u := range n.users {
		oldNames[u.Name] = u
	}
	for flow := range n.flows {
		if !reflect.DeepEqual(oldNames[flow.user], byName[flow.user]) || n.config.AWG != nil && n.config.AWG.PrivateKey != in.AWG.PrivateKey {
			flow.cancel()
			flow.local.Close()
			if flow.remote != nil {
				flow.remote.Close()
			}
		}
	}
	n.users = next
	n.config = in
	for _, u := range in.Users {
		if u.ExpiresAt != "" {
			at, _ := time.Parse(time.RFC3339, u.ExpiresAt)
			name := u.Name
			n.timers = append(n.timers, time.AfterFunc(max(0, time.Until(at)), func() { n.closeUser(name) }))
		}
	}
}
func (n *awgRuntime) closeUser(name string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for flow := range n.flows {
		if flow.user == name {
			flow.cancel()
			flow.local.Close()
			if flow.remote != nil {
				flow.remote.Close()
			}
		}
	}
}
func (n *awgRuntime) close() {
	n.mu.Lock()
	n.active = false
	for _, timer := range n.timers {
		timer.Stop()
	}
	for flow := range n.flows {
		flow.cancel()
		flow.local.Close()
		if flow.remote != nil {
			flow.remote.Close()
		}
	}
	n.mu.Unlock()
	if n.dev != nil {
		n.dev.Close()
	} else if n.tun != nil {
		n.tun.Close()
	}
	n.bind.dispose()
}
func packetIPs(data []byte) (source, dest netip.Addr, ok bool) {
	if len(data) >= 20 && data[0]>>4 == 4 {
		length := int(binary.BigEndian.Uint16(data[2:4]))
		if int(data[0]&15)*4 < 20 || length > len(data) || length < 20 {
			return
		}
		return netip.AddrFrom4([4]byte(data[12:16])), netip.AddrFrom4([4]byte(data[16:20])), true
	}
	if len(data) >= 40 && data[0]>>4 == 6 {
		if int(binary.BigEndian.Uint16(data[4:6]))+40 > len(data) {
			return
		}
		return netip.AddrFrom16([16]byte(data[8:24])), netip.AddrFrom16([16]byte(data[24:40])), true
	}
	return
}
func userActive(u User) bool {
	if u.ExpiresAt == "" {
		return true
	}
	at, err := time.Parse(time.RFC3339, u.ExpiresAt)
	return err == nil && time.Now().Before(at)
}
func (n *awgRuntime) packet(data []byte, upload bool) bool {
	source, dest, ok := packetIPs(data)
	if !ok {
		return false
	}
	address := dest
	if upload {
		address = source
	}
	n.mu.Lock()
	u, exists := n.users[address]
	allowed := n.active && exists && userActive(u)
	if allowed && upload {
		n.last[u.Name] = time.Now()
	}
	n.mu.Unlock()
	if !allowed {
		return false
	}
	a := n.engine.account(u.Name)
	a.mu.Lock()
	if upload {
		if a.up <= int64(^uint64(0)>>1)-int64(len(data)) {
			a.up += int64(len(data))
		}
	} else if a.down <= int64(^uint64(0)>>1)-int64(len(data)) {
		a.down += int64(len(data))
	}
	a.mu.Unlock()
	return true
}
func (n *awgRuntime) admitted(id stack.TransportEndpointID) (string, bool) {
	source, ok := netip.AddrFromSlice(id.RemoteAddress.AsSlice())
	if !ok || id.LocalPort == 0 || !publicIP(net.IP(id.LocalAddress.AsSlice())) {
		return "", false
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	u, ok := n.users[source]
	return u.Name, n.active && ok && userActive(u)
}
func (n *awgRuntime) forwardTCP(r *tcp.ForwarderRequest) {
	id := r.ID()
	name, ok := n.admitted(id)
	if !ok {
		r.Complete(true)
		return
	}
	select {
	case n.slots <- struct{}{}:
	default:
		r.Complete(true)
		return
	}
	var wq waiter.Queue
	ep, err := r.CreateEndpoint(&wq)
	r.Complete(err != nil)
	if err != nil {
		<-n.slots
		return
	}
	n.relay(name, "tcp", id, gonet.NewTCPConn(&wq, ep))
}
func (n *awgRuntime) forwardUDP(id stack.TransportEndpointID, pkt *stack.PacketBuffer) bool {
	name, ok := n.admitted(id)
	if !ok {
		return false
	}
	select {
	case n.slots <- struct{}{}:
	default:
		return true
	}
	var wq waiter.Queue
	// NewForwarderRequest uses this callback's packet synchronously. Unlike the
	// convenience UDP forwarder it does not retain an extra packet reference.
	ep, err := udp.NewForwarderRequest(n.tun.stack, id, pkt).CreateEndpoint(&wq)
	if err != nil {
		<-n.slots
		return true
	}
	go n.relay(name, "udp", id, gonet.NewUDPConn(&wq, ep))
	return true
}
func (n *awgRuntime) relay(name, network string, id stack.TransportEndpointID, local net.Conn) {
	defer func() { <-n.slots }()
	defer local.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	flow := &awgFlow{user: name, local: local, cancel: cancel}
	n.mu.Lock()
	allowed := false
	for _, u := range n.users {
		if u.Name == name && userActive(u) && n.active {
			allowed = true
			break
		}
	}
	if allowed {
		n.flows[flow] = true
	}
	n.mu.Unlock()
	if !allowed {
		return
	}
	defer func() { n.mu.Lock(); delete(n.flows, flow); n.mu.Unlock() }()
	target := net.JoinHostPort(id.LocalAddress.String(), strconv.Itoa(int(id.LocalPort)))
	remote, err := n.engine.network.DialContext(ctx, network, target)
	if err != nil {
		return
	}
	defer remote.Close()
	n.mu.Lock()
	flow.remote = remote
	n.mu.Unlock()
	if ctx.Err() != nil {
		return
	}
	idle := 2 * time.Minute
	if network == "udp" {
		idle = 45 * time.Second
	}
	copyFlow := func(dst, src net.Conn) error {
		buf := make([]byte, 65535)
		for {
			src.SetReadDeadline(time.Now().Add(idle))
			amount, err := src.Read(buf)
			if amount > 0 {
				dst.SetWriteDeadline(time.Now().Add(8 * time.Second))
				if _, writeErr := dst.Write(buf[:amount]); writeErr != nil {
					return writeErr
				}
			}
			if err != nil {
				return err
			}
		}
	}
	done := make(chan struct{})
	go func() { _ = copyFlow(remote, local); remote.Close(); close(done) }()
	_ = copyFlow(local, remote)
	local.Close()
	remote.Close()
	<-done
}
func (n *awgRuntime) online() map[string]map[string]int64 {
	raw, err := n.dev.IpcGet()
	if err != nil {
		return nil
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	names := map[string]string{}
	for _, u := range n.users {
		if userActive(u) {
			names[u.PublicKey] = u.Name
		}
	}
	result := map[string]map[string]int64{}
	key := ""
	for _, line := range strings.Split(raw, "\n") {
		field, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if field == "public_key" {
			key = value
		}
		if field == "endpoint" {
			name := names[key]
			host, _, err := net.SplitHostPort(value)
			last := n.last[name]
			if name != "" && err == nil && time.Since(last) < 90*time.Second && n.active {
				result[name] = map[string]int64{host: last.Unix()}
			}
		}
	}
	return result
}
