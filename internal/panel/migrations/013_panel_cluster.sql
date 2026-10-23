CREATE TABLE IF NOT EXISTS vr_panel_instances (
 id text PRIMARY KEY, hostname text NOT NULL, started_at timestamptz NOT NULL DEFAULT now(),
 heartbeat_at timestamptz NOT NULL DEFAULT now(), is_leader boolean NOT NULL DEFAULT false
);
INSERT INTO vr_schema(version) VALUES(13) ON CONFLICT DO NOTHING;
