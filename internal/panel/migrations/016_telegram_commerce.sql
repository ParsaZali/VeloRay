ALTER TABLE vr_wallets DROP CONSTRAINT IF EXISTS vr_wallets_currency_check;
ALTER TABLE vr_wallets ADD CONSTRAINT vr_wallets_currency_check CHECK(currency IN ('IRR','IRT','USD','EUR','XTR','USDT'));
ALTER TABLE vr_orders DROP CONSTRAINT IF EXISTS vr_orders_currency_check;
ALTER TABLE vr_orders ADD CONSTRAINT vr_orders_currency_check CHECK(currency IN ('IRR','IRT','USD','EUR','XTR','USDT'));
ALTER TABLE vr_cost_entries DROP CONSTRAINT IF EXISTS vr_cost_entries_currency_check;
ALTER TABLE vr_cost_entries ADD CONSTRAINT vr_cost_entries_currency_check CHECK(currency IN ('IRR','IRT','USD','EUR','XTR','USDT'));
CREATE TABLE IF NOT EXISTS vr_telegram_customers (
 user_id bigint NOT NULL CHECK(user_id>0), owner_id bigint NOT NULL REFERENCES vr_users(id),
 last_seen_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(user_id,owner_id)
);
CREATE TABLE IF NOT EXISTS vr_telegram_orders (
 order_id text PRIMARY KEY REFERENCES vr_orders(id), user_id bigint NOT NULL CHECK(user_id>0),
 token_ciphertext text NOT NULL, notified_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS vr_telegram_orders_user ON vr_telegram_orders(user_id,created_at DESC);
CREATE TABLE IF NOT EXISTS vr_telegram_inbox (
 bot_id text NOT NULL, id bigint NOT NULL, kind text NOT NULL CHECK(kind IN ('message','checkout','receipt')),
 input_ciphertext text NOT NULL, received_at timestamptz NOT NULL DEFAULT now(), processed_at timestamptz
 ,PRIMARY KEY(bot_id,id)
);
CREATE INDEX IF NOT EXISTS vr_telegram_inbox_pending ON vr_telegram_inbox(kind,id) WHERE processed_at IS NULL;
ALTER TABLE vr_telegram_updates ADD COLUMN IF NOT EXISTS bot_id text NOT NULL DEFAULT 'legacy';
ALTER TABLE vr_telegram_updates DROP CONSTRAINT IF EXISTS vr_telegram_updates_pkey;
ALTER TABLE vr_telegram_updates ADD PRIMARY KEY(bot_id,id);
INSERT INTO vr_schema(version) VALUES(16) ON CONFLICT DO NOTHING;
