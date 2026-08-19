package cli

import (
	"runtime/debug"
	"testing"
)

// The answer has to be usable in a bug report, which means saying which build
// this is even when it was not installed from a tagged version. The inputs are
// what go 1.26.6 records for each way of building trepo.
func TestVersionLine(t *testing.T) {
	tests := []struct {
		name string
		info *debug.BuildInfo
		want string
	}{
		{
			name: "installed from a tag",
			info: &debug.BuildInfo{Main: debug.Module{Version: "v0.1.0"}},
			want: "trepo v0.1.0",
		},
		{
			name: "built from a checkout",
			info: &debug.BuildInfo{Main: debug.Module{
				Version: "v0.0.0-20260819071623-e6aa163cdd05+dirty",
			}},
			want: "trepo v0.0.0-20260819071623-e6aa163cdd05+dirty",
		},
		{
			// go build -buildvcs=false, and `go test` binaries.
			name: "built without version information",
			info: &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}},
			want: "trepo (devel)",
		},
		{
			name: "no build information at all",
			info: nil,
			want: "trepo unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := versionLine(tt.info); got != tt.want {
				t.Errorf("versionLine() = %q, want %q", got, tt.want)
			}
		})
	}
}
