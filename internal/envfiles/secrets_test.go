package envfiles

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecretFilesBoundedPrivateAndIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret")
	for _, key := range []string{"VELORAY_SECRET_KEY", "VELORAY_FIELD_KEY"} {
		t.Setenv(key, "")
		t.Setenv(key+"_FILE", "")
	}
	t.Setenv("VELORAY_SECRET_KEY_FILE", path)
	if err := os.WriteFile(path, []byte("private-test-value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := LoadSecrets(); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("VELORAY_SECRET_KEY") != "private-test-value" {
		t.Fatal("secret not loaded")
	}
	if err := LoadSecrets(); err != nil {
		t.Fatal("repeated load failed", err)
	}
	for _, content := range []string{"", "one\ntwo", "zero\x00byte", strings.Repeat("x", 16385)} {
		os.WriteFile(path, []byte(content), 0600)
		if _, err := secretFile(path); err == nil {
			t.Fatal("invalid secret accepted")
		}
	}
	t.Setenv("VELORAY_SECRET_KEY_FILE", path)
	if err := LoadSecrets(); err == nil || strings.Contains(err.Error(), "private-test-value") {
		t.Fatal("ambiguous secret configuration or secret leaked", err)
	}
	if _, err := secretFile(t.TempDir()); err == nil {
		t.Fatal("directory accepted")
	}
}
