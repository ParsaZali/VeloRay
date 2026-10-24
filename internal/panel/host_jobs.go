package panel

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"time"
)

func (s *Server) lastHostJob(ctx context.Context, kind string) (Data, error) {
	var raw []byte
	err := s.Store.Pool.QueryRow(ctx, `SELECT jsonb_build_object('id',id,'kind',kind,'target',target,'state',state,'error',error,'created_at',created_at,'updated_at',updated_at) FROM vr_host_jobs WHERE kind=$1 ORDER BY id DESC LIMIT 1`, kind).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return decode(raw)
}
func (s *Server) queueHostJob(ctx context.Context, a Actor, kind, target, previous string) (Data, error) {
	tx, err := s.Store.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var id int64
	err = tx.QueryRow(ctx, `INSERT INTO vr_host_jobs(kind,target,previous,requested_by) VALUES($1,$2,$3,$4) RETURNING id`, kind, target, previous, num(a.User, "id")).Scan(&id)
	var pgerr *pgconn.PgError
	if errors.As(err, &pgerr) && pgerr.Code == "23505" {
		return nil, fail(409, "Another server change is already queued or running.")
	}
	if err != nil {
		return nil, err
	}
	if err = audit(ctx, tx, num(a.User, "id"), "host."+kind+".request", strconv.FormatInt(id, 10), Data{"target": target}); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return Data{"id": id, "kind": kind, "target": target, "state": "queued", "error": ""}, nil
}
func (s *Server) validatePanelPort(ctx context.Context, port int64) error {
	if port == int64(s.Config.PanelPort) {
		return errors.New("Choose a different panel port.")
	}
	if err := validateSubscriptionPort(s.Config, port); err != nil {
		return err
	}
	if err := subscriptionPortAvailable(ctx, s.Store.Pool, port); err != nil {
		return err
	}
	cfg, err := settings(ctx, s.Store.Pool)
	if err != nil {
		return err
	}
	endpoint := obj(cfg, "_subscription_endpoint")
	if port == num(endpoint, "port") || port == num(endpoint, "applied_port") {
		return errors.New("This port is reserved by subscriptions.")
	}
	return nil
}
func (s *Server) panelEndpointAPI(w http.ResponseWriter, r *http.Request, a Actor) (any, int, error) {
	if !flag(a.User, "is_superuser") || a.Scope != "admin" {
		return nil, 0, fail(403, "administrator access required")
	}
	if r.Method == http.MethodGet {
		job, err := s.lastHostJob(r.Context(), "panel_port")
		return Data{"managed": s.nativeControlAvailable(), "port": s.Config.PanelPort, "origin": s.Config.PublicURL, "job": job}, 200, err
	}
	if r.Method != http.MethodPut {
		return nil, 0, fail(405, "method not allowed")
	}
	if !s.nativeControlAvailable() {
		return nil, 0, fail(409, "Panel port changes require the native VeloRay host control service.")
	}
	in, err := readBody(w, r)
	if err != nil {
		return nil, 0, err
	}
	if err = allowed(in, "port"); err != nil {
		return nil, 0, fail(400, err.Error())
	}
	if err = typed(in, Data{"port": 0}); err != nil {
		return nil, 0, fail(400, err.Error())
	}
	port := num(in, "port")
	if err = s.validatePanelPort(r.Context(), port); err != nil {
		return nil, 0, fail(400, err.Error())
	}
	job, err := s.queueHostJob(r.Context(), a, "panel_port", strconv.FormatInt(port, 10), s.Config.PublicURL)
	return job, 202, err
}

// Only the root-owned systemd unit executes this worker. The browser queues
// typed operations; it cannot choose a command, flags, source or download URL.
func (s *Server) RunNativeHostJob(ctx context.Context) error {
	if os.Geteuid() != 0 || s.Config.SubscriptionDriver != "native" {
		return errors.New("native host control requires the installed root service")
	}
	return s.runNativeHostJob(ctx, s.executeHostJob)
}

