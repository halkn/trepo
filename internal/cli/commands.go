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
	"github.com/halkn/trepo/internal/picker"
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
	})
	if !ok {
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
	flags, query, code, ok := a.parse(args, spec{"repos": false, "here": false})
	if !ok {
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
	if flags["repos"] == "true" {
		var kept []checkout.Checkout
		for _, c := range cs {
			if c.Kind == checkout.KindRepo {
				kept = append(kept, c)
			}
		}
		cs = kept
	}

	chosen, err := a.choose(cs, false, "checkout> ")
	if err != nil {
		return a.selectionError(err)
	}
	fmt.Fprintln(a.stdout, chosen[0].Path)
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

// remove deletes worktrees, asking before anything that cannot be undone.
func (a *app) remove(args []string) int {
	flags, query, code, ok := a.parse(args,
		spec{"force": false, "dry-run": false, "no-confirm": false, "here": false})
	if !ok {
		return code
	}
	// Two opposite answers to the same question, so there is no reading of the
	// pair that is not a guess about work the user could lose.
	if flags["force"] == "true" && flags["no-confirm"] == "true" {
		return fail(a.stderr, errors.New(
			"--force removes without asking and --no-confirm keeps whatever needs asking; pass one"))
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

	chosen, err := a.choose(removable, true, "remove> ")
	if err != nil {
		return a.selectionError(err)
	}

	rm := checkout.Remover{
		Git:    a.opts.Git,
		Force:  flags["force"] == "true",
		DryRun: flags["dry-run"] == "true",
	}
	// A rehearsal asks nothing, because it does nothing there is anything to
	// agree to, and --no-confirm is the caller saying they cannot be asked.
	// Either way the removals that would need an answer are skipped with their
	// reason, which is what keeps the guards in place for a caller whose stdin
	// belongs to something else.
	if !rm.DryRun && flags["no-confirm"] != "true" {
		rm.Confirm = a.confirmer()
	}

	status := ExitOK
	done := 0
	for _, c := range chosen {
		base := checkout.ResolveBase(a.opts.Git, c.Repo.Root)
		if err := rm.Remove(c, base); err != nil {
			if errors.Is(err, checkout.ErrSkipped) {
				fmt.Fprintln(a.stderr, "trepo: "+oneline(err))
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
	// Answering "no" to everything leaves the same world behind as dismissing
	// the picker, and a wrapper has to be able to tell both apart from a
	// removal that happened.
	if status == ExitOK && done == 0 {
		return ExitCancelled
	}
	return status
}

// status describes one checkout, and is also what the picker previews with.
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
	c, ok := checkout.Locate(cs, path)
	if !ok {
		return fail(a.stderr, fmt.Errorf("%s is not a checkout of %s", path, target.Slug()))
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
	var repos []checkout.Checkout
	for _, c := range filter(cs, strings.Fields(query)) {
		if c.Kind == checkout.KindRepo {
			repos = append(repos, c)
		}
	}
	chosen, err := a.choose(repos, false, "repository> ")
	if err != nil {
		return repo.Repo{}, a.selectionError(err)
	}
	return chosen[0].Repo, ExitOK
}

// selectionError maps the two ways a selection ends with nothing onto statuses
// a shell can tell apart.
func (a *app) selectionError(err error) int {
	switch {
	case errors.Is(err, errNoMatch), errors.Is(err, picker.ErrNoMatch):
		if a.incomplete {
			fmt.Fprintln(a.stderr,
				"trepo: no matching checkout among the repositories that could be read")
			return ExitError
		}
		fmt.Fprintln(a.stderr, "trepo: no matching checkout")
		return ExitNoMatch
	case errors.Is(err, picker.ErrUnavailable):
		// Several candidates and no way to ask about them. That is neither an
		// empty result nor a decision the user made, so it must not be
		// reported as either; the candidates are already on stderr.
		return ExitError
	case errors.Is(err, picker.ErrCancelled):
		return ExitCancelled
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
