// Package cli is trepo's command surface.
//
// Three rules shape everything here. No command asks a question: stdin belongs
// to whatever invoked trepo, so a command that cannot settle on one action
// reports what it needs and exits. Commands that answer with a location print
// that path and nothing else on stdout, so `p=$(trepo path api)` is always
// safe. And every exit status keeps "nothing matched", "several things did"
// and "an error" apart, because a shell wrapper reacts to them differently.
package cli

import (
	"fmt"
	"io"
	"runtime/debug"
	"strings"

	"github.com/halkn/trepo/internal/config"
	"github.com/halkn/trepo/internal/git"
)

// Exit statuses.
//
// 130 is left to the caller: it is what fzf exits with when a picker is
// dismissed, and trepo never chooses on the user's behalf, so nothing here can
// produce it. Reserving it keeps a wrapper free to pass it through unchanged.
const (
	ExitOK      = 0
	ExitNoMatch = 1
	ExitError   = 2

	// ExitUndecided says trepo would not settle the run on its own: several
	// checkouts matched where one was needed, or every removal asked for needs
	// a decision the guards will not make. Both leave the caller with the same
	// next step - narrow it down, or say --force - which is why they share a
	// status and neither may collapse into "nothing matched" or "an error".
	ExitUndecided = 3
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
  trepo rm [<query>...]                remove worktrees, or reclaim finished ones
  trepo status <path>                  describe one checkout
  trepo remote [<query>...]            list repositories that could be cloned
  trepo version                        print which build this is

options:
  list   --json --repos --worktrees --here --cwd <dir>
  path   --repos --here --current --cwd <dir>
  add    --repo <query> --from <ref>
  rm     --force --dry-run --here --reclaimable
  remote --missing --refresh

--here narrows to the repository the working directory belongs to, whichever
of its checkouts you are standing in; --current narrows to that one checkout.
--cwd judges both by <dir> instead of by where trepo was run, for a caller
that asks about a directory it is not in. rm does not take it: the working
directory is what marks the checkout rm refuses to remove.

No command asks a question. rm keeps whatever would need one and says why;
--force removes it anyway. --reclaimable takes the worktrees whose work is
done — merged, retired on the remote, or already deleted by hand.

remote is the only command that reaches the network, and it does so through
gh. Its columns are host, repository, local or remote, and the path when the
repository is already here; hand one to "trepo get" to clone it. Owners are
read from trepo.remoteOwner, defaulting to the authenticated account. When it
fails, every other command still works.

Exit status: 0 success, 1 nothing matched, 2 an error, 3 nothing was decided —
several checkouts matched one query, or every removal was kept. Build a picker
over "trepo list" to choose between several; "trepo status <path>" describes
one row.`

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
	case "remote":
		return a.remoteCandidates(rest)
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