func (s *Server) runNativeHostJob(ctx context.Context, execute func(context.Context, string, string, string) error) error {
	conn, err := s.Store.Pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	var locked bool
	if err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(842119,0)`).Scan(&locked); err != nil || !locked {
		return err
	}
	defer conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock(842119,0)`)
	var id int64
	var kind, target, previous, state string
	err = conn.QueryRow(ctx, `SELECT id,kind,target,previous,state FROM vr_host_jobs WHERE state IN ('queued','running') ORDER BY id LIMIT 1`).Scan(&id, &kind, &target, &previous, &state)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	finish := func(problem error) error {
		status, detail := "succeeded", ""
		if problem != nil {
			status, detail = "failed", problem.Error()
		}
		finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_, e := conn.Exec(finishCtx, `UPDATE vr_host_jobs SET state=$1,error=$2,updated_at=now() WHERE id=$3`, status, detail, id)
		return e
	}
	if state == "running" {
		if kind == "panel_update" && installedCommit() == target || kind == "panel_port" && target == strconv.Itoa(s.Config.PanelPort) {
			return finish(nil)
		}
		return finish(errors.New("The operation was interrupted. Check server health before retrying."))
	}
	if _, err = conn.Exec(ctx, `UPDATE vr_host_jobs SET state='running',updated_at=now() WHERE id=$1`, id); err != nil {
		return err
	}
	operationCtx, cancel := context.WithTimeout(ctx, 85*time.Minute)
	defer cancel()
	return finish(execute(operationCtx, kind, target, previous))
}
func (s *Server) executeHostJob(ctx context.Context, kind, target, previous string) error {
	var args, extraEnv []string
	switch kind {
	case "panel_update":
		if !commitIDPattern.MatchString(target) || !commitIDPattern.MatchString(previous) {
			return errors.New("Invalid update revision.")
		}
		if installedCommit() == target {
			return nil
		}
		if installedCommit() != previous {
			return errors.New("The installed revision changed. Check for updates again.")
		}
		args = []string{"auto-update", "run"}
		extraEnv = []string{"VELORAY_EXPECTED_UPDATE_COMMIT=" + target}
	case "panel_port":
		port, err := strconv.ParseInt(target, 10, 32)
		if err != nil || s.Config.PublicURL != previous {
			return errors.New("The panel address changed. Refresh settings and try again.")
		}
		if err = s.validatePanelPort(ctx, port); err != nil {
			return err
		}
		listener, err := net.Listen("tcp", net.JoinHostPort("0.0.0.0", target))
		if err != nil {
			return errors.New("This port is occupied by another service.")
		}
		listener.Close()
		args = []string{"port", target}
	default:
		return errors.New("Unsupported server operation.")
	}
	if err := os.MkdirAll("/var/log/veloray", 0700); err != nil {
		return err
	}
	log, err := os.OpenFile("/var/log/veloray/host-control.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer log.Close()
	fmt.Fprintf(log, "\n%s %s target=%s\n", time.Now().UTC().Format(time.RFC3339), kind, target)
	cmd := exec.CommandContext(ctx, "/usr/local/bin/velorayctl", args...)
	cmd.Env = append(os.Environ(), extraEnv...)
	cmd.Stdout, cmd.Stderr = log, log
	if err = cmd.Run(); err != nil {
		return errors.New("The server operation failed. Review sudo journalctl -u veloray-host-control and the private host-control log.")
	}
	if kind == "panel_port" {
		port, _ := strconv.Atoi(target)
		healthCtx, stopHealth := context.WithTimeout(ctx, 10*time.Second)
		healthErr := s.checkNativeSubscriptionHTTPS(healthCtx, port)
		stopHealth()
		if healthErr != nil {
			rollbackCtx, stopRollback := context.WithTimeout(context.WithoutCancel(ctx), 45*time.Second)
			defer stopRollback()
			restore := exec.CommandContext(rollbackCtx, "/usr/local/bin/velorayctl", "port", strconv.Itoa(s.Config.PanelPort))
			restore.Stdout, restore.Stderr = log, log
			if restore.Run() != nil {
				return errors.New("Panel health check failed and the old port could not be restored. Run sudo veloray doctor on the server.")
			}
			return errors.New("Panel health check failed. The previous port was restored.")
		}
	}
	if kind == "panel_update" && installedCommit() != target {
		return errors.New("The update was not installed. GitHub verification or another update may have blocked it. Check again.")
	}
	return nil
}
