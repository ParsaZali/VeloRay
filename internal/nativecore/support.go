package nativecore

import (
	"context"
	"crypto/sha256"
	"errors"
	"net"
	"net/http"
	"sync"
	"syscall"
	"time"

	"github.com/9seconds/mtg/v2/essentials"
	"github.com/9seconds/mtg/v2/mtglib"
)

type events struct {
	account *account
	config  User
}

func (e events) Send(ctx context.Context, event mtglib.Event) {
	a := e.account
	a.mu.Lock()
	reject := false
	switch v := event.(type) {
	case mtglib.EventStart:
		a.streams[v.StreamID()] = stream{ip: v.RemoteIP.String()}
	case mtglib.EventConnectedToDC:
		s := a.streams[v.StreamID()]
		ips := map[string]bool{}
		for _, existing := range a.streams {
			if existing.authenticated {
				ips[existing.ip] = true
			}
		}
		for _, denied := range e.config.DeniedIPs {
			if net.ParseIP(s.ip).Equal(net.ParseIP(denied)) {
				reject = true
			}
		}
		if e.config.IPLimit > 0 && !ips[s.ip] && len(ips) >= e.config.IPLimit {
			reject = true
		}
		if !reject {
			s.authenticated = true
			a.streams[v.StreamID()] = s
		}
	case mtglib.EventTraffic:
		if a.streams[v.StreamID()].authenticated {
			amount := int64(v.Traffic)
			if amount > 0 {
				if v.IsRead {
					if a.down <= int64(^uint64(0)>>1)-amount {
						a.down += amount
					}
				} else if a.up <= int64(^uint64(0)>>1)-amount {
					a.up += amount
				}
			}
		}
	case mtglib.EventDomainFronting:
		delete(a.streams, v.StreamID())
	case mtglib.EventFinish:
		delete(a.streams, v.StreamID())
	}
	a.mu.Unlock()
	if reject {
		if closer, ok := ctx.(interface{ Close() }); ok {
			closer.Close()
		}
	}
}

type replayCache struct {
	mu   sync.Mutex
	seen map[[32]byte]time.Time
}

func (c *replayCache) SeenBefore(data []byte) bool {
	key := sha256.Sum256(data)
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if until := c.seen[key]; now.Before(until) {
		return true
	}
	if len(c.seen) >= 16384 {
		for k, until := range c.seen {
			if !now.Before(until) {
				delete(c.seen, k)
			}
		}
		if len(c.seen) >= 16384 {
			return true
		}
	}
	c.seen[key] = now.Add(10 * time.Minute)
	return false
}

type noBlocklist struct{}

func (noBlocklist) Contains(net.IP) bool { return false }
func (noBlocklist) Run(time.Duration)    {}
func (noBlocklist) Shutdown()            {}

// Outgoing connections cannot reach host metadata or local management services,
// including after DNS changes. The kernel receives the checked numeric address.
func publicIP(ip net.IP) bool {
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, cidr := range []string{"100.64.0.0/10", "198.18.0.0/15"} {
		_, block, _ := net.ParseCIDR(cidr)
		if block.Contains(ip) {
			return false
		}
	}
	return true
}

type publicNetwork struct{}

func (publicNetwork) NativeDialer() *net.Dialer {
	return &net.Dialer{Timeout: 8 * time.Second, KeepAlive: 30 * time.Second, ControlContext: func(_ context.Context, _, addr string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(addr)
		if err != nil || !publicIP(net.ParseIP(host)) {
			return errors.New("native outgoing address is not public")
		}
		return nil
	}}
}
func (n publicNetwork) Dial(network, addr string) (essentials.Conn, error) {
	return n.DialContext(context.Background(), network, addr)
}
func (n publicNetwork) DialContext(ctx context.Context, network, addr string) (essentials.Conn, error) {
	conn, err := n.NativeDialer().DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}
	return essentials.WrapNetConn(conn), nil
}
func (n publicNetwork) MakeHTTPClient(dial func(context.Context, string, string) (essentials.Conn, error)) *http.Client {
	if dial == nil {
		dial = n.DialContext
	}
	return &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Transport: &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) { return dial(ctx, network, addr) }}}
}

type quietLogger struct{}

func (l quietLogger) Named(string) mtglib.Logger            { return l }
func (l quietLogger) BindInt(string, int) mtglib.Logger     { return l }
func (l quietLogger) BindStr(string, string) mtglib.Logger  { return l }
func (l quietLogger) BindJSON(string, string) mtglib.Logger { return l }
func (quietLogger) Printf(string, ...any)                   {}
func (quietLogger) Info(string)                             {}
func (quietLogger) InfoError(string, error)                 {}
func (quietLogger) Warning(string)                          {}
func (quietLogger) WarningError(string, error)              {}
func (quietLogger) Debug(string)                            {}
func (quietLogger) DebugError(string, error)                {}
