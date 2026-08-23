package remote_test

import (
	"testing"

	"github.com/halkn/trepo/internal/remote"
	"github.com/halkn/trepo/internal/repo"
)

func src(url string) repo.Source {
	s, err := repo.Parse(url, "github.com")
	if err != nil {
		panic(err)
	}
	return s
}

// A repository is one repository however its URL was spelled. Two rows for the
// same thing would make the caller pick between spellings that clone the same
// commit into the same directory.
func TestJoinFoldsTheSpellingsOfOneRepository(t *testing.T) {
	got := remote.Join([]repo.Source{
		src("git@github.com:halkn/api.git"),
		src("https://github.com/halkn/api"),
		src("halkn/web"),
	}, nil)

	if len(got) != 2 {
		t.Fatalf("got %d candidates, want 2: %+v", len(got), got)
	}
}

// What is already on disk is not a candidate to acquire, but it is not absent
// either: a caller drawing the list has to be able to say "you have this one".
func TestJoinMarksWhatIsAlreadyLocal(t *testing.T) {
	got := remote.Join(
		[]repo.Source{src("halkn/api"), src("halkn/web")},
		[]repo.Repo{{Host: "github.com", Owner: "halkn", Name: "web", Root: "/repos/github.com/halkn/web"}},
	)

	byName := map[string]remote.Candidate{}
	for _, c := range got {
		byName[c.Repo.Name] = c
	}
	if byName["web"].Local != "/repos/github.com/halkn/web" {
		t.Errorf("web = %+v, want it marked local", byName["web"])
	}
	if byName["api"].Local != "" {
		t.Errorf("api = %+v, want it marked as not local", byName["api"])
	}
}

// Enumeration order is part of the output, the same way it is for checkouts: a
// caller's cursor has to land in the same place each run.
func TestJoinSortsByHostThenSlug(t *testing.T) {
	got := remote.Join([]repo.Source{
		src("halkn/web"),
		src("git@ssh.dev.azure.com:v3/org/proj/service"),
		src("halkn/api"),
	}, nil)

	var order []string
	for _, c := range got {
		order = append(order, c.Repo.Host+" "+c.Repo.Slug())
	}
	want := []string{
		"dev.azure.com org/proj/service",
		"github.com halkn/api",
		"github.com halkn/web",
	}
	for i := range want {
		if i >= len(order) || order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
}

// A local repository the provider did not report is not a candidate at all.
// Acquisition answers "what could I clone", and something already cloned that
// the account no longer lists is a question for the listing, not for this one.
func TestJoinDoesNotInventCandidatesFromLocalRepositories(t *testing.T) {
	got := remote.Join(nil, []repo.Repo{
		{Host: "github.com", Owner: "halkn", Name: "gone", Root: "/repos/github.com/halkn/gone"},
	})
	if len(got) != 0 {
		t.Errorf("got %d candidates, want none: %+v", len(got), got)
	}
}
