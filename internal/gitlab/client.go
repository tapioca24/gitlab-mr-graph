// Package gitlab accesses GitLab exclusively through the user's authenticated glab.
package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tapioca24/gitlab-mr-graph/internal/model"
)

type Runner func(context.Context, []string) ([]byte, error)

// Preserve only the HTTP status from glab's diagnostics, never raw stderr.
type apiError struct {
	status int
	err    error
}

func (e *apiError) Error() string { return e.err.Error() }
func (e *apiError) Unwrap() error { return e.err }

var httpStatusPattern = regexp.MustCompile(`\(HTTP ([0-9]{3})\)(?:\r?\n|$)`)

type Client struct {
	Run      Runner
	Hostname string
	Repo     string
}

type limitedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.limit {
		return 0, errors.New("glab の応答がサイズ上限を超えました")
	}
	return b.Buffer.Write(p)
}

func ExecRunner(dir string) Runner {
	return func(ctx context.Context, args []string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "glab", args...)
		cmd.Dir = dir
		stdout := &limitedBuffer{limit: 16 << 20}
		// Raw stderr can contain HTTP diagnostics or credentials; do not expose it.
		stderr := &limitedBuffer{limit: 64 << 10}
		cmd.Stdout, cmd.Stderr = stdout, stderr
		if err := cmd.Run(); err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			failure := fmt.Errorf("glab の実行に失敗しました (%w)。ホスト、権限、glab auth status を確認してください", err)
			if match := httpStatusPattern.FindStringSubmatch(stderr.String()); match != nil {
				status, _ := strconv.Atoi(match[1])
				return nil, &apiError{status: status, err: failure}
			}
			return nil, failure
		}
		return stdout.Bytes(), nil
	}
}

func (c *Client) get(ctx context.Context, endpoint string, out any) error {
	args := []string{"api", endpoint, "--method", "GET"}
	if c.Hostname != "" {
		args = append(args, "--hostname", c.Hostname)
	}
	data, err := c.Run(ctx, args)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("glab の JSON 応答を解釈できません: %w", err)
	}
	return nil
}

func (c *Client) Resolve(ctx context.Context) (model.Project, error) {
	path := ":fullpath"
	if c.Repo != "" {
		path = url.PathEscape(c.Repo)
	}
	var raw struct {
		ID            int    `json:"id"`
		Name          string `json:"name"`
		Path          string `json:"path_with_namespace"`
		URL           string `json:"web_url"`
		DefaultBranch string `json:"default_branch"`
	}
	if err := c.get(ctx, "projects/"+path, &raw); err != nil {
		return model.Project{}, fmt.Errorf("対象プロジェクトを確認できません: %w", err)
	}
	if raw.ID <= 0 || raw.Path == "" {
		return model.Project{}, errors.New("プロジェクトの ID またはパスがありません")
	}
	// Pin the resolved host so changing git remotes during a session cannot
	// accidentally hydrate the same numeric project ID on another instance.
	if c.Hostname == "" {
		if u, err := url.Parse(raw.URL); err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" {
			c.Hostname = u.Host
		}
	}
	p := model.Project{ID: raw.ID, Name: raw.Name, Path: raw.Path, URL: raw.URL, DefaultBranch: raw.DefaultBranch}
	var user rawUser
	if err := c.get(ctx, "user", &user); err == nil && user.ID > 0 && user.Username != "" {
		u := user.model()
		p.CurrentUser = &u
	}
	return p, nil
}

