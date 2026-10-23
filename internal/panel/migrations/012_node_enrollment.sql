CREATE TABLE IF NOT EXISTS vr_node_enrollments (
 id text PRIMARY KEY, token_hash text NOT NULL UNIQUE,
 actor_id bigint NOT NULL REFERENCES vr_users(id) ON DELETE CASCADE,
 data jsonb NOT NULL CHECK(jsonb_typeof(data)='object'),
 created_at timestamptz NOT NULL DEFAULT now(), expires_at timestamptz NOT NULL,
 used_at timestamptz, revoked_at timestamptz, node_id bigint REFERENCES vr_nodes(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS vr_node_enrollments_expiry ON vr_node_enrollments(expires_at);
INSERT INTO vr_schema(version) VALUES(12) ON CONFLICT DO NOTHING;
