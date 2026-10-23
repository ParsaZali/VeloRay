CREATE TABLE IF NOT EXISTS vr_subscription_probe_tickets (
 ticket_hash text PRIMARY KEY,
 principal_key text NOT NULL,
 client_id bigint NOT NULL REFERENCES vr_clients(id) ON DELETE CASCADE,
 subscription_hash text NOT NULL,
 device_hash text NOT NULL,
 revision text NOT NULL,
 expires_at timestamptz NOT NULL DEFAULT now()+interval '2 minutes'
);
CREATE INDEX IF NOT EXISTS vr_subscription_probe_expiry ON vr_subscription_probe_tickets(expires_at);
CREATE TABLE IF NOT EXISTS vr_connection_quality (
 principal_key text NOT NULL,
 connection_key text NOT NULL,
 client_id bigint NOT NULL REFERENCES vr_clients(id) ON DELETE CASCADE,
 node_id bigint NOT NULL REFERENCES vr_nodes(id) ON DELETE CASCADE,
 data jsonb NOT NULL CHECK(jsonb_typeof(data)='object'),
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(principal_key,connection_key)
);
CREATE INDEX IF NOT EXISTS vr_connection_quality_expiry ON vr_connection_quality(updated_at);
INSERT INTO vr_schema(version) VALUES(18) ON CONFLICT DO NOTHING;
