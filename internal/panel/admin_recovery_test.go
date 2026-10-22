package panel

import "testing"

func TestIntegrationAdministratorRecoveryAfterRename(t *testing.T) {
	s, ctx := integration(t)
	if err := s.Store.Bootstrap(ctx, s.Config, "original", "test-password-recovery-2026"); err != nil {
		t.Fatal(err)
	}
	admins, err := list(ctx, s.Store.Pool, "users", "")
	if err != nil {
		t.Fatal(err)
	}
	admin := admins[0]
	admin["username"] = "renamed"
	if err = save(ctx, s.Store.Pool, "users", admin); err != nil {
		t.Fatal(err)
	}
	if name, err := s.Store.AdministratorName(ctx, "original", false); err != nil || name != "renamed" {
		t.Fatal(name, err)
	}
	if _, err = s.Store.AdministratorName(ctx, "original", true); err == nil {
		t.Fatal("explicit missing username silently selected another account")
	}
	if err = s.Store.AdminReset(ctx, "renamed", "replacement-password-2026"); err != nil {
		t.Fatal(err)
	}
	fresh, err := get(ctx, s.Store.Pool, "users", num(admin, "id"))
	if err != nil || num(fresh, "_auth_version") != num(admin, "_auth_version")+1 || str(fresh, "_password") == str(admin, "_password") {
		t.Fatal("password or session generation unchanged", err)
	}
	staff := defaults("users")
	staff["username"] = "staff"
	if err = save(ctx, s.Store.Pool, "users", staff); err != nil {
		t.Fatal(err)
	}
	if err = s.Store.AdminReset(ctx, "staff", "replacement-password-2026"); err == nil {
		t.Fatal("administrator recovery accepted non-administrator")
	}
	if err = s.Store.Bootstrap(ctx, s.Config, "second", "another-strong-password-2026"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Store.AdministratorName(ctx, "missing", false); err == nil {
		t.Fatal("ambiguous recovery silently chose an account")
	}
	if name, err := s.Store.AdministratorName(ctx, "second", false); err != nil || name != "second" {
		t.Fatal(name, err)
	}
}
