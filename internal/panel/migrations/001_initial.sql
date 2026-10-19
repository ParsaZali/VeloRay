CREATE TABLE IF NOT EXISTS vr_schema (version integer PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS vr_nodes (id bigserial PRIMARY KEY, data jsonb NOT NULL CHECK (jsonb_typeof(data)='object'));
CREATE UNIQUE INDEX IF NOT EXISTS vr_nodes_name ON vr_nodes ((data->>'name'));
CREATE UNIQUE INDEX IF NOT EXISTS vr_one_local_node ON vr_nodes ((data->>'is_local')) WHERE data->>'is_local'='true';
CREATE TABLE IF NOT EXISTS vr_inbounds (
 id bigserial PRIMARY KEY, data jsonb NOT NULL CHECK (jsonb_typeof(data)='object'),
 node_id bigint GENERATED ALWAYS AS ((data->>'node')::bigint) STORED NOT NULL REFERENCES vr_nodes(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS vr_inbounds_node ON vr_inbounds(node_id);
CREATE UNIQUE INDEX IF NOT EXISTS vr_inbounds_port ON vr_inbounds(node_id, ((data->>'port')::integer)) WHERE (data->>'port')::integer > 0;
CREATE TABLE IF NOT EXISTS vr_clients (
 id bigserial PRIMARY KEY, data jsonb NOT NULL CHECK (jsonb_typeof(data)='object'),
 inbound_id bigint GENERATED ALWAYS AS ((data->>'inbound')::bigint) STORED NOT NULL REFERENCES vr_inbounds(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS vr_clients_inbound ON vr_clients(inbound_id);
CREATE UNIQUE INDEX IF NOT EXISTS vr_clients_name ON vr_clients(inbound_id, (data->>'name'));
CREATE UNIQUE INDEX IF NOT EXISTS vr_subscription_token ON vr_clients ((data->>'subscription_token'));
CREATE TABLE IF NOT EXISTS vr_users (id bigserial PRIMARY KEY, data jsonb NOT NULL CHECK (jsonb_typeof(data)='object'));
CREATE UNIQUE INDEX IF NOT EXISTS vr_user_name ON vr_users ((data->>'username'));
CREATE TABLE IF NOT EXISTS vr_api_keys (
 id bigserial PRIMARY KEY, data jsonb NOT NULL,
 user_id bigint GENERATED ALWAYS AS ((data->>'user')::bigint) STORED NOT NULL REFERENCES vr_users(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS vr_api_key_hash ON vr_api_keys ((data->>'_token_hash'));
CREATE TABLE IF NOT EXISTS vr_settings (id integer PRIMARY KEY CHECK(id=1), data jsonb NOT NULL);
CREATE TABLE IF NOT EXISTS vr_audit (id bigserial PRIMARY KEY, data jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now());
CREATE INDEX IF NOT EXISTS vr_audit_time ON vr_audit(created_at DESC);
CREATE TABLE IF NOT EXISTS vr_sessions (
 token_hash text PRIMARY KEY, user_id bigint NOT NULL REFERENCES vr_users(id) ON DELETE CASCADE,
 auth_version bigint NOT NULL, second_factor boolean NOT NULL DEFAULT false, expires_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS vr_sessions_expiry ON vr_sessions(expires_at);
CREATE TABLE IF NOT EXISTS vr_stats_cursors (
 node_id bigint NOT NULL REFERENCES vr_nodes(id) ON DELETE CASCADE, metric text NOT NULL,
 total bigint NOT NULL CHECK(total>=0), PRIMARY KEY(node_id, metric)
);
CREATE TABLE IF NOT EXISTS vr_traffic_windows (
 at timestamptz NOT NULL, node_id bigint NOT NULL, bytes bigint NOT NULL CHECK(bytes>=0), PRIMARY KEY(at,node_id)
);
CREATE INDEX IF NOT EXISTS vr_traffic_at ON vr_traffic_windows(at);
CREATE TABLE IF NOT EXISTS vr_jobs (
 node_id bigint PRIMARY KEY REFERENCES vr_nodes(id) ON DELETE CASCADE, attempts integer NOT NULL DEFAULT 0,
 next_at timestamptz NOT NULL DEFAULT now(), last_error text NOT NULL DEFAULT '', rollback_ops jsonb NOT NULL DEFAULT '[]',
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS vr_rate_limits (key text PRIMARY KEY, start_at timestamptz NOT NULL, hits integer NOT NULL);
CREATE INDEX IF NOT EXISTS vr_rate_limits_time ON vr_rate_limits(start_at);
CREATE TABLE IF NOT EXISTS vr_telegram_updates (id bigint PRIMARY KEY, processed_at timestamptz NOT NULL DEFAULT now());
INSERT INTO vr_schema(version) VALUES(1) ON CONFLICT DO NOTHING;
CREATE TABLE IF NOT EXISTS vr_operations (id text PRIMARY KEY, nodes jsonb NOT NULL, status text NOT NULL DEFAULT 'prepared', created_at timestamptz NOT NULL DEFAULT now());
CREATE INDEX IF NOT EXISTS vr_operations_pending ON vr_operations(created_at) WHERE status='prepared';
CREATE TABLE IF NOT EXISTS vr_accounts (id bigserial PRIMARY KEY, data jsonb NOT NULL,
 node_id bigint GENERATED ALWAYS AS ((data->>'_node')::bigint) STORED NOT NULL REFERENCES vr_nodes(id) ON DELETE CASCADE);
ALTER TABLE vr_clients ADD COLUMN IF NOT EXISTS account_id bigint GENERATED ALWAYS AS ((data->>'_account')::bigint) STORED REFERENCES vr_accounts(id);
CREATE INDEX IF NOT EXISTS vr_clients_account ON vr_clients(account_id) WHERE account_id IS NOT NULL;
CREATE TABLE IF NOT EXISTS vr_client_devices (
 id bigserial PRIMARY KEY,
 client_id bigint REFERENCES vr_clients(id) ON DELETE CASCADE,
 account_id bigint REFERENCES vr_accounts(id) ON DELETE CASCADE,
 hwid text NOT NULL CHECK(length(hwid) BETWEEN 1 AND 256),
 data jsonb NOT NULL CHECK(jsonb_typeof(data)='object'),
 first_seen_at timestamptz NOT NULL DEFAULT now(),
 last_seen_at timestamptz NOT NULL DEFAULT now(),
 CHECK((client_id IS NULL) <> (account_id IS NULL)),
 UNIQUE(client_id,hwid), UNIQUE(account_id,hwid)
);
CREATE TABLE IF NOT EXISTS vr_import_jobs (
 id text PRIMARY KEY, actor_id bigint NOT NULL REFERENCES vr_users(id) ON DELETE CASCADE,
 node_id bigint NOT NULL REFERENCES vr_nodes(id) ON DELETE CASCADE, digest text NOT NULL,
 payload text NOT NULL, preview jsonb NOT NULL, result jsonb,
 expires_at timestamptz NOT NULL DEFAULT now()+interval '30 minutes', created_at timestamptz NOT NULL DEFAULT now());
CREATE INDEX IF NOT EXISTS vr_import_expiry ON vr_import_jobs(expires_at);
INSERT INTO vr_schema(version) VALUES(2) ON CONFLICT DO NOTHING;
CREATE TABLE IF NOT EXISTS vr_telegram_actions (
 id text PRIMARY KEY, owner_id text NOT NULL, kind text NOT NULL, target_id bigint NOT NULL, action text NOT NULL,
 expires_at timestamptz NOT NULL DEFAULT now()+interval '5 minutes', used boolean NOT NULL DEFAULT false);
ALTER TABLE vr_telegram_updates ADD COLUMN IF NOT EXISTS reply jsonb;
ALTER TABLE vr_telegram_updates ADD COLUMN IF NOT EXISTS sent_at timestamptz;

CREATE TABLE IF NOT EXISTS vr_owner_traffic_windows (at timestamptz NOT NULL, owner_id bigint NOT NULL, bytes bigint NOT NULL CHECK(bytes>=0), PRIMARY KEY(at,owner_id));
CREATE INDEX IF NOT EXISTS vr_owner_traffic_lookup ON vr_owner_traffic_windows(owner_id,at);

CREATE TABLE IF NOT EXISTS vr_outbounds (id BIGSERIAL PRIMARY KEY, data JSONB NOT NULL, node_id BIGINT GENERATED ALWAYS AS ((data->>'node')::bigint) STORED REFERENCES vr_nodes(id));
CREATE TABLE IF NOT EXISTS vr_routing_rules (id BIGSERIAL PRIMARY KEY, data JSONB NOT NULL, node_id BIGINT GENERATED ALWAYS AS ((data->>'node')::bigint) STORED REFERENCES vr_nodes(id));
CREATE UNIQUE INDEX IF NOT EXISTS vr_outbounds_tag ON vr_outbounds (node_id,(data->>'core'),(data->>'tag'));

ALTER TABLE vr_traffic_windows ADD COLUMN IF NOT EXISTS upload_bytes bigint NOT NULL DEFAULT 0 CHECK(upload_bytes>=0);
ALTER TABLE vr_traffic_windows ADD COLUMN IF NOT EXISTS download_bytes bigint NOT NULL DEFAULT 0 CHECK(download_bytes>=0);
ALTER TABLE vr_owner_traffic_windows ADD COLUMN IF NOT EXISTS upload_bytes bigint NOT NULL DEFAULT 0 CHECK(upload_bytes>=0);
ALTER TABLE vr_owner_traffic_windows ADD COLUMN IF NOT EXISTS download_bytes bigint NOT NULL DEFAULT 0 CHECK(download_bytes>=0);
INSERT INTO vr_schema(version) VALUES(3) ON CONFLICT DO NOTHING;

-- Subscription history is scoped to a stable billed principal: account:<id>
-- for a shared account, client:<id> otherwise. Never store a subscription token.
CREATE TABLE IF NOT EXISTS vr_client_traffic_windows (
 at timestamptz NOT NULL,
 principal_key text NOT NULL CHECK (length(principal_key) <= 80),
 bytes bigint NOT NULL DEFAULT 0 CHECK(bytes>=0),
 upload_bytes bigint NOT NULL DEFAULT 0 CHECK(upload_bytes>=0),
 download_bytes bigint NOT NULL DEFAULT 0 CHECK(download_bytes>=0),
 PRIMARY KEY(principal_key,at)
);
CREATE INDEX IF NOT EXISTS vr_client_traffic_recent ON vr_client_traffic_windows(at);
INSERT INTO vr_schema(version) VALUES(5) ON CONFLICT DO NOTHING;

-- Durable, authenticated event delivery; safe to apply on existing installs.
CREATE TABLE IF NOT EXISTS vr_webhook_deliveries (
 id bigserial PRIMARY KEY,
 payload jsonb NOT NULL CHECK (jsonb_typeof(payload)='object'),
 attempts integer NOT NULL DEFAULT 0,
 next_at timestamptz NOT NULL DEFAULT now(),
 last_error text NOT NULL DEFAULT '',
 delivered_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS vr_webhook_ready ON vr_webhook_deliveries(next_at, id)
 WHERE delivered_at IS NULL AND attempts < 8;

-- Native accounts have no anchor node. Legacy imported anchors remain intact.
ALTER TABLE vr_accounts ALTER COLUMN node_id DROP NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS vr_account_token ON vr_accounts ((data->>'subscription_token'));
CREATE TABLE IF NOT EXISTS vr_services (id bigserial PRIMARY KEY, data jsonb NOT NULL CHECK(jsonb_typeof(data)='object'));
INSERT INTO vr_schema(version) VALUES(6) ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS vr_profiles (id bigserial PRIMARY KEY, data jsonb NOT NULL CHECK(jsonb_typeof(data)='object'));
CREATE TABLE IF NOT EXISTS vr_snippets (id bigserial PRIMARY KEY, data jsonb NOT NULL CHECK(jsonb_typeof(data)='object'));
CREATE TABLE IF NOT EXISTS vr_config_revisions (
 kind text NOT NULL CHECK(kind IN ('profiles','snippets')), entity_id bigint NOT NULL,
 revision bigint NOT NULL CHECK(revision>0), data jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(kind,entity_id,revision)
);

CREATE TABLE IF NOT EXISTS vr_subscription_requests (
 id bigserial PRIMARY KEY, principal_key text NOT NULL CHECK(length(principal_key)<=80),
 at timestamptz NOT NULL DEFAULT now(), format text NOT NULL, client_app text NOT NULL, status integer NOT NULL CHECK(status BETWEEN 100 AND 599)
);
CREATE INDEX IF NOT EXISTS vr_subscription_request_principal ON vr_subscription_requests(principal_key,id DESC);
CREATE INDEX IF NOT EXISTS vr_subscription_request_retention ON vr_subscription_requests(at);

CREATE INDEX IF NOT EXISTS vr_customer_services ON vr_accounts USING gin ((data->'service_ids'));
CREATE INDEX IF NOT EXISTS vr_service_inbounds ON vr_services USING gin ((data->'inbound_ids'));
CREATE INDEX IF NOT EXISTS vr_customer_owner ON vr_accounts (((data->>'owner_id')::bigint));
CREATE INDEX IF NOT EXISTS vr_node_profile ON vr_nodes ((COALESCE((data->>'profile_id')::bigint,0)));

-- Engine-scoped outbound counters are operational telemetry, never billable usage.
CREATE TABLE IF NOT EXISTS vr_outbound_traffic_totals (
 node_id bigint NOT NULL REFERENCES vr_nodes(id) ON DELETE CASCADE,
 core text NOT NULL CHECK(core IN ('xray','singbox')), tag text NOT NULL CHECK(length(tag) BETWEEN 1 AND 256),
 upload_bytes bigint NOT NULL DEFAULT 0 CHECK(upload_bytes>=0),
 download_bytes bigint NOT NULL DEFAULT 0 CHECK(download_bytes>=0),
 updated_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(node_id,core,tag)
);
CREATE TABLE IF NOT EXISTS vr_outbound_traffic_windows (
 at timestamptz NOT NULL, node_id bigint NOT NULL REFERENCES vr_nodes(id) ON DELETE CASCADE,
 core text NOT NULL CHECK(core IN ('xray','singbox')), tag text NOT NULL CHECK(length(tag) BETWEEN 1 AND 256),
 upload_bytes bigint NOT NULL DEFAULT 0 CHECK(upload_bytes>=0),
 download_bytes bigint NOT NULL DEFAULT 0 CHECK(download_bytes>=0), PRIMARY KEY(at,node_id,core,tag)
);
CREATE INDEX IF NOT EXISTS vr_outbound_traffic_recent ON vr_outbound_traffic_windows(node_id,core,at);
INSERT INTO vr_schema(version) VALUES(7) ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS vr_subscription_policies (id bigserial PRIMARY KEY, data jsonb NOT NULL CHECK(jsonb_typeof(data)='object'));

CREATE TABLE IF NOT EXISTS vr_webhook_endpoints(id bigserial PRIMARY KEY,data jsonb NOT NULL CHECK(jsonb_typeof(data)='object'));
ALTER TABLE vr_webhook_deliveries ADD COLUMN IF NOT EXISTS endpoint_id bigint REFERENCES vr_webhook_endpoints(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS vr_webhook_endpoint_pending ON vr_webhook_deliveries(endpoint_id,next_at) WHERE delivered_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS vr_user_oidc_identity ON vr_users((data->>'oidc_issuer'),(data->>'oidc_subject')) WHERE COALESCE(data->>'oidc_subject','')<>'';
CREATE TABLE IF NOT EXISTS vr_oidc_states(state_hash text PRIMARY KEY,browser_hash text NOT NULL,body text NOT NULL,expires_at timestamptz NOT NULL);
CREATE INDEX IF NOT EXISTS vr_oidc_state_expiry ON vr_oidc_states(expires_at);
CREATE TABLE IF NOT EXISTS vr_oidc_challenges(token_hash text PRIMARY KEY,user_id bigint NOT NULL REFERENCES vr_users(id) ON DELETE CASCADE,auth_version bigint NOT NULL,expires_at timestamptz NOT NULL);
CREATE INDEX IF NOT EXISTS vr_oidc_challenge_expiry ON vr_oidc_challenges(expires_at);
INSERT INTO vr_schema(version) VALUES(8) ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS vr_alert_states(principal text NOT NULL,event text NOT NULL,fingerprint text NOT NULL,updated_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(principal,event));

CREATE TABLE IF NOT EXISTS vr_node_billing(node_id bigint PRIMARY KEY REFERENCES vr_nodes(id) ON DELETE CASCADE,cycle_start timestamptz NOT NULL,cycle_end timestamptz NOT NULL,used_bytes bigint NOT NULL DEFAULT 0 CHECK(used_bytes>=0),lifetime_bytes bigint NOT NULL DEFAULT 0 CHECK(lifetime_bytes>=0),warned boolean NOT NULL DEFAULT false);
CREATE TABLE IF NOT EXISTS vr_node_billing_periods(node_id bigint NOT NULL REFERENCES vr_nodes(id) ON DELETE CASCADE,cycle_start timestamptz NOT NULL,cycle_end timestamptz NOT NULL,used_bytes bigint NOT NULL CHECK(used_bytes>=0),PRIMARY KEY(node_id,cycle_start));

CREATE TABLE IF NOT EXISTS vr_monitors(id bigserial PRIMARY KEY,data jsonb NOT NULL CHECK(jsonb_typeof(data)='object'),node_id bigint GENERATED ALWAYS AS ((data->>'node')::bigint) STORED NOT NULL REFERENCES vr_nodes(id) ON DELETE CASCADE,next_at timestamptz NOT NULL DEFAULT now(),lease_until timestamptz NOT NULL DEFAULT '-infinity');
CREATE INDEX IF NOT EXISTS vr_monitors_next ON vr_monitors(next_at);
CREATE TABLE IF NOT EXISTS vr_monitor_samples(id bigserial PRIMARY KEY,monitor_id bigint NOT NULL REFERENCES vr_monitors(id) ON DELETE CASCADE,at timestamptz NOT NULL DEFAULT now(),result jsonb NOT NULL);
CREATE INDEX IF NOT EXISTS vr_monitor_history ON vr_monitor_samples(monitor_id,at DESC);
INSERT INTO vr_schema(version) VALUES(9) ON CONFLICT DO NOTHING;
