package panel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

const installedCommitPath = "/var/lib/veloray/installed-commit"
const autoUpdateTimerLink = "/etc/systemd/system/timers.target.wants/veloray-auto-update.timer"
const hostControlTimerLink = "/etc/systemd/system/timers.target.wants/veloray-host-control.timer"

var commitIDPattern = regexp.MustCompile(`^[a-f0-9]{40}$`)

type updateCache struct {
	mu                  sync.Mutex
	at                  time.Time
	latest, ci, problem string
}

func installedCommit() string {
	if value := os.Getenv("VELORAY_INSTALLED_COMMIT"); os.Getenv("VELORAY_DEPLOYMENT") == "docker" && commitIDPattern.MatchString(value) {
		return value
	}
	raw, _ := os.ReadFile(installedCommitPath)
	candidate := strings.TrimSpace(string(raw))
	if commitIDPattern.MatchString(candidate) {
		return candidate
	}
	return ""
}
func (s *Server) nativeControlAvailable() bool {
	if os.Getenv("VELORAY_DEPLOYMENT") == "docker" {
		return s.Config.SubscriptionDriver == "caddy" && os.Getenv("VELORAY_HOST_CONTROL") == "true"
	}
	_, err := os.Lstat(hostControlTimerLink)
	return s.Config.SubscriptionDriver == "native" && err == nil
}
func updateState(installed, latest, ci, problem string) string {
	if problem != "" || installed == "" || latest == "" {
		return "unknown"
	}
	if installed == latest {
		return "up_to_date"
	}
	if ci == "success" {
		return "available"
	}
	return "verifying"
}
func (s *Server) githubUpdateJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/repos/ParsaZali/VeloRay/"+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "VeloRay/"+Version)
	client := s.GitHubHTTP
	if client == nil {
		client = &http.Client{}
	}
	bounded := *client
	bounded.Timeout = 10 * time.Second
	bounded.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := bounded.Do(req)
	if err != nil {
		return errors.New("GitHub could not be reached. Try again later.")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("GitHub update check returned HTTP %d. Try again later.", res.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(out)
}
func (s *Server) updateStatus(ctx context.Context, refresh bool) (Data, error) {
	cache := &s.updates
	cache.mu.Lock()
	defer cache.mu.Unlock()
	ttl := 5 * time.Minute
	if refresh {
		ttl = 30 * time.Second
	}
	if time.Since(cache.at) >= ttl {
		cache.at = time.Now()
		cache.latest, cache.ci, cache.problem = "", "", ""
		checkCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		defer cancel()
		var ref struct {
			Object struct {
				SHA string `json:"sha"`
			} `json:"object"`
		}
		err := s.githubUpdateJSON(checkCtx, "git/ref/heads/main", &ref)
		if err == nil && !commitIDPattern.MatchString(ref.Object.SHA) {
			err = errors.New("GitHub returned an invalid revision")
		}
		if err == nil {
			cache.latest = ref.Object.SHA
			if cache.latest != installedCommit() {
				var runs struct {
					Runs []struct {
						SHA        string `json:"head_sha"`
						Branch     string `json:"head_branch"`
						Event      string `json:"event"`
						Status     string `json:"status"`
						Conclusion string `json:"conclusion"`
					} `json:"workflow_runs"`
				}
				err = s.githubUpdateJSON(checkCtx, "actions/workflows/ci.yml/runs?branch=main&per_page=30", &runs)
				for _, run := range runs.Runs {
					if run.SHA == cache.latest && run.Branch == "main" && run.Event == "push" {
						cache.ci = run.Status
						if run.Status == "completed" {
							cache.ci = run.Conclusion
						}
						break
					}
				}
			}
		}
		if err != nil {
			cache.problem = err.Error()
		}
	}
	installed := installedCommit()
	_, timerErr := os.Lstat(autoUpdateTimerLink)
	job, err := s.lastHostJob(ctx, "panel_update")
	if err != nil {
		return nil, err
	}
	return Data{"repository": "ParsaZali/VeloRay", "tracking_branch": "main", "version": Version, "installed_commit": installed, "latest_commit": cache.latest, "auto_update_enabled": timerErr == nil, "managed": s.nativeControlAvailable(), "verification": "github-main-ci-and-pinned-commit", "state": updateState(installed, cache.latest, cache.ci, cache.problem), "ci_status": cache.ci, "error": cache.problem, "checked_at": stamp(cache.at), "job": job}, nil
}
func (s *Server) updatesAPI(w http.ResponseWriter, r *http.Request, path string, a Actor) (any, int, error) {
	if !flag(a.User, "is_superuser") || a.Scope != "admin" {
		return nil, 0, fail(403, "administrator access required")
	}
	if path == "updates/status" && r.Method == http.MethodGet || path == "updates/check" && r.Method == http.MethodPost {
		out, err := s.updateStatus(r.Context(), path == "updates/check")
		return out, 200, err
	}
	if path != "updates/install" || r.Method != http.MethodPost {
		return nil, 0, fail(405, "method not allowed")
	}
	if !s.nativeControlAvailable() {
		return nil, 0, fail(409, "Web updates require the VeloRay host control service.")
	}
	in, err := readBody(w, r)
	if err != nil {
		return nil, 0, err
	}
	if err = allowed(in, "commit"); err != nil {
		return nil, 0, fail(400, err.Error())
	}
	target := str(in, "commit")
	if !commitIDPattern.MatchString(target) {
		return nil, 0, fail(400, "a verified commit is required")
	}
	status, err := s.updateStatus(r.Context(), true)
	if err != nil {
		return nil, 0, err
	}
	if str(status, "state") != "available" || str(status, "latest_commit") != target {
		return nil, 0, fail(409, "This revision is not a verified update. Check for updates again.")
	}
	job, err := s.queueHostJob(r.Context(), a, "panel_update", target, installedCommit())
	return job, 202, err
}
