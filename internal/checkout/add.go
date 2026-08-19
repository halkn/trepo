package checkout

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/halkn/trepo/internal/git"
	"github.com/halkn/trepo/internal/repo"
)

// Adder creates worktrees.
type Adder struct {
	Git          git.Runner
	WorktreeRoot string
	Template     string

	// Warn reports something the caller asked for that could not be honoured,
	// while the request as a whole still succeeds. A nil Warn drops the message.
	Warn func(string)
}

// templateData is what a worktree template may refer to.
type templateData struct {
	Host   string
	Owner  string
	Repo   string
	Branch string
}

// Add makes sure a worktree for branch exists and returns its path.
//
// It is idempotent on purpose: the caller wants somewhere to work, and asking
// for a checkout that is already there is a request to go to it.
func (a Adder) Add(r repo.Repo, branch, from string) (string, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return "", fmt.Errorf("no branch given")
	}
	branch = strings.TrimPrefix(branch, "origin/")

	existing, stale, err := a.existingWorktree(r, branch)
	if err != nil {
		return "", err
	}
	if existing != "" {
		a.warnUnusedBase(branch, from)
		return existing, nil
	}
	if stale != "" {
		// The directory is gone but git still has the record, and it holds
		// both the branch and the path. Clearing it is what makes asking again
		// give the checkout back instead of a complaint about bookkeeping.
		if _, err := a.Git.Run(r.Root, "worktree", "remove", "--force", stale); err != nil {
			return "", err
		}
	}

	path, err := a.pathFor(r, branch)
	if err != nil {
		return "", err
	}
	if err := a.checkFree(r, path); err != nil {
		return "", err
	}

	branchIsNew, err := a.create(r, branch, from, path)
	if err != nil {
		return "", err
	}
	if branchIsNew {
		a.clearInheritedUpstream(r, branch)
	}
	return Resolve(path), nil
}

// create makes the checkout and reports whether the branch was created here.
// Only a branch made off a base ref can have inherited an upstream, so only
// that answer may lead to one being dropped.
func (a Adder) create(r repo.Repo, branch, from, path string) (branchIsNew bool, err error) {
	localExists := git.OK(a.Git, r.Root, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)

	switch {
	case localExists:
		a.warnUnusedBase(branch, from)
		_, err := a.Git.Run(r.Root, "worktree", "add", path, branch)
		return false, err

	case from == "" && git.OK(a.Git, r.Root, "rev-parse", "--verify", "--quiet",
		"refs/remotes/origin/"+branch):
		// The branch exists only on the remote. Creating a tracking branch
		// first is what keeps the work that is already there; branching off
		// the integration branch would quietly start from nothing. Its
		// upstream is the one it was asked to track, so it stays.
		if _, err := a.Git.Run(r.Root, "branch", "--track", branch, "origin/"+branch); err != nil {
			return false, err
		}
		_, err := a.Git.Run(r.Root, "worktree", "add", path, branch)
		return false, err

	default:
		base := from
		if base == "" {
			if b := ResolveBase(a.Git, r.Root); b.Known {
				base = b.Name
			} else {
				base = "HEAD"
			}
		}
		_, err := a.Git.Run(r.Root, "worktree", "add", "--no-track", "-b", branch, path, base)
		return err == nil, err
	}
}

// warnUnusedBase says that a branch which already exists was not rebuilt on the
// requested base. Checking out the branch as it is stays the right answer for
// an idempotent command, but silence would leave the user believing the
// checkout starts where they asked.
func (a Adder) warnUnusedBase(branch, from string) {
	if from == "" || a.Warn == nil {
		return
	}
	a.Warn(fmt.Sprintf("%s already exists, so it was not created from %s", branch, from))
}

// existingWorktree looks for a checkout already holding the branch, reporting a
// usable one and a stale record separately: the first is the answer, the second
// is in the way and has to be cleared.
func (a Adder) existingWorktree(r repo.Repo, branch string) (existing, stale string, err error) {
	worktrees, err := git.ListWorktrees(a.Git, r.Root)
	if err != nil {
		return "", "", err
	}
	for _, wt := range worktrees {
		if wt.Branch != branch {
			continue
		}
		if wt.Prunable {
			stale = wt.Path
			continue
		}
		return Resolve(wt.Path), "", nil
	}
	return "", stale, nil
}

// pathFor renders the worktree template below the worktree root.
func (a Adder) pathFor(r repo.Repo, branch string) (string, error) {
	tmpl, err := template.New("worktree").Parse(a.Template)
	if err != nil {
		return "", fmt.Errorf("trepo.worktreeTemplate: %w", err)
	}

	var out strings.Builder
	err = tmpl.Execute(&out, templateData{
		Host:   r.Host,
		Owner:  r.Owner,
		Repo:   r.Name,
		Branch: branch,
	})
	if err != nil {
		return "", fmt.Errorf("trepo.worktreeTemplate: %w", err)
	}

	rel, err := sanitize(out.String())
	if err != nil {
		return "", fmt.Errorf("trepo.worktreeTemplate: %w", err)
	}

	path := filepath.Join(a.WorktreeRoot, rel)
	// The template is configuration, so it can say anything; a checkout must
	// still land inside the root set aside for worktrees.
	if !Under(a.WorktreeRoot, path) {
		return "", fmt.Errorf("trepo.worktreeTemplate places %q outside %q", path, a.WorktreeRoot)
	}
	return path, nil
}

// checkFree refuses a target directory that another repository is using.
func (a Adder) checkFree(r repo.Repo, path string) error {
	if _, err := os.Stat(path); err != nil {
		return nil
	}

	owner, err := git.RepoRoot(a.Git, path)
	if err != nil {
		return fmt.Errorf("%s already exists and is not a worktree", path)
	}
	if SamePath(owner, r.Root) {
		return nil
	}
	return fmt.Errorf("%s already belongs to another repository; "+
		"add {{.Host}} to trepo.worktreeTemplate to keep %s separate", path, r.Slug())
}

// clearInheritedUpstream drops an upstream whose name does not match the
// branch. A branch made off origin/main inherits origin/main, and push.default
// = simple then refuses the first push because the names differ.
func (a Adder) clearInheritedUpstream(r repo.Repo, branch string) {
	upstream, err := git.Output(a.Git, r.Root,
		"for-each-ref", "--format=%(upstream:short)", "refs/heads/"+branch)
	if err != nil || upstream == "" {
		return
	}
	if !strings.HasSuffix(upstream, "/"+branch) {
		_, _ = a.Git.Run(r.Root, "branch", "--unset-upstream", branch)
	}
}

// sanitize fixes only what git allows in a ref but a filesystem does not.
//
// Slashes stay: git's own rules make nested branch directories collision-free,
// because a and a/b cannot both exist and no component may start with a dot.
// A ".." component is refused rather than dropped, so that a template which
// tries to climb out of the root says so instead of quietly landing elsewhere.
func sanitize(rel string) (string, error) {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	kept := parts[:0]
	for _, part := range parts {
		if part == ".." {
			return "", fmt.Errorf("%q climbs out of the worktree root", rel)
		}
		part = strings.ReplaceAll(part, `\`, "-")
		part = strings.TrimRight(part, ". ")
		if part == "" || part == "." {
			continue
		}
		kept = append(kept, part)
	}
	if len(kept) == 0 {
		return "", fmt.Errorf("%q renders to an empty path", rel)
	}
	return filepath.Join(kept...), nil
}
