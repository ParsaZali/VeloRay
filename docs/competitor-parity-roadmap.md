# VeloRay competitor-parity engineering roadmap

This is a **living gap backlog**, not a claim that the listed capabilities are
absent from every deployed version. Always check the latest VeloRay source,
existing implementations, upstream feature revisions and real-world behavior
before starting any work.

The goal is to close verified gaps versus 3x-ui, Marzban, Hiddify Manager,
X-UI, S-UI and Remnawave **without copying GPL-licensed source code into
VeloRay's MIT repository**. Independently implement features and consult
project maintainers on licensing whenever reusing third-party code.


## Delivered subscription slice (current source)

- [x] Simple default subscription page with persistent light/dark/system themes.
- [x] Operator-owned HTML file selection, validation, local assets and buffered fallback.
- [x] Global Clash/Mihomo and sing-box JSON templates (client-version validation remains operator-owned).
- [x] Multiple subscription delivery hosts per inbound; exported address/port/SNI/path overrides.
- [x] Transactional HWID admission cap, per-client override, revoke/allow/forget controls.
- [x] Read-only subscription compatibility/eligibility report (not a connectivity probe).
- [x] Authenticated aggregate Prometheus metrics for administrator-owned read keys.

## Delivered management and protocol slice (current source)

- [x] Native multi-node customer accounts with shared quota, devices, expiry and stable subscription tokens.
- [x] Service access groups and reusable plan defaults, with reseller budgets preserved.
- [x] Shared encrypted node profiles, reusable snippets, 50 revisions and compensated deployment.
- [x] TUIC v5 and Snell v6 server management, exports and real-engine CI round trips.
- [x] Naive TLS server management and URI/JSON export; compatible Naive client round trip remains unverified.
- [x] Live addition of users on unchanged Xray VLESS/VMess/Trojan listeners, exact acknowledgment and rollback.
- [x] Xray client JSON subscriptions and a configurable JSON template.
- [x] Calendar-aware quota renewal and bounded, token-free subscription request history.
- [x] Shared subscription-token rotation, including revocation of every legacy alias.
- [x] Installable PWA with public-only static caching and explicit update acceptance.
- [x] Automatic quota/expiry/renewal/first-use/device/node events in the signed outbox.

This remains an incomplete backlog, not a claim of parity with every panel.
External policy groups/feed aggregation, other new protocols, structural hot apply,
additional deployment targets, identity federation and the unchecked items below
remain unfinished. See `customer-management.md` and
`native-protocols-and-live-apply.md` for exact scope and verification limits.

## Stage 0 — safety and baseline
- [ ] Establish reproducible green CI on main (Go tests/race, web build,
      browser smoke tests, dependency scan, install smoke tests).
- [ ] Lock down a repeatable isolated test server with synthetic clients.
- [ ] Capture feature-by-feature baselines with screenshots, API schema and
      verified behavior, not README-only claims.
- [ ] Assess schema migrations, backwards compatibility, performance, risk,
      rollback procedure and deployment safety for each new capability.
- [ ] No changes are merged to main until maintainer review and green CI.

## Stage 1 — foundational management and automation

### Signed event webhooks — first implementation in this branch
- [x] Superuser-managed HTTPS endpoint, encrypted signing secret, disabled by default.
- [x] Audit-event outbox persisted transactionally in PostgreSQL.
- [x] HMAC-SHA256 with delivery identifier and timestamp headers.
- [x] Bounded retries, queue counts, manual retry and test event.
- [x] Avoid sending credentials, configuration, tokens or raw audit details.
- [x] Restrict private/reserved IP targets and prevent redirect/proxy bypass.
- [x] UI controls in Settings → Webhooks.
- [x] Automated tests for validation, HMAC and transaction consistency.
- [x] Broaden events to automatic client expiration, quota exhaustion,
      first connection, node transitions and device changes.
- [ ] Support separate destination endpoints and per-event channel preferences.
- [ ] Add outbound delivery end-to-end tests with an isolated mock transport.
- [ ] Verify real public HTTPS delivery and downstream idempotent consumption
      before marking this capability production-ready.

### Customer and package modeling
- [x] Native many-to-many clients ↔ inbounds / nodes with one durable account,
      shared traffic quota, expiry and mutually consistent subscription URLs.
