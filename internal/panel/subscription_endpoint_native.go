package panel

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"github.com/PaarSaAm/VeloRay/internal/host"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

// Only the root-owned installer/systemd oneshot may modify Nginx.
func (s *Server) SyncNativeSubscriptionEndpoint(ctx context.Context, force bool) error {
	if os.Geteuid() != 0 || s.Config.SubscriptionDriver != "native" {
		return errors.New("native subscription configuration requires the installed root service")
	}
	return s.syncSubscriptionEndpoint(ctx, force, func(ctx context.Context, port, previous int) error {
		const path = "/etc/nginx/sites-available/veloray"
		if port != previous && port > 0 {
			listener, err := net.Listen("tcp", net.JoinHostPort("0.0.0.0", strconv.Itoa(port)))
			if err != nil {
				return fmt.Errorf("subscription port is occupied: %w", err)
			}
			listener.Close()
		}
		if err := host.RenderNginx(ctx, s.Config.PublicURL, path, port); err != nil {
			return err
		}
		if port == 0 {
			return nil
		}
		healthCtx, stopHealth := context.WithTimeout(ctx, 5*time.Second)
		var healthErr error
		for {
			healthErr = s.checkNativeSubscriptionHTTPS(healthCtx, port)
			if healthErr == nil {
				break
			}
			select {
			case <-healthCtx.Done():
			case <-time.After(150 * time.Millisecond):
			}
			if healthCtx.Err() != nil {
				break
			}
		}
		stopHealth()
		if err := healthErr; err != nil {
			rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			if restoreErr := host.RenderNginx(rollbackCtx, s.Config.PublicURL, path, previous); restoreErr != nil {
				return fmt.Errorf("subscription health failed and proxy restoration failed: %w", restoreErr)
			}
			return err
		}
		return nil
	})
}
func (s *Server) checkNativeSubscriptionHTTPS(ctx context.Context, port int) error {
	u, err := url.Parse(s.Config.PublicURL)
	if err != nil {
		return err
	}
	certificate, err := os.ReadFile(host.NginxCertificate(u.Hostname()))
	if err != nil {
		return err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(certificate) {
		return errors.New("subscription TLS certificate is invalid")
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, ServerName: u.Hostname(), RootCAs: roots}, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	}}
	defer transport.CloseIdleConnections()
	u.Host = net.JoinHostPort(u.Hostname(), strconv.Itoa(port))
	u.Path = "/api/health"
	req, _ := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	res, err := (&http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
	if res.StatusCode != 200 {
		return errors.New("subscription HTTPS health check failed")
	}
	return nil
}
