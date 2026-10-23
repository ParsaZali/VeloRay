CREATE TABLE IF NOT EXISTS vr_store_domains (
 origin text PRIMARY KEY, owner_id bigint NOT NULL UNIQUE REFERENCES vr_users(id),
 verified_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO vr_schema(version) VALUES(14) ON CONFLICT DO NOTHING;
