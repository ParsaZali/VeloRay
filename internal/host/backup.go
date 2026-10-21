// Package host implements offline installation maintenance. It is never exposed by HTTP.
package host

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

var Roots = []string{"etc/veloray", "etc/veloray-node", "usr/local/etc/xray", "usr/local/etc/sing-box", "var/lib/veloray-agent", "etc/nginx/sites-available/veloray", "etc/systemd/system/veloray-subscriptions.service", "etc/systemd/system/veloray-subscription-endpoint.service", "etc/systemd/system/veloray-subscription-endpoint.timer", "etc/systemd/system/veloray-web.service", "etc/systemd/system/veloray-agent.service", "etc/systemd/system/xray.service", "etc/systemd/system/sing-box.service", "etc/letsencrypt"}

type Entry struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	Mode   int64  `json:"mode"`
}
type Manifest struct {
	Format  int              `json:"format"`
	Version string           `json:"version"`
	Created time.Time        `json:"created"`
	Files   map[string]Entry `json:"files"`
}

func command(ctx context.Context, name string, args []string, dsn string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cfg, e := pgx.ParseConfig(dsn)
	if e != nil {
		return e
	}
	cmd.Env = append(os.Environ(), "PGDATABASE="+cfg.Database, "PGHOST="+cfg.Host, fmt.Sprintf("PGPORT=%d", cfg.Port), "PGUSER="+cfg.User, "PGPASSWORD="+cfg.Password)
	if cfg.TLSConfig == nil {
		cmd.Env = append(cmd.Env, "PGSSLMODE=disable")
	} else {
		cmd.Env = append(cmd.Env, "PGSSLMODE=require")
	}
	// Preserve libpq TLS policy and certificate paths from URI connection strings.
	if uri, err := url.Parse(dsn); err == nil && (uri.Scheme == "postgres" || uri.Scheme == "postgresql") {
		query := uri.Query()
		for parameter, variable := range map[string]string{"sslmode": "PGSSLMODE", "sslrootcert": "PGSSLROOTCERT", "sslcert": "PGSSLCERT", "sslkey": "PGSSLKEY", "sslcrl": "PGSSLCRL"} {
			if value := query.Get(parameter); value != "" {
				cmd.Env = append(cmd.Env, variable+"="+value)
			}
		}
	}
	if name == "pg_restore" {
		restoring := false
		for _, arg := range args {
			if arg == "--single-transaction" {
				restoring = true
			}
		}
		if restoring {
			cmd.Args = append(cmd.Args, "--dbname="+cfg.Database)
		}
	}
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
func Backup(ctx context.Context, root, path, dsn, version string) error {
	tmp, e := os.MkdirTemp("", "veloray-backup-*")
	if e != nil {
		return e
	}
	defer os.RemoveAll(tmp)
	dump := filepath.Join(tmp, "database.dump")
	if e = command(ctx, "pg_dump", []string{"--format=custom", "--no-owner", "--no-acl", "--file", dump}, dsn); e != nil {
		return fmt.Errorf("database backup failed: %w", e)
	}
	paths := map[string]string{"database.dump": dump}
	for _, r := range Roots {
		full := filepath.Join(root, r)
		if _, e = os.Lstat(full); os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return e
		}
		if e = filepath.WalkDir(full, func(p string, d os.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.Type()&os.ModeSymlink != 0 {
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if !d.Type().IsRegular() {
				return nil
			}
			rel, e := filepath.Rel(root, p)
			if e != nil {
				return e
			}
			paths[filepath.ToSlash(rel)] = p
			return nil
		}); e != nil {
			return e
		}
	}
	if _, ok := paths["etc/veloray/veloray.env"]; !ok {
		return errors.New("environment file is required in a complete backup")
	}
	m := Manifest{1, version, time.Now().UTC(), map[string]Entry{}}
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	ok := false
	defer func() {
		f.Close()
		if !ok {
			os.Remove(path)
		}
	}()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for name, p := range paths {
		raw, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		sum := sha256.Sum256(raw)
		info, e := os.Stat(p)
		if e != nil {
			return e
		}
		mode := int64(info.Mode().Perm())
		m.Files[name] = Entry{hex.EncodeToString(sum[:]), int64(len(raw)), mode}
		if e = tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(raw)), Typeflag: tar.TypeReg}); e != nil {
			return e
		}
		if _, e = tw.Write(raw); e != nil {
			return e
		}
	}
	raw, _ := json.MarshalIndent(m, "", "  ")
	if e = tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0600, Size: int64(len(raw)), Typeflag: tar.TypeReg}); e != nil {
		return e
	}
	if _, e = tw.Write(raw); e != nil {
		return e
	}
	if e = tw.Close(); e != nil {
		return e
	}
	if e = gz.Close(); e != nil {
		return e
	}
	if e = f.Sync(); e != nil {
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	ok = true
	return nil
}
func allowedPath(name string) bool {
	if name == "manifest.json" || name == "database.dump" {
		return true
	}
	for _, r := range Roots {
		if name == r || strings.HasPrefix(name, r+"/") {
			return true
		}
	}
	return false
}

