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
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/9seconds/mtg/v2/mtglib"
)

type mtprotoConn struct {
	net.Conn
	send, recv      cipher.Stream
	readMu, writeMu sync.Mutex
	pending         []byte
}

func readTLSRecord(reader io.Reader) ([]byte, error) {
	head := make([]byte, 5)
	if _, err := io.ReadFull(reader, head); err != nil {
		return nil, err
	}
	size := int(binary.BigEndian.Uint16(head[3:]))
	if size > 16384 {
		return nil, errors.New("oversized FakeTLS record")
	}
	record := make([]byte, 5+size)
	copy(record, head)
	_, err := io.ReadFull(reader, record[5:])
	return record, err
}
func writeTLSRecord(writer io.Writer, payload []byte) error {
	record := []byte{23, 3, 3, byte(len(payload) >> 8), byte(len(payload))}
	record = append(record, payload...)
	for len(record) > 0 {
		n, err := writer.Write(record)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		record = record[n:]
	}
	return nil
}
func clientCipher(nonce, key []byte, reverse bool) cipher.Stream {
	material := append([]byte{}, nonce[8:56]...)
	if reverse {
		for i, j := 0, len(material)-1; i < j; i, j = i+1, j-1 {
			material[i], material[j] = material[j], material[i]
		}
	}
	hash := sha256.New()
	hash.Write(material[:32])
	hash.Write(key)
	block, _ := aes.NewCipher(hash.Sum(nil))
	return cipher.NewCTR(block, material[32:])
}
func DialMTProto(ctx context.Context, address, encodedSecret string) (net.Conn, error) {
	secret, err := mtglib.ParseSecret(encodedSecret)
	if err != nil || !secret.Valid() || !ValidHostname(secret.Host) {
		return nil, errors.New("invalid MTProto diagnostic secret")
	}
	client, server := net.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer client.Close()
		_ = tls.Client(client, &tls.Config{ServerName: secret.Host, CurvePreferences: []tls.CurveID{tls.X25519}}).Handshake()
	}()
	hello, err := readTLSRecord(server)
	server.Close()
	<-done
	if err != nil {
		return nil, errors.New("cannot initialize FakeTLS diagnostic")
	}

	conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, errors.New("MTProto server TCP connection failed")
	}
	success := false
	defer func() {
		if !success {
			conn.Close()
		}
	}()
	deadline := time.Now().Add(7 * time.Second)
	if limit, ok := ctx.Deadline(); ok && limit.Before(deadline) {
		deadline = limit
	}
	conn.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	clear(hello[11:43])
	mac := hmac.New(sha256.New, secret.Key[:])
	mac.Write(hello)
	random := mac.Sum(nil)
	binary.LittleEndian.PutUint32(random[28:], binary.LittleEndian.Uint32(random[28:])^uint32(time.Now().Unix()))
	copy(hello[11:43], random)
	if _, err = conn.Write(hello); err != nil {
		return nil, errors.New("MTProto FakeTLS request failed")
	}
	packet := []byte{}
	for index := range 3 {
		record, err := readTLSRecord(conn)
		if err != nil {
			return nil, errors.New("MTProto FakeTLS response failed")
		}
		expected := byte(22)
		if index == 1 {
			expected = 20
		} else if index == 2 {
			expected = 23
		}
		if record[0] != expected {
			return nil, errors.New("unexpected FakeTLS response")
		}
		packet = append(packet, record...)
	}
	if len(packet) < 43 {
		return nil, errors.New("invalid FakeTLS server hello")
	}
	authentication := append([]byte{}, packet[11:43]...)
	clear(packet[11:43])
	mac = hmac.New(sha256.New, secret.Key[:])
	mac.Write(random)
	mac.Write(packet)
	if !hmac.Equal(authentication, mac.Sum(nil)) {
		return nil, errors.New("MTProto server secret authentication failed")
	}
	nonce := make([]byte, 64)
	for {
		rand.Read(nonce)
		if nonce[0] != 0xef && !bytes.Equal(nonce[4:8], []byte{0, 0, 0, 0}) && !bytes.Equal(nonce[:4], []byte{0xdd, 0xdd, 0xdd, 0xdd}) && !bytes.Equal(nonce[:4], []byte{0xee, 0xee, 0xee, 0xee}) && !bytes.Equal(nonce[:4], []byte("GET ")) && !bytes.Equal(nonce[:4], []byte("POST")) && !bytes.Equal(nonce[:4], []byte("HEAD")) {
			break
		}
	}
	copy(nonce[56:60], []byte{0xdd, 0xdd, 0xdd, 0xdd})
	binary.LittleEndian.PutUint16(nonce[60:62], 2)
	send, recv := clientCipher(nonce, secret.Key[:], false), clientCipher(nonce, secret.Key[:], true)
	encrypted := append([]byte{}, nonce...)
	send.XORKeyStream(encrypted, encrypted)
	copy(nonce[56:], encrypted[56:])
	if _, err = conn.Write([]byte{20, 3, 3, 0, 1, 1}); err == nil {
		err = writeTLSRecord(conn, nonce)
	}
	if err != nil {
		return nil, errors.New("MTProto obfuscated handshake failed")
	}
	success = true
	return &mtprotoConn{Conn: conn, send: send, recv: recv}, nil
}
func (c *mtprotoConn) Read(buf []byte) (int, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()
	if len(buf) == 0 {
		return 0, nil
	}
	for len(c.pending) == 0 {
		record, err := readTLSRecord(c.Conn)
		if err != nil {
			return 0, err
		}
		if record[0] != 23 {
			return 0, errors.New("invalid FakeTLS application record")
		}
		c.pending = record[5:]
		c.recv.XORKeyStream(c.pending, c.pending)
	}
	n := copy(buf, c.pending)
	c.pending = c.pending[n:]
	return n, nil
}
func (c *mtprotoConn) Write(buf []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	sent := 0
	for len(buf) > 0 {
		size := min(len(buf), 16384)
		data := append([]byte{}, buf[:size]...)
		c.send.XORKeyStream(data, data)
		if err := writeTLSRecord(c.Conn, data); err != nil {
			return sent, err
		}
		sent += size
		buf = buf[size:]
	}
	return sent, nil
}

