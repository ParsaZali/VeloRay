package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func diagnosticFixture() subscriptionConnection {
	return subscriptionConnection{
		client:  Data{"id": int64(9), "credential": "subscriber-secret", "name": "Alice", "protocol_settings": Data{}},
		inbound: Data{"id": int64(8), "name": "Main", "core": "xray", "protocol": "vless", "transport": "raw", "security": "none", "port": int64(443), "protocol_settings": Data{}},
		node:    Data{"id": int64(7), "public_host": "vpn.example"},
	}
}

func TestSubscriptionConnectionKeyBindsEndpointWithoutUsage(t *testing.T) {
	item := diagnosticFixture()
	key := subscriptionConnectionKey("instance-secret", item)
	item.client["used_traffic_bytes"] = int64(300)
	item.client["traffic_limit_bytes"] = int64(900)
	item.node["_telemetry_at"] = stamp()
	if got := subscriptionConnectionKey("instance-secret", item); got != key {
		t.Fatal("usage and telemetry invalidated the endpoint identity")
	}
	for _, field := range []string{"credential", "port", "public_host"} {
		changed := diagnosticFixture()
		switch field {
		case "credential":
			changed.client[field] = "rotated-secret"
		case "port":
			changed.inbound[field] = int64(8443)
		case "public_host":
			changed.node[field] = "other.example"
		}
		if subscriptionConnectionKey("instance-secret", changed) == key {
			t.Fatalf("%s did not change the endpoint identity", field)
		}
	}
	if subscriptionConnectionKey("different-instance", item) == key || len(key) != 64 {
		t.Fatal("endpoint identity must be instance-specific")
	}
}

func TestSubscriptionProbeCatalogIsBoundedAndPrivate(t *testing.T) {
	s := &Server{Config: Config{SecretKey: "instance-secret"}}
	first, second := diagnosticFixture(), diagnosticFixture()
	second.node["public_host"] = "second.example"
	options, revision := s.subscriptionProbeOptions([]subscriptionConnection{first, second})
	_, reversed := s.subscriptionProbeOptions([]subscriptionConnection{second, first})
	if revision != reversed || len(options) != 2 {
		t.Fatal("delivery ordering changed the configuration revision")
	}
	for _, option := range options {
		raw, err := json.Marshal(option.public)
		if err != nil || strings.Contains(string(raw), "subscriber-secret") || strings.Contains(string(raw), "outbounds") || !flag(option.public, "supported") {
			t.Fatal("catalog exposed credentials or lost supported connection", string(raw), err)
		}
	}
	items := make([]subscriptionConnection, 220)
	for index := range items {
		items[index] = diagnosticFixture()
		items[index].node["public_host"] = fmt.Sprintf("node-%d.example", index)
	}
	bounded, _ := s.subscriptionProbeOptions(items)
	if len(bounded) != 200 {
		t.Fatal("unbounded subscriber probe catalog")
	}
	for _, protocol := range []string{"wireguard", "naive", "tun", "tunnel"} {
		item := diagnosticFixture()
		item.inbound["protocol"] = protocol
		options, _ := s.subscriptionProbeOptions([]subscriptionConnection{item})
		if flag(options[0].public, "supported") || str(options[0].public, "reason") != "client_application_required" {
			t.Fatalf("%s must be checked by its compatible client", protocol)
		}
	}
}

func TestSubscriptionDiagnosticsOriginUsesActualSubscriptionPort(t *testing.T) {
	for _, test := range []struct {
		host, origin, header string
		ok                   bool
	}{
		{"sub.example:100", "https://sub.example:100", "1", true},
		{"sub.example:100", "https://sub.example", "1", false},
		{"sub.example", "https://sub.example:443", "1", true},
		{"SUB.EXAMPLE:443", "https://sub.example", "1", true},
		{"sub.example", "https://sub.example.attacker.test", "1", false},
		{"sub.example", "https://other.example@sub.example", "1", false},
		{"sub.example", "https://sub.example/path", "1", false},
		{"sub.example", "https://sub.example?query=1", "1", false},
		{"sub.example", "null", "1", false},
		{"sub.example", "https://sub.example", "", false},
		{"sub.example", "", "1", false},
	} {
		r := httptest.NewRequest(http.MethodPost, "https://"+test.host+"/sub/token?format=diagnostics", nil)
		r.Header.Set("Origin", test.origin)
		r.Header.Set("X-VeloRay-Diagnostic", test.header)
		if got := diagnosticSameOrigin(r); got != test.ok {
			t.Errorf("host=%q origin=%q header=%q: got %v", test.host, test.origin, test.header, got)
		}
	}
}

func TestSubscriptionDiagnosticsRemainDisabledAndDoNotAdmitBrowserDevices(t *testing.T) {
	s := &Server{}
	w := httptest.NewRecorder()
	s.subscriptionDiagnostics(w, httptest.NewRequest(http.MethodGet, "/", nil), Data{}, defaultSettings(), nil)
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "diagnostics_disabled") || w.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal(w.Code, w.Body.String(), w.Header())
	}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if err := s.diagnosticDevice(context.Background(), r, Data{}, Data{}); err != nil {
		t.Fatal("optional browser checks should not register a device", err)
	}
	if err := s.diagnosticDevice(context.Background(), r, Data{}, Data{"subscription_hwid_required": true}); err == nil {
		t.Fatal("required device identification was bypassed")
	}
}
