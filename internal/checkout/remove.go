package checkout

import (
	"errors"
	"fmt"

	"github.com/halkn/trepo/internal/git"
)

// ErrSkipped is returned when the user declined a removal. It is not a failure
// of the command, so callers report it and carry on with the next target.
var ErrSkipped = errors.New("skipped")

// Remover deletes worktrees.
type Remover struct {
	Git git.Runner

	// Confirm is asked before anything that could lose work. A nil Confirm
	// means there is no way to ask, so such a removal is refused rather than
	// assumed.
	Confirm func(Checkout, Verdict) bool

	Force  bool
	DryRun bool
}

// Remove deletes one worktree, after the guards agree to it.
func (rm Remover) Remove(c Checkout, base Base) error {
	v := Guard(c, base)
	if v.Refused {
		return errors.New(v.Reason)
	}

	if len(v.Confirm) > 0 && !rm.Force {
		if rm.Confirm == nil {
			return fmt.Errorf("%s %s; rerun with --force to remove it anyway",
				c.Path, v.Confirm[0])
		}
		if !rm.Confirm(c, v) {
			return fmt.Errorf("%s: %w", c.Path, ErrSkipped)
		}
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

// detach removes the worktree itself.
//
// `git worktree remove` does the work rather than deleting the directory
// directly: it is what knows about submodules, refuses on content trepo did
// not account for, and keeps git's own records consistent. --force is passed
// only once trepo's confirmation has already been given.
func (rm Remover) detach(c Checkout) error {
	args := []string{"worktree", "remove"}
	if c.Has(FlagPrunable) {
		// Nothing is left to delete, only git's record of it. `worktree
		// remove` clears exactly that one entry, where `worktree prune` would
		// also drop every other stale record in the repository - entries the
		// user neither selected nor was shown.
		args = append(args, "--force")
	} else if rm.Force {
		args = append(args, "--force")
	} else if rm.Confirm != nil {
		// The user was asked and said yes, so the state git would object to is
		// the state they just agreed to lose.
		if v := Guard(c, Base{}); len(v.Confirm) > 0 {
			args = append(args, "--force")
		}
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
