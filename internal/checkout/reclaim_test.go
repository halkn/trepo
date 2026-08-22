package checkout_test

import (
	"testing"

	"github.com/halkn/trepo/internal/checkout"
)

// This rule decides what gets removed with nobody watching, so every state that
// could still hold work is named here rather than left to a reading of Guard.
func TestReclaimable(t *testing.T) {
	known := checkout.Base{Name: "origin/main", Known: true}

	tests := []struct {
		name string
		c    checkout.Checkout
		base checkout.Base
		want bool
	}{
		{"merged and clean", wt(checkout.FlagMerged), known, true},
		{
			// What a squash merge leaves behind: the commits are on no ancestor
			// of the base, but the branch they were opened as is gone from the
			// remote. Removing the checkout keeps the local branch, so the
			// commits stay reachable.
			name: "gone from the remote", c: wt(checkout.FlagGone), base: known, want: true,
		},
		{
			// Only git's record is left, and nothing points at a directory that
			// no longer exists.
			name: "prunable", c: wt(checkout.FlagPrunable, checkout.FlagNoUpstream),
			base: known, want: true,
		},

		{"uncommitted changes", wt(checkout.FlagMerged, checkout.FlagDirty), known, false},
		{"ignored files", wt(checkout.FlagMerged, checkout.FlagIgnored), known, false},
		{"managed by another tool", wt(checkout.FlagMerged, checkout.FlagProtected), known, false},
		{"gone but dirty", wt(checkout.FlagGone, checkout.FlagDirty), known, false},
		{"gone but protected", wt(checkout.FlagGone, checkout.FlagProtected), known, false},
		{"commits that were never pushed", wt(checkout.FlagUnpushed), known, false},
		{"no upstream and unmerged", wt(checkout.FlagNoUpstream), known, false},
		{"the checkout you are standing in", wt(checkout.FlagMerged, checkout.FlagCurrent), known, false},
		{"locked", wt(checkout.FlagMerged, checkout.FlagLocked), known, false},
		{
			name: "the main checkout",
			c: checkout.Checkout{
				Path: "/repos/app", Branch: "main", Kind: checkout.KindRepo,
				Flags: []checkout.Flag{checkout.FlagMerged},
			},
			base: known, want: false,
		},
		{
			name: "detached at a commit of its own",
			c: checkout.Checkout{
				Path: "/wt/detached", Kind: checkout.KindWorktree,
				Flags: []checkout.Flag{checkout.FlagDetached},
			},
			base: known, want: false,
		},
		{
			// Nothing distinguishes finished work from unfinished work here, and
			// an unattended removal must not guess.
			name: "no base to compare against", c: wt(checkout.FlagNoUpstream),
			base: checkout.Base{}, want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checkout.Reclaimable(tt.c, tt.base); got != tt.want {
				t.Errorf("Reclaimable() = %v, want %v", got, tt.want)
			}
		})
	}
}

// The rule reads a checkout to answer a question about it; a caller that then
// lists or removes it must see the same flags it started with.
func TestReclaimableLeavesItsInputAlone(t *testing.T) {
	c := wt(checkout.FlagGone)

	checkout.Reclaimable(c, checkout.Base{Name: "origin/main", Known: true})

	if !c.Has(checkout.FlagGone) {
		t.Errorf("Reclaimable() took the gone flag off its argument: %v", c.Flags)
	}
}
