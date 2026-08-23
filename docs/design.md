# Design

Why trepo is shaped the way it is. `README.md` says what the commands do; this
file says which decisions are load-bearing, so a change can be judged against
them instead of against the current code.

A **checkout** is the one concept everything else hangs off: a repository's main
checkout, or one of its worktrees. `checkout.Checkout` in `internal/checkout`
carries both, and `Kind` (`repo` or `worktree`) is an attribute of a checkout
rather than a type of its own.

Where each decision lives in the code:

| decision | code |
| --- | --- |
| what a checkout is, how one is listed and flagged | `internal/checkout` (`checkout.go`, `list.go`, `all.go`) |
| whether a removal is allowed, and what it needs confirmed | `internal/checkout/guard.go`, `remove.go`, `reclaim.go` |
| running git and parsing its porcelain output | `internal/git` |
| subcommands, flags, output and exit status | `internal/cli` |
| layout of a clone, and finding existing ones | `internal/repo` |

What follows is what trepo is meant to be, which is not always what the code is
today. Where the two differ, this file is the one to change the code towards;
the gap itself is tracked in issues, not here.

## Responsibilities

trepo resolves. Choosing between candidates and moving into one is the caller's
job.

Resolving means: which repositories and worktrees exist, where each one lives,
what state it is in, whether removing it would lose work. Those answers need
git, and they are the part worth testing.

Selecting and transitioning means: drawing a picker, previewing a row inside a
frame, `cd`, focusing a terminal workspace, trusting a `mise` config. Those
depend on the shell, the terminal and the tools around them, and there is more
than one caller.

The boundary follows from having several callers of the same answers. A
judgement duplicated across callers drifts; a judgement inside trepo is written
once and covered by unit tests.

So: no domain type describes a prompt or a picker, no caller reimplements a
guard, and when a caller needs something to draw a row, the fix is to add it to
the output rather than to let the caller run git. Running fzf and asking a
question are the caller's side of the line; deciding what may be removed is
trepo's.

## Command contract

- **stdout is for machines.** `get`, `path` and `add` print one path and
  nothing else; `list --json` prints one array. Progress, warnings, refusals
  and confirmations go to stderr. `p=$(trepo path api) && cd -- "$p"` holding
  is what makes a shell wrapper a wrapper rather than a parser.
- **Exit status carries the outcome**, because a wrapper branches on it before
  it looks at any text: `0` success, `1` nothing matched, `2` error, `3`
  nothing was decided. `130` is left to the caller, since that is what fzf
  exits with when a picker is dismissed and trepo runs no picker.
- **`3` is "trepo would not settle this", and it must stay apart from `1` and
  `2`.** Two things reach it: a query that named several checkouts where one
  was needed, and a run of `rm` that kept every target the guards want a
  decision on. They share a status because they leave the caller with the same
  next step — narrow it down, or say `--force` — and neither may collapse into
  "nothing called api exists" or "something went wrong", which lead the caller
  somewhere else entirely.
- **`list` never asks and never fails on emptiness.** It is a data source, so 0
  results is exit 0 with no output. `path` and `rm` are selections, so 0 results
  is exit 1.
- **A command that resolves to several candidates does not pick one.** It says
  how many matched and stops. Printing the first would hand back a path nobody
  asked for, and printing all of them would break the one-path contract; the
  candidates already have a home in `list`, which is what a caller builds a
  picker over.
- **Errors on stderr are one line, with no newline and no `()`.** They get
  embedded in other tools' UI, such as an fzf `change-header(...)` action, which
  breaks on both.
- **Commands do not interact.** A command that cannot decide reports what it
  needs and exits, rather than opening a prompt. stdin belongs to whatever
  invoked trepo: after fzf has run, or inside an fzf `transform`, there is
  nothing there to read, so a command that depends on an answer is a command
  that some callers cannot use at all.
- **A removal that needs a decision is kept, not asked about.** `rm` names the
  checkout and the reason on stderr and exits `3`; `--force` is the caller
  making that decision. There is no flag for "do not ask", because nothing
  asks, and no flag for "ask", because the answer would have nowhere to come
  from. Which reasons hold a removal back belongs to the guards, and stays the
  same however the run was invoked.

## Modes

Development, acquisition and review are separate entry points, not one screen
with everything on it.

They differ in what the candidates are and what happens after a choice:
development picks an existing checkout and moves there; acquisition picks a
repository that is not local yet and clones it; review picks an open PR or
issue and creates a worktree for it. Candidate sets have different costs, too —
local checkouts come from the filesystem, remote candidates need a network call
and a cache.

Merging them puts a slow, failure-prone list in front of the operation done
dozens of times a day, and makes "which of these can I even open right now"
ambiguous. Keeping them apart lets each one fail on its own terms: a broken
token affects acquisition and review, not the ability to reach a checkout.

The shared part is the shape of a row and of a preview, so a caller renders all
three the same way. That is a rendering contract, not a merged mode.

## Listing

**A repository and its worktrees appear in one list, and the main checkout is
marked rather than hidden.** The main checkout is a place work happens, so it
is a candidate like any other. `kind` is an attribute of a row, not a reason to
have a second command.

