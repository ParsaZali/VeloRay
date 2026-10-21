package nativecore

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/9seconds/mtg/v2/essentials"
)

type echoNetwork struct{ addr string }

func (n echoNetwork) NativeDialer() *net.Dialer { return &net.Dialer{Timeout: time.Second} }
func (n echoNetwork) Dial(network, addr string) (essentials.Conn, error) {
	return n.DialContext(context.Background(), network, addr)
}
func (n echoNetwork) DialContext(ctx context.Context, network, addr string) (essentials.Conn, error) {
	c, e := n.NativeDialer().DialContext(ctx, "tcp", n.addr)
	if e != nil {
		return nil, e
	}
	return essentials.WrapNetConn(c), nil
}
func (n echoNetwork) MakeHTTPClient(func(context.Context, string, string) (essentials.Conn, error)) *http.Client {
	return &http.Client{Timeout: time.Second}
}
func testConfig(t *testing.T, port int, users ...User) []byte {
	t.Helper()
	b, e := json.Marshal(Config{Inbounds: []Inbound{{Tag: "telegram", Listen: "127.0.0.1", Port: port, Protocol: "mtproto", Users: users}}})
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func testSecret(key byte) string {
	return "ee" + hex.EncodeToString(bytes.Repeat([]byte{key}, 16)) + hex.EncodeToString([]byte("www.example.com"))
}
func testPort(t *testing.T) int {
	t.Helper()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
func obfsCipher(nonce, secret []byte, reverse bool) cipher.Stream {
	keyiv := append([]byte{}, nonce[8:56]...)
	if reverse {
		for i, j := 0, len(keyiv)-1; i < j; i, j = i+1, j-1 {
			keyiv[i], keyiv[j] = keyiv[j], keyiv[i]
		}
	}
	key := keyiv[:32]
	if secret != nil {
		h := sha256.New()
		h.Write(key)
		h.Write(secret)
		key = h.Sum(nil)
	}
	block, _ := aes.NewCipher(key)
	return cipher.NewCTR(block, keyiv[32:])
}
func echoDC(t *testing.T) string {
	t.Helper()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, e := l.Accept()
			if e != nil {
				return
			}
			go func() {
				defer c.Close()
				c.SetDeadline(time.Now().Add(10 * time.Second))
				head := make([]byte, 64)
				if _, e := io.ReadFull(c, head); e != nil {
					return
				}
				recv, send := obfsCipher(head, nil, false), obfsCipher(head, nil, true)
				recv.XORKeyStream(head, head)
				if !bytes.Equal(head[56:60], bytes.Repeat([]byte{0xdd}, 4)) {
					return
				}
				buf := make([]byte, 8192)
				for {
					n, e := c.Read(buf)
					if n > 0 {
						recv.XORKeyStream(buf[:n], buf[:n])
						send.XORKeyStream(buf[:n], buf[:n])
						if _, err := c.Write(buf[:n]); err != nil {
							return
						}
					}
					if e != nil {
						return
					}
				}
			}()
		}
	}()
	return l.Addr().String()
}
func tlsRecord(r io.Reader) ([]byte, error) {
	h := make([]byte, 5)
	if _, e := io.ReadFull(r, h); e != nil {
		return nil, e
	}
	n := int(binary.BigEndian.Uint16(h[3:]))
	if n > 16384 {
		return nil, io.ErrUnexpectedEOF
	}
	b := make([]byte, 5+n)
	copy(b, h)
	_, e := io.ReadFull(r, b[5:])
	return b, e
}
func writeRecord(c net.Conn, b []byte) error {
	h := []byte{23, 3, 3, byte(len(b) >> 8), byte(len(b))}
	_, e := c.Write(append(h, b...))
	return e
}

type mtClient struct {
	net.Conn
	send, recv cipher.Stream
}

