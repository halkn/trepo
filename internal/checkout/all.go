package checkout

import (
	"fmt"
	"runtime"
	"sync"

	"github.com/halkn/trepo/internal/config"
	"github.com/halkn/trepo/internal/git"
	"github.com/halkn/trepo/internal/repo"
)

// Finder collects checkouts across repositories.
type Finder struct {
	Git git.Runner
	Cwd string
}

// All lists the checkouts of every repository, in the order defined by Sort.
//
// Repositories are read concurrently because the default listing spans all of
// them and each one costs several git invocations. A repository that cannot be
// read is reported and skipped: one broken checkout must not hide the rest,
// and must not disappear silently either.
func (f Finder) All(repos []repo.Repo) ([]Checkout, []error) {
	var (
		mu     sync.Mutex
		result []Checkout
		errs   []error
		wg     sync.WaitGroup
	)

	limit := runtime.GOMAXPROCS(0)
	if limit < 1 {
		limit = 1
	}
	sem := make(chan struct{}, limit)

	for _, r := range repos {
		wg.Add(1)
		go func(r repo.Repo) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			cs, err := f.Repo(r)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", r.Slug(), err))
				return
			}
			result = append(result, cs...)
		}(r)
	}
	wg.Wait()

	Sort(result)
	return result, errs
}

// Repo lists the checkouts of one repository, reading the settings that
// repository is allowed to decide for itself.
func (f Finder) Repo(r repo.Repo) ([]Checkout, error) {
	rc := config.LoadRepo(f.Git, r.Root)
	l := Lister{Git: f.Git, Cwd: f.Cwd, Protected: rc.Protected}
	return l.Repo(r)
}
