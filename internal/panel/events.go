package panel

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

func (s *Server) acquireStream(a Actor) bool {
	key := strconv.FormatInt(num(a.User, "id"), 10)
	s.streamMu.Lock()
	defer s.streamMu.Unlock()
	if s.streamUsers == nil {
		s.streamUsers = map[string]int{}
	}
	if s.streamCount >= 32 || s.streamUsers[key] >= 2 {
		return false
	}
	s.streamCount++
	s.streamUsers[key]++
	return true
}
func (s *Server) releaseStream(a Actor) {
	key := strconv.FormatInt(num(a.User, "id"), 10)
	s.streamMu.Lock()
	defer s.streamMu.Unlock()
	s.streamCount--
	s.streamUsers[key]--
	if s.streamUsers[key] == 0 {
		delete(s.streamUsers, key)
	}
}
func (s *Server) streamEvents(w http.ResponseWriter, r *http.Request, a Actor) {
	if r.Method != "GET" {
		s.respond(w, r, nil, 0, fail(405, "method not allowed"))
		return
	}
	if !s.acquireStream(a) {
		s.respond(w, r, nil, 0, fail(429, "live connection limit reached"))
		return
	}
	defer s.releaseStream(a)
	if _, ok := w.(http.Flusher); !ok {
		s.respond(w, r, nil, 0, fail(500, "streaming unavailable"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	controller := http.NewResponseController(w)
	send := func(event string, v any) bool {
		if e := controller.SetWriteDeadline(time.Now().Add(5 * time.Second)); e != nil && e != http.ErrNotSupported {
			return false
		}
		if _, e := fmt.Fprintf(w, "retry: 10000\nevent: %s\ndata: %s\n\n", event, mustJSON(v)); e != nil {
			return false
		}
		return controller.Flush() == nil
	}
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	end := time.NewTimer(2 * time.Minute)
	defer end.Stop()
	for {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		fresh, e := s.authenticate(r.WithContext(ctx))
		if e == nil {
			e = s.authorize(ctx, fresh, "GET", "events")
		}
		if e != nil {
			cancel()
			send("authorization-expired", Data{"at": stamp()})
			return
		}
		data, e := s.overviewForActor(ctx, fresh)
		cancel()
		if e != nil {
			send("unavailable", Data{"at": stamp()})
			return
		}
		if !send("overview", Data{"at": stamp(), "overview": data}) {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-end.C:
			return
		case <-ticker.C:
		}
	}
}
