[English](README.md) · [فارسی](README.fa.md)

<p align="center"><img src="docs/media/veloray-banner.svg" alt="VeloRay — Your network. One control panel." width="100%"></p>

<p align="center">
<a href="https://github.com/ParsaZali/VeloRay/actions/workflows/ci.yml"><img src="https://github.com/ParsaZali/VeloRay/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
<a href="https://github.com/ParsaZali/VeloRay"><img src="https://img.shields.io/badge/version-1.0-4e7fc0" alt="Version 1.0"></a>
<a href="LICENSE"><img src="https://img.shields.io/github/license/ParsaZali/VeloRay" alt="License"></a>
<a href="https://github.com/ParsaZali/VeloRay/stargazers"><img src="https://img.shields.io/github/stars/ParsaZali/VeloRay?style=flat" alt="GitHub stars"></a>
<img src="https://img.shields.io/badge/React-19-61dafb?logo=react&amp;logoColor=white" alt="React 19">
<img src="https://img.shields.io/badge/Go-1.26-00add8?logo=go&amp;logoColor=white" alt="Go 1.26">
</p>

**VeloRay is an open source control panel for Xray, sing-box and native MTProto/AmneziaWG services.** Manage users, nodes, subscriptions, routing and traffic policies from a compact interface with English/Persian support and light/dark themes.

[Quick start](#quick-start) · [Screenshots](#screenshots) · [Features](#features) · [Protocols](#protocols) · [Documentation](#documentation)

## Quick start

Run as **root** on **Ubuntu 24.04**, on `amd64` or `arm64`:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/ParsaZali/VeloRay/main/install.sh)
```

The installer takes you through the essentials:

1. Enter a domain pointing to the server, or use its public IP.
2. Choose the panel and subscription ports. **Press Enter for random available ports.**
3. For a domain, let the installer request a trusted TLS certificate. The email is optional.
4. Open the printed address and sign in with the **generated username and password**.

PostgreSQL, both proxy cores, the agent, independent subscriptions and systemd services are configured automatically. The initial credentials are also saved in the root-only `/etc/veloray/install-result.env`. IP-only installs use a local TLS certificate. For domain certificate issuance, DNS must point to the server and HTTP port 80 must be reachable; a failed certificate request leaves local HTTPS available for recovery.

For Persian prompts:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/ParsaZali/VeloRay/main/install.sh) --lang fa
```

No Go or Node.js installation is needed when using the verified runtime bundles. Available public ports are accepted without a Cloudflare CDN port filter.

### Update

```bash
sudo veloray update
```

Updates follow `main` after **that exact commit passes CI**. The updater verifies the pinned installer, creates a private backup, runs health checks and attempts application/service rollback if installation fails. Existing users, encryption keys and traffic records are preserved. Database restore remains an explicit operation.

### Removal and reinstall

`sudo veloray uninstall` stops and removes the services and program files while **keeping users, VPN configurations, keys and backups**. Reinstalling reuses this data. `sudo veloray reset` detects the current administrator even after a username change; use `sudo veloray admin-list` and `sudo veloray reset USERNAME` when multiple administrators exist.

For a fresh installation, `sudo veloray uninstall --purge` requires typing **PURGE** and a successful private recovery backup before deleting the installer-managed local database, configuration and keys. Backups are kept. Custom databases require manual removal.

### Server console

```bash
sudo veloray
```

A compact management menu provides service status, logs, domain/port changes, SSL, password reset, backups, repair and updates. **Leave the new password empty to generate a random one.** Successful checks are green, warnings yellow and failures red when the terminal supports color. `NO_COLOR=1` disables colors.

<details>
<summary>Useful commands</summary>

```bash
sudo veloray status
sudo veloray doctor
sudo veloray admin-list
sudo veloray reset
sudo veloray domain panel.example.com
sudo veloray port 23456
sudo veloray ssl admin@example.com
sudo veloray backup
sudo veloray auto-update enable
```

