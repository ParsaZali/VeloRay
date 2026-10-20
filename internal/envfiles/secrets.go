package envfiles

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// LoadSecrets supports read-only Docker/Kubernetes secret mounts. Only named
// credential variables participate; content and filenames are never logged.
func LoadSecrets() error {
	for _, key := range []string{"POSTGRES_PASSWORD", "DATABASE_URL", "VELORAY_SECRET_KEY", "VELORAY_FIELD_KEY", "VELORAY_ADMIN_PASSWORD", "VELORAY_AGENT_TOKEN", "VELORAY_LOCAL_NODE_TOKEN", "VELORAY_OIDC_CLIENT_SECRET","VELORAY_GITHUB_CLIENT_SECRET"} {
		path := os.Getenv(key + "_FILE")
		if path == "" {
			continue
		}
		if os.Getenv(key) != "" {
			return fmt.Errorf("set only one of %s and %s_FILE", key, key)
		}
		value, err := secretFile(path)
		if err != nil {
			return fmt.Errorf("cannot load %s_FILE: %w", key, err)
		}
		if err = os.Setenv(key, value); err != nil {
			return err
		}
		// Make repeated configuration loads idempotent in this process.
		if err = os.Unsetenv(key + "_FILE"); err != nil {
			return err
		}
	}
	return nil
}
func secretFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 16384 {
		return "", errors.New("secret must be a regular file of at most 16 KiB")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", errors.New("secret file unavailable")
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 16384 {
		return "", errors.New("secret must be a regular file of at most 16 KiB")
	}
	b, err := io.ReadAll(io.LimitReader(f, 16385))
	if err != nil || len(b) > 16384 {
		return "", errors.New("cannot read bounded secret")
	}
	value := strings.TrimSuffix(strings.TrimSuffix(string(b), "\n"), "\r")
	if value == "" || strings.ContainsAny(value, "\x00\r\n") {
		return "", errors.New("secret must contain one nonempty line")
	}
	return value, nil
}
