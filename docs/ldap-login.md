# LDAP directory login

Optional LDAPv3 simple bind over verified LDAPS connects a directory identity to
an **existing local panel account**. No directory users, roles or groups are
imported. A directory password is never stored, hashed into the local password
field, written to audit history, or tried automatically after a local failure.

## Configure

Set these variables in the panel environment (native installations use
`/etc/veloray/veloray.env`) and restart the panel service:

```
VELORAY_LDAP_URL=ldaps://directory.example.com:636
VELORAY_LDAP_CA_FILE=/etc/veloray/directory-ca.pem
```

The CA file is optional when the server certificate chains to system roots.
For a private directory, install its CA certificate as a readable, regular PEM
file of at most 1 MiB. The file extends system trust; it is not a client key.
Hostname verification and TLS 1.2 or newer are mandatory. The directory URL must
have no credentials, path, query or fragment. Plain `ldap://`, StartTLS and
certificate-verification bypasses are not supported. The panel origin must be
HTTPS with secure cookies. Restrict network access to the intended directory.
Changes to the endpoint or CA require a panel restart.

For Docker, add the environment only to the `panel` service using a Compose
override and mount the CA read-only at the configured path. For example:

```yaml
services:
  panel:
    environment:
      VELORAY_LDAP_URL: ldaps://directory.example.com:636
      VELORAY_LDAP_CA_FILE: /etc/veloray/directory-ca.pem
    volumes:
      - ./directory-ca.pem:/etc/veloray/directory-ca.pem:ro
```

The container's UID 10001 must be able to read the certificate. Do not mount a
private key. Apply the override with the normal generated environment and base
Compose file described in `container-deployment.md`.

## Link an account

In **Users**, create or edit a local account and set **LDAP distinguished name**
to the exact full DN, for example:

```
uid=alice,ou=people,dc=example,dc=com
```

An Active Directory deployment can use the account's actual full DN, such as
`CN=Alice Doe,OU=People,DC=example,DC=com`, when that directory permits simple
bind over LDAPS. The adapter does not search by email, username, UPN or group.
Use the directory administrator's verified DN; do not derive it from a display
name. DNs are parsed and canonicalized, with a 1024-byte limit. Case-only
variants cannot be linked to multiple panel accounts. Each account can have
one LDAP binding in addition to its existing OIDC/GitHub identity binding.

The login page offers **Local account** and **LDAP directory** when enabled.
Choose LDAP and enter the **local panel username** and the **directory password**.
The local account must be active and have staff access. Existing roles, reseller
ownership, permissions, TOTP and one-use recovery codes remain authoritative.
A local password is still required for newly created panel accounts; keep it
independent from the directory password. Local login remains available during
directory outages. Switching the form's method clears its password.

Changing or removing a DN revokes the account's existing sessions through the
normal authentication version. Directory changes made outside VeloRay affect
new directory sign-ins; existing panel sessions last until their normal expiry
or local revocation. Bindings and authentication versions are rechecked under a
row lock after the network bind, so concurrent local changes cannot authenticate
against an obsolete account snapshot. No database row is locked while waiting
for the directory.

## API and limits

- `GET /api/auth/oidc` includes `ldap_enabled`; it exposes no address, DN or CA.
- `POST /api/auth/login`: `username`, `password`, optional `provider` (`local`
  by default, or `ldap`) and `totp_code`. Existing same-origin CSRF protections
  and IP/account rate limits apply. A `202` response requires retrying with the
  same method and credentials plus the local second factor.
- `POST /api/admin/users/` and `PATCH /api/admin/users/{id}/` accept optional
  `ldap_dn`. Empty removes it. Only administrators can manage these accounts;
  an already linked identity returns `409`.
- Directory connection, TLS handshake and bind share a seven-second deadline;
  request cancellation closes the socket. Total decoded LDAP input is bounded
  to 64 KiB. Empty or NUL-containing directory passwords are rejected before
  connecting. All authentication failures use generic credential errors.
- Successful `auth.login` audit entries record `provider=ldap` or `provider=local`,
  never the directory password or bind response.

Tests perform actual LDAP bind encoding over a local TLS server and exercise
certificate trust/hostname failures, invalid credentials, cancellation,
malformed/oversized responses, account isolation, concurrent binding changes,
local password preservation, recovery-code replay, migrations and browser flow.
This fixture is not an OpenLDAP or Active Directory installation. Verify the
actual directory's certificate, account DN, bind policy and lockout behavior in
operator staging before enabling production sign-in. Search/service-account
binds, provisioning, group sync, NTLM, Kerberos and LDAP password changes are not
implemented.

Release builds pin Go 1.26.10, including the standard-library certificate trust
fix GO-2026-6809. Both panel binaries and both proxy engines are rebuilt with
that compiler. Xray stays at v26.3.27; its source archive is SHA-256 pinned and
shipped alongside the rebuilt engine. The official release archive is used
only for checksum-verified GeoIP/Geosite data and documentation. A custom
`VELORAY_XRAY_VERSION` also requires the matching reviewed
`VELORAY_XRAY_SOURCE_SHA256`; `VELORAY_XRAY_SOURCE` can supply a local archive
with that exact digest.

Native panel and node upgrades install the verified bundled Xray even when an
engine executable already exists, so same-version compiler security fixes take
effect. Existing core configuration, installed GeoIP/Geosite assets, keys and accounting
state are preserved.
