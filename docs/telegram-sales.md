# Telegram sales and Mini App

The customer bot shares the existing panel bot token but has separate customer routes and payment processing. Telegram users never receive a panel identity or staff permissions. The same immutable offers, inventory reservations and paid-order fulfillment serve web and Telegram purchases.

## Setup

1. Configure and test the bot in **Settings → Telegram**, then enable it. Keep its token dedicated to this panel's polling service; an existing webhook or another poller conflicts with it.
2. Serve the public panel/store URL over trusted HTTPS. Configure the Mini App domain for your bot through BotFather if required by the Telegram client.
3. In **Sales & billing → Storefront**, provide HTTPS purchase terms and support links and enable **Telegram store**.
4. Publish an offer in **Telegram Stars (XTR)**, priced at 1–10,000 whole Stars. Price and cost entries retain their explicit currency; there is no automatic conversion to tomans or wallet currencies.
5. Share the generated `t.me/BOT?start=shop_SELLER` link. A customer starts that private chat, opens the Mini App, accepts the terms and pays the Stars invoice.

Digital purchases inside Telegram use Stars. The regular web store shows IRR/IRT offers for Zarinpal; matching-currency staff wallet purchases can also use USD/EUR/XTR. A Stars accounting balance is panel credit denominated in Stars, not an automated transfer of a bot's actual Stars balance. All online Stars payments reach the configured bot; seller settlement and cash-out remain operator responsibilities.

## Verification and delivery

The server validates raw Mini App `initData` with Telegram's HMAC algorithm, a one-hour age bound, duplicate-field rejection and a positive user ID. Checkout also requires that customer to have started the selected store's private bot chat. Browser-submitted user IDs, a callback, or an `invoiceClosed` status never prove payment.

Invoices bind the order, bot, buyer, currency and exact integer amount. A short pre-checkout check confirms the pending invoice and reserved capacity. Only a matching `successful_payment` receipt from authenticated Bot API polling records a Stars payment. Its unique charge identifier is retained; replaying it cannot create another order or delivery. Recurring invoices are not enabled.

Payment polling and acknowledgements run independently of node operations and ordinary bot replies. Provider updates enter an encrypted PostgreSQL inbox before its polling offset advances. Pending messages survive process restart. Bot IDs namespace updates, preventing collisions when an administrator switches bots. Finish outstanding payments before replacing a production bot; the panel retains earlier invoices and receipts.

Paid orders use the normal retryable node deployment. Once fulfilled, the bot sends a private order link; its bearer is encrypted at rest and carried in a URL fragment, which the browser imports into tab storage and removes from the address. `/orders` shows only that buyer's recent orders for the selected store. Delivery notification retries may repeat the same private link after a lost send acknowledgement; payment and account creation stay idempotent.

Customers use `/shop`, `/orders`, `/terms` and `/paysupport`. Refunds, chargebacks and Telegram cash-out are handled by the operator using the retained charge IDs and Telegram's official tools/APIs; this release does not automatically revoke a subscription or reconcile a refunded payment. Test requests in the automated suite use a mocked Bot API and never charge a live Telegram account.

## Official references

- [Telegram digital goods and Stars](https://core.telegram.org/bots/payments-stars)
- [Mini App data validation](https://core.telegram.org/bots/webapps#validating-data-received-via-the-mini-app)
- [Bot API invoices and payment receipts](https://core.telegram.org/bots/api#payments)
