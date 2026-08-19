package checkout_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/halkn/trepo/internal/checkout"
)

func TestUnder(t *testing.T) {
	tests := []struct {
		name          string
		parent, child string
		want          bool
	}{
		{"same directory", "/a/wt", "/a/wt", true},
		{"nested", "/a/wt", "/a/wt/sub", true},
		{"sibling sharing a prefix", "/a/wt", "/a/wt2", false},
		{"unrelated", "/a/wt", "/b/wt", false},
		{"parent is not under its child", "/a/wt/sub", "/a/wt", false},
		{"trailing slash", "/a/wt/", "/a/wt/sub", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checkout.Under(tt.parent, tt.child); got != tt.want {
				t.Errorf("Under(%q, %q) = %v, want %v", tt.parent, tt.child, got, tt.want)
			}
		})
	}
}

// git resolves symlinks in the paths it reports while the shell keeps the
// symlinked spelling of the same directory, which is the default state on
// macOS for anything below /tmp. Comparing the two as plain strings would let
// trepo delete the directory the user is standing in.
func TestUnderResolvesSymlinks(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if !checkout.Under(real, filepath.Join(link, "sub")) {
		t.Errorf("Under(%q, %q) = false, want true", real, filepath.Join(link, "sub"))
	}
	if !checkout.Under(link, real) {
		t.Errorf("Under(%q, %q) = false, want true", link, real)
	}
}

func TestIsProtected(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		patterns []string
		want     bool
	}{
		{
			name:     "matches a run of path segments anywhere",
			path:     "/home/u/repos/app/.claude/worktrees/agent-1",
			patterns: []string{".claude/worktrees"},
			want:     true,
		},
		{
			name:     "matches a single segment",
			path:     "/home/u/vendor/thing",
			patterns: []string{"vendor"},
			want:     true,
		},
		{
			name:     "does not match a partial segment",
			path:     "/home/u/vendored/thing",
			patterns: []string{"vendor"},
			want:     false,
		},
		{
			name:     "segments must be adjacent and in order",
			path:     "/home/u/.claude/other/worktrees/x",
			patterns: []string{".claude/worktrees"},
			want:     false,
		},
		{
			name:     "any pattern matching is enough",
			path:     "/home/u/vendor/thing",
			patterns: []string{"nope", "vendor"},
			want:     true,
		},
		{
			name:     "no patterns protects nothing",
			path:     "/home/u/vendor/thing",
			patterns: nil,
			want:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checkout.IsProtected(tt.path, tt.patterns); got != tt.want {
				t.Errorf("IsProtected(%q, %v) = %v, want %v", tt.path, tt.patterns, got, tt.want)
			}
		})
	}
}
