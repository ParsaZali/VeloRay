package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protowire"
)

func geoOfficialFixture(kind, tag string, raw []byte) officialRelease {
	repo, name := geoRepository(kind)
	return officialRelease{Tag: tag, Assets: []releaseAsset{{Name: name, URL: "https://github.com/" + repo + "/releases/download/" + tag + "/" + name, Digest: "sha256:" + geoDigest(raw), Size: int64(len(raw))}}}
}
func TestGeoOfficialDownloadRequiresExactRepositoryDigestSizeAndIndex(t *testing.T) {
	data := geoFixture("IR", 1)
	for _, kind := range []string{"geoip", "geosite"} {
		release := geoOfficialFixture(kind, "20261023064708", data)
		if _, err := geoDownload(kind, release); err != nil {
			t.Fatal(err)
		}
		for _, change := range []func(*officialRelease){
			func(r *officialRelease) { r.Draft = true },
			func(r *officialRelease) { r.Prerelease = true },
			func(r *officialRelease) { r.Tag = "../../bad" },
			func(r *officialRelease) {
				r.Assets[0].URL = "https://github.com/other/project/releases/download/" + r.Tag + "/geoip.dat"
			},
			func(r *officialRelease) { r.Assets[0].Digest = "" },
			func(r *officialRelease) { r.Assets[0].Size = maxGeoAsset + 1 },
		} {
			copy := release
			copy.Assets = append([]releaseAsset(nil), release.Assets...)
			change(&copy)
			if _, err := geoDownload(kind, copy); err == nil {
				t.Fatal("unverified geo metadata accepted", kind)
			}
		}
		for _, scenario := range []string{"success", "bad-hash", "bad-index", "bad-size", "tag-mismatch"} {
			t.Run(kind+"/"+scenario, func(t *testing.T) {
				body := append([]byte(nil), data...)
				meta := geoOfficialFixture(kind, "20261023064708", data)
				switch scenario {
				case "bad-hash":
					body[0] ^= 0xff
				case "bad-index":
					body = []byte{0xff}
					meta = geoOfficialFixture(kind, meta.Tag, body)
				case "bad-size":
					meta.Assets[0].Size++
				case "tag-mismatch":
					meta.Tag = "20261022000000"
				}
				u, err := NewCoreUpdater(context.Background(), t.TempDir(), nil)
				if err != nil {
					t.Fatal(err)
				}
				u.HTTP = &http.Client{Transport: coreTransport(func(r *http.Request) (*http.Response, error) {
					var raw []byte
					if r.URL.Host == "api.github.com" {
						raw, _ = json.Marshal(meta)
					} else if r.URL.String() == release.Assets[0].URL {
						raw = body
					} else {
						t.Errorf("unexpected download: %s", r.URL)
						return nil, errors.New("unexpected URL")
					}
					return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(raw))}, nil
				})}
				_, version, err := u.downloadGeo(context.Background(), kind, release.Tag)
				if (scenario == "success") != (err == nil) {
					t.Fatal(scenario, err)
				}
				if err == nil && version.SHA256 != geoDigest(data) {
					t.Fatal("download hash changed")
				}
			})
		}
	}
}

type geoTestRuntime struct {
	base   *updateRuntime
	mu     sync.Mutex
	reject bool
}

