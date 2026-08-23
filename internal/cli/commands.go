package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/halkn/trepo/internal/checkout"
	"github.com/halkn/trepo/internal/config"
	"github.com/halkn/trepo/internal/git"
	"github.com/halkn/trepo/internal/repo"
)

// get clones a repository into the place its URL implies.
func (a *app) get(args []string) int {
	_, rest, code, ok := a.parse(args, spec{})
	if !ok {
		return code
	}
	if len(rest) != 1 {
		return fail(a.stderr, errors.New("usage: trepo get <owner/repo|url>"))
	}

	src, err := repo.Parse(rest[0], a.cfg.DefaultHost)
	if err != nil {
		return fail(a.stderr, err)
	}
	dest := filepath.Join(a.cfg.Root, src.Repo.RelPath())

	if _, err := os.Stat(dest); err == nil {
		if !repo.IsRepo(dest) {
			// What an interrupted clone leaves behind. Printing the path would
			// send every later `cd -- "$(trepo get ...)"` into a directory that
			// no run of get would ever repair.
			return fail(a.stderr, fmt.Errorf(
				"%s already exists but is not a repository; remove it and run get again", dest))
		}
		// Already there. Printing the path rather than complaining keeps
		// `cd -- "$(trepo get ...)"` working whether or not this is the first
		// time the repository was asked for.
		fmt.Fprintln(a.stdout, checkout.Resolve(dest))
		return ExitOK
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fail(a.stderr, err)
	}
	fmt.Fprintf(a.stderr, "trepo: cloning %s\n", src.CloneURL)
	if _, err := a.opts.Git.Run("", "clone", src.CloneURL, dest); err != nil {
		return fail(a.stderr, err)
	}

	fmt.Fprintln(a.stdout, checkout.Resolve(dest))
	return ExitOK
}

// list prints checkouts without ever asking a question, which is what makes it
// usable as a data source for other tools.
func (a *app) list(args []string) int {
	flags, query, code, ok := a.parse(args, spec{
		"json": false, "repos": false, "worktrees": false, "here": false,
		"cwd": true,
	})
	if !ok {
		return code
	}
	if code := a.standIn(flags); code != ExitOK {
		return code
	}
	asJSON := flags["json"] == "true"
	onlyRepos := flags["repos"] == "true"
	onlyWorktrees := flags["worktrees"] == "true"

	cs, err := a.checkouts()
	if err != nil {
		return fail(a.stderr, err)
	}
	cs, code = a.here(filter(cs, query), flags)
	if code != ExitOK {
		return code
	}

	var kept []checkout.Checkout
	for _, c := range cs {
		if onlyRepos && c.Kind != checkout.KindRepo {
			continue
		}
		if onlyWorktrees && c.Kind != checkout.KindWorktree {
			continue
		}
		kept = append(kept, c)
	}

	if asJSON {
		return a.printJSON(kept)
	}
	for _, c := range kept {
		fmt.Fprintln(a.stdout, strings.Join(append(row(c), c.Path), "\t"))
	}
	return ExitOK
}

// path answers with one location and nothing else.
func (a *app) path(args []string) int {
	flags, query, code, ok := a.parse(args, spec{
		"repos": false, "here": false, "current": false, "cwd": true,
	})
	if !ok {
		return code
	}
	if code := a.standIn(flags); code != ExitOK {
		return code
	}

	cs, err := a.checkouts()
	if err != nil {
		return fail(a.stderr, err)
	}
	cs, code = a.here(filter(cs, query), flags)
	if code != ExitOK {
		return code
	}
	cs = current(cs, flags, a.opts.Cwd)
	if flags["repos"] == "true" {
		var kept []checkout.Checkout
		for _, c := range cs {
			if c.Kind == checkout.KindRepo {
				kept = append(kept, c)
			}
		}
		cs = kept
	}

	chosen, err := only(cs, "checkouts", query)
	if err != nil {
		return a.selectionError(err)
	}
	fmt.Fprintln(a.stdout, chosen.Path)
	return ExitOK
}

// add creates a worktree and prints where it is.
func (a *app) add(args []string) int {
	flags, rest, code, ok := a.parse(args, spec{"repo": true, "from": true})
	if !ok {
		return code
	}
	if len(rest) != 1 {
		return fail(a.stderr, errors.New("usage: trepo add <branch> [--repo <query>] [--from <ref>]"))
	}

	target, code := a.targetRepo(flags["repo"])
	if code != ExitOK {
		return code
	}

	rc := config.LoadRepo(a.opts.Git, target.Root)
	path, err := checkout.Adder{
		Git:          a.opts.Git,
		WorktreeRoot: a.cfg.WorktreeRoot,
		Template:     rc.WorktreeTemplate,
		Warn:         func(msg string) { fmt.Fprintln(a.stderr, "trepo: "+msg) },
	}.Add(target, rest[0], flags["from"])
	if err != nil {
		return fail(a.stderr, err)
	}

	fmt.Fprintln(a.stdout, path)
	return ExitOK
}

