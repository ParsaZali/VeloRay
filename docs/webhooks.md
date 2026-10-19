# Signed Audit Webhooks (experimental)

VeloRay queues administrative and automatic client/device/node audit events in
PostgreSQL and delivers signed HTTPS POST requests. Each destination has its own
encrypted URL, signing key, enabled flag and event filters.

## Configure

As a superuser, visit **Settings → Webhooks**.

1. Enter a public HTTPS endpoint. Loopback, private networks and IP metadata
   endpoints are not allowed. Redirects are never followed.
2. Generate a random signing secret of at least 32 characters and save it.
   VeloRay encrypts it with `VELORAY_FIELD_KEY` and never returns the secret
   through the settings API.
3. Enable Webhooks and save. Select **Queue test event**.
4. Select **Queue status** to review pending, failed and delivered counts.
   Failed deliveries after eight attempts can be requeued with **Retry failed**.

Disable webhooks to stop new audit events from being enqueued and to pause
delivery of existing pending events. Purging queued records is a separate
administrator database operation.

## Payload

Each delivery contains a JSON object similar to:

```json
{
  "id": 123,
  "event": "clients.create",
  "target": "clients",
  "actor": "operator",
  "occurred_at": "2026-10-16T12:00:00Z"
}
```

`event` uses the existing audit action name. **The payload intentionally
does not include client passwords, subscription tokens, server settings,
credentials, or raw audit details.** Downstream consumers should query
authorized VeloRay API endpoints for any additional information.

Headers:

- `Content-Type: application/json`
- `X-VeloRay-Delivery: <delivery id>`
- `X-VeloRay-Timestamp: <unix epoch seconds>`
- `X-VeloRay-Signature: sha256=<lowercase hexadecimal HMAC>`

The HMAC-SHA256 key is the administrator-configured secret. The signed
message is `<timestamp> + "." + <exact raw JSON body bytes>`.

Always verify the HMAC in constant time, compare the timestamp to a short
acceptance window, and reject stale messages. Use `X-VeloRay-Delivery` as
an idempotency key: delivery is **at least once**, not exactly once.

## Delivery guarantees

Events are queued in PostgreSQL in the same transaction as the audit entry,
so rolled-back audited mutations do not produce notifications. A background
worker claims up to eight entries at a time, with DNS/public-IP checks,
a five-second HTTP timeout, and retries with exponential backoff. A delivery
counts as successful only for a 2xx HTTP response.

Delivery attempts stop after eight failures. Successful delivery records are
retained for seven days for dashboard counts. Automatic quota, expiry, renewal, first-use, device and node-state events are
included. This is an audit-event stream, not a complete network trace.

## API (superuser only)

- `GET /api/webhooks/status`: `pending`, `failed`, `delivered`
- `POST /api/webhooks/test`: queue an audit test event
- `POST /api/webhooks/retry`: schedule all exhausted deliveries for retry
- `PATCH /api/settings`: `webhook_enabled`, `webhook_url`,
  `webhook_signing_secret` (write-only) and
  `webhook_clear_secret` (write-only)

Check deployment logs for dispatch failures. Never configure the destination
to a private network service. Rotate signing secrets at both sender and
recipient together.

## Independent destinations and Discord

In Settings → Webhooks → Notification destinations, add up to 16 named generic
webhook or Discord endpoints. Each destination is disabled initially. Generic
webhooks require a 32–1024 byte signing secret; blank updates preserve the key.
URLs and keys are encrypted at rest. List responses omit both; authorized detail
returns the URL for editing but never the key. Select exact event names,
`client.*`/`node.*` prefixes, or `*`. Up to 64 filters are supported; `client.*`
does not match the separate CRUD name `clients.create`. Empty filters match all.

The legacy global destination can continue independently. Disabled destinations
stop new enqueueing and pause existing deliveries. Deleting a destination also
deletes its queued delivery records. **Queue test event** targets the selected
destination regardless of filters; it must be enabled. Retry is available per
destination. The global retry action requeues exhausted deliveries across the
queue. Queue-status counts aggregate every destination.

Discord receives a bounded `content` string containing audit metadata and an
empty `allowed_mentions.parse` list. It never receives raw audit detail. Generic
webhooks receive the JSON payload documented above. Both use the same bounded
outbox and isolated retry state; signatures cover the exact delivered bytes.
Transport failure messages are sanitized so webhook URL secrets cannot enter
logs or queue errors. Real public delivery and downstream idempotence must be
verified against your chosen service; automated delivery tests use an isolated
mock transport, including HMAC, retry identity and Discord mention suppression.

Additional administrator-only API:

- `GET/POST /api/webhooks/endpoints`: list metadata/create
- `GET/PATCH/DELETE /api/webhooks/endpoints/{id}`: detail/update/delete
- `POST /api/webhooks/endpoints/{id}/test`: targeted test (202)
- `POST /api/webhooks/endpoints/{id}/retry`: retry exhausted deliveries

Inputs: `name`, `enabled`, `channel` (`webhook` or `discord`), `url`, write-only
`signing_secret`, and `events` array. Email/SMTP is not implemented by this
integration.

## SMTP and operational event additions

Independent destinations also support encrypted email recipient lists through
TLS/STARTTLS SMTP. Global SMTP enable/pause is separate from webhook enable. New
configurable account warnings and monitor/billing events use the same outbox.
See [operations and alerts](operations-and-alerts.md) for setup and limits.
