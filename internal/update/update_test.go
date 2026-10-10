package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifiedUpdateAndFailurePreserveExecutable(t *testing.T) {
	for _, valid := range []bool{true, false} {
		t.Run(map[bool]string{true: "valid", false: "checksum_mismatch"}[valid], func(t *testing.T) {
			binary := []byte("new executable")
			sum := sha256.Sum256(binary)
			hash := hex.EncodeToString(sum[:])
			if !valid {
				hash = strings.Repeat("0", 64)
			}
			var server *httptest.Server
			server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/repos/" + Repository + "/releases/latest":
					json.NewEncoder(w).Encode(map[string]any{"tag_name": "v1.2.0", "assets": []map[string]string{{"name": "gitlab-mr-graph_v1.2.0_linux_amd64", "browser_download_url": server.URL + "/binary"}, {"name": "checksums.txt", "browser_download_url": server.URL + "/checksums"}}})
				case "/binary":
					w.Write(binary)
				case "/checksums":
					w.Write([]byte(hash + "  gitlab-mr-graph_v1.2.0_linux_amd64\n"))
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			path := filepath.Join(t.TempDir(), "app")
			old := []byte("old executable")
			if err := os.WriteFile(path, old, 0755); err != nil {
				t.Fatal(err)
			}
			client := Client{HTTP: server.Client(), APIBase: server.URL, OS: "linux", Arch: "amd64"}
			result, err := client.CheckAndInstall(context.Background(), "v1.1.0", path)
			got, _ := os.ReadFile(path)
			if valid {
				if err != nil || !result.Updated || string(got) != string(binary) {
					t.Fatalf("%+v %v %s", result, err, got)
				}
			} else if err == nil || string(got) != string(old) {
				t.Fatalf("failed update changed executable: %v %s", err, got)
			}
			info, _ := os.Stat(path)
			if info.Mode().Perm() != 0755 {
				t.Fatal("executable permission was not preserved")
			}
			files, _ := os.ReadDir(filepath.Dir(path))
			if len(files) != 1 {
				t.Fatalf("leftovers: %v", files)
			}
		})
	}
}

func TestVersionComparisonAndNoDowngrade(t *testing.T) {
	for _, test := range []struct {
		candidate, current string
		want               bool
	}{
		{"v1.10.0", "v1.9.0", true}, {"v1.1.0", "v2.0.0", false}, {"v1.0.0", "v1.0.0", false},
	} {
		got, err := newer(test.candidate, test.current)
		if err != nil || got != test.want {
			t.Fatalf("%+v: %v %v", test, got, err)
		}
	}
	for _, v := range []string{"dev", "v1.0.0-beta", "v01.0.0", "v1.0.0;cmd"} {
		if _, err := version(v); err == nil {
			t.Errorf("invalid version accepted: %s", v)
		}
	}
	path := filepath.Join(t.TempDir(), "app")
	os.WriteFile(path, []byte("old"), 0755)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"tag_name":"v1.0.0","assets":[]}`)) }))
	defer server.Close()
	result, err := (Client{HTTP: server.Client(), APIBase: server.URL}).CheckAndInstall(context.Background(), "v2.0.0", path)
	if err != nil || result.Updated {
		t.Fatalf("downgrade: %+v %v", result, err)
	}
}

func TestInstallRejectsEmptyDataAndSymlink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app")
	os.WriteFile(path, []byte("original"), 0755)
	if err := Install(path, nil); err == nil {
		t.Fatal("empty update accepted")
	}
	link := filepath.Join(dir, "link")
	os.Symlink(path, link)
	if err := Install(link, []byte("new")); err == nil {
		t.Fatal("symlink installation accepted")
	}
	got, _ := os.ReadFile(path)
	if string(got) != "original" {
		t.Fatal("existing binary changed")
	}
}

func TestHTTPUpdateURLRejected(t *testing.T) {
	if _, err := (Client{}).fetch(context.Background(), "http://example.com/update", 100); err == nil {
		t.Fatal("unencrypted update accepted")
	}
}
