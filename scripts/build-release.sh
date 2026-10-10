#!/usr/bin/env bash
set -euo pipefail

release_version="${1:?使い方: scripts/build-release.sh v0.1.0}"
if [[ ! "$release_version" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
  echo 'バージョンは vMAJOR.MINOR.PATCH で指定してください。' >&2
  exit 1
fi
project_dir="$(cd "$(dirname "$0")/.." && pwd)"
cd "$project_dir"
mkdir -p dist
for target_os in darwin linux; do
  for target_arch in arm64 amd64; do
    asset_name="gitlab-mr-graph_${release_version}_${target_os}_${target_arch}"
    CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" go build -trimpath \
      -ldflags "-s -w -X main.version=${release_version}" -o "dist/$asset_name" ./cmd/gitlab-mr-graph
  done
done
cd dist
if command -v sha256sum >/dev/null 2>&1; then
  sha256sum "gitlab-mr-graph_${release_version}_"* > checksums.txt
else
  shasum -a 256 "gitlab-mr-graph_${release_version}_"* > checksums.txt
fi
printf '配布ファイルを %s/dist に生成しました。\n' "$project_dir"
