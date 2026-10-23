# VeloRay expansion tracker

This tracker distinguishes working features from planned integrations. Existing green/light/dark styling and the restored sidebar are the UI baseline. A feature is complete only when its API, access control, deployment behavior, UI and meaningful verification agree.

| # | Capability | Status / dependencies |
|---|---|---|
| 1 | Storefront, verified payments, purchase/renew/traffic top-up | Available: storefront, verified Zarinpal, wallet purchase, renewal and traffic grants; merchant configuration required |
| 2 | Reseller wallets, credit, immutable ledger and settlement | Available: scoped wallets/credit, immutable ledger, manual settlements and seller-specific offers |
| 3 | Subreseller hierarchy and inherited limits | Available: bounded nested delegation, inherited inbounds, reserved capacity, descendant commerce access and atomic parent-to-child wallet transfers |
| 4 | Per-reseller storefront/subscription branding and domains | Available: individual store and subscription branding; domain activation requires DNS ownership, valid HTTPS and matching instance proof; configure proxy and certificates first |
| 5 | Telegram customer sales bot and Mini App | Available: signed Mini App checkout, Stars invoices/receipts, durable prioritized payment inbox, private order history and delivery; configure bot, terms and support first |
| 6 | Expanded migration with mapping and preview | Planned; extend existing import preview |
| 7 | Encrypted S3/SFTP backups, retention and restore verification | Planned |
| 8 | Node failover retaining the canonical subscription URL | Planned |
| 9 | Subscriber-side connection diagnostics | Planned; browser measurements have explicit limits |
| 10 | Per-user Mbps caps and scheduled fair use | Planned; support must be engine/protocol-specific |
| 11 | Shadowsocks 2022 multiuser | Available: AES-128/AES-256 multiuser on both cores; keys, metering and app formats tested |
| 12 | Supported live user add/change/removal | Planned; preserve full-deploy fallback |
| 13 | Managed WARP lifecycle and routing | Planned |
| 14 | Certificate fleet, DNS-01 wildcard and distribution | Planned |
| 15 | GeoIP/Geosite verified update and rollback | Available: opt-in official v2fly releases, bounded downloads, SHA-256 verification, complete Xray tag/config validation, atomic replacement, two-snapshot retention and crash/restart rollback; managed systemd agents |
| 16 | Single-use agent enrollment and verification | Available: expiring/revocable one-use commands, pinned certificate and initial agent identity/core checks |
| 17 | Canary/rolling core updates and rollback | Planned |
| 18 | Bounded subscription freeze preserving duration | Available: 30-day annual allowance, manual/automatic bounded resume, preserved token and usage |
| 19 | Adaptive subscription order from measured quality | Planned |
| 20 | Pre-save impact preview | Available for service membership changes, with a stale-preview revision guard |
| 21 | Passkey authentication | Available: discoverable WebAuthn with required device verification, account-scoped enrollment/revocation, MFA/session protections and root-only administrator recovery; stable trusted HTTPS panel domain required |
| 22 | Active/standby panel operation | Available: worker leadership and takeover, cluster diagnostics and deployment guide; external load balancing/shared infrastructure required |
| 23 | Managed SSH proxy users/keys/expiry/accounting | Planned; requires independent host/runtime verification |
| 24 | Revenue/cost/profit by seller, offer and node | Available: currency-separated seller/offer reports and explicit equal node revenue allocation, operating costs and profit |

External services stay disabled until an administrator configures their credentials. Credentials are encrypted at rest; payment confirmation always comes from the provider API. No simulated provider response is treated as a real payment.
