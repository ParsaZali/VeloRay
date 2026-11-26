# Subscription Portal v2 — verified delivery inventory

VeloRay’s modern portal uses its own templates, accounting and subscription-token authorization. The compact layout remains available.

## Implemented behaviors and improvements (individually inspectable)

1. Default modern subscription dashboard layout.
2. Keep the old compact portal selectable without breaking existing links.
3. Responsive desktop, tablet and phone layouts.
4. Test horizontal overflow at 390 and 320 CSS pixels.
5. Light visual theme.
6. Dark visual theme.
7. Follow OS/browser theme when administrator chooses `system`.
8. Persist visitor's explicit theme choice locally.
9. Reuse the configured VeloRay accent palette.
10. Persian translation and RTL page direction.
11. English LTR translation.
12. Russian translation.
13. Simplified Chinese translation.
14. Allow visitors to change language from the portal.
15. Localized Persian/Jalali expiry and last-use date presentation.
16. Display subscription status and expiry countdown.
17. Show billed traffic used and total allowance.
18. Show calculated remaining traffic without negative balances.
19. Render visual quota consumption/progress.
20. Display raw usage independently from the billing multiplier.
21. Display the number of active connection configurations.
22. Display registered-device count and user device cap.
23. Display recorded last-traffic activity where available.
24. Store traffic series from existing accounting deltas by billing principal.
25. Aggregate shared-account traffic for subscriber history.
26. Retain subscription history only for the configured number of days.
27. Never store subscription secrets/tokens in traffic-series rows.
28. Display real historical charts in six validated time ranges.
29. Switch chart between total, upload and download without refetching.
30. Download traffic history as numeric CSV with bounded, non-injectable output.
31. Show universal subscription URL with a clipboard action.
32. Show subscription QR code in a keyboard-accessible dialog.
33. List connection profiles associated with the subscription account.
34. Search the connection list by name/protocol/server.
35. Filter connection profiles by protocol.
36. Copy any individual portable connection URI.
37. Copy all portable connection URIs at once.
38. Download an individual portable connection URI as a local text file.
39. Show per-connection QR codes without an external QR service.
40. Link to supported Base64, RAW, Clash, sing-box and WireGuard exports.
41. Suggest open-source clients with visitor OS prioritization.
42. Display announcements and the configured support link.
43. Configure subscription refresh interval (1–168 hours) in Settings.
44. Configure fallback client response format in Settings.
45. Configure chart visibility and CSV availability with admin settings.
46. Configure subscription-download button visibility separately.
47. Configure portal theme choice in Settings.
48. Keep the existing subscription token, format endpoints and compact template.
49. Use existing strict same-origin script/style CSP rather than a third-party CDN.
50. Test escaping of customer names, QR visibility, privacy settings and HEAD behavior.

Relevant code: `internal/panel/portal_modern.go`,
`internal/panel/portal_modern.html`, `internal/panel/subscription_stats.go`,
`internal/panel/subscription.go`, `internal/panel/node.go`,
`internal/panel/worker.go`, `internal/panel/migrations/001_initial.sql`,
`frontend/public/subscription-modern.{css,js}` and
`frontend/src/pages/settings.tsx`.

## Product parity / remaining work

**Complete or substantially improved for subscription UX:** HTML template,
hosted portal, QR/copy usability, multi-profile presentation, localized UI,
Jalali dates, historical subscriber accounting/charting and configurable
response defaults. Some of these capabilities existed in basic form before
this branch, so they are improvements rather than new parity categories.

**Not yet delivered by this PR:** independent thirty other categories from the
96-item backlog, native TUIC/AmneziaWG/ShadowTLS/Snell support, external
subscription aggregation, configurable internal/external access squads,
truly user-editable configuration templates, distinct subscription
domain deployment, HA cluster, cloud provisioning, OAuth/OIDC/LDAP,
Discord alerting, Prometheus export, production Windows runtime and hot-apply
of traffic-core config. These require their own tested implementation slices.

## Required verification and rollout

- CI should pass Go race tests, vet, dependency audit, npm format/typecheck,
  the complete browser regression suite and PostgreSQL integration tests.
- Browser tests include a deterministic mocked subscriber stats response;
  SQL aggregation is covered separately by a PostgreSQL-backed test.
- Verify existing account subscriptions, devices, real node ledgers and
  upgrade/restore procedures on a staging installation.
- Admin/maintainer review is required before merging. **No GitHub PR or
  successful CI action deploys anything to the user's production server.**
