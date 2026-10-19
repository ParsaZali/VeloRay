package panel

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestOutboundGraphRejectsMissingCandidatesAndRecursiveChains(t *testing.T) {
	for _, core := range []string{"xray", "singbox"} {
		t.Run(core, func(t *testing.T) {
			next := func(tag, target string) Data {
				if core == "xray" {
					return Data{"tag": tag, "protocol": "freedom", "streamSettings": Data{"sockopt": Data{"dialerProxy": target}}}
				}
				return Data{"tag": tag, "type": "socks", "detour": target, "server": "127.0.0.1", "server_port": 1080}
			}
			direct := Data{"tag": "direct", "protocol": "freedom", "type": "direct"}
			for _, tc := range []struct {
				name string
				outs []any
				want string
			}{
				{"valid", []any{direct, next("first", "second"), next("second", "direct")}, ""},
				{"missing", []any{direct, next("first", "gone")}, "missing next hop"},
				{"self", []any{direct, next("first", "first")}, "cycle"},
				{"recursive", []any{direct, next("first", "second"), next("second", "first")}, "cycle"},
				{"duplicate", []any{direct, next("direct", "")}, "duplicate"},
			} {
				e := validateOutboundGraph(Data{"outbounds": tc.outs}, core)
				if tc.want == "" && e != nil || tc.want != "" && (e == nil || !strings.Contains(e.Error(), tc.want)) {
					t.Fatal(tc.name, e)
				}
			}
		})
	}
	for _, group := range []Data{
		{"type": "selector", "tag": "auto", "outbounds": []any{}},
		{"type": "selector", "tag": "auto", "outbounds": []any{"direct"}, "default": "gone"},
		{"type": "urltest", "tag": "auto", "outbounds": []any{"direct", "direct"}},
		{"type": "selector", "tag": "auto", "outbounds": []any{"auto"}},
		{"type": "urltest", "tag": "auto", "outbounds": []any{1}},
	} {
		if e := validateOutboundGraph(Data{"outbounds": []any{Data{"type": "direct", "tag": "direct"}, group}}, "singbox"); e == nil {
			t.Fatal("bad group accepted", group)
		}
	}
}

func TestIntegrationOutboundChainsDeployAndRejectBrokenDependencies(t *testing.T) {
	s, ctx := integration(t)
	n, _, _ := fixture(t, s, ctx, &nodeMock{running: true, stats: []any{}})
	for _, core := range []string{"xray", "singbox"} {
		base := Data{"name": "transit-" + core, "tag": "transit", "node": num(n, "id"), "core": core, "enabled": true, "protocol": "socks", "config": Data{"server": "127.0.0.1", "server_port": 1080}}
		if core == "xray" {
			base["config"] = Data{"settings": Data{"servers": []any{Data{"address": "127.0.0.1", "port": 1080}}}}
		}
		out, _, e := managementRequest(t, s, ctx, Actor{}, "POST", "outbounds", base)
		if e != nil {
			t.Fatal(e)
		}
		transit := num(out.(Data), "id")
		chain := clone(base)
		chain["name"], chain["tag"] = "chain-"+core, "chain"
		if core == "xray" {
			obj(chain, "config")["streamSettings"] = Data{"sockopt": Data{"dialerProxy": "transit"}}
		} else {
			obj(chain, "config")["detour"] = "transit"
		}
		out, _, e = managementRequest(t, s, ctx, Actor{}, "POST", "outbounds", chain)
		if e != nil {
			t.Fatal(e)
		}
		chainID := num(out.(Data), "id")
		if _, _, e = managementRequest(t, s, ctx, Actor{}, "PATCH", fmt.Sprintf("outbounds/%d", transit), Data{"enabled": false}); e == nil {
			t.Fatal("disabled referenced next hop")
		}
		fresh, _ := get(ctx, s.Store.Pool, "outbounds", transit)
		if !flag(fresh, "enabled") {
			t.Fatal("dependency failure committed")
		}
		cfg, e := buildConfig(ctx, s.Store.Pool, n)
		if e != nil {
			t.Fatal(e)
		}
		if core == "singbox" {
			// Produce a sing-box config even when the node has no sing-box listener yet.
			node := clone(n)
			node["_singbox_deployed"] = true
			cfg, e = buildSingBoxConfig(ctx, s.Store.Pool, node)
			if e != nil {
				t.Fatal(e)
			}
		}
		binary := os.Getenv("VELORAY_TEST_XRAY_BINARY")
		args := []string{"run", "-test", "-config"}
		if core == "singbox" {
			binary = os.Getenv("VELORAY_TEST_SINGBOX_BINARY")
			args = []string{"check", "-c"}
		}
		if binary != "" {
			delete(cfg, "_singbox")
			file := nativeConfigFile(t, cfg)
			if raw, e := exec.CommandContext(context.Background(), binary, append(args, file)...).CombinedOutput(); e != nil {
				t.Fatal(core, e, string(raw))
			}
		}
		if _, _, e = managementRequest(t, s, ctx, Actor{}, "DELETE", fmt.Sprintf("outbounds/%d", chainID), nil); e != nil {
			t.Fatal(e)
		}
		if _, _, e = managementRequest(t, s, ctx, Actor{}, "DELETE", fmt.Sprintf("outbounds/%d", transit), nil); e != nil {
			t.Fatal(e)
		}
	}
}
