package panel

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSubscriptionErrorAndPanicLogsOmitBearerPath(t *testing.T) {
	for _, panicCase := range []bool{false, true} {
		var log bytes.Buffer
		s := &Server{Logger: slog.New(slog.NewJSONHandler(&log, nil))}
		token := "synthetic-private-subscription-token"
		r := httptest.NewRequest("GET", "/sub/"+token+"/assets/private.js", nil)
		w := httptest.NewRecorder()
		if panicCase {
			// An unavailable store exercises the HTTP panic boundary without a database.
			s.Handler().ServeHTTP(w, r)
		} else {
			s.respond(w, r, nil, 0, errors.New("synthetic storage outage"))
		}
		if w.Code != 500 || strings.Contains(log.String(), token) || strings.Contains(log.String(), "private.js") || !strings.Contains(log.String(), "/sub/[redacted]") {
			t.Fatalf("subscription path leaked or diagnostic missing: status=%d log=%s", w.Code, log.String())
		}
	}
}
