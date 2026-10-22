package agent

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func testELF(arch string) []byte {
	b := make([]byte, 64)
	copy(b, "\x7fELF")
	b[4], b[5] = 2, 1
	machine := uint16(62)
	if arch == "arm64" {
		machine = 183
	}
	binary.LittleEndian.PutUint16(b[18:20], machine)
	return b
}
func coreZip(t *testing.T, name string, body []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	w, err := z.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write(body)
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestOfficialCoreAssetsAndArchiveValidation(t *testing.T) {
	raw := coreZip(t, "xray", testELF("amd64"))
	sum := sha256.Sum256(raw)
	release := officialRelease{Tag: "v1.2.3", Assets: []releaseAsset{{Name: "Xray-linux-64.zip", URL: "https://github.com/XTLS/Xray-core/releases/download/v1.2.3/Xray-linux-64.zip", Digest: "sha256:" + hex.EncodeToString(sum[:]), Size: int64(len(raw))}}}
	if _, err := releaseDownload("xray", "amd64", release); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*officialRelease){func(r *officialRelease) { r.Prerelease = true }, func(r *officialRelease) { r.Draft = true }, func(r *officialRelease) { r.Assets[0].Digest = "" }, func(r *officialRelease) { r.Assets[0].URL = "https://example.com/xray" }, func(r *officialRelease) { r.Tag = "../../bad" }} {
		copy := release
		copy.Assets = append([]releaseAsset(nil), release.Assets...)
		change(&copy)
		if _, err := releaseDownload("xray", "amd64", copy); err == nil {
			t.Fatal("unsafe release accepted")
		}
	}
	if _, err := extractCore("xray", "v1.2.3", "amd64", raw); err != nil {
		t.Fatal(err)
	}
	for _, archive := range [][]byte{coreZip(t, "../xray", testELF("amd64")), coreZip(t, "xray", testELF("arm64")), coreZip(t, "xray", []byte("not executable"))} {
		if _, err := extractCore("xray", "v1.2.3", "amd64", archive); err == nil {
			t.Fatal("unsafe executable accepted")
		}
	}
	for _, host := range []string{"http://github.com/file", "https://127.0.0.1/file", "https://github.com.evil.test/file"} {
		r, _ := http.NewRequest("GET", host, nil)
		if officialRedirect(r, nil) == nil {
			t.Fatal("unsafe redirect accepted")
		}
	}
}

type updateRuntime struct {
	mu              sync.Mutex
	epoch, restarts int
	fail            bool
}

