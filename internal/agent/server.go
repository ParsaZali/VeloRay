package agent

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"github.com/PaarSaAm/VeloRay/internal/envfiles"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Server struct {
	Updater                *CoreUpdater
	NodeID, Version, Token string
	Manager                *Manager
	SingBox                *Manager
	Native                 *Manager
}

func (s Server) Handler() http.Handler {
	mux := http.NewServeMux()
	var coreMu sync.Mutex
	if s.Updater != nil {
		s.Updater.changeMu = &coreMu
	}
	var geo geoCatalog
	validate := func(c context.Context, raw []byte) error {
		var policy struct {
			Required bool `json:"_singbox_user_ips_required"`
		}
		if json.Unmarshal(raw, &policy) != nil {
			return errors.New("invalid configuration")
		}
		if policy.Required && s.Updater != nil && !s.Updater.SupportsUserIPs() {
			return errors.New("per-user IP limits require the bundled sing-box core")
		}

		x, b, e := splitCore(raw)
		if e != nil {
			return e
		}
		if e = s.Manager.Validate(c, x); e != nil {
			return e
		}
		if n, err := nativePart(raw); err != nil {
			return err
		} else if len(n) > 0 {
			if s.Native == nil {
				return errors.New("upgrade the node agent for native protocols")
			}
			if err = s.Native.Validate(c, n); err != nil {
				return err
			}
		}
		if len(b) > 0 {
			if s.SingBox == nil {
				return errors.New("sing-box is not installed on this node")
			}
			return s.SingBox.Validate(c, b)
		}
		return nil
	}
	write := func(w http.ResponseWriter, code int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(v)
	}
	wrap := func(fn func(context.Context, map[string]json.RawMessage) (any, error), method string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			got := r.Header.Get("Authorization")
			want := "Bearer " + s.Token
			if len(got) != len(want) || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
				write(w, 401, map[string]string{"detail": "unauthorized"})
				return
			}
			if r.Method != method {
				w.Header().Set("Allow", method)
				write(w, 405, map[string]string{"detail": "method not allowed"})
				return
			}
			v := map[string]json.RawMessage{}
			if method == "POST" {
				r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
				if e := json.NewDecoder(r.Body).Decode(&v); e != nil {
					write(w, 400, map[string]string{"detail": "invalid JSON body"})
					return
				}
			}
			ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
			defer cancel()
			out, e := fn(ctx, v)
			if e != nil {
				write(w, 503, map[string]string{"detail": e.Error()})
				return
			}
			write(w, 200, out)
		}
	}
	mux.HandleFunc("/cores/status", wrap(func(c context.Context, v map[string]json.RawMessage) (any, error) {
		if s.Updater == nil {
			return map[string]any{"managed": false, "enabled": false, "reason": "Core updates require managed systemd installation; update container images instead."}, nil
		}
		return s.Updater.Status(), nil
	}, "GET"))
	mux.HandleFunc("/cores/catalog", wrap(func(c context.Context, v map[string]json.RawMessage) (any, error) {
		if s.Updater == nil {
			return nil, errors.New("core manager unavailable")
		}
		var core string
		_ = json.Unmarshal(v["core"], &core)
		return s.Updater.Catalog(c, core)
	}, "POST"))
	mux.HandleFunc("/cores/policy", wrap(func(c context.Context, v map[string]json.RawMessage) (any, error) {
		if s.Updater == nil {
			return nil, errors.New("core manager unavailable")
		}
		var enabled bool
		if json.Unmarshal(v["enabled"], &enabled) != nil {
			return nil, errors.New("enabled is required")
		}
		err := s.Updater.SetEnabled(enabled)
		return s.Updater.Status(), err
	}, "POST"))
	mux.HandleFunc("/cores/install", wrap(func(c context.Context, v map[string]json.RawMessage) (any, error) {
		if s.Updater == nil {
			return nil, errors.New("core manager unavailable")
		}
		var core, tag string
		var rollback, required bool
		_ = json.Unmarshal(v["core"], &core)
		_ = json.Unmarshal(v["tag"], &tag)
		_ = json.Unmarshal(v["rollback"], &rollback)
		_ = json.Unmarshal(v["require_user_ips"], &required)
		coreMu.Lock()
		err := s.Updater.Start(core, tag, rollback, required)
		coreMu.Unlock()
		return s.Updater.Status(), err
	}, "POST"))
	mux.HandleFunc("/geo/status", wrap(func(c context.Context, _ map[string]json.RawMessage) (any, error) {
		if s.Updater == nil {
			return map[string]any{"managed": false, "enabled": false}, nil
		}
		return s.Updater.GeoStatus(), nil
	}, "GET"))
	mux.HandleFunc("/geo/catalog", wrap(func(c context.Context, v map[string]json.RawMessage) (any, error) {
		if s.Updater == nil {
			return nil, errors.New("geo manager unavailable")
		}
		var kind string
		if json.Unmarshal(v["kind"], &kind) != nil {
			return nil, errors.New("geo kind is required")
		}
		return s.Updater.GeoCatalog(c, kind)
	}, "POST"))
	mux.HandleFunc("/geo/install", wrap(func(c context.Context, v map[string]json.RawMessage) (any, error) {
		if s.Updater == nil {
			return nil, errors.New("geo manager unavailable")
		}
		var kind, tag string
		var rollback bool
		if json.Unmarshal(v["kind"], &kind) != nil {
			return nil, errors.New("geo kind is required")
		}
		if value, ok := v["tag"]; ok && json.Unmarshal(value, &tag) != nil {
			return nil, errors.New("invalid geo release tag")
		}
		if value, ok := v["rollback"]; ok && json.Unmarshal(value, &rollback) != nil {
			return nil, errors.New("rollback must be boolean")
		}
		coreMu.Lock()
		err := s.Updater.StartGeo(kind, tag, rollback)
		coreMu.Unlock()
		return s.Updater.GeoStatus(), err
	}, "POST"))
	mux.HandleFunc("/xray/geo-assets", wrap(func(c context.Context, v map[string]json.RawMessage) (any, error) {
		var kind, query string
		limit := 100
		if json.Unmarshal(v["kind"], &kind) != nil {
			return nil, errors.New("asset kind required")
		}
		if raw, ok := v["query"]; ok && json.Unmarshal(raw, &query) != nil {
			return nil, errors.New("invalid search")
		}
		if raw, ok := v["limit"]; ok && json.Unmarshal(raw, &limit) != nil {
			return nil, errors.New("invalid limit")
		}
		return geo.browse(c, kind, query, limit)
	}, "POST"))
	mux.HandleFunc("/native/status", wrap(func(c context.Context, _ map[string]json.RawMessage) (any, error) {
		if s.Native == nil {
			return nil, errors.New("native runtime unavailable; upgrade the node agent")
		}
		return map[string]any{"version": 1, "protocols": []string{"mtproto", "amneziawg"}, "per_user_traffic": true, "hot_users": true, "client_probes": true}, nil
	}, "GET"))
	mux.HandleFunc("/diagnostics", wrap(func(c context.Context, _ map[string]json.RawMessage) (any, error) {
		result := map[string]any{"xray": s.Manager.Diagnostics(c)}
		if s.SingBox != nil {
			result["singbox"] = s.SingBox.Diagnostics(c)
		}
		if s.Native != nil {
			result["native"] = s.Native.Diagnostics(c)
		}
		return result, nil
	}, "GET"))
	mux.HandleFunc("/clients/probe", wrap(func(c context.Context, v map[string]json.RawMessage) (any, error) {
		var core, target string
		if json.Unmarshal(v["core"], &core) != nil || json.Unmarshal(v["url"], &target) != nil {
			return nil, errors.New("client core and target URL required")
		}
		if core == "native" {
			if s.Native == nil {
				return nil, errors.New("native diagnostic runtime unavailable; upgrade this agent")
			}
			return s.Native.ProbeNativeClient(c, v["config"], target)
		}
		manager := s.Manager
		if core == "singbox" {
			manager = s.SingBox
		} else if core != "xray" {
			return nil, errors.New("unsupported client probe core")
		}
		if manager == nil {
			return nil, errors.New("client probe core unavailable")
		}
		return manager.ProbeClient(c, v["config"], target)
	}, "POST"))
	mux.HandleFunc("/outbounds/probe", wrap(func(c context.Context, v map[string]json.RawMessage) (any, error) {
		var core, tag, target string
		_ = json.Unmarshal(v["core"], &core)
		_ = json.Unmarshal(v["tag"], &tag)
		_ = json.Unmarshal(v["url"], &target)
		manager := s.Manager
		if core == "singbox" {
			manager = s.SingBox
		} else if core != "xray" {
			return nil, errors.New("invalid core")
		}
		if manager == nil {
			return nil, errors.New("core is unavailable")
		}
		return manager.ProbeOutbound(c, tag, target)
	}, "POST"))
	mux.HandleFunc("/health", wrap(func(c context.Context, _ map[string]json.RawMessage) (any, error) {
		st := s.Manager.Status(c)
		status := "ok"
		if !st.Running {
			status = "degraded"
		}
		return map[string]any{"status": status, "node_id": s.NodeID, "version": s.Version, "time": time.Now().UTC(), "xray": st, "singbox": func() any {
			if s.SingBox != nil {
				return s.SingBox.Status(c)
			}
			return nil
		}()}, nil
	}, "GET"))
	mux.HandleFunc("/xray/status", wrap(func(c context.Context, _ map[string]json.RawMessage) (any, error) { return s.Manager.Status(c), nil }, "GET"))
	mux.HandleFunc("/xray/stats", wrap(func(c context.Context, _ map[string]json.RawMessage) (any, error) {
		a, e := s.Manager.Stats(c)
		if e != nil {
			return a, e
		}
		if s.SingBox != nil {
			b, e := s.SingBox.Stats(c)
			if e != nil {
				return a, e
			}
			a.CoreLedgers = map[string]string{"singbox": b.LedgerID}
			a.Stat = append(a.Stat, b.Stat...)

			a.OnlineUsers = append(a.OnlineUsers, b.OnlineUsers...)
			a.OnlineIPCoreAvailable["singbox"] = b.OnlineIPsAvailable
			a.OnlineIPsAvailable = a.OnlineIPsAvailable && b.OnlineIPsAvailable
			for name, ips := range b.OnlineIPs {
				a.OnlineIPs[name] = ips
			}
		}
		if s.Native != nil {
			b, e := s.Native.Stats(c)
			if e != nil {
				return a, e
			}
			if a.CoreLedgers == nil {
				a.CoreLedgers = map[string]string{}
			}
			a.CoreLedgers["native"] = b.LedgerID
			a.Stat = append(a.Stat, b.Stat...)
			a.OnlineUsers = append(a.OnlineUsers, b.OnlineUsers...)
			a.OnlineIPCoreAvailable["native"] = b.OnlineIPsAvailable
			a.OnlineIPsAvailable = a.OnlineIPsAvailable && b.OnlineIPsAvailable
			for name, ips := range b.OnlineIPs {
				a.OnlineIPs[name] = ips
			}
		}
		return a, nil
	}, "GET"))
	mux.HandleFunc("/xray/ports", wrap(func(c context.Context, v map[string]json.RawMessage) (any, error) {
		x, b, err := splitCore(v["config"])
		if err != nil {
			return nil, err
		}
		checks, err := s.Manager.Ports(c, x)
		if err == nil && len(b) > 0 && s.SingBox != nil {
			other, e := s.SingBox.Ports(c, b)
			err = e
			checks = append(checks, other...)
		}
		if err == nil {
			n, e := nativePart(v["config"])
			err = e
			if err == nil && len(n) > 0 {
				if s.Native == nil {
					err = errors.New("upgrade the node agent for native protocols")
				} else {
					more, e := s.Native.Ports(c, n)
					err = e
					checks = append(checks, more...)
				}
			}
		}
		return map[string]any{"ports": checks}, err
	}, "POST"))
	mux.HandleFunc("/xray/validate", wrap(func(c context.Context, v map[string]json.RawMessage) (any, error) {
		return map[string]string{"status": "valid"}, validate(c, v["config"])
	}, "POST"))
	mux.HandleFunc("/xray/apply", wrap(func(c context.Context, v map[string]json.RawMessage) (any, error) {
		var id string
		_ = json.Unmarshal(v["operation_id"], &id)
		coreMu.Lock()
		defer coreMu.Unlock()
		if e := validate(c, v["config"]); e != nil {
			return nil, e
		}
		x, b, e := splitCore(v["config"])
		if e != nil {
			return nil, e
		}
		if e = s.Manager.Apply(c, id, x); e != nil {
			return nil, e
		}
		if len(b) > 0 {
			if e = s.SingBox.Apply(c, id, b); e != nil {
				if re := s.Manager.Rollback(c, id); re != nil {
					return nil, errors.Join(e, re)
				}
				return nil, e
			}
		}
		n, _ := nativePart(v["config"])
		if len(n) > 0 {
			if e = s.Native.Apply(c, id, n); e != nil {
				recovery := s.Native.Rollback(c, id)
				if s.SingBox != nil && s.SingBox.hasOperation(id) {
					recovery = errors.Join(recovery, s.SingBox.Rollback(c, id))
				}
				return nil, errors.Join(e, recovery, s.Manager.Rollback(c, id))
			}
		}
		return map[string]string{"status": "applied", "operation_id": id}, nil
	}, "POST"))
	mux.HandleFunc("/xray/rollback", wrap(func(c context.Context, v map[string]json.RawMessage) (any, error) {
		var id string
		_ = json.Unmarshal(v["operation_id"], &id)
		coreMu.Lock()
		defer coreMu.Unlock()
		var nativeErr error
		if s.Native != nil && s.Native.hasOperation(id) {
			nativeErr = s.Native.Rollback(c, id)
		}
		e := errors.Join(nativeErr, s.Manager.Rollback(c, id))
		if s.SingBox != nil {
			if s.SingBox.hasOperation(id) {
				e = errors.Join(e, s.SingBox.Rollback(c, id))
			}
		}
		return map[string]string{"status": "rolled_back"}, e
	}, "POST"))
	mux.HandleFunc("/xray/restart", wrap(func(c context.Context, _ map[string]json.RawMessage) (any, error) {
		return map[string]string{"status": "restarted"}, s.Manager.Restart(c)
	}, "POST"))
	mux.HandleFunc("/sni/scan", wrap(func(c context.Context, v map[string]json.RawMessage) (any, error) {
		var domains []string
		if b, ok := v["domains"]; ok {
			if e := json.Unmarshal(b, &domains); e != nil {
				return nil, e
			}
		}
		result, e := scanSNI(c, domains)
		return map[string]any{"results": result}, e
	}, "POST"))
	for path, wg := range map[string]bool{"/xray/x25519": false, "/xray/wg": true} {
		mux.HandleFunc(path, wrap(func(c context.Context, _ map[string]json.RawMessage) (any, error) { return s.Manager.Keypair(c, wg) }, "POST"))
	}
	if s.SingBox != nil {
		mux.HandleFunc("/singbox/status", wrap(func(c context.Context, _ map[string]json.RawMessage) (any, error) { return s.SingBox.Status(c), nil }, "GET"))
		mux.HandleFunc("/singbox/restart", wrap(func(c context.Context, _ map[string]json.RawMessage) (any, error) {
			return map[string]string{"status": "restarted"}, s.SingBox.Restart(c)
		}, "POST"))
	}
	return mux
}
func Serve(ctx context.Context, version string) (serveErr error) {
	if err := envfiles.LoadSecrets(); err != nil {
		return err
	}
	env := func(k, d string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		return d
	}
	token := env("VELORAY_AGENT_TOKEN", "")
	if len(token) < 32 {
		return errors.New("VELORAY_AGENT_TOKEN must contain at least 32 characters")
	}
	listen := env("VELORAY_AGENT_LISTEN", "127.0.0.1:9191")
	host, _, err := net.SplitHostPort(listen)
	if err != nil {
		return err
	}
	cert, key := os.Getenv("VELORAY_NODE_CERT"), os.Getenv("VELORAY_NODE_KEY")
	loopback := strings.EqualFold(host, "localhost") || net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()
	if (cert == "") != (key == "") {
		return errors.New("both TLS certificate and key are required")
	}
	if cert == "" && !loopback {
		return errors.New("remote agent requires TLS")
	}
	mode := env("VELORAY_CORE_MANAGER", "systemd")
	if mode != "systemd" && mode != "process" {
		return errors.New("VELORAY_CORE_MANAGER must be systemd or process")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var runtime Runtime
	var processes *ProcessRuntime
	if mode == "process" {
		processes = NewProcessRuntime()
		runtime = processes
		defer processes.Close()
	}
	m, err := NewManager(env("VELORAY_XRAY_BINARY", "/usr/local/bin/xray"), env("VELORAY_XRAY_CONFIG", "/usr/local/etc/xray/config.json"), env("VELORAY_XRAY_SERVICE", "xray.service"), env("VELORAY_AGENT_STATE", "/var/lib/veloray-agent/state.json"), runtime)
	if err != nil {
		return err
	}
	var second *Manager
	if _, e := os.Stat(env("VELORAY_SINGBOX_BINARY", "/usr/local/bin/sing-box")); e == nil {
		second, err = NewManager(env("VELORAY_SINGBOX_BINARY", "/usr/local/bin/sing-box"), env("VELORAY_SINGBOX_CONFIG", "/usr/local/etc/sing-box/config.json"), "sing-box.service", env("VELORAY_SINGBOX_STATE", "/var/lib/veloray-agent/sing-box-state.json"), runtime)
		if err != nil {
			return err
		}
		second.Core = "singbox"
	}
	native, err := newNativeManager(filepath.Dir(m.StatePath))
	if err != nil {
		return err
	}
	defer func() {
		final, stop := context.WithTimeout(context.WithoutCancel(ctx), 6*time.Second)
		defer stop()
		native.mu.Lock()
		if native.status(final).Running {
			_ = native.observe(final)
		}
		native.mu.Unlock()
		native.native.close()
	}()
	go native.Poll(ctx)
	if processes != nil {
		defer func() {
			final, stop := context.WithTimeout(context.WithoutCancel(ctx), 6*time.Second)
			defer stop()
			for _, manager := range []*Manager{m, second} {
				if manager != nil {
					manager.mu.Lock()
					if manager.status(final).Running {
						_ = manager.observe(final)
					}
					manager.mu.Unlock()
				}
			}
		}()
		for _, manager := range []*Manager{m, second} {
			if manager == nil {
				continue
			}
			if err = processes.register(manager); err != nil {
				return err
			}
			if err = initializeProcessCore(manager); err != nil {
				return err
			}

		}
	}
	var updater *CoreUpdater
	if directory := os.Getenv("VELORAY_CORE_DIRECTORY"); directory != "" {
		updater, err = NewCoreUpdater(ctx, directory, map[string]*Manager{"xray": m, "singbox": second})
		if err != nil {
			return err
		}
	}
	if processes != nil {
		for _, manager := range []*Manager{m, second} {
			if manager == nil {
				continue
			}
			raw, readErr := os.ReadFile(manager.ConfigPath)
			if readErr != nil {
				return readErr
			}
			if err = manager.validate(ctx, raw); err != nil {
				return err
			}
			if err = manager.Restart(ctx); err != nil {
				return err
			}
		}
	}
	go m.Poll(ctx)
	if second != nil {
		go second.Poll(ctx)
	}
	s := &http.Server{Addr: listen, Handler: Server{Updater: updater, NodeID: env("VELORAY_NODE_ID", "local"), Version: version, Token: token, Manager: m, SingBox: second, Native: native}.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 55 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	shutdown := make(chan struct{})
	go func() {
		defer close(shutdown)
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.Shutdown(c); err != nil {
			_ = s.Close()
		}
	}()
	defer func() { cancel(); <-shutdown }()
	if processes != nil {
		fatal := make(chan error, 1)
		go func() {
			select {
			case err := <-processes.failure:
				fatal <- err
				cancel()
			case <-ctx.Done():
			}
		}()
		defer func() {
			select {
			case err := <-fatal:
				serveErr = err
			default:
			}
		}()
	}
	if cert != "" {
		s.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS13}
		if ca := os.Getenv("VELORAY_CA_CERT"); ca != "" {
			raw, e := os.ReadFile(ca)
			if e != nil {
				return e
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(raw) {
				return errors.New("invalid client CA certificate")
			}
			s.TLSConfig.ClientCAs = pool
			s.TLSConfig.ClientAuth = tls.RequireAndVerifyClientCert
		}
		return s.ListenAndServeTLS(cert, key)
	}
	return s.ListenAndServe()
}
