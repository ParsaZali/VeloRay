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
- [x] Naive TLS server management, corrected Basic-auth identities, URI/JSON export and HTTP/1.1 CONNECT/padding interoperability. Chromium/HTTP2 client testing remains separate.
- [x] Live addition of users on unchanged Xray VLESS/VMess/Trojan listeners, exact acknowledgment and rollback.
- [x] Xray client JSON subscriptions and a configurable JSON template.
- [x] Calendar-aware quota renewal and bounded, token-free subscription request history.
- [x] Shared subscription-token rotation, including revocation of every legacy alias.
- [x] Installable PWA with public-only static caching and explicit update acceptance.
- [x] Automatic quota/expiry/renewal/first-use/device/node events in the signed outbox.

This remains an incomplete backlog, not a claim of parity with every panel.
External policies/feed aggregation, optional OIDC, independent subscription
serving, Discord destinations, scoped SSE and browser date preferences are now
implemented within their documented limits. Other protocols, structural hot
apply, deployment targets and the unchecked items below remain unfinished. See `customer-management.md` and
`native-protocols-and-live-apply.md` for exact scope and verification limits.

## Additional delivered slice

See [external policies](subscription-policies.md), [OIDC/live UI](oidc-and-live-dashboard.md),
[independent subscriptions](independent-subscriptions.md) and [destinations](webhooks.md).
The owner has explicitly requested publication to main so the server updater can
receive these changes. Publication still requires verified runtime bundles and
green CI. An unchecked item must not be presented as completed.

## Delivered operations slice

See [operations, email and calendar inputs](operations-and-alerts.md) for API,
permissions, retention, scheduling and rollout limits.

- [x] Isolated outbound HTTP probes, request latency, manual checks and periodic failure/recovery alerts.
- [x] Administrator operations dashboard with monitor search/history/CSV and structured deployment diagnostics.
- [x] Encrypted hosting metadata, monthly raw-traffic budgets, reset timezone/day and archived periods.
- [x] TLS/STARTTLS SMTP destinations and deduplicated quota/expiry/inactivity events.
- [x] Jalali date input picker with leap-boundary and keyboard regression coverage.

## Delivered network policy slice

See [fallbacks and balancing](fallbacks-and-balancing.md) for limits and verification.

- [x] Guided Xray VLESS/Trojan RAW fallbacks, default/SNI/ALPN/path routing and PROXY protocol versions.
- [x] Xray outbound balancing, routing group selection, prefix preview and atomic revision-checked deployment.
- [x] Final merged routing references, inherited outbounds and dependent-group deletion protection.

## Stage 0 — safety and baseline
- [x] Establish reproducible green CI on main (Go tests/race, web build,
      browser smoke tests, dependency scan, install smoke tests).
- [x] Repeatable isolated PostgreSQL/core/browser fixtures with synthetic clients.
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
- [x] Support separate destination endpoints and per-event channel preferences.
- [x] Add outbound delivery end-to-end tests with an isolated mock transport.
- [ ] Verify real public HTTPS delivery and downstream idempotent consumption
      before marking this capability production-ready.

### Customer and package modeling
- [x] Native many-to-many clients ↔ inbounds / nodes with one durable account,
      shared traffic quota, expiry and mutually consistent subscription URLs.
- [x] Reusable customer service plans and user templates (policy defaults).
- [x] Internal service access groups.
- [x] External subscription policy groups with encrypted explicit-refresh caches.
- [x] Service-filtered selection and atomic bulk quota, expiry, enable/reset/delete and access edits (up to 200 explicit accounts).
- [x] Reusable config profiles shared across nodes with snippets/version history.
- [x] Preserve existing reseller authorization and accounting semantics (integration coverage).

## Stage 2 — delivery and connection quality
- [x] Flexible hosts, public addresses, SNI, path and visibility independent of
      the actual inbound listener.
- [x] Pluggable client-specific subscription templates: sing-box, Xray JSON,
      Clash/Mihomo, Stash and future formats.
