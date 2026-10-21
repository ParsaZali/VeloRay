# MTProto and AmneziaWG

These listeners run inside the VeloRay agent under `core: native`. They do not
invoke shell commands, add host routes, change firewall rules, open a control
socket, or require a TUN device / NET_ADMIN. TCP/UDP ingress must be reachable
through the operator's existing firewall and container networking.

## MTProto

Create a native MTProto inbound, set a TCP port and a public `fake_tls_host` in
`protocol_settings`. The host must serve HTTPS and be reachable from the node.
The MIT-licensed, pinned mtg v2.2.8 library implements the FakeTLS/obfuscated
Telegram transport. VeloRay dispatches users on the same port using the FakeTLS
HMAC. Each client receives a separate 128-bit key, derived with domain separation
from its random panel credential. The Telegram link has the `ee` secret form;
rotating the credential revokes the old key on deployment. This is an ordinary
Telegram proxy, not a tunnel for other applications or a sponsored-channel proxy.

The portal offers Telegram open links and per-connection QR codes. RAW and JSON
feeds include the Telegram URI; Xray/sing-box/Clash formats explicitly report
incompatibility. Normal HTTPS probes / unknown keys follow mtg's domain-fronting
path and are excluded from authenticated-user billing. No operator TLS key is
needed. Users on one listener share its FakeTLS hostname.

Authenticated relay traffic, online source IPs, concurrent-IP caps, account
expiry and the existing shared customer quota / renewal policy are supported.
Expiry closes active sessions; quota revocation uses the panel's existing
collection and compensated deployment cycle, so this is not a packet-exact byte
cap. HWID admission controls subscription delivery in compatible applications;
Telegram itself does not send the panel an HWID.

## AmneziaWG

Create a native AmneziaWG inbound, generate the server X25519 keypair and choose
its UDP port. Client keypairs and unique private addresses are generated through
the existing client/customer flow. The implementation embeds MIT-licensed
amneziawg-go at `v0.2.20-0.20260724121833-457d920a1a7d`, with an Apache-2.0 gVisor
userspace TCP/UDP stack. Peer source addresses are authenticated by the engine's
AllowedIPs. Public TCP and UDP destinations are relayed through scoped sockets;
private/local management destinations are blocked. This implementation provides
TCP/UDP internet access, not host-LAN routing or ICMP forwarding.

Supported obfuscation controls: Jc, Jmin, Jmax, S1–S4 and four distinct fixed H1–H4
values. Header ranges and I1–I5 instruction chains are not exposed. MTU is bounded
to 1280–1420. Amnezia-compatible `.conf` exports include the server parameters,
client private key, optional preshared key, address, DNS and endpoint. These are
not ordinary WireGuard exports: import them in a compatible Amnezia client.
Each peer currently has one private /32 or /128 address; the export routes that
address family only. A shared customer can have multiple independent connections.
Use `?format=awg&connection=N` to download one eligible connection from its token.
Indices refer only to the authorized subscription's current connections.

Traffic is measured as authenticated plaintext IP bytes in both directions,
including IP/TCP/UDP headers and retransmissions, excluding encryption/junk
packets. Each peer uses one roaming endpoint; concurrent-IP caps are unavailable
for this protocol. Expiry / revocation blocks packets and closes active relays.

## State, updates and limits

Both protocols use the existing durable agent manager: mode-0600 configuration,
write-ahead operations, idempotency, rollback, core-scoped persistent counters and
crash recovery. Counters survive hot configuration changes without resetting the
accounting epoch. Agent process restarts preserve the durable totals and start a
new raw-counter epoch. Native user/peer updates preserve unchanged users' active
connections. New/removed ports are prepared/closed by the native engine; changes
to other cores retain their documented deployment behavior.

Panel deployment requires `/native/status` capability preflight. Upgrade each
remote agent before deploying a native inbound. Supported bounds: 512 native
listeners, 16 AmneziaWG listeners, 4096 users per listener, 16384 users per agent,
4 MiB combined native configuration and 1024 concurrent relays per listener.
Native configs and ledgers live beside the existing agent state files and must
be included in backups. A damaged ledger is not silently discarded.

Automated verification includes independent FakeTLS/obfuscation wire fixtures,
isolated per-user accounting and hot revocation; two actual AmneziaWG peers
perform encrypted TCP and UDP round trips, with unchanged-peer continuity after
revocation. Agent tests cover idempotency, stable epochs, rollback and recovery.
PostgreSQL integration tests verify schema/exports, delimiter-safe principals,
online attribution and repeated-sample billing. These fixtures do not claim a
live public Telegram DC or a particular mobile app has been tested.
