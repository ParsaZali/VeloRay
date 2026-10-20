# Platform and database portability assessment

## Windows investigation

An actual `GOOS=windows GOARCH=amd64 go build ./cmd/veloray` was attempted for this
source. Compilation stops at `syscall.Stat_t` in host backup ownership handling,
`unix.Statfs_t`/`unix.Statfs` in system metrics and `unix.O_NONBLOCK` in the
confined custom-template reader. The CLI also uses Unix exec semantics; native
installation and recovery use systemd, Unix permissions, nginx and apt.

Changing just a build tag is insufficient: a Windows service controller,
ownership/backup semantics, safe nonblocking template-file access, port-owner
inspection, resource metrics and privileged network operations need distinct
implementations and Windows integration fixtures. No native Windows binary is
advertised or shipped. The new process core controller removes the systemd
dependency for Linux containers, not the remaining OS contracts. Docker Desktop
host networking/WSL2 can be evaluated separately; it is not a tested production
target for this release.

## SQLite, MySQL and MariaDB

PostgreSQL is authoritative. Existing SQLite/relational backup adapters import
data into PostgreSQL; they do not implement alternate runtime storage engines.
Adding a database driver alone would leave transaction safety and accounting
incorrect. Confirmed backend dependencies include:

- JSONB operators, typed casts, object/array containment, computed indexes and
  upserts in store/migration, management, policy, renewal and worker paths.
- PostgreSQL advisory transaction locks serialize configuration, principal
  allocation, migration and collection work across multiple panel processes.
- Row locks, `SKIP LOCKED` leases and timestamp arithmetic coordinate monitor,
  notification and deployment outboxes without duplicate consumption.
- `pgx.Tx`, PostgreSQL error/constraint handling, arrays/`ANY`, `RETURNING` and
  principal/account SQL expressions are shared by reseller-budget and quota
  mutation code. The independent subscription service relies on the same DB.
- Backups and restore are tied to PostgreSQL tools and the current relational
  schema, encryption keys and durable agent ledger.

SQLite would require a single-writer deployment model and a replacement for
distributed advisory locks, while preserving atomic membership/quota changes.
MySQL/MariaDB need explicit dialect repositories and lock/lease semantics; their
JSON and locking behavior cannot be assumed equivalent. Migrating only read
queries or storing blobs without constraints is not a compatible backend.

A future implementation needs a typed storage boundary, equivalent constraints
and atomic repository operations, differential tests for every mutation and
traffic/outbox transaction, multi-process concurrency tests, migration/restore
tools and a staged cutover/rollback plan. Existing PostgreSQL tests must remain
the reference. There is no new `DATABASE_ENGINE` option or untested backend
switch in this release. This assessment completes the investigation item; the
alternate database implementations remain a separate undertaking.