// remove deletes worktrees, and keeps the ones whose removal it will not
// decide on its own.
func (a *app) remove(args []string) int {
	flags, query, code, ok := a.parse(args, spec{
		"force": false, "dry-run": false, "here": false, "reclaimable": false,
	})
	if !ok {
		return code
	}
	reclaim := flags["reclaimable"] == "true"
	// Reclaiming only ever selects what needs no question, so --force could not
	// widen it; passing both reads as authority the run does not have.
	if flags["force"] == "true" && reclaim {
		return fail(a.stderr, errors.New(
			"--reclaimable only takes what needs no confirmation, so --force adds nothing; pass one"))
	}

	all, err := a.checkouts()
	if err != nil {
		return fail(a.stderr, err)
	}

	candidates, code := a.here(filter(all, query), flags)
	if code != ExitOK {
		return code
	}
	// The main checkout can never be a target, so offering it would only make
	// the list longer and the refusal more likely.
	var removable []checkout.Checkout
	for _, c := range candidates {
		if c.Kind == checkout.KindWorktree {
			removable = append(removable, c)
		}
	}

	bases := &baseCache{git: a.opts.Git, seen: map[string]checkout.Base{}}
	var chosen []checkout.Checkout
	if reclaim {
		// The flag is the selection, so several targets are what was asked for
		// rather than a query that failed to name one.
		for _, c := range removable {
			if checkout.Reclaimable(c, bases.of(c.Repo.Root)) {
				chosen = append(chosen, c)
			}
		}
		if len(chosen) == 0 {
			return a.selectionError(errNoMatch)
		}
	} else {
		c, err := only(removable, "worktrees", query)
		if err != nil {
			return a.selectionError(err)
		}
		chosen = []checkout.Checkout{c}
	}

	rm := checkout.Remover{
		Git:     a.opts.Git,
		Force:   flags["force"] == "true",
		Reclaim: reclaim,
		DryRun:  flags["dry-run"] == "true",
	}

	status := ExitOK
	done, kept := 0, 0
	for _, c := range chosen {
		base := bases.of(c.Repo.Root)
		if err := rm.Remove(c, base); err != nil {
			if errors.Is(err, checkout.ErrSkipped) {
				fmt.Fprintln(a.stderr, "trepo: "+oneline(err))
				kept++
				continue
			}
			fail(a.stderr, err)
			status = ExitError
			continue
		}
		done++
		// Reported only once the guards have agreed, so a rehearsal never
		// announces a removal that would in fact be refused.
		if rm.DryRun {
			fmt.Fprintf(a.stderr, "trepo: would remove %s\n", c.Path)
		}
	}
	// A run that removed one checkout and kept another still succeeded, and
	// which ones were kept is on stderr. Keeping every one of them is the case
	// a wrapper has to tell apart: nothing changed, and the reasons on stderr
	// say what a further run would have to carry.
	if status == ExitOK && done == 0 && kept > 0 {
		return ExitUndecided
	}
	return status
}

// baseCache answers what a repository's integration branch is, once per
// repository. The answer is the same for every checkout of one repository, and
// resolving it per candidate would run git twice over for each of them: once to
// select, once to remove.
type baseCache struct {
	git  git.Runner
	seen map[string]checkout.Base
}

func (b *baseCache) of(root string) checkout.Base {
	if base, ok := b.seen[root]; ok {
		return base
	}
	base := checkout.ResolveBase(b.git, root)
	b.seen[root] = base
	return base
}

