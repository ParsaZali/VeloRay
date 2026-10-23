# GeoIP / Geosite updates

Open **Cores → GeoIP / Geosite** on a managed systemd node. The section loads its status and catalog when opened. Enable **Allow official downloads** in **Core versions**, check releases for the desired asset, choose a version and confirm **Update**. The page follows the background job; closing it does not cancel an accepted update.

| Asset | Official source | Release file | Installed file |
|---|---|---|---|
| GeoIP | [v2fly/geoip](https://github.com/v2fly/geoip) | `geoip.dat` | `geoip.dat` |
| Geosite | [v2fly/domain-list-community](https://github.com/v2fly/domain-list-community) | `dlc.dat` | `geosite.dat` |

Only stable releases with official GitHub SHA-256 asset metadata are accepted. Repository, download URL, destination filename and commands are fixed by the agent. Callers cannot supply a URL, local path or shell command. Metadata/downloads have size and time limits, and redirects are restricted to official GitHub storage.

## Validation and recovery

The agent verifies the complete downloaded file's size and SHA-256, checks its protobuf index, and uses the currently installed Xray to parse **every geo tag**, including unused categories. Routing matchers are checked in bounded batches. The current node configuration must also validate with the staged file and the other installed asset. This avoids accepting a valid outer index containing invalid domains or CIDRs.

The current file must be readable and valid enough to retain as a rollback snapshot. A running Xray has its durable traffic counters collected before any restart. The agent writes a private recovery journal, atomically replaces the asset, restarts a previously running Xray and requires healthy traffic accounting before committing. A stopped Xray remains stopped.

Failed installation/restart restores the previous file and metadata. Interrupted pending operations are recovered when the agent starts; committed operations keep the new file. An interrupted browser job is reported for inspection rather than falsely reported as a successful update. Accounting identities and subscription/user credentials remain unchanged.

**Rollback** restores the retained previous asset after checking it against the current Xray/configuration. It is available even when new downloads are disabled. Reinstalling identical content preserves the earlier rollback target. Completed updates retain only current and previous managed snapshots, rather than accumulating daily releases.

Core replacement, geo replacement and configuration application share the agent's deployment lock. Policy changes and concurrent core/geo updates are rejected while an update is running. Core-version changes can require different geo formats; incompatible files are rejected before replacing the live asset.

## Installation and access

Upgrade remote agents and their systemd unit before using these controls. The default live directory is `/usr/local/share/xray`; the agent's sandbox permits writes to this directory. If an operator changes `XRAY_LOCATION_ASSET`, its systemd drop-in must allow the new absolute directory and Xray must use that same directory. Recovery snapshots, metadata and journals live under `/var/lib/veloray-cores/geo`.

Only a superuser with administrator API scope can view the updater, list releases or request updates. Mutations use normal authentication/CSRF checks and record an audit request. Browsing installed tag names in the routing editor remains a separate read-only feature.

Container deployments keep their assets in the image and update by replacing that image. These controls manage Xray protobuf assets; sing-box SRS rule sets use a different format and are not converted or replaced by this updater. Downloads are manual and disabled until configured. Xray connections can reconnect during a successful update or recovery.

## API

- `GET /api/nodes/{id}/geo-manager/`: policy, installed hashes/source, availability, rollback and job status.
- `GET /api/nodes/{id}/geo-releases/?kind=geoip|geosite`: official verified release catalog.
- `POST /api/nodes/{id}/geo-install/` with `{"kind":"geosite","tag":"20261023064708","rollback":false}`: accept a background update and return `202`.
- The same route with `{"kind":"geosite","rollback":true}`: restore the previous retained asset.
- Poll the status endpoint until the job succeeds or returns its error.
