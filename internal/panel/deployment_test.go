package panel

import (
	"net/http/httptest"
	"testing"
)

func TestDockerDeploymentMetadataAndHostControlBoundary(t *testing.T) {
	t.Setenv("VELORAY_DEPLOYMENT", "docker")
	t.Setenv("VELORAY_HOST_CONTROL", "true")
	t.Setenv("VELORAY_INSTALLED_COMMIT", "0123456789012345678901234567890123456789")
	s := &Server{Config: Config{SubscriptionDriver: "caddy", PublicURL: "https://example.com:24443", SubscriptionPort: 100}}
	admin := Actor{User: Data{"is_superuser": true}, Scope: "admin"}
	if err := s.roleAuthorize(admin, "GET", "deployment"); err != nil {
		t.Fatal("administrator blocked by route authorization", err)
	}
	result, status, err := s.deploymentAPI(httptest.NewRequest("GET", "/api/deployment", nil), admin)
	if err != nil || status != 200 || str(result.(Data), "mode") != "docker" || !flag(result.(Data), "managed_updates") {
		t.Fatal(result, status, err)
	}
	if installedCommit() != "0123456789012345678901234567890123456789" {
		t.Fatal("container revision missing")
	}
	for _, actor := range []Actor{{User: Data{"is_superuser": true}, Scope: "read"}, {User: Data{"is_staff": true}, Scope: "admin"}} {
		if err := s.roleAuthorize(actor, "GET", "deployment"); err == nil {
			t.Fatal("restricted actor passed route authorization")
		}
		if _, _, err := s.deploymentAPI(httptest.NewRequest("GET", "/api/deployment", nil), actor); err == nil {
			t.Fatal("deployment details leaked outside administrator scope")
		}
	}
	t.Setenv("VELORAY_HOST_CONTROL", "false")
	if s.nativeControlAvailable() {
		t.Fatal("manual Compose advertised an uninstalled root worker")
	}
}
