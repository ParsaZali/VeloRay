package panel

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// Independent HTTP/1.1 Naive compatibility fixture: CONNECT + basic proxy
// authentication + the first eight padded records in each direction. It does
// not depend on the sing-box client/Chromium outbound implementation.
type naiveTestConn struct {
	net.Conn
	reader        io.Reader
	reads, writes int
	buffer        []byte
}

func (c *naiveTestConn) Write(data []byte) (int, error) {
	if c.writes >= 8 {
		return c.Conn.Write(data)
	}
	if len(data) > 65535 {
		return 0, fmt.Errorf("test record too large")
	}
	padding := c.writes + 1
	record := make([]byte, 3+len(data)+padding)
	binary.BigEndian.PutUint16(record, uint16(len(data)))
	record[2] = byte(padding)
	copy(record[3:], data)
	if _, err := io.Copy(c.Conn, bytes.NewReader(record)); err != nil {
		return 0, err
	}
	c.writes++
	return len(data), nil
}
func (c *naiveTestConn) Read(data []byte) (int, error) {
	for len(c.buffer) == 0 && c.reads < 8 {
		var header [3]byte
		if _, err := io.ReadFull(c.reader, header[:]); err != nil {
			return 0, err
		}
		length, padding := int(binary.BigEndian.Uint16(header[:2])), int(header[2])
		record := make([]byte, length+padding)
		if _, err := io.ReadFull(c.reader, record); err != nil {
			return 0, err
		}
		c.buffer = record[:length]
		c.reads++
	}
	if len(c.buffer) > 0 {
		n := copy(data, c.buffer)
		c.buffer = c.buffer[n:]
		return n, nil
	}
	return c.reader.Read(data)
}

func naiveRoundTrip(t *testing.T, binaryPath, configFile, certificate string, outbound Data, controller int, principal string) {
	t.Helper()
	nativeRun(t, binaryPath, configFile)
	cert, err := os.ReadFile(certificate)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(cert) {
		t.Fatal("test certificate invalid")
	}
	tlsConfig := &tls.Config{RootCAs: roots, ServerName: "vpn.example", MinVersion: tls.VersionTLS12, NextProtos: []string{"http/1.1"}}
	address := net.JoinHostPort("127.0.0.1", fmt.Sprint(num(outbound, "server_port")))
	var conn *tls.Conn
	for attempt := 0; attempt < 50; attempt++ {
		conn, err = tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", address, tlsConfig)
		if err == nil {
			break
		}
		time.Sleep(40 * time.Millisecond)
	}
	if err != nil {
		t.Fatal("Naive TLS listener unavailable", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	upstream, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer upstream.Close()
	hold := make(chan struct{})
	defer close(hold)
	done := make(chan error, 1)
	go func() {
		peer, e := upstream.Accept()
		if e != nil {
			done <- e
			return
		}
		defer peer.Close()
		_ = peer.SetDeadline(time.Now().Add(10 * time.Second))
		for range 10 {
			payload := make([]byte, 1024)
			if _, e = io.ReadFull(peer, payload); e != nil {
				done <- e
				return
			}
			if _, e = io.Copy(peer, bytes.NewReader(payload)); e != nil {
				done <- e
				return
			}
		}
		done <- nil
		<-hold
	}()
	writeConnect := func(c net.Conn, password string, padding bool) *http.Response {
		t.Helper()
		auth := base64.StdEncoding.EncodeToString([]byte(str(outbound, "username") + ":" + password))
		header := ""
		if padding {
			header = "Padding: veloray-compatibility-test\r\n"
		}
		_, e := fmt.Fprintf(c, "CONNECT %s HTTP/1.1\r\nHost: %s\r\nProxy-Authorization: Basic %s\r\n%s\r\n", upstream.Addr(), upstream.Addr(), auth, header)
		if e != nil {
			t.Fatal("CONNECT write failed", e)
		}
		response, e := http.ReadResponse(bufio.NewReader(c), &http.Request{Method: "CONNECT"})
		if e != nil {
			t.Fatal("CONNECT response invalid", e)
		}
		return response
	}
	response := writeConnect(conn, str(outbound, "password"), true)
	if response.StatusCode != 200 || response.Header.Get("Padding") == "" {
		t.Fatal("Naive CONNECT failed", response.StatusCode)
	}
	framed := &naiveTestConn{Conn: conn, reader: conn}
	for round := range 10 {
		payload := bytes.Repeat([]byte{byte(round + 1)}, 1024)
		if _, err = framed.Write(payload); err != nil {
			t.Fatal("Naive upload", round, err)
		}
		got := make([]byte, len(payload))
		if _, err = io.ReadFull(framed, got); err != nil || !bytes.Equal(payload, got) {
			t.Fatal("Naive download", round, err)
		}
	}
	if err = <-done; err != nil {
		t.Fatal("upstream transfer", err)
	}
	telemetry, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/connections", controller))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(telemetry.Body)
	telemetry.Body.Close()
	if !strings.Contains(string(raw), principal) {
		t.Fatal("Naive principal missing from live telemetry")
	}
	for _, test := range []struct {
		password string
		padding  bool
		status   int
	}{{"incorrect-password", true, 407}, {str(outbound, "password"), false, 400}} {
		bad, e := tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", address, tlsConfig)
		if e != nil {
			t.Fatal(e)
		}
		_ = bad.SetDeadline(time.Now().Add(3 * time.Second))
		result := writeConnect(bad, test.password, test.padding)
		bad.Close()
		if result.StatusCode != test.status {
			t.Fatal("Naive accepted invalid request", result.StatusCode)
		}
	}
}
