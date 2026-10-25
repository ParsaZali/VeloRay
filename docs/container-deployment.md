# Docker deployment

The quick installer now defaults to Docker and installs Docker Engine and Compose from Docker's signed official apt repository when needed:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/ParsaZali/VeloRay/main/install.sh)
```

Supported hosts are Ubuntu 22.04/24.04 on amd64/arm64. Enter a domain or public IP, then panel and subscription ports; Enter generates independent random free ports. DNS uses automatic Caddy HTTPS; IP installs use a local certificate. No CDN port filter is applied. The generated administrator name is random. The initial password is displayed only on an interactive terminal and remains in `/etc/veloray/docker/secrets/admin_password`. Resetting a password never reuses that bootstrap file.

The panel, subscription process, agent, Xray/sing-box/native protocol runtimes, PostgreSQL and HTTPS proxy run inside containers. Docker Engine and small root-only maintenance timers run on the host. The default preserves host networking for arbitrary operator-selected TCP/UDP listener ports. It does not mount the Docker socket or run privileged containers.

## Management

```bash
sudo veloray
sudo veloray doctor
sudo veloray reset
sudo veloray domain vpn.example.com
sudo veloray port 23456
sudo veloray backup
sudo veloray restore /private/recovery.tar.gz
sudo veloray update
sudo veloray auto-update enable
sudo veloray backup-schedule daily
```

`veloray update` checks the exact current main commit's successful push CI, verifies source checksums and both runtime manifests, builds revision-tagged images before interrupting traffic, then creates a coordinated recovery snapshot. Failed activation restores the matching database, encryption keys, agent ledger/configs and proxy state. Restoring a backup creates another recovery point first. Treat the root-only backup as sensitive: it contains the keys needed to recover encrypted records. Recovery requires its matching release directory under `/opt/veloray/releases/`.

Normal uninstall removes only this Compose project's containers, preserving volumes, credentials, releases and backups. `--purge` additionally removes that project's volumes and private deployment configuration after a recovery backup and explicit PURGE confirmation; Docker and other applications are preserved. Reinstalling an unpurged deployment resumes its original identities and secrets. After native migration, the original stopped database remains a recovery source. An explicit Docker purge never silently imports those archived users again. Missing Docker keys after a completed migration require restoring the Docker recovery configuration.

**Settings → System** shows deployment metadata and verified updates. **Settings → General** changes the panel port through an audited allowlisted host job; **Settings → Subscription** independently controls the subscription listener. The browser and application container cannot execute arbitrary host commands. The root timer polls only typed database jobs for panel updates/ports.

## Migrate an existing native installation

Rerun the quick installer. It preserves an existing native installation rather than treating it as a new empty database. Preflight requires its original environment keys, a local PostgreSQL database and an existing administrator. Other nginx applications and external/custom databases require an operator-managed migration. An unmodified Ubuntu nginx welcome site is allowed.

The installer builds images before stopping services, takes a coordinated native backup, copies the database and durable agent ledger, preserves installed core slots and certificates, and starts Docker using the original origin, subscription port, users and tokens. It disables native application units only after HTTPS health succeeds. A failed migration restores native units and keeps the recovery set. A power interruption leaves an explicit migration marker; recover the original native services before retrying:

```bash
sudo python3 /opt/veloray/docker-current/deploy/docker/manage.py recover-migration
```

The old native database and files remain available for recovery; they are not a live second panel. Never start the native agent and container agent together. Existing native `veloray update` deliberately retains native deployment; use the quick installer for migration. Remote `install.sh node` installations retain the existing native-agent installer.

## VPN TLS certificates and core updates

The automatic installer exports the matching Caddy certificate into `/etc/veloray/docker/certificates/HOST/`, mounted read-only inside the agent at `/etc/veloray/certificates/HOST/`. Use `fullchain.pem` and `privkey.pem` there in TLS inbounds. A host timer checks renewed certificates every five minutes and restarts the agent only when the key pair changes. This also covers short-lived local certificates. Certificate export does not expose Caddy's complete private storage to the panel or cores. Import an operator-provided pair with:

```bash
sudo veloray certificate-import vpn.example.com /private/fullchain.pem /private/privkey.pem
sudo veloray certificate-sync
```

Managed core downloads, previous-version rollback and GeoIP/Geosite updates work in process mode using the existing official-source/checksum/configuration validation. Core slots and assets are stored in the agent data volume, so recreating a container does not discard an explicitly installed core. Downloads remain opt-in. Kernel TUN/transparent routing still needs an explicit Compose override for NET_ADMIN and /dev/net/tun; it is not enabled by default.

## Manual Compose setup

The steps below remain available for operators managing their own images and lifecycle. Such deployments do not advertise the automatic root host worker unless it was installed.

The production Compose definition is `deploy/docker/compose.yaml`. The root
`compose.yaml` remains the existing database-only development fixture. The new
stack includes PostgreSQL, the panel, independent subscription serving, the
node agent, both pinned VPN cores, and a Caddy HTTPS frontend.

## Install

Use a dedicated Linux server with Docker Engine and Compose v2, free ports
80/443, 5432, 8610/8612, 9191 and 10085–10087, and a DNS hostname pointing to it.
Container smoke checks run on Ubuntu 22.04 and 24.04 when matching bundles are
present. Runtime bundles and images support amd64/arm64; ARM execution requires
operator staging validation. The legacy native systemd installer targets Ubuntu
24.04. This is not native Windows or rootless-Docker support.

```bash
git clone https://github.com/ParsaZali/VeloRay.git
cd VeloRay
bash scripts/verify-runtime.sh
sha256sum --check --quiet SHA256SUMS
python3 deploy/docker/generate.py --domain vpn.example.com
docker compose --env-file deploy/docker/.env -f deploy/docker/compose.yaml up -d --build --wait
```

The generator refuses to replace existing files, creates an operator-only
`secrets` directory and never prints passwords. Read `deploy/docker/secrets/admin_password`
privately on the server for the first login as `admin`. Keep this directory with
your recovery backup. The generated `.env` contains only the domain. The image
build context excludes credentials, local dependencies and test output. Images
are built from the verified bundled binaries, not an unverified downloaded panel.

The public origin is `https://vpn.example.com`. Caddy automatically requests and
renews its public TLS certificate when DNS and ports 80/443 permit ACME validation.
This covers the panel and subscription HTTPS frontend. VPN listeners that need
their own TLS certificates still require files mounted read-only into the agent
and readable by UID 10002. Caddy's private certificate storage is not silently
shared with the VPN cores. No public domain or DNS record is created automatically.