To install a remote agent, use the same installer with `node`. Upgrade remote agents before assigning new core or native protocol features to them.

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/ParsaZali/VeloRay/main/install.sh) node
```

</details>

## Screenshots

Screenshots use demonstration accounts, not production credentials.

<picture><source media="(prefers-color-scheme: dark)" srcset="docs/media/settings-dark.webp"><img src="docs/media/settings-light.webp" alt="Compact navigation and categorized VeloRay settings" width="100%"></picture>

<details>
<summary>Login and the default subscription page</summary>

<img src="docs/media/login.webp" alt="VeloRay login with light/dark appearance and language selection" width="100%">
<img src="docs/media/subscription.webp" alt="Default subscription page with quota, expiry, app formats, QR and traffic history" width="800">

</details>

## Features

| Area | What you can manage |
|---|---|
| **Users & customers** | Shared accounts across nodes, quotas, expiry, first-use activation, scheduled renewal, IP/HWID limits, bulk actions and token rotation |
| **Sales & billing** | React storefront, verified Zarinpal checkout, reserved inventory, wallet purchase/renew/top-up, immutable ledger, reseller hierarchy, credit transfers and currency-separated profit reports |
| **Seller branding** | Individual store/subscription name, logo, support link and DNS/HTTPS-verified subscription domains |
| **Telegram sales** | Customer bot and Mini App, verified Stars payments, private order history and automatic delivery with the same fulfillment workflow |
| **Passkeys** | Device-verified WebAuthn sign-in, key enrollment/revocation, required-MFA protections and host-console recovery |
| **Multi-node** | Local and remote agents, scoped deployments, listener inspection, configuration validation, journals and compensated rollback |
| **Availability** | Single-use verified node enrollment, certificate pinning, automatic background-worker election for active/standby panels, bounded customer freeze and service impact review |
| **Subscriptions** | A simple default page, custom HTML files, configurable URL paths, a separate HTTPS port, QR codes and app-specific exports |
| **Core versions** | Opt-in manual downloads from official GitHub releases, verified SHA-256, version selection, configuration checks and previous-version rollback |
| **Traffic** | Durable counters, actual upload/download history, quota multipliers, charts, usage CSV and outbound accounting |
| **Network** | Guided inbounds/outbounds, routing rules, chaining, Xray fallbacks/balancing and sing-box selectors/URL-test groups |
| **Connection tests** | Generated client credentials, delivery-host selection and node-side protocol/HTTPS probes |
| **Accounts & access** | Superuser, reseller, viewer and custom roles, inbound assignment, scoped API keys and account budgets |
| **Authentication** | Local passwords and 2FA, recovery codes, optional OIDC, GitHub OAuth, Telegram OIDC and LDAPS |
| **Operations** | Health monitors, deployment diagnostics, hosting budgets, monthly traffic cycles, Prometheus and audit history |
| **Automation** | Telegram controls, signed webhooks, Discord/SMTP notifications, event preferences and scheduled backups |
| **Interface** | Responsive React pages, lazy loading, English/Persian, Gregorian/Jalali input, keyboard controls, local fonts and installable PWA |

### Protocols

| Engine | Managed protocols |
|---|---|
| **Xray** | VLESS, VMess, Trojan, Shadowsocks, Hysteria2, WireGuard, HTTP, SOCKS, tunnel and TUN |
| **sing-box** | VLESS, VMess, Trojan, Shadowsocks, Hysteria2, TUIC v5, Snell v6, Naive TLS, ShadowTLS v3, HTTP and SOCKS |
| **VeloRay Native** | MTProto/FakeTLS and AmneziaWG with per-user traffic, credentials, expiry and revocation |

Shadowsocks 2022 AES multiuser is available on both Xray and sing-box, with separate user keys/counters and compatible app exports. See the [protocol guide](docs/shadowsocks-2022.md).

Transports include **TCP/raw, WebSocket, gRPC, XHTTP, HTTPUpgrade, mKCP and QUIC** where supported by the selected engine. TLS and REALITY are available for compatible combinations. The editor validates supported combinations instead of advertising unavailable ones.

Native services support user changes without restarting unchanged sessions. Xray supports hot user addition on unchanged VLESS/VMess/Trojan listeners; other structural changes use validated restart. TUN/WireGuard may require host networking privileges. Native AmneziaWG uses a userspace stack for public TCP/UDP destinations. See the protocol guides for exact limits.

### Subscriptions that fit your setup

A browser opens the simple account page. VPN clients receive raw/Base64 links, Clash/Mihomo YAML, sing-box JSON or Xray JSON. Mixed accounts retain compatible connections and report omitted entries.

```text
https://vpn.example.com:23457/sub/USER_TOKEN
https://vpn.example.com:23457/access/USER_TOKEN
https://vpn.example.com:23457/USER_TOKEN
```

Set **Settings → Subscription → Subscription path** to `/sub/`, a custom path, or empty for a root-level token. Changing it changes existing subscription URLs; the user token stays the same. Set the independent HTTPS port in the same section. Customer links move to a new port after the listener has been successfully applied.

Choose **Custom HTML file** to use your own page from `/etc/veloray/templates`. Template validation, local assets and fallback to the default page are built in. [Customization guide →](docs/subscription-customization.md)

### Official core releases

Open **Cores → Core versions**, enable official downloads, check releases and choose a version. Downloads come from **[XTLS/Xray-core](https://github.com/XTLS/Xray-core)** and **[SagerNet/sing-box](https://github.com/SagerNet/sing-box)**. Only stable assets with official GitHub SHA-256 metadata are offered. The current configuration is checked before replacement, counters are harvested, and an unsuccessful restart restores the previous executable.

Downloads are manual; enabling them does not schedule automatic core changes. Panel updates preserve an explicitly selected official core. Per-user IP attribution in sing-box requires VeloRay's bundled build, so switching to an official binary is blocked while per-user IP caps are configured. Container deployments update their cores through their image. [Core management guide →](docs/core-versions.md)

## Deployment options

- **Native:** Ubuntu 24.04 + systemd, `amd64`/`arm64`, PostgreSQL.
- **Docker Compose:** panel, agent, both cores, PostgreSQL, HTTPS proxy and independent subscriptions. Host CI covers Ubuntu 22.04/24.04.
- **Cloud-init:** first-boot installation with commit/checksum verification and optional Cloudflare DNS configuration.

[Container & cloud deployment →](docs/container-deployment.md) · [Platform/database scope →](docs/platform-and-database-assessment.md)

## Documentation

- [Sales, payments and wallets](docs/commerce.md)
- [Telegram sales and Mini App](docs/telegram-sales.md)
- [Passkey authentication and recovery](docs/passkeys.md)
- [Customer freeze and service impact review](docs/customer-freeze-and-impact.md)
- [Single-use node enrollment](docs/node-enrollment.md)
- [Active/standby panel deployment](docs/panel-availability.md)
- [Expansion tracker and remaining integrations](docs/expansion-roadmap.md)

| Guide | Contents |
|---|---|
| [Customers and plans](docs/customer-management.md) | Shared accounts, policies, renewals and reseller budgets |
| [Subscriptions](docs/subscription-customization.md) | Templates, delivery hosts, device admission and feeds |
| [Independent subscriptions](docs/independent-subscriptions.md) | HTTPS ports and listener rollover |
| [MTProto & AmneziaWG](docs/mtproto-and-amneziawg.md) | Native setup, exports, accounting and limitations |
| [Core versions](docs/core-versions.md) | Official downloads, rollback and compatibility |
| [Connection tests](docs/client-connection-tests.md) | Protocol checks and diagnostic limits |
| [Routing & balancing](docs/fallbacks-and-balancing.md) | Fallbacks, selectors and safe deployment |
| [Notifications](docs/webhooks.md) | Signed webhooks and event destinations |
| [Operations](docs/operations-and-alerts.md) | Health checks, email and hosting budgets |
| [Feature roadmap](docs/competitor-parity-roadmap.md) | Delivered features and remaining work |

The authenticated **API** page contains the REST/OpenAPI reference. Server backups and operational logs stay private; subscription tokens are redacted from application request logs.

## Development and verification

```bash
go test -race ./...
go vet ./...
cd frontend && npm ci && npm run format:check && npm run build
```

CI also runs PostgreSQL integration tests, pinned-core protocol checks, Playwright UI/PWA tests, dependency checks, installer fixtures and actual Docker HTTPS/traffic/restart checks. Release archives for both architectures must match the source manifest.

Contributions are welcome. Include a clear behavior change and relevant tests; keep documentation aligned with the capabilities actually shipped. VeloRay is MIT licensed. Bundled engines and dependencies retain their own licenses and notices.

If VeloRay is useful to you, **[give the project a star](https://github.com/ParsaZali/VeloRay)** or report a reproducible issue with sensitive information removed.
