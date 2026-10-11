# gitlab-mr-graph

GitLab の Merge Request を、ブランチの依存からグラフ化する CLI。GitLab CLI (`glab`) の認証を利用し、ローカルのブラウザで表示します。

対象のリポジトリの開いている MR 全件を表示し、選択した MR の上流・下流を可視化します。

## インストール

macOS / Linux の arm64 / amd64 に対応しています。利用時に Go や Node.js は不要です。

```sh
curl -fsSL https://raw.githubusercontent.com/tapioca24/gitlab-mr-graph/main/scripts/install.sh | sh
export PATH="$HOME/.local/bin:$PATH"
```

GitHub Releases の配布ファイルを取得し、SHA-256 を確認して `~/.local/bin/gitlab-mr-graph` に配置します。配置先やバージョンを指定する場合:

```sh
curl -fsSL https://raw.githubusercontent.com/tapioca24/gitlab-mr-graph/main/scripts/install.sh | \
  GITLAB_MR_GRAPH_INSTALL_DIR="$HOME/bin" GITLAB_MR_GRAPH_VERSION=v0.1.0 sh
```

## すぐに試す

認証不要のサンプルデータで起動できます。

```sh
gitlab-mr-graph --demo
```

100 MR を表示する場合:

```sh
gitlab-mr-graph --demo --demo-count 100
```

## GitLab で使う

事前に [GitLab CLI](https://docs.gitlab.com/cli/) をインストールし、対象ホストへのログインを済ませてください。

GitLab のリポジトリ内で実行します。

```sh
gitlab-mr-graph
```

ブラウザが開きます。Ctrl-C で終了します。

```sh
# ブラウザを自動で開かない
gitlab-mr-graph --no-open
# ポートを指定
gitlab-mr-graph --port 8080
# 別の場所から対象を指定
gitlab-mr-graph --repo group/project --hostname gitlab.example.com
gitlab-mr-graph --repo https://gitlab.example.com/group/project
```

MR 情報は60秒間隔で更新します。タブが非表示の間は自動取得を止めます。

## ツールの更新

起動時に最新版をバックグラウンドで取得し、次回起動から使用します。デモと開発ビルドは自動更新しません。

```sh
# 自動更新を無効にして起動
gitlab-mr-graph --no-update
# 明示的に更新
gitlab-mr-graph update
```

## 開発

ビルド・テスト・リリースの手順は [CONTRIBUTING.md](CONTRIBUTING.md) を参照してください。

## 謝辞

- [gh-pr-graph](https://github.com/orangain/gh-pr-graph)
- [Catppuccin](https://github.com/catppuccin/catppuccin)
- [Lucide](https://lucide.dev/): SVG 0.468.0 とライセンスをローカルに同梱

## キーボードショートカット

| キー                  | 操作                                                                   | 条件                                                                      |
| --------------------- | ---------------------------------------------------------------------- | ------------------------------------------------------------------------- |
| `/`                   | Query にフォーカス                                                     |                                                                           |
| `A`                   | Authored by me の切り替え                                              |                                                                           |
| `S`                   | Assigned to me の切り替え                                              |                                                                           |
| `B`                   | Hide bots の切り替え                                                   |                                                                           |
| `U`                   | 自動更新の切り替え                                                     |                                                                           |
| `R`                   | 更新                                                                   |                                                                           |
| `T`                   | テーマの切り替え                                                       |                                                                           |
| `F` / `0`             | グラフ全体を表示                                                       |                                                                           |
| `+` / `=`             | 拡大                                                                   |                                                                           |
| `-`                   | 縮小                                                                   |                                                                           |
| `H` / `←`             | 直接の上流 MR へ移動                                                   | MR 選択中。表示中の候補から高さが近い MR を選び、移動先がなければそのまま |
| `L` / `→`             | 直接の下流 MR へ移動                                                   | MR 選択中。表示中の候補から高さが近い MR を選び、移動先がなければそのまま |
| `K` / `↑`             | 同じ直属の上流を共有する、上の MR へ移動                               | MR 選択中。表示中の MR が対象で、移動先がなければそのまま                 |
| `J` / `↓`             | 同じ直属の上流を共有する、下の MR へ移動                               | MR 選択中。表示中の MR が対象で、移動先がなければそのまま                 |
| `↑` / `↓` / `←` / `→` | グラフを移動                                                           | MR 未選択でグラフにフォーカス中                                           |
| `Esc`                 | 詳細・ヘルプを閉じる・文字入力からグラフに戻る                         |                                                                           |
| `?`                   | 全ショートカットの一覧を表示（ヘッダーのキーボードボタンでも表示可能） |                                                                           |
