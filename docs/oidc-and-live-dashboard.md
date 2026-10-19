# Optional OIDC login, live dashboard and date presentation

## OIDC

Local password login remains available. SSO is disabled until configured by the
server operator. Register a confidential OIDC authorization-code application at
your identity provider with an exact redirect URI:

```
https://YOUR-PANEL-ORIGIN/api/auth/oidc/callback
```

Include the HTTPS port if it is not 443. Put these variables in the root-managed
`/etc/veloray/veloray.env`, with shell-safe quoting for their values, and restart
`veloray-web.service`:

```
VELORAY_OIDC_ISSUER=https://identity.example/realms/operations
VELORAY_OIDC_CLIENT_ID=veloray-panel
VELORAY_OIDC_CLIENT_SECRET=provider-issued-secret
```

The public URL must be the actual panel origin and secure cookies must remain
enabled. Discovery, authorization, token and JWKS endpoints must use HTTPS,
without credentials, query parameters or fragments. Discovery/token redirects
are not followed. Keep provider credentials out of browser configuration.

An administrator must pre-bind the exact **OIDC issuer** and opaque **OIDC
subject** in the existing administrator/reseller/staff user editor. Bind to
trusted subjects obtained from the provider's administrator tools. Email claims,
usernames, provider groups and role claims never provision users or grant roles.
The account must remain active and staff; existing local permissions and budgets
apply. Changing a binding invalidates old sessions/challenges through the local
authentication version. Both binding fields can be cleared to disable that user.

Login uses PKCE S256, a nonce and a five-minute single-use state bound to an
HttpOnly, Secure, SameSite=Lax browser cookie. ID-token signature, issuer,
audience, expiry, nonce and authorized party are checked. A user with local TOTP
still needs a valid TOTP/recovery code after provider login: the two-minute
single-use challenge is not a full panel session. Existing administrator 2FA
setup requirements continue to apply. Final local sessions retain the existing
strict cookie policy. Keep a tested local administrator login for provider
outages. Direct GitHub/Telegram OAuth and LDAP are not implemented by this OIDC
adapter; use an OIDC-capable identity broker if required.

`GET /api/auth/oidc` exposes only an enabled flag. `POST /api/auth/oidc/start`
and `/complete` require the normal CSRF cookie/header. `/callback` consumes
provider state and redirects to the local UI; errors do not expose provider
claims or tokens. Authentication attempts are rate limited. Integration tests
use an isolated TLS issuer, JWKS and signed tokens; configure and verify your
actual provider before relying on it for operations.

## Live dashboard

`GET /api/events` requires `dashboard.read` and normal authentication. Browser
EventSource uses the session cookie; API clients may use an authorized bearer
key. It sends an `overview` event every ten seconds, filtered through the same
owner restrictions as the normal dashboard. Authentication and authorization
are rechecked before each event. Limits are two streams per user and 32 per
process, five-second writes/queries and a two-minute connection lifetime with
reconnection. No client credentials or tokens appear in event data. Existing
HTTP polling remains available if streaming is unavailable. Reverse proxies
must allow streaming and honor `X-Accel-Buffering: no`.

## Calendar and timezone

Settings → System → Date presentation supports automatic, Gregorian or Persian
(Jalali) display and a validated IANA timezone, saved in the current browser.
Expiry and outbound collection timestamps use `Intl.DateTimeFormat` with these
preferences. Automatic calendar follows the selected UI language. Persian New
Year and Tehran day-boundary behavior are covered by browser tests. Date input
controls remain explicitly Gregorian; this change does not implement a Jalali
input picker or change renewal/accounting timestamps stored by the server.