## Process and permission boundaries

All application containers run as separate non-root identities with read-only
root filesystems, bounded tmpfs, dropped capabilities and `no-new-privileges`.
Only the proxy and agent receive `NET_BIND_SERVICE`; PostgreSQL runs as its own
non-root database user. No container gets `privileged`, the Docker socket, the
host PID namespace or host systemd sockets. Host networking supports operator-
selected TCP/UDP listener ports; database, panel, subscription administration and
agent control bind only to loopback. Open public VPN ports explicitly in the
server firewall. This stack does not automatically change firewall rules.

The agent's bounded `setpriv` entrypoint carries only `NET_BIND_SERVICE` through
the shell and managed core executions as an ambient capability. This permits
ports below 1024 without running the agent as root or changing the host's
unprivileged-port sysctl. Docker Engine 24+ is required for non-root capabilities
with `no-new-privileges`. CI explicitly tests a protected port under the normal
1024 threshold and restores the disposable VM's original setting afterwards.

The agent's opt-in `VELORAY_CORE_MANAGER=process` replaces only the known core
service-control operations. The default remains `systemd`. The process supervisor
starts only the registered Xray/sing-box binaries and config paths, tracks child
PID/start epochs, waits/reaps children, and rejects unknown services or control
commands. An unexpected core exit terminates the agent so Docker's restart policy
can recover it; it is not an unbounded core-restart loop hidden from monitoring.
Initial core configs expose only private telemetry, no public proxy listeners.

Prepared/failed rollback journals matching an interrupted config are recovered
before process-mode listeners start. During graceful shutdown, HTTP work drains
and a final bounded counter observation precedes core termination. Durable ledger
IDs, configuration journals and counters live in the `agent-data` volume. A hard
crash can lose traffic the engine had not exposed to a completed sample; existing
durable accounting does not invent missing bytes or reset customer usage.

Kernel TUN/transparent capture is opt-in and is not enabled by this Compose file.
It needs an explicit deployment override granting `NET_ADMIN` and the appropriate
device, such as `/dev/net/tun`. In host-network mode that capability can change
host routing. Test the operator's route design on an isolated server first.

