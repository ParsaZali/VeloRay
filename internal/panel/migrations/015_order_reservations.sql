CREATE INDEX IF NOT EXISTS vr_orders_capacity ON vr_orders(owner_id,state,expires_at) WHERE state IN ('pending','paid');
INSERT INTO vr_schema(version) VALUES(15) ON CONFLICT DO NOTHING;
