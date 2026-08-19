package checkout

import (
	"path/filepath"
	"strings"
)

// Under reports whether child is parent or lives below it.
//
// Both sides go through symlink resolution first. git reports the resolved
// spelling of a path while a shell keeps the symlinked one — the default
// situation for anything below /tmp on macOS — so a plain string comparison
// answers "no" for two names of the same directory. The comparison is by path
// segment as well, so that /a/wt2 is not read as living under /a/wt.
func Under(parent, child string) bool {
	parent = Resolve(parent)
	child = Resolve(child)
	if parent == child {
		return true
	}
	return strings.HasPrefix(child, parent+string(filepath.Separator))
}

// SamePath reports whether two spellings name the same directory.
func SamePath(a, b string) bool { return Resolve(a) == Resolve(b) }

// Resolve is the canonical spelling of a path: absolute, cleaned, and with
// symlinks followed as far as the path exists. Every path trepo reports goes
// through it, so that a path taken from trepo's output compares equal to what
// git reports for the same directory.
func Resolve(p string) string { return resolve(p) }

func resolve(p string) string {
	p = filepath.Clean(p)
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	// A path that does not exist cannot be resolved, which is normal here: a
	// removed worktree is exactly the case callers ask about.
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return real
	}
	// Resolve the longest existing ancestor so a missing leaf still compares
	// against the resolved spelling of the directories above it.
	dir, leaf := filepath.Split(p)
	if dir == "" || dir == p {
		return p
	}
	return filepath.Join(resolve(filepath.Clean(dir)), leaf)
}

// IsProtected reports whether path lies in a location the user has declared
// another tool owns.
//
// A pattern is a run of path segments, matched against consecutive segments of
// the path. This is deliberately not a glob: filepath.Match is the only
// matcher available without a dependency, its * does not cross a separator and
// it has no **, so the pattern everyone would reach for first —
// "*/.claude/worktrees/*" — could never match anything.
func IsProtected(path string, patterns []string) bool {
	segs := strings.Split(filepath.ToSlash(filepath.Clean(path)), "/")
	for _, pattern := range patterns {
		want := strings.Split(strings.Trim(filepath.ToSlash(pattern), "/"), "/")
		if len(want) == 0 || (len(want) == 1 && want[0] == "") {
			continue
		}
		if containsRun(segs, want) {
			return true
		}
	}
	return false
}

func containsRun(segs, want []string) bool {
	if len(want) > len(segs) {
		return false
	}
	for i := 0; i+len(want) <= len(segs); i++ {
		if slicesEqual(segs[i:i+len(want)], want) {
			return true
		}
	}
	return false
}

func slicesEqual(a, b []string) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
