CREATE TABLE IF NOT EXISTS vr_host_jobs (
 id bigserial PRIMARY KEY,
 kind text NOT NULL CHECK (kind IN ('panel_update','panel_port')),
 target text NOT NULL,
 previous text NOT NULL,
 state text NOT NULL DEFAULT 'queued' CHECK (state IN ('queued','running','succeeded','failed')),
 error text NOT NULL DEFAULT '',
 requested_by bigint,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS vr_host_jobs_one_active ON vr_host_jobs ((true)) WHERE state IN ('queued','running');
