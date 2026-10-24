# API integrations

Create an API key under the panel's API-key management page. Treat it as a secret. Send `Authorization: Bearer vr_...` over HTTPS. API-key scope (`read`, `write`, `admin`), linked account permissions and seller ownership are all enforced; an integration must not bypass those boundaries. Read-only keys cannot approve payments or change clients. `/api/openapi` documents available routes.

## Clients, accounts and telemetry

| Endpoint | Returns |
| --- | --- |
| `GET /api/clients` | Authorized client connections, accounting and status |
| `GET /api/clients/{id}/usage` | Billed bytes, raw upload/download, quota, remaining bytes, multiplier, telemetry age/status and expiry |
| `GET /api/clients/{id}/ips` | Observed IP list, count and `available` flag |
| `GET /api/clients/{id}/devices` | Registered subscription/HWID device records and count |
| `POST /api/clients/{id}/device-action` | Existing allow/revoke/forget operation |
| `GET /api/clients/{id}/subscription-history` | Recorded subscription downloads |
| `GET /api/traffic-history` | Retained traffic windows in the caller's scope |
| `GET /api/customers/{id}` | Canonical shared account and member connections |

Usage values are integer bytes. Each counter also has an `_exact` decimal-string representation for clients whose JSON number implementation cannot preserve integers above 2^53. `remaining_bytes: null` and `unlimited: true` mean no quota. Zero means an exhausted finite quota. `telemetry_fresh: false`, null online state, or `ips.available: false` indicate incomplete/stale observation, not proof of zero connections. Byte accounting comes from the core's last collected counters, not a claim of an instantaneous packet capture. A shared account's billed quota is shared across its member client rows; do not sum that quota or billed total repeatedly. Registered device records do not identify every physical device that has used a copied configuration.

## Sales administration

Use `?owner=SELLER_ID` to select an authorized seller; omit it for self. A parent may select descendants and a superuser may select sellers. Siblings/children cannot access a parent or another seller.

- `GET/PATCH /api/commerce/store`: branding, separate `enabled` and `telegram_sales` switches, and bounded `bot` customization.
- `GET/POST /api/commerce/offers`, `PATCH /api/commerce/offers/{id}`: prices/grants, category, HTTPS image and display order. USDT uses integer millionths.
- `GET /api/commerce/orders`, `POST /api/commerce/orders/{id}/fulfill`: order status and idempotent retry of already paid delivery.
- `GET /api/commerce/receipts`, `GET /api/commerce/receipts/{id}`: receipt metadata or authorized decrypted detail.
- `POST /api/commerce/receipts/{id}/approve`: `{ "expected_amount": 10000, "expected_currency": "IRT", "reference": "BANK_REFERENCE", "note": "checked" }`. The bank reference is required for card transfers; USDT uses its persisted network/hash. Submission does not verify an on-chain transfer.
- `POST /api/commerce/receipts/{id}/reject`: `{ "note": "Reason" }`.
- `GET /api/commerce/tickets`, `GET /api/commerce/tickets/{id}`, `POST /api/commerce/tickets/{id}` with `{ "text": "Reply", "close": false }`.
- Existing wallet, immutable ledger, credit-limit, cost, reseller and reporting APIs retain their permission checks.

## Customer Mini App

`GET /api/shop/me?store=ID` requires `X-Telegram-Init-Data` with the unmodified, Telegram-signed raw initData. An administrator API key is not a buyer identity. Use the same header for `/shop/me/orders`, `/shop/me/trial`, `/shop/me/services/{id}/{enable|disable|rotate}`, and `/shop/me/tickets`. Every service action requires `{ "confirm": true }`; trial requires `{ "accept_terms": true }`. Buyers must have started that seller's bot.

A public checkout returns a private order token. Use `Authorization: Bearer ORDER_TOKEN` only for `/api/shop/orders/{id}` and its `/pay` or `/receipt` subroutes. Receipt input is `{ "method": "card", "image": "data:image/png;base64,...", "note": "optional" }` or `{ "method": "usdt", "transaction_id": "64_HEX_HASH", "note": "optional" }`. Images are bounded to 1 MiB. Order IDs alone do not grant access. Neither payment evidence nor a client-side success event directly marks payment verified.
