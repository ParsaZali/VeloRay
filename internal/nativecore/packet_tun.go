package nativecore

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"

	"github.com/amnezia-vpn/amneziawg-go/tun"
	"github.com/sagernet/gvisor/pkg/buffer"
	"github.com/sagernet/gvisor/pkg/tcpip"
	"github.com/sagernet/gvisor/pkg/tcpip/header"
	"github.com/sagernet/gvisor/pkg/tcpip/link/channel"
	"github.com/sagernet/gvisor/pkg/tcpip/network/ipv4"
	"github.com/sagernet/gvisor/pkg/tcpip/network/ipv6"
	"github.com/sagernet/gvisor/pkg/tcpip/stack"
	"github.com/sagernet/gvisor/pkg/tcpip/transport/tcp"
	"github.com/sagernet/gvisor/pkg/tcpip/transport/udp"
)

type packetTun struct {
	stack  *stack.Stack
	ep     *channel.Endpoint
	ctx    context.Context
	cancel context.CancelFunc
	events chan tun.Event
	mtu    atomic.Int64
	close  sync.Once
	allow  func([]byte, bool) bool
}

func newPacketTun(mtu int, allow func([]byte, bool) bool) (*packetTun, error) {
	ctx, cancel := context.WithCancel(context.Background())
	t := &packetTun{ctx: ctx, cancel: cancel, events: make(chan tun.Event, 2), ep: channel.New(1024, uint32(mtu), ""), allow: allow}
	t.mtu.Store(int64(mtu))
	t.stack = stack.New(stack.Options{NetworkProtocols: []stack.NetworkProtocolFactory{ipv4.NewProtocol, ipv6.NewProtocol}, TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol, udp.NewProtocol}, HandleLocal: false})
	if err := t.stack.CreateNIC(1, t.ep); err != nil {
		t.Close()
		return nil, errors.New("cannot initialize native IP stack")
	}
	_ = t.stack.SetPromiscuousMode(1, true)
	_ = t.stack.SetSpoofing(1, true)
	t.stack.SetRouteTable([]tcpip.Route{{Destination: header.IPv4EmptySubnet, NIC: 1}, {Destination: header.IPv6EmptySubnet, NIC: 1}})
	return t, nil
}
func (*packetTun) File() *os.File             { return nil }
func (*packetTun) Name() (string, error)      { return "veloray-userspace", nil }
func (t *packetTun) Events() <-chan tun.Event { return t.events }
func (t *packetTun) MTU() (int, error)        { return int(t.mtu.Load()), nil }
func (*packetTun) BatchSize() int             { return 1 }
func (t *packetTun) setMTU(mtu int) {
	t.mtu.Store(int64(mtu))
	t.ep.SetMTU(uint32(mtu))
	select {
	case t.events <- tun.EventMTUUpdate:
	default:
	}
}
func (t *packetTun) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	for {
		pkt := t.ep.ReadContext(t.ctx)
		if pkt == nil {
			return 0, os.ErrClosed
		}
		view := pkt.ToView()
		data := append([]byte{}, view.AsSlice()...)
		view.Release()
		pkt.DecRef()
		if t.allow != nil && !t.allow(data, false) {
			continue
		}
		if len(data) > len(bufs[0])-offset {
			return 0, errors.New("native packet exceeds buffer")
		}
		sizes[0] = copy(bufs[0][offset:], data)
		return 1, nil
	}
}
func (t *packetTun) Write(bufs [][]byte, offset int) (int, error) {
	for _, buf := range bufs {
		data := buf[offset:]
		if len(data) == 0 || t.allow != nil && !t.allow(data, true) {
			continue
		}
		var protocol tcpip.NetworkProtocolNumber
		switch data[0] >> 4 {
		case 4:
			protocol = ipv4.ProtocolNumber
		case 6:
			protocol = ipv6.ProtocolNumber
		default:
			continue
		}
		pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{Payload: buffer.MakeWithData(append([]byte{}, data...))})
		t.ep.InjectInbound(protocol, pkt)
		pkt.DecRef()
	}
	return len(bufs), nil
}
func (t *packetTun) Close() error {
	t.close.Do(func() { t.cancel(); t.stack.Close(); t.ep.Close(); t.ep.Drain(); close(t.events) })
	return nil
}
