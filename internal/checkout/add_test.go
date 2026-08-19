package checkout_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/halkn/trepo/internal/checkout"
	"github.com/halkn/trepo/internal/config"
	"github.com/halkn/trepo/internal/git"
	"github.com/halkn/trepo/internal/gittest"
	"github.com/halkn/trepo/internal/repo"
)

func adder(t *testing.T, fixture *gittest.Repo) (checkout.Adder, string) {
	t.Helper()
	root := t.TempDir()
	return checkout.Adder{
		Git:          git.Exec{Env: fixture.Env()},
		WorktreeRoot: root,
		Template:     config.DefaultWorktreeTemplate,
	}, root
}

func TestAddCreatesWorktreeAtTheTemplatedPath(t *testing.T) {
	fixture := gittest.New(t)
	a, root := adder(t, fixture)

	got, err := a.Add(rp(fixture), "feat/x", "")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "halkn", "app", "feat", "x")
	if !checkout.SamePath(got, want) {
		t.Errorf("Add() = %q, want %q", got, want)
	}
}

// Flattening the slash would put feat/x and feat-x in the same directory, and
// the second one created would fail with an error about a path nobody named.
func TestAddKeepsBranchSlashesAsDirectories(t *testing.T) {
	fixture := gittest.New(t)
	a, _ := adder(t, fixture)

	slashed, err := a.Add(rp(fixture), "feat/x", "")
	if err != nil {
		t.Fatal(err)
	}
	dashed, err := a.Add(rp(fixture), "feat-x", "")
	if err != nil {
		t.Fatal(err)
	}
	if checkout.SamePath(slashed, dashed) {
		t.Errorf("feat/x and feat-x both landed at %q", slashed)
	}
}