**The order is part of the spec:** by repository path, main checkout first
within a repository, then branch name (`checkout.Sort`). Enumeration runs
concurrently across repositories, so without a fixed order the output is
non-deterministic — tests cannot pin it and the picker's cursor lands somewhere
different each run.

**The repository row is not deduplicated away when it has worktrees.** It is
where the default branch is checked out; suppressing it would make the one
checkout that always exists the one that cannot be selected.

**A row carries repository, kind, branch, flags and path, in that order.** The
set is fixed by one requirement: a caller draws a row without running git. Each
column earns its place — the repository and branch name the checkout, kind
decides what selecting it does, flags decide how it is drawn, and the path is
what the caller acts on. The path stays last so that `${row##*<tab>}` keeps
working however many columns grow in front of it.

**kind is a column, not a flag.** Flags are the state a checkout is in,
computed per listing and read by the removal guards; kind is what the checkout
is. Putting it among the flags would add something the guards cannot act on to
the vocabulary they read. It also cannot be derived from the rest, which is why
it is printed rather than left implicit: a branch named `fix/main-nav` must not
be mistaken for the main checkout by a caller matching on text.

**Columns are values, not a rendering.** Widths belong to whatever draws the
row, since that is what knows the terminal and the font. A width fixed here is
stripped again by every caller that splits on tabs, and is overrun by the first
branch name longer than it, so it does not even buy the alignment it costs.
That leaves two forms — `--json` and tab-separated values — and no third,
pre-formatted one.

**A checkout's state is one vocabulary, computed once.** `checkout.Lister`
fills `Flags` (`dirty`, `merged`, `gone`, `locked`, `protected`, `current`, …),
and the listing, `status` and the removal guards all read that same value —
`checkout.Guard` runs no git of its own. A new condition is a flag, not a
second query issued from whichever command happens to need it, because two
places asking git the same question is how a listing and a guard come to
disagree about the checkout in front of them.

**"Which checkout holds this directory" is the `current` flag, and it is
answered for any directory.** A caller drawing checkouts often asks about a
directory it is not running in — a terminal multiplexer reports the working
directory of a pane, which is any directory below a checkout. So the directory
the flag is judged by is an input, and resolution goes upward to the innermost
checkout containing it rather than matching an exact path.

**That input is not offered to `rm`.** The same flag is what refuses to remove
the checkout the user is standing in, so a caller able to move it could delete
that checkout out from under them. A guard whose premise the caller supplies is
not a guard, which is why this one input is readable-only: the commands that
report take it, the command that destroys reads the real working directory.

## State

**The truth is the filesystem and git config.** trepo keeps no database of
checkouts. A checkout exists because git has a record of it; where it belongs
follows from its URL through the layout rules; how it is configured comes from
`trepo.*` in git config.

Paths are never persisted. A worktree is moved, renamed or deleted outside
trepo all the time, and a stored path would be a second source of truth that is
wrong exactly when it matters — during removal. This is also why removal starts
from the enumeration rather than from `git -C <path>`: a worktree whose
directory is gone cannot answer for itself, and that is precisely the case
worth reclaiming.

The same rule constrains anything else worth remembering about a checkout, such
as the fact that it came from a review: whatever holds it has to survive the
directory being moved and the branch being renamed, which rules out a side file
keyed by path.

Configuration scope follows from the same idea. `trepo.root`,
`trepo.worktreeRoot` and `trepo.defaultHost` decide which checkouts exist at
all, and are read from the global and system scopes only: a repository-local
override would make the same `trepo list` answer differently depending on the
directory it ran in. `trepo.worktreeTemplate` and `trepo.protected` describe
one repository's own layout and ownership, so a local override is meaningful
there.

Where git can be the record, it is: a merged branch is deleted with `git branch
-d` only. A branch git refuses to delete on its own terms is holding something
trepo will not discard, which means the safety rule lives in git rather than in
a reimplementation of reachability.

`Base` — the ref a branch is meant to merge into — carries `Known`. A
repository with no `origin` and no main branch has no answer, and that has to
be a value rather than a guess, because "not merged" and "cannot tell" lead to
different confirmations. Absence of the `merged` flag never means "unmerged" is
established.

## Alternatives not taken

**ghq plus a worktree tool.** They split the domain the wrong way: ghq places
and lists repositories, worktree tools list worktrees, and neither can answer
"where do I work next" without the caller joining the two lists and deciding
which flavour of entry it is holding. Composing them also means two layout
rules, two configuration systems and two notions of what is safe to delete.
Unifying repositories and worktrees into one list is the reason this tool
exists, and it cannot be added on top.

**One screen for all modes.** Rejected for the reasons in [Modes](#modes).

**Deduplicating the repository row when worktrees exist.** It reads as a
cleaner list, at the cost of dropping the checkout that holds the default
branch out of the candidates. See [Listing](#listing).

**A port to Rust, or staying in zsh.** trepo takes over from a pair of zsh
functions that could not be tested where it mattered: the flag computation and
the removal guards were the untested part, and they are the part that loses
work when it is wrong. Rust would buy nothing here — the work is process
orchestration around git, the standard library covers it with no dependencies,
and `go install` plus a tagged release is the whole distribution story.
