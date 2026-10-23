-- Integer amounts: IRR/IRT are whole rials/tomans; USD/EUR are cents.
CREATE TABLE IF NOT EXISTS vr_stores (
 owner_id bigint PRIMARY KEY REFERENCES vr_users(id), data jsonb NOT NULL CHECK(jsonb_typeof(data)='object')
);
CREATE TABLE IF NOT EXISTS vr_offers (
 id bigserial PRIMARY KEY, owner_id bigint NOT NULL REFERENCES vr_users(id),
 data jsonb NOT NULL CHECK(jsonb_typeof(data)='object')
);
CREATE INDEX IF NOT EXISTS vr_offers_owner ON vr_offers(owner_id,id);
CREATE TABLE IF NOT EXISTS vr_wallets (
 user_id bigint NOT NULL REFERENCES vr_users(id), currency text NOT NULL CHECK(currency IN ('IRR','IRT','USD','EUR')),
 balance bigint NOT NULL DEFAULT 0, credit_limit bigint NOT NULL DEFAULT 0 CHECK(credit_limit BETWEEN 0 AND 1000000000000),
 updated_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(user_id,currency),
 CHECK(balance BETWEEN -credit_limit AND 1000000000000)
);
CREATE TABLE IF NOT EXISTS vr_ledger (
 id bigserial PRIMARY KEY, user_id bigint NOT NULL REFERENCES vr_users(id), currency text NOT NULL,
 amount bigint NOT NULL CHECK(amount<>0 AND amount BETWEEN -1000000000000 AND 1000000000000),
 balance_after bigint NOT NULL, reference text NOT NULL UNIQUE, note text NOT NULL DEFAULT '',
 actor_id bigint REFERENCES vr_users(id), created_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY(user_id,currency) REFERENCES vr_wallets(user_id,currency)
);
CREATE INDEX IF NOT EXISTS vr_ledger_owner ON vr_ledger(user_id,id DESC);
CREATE OR REPLACE FUNCTION vr_ledger_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Financial ledger is append-only; create a correcting entry'; END $$;
DROP TRIGGER IF EXISTS vr_ledger_immutable ON vr_ledger;
CREATE TRIGGER vr_ledger_immutable BEFORE UPDATE OR DELETE ON vr_ledger FOR EACH ROW EXECUTE FUNCTION vr_ledger_immutable();
CREATE TABLE IF NOT EXISTS vr_orders (
 id text PRIMARY KEY CHECK(length(id) BETWEEN 20 AND 80),
 owner_id bigint NOT NULL REFERENCES vr_users(id), payer_id bigint REFERENCES vr_users(id),
 offer_id bigint REFERENCES vr_offers(id), customer_id bigint REFERENCES vr_accounts(id) ON DELETE SET NULL,
 kind text NOT NULL CHECK(kind IN ('purchase','renewal','traffic','topup')),
 amount bigint NOT NULL CHECK(amount BETWEEN 1 AND 1000000000000),
 currency text NOT NULL CHECK(currency IN ('IRR','IRT','USD','EUR')),
 state text NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','paid','fulfilled','cancelled')),
 gateway text NOT NULL DEFAULT '', authority text, payment_reference text, token_hash text NOT NULL,
 snapshot jsonb NOT NULL CHECK(jsonb_typeof(snapshot)='object'),
 attempts integer NOT NULL DEFAULT 0, next_attempt_at timestamptz NOT NULL DEFAULT now(), last_error text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(), expires_at timestamptz NOT NULL DEFAULT now()+interval '30 minutes',
 paid_at timestamptz, fulfilled_at timestamptz,
 UNIQUE(gateway,authority), UNIQUE(gateway,payment_reference)
);
CREATE INDEX IF NOT EXISTS vr_orders_owner ON vr_orders(owner_id,created_at DESC);
CREATE INDEX IF NOT EXISTS vr_orders_pending ON vr_orders(next_attempt_at) WHERE state='paid';
ALTER TABLE vr_orders ADD COLUMN IF NOT EXISTS client_reference text;
CREATE UNIQUE INDEX IF NOT EXISTS vr_orders_request ON vr_orders(payer_id,client_reference) WHERE client_reference IS NOT NULL;
CREATE TABLE IF NOT EXISTS vr_cost_entries (
 id bigserial PRIMARY KEY, owner_id bigint REFERENCES vr_users(id), node_id bigint REFERENCES vr_nodes(id) ON DELETE SET NULL,
 amount bigint NOT NULL CHECK(amount BETWEEN 1 AND 1000000000000), currency text NOT NULL CHECK(currency IN ('IRR','IRT','USD','EUR')),
 note text NOT NULL, reference text NOT NULL UNIQUE, incurred_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO vr_schema(version) VALUES(11) ON CONFLICT DO NOTHING;
