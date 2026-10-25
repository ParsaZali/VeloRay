package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProcessRuntimeCoreReplacementAndRollbackWithPinnedBinaries(t *testing.T) {
	if os.Getenv("VELORAY_TEST_NO_NETLINK") == "1" {
		t.Skip("real process ownership inspection requires netlink; exercised by pinned-core CI")
	}
	for _, core := range []string{"xray", "singbox"} {
		t.Run(core, func(t *testing.T) {
			binary := os.Getenv("VELORAY_TEST_XRAY_BINARY")
			service := "xray.service"
			if core == "singbox" {
				binary = os.Getenv("VELORAY_TEST_SINGBOX_BINARY")
				service = "sing-box.service"
			}
			if binary == "" {
				t.Skip("set pinned core binary")
			}
			root := t.TempDir()
			slot := filepath.Join(root, core)
			if err := os.Mkdir(slot, 0700); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(binary)
			if err != nil {
				t.Fatal(err)
			}
			old, next := filepath.Join(slot, "binary-old"), filepath.Join(slot, "binary-next")
			for _, file := range []string{old, next} {
				if err := os.WriteFile(file, raw, 0755); err != nil {
					t.Fatal(err)
				}
			}
			for link, target := range map[string]string{"current": old, "entry": filepath.Join(slot, "current")} {
				if err := os.Symlink(target, filepath.Join(slot, link)); err != nil {
					t.Fatal(err)
				}
			}
			processes := NewProcessRuntime()
			defer processes.Close()
			manager, err := NewManager(filepath.Join(slot, "entry"), filepath.Join(root, "config.json"), service, filepath.Join(root, "state.json"), processes)
			if err != nil {
				t.Fatal(err)
			}
			manager.Core = core
			if err := processes.register(manager); err != nil {
				t.Fatal(err)
			}
			if err := initializeProcessCore(manager); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			updater, err := NewCoreUpdater(ctx, root, map[string]*Manager{core: manager})
			if err != nil {
				t.Fatal(err)
			}
			if err := manager.Restart(ctx); err != nil {
				t.Fatal(err)
			}
			ledger := manager.state.ID
			if err := manager.installCore(ctx, next, CoreRelease{Tag: "copy", Source: "bundled"}); err != nil {
				t.Fatal(err)
			}
			if !manager.status(ctx).Running || manager.state.ID != ledger || manager.coreRecord().PreviousPath != old {
				t.Fatal("replacement lost process health, ledger identity or rollback metadata")
			}
			if err := manager.installCore(ctx, old, CoreRelease{Tag: "original", Source: "bundled"}); err != nil {
				t.Fatal(err)
			}
			current, err := os.Readlink(filepath.Join(slot, "current"))
			if err != nil || current != old || manager.state.ID != ledger {
				t.Fatal("rollback lost executable or ledger", current, err)
			}
			status, _ := json.Marshal(updater.Status())
			if !strings.Contains(string(status), "original") {
				t.Fatal(string(status))
			}
		})
	}
}
