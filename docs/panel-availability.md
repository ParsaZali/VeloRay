# Active/standby panels

Panel processes sharing one PostgreSQL database now elect a background-job leader using a dedicated PostgreSQL session advisory lock. Other processes keep serving HTTP but remain standby for recurring jobs. When the owning session ends, a standby acquires the lock and starts reconciliation, monitors, order delivery and housekeeping. Existing per-node/operation/financial transactions continue to serialize changes and make retries idempotent.

Inspect **Operations → Diagnostics → Panel availability** or `GET /api/cluster` as a superuser. Records show the hostname, last heartbeat and active/standby/offline status. A single ordinary installation also uses this scheduler; it needs no extra configuration.

For HTTP availability, deploy the same VeloRay version on two panel hosts behind an external HTTPS load balancer and use:

- One shared PostgreSQL database with a direct or session-affine connection. Transaction-mode PgBouncer is unsuitable for session advisory locks.
- Identical panel field-encryption, session/CSRF secrets, public URL, trusted origins and trusted proxy settings. Run migrations before exposing a new version.
- The same custom templates and supporting files on both hosts. Distribute TLS certificates consistently or terminate TLS at the load balancer.
- Reachability from both panel hosts to every agent. The same agent certificate pins and encrypted credentials come from the shared database.
- External subscription listener/proxy port configuration kept consistent on both hosts. Use manual endpoint management when the load balancer owns these listeners; a local installer-managed port change is not a cluster-wide proxy rollout.
- Load-balancer health checks against `/api/health`, which returns a non-success status when the database is unavailable.

Takeover starts after the old worker session is released, checked every five seconds. A clean shutdown first cancels and drains its worker goroutines. PostgreSQL, the load balancer, templates and certificate distribution require their own availability setup; this feature does not create a database cluster or clone a server. Both panel hosts should use the same verified release and have synchronized clocks.