- [x] Customer/service-assigned presentation rules with client-app hints (never authorization).
- [x] Custom HTML subscription page templates and safe variables.
- [x] External portable URI aggregation and connection links; raw/Base64 input.
- [x] Optional independent subscription-only systemd service and manual domain setup.
- [x] Subscription request history with bounded retention, no URL-token logging.
- [x] HWID enrollment, device revocation and cap enforcement for supporting apps.
- [x] Calendar-aware renewals: hourly, daily, weekly, monthly date and rolling.
- [x] Bulk customer editing and independent core/tag-scoped outbound accounting.
- [x] Robust handling of revoked shared-token aliases and device reassignment.

## Stage 3 — Xray, sing-box and network functionality
- [x] Hot addition of users on unchanged Xray VLESS/VMess/Trojan listeners.
- [ ] Hot removal/update, sing-box changes and structural inbound hot apply.
- [x] Native TUIC v5 and Snell v6 management with real-core round trips.
- [x] Native Naive TLS management and exports.
- [x] Verify a compatible Naive HTTP/1.1 client round trip with native user attribution; Chromium/HTTP2 client coverage remains separate.
- [ ] Native, tested AmneziaWG, MTProto, ShadowTLS and optional SSH proxy management.
- [x] Validate feasibility per pinned engine and distinguish upstream engine capability
      from supported forms; see `native-protocols-and-live-apply.md`. This does not mark the remaining protocol implementations complete.
- [x] Guided outbound chaining and sing-box selector/URL-test controls, with cycle/dependency validation.
- [x] Xray fallbacks and outbound balancing within the documented engine/form limits.
- [x] Read-only per-node GeoIP/Geosite tag browser, asset fingerprints and guided Xray rule creation.
- [ ] Transparent proxy, transparent capture, WARP / provider integrations, external-provider
      subscriptions, link-aware DNS and domain fronting controls.
- [x] Outbound HTTP health probes, latency and periodic watchdog alerts (no automatic production restarts).
- [ ] Maintain safe privileged-agent boundaries for every protocol addition.

## Stage 4 — deployment, monitoring and external integrations
- [x] Full container stack, non-root images, process-owned cores, independent subscriptions and persistent accounting; see `container-deployment.md` for CI/staging boundaries.
- [x] Portable container installation and host CI on Ubuntu 22.04/24.04; amd64/arm64 runtime bundles.
- [ ] Native distro installer beyond Ubuntu 24.04 and native ARM host smoke coverage.
- [x] Windows feasibility investigation with actual cross-build failures and required OS/service boundaries; native Windows is not shipped.
- [x] SQLite/MySQL/MariaDB architectural assessment; PostgreSQL remains the runtime backend. See `platform-and-database-assessment.md`.
- [ ] Implement and validate alternate runtime database backends as a separate migration project.
- [x] Cloud-init first boot with main-CI/checksum gates, automatic frontend HTTPS and explicit Cloudflare A/AAAA upsert with secret-file tokens.
- [ ] Live public ACME/DNS-provider verification on the operator's domain; wildcard DNS-01 automation.
- [x] Prometheus-compatible metrics and meaningful exporter access controls.
- [x] Per-node billing budgets, reset dates and encrypted hosting provider metadata.
- [ ] Online status, web-accessible diagnostic logs, deeper search,
      operational health dashboards and export formats.
- [x] Optional Discord and signed webhook destinations with audit-event routing.
- [x] SMTP/email notifications, inactive-client and configurable quota-threshold alerts.
- [x] Optional OIDC code/PKCE login with exact pre-bound subjects and local 2FA.
- [x] Direct GitHub OAuth with PKCE, exact account-ID binding and local second factor.
- [x] Telegram sign-in through the existing standard OIDC code flow and provider-aware login button (operator registration required; see `external-login-and-proxies.md`).
- [x] Direct LDAPS simple-bind adapter with explicit local account DNs, verified TLS and preserved permissions/2FA; actual directory staging remains operator-owned (see `ldap-login.md`).

## Stage 5 — internationalization and UX
- [x] Persian Jalali/timezone display preferences for expiry and collection dates.
- [x] Jalali date input picker, including leap-month boundaries and keyboard control.
- [ ] Timezone presentation throughout every page (expanded, not yet universal).
- [ ] Expand language catalogs with translator-reviewed strings.
- [x] Service worker and safe PWA installation with bounded static caches, real server-loss and update-acceptance browser tests.
- [x] Live owner-scoped dashboard with bounded, reauthorized SSE and polling fallback.
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
