package panel

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"
)

type projectCache struct {
	mu    sync.Mutex
	next  time.Time
	stars *int64
}

func (s *Server) projectInfo(ctx context.Context) Data {
	s.project.mu.Lock()
	defer s.project.mu.Unlock()
	if time.Now().After(s.project.next) {
		s.project.next = time.Now().Add(time.Minute)
		ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/repos/ParsaZali/VeloRay", nil)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "VeloRay/"+Version)
		client := s.GitHubHTTP
		if client == nil {
			client = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		}
		response, err := client.Do(req)
		if err == nil {
			defer response.Body.Close()
			var data struct {
				Stars    int64  `json:"stargazers_count"`
				FullName string `json:"full_name"`
			}
			if response.StatusCode == 200 && json.NewDecoder(io.LimitReader(response.Body, 128<<10)).Decode(&data) == nil && data.FullName == "ParsaZali/VeloRay" && data.Stars >= 0 {
				s.project.stars = &data.Stars
				s.project.next = time.Now().Add(15 * time.Minute)
			}
		}
	}
	return Data{"repository": "ParsaZali/VeloRay", "url": "https://github.com/ParsaZali/VeloRay", "stars": s.project.stars, "version": Version}
}
