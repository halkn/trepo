package checkout_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/halkn/trepo/internal/checkout"
	"github.com/halkn/trepo/internal/git"
	"github.com/halkn/trepo/internal/gittest"
)

func finder(fixture *gittest.Repo, cwd string) checkout.Finder {
	return checkout.Finder{Git: git.Exec{Env: fixture.Env()}, Cwd: cwd}
}

// list re-reads the checkouts so assertions see what git actually has now.
func list(t *testing.T, fixture *gittest.Repo) []checkout.Checkout {
	t.Helper()
	cs, err := finder(fixture, "/elsewhere").Repo(rp(fixture))
	if err != nil {
		t.Fatal(err)
	}
	return cs
}

func remover(fixture *gittest.Repo) checkout.Remover {
	return checkout.Remover{Git: git.Exec{Env: fixture.Env()}}
}

func TestRemoveDeletesAMergedWorktreeAndItsBranch(t *testing.T) {
	fixture := gittest.New(t)
	wtPath := filepath.Join(filepath.Dir(fixture.Dir), "wt-done")
	fixture.Git("worktree", "add", "-b", "done", wtPath)

	target := find(t, list(t, fixture), wtPath)
	base := checkout.ResolveBase(git.Exec{Env: fixture.Env()}, fixture.Dir)
	if err := remover(fixture).Remove(target, base); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Errorf("worktree directory still present: %v", err)
	}
	if _, err := fixture.TryGit("rev-parse", "--verify", "refs/heads/done"); err == nil {
		t.Error("merged branch was left behind")
	}
}

// The branch is the only copy of unmerged work, so it outlives the checkout.
func TestRemoveKeepsAnUnmergedBranch(t *testing.T) {
	fixture := gittest.New(t)
	wtPath := filepath.Join(filepath.Dir(fixture.Dir), "wt-work")
	fixture.Git("worktree", "add", "-b", "work", wtPath)
	fixture.GitIn(wtPath, "commit", "--allow-empty", "-m", "add: work")

	target := find(t, list(t, fixture), wtPath)
	base := checkout.ResolveBase(git.Exec{Env: fixture.Env()}, fixture.Dir)
	r := remover(fixture)
	r.Force = true
	if err := r.Remove(target, base); err != nil {
		t.Fatal(err)
	}

	if _, err := fixture.TryGit("rev-parse", "--verify", "refs/heads/work"); err != nil {
		t.Error("unmerged branch was deleted along with its checkout")
	}
}

func TestRemoveRefusesTheMainCheckout(t *testing.T) {
	fixture := gittest.New(t)
	target := find(t, list(t, fixture), fixture.Dir)

	err := remover(fixture).Remove(target, checkout.Base{})
	if err == nil {
		t.Fatal("Remove() deleted the main checkout")
	}
	if _, statErr := os.Stat(fixture.Dir); statErr != nil {
		t.Errorf("main checkout is gone: %v", statErr)
	}
}