// status describes one checkout, and is what a caller's preview window renders
// a row with.
func (a *app) status(args []string) int {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Fprintln(a.stdout, usage)
		return ExitOK
	}
	if len(args) != 1 {
		return fail(a.stderr, errors.New("usage: trepo status <path>"))
	}
	path := args[0]

	if _, err := os.Stat(path); err != nil {
		// Say so plainly instead of failing: this runs inside a preview
		// window, where an error would replace the whole pane with noise.
		fmt.Fprintf(a.stdout, "missing: %s\n", path)
		return ExitOK
	}

	root, err := git.RepoRoot(a.opts.Git, path)
	if err != nil {
		return fail(a.stderr, fmt.Errorf("%s is not inside a git repository", path))
	}

	target := repo.FromRoot(root, a.cfg.Root)
	cs, err := (checkout.Finder{Git: a.opts.Git, Cwd: a.opts.Cwd}).Repo(target)
	if err != nil {
		return fail(a.stderr, err)
	}
	// Any directory below a checkout, not only a path trepo printed: a preview
	// renders whatever the caller is looking at, which is a pane's working
	// directory as often as it is a row taken from the listing.
	c, ok := checkout.Containing(cs, path)
	if !ok {
		return fail(a.stderr, fmt.Errorf("%s is not inside a checkout of %s", path, target.Slug()))
	}

	fmt.Fprintf(a.stdout, "%s\n", c.Repo.Slug())
	fmt.Fprintf(a.stdout, "branch  %s\n", orDash(c.Branch))
	fmt.Fprintf(a.stdout, "kind    %s\n", c.Kind)
	fmt.Fprintf(a.stdout, "flags   %s\n", strings.Join(flagNames(c), ","))
	fmt.Fprintf(a.stdout, "path    %s\n", c.Path)

	if c.Branch != "" {
		if track, err := git.Output(a.opts.Git, c.Repo.Root,
			"for-each-ref", "--format=%(upstream:short) %(upstream:track)",
			"refs/heads/"+c.Branch); err == nil && strings.TrimSpace(track) != "" {
			fmt.Fprintf(a.stdout, "upstream %s\n", strings.TrimSpace(track))
		}
	}
	if out, err := git.Output(a.opts.Git, c.Path, "status", "--porcelain"); err == nil && out != "" {
		fmt.Fprintf(a.stdout, "changed %d file(s)\n", len(strings.Split(out, "\n")))
	}
	return ExitOK
}

// targetRepo decides which repository a command acts on: the one named by a
// query, or the one the working directory is in. There is no implicit fallback
// beyond that, because listing spans every repository and a silently guessed
// target would create worktrees somewhere the user never looked.
func (a *app) targetRepo(query string) (repo.Repo, int) {
	if query == "" {
		root, err := git.RepoRoot(a.opts.Git, a.opts.Cwd)
		if err != nil {
			return repo.Repo{}, fail(a.stderr,
				errors.New("not inside a repository; name one with --repo <query>"))
		}
		return repo.FromRoot(root, a.cfg.Root), ExitOK
	}

	cs, err := a.checkouts()
	if err != nil {
		return repo.Repo{}, fail(a.stderr, err)
	}
	terms := strings.Fields(query)
	var repos []checkout.Checkout
	for _, c := range filter(cs, terms) {
		if c.Kind == checkout.KindRepo {
			repos = append(repos, c)
		}
	}
	chosen, err := only(repos, "repositories", terms)
	if err != nil {
		return repo.Repo{}, a.selectionError(err)
	}
	return chosen.Repo, ExitOK
}

// selectionError maps the ways a selection ends without one checkout onto
// statuses a shell can tell apart.
func (a *app) selectionError(err error) int {
	switch {
	case errors.Is(err, errNoMatch):
		if a.incomplete {
			fmt.Fprintln(a.stderr,
				"trepo: no matching checkout among the repositories that could be read")
			return ExitError
		}
		fmt.Fprintln(a.stderr, "trepo: no matching checkout")
		return ExitNoMatch
	case errors.Is(err, errAmbiguous):
		fmt.Fprintln(a.stderr, "trepo: "+oneline(err))
		return ExitUndecided
	default:
		return fail(a.stderr, err)
	}
}

type jsonCheckout struct {
	Repo   string   `json:"repo"`
	Host   string   `json:"host"`
	Owner  string   `json:"owner"`
	Name   string   `json:"name"`
	Branch string   `json:"branch"`
	Path   string   `json:"path"`
	Kind   string   `json:"kind"`
	Flags  []string `json:"flags"`
}

// printJSON marshals a shape of its own rather than the internal checkout, so
// the published format does not follow whatever the internals become.
func (a *app) printJSON(cs []checkout.Checkout) int {
	out := make([]jsonCheckout, 0, len(cs))
	for _, c := range cs {
		out = append(out, jsonCheckout{
			Repo:   c.Repo.Slug(),
			Host:   c.Repo.Host,
			Owner:  c.Repo.Owner,
			Name:   c.Repo.Name,
			Branch: c.Branch,
			Path:   c.Path,
			Kind:   string(c.Kind),
			Flags:  flagNames(c),
		})
	}
	enc := json.NewEncoder(a.stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return fail(a.stderr, err)
	}
	return ExitOK
}

func flagNames(c checkout.Checkout) []string {
	names := make([]string, 0, len(c.Flags))
	for _, f := range c.Flags {
		names = append(names, string(f))
	}
	return names
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
