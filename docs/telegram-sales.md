# Sales bot, Mini App and owner management

The sales bot is **optional and off by default**. It uses the panel's existing Telegram token. Owner management remains available when a seller turns sales off. Customers never receive a panel identity, owner access or infrastructure permissions.

## Set up

1. Connect and check the bot in **Settings → Telegram**. Owner IDs are administrator allowlists; do not add buyers there.
2. Serve the panel and Mini App on a trusted HTTPS domain. Keep this token dedicated to VeloRay polling; another poller/webhook conflicts with it.
3. Create a service with the inbounds to sell, then publish offers in **Services and plans → Sales & billing → Offers**. Set categories, display order and optional product images.
4. In **Storefront**, enable the store and, separately, **Sales bot & Mini App**. Add purchase terms and support links. Share the generated `t.me/BOT?start=shop_SELLER` link.
5. Configure **Payment methods**. Publish an XTR offer for Stars, an IRT/IRR offer for bank transfer, or a USDT offer for Tether. Prices are separate; no exchange rate is guessed.

The same private bot presents the catalog, invoice confirmation, payment choices, receipt submission, service usage, trial, tutorials and support. Customers can also use the Mini App. Commands include `/buy`, `/services`, `/orders`, `/account`, `/free`, `/tutorial`, `/support`, `/ticket MESSAGE`, `/terms` and `/cancel`.

## Customization

Storefront settings include customer language, inline buttons or a persistent reply keyboard, message templates, HTTPS images, main-button text/visibility/row order, content/link buttons, device tutorials, optional service enable/disable and subscription-link rotation. A menu preview is available before saving. Templates accept `{store}` and `{user_id}`; leave a message blank to use its default. Media is referenced by an operator-supplied HTTPS URL. Telegram may reject media it cannot fetch.

Trials have a service, finite traffic grant, 1–168 hours, once-per-Telegram-user policy, optional existing-customer requirement and optional channel membership. Make the bot an administrator of that channel to check membership. Claiming again returns the original trial; deleting the subscription does not reset eligibility. Free-trial delivery and quota validation use the normal compensated deployment.

## Payments

- **Stars:** XTR offers use whole Stars, 1–10,000 per invoice. The server binds the bot, buyer, currency, amount and invoice. Only a matching `successful_payment` from Bot API polling proves payment. Pre-checkout acknowledgement, a browser callback or an invoice-closed event does not. Unique charge IDs prevent duplicate fulfillment.
- **Bank transfer:** set the receiving card and cardholder. The customer sends a JPG/PNG/WebP receipt photo up to 1 MiB in the bot or Mini App. A receipt remains unpaid until reviewed. The reviewer supplies the bank reference and confirms the invoice amount/currency.
- **USDT:** explicitly choose TRC20, ERC20 or BEP20 and its receiving address. The customer submits a transaction hash in the same bot or Mini App. USDT is stored in integer millionths (1 USDT = 1,000,000 units). VeloRay does **not** claim to verify the chain automatically. The operator checks token, network, recipient, amount and transaction finality before approval. Reused network/hash pairs are rejected.

**Receipts** contains private evidence and approve/reject actions. A rejection requires a reason. Submitted evidence is encrypted at rest and is excluded from list responses. Submission extends the existing inventory reservation to 24 hours for review. Approval uses the invoice's persisted amount and normal retryable delivery. A second approval cannot create another account. Existing financial records remain retained.

Telegram's published platform rules require Stars for digital sales inside Telegram. Operators remain responsible for platform compliance and the payment methods they enable. External provider gateways have been retired; **Payment gateway** displays `Soon...` and asks users to suggest providers through Telegram. Old provider callback URLs return unavailable and cannot mark an order paid.

## Customer and owner access

The Mini App validates raw Telegram `initData` using HMAC, duplicate-field rejection and a one-hour age bound. Buyer IDs are never trusted from JSON. Every service action checks the signed buyer's association with that seller's fulfilled order or trial. Link changes and enable/disable operations require explicit confirmation. The UI never exposes infrastructure credentials to a customer. IPs describe observed connections; registered devices describe subscription/HWID records, not a guaranteed physical-device inventory.

Customer support tickets are available in both bot and Mini App. Staff reply in **Support** or use `/reply TICKET_ID MESSAGE`; ticket bodies are encrypted. The owner management bot adds `/usage ID`, `/ips ID`, `/devices ID`, `/sales`, `/receipts` and `/tickets`. USDT approval prompts a one-use confirmation; bank approval requires `/receipt ID approve BANK_REFERENCE`. Verify receipt photos in the panel before approving. Changes require owner allowlisting, enabled bot management and no enforced administrator MFA; when MFA is required, use the authenticated panel.

Payment polling remains separate from slow node operations. Updates and private sales replies are encrypted and survive restart. Payment/account creation is idempotent; after a lost Telegram send acknowledgement a notification can repeat. Refunds, settlement, chargebacks and cash-out remain operator responsibilities. Tests use a mocked Bot API and do not charge a live account.

## API

Panel APIs use scoped API keys plus the linked user's permissions. Read-only keys cannot mutate data. See `/api/openapi` and [API integration](api-integrations.md) for order, receipt, support, device, IP and exact-byte usage endpoints.

Official references: [Stars](https://core.telegram.org/bots/payments-stars), [Mini App validation](https://core.telegram.org/bots/webapps#validating-data-received-via-the-mini-app), [Bot API](https://core.telegram.org/bots/api).
