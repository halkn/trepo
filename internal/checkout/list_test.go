package checkout_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/halkn/trepo/internal/checkout"
	"github.com/halkn/trepo/internal/git"
	"github.com/halkn/trepo/internal/gittest"
	"github.com/halkn/trepo/internal/repo"
)

func lister(fixture *gittest.Repo, cwd string, protected ...string) checkout.Lister {
	return checkout.Lister{
		Git:       git.Exec{Env: fixture.Env()},
		Cwd:       cwd,
		Protected: protected,
	}
}

func rp(fixture *gittest.Repo) repo.Repo {
	return repo.Repo{Host: "github.com", Owner: "halkn", Name: "app", Root: fixture.Dir}
}

func find(t *testing.T, cs []checkout.Checkout, path string) checkout.Checkout {
	t.Helper()
	for _, c := range cs {
		if checkout.SamePath(c.Path, path) {
			return c
		}
	}
	t.Fatalf("no checkout at %q in %v", path, cs)
	return checkout.Checkout{}
}

func TestListReportsMainCheckoutAndWorktrees(t *testing.T) {
	fixture := gittest.New(t)
	wt := filepath.Join(filepath.Dir(fixture.Dir), "wt-feat")
	fixture.Git("worktree", "add", "-b", "feat/x", wt)

	got, err := lister(fixture, "/elsewhere").Repo(rp(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d checkouts, want 2", len(got))
	}
	main := find(t, got, fixture.Dir)
	if main.Kind != checkout.KindRepo || main.Branch != "main" {
		t.Errorf("main checkout = %+v", main)
	}
	linked := find(t, got, wt)
	if linked.Kind != checkout.KindWorktree || linked.Branch != "feat/x" {
		t.Errorf("worktree = %+v", linked)
	}
}

func TestListFlagsDirtyAndIgnored(t *testing.T) {
	fixture := gittest.New(t)
	fixture.Write(".gitignore", ".env\n")
	fixture.Commit("add: gitignore")
	fixture.Write("tracked.txt", "changed")
	fixture.Write(".env", "SECRET=1")

	got, err := lister(fixture, "/elsewhere").Repo(rp(fixture))
	if err != nil {
		t.Fatal(err)
	}
	c := find(t, got, fixture.Dir)
	if !c.Has(checkout.FlagDirty) {
		t.Errorf("flags = %v, want dirty", c.Flags)
	}
	// An ignored file is uncommitted work that `git status` stays silent about;
	// deleting a checkout over it loses a .env nobody can regenerate.
	if !c.Has(checkout.FlagIgnored) {
		t.Errorf("flags = %v, want ignored", c.Flags)
	}
}

func TestListFlagsCleanCheckoutHasNeither(t *testing.T) {
	fixture := gittest.New(t)

	got, err := lister(fixture, "/elsewhere").Repo(rp(fixture))
	if err != nil {
		t.Fatal(err)
	}
	c := find(t, got, fixture.Dir)
	if c.Has(checkout.FlagDirty) || c.Has(checkout.FlagIgnored) {
		t.Errorf("flags = %v, want neither dirty nor ignored", c.Flags)
	}
}

func TestListFlagsCurrent(t *testing.T) {
	fixture := gittest.New(t)
	wt := filepath.Join(filepath.Dir(fixture.Dir), "wt-feat")
	fixture.Git("worktree", "add", "-b", "feat/x", wt)

	got, err := lister(fixture, filepath.Join(wt, "sub")).Repo(rp(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if !find(t, got, wt).Has(checkout.FlagCurrent) {
		t.Error("the checkout holding the working directory is not marked current")
	}
	if find(t, got, fixture.Dir).Has(checkout.FlagCurrent) {
		t.Error("a checkout that does not hold the working directory is marked current")
	}
}

func TestListFlagsNoUpstream(t *testing.T) {
	fixture := gittest.New(t)
	wt := filepath.Join(filepath.Dir(fixture.Dir), "wt-feat")
	fixture.Git("worktree", "add", "-b", "feat/x", wt)

	got, err := lister(fixture, "/elsewhere").Repo(rp(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if !find(t, got, wt).Has(checkout.FlagNoUpstream) {
		t.Errorf("flags = %v, want no-upstream", find(t, got, wt).Flags)
	}
}

func TestListFlagsMerged(t *testing.T) {
	fixture := gittest.New(t)
	// A branch pointing at main has nothing main does not already have.
	wt := filepath.Join(filepath.Dir(fixture.Dir), "wt-merged")
	fixture.Git("worktree", "add", "-b", "done", wt)

	got, err := lister(fixture, "/elsewhere").Repo(rp(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if !find(t, got, wt).Has(checkout.FlagMerged) {
		t.Errorf("flags = %v, want merged", find(t, got, wt).Flags)
	}
}

func TestListFlagsUnmergedBranchIsNotMerged(t *testing.T) {
	fixture := gittest.New(t)
	wt := filepath.Join(filepath.Dir(fixture.Dir), "wt-work")
	fixture.Git("worktree", "add", "-b", "work", wt)
	if err := os.WriteFile(filepath.Join(wt, "new.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixture.GitIn(wt, "add", "-A")
	fixture.GitIn(wt, "commit", "-m", "add: work")

	got, err := lister(fixture, "/elsewhere").Repo(rp(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if find(t, got, wt).Has(checkout.FlagMerged) {
		t.Errorf("flags = %v, want merged absent", find(t, got, wt).Flags)
	}
}

func TestListFlagsProtected(t *testing.T) {
	fixture := gittest.New(t)
	wt := filepath.Join(fixture.Dir, ".claude", "worktrees", "agent-1")
	fixture.Git("worktree", "add", "-b", "agent", wt)

	got, err := lister(fixture, "/elsewhere", ".claude/worktrees").Repo(rp(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if !find(t, got, wt).Has(checkout.FlagProtected) {
		t.Errorf("flags = %v, want protected", find(t, got, wt).Flags)
	}
}

func TestListFlagsPrunableAfterTheDirectoryIsGone(t *testing.T) {
	fixture := gittest.New(t)
	wt := filepath.Join(filepath.Dir(fixture.Dir), "wt-gone")
	fixture.Git("worktree", "add", "-b", "gone", wt)
	if err := os.RemoveAll(wt); err != nil {
		t.Fatal(err)
	}

	got, err := lister(fixture, "/elsewhere").Repo(rp(fixture))
	if err != nil {
		t.Fatal(err)
	}
	c := find(t, got, wt)
	if !c.Has(checkout.FlagPrunable) {
		t.Errorf("flags = %v, want prunable", c.Flags)
	}
	// Nothing can be read from a directory that is gone, so no state derived
	// from its contents may be claimed.
	if c.Has(checkout.FlagDirty) {
		t.Errorf("flags = %v, want dirty absent for a missing directory", c.Flags)
	}
}

func TestListFlagsDetached(t *testing.T) {
	fixture := gittest.New(t)
	wt := filepath.Join(filepath.Dir(fixture.Dir), "wt-detached")
	fixture.Git("worktree", "add", "--detach", wt)

	got, err := lister(fixture, "/elsewhere").Repo(rp(fixture))
	if err != nil {
		t.Fatal(err)
	}
	c := find(t, got, wt)
	if !c.Has(checkout.FlagDetached) || c.Branch != "" {
		t.Errorf("checkout = %+v, want detached with no branch", c)
	}
}

func TestBaseRefPrefersOriginHead(t *testing.T) {
	upstream := gittest.NewBare(t)
	fixture := gittest.New(t)
	fixture.Git("remote", "add", "origin", upstream.Dir)
	fixture.Git("push", "-q", "-u", "origin", "main")
	fixture.Git("remote", "set-head", "origin", "-a")

	got := checkout.ResolveBase(git.Exec{Env: fixture.Env()}, fixture.Dir)
	if !got.Known || got.Name != "origin/main" {
		t.Errorf("ResolveBase() = %+v, want origin/main", got)
	}
}

// A repository that has never had a remote is ordinary, and trepo still has to
// answer what a branch would be merged into.
func TestBaseRefFallsBackToTheLocalDefaultBranch(t *testing.T) {
	fixture := gittest.New(t)

	got := checkout.ResolveBase(git.Exec{Env: fixture.Env()}, fixture.Dir)
	if !got.Known || got.Name != "main" {
		t.Errorf("ResolveBase() = %+v, want main", got)
	}
}

// Without a base there is no way to tell merged work from unmerged work, and
// saying "not merged" would be a claim trepo cannot support.
func TestBaseRefUnknownInARepositoryWithoutCommits(t *testing.T) {
	fixture := gittest.NewEmpty(t)

	got := checkout.ResolveBase(git.Exec{Env: fixture.Env()}, fixture.Dir)
	if got.Known {
		t.Errorf("ResolveBase() = %+v, want unknown", got)
	}
}

func TestListInARepositoryWithoutCommits(t *testing.T) {
	fixture := gittest.NewEmpty(t)

	got, err := lister(fixture, "/elsewhere").Repo(rp(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d checkouts, want 1", len(got))
	}
	if got[0].Branch != "" || !got[0].Has(checkout.FlagUnborn) {
		t.Errorf("checkout = %+v, want unborn with no branch", got[0])
	}
	if got[0].Has(checkout.FlagMerged) {
		t.Errorf("flags = %v, want merged absent when there is no base", got[0].Flags)
	}
}

// Reachability is what decides whether removing a checkout loses work, and a
// detached HEAD has it just as much as a branch does.
func TestListFlagsMergedForADetachedCheckout(t *testing.T) {
	fixture := gittest.New(t)
	at := filepath.Join(filepath.Dir(fixture.Dir), "wt-at-main")
	fixture.Git("worktree", "add", "--detach", at)

	ahead := filepath.Join(filepath.Dir(fixture.Dir), "wt-ahead")
	fixture.Git("worktree", "add", "--detach", ahead)
	if _, err := fixture.TryGitIn(ahead, "commit", "--allow-empty", "-m", "add: loose work"); err != nil {
		t.Fatal(err)
	}

	got, err := lister(fixture, "/elsewhere").Repo(rp(fixture))
	if err != nil {
		t.Fatal(err)
	}
	if !find(t, got, at).Has(checkout.FlagMerged) {
		t.Errorf("flags = %v, want merged for a detached checkout at the base",
			find(t, got, at).Flags)
	}
	if find(t, got, ahead).Has(checkout.FlagMerged) {
		t.Errorf("flags = %v, want merged absent for a detached checkout ahead of the base",
			find(t, got, ahead).Flags)
	}
}
