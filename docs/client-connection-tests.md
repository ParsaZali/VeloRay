# Client connection tests

Clients → Check subscription → Test connection runs an isolated diagnostic client
on the connection's node, using that user's real credential and selected delivery
host/port/SNI/transport. It returns measured request latency, status, timestamp,
`network_tested`, `vantage: node`, `traffic_counts` and a bounded error message.
It does not measure the subscriber's ISP path or mobile application's behavior.

`POST /api/clients/ID/connection-test` requires `clients.write`, normal ownership,
CSRF/session/API-key policy, and an active account/inbound/node. Body:
`{"inbound":42,"host_index":0,"url":"https://www.gstatic.com/generate_204"}`.
The inbound must belong to this client's shared account; a reseller cannot
supply an arbitrary outbound or target someone else's connection. `host_index`
matches the host index returned by `delivery-check`. Hidden hosts are rejected.
The user-supplied target must be public HTTPS, without credentials/fragments.
DNS results are checked and the numeric destination is pinned; redirects and
environment proxies are disabled, and TLS certificate verification remains on.

Xray and sing-box tests execute an independent temporary core with an authenticated
loopback HTTP proxy, selected generated client outbound/dependencies and logging
disabled. They do not rewrite or restart the production configuration. Direct,
blackhole, selectors, TUN and host WireGuard outbounds cannot masquerade as a
successful client test. Naive requires a separate compatible Chromium client and
is reported as unavailable in the current bundled probe engine.

MTProto authenticates FakeTLS, establishes the obfuscated transport and sends
`req_pq_multi` to Telegram DC 2; it verifies the `resPQ` constructor and request
nonce. No Telegram account, session or login is used. Its latency includes the
proxy/Telegram exchange; the HTTPS target field does not apply. AmneziaWG creates
an ephemeral userspace peer with the user's existing key/address, then requests
the public HTTPS destination through the actual tunnel. A peer has one roaming
endpoint: testing it from the node may temporarily move an already connected
AmneziaWG client's endpoint until that client sends again. Both require public
server endpoint resolution and an upgraded native-capable agent.

All test traffic follows normal accounting/quota rules and may activate a deferred
first-use duration. Do not use this as a free/billing-exempt health monitor. Native
AmneziaWG diagnostics can consume a user's current endpoint; operators should use
separate test accounts for automated checks. Existing outbound monitors remain a
separate, credential-free route-health feature.

Requests are limited to one concurrent diagnostic per core/agent, 64 KiB client
configuration, 16 outbound dependencies, a 10-second overall deadline and 4 KiB
response sampling. Temporary files are private, removed after use, and core output
is discarded. Audit entries contain only inbound ID and success, never credentials
or target URL. Automated tests include real paired Xray/sing-box inbound auth,
MTProto request-nonce rejection and a real AmneziaWG diagnostic peer round trip.
