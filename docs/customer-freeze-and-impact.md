# Customer freeze and change review

The unified **Clients** page can freeze an active shared subscription with a finite expiry for 1–30 days, within a 30-day annual allowance. The panel first disables the customer's deployed connections and then stores their remaining time. It preserves credentials, usage and the canonical subscription token. Generic enable/expiry/renewal edits cannot bypass this saved state.

Resume manually or let the worker resume automatically. If the panel was offline beyond the freeze deadline, the expiry is calculated from that original deadline, so an outage does not extend the freeze indefinitely. Resuming persists the deadline and queues node deployment even when the node is temporarily unreachable. The quota-renewal clock shifts by the actual bounded pause. Expired or exhausted accounts remain subject to their normal policy.

Editing an existing service shows a review before applying: affected customers/nodes, added/removed connections and preserved connections. Existing credentials and counters remain unchanged; editing service defaults does not reapply them to customers. The preview includes a revision. A changed service or customer membership makes that revision stale and the write returns 409 for another review.

Endpoints: `POST /api/customers/{id}/freeze` with `{"days":7}`, `POST /api/customers/{id}/resume` with `{}`, and `POST /api/services/{id}/impact` with the proposed service fields. Send the preview's `revision` as `expected_revision` on the service PATCH. Scope and deployment compensation are the same as ordinary customer/service management.
