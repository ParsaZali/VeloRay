package panel

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/PaarSaAm/VeloRay/internal/nativecore"
)

func TestIntegrationNativeCoreConfigurationAccountingAndExports(t *testing.T) {
	for _, protocol := range []string{"mtproto", "amneziawg"} {
		t.Run(protocol, func(t *testing.T) {
			s, ctx := integration(t)
			mock := &nodeMock{running: true}
			n, i, c := fixture(t, s, ctx, mock)
			i["core"], i["protocol"], i["transport"], i["security"], i["flow"] = "native", protocol, "raw", "none", ""
			i["protocol_settings"] = Data{"fake_tls_host": "www.example.com"}
			if protocol == "amneziawg" {
				server, err := panelTestWirePair()
				if err != nil {
					t.Fatal(err)
				}
				client, err := panelTestWirePair()
				if err != nil {
					t.Fatal(err)
				}
				i["protocol_settings"] = Data{"secret_key": server["private_key"], "public_key": server["public_key"]}
				c["protocol_settings"] = Data{"private_key": client["private_key"], "public_key": client["public_key"], "address": "10.66.1.2/32"}
			}
			c["name"] = "مشتری >>> : B"
			c["credential"] = "independent-native-client-secret"
			c["device_limit"] = 0
			if err := validateInbound(ctx, s.Store.Pool, s.Config, i); err != nil {
				t.Fatal(err)
			}
			if err := save(ctx, s.Store.Pool, "inbounds", i); err != nil {
				t.Fatal(err)
			}
			if err := validateClient(ctx, s.Store.Pool, c); err != nil {
				t.Fatal(err)
			}
			if err := save(ctx, s.Store.Pool, "clients", c); err != nil {
				t.Fatal(err)
			}
			cfg, err := buildConfig(ctx, s.Store.Pool, n)
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(cfg["_native"])
			native, err := nativecore.Decode(raw)
			if err != nil || len(native.Inbounds) != 1 || len(native.Inbounds[0].Users) != 1 {
				t.Fatal(string(raw), err)
			}
			for _, v := range array(cfg, "inbounds") {
				if oneOf(str(asData(v), "protocol"), "mtproto", "amneziawg") {
					t.Fatal("native protocol leaked into Xray")
				}
			}
			principal := native.Inbounds[0].Users[0].Name
			if strings.Contains(principal, ">>>") || canonicalNaiveUsername(principal) != fmt.Sprintf("veloray:%d:%s", num(c, "id"), str(c, "name")) {
				t.Fatal("unsafe metric identity")
			}
			if protocol == "mtproto" {
				link, err := url.Parse(shareLink(c, i, n))
				if err != nil || link.Scheme != "tg" || link.Host != "proxy" || link.Query().Get("secret") != native.Inbounds[0].Users[0].Secret {
					t.Fatal(link, err)
				}
			} else {
				profile := profile(c, i, n)
				for _, expected := range []string{"Jc = 4", "S4 = 16", "H4 = 456789012", "AllowedIPs = 0.0.0.0/0\n"} {
					if !strings.Contains(profile, expected) {
						t.Fatal("incomplete AWG export", expected, profile)
					}
				}
			}
			mock.stats = []any{Data{"name": "user>>>" + principal + ">>>traffic>>>uplink", "value": 100}, Data{"name": "user>>>" + principal + ">>>traffic>>>downlink", "value": 200}}
			mock.statMetadata = Data{"online_users": []any{principal}, "online_available": true, "online_ips_available": true, "online_ip_cores": Data{"native": true}, "online_ips": Data{principal: Data{"203.0.113.1": time.Now().Unix()}}, "core_ledgers": Data{"native": "native-stable-ledger"}}
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
			if err != nil || num(fresh, "used_traffic_bytes") != 300 {
				t.Fatal("native counters duplicated or lost", fresh, err)
			}
			ips, complete := connectedIPs([]Data{fresh}, map[int64]Data{num(i, "id"): i}, map[int64]Data{num(n, "id"): n}, time.Now().UTC())
			if !complete || len(ips) != 1 {
				t.Fatal("native endpoint missing", ips, complete)
			}
			request := httptest.NewRequest("GET", "/sub/"+str(c, "subscription_token")+"?format=portal", nil)
			response := httptest.NewRecorder()
			s.subscription(response, request)
			if response.Code != 200 {
				t.Fatal(response.Code, response.Body.String())
			}
			if protocol == "mtproto" && !strings.Contains(response.Body.String(), "https://t.me/proxy?") {
				t.Fatal("Telegram action missing from portal")
			}
			if protocol == "amneziawg" && !strings.Contains(response.Body.String(), "connection=0") {
				t.Fatal("AmneziaWG download missing from portal")
			}
		})
	}
}

func panelTestWirePair() (Data, error) {
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return Data{"private_key": base64.StdEncoding.EncodeToString(key.Bytes()), "public_key": base64.StdEncoding.EncodeToString(key.PublicKey().Bytes())}, nil
}
