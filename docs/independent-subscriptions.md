# Separate HTTPS subscription endpoint

The installed subscription process serves `/sub/*`, public subscription assets,
and `/api/health`. Administration and authentication routes return 404. It shares
the panel database and private template directory, but does not migrate the
database or run the panel's background jobs. It listens on `127.0.0.1:8611` on a
native server and `127.0.0.1:8612` in Docker. Those addresses are local only.

The **public HTTPS port** is independently managed in **Settings → Subscription
→ Separate subscription port**. The first native install selects an available
port starting at `8444`, avoiding existing local inbounds; Docker defaults to
`8444` and its initial port can be set with `VELORAY_SUBSCRIPTION_PORT` before
the stack is created. The control is available to superuser administrators.
Open the chosen TCP port in the host and provider firewalls. Existing `/sub/`
links on the panel port redirect to the applied port and preserve the path and
query. New customer links, portal URLs and QR codes use the applied port.

Port changes enter a pending state. Links switch only after the proxy listener
is configured successfully; an apply error leaves the old listener and URLs in
place. The screen shows both addresses and the application status. A stale
revision or conflict with the panel, a reserved service or a local VPN inbound
is rejected. Existing customer tokens, usage, quotas and templates remain the
same. Apps that do not follow HTTP redirects should receive the updated URL.

## Native server

The installer enables `veloray-subscriptions.service` and a root-only
`veloray-subscription-endpoint.timer`. The timer checks requested changes every
few seconds. It renders an Nginx HTTPS listener that forwards to the
subscription-only service, validates and reloads Nginx, checks the local HTTPS
health route, then records the new port in the database. Nginx reuses the
panel's existing certificate. The web process cannot write Nginx configuration
or read the certificate key. `sudo veloray stop` stops the timer before the
services; `sudo veloray start` starts it again. Inspect the independent process
with `sudo veloray logs subscriptions` and the timer with
`sudo journalctl -u veloray-subscription-endpoint.service -n 100`.

## Docker

The Caddy proxy serves the panel on `443` and subscriptions on the configured
port. Only the subscription-only process handles requests to the latter. The
panel requests port changes over a dedicated Unix socket restricted to one
integer operation. Caddy's configuration API uses a separate private socket
inside the proxy container. Its applied port is saved in the proxy config
volume so Compose restarts preserve the choice. See
[container deployment](container-deployment.md) for the stack and backups.

For a custom HTML subscription page, change the template path separately in
**Settings → Subscription**. That setting controls the page content; the port
setting controls its public origin. See [subscription customization](subscription-customization.md).
