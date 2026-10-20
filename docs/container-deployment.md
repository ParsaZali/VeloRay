# Full container deployment

The production Compose definition is `deploy/docker/compose.yaml`. The root
`compose.yaml` remains the existing database-only development fixture. The new
stack includes PostgreSQL, the panel, independent subscription serving, the
node agent, both pinned VPN cores, and a Caddy HTTPS frontend.

## Install

Use a dedicated Linux server with Docker Engine and Compose v2, free ports
80/443, 5432, 8610/8612, 9191 and 10085–10087, and a DNS hostname pointing to it.
Container smoke checks run on Ubuntu 22.04 and 24.04 when matching bundles are
present. Runtime bundles and images support amd64/arm64; ARM execution requires
operator staging validation. The native systemd installer still targets Ubuntu
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

Use Compose for container lifecycle management. The native `sudo veloray update`
continues to update systemd installations; it does not upgrade a Docker stack.
For Docker, keep a private backup, check out a main revision with green CI and
matching verified runtime bundles, then rebuild and recreate with the install
command above. Do not regenerate encryption keys. Use `docker compose ... ps`
for health and `docker compose ... restart agent panel subscriptions` for an
explicit restart. The independent subscription process serves `/sub/*`; its
administration API is unavailable. The panel/agent API remains authenticated.

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
