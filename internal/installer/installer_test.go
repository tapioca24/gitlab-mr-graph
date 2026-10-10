package installer

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallerVerifiesAndPreservesPreviousOnFailure(t *testing.T) {
	for _, test := range []struct {
		name          string
		valid, latest bool
	}{{"valid_pinned", true, false}, {"valid_latest", true, true}, {"invalid_checksum", false, false}} {
		t.Run(test.name, func(t *testing.T) {
			valid := test.valid
			dir := t.TempDir()
			fixtureDir := filepath.Join(dir, "download")
			fakeBin := filepath.Join(dir, "commands")
			installDir := filepath.Join(dir, "install with spaces")
			for _, p := range []string{fixtureDir, fakeBin, installDir} {
				if err := os.Mkdir(p, 0755); err != nil {
					t.Fatal(err)
				}
			}
			binary := []byte("#!/bin/sh\nprintf 'gitlab-mr-graph v1.2.0\\n'\n")
			os.WriteFile(filepath.Join(fixtureDir, "binary"), binary, 0755)
			sum := sha256.Sum256(binary)
			hash := hex.EncodeToString(sum[:])
			if !valid {
				hash = strings.Repeat("0", 64)
			}
			// Supply all four architecture names, so the same fixture works in macOS/Linux CI.
			var lines string
			for _, platform := range []string{"darwin_arm64", "darwin_amd64", "linux_arm64", "linux_amd64"} {
				lines += hash + "  gitlab-mr-graph_v1.2.0_" + platform + "\n"
			}
			os.WriteFile(filepath.Join(fixtureDir, "checksums"), []byte(lines), 0644)
			fakeCurl := `#!/bin/sh
set -eu
output=''
source='binary'
latest='false'
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) output="$2"; shift 2 ;;
    -w) latest='true'; shift 2 ;;
    */checksums.txt) source='checksums'; shift ;;
    *) shift ;;
  esac
done
if [ "$latest" = 'true' ]; then
  printf 'https://github.com/tapioca24/gitlab-mr-graph/releases/tag/v1.2.0'
  exit 0
fi
cp "$INSTALLER_FIXTURE_DIR/$source" "$output"
`
			os.WriteFile(filepath.Join(fakeBin, "curl"), []byte(fakeCurl), 0755)
			target := filepath.Join(installDir, "gitlab-mr-graph")
			os.WriteFile(target, []byte("old binary"), 0755)
			cmd := exec.Command("sh", "../../scripts/install.sh")
			cmd.Env = append(os.Environ(), "PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"), "GITLAB_MR_GRAPH_VERSION=v1.2.0", "GITLAB_MR_GRAPH_INSTALL_DIR="+installDir, "INSTALLER_FIXTURE_DIR="+fixtureDir)
			if test.latest {
				cmd.Env = append(cmd.Env, "GITLAB_MR_GRAPH_VERSION=")
			}
			output, err := cmd.CombinedOutput()
			got, _ := os.ReadFile(target)
			if valid {
				if err != nil || string(got) != string(binary) {
					t.Fatalf("%v %s %s", err, output, got)
				}
			} else if err == nil || string(got) != "old binary" {
				t.Fatalf("invalid checksum changed binary: %v %s", err, got)
			}
			files, _ := os.ReadDir(installDir)
			if len(files) != 1 {
				t.Fatalf("temporary file left: %v", files)
			}
		})
	}
}
