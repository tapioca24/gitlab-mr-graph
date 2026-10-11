# Repository Guidelines

## プロジェクト構成

GitLab の MR をブランチ依存のグラフとして表示する Go 製 CLI です。

- `cmd/gitlab-mr-graph/`: CLI の起動、引数処理、バージョン表示。
- `internal/gitlab/`: 認証済み `glab` を通した API 呼び出し。
- `internal/graph/`、`internal/model/`: 依存グラフの構築と共有データ型。
- `internal/server/`: ローカル HTTP サーバー。`web/` の JavaScript、CSS、HTML、SVG は実行ファイルに埋め込まれます。
- `internal/demo/`: 認証不要のサンプルデータ。
- `scripts/`、`.github/workflows/`: 配布ビルドと CI。テストは対象コードの近くに配置します。

## ビルド・テスト・開発コマンド

Go 1.24 以上、Node.js、make、POSIX shell、Bash を用意してください。追加の JavaScript パッケージは不要です。

- `make build`: `bin/gitlab-mr-graph` を生成します。
- `make demo`: ビルド後、サンプルデータでブラウザ表示を起動します。
- `./bin/gitlab-mr-graph --demo --no-open --port 8080`: ブラウザを自動起動せず動作確認します。
- `make test`: race detector 付き Go テストと Node.js テストを実行します。
- `make check`: `go vet`、JavaScript と Bash の構文チェックを実行します。
- `bash scripts/build-release.sh v0.1.0`: macOS / Linux の arm64 / amd64 向け配布物を `dist/` に生成します。

詳細は `CONTRIBUTING.md` を参照してください。

## コーディング規約

Go は `gofmt` で整形し、タブ字下げと標準的な命名に従ってください。公開識別子は PascalCase、非公開識別子は camelCase、パッケージ名は小文字にします。JavaScript と CSS は既存の 2 スペース字下げに合わせ、JavaScript の文字列にはダブルクォートを使ってください。専用の formatter / linter 設定はありません。

API 取得、グラフ構築、HTTP 配信、UI の責務を既存のパッケージ境界に沿って配置してください。

## テスト方針

Go 標準の `testing` を使い、`*_test.go` に `Test機能名` 形式で追加します。JavaScript は `node:test` と `node:assert/strict` を使い、`internal/server/graph_view.test.cjs` で検証します。

変更した挙動と不具合の再発を検証してください。API テストには固定応答やローカル HTTP サーバーを使い、実 GitLab の認証に依存させないでください。数値のカバレッジ閾値は設定されていません。提出前に `make test check build` を実行してください。

## コミット・Pull Request

履歴では `feat(web): …`、`fix(web): …`、`docs: …` などの Conventional Commits を使用しています。コミットの説明は日本語で、変更の目的を簡潔に記述してください。

PR には目的、主な変更、実行した検証と結果を記載し、関連 issue があればリンクしてください。UI 変更にはスクリーンショットを添付し、CLI の挙動変更には実行例を示してください。CI は macOS / Linux と Go 1.24 / 1.26 で検証します。

## 認証・安全性

GitLab 認証は `glab` に委ね、トークンをコードやログに含めないでください。ローカル API のセッション、Host、Origin 検証を維持し、同梱 SVG のライセンスも保持してください。

## エージェント向け指示

応答とコミットの説明は日本語にしてください。検証の失敗や未確認事項は隠さず報告してください。Node.js のパッケージ操作が必要になった場合は `npm` / `npx` より `pnpm` / `pnpm dlx` を優先してください。

## 説明・ドキュメントの方針

ユーザーは長文を読みたくありません。一般的な説明は省き、このリポジトリ固有で、ユーザーが理解する必要のあることだけを簡潔に記載してください。事実を網羅するのではなく、あえてユーザーに提示する価値があるかを十分に吟味し、必要な情報だけを残してください。README や UI の説明にもこの方針を適用します。

必要な一覧情報は箇条書きや表にまとめ、関連する説明は同じ箇所に集約してください。形式を整える前に、情報自体を提示する必要があるかを判断してください。

## 作業用ファイルの扱い

プランやタスクリストを作成する場合は Markdown ファイルに保存し、`mo ファイル名.md` で開いてください。実装過程だけで必要な計画・設計・作業記録は、作業完了時に削除し、最終成果物やコミットに含めないでください。
