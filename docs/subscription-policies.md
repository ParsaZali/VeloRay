# Subscription policies and external feeds

Administrators can open **Infrastructure → Subscription policies** to assign an
external feed, portable connection links and client presentation rules to native
customers or services. A policy never creates a listener or deploys to an agent.
Resellers cannot read or modify source credentials through this API.

Create a named policy, select customers/services, and optionally add a public
HTTPS feed URL or newline-separated URIs. Save, then use **Refresh external feed**
to fetch the remote source. Refresh is explicit: subscriber requests read the
last successful encrypted cache and never initiate outbound requests. A failed
refresh retains that cache. Changing the source URL clears it. Disabled policies
are ignored immediately. Deleting a customer/service does not grant its old
assignment to another account.

Supported portable links are VLESS, VMess, Trojan, SIP002 Shadowsocks (AEAD/2022),
Hysteria2 and TUIC v5. Feeds can contain raw or Base64-encoded URI lists. This is
not a general Clash YAML/sing-box configuration importer. Unsupported obfuscation,
insecure TLS and transport/security options are rejected instead of stripped.
Validate exported files in the intended client before enabling the policy.
External providers enforce their own connection traffic and quotas; VeloRay
cannot measure external traffic that never traverses its nodes.

The account must already have an eligible native connection. Its expiry, quota,
access restrictions, shared-token revocation and HWID admission still run before
external entries are returned. External links do not bypass local admission.
Client-app selectors are presentation hints from User-Agent, not authentication.
An app filter of zero entries matches every app. Multiple applicable policies
are processed by increasing ID; the last configured format/template wins.
Explicit format requests still override the default format. `mihomo` and `stash`
are aliases for Clash output.

Optional JSON templates for Clash, sing-box and Xray use the same generated
proxy/outbound replacement rules as global subscription templates. For example,
Clash groups can use the `$VELO_PROXIES` array item. Empty template values use
built-in output. Refer to [subscription customization](subscription-customization.md).

Limits: 100 policies, 200 assignments per list, 200 URIs per source, 300 combined
external entries per account, 1 MiB remote response, 16 KiB per URI, and 128 KiB
per JSON template (512 KiB total policy body). Refresh uses public-IP-pinned HTTPS,
no redirects/proxies, and a five-second timeout. Internal/private destinations
are rejected. List responses omit URLs, links and template bodies. Authorized
administrator detail responses include the editable body; both body and cached
connections are encrypted with `VELORAY_FIELD_KEY` at rest.

API (administrator role and administrator API scope):

| Route | Methods | Result |
|---|---|---|
| `/api/subscription-policies` | GET, POST | Metadata list / create |
| `/api/subscription-policies/{id}` | GET, PATCH, DELETE | Authorized body / update / delete |
| `/api/subscription-policies/{id}/refresh` | POST | Atomic cache replacement; conflict if policy changed during fetch |

Inputs are `name`, `enabled`, `customer_ids`, `service_ids`, and `body` containing
`url`, `links`, `apps`, `default_format`, `templates`. Omitted top-level fields
are preserved by PATCH; a supplied `body` replaces the whole body with defaults
for its omitted fields. Source credentials never enter audit details.
