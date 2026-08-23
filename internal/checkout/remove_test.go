package checkout_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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

// Nothing asks, so a checkout the guards want a decision on must survive. That
// is not a failure of the command: the guards did their job, so it reports as a
// skip, and the message has to carry the reason and the flag that would let it
// through.
func TestRemoveSkipsWhatNeedsADecisionAndSaysWhy(t *testing.T) {
	fixture := gittest.New(t)
	wtPath := filepath.Join(filepath.Dir(fixture.Dir), "wt-dirty")
	fixture.Git("worktree", "add", "-b", "dirty-one", wtPath)
	if err := os.WriteFile(filepath.Join(wtPath, "scratch.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	target := find(t, list(t, fixture), wtPath)
	err := remover(fixture).Remove(target, checkout.Base{Name: "main", Known: true})
	if !errors.Is(err, checkout.ErrSkipped) {
		t.Fatalf("Remove() = %v, want ErrSkipped", err)
	}
	for _, want := range []string{wtPath, "uncommitted", "--force"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Remove() = %q, want it to mention %q", err, want)
		}
	}
	if _, statErr := os.Stat(wtPath); statErr != nil {
		t.Errorf("worktree was removed anyway: %v", statErr)
	}
}

// --force is the decision the guards were waiting for, and it has to reach git
// too: `git worktree remove` refuses a dirty worktree on its own terms.
func TestRemoveForceTakesWhatTheGuardsHeldBack(t *testing.T) {
	fixture := gittest.New(t)
	wtPath := filepath.Join(filepath.Dir(fixture.Dir), "wt-dirty")
	fixture.Git("worktree", "add", "-b", "dirty-one", wtPath)
	if err := os.WriteFile(filepath.Join(wtPath, "scratch.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := remover(fixture)
	r.Force = true
	target := find(t, list(t, fixture), wtPath)

	if err := r.Remove(target, checkout.Base{Name: "main", Known: true}); err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(wtPath); !os.IsNotExist(statErr) {
		t.Errorf("worktree survived a forced removal: %v", statErr)
	}
}

// Reclaiming settles one question - a branch retired on the remote - and no
// other. Widening it to everything --force covers would make the unattended
// path the destructive one.
func TestRemoveReclaimStillHoldsBackOnUncommittedWork(t *testing.T) {
	fixture := gittest.New(t)
	wtPath := filepath.Join(filepath.Dir(fixture.Dir), "wt-dirty")
	fixture.Git("worktree", "add", "-b", "dirty-one", wtPath)
	if err := os.WriteFile(filepath.Join(wtPath, "scratch.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := remover(fixture)
	r.Reclaim = true
	target := find(t, list(t, fixture), wtPath)

	err := r.Remove(target, checkout.Base{Name: "main", Known: true})
	if !errors.Is(err, checkout.ErrSkipped) {
		t.Fatalf("Remove() = %v, want ErrSkipped", err)
	}
	if _, statErr := os.Stat(wtPath); statErr != nil {
		t.Errorf("reclaiming removed a worktree with uncommitted changes: %v", statErr)
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

// A caller hands trepo a path, and the checkout it belongs to has to be found
// from that alone — including when the directory no longer exists, which is
// exactly the case worth reclaiming. Asking git about the path cannot answer
// it: `git -C <gone>` fails before it can report anything.
func TestContainingFindsACheckoutWhoseDirectoryIsGone(t *testing.T) {
	fixture := gittest.New(t)
	wtPath := filepath.Join(filepath.Dir(fixture.Dir), "wt-gone")
	fixture.Git("worktree", "add", "-b", "gone", wtPath)
	if err := os.RemoveAll(wtPath); err != nil {
		t.Fatal(err)
	}
	all := list(t, fixture)

	got, ok := checkout.Containing(all, wtPath)
	if !ok {
		t.Fatalf("Containing() did not find %q among %v", wtPath, all)
	}
	if !checkout.SamePath(got.Path, wtPath) {
		t.Errorf("Containing() = %q, want %q", got.Path, wtPath)
	}
	if _, ok := checkout.Containing(all, filepath.Join(t.TempDir(), "nowhere")); ok {
		t.Error("Containing() matched a path that is not inside a checkout")
	}
}
