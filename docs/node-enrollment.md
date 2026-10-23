# Single-use node enrollment

Open **Nodes → Connect new node** and enter the node name and its public host. The panel shows a command valid once for 15 minutes. Run it as root on the new Ubuntu 24.04 node. It uses the normal verified runtime installer and a random agent port.

After installation, the agent installer sends its HTTPS origin, generated bearer credential and certificate SHA-256 to the panel over trusted HTTPS. The panel matches the issued host, checks that the issuer remains an active administrator, pins the exact certificate, verifies agent authentication and node identity, and requires a running Xray core before consuming the token and recording the node. Failed verification leaves the token available until expiry. Concurrent attempts can register only one node.

Only token hashes are retained. Administrators can revoke unused commands. Keep installation commands private and generate another if expired. The long-lived agent bearer is encrypted at rest and is not printed after successful automatic enrollment.

The panel's own HTTPS certificate must be trusted by the node's operating system. For a private CA, set `VELORAY_ENROLL_CA_FILE` on the node to its trusted CA bundle. Enrollment never disables verification of the panel certificate. The node may use the generated self-signed certificate, because its exact leaf certificate is pinned during registration.

Firewall the agent port to the panel hosts. Certificate pinning authenticates the agent; it does not replace firewall policy. When intentionally rotating an agent certificate, update its SHA-256 through the node editor using a fingerprint obtained directly from the node, then probe it again. Expired or mismatched pinned certificates fail closed.
