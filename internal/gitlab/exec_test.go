package gitlab

import (
	"context"
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
printf '%s\n' "$@" > "$GLAB_TEST_ARGUMENTS"
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
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho 'fake-private-diagnostic' >&2\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	_, err := c.Resolve(context.Background())
	if err == nil || strings.Contains(err.Error(), "fake-private-diagnostic") {
		t.Fatalf("stderr leak: %v", err)
	}
}
