package checkout

// Verdict is what the guards decided about removing one checkout.
type Verdict struct {
	// Refused means the removal is not offered at all, not even with --force.
	Refused bool
	Reason  string

	// Confirm holds what the user should be told before the removal happens.
	// --force skips the asking, not the reasons.
	Confirm []string

	// BaseUnknown says the integration branch could not be resolved, so
	// nothing here distinguishes merged work from unmerged work.
	BaseUnknown bool
}

// Guard decides whether a checkout may be removed.
//
// It reads only the flags already computed for the checkout, which is what
// keeps this a pure decision: the same state always produces the same verdict,
// and the rules can be read in one place instead of being spread across the
// commands that remove things.
func Guard(c Checkout, base Base) Verdict {
	var v Verdict

	switch {
	case c.Kind == KindRepo:
		return Verdict{Refused: true, Reason: "refusing to remove the main checkout of " + c.Repo.Slug()}
	case c.Has(FlagCurrent):
		return Verdict{Refused: true, Reason: "refusing to remove the checkout you are standing in"}
	case c.Has(FlagLocked):
		return Verdict{Refused: true, Reason: "checkout is locked; run git worktree unlock first"}
	}

	// A record whose directory is already gone has nothing left to lose.
	if c.Has(FlagPrunable) {
		return v
	}

	if c.Has(FlagDirty) {
		v.Confirm = append(v.Confirm, "has uncommitted changes")
	}
	if c.Has(FlagIgnored) {
		v.Confirm = append(v.Confirm, "has ignored files that no later step can regenerate")
	}
	if c.Has(FlagProtected) {
		v.Confirm = append(v.Confirm, "is managed by another tool")
	}

	if c.Branch != "" && !c.Has(FlagMerged) {
		switch {
		case c.Has(FlagUnpushed):
			v.Confirm = append(v.Confirm, "has commits that were never pushed")
		case c.Has(FlagNoUpstream):
			v.Confirm = append(v.Confirm, "has no upstream, so its commits exist only here")
		case c.Has(FlagGone):
			v.Confirm = append(v.Confirm, "tracks a branch that no longer exists on the remote")
		}
		v.BaseUnknown = !base.Known
	}
	return v
}
