# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

Git の repository と worktree を **checkout** という 1 つの概念にまとめる Go 製 CLI。並列開発で 1 repository に複数 checkout があるのを通常の状態として扱う。利用者向けの説明は `README.md`。

## Commands

```sh
mise run test                             # go test -race ./...
mise run lint                             # gofmt チェック + go mod tidy -diff + golangci-lint
mise run fmt                              # gofmt -w .
mise run build                            # ./trepo
go test ./internal/checkout -run TestGuard   # 単一テスト
go test ./internal/cli -run TestPath -v      # 出力付き
```

`mise run audit`（govulncheck）は CI 専任。ネットワークが要るので手元では通らないことがある。

## Core concept

`checkout.Checkout` が中心。repository の main checkout と worktree を 1 つのリストに畳み、`Kind` は属性でしかない。「repository を選ぶ」「worktree を選ぶ」を別操作にしないことがこの CLI の存在理由なので、種別ごとにコマンドを分ける方向の変更は設計に反する。

`Flags` は `checkout.Lister` が 1 箇所で計算し、一覧・`status`・削除ガードが全てそれを読む。判定を足すときは `Lister` に足す。削除ガード（`checkout.Guard`）の中で git を叩かない。

`Base`（統合先 ref）は `Known` を持つ。解決できない repository は普通にあるので、**`merged` が付いていないことを「未 merge が確定」と読まない**。確認メッセージもそれを反映する。

## Invariants（壊しやすい約束）

- **`get` / `path` / `add` の stdout は path 1 行だけ。** 進捗も警告も stderr。`p=$(trepo path api) && cd -- "$p"` が成立することが契約
- **終了コードに意味がある**: 0 成功 / 1 該当なし / 2 エラー / 130 選択キャンセル。shell wrapper が分岐するので混ぜない
- **path 比較は必ず `checkout.Resolve` / `SamePath` / `Under` を通す。** git は symlink 解決済みの path を返し shell は返さない（macOS の `/tmp` → `/private/tmp`）。素の文字列比較にすると、自分が立っている checkout を削除できてしまう
- **repository root は `git.RepoRoot`（worktree list の先頭エントリ）で求める。** `--git-common-dir` の親は bare・`--separate-git-dir`・submodule で別の場所を指す
- **stderr のエラーは 1 行、改行と `()` を含めない。** fzf の `change-header(...)` など他ツールの UI に埋め込まれても壊れないように
- **依存を足さない。** 標準ライブラリのみ。`go mod tidy -diff` が lint に入っているのはこの記録を守るため。ツールは mise、govulncheck は `go run pkg@version` で go.mod に入れない

## Conventions

- `internal/git` は git の実行と porcelain の parse だけを持ち、上位は型付き関数だけを見る。`worktree list` は `--porcelain -z` で読む（path や branch 名に改行が入りうるため）
- git config の scope を分けている。`trepo.root` / `worktreeRoot` / `defaultHost` は global と system だけを読む（cwd 次第で `trepo list` の答えが変わらないように）。`worktreeTemplate` / `protected` は repository の override を許す
- テストは `internal/gittest` で作る。開発者の global gitconfig（`commit.gpgsign`・hook・`init.defaultBranch`）を拾うと手元で通って CI で落ちる
- git の実挙動に依存する箇所は fixture への unit test ではなく実 repository への統合テストで pin する。mock は `internal/git.Fake` があるが、用途はコマンド組み立ての確認に限る
- linter は golangci-lint の既定セットのみ。除外は `.golangci.yml` の errcheck 1 件だけで、増やすときは理由をファイルに書く
- コードコメントは英語。「なぜ今この実装なのか」だけを書き、What と経緯は書かない
- commit: 小文字 conventional prefix（`fix:` `add:` `feat:` `refactor:` `chore:` `docs:`）+ 短い英語要約。1 コミット 1 テーマ
