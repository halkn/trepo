// Package cli is trepo's command surface.
//
// Two rules shape everything here. Commands that answer with a location print
// that path and nothing else on stdout, so `cd -- "$(trepo path api)"` is
// always safe. And every exit status distinguishes "you chose nothing" from
// "there was nothing to choose", because a shell wrapper has to react to those
// differently.
package cli

import (
	"fmt"
	"io"
	"runtime/debug"
	"strings"

	"github.com/halkn/trepo/internal/config"
	"github.com/halkn/trepo/internal/git"
)

// Exit statuses. Cancellation borrows fzf's 130 so a wrapper can pass it
// through unchanged.
const (
	ExitOK        = 0
	ExitNoMatch   = 1
	ExitError     = 2
	ExitCancelled = 130
)

// Options are the surroundings a run happens in, injected so tests can put a
// run somewhere other than the developer's own machine.
type Options struct {
	Git git.Runner
	Cwd string
}

type app struct {
	opts   Options
	cfg    config.Config
	stdout io.Writer
	stderr io.Writer

	// incomplete records that some repository could not be read, so that an
	// empty result is not reported as a certainty.
	incomplete bool
}

const usage = `trepo - repositories and worktrees as one set of checkouts

usage:
  trepo get <repo>                     clone a repository into the trepo root
  trepo list [<query>...]              list checkouts
  trepo path [<query>...]              print the path of one checkout
  trepo add <branch>                   create a worktree and print its path
  trepo rm [<query>...]                remove worktrees
  trepo status <path>                  describe one checkout
  trepo version                        print which build this is

options:
  list   --json --repos --worktrees --here
  path   --repos --here
  add    --repo <query> --from <ref>
  rm     --force --dry-run --no-confirm --here

--here narrows to the repository the working directory belongs to, whichever
of its checkouts you are standing in.

--force removes what rm would otherwise ask about; --no-confirm keeps it and
says why, for callers that cannot answer a question.

For path, add and rm: finding nothing exits 1, and cancelling exits 130 —
dismissing the picker, or every removal being declined or kept.`

// Run executes one command and returns the process exit status.
func Run(args []string, stdout, stderr io.Writer, opts Options) int {
	if opts.Git == nil {
		opts.Git = git.Exec{}
	}

	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return ExitError
	}

	name, rest := args[0], args[1:]
	switch name {
	case "-h", "--help", "help":
		fmt.Fprintln(stdout, usage)
		return ExitOK
	case "--version", "version":
		// Answered before the configuration is read: an unreadable git config
		// is one of the things a version is asked for in order to report.
		info, _ := debug.ReadBuildInfo()
		fmt.Fprintln(stdout, versionLine(info))
		return ExitOK
	}

	cfg, err := config.Load(opts.Git)
	if err != nil {
		return fail(stderr, err)
	}
	a := &app{opts: opts, cfg: cfg, stdout: stdout, stderr: stderr}

	switch name {
	case "get":
		return a.get(rest)
	case "list":
		return a.list(rest)
	case "path":
		return a.path(rest)
	case "add":
		return a.add(rest)
	case "rm":
		return a.remove(rest)
	case "status":
		return a.status(rest)
	default:
		fmt.Fprintf(stderr, "trepo: unknown command %q\n\n%s\n", name, usage)
		return ExitError
	}
}

// oneline flattens a message so other tools can embed it in their own
// interfaces — an fzf header among them — where a newline or a bracket would
// break the surrounding syntax.
func oneline(err error) string {
	msg := strings.Join(strings.Fields(err.Error()), " ")
	return strings.NewReplacer("(", "[", ")", "]").Replace(msg)
}

// fail reports on stderr and gives the caller the error status.
func fail(stderr io.Writer, err error) int {
	fmt.Fprintln(stderr, "trepo: "+oneline(err))
	return ExitError
}
