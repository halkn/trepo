package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/halkn/trepo/internal/checkout"
	"github.com/halkn/trepo/internal/remote"
	"github.com/halkn/trepo/internal/repo"
)

// cacheTTL is how long a provider's answer is reused. Short, because the list
// exists to find a repository made moments ago; long enough that moving the
// cursor through a picker does not call the network.
const cacheTTL = time.Hour

// remoteCandidates lists what could be cloned, marking what is already here.
//
// Acquisition is its own entry point rather than a flag on list. It needs the
// network and a token, it is used a few times a week where list is used dozens
// of times a day, and keeping them apart is what lets a broken token stop this
// command without touching the ability to reach a checkout.
func (a *app) remoteCandidates(args []string) int {
	flags, query, code, ok := a.parse(args, spec{"missing": false, "refresh": false})
	if !ok {
		return code
	}

	sources, err := a.sources(flags["refresh"] == "true")
	if err != nil {
		return fail(a.stderr, err)
	}
	local, err := repo.Discover(a.cfg.Root)
	if err != nil {
		return fail(a.stderr, err)
	}

	onlyMissing := flags["missing"] == "true"
	for _, c := range remote.Join(sources, local) {
		if onlyMissing && c.Local != "" {
			continue
		}
		if !matches(c, query) {
			continue
		}
		state, path := "remote", "-"
		if c.Local != "" {
			// Resolved, like every other path trepo prints, so that a path
			// taken from this list compares equal to one taken from the
			// listing.
			state, path = "local", checkout.Resolve(c.Local)
		}
		fmt.Fprintln(a.stdout, strings.Join([]string{c.Repo.Host, c.Repo.Slug(), state, path}, "\t"))
	}
	return ExitOK
}

// sources asks the provider, or reuses its last answer.
//
// A failure is reported rather than turned into an empty list. "Your account
// holds nothing" and "trepo could not ask" send the caller to different places,
// and the second is what an expired token looks like.
func (a *app) sources(refresh bool) ([]repo.Source, error) {
	provider := remote.GitHub{Run: remote.Command{Name: "gh"}}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	cache := remote.Cache{Dir: remote.DefaultDir(home), TTL: cacheTTL}

	if !refresh {
		if sources, ok := cache.Load(provider.Name()); ok {
			return sources, nil
		}
	}

	sources, err := provider.List(a.cfg.RemoteOwners)
	if err != nil {
		return nil, err
	}
	if err := cache.Save(provider.Name(), sources); err != nil {
		// The answer is already correct, so a cache that cannot be written
		// costs the next run a network call and nothing else.
		fmt.Fprintln(a.stderr, "trepo: "+oneline(err))
	}
	return sources, nil
}

// matches narrows candidates the way filter narrows checkouts, so one query
// means the same thing in both modes.
func matches(c remote.Candidate, query []string) bool {
	haystack := strings.ToLower(c.Repo.Host + " " + c.Repo.Slug())
	for _, term := range query {
		if !strings.Contains(haystack, strings.ToLower(term)) {
			return false
		}
	}
	return true
}
