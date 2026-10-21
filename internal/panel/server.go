package panel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var panelProcessStartedAt = time.Now()

type Server struct {
	EmailSend    func(context.Context, string, []string, []byte) error
	alertMu      sync.Mutex
	alertCursor  string
	OIDCHTTP     *http.Client
	GitHubHTTP   *http.Client
	streamMu     sync.Mutex
	streamCount  int
	streamUsers  map[string]int
	StartedAt    time.Time
	Config       Config
	Store        *Store
	HTTP         *http.Client
	TelegramHTTP *http.Client
	WebhookHTTP  *http.Client
	ExternalHTTP *http.Client
	Logger       *slog.Logger
}
type apiError struct {
	Status       int
	Code, Detail string
}

func (e *apiError) Error() string                 { return e.Detail }
func fail(status int, msg string) error           { return &apiError{Status: status, Detail: msg} }
func failCode(status int, code, msg string) error { return &apiError{status, code, msg} }
func readBody(w http.ResponseWriter, r *http.Request) (Data, error) {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.UseNumber()
	d := Data{}
	if e := dec.Decode(&d); e != nil || d == nil {
		return nil, fail(400, "JSON object required")
	}
	if e := dec.Decode(new(any)); e != io.EOF {
		return nil, fail(400, "unexpected data after JSON object")
	}
	return d, nil
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if status != 204 {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// Subscription URLs are bearer credentials. Keep the route useful for diagnostics
// without copying its private token (or asset suffix) into application logs.
func requestLogPath(r *http.Request) string {
	if strings.HasPrefix(r.URL.Path, "/sub/") {
		return "/sub/[redacted]"
	}
	return r.URL.Path
}

func (s *Server) respond(w http.ResponseWriter, r *http.Request, out any, status int, e error) {
	if e != nil {
		var ae *apiError
		if errors.As(e, &ae) {
			out = Data{"detail": ae.Detail}
			if ae.Code != "" {
				out.(Data)["code"] = ae.Code
			}
			status = ae.Status
		} else if errors.Is(e, pgx.ErrNoRows) {
			status = 404
			out = Data{"detail": "not found"}
		} else {
			status = 500
			out = Data{"detail": "The request could not be completed"}
			s.Logger.Error("request failed", "method", r.Method, "path", requestLogPath(r), "error", e)
		}
	}
	if status == 0 {
		status = 200
	}
	writeJSON(w, status, out)
}
func (s *Server) Handler() http.Handler {
	if s.Logger == nil {
		s.Logger = slog.Default()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = s.withProxyIP(r)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		defer func() {
			if v := recover(); v != nil {
				s.Logger.Error("panic recovered", "path", requestLogPath(r), "panic", v)
				writeJSON(w, 500, Data{"detail": "Internal server error"})
			}
		}()
		if strings.HasPrefix(r.URL.Path, "/sub/") {
			s.subscription(w, r)
			return
		}
		if s.serveSubscriptionAsset(w, r) {
			return
		}
		if s.Config.SubscriptionOnly && r.URL.Path != "/api/health" {
			http.NotFound(w, r)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			s.frontend(w, r)
			return
		}
		path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/"), "/")
		if path == "health" && r.Method == "GET" {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			if e := s.Store.Pool.Ping(ctx); e != nil {
				d := s.panelRuntime(time.Now())
				d["status"] = "degraded"
				d["database"] = "unavailable"
				d["version"] = Version
				writeJSON(w, 503, d)
				return
			}
			d := s.panelRuntime(time.Now())
			d["status"] = "ok"
			d["database"] = "ok"
			d["version"] = Version
			writeJSON(w, 200, d)
			return
		}
		if path == "version" && r.Method == "GET" {
			writeJSON(w, 200, Data{"version": Version})
			return
		}
		if path == "auth/csrf" && r.Method == "GET" {
			token := csrfToken(s.Config.SecretKey)
			s.cookie(w, "csrftoken", token, 86400, false)
			writeJSON(w, 200, Data{"csrfToken": token})
			return
		}
		if path == "auth/oidc" && r.Method == "GET" {
			writeJSON(w, 200, Data{"enabled": s.Config.OIDCIssuer != "" && s.Config.OIDCClientID != "", "github_enabled": s.githubEnabled(), "ldap_enabled": s.ldapEnabled(), "telegram": s.Config.OIDCIssuer == "https://oauth.telegram.org"})
			return
		}
		if path == "auth/oidc/callback" && r.Method == "GET" {
			s.oidcCallback(w, r)
			return
		}
		if path == "auth/github/callback" && r.Method == "GET" {
			s.githubCallback(w, r)
			return
		}
		if e := s.checkCSRF(r); e != nil {
			s.respond(w, r, nil, 0, e)
			return
		}
		if path == "auth/oidc/start" && r.Method == "POST" {
			out, status, e := s.oidcStart(w, r)
			s.respond(w, r, out, status, e)
			return
		}
		if path == "auth/github/start" && r.Method == "POST" {
			out, status, e := s.githubStart(w, r)
			s.respond(w, r, out, status, e)
			return
		}
		if path == "auth/oidc/complete" && r.Method == "POST" {
			out, status, e := s.oidcComplete(w, r)
			s.respond(w, r, out, status, e)
			return
		}
		if path == "auth/login" && r.Method == "POST" {
			out, status, e := s.login(w, r)
			s.respond(w, r, out, status, e)
			return
		}
		a, e := s.authenticate(r)
		if e != nil {
			s.respond(w, r, nil, 0, e)
			return
		}
		if e = s.authorize(r.Context(), a, r.Method, path); e != nil {
			s.respond(w, r, nil, 0, e)
			return
		}
		if path == "metrics" {
			s.prometheus(w, r, a)
			return
		}
		if strings.HasPrefix(path, "auth/") {
			out, status, e := s.authAPI(w, r, path, a)
			s.respond(w, r, out, status, e)
			return
		}
		if path == "events" {
			s.streamEvents(w, r, a)
			return
		}
		out, status, e := s.api(w, r, path, a)
		s.respond(w, r, out, status, e)
	})
}
func (s *Server) frontend(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		writeJSON(w, 405, Data{"detail": "method not allowed"})
		return
	}
	root := s.Config.FrontendDir
	path := filepath.Clean("/" + r.URL.Path)
	if strings.HasPrefix(path, "/assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	file := filepath.Join(root, path)
	if info, e := os.Stat(file); e == nil && !info.IsDir() {
		http.ServeFile(w, r, file)
		return
	}
	if strings.Contains(filepath.Base(path), ".") {
		http.NotFound(w, r)
		return
	}
	if _, e := os.Stat(filepath.Join(root, "index.html")); e != nil {
		writeJSON(w, 503, Data{"detail": "Frontend build is missing"})
		return
	}
	http.ServeFile(w, r, filepath.Join(root, "index.html"))
}
func Serve(ctx context.Context, c Config) error {
	listener, e := net.Listen("tcp", c.Listen)
	if e != nil {
		return fmt.Errorf("cannot listen on %s: %w; use veloray restart to manage the installed service", c.Listen, e)
	}
	defer listener.Close()
	store, e := OpenStore(ctx, c.DatabaseURL)
	if e != nil {
		return e
	}
	defer store.Pool.Close()
	if !c.SubscriptionOnly {
		if e = store.Migrate(ctx); e != nil {
			return e
		}
	}
	s := &Server{Config: c, Store: store, Logger: slog.Default()}
	workerCtx, stopWorker := context.WithCancel(ctx)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		if !c.SubscriptionOnly {
			s.Worker(workerCtx)
		}
	}()
	defer func() { stopWorker(); <-workerDone }()
	server := &http.Server{Addr: c.Listen, Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 180 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	go func() {
		select {
		case <-ctx.Done():
		case <-workerCtx.Done():
			return
		}
		c, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = server.Shutdown(c)
	}()
	s.Logger.Info("VeloRay started", "version", Version, "listen", c.Listen)
	return server.Serve(listener)
}
