# Outbound traffic and chains

`GET /api/outbound-traffic?node=1&core=xray&hours=24` returns independent
raw upload/download totals for each outbound tag, plus retained-period totals.
`core` is `xray` or `singbox`; `hours` is an integer from 1 to 8760 (default 24).
At most 1000 tags are returned. It requires both `nodes.read` and `traffic.read`,
and is unavailable to reseller/owner-restricted identities because it describes
all traffic on a node. Read-only administrator-owned API keys are supported.

The Outbounds page contains a collapsible traffic table, period selector and
refresh control. Closing it cancels the request. Changing node/core clears the
previous view. It shows collection time and an unavailable state for older agents.
Refreshing reads the most recent persisted sample; it does not contact a target
or force a live probe.

## Accounting contract

New agents annotate each durable counter with its engine (`core`) and advertise
`outbound_accounting: core-scoped-v1`. Existing customer metric names and cursors
are unchanged. Xray and sing-box outbound tags with the same name have separate
panel cursors and totals. Legacy outbound samples without an engine are ignored;
customer billing continues normally. Both engines must be upgraded for this view.
Apply the node configuration to enable sing-box outbound statistics for every
generated outbound, including profile and built-in entries.

Outbound counters **never consume customer quota**, nor are they added to the
existing billed node/owner traffic graphs. A multi-hop connection can be counted
at several outbounds; a selector and its selected outbound can also overlap.
They measure raw operational traffic, independently of customer multipliers.
Do not sum them as a billable network-transfer figure.

Totals begin with the first durable agent sample (which can include earlier
traffic retained by that agent). Subsequent samples add only the positive delta;
repeating a sample adds nothing. Decreased durable counters or changed ledgers
fail collection rather than guessing a reset. The cursor, totals, history and
customer usage commit in the same transaction. History uses five-minute buckets
and `traffic_history_days` retention. Tracked totals survive history pruning;
deleting the node removes them. Reusing a tag continues its totals. They are not
reset by a customer usage reset.

The idempotent migration adds `vr_outbound_traffic_totals` and
`vr_outbound_traffic_windows`. These contain only node, engine, tag, timestamps
and byte counts, without outbound configurations or credentials.

## Chaining and automatic selection

The outbound editor exposes Xray `streamSettings.sockopt.dialerProxy` and
sing-box `detour` through **Next hop (optional)**. It excludes the current tag,
and preserves unrelated transport settings. Legacy Xray `proxySettings.tag` is
recognized; explicitly changing the next hop replaces that legacy chain field.
sing-box direct outbounds do not support `detour`; the control is hidden there.

For sing-box selectors, select the default candidate. URL-test groups expose the
probe URL, interval, latency tolerance and idle timeout. Both expose explicit
connection interruption on switching, disabled unless selected. These are native
engine selection features, not panel-issued arbitrary URL fetches. A selector's
default is applied through configuration; changing it currently deploys the node.
Live selector switching and a panel health-history dashboard remain separate work.

After profiles and managed entries merge, the panel rejects duplicate tags,
missing next hops, empty/duplicate candidates, invalid selector defaults and
recursive chains, including cycles between selectors and proxy detours. A
referenced next hop cannot be deleted or disabled without updating its dependents.
Graphs are checked even before a node has its first sing-box inbound; such nodes
still do not start an unused sing-box service. Actual engine configuration
validation remains mandatory when that engine is deployed.

Checks cover transaction rollback, quota isolation, same-tag core separation,
sample idempotence, legacy compatibility, graph cycles, authorization, guided UI
controls, and configuration validation against the pinned runtimes. Real-engine
loopback tests additionally assert outbound upload/download counters for a VLESS
connection on both engines. Chained remote transport availability and production
failover still require a staging deployment with the operator's actual outbounds.

Independent specification references:

- https://sing-box.sagernet.org/configuration/outbound/urltest/
- https://sing-box.sagernet.org/configuration/outbound/selector/
- https://sing-box.sagernet.org/configuration/shared/dial/
- https://xtls.github.io/en/config/transport.html