// Asking for a checkout that already exists is a request to go there, not an
// error: the caller wants a path to cd into either way.
func TestAddIsIdempotent(t *testing.T) {
	fixture := gittest.New(t)
	a, _ := adder(t, fixture)

	first, err := a.Add(rp(fixture), "feat/x", "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := a.Add(rp(fixture), "feat/x", "")
	if err != nil {
		t.Fatalf("second Add() failed: %v", err)
	}
	if !checkout.SamePath(first, second) {
		t.Errorf("Add() returned %q then %q", first, second)
	}
}

// A branch that only exists on the remote holds work someone already pushed.
// Branching off the integration branch instead would silently start from
// nothing and leave that work behind.
func TestAddTracksAnExistingRemoteBranch(t *testing.T) {
	upstream := gittest.NewBare(t)
	fixture := gittest.New(t)
	fixture.Git("remote", "add", "origin", upstream.Dir)
	fixture.Git("push", "-q", "-u", "origin", "main")
	fixture.Git("switch", "-q", "-c", "shared")
	fixture.Write("shared.txt", "from the remote")
	fixture.Commit("add: shared work")
	fixture.Git("push", "-q", "-u", "origin", "shared")
	fixture.Git("switch", "-q", "main")
	fixture.Git("branch", "-D", "shared")

	a, _ := adder(t, fixture)
	path, err := a.Add(rp(fixture), "shared", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.TryGitIn(path, "cat-file", "-e", "HEAD:shared.txt"); err != nil {
		t.Errorf("the remote branch's work is missing from %q: %v", path, err)
	}
}

// An explicit base states the intent to start something new, so the remote
// branch of the same name must not be picked up instead.
func TestAddWithAnExplicitBaseBranchesFromIt(t *testing.T) {
	fixture := gittest.New(t)
	fixture.Write("second.txt", "x")
	fixture.Commit("add: second commit")
	first := fixture.Git("rev-parse", "HEAD~1")

	a, _ := adder(t, fixture)
	path, err := a.Add(rp(fixture), "from-first", first)
	if err != nil {
		t.Fatal(err)
	}
	got, err := fixture.TryGitIn(path, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if got != first {
		t.Errorf("HEAD = %q, want the requested base %q", got, first)
	}
}

// A branch created off origin/main inherits it as upstream, and push.default =
// simple then refuses the first push because the names differ.
func TestAddDoesNotLeaveAnInheritedUpstream(t *testing.T) {
	upstream := gittest.NewBare(t)
	fixture := gittest.New(t)
	fixture.Git("remote", "add", "origin", upstream.Dir)
	fixture.Git("push", "-q", "-u", "origin", "main")
	fixture.Git("remote", "set-head", "origin", "-a")

	a, _ := adder(t, fixture)
	if _, err := a.Add(rp(fixture), "feat/x", ""); err != nil {
		t.Fatal(err)
	}
	got, _ := fixture.TryGit("for-each-ref", "--format=%(upstream:short)", "refs/heads/feat/x")
	if got != "" {
		t.Errorf("upstream = %q, want none", got)
	}
}

// The default template leaves the host out, so two repositories with the same
// owner and name on different hosts want the same directory. Creating over the
// other one's checkout would be silent damage.
func TestAddRefusesADirectoryOwnedByAnotherRepository(t *testing.T) {
	other := gittest.New(t)
	fixture := gittest.New(t)
	a, root := adder(t, fixture)

	otherRepo := repo.Repo{Host: "gitlab.com", Owner: "halkn", Name: "app", Root: other.Dir}
	if _, err := (checkout.Adder{
		Git: git.Exec{Env: other.Env()}, WorktreeRoot: root, Template: config.DefaultWorktreeTemplate,
	}).Add(otherRepo, "feat/x", ""); err != nil {
		t.Fatal(err)
	}

	_, err := a.Add(rp(fixture), "feat/x", "")
	if err == nil {
		t.Fatal("Add() overwrote a directory belonging to another repository")
	}
	if !strings.Contains(err.Error(), "worktreeTemplate") {
		t.Errorf("error %q does not say how to resolve the collision", err)
	}
}

// The template comes from configuration, so it must not be able to place a
// checkout outside the root the user set aside for them.
func TestAddRefusesATemplateEscapingTheRoot(t *testing.T) {
	fixture := gittest.New(t)
	a, _ := adder(t, fixture)
	a.Template = "../outside/{{.Branch}}"

	if _, err := a.Add(rp(fixture), "feat/x", ""); err == nil {
		t.Error("Add() accepted a template pointing outside the worktree root")
	}
}

func TestAddRejectsAnEmptyBranch(t *testing.T) {
	fixture := gittest.New(t)
	a, _ := adder(t, fixture)

	if _, err := a.Add(rp(fixture), "", ""); err == nil {
		t.Error("Add() accepted an empty branch name")
	}
}

// An upstream the user set on purpose is theirs. Only a branch created here off
// a base ref can have inherited one, so only that case may drop it.
func TestAddKeepsAnUpstreamSetOnAnExistingBranch(t *testing.T) {
	upstream := gittest.NewBare(t)
	fixture := gittest.New(t)
	fixture.Git("remote", "add", "origin", upstream.Dir)
	fixture.Git("push", "-q", "-u", "origin", "main")
	fixture.Git("branch", "feature")
	fixture.Git("branch", "--set-upstream-to=origin/main", "feature")

	a, _ := adder(t, fixture)
	if _, err := a.Add(rp(fixture), "feature", ""); err != nil {
		t.Fatal(err)
	}

	got, _ := fixture.TryGit("for-each-ref", "--format=%(upstream:short)", "refs/heads/feature")
	if got != "origin/main" {
		t.Errorf("upstream = %q, want the one the user set", got)
	}
}

// The record of a worktree whose directory was deleted still occupies the
// branch, so a plain retry fails; asking again is a request to get the checkout
// back, not to be told about bookkeeping.
func TestAddRecreatesAWorktreeWhoseDirectoryIsGone(t *testing.T) {
	fixture := gittest.New(t)
	a, _ := adder(t, fixture)

	first, err := a.Add(rp(fixture), "feat/x", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(first); err != nil {
		t.Fatal(err)
	}

	second, err := a.Add(rp(fixture), "feat/x", "")
	if err != nil {
		t.Fatalf("Add() after the directory was deleted failed: %v", err)
	}
	if !checkout.SamePath(first, second) {
		t.Errorf("Add() = %q, want it back at %q", second, first)
	}
	if _, err := os.Stat(second); err != nil {
		t.Errorf("the printed path does not exist: %v", err)
	}
}
