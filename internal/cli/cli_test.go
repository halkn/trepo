package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/halkn/trepo/internal/checkout"
	"github.com/halkn/trepo/internal/cli"
	"github.com/halkn/trepo/internal/git"
	"github.com/halkn/trepo/internal/gittest"
	"github.com/halkn/trepo/internal/picker"
)

// world is a trepo installation with its own roots and its own git
// configuration, so a run in a test never reads or writes the real ones.
type world struct {
	t       *testing.T
	fixture *gittest.Repo
	root    string
	wtRoot  string
	cwd     string
	git     git.Runner
}

func newWorld(t *testing.T) *world {
	t.Helper()
	base := t.TempDir()
	w := &world{
		t:       t,
		fixture: gittest.New(t),
		root:    filepath.Join(base, "repos"),
		wtRoot:  filepath.Join(base, "worktrees"),
		cwd:     base,
	}
	w.fixture.Git("config", "--global", "trepo.root", w.root)
	w.fixture.Git("config", "--global", "trepo.worktreeRoot", w.wtRoot)
	w.git = git.Exec{Env: w.fixture.Env()}
	return w
}

// addRepo puts a repository into the root the way `trepo get` would.
func (w *world) addRepo(host, owner, name string) string {
	w.t.Helper()
	dir := filepath.Join(w.root, host, owner, name)
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		w.t.Fatal(err)
	}
	if _, err := w.fixture.TryGitIn(filepath.Dir(dir), "init", "--initial-branch=main", name); err != nil {
		w.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("x"), 0o644); err != nil {
		w.t.Fatal(err)
	}
	if _, err := w.fixture.TryGitIn(dir, "add", "-A"); err != nil {
		w.t.Fatal(err)
	}
	if _, err := w.fixture.TryGitIn(dir, "commit", "-m", "add: initial"); err != nil {
		w.t.Fatal(err)
	}
	return dir
}

func (w *world) run(args ...string) (code int, stdout, stderr string) {
	w.t.Helper()
	var out, errBuf bytes.Buffer
	code = cli.Run(args, &out, &errBuf, cli.Options{Git: w.git, Cwd: w.cwd})
	return code, out.String(), errBuf.String()
}

