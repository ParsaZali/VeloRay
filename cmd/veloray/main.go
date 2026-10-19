package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	hostpkg "github.com/PaarSaAm/VeloRay/internal/host"
	"github.com/PaarSaAm/VeloRay/internal/panel"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
)

func loadEnv(path string) error {
	raw, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	for n, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid environment line %d", n+1)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if len(value) > 1 && (value[0] == '"' && value[len(value)-1] == '"' || value[0] == '\'' && value[len(value)-1] == '\'') {
			value = value[1 : len(value)-1]
		}
		if _, set := os.LookupEnv(key); !set {
			if e = os.Setenv(key, value); e != nil {
				return e
			}
		}
	}
	return nil
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "VeloRay:", e)
		os.Exit(1)
	}
}
func run() error {
	args := os.Args[1:]
	command := "menu"
	if len(args) > 0 {
		command = args[0]
	}
	if command == "version" {
		fmt.Println(panel.Version)
		return nil
	}
	switch command {
	case "audit", "menu", "status", "doctor", "start", "stop", "restart", "logs", "url", "reset", "admin-reset", "domain", "port", "ssl", "update", "auto-update", "uninstall", "repair", "ports", "inbounds", "inbound-port", "backup-schedule", "language", "node":
		controller, err := exec.LookPath("velorayctl")
		if err != nil {
			return errors.New("management is available after installation; run veloray help for development commands")
		}
		if len(args) == 0 {
			args = []string{"menu"}
		}
		return syscall.Exec(controller, append([]string{"velorayctl"}, args...), os.Environ())
	}
	if command == "backup" && len(args) == 1 || command == "restore" {
		controller, err := exec.LookPath("velorayctl")
		if err != nil {
			return err
		}
		return syscall.Exec(controller, append([]string{"velorayctl"}, args...), os.Environ())
	}
	if command == "help" || command == "--help" {
		fmt.Println(`VeloRay 1.0

  veloray                         Open the management menu
  veloray status | doctor | url    Service health and panel address
  veloray start | stop | restart   Manage panel, agent and both cores
  veloray reset [USERNAME]         Change an administrator password
  veloray logs [web|agent|xray|singbox|subscriptions] Follow service logs
  veloray audit [LIMIT]            Read server audit events
  veloray backup [FILE]            Create a private recovery backup
  veloray restore FILE            Verify and restore a backup
  veloray domain HOST | port PORT  Configure the panel address
  veloray ssl EMAIL                Issue a trusted HTTPS certificate
  veloray repair                  Check and recover the Xray service
  veloray ports | inbounds         Inspect local listeners and saved inbounds
  veloray inbound-port ID PORT     Change an inbound port and update its links
  veloray update [DIRECTORY]       Install an update with a recovery backup
  veloray auto-update enable|disable|status|check|run
  veloray uninstall                Remove services and program files

Development: serve | migrate | bootstrap | bootstrap-local | reconcile | version
Run veloray serve explicitly to start the application in the foreground.`)
		return nil
	}
	env := os.Getenv("VELORAY_ENV_FILE")
	if env == "" {
		env = "/etc/veloray/veloray.env"
	}
	if e := loadEnv(env); e != nil {
		return e
	}
	c, e := panel.LoadConfig()
	if e != nil {
		return e
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if command == "serve" {
		e = panel.Serve(ctx, c)
		if errors.Is(e, http.ErrServerClosed) {
			return nil
		}
		return e
	}
	switch command {
	case "backup":
		if len(args) != 2 {
			return errors.New("usage: veloray backup FILE.tar.gz")
		}
		return hostpkg.Backup(ctx, "/", args[1], c.DatabaseURL, panel.Version)
	case "backup-verify":
		if len(args) != 2 {
			return errors.New("usage: veloray backup-verify FILE.tar.gz")
		}
		tmp, m, e := hostpkg.Verify(args[1])
		if e != nil {
			return e
		}
		defer os.RemoveAll(tmp)
		return json.NewEncoder(os.Stdout).Encode(m)
	case "restore-data":
		if len(args) != 2 {
			return errors.New("usage: veloray restore FILE.tar.gz (stop services first)")
		}
		return hostpkg.Restore(ctx, "/", args[1], c.DatabaseURL)
	case "set-env":
		if len(args) != 3 {
			return errors.New("usage: veloray set-env KEY VALUE")
		}
		return hostpkg.SetEnv(env, args[1], args[2])
	case "render-nginx":
		return hostpkg.RenderNginx(ctx, c.PublicURL, "/etc/nginx/sites-available/veloray")
	}
	store, e := panel.OpenStore(ctx, c.DatabaseURL)
	if e != nil {
		return e
	}
	defer store.Pool.Close()
	if e = store.Migrate(ctx); e != nil {
		return e
	}
	switch command {
	case "host-audit":
		if os.Geteuid() != 0 {
			return errors.New("audit requires root access")
		}
		limit := 100
		if len(args) > 2 {
			return errors.New("usage: veloray audit [LIMIT]")
		}
		if len(args) == 2 {
			limit, e = strconv.Atoi(args[1])
			if e != nil {
				return e
			}
		}
		events, e := (&panel.Server{Config: c, Store: store}).AuditEvents(ctx, limit)
		if e != nil {
			return e
		}
		encoder := json.NewEncoder(os.Stdout)
		for _, event := range events {
			if e = encoder.Encode(event); e != nil {
				return e
			}
		}
		return nil
	case "host-inbounds":
		if os.Geteuid() != 0 {
			return errors.New("run with sudo")
		}
		s := &panel.Server{Config: c, Store: store}
		out, err := s.LocalInbounds(ctx)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(out)
	case "host-inbound-port":
		if os.Geteuid() != 0 {
			return errors.New("run with sudo")
		}
		if len(args) != 3 {
			return errors.New("usage: veloray inbound-port ID PORT")
		}
		id, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil || id < 1 {
			return errors.New("invalid inbound ID")
		}
		port, err := strconv.Atoi(args[2])
		if err != nil || port < 1 || port > 65535 {
			return errors.New("port must be 1–65535")
		}
		s := &panel.Server{Config: c, Store: store}
		out, err := s.ChangeLocalInboundPort(ctx, id, port)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(out)
	case "configure-origin":
		if len(args) != 3 {
			return errors.New("usage: veloray configure-origin HOST PORT")
		}
		host, port := args[1], args[2]
		if strings.ContainsAny(host, "/\\?#@\r\n ") || host == "" {
			return errors.New("invalid host")
		}
		n, e := strconv.Atoi(port)
		if e != nil || n < 1 || n > 65535 || n == 8610 || n == 9191 || n == 10085 {
			return errors.New("invalid/reserved port")
		}
		var used bool
		if e = store.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vr_inbounds i JOIN vr_nodes n ON n.id=i.node_id WHERE n.data->>'is_local'='true' AND (i.data->>'port')::integer=$1)`, n).Scan(&used); e != nil {
			return e
		}
		if used {
			return errors.New("panel port is already used by a local inbound")
		}
		for k, v := range map[string]string{"VELORAY_PUBLIC_HOST": host, "VELORAY_PANEL_PORT": port, "VELORAY_PUBLIC_URL": "https://" + net.JoinHostPort(host, port)} {
			if e = hostpkg.SetEnv(env, k, v); e != nil {
				return e
			}
		}
		return nil
	case "migrate":
		fmt.Println("Database schema is current")
		return nil
	case "import-legacy":
		dry := len(args) > 1 && args[1] == "--dry-run"
		out, e := store.ImportLegacy(ctx, c, dry)
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(out)
	case "bootstrap", "reset-password":
		username := os.Getenv("VELORAY_ADMIN_USERNAME")
		if username == "" {
			username = "admin"
		}
		password := os.Getenv("VELORAY_ADMIN_PASSWORD")
		if password == "" {
			fmt.Fprint(os.Stderr, "Administrator password: ")
			raw, e := bufio.NewReader(io.LimitReader(os.Stdin, 4096)).ReadString('\n')
			if e != nil && e != io.EOF {
				return e
			}
			password = strings.TrimRight(string(raw), "\r\n")
		}
		if command == "bootstrap" {
			return store.Bootstrap(ctx, c, username, password)
		}
		return store.AdminReset(ctx, username, password)
	case "bootstrap-local":
		return store.BootstrapLocal(ctx, c, os.Getenv("VELORAY_PUBLIC_HOST"), os.Getenv("VELORAY_LOCAL_NODE_TOKEN"))
	case "reconcile":
		s := &panel.Server{Config: c, Store: store}
		_ = s.Handler()
		return s.Reconcile(ctx)
	}
	return fmt.Errorf("unknown command %q; run veloray help", command)
}
