package panel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestProjectStarsCacheAndOutageFallback(t *testing.T) {
	requests, code := 0, 200
	body := `{"full_name":"ParsaZali/VeloRay","stargazers_count":17}`
	s := &Server{GitHubHTTP: &http.Client{Transport: featureTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		if r.URL.String() != "https://api.github.com/repos/ParsaZali/VeloRay" || r.Header.Get("Authorization") != "" {
			t.Error("project metadata request escaped the public repository")
		}
		return featureResponse(code, body), nil
	})}}
	var group sync.WaitGroup
	for range 20 {
		group.Go(func() {
			data := s.projectInfo(context.Background())
			if stars := data["stars"].(*int64); stars == nil || *stars != 17 {
				t.Error("actual count unavailable", data)
			}
		})
	}
	group.Wait()
	if requests != 1 {
		t.Fatal("parallel requests bypassed cache", requests)
	}
	code = 503
	s.project.next = time.Time{}
	data := s.projectInfo(context.Background())
	if *data["stars"].(*int64) != 17 || requests != 2 {
		t.Fatal("outage discarded the last count")
	}
	s.projectInfo(context.Background())
	if requests != 2 {
		t.Fatal("outage retry backoff missing")
	}
	code, body = 200, `{"full_name":"another/repository","stargazers_count":999999}`
	s.project.next = time.Time{}
	if *s.projectInfo(context.Background())["stars"].(*int64) != 17 {
		t.Fatal("foreign repository count accepted")
	}
	unknown := &Server{GitHubHTTP: s.GitHubHTTP}
	if unknown.projectInfo(context.Background())["stars"].(*int64) != nil {
		t.Fatal("missing count was fabricated")
	}
}

func TestCoreManagementRequiresAdministrator(t *testing.T) {
	s := &Server{}
	for _, actor := range []Actor{
		{User: Data{"role": "reseller"}, Scope: "admin"},
		{User: Data{"role": "custom", "permissions": []any{"nodes.write"}}, Scope: "admin"},
		{User: Data{"is_superuser": true}, Scope: "client"},
	} {
		for _, action := range []string{"core-manager", "core-releases", "core-policy", "core-install"} {
			r := httptest.NewRequest("POST", "/api/nodes/1/"+action+"/", nil)
			if _, _, err := s.coreManagerAPI(httptest.NewRecorder(), r, Data{}, action, actor); err == nil {
				t.Fatal("core management escaped administrator scope", action, actor)
			}
		}
	}
}
