package checkout

// Reclaimable reports whether a worktree has finished its job and can be
// removed with nobody watching.
//
// The decision is deferred to Guard rather than restated here, so a state that
// Guard learns to ask about later stops being reclaimed without this rule
// having to be updated too. What is left is one question of its own — has this
// checkout finished — and one exemption.
//
// The exemption is the gone flag. A branch whose remote counterpart was deleted
// is what a squash merge leaves behind: the commits are on no ancestor of the
// base, so nothing marks them merged, yet the branch was retired on purpose.
// Named on its own that is worth holding back on; asking for what is finished
// is what settles it. Removing the checkout also keeps the local branch, since
// only a merged branch is deleted along with it, so the commits stay reachable
// either way.
func Reclaimable(c Checkout, base Base) bool {
	if c.Kind != KindWorktree {
		return false
	}
	if !c.Has(FlagPrunable) && !c.Has(FlagMerged) && !c.Has(FlagGone) {
		return false
	}

	v := Guard(withoutGone(c), base)
	return !v.Refused && len(v.Confirm) == 0
}

// withoutGone copies the checkout with the gone flag dropped, so Guard answers
// the question the caller has already settled. The flags are copied rather than
// filtered in place: the same checkout is listed and removed afterwards.
func withoutGone(c Checkout) Checkout {
	flags := make([]Flag, 0, len(c.Flags))
	for _, f := range c.Flags {
		if f != FlagGone {
			flags = append(flags, f)
		}
	}
	c.Flags = flags
	return c
}
