CREATE TABLE IF NOT EXISTS vr_passkeys (
 id bytea PRIMARY KEY, user_id bigint NOT NULL REFERENCES vr_users(id) ON DELETE CASCADE,
 label text NOT NULL, credential jsonb NOT NULL CHECK(jsonb_typeof(credential)='object'),
 created_at timestamptz NOT NULL DEFAULT now(), last_used_at timestamptz
);
CREATE INDEX IF NOT EXISTS vr_passkeys_user ON vr_passkeys(user_id,created_at);
CREATE TABLE IF NOT EXISTS vr_passkey_challenges (
 token_hash text PRIMARY KEY, purpose text NOT NULL CHECK(purpose IN ('login','register')),
 user_id bigint REFERENCES vr_users(id) ON DELETE CASCADE, auth_version bigint NOT NULL DEFAULT 0,
 session_hash text NOT NULL DEFAULT '', label text NOT NULL DEFAULT '',
 data jsonb NOT NULL CHECK(jsonb_typeof(data)='object'),
 expires_at timestamptz NOT NULL DEFAULT now()+interval '2 minutes'
);
CREATE INDEX IF NOT EXISTS vr_passkey_challenges_expiry ON vr_passkey_challenges(expires_at);
INSERT INTO vr_schema(version) VALUES(17) ON CONFLICT DO NOTHING;
