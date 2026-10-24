-- Sales remains opt-in, independently of the owner management bot.
ALTER TABLE vr_wallets DROP CONSTRAINT IF EXISTS vr_wallets_currency_check;
ALTER TABLE vr_wallets ADD CONSTRAINT vr_wallets_currency_check CHECK(currency IN ('IRR','IRT','USD','EUR','XTR','USDT'));
ALTER TABLE vr_orders DROP CONSTRAINT IF EXISTS vr_orders_currency_check;
ALTER TABLE vr_orders ADD CONSTRAINT vr_orders_currency_check CHECK(currency IN ('IRR','IRT','USD','EUR','XTR','USDT'));
ALTER TABLE vr_cost_entries DROP CONSTRAINT IF EXISTS vr_cost_entries_currency_check;
ALTER TABLE vr_cost_entries ADD CONSTRAINT vr_cost_entries_currency_check CHECK(currency IN ('IRR','IRT','USD','EUR','XTR','USDT'));

CREATE TABLE IF NOT EXISTS vr_payment_proofs (
 id bigserial PRIMARY KEY, order_id text NOT NULL UNIQUE REFERENCES vr_orders(id),
 method text NOT NULL CHECK(method IN ('card','usdt')),
 state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','approved','rejected')),
 network text NOT NULL DEFAULT '', transaction_id text, evidence_ciphertext text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), reviewed_at timestamptz,
 reviewer_id bigint REFERENCES vr_users(id), review_note text NOT NULL DEFAULT '',
 UNIQUE(network,transaction_id)
);
CREATE INDEX IF NOT EXISTS vr_payment_proofs_pending ON vr_payment_proofs(id) WHERE state='pending';
ALTER TABLE vr_payment_proofs ADD COLUMN IF NOT EXISTS notification_state text NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS vr_bot_trials (
 owner_id bigint NOT NULL REFERENCES vr_users(id), user_id bigint NOT NULL CHECK(user_id>0),
 customer_id bigint REFERENCES vr_accounts(id) ON DELETE SET NULL,
 created_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(owner_id,user_id)
);
CREATE TABLE IF NOT EXISTS vr_bot_confirmations (
 id text PRIMARY KEY, owner_id bigint NOT NULL REFERENCES vr_users(id), user_id bigint NOT NULL,
 kind text NOT NULL CHECK(kind IN ('purchase','enable','disable','rotate')),
 target_id bigint NOT NULL, used boolean NOT NULL DEFAULT false,
 result_ciphertext text NOT NULL DEFAULT '', expires_at timestamptz NOT NULL DEFAULT now()+interval '5 minutes'
);
CREATE UNIQUE INDEX IF NOT EXISTS vr_orders_telegram_request ON vr_orders(owner_id,(snapshot->>'_telegram_user'),client_reference) WHERE client_reference IS NOT NULL AND payer_id IS NULL;
CREATE TABLE IF NOT EXISTS vr_bot_sessions (
 owner_id bigint NOT NULL REFERENCES vr_users(id), user_id bigint NOT NULL,
 state_ciphertext text NOT NULL, expires_at timestamptz NOT NULL DEFAULT now()+interval '30 minutes',
 PRIMARY KEY(owner_id,user_id)
);
CREATE TABLE IF NOT EXISTS vr_support_tickets (
 id bigserial PRIMARY KEY, owner_id bigint NOT NULL REFERENCES vr_users(id),
 user_id bigint NOT NULL CHECK(user_id>0), subject text NOT NULL,
 state text NOT NULL DEFAULT 'open' CHECK(state IN ('open','closed')),
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS vr_support_tickets_owner ON vr_support_tickets(owner_id,updated_at DESC);
CREATE TABLE IF NOT EXISTS vr_ticket_messages (
 id bigserial PRIMARY KEY, ticket_id bigint NOT NULL REFERENCES vr_support_tickets(id) ON DELETE CASCADE,
 author text NOT NULL CHECK(author IN ('customer','staff')), body_ciphertext text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), notified_at timestamptz
);
-- Retain invoice history while retiring the external provider and its credentials.
UPDATE vr_settings SET data=data-'_commerce_merchant'-'commerce_payments_enabled'-'commerce_sandbox' WHERE id=1;
INSERT INTO vr_schema(version) VALUES(19) ON CONFLICT DO NOTHING;
