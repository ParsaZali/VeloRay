# External login and trusted proxies

Local login remains available. External login never creates users or matches
email addresses. Administrators bind an existing active staff account to one
exact issuer/subject pair in **Users**. Existing permissions, suspension,
session revocation and local TOTP/recovery codes continue to apply. Each local
user currently has one OIDC/GitHub identity binding and an optional separate
[LDAP directory DN](ldap-login.md).

## GitHub OAuth

Register a dedicated OAuth app with exact HTTPS callback
`https://panel.example/api/auth/github/callback`. Set `VELORAY_GITHUB_CLIENT_ID`
and `VELORAY_GITHUB_CLIENT_SECRET` in the panel environment, or mount
`VELORAY_GITHUB_CLIENT_SECRET_FILE` instead of the secret value. Restart the
panel. HTTPS and secure cookies are required. Bind the local user's issuer to
`https://github.com` and subject to their **numeric GitHub account ID**, not the
mutable login name. The login page shows the button when both keys are configured.

The adapter uses fixed GitHub.com endpoints, S256 PKCE, five-minute encrypted
server-side state and a secure HttpOnly browser-binding cookie. State is single
use. Access tokens are used only to fetch `/user`, are not persisted and are not
sent to the browser. Token/identity endpoint redirects are rejected. No email or
repository scopes are requested; GitHub can retain previous grants for an app,
so use a dedicated login app. GitHub Enterprise Server is not selected by this
adapter. Existing `auth.oidc-login` audit events cover both external login paths.

## Telegram through standard OIDC

In BotFather's Login settings register the panel origin and exact callback
`https://panel.example/api/auth/oidc/callback`, then configure:

```
VELORAY_OIDC_ISSUER=https://oauth.telegram.org
VELORAY_OIDC_CLIENT_ID=<Client ID from BotFather Login settings>
VELORAY_OIDC_CLIENT_SECRET_FILE=/path/to/telegram-oidc-client-secret
```

Use the **separate OIDC Client Secret**, not the bot API token. Bind the local
account to issuer `https://oauth.telegram.org` and the verified ID token's exact
`sub` value. Telegram's OIDC subject need not equal the Bot API user ID. Keep
RS256 signing. The panel requests only `openid`, verifies the signed token,
issuer, audience, expiry, nonce and authorized party, and does not call UserInfo.
The login button is labelled Telegram for this issuer. The one generic OIDC
provider slot may run alongside direct GitHub login.

Contract tests use simulated providers; live sign-in with your app credentials
must be verified after registration. The implementation does not include the
legacy Telegram iframe widget. Direct directory sign-in is documented in
[LDAP directory login](ldap-login.md).

Protocols: [GitHub OAuth](https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/authorizing-oauth-apps),
[Telegram OIDC](https://core.telegram.org/bots/telegram-login#openid-connect).

## Trusted proxies and per-client rate limits

`VELORAY_TRUSTED_PROXIES` accepts up to 32 comma-separated explicit CIDRs.
Defaults are `127.0.0.1/32,::1/128`, matching bundled Nginx/Caddy deployments.
Set an empty value to disable forwarded addresses. For a separate proxy specify
only its actual source addresses. Default routes (`0.0.0.0/0`, `::/0`) are rejected.

Only trusted socket peers may supply `X-Forwarded-For`. At most 16 hops are
evaluated from right to left, stopping at the first untrusted address.
Malformed/oversized headers fall back to the socket peer. Direct connections
cannot substitute a rate-limit address. Nginx overwrites the header with
`$remote_addr`; Caddy supplies its default validated chain. An upstream CDN
requires an explicitly maintained proxy configuration.

Login rate limits and password-login audit records use the resolved address.
Subscription history continues to omit source IPs. `RemoteAddr` retains the
socket peer. Loopback trust assumes that local
processes are trusted to act as reverse proxies.
