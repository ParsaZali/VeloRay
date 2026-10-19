# Subscription delivery and customization — VeloRay 1.0

This delivery adds file-backed HTML pages, a simple built-in page, export
configuration templates, subscription hosts, HWID admission/revocation,
a compatibility report and authenticated Prometheus metrics. It does **not**
complete the entire competitor-parity backlog.

## Built-in pages

Settings → Subscription → Template:

- **Simple subscription page** (`default`): the new default. Account name,
  quota, expiry, copy link, optional downloads/QR, collapsed connection details
  and a persistent system/light/dark selector. No dashboard or chart library.
- **Modern dashboard** (`dashboard`): retains the existing analytics portal.
- **Classic compact portal** (`compact`): retains the original compact page.
- **Custom HTML file** (`custom`): operator-owned HTML from disk.

Existing `default` selections now use the simple page. Select `dashboard` to
keep the previous appearance. Existing tokens and client format URLs continue
to work. Changing a page or delivery host does not deploy/restart traffic cores.

## Custom HTML files

The installer creates `/etc/veloray/templates` owned by `root:veloray`.
The panel service needs read/traverse permission; it never writes templates.
An example is installed at `/etc/veloray/templates/example/index.html` on fresh
installs or upgrades containing the `examples/subscription` source directory.
Existing example files are not overwritten.

1. Put your HTML file and optional `assets/` directory there.
2. Select **Custom HTML file** in Settings → Subscription.
3. Enter `/etc/veloray/templates/example/index.html` (or `example/index.html`).
4. Select **Validate template**, then **Save settings**.

Plain static HTML works. Dynamic values use Go's `html/template` syntax:

```html
<h1>{{.Name}}</h1>
<p>{{.Used}} / {{.Quota}}</p>
<p>{{.Expiry}}</p>
<input readonly value="{{.URL}}">
<link rel="stylesheet" href="{{.AssetsURL}}style.css">
{{if .ShowQR}}<img src="{{.QR}}" alt="{{.QRLabel}}">{{end}}
```

Useful values: `Site`, `Name`, `Locale`, `Direction`, `URL`, `AssetsURL`,
`Used`, `Quota`, `Remaining`, `Expiry`, `ExpiryISO`, `Percent`, `Limited`,
`Active`, `Support`, `Footer`, `Announcement`, `AnnouncementURL`,
`SubscriptionTheme`, `ShowQR`, `QR`, `ShowURI`, `ShowDownloads`,
`Connections`, `ConnectionCount`. `Connections` entries expose `Name`,
`Protocol`, `Host`, `Node`, `Link`, `QR`.
Only the current subscriber's data is passed; database/environment/file access
functions are never exposed to the template.

Files reload on each browser page request. A missing, oversized, malformed or
unrenderable file falls back to the simple page with the response header
`X-VeloRay-Template-Fallback: true`. Output is buffered before transmission,
so a template error cannot leak a partial page. This is not a guarantee that an
operator-authored page is visually correct: validate it in a browser too.

The template directory defaults to `/etc/veloray/templates`. To use another
folder, set `VELORAY_TEMPLATE_ROOT=/absolute/folder` in the panel service's
environment and restart the panel. Paths and symlinks cannot escape that root.
The standard backup includes the default folder; a custom folder outside
`/etc/veloray` needs a separate operator backup policy.

Only regular `.html`, `.htm` and `.tmpl` files up to 512 KiB are accepted;
rendered pages are limited to 2 MiB. CSS, JavaScript, common bitmap images,
icons and WOFF fonts can be loaded from a sibling `assets/` folder through
`{{.AssetsURL}}`. Asset reads require a valid, active subscription and cannot
escape that assets folder. Each asset is limited to 4 MiB.

Custom HTML is trusted operator code. Scripts loaded from local assets work;
inline `<script>` bodies receive exact-content CSP hashes after rendering.
Inline event-handler attributes and third-party script origins remain blocked.
Convert a compiled SPA to local static assets if needed; PHP, Node.js and
other server-side code are not executed by the template loader.

