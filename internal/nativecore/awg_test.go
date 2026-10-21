package nativecore

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/9seconds/mtg/v2/essentials"
	"github.com/amnezia-vpn/amneziawg-go/device"
	"github.com/sagernet/gvisor/pkg/tcpip"
	"github.com/sagernet/gvisor/pkg/tcpip/adapters/gonet"
	"github.com/sagernet/gvisor/pkg/tcpip/network/ipv4"
	"github.com/sagernet/gvisor/pkg/tcpip/stack"
)

type wireNetwork struct{ tcp, udp string }

func (n wireNetwork) NativeDialer() *net.Dialer { return &net.Dialer{Timeout: time.Second} }
func (n wireNetwork) Dial(network, address string) (essentials.Conn, error) {
	return n.DialContext(context.Background(), network, address)
}
func (n wireNetwork) DialContext(ctx context.Context, network, address string) (essentials.Conn, error) {
	target := n.tcp
	if network == "udp" {
		target = n.udp
	}
	c, err := n.NativeDialer().DialContext(ctx, network, target)
	if err != nil {
		return nil, err
	}
	return essentials.WrapNetConn(c), nil
}
func (n wireNetwork) MakeHTTPClient(func(context.Context, string, string) (essentials.Conn, error)) *http.Client {
	return &http.Client{Timeout: time.Second}
}
func wireEcho(t *testing.T) wireNetwork {
	t.Helper()
	tcpSocket, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tcpSocket.Close() })
	go func() {
		for {
			c, err := tcpSocket.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); c.SetDeadline(time.Now().Add(10 * time.Second)); io.Copy(c, c) }()
		}
	}()
	udpSocket, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { udpSocket.Close() })
	go func() {
		buf := make([]byte, 65535)
		for {
			n, addr, err := udpSocket.ReadFrom(buf)
			if err != nil {
				return
			}
			udpSocket.WriteTo(buf[:n], addr)
		}
	}()
	return wireNetwork{tcpSocket.Addr().String(), udpSocket.LocalAddr().String()}
}
func wireKey(t *testing.T) (string, string) {
	t.Helper()
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(key.Bytes()), hex.EncodeToString(key.PublicKey().Bytes())
}
func wireSettings(key string) *AWGSettings {
	return &AWGSettings{PrivateKey: key, MTU: 1280, Jc: 3, Jmin: 40, Jmax: 70, S1: 33, S2: 57, S3: 8, S4: 16, H1: "123456", H2: "234567", H3: "345678", H4: "456789"}
}
func awgClient(t *testing.T, port int, private, public, address string, settings *AWGSettings) *packetTun {
	t.Helper()
	tun, err := newPacketTun(1280, nil)
	if err != nil {
		t.Fatal(err)
	}
	ip := netip.MustParseAddr(address)
	if err := tun.stack.AddProtocolAddress(1, tcpip.ProtocolAddress{Protocol: ipv4.ProtocolNumber, AddressWithPrefix: tcpip.AddrFromSlice(ip.AsSlice()).WithPrefix()}, stack.AddressProperties{}); err != nil {
		t.Fatal(err)
	}
	bind, err := reserveBind("127.0.0.1", 0)
	if err != nil {
		t.Fatal(err)
	}
	dev := device.NewDevice(tun, bind, &device.Logger{Verbosef: func(string, ...any) {}, Errorf: func(string, ...any) {}})
	t.Cleanup(func() { dev.Close(); bind.dispose() })
	s := *settings
	s.PrivateKey = private
	in := Inbound{Port: 0, AWG: &s}
	cfg := awgUAPI(in, nil, true) + "public_key=" + public + "\nallowed_ip=0.0.0.0/0\nendpoint=127.0.0.1:" + fmtPort(port) + "\npersistent_keepalive_interval=1\n"
	if err := dev.IpcSet(cfg); err != nil {
		t.Fatal(err)
	}
	if err := dev.Up(); err != nil {
		t.Fatal(err)
	}
	return tun
}
func awgDial(t *testing.T, tun *packetTun, network string) net.Conn {
	t.Helper()
	addr := tcpip.FullAddress{NIC: 1, Addr: tcpip.AddrFrom4([4]byte{203, 0, 113, 1}), Port: 443}
	if network == "tcp" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		c, err := gonet.DialContextTCP(ctx, tun.stack, addr, ipv4.ProtocolNumber)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	c, err := gonet.DialUDP(tun.stack, nil, &addr, ipv4.ProtocolNumber)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func wireRoundtrip(c net.Conn, text string) error {
	c.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := c.Write([]byte(text)); err != nil {
		return err
	}
	buf := make([]byte, len(text))
	_, err := io.ReadFull(c, buf)
	if err != nil {
		return err
	}
	if string(buf) != text {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func TestAmneziaWGRealTCPUDPAndHotPeerRevocation(t *testing.T) {
	e := New()
	e.network = wireEcho(t)
	defer e.Close()
	serverPrivate, serverPublic := wireKey(t)
	alicePrivate, alicePublic := wireKey(t)
	bobPrivate, bobPublic := wireKey(t)
	port := testPort(t)
	alice := User{Name: "veloray:10:alice", PublicKey: alicePublic, Address: "10.66.1.2/32"}
	bob := User{Name: "veloray:11:bob", PublicKey: bobPublic, Address: "10.66.1.3/32"}
	in := Inbound{Tag: "awg", Listen: "127.0.0.1", Port: port, Protocol: "amneziawg", AWG: wireSettings(serverPrivate), Users: []User{alice, bob}}
	config := func() []byte { raw, _ := json.Marshal(Config{Inbounds: []Inbound{in}}); return raw }
	if err := e.Apply(config()); err != nil {
		t.Fatal(err)
	}
	a := awgClient(t, port, alicePrivate, serverPublic, "10.66.1.2", in.AWG)
	b := awgClient(t, port, bobPrivate, serverPublic, "10.66.1.3", in.AWG)
	ac := awgDial(t, a, "tcp")
	defer ac.Close()
	bc := awgDial(t, b, "tcp")
	defer bc.Close()
	for _, c := range []net.Conn{ac, bc} {
		if err := wireRoundtrip(c, "native encrypted TCP"); err != nil {
			t.Fatal(err)
		}
	}
	for _, client := range []*packetTun{a, b} {
		udp := awgDial(t, client, "udp")
		if err := wireRoundtrip(udp, "native encrypted UDP"); err != nil {
			t.Fatal(err)
		}
		udp.Close()
	}
	counters, online := e.Snapshot()
	if len(online[alice.Name]) != 1 || len(online[bob.Name]) != 1 {
		t.Fatalf("missing peer online endpoints: %v", online)
	}
	for _, c := range counters {
		if strings.Contains(c.Name, "veloray:10:") && c.Value == 0 {
			t.Fatal("missing AmneziaWG per-peer accounting")
		}
	}
	in.Users = []User{bob}
	if err := e.Apply(config()); err != nil {
		t.Fatal(err)
	}
	if err := wireRoundtrip(ac, "revoked"); err == nil {
		t.Fatal("removed AmneziaWG peer retained active stream")
	}
	if err := wireRoundtrip(bc, "unchanged peer still connected"); err != nil {
		t.Fatal(err)
	}
}
