# Independent subscription service

`VELORAY_SUBSCRIPTION_ONLY=true veloray serve` exposes only subscription paths
and `/api/health`; management, authentication and frontend routes return 404.
It uses the shared prepared PostgreSQL schema, encryption keys and template
files, but runs no migrations, traffic reconciliation or background workers.
HWID registration and subscription history still write to the shared database.
The primary panel must run migrations before starting the separate service.

The installer provides `veloray-subscriptions.service` without enabling it.
To run the optional local instance after installation:

```bash
sudo systemctl enable --now veloray-subscriptions.service
sudo systemctl status veloray-subscriptions.service
```

The server controller includes enabled/active subscription instances in
start/stop/restart, status and doctor. `veloray logs subscriptions` shows its
journal. Restore pauses an active instance during data replacement; uninstall
stops it and disables the automatic updater timer.

Its default listener is `127.0.0.1:8611`. Keep that port free. The managed updater
stops/restarts an already active instance around migrations and checks its local
health. Its optional unit is preserved during application rollback. The service
uses the existing root-managed environment and read-only template directory.
Do not give it the privileged agent's credentials. Separate-host deployment
requires an operator-prepared database connection with TLS, the same field key,
template files and matching application/schema versions; automated provisioning
of a remote host is outside this unit's scope.

Configure your own TLS certificate and Nginx subscription virtual host, forwarding
only `/sub/` and optionally health. For example, inside a TLS server block:

```nginx
location /sub/ {
    proxy_pass http://127.0.0.1:8611;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Proto https;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
}
location / { return 404; }
```

Set `VELORAY_PUBLIC_URL` for that process through a systemd drop-in if its browser
portal must generate URLs at the subscription origin. A second service does not
automatically rewrite links copied from the primary panel; substitute only the
origin, preserving the complete secret path. Never log subscription tokens in
reverse-proxy access logs. Check a synthetic active, expired and HWID-limited
subscription at the new domain before changing customer-facing links.