// Verify extracts only regular files into a private staging directory; no links or traversal.
func Verify(path string) (string, Manifest, error) {
	tmp, e := os.MkdirTemp("", "veloray-restore-*")
	if e != nil {
		return "", Manifest{}, e
	}
	success := false
	defer func() {
		if !success {
			os.RemoveAll(tmp)
		}
	}()
	f, e := os.Open(path)
	if e != nil {
		return "", Manifest{}, e
	}
	defer f.Close()
	gz, e := gzip.NewReader(f)
	if e != nil {
		return "", Manifest{}, e
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	seen := map[string]bool{}
	var total int64
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return "", Manifest{}, e
		}
		name := h.Name
		if h.Typeflag != tar.TypeReg || name != filepath.ToSlash(filepath.Clean(name)) || filepath.IsAbs(name) || strings.HasPrefix(name, "../") || !allowedPath(name) || seen[name] || h.Size < 0 || h.Size > 4<<30 {
			return "", Manifest{}, errors.New("unsafe backup member")
		}
		seen[name] = true
		total += h.Size
		if total > 8<<30 {
			return "", Manifest{}, errors.New("backup exceeds size limit")
		}
		p := filepath.Join(tmp, name)
		if e = os.MkdirAll(filepath.Dir(p), 0700); e != nil {
			return "", Manifest{}, e
		}
		out, e := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return "", Manifest{}, e
		}
		_, e = io.Copy(out, tr)
		closeErr := out.Close()
		if e == nil {
			e = closeErr
		}
		if e != nil {
			return "", Manifest{}, e
		}
	}
	raw, e := os.ReadFile(filepath.Join(tmp, "manifest.json"))
	if e != nil {
		return "", Manifest{}, e
	}
	var m Manifest
	if e = json.Unmarshal(raw, &m); e != nil || m.Format != 1 {
		return "", m, errors.New("unsupported backup manifest")
	}
	if len(seen) != len(m.Files)+1 || !seen["database.dump"] || !seen["etc/veloray/veloray.env"] {
		return "", m, errors.New("incomplete backup")
	}
	for name, entry := range m.Files {
		if !seen[name] || name == "manifest.json" {
			return "", m, errors.New("manifest mismatch")
		}
		f, e := os.Open(filepath.Join(tmp, name))
		if e != nil {
			return "", m, e
		}
		hash := sha256.New()
		size, e := io.Copy(hash, f)
		f.Close()
		if e != nil || size != entry.Size || hex.EncodeToString(hash.Sum(nil)) != entry.SHA256 {
			return "", m, fmt.Errorf("backup integrity check failed: %s", name)
		}
	}
	success = true
	return tmp, m, nil
}
func AtomicFile(path string, raw []byte, mode os.FileMode) error {
	uid, gid := -1, -1
	if info, err := os.Stat(path); err == nil {
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			uid, gid = int(stat.Uid), int(stat.Gid)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".veloray-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if e = f.Chown(uid, gid); e == nil {
		e = f.Chmod(mode)
	}
	if e == nil {
		_, e = f.Write(raw)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return os.Rename(f.Name(), path)
}
func Restore(ctx context.Context, root, path, dsn string) error {
	tmp, m, e := Verify(path)
	if e != nil {
		return e
	}
	defer os.RemoveAll(tmp)
	if _, e = exec.LookPath("pg_restore"); e != nil {
		return e
	} // Validate dump before touching the current database.
	if e = command(ctx, "pg_restore", []string{"--list", filepath.Join(tmp, "database.dump")}, dsn); e != nil {
		return fmt.Errorf("invalid database dump: %w", e)
	}
	// Refuse symlink destinations, including parent directories. Certificate live/ links are rebuilt separately.
	for name := range m.Files {
		if name == "database.dump" {
			continue
		}
		p := filepath.Join(root, name)
		for parent := p; parent != root && parent != "/"; parent = filepath.Dir(parent) {
			if st, e := os.Lstat(parent); e == nil && st.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("restore destination is a symlink: %s", parent)
			}
		}
	}
	if e = command(ctx, "pg_restore", []string{"--clean", "--if-exists", "--single-transaction", "--no-owner", "--no-acl", filepath.Join(tmp, "database.dump")}, dsn); e != nil {
		return fmt.Errorf("database restore failed; transaction rolled back: %w", e)
	}
	for name, entry := range m.Files {
		if name == "database.dump" {
			continue
		}
		raw, e := os.ReadFile(filepath.Join(tmp, name))
		if e != nil {
			return e
		}
		mode := os.FileMode(entry.Mode) & 0777
		if strings.HasPrefix(name, "etc/veloray") || strings.HasPrefix(name, "var/lib/veloray-agent") {
			mode = 0600
		}
		if e = AtomicFile(filepath.Join(root, name), raw, mode); e != nil {
			return fmt.Errorf("database restored but file restoration failed (%s): %w", name, e)
		}
	}
	return nil
}
