package nativecore

import (
	"errors"
	"net"
	"net/netip"
	"sync"

	"github.com/amnezia-vpn/amneziawg-go/conn"
)

// A scoped UDP bind avoids host netlink, interface changes and packet marks.
// The socket is reserved before committing a listener replacement.
type udpBind struct {
	mu       sync.Mutex
	host     string
	socket   *net.UDPConn
	reserved *net.UDPConn
}

func reserveBind(host string, port int) (*udpBind, error) {
	socket, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP(host), Port: port})
	if err != nil {
		return nil, err
	}
	return &udpBind{host: host, reserved: socket}, nil
}
func (b *udpBind) Open(port uint16) ([]conn.ReceiveFunc, uint16, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.socket != nil {
		return nil, 0, conn.ErrBindAlreadyOpen
	}
	socket := b.reserved
	b.reserved = nil
	if socket == nil {
		var err error
		socket, err = net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP(b.host), Port: int(port)})
		if err != nil {
			return nil, 0, err
		}
	}
	b.socket = socket
	read := func(packets [][]byte, sizes []int, endpoints []conn.Endpoint) (int, error) {
		n, addr, err := socket.ReadFromUDPAddrPort(packets[0])
		if err != nil {
			return 0, err
		}
		sizes[0] = n
		endpoints[0] = &conn.StdNetEndpoint{AddrPort: netip.AddrPortFrom(addr.Addr().Unmap(), addr.Port())}
		return 1, nil
	}
	return []conn.ReceiveFunc{read}, uint16(socket.LocalAddr().(*net.UDPAddr).Port), nil
}
func (b *udpBind) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.socket != nil {
		b.socket.Close()
		b.socket = nil
	}
	return nil
}
func (b *udpBind) dispose() {
	b.Close()
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.reserved != nil {
		b.reserved.Close()
		b.reserved = nil
	}
}
func (b *udpBind) SetMark(mark uint32) error {
	if mark != 0 {
		return errors.New("native UDP bind does not support host packet marks")
	}
	return nil
}
func (b *udpBind) Send(bufs [][]byte, endpoint conn.Endpoint) error {
	ep, ok := endpoint.(*conn.StdNetEndpoint)
	if !ok {
		return conn.ErrWrongEndpointType
	}
	b.mu.Lock()
	socket := b.socket
	b.mu.Unlock()
	if socket == nil {
		return net.ErrClosed
	}
	for _, buf := range bufs {
		if _, err := socket.WriteToUDPAddrPort(buf, ep.AddrPort); err != nil {
			return err
		}
	}
	return nil
}
func (*udpBind) ParseEndpoint(value string) (conn.Endpoint, error) {
	addr, err := netip.ParseAddrPort(value)
	if err != nil {
		return nil, err
	}
	return &conn.StdNetEndpoint{AddrPort: addr}, nil
}
func (*udpBind) BatchSize() int { return 1 }
