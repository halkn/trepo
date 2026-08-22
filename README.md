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
trepo version                        print which build this is
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
choice was cancelled — the picker was dismissed, or every removal `rm`
offered was declined.

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
