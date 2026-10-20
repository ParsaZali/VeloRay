<div align="center">

# VeloRay 1.0

**A server panel for connections, clients and distributed VPN infrastructure.**

Xray as the primary core. sing-box as the secondary core. One interface for both.

[Install](#installation) · [Features](#features) · [Subscriptions](#subscriptions) · [API](#api) · [Operations](#server-operations) · [Build](#development-and-verification)

</div>

## Features

| Area | Capabilities |
|---|---|
| Dashboard | Server resources, live client status, actual upload/download counters, dual-series traffic charts, transfer rate and time-range zoom |
| Clients | Individual and batch creation, quotas, scheduled renewals, expiry, first-use activation, device limits, usage export, bulk actions |
| Inbounds | Full-width directory, separate creation editor, protocol presets, custom selectors, port inspection, cloning between nodes, Xray VLESS/Trojan fallbacks |
| Cores | Xray primary; sing-box secondary; configuration validation, durable traffic accounting and deployment recovery |
| Outbounds | Built-in routes, node/core filters, guided proxy setup, protocol templates, duplicate and enable/disable actions |
| Routing | Protected defaults, ordered rules, domain/IP/port/source/user matching, routing presets, sing-box selectors and Xray load balancing |
| Subscriptions | Portable URI feeds, Base64, Clash/Mihomo, sing-box JSON, account metadata, browser portal and QR codes |
| Announcements | Subscription announcement and announcement URL; editable in Settings |
| SNI scanner | Node-side DNS and TLS 1.3 probes, custom domains, suggested domains, latency and certificate checks |
| Accounts | Administrator, reseller, viewer and custom roles, inbound assignments, client and traffic budgets |
| Security | Password policy, encrypted sensitive fields, scoped API keys, CSRF, administrator 2FA, optional pre-bound OIDC login and audit history |
| Migration | Bounded SQLite and relational JSON adapters, private preview, selection, port overrides and disabled imports |
| Automation | Telegram controls, signed event outbox, independent webhook/Discord/email destinations, configurable account alerts, scheduled backups and REST API |
| Operations | Isolated outbound health probes, failure/recovery alerts, private provider budgets, monthly raw-traffic cycles, structured diagnostics and CSV |
| Interface | Dark and light themes, English and Persian, responsive layouts, local fonts, custom dropdowns, Gregorian/Jalali date picker and checkboxes |

Client action menus stay above scrollable tables. SNI and configuration dialogs use bounded, responsive layouts. Theme changes reveal the new palette from the theme button, with reduced-motion and browser fallbacks.

The product version is **1.0**. Maintenance updates retain this version until the project owner requests a version change. The frontend package uses the equivalent npm version `1.0.0`.

New management and subscription capabilities are described in the
[delivery guide](docs/subscription-customization.md),
[3x-ui HTML compatibility guide](docs/legacy-subscription-templates.md),
[customer guide](docs/customer-management.md),
[external policy guide](docs/subscription-policies.md),
[independent service guide](docs/independent-subscriptions.md),
[OIDC/live dashboard guide](docs/oidc-and-live-dashboard.md) and
[notification guide](docs/webhooks.md) and
[operations/email guide](docs/operations-and-alerts.md). The [parity backlog](docs/competitor-parity-roadmap.md)
records remaining architectural work; this release does not claim every feature
of every competing panel.

See the [fallback and balancing guide](docs/fallbacks-and-balancing.md) for network
policy controls, native engine limits and verified behavior.

## Installation

### Requirements

- Ubuntu 24.04 with systemd and apt (currently the explicitly supported installation target).
- `amd64` or `arm64` architecture.
- Root access for service installation.
- PostgreSQL. The installer can configure a local database.
- A public IP or domain and available panel/VPN ports.

### Install from GitHub

```bash
curl -fsSL https://raw.githubusercontent.com/ParsaZali/VeloRay/main/install.sh -o /tmp/veloray-install.sh
sudo bash /tmp/veloray-install.sh install --lang en
```

The installer retrieves the project, verifies its ready-made runtime, configures PostgreSQL, installs the panel and agent, installs both cores and sets up HTTPS through Nginx. It asks for the public address and administrator password when needed.

Use `--lang fa` for Persian installation prompts. The server management menu stays in English.

The default panel port is `8443`. Open the address printed after installation and sign in with the administrator account. Add an inbound before creating clients.

### Install an extracted release

```bash
cd VeloRay
sudo bash install.sh install --lang en
```

Ready-made packages contain both Linux architectures. Go and Node.js are only required when building from source or when a source change invalidates the runtime manifest.

### Upgrade an existing installation

```bash
sudo veloray backup
sudo veloray update
sudo veloray doctor
```

The updater checks the latest **ParsaZali/VeloRay** `main` commit and proceeds only after the exact commit passes GitHub Actions CI. It downloads the installer at the immutable commit, verifies its Git blob hash, creates a restricted pre-upgrade backup and performs a post-install health check. If installation fails, it attempts an application binary/service rollback and preserves the backup for manual database recovery. Schema changes are idempotent; a database rollback is **not** performed automatically.

#### Automatic updates from GitHub

After deploying a version containing the auto-update feature, enable daily automatic checks on the server **once**:

```bash
sudo veloray auto-update enable
sudo veloray auto-update status
sudo veloray auto-update check
```

To stop the timer while retaining your current installation, run `sudo veloray auto-update disable`. To perform an immediate CI-verified update, run `sudo veloray auto-update run` (or `sudo veloray update`). The systemd timer checks once per day around 05:00 server time with a randomized delay. Updating does not require any future user action once this timer is enabled.

This feature is **disabled by default**. It never applies untested commits, never performs unattended updates when GitHub CI status is unknown, and does not automatically restore PostgreSQL data after a failure. It follows `main`, so review permissions for everyone able to push to that branch. For a sensitive production environment, protecting `main` and requiring code review before merges is essential. You can monitor the installation under **Settings → System → Verified GitHub auto-update**. No administrator endpoint directly runs root update commands. See [the maintenance notes](docs/maintenance-audit.md).

Previous frontend assets are retained during the switch so open browser sessions can finish loading their existing files.

The second core requires the complete 1.0 runtime. Upgrade remote agents before assigning sing-box inbounds to them.

## Getting started

1. Open **Inbounds → New inbound**.
2. Select the node, core, protocol, transport and security mode.
3. Generate the required credentials and check the port.
4. Create and deploy the inbound.
5. Open **Clients → Add client**, select the inbound and set its policy.
6. Give the client its subscription URL or connection URI.

Changes affecting a node are validated by the relevant core before application. Failed deployments use operation journals and compensating rollback. Unavailable nodes retain pending recovery jobs.

## Protocols and transports

| Core | Inbound protocols | Transports |
|---|---|---|
| Xray | VLESS, VMess, Trojan, Shadowsocks, Hysteria2, WireGuard, HTTP, SOCKS, tunnel, TUN | TCP/raw, WebSocket, gRPC, XHTTP, HTTPUpgrade, mKCP, Hysteria2/QUIC where supported |
| Secondary core | VLESS, VMess, Trojan, Shadowsocks, Hysteria2, TUIC v5, Snell v6, Naive TLS, HTTP, SOCKS | TCP/raw, WebSocket, gRPC, HTTPUpgrade, Hysteria2/QUIC where supported |

Security modes include TLS and REALITY for supported combinations. The editor and backend reject incompatible combinations. Certificate and private-key paths must exist and be readable on the selected node. TUN and WireGuard need the appropriate host networking permissions.

HTTP and SOCKS have no managed per-client quota accounting in this panel. Shadowsocks uses one managed client per inbound and inbound-level statistics.

## Client policies

### Traffic and multipliers

**Used** is the amount charged against the quota. A multiplier of `3` means a 1 GiB transfer consumes 3 GiB of quota. The client directory displays this charged amount once, with `3x` beside it.

The subscription's usage header uses the charged amount as well. **Overview** and **Top consumers** show actual transferred bytes without a multiplier. Traffic overview separates **Upload** and **Download**, with a separate chart series for each direction. Transfer rate uses the real bytes recorded in each five-minute interval. The chart has a custom time-range control with draggable handles, keyboard support and a full-range reset.

Existing usage is retained during the upgrade. Traffic collected before directional accounting was available appears as **Earlier traffic**; its direction is not guessed. New transfers are recorded separately. Resetting an account's usage resets its current upload/download counters alongside its quota usage, while lifetime counters and retained traffic history remain intact.

When the effective multiplier exceeds `1`, exported configuration names receive a suffix such as `2.0x`. The stored client name stays unchanged. Decimal multipliers support up to three decimal places. Negative values represent discounts: `-0.5` charges half of the transferred bytes.

Persistent counters and fractional remainders preserve usage across polling and core restarts. Restore the agent's accounting state with the database when recovering from a backup.

### Start after first use

Enable **Start after first use** and enter the number of days. The account remains without a running expiry clock until VPN traffic or an authenticated online session is observed. Fetching a subscription or opening the browser portal does not start it. The first-use timestamp is retained, so later connections do not extend the expiry.

### Devices

Set **Devices** to the allowed number of concurrent source IPs. `0` means unlimited. When another IP is admitted beyond the limit, an earlier IP is denied for ten minutes. Shared accounts enforce one limit across their connections.

The core exposes source IPs, not hardware identities. Devices behind the same NAT share one IP; a device changing networks can appear as another IP. Enforcement follows the five-second reconciliation loop. Applying routing policy restarts the affected core, so other connections on that core can briefly reconnect.

### Connected IPs and registered devices

Open a client’s action menu and select **IP's** for current VPN source IPs, or **Device's** for devices registered through subscription downloads. IP lists refresh from native core telemetry and show node/inbound associations; stale or unavailable telemetry is reported explicitly.

Device registration uses the optional `X-HWID` header, with `X-Device-OS`, `X-Ver-OS`, `X-Device-Model` and `User-Agent` metadata. First and latest subscription downloads are recorded, with one entry per identifier across a shared account. Browser portal visits, HEAD requests and failed downloads do not register devices. Apps without HWID remain compatible but cannot populate this list. Registered devices are not an online-device count, and their metadata is reported by the client app. The existing Devices limit still controls concurrent source IPs.

### Renewals and shared accounts

Quota renewal resets charged usage on the configured interval. Expiry and quota exhaustion disable the account. Imported shared accounts retain one quota, expiry and subscription across their connections.

## Outbounds and routing

Select a node and core to see its built-in outbounds and custom entries. Xray starts with `direct` and `blocked`; sing-box starts with `direct`. Node configuration patches are reflected in the Xray defaults. Built-in entries can be inspected; custom entries can be edited, duplicated, enabled, disabled or deleted.

Create outbounds with a unique tag. Configure server address, port, credentials, TLS, SNI and common transports through the guided form. **Advanced configuration** retains access to the native JSON and preserves extra settings when guided fields are edited. All submitted settings are checked by the running core.

Routing displays Xray's protected panel API route and each core's default route. Custom rules have a numeric priority; lower values are evaluated first, with creation order resolving equal priorities. Templates cover private IP blocking, BitTorrent blocking and direct HTTPS; Xray also offers an ad-domain template using its bundled geosite database. Templates are applied only when you save them.

Use domain, IP, port, source or user match controls and select the destination outbound. Domain entries use the native matching format, such as `domain:example.com` for Xray or `example.com` as a sing-box domain suffix. Ports accept individual values and ranges such as `443,8000-9000`:


- Xray uses `outboundTag`, `domain`, `ip`, `source`, `user` and `port`.
- The secondary core uses `outbound`, `domain_suffix`, `ip_cidr`, `source_ip_cidr`, `auth_user` and `port`.

sing-box blocking uses `action: "reject"`; it does not require a `blocked` outbound. Protocol and domain matching automatically enables a sniff action before custom rules on that core.

Rules and outbounds are isolated by node and core. Xray reserves `api`, `direct` and `blocked`. Internal device enforcement rules precede user-created rules. Dependencies are validated before deployment.

## Subscriptions

The default browser page is now a simple light/dark page. Settings → Subscription also offers the previous dashboard, the compact page, or a custom HTML file. File paths, local assets, client configuration templates, delivery hosts, HWID admission and the compatibility report are documented in [Subscription customization](docs/subscription-customization.md).

Each subscription URL carries a private token:

```text
https://YOUR_PANEL/sub/TOKEN
```

| Format | URL suffix | Content |
|---|---|---|
| Auto | none | Browser portal or detected client feed |
| Base64 | `?format=base64` | Encoded portable connection URIs |
| Raw | `?format=raw` | Plain connection URIs |
| Clash/Mihomo | `?format=clash` | YAML-compatible JSON with proxies, a group and a rule |
| Secondary-core JSON | `?format=singbox` | Outbound profile and selector |
| Metadata | `?format=json` | Account policy and connection details |
| WireGuard | `?format=wireguard` | WireGuard profile when available |

Clash/Mihomo and sing-box user agents select their respective formats automatically. Other URI clients, including V2Box, receive the Base64 feed by default. Use explicit format URLs when an app does not send a recognizable user agent.

Feeds include `Subscription-Userinfo`, `Profile-Title`, `Profile-Update-Interval`, `Profile-Web-Page-URL`, and configured `Support-URL`, `Announce` and `Announce-URL` headers. Announcement text is Base64 encoded. Configure announcements and portal options in **Settings → Subscription** or **General** as presented in the panel.

The browser portal keeps usage, expiry, subscription formats and a QR code in a compact layout. Connection URIs and app information expand on demand. Settings reports database connectivity independently of host metrics, and its Uptime counter starts with the panel process.

Protocol, transport and header support vary between app releases. Export formats reject configurations that cannot be represented. Announcement rendering depends on the app's support for these headers.

## SNI scanner

Click the rocket inside **Server name**. Scan one or more domains, or use **Find SNI** to test the built-in candidate list from the chosen node.

Probes verify public DNS addresses, certificate hostname validation, a TLS 1.3 handshake using X25519, negotiated ALPN and response time. The fastest successful result is listed first. Private and loopback destinations are excluded. A successful probe establishes TLS reachability from that node; it does not measure reachability from every client network.

## Remote nodes

```bash
sudo bash /tmp/veloray-install.sh node --lang en
```

The node installer prints its agent token and certificate fingerprint. Add the node in the panel with its public host, agent URL and token. Remote agents require HTTPS. Use a trusted certificate or the configured CA bundle; restrict the agent port to the panel server.

| Service | Default address |
|---|---|
| HTTPS panel | public `8443` |
| Web backend | `127.0.0.1:8610` |
| Agent | `127.0.0.1:9191` on the local node |
| Xray statistics | `127.0.0.1:10085` |
| Secondary statistics | `127.0.0.1:10086` |
| Secondary connection controller | `127.0.0.1:10087` |

Statistics and connection-controller ports remain local. Remote agents use their configured HTTPS listen address.

## API

Open **API** to create keys, select a scope, set expiry, revoke keys or reset them. A full key is returned once at creation or reset; subsequent reads contain only its prefix. Reset immediately invalidates the previous token. Tokens are hashed in the database.

| Scope | Access |
|---|---|
| Read | Read endpoints allowed by the owner's permissions |
| Write | Allowed read/write endpoints, excluding administrator-only operations |
| Admin | Administrator endpoints when owned by an administrator |

```bash
curl -H "Authorization: Bearer $VELO_API_KEY" https://YOUR_PANEL/api/clients/
```

The key owner's role, assigned inbounds and client ownership still apply. Browser sessions use CSRF tokens; bearer API calls use their key scopes.

| Resource | Operations |
|---|---|
| `/api/clients/` | List, create; patch/delete by ID; batch, bulk, toggle, reset-usage |
| `/api/inbounds/` | List, create; patch/delete by ID; clone |
| `/api/outbounds/` | List, create; patch/delete by ID |
| `/api/routing_rules/` | List, create; patch/delete by ID |
| `/api/nodes/` | List, create; probe, deploy, ports, config preview/validation, core status/restart, SNI scan |
| `/api/admin/users/` | Administrator account management |
| `/api/admin/api-keys/` | Key management; `POST /:id/reset` rotates a key |
| `/api/settings` | Settings |
| `/api/openapi` | OpenAPI schema for the principal CRUD resources |

Lists are paginated using `limit` and `offset`. See the in-panel reference for additional action endpoints.

## Import data

Open **Settings → Import data**. Choose the destination node and a SQLite backup or a relational tables JSON export. Preview it, select supported entries, resolve port conflicts and acknowledge the review before committing.

Imports initially save inbounds disabled. Review target-node certificates, ports and connection settings before activation. Unsupported entries show their reason and remain unavailable for selection. Passwords for source panel administrators are excluded.

The parser accepts at most 64 MiB, 20,000 source records, 500 inbounds and 10,000 client connections per import. Compressed archives and SQL dumps are not direct import formats.

For relational source databases:

```bash
python3 scripts/export-relational.py --output /private/path/migration.json
python3 scripts/export-relational.py --sqlite /private/path/source.db --output /private/path/migration.json
```

The PostgreSQL exporter reads `SQLALCHEMY_DATABASE_URL` or `DATABASE_URL` and requires `psql`. Run it in the source environment with read access.

## Server operations

```bash
sudo veloray
```

The English menu groups services, configuration, maintenance and networking in a vertical list. Commands also work directly:

```bash
sudo veloray status
sudo veloray doctor
sudo veloray url
sudo veloray restart
sudo veloray logs web
sudo veloray logs agent
sudo veloray logs xray
sudo veloray logs singbox
sudo veloray reset admin
sudo veloray domain panel.example.com
sudo veloray port 8443
sudo veloray ssl admin@example.com
sudo veloray backup
sudo veloray restore /path/to/backup.tar.gz
sudo veloray backup-schedule daily
sudo veloray ports
sudo veloray inbounds
sudo veloray inbound-port 1 2053
sudo veloray update
```

Backups include PostgreSQL, panel and node environment files, both core configurations, service definitions, certificates and agent accounting state. Keep backups private: they contain credentials. Restore into a compatible installation and run the health check afterward.

Operation logs are stored under `/var/log/veloray`. Uninstall preserves configuration and database contents for recovery.

## Development and verification

Go 1.26.9 and Node.js 22.12+ are used by the build workflow. Copy `.env.example` for local development and generate distinct secret and field keys. Use an isolated PostgreSQL database.

```bash
go run ./cmd/veloray migrate
go run ./cmd/veloray bootstrap
go run ./cmd/veloray serve
```

```bash
export VELORAY_TEST_DATABASE_URL='postgres://USER:PASSWORD@127.0.0.1:5432/veloray_test?sslmode=disable'
go test -race ./... -count=1
go vet ./...
cd frontend
npm ci
npm run format:check
npm run build
npm test
```

Set `VELORAY_TEST_XRAY_BINARY` and `VELORAY_TEST_SINGBOX_BINARY` to run protocol-matrix tests against real core validators. Browser tests expect an isolated running panel and administrator credentials through `VELORAY_E2E_URL` and `VELORAY_E2E_PASSWORD`.

### Runtime packages

```bash
bash scripts/build-runtime.sh
bash scripts/verify-runtime.sh
python3 scripts/package-release.py
```

The build produces `runtime/linux-amd64.tar.xz` and `runtime/linux-arm64.tar.xz`, with SHA-256 checksums and source manifests. Modified source inputs cannot silently reuse an outdated runtime.

The secondary executable is built from pinned upstream source with the statistics and connection APIs enabled. Its local connection metadata includes the authenticated user for device enforcement. The source archive, upstream license, GPL text and build recipe accompany the distributed runtime. Xray binaries and assets use pinned release checksums. Bundled dependency licenses are included in both runtime packages.

Inter and Vazirmatn fonts are served locally. No font CDN is required.

## License

VeloRay is distributed under the repository's [LICENSE](LICENSE). Bundled cores, fonts and dependencies retain their own licenses and attribution.

### Shared customer management and protocol extensions

VeloRay 1.0 now includes multi-node Customers, Services and plans, shared encrypted
Config profiles with reusable snippets and revision restore, calendar quota renewal,
and token-free subscription history. Native sing-box forms support TUIC v5, Naive
TLS inbound and Snell v6; Xray JSON subscription export and live user additions
on unchanged Xray listeners are available.

See [customer management](docs/customer-management.md),
[protocol and live-apply limits](docs/native-protocols-and-live-apply.md), and the
[remaining feature backlog](docs/competitor-parity-roadmap.md). These additions do
not claim completion of every competing panel feature.

The panel also supports [PWA installation](docs/pwa.md) with static-only caching
and a reconnect page; authenticated data and subscription URLs are never cached.
