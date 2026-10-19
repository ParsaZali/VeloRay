package agent

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestRealCoresTrafficAndAuthenticatedIPs(t *testing.T) {
	for _, core := range []string{"xray", "singbox"} {
		t.Run(core, func(t *testing.T) {
			binaryPath := os.Getenv("VELORAY_TEST_XRAY_BINARY")
			if core == "singbox" {
				if os.Getenv("VELORAY_TEST_NO_NETLINK") == "1" {
					t.Skip("execution environment denies netlink sockets; run on Linux CI")
				}
				binaryPath = os.Getenv("VELORAY_TEST_SINGBOX_BINARY")
			}
			if binaryPath == "" {
				t.Skip("set real core binary")
			}
			target, e := net.Listen("tcp", "127.0.0.1:0")
			if e != nil {
				t.Fatal(e)
			}
			defer target.Close()
			go func() {
				conn, e := target.Accept()
				if e != nil {
					return
				}
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}()
			port := target.Addr().(*net.TCPAddr).Port
			reserved, e := net.Listen("tcp", "127.0.0.1:0")
			if e != nil {
				t.Fatal(e)
			}
			listen := reserved.Addr().(*net.TCPAddr).Port
			reserved.Close()
			name := "veloray:71:runtime-test"
			id := "955cc471-391e-4f25-bc37-c8336ed2be5b"
			var config any
			args := []string{"run", "-config"}
			if core == "xray" {
				config = map[string]any{"log": map[string]any{"loglevel": "warning"}, "stats": map[string]any{}, "api": map[string]any{"tag": "api", "services": []string{"StatsService"}}, "policy": map[string]any{"levels": map[string]any{"0": map[string]any{"statsUserUplink": true, "statsUserDownlink": true, "statsUserOnline": true}}}, "inbounds": []any{map[string]any{"tag": "api", "listen": "127.0.0.1", "port": 10085, "protocol": "dokodemo-door", "settings": map[string]any{"address": "127.0.0.1"}}, map[string]any{"tag": "in-test", "listen": "127.0.0.1", "port": listen, "protocol": "vless", "settings": map[string]any{"decryption": "none", "clients": []any{map[string]any{"id": id, "email": name, "level": 0}}}}}, "outbounds": []any{map[string]any{"protocol": "freedom", "tag": "direct"}}, "routing": map[string]any{"rules": []any{map[string]any{"type": "field", "inboundTag": []string{"api"}, "outboundTag": "api"}}}}
			} else {
				args = []string{"run", "-c"}
				config = map[string]any{"log": map[string]any{"level": "warn"}, "inbounds": []any{map[string]any{"type": "vless", "tag": "in-test", "listen": "127.0.0.1", "listen_port": listen, "users": []any{map[string]any{"name": name, "uuid": id}}}}, "outbounds": []any{map[string]any{"type": "direct", "tag": "direct"}}, "experimental": map[string]any{"v2ray_api": map[string]any{"listen": "127.0.0.1:10086", "stats": map[string]any{"enabled": true, "users": []string{name}}}, "clash_api": map[string]any{"external_controller": "127.0.0.1:10087"}}}
			}
			cfg := config.(map[string]any)
			if core == "xray" {
				cfg["policy"].(map[string]any)["system"] = map[string]any{"statsOutboundUplink": true, "statsOutboundDownlink": true}
			} else {
				cfg["experimental"].(map[string]any)["v2ray_api"].(map[string]any)["stats"].(map[string]any)["outbounds"] = []string{"direct"}
			}
			raw, _ := json.Marshal(config)
			path := filepath.Join(t.TempDir(), "config.json")
			if e = os.WriteFile(path, raw, 0600); e != nil {
				t.Fatal(e)
			}
			cmd := exec.Command(binaryPath, append(args, path)...)
			var logs synchronizedTestBuffer
			cmd.Stdout = &logs
			cmd.Stderr = &logs
			if e = cmd.Start(); e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
			address := net.JoinHostPort("127.0.0.1", intText(listen))
			deadline := time.Now().Add(5 * time.Second)
			for {
				conn, e := net.DialTimeout("tcp", address, 50*time.Millisecond)
				if e == nil {
					conn.Close()
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("core did not start", logs.String())
				}
				time.Sleep(20 * time.Millisecond)
			}
			dialer := net.Dialer{Timeout: time.Second, LocalAddr: &net.TCPAddr{IP: net.ParseIP("127.0.0.2")}}
			conn, e := dialer.Dial("tcp", address)
			if e != nil {
				t.Fatal(e)
			}
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
			header := []byte{0, 0x95, 0x5c, 0xc4, 0x71, 0x39, 0x1e, 0x4f, 0x25, 0xbc, 0x37, 0xc8, 0x33, 0x6e, 0xd2, 0xbe, 0x5b, 0, 1}
			header = binary.BigEndian.AppendUint16(header, uint16(port))
			header = append(header, 1, 127, 0, 0, 1)
			payload := bytes.Repeat([]byte("VeloRay"), 2048)
			if _, e = conn.Write(append(header, payload...)); e != nil {
				t.Fatal(e)
			}
			response := make([]byte, len(payload)+2)
			if _, e = io.ReadFull(conn, response); e != nil {
				t.Fatal(e, logs.String())
			}
			if !bytes.Equal(response[2:], payload) {
				t.Fatal("traffic payload changed")
			}
			ctx := context.Background()
			var traffic []byte
			if core == "xray" {
				traffic, e = exec.Command(binaryPath, "api", "statsquery", "--server=127.0.0.1:10085").Output()
			} else {
				traffic, e = (&Manager{Core: "singbox"}).querySingStats(ctx)
			}
			var outboundCounters struct{ Stat []Counter }
			if e != nil {
				t.Fatal(e)
			}
			if e = json.Unmarshal(traffic, &outboundCounters); e != nil {
				t.Fatal(e)
			}
			outboundValues := map[string]int64{}
			for _, counter := range outboundCounters.Stat {
				outboundValues[counter.Name] = counter.Value
			}
			for _, direction := range []string{"uplink", "downlink"} {
				if outboundValues["outbound>>>direct>>>traffic>>>"+direction] < int64(len(payload)) {
					t.Fatal("real outbound traffic missing", string(traffic))
				}
			}
			if core == "xray" {
				ips, e := xrayOnlineIPs(ctx)
				if e != nil || ips[name]["127.0.0.2"] == 0 {
					t.Fatal("authenticated IP missing", ips, e)
				}
			} else {
				m := &Manager{Core: "singbox"}
				stats, e := m.querySingStats(ctx)
				if e != nil {
					t.Fatal(e)
				}
				var counters struct{ Stat []Counter }
				if e = json.Unmarshal(stats, &counters); e != nil {
					t.Fatal(e)
				}
				var total int64
				for _, counter := range counters.Stat {
					total += counter.Value
				}
				if total < int64(len(payload)) {
					t.Fatal("real traffic was not counted", string(stats))
				}
				ips, err := m.singOnlineIPs(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if ips[name]["127.0.0.2"] == 0 {
					t.Fatal("secondary authenticated IP missing", ips)
				}
			}
		})
	}
}
func intText(value int) string {
	var b [20]byte
	i := len(b)
	for {
		i--
		b[i] = byte(value%10) + '0'
		value /= 10
		if value == 0 {
			return string(b[i:])
		}
	}
}

// Process output and failure diagnostics are accessed concurrently by os/exec.
type synchronizedTestBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *synchronizedTestBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}
func (b *synchronizedTestBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}
