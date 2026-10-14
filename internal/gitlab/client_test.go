package gitlab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/tapioca24/gitlab-mr-graph/internal/model"
)

func encode(v any) []byte { b, _ := json.Marshal(v); return b }
func fixture(id int) rawMR {
	return rawMR{ID: id, IID: id, SourceProjectID: 1, TargetProjectID: 1, SourceBranch: fmt.Sprintf("feature-%d", id), TargetBranch: "main"}
}

func TestCompletePaginationAndStatusHydration(t *testing.T) {
	var mu sync.Mutex
	pages := []string{}
	c := Client{Hostname: "gitlab.example.test", Run: func(ctx context.Context, args []string) ([]byte, error) {
		if len(args) != 6 || args[0] != "api" || args[2] != "--method" || args[3] != "GET" || args[5] != "gitlab.example.test" {
			t.Errorf("unexpected args: %v", args)
		}
		endpoint := args[1]
		if strings.Contains(endpoint, "?state=opened&scope=all") {
			mu.Lock()
			pages = append(pages, endpoint)
			mu.Unlock()
			if strings.HasSuffix(endpoint, "page=1") {
				list := make([]rawMR, 100)
				for i := range list {
					list[i] = fixture(i + 1)
				}
				return encode(list), nil
			}
			if strings.HasSuffix(endpoint, "page=2") {
				return encode([]rawMR{fixture(101)}), nil
			}
			t.Fatal("unexpected page")
		}
		if strings.HasSuffix(endpoint, "/approvals") {
			return []byte(`{"approvals_required":2,"approvals_left":1,"approved_by":[{"user":{"id":9}}]}`), nil
		}
		if strings.HasSuffix(endpoint, "/approval_state") {
			return []byte(`{"rules":[{"approved":false}]}`), nil
		}
		var id int
		fmt.Sscanf(endpoint, "projects/1/merge_requests/%d", &id)
		return []byte(fmt.Sprintf(`{"id":%d,"iid":%d,"head_pipeline":{"status":"running"},"has_conflicts":false,"detailed_merge_status":"checking"}`, id, id)), nil
	}}
	var first, last []model.MR
	if err := c.Load(context.Background(), model.Project{ID: 1}, func(mrs []model.MR, warnings []string) {
		if first == nil {
			first = mrs
		}
		last = mrs
	}); err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 || len(first) != 101 || len(last) != 101 {
		t.Fatalf("pages=%d first=%d last=%d", len(pages), len(first), len(last))
	}
	if first[0].CI != "loading" {
		t.Fatal("complete list was not published before hydration")
	}
	if last[100].CI != "running" || last[100].Conflict != "unknown" || last[100].Approval.State != "waiting" {
		t.Fatalf("bad hydrated state: %+v", last[100])
	}
}

func TestPaginationFailureNeverPublishesIncompleteList(t *testing.T) {
	c := Client{Run: func(ctx context.Context, args []string) ([]byte, error) {
		if strings.HasSuffix(args[1], "page=1") {
			list := make([]rawMR, 100)
			for i := range list {
				list[i] = fixture(i + 1)
			}
			return encode(list), nil
		}
		return nil, errors.New("page unavailable")
	}}
	published := false
	err := c.Load(context.Background(), model.Project{ID: 1}, func([]model.MR, []string) { published = true })
	if err == nil || published {
		t.Fatal("途中のページの失敗で部分的な一覧を公開しました")
	}
}

func TestOptionalFailureAndAbsentPipelineStayUnknown(t *testing.T) {
	c := Client{Run: func(ctx context.Context, args []string) ([]byte, error) {
		if strings.Contains(args[1], "?") {
			return encode([]rawMR{fixture(1), fixture(2)}), nil
		}
		if strings.HasSuffix(args[1], "/approvals") {
			return nil, errors.New("unsupported")
		}
		if strings.HasSuffix(args[1], "/1") {
			return nil, errors.New("timeout")
		}
		return []byte(`{"id":2,"iid":2,"has_conflicts":false,"detailed_merge_status":"unchecked"}`), nil
	}}
	var result []model.MR
	err := c.Load(context.Background(), model.Project{ID: 1}, func(mrs []model.MR, _ []string) { result = mrs })
	if err != nil || len(result) != 2 {
		t.Fatalf("%v %v", err, result)
	}
	for _, m := range result {
		if m.CI != "unknown" || m.Conflict != "unknown" || m.Approval.State != "unknown" {
			t.Errorf("missing data converted to positive state: %+v", m)
		}
	}
}

