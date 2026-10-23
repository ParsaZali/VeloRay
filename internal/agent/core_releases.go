package agent

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

const maxCoreArchive = 100 << 20
const maxCoreBinary = 128 << 20
const maxReleaseMetadata = 8 << 20

var releaseTag = regexp.MustCompile(`^v?[0-9][0-9A-Za-z.-]{0,63}$`)

type releaseAsset struct {
	Name, Digest string
	URL          string `json:"browser_download_url"`
	Size         int64
}
type officialRelease struct {
	Tag               string `json:"tag_name"`
	Draft, Prerelease bool
	Assets            []releaseAsset
	URL               string `json:"html_url"`
}
type CoreRelease struct {
	Tag            string `json:"tag"`
	Source         string `json:"source"`
	SHA256         string `json:"sha256"`
	Path           string `json:"-"`
	PreviousPath   string `json:"previous_path,omitempty"`
	PreviousTag    string `json:"previous_tag,omitempty"`
	PreviousSource string `json:"previous_source,omitempty"`
}
type coreJournal struct {
	State       string
	OldPath     string
	OldMetadata []byte
	WasRunning  bool
}
type coreJob struct {
	Core     string `json:"core"`
	Tag      string `json:"tag"`
	State    string `json:"state"`
	Error    string `json:"error,omitempty"`
	Started  string `json:"started_at,omitempty"`
	Finished string `json:"finished_at,omitempty"`
}

// No caller-supplied repository, asset URL, filesystem path or command is accepted.
// Updates are manual and disabled until an administrator enables downloads.
type CoreUpdater struct {
	mu              sync.Mutex
	ctx             context.Context
	Directory, Arch string
	HTTP            *http.Client
	Managers        map[string]*Manager
	enabled         bool
	job             coreJob
	geoJob          geoUpdateJob
	changeMu        *sync.Mutex
}

