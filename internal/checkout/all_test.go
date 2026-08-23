package checkout_test

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/halkn/trepo/internal/checkout"
	"github.com/halkn/trepo/internal/git"
	"github.com/halkn/trepo/internal/gittest"
	"github.com/halkn/trepo/internal/repo"
)

func TestAllSpansRepositoriesInAStableOrder(t *testing.T) {
	beta := gittest.New(t)
	alpha := gittest.New(t)
	wt := filepath.Join(filepath.Dir(alpha.Dir), "wt-zeta")
	alpha.Git("worktree", "add", "-b", "zeta", wt)

	// Roots decide the order, so name them rather than relying on TempDir.
	repos := []repo.Repo{
		{Host: "github.com", Owner: "o", Name: "beta", Root: beta.Dir},
		{Host: "github.com", Owner: "o", Name: "alpha", Root: alpha.Dir},
	}
	f := checkout.Finder{Git: git.Exec{Env: alpha.Env()}, Cwd: "/elsewhere"}

	first, errs := f.All(repos)
	if len(errs) != 0 {
		t.Fatalf("errors: %v", errs)
	}
	if len(first) != 3 {
		t.Fatalf("got %d checkouts, want 3", len(first))
	}

	second, _ := f.All(repos)
	for i := range first {
		if first[i].Path != second[i].Path {
			t.Fatalf("order changed between runs at %d: %q then %q",
				i, first[i].Path, second[i].Path)
		}
	}

	// Within a repository the main checkout comes first, so the place most
	// commands mean by default is at the top of the picker.
	for i := 0; i < len(first)-1; i++ {
		if first[i].Repo.Root == first[i+1].Repo.Root &&
			first[i].Kind == checkout.KindWorktree && first[i+1].Kind == checkout.KindRepo {
			t.Errorf("worktree sorted before the main checkout at %d", i)
		}
	}
}

// One unreadable repository must not hide every other checkout, but it must
// not vanish without a word either.
func TestAllReportsPerRepositoryFailuresWithoutDroppingTheRest(t *testing.T) {
	good := gittest.New(t)
	repos := []repo.Repo{
		{Host: "github.com", Owner: "o", Name: "good", Root: good.Dir},
		{Host: "github.com", Owner: "o", Name: "broken", Root: filepath.Join(t.TempDir(), "absent")},
	}
	f := checkout.Finder{Git: git.Exec{Env: good.Env()}, Cwd: "/elsewhere"}

	got, errs := f.All(repos)
	if len(got) != 1 {
		t.Errorf("got %d checkouts, want the one from the readable repository", len(got))
	}
	if len(errs) != 1 {
		t.Errorf("got %d errors, want 1", len(errs))
	}
}

func TestAllAppliesPerRepositoryProtectedPatterns(t *testing.T) {
	fixture := gittest.New(t)
	fixture.Git("config", "--local", "--add", "trepo.protected", "sandboxes")
	wt := filepath.Join(filepath.Dir(fixture.Dir), "sandboxes", "one")
	fixture.Git("worktree", "add", "-b", "sandbox", wt)

	f := checkout.Finder{Git: git.Exec{Env: fixture.Env()}, Cwd: "/elsewhere"}
	got, errs := f.All([]repo.Repo{
		{Host: "github.com", Owner: "o", Name: "app", Root: fixture.Dir},
	})
	if len(errs) != 0 {
		t.Fatalf("errors: %v", errs)
	}
	if !find(t, got, wt).Has(checkout.FlagProtected) {
		t.Errorf("flags = %v, want protected", find(t, got, wt).Flags)
	}
}

// Listing needs the protected patterns of a repository and nothing else from
// its configuration. Reading the worktree template as well spends one git
// process per repository on an answer only add ever looks at.
func TestAllDoesNotReadTheWorktreeTemplate(t *testing.T) {
	fixture := gittest.New(t)
	spy := &countingRunner{inner: git.Exec{Env: fixture.Env()}}
	f := checkout.Finder{Git: spy, Cwd: "/elsewhere"}

	if _, errs := f.All([]repo.Repo{
		{Host: "github.com", Owner: "o", Name: "alpha", Root: fixture.Dir},
	}); len(errs) != 0 {
		t.Fatalf("errors: %v", errs)
	}
	for _, args := range spy.calls {
		if strings.Contains(strings.Join(args, " "), "worktreeTemplate") {
			t.Errorf("listing read the worktree template: %v", args)
		}
	}
}

type countingRunner struct {
	inner git.Runner
	mu    sync.Mutex
	calls [][]string
}

func (c *countingRunner) Run(dir string, args ...string) ([]byte, error) {
	c.mu.Lock()
	c.calls = append(c.calls, args)
	c.mu.Unlock()
	return c.inner.Run(dir, args...)
}
