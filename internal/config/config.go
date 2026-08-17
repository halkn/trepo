// Package config resolves trepo's settings from git config.
//
// git config is the store rather than a file of trepo's own because git
// already owns the precedence rules, the multi-value semantics and the
// tooling, and because trepo has no dependencies to parse another format with.
package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/halkn/trepo/internal/git"
)

// DefaultWorktreeTemplate places worktrees below the worktree root. The host is
// left out: it rarely tells two checkouts apart, and repository placement under
// the trepo root already carries it.
const DefaultWorktreeTemplate = "{{.Owner}}/{{.Repo}}/{{.Branch}}"

// Config holds the settings that decide which checkouts exist at all.
//
// These are read from the global and system scopes only. Were a repository
// allowed to override them, the same `trepo list` would answer differently
// depending on the directory it was run from, with nothing in the output to
// explain why.
type Config struct {
	Root         string
	WorktreeRoot string
	DefaultHost  string
}

// RepoConfig holds the settings a repository may reasonably decide for itself.
type RepoConfig struct {
	WorktreeTemplate string
	Protected        []string
	PreviewCommand   string
}

// Load reads the universe settings, falling back to defaults.
func Load(r git.Runner) (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Root:         filepath.Join(home, "repos"),
		WorktreeRoot: filepath.Join(dataHome(home), "trepo", "worktrees"),
		DefaultHost:  "github.com",
	}

	for _, key := range []struct {
		name string
		set  func(string)
	}{
		{"trepo.root", func(v string) { cfg.Root = expand(v, home) }},
		{"trepo.worktreeRoot", func(v string) { cfg.WorktreeRoot = expand(v, home) }},
		{"trepo.defaultHost", func(v string) { cfg.DefaultHost = v }},
	} {
		v, ok, err := universe(r, key.name)
		if err != nil {
			return Config{}, err
		}
		if ok {
			key.set(v)
		}
	}
	return cfg, nil
}

// LoadRepo reads the settings a repository may override, with git's usual
// precedence. Failures fall back to the defaults: a repository that cannot be
// read is a problem for the caller that needs its checkouts, not for config.
func LoadRepo(r git.Runner, dir string) RepoConfig {
	rc := RepoConfig{WorktreeTemplate: DefaultWorktreeTemplate}

	if v, ok, _ := scoped(r, dir, "trepo.worktreeTemplate"); ok {
		rc.WorktreeTemplate = v
	}
	if v, ok, _ := scoped(r, dir, "trepo.previewCommand"); ok {
		rc.PreviewCommand = v
	}
	rc.Protected = scopedAll(r, dir, "trepo.protected")
	return rc
}

// A worktree can hold uncommitted work, so its default home is the data
// directory rather than cache or state.
func dataHome(home string) string {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return v
	}
	return filepath.Join(home, ".local", "share")
}

// universe reads a key from the global scope, then the system scope. It never
// consults the repository, which is the whole point of the distinction.
func universe(r git.Runner, key string) (string, bool, error) {
	for _, scope := range []string{"--global", "--system"} {
		v, ok, err := configGet(r, "", scope, "--get", key)
		if err != nil {
			return "", false, err
		}
		if ok {
			return v, true, nil
		}
	}
	return "", false, nil
}

func scoped(r git.Runner, dir, key string) (string, bool, error) {
	return configGet(r, dir, "--get", key)
}

func scopedAll(r git.Runner, dir, key string) []string {
	out, err := git.Output(r, dir, append([]string{"config"}, "--get-all", key)...)
	if err != nil || out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

// configGet separates "the key is unset" from "the configuration could not be
// read". git reports the first with exit status 1 and reserves higher statuses
// for a missing binary or a file it cannot parse; collapsing the two would let
// a broken ~/.gitconfig quietly hand back trepo's defaults.
func configGet(r git.Runner, dir string, args ...string) (string, bool, error) {
	out, err := git.Output(r, dir, append([]string{"config"}, args...)...)
	if err != nil {
		var gitErr *git.Error
		if errors.As(err, &gitErr) && gitErr.ExitCode == 1 {
			return "", false, nil
		}
		return "", false, err
	}
	if out == "" {
		return "", false, nil
	}
	return out, true, nil
}

func expand(v, home string) string {
	switch {
	case v == "~":
		return home
	case strings.HasPrefix(v, "~/"):
		return filepath.Join(home, v[2:])
	}
	return os.ExpandEnv(v)
}
