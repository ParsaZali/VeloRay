# Sales and accounting

Open **Services and plans → Sales & billing**. The existing sidebar and green/light/dark themes are retained. The public React store is `/shop?store=SELLER_ID`.

## Configure a store

1. Create a service containing the inbounds you will sell. Its defaults supply client policies; the sale offer supplies purchased time and volume.
2. Create an offer with an explicit currency, price, duration, traffic grant and optional unit cost. Publish it when ready.
3. Enable the seller's storefront and set its name, description, support link and optional HTTPS logo.
4. Enable optional Stars, bank-transfer receipt or USDT methods under **Storefront**. **Payment gateway** currently displays `Soon...`; provider integrations are unavailable.

IRR means whole rials, IRT whole tomans, USD/EUR integer cents, XTR whole Stars, and USDT integer millionths. Wallet currencies never mix. Offers keep their explicit currency; VeloRay does not infer exchange rates. The panel price editor displays human-readable USDT while the API uses integer millionths.

Stars payments are validated against authenticated Telegram receipts. Bank and USDT transfers require explicit operator review under **Receipts**. Receipt submission alone does not mark an order paid. Full setup and review guidance: [Sales bot and payments](telegram-sales.md).

## Delivery and renewal

Paid orders use the same validated, journaled node deployment as customer management. Customer creation and the fulfilled order commit together. A failed deployment leaves a paid order with a retry status; the worker retries with backoff and the seller can retry from **Orders**. Repeated callbacks or fulfillment requests do not create duplicate credit or customers.

An unpaid invoice reserves its seller's client and traffic capacity for 30 minutes. Paid orders keep their reservation until delivery. Manual client creation, new child allocations and new invoices honor these holds; expiration or a failed wallet debit releases them. A delivery excludes its own hold when checking capacity. Quotas and device policies unsupported by a service protocol are rejected before checkout. A late verified payment is retained even if its old reservation expired; delivery may need an operator to restore capacity.

Renewal requires the customer's canonical subscription token and an offer for their existing seller/service. It extends from the later of now or the existing expiry and adds finite traffic to the existing allowance, preserving consumed traffic, credentials and the canonical subscription URL. A finite traffic top-up does not extend expiry. A zero-volume renewal offer explicitly grants unlimited volume. Resume a frozen subscription before buying a renewal.

The public order bearer is shown once and stored in the browser tab's session storage. That browser can retrieve the private order and delivered subscription. Verified Telegram buyers can recover their own order tokens in the Mini App. Audit logs do not carry these bearers. A private order cannot be recovered by guessing its public ID.

## Wallets and settlement

Resellers read their own balances, credit limit, orders and ledger. A parent may manage commerce for descendants; children cannot inspect parent or sibling records. Only a superuser can record a deposit, correcting entry, debit, credit limit or settlement. A parent may transfer its existing wallet credit to a direct child. Alternatively, a child can create a bank-transfer funding invoice owned by its parent, submit evidence, and receive credit only after parent review. Sellers cannot approve self-owned funding invoices to mint their own balance. Negative balances require an explicit credit limit. Every balance mutation locks the wallet and appends a ledger entry in the same PostgreSQL transaction. Database triggers reject ledger updates and deletes; correct an error with a new signed entry.

Adjustments require a unique reference. Reusing a reference with different details is rejected. Wallet purchases require a unique `reference`; repeat the identical request/reference after a lost response to avoid another debit. Currency, amount and product details are checked on replay.

Revenue reports exclude wallet top-ups and sandbox transactions, preserve currency boundaries and use the recorded sale price, snapshotted unit cost and operating expenses. A manual deposit records accounting credit; it does not represent an online provider payment. Actual gateway fees and refunds are not automatically reconciled in this version.

API contracts are included in `/api/openapi`. Financial records are retained in the PostgreSQL backup. Keep database and field-encryption keys together when restoring.

## Subresellers

Open **Team** to create a direct child, select its inbounds and reserve customer/traffic capacity. A blank initial password generates a strong password displayed once. An administrator enables delegation and sets the maximum number of direct children; nesting is bounded to eight parent links. Children always have reseller privileges, an immutable parent and inherited inbound scope.

A finite parent cannot grant unlimited capacity to a child. Child capacity is reserved from the parent, including inactive children, so the parent's own customers and invoices cannot spend it. Reducing a parent below existing allocations or removing an inbound still assigned to a child is rejected. Deactivating an ancestor invalidates descendant sessions. Financial history and child identities are retained; deactivate an account instead of deleting its history.

Credit transfers debit the actual parent's wallet and credit the direct child's wallet atomically. Repeating the same reference and payload is safe; concurrent transfers cannot spend the same balance twice. This does not grant children the ability to mint accounting credit or adjust credit limits.

## Subscription branding and domains

In **Storefront**, enable subscription branding to apply that seller's name, HTTPS logo and support link to its customers. This setting is independent of whether its public store is enabled. The simple light/dark subscription template remains the default.

For a custom origin such as `https://sub.example.com:100`, configure DNS and a trusted HTTPS reverse proxy to the VeloRay subscription listener. Save the origin, add the displayed `_veloray.sub.example.com` TXT record, then choose **Verify domain**. Verification checks the TXT proof, valid TLS, and a nonce-bound response proving this VeloRay instance serves that origin. It follows no redirects and does not contact private addresses. This workflow validates existing DNS/proxy/certificate setup; it does not provision them.

Until verification succeeds, generated links use the working panel/subscription origin. Successful verification changes only the origin: the configurable subscription path and private customer identifier remain intact. Changing or removing the origin clears its verification. A verified origin cannot be assigned to another seller. Keep the domain and certificate active for previously distributed links.