func TestApprovalFallbackOnlyFor404WithKnownCount(t *testing.T) {
	notFound := &apiError{status: 404, err: errors.New("not found")}
	for _, test := range []struct {
		name         string
		approvals    string
		approvalsErr error
		rules        string
		rulesErr     error
		want         string
	}{
		{name: "404 without approvals", approvals: `{"approved_by":[]}`, rulesErr: notFound, want: "unapproved"},
		{name: "404 with approval", approvals: `{"approved_by":[{"user":{"id":9}}]}`, rulesErr: notFound, want: "received"},
		{name: "404 with missing count", approvals: `{}`, rulesErr: notFound, want: "unknown"},
		{name: "404 with null count", approvals: `{"approved_by":null}`, rulesErr: notFound, want: "unknown"},
		{name: "404 with approvals failure", approvalsErr: errors.New("unavailable"), rulesErr: notFound, want: "unknown"},
		{name: "404 with malformed approvals", approvals: `{`, rulesErr: notFound, want: "unknown"},
		{name: "forbidden", approvals: `{"approved_by":[]}`, rulesErr: &apiError{status: 403, err: errors.New("forbidden")}, want: "unknown"},
		{name: "unauthorized", approvals: `{"approved_by":[]}`, rulesErr: &apiError{status: 401, err: errors.New("unauthorized")}, want: "unknown"},
		{name: "server error", approvals: `{"approved_by":[]}`, rulesErr: &apiError{status: 500, err: errors.New("server error")}, want: "unknown"},
		{name: "timeout", approvals: `{"approved_by":[]}`, rulesErr: context.DeadlineExceeded, want: "unknown"},
		{name: "missing rules", approvals: `{"approved_by":[]}`, rules: `{}`, want: "unknown"},
		{name: "malformed rules", approvals: `{"approved_by":[]}`, rules: `{`, want: "unknown"},
		{name: "rules not required", approvals: `{"approved_by":[]}`, rules: `{"rules":[]}`, want: "not_required"},
		{name: "rules waiting despite approval", approvals: `{"approved_by":[{"user":{"id":9}}]}`, rules: `{"rules":[{"approved":false}]}`, want: "waiting"},
		{name: "rules met", approvals: `{"approved_by":[]}`, rules: `{"rules":[{"approved":true}]}`, want: "met"},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := Client{Run: func(_ context.Context, args []string) ([]byte, error) {
				switch {
				case strings.Contains(args[1], "?"):
					return encode([]rawMR{fixture(1)}), nil
				case strings.HasSuffix(args[1], "/approvals"):
					return []byte(test.approvals), test.approvalsErr
				case strings.HasSuffix(args[1], "/approval_state"):
					return []byte(test.rules), test.rulesErr
				default:
					return encode(fixture(1)), nil
				}
			}}
			var result []model.MR
			if err := c.Load(context.Background(), model.Project{ID: 1}, func(mrs []model.MR, _ []string) { result = mrs }); err != nil {
				t.Fatal(err)
			}
			if len(result) != 1 || result[0].Approval.State != test.want {
				t.Fatalf("want %s, got %+v", test.want, result)
			}
		})
	}
}

func TestApprovalCountsDoNotInventRequirements(t *testing.T) {
	zero, two, one := 0, 2, 1
	for _, test := range []struct {
		raw  rawApproval
		want string
	}{
		{rawApproval{ApprovedBy: []json.RawMessage{json.RawMessage(`{}`)}}, "received"},
		{rawApproval{Required: &zero, Left: &zero}, "unknown"},
		{rawApproval{Required: &two, Left: &zero}, "unknown"},
		{rawApproval{Required: &two, Left: &one}, "unknown"},
	} {
		if got := approval(test.raw); got.State != test.want {
			t.Errorf("got %s want %s", got.State, test.want)
		}
	}
	if ci(rawMR{Pipeline: json.RawMessage(`null`)}) != "none" {
		t.Fatal("null pipeline is unexecuted")
	}
	if ci(rawMR{}) != "unknown" {
		t.Fatal("missing pipeline is unknown")
	}
	for _, test := range []struct {
		rules string
		want  string
	}{
		{`[{"approved":true},{"approved":true}]`, "met"},
		{`[{"approved":true},{"approved":false}]`, "waiting"},
		{`[{"approved":true},{}]`, "unknown"},
		{`[]`, "not_required"},
		{`null`, "unknown"},
	} {
		if got := applyRules(model.Approval{State: "unknown"}, json.RawMessage(test.rules)); got.State != test.want {
			t.Errorf("rules %s: got %s want %s", test.rules, got.State, test.want)
		}
	}
}

