package panel

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIntegrationClientProbeUsesOwnedDeliveryHostAndRejectsOtherConnections(t *testing.T) {
	s, ctx := integration(t)
	mock := &nodeMock{running: true}
	_, i, c := fixture(t, s, ctx, mock)
	cfg, err := settings(ctx, s.Store.Pool)
	if err != nil {
		t.Fatal(err)
	}
	cfg["subscription_hosts"] = []any{Data{"inbound": num(i, "id"), "name": "CDN", "address": "edge.example", "port": 8443, "sni": "tls.example", "enabled": true}}
	if err = saveSettings(ctx, s.Store.Pool, cfg); err != nil {
		t.Fatal(err)
	}
	call := func(inbound int64) (any, error) {
		request := httptest.NewRequest("POST", "/", strings.NewReader(fmt.Sprintf(`{"inbound":%d,"host_index":0}`, inbound))).WithContext(ctx)
		out, _, err := s.api(httptest.NewRecorder(), request, fmt.Sprintf("clients/%d/connection-test", num(c, "id")), Actor{})
		return out, err
	}
	out, err := call(num(i, "id"))
	if err != nil || !flag(asData(out), "ok") || mock.probeCalls != 1 {
		t.Fatal(out, err, mock.probeCalls)
	}
	entries := array(obj(mock.probeConfig, "config"), "outbounds")
	proxy := asData(entries[0])
	server := asData(array(obj(proxy, "settings"), "vnext")[0])
	if str(server, "address") != "edge.example" || num(server, "port") != 8443 {
		t.Fatal("delivery override lost", mock.probeConfig)
	}
	if _, err = call(num(i, "id") + 1); err == nil || mock.probeCalls != 1 {
		t.Fatal("probed a connection outside the account")
	}
	c["enabled"] = false
	if err = save(ctx, s.Store.Pool, "clients", c); err != nil {
		t.Fatal(err)
	}
	if _, err = call(num(i, "id")); err == nil || mock.probeCalls != 1 {
		t.Fatal("inactive client was probed")
	}
}
func TestNativeProbeConfigOmitsServerPrivateKey(t *testing.T) {
	server, err := panelTestWirePair()
	if err != nil {
		t.Fatal(err)
	}
	client, err := panelTestWirePair()
	if err != nil {
		t.Fatal(err)
	}
	c := Data{"protocol_settings": Data{"private_key": client["private_key"], "public_key": client["public_key"], "address": "10.66.1.2/32"}}
	i := Data{"protocol": "amneziawg", "core": "native", "port": 8443, "protocol_settings": Data{"secret_key": server["private_key"], "public_key": server["public_key"]}}
	core, config, err := connectionProbeConfig(c, i, Data{"public_host": "vpn.example"})
	if err != nil || core != "native" {
		t.Fatal(config, err)
	}
	raw, _ := json.Marshal(config)
	if strings.Contains(string(raw), wireHex(str(server, "private_key"))) || !strings.Contains(string(raw), wireHex(str(client, "private_key"))) {
		t.Fatal("probe contains incorrect private key material")
	}
}
