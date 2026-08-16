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

	existing, err := a.existingWorktree(r, branch)
	if err != nil {
		return "", err
	}
	if existing != "" {
		return existing, nil
	}

	path, err := a.pathFor(r, branch)
	if err != nil {
		return "", err
	}
	if err := a.checkFree(r, path); err != nil {
		return "", err
	}

	if err := a.create(r, branch, from, path); err != nil {
		return "", err
	}
	a.clearInheritedUpstream(r, branch)
	return Resolve(path), nil
}

func (a Adder) create(r repo.Repo, branch, from, path string) error {
	localExists := git.OK(a.Git, r.Root, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)

	switch {
	case localExists:
		_, err := a.Git.Run(r.Root, "worktree", "add", path, branch)
		return err

	case from == "" && git.OK(a.Git, r.Root, "rev-parse", "--verify", "--quiet",
		"refs/remotes/origin/"+branch):
		// The branch exists only on the remote. Creating a tracking branch
		// first is what keeps the work that is already there; branching off
		// the integration branch would quietly start from nothing.
		if _, err := a.Git.Run(r.Root, "branch", "--track", branch, "origin/"+branch); err != nil {
			return err
		}
		_, err := a.Git.Run(r.Root, "worktree", "add", path, branch)
		return err

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
		return err
	}
}

// existingWorktree finds a checkout already holding the branch.
func (a Adder) existingWorktree(r repo.Repo, branch string) (string, error) {
	worktrees, err := git.ListWorktrees(a.Git, r.Root)
	if err != nil {
		return "", err
	}
	for _, wt := range worktrees {
		if wt.Branch == branch && !wt.Prunable {
			return Resolve(wt.Path), nil
		}
	}
	return "", nil
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

	common, err := git.Output(a.Git, path, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return fmt.Errorf("%s already exists and is not a worktree", path)
	}
	if SamePath(filepath.Dir(strings.TrimSuffix(common, "/")), r.Root) {
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