// Without a way to say yes, a checkout that needs confirmation must survive,
// and the error has to name the flag that would let it through.
func TestRemoveWithoutAConfirmerRefusesAndSaysHow(t *testing.T) {
	fixture := gittest.New(t)
	wtPath := filepath.Join(filepath.Dir(fixture.Dir), "wt-dirty")
	fixture.Git("worktree", "add", "-b", "dirty-one", wtPath)
	if err := os.WriteFile(filepath.Join(wtPath, "scratch.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	target := find(t, list(t, fixture), wtPath)
	err := remover(fixture).Remove(target, checkout.Base{Name: "main", Known: true})
	if err == nil {
		t.Fatal("Remove() dropped uncommitted work without asking")
	}
	if _, statErr := os.Stat(wtPath); statErr != nil {
		t.Errorf("worktree was removed anyway: %v", statErr)
	}
}

func TestRemoveSkipsWhenTheConfirmerSaysNo(t *testing.T) {
	fixture := gittest.New(t)
	wtPath := filepath.Join(filepath.Dir(fixture.Dir), "wt-dirty")
	fixture.Git("worktree", "add", "-b", "dirty-one", wtPath)
	if err := os.WriteFile(filepath.Join(wtPath, "scratch.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := remover(fixture)
	r.Confirm = func(checkout.Checkout, checkout.Verdict) bool { return false }
	target := find(t, list(t, fixture), wtPath)

	err := r.Remove(target, checkout.Base{Name: "main", Known: true})
	if !errors.Is(err, checkout.ErrSkipped) {
		t.Fatalf("Remove() = %v, want ErrSkipped", err)
	}
	if _, statErr := os.Stat(wtPath); statErr != nil {
		t.Errorf("worktree was removed after the answer was no: %v", statErr)
	}
}

func TestRemoveProceedsWhenTheConfirmerSaysYes(t *testing.T) {
	fixture := gittest.New(t)
	wtPath := filepath.Join(filepath.Dir(fixture.Dir), "wt-dirty")
	fixture.Git("worktree", "add", "-b", "dirty-one", wtPath)
	if err := os.WriteFile(filepath.Join(wtPath, "scratch.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := remover(fixture)
	r.Confirm = func(checkout.Checkout, checkout.Verdict) bool { return true }
	target := find(t, list(t, fixture), wtPath)

	if err := r.Remove(target, checkout.Base{Name: "main", Known: true}); err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(wtPath); !os.IsNotExist(statErr) {
		t.Errorf("worktree survived a confirmed removal: %v", statErr)
	}
}

// A worktree whose directory a user deleted by hand still occupies git's
// records, and reclaiming it is the case a removal command is most wanted for.
func TestRemoveReclaimsAWorktreeWhoseDirectoryIsGone(t *testing.T) {
	fixture := gittest.New(t)
	wtPath := filepath.Join(filepath.Dir(fixture.Dir), "wt-gone")
	fixture.Git("worktree", "add", "-b", "gone", wtPath)
	if err := os.RemoveAll(wtPath); err != nil {
		t.Fatal(err)
	}

	target := find(t, list(t, fixture), wtPath)
	if err := remover(fixture).Remove(target, checkout.Base{}); err != nil {
		t.Fatal(err)
	}

	for _, c := range list(t, fixture) {
		if checkout.SamePath(c.Path, wtPath) {
			t.Errorf("the record for %q is still there", wtPath)
		}
	}
}

func TestRemoveDryRunChangesNothing(t *testing.T) {
	fixture := gittest.New(t)
	wtPath := filepath.Join(filepath.Dir(fixture.Dir), "wt-done")
	fixture.Git("worktree", "add", "-b", "done", wtPath)

	r := remover(fixture)
	r.DryRun = true
	target := find(t, list(t, fixture), wtPath)

	if err := r.Remove(target, checkout.Base{Name: "main", Known: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(wtPath); err != nil {
		t.Errorf("dry run removed the worktree: %v", err)
	}
	if _, err := fixture.TryGit("rev-parse", "--verify", "refs/heads/done"); err != nil {
		t.Error("dry run deleted the branch")
	}
}

// The picker hands trepo a path, and the repository it belongs to has to be
// found from that alone — including when the directory no longer exists.
func TestLocate(t *testing.T) {
	fixture := gittest.New(t)
	wtPath := filepath.Join(filepath.Dir(fixture.Dir), "wt-gone")
	fixture.Git("worktree", "add", "-b", "gone", wtPath)
	if err := os.RemoveAll(wtPath); err != nil {
		t.Fatal(err)
	}
	all := list(t, fixture)

	got, ok := checkout.Locate(all, wtPath)
	if !ok {
		t.Fatalf("Locate() did not find %q among %v", wtPath, all)
	}
	if !checkout.SamePath(got.Path, wtPath) {
		t.Errorf("Locate() = %q, want %q", got.Path, wtPath)
	}
	if _, ok := checkout.Locate(all, filepath.Join(t.TempDir(), "nowhere")); ok {
		t.Error("Locate() matched a path that is not a checkout")
	}
}
