// Package checkout treats a repository's main checkout and its worktrees as
// one kind of thing.
//
// That is the whole point of trepo: with agents working in parallel, several
// checkouts of one repository is the normal state, and picking a place to work
// should not first require deciding whether that place is a clone or a
// worktree.
package checkout

import (
	"sort"

	"github.com/halkn/trepo/internal/repo"
)

// Kind says whether a checkout is the repository itself or one of its
// worktrees. It is an attribute, not a separate concept.
type Kind string

const (
	KindRepo     Kind = "repo"
	KindWorktree Kind = "worktree"
)

// Flag is one observation about a checkout. Listing, status and the deletion
// guards all read these instead of running their own git commands, so there is
// a single answer to "what state is this checkout in".
type Flag string

const (
	FlagCurrent    Flag = "current"
	FlagDirty      Flag = "dirty"
	FlagIgnored    Flag = "ignored"
	FlagDetached   Flag = "detached"
	FlagUnborn     Flag = "unborn"
	FlagPrunable   Flag = "prunable"
	FlagLocked     Flag = "locked"
	FlagMerged     Flag = "merged"
	FlagGone       Flag = "gone"
	FlagUnpushed   Flag = "unpushed"
	FlagNoUpstream Flag = "no-upstream"
	FlagProtected  Flag = "protected"
	FlagBare       Flag = "bare"
)

// Checkout is a place work can happen.
type Checkout struct {
	Repo   repo.Repo
	Path   string
	Branch string // empty when detached or unborn
	Kind   Kind
	Flags  []Flag
}

func (c Checkout) Has(f Flag) bool {
	for _, got := range c.Flags {
		if got == f {
			return true
		}
	}
	return false
}

// Base is the ref a branch would be merged into.
//
// Whether it could be resolved is part of the value: a repository with no
// remote and no main branch has no answer, and callers must not read the
// absence of the merged flag as proof that work is unmerged.
type Base struct {
	Name  string
	Known bool
}

// Sort orders checkouts the way every command prints them: by repository, main
// checkout first within a repository, then by branch. Listing runs
// concurrently, so without this the output — and the cursor position in
// whatever draws it — would move between invocations.
func Sort(cs []Checkout) {
	sort.SliceStable(cs, func(i, j int) bool {
		a, b := cs[i], cs[j]
		if a.Repo.Root != b.Repo.Root {
			return a.Repo.Root < b.Repo.Root
		}
		if (a.Kind == KindRepo) != (b.Kind == KindRepo) {
			return a.Kind == KindRepo
		}
		if a.Branch != b.Branch {
			return a.Branch < b.Branch
		}
		return a.Path < b.Path
	})
}
