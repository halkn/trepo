package checkout_test

import (
	"strings"
	"testing"

	"github.com/halkn/trepo/internal/checkout"
)

func wt(flags ...checkout.Flag) checkout.Checkout {
	return checkout.Checkout{
		Path:   "/wt/feat-x",
		Branch: "feat/x",
		Kind:   checkout.KindWorktree,
		Flags:  flags,
	}
}

func TestGuardRefusals(t *testing.T) {
	known := checkout.Base{Name: "origin/main", Known: true}

	tests := []struct {
		name string
		c    checkout.Checkout
	}{
		{
			// Removing it would take the repository with it.
			name: "the main checkout",
			c: checkout.Checkout{
				Path: "/repos/app", Branch: "main", Kind: checkout.KindRepo,
				Flags: []checkout.Flag{checkout.FlagMerged},
			},
		},
		{
			// The shell would be left standing in a directory that is gone.
			name: "the checkout holding the working directory",
			c:    wt(checkout.FlagCurrent, checkout.FlagMerged),
		},
		{
			// A lock is a statement that something else is using this; git
			// refuses too, so honouring --force here would only disagree with
			// the command doing the work.
			name: "a locked checkout",
			c:    wt(checkout.FlagLocked, checkout.FlagMerged),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := checkout.Guard(tt.c, known)
			if !v.Refused {
				t.Errorf("Guard() = %+v, want refused", v)
			}
			if v.Reason == "" {
				t.Error("a refusal with no reason")
			}
		})
	}
}

func TestGuardConfirmations(t *testing.T) {
	known := checkout.Base{Name: "origin/main", Known: true}

	tests := []struct {
		name string
		c    checkout.Checkout
		want string
	}{
		{"uncommitted changes", wt(checkout.FlagDirty, checkout.FlagMerged), "uncommitted"},
		{"ignored files", wt(checkout.FlagIgnored, checkout.FlagMerged), "ignored"},
		{"commits that were never pushed", wt(checkout.FlagUnpushed), "pushed"},
		{"no upstream and unmerged work", wt(checkout.FlagNoUpstream), "upstream"},
		{"managed by another tool", wt(checkout.FlagProtected, checkout.FlagMerged), "another tool"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := checkout.Guard(tt.c, known)
			if v.Refused {
				t.Fatalf("Guard() = %+v, want a confirmation rather than a refusal", v)
			}
			if len(v.Confirm) == 0 {
				t.Fatalf("Guard() = %+v, want a confirmation", v)
			}
			if !strings.Contains(strings.Join(v.Confirm, " "), tt.want) {
				t.Errorf("confirmations %v do not mention %q", v.Confirm, tt.want)
			}
		})
	}
}

// A branch already merged into the integration branch, with nothing left in
// the working tree, is the case this whole flow exists to make cheap.
func TestGuardAllowsAMergedCleanWorktreeOutright(t *testing.T) {
	v := checkout.Guard(wt(checkout.FlagMerged), checkout.Base{Name: "origin/main", Known: true})

	if v.Refused || len(v.Confirm) != 0 {
		t.Errorf("Guard() = %+v, want it allowed without confirmation", v)
	}
}

// Nothing can be lost by removing a record whose directory is already gone.
func TestGuardAllowsAPrunableEntry(t *testing.T) {
	v := checkout.Guard(wt(checkout.FlagPrunable, checkout.FlagNoUpstream),
		checkout.Base{Name: "origin/main", Known: true})

	if v.Refused || len(v.Confirm) != 0 {
		t.Errorf("Guard() = %+v, want it allowed without confirmation", v)
	}
}

// With no base ref, "not merged" is not something trepo established; the
// confirmation has to say that rather than assert unmerged work exists.
func TestGuardSaysSoWhenItCannotTellMergedFromUnmerged(t *testing.T) {
	v := checkout.Guard(wt(checkout.FlagUnpushed), checkout.Base{})

	if !v.BaseUnknown {
		t.Errorf("Guard() = %+v, want BaseUnknown", v)
	}
}

func TestGuardOnADetachedCheckoutDoesNotClaimBranchState(t *testing.T) {
	c := checkout.Checkout{
		Path: "/wt/detached", Kind: checkout.KindWorktree,
		Flags: []checkout.Flag{checkout.FlagDetached},
	}
	v := checkout.Guard(c, checkout.Base{Name: "origin/main", Known: true})

	if v.Refused {
		t.Errorf("Guard() = %+v, want it allowed", v)
	}
	if v.BaseUnknown {
		t.Errorf("Guard() = %+v, want no base claim for a detached checkout", v)
	}
}
