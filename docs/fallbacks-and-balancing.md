# Xray fallbacks and outbound balancing

## Inbound fallbacks

The inbound editor exposes **Fallback destinations** for Xray VLESS and Trojan,
RAW/TCP transport, with TLS or no stream security. REALITY, other transports,
other protocols and sing-box are excluded. Changing to an unsupported combination
removes the fallback field from the saved payload. Existing unrelated protocol
settings remain intact when editing VLESS/Trojan.

The normal inbound API accepts `protocol_settings.fallbacks`, at most 32 objects:

```json
[
  {"dest": "127.0.0.1:8080", "xver": 0},
  {"path": "/alternate", "dest": "127.0.0.1:8081", "xver": 0}
]
```

Optional `name` (SNI), `alpn` and `path` select a destination. Destinations accept
an integer port, a port string, host:port (IPv6 bracketed), absolute Unix socket
or Linux @abstract socket. HTTP URLs and Xray's special internal `serve` targets
are not part of this form/API. The overall default must have empty name, ALPN
and path. Every additional name/ALPN group must also have an empty-path default;
without one the pinned engine can accept the configuration but close requests.
Duplicate matches, malformed targets, invalid PROXY versions and h2/path
combinations are rejected before deployment. `xver` is 0 (off), 1 or 2; the
destination must understand PROXY protocol when enabled. Path matching is for
HTTP/1 traffic, not an HTTP/2 reverse proxy.

The core forwards unauthenticated traffic to the operator's destination; the
panel never fetches it. This grants no customer access and no authenticated
customer usage. Fallback web traffic has no customer identity and is not billed
to a customer's quota. It can contribute to host/network transfer. The web
server and its exposure remain operator-managed. Changes use existing inbound
permissions, validation, audit and compensated deployment. Subscription exports
do not include server fallback destinations.

## Balancers

Administrators can expand **Routing → Xray load balancing** on a selected node.
Create a group, choose the strategy, enter one outbound-tag prefix per line,
optionally select a fallback outbound, then **Save balancing & deploy**. Choose
the saved group as the destination of a routing rule. A group alone does not
change the default route. sing-box continues to use its native selector and
URL-test outbounds.

Xray selectors match **prefixes**, not exact membership. `edge-` matches both
`edge-a` and `edge-b`; a new matching outbound joins on the next deployment.
The form previews the currently matched tags. Random and round-robin distribute
connections and do not promise availability checks, connection retry, weighted
bandwidth or per-packet balancing. Least-ping and least-load require an
operator-configured `observatory` or `burstObservatory` in the node patch or
shared Xray profile/snippet. Those optional profile sections are now accepted.
Operators must configure observation selectors and probes for the selected
outbounds; the form does not create probes or claim tested production failover.
Native strategy settings already present in a group are preserved unless its
strategy is changed. Advanced observation behavior requires staging validation.

`GET /api/nodes/{id}/balancing` returns resolved balancers, sorted outbound tags,
an observatory-presence flag and a revision. Only administrator identities are
allowed; administrator-owned read-scoped keys can read. No outbound credentials
or complete configurations are returned.

`PUT` requires administrator scope and `{ "revision": "<current GET value>",
"balancers": [...] }`. At most 64 groups, 64 unique nonempty prefixes per group
and a 64 KiB request are allowed. The revision covers the resolved groups, tags
and observation availability. A stale view returns 409 without overwriting the
draft; reload to discard the draft and read current settings. The server reads
the fresh node under the existing deployment lock and updates only
`config_patch.routing.balancers`, preserving unrelated concurrent node settings.
Saving materializes inherited groups as a node override; an empty array explicitly
overrides inherited groups with none. Editing the shared profile does not replace
that node override. Remove the patch key through advanced node settings to resume
inheritance.

All final merged Xray routing references are checked after profile, node patch
and managed entries merge. Missing groups/outbounds, unmatched prefixes,
duplicates and conflicting outbound/balancer rule destinations fail atomically.
Deleting or disabling a referenced group/last matching outbound requires updating
its dependents first. sing-box top-level route references are likewise checked
against the merged configuration. Core validation still runs before application.
Deploy failure preserves database settings and compensates the agent. Changes
can restart the core and affect current connections. No schema migration, quota
reset, token rotation or version bump is required. These features remain opt-in;
nodes without fallbacks/balancers keep their existing behavior.

## Verification

Pinned Xray **26.3.27** tests deliver actual TLS HTTP/1 requests through generated
VLESS/Trojan fallback settings, including SNI/ALPN/path and default destinations.
Eight independent proxied requests through a round-robin group must reach its
two selected outbounds four times each. PostgreSQL integration tests cover
profile merging, revision conflicts, authorization, dependency protection,
deployment rollback and unchanged customer accounting. Browser tests cover
guided payloads, routing destinations, conflict-draft retention, unsupported
transport visibility and mobile width. sing-box **1.14.3** retains its existing
real-core regression suite; Xray balancers are not translated into sing-box.

Independent specification references (no upstream implementation code copied):

- https://xtls.github.io/en/config/features/fallback.html
- https://xtls.github.io/en/config/routing.html
- https://github.com/XTLS/Xray-core/tree/v26.3.27/infra/conf