func coreRepository(core string) string {
	switch core {
	case "xray":
		return "XTLS/Xray-core"
	case "singbox":
		return "SagerNet/sing-box"
	}
	return ""
}
func coreAssetName(core, arch, tag string) string {
	if arch != "amd64" && arch != "arm64" {
		return ""
	}
	if core == "xray" {
		if arch == "amd64" {
			return "Xray-linux-64.zip"
		}
		return "Xray-linux-arm64-v8a.zip"
	}
	return "sing-box-" + strings.TrimPrefix(tag, "v") + "-linux-" + arch + ".tar.gz"
}
func officialRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > 4 || req.URL.Scheme != "https" || req.URL.User != nil {
		return errors.New("invalid official download redirect")
	}
	switch req.URL.Host {
	case "github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com":
		return nil
	}
	return errors.New("download redirected outside official GitHub storage")
}
func NewCoreUpdater(ctx context.Context, directory string, managers map[string]*Manager) (*CoreUpdater, error) {
	u := &CoreUpdater{ctx: ctx, Directory: directory, Arch: runtime.GOARCH, Managers: managers, HTTP: &http.Client{Timeout: 150 * time.Second, CheckRedirect: officialRedirect}}
	if !filepath.IsAbs(directory) {
		return nil, errors.New("absolute managed core directory required")
	}
	var policy struct {
		Enabled bool `json:"enabled"`
	}
	if raw, err := os.ReadFile(filepath.Join(directory, "policy.json")); err == nil {
		if err = json.Unmarshal(raw, &policy); err != nil {
			return nil, err
		}
		u.enabled = policy.Enabled
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if raw, err := os.ReadFile(filepath.Join(directory, "job.json")); err == nil {
		if err = json.Unmarshal(raw, &u.job); err != nil {
			return nil, err
		}
		if u.job.State == "running" {
			u.job.State = "error"
			u.job.Error = "Operation interrupted. Check the active core version before retrying."
			raw, _ = json.Marshal(u.job)
			if err = atomicWrite(filepath.Join(directory, "job.json"), raw, 0600); err != nil {
				return nil, err
			}
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	for core, m := range managers {
		if m == nil {
			continue
		}
		m.releaseSlot = filepath.Join(directory, core)
		if err := m.recoverCore(ctx); err != nil {
			return nil, fmt.Errorf("%s update recovery: %w", core, err)
		}
	}
	for _, kind := range []string{"geoip", "geosite"} {
		if err := u.recoverGeo(ctx, kind); err != nil {
			return nil, fmt.Errorf("%s update recovery: %w", kind, err)
		}
	}
	if raw, err := readBoundedGeoFile(filepath.Join(directory, "geo-job.json"), 65536); err == nil {
		if err = json.Unmarshal(raw, &u.geoJob); err != nil {
			return nil, err
		}
		if u.geoJob.State == "running" {
			u.geoJob.State, u.geoJob.Error = "error", "Operation interrupted. Check the active geo asset before retrying."
			u.geoJob.Finished = time.Now().UTC().Format(time.RFC3339)
			raw, _ = json.Marshal(u.geoJob)
			if err = atomicWrite(filepath.Join(directory, "geo-job.json"), raw, 0600); err != nil {
				return nil, err
			}
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return u, nil
}
func (u *CoreUpdater) fetchJSON(ctx context.Context, endpoint string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/repos/"+endpoint, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "VeloRay-core-manager")
	res, err := u.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("official GitHub release API returned %d", res.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxReleaseMetadata+1))
	if err != nil {
		return err
	}
	if len(raw) > maxReleaseMetadata {
		return errors.New("release metadata too large")
	}
	return json.Unmarshal(raw, out)
}
func (u *CoreUpdater) Catalog(ctx context.Context, core string) (map[string]any, error) {
	repo := coreRepository(core)
	if repo == "" {
		return nil, errors.New("unsupported core")
	}
	// Large upstream asset inventories can exceed several MiB. Keep history
	// bounded and always include the latest stable release, even when many
	// prereleases occupy the recent history.
	var latest officialRelease
	if err := u.fetchJSON(ctx, repo+"/releases/latest", &latest); err != nil {
		return nil, err
	}
	var history []officialRelease
	_ = u.fetchJSON(ctx, repo+"/releases?per_page=10", &history)
	releases := append([]officialRelease{latest}, history...)
	items := []map[string]string{}
	seen := map[string]bool{}
	for _, r := range releases {
		if r.Draft || r.Prerelease || !releaseTag.MatchString(r.Tag) || seen[r.Tag] {
			continue
		}
		if a, err := releaseDownload(core, u.Arch, r); err == nil {
			seen[r.Tag] = true
			items = append(items, map[string]string{"tag": r.Tag, "sha256": strings.TrimPrefix(a.Digest, "sha256:"), "url": "https://github.com/" + repo + "/releases/tag/" + r.Tag})
		}
	}
	return map[string]any{"repository": repo, "architecture": u.Arch, "releases": items}, nil
}
func releaseDownload(core, arch string, r officialRelease) (releaseAsset, error) {
	if r.Draft || r.Prerelease || !releaseTag.MatchString(r.Tag) {
		return releaseAsset{}, errors.New("only stable official releases can be installed")
	}
	name := coreAssetName(core, arch, r.Tag)
	if name == "" {
		return releaseAsset{}, errors.New("unsupported architecture")
	}
	expected := "https://github.com/" + coreRepository(core) + "/releases/download/" + r.Tag + "/" + name
	for _, a := range r.Assets {
		if a.Name == name {
			digest := strings.TrimPrefix(a.Digest, "sha256:")
			b, err := hex.DecodeString(digest)
			if a.URL != expected || !strings.HasPrefix(a.Digest, "sha256:") || err != nil || len(b) != 32 || a.Size < 1 || a.Size > maxCoreArchive {
				return a, errors.New("release has no valid official SHA-256 asset metadata")
			}
			return a, nil
		}
	}
	return releaseAsset{}, errors.New("official Linux binary unavailable for this version")
}
func (u *CoreUpdater) Status() map[string]any {
	u.mu.Lock()
	defer u.mu.Unlock()
	cores := map[string]any{}
	for core, m := range u.Managers {
		if m == nil {
			continue
		}
		record := m.coreRecord()
		cores[core] = map[string]any{"tag": record.Tag, "source": record.Source, "sha256": record.SHA256, "can_rollback": record.PreviousPath != "", "user_ip_attribution": core != "singbox" || record.Source != "official"}
	}
	return map[string]any{"managed": true, "enabled": u.enabled, "automatic": false, "cores": cores, "job": u.job, "geo_job": u.geoJob}
}
func (u *CoreUpdater) SetEnabled(enabled bool) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.job.State == "running" || u.geoJob.State == "running" {
		return errors.New("wait for the current core or geo operation")
	}
	raw, _ := json.Marshal(map[string]bool{"enabled": enabled})
	if err := atomicWrite(filepath.Join(u.Directory, "policy.json"), raw, 0600); err != nil {
		return err
	}
	u.enabled = enabled
	return nil
}
func (u *CoreUpdater) SupportsUserIPs() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.job.Core == "singbox" && u.job.State == "running" {
		return false
	}
	m := u.Managers["singbox"]
	return m == nil || m.coreRecord().Source != "official"
}
func (u *CoreUpdater) Start(core, tag string, rollback, requireIPs bool) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	m := u.Managers[core]
	if coreRepository(core) == "" || m == nil {
		return errors.New("core unavailable")
	}
	if u.job.State == "running" || u.geoJob.State == "running" {
		return errors.New("another core or geo operation is running")
	}
	if !rollback && !u.enabled {
		return errors.New("official core downloads are disabled")
	}
	if !rollback && !releaseTag.MatchString(tag) {
		return errors.New("invalid release tag")
	}
	if core == "singbox" && requireIPs && (!rollback || m.coreRecord().PreviousSource == "official") {
		return errors.New("official sing-box does not provide VeloRay user-IP attribution; remove per-user IP caps or keep the bundled core")
	}
	u.job = coreJob{Core: core, Tag: tag, State: "running", Started: time.Now().UTC().Format(time.RFC3339)}
	raw, err := json.Marshal(u.job)
	if err != nil {
		return err
	}
	if err = atomicWrite(filepath.Join(u.Directory, "job.json"), raw, 0600); err != nil {
		u.job.State = "error"
		return err
	}
	go func() {
		ctx, cancel := context.WithTimeout(u.ctx, 3*time.Minute)
		defer cancel()
		var candidate string
		var record CoreRelease
		var err error
		if rollback {
			previous := m.coreRecord()
			candidate = previous.PreviousPath
			record = CoreRelease{Tag: previous.PreviousTag, Source: previous.PreviousSource}
			if candidate == "" {
				err = errors.New("no previous core available")
			}
		} else {
			candidate, record, err = u.download(ctx, core, tag)
		}
		if err == nil {
			if u.changeMu != nil {
				u.changeMu.Lock()
			}
			err = m.installCore(ctx, candidate, record)
			if u.changeMu != nil {
				u.changeMu.Unlock()
			}
		}
		u.mu.Lock()
		defer u.mu.Unlock()
		u.job.Finished = time.Now().UTC().Format(time.RFC3339)
		u.job.State = "success"
		if err != nil {
			u.job.State = "error"
			u.job.Error = err.Error()
		}
		raw, _ := json.Marshal(u.job)
		_ = atomicWrite(filepath.Join(u.Directory, "job.json"), raw, 0600)
	}()
	return nil
}
func (u *CoreUpdater) download(ctx context.Context, core, tag string) (string, CoreRelease, error) {
	r := CoreRelease{Tag: tag, Source: "official"}
	var release officialRelease
	if err := u.fetchJSON(ctx, coreRepository(core)+"/releases/tags/"+url.PathEscape(tag), &release); err != nil {
		return "", r, err
	}
	if release.Tag != tag {
		return "", r, errors.New("release tag mismatch")
	}
	a, err := releaseDownload(core, u.Arch, release)
	if err != nil {
		return "", r, err
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", a.URL, nil)
	res, err := u.HTTP.Do(req)
	if err != nil {
		return "", r, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", r, fmt.Errorf("official download returned %d", res.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxCoreArchive+1))
	if err != nil {
		return "", r, err
	}
	digest := sha256.Sum256(raw)
	if len(raw) > maxCoreArchive || int64(len(raw)) != a.Size || hex.EncodeToString(digest[:]) != strings.TrimPrefix(a.Digest, "sha256:") {
		return "", r, errors.New("official archive SHA-256 verification failed")
	}
	data, err := extractCore(core, tag, u.Arch, raw)
	if err != nil {
		return "", r, err
	}
	h := sha256.Sum256(data)
	r.SHA256 = hex.EncodeToString(h[:])
	candidate := filepath.Join(u.Directory, core, "binary-"+r.SHA256)
	if err = atomicWrite(candidate, data, 0755); err != nil {
		return "", r, err
	}
	return candidate, r, nil
}
func extractCore(core, tag, arch string, raw []byte) ([]byte, error) {
	want := "xray"
	if core == "singbox" {
		want = "sing-box-" + strings.TrimPrefix(tag, "v") + "-linux-" + arch + "/sing-box"
	}
	var binaryData []byte
	read := func(name string, regular bool, size int64, reader io.Reader) error {
		if strings.HasPrefix(name, "/") || path.Clean(name) != strings.TrimSuffix(name, "/") || strings.HasPrefix(name, "../") || strings.Contains(name, "\\") {
			return errors.New("unsafe core archive path")
		}
		if name != want {
			return nil
		}
		if !regular || size < 20 || size > maxCoreBinary || binaryData != nil {
			return errors.New("invalid core executable entry")
		}
		var err error
		binaryData, err = io.ReadAll(io.LimitReader(reader, maxCoreBinary+1))
		return err
	}
	if core == "xray" {
		z, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
		if err != nil {
			return nil, err
		}
		if len(z.File) > 2048 {
			return nil, errors.New("too many archive entries")
		}
		for _, f := range z.File {
			if f.Mode()&os.ModeSymlink != 0 {
				return nil, errors.New("archive links are not permitted")
			}
			r, err := f.Open()
			if err != nil {
				return nil, err
			}
			err = read(f.Name, f.Mode().IsRegular(), int64(f.UncompressedSize64), r)
			_ = r.Close()
			if err != nil {
				return nil, err
			}
		}
	} else {
		g, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		defer g.Close()
		t := tar.NewReader(io.LimitReader(g, 512<<20))
		for entries := 0; ; entries++ {
			h, err := t.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			if entries >= 2048 {
				return nil, errors.New("too many archive entries")
			}
			if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeDir {
				return nil, errors.New("archive links and special files are not permitted")
			}
			if err = read(h.Name, h.Typeflag == tar.TypeReg, h.Size, t); err != nil {
				return nil, err
			}
		}
	}
	machine := uint16(62)
	if arch == "arm64" {
		machine = 183
	}
	if len(binaryData) < 20 || len(binaryData) > maxCoreBinary || string(binaryData[:4]) != "\x7fELF" || binaryData[4] != 2 || binaryData[5] != 1 || binary.LittleEndian.Uint16(binaryData[18:20]) != machine {
		return nil, errors.New("archive does not contain the expected Linux executable")
	}
	return binaryData, nil
}
