package nativecore

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"reflect"
	"sync"
	"time"

	"github.com/9seconds/mtg/v2/essentials"
	"github.com/9seconds/mtg/v2/mtglib"
)

type account struct {
	mu       sync.Mutex
	up, down int64
	streams  map[string]stream
}
type stream struct {
	ip            string
	authenticated bool
}
type proxyUser struct {
	config User
	secret mtglib.Secret
	proxy  *mtglib.Proxy
	mu     sync.Mutex
	conns  map[net.Conn]struct{}
	timer  *time.Timer
}
type listener struct {
	ln    net.Listener
	slots chan struct{}
}
type liveInbound struct {
	listener *listener
	users    []*proxyUser
}
type Engine struct {
	apply     sync.Mutex
	mu        sync.RWMutex
	live      map[string]liveInbound
	counters  map[string]*account
	counterMu sync.Mutex
	network   mtglib.Network
	closed    bool
}

func New() *Engine {
	return &Engine{live: map[string]liveInbound{}, counters: map[string]*account{}, network: publicNetwork{}}
}
func (e *Engine) account(name string) *account {
	e.counterMu.Lock()
	defer e.counterMu.Unlock()
	if e.counters[name] == nil {
		e.counters[name] = &account{streams: map[string]stream{}}
	}
	return e.counters[name]
}
func address(in Inbound) string { return net.JoinHostPort(in.Listen, fmt.Sprint(in.Port)) }
func (e *Engine) Owns(addr string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	_, ok := e.live[addr]
	return ok
}
func (e *Engine) Apply(raw []byte) error {
	cfg, err := Decode(raw)
	if err != nil {
		return err
	}
	e.apply.Lock()
	defer e.apply.Unlock()
	e.mu.RLock()
	if e.closed {
		e.mu.RUnlock()
		return errors.New("native runtime is closed")
	}
	old := e.live
	e.mu.RUnlock()
	next := map[string]liveInbound{}
	created := []*proxyUser{}
	opened := []*listener{}
	cleanup := func() {
		for _, ln := range opened {
			ln.ln.Close()
		}
		for _, u := range created {
			u.close()
		}
	}
	for _, in := range cfg.Inbounds {
		key := address(in)
		prior := old[key]
		ln := prior.listener
		if ln == nil {
			socket, err := net.Listen("tcp", key)
			if err != nil {
				cleanup()
				return fmt.Errorf("native listener %s could not bind", in.Tag)
			}
			ln = &listener{ln: socket, slots: make(chan struct{}, 1024)}
			opened = append(opened, ln)
		}
		users := []*proxyUser{}
		for _, config := range in.Users {
			var user *proxyUser
			for _, v := range prior.users {
				if reflect.DeepEqual(v.config, config) {
					user = v
					break
				}
			}
			if user == nil {
				secret, _ := mtglib.ParseSecret(config.Secret)
				user = &proxyUser{config: config, secret: secret, conns: map[net.Conn]struct{}{}}
				user.proxy, err = mtglib.NewProxy(mtglib.ProxyOpts{Secret: secret, Network: e.network, AntiReplayCache: &replayCache{seen: map[[32]byte]time.Time{}}, IPBlocklist: noBlocklist{}, IPAllowlist: noBlocklist{}, EventStream: events{account: e.account(config.Name), config: config}, Logger: quietLogger{}, PreferIP: "prefer-ipv4", Concurrency: 1024, HandshakeTimeout: 8 * time.Second, IdleTimeout: 5 * time.Minute})
				if err != nil {
					cleanup()
					return errors.New("cannot prepare MTProto engine")
				}
				created = append(created, user)
			}
			users = append(users, user)
		}
		next[key] = liveInbound{listener: ln, users: users}
	}
	e.mu.Lock()
	e.live = next
	e.mu.Unlock()
	for _, ln := range opened {
		go e.accept(ln)
	}
	keep := map[*proxyUser]bool{}
	for _, in := range next {
		for _, u := range in.users {
			keep[u] = true
		}
	}
	for key, in := range old {
		if next[key].listener != in.listener {
			in.listener.ln.Close()
		}
		for _, u := range in.users {
			if !keep[u] {
				u.close()
			}
		}
	}
	for _, u := range created {
		if u.config.ExpiresAt != "" {
			expiry, _ := time.Parse(time.RFC3339, u.config.ExpiresAt)
			u.mu.Lock()
			u.timer = time.AfterFunc(max(0, time.Until(expiry)), u.closeConnections)
			u.mu.Unlock()
		}
	}
	return nil
}
func (u *proxyUser) closeConnections() {
	u.mu.Lock()
	defer u.mu.Unlock()
	for c := range u.conns {
		c.Close()
	}
}
func (u *proxyUser) close() {
	u.mu.Lock()
	if u.timer != nil {
		u.timer.Stop()
	}
	u.mu.Unlock()
	u.closeConnections()
	u.proxy.Shutdown()
}
func (e *Engine) Close() {
	e.apply.Lock()
	defer e.apply.Unlock()
	e.mu.Lock()
	old := e.live
	e.live = map[string]liveInbound{}
	e.closed = true
	e.mu.Unlock()
	for _, in := range old {
		in.listener.ln.Close()
		for _, u := range in.users {
			u.close()
		}
	}
}
func (e *Engine) accept(ln *listener) {
	for {
		conn, err := ln.ln.Accept()
		if err != nil {
			return
		}
		select {
		case ln.slots <- struct{}{}:
			go func() { defer func() { <-ln.slots }(); defer conn.Close(); e.serve(conn, ln) }()
		default:
			conn.Close()
		}
	}
}

