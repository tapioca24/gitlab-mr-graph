# 開発ガイド

## 必要な環境

- Go 1.24 以上
- Node.js（JavaScript のテスト・構文チェック用）
- make、POSIX shell、Bash

実 GitLab に接続する場合は、認証済みの GitLab CLI (`glab`) も必要です。

## ビルドと動作確認

```sh
make build
./bin/gitlab-mr-graph --demo
```

実行ファイルは `bin/gitlab-mr-graph` に生成されます。デモはサンプルデータを使うため、GitLab の認証は不要です。

## テストと静的チェック

```sh
make test
make check
```

`make test` は race detector を有効にして Go テストを実行します。テストは固定した API 応答とローカル HTTP サーバーを使い、実 GitLab への接続は不要です。

JavaScript は Node.js 標準のテストランナーで絞り込みとグラフ配置を検証します。追加パッケージのインストールは不要です。

`make check` は `go vet`、JavaScript と shell の構文チェックを実行します。CI は macOS / Linux と Go 1.24 / 1.26 の組み合わせでテスト・チェック・ビルドを実行します。

## 配布ファイルのビルド

```sh
bash scripts/build-release.sh v0.1.0
```

`dist/` に macOS / Linux × arm64 / amd64 の実行ファイルと `checksums.txt` を生成します。バージョンは `vMAJOR.MINOR.PATCH` 形式で指定してください。配布ビルドにはこのバージョンを埋め込みます。`go install ...@バージョン` の場合は、Go のビルド情報からモジュールのバージョンを表示します。

## リリース

Codex で `$release` を呼び出すと、リポジトリ内の [release スキル](.agents/skills/release/SKILL.md) が前回リリースからの変更を調べ、次のバージョンを理由付きで提案します。検証後、ユーザーがバージョンと対象コミットを確認して確定すると、タグの push と公開確認を実行します。

対象は通常 `origin/main` の先端で、未コミット変更や未 push のコミットがある場合は停止します。

公開するコミットに `vMAJOR.MINOR.PATCH` 形式のタグを付け、GitHub に push します。

```sh
git tag v0.1.0
git push origin v0.1.0
```

Release workflow がテストと静的チェック、配布ファイルのビルドを実行し、実行ファイルとチェックサムを GitHub Release に公開します。

同じタグは `go install github.com/tapioca24/gitlab-mr-graph/cmd/gitlab-mr-graph@latest` の配布にも使われます。インストールスクリプトやツール本体の自動更新機能は提供しません。
