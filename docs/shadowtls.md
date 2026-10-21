# ShadowTLS v3

VeloRay manages a public ShadowTLS v3 listener and an authenticated multi-user
Shadowsocks 2022 inner inbound using the bundled sing-box 1.14.3. This integration
is TCP only. It does not create a UDP listener, TUN device or privileged host rule.

## Configuration

In Inbounds, choose sing-box → ShadowTLS v3. RAW and no **additional** stream
security are selected automatically: the ShadowTLS wrapper provides the TLS
handshake. Enter a handshake hostname whose server supports TLS 1.3 and presents
a valid certificate, plus its port (default 443). No local certificate/key files
are required. Strict mode is enabled and wildcard SNI is disabled.

The panel generates an inner server key on creation if omitted. Existing keys
must be preserved when editing; a replacement must be standard padded base64 of
16 bytes. The inner method is fixed to `2022-blake3-aes-128-gcm`.

Create normal Clients or add this inbound to a Customer/Service. Each member gets
an independent 16-byte inner user key and a separate ShadowTLS password. Existing
customer membership changes preserve these credentials. The same active-account
filter used for expiry, quota exhaustion and disablement removes a member from
both layers. With no active members, neither listener is emitted. Apply changes
through the usual validated deployment/rollback path; sing-box changes restart
its configuration rather than using Xray's hot-add API.

## Delivery

The subscription portal offers sing-box JSON and Clash/Mihomo YAML. JSON contains
both the Shadowsocks outbound and its ShadowTLS detour; selector groups include
only the usable Shadowsocks entry. Mihomo uses its `shadow-tls` plugin, version 3,
and disables UDP. Generic raw/Base64 and Xray exports are unavailable for this
protocol; no speculative portable URI is emitted. Client apps need support for
these specific formats and ShadowTLS v3 with Shadowsocks 2022.

Delivery-host address/port overrides change the dialled endpoint. The handshake
SNI remains the configured handshake hostname; an ordinary CDN/path/SNI override
cannot turn this TCP protocol into a WebSocket/HTTP transport. Production JSON
retains certificate verification and requires TLS 1.3.

## Accounting and boundaries

The final inner authenticated user is the authority for traffic, first use and
concurrent-IP policy. It retains the canonical managed inbound tag, so existing
routing references target the authenticated payload. The wrapper is a gate; it
is not counted as a second copy of user traffic. Native live-connection telemetry
retains the source IP and canonical per-client principal.

The inner TCP listener binds only to `127.0.0.1` on an automatically assigned port;
it still requires the full Shadowsocks 2022 server/user key pair. The wrapper
injects directly into that inbound. Both layers authenticate, but their user
identities are not cryptographically bound together by the upstream protocol:
combining a valid wrapper credential with another active inner credential is
attributed to the authenticated inner account. Do not treat a wrapper password
as a separate per-account authorization boundary.

Tests validate complete server/client graphs against the shipped core, exercise
two users through a local TLS 1.3 handshake fixture, verify native per-user
counters and original source IP, reject either invalid credential, and check
inactive/quota-exhausted listener removal. A browser regression verifies creation
and editing preserve handshake options and keys. Real external handshake hosts,
network-specific blocking resistance and third-party client apps require
operator verification; Mihomo export follows its documented plugin schema and
is not an independent Mihomo engine round trip.

References: [sing-box inbound](https://sing-box.sagernet.org/configuration/inbound/shadowtls/),
[sing-box outbound](https://sing-box.sagernet.org/configuration/outbound/shadowtls/),
[Mihomo Shadowsocks plugin](https://wiki.metacubex.one/en/config/proxies/ss/).
