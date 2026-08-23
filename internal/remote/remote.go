// Package remote lists the repositories that could be cloned, and says which
// of them are already here.
//
// It is trepo's acquisition mode and nothing else: no command outside it needs
// the network, so a missing token or an unreachable host stops this one and
// leaves reaching a checkout alone.
package remote

import (
	"sort"

	"github.com/halkn/trepo/internal/repo"
)

// Runner executes an external command and returns its stdout.
//
// It mirrors git.Runner rather than reusing it because the command being run is
// not git: keeping the two apart is what stops a provider from quietly issuing
// git commands, and lets a test stand in for gh alone.
type Runner interface {
	Run(dir string, args ...string) ([]byte, error)
}

// Provider lists the repositories one account or organisation can offer.
type Provider interface {
	// Name identifies the provider in cache files and in errors.
	Name() string

	// List reports every repository the owners hold. An error means the answer
	// is unknown, never that there is nothing: reporting no repositories
	// because a token expired would read as an empty account.
	List(owners []string) ([]repo.Source, error)
}

// Candidate is a repository that could be cloned, and where it already is.
type Candidate struct {
	Repo     repo.Repo
	CloneURL string

	// Local is the path of the main checkout when trepo already has this
	// repository, and empty when it does not.
	Local string
}

// Join folds the reported repositories onto one row each and marks the ones
// already on disk.
//
// The fold is by the location a repository would occupy under the trepo root,
// which is what repo.Parse already derives from every spelling of a URL. Two
// rows that clone the same commit into the same directory are one candidate,
// however differently the provider and the local clone spell it.
//
// Local repositories the provider did not report are left out. This answers
// what could be acquired; what is already here is the listing's question.
func Join(sources []repo.Source, local []repo.Repo) []Candidate {
	here := make(map[string]string, len(local))
	for _, r := range local {
		here[r.RelPath()] = r.Root
	}

	seen := make(map[string]bool, len(sources))
	out := make([]Candidate, 0, len(sources))
	for _, s := range sources {
		key := s.Repo.RelPath()
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, Candidate{Repo: s.Repo, CloneURL: s.CloneURL, Local: here[key]})
	}

	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].Repo, out[j].Repo
		if a.Host != b.Host {
			return a.Host < b.Host
		}
		return a.Slug() < b.Slug()
	})
	return out
}
