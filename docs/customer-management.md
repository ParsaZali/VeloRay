# Shared customer accounts and network profiles

VeloRay 1.0 exposes **Customers**, **Services and plans**, and **Config profiles**
in the existing light/dark panel. The original Clients page remains the connection
view. Panel administrator accounts are still under Accounts.

## Customers and services

`GET/POST /api/customers` and `GET/PATCH/DELETE /api/customers/{id}` use
`clients.read/write/delete`. Resellers see only their own accounts. A customer
has `name`, `note`, `inbound_ids` (direct grants), `service_ids` (group grants),
optional `plan_id`, and the same quota/expiry/multiplier/HWID/IP/first-use fields
as Clients. Limits are 200 effective connections and 20 service groups per account.

An account owns its quota, traffic counters, expiry, device registry, renewal and
subscription token. Each inbound retains its own protocol credential. Adding or
removing a connection preserves the remaining credentials, counters and canonical
subscription URL. Usage from different nodes is serialized and billed once to
that account. Existing imported accounts and historical connection tokens remain
readable; editing an imported account supplies its existing membership.

`POST /api/customers/{id}/rotate-token` with `{}` replaces the canonical token
and **all** legacy connection tokens. This does not rotate VPN credentials.
The confirmation dialog explains that every old subscription URL stops working.

`GET/POST /api/services` and `GET/PATCH/DELETE /api/services/{id}` manage a named
`inbound_ids` group and optional `defaults` plan. Writes require administrator
scope. Restricted readers see only services whose entire inbound set is assigned
to them. A customer's effective membership is the union of direct and group grants.
Changing a group synchronizes all its customers and deploys affected nodes in one
compensated operation. The group cannot expand an owner's assigned inbound set.

Selecting `plan_id` applies its defaults only on explicit customer creation/edit;
changing a plan never silently resets existing quotas. Explicit policy values in
the request take precedence over defaults. Deleting a referenced service, shared
connection, or inbound is refused until its memberships have been removed. Use the
Customers editor to change a shared account's membership. Bulk Client actions act
once per account and deploy every member node; delete shared accounts in Customers.

All membership and policy changes retain reseller account-count and total quota
budgets. An unavailable node or invalid configuration rolls the database mutation
back and compensates nodes that already applied it. A concurrent membership change
outside the prepared node set returns 409 and can be retried.

## Calendar quota renewal

Existing `renewal_interval_days` remains supported. New customer/plan fields are:

| Field | Values |
| --- | --- |
| `renewal_period` | `none` (legacy day interval), `hour`, `day`, `week`, `month` |
| `renewal_every` | 1–3650; zero disables the new schedule |
| `renewal_timezone` | IANA name, e.g. `Asia/Tehran`, `America/New_York`, `UTC` |
| `next_renewal_at` | Optional RFC3339 instant; generated when omitted |

Hourly renewals use elapsed time. Daily/weekly/monthly renewals preserve local
wall-clock time across DST. Monthly schedules retain the original day, clipping
short months and restoring the day in a longer month (January 31 → February 28/29
→ March 31). Missed periods advance directly to the next future renewal; usage
resets once, without extending expiry. Quota-disabled accounts can reactivate,
while expired or manually disabled accounts remain governed by their policy.
Changing schedule fields reanchors the next renewal.

## Shared profiles, snippets and revision history

These endpoints require an administrator session or administrator-scope API key:

| Endpoint | Purpose |
| --- | --- |
| `/api/profiles` and `/api/profiles/{id}` | List/create/read/edit/delete profiles |
| `/api/snippets` and `/api/snippets/{id}` | Reusable named policy fragments |
| `POST /api/profiles/{id}/assign` | Replace assignments using `node_ids` |
| `GET /api/{profiles,snippets}/{id}/revisions` | Revision metadata (last 50) |
| `POST /api/{profiles,snippets}/{id}/restore` | Restore `revision` as a new version |

Profile body: `{ "xray": {}, "singbox": {}, "snippet_ids": [] }`.
Snippet body: `{ "core": "xray", "config": {} }` (or `singbox`).
Create/edit accepts `name`, `note`, `body`, and an optional expected `revision`;
a stale revision returns 409. Snippets merge in selected order, then profile
policy, then node-specific Xray `config_patch`, then panel-managed network entries
and protected telemetry/device rules. Array sections replace the preceding array.

Allowed policy sections are DNS, routing, outbounds, logging and Xray policy.
Profiles do not replace managed listeners, accounting or API services. Each patch
is bounded to 128 KiB; at most 32 snippets per profile. The panel validates the
result with node runtimes before deployment. Configuration containing credentials
is encrypted with the field key both in current storage and in revision history.
List endpoints return metadata only. Detail endpoints decrypt for authorized
editing. Keep the field key with backups; losing it makes these records unreadable.

Profile and referenced-snippet edits deploy all assigned nodes atomically using
the existing operation journal and compensation flow. An assigned profile or
referenced snippet cannot be deleted. The Node API may also set `profile_id`.

## Delivery history and automatic events

`GET /api/clients/{id}/subscription-history` returns the last 100 requests for its
shared account, with timestamp, requested format, normalized application family
and HTTP status. Owner scoping matches Clients. No token, URL, query string, IP,
HWID or raw user-agent is stored. Assets, HEAD and portal polling are excluded.
Retention uses `traffic_history_days` (default 30). History failures do not expose
or disable a subscription.

Signed webhook audit events now include `client.first_connection`,
`client.first_use` (deferred expiry activation), `client.quota`, `client.expired`,
`client.renewed`, `device.registered`, and node status transitions. They use the
same transactional outbox and credential-free payload as administrator events.
Multi-destination routing and email/Discord adapters remain backlog work.

## Database and rollout

The idempotent PostgreSQL migration makes the legacy account anchor nullable and
adds service/profile/snippet/revision/request-history tables. It does not rewrite
existing client tokens, credentials or counters. Native accounts have no anchor
node. Imported accounts retain their original anchor relationship.

Back up the database **and field key** before rollout. Verify the migration and a
multi-node edit on staging, then run the integration/race suite and browser suite.
The database additions are compatible with the preceding binary, but native
customer/group/profile management requires this version; an older binary will
not apply those policies. Never treat binary downgrade as a policy rollback:
restore a tested pre-upgrade database snapshot or forward-fix on staging first.
