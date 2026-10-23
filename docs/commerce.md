# Sales and accounting

Open **Services and plans → Sales & billing**. The existing sidebar and green/light/dark themes are retained. The public React store is `/shop?store=SELLER_ID`.

## Configure a store

1. Create a service containing the inbounds you will sell. Its defaults supply client policies; the sale offer supplies purchased time and volume.
2. Create an offer with an explicit currency, price, duration, traffic grant and optional unit cost. Publish it when ready.
3. Enable the seller's storefront and set its name, description, support link and optional HTTPS logo.
4. A superuser configures Zarinpal in **Payment gateway**. The public panel URL needs HTTPS. Merchant IDs are encrypted and never returned by a read API.

IRR means whole rials, IRT whole tomans, and USD/EUR are integer cents. Wallet currencies never mix. Zarinpal supports IRR/IRT; both request and verification use whole rials, with an explicit ×10 conversion for a toman invoice. USD/EUR offers can be purchased with matching wallet credit.

Sandbox payments are visibly marked as tests and **never** create real wallet credit or customer accounts. Only a successful provider verification of the persisted authority, merchant and amount marks a payment paid. A browser callback with `Status=OK` is insufficient. The invoice snapshots its amount and product grant; changing an offer does not change an outstanding invoice.

## Delivery and renewal

Paid orders use the same validated, journaled node deployment as customer management. Customer creation and the fulfilled order commit together. A failed deployment leaves a paid order with a retry status; the worker retries with backoff and the seller can retry from **Orders**. Repeated callbacks or fulfillment requests do not create duplicate credit or customers.

Renewal requires the customer's canonical subscription token and an offer for their existing seller/service. It extends from the later of now or the existing expiry and adds finite traffic to the existing allowance, preserving consumed traffic, credentials and the canonical subscription URL. A finite traffic top-up does not extend expiry. A zero-volume renewal offer explicitly grants unlimited volume. Resume a frozen subscription before buying a renewal.

The public order bearer is shown once and stored in the browser tab's session storage. Returning from the gateway in that browser can retrieve the private order and delivered subscription. Neither callback redirects nor audit logs carry this bearer. A private order cannot be recovered by guessing its public ID.

## Wallets and settlement

Resellers read their own balances, credit limit, orders and ledger. Only a superuser can record a deposit, correcting entry, debit, credit limit or settlement. Negative balances require an explicit credit limit. Every balance mutation locks the wallet and appends a ledger entry in the same PostgreSQL transaction. Database triggers reject ledger updates and deletes; correct an error with a new signed entry.

Adjustments require a unique reference. Reusing a reference with different details is rejected. Wallet purchases require a unique `reference`; repeat the identical request/reference after a lost response to avoid another debit. Currency, amount and product details are checked on replay.

Revenue reports exclude wallet top-ups and sandbox transactions, preserve currency boundaries and use the recorded sale price, snapshotted unit cost and operating expenses. A manual deposit records accounting credit; it does not represent an online provider payment. Actual gateway fees and refunds are not automatically reconciled in this version.

API contracts are included in `/api/openapi`. Financial records are retained in the PostgreSQL backup. Keep database and field-encryption keys together when restoring.