func (r *geoTestRuntime) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if name == "/usr/bin/env" {
		r.mu.Lock()
		reject := r.reject
		r.mu.Unlock()
		if reject {
			return []byte("test invalid routing"), errors.New("invalid staged geo")
		}
		return nil, nil
	}
	return r.base.Run(ctx, name, args...)
}
func geoUpdateFixture(t *testing.T) (*CoreUpdater, *Manager, *geoTestRuntime, []byte) {
	t.Helper()
	m, base, _, _ := coreUpdateFixture(t)
	run := &geoTestRuntime{base: base}
	m.Runtime = run
	dir := t.TempDir()
	t.Setenv("XRAY_LOCATION_ASSET", dir)
	raw := geoFixture("ORIGINAL", 1)
	for _, kind := range []string{"geoip", "geosite"} {
		if err := os.WriteFile(filepath.Join(dir, kind+".dat"), raw, 0644); err != nil {
			t.Fatal(err)
		}
	}
	u, err := NewCoreUpdater(context.Background(), filepath.Dir(m.releaseSlot), map[string]*Manager{"xray": m})
	if err != nil {
		t.Fatal(err)
	}
	return u, m, run, raw
}
func TestGeoInstallStagesBeforeMutationAndRestoresOnRestartFailure(t *testing.T) {
	for _, scenario := range []string{"success", "validation-failure", "restart-failure"} {
		t.Run(scenario, func(t *testing.T) {
			u, m, run, original := geoUpdateFixture(t)
			ledger := m.state.ID
			if scenario == "validation-failure" {
				run.reject = true
			}
			if scenario == "restart-failure" {
				run.base.fail = true
			}
			next := geoFixture("UPDATED", 2)
			err := u.installGeo(context.Background(), "geosite", next, geoVersion{Tag: "20261023", Repository: "v2fly/domain-list-community", SHA256: geoDigest(next)})
			if (scenario == "success") != (err == nil) {
				t.Fatal(scenario, err)
			}
			want := original
			if scenario == "success" {
				want = next
				record := u.geoRecord("geosite")
				if record.Previous == nil || record.Previous.SHA256 != geoDigest(original) || record.Current.SHA256 != geoDigest(next) {
					t.Fatal("rollback snapshot missing", record)
				}
			}
			current, err := os.ReadFile(filepath.Join(geoBase(), "geosite.dat"))
			if err != nil || !bytes.Equal(current, want) || m.state.ID != ledger {
				t.Fatal("live file or accounting identity changed incorrectly", err)
			}
			if scenario == "validation-failure" && run.base.restarts != 0 {
				t.Fatal("failed validation restarted the running core")
			}
			if scenario != "validation-failure" && m.state.Counters["user>>>test>>>traffic>>>uplink"].Total < 10 {
				t.Fatal("pre-restart traffic was discarded")
			}
			if _, err := os.Stat(filepath.Join(u.geoSlot("geosite"), "pending.json")); !os.IsNotExist(err) {
				t.Fatal("completed journal retained", err)
			}
		})
	}
}
func TestGeoInterruptedRecoveryAndPreviousSnapshotVerification(t *testing.T) {
	for _, corrupted := range []bool{false, true} {
		t.Run(map[bool]string{false: "valid-backup", true: "corrupt-backup"}[corrupted], func(t *testing.T) {
			u, m, _, original := geoUpdateFixture(t)
			path, _ := u.geoPath("geosite", geoDigest(original))
			backup := original
			if corrupted {
				backup = []byte("corrupt")
			}
			if err := atomicWrite(path, backup, 0600); err != nil {
				t.Fatal(err)
			}
			journal, _ := json.Marshal(geoJournal{State: "pending", Previous: geoVersion{SHA256: geoDigest(original), Tag: "installed"}, WasRunning: true})
			if err := atomicWrite(filepath.Join(u.geoSlot("geosite"), "pending.json"), journal, 0600); err != nil {
				t.Fatal(err)
			}
			next := geoFixture("INTERRUPTED", 2)
			_ = os.WriteFile(filepath.Join(geoBase(), "geosite.dat"), next, 0644)
			job, _ := json.Marshal(geoUpdateJob{Kind: "geosite", Tag: "20261023", State: "running"})
			_ = atomicWrite(filepath.Join(u.Directory, "geo-job.json"), job, 0600)
			again, err := NewCoreUpdater(context.Background(), u.Directory, map[string]*Manager{"xray": m})
			if corrupted {
				if err == nil {
					t.Fatal("corrupt recovery snapshot accepted")
				}
				current, _ := os.ReadFile(filepath.Join(geoBase(), "geosite.dat"))
				if !bytes.Equal(current, next) {
					t.Fatal("corrupt recovery mutated the live file")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			current, _ := os.ReadFile(filepath.Join(geoBase(), "geosite.dat"))
			if !bytes.Equal(current, original) || again.geoJob.State != "error" || !strings.Contains(again.geoJob.Error, "interrupted") {
				t.Fatal("interrupted update was not recovered", again.geoJob, err)
			}
		})
	}
}
func waitGeoJob(t *testing.T, u *CoreUpdater) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		u.mu.Lock()
		job := u.geoJob
		u.mu.Unlock()
		if job.State != "running" {
			if job.State != "success" {
				t.Fatal(job)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("geo update did not finish")
}
func TestGeoRollbackRetainsOnlyTwoSnapshotsAndSameVersionIsIdempotent(t *testing.T) {
	u, _, _, _ := geoUpdateFixture(t)
	for _, tag := range []string{"20261021", "20261022", "20261023"} {
		raw := geoFixture(tag, 1)
		if err := u.installGeo(context.Background(), "geosite", raw, geoVersion{Tag: tag, Repository: "v2fly/domain-list-community", SHA256: geoDigest(raw)}); err != nil {
			t.Fatal(err)
		}
	}
	before := u.geoRecord("geosite")
	raw := geoFixture("20261023", 1)
	if err := u.installGeo(context.Background(), "geosite", raw, before.Current); err != nil {
		t.Fatal(err)
	}
	if u.geoRecord("geosite").Previous.SHA256 != before.Previous.SHA256 {
		t.Fatal("same version lost the rollback target")
	}
	entries, _ := os.ReadDir(u.geoSlot("geosite"))
	count := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "asset-") {
			count++
		}
	}
	if count != 2 {
		t.Fatal("unexpected unbounded geo history", count)
	}
	if err := u.StartGeo("geosite", "", true); err != nil {
		t.Fatal("rollback must work with downloads disabled", err)
	}
	waitGeoJob(t, u)
	after := u.geoRecord("geosite")
	if after.Current.Tag != "20261022" || after.Previous.Tag != "20261023" {
		t.Fatal(after)
	}
	u.geoJob.State = "running"
	if err := u.Start("xray", "v26.3.27", false, false); err == nil {
		t.Fatal("core update overlapped geo update")
	}
	if err := u.SetEnabled(true); err == nil {
		t.Fatal("policy changed during geo update")
	}
	u.geoJob.State = ""
	u.job.State = "running"
	if err := u.StartGeo("geoip", "20261023", false); err == nil {
		t.Fatal("geo update overlapped core update")
	}
	u.job.State = ""
	if err := u.StartGeo("geoip", "20261023", false); err == nil {
		t.Fatal("disabled downloads bypassed")
	}
	for _, path := range []string{"../../bad", strings.Repeat("a", 63), strings.Repeat("A", 64)} {
		if _, err := u.geoPath("geoip", path); err == nil {
			t.Fatal("unsafe managed snapshot accepted")
		}
	}
}

type realGeoRuntime struct{}

func (realGeoRuntime) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if name == "systemctl" {
		return []byte("inactive"), errors.New("not running in test")
	}
	return (Commands{}).Run(ctx, name, args...)
}
func realGeoSite(tag string, payload []byte) []byte {
	entry := protowire.AppendString(protowire.AppendTag(nil, 1, protowire.BytesType), strings.ToUpper(tag))
	entry = protowire.AppendBytes(protowire.AppendTag(entry, 2, protowire.BytesType), payload)
	return protowire.AppendBytes(protowire.AppendTag(nil, 1, protowire.BytesType), entry)
}
func TestRealXrayGeoValidationRejectsInvalidUnusedInnerPayload(t *testing.T) {
	binary := os.Getenv("VELORAY_TEST_XRAY_BINARY")
	if binary == "" {
		t.Skip("set VELORAY_TEST_XRAY_BINARY")
	}
	root := t.TempDir()
	t.Setenv("XRAY_LOCATION_ASSET", root)
	domain := protowire.AppendVarint(protowire.AppendTag(nil, 1, protowire.VarintType), 2)
	domain = protowire.AppendString(protowire.AppendTag(domain, 2, protowire.BytesType), "example.com")
	original := realGeoSite("original", domain)
	for _, kind := range []string{"geoip", "geosite"} {
		_ = os.WriteFile(filepath.Join(root, kind+".dat"), original, 0644)
	}
	config := filepath.Join(root, "config.json")
	_ = os.WriteFile(config, []byte("{\"inbounds\":[],\"outbounds\":[{\"protocol\":\"freedom\"}]}"), 0600)
	m, err := NewManager(binary, config, "uninstalled-test-service", filepath.Join(root, "state.json"), realGeoRuntime{})
	if err != nil {
		t.Fatal(err)
	}
	u, err := NewCoreUpdater(context.Background(), filepath.Join(root, "managed"), map[string]*Manager{"xray": m})
	if err != nil {
		t.Fatal(err)
	}
	valid := realGeoSite("valid", domain)
	if err = u.installGeo(context.Background(), "geosite", valid, geoVersion{Tag: "20261023", Repository: "v2fly/domain-list-community", SHA256: geoDigest(valid)}); err != nil {
		t.Fatal("real Xray rejected valid asset", err)
	}
	invalid := realGeoSite("unused-broken-tag", []byte{0xff})
	if _, err = indexGeoAsset(context.Background(), "geosite", invalid); err != nil {
		t.Fatal("test requires a valid outer index", err)
	}
	if err = u.installGeo(context.Background(), "geosite", invalid, geoVersion{Tag: "20261024", SHA256: geoDigest(invalid)}); err == nil {
		t.Fatal("Xray did not parse every tag")
	}
	current, _ := os.ReadFile(filepath.Join(root, "geosite.dat"))
	if !bytes.Equal(current, valid) {
		t.Fatal("invalid inner protobuf replaced live geo data")
	}
}

func TestRealXrayChecksCompletePinnedGeoInventories(t *testing.T) {
	binary, assets := os.Getenv("VELORAY_TEST_XRAY_BINARY"), os.Getenv("VELORAY_TEST_GEO_ASSETS")
	if binary == "" || assets == "" {
		t.Skip("set pinned Xray and geo asset paths")
	}
	root := t.TempDir()
	t.Setenv("XRAY_LOCATION_ASSET", assets)
	config := filepath.Join(root, "config.json")
	if err := os.WriteFile(config, []byte("{\"inbounds\":[],\"outbounds\":[{\"protocol\":\"freedom\"}]}"), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := NewManager(binary, config, "uninstalled-geo-test", filepath.Join(root, "state.json"), realGeoRuntime{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	for _, kind := range []string{"geoip", "geosite"} {
		raw, err := readBoundedGeoFile(filepath.Join(assets, kind+".dat"), maxGeoAsset)
		if err != nil {
			t.Fatal(err)
		}
		if err = validateGeo(ctx, m, kind, raw); err != nil {
			t.Fatal(kind, err)
		}
	}
}
