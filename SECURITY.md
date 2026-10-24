# Security

Report vulnerabilities privately through GitHub's **Report a vulnerability** feature when the repository maintainer has enabled it. Do not post live tokens, subscription URLs, passwords, database dumps or server logs containing secrets in public issues.

Test deployments on a separate server before enabling production traffic.

## Deployment boundaries

- The panel runs as `veloray`; the agent needs host privileges to write Xray configuration and restart its service. A compromised administrator or agent token can control that node's Xray configuration.
- Browser writes, including login, require a signed CSRF token and trusted origin. Session cookies are HttpOnly, SameSite Strict and secure in production.
- Passwords use Argon2id. Recovery codes and API tokens are hashed; node tokens, TOTP secrets and Telegram tokens are encrypted with AES-GCM.
- Subscription URLs are bearer credentials. Anyone possessing one can retrieve the client's connection details.
- Enabling mandatory 2FA restricts all management routes, including administrator APIs. API key owners must have enrolled 2FA; API keys themselves are noninteractive credentials and do not prompt for TOTP on each request.
- Preserve `VELORAY_FIELD_KEY`. Replacing it does not rotate existing encrypted secrets and makes them unreadable.
- Remote agents require HTTPS. Disabling TLS verification is an explicit per-node exception and should be replaced with a trusted certificate or CA bundle.
- The agent refuses remote plaintext listeners and supports optional client-certificate authentication.
- Configuration patches are privileged input. They can alter routing and listeners; they cannot disable the panel's local stats API or required counters.
- External backup imports require an administrator session or administrator-scoped API key, including the configured 2FA policy. SQLite files are opened read-only with bounded parsing; SQL dumps are never executed. Preview payloads are encrypted, expire after 30 minutes and are cleared after commit. Imported inbounds remain disabled for review.
- Telegram commands require a whitelisted sender in that sender's private chat. Bot changes are opt-in, require expiring owner-bound confirmations and are unavailable when administrator 2FA is mandatory. Cancellation and execution consume the confirmation. Bot tokens are encrypted; audit entries record the owner ID without recording credentials.
- Traffic policy affects billed usage while retaining actual counters. Quota and access policy for imported shared accounts applies across their connections. Passwords and subscription tokens are excluded from usage CSV exports and import previews.

## Native host control

The web process retains its unprivileged account. A separate root-owned `veloray-host-control` systemd worker accepts two typed database jobs: updating VeloRay from the fixed official repository and changing the panel's public HTTPS port. Only superusers with administrator scope may queue them; normal CSRF, origin and 2FA authorization still applies. A unique active-job constraint and a PostgreSQL advisory lock serialize execution. Interrupted jobs are reconciled instead of executed again automatically.

Update jobs pin the revision the administrator confirmed. The updater independently checks the current official `main` head and that exact revision's successful push CI, verifies the immutable installer, makes a private backup and attempts application rollback on failure. A moved branch head requires a fresh check. Port changes reject service, subscription and local inbound collisions and check HTTPS health, attempting to restore the old port on failure. The browser cannot supply shell commands, executable paths, arbitrary repositories or download URLs. Logs remain in the root-owned `/var/log/veloray/host-control.log`; API status exposes only the operation and its outcome. A panel port change cannot open a provider or host firewall.

Backups are not encrypted by the archive format. Their files and directory are private, but copy them only to access-controlled, encrypted storage. The manifest detects corruption; it is not a signature against an attacker who can rewrite the archive and manifest together.