type replayConn struct {
	essentials.Conn
	prefix io.Reader
}

func (c replayConn) Read(b []byte) (int, error) { return c.prefix.Read(b) }
func (e *Engine) serve(conn net.Conn, ln *listener) {
	_ = conn.SetDeadline(time.Now().Add(8 * time.Second))
	head := make([]byte, 5)
	if _, err := io.ReadFull(conn, head); err != nil {
		return
	}
	size := int(binary.BigEndian.Uint16(head[3:]))
	if size < 43 || size > 16384 || head[0] != 22 {
		return
	}
	frame := make([]byte, 5+size)
	copy(frame, head)
	if _, err := io.ReadFull(conn, frame[5:]); err != nil {
		return
	}
	// Select a user's secret by the FakeTLS HMAC. The pinned engine performs the
	// complete SNI, timestamp, anti-replay and obfuscated transport validation.
	random := append([]byte{}, frame[11:43]...)
	zeroed := append([]byte{}, frame...)
	clear(zeroed[11:43])
	e.mu.RLock()
	var chosen *proxyUser
	for _, in := range e.live {
		if in.listener == ln {
			for _, u := range in.users {
				mac := hmac.New(sha256.New, u.secret.Key[:])
				mac.Write(zeroed)
				if hmac.Equal(mac.Sum(nil)[:28], random[:28]) {
					chosen = u
					break
				}
			}
			break
		}
	}
	if chosen == nil {
		e.mu.RUnlock()
		return
	}
	if chosen.config.ExpiresAt != "" {
		expiry, _ := time.Parse(time.RFC3339, chosen.config.ExpiresAt)
		if !time.Now().Before(expiry) {
			e.mu.RUnlock()
			return
		}
	}
	chosen.mu.Lock()
	chosen.conns[conn] = struct{}{}
	chosen.mu.Unlock()
	e.mu.RUnlock()
	defer func() { chosen.mu.Lock(); delete(chosen.conns, conn); chosen.mu.Unlock() }()
	reader := bytesReader(frame)
	chosen.proxy.ServeConn(replayConn{Conn: essentials.WrapNetConn(conn), prefix: io.MultiReader(&reader, conn)})
}

// A small reader avoids retaining a pooled handshake buffer after the proxy has
// consumed it; io.MultiReader drops the exhausted reader.
type bytesReader []byte

func (b *bytesReader) Read(p []byte) (int, error) {
	if len(*b) == 0 {
		return 0, io.EOF
	}
	n := copy(p, *b)
	*b = (*b)[n:]
	return n, nil
}

type Counter struct {
	Name  string `json:"name"`
	Value int64  `json:"value"`
}

func (e *Engine) Snapshot() ([]Counter, map[string]map[string]int64) {
	e.counterMu.Lock()
	defer e.counterMu.Unlock()
	counters := []Counter{}
	online := map[string]map[string]int64{}
	for name, a := range e.counters {
		a.mu.Lock()
		counters = append(counters, Counter{"user>>>" + name + ">>>traffic>>>uplink", a.up}, Counter{"user>>>" + name + ">>>traffic>>>downlink", a.down})
		for _, s := range a.streams {
			if s.authenticated {
				if online[name] == nil {
					online[name] = map[string]int64{}
				}
				online[name][s.ip] = time.Now().Unix()
			}
		}
		a.mu.Unlock()
	}
	return counters, online
}