Each service mounts only its required secret files. `*_FILE` support reads bounded
regular files and rejects ambiguous simultaneous direct/file credentials. The
host's secret directory is mode 0700; its file mounts are readable by dedicated
container users. First-boot administrator creation runs only when the user table
is empty. Restarting a deployment or changing the bootstrap password file does
not overwrite an existing administrator or add another account. Local-node
bootstrap preserves configured inbounds, quotas and tokens.

Raw core output and public proxy request/error logs are deliberately discarded
to keep credentials and subscription URL tokens out of container logs. Use the
panel's structured Operations diagnostics, status and monitor history. This is
not a raw journal browser. Container system-resource values describe the running
environment and are not a hosting-provider invoice.

## Update and recovery

For manually deployed stacks, use Compose for lifecycle management. The automatic Docker installer described above supplies its own `sudo veloray update` controller.
For Docker, keep a private backup, check out a main revision with green CI and
matching verified runtime bundles, then rebuild and recreate with the install
command above. Do not regenerate encryption keys. Use `docker compose ... ps`
for health and `docker compose ... restart agent panel subscriptions` for an
explicit restart. The independent subscription process serves `/sub/*`; its
administration API is unavailable. The panel/agent API remains authenticated.

Compose starts subscription HTTPS on `8444`, separately from panel port `443`.
For a different initial port, set `VELORAY_SUBSCRIPTION_PORT` in the generated
Compose `.env` before first start. Afterwards, change it in **Settings →
Subscription**; the proxy config volume preserves it across restarts. The panel
can access only a constrained Unix control socket. The Caddy configuration
socket remains private inside the proxy container. Open the chosen TCP port in
the host and provider firewalls. Old `/sub/` links on `443` redirect to it.

A recovery set must include the PostgreSQL dump, agent-data volume, generated
secrets and Caddy data/config volumes. Stop proxy, panel, subscriptions and agent
before taking a coordinated snapshot; keep the database running for `pg_dump`.
`docker cp -a` can preserve the stopped containers' data ownership. Protect the
backup directory with mode 0700 and restore numeric UID/GID metadata. Restore
the same encryption keys and accounting ledger with the matching database.
Never combine an older ledger with a newer customer database. Starting again
does not intentionally reset quota, expiry or subscription credentials.

## Verification

`VELORAY_DOCKER_TEST=1 bash scripts/test-docker.sh` creates a uniquely named,
disposable project and removes only that project's volumes. It builds the actual
images, starts all five services, uses real HTTPS, CSRF/login/session cookies,
creates a managed VLESS listener/customer, moves data through the real core,
checks charged usage, restarts services and verifies ledger identity, quota,
tokens, sessions and bootstrap-password preservation. CI uses localhost/internal
TLS; real public ACME issuance and a user's firewall/domain require staging.
The assembler runs this check before publishing runtime bundles. Main/PR CI also
tests matching bundles on the two supported Linux host labels. Source revisions
with stale bundles explicitly wait for assembly; final release verification
must run on the packaged revision.

## Cloud-init and Cloudflare DNS

`python3 deploy/cloud-init.py --domain vpn.example.com > user-data.yaml` creates
Ubuntu 24.04 cloud-init data. It installs Docker prerequisites, checks the exact
selected commit's main CI, verifies runtime/source checksums, generates secrets
on the target server and starts the complete stack. No administrator password or
provider token is embedded in user-data. An optional exact recent main commit can
be selected using `--revision`; the CI lookup must still be available. First boot
refuses to overwrite an existing installation or local source changes. Retry
`sudo veloray-firstboot` after correcting a transient failure; existing keys are
preserved. A completion marker is written only after verified local HTTPS health.

The optional `deploy/cloudflare-dns.py` CLI creates or updates one explicit A/AAAA
record through Cloudflare's HTTPS API:

```bash
python3 deploy/cloudflare-dns.py --zone-id ZONE_ID --hostname vpn.example.com \
  --address SERVER_IP --token-file /private/cloudflare-token
```

Use a narrowly scoped DNS-edit API token for that zone. Token contents never enter
arguments or logs; redirects, ambiguous results and unexpected records fail
closed. DNS-only is the default. `--proxied` is explicit and is for compatible
HTTP services, not arbitrary VPN/UDP listeners. This is exact-name DNS upsert,
not zone discovery, provider subscription import or wildcard DNS-01 issuance.
Provider tests use a mock transport; no user's DNS zone is modified by CI.
