package panel

import "testing"

func TestIntegrationContainerBootstrapNeverResetsExistingIdentity(t *testing.T) {
	s, ctx := integration(t)
	if err := s.Store.BootstrapIfEmpty(ctx, s.Config, "admin", "correct-horse-test-2026"); err != nil {
		t.Fatal(err)
	}
	before, err := list(ctx, s.Store.Pool, "users", "")
	if err != nil || len(before) != 1 {
		t.Fatal(before, err)
	}
	if err = s.Store.BootstrapIfEmpty(ctx, s.Config, "different", ""); err != nil {
		t.Fatal("restart tried to replace administrator", err)
	}
	after, _ := list(ctx, s.Store.Pool, "users", "")
	if len(after) != 1 || str(before[0], "_password") != str(after[0], "_password") || str(after[0], "username") != "admin" {
		t.Fatal("administrator changed during repeated bootstrap")
	}
}
