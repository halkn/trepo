# trepo

Repositories and their worktrees as one set of **checkouts**.

A checkout is a place work can happen: a repository's main checkout, or any of
its worktrees. With several agents working in parallel, one repository having
several checkouts is the normal state, so choosing where to work should not
start by deciding whether that place is a clone or a worktree.

`docs/design.md` records the decisions behind this and what they rule out.

## Install

```sh
go install github.com/halkn/trepo/cmd/trepo@latest
```

Requires git 2.36 or newer. `trepo remote` additionally needs `gh`; no other
command reaches the network. trepo runs no picker and asks no question; see
[Choosing between checkouts](#choosing-between-checkouts).

## Commands

```text
trepo get <repo>                     clone a repository into the trepo root
trepo list [<query>...]              list checkouts
trepo path [<query>...]              print the path of one checkout
trepo add <branch>                   create a worktree and print its path
trepo rm [<query>...]                remove worktrees, or reclaim finished ones
trepo status <path>                  describe one checkout
trepo remote [<query>...]            list repositories that could be cloned
trepo version                        print which build this is
```

`get`, `path` and `add` print a path on stdout and nothing else, so they
compose with the shell:

```sh
p=$(trepo path api) && cd -- "$p"
p=$(trepo add feat/login --repo api) && cd -- "$p"
```

Use two commands rather than `cd -- "$(trepo path api)"`: when nothing is
resolved stdout is empty, and `cd` with no argument goes home in bash and fails
in zsh.

`list` never fails on emptiness and exits 0 with no results, which makes it
usable as a data source. It prints one tab-separated row per checkout —
repository, kind, branch, flags, path — with the values as they are and no
padding, so widths are the caller's to choose. `--json` prints a single array
with the same information plus the host, owner and name split out.

```sh
$ trepo list | column -t -s $'\t'
halkn/api  repo      main    current,dirty  ~/repos/github.com/halkn/api
halkn/api  worktree  feat/x  merged         ~/worktrees/halkn/api/feat/x
```

`kind` is what tells a caller whether selecting a row should open that checkout
or make a new one for it, so the path stays last and columns may grow in front
of it: `${row##*$'\t'}` keeps working.

`path` answers with one checkout or with nothing. A query that matches several
exits `3` and says how many, rather than picking one:

```sh
$ trepo path alp
trepo: 2 checkouts match alp; narrow the query or choose one from trepo list
```

`list`, `path` and `rm` take `--here`, which narrows to the repository the
working directory belongs to — standing in one of its worktrees counts, since
what it narrows to is the repository rather than the checkout. Where it has
nothing to narrow to — outside any repository, or inside one that is not below
the trepo root — the command says so and exits `2` rather than answering with
nothing, since an empty answer would claim the repository holds no checkouts.

```sh
p=$(trepo path --here) && cd -- "$p"      # back to the repository's own checkout
trepo rm --here feat/login                # its worktrees only
```

`list` and `path` take `--cwd <dir>`, which judges "where you are" by `<dir>`
rather than by where trepo was run, and `path --current` answers with the one
checkout holding it. Any directory below a checkout works, so a caller can pass
the working directory of a terminal pane straight through:

```sh
trepo path --current --cwd "$pane_cwd"    # the checkout that directory is in
trepo status "$pane_cwd"                  # and a description of it
```

`rm` does not take `--cwd`. The working directory is what marks the checkout
`rm` refuses to remove, so a caller able to move that mark could delete the
checkout you are standing in.

Exit statuses: `0` success, `1` nothing matched, `2` an error, `3` nothing was
decided — several checkouts matched one query, or `rm` kept every target. `130`
is left free for the caller, since that is what `fzf` exits with when a picker
is dismissed.

## Choosing between checkouts

trepo resolves; choosing between what it resolved to, and moving there, is the
caller's. It runs no picker, so a wrapper builds one out of four pieces: `list`
for the candidates, `status <path>` for a description of one of them, `path
--current --cwd <dir>` for the checkout a directory is in, and `130` left
unused so the wrapper can pass a dismissed picker straight through.

None of those need the caller to run git.

## Layout

Repositories go under the trepo root by their URL, so the same repository
always lands in the same place no matter which spelling was cloned:

```text
~/repos/github.com/halkn/trepo
~/repos/dev.azure.com/org/proj/service
```

Worktrees go under their own root, named by a template. Branch slashes stay
slashes, so `feat/x` and `feat-x` are different directories:

```text
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
| `trepo.remoteOwner` | the authenticated account | global and system only, repeatable |

The three keys that decide which checkouts exist at all are read from the
global and system scopes only, so that `trepo list` answers the same in every
directory (`docs/design.md`).

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

## Development

```sh
mise install     # go and golangci-lint, at the versions in mise.toml
mise run test    # go test -race ./...
mise run lint    # gofmt check, go mod tidy -diff, golangci-lint
mise run fmt     # gofmt -w .
mise run audit   # govulncheck against the toolchain (needs network)
mise run dist    # release archives and checksums into dist/
```

Linting is golangci-lint's default set — errcheck, govet, ineffassign,
staticcheck, unused — with one exclusion documented in `.golangci.yml`. CI runs
the same mise tasks, so there is one definition of what passing means.

## Releasing

Push an annotated tag named `vX.Y.Z`. The tag message becomes the release note,
so write it for readers of the release page. CI then runs the tests and linters,
builds the archives with `mise run dist`, and publishes them with their
checksums.

```sh
git tag -a v0.2.0 -m "v0.2.0 - <what changed>"
git push origin v0.2.0
```

The archives carry signed build provenance, so a download can be traced back to
the workflow run and commit that produced it:

```sh
gh attestation verify trepo_v0.2.0_darwin_arm64.tar.gz --repo halkn/trepo
```

`v0.1.0` predates that workflow and was built by hand, so it has no attestation.

Versions are not written down in the source: `trepo version` reports what Go
stamped from the tag, so a build that is not made from a tagged, clean checkout
says so rather than claiming a release number.

## Acquiring repositories

`trepo remote` lists what could be cloned and marks what is already here. It is
the only command that reaches the network, and it does so through `gh`, which
already holds the token and knows the endpoint of an enterprise host:

```sh
$ trepo remote | column -t -s $'\t'
github.com  halkn/api      local   ~/repos/github.com/halkn/api
github.com  halkn/trepo    local   ~/repos/github.com/halkn/trepo
github.com  halkn/scratch  remote  -

$ trepo remote --missing            # only what is not here yet
$ p=$(trepo get halkn/scratch) && cd -- "$p"
```

One repository is one row however its URL is spelled: SSH and HTTPS, with or
without `.git`, fold onto the location it would occupy under the trepo root.

Answers are cached for an hour under `${XDG_CACHE_HOME:-~/.cache}/trepo`, so a
picker built on this stays responsive; `--refresh` asks again. The cache is
never consulted to decide what is on disk — that is always read from the
filesystem — so deleting it costs one network call and changes no answer.

When `gh` is missing, unauthenticated or offline, `trepo remote` says so and
exits `2` rather than reporting an empty account. Every other command keeps
working: a broken token stops acquisition, not the ability to reach a checkout.

By default the list is the authenticated account's own repositories. Add
organisations with `trepo.remoteOwner`:

```sh
git config --global --add trepo.remoteOwner some-org
```

## Removing worktrees

`trepo rm` takes one worktree, named by a query that matches exactly one —
a full path from a picker being the reliable spelling. It refuses outright to
remove the main checkout, the checkout you are standing in, and a locked
worktree.

It also holds back, rather than asking, when a worktree has uncommitted
changes, has ignored files such as a `.env`, has commits the base branch does
not already contain, or is protected. A detached worktree counts: with no
branch pointing at them, its commits become unreachable once it is gone. The
kept checkout and its reason go to stderr, and the run exits `3`:

```sh
$ trepo rm feat/login
trepo: skipped ~/worktrees/halkn/api/feat/login: it has uncommitted changes; rerun with --force to remove it anyway
```

`--force` is how the caller makes that decision. There is no flag to suppress
the question, because nothing asks: the behaviour is the same in a terminal, in
a script and inside an `fzf` key binding, where stdin belongs to something else.

`--dry-run` runs the guards and reports what would happen without removing
anything, so a refusal shows up as a refusal rather than as a removal that
would have gone ahead. It ends on the status the real run would.

A merged branch is deleted along with its worktree, with `git branch -d` only,
so a branch git refuses to delete stays.

## Reclaiming finished worktrees

`trepo rm --reclaimable` is the one form that acts on several at once: the flag
is the selection, so it takes every worktree whose job is done rather than
asking the query to name one.

```sh
trepo rm --reclaimable            # everywhere
trepo rm --reclaimable --here     # this repository only
```

A worktree is reclaimed when it is merged into the base, when the branch it
tracks was deleted on the remote, or when its directory is already gone and
only git's record of it is left. Anything holding work is left alone:
uncommitted changes, ignored files, a protected checkout, a detached one, and
commits that are on no remote. The checkout you are standing in, the main
checkout and a locked worktree are never candidates.

The branch retired on the remote is what a squash merge leaves behind — the
commits never become ancestors of the base, so nothing marks them merged.
Removing the worktree keeps the local branch either way, since only a merged
branch is deleted with its checkout, so the commits stay reachable.

A query still narrows what may be taken, and `--dry-run` reports it without
removing anything. Finding nothing to reclaim exits `1`, like any other command
that answers with a checkout. `--force` cannot be combined with it: the
selection already excludes everything `--force` would push past.
