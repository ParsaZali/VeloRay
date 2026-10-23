package panel

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"
)

// PostgreSQL session ownership is the fence: standby processes never start
// background jobs until they own this session-level advisory lock.
func (s *Server) Worker(ctx context.Context) { s.leadershipLoop(ctx, 5*time.Second, s.workerLoops) }
func (s *Server) leadershipLoop(ctx context.Context, interval time.Duration, work func(context.Context)) {
	host, _ := os.Hostname()
	id := host + ":" + randomToken(8)
	logger := s.Logger
	if logger == nil {
		logger = slog.Default()
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	heartbeat := func(leader bool) {
		c, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		_, _ = s.Store.Pool.Exec(c, `INSERT INTO vr_panel_instances(id,hostname,is_leader) VALUES($1,$2,$3) ON CONFLICT(id) DO UPDATE SET heartbeat_at=now(),is_leader=excluded.is_leader`, id, host, leader)
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = s.Store.Pool.Exec(c, `DELETE FROM vr_panel_instances WHERE id=$1`, id)
	}()
	for ctx.Err() == nil {
		heartbeat(false)
		conn, e := s.Store.Pool.Acquire(ctx)
		if e != nil {
			if ctx.Err() != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				continue
			}
		}
		var acquired bool
		e = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(842100,99)`).Scan(&acquired)
		if e != nil || !acquired {
			conn.Release()
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				continue
			}
		}
		logger.Info("background worker leadership acquired", "instance", id)
		child, cancel := context.WithCancel(ctx)
		done := make(chan struct{})
		go func() { defer close(done); work(child) }()
		leader := true
		heartbeat(true)
		for leader {
			select {
			case <-ctx.Done():
				leader = false
			case <-done:
				leader = false
			case <-ticker.C:
				probe, stop := context.WithTimeout(ctx, 2*time.Second)
				e = conn.Ping(probe)
				stop()
				if e != nil {
					leader = false
				} else {
					heartbeat(true)
				}
			}
		}
		cancel()
		<-done
		release, stop := context.WithTimeout(context.Background(), 2*time.Second)
		_, _ = conn.Exec(release, `SELECT pg_advisory_unlock(842100,99)`)
		stop()
		conn.Release()
		heartbeat(false)
		if ctx.Err() != nil {
			return
		}
		logger.Warn("background worker leadership released", "instance", id)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (s *Server) clusterAPI(r *http.Request, a Actor) (any, int, error) {
	if r.Method != "GET" {
		return nil, 0, fail(405, "method not allowed")
	}
	if !commerceAdmin(a) {
		return nil, 0, fail(403, "administrator access required")
	}
	rows, e := commerceRows(r.Context(), s.Store.Pool, `SELECT jsonb_build_object('id',id,'hostname',hostname,'started_at',started_at,'heartbeat_at',heartbeat_at,'role',CASE WHEN heartbeat_at<now()-interval '20 seconds' THEN 'offline' WHEN is_leader THEN 'active' ELSE 'standby' END) FROM vr_panel_instances WHERE heartbeat_at>now()-interval '1 day' ORDER BY is_leader DESC,started_at`)
	if e != nil {
		return nil, 0, e
	}
	active, standby := 0, 0
	for _, row := range rows {
		switch str(row, "role") {
		case "active":
			active++
		case "standby":
			standby++
		}
	}
	return Data{"instances": rows, "active": active, "standby": standby, "election": "postgresql-session-lock", "requirements": []any{"shared PostgreSQL with session affinity", "identical panel secrets and trusted public URL", "shared templates and TLS/proxy configuration", "external load balancer for HTTP failover"}, "summary": fmt.Sprintf("%d active, %d standby", active, standby)}, 200, nil
}