// ReqPQ tests the Telegram transport without a Telegram account or session.
// The caller must validate/resolve the public proxy endpoint before dialing.
func ProbeMTProto(ctx context.Context, address, secret string) error {
	conn, err := DialMTProto(ctx, address, secret)
	if err != nil {
		return err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	request := make([]byte, 4+40+7)
	binary.LittleEndian.PutUint32(request[:4], 47)
	now := time.Now()
	id := (uint64(now.Unix()) << 32) | (uint64(now.Nanosecond())<<32)/1_000_000_000
	binary.LittleEndian.PutUint64(request[12:20], id&^3)
	binary.LittleEndian.PutUint32(request[20:24], 20)
	binary.LittleEndian.PutUint32(request[24:28], 0xbe7e8ef1)
	rand.Read(request[28:])
	nonce := append([]byte{}, request[28:44]...)
	if _, err = conn.Write(request); err != nil {
		return errors.New("Telegram request failed")
	}
	head := make([]byte, 4)
	if _, err = io.ReadFull(conn, head); err != nil {
		return errors.New("Telegram data center did not respond")
	}
	size := binary.LittleEndian.Uint32(head)
	if size < 56 || size > 65536 {
		return errors.New("unexpected Telegram response size")
	}
	response := make([]byte, int(size))
	if _, err = io.ReadFull(conn, response); err != nil {
		return errors.New("incomplete Telegram response")
	}
	length := binary.LittleEndian.Uint32(response[16:20])
	if binary.LittleEndian.Uint64(response[:8]) != 0 || length < 36 || int(length) > len(response)-20 || binary.LittleEndian.Uint32(response[20:24]) != 0x05162463 || !hmac.Equal(response[24:40], nonce) {
		return errors.New("Telegram resPQ authentication check failed")
	}
	return nil
}