func connectMT(t *testing.T, addr, secret string) *mtClient {
	t.Helper()
	a, b := net.Pipe()
	go func() {
		defer a.Close()
		_ = tls.Client(a, &tls.Config{ServerName: "www.example.com", CurvePreferences: []tls.CurveID{tls.X25519}}).Handshake()
	}()
	hello, e := tlsRecord(b)
	b.Close()
	if e != nil {
		t.Fatal(e)
	}
	key, _ := hex.DecodeString(secret[2:34])
	clear(hello[11:43])
	mac := hmac.New(sha256.New, key)
	mac.Write(hello)
	random := mac.Sum(nil)
	binary.LittleEndian.PutUint32(random[28:], binary.LittleEndian.Uint32(random[28:])^uint32(time.Now().Unix()))
	copy(hello[11:43], random)
	c, e := net.DialTimeout("tcp", addr, time.Second)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { c.Close() })
	c.SetDeadline(time.Now().Add(4 * time.Second))
	if _, e = c.Write(hello); e != nil {
		t.Fatal(e)
	}
	packet := []byte{}
	for range 3 {
		record, e := tlsRecord(c)
		if e != nil {
			t.Fatalf("FakeTLS response: %v", e)
		}
		packet = append(packet, record...)
	}
	got := append([]byte{}, packet[11:43]...)
	clear(packet[11:43])
	mac = hmac.New(sha256.New, key)
	mac.Write(random)
	mac.Write(packet)
	if !hmac.Equal(got, mac.Sum(nil)) {
		t.Fatal("invalid server authentication")
	}
	nonce := make([]byte, 64)
	rand.Read(nonce)
	copy(nonce[56:60], bytes.Repeat([]byte{0xdd}, 4))
	binary.LittleEndian.PutUint16(nonce[60:62], 2)
	send, recv := obfsCipher(nonce, key, false), obfsCipher(nonce, key, true)
	enc := append([]byte{}, nonce...)
	send.XORKeyStream(enc, enc)
	copy(nonce[56:], enc[56:])
	c.Write([]byte{20, 3, 3, 0, 1, 1})
	if e = writeRecord(c, nonce); e != nil {
		t.Fatal(e)
	}
	return &mtClient{c, send, recv}
}
func (c *mtClient) echo(text string) error {
	c.SetDeadline(time.Now().Add(3 * time.Second))
	payload := []byte(text)
	c.send.XORKeyStream(payload, payload)
	if e := writeRecord(c.Conn, payload); e != nil {
		return e
	}
	out := []byte{}
	for len(out) < len(text) {
		record, e := tlsRecord(c.Conn)
		if e != nil {
			return e
		}
		payload = record[5:]
		c.recv.XORKeyStream(payload, payload)
		out = append(out, payload...)
	}
	if string(out) != text {
		return io.ErrUnexpectedEOF
	}
	return nil
}
func TestMTProtoWireHotRevocationAndAccounting(t *testing.T) {
	e := New()
	e.network = echoNetwork{echoDC(t)}
	defer e.Close()
	port := testPort(t)
	alice := User{Name: "veloray:1:alice", Secret: testSecret(1)}
	bob := User{Name: "veloray:2:bob", Secret: testSecret(2)}
	if err := e.Apply(testConfig(t, port, alice, bob)); err != nil {
		t.Fatal(err)
	}
	addr := net.JoinHostPort("127.0.0.1", fmtPort(port))
	a := connectMT(t, addr, alice.Secret)
	b := connectMT(t, addr, bob.Secret)
	if err := a.echo("alice payload"); err != nil {
		t.Fatal(err)
	}
	if err := b.echo("bob payload"); err != nil {
		t.Fatal(err)
	}

	var counters []Counter
	deadline := time.Now().Add(time.Second)
	for {
		var online map[string]map[string]int64
		counters, online = e.Snapshot()
		counts := map[string]int64{}
		for _, c := range counters {
			counts[c.Name] = c.Value
		}
		ready := true
		for _, user := range []string{alice.Name, bob.Name} {
			if counts["user>>>"+user+">>>traffic>>>uplink"] == 0 || counts["user>>>"+user+">>>traffic>>>downlink"] == 0 || len(online[user]) != 1 {
				ready = false
			}
		}
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("missing isolated accounting/online state: %v %v", counts, online)
		}
		time.Sleep(time.Millisecond)
	}

	if err := e.Apply(testConfig(t, port, bob)); err != nil {
		t.Fatal(err)
	}
	if err := a.echo("revoked"); err == nil {
		t.Fatal("revoked session survived")
	}
	if err := b.echo("unchanged user survives apply"); err != nil {
		t.Fatal(err)
	}
	if err := e.Apply([]byte(`{"inbounds":[],"unknown":true}`)); err == nil {
		t.Fatal("unknown field accepted")
	}
	if err := b.echo("failed update retained old listener"); err != nil {
		t.Fatal(err)
	}
	counters, _ = e.Snapshot()
	for _, c := range counters {
		if strings.Contains(c.Name, alice.Name) && c.Value == 0 {
			t.Fatal("revoked counters lost")
		}
	}
}
func fmtPort(port int) string {
	var b [5]byte
	i := len(b)
	for port > 0 {
		i--
		b[i] = byte(port%10) + '0'
		port /= 10
	}
	return string(b[i:])
}
func TestNativeConfigRejectsMalformedSecrets(t *testing.T) {
	for _, secret := range []string{"", "dd" + strings.Repeat("01", 16), "ee" + strings.Repeat("01", 16) + hex.EncodeToString([]byte("localhost"))} {
		if _, err := Decode(testConfig(t, 14443, User{Name: "veloray:1:test", Secret: secret})); err == nil {
			t.Fatal("accepted invalid FakeTLS secret")
		}
	}
}