func TestResolveUsesGlabContextAndEscapesExplicitPath(t *testing.T) {
	for _, path := range []string{"", "group/subgroup/project"} {
		c := Client{Repo: path, Run: func(ctx context.Context, args []string) ([]byte, error) {
			if args[1] == "user" {
				return []byte(`{"id":9,"username":"me","name":"My Name","avatar_url":"https://gitlab.example.test/avatar.png"}`), nil
			}
			want := "projects/:fullpath"
			if path != "" {
				want = "projects/group%2Fsubgroup%2Fproject"
			}
			if args[1] != want {
				t.Errorf("%s != %s", args[1], want)
			}
			return []byte(`{"id":1,"name":"Project","path_with_namespace":"group/project","web_url":"https://gitlab.example.test/group/project","default_branch":"main"}`), nil
		}}
		p, err := c.Resolve(context.Background())
		if err != nil || p.ID != 1 || p.DefaultBranch != "main" {
			t.Fatalf("%+v %v", p, err)
		}
		if c.Hostname != "gitlab.example.test" {
			t.Fatal("resolved host was not pinned")
		}
		if p.CurrentUser == nil || p.CurrentUser.ID != 9 || p.CurrentUser.AvatarURL == "" {
			t.Fatal("current user was not resolved")
		}
	}
}

func TestUserLabelsAndApproverHydration(t *testing.T) {
	c := Client{Run: func(ctx context.Context, args []string) ([]byte, error) {
		switch {
		case strings.Contains(args[1], "?"):
			if !strings.Contains(args[1], "with_labels_details=true") {
				t.Error("label details were not requested")
			}
			return []byte(`[{"id":1,"iid":1,"author":{"id":7,"username":"author","name":"Author Name","avatar_url":"https://example.test/a.png"},"assignees":[{"id":8,"username":"assignee","name":"Assigned User","avatar_url":"https://example.test/b.png"}],"labels":[{"name":"backend","color":"#1e66f5","text_color":"#ffffff"},"legacy"]}]`), nil
		case strings.HasSuffix(args[1], "/approvals"):
			return []byte(`{"approved_by":[{"user":{"id":9,"username":"approver","name":"Approver Name","avatar_url":"https://example.test/c.png"}}]}`), nil
		case strings.HasSuffix(args[1], "/approval_state"):
			return []byte(`{"rules":[{"approved":true}]}`), nil
		default:
			return []byte(`{"id":1,"iid":1,"head_pipeline":null}`), nil
		}
	}}
	var result []model.MR
	if err := c.Load(context.Background(), model.Project{ID: 1}, func(mrs []model.MR, _ []string) { result = mrs }); err != nil {
		t.Fatal(err)
	}
	m := result[0]
	if m.Author != "author" || m.AuthorUser.Name != "Author Name" || m.AuthorUser.AvatarURL == "" || len(m.Assignees) != 1 || m.Assignees[0].ID != 8 {
		t.Fatalf("missing users: %+v", m)
	}
	if len(m.Labels) != 2 || m.Labels[0].Color != "#1e66f5" || m.Labels[0].TextColor != "#ffffff" || m.Labels[1].Name != "legacy" {
		t.Fatalf("missing labels: %+v", m.Labels)
	}
	if m.Approval.State != "met" || len(m.Approval.Users) != 1 || m.Approval.Users[0].Name != "Approver Name" {
		t.Fatalf("missing approvers after applying rules: %+v", m.Approval)
	}
}

func TestResolveKeepsProjectWhenCurrentUserUnavailable(t *testing.T) {
	c := Client{Run: func(ctx context.Context, args []string) ([]byte, error) {
		if args[1] == "user" {
			return nil, errors.New("unavailable")
		}
		return []byte(`{"id":1,"path_with_namespace":"group/project","web_url":"https://example.test/group/project"}`), nil
	}}
	p, err := c.Resolve(context.Background())
	if err != nil || p.ID != 1 || p.CurrentUser != nil {
		t.Fatalf("%+v %v", p, err)
	}
}
