package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tapioca24/gitlab-mr-graph/internal/model"
)

type controlledLoader struct {
	mu      sync.Mutex
	fail    bool
	release chan struct{}
}

func (l *controlledLoader) Load(ctx context.Context, p model.Project, publish func([]model.MR, []string)) error {
	if l.release != nil {
		select {
		case <-l.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	l.mu.Lock()
	fail := l.fail
	l.mu.Unlock()
	if fail {
		return errors.New("connection unavailable")
	}
	publish([]model.MR{{ID: 1, IID: 1, SourceProjectID: 1, TargetProjectID: 1, SourceBranch: "a", TargetBranch: "main", Title: "test"}}, nil)
	return nil
}
func wait(t *testing.T, s *Service) {
	t.Helper()
	deadline := time.Now().Add(time.Second * 2)
	for s.Snapshot().Loading {
		if time.Now().After(deadline) {
			t.Fatal("refresh did not finish")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestRefreshCoalescesAndFailureKeepsLastGraph(t *testing.T) {
	loader := &controlledLoader{release: make(chan struct{})}
	s := NewService(context.Background(), model.Project{ID: 1, DefaultBranch: "main"}, loader, "dev", false)
	if !s.Refresh() || s.Refresh() {
		t.Fatal("concurrent refresh was not coalesced")
	}
	close(loader.release)
	wait(t, s)
	first := s.Snapshot()
	if first.UpdatedAt == nil || len(first.Graph.Nodes) != 2 {
		t.Fatal("missing first graph")
	}
	loader.mu.Lock()
	loader.fail = true
	loader.mu.Unlock()
	s.Refresh()
	wait(t, s)
	next := s.Snapshot()
	if next.Error == "" || len(next.Graph.Nodes) != 2 || !next.UpdatedAt.Equal(*first.UpdatedAt) {
		t.Fatal("failed refresh erased the last good graph")
	}
}

func TestHostSessionOriginAndAssets(t *testing.T) {
	s := NewService(context.Background(), model.Project{}, &controlledLoader{}, "dev", true)
	handler, err := NewHandler(s, "127.0.0.1:8787")
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, token, origin, host string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://"+host+path, nil)
		r.Header.Set("X-MR-Graph-Session", token)
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	root := request("GET", "/", "", "", "127.0.0.1:8787")
	match := regexp.MustCompile(`name="mr-graph-session" content="([a-f0-9]+)"`).FindStringSubmatch(root.Body.String())
	if root.Code != 200 || len(match) != 2 {
		t.Fatal("bootstrap failed")
	}
	for _, bad := range []struct{ token, origin, host string }{{"", "", "127.0.0.1:8787"}, {match[1], "https://other.test", "127.0.0.1:8787"}, {match[1], "", "attacker.test:8787"}} {
		if got := request("POST", "/api/refresh", bad.token, bad.origin, bad.host); got.Code != http.StatusForbidden {
			t.Fatalf("unsafe request accepted: %d", got.Code)
		}
	}
	if got := request("GET", "/api/graph", match[1], "", "127.0.0.1:8787"); got.Code != 200 || !strings.Contains(got.Body.String(), `"nodes":[]`) {
		t.Fatal("graph API failed")
	}
	if got := request("GET", "/app.js", "", "", "127.0.0.1:8787"); got.Code != 200 || !strings.Contains(got.Body.String(), "function renderGraph") {
		t.Fatal("embedded assets missing")
	}
	for _, path := range []string{"/theme.js", "/graph-view.js", "/icons/external-link.svg", "/icons/LICENSE"} {
		if got := request("GET", path, "", "", "127.0.0.1:8787"); got.Code != 200 {
			t.Fatalf("embedded UI asset %s: %d", path, got.Code)
		}
	}
	branch := request("GET", "/icons/git-branch.svg", "", "", "127.0.0.1:8787")
	for _, theme := range []struct{ name, accent string }{{"latte", "#1e66f5"}, {"mocha", "#89b4fa"}} {
		favicon := request("GET", "/mark.svg?theme="+theme.name, "", "", "127.0.0.1:8787")
		want := strings.ReplaceAll(branch.Body.String(), "currentColor", theme.accent)
		if favicon.Code != 200 || favicon.Header().Get("Content-Type") != "image/svg+xml" || favicon.Body.String() != want {
			t.Fatalf("%s favicon does not match the header icon and theme", theme.name)
		}
	}
	csp := root.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "img-src 'self' data: https: http:") || !strings.Contains(csp, "script-src 'self';") {
		t.Fatalf("avatar images or script restriction missing: %s", csp)
	}
}
