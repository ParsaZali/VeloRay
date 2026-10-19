package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protowire"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type wireCodec struct{}

func (wireCodec) Name() string                    { return "proto" }
func (wireCodec) Marshal(v any) ([]byte, error)   { return *v.(*[]byte), nil }
func (wireCodec) Unmarshal(b []byte, v any) error { *v.(*[]byte) = append([]byte{}, b...); return nil }
func rpc(ctx context.Context, address, method string) ([]byte, error) {
	c, e := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if e != nil {
		return nil, e
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	request, response := []byte{}, []byte{}
	e = c.Invoke(ctx, method, &request, &response, grpc.ForceCodec(wireCodec{}))
	return response, e
}
func wireFields(b []byte) map[protowire.Number][][]byte {
	out := map[protowire.Number][][]byte{}
	for len(b) > 0 {
		n, t, k := protowire.ConsumeTag(b)
		if k < 0 {
			break
		}
		b = b[k:]
		if t == protowire.BytesType {
			v, k := protowire.ConsumeBytes(b)
			if k < 0 {
				break
			}
			out[n] = append(out[n], v)
			b = b[k:]
		} else if t == protowire.VarintType {
			v, k := protowire.ConsumeVarint(b)
			if k < 0 {
				break
			}
			out[n] = append(out[n], protowire.AppendVarint(nil, v))
			b = b[k:]
		} else {
			k := protowire.ConsumeFieldValue(n, t, b)
			if k < 0 {
				break
			}
			b = b[k:]
		}
	}
	return out
}
func first(m map[protowire.Number][][]byte, n protowire.Number) []byte {
	if len(m[n]) > 0 {
		return m[n][0]
	}
	return nil
}
func (m *Manager) querySingStats(ctx context.Context) ([]byte, error) {
	b, e := rpc(ctx, "127.0.0.1:10086", "/v2ray.core.app.stats.command.StatsService/QueryStats")
	if e != nil {
		return nil, e
	}
	stats := []Counter{}
	for _, v := range wireFields(b)[1] {
		f := wireFields(v)
		value, _ := protowire.ConsumeVarint(first(f, 2))
		stats = append(stats, Counter{Name: string(first(f, 1)), Value: int64(value)})
	}
	return json.Marshal(map[string]any{"stat": stats})
}
func xrayOnlineIPs(ctx context.Context) (map[string]map[string]int64, error) {
	c, e := grpc.NewClient("127.0.0.1:10085", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if e != nil {
		return nil, e
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	invoke := func(method string, request []byte) ([]byte, error) {
		response := []byte{}
		e := c.Invoke(ctx, "/xray.app.stats.command.StatsService/"+method, &request, &response, grpc.ForceCodec(wireCodec{}))
		return response, e
	}
	b, e := invoke("GetAllOnlineUsers", []byte{})
	if e != nil {
		return nil, e
	}
	out := map[string]map[string]int64{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, 8)
	var failure error
	for _, raw := range wireFields(b)[1] {
		name := string(raw)
		parts := strings.Split(name, ">>>")
		if len(parts) == 3 {
			name = parts[1]
		}
		if !strings.HasPrefix(name, "veloray:") {
			continue
		}
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				mu.Lock()
				failure = ctx.Err()
				mu.Unlock()
				return
			}
			defer func() { <-slots }()
			req := protowire.AppendTag(nil, 1, protowire.BytesType)
			req = protowire.AppendString(req, "user>>>"+name+">>>online")
			b, e := invoke("GetStatsOnlineIpList", req)
			if e != nil {
				mu.Lock()
				failure = e
				mu.Unlock()
				return
			}
			ips := map[string]int64{}
			for _, entry := range wireFields(b)[2] {
				v := wireFields(entry)
				seen, _ := protowire.ConsumeVarint(first(v, 2))
				ip := string(first(v, 1))
				if net.ParseIP(ip) != nil {
					ips[ip] = int64(seen)
				}
			}
			if len(ips) > 0 {
				mu.Lock()
				out[name] = ips
				mu.Unlock()
			}
		}(name)
	}
	wg.Wait()
	return out, failure
}
func (m *Manager) inspectPorts(ctx context.Context, raw []byte) ([]PortCheck, error) {
	if m.Core == "singbox" {
		var cfg map[string]any
		if e := json.Unmarshal(raw, &cfg); e != nil {
			return nil, e
		}
		for _, item := range func() []any { v, _ := cfg["inbounds"].([]any); return v }() {
			v := item.(map[string]any)
			v["port"] = v["listen_port"]
			v["protocol"] = v["type"]
			if v["type"] == "hysteria2" {
				v["protocol"] = "hysteria"
			}
		}
		raw, _ = json.Marshal(cfg)
	}
	return InspectConfigPorts(ctx, m.Runtime, m.ServiceName, raw)
}

func (m *Manager) singOnlineIPs(ctx context.Context) (map[string]map[string]int64, error) {
	out := map[string]map[string]int64{}
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:10087/connections", nil)
	client := http.Client{Timeout: 2 * time.Second}
	res, e := client.Do(req)
	if e != nil {
		return out, e
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return out, fmt.Errorf("connection telemetry returned %d", res.StatusCode)
	}
	raw, e := io.ReadAll(io.LimitReader(res.Body, (4<<20)+1))
	if e != nil {
		return out, e
	}
	if len(raw) > 4<<20 {
		return out, fmt.Errorf("connection telemetry exceeds size limit")
	}
	var live struct {
		Connections []struct {
			Metadata struct{ SourceIP, User string }
		}
	}
	if e = json.Unmarshal(raw, &live); e != nil {
		return out, e
	}
	for _, c := range live.Connections {
		ip, name := c.Metadata.SourceIP, c.Metadata.User
		if name == "" || net.ParseIP(ip) == nil {
			continue
		}
		if out[name] == nil {
			out[name] = map[string]int64{}
		}
		out[name][ip] = time.Now().Unix()
	}
	return out, nil
}
func splitCore(raw []byte) ([]byte, []byte, error) {
	var cfg map[string]json.RawMessage
	if e := json.Unmarshal(raw, &cfg); e != nil {
		return nil, nil, e
	}
	sing := cfg["_singbox"]
	delete(cfg, "_singbox")
	x, e := json.Marshal(cfg)
	if len(x) == 0 {
		return nil, nil, fmt.Errorf("empty configuration")
	}
	return x, sing, e
}
