# Operations, email and calendar inputs

These additions retain product version **1.0**. Update the panel and each remote
agent before using node diagnostics or outbound probes. Existing agents continue
serving traffic; missing new endpoints produce an unavailable result. PostgreSQL
schema 9 adds idempotent tables without changing subscription tokens or resetting
customer traffic. Take the normal backup before upgrading.

## Administrator Operations page

**Infrastructure → Operations** contains health monitors, node billing and
structured diagnostics. All `/api/operations/*` and `/api/monitors*` endpoints
require administrator role and administrator API scope. Resellers and viewers
cannot read hosting metadata, monitor target details or deployment diagnostics.

### Outbound health monitors

Create a monitor with a node, core, exact deployed outbound tag and public HTTPS
URL that returns 2xx. A GET is sent through an isolated instance of the selected
core using that outbound and its dependencies. This measures HTTP request latency
(including connection/TLS), not ICMP ping. The production core is neither changed
nor restarted, and successful direct connectivity cannot override a failed selected
outbound. Results do not measure the production listener, its routing rules or
existing customer sessions. Selector groups use their configured default, not a
runtime choice made in another core process.

Targets are resolved on the agent, restricted to public IPs, and pinned before
proxy CONNECT; original TLS hostname verification and HTTP Host are retained.
Redirects, URL credentials, private/local addresses and plaintext targets are
rejected. Kernel WireGuard/TUN outbounds are excluded from isolated probing.
The temporary loopback proxy requires a fresh random credential for each probe.
The helper copies outbound dependencies and DNS, not the production inbound,
control API, routing rules or persistent cache. Unavailable dependencies produce
failure. No automatic restart/failover is performed.

Limits: 100 monitors, interval 60–86400 seconds, failure threshold 1–10. Four
workers run best-effort schedules; each node/core allows one concurrent isolated
probe. Busy cores defer checks rather than count them as failures. Scheduling can
lag under load. A probe has an eight-second overall limit, including a two-second
startup allowance and a five-second HTTP timeout. Only bounded result metadata is
stored; target URLs are encrypted and omitted from lists. The administrator detail
endpoint returns the target for editing.

`monitor.down` emits when consecutive failures first reach the threshold;
`monitor.recovered` emits only when a previously down monitor succeeds. Repeated
unchanged states do not generate repeated events. Changing a target/node/core/tag
clears previous probe state. Up to 500 samples per monitor are retained, with the
newest 100 available through the UI/API and CSV export. Deleting a monitor deletes
its samples. CSV export escapes spreadsheet formula prefixes.

### Node billing

Configure provider, region, private account reference, three-letter currency,
monthly cost in minor units, traffic budget, warning percent, monthly reset day
and IANA timezone. Cost is metadata; VeloRay does not charge a payment method or
invoice customers. Provider metadata is encrypted separately from ordinary node
list fields.

Counters start when configured, using subsequently collected **raw managed VPN
traffic**, before customer multipliers. Independent outbound telemetry is not
added again. These are estimates for panel-managed traffic, not physical NIC
counters or an authoritative hosting-provider invoice. Probe/helper traffic and
unmanaged host traffic are not included. Disabling billing pauses accumulation;
it does not retroactively recover that period.

Reset dates clamp to the last day of short months while preserving the requested
anchor (day 31 becomes February's last day, then March 31). Cycle counters rotate
without changing customer usage. Previous periods and lifetime usage remain;
the UI returns the newest 24 periods and maintenance removes periods older than
two years. Offline nodes also rotate on maintenance. Changing settings preserves
current counters and the current cycle end; the new boundary is used on rollover.
A `node.budget_warning` event is emitted on a threshold crossing. A budget is
informational and does not stop traffic. Monthly cost is not prorated.

### Structured diagnostics

Read core status, ledger identifier and the newest 50 deployment operation
identifiers/states/timestamps/configuration hashes per core. Raw journal text,
configuration contents and credentials are not exposed by this endpoint. Existing
node status/online information remains available on its respective pages.

## Email and lifecycle alerts

In **Settings → Webhooks**, configure SMTP hostname/port, sender, optional username
and password. Use implicit TLS (usually port 465) or required STARTTLS (usually
587). Certificates and hostname are verified with TLS 1.2 or later; STARTTLS never
falls back to plaintext. The SMTP connection uses the same public-IP-pinned dialer
as webhooks, without environment proxies. Private SMTP relays are unsupported.

The SMTP password is encrypted and write-only. A blank update preserves it;
explicit clear removes it. `smtp_configured` is a read-only flag. Add an independent
**Email** destination with 1–10 comma-separated recipient addresses, enable it and
select event filters such as `monitor.*`, `node.budget_warning`, or `client.*`.
Save SMTP settings before queueing the destination's test event. SMTP recipients
are encrypted using the existing destination storage and omitted from lists.

Email uses the same transactional audit outbox, bounded retries and sanitized
metadata as webhook/Discord destinations. Successful SMTP DATA acceptance is the
delivery boundary; this is at-least-once delivery. Retries retain a stable
Message-ID; downstream deduplication is still needed. Global SMTP disable pauses
pending email without spending retries. Each destination's enabled flag separately
controls enqueue/delivery. No email is sent until explicitly configured.

Optional alert thresholds (zero disables each):

| Setting | Range | Event |
|---|---:|---|
| `alert_quota_percent` | 0–99 | `client.quota_warning` before exhaustion |
| `alert_expiry_days` | 0–365 | `client.expiry_warning` before expiry |
| `alert_inactive_days` | 0–365 | `client.inactive` for active accounts without recent traffic |

Maintenance scans up to 250 principals per pass using a rotating cursor, with a
30-second scheduling tick (other housekeeping work can delay a pass). Large
installations therefore have longer full-scan intervals. Shared accounts generate
one event across their connections/nodes; legacy standalone clients remain
supported. Transactional condition fingerprints suppress repeats while a condition
persists. Clearing/re-entering a condition or changing a quota cycle allows a new
event. These alerts do not change quotas, expiry or access. Native Telegram alerts
retain their existing independent configuration.

## Gregorian and Jalali date inputs

**Settings → System → Calendar** now controls the input month/day grid as well
as supported display dates. Jalali leap-year boundaries use the browser Intl
calendar; arrow keys, Page Up/Down and Escape work inside the date popup. Escape
closes the calendar before its parent editor.

Form values and API timestamps remain Gregorian. A field's existing UTC or
browser-wall-time semantics remain unchanged; selecting Jalali only changes the
calendar used to choose the civil date. Configured display timezone affects
formatted timestamps, not the interpretation of every date input. Provider billing
and renewal timezones are explicit independent settings.

## Verification and rollout limits

Automated coverage includes shared-alert deduplication and reset, encrypted
secrets, role boundaries, billing raw counters and month-end archive, monitor
failure/recovery/history, isolated core routing, TLS/STARTTLS SMTP protocol tests,
and browser CRUD/mobile/calendar tests. SMTP tests use a local TLS fixture; they
are not a claim that an operator's public mail provider has accepted production
mail. Verify its sender/authentication policy during your own setup. The same
applies to real public webhook consumer idempotency.

For rollback, restore matching application/agent binaries and configuration;
restore PostgreSQL from the pre-upgrade backup only with an explicit recovery
plan. The updater does not automatically roll back database contents. New tables
are additive and retained by an application-only rollback.