func TestMTProtoDiagnosticResPQAndNonceValidation(t *testing.T) {
	for _, valid := range []bool{true, false} {
		t.Run(map[bool]string{true: "response", false: "wrong-nonce"}[valid], func(t *testing.T) {
			dc, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer dc.Close()
			serve := func(conn net.Conn) {
				var err error
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(4 * time.Second))
				nonce := make([]byte, 64)
				if _, err = io.ReadFull(conn, nonce); err != nil {
					return
				}
				recv, send := obfsCipher(nonce, nil, false), obfsCipher(nonce, nil, true)
				recv.XORKeyStream(nonce, nonce)
				input := &cipher.StreamReader{S: recv, R: conn}
				head := make([]byte, 4)
				if _, err = io.ReadFull(input, head); err != nil {
					return
				}
				size := binary.LittleEndian.Uint32(head)
				if size != 47 {
					return
				}
				request := make([]byte, int(size))
				if _, err = io.ReadFull(input, request); err != nil {
					return
				}
				if binary.LittleEndian.Uint32(request[20:24]) != 0xbe7e8ef1 {
					return
				}
				response := make([]byte, 4+80)
				binary.LittleEndian.PutUint32(response, 80)
				binary.LittleEndian.PutUint32(response[20:24], 60)
				binary.LittleEndian.PutUint32(response[24:28], 0x05162463)
				copy(response[28:44], request[24:40])
				if !valid {
					response[28] ^= 1
				}
				send.XORKeyStream(response, response)
				conn.Write(response)
				_, _ = io.Copy(io.Discard, input) // Telegram keeps the transport open after resPQ.
			}
			go func() {
				for {
					conn, err := dc.Accept()
					if err != nil {
						return
					}
					go serve(conn)
				}
			}()
			e := New()
			e.network = echoNetwork{dc.Addr().String()}
			defer e.Close()
			port := testPort(t)
			secret := testSecret(5)
			if err = e.Apply(testConfig(t, port, User{Name: "veloray:50:probe", Secret: secret})); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			err = ProbeMTProto(ctx, net.JoinHostPort("127.0.0.1", fmtPort(port)), secret)
			if valid && err != nil {
				t.Fatal(err)
			}
			if !valid && err == nil {
				t.Fatal("accepted a different request nonce")
			}
		})
	}
}

func TestNativeConfigurationRequiresExplicitEmptyArray(t *testing.T) {
	for _, raw := range []string{"null", "{}", `{"inbounds":null}`} {
		if _, err := Decode([]byte(raw)); err == nil {
			t.Fatal("ambiguous empty configuration accepted", raw)
		}
	}
	if _, err := Decode([]byte(`{"inbounds":[]}`)); err != nil {
		t.Fatal(err)
	}
}
