package remote_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/halkn/trepo/internal/remote"
)

type fakeRunner struct {
	out  string
	err  error
	args [][]string
}

func (f *fakeRunner) Run(_ string, args ...string) ([]byte, error) {
	f.args = append(f.args, args)
	return []byte(f.out), f.err
}

// gh lists 30 repositories unless told otherwise, and a silently truncated
// account looks exactly like a small one.
func TestGitHubAsksForMoreThanTheDefaultPage(t *testing.T) {
	fake := &fakeRunner{out: "[]"}
	if _, err := (remote.GitHub{Run: fake}).List(nil); err != nil {
		t.Fatal(err)
	}
	if len(fake.args) != 1 {
		t.Fatalf("ran %d commands, want 1: %v", len(fake.args), fake.args)
	}
	if !strings.Contains(strings.Join(fake.args[0], " "), "--limit") {
		t.Errorf("gh was run without a limit: %v", fake.args[0])
	}
}

func TestGitHubReadsTheReportedRepositories(t *testing.T) {
	fake := &fakeRunner{out: `[
	  {"nameWithOwner":"halkn/api","sshUrl":"git@github.com:halkn/api.git"},
	  {"nameWithOwner":"halkn/web","sshUrl":"git@github.com:halkn/web.git"}
	]`}

	got, err := (remote.GitHub{Run: fake}).List(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d repositories, want 2: %+v", len(got), got)
	}
	if got[0].Repo.Host != "github.com" || got[0].Repo.Slug() != "halkn/api" {
		t.Errorf("first = %+v", got[0].Repo)
	}
	if got[0].CloneURL != "git@github.com:halkn/api.git" {
		t.Errorf("clone url = %q, want the one gh reported", got[0].CloneURL)
	}
}

// One command per owner, so that an organisation is asked after by name rather
// than hoped for in the account's own list.
func TestGitHubAsksAfterEachOwner(t *testing.T) {
	fake := &fakeRunner{out: "[]"}
	if _, err := (remote.GitHub{Run: fake}).List([]string{"halkn", "someorg"}); err != nil {
		t.Fatal(err)
	}
	if len(fake.args) != 2 {
		t.Fatalf("ran %d commands, want one per owner: %v", len(fake.args), fake.args)
	}
	for i, want := range []string{"halkn", "someorg"} {
		if !strings.Contains(strings.Join(fake.args[i], " "), want) {
			t.Errorf("command %d does not name %q: %v", i, want, fake.args[i])
		}
	}
}

// An expired token must not read as an account holding nothing. The caller has
// to be able to tell "you have no repositories" from "trepo could not ask".
func TestGitHubReportsAFailureRatherThanAnEmptyList(t *testing.T) {
	fake := &fakeRunner{out: "", err: errors.New("gh: authentication required")}

	got, err := (remote.GitHub{Run: fake}).List(nil)
	if err == nil {
		t.Fatalf("List() = %+v, nil; want an error", got)
	}
	if got != nil {
		t.Errorf("List() returned %d repositories alongside an error", len(got))
	}
}