## Client configuration templates

Settings → Subscription → Client configuration templates accepts optional
JSON objects for Clash/Mihomo and sing-box. Empty values retain built-in output.
These are JSON, even for the Clash endpoint (JSON is valid YAML syntax).
The panel checks JSON syntax/size, not compatibility with every client version.

For Clash, generated proxy objects replace `proxies`. Use a complete array item
`"$VELO_PROXIES"` in a proxy group's `proxies` list to insert generated names:

```json
{
  "proxy-groups": [{"name":"VeloRay","type":"select","proxies":["$VELO_PROXIES","DIRECT"]}],
  "rules": ["GEOIP,private,DIRECT", "MATCH,VeloRay"]
}
```

For sing-box, generated `outbounds` replace that property in your template.
Other settings (DNS, routing, local inbounds, rule sets) remain operator-defined.
When omitted, the panel's generated routing/proxy groups are retained.
Templates cannot inject account credentials through string substitution.
Do not enable a template incompatible with the clients you serve.

## Subscription hosts

Settings → Subscription → Subscription hosts maps a managed inbound to one or
more exported names/addresses/ports. Optional SNI, path and Host header overrides
are applied only to exported configs; listener settings remain untouched.
A public port of `0` uses the listener's port. Removing all mappings restores
the default node address; leaving mappings present but all disabled hides that
inbound from feeds. The operator must configure the corresponding DNS/CDN/
reverse proxy; adding a host does not provision these external systems.
Hosts only publish connections already present in the subscriber's account.
They do not grant new inbound access and are not a replacement for service groups.

## HWID admission and registered devices

- Global **Default HWID device limit**: `0` is unlimited; `1..1000` limits
  devices admitted to a configuration download and requires an HWID header.
- Global **Require HWID** can require the header even with no cap.
- Per-client `hwid_limit`: `0` inherits the global policy, `-1` disables the
  HWID requirement/cap for that account, `1..1000` overrides the cap.
- The existing **Devices** field remains the concurrent-IP limit.
- Browser pages and chart requests are available without an HWID. While HWID
  is required, built-in and custom pages receive no raw URIs, WireGuard profiles
  or HTTP/SOCKS credentials; configuration downloads apply the policy. HEAD checks do not register devices or bypass caps.
- Registration and admission use one PostgreSQL transaction/advisory lock per
  billed account. Shared-account connections share their registry and cap.
- Client → Devices → **Revoke** blocks that identifier from future downloads;
  **Allow** removes the block; **Forget** deletes the entry and lets it enroll again.
  These actions require write permission and obey reseller ownership checks.

HWID is reported by the app and can be spoofed; NAT/IP identity and hardware
identity are different. This controls subscription delivery, not already copied
connection credentials. Use credential rotation when revoking existing VPN access.
Apps without HWID remain compatible while the requirement is disabled.

## Compatibility report

Client → **Check subscription** (`GET /api/clients/:id/delivery-check`) explains
account/node/inbound/host eligibility and which formats can represent each
connection. It also reports HWID requirements and template-file availability.
The endpoint obeys the same customer ownership rules as other client reads.
It contains no connection secrets and performs no external connectivity test.

## Prometheus

`GET /api/metrics` returns Prometheus text exposition. Use an **administrator-owned
read API key**, never a token in the query string. Other roles receive 403.
Metrics include aggregate nodes/healthy nodes, inbound and connection counts,
current charged/actual usage, and panel uptime. They are gauges because user
traffic counters can reset. Shared billed usage is not counted multiple times.
No per-user, IP or credential labels are emitted.

## Scope and remaining work

The version stays **1.0**. Native TUIC/AmneziaWG/MTProto, hot application of
traffic-core changes, service/access squads, shared config profiles, remote
subscription fetching, Cloudflare provisioning, Discord integration, expanded
OS support and full container deployment remain separate engineering work.
See `docs/competitor-parity-roadmap.md`; unchecked entries are not delivered
features. A PR/CI run does not install these changes on the production server.
