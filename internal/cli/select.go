package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/halkn/trepo/internal/checkout"
	"github.com/halkn/trepo/internal/git"
	"github.com/halkn/trepo/internal/repo"
)

// errNoMatch means the query matched nothing.
var errNoMatch = errors.New("no matching checkout")

// errAmbiguous means the query matched more than one candidate where one was
// needed. It is neither an empty result nor a failure, and a caller that wants
// to choose between them has trepo list to build a picker over.
var errAmbiguous = errors.New("ambiguous query")

// ambiguous names the candidates that were found. It reads as errAmbiguous
// while printing only its own message, so the caller can both recognise the
// case and tell the user what would narrow it.
type ambiguous struct{ msg string }

func (a ambiguous) Error() string { return a.msg }
func (a ambiguous) Unwrap() error { return errAmbiguous }

// checkouts lists every checkout trepo manages.
//
// A repository that cannot be read is reported and left out, and the fact that
// something was left out is remembered: "no matching checkout" would otherwise
// be indistinguishable from "the repository holding it could not be read".
func (a *app) checkouts() ([]checkout.Checkout, error) {
	repos, err := repo.Discover(a.cfg.Root)
	if err != nil {
		return nil, err
	}
	f := checkout.Finder{Git: a.opts.Git, Cwd: a.opts.Cwd}
	cs, errs := f.All(repos)
	for _, e := range errs {
		fmt.Fprintln(a.stderr, "trepo: "+strings.Join(strings.Fields(e.Error()), " "))
	}
	a.incomplete = len(errs) > 0
	return cs, nil
}

// filter narrows checkouts by substring, matching every term against the
// repository, the branch and the path together.
func filter(cs []checkout.Checkout, query []string) []checkout.Checkout {
	if len(query) == 0 {
		return cs
	}
	var out []checkout.Checkout
	for _, c := range cs {
		haystack := strings.ToLower(c.Repo.Slug() + " " + c.Branch + " " + c.Path)
		matched := true
		for _, term := range query {
			if !strings.Contains(haystack, strings.ToLower(term)) {
				matched = false
				break
			}
		}
		if matched {
			out = append(out, c)
		}
	}
	return out
}

// here narrows checkouts to the repository the working directory belongs to,
// when --here was given.
//
// What it narrows to is the repository, not the checkout, so standing in a
// worktree still means every checkout of the repository that worktree is part
// of. The lookup happens once rather than per checkout, and failing to find a
// repository is reported rather than treated as "nothing is here": outside a
// repository the filter has no meaning, and an empty answer would read as a
// repository with no other checkouts.
func (a *app) here(cs []checkout.Checkout, flags map[string]string) ([]checkout.Checkout, int) {
	if flags["here"] != "true" {
		return cs, ExitOK
	}
	root, err := git.RepoRoot(a.opts.Git, a.opts.Cwd)
	if err != nil {
		return nil, fail(a.stderr,
			errors.New("not inside a repository, so --here has nothing to narrow to"))
	}

	var kept []checkout.Checkout
	for _, c := range cs {
		if checkout.SamePath(root, c.Repo.Root) {
			kept = append(kept, c)
		}
	}
	return kept, ExitOK
}

// standIn points the run at a directory other than the process's own.
//
// A caller that draws checkouts does not always run from the one it is asking
// about: a terminal multiplexer reports the working directory of a pane, which
// is any directory below a checkout. Moving the mark rather than adding a way
// to match paths keeps one answer to "which checkout holds this" — the current
// flag the listing already computes.
//
// The directory has to exist. One that does not marks no checkout, which would
// read as "you are nowhere" instead of as the mistake it is. Only commands that
// read take this: the same mark is what stops rm removing the checkout the user
// is standing in, so a caller able to move it could delete that checkout.
func (a *app) standIn(flags map[string]string) int {
	dir, ok := flags["cwd"]
	if !ok {
		return ExitOK
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return fail(a.stderr, fmt.Errorf("%s is not a directory to stand in", dir))
	}
	a.opts.Cwd = dir
	return ExitOK
}

// current narrows to the checkout the run is standing in, when --current was
// given. Nesting settles which one that is, so this is a resolution rather than
// a choice between candidates.
func current(cs []checkout.Checkout, flags map[string]string, cwd string) []checkout.Checkout {
	if flags["current"] != "true" {
		return cs
	}
	c, ok := checkout.Containing(cs, cwd)
	if !ok {
		return nil
	}
	return []checkout.Checkout{c}
}

// only takes the single candidate a query identified.
//
// Several matches is an answer rather than a prompt: the query did not name one
// checkout, and picking the first would hand back a path the caller never
// asked for. The candidates are not repeated on stderr either, because the same
// query through trepo list is what produces them in a form worth reading.
func only(cs []checkout.Checkout, noun string, query []string) (checkout.Checkout, error) {
	switch len(cs) {
	case 0:
		return checkout.Checkout{}, errNoMatch
	case 1:
		return cs[0], nil
	default:
		return checkout.Checkout{}, ambiguous{fmt.Sprintf(
			"%d %s match %s; narrow the query or choose one from trepo list",
			len(cs), noun, describe(query))}
	}
}

// describe names the query in an error, without a newline or a bracket for the
// tools that embed the line in their own interface.
func describe(query []string) string {
	if len(query) == 0 {
		return "an empty query"
	}
	return strings.Join(query, " ")
}

// row is the human-readable form of a checkout: repository, branch, flags.
// Flags get a column of their own so a filter matches them as a field; a
// branch called fix/main-nav must not read as the main marker.
func row(c checkout.Checkout) []string {
	branch := c.Branch
	if branch == "" {
		branch = "-"
	}
	flags := "-"
	if len(c.Flags) > 0 {
		var names []string
		for _, f := range c.Flags {
			names = append(names, string(f))
		}
		flags = strings.Join(names, ",")
	}
	return []string{pad(c.Repo.Slug(), 28), pad(branch, 28), flags}
}

func pad(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
