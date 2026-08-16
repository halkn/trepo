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

	if v, ok := universe(r, "trepo.root"); ok {
		cfg.Root = expand(v, home)
	}
	if v, ok := universe(r, "trepo.worktreeRoot"); ok {
		cfg.WorktreeRoot = expand(v, home)
	}
	if v, ok := universe(r, "trepo.defaultHost"); ok {
		cfg.DefaultHost = v
	}
	return cfg, nil
}

// LoadRepo reads the settings a repository may override, with git's usual
// precedence. Failures fall back to the defaults: a repository that cannot be
// read is a problem for the caller that needs its checkouts, not for config.
func LoadRepo(r git.Runner, dir string) RepoConfig {
	rc := RepoConfig{WorktreeTemplate: DefaultWorktreeTemplate}

	if v, ok := scoped(r, dir, "trepo.worktreeTemplate"); ok {
		rc.WorktreeTemplate = v
	}
	if v, ok := scoped(r, dir, "trepo.previewCommand"); ok {
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
func universe(r git.Runner, key string) (string, bool) {
	for _, scope := range []string{"--global", "--system"} {
		if v, ok := configGet(r, "", scope, "--get", key); ok {
			return v, true
		}
	}
	return "", false
}

func scoped(r git.Runner, dir, key string) (string, bool) {
	return configGet(r, dir, "--get", key)
}

func scopedAll(r git.Runner, dir, key string) []string {
	out, err := git.Output(r, dir, append([]string{"config"}, "--get-all", key)...)
	if err != nil || out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

// configGet treats "the key is unset" as an answer rather than a failure: git
// reports it with exit status 1, which is indistinguishable from success for
// any caller that only checks err != nil.
func configGet(r git.Runner, dir string, args ...string) (string, bool) {
	out, err := git.Output(r, dir, append([]string{"config"}, args...)...)
	if err != nil {
		var gitErr *git.Error
		if errors.As(err, &gitErr) && gitErr.ExitCode == 1 {
			return "", false
		}
		return "", false
	}
	if out == "" {
		return "", false
	}
	return out, true
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
