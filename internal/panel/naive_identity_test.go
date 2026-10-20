package panel

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestNaiveBasicUsernameAndTelemetryIdentity(t *testing.T) {
	c := Data{"id": int64(17), "name": "alice", "credential": "secret"}
	username := naiveUsername(c)
	if strings.Contains(username, ":") || canonicalNaiveUsername(username) != "veloray:17:alice" {
		t.Fatal("Naive username is not Basic-compatible")
	}
	i := Data{"protocol": "naive", "core": "singbox", "security": "tls", "transport": "raw", "port": 443, "tls_server_name": "vpn.example"}
	out, err := singClientOutbound(c, i, Data{"public_host": "vpn.example"})
	if err != nil || str(out, "username") != username {
		t.Fatal(out, err)
	}
	link, err := url.Parse(shareLink(c, i, Data{"public_host": "vpn.example"}))
	if err != nil || link.User.Username() != username {
		t.Fatal("URI username does not match server", err)
	}
	metric := "user>>>" + username + ">>>traffic>>>uplink"
	stats := Data{"stat": []any{Data{"name": metric, "value": 10}}, "online_users": []any{username, "user>>>" + username + ">>>online"}, "online_ips": Data{username: Data{"203.0.113.1": time.Now().Unix()}}}
	normalizeNaiveTelemetry(stats)
	if str(asData(array(stats, "stat")[0]), "name") != "user>>>veloray:17:alice>>>traffic>>>uplink" || array(stats, "online_users")[0] != "veloray:17:alice" || array(stats, "online_users")[1] != "user>>>veloray:17:alice>>>online" || len(obj(obj(stats, "online_ips"), "veloray:17:alice")) != 1 {
		t.Fatal("identity normalization lost accounting or online state", stats)
	}
	for _, bad := range []string{"veloray-naive-invalid", "veloray-naive-", "external-user"} {
		if canonicalNaiveUsername(bad) != bad {
			t.Fatal("rewrote invalid identity")
		}
	}
}

func TestIntegrationNaiveAccountingOnlineAndDevicePolicy(t *testing.T) {
	s, ctx := integration(t)
	m := &nodeMock{running: true}
	n, i, c := fixture(t, s, ctx, m)
	i["protocol"], i["core"], i["security"], i["transport"] = "naive", "singbox", "tls", "raw"
	if err := save(ctx, s.Store.Pool, "inbounds", i); err != nil {
		t.Fatal(err)
	}
	c["device_limit"], c["_device_denied"] = 1, Data{"203.0.113.9": time.Now().Unix()}
	if err := save(ctx, s.Store.Pool, "clients", c); err != nil {
		t.Fatal(err)
	}
	username := naiveUsername(c)
	m.stats = []any{Data{"name": "user>>>" + username + ">>>traffic>>>uplink", "value": 100}, Data{"name": "user>>>" + username + ">>>traffic>>>downlink", "value": 200}}
	m.statMetadata = Data{"online_users": []any{username}, "online_available": true, "online_ips_available": true, "online_ip_cores": Data{"singbox": true}, "online_ips": Data{username: Data{"203.0.113.1": time.Now().Unix()}}}
	for range 2 {
		tx, err := s.Store.Pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.collect(ctx, tx, n); err != nil {
			tx.Rollback(ctx)
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	fresh, err := get(ctx, s.Store.Pool, "clients", num(c, "id"))
	if err != nil {
		t.Fatal(err)
	}
	if num(fresh, "used_traffic_bytes") != 300 {
		t.Fatal("lost or duplicated Naive usage", fresh)
	}
	if !flag(obj(n, "_online_clients"), fmt.Sprint(num(c, "id"))) {
		t.Fatal("Naive online principal lost")
	}
	ips, complete := connectedIPs([]Data{c}, map[int64]Data{num(i, "id"): i}, map[int64]Data{num(n, "id"): n}, time.Now().UTC())
	if !complete || len(ips) != 1 || str(ips[0], "ip") != "203.0.113.1" {
		t.Fatal("Naive device IP lost", ips, complete)
	}
	rules, err := deviceRulesForNode(ctx, s.Store.Pool, n, "singbox")
	if err != nil || len(rules) != 1 || array(asData(rules[0]), "auth_user")[0] != username {
		t.Fatal("device rejection did not use the wire identity", rules, err)
	}
	config, err := buildSingBoxConfig(ctx, s.Store.Pool, n)
	if err != nil || array(obj(obj(obj(config, "experimental"), "v2ray_api"), "stats"), "users")[0] != username {
		t.Fatal("engine counters use wrong principal", err)
	}
}
