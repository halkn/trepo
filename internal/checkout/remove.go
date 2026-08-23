package checkout

import (
	"errors"
	"fmt"

	"github.com/halkn/trepo/internal/git"
)

// ErrSkipped marks a removal the guards stopped short of, because it needs a
// decision trepo will not make on its own. That is not a failure of the
// command, so callers report it and carry on with the next target.
var ErrSkipped = errors.New("skipped")

// skipped carries the reason one checkout was left alone. It reads as ErrSkipped
// while printing only its own message, so a caller can both recognise the case
// and tell the user what would have to change for the removal to happen.
type skipped struct{ msg string }

func (s skipped) Error() string { return s.msg }
func (s skipped) Unwrap() error { return ErrSkipped }

// Remover deletes worktrees.
type Remover struct {
	Git git.Runner

	// Force removes what the guards would otherwise leave to a decision.
	Force bool

	// Reclaim says the caller selected this checkout because its work is
	// finished, which settles the one question that selection turns on: a
	// branch retired on the remote. Every other reason to hold back still
	// applies.
	Reclaim bool

	DryRun bool
}

// Remove deletes one worktree, after the guards agree to it.
//
// Nothing here asks. A removal the guards want a decision on is reported with
// its reason and left alone: a caller has no stdin to answer with, and
// assuming an answer on its behalf is how work gets lost.
func (rm Remover) Remove(c Checkout, base Base) error {
	v := rm.verdict(c, base)
	if v.Refused {
		return errors.New(v.Reason)
	}

	if len(v.Confirm) > 0 && !rm.Force {
		// One reason, not all of them: this line ends up inside other tools'
		// interfaces, and the first reason is already enough to say why the
		// checkout is still there.
		return skipped{fmt.Sprintf("skipped %s: it %s; rerun with --force to remove it anyway",
			c.Path, v.Confirm[0])}
	}

	if rm.DryRun {
		return nil
	}

	if err := rm.detach(c); err != nil {
		return err
	}
	rm.deleteMergedBranch(c, base)
	return nil
}

// verdict is Guard asked the question the caller has left open. Reclaiming is
// the only exemption, and it is expressed the same way Reclaimable expresses
// it, so the two cannot come to disagree about what was already settled.
func (rm Remover) verdict(c Checkout, base Base) Verdict {
	if rm.Reclaim {
		return Guard(withoutGone(c), base)
	}
	return Guard(c, base)
}

// detach removes the worktree itself.
//
// `git worktree remove` does the work rather than deleting the directory
// directly: it is what knows about submodules, refuses on content trepo did
// not account for, and keeps git's own records consistent. --force is passed
// only where trepo has already decided in favour of the state git would object
// to, so a refusal from git never turns into a removal nobody asked for.
func (rm Remover) detach(c Checkout) error {
	args := []string{"worktree", "remove"}
	if c.Has(FlagPrunable) || rm.Force {
		// For a prunable record there is nothing left to delete but git's own
		// entry. `worktree remove` clears exactly that one entry, where
		// `worktree prune` would also drop every other stale record in the
		// repository - entries the user neither selected nor was shown.
		args = append(args, "--force")
	}

	_, err := rm.Git.Run(c.Repo.Root, append(args, c.Path)...)
	return err
}

// deleteMergedBranch tidies up a branch whose work is already in the base.
// Only -d, never -D: a branch that git will not delete on its own terms is
// holding something, and that is not trepo's to discard.
func (rm Remover) deleteMergedBranch(c Checkout, base Base) {
	if c.Branch == "" || !base.Known || !c.Has(FlagMerged) {
		return
	}
	_, _ = rm.Git.Run(c.Repo.Root, "branch", "-d", c.Branch)
}

// Locate finds the checkout at path among ones already listed.
//
// Matching against the listing rather than asking git about the path is what
// makes a removed directory reclaimable: `git -C <gone>` fails before it can
// report which repository the worktree belonged to.
func Locate(all []Checkout, path string) (Checkout, bool) {
	for _, c := range all {
		if SamePath(c.Path, path) {
			return c, true
		}
	}
	return Checkout{}, false
}
