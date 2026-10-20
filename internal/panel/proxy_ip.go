package panel

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

type proxyIPKey struct{}

func parseTrustedProxies(raw string) ([]netip.Prefix, error) {
	prefixes := []netip.Prefix{}
	if strings.TrimSpace(raw) == "" {
		return prefixes, nil
	}
	if len(raw) > 4096 {
		return nil, errors.New("trusted proxy CIDR list is too long")
	}
	for _, part := range strings.Split(raw, ",") {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(part))
		if err != nil || prefix.Bits() == 0 || len(prefixes) >= 32 {
			return nil, errors.New("VELORAY_TRUSTED_PROXIES requires up to 32 explicit non-default CIDRs")
		}
		prefixes = append(prefixes, prefix.Masked())
	}
	return prefixes, nil
}
func (s *Server) withProxyIP(r *http.Request) *http.Request {
	peer := r.RemoteAddr
	if host, _, err := net.SplitHostPort(peer); err == nil {
		peer = host
	}
	address, err := netip.ParseAddr(peer)
	if err != nil {
		return r
	}
	address = address.Unmap()
	trusted := func(ip netip.Addr) bool {
		for _, prefix := range s.Config.TrustedProxies {
			if prefix.Contains(ip) {
				return true
			}
		}
		return false
	}
	if !trusted(address) {
		return r
	}
	header := strings.Join(r.Header.Values("X-Forwarded-For"), ",")
	if header == "" || len(header) > 2048 {
		return r
	}
	parts := strings.Split(header, ",")
	if len(parts) > 16 {
		return r
	}
	chain := make([]netip.Addr, 0, len(parts))
	for _, part := range parts {
		ip, e := netip.ParseAddr(strings.TrimSpace(part))
		if e != nil || ip.Zone() != "" || ip.IsUnspecified() || ip.IsMulticast() {
			return r
		}
		chain = append(chain, ip.Unmap())
	}
	// Work from the socket peer toward the client. A client-supplied leftmost
	// value cannot bypass an untrusted intermediate address in the chain.
	for index := len(chain) - 1; index >= 0 && trusted(address); index-- {
		address = chain[index]
	}
	return r.WithContext(context.WithValue(r.Context(), proxyIPKey{}, address.String()))
}