func (r *updateRuntime) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if name == "systemctl" {
		switch args[0] {
		case "is-active":
			return []byte("active"), nil
		case "show":
			return []byte("MainPID=123\nExecMainStartTimestampMonotonic=" + strings.Repeat("1", r.epoch+1)), nil
		case "restart":
			r.restarts++
			if r.fail {
				r.fail = false
				return []byte("test restart failure"), errors.New("restart failed")
			}
			r.epoch++
		}
		return nil, nil
	}
	if name == "ss" {
		return nil, nil
	}
	if len(args) > 0 && args[0] == "version" {
		return []byte("Xray 1.2.3"), nil
	}
	if len(args) > 0 && args[0] == "api" {
		return []byte(`{"stat":[{"name":"user>>>test>>>traffic>>>uplink","value":10}]}`), nil
	}
	if data, err := os.ReadFile(name); err == nil && bytes.Contains(data, []byte("reject")) {
		return []byte("configuration rejected"), errors.New("invalid config")
	}
	return nil, nil
}
func coreUpdateFixture(t *testing.T) (*Manager, *updateRuntime, string, string) {
	t.Helper()
	root := t.TempDir()
	slot := filepath.Join(root, "xray")
	if err := os.Mkdir(slot, 0755); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(slot, "binary-old")
	next := filepath.Join(slot, "binary-new")
	for _, name := range []string{old, next} {
		if err := os.WriteFile(name, testELF("amd64"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	current := filepath.Join(slot, "current")
	if err := os.Symlink(old, current); err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(root, "xray-link")
	if err := os.Symlink(current, entry); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(root, "config.json")
	if err := os.WriteFile(config, []byte(`{"inbounds":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	run := &updateRuntime{}
	m, err := NewManager(entry, config, "xray.service", filepath.Join(root, "state.json"), run)
	if err != nil {
		t.Fatal(err)
	}
	m.releaseSlot = slot
	return m, run, old, next
}
func TestCoreInstallValidationRollbackAndRecovery(t *testing.T) {
	for _, scenario := range []string{"success", "config-rejected", "restart-failed", "interrupted"} {
		t.Run(scenario, func(t *testing.T) {
			m, run, old, next := coreUpdateFixture(t)
			ledger := m.state.ID
			record := CoreRelease{Tag: "v1.2.3", Source: "official"}
			switch scenario {
			case "config-rejected":
				_ = os.WriteFile(next, []byte("reject"), 0755)
			case "restart-failed":
				run.fail = true
			case "interrupted":
				journal, _ := json.Marshal(coreJournal{State: "pending", OldPath: old, WasRunning: true})
				if err := atomicWrite(filepath.Join(m.releaseSlot, "pending.json"), journal, 0600); err != nil {
					t.Fatal(err)
				}
				if err := m.swapCore(next); err != nil {
					t.Fatal(err)
				}
				if err := m.recoverCore(context.Background()); err != nil {
					t.Fatal(err)
				}
			}
			if scenario != "interrupted" {
				err := m.installCore(context.Background(), next, record)
				if (scenario == "success") != (err == nil) {
					t.Fatal(scenario, err)
				}
			}
			current, err := os.Readlink(filepath.Join(m.releaseSlot, "current"))
			if err != nil {
				t.Fatal(err)
			}
			want := old
			if scenario == "success" {
				want = next
				if m.coreRecord().PreviousPath != old || m.coreRecord().SHA256 == "" {
					t.Fatal("rollback metadata missing")
				}
			}
			if current != want || m.state.ID != ledger {
				t.Fatal("current executable or durable ledger changed incorrectly")
			}
			if _, err = os.Stat(filepath.Join(m.releaseSlot, "pending.json")); !os.IsNotExist(err) {
				t.Fatal("completed operation journal retained", err)
			}
			if scenario == "config-rejected" && run.restarts != 0 {
				t.Fatal("validation failure restarted production core")
			}
		})
	}
}

type coreTransport func(*http.Request) (*http.Response, error)

func (f coreTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestOfficialDownloadDigestAndDisabledPolicy(t *testing.T) {
	m, _, _, _ := coreUpdateFixture(t)
	u, err := NewCoreUpdater(context.Background(), filepath.Dir(m.releaseSlot), map[string]*Manager{"xray": m})
	if err != nil {
		t.Fatal(err)
	}
	u.Arch = "amd64"
	if err = u.Start("xray", "v1.2.3", false, false); err == nil {
		t.Fatal("disabled downloads accepted")
	}
	raw := coreZip(t, "xray", testELF("amd64"))
	sum := sha256.Sum256(raw)
	digest := hex.EncodeToString(sum[:])
	corrupt := false
	u.HTTP = &http.Client{Transport: coreTransport(func(r *http.Request) (*http.Response, error) {
		var body []byte
		if r.URL.Host == "api.github.com" {
			body, _ = json.Marshal(officialRelease{Tag: "v1.2.3", Assets: []releaseAsset{{Name: "Xray-linux-64.zip", URL: "https://github.com/XTLS/Xray-core/releases/download/v1.2.3/Xray-linux-64.zip", Digest: "sha256:" + digest, Size: int64(len(raw))}}})
		} else {
			body = append([]byte(nil), raw...)
			if corrupt {
				body[0] ^= 1
			}
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}, nil
	})}
	if _, _, err = u.download(context.Background(), "xray", "v1.2.3"); err != nil {
		t.Fatal(err)
	}
	corrupt = true
	if _, _, err = u.download(context.Background(), "xray", "v1.2.3"); err == nil {
		t.Fatal("corrupt archive accepted")
	}
	if err = u.SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	again, err := NewCoreUpdater(context.Background(), u.Directory, map[string]*Manager{"xray": m})
	if err != nil || !again.enabled {
		t.Fatal("download policy did not persist", err)
	}
	if err = again.Start("xray", "../invalid", false, false); err == nil {
		t.Fatal("unsafe version accepted")
	}
}

func TestOfficialSingBoxPreservesIPLimitEnforcement(t *testing.T) {
	m, _, _, _ := coreUpdateFixture(t)
	u, err := NewCoreUpdater(context.Background(), filepath.Dir(m.releaseSlot), map[string]*Manager{"singbox": m})
	if err != nil {
		t.Fatal(err)
	}
	if err = u.SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	if err = u.Start("singbox", "v1.14.3", false, true); err == nil {
		t.Fatal("official binary accepted while user IP caps require bundled attribution")
	}
	raw, _ := json.Marshal(CoreRelease{Source: "official", PreviousSource: "official", PreviousPath: filepath.Join(m.releaseSlot, "binary-previous")})
	if err = os.MkdirAll(m.releaseSlot, 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(m.releaseSlot, "active.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	if u.SupportsUserIPs() {
		t.Fatal("official binary advertised unsupported user IP attribution")
	}
	if err = u.Start("singbox", "", true, true); err == nil {
		t.Fatal("rollback to official binary bypassed user IP caps")
	}
}
