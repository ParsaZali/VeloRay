package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxGeoAsset = 64 << 20

type geoVersion struct {
	Tag        string `json:"tag"`
	Repository string `json:"repository"`
	SHA256     string `json:"sha256"`
}
type geoRecord struct {
	Current  geoVersion  `json:"current"`
	Previous *geoVersion `json:"previous,omitempty"`
}
type geoJournal struct {
	State      string
	Previous   geoVersion
	Metadata   []byte
	WasRunning bool
}
type geoUpdateJob struct {
	Kind     string `json:"kind"`
	Tag      string `json:"tag"`
	State    string `json:"state"`
	Error    string `json:"error,omitempty"`
	Started  string `json:"started_at,omitempty"`
	Finished string `json:"finished_at,omitempty"`
}

func geoRepository(kind string) (string, string) {
	switch kind {
	case "geoip":
		return "v2fly/geoip", "geoip.dat"
	case "geosite":
		return "v2fly/domain-list-community", "dlc.dat"
	}
	return "", ""
}
func geoDownload(kind string, release officialRelease) (releaseAsset, error) {
	repo, name := geoRepository(kind)
	if repo == "" || release.Draft || release.Prerelease || !releaseTag.MatchString(release.Tag) {
		return releaseAsset{}, errors.New("choose a stable official geo release")
	}
	for _, asset := range release.Assets {
		if asset.Name != name {
			continue
		}
		sum := strings.TrimPrefix(asset.Digest, "sha256:")
		decoded, err := hex.DecodeString(sum)
		expected := "https://github.com/" + repo + "/releases/download/" + release.Tag + "/" + name
		if asset.URL != expected || !strings.HasPrefix(asset.Digest, "sha256:") || err != nil || len(decoded) != 32 || asset.Size < 1 || asset.Size > maxGeoAsset {
			return asset, errors.New("geo release has no valid official SHA-256 asset metadata")
		}
		return asset, nil
	}
	return releaseAsset{}, errors.New("official geo asset is unavailable")
}
func (u *CoreUpdater) GeoCatalog(ctx context.Context, kind string) (map[string]any, error) {
	repo, _ := geoRepository(kind)
	if repo == "" {
		return nil, errors.New("choose geoip or geosite")
	}
	var latest officialRelease
	if err := u.fetchJSON(ctx, repo+"/releases/latest", &latest); err != nil {
		return nil, err
	}
	var history []officialRelease
	_ = u.fetchJSON(ctx, repo+"/releases?per_page=5", &history)
	items := []map[string]string{}
	seen := map[string]bool{}
	for _, release := range append([]officialRelease{latest}, history...) {
		if seen[release.Tag] {
			continue
		}
		if asset, err := geoDownload(kind, release); err == nil {
			seen[release.Tag] = true
			items = append(items, map[string]string{"tag": release.Tag, "sha256": strings.TrimPrefix(asset.Digest, "sha256:"), "url": "https://github.com/" + repo + "/releases/tag/" + release.Tag})
		}
	}
	return map[string]any{"kind": kind, "repository": repo, "releases": items}, nil
}
func geoBase() string {
	if base := os.Getenv("XRAY_LOCATION_ASSET"); base != "" {
		return base
	}
	return "/usr/local/share/xray"
}
func (u *CoreUpdater) geoSlot(kind string) string {
	return filepath.Join(u.Directory, "geo", kind)
}
func (u *CoreUpdater) geoPath(kind, sum string) (string, error) {
	repo, _ := geoRepository(kind)
	decoded, err := hex.DecodeString(sum)
	if repo == "" || err != nil || len(decoded) != 32 || sum != strings.ToLower(sum) {
		return "", errors.New("invalid managed geo asset identity")
	}
	return filepath.Join(u.geoSlot(kind), "asset-"+sum+".dat"), nil
}
func readBoundedGeoFile(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > limit {
		return nil, errors.New("geo file must be a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil || !os.SameFile(info, stat) || !stat.Mode().IsRegular() || stat.Size() > limit {
		return nil, errors.New("invalid geo file")
	}
	raw, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(raw)) > limit {
		return nil, errors.New("geo file exceeds its size limit")
	}
	return raw, nil
}
func geoDigest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func (u *CoreUpdater) geoRecord(kind string) geoRecord {
	var record geoRecord
	if raw, err := readBoundedGeoFile(filepath.Join(u.geoSlot(kind), "active.json"), 65536); err == nil {
		_ = json.Unmarshal(raw, &record)
	}
	return record
}
func (u *CoreUpdater) GeoStatus() map[string]any {
	u.mu.Lock()
	defer u.mu.Unlock()
	assets := map[string]any{}
	for _, kind := range []string{"geoip", "geosite"} {
		record := u.geoRecord(kind)
		current := record.Current
		raw, err := readBoundedGeoFile(filepath.Join(geoBase(), kind+".dat"), maxGeoAsset)
		if err == nil {
			sum := geoDigest(raw)
			if sum != current.SHA256 {
				current = geoVersion{Tag: "installed", Repository: "installed", SHA256: sum}
			}
		}
		repo, _ := geoRepository(kind)
		assets[kind] = map[string]any{"tag": current.Tag, "source": current.Repository, "sha256": current.SHA256, "available": err == nil, "repository": repo, "can_rollback": record.Previous != nil}
	}
	return map[string]any{"managed": u.Managers["xray"] != nil, "enabled": u.enabled, "assets": assets, "job": u.geoJob, "core_busy": u.job.State == "running"}
}
func (u *CoreUpdater) downloadGeo(ctx context.Context, kind, tag string) ([]byte, geoVersion, error) {
	repo, _ := geoRepository(kind)
	version := geoVersion{Tag: tag, Repository: repo}
	if repo == "" || !releaseTag.MatchString(tag) {
		return nil, version, errors.New("invalid geo release")
	}
	var release officialRelease
	if err := u.fetchJSON(ctx, repo+"/releases/tags/"+url.PathEscape(tag), &release); err != nil {
		return nil, version, err
	}
	if release.Tag != tag {
		return nil, version, errors.New("geo release tag mismatch")
	}
	asset, err := geoDownload(kind, release)
	if err != nil {
		return nil, version, err
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", asset.URL, nil)
	res, err := u.HTTP.Do(req)
	if err != nil {
		return nil, version, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, version, fmt.Errorf("official geo download returned %d", res.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxGeoAsset+1))
	version.SHA256 = geoDigest(raw)
	if err != nil || len(raw) > maxGeoAsset || int64(len(raw)) != asset.Size || version.SHA256 != strings.TrimPrefix(asset.Digest, "sha256:") {
		return nil, version, errors.New("official geo SHA-256 verification failed")
	}
	if _, err = indexGeoAsset(ctx, kind, raw); err != nil {
		return nil, version, errors.New("downloaded geo asset has an invalid protobuf index")
	}
	return raw, version, nil
}

// Xray parses every tag, including tags unused by the current configuration.
// The installed other asset is also present while checking current routing.
func validateGeo(ctx context.Context, m *Manager, kind string, raw []byte) error {
	entries, err := indexGeoAsset(ctx, kind, raw)
	if err != nil {
		return errors.New("invalid geo asset index")
	}
	dir, err := os.MkdirTemp("", "veloray-geo-check-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	for _, other := range []string{"geoip", "geosite"} {
		target := filepath.Join(dir, other+".dat")
		if other == kind {
			err = os.WriteFile(target, raw, 0600)
		} else {
			source, pathErr := filepath.Abs(filepath.Join(geoBase(), other+".dat"))
			if pathErr != nil {
				return pathErr
			}
			err = os.Symlink(source, target)
		}
		if err != nil {
			return err
		}
	}
	references := make([]string, 0, len(entries))
	for _, entry := range entries {
		references = append(references, entry.Reference)
	}
	field := "domain"
	if kind == "geoip" {
		field = "ip"
	}
	probe := filepath.Join(dir, "all-tags.json")
	run := func(path string) error {
		out, runErr := m.Runtime.Run(ctx, "/usr/bin/env", "XRAY_LOCATION_ASSET="+dir, m.Binary, "run", "-test", "-config", path)
		if runErr != nil {
			return fmt.Errorf("Xray rejected the staged geo asset or current routing: %.1500s", out)
		}
		return nil
	}
	// Bound the number of simultaneously compiled routing matchers. Large
	// GeoIP inventories contain overlapping ranges across many categories.
	for start := 0; start < len(references); start += 64 {
		check := map[string]any{
			"log":       map[string]any{"loglevel": "error"},
			"inbounds":  []any{},
			"outbounds": []any{map[string]any{"tag": "direct", "protocol": "freedom"}},
			"routing":   map[string]any{"rules": []any{map[string]any{"type": "field", field: references[start:min(start+64, len(references))], "outboundTag": "direct"}}},
		}
		config, _ := json.Marshal(check)
		if err = os.WriteFile(probe, config, 0600); err != nil {
			return err
		}
		if err = run(probe); err != nil {
			return err
		}
	}
	return run(m.ConfigPath)
}
func (u *CoreUpdater) recoverGeo(ctx context.Context, kind string) error {
	journalPath := filepath.Join(u.geoSlot(kind), "pending.json")
	raw, err := readBoundedGeoFile(journalPath, 131072)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var journal geoJournal
	if err = json.Unmarshal(raw, &journal); err != nil {
		return err
	}
	if journal.State == "committed" {
		return os.Remove(journalPath)
	}
	if journal.State != "pending" {
		return errors.New("invalid geo recovery state")
	}
	path, err := u.geoPath(kind, journal.Previous.SHA256)
	if err != nil {
		return err
	}
	data, err := readBoundedGeoFile(path, maxGeoAsset)
	if err != nil || geoDigest(data) != journal.Previous.SHA256 {
		return errors.New("previous geo asset failed recovery SHA-256 verification")
	}
	m := u.Managers["xray"]
	if m == nil {
		return errors.New("Xray is required for geo recovery")
	}
	if err = validateGeo(ctx, m, kind, data); err != nil {
		return err
	}
	if m.status(ctx).Running {
		if err = m.observe(ctx); err != nil {
			return err
		}
	}
	if err = atomicWrite(filepath.Join(geoBase(), kind+".dat"), data, 0644); err != nil {
		return err
	}
	metadata := filepath.Join(u.geoSlot(kind), "active.json")
	if len(journal.Metadata) > 0 {
		err = atomicWrite(metadata, journal.Metadata, 0600)
	} else {
		err = os.Remove(metadata)
		if os.IsNotExist(err) {
			err = nil
		}
	}
	if err != nil {
		return err
	}
	if journal.WasRunning {
		if err = m.restart(ctx); err != nil {
			return err
		}
	}
	return os.Remove(journalPath)
}
func (u *CoreUpdater) pruneGeo(kind string) {
	record := u.geoRecord(kind)
	keep := map[string]bool{record.Current.SHA256: true}
	if record.Previous != nil {
		keep[record.Previous.SHA256] = true
	}
	if raw, err := readBoundedGeoFile(filepath.Join(geoBase(), kind+".dat"), maxGeoAsset); err == nil {
		keep[geoDigest(raw)] = true
	}
	entries, _ := os.ReadDir(u.geoSlot(kind))
	for _, entry := range entries {
		sum := strings.TrimSuffix(strings.TrimPrefix(entry.Name(), "asset-"), ".dat")
		path, err := u.geoPath(kind, sum)
		if err == nil && filepath.Base(path) == entry.Name() && !keep[sum] {
			_ = os.Remove(path)
		}
	}
}
func (u *CoreUpdater) installGeo(ctx context.Context, kind string, data []byte, version geoVersion) error {
	m := u.Managers["xray"]
	if m == nil {
		return errors.New("Xray manager unavailable")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	defer func() {
		if _, err := os.Stat(filepath.Join(u.geoSlot(kind), "pending.json")); os.IsNotExist(err) {
			u.pruneGeo(kind)
		}
	}()
	if version.SHA256 != geoDigest(data) {
		return errors.New("selected geo asset failed SHA-256 verification")
	}
	if err := validateGeo(ctx, m, kind, data); err != nil {
		return err
	}
	old, err := readBoundedGeoFile(filepath.Join(geoBase(), kind+".dat"), maxGeoAsset)
	if err != nil {
		return errors.New("installed geo asset is unavailable; repair the node before updating")
	}
	record := u.geoRecord(kind)
	if geoDigest(old) == version.SHA256 && record.Current.SHA256 == version.SHA256 {
		record.Current = version
		metadata, _ := json.Marshal(record)
		return atomicWrite(filepath.Join(u.geoSlot(kind), "active.json"), metadata, 0600)
	}
	previous := u.geoRecord(kind).Current
	if sum := geoDigest(old); previous.SHA256 != sum {
		previous = geoVersion{Tag: "installed", Repository: "installed", SHA256: sum}
	}
	// A valid rollback snapshot is required before changing the live file.
	if err = validateGeo(ctx, m, kind, old); err != nil {
		return fmt.Errorf("installed geo asset cannot be used for rollback: %w", err)
	}
	for _, snapshot := range []struct {
		version geoVersion
		data    []byte
	}{{previous, old}, {version, data}} {
		path, pathErr := u.geoPath(kind, snapshot.version.SHA256)
		if pathErr != nil {
			return pathErr
		}
		if err = atomicWrite(path, snapshot.data, 0600); err != nil {
			return err
		}
	}
	wasRunning := m.status(ctx).Running
	if wasRunning {
		if err = m.observe(ctx); err != nil {
			return err
		}
	}
	metadata, err := readBoundedGeoFile(filepath.Join(u.geoSlot(kind), "active.json"), 65536)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	journal := geoJournal{State: "pending", Previous: previous, Metadata: metadata, WasRunning: wasRunning}
	journalPath := filepath.Join(u.geoSlot(kind), "pending.json")
	journalRaw, _ := json.Marshal(journal)
	if err = atomicWrite(journalPath, journalRaw, 0600); err != nil {
		return err
	}
	rollback := func(cause error) error {
		recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), 45*time.Second)
		defer cancel()
		if restore := u.recoverGeo(recovery, kind); restore != nil {
			return fmt.Errorf("%w; geo rollback requires attention: %v", cause, restore)
		}
		return fmt.Errorf("%w; previous geo asset restored", cause)
	}
	if err = atomicWrite(filepath.Join(geoBase(), kind+".dat"), data, 0644); err != nil {
		return rollback(err)
	}
	if wasRunning {
		if err = m.restart(ctx); err != nil {
			return rollback(err)
		}
	}
	recordRaw, _ := json.Marshal(geoRecord{Current: version, Previous: &previous})
	if err = atomicWrite(filepath.Join(u.geoSlot(kind), "active.json"), recordRaw, 0600); err != nil {
		return rollback(err)
	}
	journal.State = "committed"
	journalRaw, _ = json.Marshal(journal)
	if err = atomicWrite(journalPath, journalRaw, 0600); err != nil {
		return rollback(err)
	}
	_ = os.Remove(journalPath)
	u.pruneGeo(kind)
	return nil
}
func (u *CoreUpdater) StartGeo(kind, tag string, rollback bool) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	repo, _ := geoRepository(kind)
	if repo == "" || u.Managers["xray"] == nil {
		return errors.New("choose an available geoip or geosite asset")
	}
	if u.job.State == "running" || u.geoJob.State == "running" {
		return errors.New("another core or geo operation is running")
	}
	if !rollback && (!u.enabled || !releaseTag.MatchString(tag)) {
		return errors.New("enable official downloads and choose a valid release")
	}
	record := u.geoRecord(kind)
	if rollback && record.Previous == nil {
		return errors.New("no previous geo asset is available")
	}
	u.geoJob = geoUpdateJob{Kind: kind, Tag: tag, State: "running", Started: time.Now().UTC().Format(time.RFC3339)}
	if rollback {
		u.geoJob.Tag = record.Previous.Tag
	}
	raw, _ := json.Marshal(u.geoJob)
	if err := atomicWrite(filepath.Join(u.Directory, "geo-job.json"), raw, 0600); err != nil {
		u.geoJob.State = "error"
		return err
	}
	go func() {
		ctx, cancel := context.WithTimeout(u.ctx, 3*time.Minute)
		defer cancel()
		var data []byte
		var version geoVersion
		var err error
		if rollback {
			version = *record.Previous
			var path string
			path, err = u.geoPath(kind, version.SHA256)
			if err == nil {
				data, err = readBoundedGeoFile(path, maxGeoAsset)
			}
		} else {
			data, version, err = u.downloadGeo(ctx, kind, tag)
		}
		if err == nil {
			if u.changeMu != nil {
				u.changeMu.Lock()
			}
			err = u.installGeo(ctx, kind, data, version)
			if u.changeMu != nil {
				u.changeMu.Unlock()
			}
		}
		u.mu.Lock()
		defer u.mu.Unlock()
		u.geoJob.State, u.geoJob.Finished = "success", time.Now().UTC().Format(time.RFC3339)
		if err != nil {
			u.geoJob.State, u.geoJob.Error = "error", err.Error()
		}
		raw, _ := json.Marshal(u.geoJob)
		_ = atomicWrite(filepath.Join(u.Directory, "geo-job.json"), raw, 0600)
	}()
	return nil
}
