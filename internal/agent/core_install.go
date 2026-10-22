package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (m *Manager) coreRecord() CoreRelease {
	record := CoreRelease{Tag: "bundled", Source: "bundled"}
	if m.releaseSlot != "" {
		if raw, err := os.ReadFile(filepath.Join(m.releaseSlot, "active.json")); err == nil {
			_ = json.Unmarshal(raw, &record)
		}
	}
	return record
}
func (m *Manager) safeCorePath(candidate string) bool {
	return m.releaseSlot != "" && filepath.Dir(candidate) == m.releaseSlot && strings.HasPrefix(filepath.Base(candidate), "binary-")
}
func (m *Manager) swapCore(candidate string) error {
	if !m.safeCorePath(candidate) {
		return errors.New("executable is outside the managed core slot")
	}
	info, err := os.Lstat(candidate)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("managed executable must be a regular file")
	}
	f, err := os.CreateTemp(m.releaseSlot, ".link-")
	if err != nil {
		return err
	}
	temporary := f.Name()
	_ = f.Close()
	_ = os.Remove(temporary)
	defer os.Remove(temporary)
	if err = os.Symlink(candidate, temporary); err != nil {
		return err
	}
	return os.Rename(temporary, filepath.Join(m.releaseSlot, "current"))
}
func (m *Manager) recoverCore(ctx context.Context) error {
	journalPath := filepath.Join(m.releaseSlot, "pending.json")
	raw, err := os.ReadFile(journalPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var journal coreJournal
	if err = json.Unmarshal(raw, &journal); err != nil {
		return err
	}
	if journal.State == "committed" {
		return os.Remove(journalPath)
	}
	if !m.safeCorePath(journal.OldPath) {
		return errors.New("invalid core recovery journal")
	}
	if m.status(ctx).Running {
		_ = m.observe(ctx)
	}
	if err = m.swapCore(journal.OldPath); err != nil {
		return err
	}
	if len(journal.OldMetadata) > 0 {
		err = atomicWrite(filepath.Join(m.releaseSlot, "active.json"), journal.OldMetadata, 0644)
	} else {
		err = os.Remove(filepath.Join(m.releaseSlot, "active.json"))
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
func (m *Manager) installCore(ctx context.Context, candidate string, record CoreRelease) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.safeCorePath(candidate) {
		return errors.New("invalid managed executable")
	}
	link, err := os.Readlink(m.Binary)
	if err != nil || link != filepath.Join(m.releaseSlot, "current") {
		return errors.New("upgrade the node installation to enable managed core slots")
	}
	old, err := os.Readlink(filepath.Join(m.releaseSlot, "current"))
	if err != nil || !m.safeCorePath(old) {
		return errors.New("current executable is not a managed slot")
	}
	args := []string{"run", "-test", "-config", m.ConfigPath}
	if m.Core == "singbox" {
		args = []string{"check", "-c", m.ConfigPath}
	}
	if out, err := m.Runtime.Run(ctx, candidate, args...); err != nil {
		return fmt.Errorf("selected core rejected the current configuration: %.1500s", out)
	}
	if record.Source == "official" {
		out, err := m.Runtime.Run(ctx, candidate, "version")
		if err != nil || !strings.Contains(string(out), strings.TrimPrefix(record.Tag, "v")) {
			return errors.New("downloaded executable version does not match the selected release")
		}
	}
	wasRunning := m.status(ctx).Running
	if wasRunning {
		if err = m.observe(ctx); err != nil {
			return err
		}
	}
	metadata, _ := os.ReadFile(filepath.Join(m.releaseSlot, "active.json"))
	previous := m.coreRecord()
	journal := coreJournal{State: "pending", OldPath: old, OldMetadata: metadata, WasRunning: wasRunning}
	raw, _ := json.Marshal(journal)
	journalPath := filepath.Join(m.releaseSlot, "pending.json")
	if err = atomicWrite(journalPath, raw, 0600); err != nil {
		return err
	}
	rollback := func(cause error) error {
		recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		if restore := m.recoverCore(recovery); restore != nil {
			return fmt.Errorf("%w; rollback requires attention: %v", cause, restore)
		}
		return fmt.Errorf("%w; previous core restored", cause)
	}
	if err = m.swapCore(candidate); err != nil {
		return rollback(err)
	}
	if wasRunning {
		if err = m.restart(ctx); err != nil {
			return rollback(err)
		}
	}
	data, err := os.ReadFile(candidate)
	if err != nil {
		return rollback(err)
	}
	checksum := sha256.Sum256(data)
	record.SHA256 = hex.EncodeToString(checksum[:])
	record.PreviousPath, record.PreviousTag, record.PreviousSource = old, previous.Tag, previous.Source
	raw, _ = json.Marshal(record)
	if err = atomicWrite(filepath.Join(m.releaseSlot, "active.json"), raw, 0644); err != nil {
		return rollback(err)
	}
	journal.State = "committed"
	raw, _ = json.Marshal(journal)
	if err = atomicWrite(journalPath, raw, 0600); err != nil {
		return rollback(err)
	}
	_ = os.Remove(journalPath)
	return nil
}
