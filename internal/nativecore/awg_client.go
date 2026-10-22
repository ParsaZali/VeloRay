package nativecore

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"sync"

	"github.com/amnezia-vpn/amneziawg-go/device"
	"github.com/sagernet/gvisor/pkg/tcpip"
	"github.com/sagernet/gvisor/pkg/tcpip/adapters/gonet"
	"github.com/sagernet/gvisor/pkg/tcpip/network/ipv4"
	"github.com/sagernet/gvisor/pkg/tcpip/network/ipv6"
	"github.com/sagernet/gvisor/pkg/tcpip/stack"
)

type AWGClientConfig struct {
	Settings     AWGSettings `json:"settings"`
	PublicKey    string      `json:"public_key"`
	PresharedKey string      `json:"preshared_key,omitempty"`
	Address      string      `json:"address"`
	Endpoint     string      `json:"endpoint"`
}
type AWGClient struct {
	tun   *packetTun
	dev   *device.Device
	bind  *udpBind
	close sync.Once
}

func NewAWGClient(config AWGClientConfig, endpoint netip.AddrPort) (*AWGClient, error) {
	in := Inbound{Protocol: "amneziawg", AWG: &config.Settings, Users: []User{{PublicKey: config.PublicKey, PresharedKey: config.PresharedKey, Address: config.Address}}}
	if err := validateAWG(in); err != nil {
		return nil, err
	}
	if !endpoint.IsValid() || endpoint.Port() == 0 {
		return nil, errors.New("invalid AmneziaWG server endpoint")
	}
	host := "0.0.0.0"
	if endpoint.Addr().Is6() {
		host = "::"
	}
	bind, err := reserveBind(host, 0)
	if err != nil {
		return nil, err
	}
	client := &AWGClient{bind: bind}
	client.tun, err = newPacketTun(config.Settings.MTU, nil)
	if err != nil {
		bind.dispose()
		return nil, err
	}
	prefix, _ := netip.ParsePrefix(config.Address)
	protocol := ipv4.ProtocolNumber
	if prefix.Addr().Is6() {
		protocol = ipv6.ProtocolNumber
	}
	if tcpErr := client.tun.stack.AddProtocolAddress(1, tcpip.ProtocolAddress{Protocol: protocol, AddressWithPrefix: tcpip.AddrFromSlice(prefix.Addr().AsSlice()).WithPrefix()}, stack.AddressProperties{}); tcpErr != nil {
		client.tun.Close()
		bind.dispose()
		return nil, errors.New("cannot initialize AmneziaWG client address")
	}
	client.dev = device.NewDevice(client.tun, bind, &device.Logger{Verbosef: func(string, ...any) {}, Errorf: func(string, ...any) {}})
	route := "0.0.0.0/0"
	if prefix.Addr().Is6() {
		route = "::/0"
	}
	in.Users[0].Address = route
	if err = client.dev.IpcSet(awgUAPI(in, nil, true) + "endpoint=" + endpoint.String() + "\n"); err == nil {
		err = client.dev.Up()
	}
	if err != nil {
		client.Close()
		return nil, errors.New("cannot initialize AmneziaWG test client")
	}
	return client, nil
}
func (c *AWGClient) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, errors.New("AmneziaWG diagnostic client requires TCP")
	}
	endpoint, err := netip.ParseAddrPort(address)
	if err != nil {
		return nil, errors.New("AmneziaWG diagnostic destination must be a numeric address")
	}
	protocol := ipv4.ProtocolNumber
	if endpoint.Addr().Is6() {
		protocol = ipv6.ProtocolNumber
	}
	return gonet.DialContextTCP(ctx, c.tun.stack, tcpip.FullAddress{NIC: 1, Addr: tcpip.AddrFromSlice(endpoint.Addr().AsSlice()), Port: endpoint.Port()}, protocol)
}
func (c *AWGClient) Close() error { c.close.Do(func() { c.dev.Close(); c.bind.dispose() }); return nil }
