package config_test

import (
	"path/filepath"
	"testing"

	"github.com/halkn/trepo/internal/config"
	"github.com/halkn/trepo/internal/git"
	"github.com/halkn/trepo/internal/gittest"
)

func TestLoadDefaults(t *testing.T) {
	fixture := gittest.New(t)
	r := git.Exec{Env: fixture.Env()}

	cfg, err := config.Load(r)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultHost != "github.com" {
		t.Errorf("DefaultHost = %q, want github.com", cfg.DefaultHost)
	}
	if !filepath.IsAbs(cfg.Root) {
		t.Errorf("Root = %q, want an absolute path", cfg.Root)
	}
	if !filepath.IsAbs(cfg.WorktreeRoot) {
		t.Errorf("WorktreeRoot = %q, want an absolute path", cfg.WorktreeRoot)
	}
}

func TestLoadReadsGlobalKeys(t *testing.T) {
	fixture := gittest.New(t)
	fixture.Git("config", "--global", "trepo.root", "/tmp/roots")
	fixture.Git("config", "--global", "trepo.worktreeRoot", "/tmp/wt")
	fixture.Git("config", "--global", "trepo.defaultHost", "gitlab.com")
	r := git.Exec{Env: fixture.Env()}

	cfg, err := config.Load(r)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Root != "/tmp/roots" {
		t.Errorf("Root = %q, want /tmp/roots", cfg.Root)
	}
	if cfg.WorktreeRoot != "/tmp/wt" {
		t.Errorf("WorktreeRoot = %q, want /tmp/wt", cfg.WorktreeRoot)
	}
	if cfg.DefaultHost != "gitlab.com" {
		t.Errorf("DefaultHost = %q, want gitlab.com", cfg.DefaultHost)
	}
}

// The set of checkouts trepo manages must not depend on where the command was
// run from, so a repository-local override of a universe key is ignored.
func TestLoadIgnoresRepositoryLocalUniverseKeys(t *testing.T) {
	fixture := gittest.New(t)
	fixture.Git("config", "--global", "trepo.root", "/tmp/global")
	fixture.Git("config", "--local", "trepo.root", "/tmp/local")
	r := git.Exec{Env: fixture.Env()}

	cfg, err := config.Load(r)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Root != "/tmp/global" {
		t.Errorf("Root = %q, want the global value /tmp/global", cfg.Root)
	}
}

func TestLoadExpandsTilde(t *testing.T) {
	fixture := gittest.New(t)
	fixture.Git("config", "--global", "trepo.root", "~/somewhere")
	r := git.Exec{Env: fixture.Env()}

	cfg, err := config.Load(r)
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(cfg.Root) {
		t.Errorf("Root = %q, want the tilde expanded", cfg.Root)
	}
}

func TestRepoConfigDefaults(t *testing.T) {
	fixture := gittest.New(t)
	r := git.Exec{Env: fixture.Env()}

	rc := config.LoadRepo(r, fixture.Dir)
	if rc.WorktreeTemplate != "{{.Owner}}/{{.Repo}}/{{.Branch}}" {
		t.Errorf("WorktreeTemplate = %q", rc.WorktreeTemplate)
	}
	if len(rc.Protected) != 0 {
		t.Errorf("Protected = %v, want empty", rc.Protected)
	}
}

// Protected is the safety mechanism: reading it with `git config --get` would
// silently keep only the last value and quietly unprotect the rest.
func TestRepoConfigKeepsEveryProtectedValue(t *testing.T) {
	fixture := gittest.New(t)
	fixture.Git("config", "--global", "--add", "trepo.protected", ".claude/worktrees")
	fixture.Git("config", "--global", "--add", "trepo.protected", "vendor/checkouts")
	r := git.Exec{Env: fixture.Env()}

	rc := config.LoadRepo(r, fixture.Dir)
	if len(rc.Protected) != 2 {
		t.Fatalf("Protected = %v, want both values", rc.Protected)
	}
}

func TestRepoConfigHonoursLocalOverride(t *testing.T) {
	fixture := gittest.New(t)
	fixture.Git("config", "--global", "trepo.worktreeTemplate", "{{.Repo}}/{{.Branch}}")
	fixture.Git("config", "--local", "trepo.worktreeTemplate", "wt/{{.Branch}}")
	r := git.Exec{Env: fixture.Env()}

	rc := config.LoadRepo(r, fixture.Dir)
	if rc.WorktreeTemplate != "wt/{{.Branch}}" {
		t.Errorf("WorktreeTemplate = %q, want the local override", rc.WorktreeTemplate)
	}
}
