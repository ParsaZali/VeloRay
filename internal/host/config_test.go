package host

import (
	"strings"
	"testing"
)

func TestSubscriptionNginxListenerIsSeparate(t *testing.T) {
	raw := string(nginxSubscriptionConfiguration("panel.example", "9444", "/private/panel.crt", "/private/panel.key"))
	for _, wanted := range []string{"listen 9444 ssl;", "listen [::]:9444 ssl;", "proxy_pass http://127.0.0.1:8611;", "access_log off;", "error_log /dev/null;"} {
		if !strings.Contains(raw, wanted) {
			t.Fatal("missing", wanted, raw)
		}
	}
	if strings.Contains(raw, "127.0.0.1:8610") {
		t.Fatal("subscription listener forwards to management")
	}
}
