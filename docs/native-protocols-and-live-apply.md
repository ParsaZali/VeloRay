# Native protocols, Xray JSON and live apply

The supported runtime pair remains Xray 26.3.27 and sing-box 1.14.3. The server
binaries are independently bundled dependencies with their own source/license
notices; no upstream GPL code is copied into panel source.

| Protocol | Server core / transport | Security | Exports |
| --- | --- | --- | --- |
| TUIC v5 | sing-box / RAW, UDP listener | TLS required | URI, sing-box, Mihomo |
| Naive | sing-box / RAW, TCP HTTP/2 listener | TLS required | `naive+https` URI, sing-box for compatible client builds |
| Snell v6 | sing-box / RAW, TCP listener | Native PSK + per-user key | sing-box 1.14-compatible JSON |

Create these in Inbounds, with the same certificate-path fields as other TLS
protocols. TUIC creates a UUID plus a separate random password per client, uses
BBR by default and disables zero-RTT. Its `protocol_settings.congestion_control`
may be `bbr`, `cubic`, or `new_reno`. Naive usernames use the panel's stable telemetry
identity. Snell uses `protocol_settings.psk` (generated on creation) and optional
`mode`: `default`, `unshaped`, or `unsafe-raw`; the client credential is its user key.
These protocols use ordinary client ownership, shared quota/expiry and source-IP
rules. With no enabled users the listener is omitted to prevent shared-password
or unauthenticated access. Port inspection treats TUIC as UDP.

**Client compatibility matters:** the bundled CGO-free sing-box server implements
Naive inbound, but does not include the optional Chromium/cronet Naive outbound.
A compatible client build is required for that exported outbound. Snell v6 is not
exported as older Snell v3/v4 or advertised as broadly Mihomo-compatible.

## Xray subscription export

`/sub/TOKEN?format=xray` returns an array of named Xray client configurations,
one per eligible delivery host/connection. Supported protocols: VLESS, VMess,
Trojan and Shadowsocks; supported transports: RAW, WS, gRPC, XHTTP, HTTPUpgrade,
MKCP. Unsupported combinations return an explicit error. Server certificate paths,
private REALITY keys, server handshake targets and socket options are omitted.
The exporter supplies only public client TLS/REALITY settings. This endpoint uses
the same HWID admission, quota checks and no-store headers as other feeds.

Settings → subscription templates provides `subscription_xray_template`: an
optional JSON object applied per connection. Generated outbounds and remarks
replace dynamic fields; other template fields can supply a loopback SOCKS inbound,
DNS or routing. Leave blank for the built-in output. Default subscription format
can also be `xray`. The delivery check reports format compatibility.

## Live client additions

For an already deployed Xray configuration with HandlerService enabled, adding
users to unchanged VLESS, VMess or Trojan listeners uses the loopback `api adu`
command. Listener/transport/routing/policy changes, credential updates and removals
retain full restart behavior. Removing a user must also end existing sessions for
quota/expiry enforcement; it is not advertised as a disconnect-free operation.
Sing-box changes retain restart behavior.

The classifier requires every old user to remain byte-equivalent and all other
configuration sections to match. It sends only additions via a mode-0600 temporary
file; credentials do not enter process arguments. The manager validates first,
observes durable traffic, journals the operation, persists the final config, then
adds users. It verifies the exact number acknowledged by Xray because the CLI can
exit successfully after per-user errors. Partial additions, failed acknowledgments
or other failures restore the previous configuration. Existing panel compensation
and crash-recovery behavior remains available. Idempotent operations do not add a
user twice.

## Verification

CI extracts the pinned bundled runtimes, runs the complete PostgreSQL/Go race
suite, validates the protocol/configuration matrix, exercises real Xray live-user
addition, and performs loopback TUIC/Snell round trips with authenticated-user
telemetry. An independent Naive HTTP/1.1 CONNECT fixture verifies TLS, ten
bidirectional transfers across the eight padded-record boundary, authenticated
principal attribution and rejection of invalid passwords/missing padding. This
does not claim testing of a Chromium/cronet or HTTP/2 client.

Naive Basic-auth usernames use a reversible colon-free encoding of the canonical
panel principal. Refresh existing Naive subscriptions after upgrading. Previously
generated colon-containing usernames could pass configuration validation but
could not authenticate over HTTP Basic. Counter and live-IP normalization retain
the canonical client ID for quota, online status and device enforcement. Native
sing-box auth-user rules use the encoded wire identity. A PostgreSQL integration
test checks that repeated samples charge usage once and preserve connected IPs.

The restricted development environment denies netlink sockets. Local full test
runs set `VELORAY_TEST_NO_NETLINK=1` and omit sing-box runtime round trips while
still running configuration checks; CI does not set that escape hatch. No successful
local network test is claimed for a test omitted under that flag.

AmneziaWG, MTProto, ShadowTLS chaining, SSH, additional client ecosystems, full
hot remove/update and structural hot apply remain tracked individually in the
competitor roadmap. They are not implemented by merely adding a protocol label.

CI gives `CAP_NET_ADMIN` only to its extracted temporary Xray binary so the existing
TUN configuration check can open a TUN device. This is runner-only test setup; it
does not modify bundled binaries or grant privileges to the panel process.


## Feasibility of additional engines and capture modes

This assessment uses the shipped Xray v26.3.27 JSON protocol registry and the
bundled sing-box 1.14.3 source/build tags, rather than a dropdown of protocol names.

| Capability | Bundled engine boundary | Required panel work before support can be advertised |
| --- | --- | --- |
| AmneziaWG | No AmneziaWG inbound in either current protocol registry; ordinary WireGuard is a different protocol | Package and supervise a separate compatible engine/module, then implement peer lifecycle, counters, revocation and client exports |
| MTProto | Absent from both current inbound registries | Separate MTProxy service, secret lifecycle, proven per-principal accounting and supported quota semantics |
| ShadowTLS | sing-box supplies a TLS wrapper and inner-inbound detour | Manage and validate the paired wrapper/inner graph, both authentication layers, exports, principal attribution and revocation |
| SSH server | sing-box supplies an SSH outbound, not an SSH server | Separate SSH server lifecycle, authorization/isolation, tunnel-only policy and independently tested traffic accounting |
| Naive Chromium client | Naive server is bundled; optional cronet outbound is excluded from the CGO-free build | Separate compatible Chromium/cronet client packaging and HTTP/2 interoperability coverage; HTTP/1.1 fixture coverage is shipped |
| TUN / transparent capture | Engine options exist but require host interfaces, policy routing and privileges | Confined privileged operations, management-path exclusions, rollback and host-level network tests; the default container grants no TUN or NET_ADMIN access |

These are implementation boundaries, not completed integrations. Enabling a new
protocol must preserve authenticated telemetry, quota/device enforcement and
crash-safe state restoration. An absent server is not substituted with an outbound
of the same name. The default agent cannot execute arbitrary shell commands or
control unrelated host services to approximate these features.

Sources: [pinned Xray protocol registry](https://github.com/XTLS/Xray-core/blob/v26.3.27/infra/conf/xray.go),
[pinned sing-box source](https://github.com/SagerNet/sing-box/tree/v1.14.3/protocol),
and this repository's `scripts/build-singbox.sh`.
