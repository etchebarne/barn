#!/usr/bin/env bash
# Builds release archives of openbotd (with the web app embedded) for Linux.
#
#   scripts/build-release.sh v0.1.0
#
# Output: release/openbot_<version>_linux_<arch>.tar.gz and release/checksums.txt
set -euo pipefail

version="${1:?usage: $0 <version, e.g. v0.1.0>}"
read -ra archs <<<"${OPENBOT_ARCHS:-amd64 arm64}"
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

echo "==> Building web app"
pnpm install --frozen-lockfile
pnpm --filter @openbot/web build

echo "==> Embedding web app"
find internal/webui/dist -mindepth 1 ! -name .gitkeep -delete
cp -R web/dist/. internal/webui/dist/
trap 'find internal/webui/dist -mindepth 1 ! -name .gitkeep -delete' EXIT

rm -rf release
mkdir -p release
for arch in "${archs[@]}"; do
  echo "==> Building openbotd ${version} linux/${arch}"
  stage="release/openbot_${version}_linux_${arch}"
  mkdir -p "$stage"
  CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath \
    -ldflags "-s -w -X main.version=${version}" -o "$stage/openbotd" ./cmd/openbotd
  cp README.md "$stage/"
  tar -C release -czf "${stage}.tar.gz" "$(basename "$stage")"
  rm -rf "$stage"
done

(cd release && sha256sum ./*.tar.gz | sed 's# \./# #' > checksums.txt)
echo "==> Done"
cat release/checksums.txt
