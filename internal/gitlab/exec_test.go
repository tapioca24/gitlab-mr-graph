package gitlab

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecRunnerUsesArgumentsAndDoesNotLeakStderr(t *testing.T) {
	dir := t.TempDir()
	arguments := filepath.Join(dir, "arguments")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GLAB_TEST_ARGUMENTS", arguments)
	script := `#!/bin/sh
printf '%s\n' "$@" >> "$GLAB_TEST_ARGUMENTS"
if [ "$2" = "user" ]; then
  printf '{"id":2,"username":"me"}'
  exit 0
fi
printf '{"id":1,"path_with_namespace":"group/project","default_branch":"main"}'
`
	path := filepath.Join(dir, "glab")
	if err := os.WriteFile(path, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	c := Client{Run: ExecRunner(dir), Repo: "group/project"}
	if _, err := c.Resolve(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(arguments)
	if !strings.Contains(string(got), "projects/group%2Fproject\n--method\nGET\n") {
		t.Fatalf("args: %s", got)
	}
	if !strings.Contains(string(got), "api\nuser\n--method\nGET\n") {
		t.Fatalf("current user request missing: %s", got)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho 'fake-private-diagnostic' >&2\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	_, err := c.Resolve(context.Background())
	if err == nil || strings.Contains(err.Error(), "fake-private-diagnostic") {
		t.Fatalf("stderr leak: %v", err)
	}
}

func TestExecRunnerPreservesHTTPStatusWithoutLeakingDiagnostics(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	path := filepath.Join(dir, "glab")
	for _, test := range []struct {
		name   string
		stderr string
		status int
	}{
		{"not found", "glab: private-diagnostic (HTTP 404)", 404},
		{"forbidden", "glab: private-diagnostic (HTTP 403)", 403},
		{"server error", "glab: private-diagnostic (HTTP 500)", 500},
		{"no status", "private-diagnostic 404", 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			script := "#!/bin/sh\nprintf '%s\\n' '" + test.stderr + "' >&2\nexit 1\n"
			if err := os.WriteFile(path, []byte(script), 0755); err != nil {
				t.Fatal(err)
			}
			_, err := ExecRunner(dir)(context.Background(), []string{"api", "test"})
			if err == nil || strings.Contains(err.Error(), "private-diagnostic") {
				t.Fatalf("missing failure or leaked diagnostics: %v", err)
			}
			var failure *apiError
			if test.status == 0 {
				if errors.As(err, &failure) {
					t.Fatal("invented HTTP status")
				}
			} else if !errors.As(err, &failure) || failure.status != test.status {
				t.Fatalf("HTTP status not preserved: %v", err)
			}
		})
	}
}
