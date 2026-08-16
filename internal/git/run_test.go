package git_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/halkn/trepo/internal/git"
	"github.com/halkn/trepo/internal/gittest"
)

func TestExecRunReturnsStdoutUntouched(t *testing.T) {
	fixture := gittest.New(t)
	r := git.Exec{Env: fixture.Env()}

	out, err := r.Run(fixture.Dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(out), "main\n"; got != want {
		t.Errorf("Run() = %q, want %q", got, want)
	}
}

func TestOutputTrims(t *testing.T) {
	fixture := gittest.New(t)
	r := git.Exec{Env: fixture.Env()}

	got, err := git.Output(r, fixture.Dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if got != "main" {
		t.Errorf("Output() = %q, want %q", got, "main")
	}
}

// A failing git command is the normal way trepo learns about repository state,
// so the error has to say which command failed and what git printed.
func TestExecRunErrorCarriesCommandAndStderr(t *testing.T) {
	fixture := gittest.New(t)
	r := git.Exec{Env: fixture.Env()}

	_, err := r.Run(fixture.Dir, "rev-parse", "--verify", "refs/heads/nope")
	if err == nil {
		t.Fatal("Run() succeeded on a missing ref, want an error")
	}
	var gitErr *git.Error
	if !errors.As(err, &gitErr) {
		t.Fatalf("error is %T, want *git.Error", err)
	}
	if !strings.Contains(err.Error(), "rev-parse") {
		t.Errorf("error %q does not name the command", err)
	}
	if gitErr.ExitCode == 0 {
		t.Errorf("ExitCode = 0, want non-zero")
	}
}

// Asking whether a ref exists is a question, not a failure, so callers need a
// form that does not manufacture an error for the "no" answer.
func TestOK(t *testing.T) {
	fixture := gittest.New(t)
	r := git.Exec{Env: fixture.Env()}

	if !git.OK(r, fixture.Dir, "rev-parse", "--verify", "refs/heads/main") {
		t.Error("OK() = false for an existing ref, want true")
	}
	if git.OK(r, fixture.Dir, "rev-parse", "--verify", "refs/heads/nope") {
		t.Error("OK() = true for a missing ref, want false")
	}
}

func TestFakeRecordsCallsAndReplays(t *testing.T) {
	f := &git.Fake{
		Responses: map[string]git.Response{
			"rev-parse --abbrev-ref HEAD": {Stdout: "main\n"},
			"rev-parse --verify nope":     {Err: &git.Error{ExitCode: 128}},
		},
	}

	if got, _ := git.Output(f, "/somewhere", "rev-parse", "--abbrev-ref", "HEAD"); got != "main" {
		t.Errorf("Output() = %q, want main", got)
	}
	if _, err := f.Run("/somewhere", "rev-parse", "--verify", "nope"); err == nil {
		t.Error("Run() on a scripted failure returned nil error")
	}
	if len(f.Calls) != 2 {
		t.Fatalf("recorded %d calls, want 2", len(f.Calls))
	}
	if f.Calls[0].Dir != "/somewhere" {
		t.Errorf("Calls[0].Dir = %q, want /somewhere", f.Calls[0].Dir)
	}
}

// An unscripted command must be loud: a silent empty string would show up much
// later as a wrong flag on a checkout.
func TestFakeFailsOnUnscriptedCommand(t *testing.T) {
	f := &git.Fake{}
	if _, err := f.Run("/somewhere", "status", "--porcelain"); err == nil {
		t.Error("Run() on an unscripted command returned nil error")
	}
}
