package checkout

import (
	"os"
	"strings"

	"github.com/halkn/trepo/internal/git"
	"github.com/halkn/trepo/internal/repo"
)

// Lister reads the checkouts of a repository and the state of each one.
type Lister struct {
	Git       git.Runner
	Cwd       string
	Protected []string
}

// Repo lists the checkouts of one repository, main checkout first.
func (l Lister) Repo(r repo.Repo) ([]Checkout, error) {
	worktrees, err := git.ListWorktrees(l.Git, r.Root)
	if err != nil {
		return nil, err
	}

	base := ResolveBase(l.Git, r.Root)
	out := make([]Checkout, 0, len(worktrees))
	for _, wt := range worktrees {
		out = append(out, l.describe(r, wt, base))
	}
	return out, nil
}

func (l Lister) describe(r repo.Repo, wt git.Worktree, base Base) Checkout {
	c := Checkout{
		Repo:   r,
		Path:   Resolve(wt.Path),
		Branch: wt.Branch,
		Kind:   KindWorktree,
	}
	if wt.Main {
		c.Kind = KindRepo
	}

	add := func(cond bool, f Flag) {
		if cond {
			c.Flags = append(c.Flags, f)
		}
	}

	add(wt.Bare, FlagBare)
	add(wt.Prunable, FlagPrunable)
	add(wt.Locked, FlagLocked)
	add(wt.Detached, FlagDetached)
	add(wt.Unborn, FlagUnborn)
	add(IsProtected(wt.Path, l.Protected), FlagProtected)
	add(l.Cwd != "" && Under(wt.Path, l.Cwd), FlagCurrent)

	if !wt.Bare && !wt.Prunable && dirExists(wt.Path) {
		dirty, ignored := l.worktreeState(wt.Path)
		add(dirty, FlagDirty)
		add(ignored, FlagIgnored)
	}

	if wt.Branch != "" {
		c.Flags = append(c.Flags, l.branchFlags(r.Root, wt.Branch, base)...)
	} else if wt.Head != "" && !wt.Unborn {
		// A detached checkout has no branch to ask about, but its commit is
		// still either reachable from the base or reachable only from here.
		// That distinction is what says whether removing it loses work.
		add(base.Known && git.OK(l.Git, r.Root,
			"merge-base", "--is-ancestor", wt.Head, base.Name), FlagMerged)
	}
	return c
}

// worktreeState reports uncommitted work. Ignored files are counted separately
// because `git status` stays silent about them, yet a .env or a local
// credential file is work that no later step can reproduce.
func (l Lister) worktreeState(path string) (dirty, ignored bool) {
	out, err := git.Output(l.Git, path, "status", "--porcelain", "--ignored=matching")
	if err != nil {
		return false, false
	}
	for _, line := range strings.Split(out, "\n") {
		switch {
		case line == "":
		case strings.HasPrefix(line, "!!"):
			ignored = true
		default:
			dirty = true
		}
	}
	return dirty, ignored
}

func (l Lister) branchFlags(dir, branch string, base Base) []Flag {
	var flags []Flag

	out, err := git.Output(l.Git, dir,
		"for-each-ref", "--format=%(upstream)%09%(upstream:track)", "refs/heads/"+branch)
	if err == nil {
		upstream, track, _ := strings.Cut(out, "\t")
		switch {
		case upstream == "":
			flags = append(flags, FlagNoUpstream)
		case strings.Contains(track, "[gone]"):
			flags = append(flags, FlagGone)
		case strings.Contains(track, "ahead"):
			flags = append(flags, FlagUnpushed)
		}
	}

	if base.Known && git.OK(l.Git, dir, "merge-base", "--is-ancestor", branch, base.Name) {
		flags = append(flags, FlagMerged)
	}
	return flags
}

// ResolveBase finds the ref a branch would be merged into.
//
// origin/HEAD is read with symbolic-ref rather than rev-parse: rev-parse
// treats a missing origin/HEAD as a fatal error, and the message would end up
// parsed as if it were a ref name.
func ResolveBase(r git.Runner, dir string) Base {
	// One process for every candidate rather than one per candidate. Listing
	// spends its time starting git rather than running it, and this ran for
	// each repository enumerated. for-each-ref takes several patterns, leaves
	// the missing ones out of its output and still succeeds, so the whole
	// question fits in a single command.
	args := append([]string{"for-each-ref", "--format=%(refname)%09%(symref:short)"},
		baseCandidates...)
	out, err := git.Output(r, dir, args...)
	if err != nil {
		return Base{}
	}

	found := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		name, target, _ := strings.Cut(line, "\t")
		found[name] = target
	}

	// git sorts its answer by refname, so precedence is applied here: the
	// remote's own idea of the default branch first, then the conventional
	// names, remote before local.
	//
	// origin/HEAD counts only while it is symbolic. A plain ref of that name
	// says which commit the remote's HEAD was at, not which branch trepo should
	// compare against, and naming it as the base would report merged work
	// against a moving target.
	if target := found[originHead]; target != "" {
		return Base{Name: target, Known: true}
	}
	for _, candidate := range baseCandidates[1:] {
		if _, ok := found[candidate]; ok {
			return Base{Name: strings.TrimPrefix(
				strings.TrimPrefix(candidate, "refs/remotes/"), "refs/heads/"), Known: true}
		}
	}
	return Base{}
}

const originHead = "refs/remotes/origin/HEAD"

// baseCandidates is in precedence order, which is also the order they are
// handed to git. origin/HEAD leads it and is the one that has to be symbolic.
var baseCandidates = []string{
	originHead,
	"refs/remotes/origin/main", "refs/remotes/origin/master",
	"refs/heads/main", "refs/heads/master",
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
