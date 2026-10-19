package panel

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Query interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}
type Store struct{ Pool *pgxpool.Pool }

var tables = map[string]string{"webhook_endpoints": "vr_webhook_endpoints", "subscription_policies": "vr_subscription_policies", "profiles": "vr_profiles", "snippets": "vr_snippets", "customers": "vr_accounts", "services": "vr_services", "nodes": "vr_nodes", "inbounds": "vr_inbounds", "clients": "vr_clients", "users": "vr_users", "api_keys": "vr_api_keys", "audit": "vr_audit", "outbounds": "vr_outbounds", "routing_rules": "vr_routing_rules"}

func OpenStore(ctx context.Context, dsn string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 12
	cfg.MinConns = 1
	cfg.MaxConnIdleTime = 5 * time.Minute
	p, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err = p.Ping(ctx); err != nil {
		p.Close()
		return nil, err
	}
	return &Store{p}, nil
}
func (s *Store) Migrate(ctx context.Context) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(842100,0)"); err != nil {
		return err
	}
	raw, err := migrations.ReadFile("migrations/001_initial.sql")
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, string(raw)); err != nil {
		return err
	}
	settings, _ := json.Marshal(defaultSettings())
	if _, err = tx.Exec(ctx, "INSERT INTO vr_settings(id,data) VALUES(1,$1) ON CONFLICT DO NOTHING", string(settings)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func get(ctx context.Context, q Query, kind string, id int64) (Data, error) {
	table, ok := tables[kind]
	if !ok {
		return nil, errors.New("unknown entity")
	}
	var raw []byte
	err := q.QueryRow(ctx, "SELECT data || jsonb_build_object('id',id) FROM "+table+" WHERE id=$1", id).Scan(&raw)
	if err != nil {
		return nil, err
	}
	d, err := decode(raw)
	if err == nil && kind == "clients" {
		err = hydrateClient(ctx, q, d, nil)
	}
	if err == nil && kind == "customers" {
		err = hydrateCustomer(ctx, q, d)
	}
	return d, err
}
func list(ctx context.Context, q Query, kind, where string, args ...any) ([]Data, error) {
	table, ok := tables[kind]
	if !ok {
		return nil, errors.New("unknown entity")
	}
	rows, err := q.Query(ctx, "SELECT data || jsonb_build_object('id',id) FROM "+table+" "+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Data{}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		d, e := decode(raw)
		if e != nil {
			return nil, e
		}
		out = append(out, d)
	}
	err = rows.Err()
	rows.Close()
	if err == nil && kind == "clients" {
		cache := map[int64]Data{}
		for _, d := range out {
			if err = hydrateClient(ctx, q, d, cache); err != nil {
				return nil, err
			}
		}
	}
	if err == nil && kind == "customers" {
		for _, d := range out {
			if err = hydrateCustomer(ctx, q, d); err != nil {
				break
			}
		}
	}
	return out, err
}
func save(ctx context.Context, q Query, kind string, d Data) error {
	table, ok := tables[kind]
	if !ok {
		return errors.New("unknown entity")
	}
	now := stamp()
	d["updated_at"] = now
	id := num(d, "id")
	if kind == "clients" && num(d, "_account") > 0 {
		shared := Data{}
		for _, k := range accountFields {
			if v, ok := d[k]; ok {
				shared[k] = v
			}
		}
		b, e := json.Marshal(shared)
		if e != nil {
			return e
		}
		tag, e := q.Exec(ctx, `UPDATE vr_accounts SET data=data || $1::jsonb WHERE id=$2`, string(b), num(d, "_account"))
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return errors.New("shared account no longer exists")
		}
	}
	if id == 0 {
		d["created_at"] = now
	}
	raw, err := json.Marshal(d)
	if err != nil {
		return err
	}
	if id == 0 {
		err = q.QueryRow(ctx, "INSERT INTO "+table+"(data) VALUES($1) RETURNING id", string(raw)).Scan(&id)
		if err != nil {
			return err
		}
		d["id"] = id
	} else {
		tag, e := q.Exec(ctx, "UPDATE "+table+" SET data=$1 WHERE id=$2", string(raw), id)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return pgx.ErrNoRows
		}
	}
	return nil
}

func hydrateClient(ctx context.Context, q Query, d Data, cache map[int64]Data) error {
	if id := num(d, "_account"); id > 0 {
		shared := cache[id]
		if shared == nil {
			var raw []byte
			if err := q.QueryRow(ctx, `SELECT data FROM vr_accounts WHERE id=$1`, id).Scan(&raw); err != nil {
				return err
			}
			var err error
			shared, err = decode(raw)
			if err != nil {
				return err
			}
			if cache != nil {
				cache[id] = shared
			}
		}
		for _, k := range accountFields {
			if v, ok := shared[k]; ok {
				d[k] = v
			}
		}
	}
	trafficDefaults(d)
	return nil
}
func remove(ctx context.Context, q Query, kind string, id int64) error {
	table, ok := tables[kind]
	if !ok {
		return errors.New("unknown entity")
	}
	tag, err := q.Exec(ctx, "DELETE FROM "+table+" WHERE id=$1", id)
	if err == nil && tag.RowsAffected() != 1 {
		return pgx.ErrNoRows
	}
	return err
}
func settings(ctx context.Context, q Query) (Data, error) {
	var raw []byte
	err := q.QueryRow(ctx, "SELECT data FROM vr_settings WHERE id=1").Scan(&raw)
	if err != nil {
		return nil, err
	}
	d, err := decode(raw)
	if err != nil {
		return nil, err
	}
	return merge(defaultSettings(), d), nil
}
func saveSettings(ctx context.Context, q Query, d Data) error {
	raw, err := json.Marshal(d)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, "UPDATE vr_settings SET data=$1 WHERE id=1", string(raw))
	return err
}
func lockNodes(ctx context.Context, tx pgx.Tx, ids ...int64) error {
	unique := map[int64]bool{}
	for _, id := range ids {
		if id > 0 {
			unique[id] = true
		}
	}
	ids = ids[:0]
	for id := range unique {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", int64(842100000000)+id); err != nil {
			return err
		}
	}
	return nil
}
func audit(ctx context.Context, q Query, actor int64, action, target string, detail Data) error {
	d := Data{"actor": nil, "actor_name": "", "action": action, "target": target, "ip_address": nil, "detail": detail}
	if actor > 0 {
		d["actor"] = actor
		if u, e := get(ctx, q, "users", actor); e == nil {
			d["actor_name"] = str(u, "username")
		}
	}
	if ip := str(detail, "ip"); ip != "" {
		d["ip_address"] = ip
	}
	if err := save(ctx, q, "audit", d); err != nil {
		return err
	}
	return enqueueAuditWebhook(ctx, q, d)
}
func databaseError(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23505":
			return errors.New("a record with this name, port or credential already exists")
		case "23503":
			return errors.New("the selected node, inbound or user no longer exists")
		}
	}
	return fmt.Errorf("database operation failed: %w", err)
}