type rawUser struct {
	ID        int    `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

func (u rawUser) model() model.User {
	return model.User{ID: u.ID, Username: u.Username, Name: u.Name, AvatarURL: u.AvatarURL}
}

type rawMR struct {
	ID                  int               `json:"id"`
	IID                 int               `json:"iid"`
	Title               string            `json:"title"`
	URL                 string            `json:"web_url"`
	Author              rawUser           `json:"author"`
	Assignees           []rawUser         `json:"assignees"`
	Labels              []json.RawMessage `json:"labels"`
	SourceProjectID     int               `json:"source_project_id"`
	TargetProjectID     int               `json:"target_project_id"`
	SourceBranch        string            `json:"source_branch"`
	TargetBranch        string            `json:"target_branch"`
	Draft               bool              `json:"draft"`
	WIP                 bool              `json:"work_in_progress"`
	UpdatedAt           time.Time         `json:"updated_at"`
	HasConflicts        *bool             `json:"has_conflicts"`
	MergeStatus         string            `json:"merge_status"`
	DetailedMergeStatus string            `json:"detailed_merge_status"`
	Pipeline            json.RawMessage   `json:"head_pipeline"`
}

func (r rawMR) model() model.MR {
	assignees := make([]model.User, 0, len(r.Assignees))
	for _, u := range r.Assignees {
		assignees = append(assignees, u.model())
	}
	labels := make([]model.Label, 0, len(r.Labels))
	for _, data := range r.Labels {
		var label struct {
			Name      string `json:"name"`
			Color     string `json:"color"`
			TextColor string `json:"text_color"`
		}
		if json.Unmarshal(data, &label) == nil && label.Name != "" {
			labels = append(labels, model.Label{Name: label.Name, Color: label.Color, TextColor: label.TextColor})
		} else {
			var name string
			if json.Unmarshal(data, &name) == nil && name != "" {
				labels = append(labels, model.Label{Name: name})
			}
		}
	}
	return model.MR{ID: r.ID, IID: r.IID, Title: r.Title, URL: r.URL, Author: r.Author.Username,
		AuthorUser: r.Author.model(), Assignees: assignees, Labels: labels,
		SourceProjectID: r.SourceProjectID, TargetProjectID: r.TargetProjectID, SourceBranch: r.SourceBranch,
		TargetBranch: r.TargetBranch, Draft: r.Draft || r.WIP, UpdatedAt: r.UpdatedAt,
		CI: "loading", Conflict: "loading", Approval: model.Approval{State: "loading"}, Warnings: []string{}}
}

func conflict(r rawMR) string {
	if r.DetailedMergeStatus == "conflict" || (r.HasConflicts != nil && *r.HasConflicts) {
		return "conflict"
	}
	if r.HasConflicts != nil && !*r.HasConflicts && (r.DetailedMergeStatus == "mergeable" || r.MergeStatus == "can_be_merged") {
		return "clear"
	}
	// has_conflicts:false before mergeability evaluation does not prove no conflict.
	return "unknown"
}

func ci(r rawMR) string {
	if len(r.Pipeline) == 0 {
		return "unknown"
	}
	if string(r.Pipeline) == "null" {
		return "none"
	}
	var pipeline struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(r.Pipeline, &pipeline); err != nil || pipeline.Status == "" {
		return "unknown"
	}
	return pipeline.Status
}

type rawApproval struct {
	ApprovedBy []json.RawMessage `json:"approved_by"`
	Required   *int              `json:"approvals_required"`
	Left       *int              `json:"approvals_left"`
}

func approval(r rawApproval) model.Approval {
	a := model.Approval{State: "unknown", Count: len(r.ApprovedBy), CountKnown: r.ApprovedBy != nil}
	a.Users = []model.User{}
	for _, data := range r.ApprovedBy {
		var entry struct {
			User rawUser `json:"user"`
		}
		if json.Unmarshal(data, &entry) == nil && (entry.User.ID > 0 || entry.User.Username != "") {
			a.Users = append(a.Users, entry.User.model())
		}
	}
	// Legacy aggregate counts do not establish that every approval rule is met.
	if a.Count > 0 {
		a.State = "received"
	}
	return a
}

func applyRules(a model.Approval, rules json.RawMessage) model.Approval {
	if len(rules) == 0 || string(rules) == "null" {
		return a
	}
	var parsed []struct {
		Approved *bool `json:"approved"`
	}
	if err := json.Unmarshal(rules, &parsed); err != nil {
		return a
	}
	if len(parsed) == 0 {
		a.State = "not_required"
		return a
	}
	unknown := false
	for _, rule := range parsed {
		if rule.Approved == nil {
			unknown = true
			continue
		}
		if !*rule.Approved {
			a.State = "waiting"
			return a
		}
	}
	if !unknown {
		a.State = "met"
	}
	return a
}

// Load publishes the complete MR set before optional status hydration. It never
// publishes a partial list if a later page fails.
func (c *Client) Load(ctx context.Context, p model.Project, publish func([]model.MR, []string)) error {
	var mrs []model.MR
	seen := map[int]bool{}
	for page := 1; ; page++ {
		var batch []rawMR
		endpoint := fmt.Sprintf("projects/%d/merge_requests?state=opened&scope=all&with_labels_details=true&per_page=100&page=%d", p.ID, page)
		if err := c.get(ctx, endpoint, &batch); err != nil {
			return fmt.Errorf("MR 一覧の取得に失敗しました (ページ %d): %w", page, err)
		}
		added := 0
		for _, r := range batch {
			if r.ID <= 0 || r.IID <= 0 {
				return errors.New("MR 応答に有効な ID がありません")
			}
			if !seen[r.ID] {
				mrs = append(mrs, r.model())
				seen[r.ID] = true
				added++
			}
		}
		if len(batch) < 100 {
			break
		}
		if added == 0 {
			return errors.New("MR ページングが進みません。全件取得を確認できませんでした")
		}
	}
	sort.Slice(mrs, func(i, j int) bool { return mrs[i].IID < mrs[j].IID })
	if mrs == nil {
		mrs = []model.MR{}
	}
	publish(append([]model.MR(nil), mrs...), []string{})
	var mu sync.Mutex
	var wg sync.WaitGroup
	jobs := make(chan int)
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				mr := mrs[i]
				base := "projects/" + strconv.Itoa(p.ID) + "/merge_requests/" + strconv.Itoa(mr.IID)
				var detail rawMR
				mr.CI, mr.Conflict = "unknown", "unknown"
				if err := c.get(ctx, base, &detail); err != nil {
					mr.Warnings = append(mr.Warnings, "CI・競合の情報を取得できませんでした")
				} else if detail.ID != mr.ID || detail.IID != mr.IID {
					mr.Warnings = append(mr.Warnings, "MR 詳細の ID が一致しないため、状態を確認できませんでした")
				} else {
					mr.CI = ci(detail)
					mr.Conflict = conflict(detail)
				}
				var a rawApproval
				mr.Approval = model.Approval{State: "unknown"}
				if err := c.get(ctx, base+"/approvals", &a); err != nil {
					mr.Warnings = append(mr.Warnings, "承認情報を取得できませんでした")
				} else {
					mr.Approval = approval(a)
				}
				// /approval_state is edition/tier dependent. Only a 404 allows
				// falling back to a known zero approval count; other errors stay unknown.
				var rules struct {
					Rules json.RawMessage `json:"rules"`
				}
				if err := c.get(ctx, base+"/approval_state", &rules); err == nil {
					mr.Approval = applyRules(mr.Approval, rules.Rules)
				} else {
					var failure *apiError
					if errors.As(err, &failure) && failure.status == 404 && mr.Approval.CountKnown && mr.Approval.Count == 0 {
						mr.Approval.State = "unapproved"
					}
				}
				mu.Lock()
				mrs[i] = mr
				copy := append([]model.MR(nil), mrs...)
				var warnings []string
				for _, m := range mrs {
					if len(m.Warnings) > 0 {
						warnings = append(warnings, fmt.Sprintf("!%d: %s", m.IID, strings.Join(m.Warnings, " / ")))
					}
				}
				// Serial callbacks preserve completion order across workers.
				publish(copy, warnings)
				mu.Unlock()
			}
		}()
	}
	for i := range mrs {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return nil
}
