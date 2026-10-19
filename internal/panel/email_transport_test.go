package panel

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"net"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSMTPRequiresVerifiedTLSAndDeliversThroughRealProtocol(t *testing.T) {
	for _, mode := range []string{"tls", "starttls", "refused-starttls"} {
		t.Run(mode, func(t *testing.T) {
			fixture := httptest.NewTLSServer(nil)
			certificate := fixture.TLS.Certificates[0]
			roots := x509.NewCertPool()
			roots.AddCert(fixture.Certificate())
			fixture.Close()
			listener, e := net.Listen("tcp", "127.0.0.1:0")
			if e != nil {
				t.Fatal(e)
			}
			defer listener.Close()
			received := make(chan string, 1)
			done := make(chan error, 1)
			go func() {
				conn, e := listener.Accept()
				if e != nil {
					done <- e
					return
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
				secure := false
				if mode == "tls" {
					wrapped := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12})
					if e = wrapped.Handshake(); e != nil {
						done <- e
						return
					}
					conn = wrapped
					secure = true
				}
				reader := bufio.NewReader(conn)
				fmt.Fprint(conn, "220 example SMTP\r\n")
				for {
					line, e := reader.ReadString('\n')
					if e != nil {
						if mode == "refused-starttls" {
							done <- nil
						} else {
							done <- e
						}
						return
					}
					line = strings.TrimSpace(line)
					switch {
					case strings.HasPrefix(line, "EHLO"):
						if mode == "refused-starttls" {
							fmt.Fprint(conn, "250 example\r\n")
						} else {
							fmt.Fprint(conn, "250-example\r\n250-STARTTLS\r\n250 AUTH PLAIN\r\n")
						}
					case line == "STARTTLS":
						fmt.Fprint(conn, "220 begin TLS\r\n")
						wrapped := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12})
						if e = wrapped.Handshake(); e != nil {
							done <- e
							return
						}
						conn = wrapped
						reader = bufio.NewReader(conn)
						secure = true
					case strings.HasPrefix(line, "AUTH PLAIN "):
						decoded, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(line, "AUTH PLAIN "))
						if !secure || string(decoded) != "\x00user\x00private-password" {
							done <- fmt.Errorf("authentication did not use verified TLS")
							return
						}
						fmt.Fprint(conn, "235 authenticated\r\n")
					case strings.HasPrefix(line, "MAIL FROM:") || strings.HasPrefix(line, "RCPT TO:"):
						if !secure {
							done <- fmt.Errorf("unencrypted envelope")
							return
						}
						fmt.Fprint(conn, "250 accepted\r\n")
					case line == "DATA":
						fmt.Fprint(conn, "354 data\r\n")
						var message strings.Builder
						for {
							line, e := reader.ReadString('\n')
							if e != nil {
								done <- e
								return
							}
							if line == ".\r\n" {
								break
							}
							message.WriteString(line)
						}
						received <- message.String()
						fmt.Fprint(conn, "250 queued\r\n")
					case line == "QUIT":
						fmt.Fprint(conn, "221 bye\r\n")
						done <- nil
						return
					default:
						done <- fmt.Errorf("unexpected SMTP command")
						return
					}
				}
			}()
			cfg := Data{"smtp_host": "example.com", "smtp_port": 465, "smtp_tls_mode": mode, "smtp_username": "user"}
			if mode == "refused-starttls" {
				cfg["smtp_tls_mode"] = "starttls"
			}
			dial := func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "tcp", listener.Addr().String())
			}
			// httptest's certificate is valid for example.com and trusted only here.
			e = sendSMTP(context.Background(), cfg, "from@example.com", []string{"to@example.com"}, "private-password", []byte("Subject: test\r\n\r\nprivate test fixture\r\n"), dial, &tls.Config{RootCAs: roots, ServerName: "example.com", MinVersion: tls.VersionTLS12})
			if mode == "refused-starttls" {
				if e == nil {
					t.Fatal("plaintext fallback accepted")
				}
			} else if e != nil {
				t.Fatal(e)
			}
			if serverErr := <-done; serverErr != nil {
				t.Fatal(serverErr)
			}
			if mode != "refused-starttls" {
				select {
				case body := <-received:
					if !strings.Contains(body, "private test fixture") {
						t.Fatal("missing SMTP message")
					}
				default:
					t.Fatal("SMTP body missing")
				}
			}
		})
	}
}