- [x] Reusable customer service plans and user templates (policy defaults).
- [x] Internal service access groups.
- [ ] External subscription policy groups.
- [ ] Group-scoped operations for quotas, expiries, filtering and access.
- [x] Reusable config profiles shared across nodes with snippets/version history.
- [x] Preserve existing reseller authorization and accounting semantics (integration coverage).

## Stage 2 — delivery and connection quality
- [x] Flexible hosts, public addresses, SNI, path and visibility independent of
      the actual inbound listener.
- [ ] Pluggable client-specific subscription templates: sing-box, Xray JSON,
      Clash/Mihomo, Stash and future formats.
- [ ] Contextual response rules based on authorized user/group/client app.
- [x] Custom HTML subscription page templates and safe variables.
- [ ] External subscription aggregation and external connection links.
- [ ] Independent subscription domain/service deployment.
- [x] Subscription request history with bounded retention, no URL-token logging.
- [x] HWID enrollment, device revocation and cap enforcement for supporting apps.
- [x] Calendar-aware renewals: hourly, daily, weekly, monthly date and rolling.
- [ ] Bulk editing and independent outbound accounting.
- [x] Robust handling of revoked shared-token aliases and device reassignment.

## Stage 3 — Xray, sing-box and network functionality
- [x] Hot addition of users on unchanged Xray VLESS/VMess/Trojan listeners.
- [ ] Hot removal/update, sing-box changes and structural inbound hot apply.
- [ ] Native, tested protocol management for TUIC v5, AmneziaWG, MTProto,
      ShadowTLS, Naive, Snell and optional SSH proxy.
- [ ] Validate feasibility per engine and distinguish upstream engine capability
      from the panel UI's explicitly supported configuration forms.
- [ ] Fallbacks, outbound balancing, geo asset browsing, transparent proxy,
      transparent capture, WARP / provider integrations, external-provider
      subscriptions, link-aware DNS and domain fronting controls.
- [ ] Outbound health probes, latency and periodic tunnel watchdog.
- [ ] Maintain safe privileged-agent boundaries for every protocol addition.

## Stage 4 — deployment, monitoring and external integrations
- [ ] Full containerized production deployment with minimal-permission images.
- [ ] Portable installation on supported Linux distributions and architectures.
- [ ] Investigate Windows support while accounting for systemd and privileged
      network agent assumptions.
- [ ] Evaluate SQLite/MySQL/MariaDB portability as a separate architectural
      undertaking; PostgreSQL remains the authoritative backend until complete.
- [ ] Cloud-init unattended install, TLS/Cloudflare automation and supported
      self-updates with verified checksums.
- [x] Prometheus-compatible metrics and meaningful exporter access controls.
- [ ] Per-node billing budgets, reset dates and hosting provider metadata.
- [ ] Online status, web-accessible diagnostic logs, deeper search,
      operational health dashboards and export formats.
- [ ] Optional email and Discord notifications, event routing, and alerts for
      inactive clients, logins, device changes and quota thresholds.
- [ ] Optional identity federation (GitHub OAuth, Telegram OAuth, OIDC and LDAP),
      subject to a threat model for administrator identities.

## Stage 5 — internationalization and UX
- [ ] Robust Persian Jalali calendar and timezone-aware date presentation.
- [ ] Expand language catalogs with translator-reviewed strings.
- [x] Service worker and safe PWA installation with bounded static caches, real server-loss and update-acceptance browser tests.
- [ ] Live dashboards with bounded WebSocket/SSE updates where appropriate.
- [ ] Config builders, realtime network diagnostics and deep table filtering.
- [ ] Accessibility, keyboard navigation, screen-reader and mobile regression
      suite.

## Acceptance criteria for *every* implementation
1. Public API schema and authorization documented; no cross-reseller leakage.
2. Backend tests, migration tests, frontend typecheck/build and UI smoke tests.
3. Existing data and tokens migrate cleanly; rollback and partial deployment
   behavior are validated in a staging database.
4. Secrets are stored encrypted and never appear in list/search exports or logs.
5. No silent quota resets, double billing, traffic accounting regressions,
   plaintext admin credentials, or new network-SSRF surfaces.
6. Tested with both Xray and sing-box where relevant, with named versions.
7. Owners sign off explicitly before production deployment or merge to main.

## Upstream sources for independent specification review
- https://github.com/MHSanaei/3x-ui
- https://github.com/Gozargah/Marzban
- https://github.com/hiddify/Hiddify-Manager
- https://github.com/alireza0/x-ui
- https://github.com/alireza0/s-ui
- https://github.com/remnawave/panel
