package git_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/halkn/trepo/internal/git"
	"github.com/halkn/trepo/internal/gittest"
)

// z builds the NUL-separated form git emits: every attribute is terminated by
// a NUL and an extra NUL closes the record.
func z(records ...[]string) []byte {
	var b strings.Builder
	for _, rec := range records {
		for _, attr := range rec {
			b.WriteString(attr)
			b.WriteByte(0)
		}
		b.WriteByte(0)
	}
	return []byte(b.String())
}

func TestParseWorktreeList(t *testing.T) {
	out := z(
		[]string{"worktree /repos/trepo", "HEAD abc123", "branch refs/heads/main"},
		[]string{"worktree /wt/feat-x", "HEAD abc123", "branch refs/heads/feat/x"},
		[]string{"worktree /wt/gone", "HEAD abc123", "branch refs/heads/old",
			"prunable gitdir file points to non-existent location"},
		[]string{"worktree /wt/detached", "HEAD abc123", "detached", "locked"},
	)

	got, err := git.ParseWorktreeList(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("parsed %d worktrees, want 4", len(got))
	}

	if got[0].Path != "/repos/trepo" || got[0].Branch != "main" {
		t.Errorf("first entry = %+v", got[0])
	}
	// A branch name keeps its slashes; only the refs/heads/ prefix goes.
	if got[1].Branch != "feat/x" {
		t.Errorf("Branch = %q, want feat/x", got[1].Branch)
	}
	if !got[2].Prunable {
		t.Errorf("entry with a prunable attribute is not marked prunable: %+v", got[2])
	}
	if !got[3].Detached || got[3].Branch != "" {
		t.Errorf("detached entry = %+v", got[3])
	}
	if !got[3].Locked {
		t.Errorf("locked entry is not marked locked: %+v", got[3])
	}
}

// git prints the main checkout first and trepo relies on that, because
// deriving it from the git dir is wrong for bare repositories, for clones made
// with --separate-git-dir and for submodules.
func TestParseWorktreeListMarksTheFirstEntryAsMain(t *testing.T) {
	out := z(
		[]string{"worktree /repos/trepo", "HEAD abc123", "branch refs/heads/main"},
		[]string{"worktree /wt/feat-x", "HEAD abc123", "branch refs/heads/feat/x"},
	)

	got, err := git.ParseWorktreeList(out)
	if err != nil {
		t.Fatal(err)
	}
	if !got[0].Main {
		t.Error("first entry is not marked as the main checkout")
	}
	if got[1].Main {
		t.Error("second entry is marked as the main checkout")
	}
}

func TestParseWorktreeListBare(t *testing.T) {
	got, err := git.ParseWorktreeList(z([]string{"worktree /repos/thing.git", "bare"}))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !got[0].Bare {
		t.Fatalf("got %+v, want one bare entry", got)
	}
}

// A repository whose first commit is still missing reports an all-zero HEAD
// alongside a branch that does not exist yet. Treating that branch as real
// would make trepo believe there is something to compare against.
func TestParseWorktreeListUnbornHead(t *testing.T) {
	got, err := git.ParseWorktreeList(z(
		[]string{"worktree /repos/fresh", "HEAD 0000000000000000000000000000000000000000",
			"branch refs/heads/main"},
	))
	if err != nil {
		t.Fatal(err)
	}
	if !got[0].Unborn {
		t.Errorf("entry with an all-zero HEAD is not marked unborn: %+v", got[0])
	}
}

// Paths are emitted raw, so a newline in one would split a line-based parser's
// record in two and shift every field after it.
func TestParseWorktreeListPathWithNewline(t *testing.T) {
	got, err := git.ParseWorktreeList(z(
		[]string{"worktree /repos/od\nd", "HEAD abc123", "branch refs/heads/main"},
	))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("parsed %d worktrees, want 1", len(got))
	}
	if got[0].Path != "/repos/od\nd" {
		t.Errorf("Path = %q, want the newline preserved", got[0].Path)
	}
}

func TestParseWorktreeListEmpty(t *testing.T) {
	got, err := git.ParseWorktreeList(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("got %+v, want none", got)
	}
}

// The fixtures above encode a format trepo does not control, so one case runs
// against real git to catch the format drifting out from under them.
func TestListWorktreesAgainstRealGit(t *testing.T) {
	fixture := gittest.New(t)
	wt := filepath.Join(filepath.Dir(fixture.Dir), "wt-feat")
	fixture.Git("worktree", "add", "-b", "feat/x", wt)

	got, err := git.ListWorktrees(git.Exec{Env: fixture.Env()}, fixture.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d worktrees, want 2", len(got))
	}
	if !got[0].Main || got[0].Branch != "main" {
		t.Errorf("main checkout = %+v", got[0])
	}
	if got[1].Main || got[1].Branch != "feat/x" {
		t.Errorf("linked worktree = %+v", got[1])
	}
}