func TestListIsEmptyWithoutRepositories(t *testing.T) {
	w := newWorld(t)

	code, stdout, _ := w.run("list")
	if code != cli.ExitOK {
		t.Errorf("exit = %d, want %d", code, cli.ExitOK)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
}

func TestListSpansRepositories(t *testing.T) {
	w := newWorld(t)
	w.addRepo("github.com", "halkn", "alpha")
	w.addRepo("github.com", "halkn", "beta")

	code, stdout, _ := w.run("list")
	if code != cli.ExitOK {
		t.Fatalf("exit = %d", code)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 2 {
		t.Fatalf("listed %d checkouts, want 2:\n%s", len(lines), stdout)
	}
	for _, line := range lines {
		if n := len(strings.Split(line, "\t")); n != 4 {
			t.Errorf("line %q has %d columns, want 4", line, n)
		}
	}
}

// The listing must not depend on where the command was run, or the same query
// would answer differently from inside a repository than from outside it.
func TestListIsTheSameFromAnywhere(t *testing.T) {
	w := newWorld(t)
	inside := w.addRepo("github.com", "halkn", "alpha")
	w.addRepo("github.com", "halkn", "beta")

	_, outside, _ := w.run("list")
	w.cwd = inside
	_, within, _ := w.run("list")

	if countLines(outside) != countLines(within) {
		t.Errorf("listing differs by working directory:\noutside:\n%s\ninside:\n%s", outside, within)
	}
}

func TestListJSONShape(t *testing.T) {
	w := newWorld(t)
	w.addRepo("github.com", "halkn", "alpha")

	code, stdout, _ := w.run("list", "--json")
	if code != cli.ExitOK {
		t.Fatalf("exit = %d", code)
	}
	var got []struct {
		Repo  string   `json:"repo"`
		Kind  string   `json:"kind"`
		Flags []string `json:"flags"`
		Path  string   `json:"path"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("output is not a json array: %v\n%s", err, stdout)
	}
	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1", len(got))
	}
	if got[0].Repo != "halkn/alpha" || got[0].Kind != "repo" {
		t.Errorf("entry = %+v", got[0])
	}
}

// cloneSpy stands in for the clone itself. What get decides — the URL to hand
// to git and the directory to put the result in — is trepo's; whether git can
// reach that URL is git's, and testing it here would need a network.
type cloneSpy struct {
	inner git.Runner
	url   string
	dest  string
}

func (c *cloneSpy) Run(dir string, args ...string) ([]byte, error) {
	if len(args) > 0 && args[0] == "clone" {
		c.url, c.dest = args[1], args[2]
		return nil, os.MkdirAll(filepath.Join(c.dest, ".git"), 0o755)
	}
	return c.inner.Run(dir, args...)
}

func TestGetPlacesACloneByItsUrlAndPrintsOnlyThePath(t *testing.T) {
	w := newWorld(t)
	spy := &cloneSpy{inner: git.Exec{Env: w.fixture.Env()}}
	w.git = spy

	code, stdout, stderr := w.run("get", "git@github.com:halkn/alpha.git")
	if code != cli.ExitOK {
		t.Fatalf("exit = %d, stderr = %s", code, stderr)
	}
	if spy.url != "git@github.com:halkn/alpha.git" {
		t.Errorf("cloned from %q, want the url as given", spy.url)
	}
	want := filepath.Join(w.root, "github.com", "halkn", "alpha")
	if spy.dest != want {
		t.Errorf("cloned into %q, want %q", spy.dest, want)
	}
	if countLines(stdout) != 1 {
		t.Fatalf("stdout has %d lines, want exactly the path:\n%s", countLines(stdout), stdout)
	}
	if strings.TrimSpace(stdout) != resolved(t, want) {
		t.Errorf("printed %q, want %q", strings.TrimSpace(stdout), resolved(t, want))
	}
}

// Asking for a repository that is already there is a request for its path, not
// a conflict: the caller wants somewhere to cd into either way.
func TestGetIsIdempotent(t *testing.T) {
	w := newWorld(t)
	spy := &cloneSpy{inner: git.Exec{Env: w.fixture.Env()}}
	w.git = spy

	_, first, _ := w.run("get", "halkn/alpha")
	spy.url = ""
	code, second, _ := w.run("get", "halkn/alpha")
	if code != cli.ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if first != second {
		t.Errorf("get printed %q then %q", first, second)
	}
	if spy.url != "" {
		t.Errorf("cloned again into an existing checkout: %q", spy.url)
	}
}

func TestAddCreatesAWorktreeAndPrintsOnlyThePath(t *testing.T) {
	w := newWorld(t)
	w.cwd = w.addRepo("github.com", "halkn", "alpha")

	code, stdout, _ := w.run("add", "feat/x")
	if code != cli.ExitOK {
		t.Fatalf("exit = %d", code)
	}
	path := strings.TrimSpace(stdout)
	if countLines(stdout) != 1 {
		t.Fatalf("stdout has %d lines, want exactly the path:\n%s", countLines(stdout), stdout)
	}
	if !strings.HasPrefix(path, resolved(t, w.wtRoot)) {
		t.Errorf("worktree at %q, want it below %q", path, w.wtRoot)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("the printed path does not exist: %v", err)
	}
}

// Outside a repository there is nothing to guess from, and guessing would
// create a worktree in a repository the user never named.
func TestAddOutsideARepositoryExplainsHowToNameOne(t *testing.T) {
	w := newWorld(t)

	code, stdout, stderr := w.run("add", "feat/x")
	if code != cli.ExitError {
		t.Errorf("exit = %d, want %d", code, cli.ExitError)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "--repo") {
		t.Errorf("stderr %q does not mention --repo", stderr)
	}
}

func TestAddNamingTheRepositoryFromOutside(t *testing.T) {
	w := newWorld(t)
	w.addRepo("github.com", "halkn", "alpha")

	code, stdout, stderr := w.run("add", "feat/x", "--repo", "alpha")
	if code != cli.ExitOK {
		t.Fatalf("exit = %d, stderr = %s", code, stderr)
	}
	if _, err := os.Stat(strings.TrimSpace(stdout)); err != nil {
		t.Errorf("the printed path does not exist: %v", err)
	}
}

func TestPathFindsAUniqueMatchWithoutAsking(t *testing.T) {
	w := newWorld(t)
	dir := w.addRepo("github.com", "halkn", "alpha")

	code, stdout, _ := w.run("path", "alpha")
	if code != cli.ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if got := strings.TrimSpace(stdout); got != resolved(t, dir) {
		t.Errorf("path = %q, want %q", got, resolved(t, dir))
	}
}

// A shell wrapper reacts differently to "there is nothing called that" than to
// "the picker was dismissed", so the two must not share an exit status.
func TestPathWithNoMatchExitsOne(t *testing.T) {
	w := newWorld(t)
	w.addRepo("github.com", "halkn", "alpha")

	code, stdout, _ := w.run("path", "nothing-like-this")
	if code != cli.ExitNoMatch {
		t.Errorf("exit = %d, want %d", code, cli.ExitNoMatch)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
}

func TestStatusDescribesACheckout(t *testing.T) {
	w := newWorld(t)
	dir := w.addRepo("github.com", "halkn", "alpha")

	code, stdout, _ := w.run("status", dir)
	if code != cli.ExitOK {
		t.Fatalf("exit = %d", code)
	}
	for _, want := range []string{"halkn/alpha", "branch", "main", "path"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("status output does not mention %q:\n%s", want, stdout)
		}
	}
}

// status is what the picker previews with, so a stale path has to produce a
// readable line instead of an error that replaces the preview pane.
func TestStatusOnAMissingPathStaysCalm(t *testing.T) {
	w := newWorld(t)

	code, stdout, _ := w.run("status", filepath.Join(w.root, "not", "there"))
	if code != cli.ExitOK {
		t.Errorf("exit = %d, want %d", code, cli.ExitOK)
	}
	if !strings.HasPrefix(stdout, "missing:") {
		t.Errorf("stdout = %q, want a missing: line", stdout)
	}
}

func TestRemoveDryRunLeavesEverythingInPlace(t *testing.T) {
	w := newWorld(t)
	w.cwd = w.addRepo("github.com", "halkn", "alpha")
	_, added, _ := w.run("add", "feat/x")
	path := strings.TrimSpace(added)

	code, _, _ := w.run("rm", "feat/x", "--dry-run")
	if code != cli.ExitOK {
		t.Fatalf("exit = %d", code)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("dry run removed the worktree: %v", err)
	}
}

func TestRemoveDeletesAMergedWorktree(t *testing.T) {
	w := newWorld(t)
	w.cwd = w.addRepo("github.com", "halkn", "alpha")
	_, added, _ := w.run("add", "feat/x")
	path := strings.TrimSpace(added)

	code, _, stderr := w.run("rm", "feat/x")
	if code != cli.ExitOK {
		t.Fatalf("exit = %d, stderr = %s", code, stderr)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("worktree survived: %v", err)
	}
}

// A rehearsal asks nothing, so it can only rehearse the removals that need no
// answer. One that would need an answer is reported as kept, with the status
// the real unattended run would end on rather than an error.
func TestRemoveDryRunKeepsWhatWouldNeedAsking(t *testing.T) {
	w := newWorld(t)
	w.cwd = w.addRepo("github.com", "halkn", "alpha")
	_, added, _ := w.run("add", "feat/x")
	path := strings.TrimSpace(added)
	if err := os.WriteFile(filepath.Join(path, "scratch.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, _, stderr := w.run("rm", "feat/x", "--dry-run")
	if code != cli.ExitCancelled {
		t.Fatalf("exit = %d, want %d; stderr = %s", code, cli.ExitCancelled, stderr)
	}
	if strings.Contains(stderr, "would remove") {
		t.Errorf("dry run announced a removal that needs an answer first:\n%s", stderr)
	}
	if !strings.Contains(stderr, "uncommitted") {
		t.Errorf("stderr %q does not say why the checkout was kept", stderr)
	}
}

// The point of --no-confirm: a caller that cannot answer a question keeps the
// guards instead of reaching for --force. Nothing was removed and nothing was
// chosen, which is the same outcome as declining every removal.
func TestRemoveNoConfirmKeepsWhatWouldNeedAsking(t *testing.T) {
	w := newWorld(t)
	w.cwd = w.addRepo("github.com", "halkn", "alpha")
	_, added, _ := w.run("add", "feat/x")
	path := strings.TrimSpace(added)
	if err := os.WriteFile(filepath.Join(path, "scratch.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := w.run("rm", "feat/x", "--no-confirm")
	if code != cli.ExitCancelled {
		t.Fatalf("exit = %d, want %d; stderr = %s", code, cli.ExitCancelled, stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "uncommitted") || !strings.Contains(stderr, path) {
		t.Errorf("stderr %q does not say which checkout was kept and why", stderr)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("--no-confirm removed a worktree with uncommitted changes: %v", err)
	}
}

func TestRemoveNoConfirmStillRemovesWhatNeedsNoAsking(t *testing.T) {
	w := newWorld(t)
	w.cwd = w.addRepo("github.com", "halkn", "alpha")
	_, added, _ := w.run("add", "feat/x")
	path := strings.TrimSpace(added)

	code, _, stderr := w.run("rm", "feat/x", "--no-confirm")
	if code != cli.ExitOK {
		t.Fatalf("exit = %d, want %d; stderr = %s", code, cli.ExitOK, stderr)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("worktree survived: %v", err)
	}
}

// One flag removes without asking, the other keeps whatever would need asking.
// Choosing either for the caller would be a guess about work they could lose.
func TestRemoveRejectsForceWithNoConfirm(t *testing.T) {
	w := newWorld(t)
	w.cwd = w.addRepo("github.com", "halkn", "alpha")
	_, added, _ := w.run("add", "feat/x")
	path := strings.TrimSpace(added)

	code, _, stderr := w.run("rm", "feat/x", "--force", "--no-confirm")
	if code != cli.ExitError {
		t.Fatalf("exit = %d, want %d", code, cli.ExitError)
	}
	if !strings.Contains(stderr, "--force") || !strings.Contains(stderr, "--no-confirm") {
		t.Errorf("stderr %q does not name both options", stderr)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("the rejected run removed the worktree anyway: %v", err)
	}
}

// The main checkout is never a removal target, so it must not even be offered.
func TestRemoveNeverTargetsTheMainCheckout(t *testing.T) {
	w := newWorld(t)
	dir := w.addRepo("github.com", "halkn", "alpha")
	w.cwd = dir

	code, _, _ := w.run("rm", "alpha")
	if code == cli.ExitOK {
		t.Error("rm reported success with only the main checkout matching")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("main checkout is gone: %v", err)
	}
}

// Without a picker, an ambiguous query cannot be resolved. Reporting that as a
// cancellation would tell a wrapper the user made a choice, and as no-match
// that nothing exists, when in fact several things do.
func TestPathWithSeveralMatchesAndNoPickerListsThemAndFails(t *testing.T) {
	if picker.Available() {
		// A PATH with git on it but no fzf, which is what a machine that has
		// not installed fzf looks like.
		gitPath, err := exec.LookPath("git")
		if err != nil {
			t.Skip("git is not on PATH")
		}
		bin := t.TempDir()
		if err := os.Symlink(gitPath, filepath.Join(bin, "git")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		t.Setenv("PATH", bin)
	}
	w := newWorld(t)
	w.addRepo("github.com", "halkn", "alpha")
	w.addRepo("github.com", "halkn", "alpine")

	code, stdout, stderr := w.run("path", "alp")
	if code != cli.ExitError {
		t.Errorf("exit = %d, want %d", code, cli.ExitError)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	for _, want := range []string{"alpha", "alpine", "fzf"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr does not mention %q:\n%s", want, stderr)
		}
	}
}

func TestUnknownCommandIsAUsageError(t *testing.T) {
	w := newWorld(t)

	code, stdout, stderr := w.run("frobnicate")
	if code != cli.ExitError {
		t.Errorf("exit = %d, want %d", code, cli.ExitError)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "frobnicate") {
		t.Errorf("stderr %q does not name the command", stderr)
	}
}

func TestHelpGoesToStdout(t *testing.T) {
	w := newWorld(t)

	code, stdout, _ := w.run("--help")
	if code != cli.ExitOK {
		t.Errorf("exit = %d, want %d", code, cli.ExitOK)
	}
	if !strings.Contains(stdout, "trepo") {
		t.Errorf("help output = %q", stdout)
	}
}

func countLines(s string) int {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return 0
	}
	return len(strings.Split(s, "\n"))
}

func resolved(t *testing.T, p string) string {
	t.Helper()
	real, err := filepath.EvalSymlinks(p)
	if err != nil {
		return p
	}
	return real
}

// A typo in an option name must not be read as a request. The same parser feeds
// rm, where "--dryrun" silently becoming nothing turns a rehearsal into a
// deletion.
func TestUnknownOptionIsRejected(t *testing.T) {
	w := newWorld(t)
	w.cwd = w.addRepo("github.com", "halkn", "alpha")
	_, added, _ := w.run("add", "feat/x")
	path := strings.TrimSpace(added)

	code, _, stderr := w.run("rm", "feat/x", "--dryrun")
	if code != cli.ExitError {
		t.Errorf("exit = %d, want %d", code, cli.ExitError)
	}
	if !strings.Contains(stderr, "dryrun") {
		t.Errorf("stderr %q does not name the unknown option", stderr)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("a mistyped option removed the worktree: %v", err)
	}
}

// A switch carries no value, so a value attached to one was meant as something
// else. Reading it as truth would make --dry-run=0 delete.
func TestValueOnASwitchIsRejected(t *testing.T) {
	w := newWorld(t)
	w.cwd = w.addRepo("github.com", "halkn", "alpha")
	_, added, _ := w.run("add", "feat/x")
	path := strings.TrimSpace(added)

	code, _, stderr := w.run("rm", "feat/x", "--dry-run=1")
	if code != cli.ExitError {
		t.Errorf("exit = %d, want %d", code, cli.ExitError)
	}
	if !strings.Contains(stderr, "dry-run") {
		t.Errorf("stderr %q does not name the option", stderr)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("--dry-run=1 removed the worktree: %v", err)
	}
}

// An unknown short option must be refused rather than searched for: reading
// "-f" as a query word makes rm quietly match nothing instead of forcing.
func TestUnknownShortOptionIsRejected(t *testing.T) {
	w := newWorld(t)
	w.addRepo("github.com", "halkn", "alpha")

	code, stdout, stderr := w.run("path", "-f", "alpha")
	if code != cli.ExitError {
		t.Errorf("exit = %d, want %d", code, cli.ExitError)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "-f") {
		t.Errorf("stderr %q does not name the option", stderr)
	}
}

// A query term that starts with a dash is still reachable, so the guard above
// does not cost the ability to search for one.
func TestQueryAfterDoubleDashIsNotAnOption(t *testing.T) {
	w := newWorld(t)
	w.addRepo("github.com", "halkn", "alpha")

	code, _, stderr := w.run("path", "--", "-f")
	if code != cli.ExitNoMatch {
		t.Errorf("exit = %d, want %d (stderr = %s)", code, cli.ExitNoMatch, stderr)
	}
}

// Every command answers --help itself. Failing on it would be an unfortunate
// response from rm in particular.
func TestHelpIsAcceptedPerCommand(t *testing.T) {
	w := newWorld(t)

	for _, args := range [][]string{{"rm", "--help"}, {"list", "-h"}, {"status", "--help"}} {
		code, stdout, _ := w.run(args...)
		if code != cli.ExitOK {
			t.Errorf("%v: exit = %d, want %d", args, code, cli.ExitOK)
		}
		if !strings.Contains(stdout, "usage:") {
			t.Errorf("%v: stdout = %q, want the usage text", args, stdout)
		}
	}
}

// The version is asked for when something is wrong, so it must answer without
// reading git config: an unreadable config is one of the things being reported.
func TestVersionIsAnsweredWithoutConfig(t *testing.T) {
	w := newWorld(t)

	for _, args := range [][]string{{"--version"}, {"version"}} {
		code, stdout, stderr := w.run(args...)
		if code != cli.ExitOK {
			t.Errorf("%v: exit = %d, want %d (stderr = %s)", args, code, cli.ExitOK, stderr)
		}
		if countLines(stdout) != 1 || !strings.HasPrefix(stdout, "trepo ") {
			t.Errorf("%v: stdout = %q, want one line naming the build", args, stdout)
		}
	}
}

// Outside a repository --here has nothing to narrow to. Printing nothing would
// be indistinguishable from a repository with no other checkouts.
func TestListHereOutsideARepositoryIsAnError(t *testing.T) {
	w := newWorld(t)
	w.addRepo("github.com", "halkn", "alpha")

	code, stdout, stderr := w.run("list", "--here")
	if code != cli.ExitError {
		t.Errorf("exit = %d, want %d", code, cli.ExitError)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "--here") {
		t.Errorf("stderr %q does not explain the refusal", stderr)
	}
}

// worktreeWith creates a worktree and puts it into the state the name suggests.
func (w *world) worktreeWith(branch string, prepare func(path string)) string {
	w.t.Helper()
	_, added, _ := w.run("add", branch)
	path := strings.TrimSpace(added)
	if path == "" {
		w.t.Fatalf("add %s produced no path", branch)
	}
	if prepare != nil {
		prepare(path)
	}
	return path
}

func TestRemoveReclaimableTakesOnlyTheFinishedWorktrees(t *testing.T) {
	w := newWorld(t)
	w.cwd = w.addRepo("github.com", "halkn", "alpha")

	done := w.worktreeWith("feat/done", nil)
	dirty := w.worktreeWith("feat/dirty", func(path string) {
		if err := os.WriteFile(filepath.Join(path, "scratch.txt"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	unmerged := w.worktreeWith("feat/unmerged", func(path string) {
		if _, err := w.fixture.TryGitIn(path, "commit", "--allow-empty", "-m", "add: work"); err != nil {
			t.Fatal(err)
		}
	})

	code, stdout, stderr := w.run("rm", "--reclaimable")
	if code != cli.ExitOK {
		t.Fatalf("exit = %d, stderr = %s", code, stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if _, err := os.Stat(done); !os.IsNotExist(err) {
		t.Errorf("a merged, clean worktree survived reclamation: %v", err)
	}
	for _, kept := range []string{dirty, unmerged} {
		if _, err := os.Stat(kept); err != nil {
			t.Errorf("%s was reclaimed although it still holds work: %v", kept, err)
		}
	}
}

// Nothing to reclaim is nothing found, which is what every other command that
// answers with a checkout already reports.
func TestRemoveReclaimableWithNothingToTakeExitsNoMatch(t *testing.T) {
	w := newWorld(t)
	w.cwd = w.addRepo("github.com", "halkn", "alpha")
	kept := w.worktreeWith("feat/dirty", func(path string) {
		if err := os.WriteFile(filepath.Join(path, "scratch.txt"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	})

	code, _, _ := w.run("rm", "--reclaimable")
	if code != cli.ExitNoMatch {
		t.Errorf("exit = %d, want %d", code, cli.ExitNoMatch)
	}
	if _, err := os.Stat(kept); err != nil {
		t.Errorf("the worktree was removed anyway: %v", err)
	}
}

func TestRemoveReclaimableDryRunReportsWithoutRemoving(t *testing.T) {
	w := newWorld(t)
	w.cwd = w.addRepo("github.com", "halkn", "alpha")
	done := w.worktreeWith("feat/done", nil)

	code, _, stderr := w.run("rm", "--reclaimable", "--dry-run")
	if code != cli.ExitOK {
		t.Fatalf("exit = %d, stderr = %s", code, stderr)
	}
	if !strings.Contains(stderr, "would remove") || !strings.Contains(stderr, done) {
		t.Errorf("stderr %q does not name what would be reclaimed", stderr)
	}
	if _, err := os.Stat(done); err != nil {
		t.Errorf("the rehearsal removed the worktree: %v", err)
	}
}

// The selection already excludes everything --force would push past, so the
// option can only mislead about how much is being taken.
func TestRemoveReclaimableRejectsForce(t *testing.T) {
	w := newWorld(t)
	w.cwd = w.addRepo("github.com", "halkn", "alpha")
	done := w.worktreeWith("feat/done", nil)

	code, _, stderr := w.run("rm", "--reclaimable", "--force")
	if code != cli.ExitError {
		t.Fatalf("exit = %d, want %d", code, cli.ExitError)
	}
	if !strings.Contains(stderr, "--reclaimable") || !strings.Contains(stderr, "--force") {
		t.Errorf("stderr %q does not name both options", stderr)
	}
	if _, err := os.Stat(done); err != nil {
		t.Errorf("the rejected run removed the worktree anyway: %v", err)
	}
}

// The query keeps its meaning: reclaiming is what may be taken, not what must.
func TestRemoveReclaimableNarrowsWithTheQuery(t *testing.T) {
	w := newWorld(t)
	w.cwd = w.addRepo("github.com", "halkn", "alpha")
	target := w.worktreeWith("feat/one", nil)
	other := w.worktreeWith("feat/two", nil)

	code, _, stderr := w.run("rm", "--reclaimable", "one")
	if code != cli.ExitOK {
		t.Fatalf("exit = %d, stderr = %s", code, stderr)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("the queried worktree survived: %v", err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("a worktree outside the query was reclaimed: %v", err)
	}
}

// Narrowing to the repository at hand is the common case for both commands, and
// the query that would express it otherwise is the repository slug the caller
// would have to go and find first.
func TestPathHereNarrowsToTheRepositoryYouAreIn(t *testing.T) {
	w := newWorld(t)
	alpha := w.addRepo("github.com", "halkn", "alpha")
	w.addRepo("github.com", "halkn", "beta")
	w.cwd = alpha

	code, stdout, stderr := w.run("path", "--here")
	if code != cli.ExitOK {
		t.Fatalf("exit = %d, stderr = %s", code, stderr)
	}
	if got := strings.TrimSpace(stdout); !checkout.SamePath(got, alpha) {
		t.Errorf("path = %q, want %q", got, alpha)
	}
}

// The repository is what --here narrows to, not the checkout: standing in a
// worktree still means the repository that worktree belongs to.
func TestPathHereFromInsideAWorktreeMeansItsRepository(t *testing.T) {
	w := newWorld(t)
	alpha := w.addRepo("github.com", "halkn", "alpha")
	w.addRepo("github.com", "halkn", "beta")
	w.cwd = alpha
	_, added, _ := w.run("add", "feat/x")
	w.cwd = strings.TrimSpace(added)

	code, stdout, stderr := w.run("path", "--here", "--repos")
	if code != cli.ExitOK {
		t.Fatalf("exit = %d, stderr = %s", code, stderr)
	}
	if got := strings.TrimSpace(stdout); !checkout.SamePath(got, alpha) {
		t.Errorf("path = %q, want %q", got, alpha)
	}
}

func TestRemoveHereNarrowsToTheRepositoryYouAreIn(t *testing.T) {
	w := newWorld(t)
	alpha := w.addRepo("github.com", "halkn", "alpha")
	beta := w.addRepo("github.com", "halkn", "beta")

	w.cwd = beta
	_, added, _ := w.run("add", "feat/x")
	kept := strings.TrimSpace(added)
	w.cwd = alpha
	_, added, _ = w.run("add", "feat/x")
	target := strings.TrimSpace(added)

	code, _, stderr := w.run("rm", "feat/x", "--here")
	if code != cli.ExitOK {
		t.Fatalf("exit = %d, stderr = %s", code, stderr)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("the worktree of the repository at hand survived: %v", err)
	}
	if _, err := os.Stat(kept); err != nil {
		t.Errorf("--here reached into another repository: %v", err)
	}
}

// Outside a repository the option cannot mean anything, and both commands have
// to say so the same way list does rather than answering with nothing.
func TestHereOutsideARepositoryIsTheSameErrorEverywhere(t *testing.T) {
	w := newWorld(t)
	w.addRepo("github.com", "halkn", "alpha")

	for _, args := range [][]string{{"list", "--here"}, {"path", "--here"}, {"rm", "--here"}} {
		code, stdout, stderr := w.run(args...)
		if code != cli.ExitError {
			t.Errorf("%v: exit = %d, want %d", args, code, cli.ExitError)
		}
		if stdout != "" {
			t.Errorf("%v: stdout = %q, want empty", args, stdout)
		}
		if !strings.Contains(stderr, "--here") {
			t.Errorf("%v: stderr %q does not explain the refusal", args, stderr)
		}
	}
}

// What an interrupted clone leaves behind must not be handed out as a checkout,
// or `cd -- "$(trepo get ...)"` lands in it forever.
func TestGetRefusesADirectoryThatIsNotARepository(t *testing.T) {
	w := newWorld(t)
	dest := filepath.Join(w.root, "github.com", "halkn", "alpha")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := w.run("get", "halkn/alpha")
	if code != cli.ExitError {
		t.Errorf("exit = %d, want %d", code, cli.ExitError)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "not a repository") {
		t.Errorf("stderr %q does not say why", stderr)
	}
}

// add is idempotent, so an existing branch is checked out as it is. Saying so
// is what keeps the user from believing it starts at the base they named.
func TestAddSaysWhenFromWasNotUsed(t *testing.T) {
	w := newWorld(t)
	w.cwd = w.addRepo("github.com", "halkn", "alpha")
	if _, err := w.fixture.TryGitIn(w.cwd, "branch", "feat/x"); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := w.run("add", "feat/x", "--from", "main")
	if code != cli.ExitOK {
		t.Fatalf("exit = %d, stderr = %s", code, stderr)
	}
	if countLines(stdout) != 1 {
		t.Errorf("stdout has %d lines, want exactly the path:\n%s", countLines(stdout), stdout)
	}
	if !strings.Contains(stderr, "from main") {
		t.Errorf("stderr %q does not report that the base was unused", stderr)
	}

	// The same holds once the worktree itself is there, which is the answer a
	// repeated add returns.
	_, _, stderr = w.run("add", "feat/x", "--from", "main")
	if !strings.Contains(stderr, "from main") {
		t.Errorf("stderr %q does not report that the base was unused", stderr)
	}
}

// A rehearsal must describe what would actually happen, so it cannot announce
// removals the guards go on to refuse.
func TestDryRunDoesNotAnnounceARefusedRemoval(t *testing.T) {
	w := newWorld(t)
	w.cwd = w.addRepo("github.com", "halkn", "alpha")
	_, added, _ := w.run("add", "feat/x")
	w.cwd = strings.TrimSpace(added)

	code, _, stderr := w.run("rm", "feat/x", "--dry-run")
	if strings.Contains(stderr, "would remove") {
		t.Errorf("dry run announced a removal it then refused:\n%s", stderr)
	}
	if code == cli.ExitOK {
		t.Error("dry run reported success for a refused removal")
	}
}

// A repository that could not be read is not a repository that holds nothing.
// Reporting "no match" for it would tell a wrapper the checkout does not exist.
func TestPathDoesNotReportNoMatchWhenARepositoryCouldNotBeRead(t *testing.T) {
	w := newWorld(t)
	broken := filepath.Join(w.root, "github.com", "halkn", "broken")
	if err := os.MkdirAll(filepath.Join(broken, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := w.run("path", "broken")
	if code == cli.ExitNoMatch {
		t.Errorf("exit = %d, want it distinguished from a genuine no-match", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "broken") {
		t.Errorf("stderr %q does not name the repository that failed", stderr)
	}
}
