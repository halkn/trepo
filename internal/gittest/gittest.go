// Package gittest builds throwaway git repositories for tests.
//
// Every git invocation runs with the developer's own configuration switched
// off. Without that, fixtures inherit init.defaultBranch, commit.gpgsign,
// hooks and safe.* policies from whoever runs the suite, and the same test
// passes on one machine and fails on another.
package gittest

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Repo is a git repository living in a directory that the test framework
// removes on cleanup.
type Repo struct {
	t   *testing.T
	Dir string
	env []string
}

// New creates a repository on branch main holding a single commit.
func New(t *testing.T) *Repo {
	t.Helper()
	r := newRepo(t)
	r.Git("init", "--initial-branch=main")
	r.Write("README.md", "fixture\n")
	r.Commit("add: initial commit")
	return r
}

// NewEmpty creates a repository that has no commit yet, which is the state
// between `git init` and the first commit.
func NewEmpty(t *testing.T) *Repo {
	t.Helper()
	r := newRepo(t)
	r.Git("init", "--initial-branch=main")
	return r
}

// NewBare creates a bare repository, which has no working tree of its own.
func NewBare(t *testing.T) *Repo {
	t.Helper()
	r := newRepo(t)
	r.Git("init", "--bare", "--initial-branch=main")
	return r
}

func newRepo(t *testing.T) *Repo {
	t.Helper()
	home := t.TempDir()
	dir := filepath.Join(home, "repo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return &Repo{t: t, Dir: dir, env: isolatedEnv(home)}
}

// isolatedEnv points every configuration git might read at the throwaway home.
// GIT_CONFIG_SYSTEM and GIT_CONFIG_GLOBAL are what actually disable the user's
// files; HOME is set as well because git falls back to it for other lookups
// such as the credential store.
func isolatedEnv(home string) []string {
	env := append(os.Environ(),
		"HOME="+home,
		"XDG_CONFIG_HOME="+filepath.Join(home, "config"),
		"GIT_CONFIG_GLOBAL="+filepath.Join(home, "gitconfig"),
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=trepo test",
		"GIT_AUTHOR_EMAIL=test@example.invalid",
		"GIT_COMMITTER_NAME=trepo test",
		"GIT_COMMITTER_EMAIL=test@example.invalid",
		"GIT_CONFIG_COUNT=2",
		"GIT_CONFIG_KEY_0=init.defaultBranch",
		"GIT_CONFIG_VALUE_0=main",
		"GIT_CONFIG_KEY_1=commit.gpgsign",
		"GIT_CONFIG_VALUE_1=false",
	)
	return env
}

// Env returns the isolated environment, for code under test that shells out to
// git itself.
func (r *Repo) Env() []string {
	return append([]string(nil), r.env...)
}

// Git runs a git command in the repository and returns its trimmed stdout,
// failing the test if the command does.
func (r *Repo) Git(args ...string) string {
	r.t.Helper()
	out, err := r.TryGit(args...)
	if err != nil {
		r.t.Fatal(err)
	}
	return out
}

// TryGit runs a git command and returns its trimmed stdout, or an error that
// carries stderr so a failing fixture explains itself.
func (r *Repo) TryGit(args ...string) (string, error) {
	r.t.Helper()
	return r.TryGitIn(r.Dir, args...)
}

// TryGitIn is TryGit with an explicit working directory, for commands that must
// run from outside the repository.
func (r *Repo) TryGitIn(dir string, args ...string) (string, error) {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = r.env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s",
			strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// Write creates or replaces a file in the working tree, creating parents.
func (r *Repo) Write(rel, content string) {
	r.t.Helper()
	path := filepath.Join(r.Dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

// Commit stages everything in the working tree and commits it.
func (r *Repo) Commit(message string) {
	r.t.Helper()
	r.Git("add", "-A")
	r.Git("commit", "-m", message)
}
