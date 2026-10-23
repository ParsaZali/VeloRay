# Passkey authentication

Use a stable panel domain with trusted HTTPS, then open **Security → Passkeys**. Device PINs, biometrics and compatible security keys use the browser's WebAuthn interface. Synced passkeys and resident hardware credentials are supported. IP-only panel addresses do not enable this flow; localhost HTTP is allowed for development.

## Enroll and sign in

1. Sign in using the existing account method. Enter a clear key name, the account's local password, and a current authenticator/recovery code when TOTP is enabled.
2. Choose **Add passkey** and complete the device's verification prompt. Registration requires a discoverable credential and user verification. Up to 20 keys may belong to an account.
3. On the login page, select **Sign in with passkey** and choose the account/device. The browser identifies the account from the credential, so a username is not required.

Accounts using LDAP or SSO also need a working local password to enroll or revoke keys. A verified passkey login can satisfy the panel's required MFA policy; existing TOTP and password/SSO flows remain available. A password-only session cannot replace an already-enrolled passkey when that key is the account's existing security factor: sign in with it first. TOTP-enabled accounts additionally confirm a current code when managing keys.

The administrator can disable new sign-in in **Settings → Security & API → Enable passkeys**. Disabling it is rejected when required-MFA accounts depend exclusively on passkeys. Revocation from an already-authorized browser session remains available while passkey sign-in is disabled.

## Revocation and recovery

Enter the local password and applicable authenticator code, then remove a key from **Security**. Enrollment and revocation advance the account's session version and revoke its other browser sessions. Required MFA prevents removal of the last key until another passkey or TOTP is enrolled. Revocation removes the server's credential record; the user can also remove the device's local copy through its password manager.

If an administrator loses every passkey, recover through the installed host console:

```bash
sudo veloray passkeys-reset USERNAME
```

The root-only command requires typing `RESET`, revokes that administrator's keys and sessions, and retains the password, TOTP and required-MFA policy. Sign in with the remaining factors and enroll a replacement. The existing `veloray reset` changes the password; it does not silently delete security keys. TOTP recovery still requires a valid authenticator/recovery code.

## Verification and storage

VeloRay pins the relying-party ID to the panel hostname and validates the exact configured origin. Each two-minute challenge is one-use and browser-cookie bound; registration also binds the account, current browser session and authentication version. The server verifies the WebAuthn signature, challenge, relying party, origin, user presence and user verification. A signed response without the required UV flag is rejected.

Public credential records and counters are stored in PostgreSQL; private keys remain with the authenticator. Non-backed-up credentials with a decreasing/repeated nonzero signature counter are rejected. Synced credentials may legitimately have non-monotonic counters; their backup eligibility and counter warnings are recorded in login audit metadata. Existing inactive-account, reseller-ancestor and session-expiry checks apply.

Back up the database and panel secrets together. Both active/standby panel hosts need the same secrets and public origin. Changing the panel hostname generally requires registering credentials for the new domain; changing only the port retains the relying-party hostname but changes the verified origin for new ceremonies.

Verification includes generated ECDSA/CBOR authenticators exercising the real backend verifier, negative origin/challenge/RP/UV/signature/replay cases, revocation and administrator recovery, plus a Chromium virtual-authenticator browser flow for registration, discoverable login and removal. The implementation uses the pinned official [go-webauthn library](https://github.com/go-webauthn/webauthn/releases/tag/v0.18.2) and the [WebAuthn standard](https://www.w3.org/TR/webauthn-3/).
