package gittest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/halkn/trepo/internal/gittest"
)

// The helper must win over a global configuration that is already in effect for
// the test process: otherwise whatever the developer has in ~/.gitconfig leaks
// into every fixture and the suite passes or fails per machine.
func TestNewIgnoresAmbientGlobalConfig(t *testing.T) {
	leaked := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(leaked, []byte("[trepo]\n\troot = /leaked\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", leaked)

	repo := gittest.New(t)
	// `git config --get` exits 1 when the key is unset, which is the outcome
	// wanted here: the leaked value must not be visible.
	if got, _ := repo.TryGit("config", "--get", "trepo.root"); got != "" {
		t.Errorf("ambient global config leaked into the fixture: trepo.root = %q", got)
	}
}

func TestNewCreatesRepoOnMainWithOneCommit(t *testing.T) {
	repo := gittest.New(t)

	if got := repo.Git("rev-parse", "--abbrev-ref", "HEAD"); got != "main" {
		t.Errorf("branch = %q, want main", got)
	}
	if got := repo.Git("rev-list", "--count", "HEAD"); got != "1" {
		t.Errorf("commit count = %q, want 1", got)
	}
	if _, err := os.Stat(filepath.Join(repo.Dir, ".git")); err != nil {
		t.Errorf(".git missing: %v", err)
	}
}

func TestCommitRecordsWrittenFiles(t *testing.T) {
	repo := gittest.New(t)
	repo.Write("docs/readme.md", "hello")
	repo.Commit("add: readme")

	if got := repo.Git("status", "--porcelain"); got != "" {
		t.Errorf("worktree not clean after Commit: %q", got)
	}
	files := repo.Git("ls-tree", "-r", "--name-only", "HEAD")
	if !strings.Contains(files, "docs/readme.md") {
		t.Errorf("committed files = %q, want it to contain docs/readme.md", files)
	}
}

func TestTryGitReportsFailureWithStderr(t *testing.T) {
	repo := gittest.New(t)

	_, err := repo.TryGit("rev-parse", "--verify", "refs/heads/nope")
	if err == nil {
		t.Fatal("TryGit succeeded on a missing ref, want an error")
	}
}
