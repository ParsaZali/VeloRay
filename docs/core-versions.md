# Core versions

Native systemd installations can manage **Xray** and **sing-box** under **Cores → Core versions**. Remote agents must be upgraded to this installer before the controls become available. Containers keep their cores in the image and are upgraded by replacing that image.

## Official downloads

Downloads are **disabled by default**. Enabling **Allow official downloads** permits manual changes; it does not enable automatic core updates. The catalog is restricted to stable Linux `amd64`/`arm64` assets from `XTLS/Xray-core` and `SagerNet/sing-box`. The agent retrieves release metadata from the official GitHub API, requires an asset SHA-256 digest, verifies the complete archive and rejects redirects outside official GitHub asset storage. Archives are bounded and only the expected regular ELF executable is adopted.

Choose a release and select **Install version**. Installation runs on the agent as a bounded background job, so closing the browser does not cancel an in-progress change. The page polls the current job. Only a superuser using administrator scope can change the policy or install/roll back a core. Catalog/policy/install endpoints use normal CSRF and authentication checks and record audit intent without credentials.

## Validation, traffic and rollback

The selected binary must validate the current configuration before the active executable is replaced. The agent harvests traffic counters, stores an operation journal, switches the managed executable and restarts a previously running service. Stopped services remain stopped. A running core must return healthy accounting statistics before the change is committed. The durable traffic ledger identity is retained.

A failed replacement/restart restores the previous binary and metadata. An interrupted operation is recovered from its journal when the agent starts. **Rollback** switches to the previous retained executable after checking it against the current configuration. Updating VeloRay preserves an explicitly selected official core; native MTProto/AmneziaWG update with the VeloRay agent itself.

Executables live under `/var/lib/veloray-cores/{xray,singbox}`. System binaries point to the controlled `current` slot. The agent can write this directory without making `/usr/local/bin` writable in its systemd sandbox. Policy and journal files are root-only. Ordinary configuration deployment remains serialized with executable replacement.

## Compatibility

Official versions may support different configuration options or compile-time features. Validation failures preserve the previous core. Core version changes reconnect active sessions; they are explicit administrator operations, not transparent traffic migration.

VeloRay's bundled sing-box build includes an additional per-user IP telemetry field. Official binaries do not provide that attribution. A switch to an official sing-box binary is therefore rejected while per-user IP caps are configured. Later deployments requiring that feature are also rejected while an official binary is active or being installed. Ordinary traffic counters still use the official stats API; missing user-IP attribution is reported as unavailable rather than a fabricated empty result. Keep or return to the bundled core when those controls are needed.

Only binaries are replaced. Geo assets, TLS keys, routing policy, users and configurations are preserved. The bundled cores remain the CI-tested reference versions; arbitrary upstream releases still need operator verification with the actual client applications and network.

GeoIP and Geosite have separate [verified update and rollback controls](geo-updates.md). Core and geo operations cannot run together. Both use the same opt-in official-download policy; neither enables automatic updates.

## API

All following routes are superuser/administrator-scope only:

- `GET /api/nodes/{id}/core-manager/`: managed capability, manual-download policy, installed metadata and job status.
- `GET /api/nodes/{id}/core-releases/?core=xray|singbox`: stable releases with verified asset metadata.
- `POST /api/nodes/{id}/core-policy/` with `{"enabled":true|false}`.
- `POST /api/nodes/{id}/core-install/` with `{"core":"xray","tag":"v...","rollback":false}`; returns `202` for the queued job.
- The same install route with `rollback:true` restores the previous retained executable. It remains available when new downloads are disabled.
