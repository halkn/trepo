# trepo

Repositories and their worktrees as one set of **checkouts**.

A checkout is a place work can happen: a repository's main checkout, or any of
its worktrees. With several agents working in parallel, one repository having
several checkouts is the normal state, so choosing where to work should not
start by deciding whether that place is a clone or a worktree.

## Install

```sh
go install github.com/halkn/trepo/cmd/trepo@latest
```

Requires git 2.36 or newer. `fzf` is optional: it is used for choosing between
several candidates, and everything else works without it.

## Commands

```
trepo get <repo>                     clone a repository into the trepo root
trepo list [<query>...]              list checkouts
trepo path [<query>...]              print the path of one checkout
trepo add <branch>                   create a worktree and print its path
trepo rm [<query>...]                remove worktrees
trepo status <path>                  describe one checkout
```

`get`, `path` and `add` print a path on stdout and nothing else, so they
compose with the shell:

```sh
p=$(trepo path api) && cd -- "$p"
p=$(trepo add feat/login --repo api) && cd -- "$p"
```

Use two commands rather than `cd -- "$(trepo path api)"`: when a selection is
cancelled stdout is empty, and `cd` with no argument goes home in bash and
fails in zsh.

`list` never asks a question and exits 0 even with no results, which makes it
usable as a data source; `--json` prints a single array.

Exit statuses: `0` success, `1` nothing matched, `2` an error, `130` the
selection was cancelled.

## Layout

Repositories go under the trepo root by their URL, so the same repository
always lands in the same place no matter which spelling was cloned:

```
~/repos/github.com/halkn/trepo
~/repos/dev.azure.com/org/proj/service
```

Worktrees go under their own root, named by a template. Branch slashes stay
slashes, so `feat/x` and `feat-x` are different directories:

```
~/.local/share/trepo/worktrees/halkn/trepo/feat/x
```

## Configuration

Settings live in git config, under `trepo.*`.

| key | default | scope |
| --- | --- | --- |
| `trepo.root` | `~/repos` | global and system only |
| `trepo.worktreeRoot` | `${XDG_DATA_HOME:-~/.local/share}/trepo/worktrees` | global and system only |
| `trepo.defaultHost` | `github.com` | global and system only |
| `trepo.worktreeTemplate` | `{{.Owner}}/{{.Repo}}/{{.Branch}}` | any scope |
| `trepo.protected` | none | any scope, repeatable |

The three keys that decide which checkouts exist at all are read from the
global and system scopes only. A repository-local override would make the same
`trepo list` answer differently depending on the directory it ran in.

```sh
git config --global trepo.root ~/src
git config --global --add trepo.protected .claude/worktrees
```

`trepo.protected` marks checkouts another tool owns. A value is a run of path
segments, matched against consecutive segments of the checkout's path — not a
glob. A protected checkout is listed and can still be removed, but never
without confirmation.

Template variables: `{{.Host}}`, `{{.Owner}}`, `{{.Repo}}`, `{{.Branch}}`. The
default leaves the host out, so add `{{.Host}}` if you keep repositories with
the same owner and name on more than one host.

## Removing worktrees

`trepo rm` refuses outright to remove the main checkout, the checkout you are
standing in, and a locked worktree. It asks first when a worktree has
uncommitted changes, has ignored files such as a `.env`, has commits the base
branch does not already contain, or is protected. A detached worktree counts:
with no branch pointing at them, its commits become unreachable once it is
gone.

`--force` skips the asking. `--dry-run` runs the guards and reports what would
happen without removing anything or asking anything, so a refusal shows up as a
refusal rather than as a removal that would have gone ahead.

A merged branch is deleted along with its worktree, with `git branch -d` only —
a branch git will not delete on its own terms is holding something trepo will
not discard.
