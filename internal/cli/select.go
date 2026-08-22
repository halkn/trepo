package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/halkn/trepo/internal/checkout"
	"github.com/halkn/trepo/internal/git"
	"github.com/halkn/trepo/internal/picker"
	"github.com/halkn/trepo/internal/repo"
)

// errNoMatch means the query matched nothing, which is different from the user
// declining to choose.
var errNoMatch = errors.New("no matching checkout")

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

// choose narrows a list to what the user meant.
//
// One candidate needs no interaction, and asking anyway would make the command
// unusable in a script. Without fzf the candidates go to stderr so the user
// can retype a narrower query, while stdout stays empty and the caller's
// command substitution yields nothing.
func (a *app) choose(cs []checkout.Checkout, multi bool, prompt string) ([]checkout.Checkout, error) {
	switch {
	case len(cs) == 0:
		return nil, errNoMatch
	case len(cs) == 1:
		return cs, nil
	case !picker.Available():
		fmt.Fprintf(a.stderr, "trepo: %d checkouts match; narrow the query or install fzf\n", len(cs))
		for _, c := range cs {
			fmt.Fprintln(a.stderr, "  "+strings.Join(row(c), "  "))
		}
		return nil, picker.ErrUnavailable
	}

	rows := make([]picker.Row, 0, len(cs))
	for _, c := range cs {
		rows = append(rows, picker.Row{Display: row(c), Key: c.Path})
	}

	header := "enter: select"
	if multi {
		header = "tab: mark / enter: confirm"
	}
	keys, err := picker.Picker{
		Prompt:  prompt,
		Header:  header,
		Multi:   multi,
		Preview: previewCommand(),
	}.Pick(rows)
	if err != nil {
		return nil, err
	}

	var chosen []checkout.Checkout
	for _, key := range keys {
		if c, ok := checkout.Locate(cs, key); ok {
			chosen = append(chosen, c)
		}
	}
	if len(chosen) == 0 {
		// The user did choose; trepo just cannot say what. Reporting a
		// cancellation would tell a wrapper the opposite of what happened.
		return nil, fmt.Errorf("the picker returned %d rows that match no checkout", len(keys))
	}
	return chosen, nil
}

// previewCommand runs trepo itself. Resolving the executable rather than
// trusting PATH is what makes the preview work while developing, when the
// binary is a temporary file that `go run` built. fzf hands the command to a
// shell, so the path is quoted: an installation below a directory with a space
// in its name would otherwise break every preview.
func previewCommand() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return shellQuote(exe) + " status {}"
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
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
