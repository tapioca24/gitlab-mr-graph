#!/bin/sh
set -eu

repository='tapioca24/gitlab-mr-graph'
install_dir="${GITLAB_MR_GRAPH_INSTALL_DIR:-${HOME:?}/.local/bin}"
release_version="${GITLAB_MR_GRAPH_VERSION:-}"

case "$(uname -s)" in
  Darwin) target_os='darwin' ;;
  Linux) target_os='linux' ;;
  *) echo 'macOS と Linux に対応しています。' >&2; exit 1 ;;
esac
case "$(uname -m)" in
  arm64|aarch64) target_arch='arm64' ;;
  x86_64|amd64) target_arch='amd64' ;;
  *) echo 'arm64 と amd64 に対応しています。' >&2; exit 1 ;;
esac
command -v curl >/dev/null 2>&1 || { echo 'curl が必要です。' >&2; exit 1; }
if [ -z "$release_version" ]; then
  release_url="$(curl -fsSL --proto '=https' --proto-redir '=https' --tlsv1.2 --connect-timeout 10 --max-time 60 \
    -o /dev/null -w '%{url_effective}' "https://github.com/$repository/releases/latest")"
  release_version="${release_url##*/}"
fi
if ! printf '%s\n' "$release_version" | awk '/^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/ { valid=1 } END { exit !valid }'; then
  echo '安定版のバージョンを確認できませんでした。' >&2
  exit 1
fi
asset_name="gitlab-mr-graph_${release_version}_${target_os}_${target_arch}"
download_base="https://github.com/$repository/releases/download/$release_version"
work_dir="$(mktemp -d "${TMPDIR:-/tmp}/gitlab-mr-graph.XXXXXX")"
staged_file=''
cleanup() { rm -rf "$work_dir"; if [ -n "$staged_file" ]; then rm -f "$staged_file"; fi; }
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
curl -fsSL --proto '=https' --proto-redir '=https' --tlsv1.2 --connect-timeout 10 --max-time 120 "$download_base/$asset_name" -o "$work_dir/$asset_name"
curl -fsSL --proto '=https' --proto-redir '=https' --tlsv1.2 --connect-timeout 10 --max-time 60 "$download_base/checksums.txt" -o "$work_dir/checksums.txt"
expected_hash="$(awk -v file="$asset_name" '$2==file || $2=="*"file { print $1; exit }' "$work_dir/checksums.txt")"
if command -v sha256sum >/dev/null 2>&1; then
  actual_hash="$(sha256sum "$work_dir/$asset_name" | awk '{print $1}')"
else
  actual_hash="$(shasum -a 256 "$work_dir/$asset_name" | awk '{print $1}')"
fi
if [ -z "$expected_hash" ] || [ "$actual_hash" != "$expected_hash" ]; then
  echo 'チェックサムが一致しません。既存のツールは変更していません。' >&2
  exit 1
fi
chmod 755 "$work_dir/$asset_name"
if [ "$("$work_dir/$asset_name" --version)" != "gitlab-mr-graph $release_version" ]; then
  echo '取得した実行ファイルのバージョンを確認できません。' >&2
  exit 1
fi
mkdir -p "$install_dir"
staged_file="$(mktemp "$install_dir/.gitlab-mr-graph.XXXXXX")"
cp "$work_dir/$asset_name" "$staged_file"
chmod 755 "$staged_file"
mv -f "$staged_file" "$install_dir/gitlab-mr-graph"
staged_file=''
printf '%s を %s にインストールしました。\n' "$release_version" "$install_dir"
printf 'PATH にインストール先を追加し、glab auth login で認証してから実行してください。\n'
