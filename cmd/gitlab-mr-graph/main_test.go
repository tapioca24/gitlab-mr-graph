package main

import (
	"runtime/debug"
	"testing"
)

func TestVersionFromBuildInfo(t *testing.T) {
	for _, tc := range []struct {
		name, release, module, want string
	}{
		{"release build", "v1.2.3", "(devel)", "v1.2.3"},
		{"release takes precedence", "v1.2.3", "v1.0.0", "v1.2.3"},
		{"go install", "dev", "v1.2.3", "v1.2.3"},
		{"go install revision", "dev", "v0.0.0-20261011000000-abcdef123456", "v0.0.0-20261011000000-abcdef123456"},
		{"local build without module version", "dev", "(devel)", "dev"},
		{"local build with VCS version", "dev", "v0.0.0-20261011000000-abcdef123456+dirty", "v0.0.0-20261011000000-abcdef123456+dirty"},
		{"empty metadata", "dev", "", "dev"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := &debug.BuildInfo{Main: debug.Module{Version: tc.module}}
			if got := versionFromBuildInfo(tc.release, info); got != tc.want {
				t.Fatalf("version = %q, want %q", got, tc.want)
			}
		})
	}
	if got := versionFromBuildInfo("dev", nil); got != "dev" {
		t.Fatalf("missing build info: %q", got)
	}
}
