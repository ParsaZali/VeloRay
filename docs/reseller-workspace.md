# Reseller workspace

Resellers have four navigation entries: Dashboard, Clients, Sales & wallet and
Security. Empty infrastructure groups are omitted. The original sidebar sizing,
green accent, light/dark themes and bottom VeloRay branding are retained.

The preset allows management of the reseller's own users, traffic, storefront,
orders and wallet. Subreseller writes additionally require enabled delegation and
a positive child limit. Resellers cannot access inbound, outbound, routing, node,
core or global settings management. API authorization enforces this independently
of the browser. The same browser route policy filters navigation and blocks direct
URLs before their components mount; it also handles refreshed permissions and
older sessions containing infrastructure grants.

User creation and subreseller delegation use `GET /api/clients/inbound-options`.
It returns only assigned connection labels and protocol/client capabilities,
without addresses, ports, keys, file paths, stream settings or global client
counts. The management `/api/inbounds` routes require their explicit permissions;
`clients.read` no longer implicitly grants inbound access.

The reseller dashboard counts each shared subscription once and shows own users,
active users, allocations and expiry reminders. Remaining capacity subtracts both
assigned user quotas and reserved capacity, including pending orders and child
resellers. Zero limits mean unlimited; unknown server metrics are not displayed
as zero-valued server health for resellers.

The Customers screen is removed. Clients now groups shared subscriptions into one
row and manages existing connections, quotas, subscriptions and freeze controls.
The compatibility account APIs and paid-order data remain in place. No database
deletion or token rotation occurs during upgrade.
