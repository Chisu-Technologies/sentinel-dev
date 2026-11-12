#!/usr/bin/env bash
# Builds the archives of one release and their SHA256SUMS.
#
#   scripts/build-release.sh <version tag> <output directory>
#
# One archive per system, named without the version so that the address of the
# latest release never changes: sentinel_<os>_<arch>.tar.gz, and .zip for Windows.
# Each holds the binary beside LICENSE, NOTICE and THIRD_PARTY-NOTICES.md. The same
# commit gives the same bytes: paths are trimmed from the binary, and file times,
# owners and order in the archives are fixed.
# SENTINEL_RELEASE_TARGETS narrows the list, for example "linux/amd64 darwin/arm64".
set -euo pipefail

version=${1:?usage: build-release.sh <version tag> <output directory>}
out=${2:?usage: build-release.sh <version tag> <output directory>}
[[ $version =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || {
  echo "build-release: the version must be a tag such as v0.1.0" >&2
  exit 2
}

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
# The Go toolchain records in each binary whether the tree had changes that are not
# committed, a file that is not tracked included. A release is built from a commit
# and nothing else, so a tree with anything more is refused.
if [ -n "$(git -C "$root" status --porcelain)" ]; then
  echo "build-release: the working tree has changes that are not committed" >&2
  exit 2
fi
targets=${SENTINEL_RELEASE_TARGETS:-linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64}
# The time of the commit, not of the build: it is what makes two builds equal.
epoch=$(git -C "$root" log -1 --format=%ct)
notices=(LICENSE NOTICE THIRD_PARTY-NOTICES.md)

mkdir -p "$out"
out=$(cd "$out" && pwd)
if [ -n "$(ls -A "$out")" ]; then
  echo "build-release: $out is not empty" >&2
  exit 2
fi
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT

# Every binary is built before the first archive is written: the output directory
# may be inside the tree, and a file there would change what the next build records.
for target in $targets; do
  os=${target%/*}
  arch=${target#*/}
  dir=$stage/$os-$arch
  mkdir -p "$dir"
  binary=sentinel
  [ "$os" = windows ] && binary=sentinel.exe
  (cd "$root" && CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath \
    -ldflags "-s -w -X github.com/Chisu-Technologies/sentinel-dev/internal/buildinfo.version=$version" \
    -o "$dir/$binary" ./cmd/sentinel)
done

for target in $targets; do
  os=${target%/*}
  arch=${target#*/}
  dir=$stage/$os-$arch
  binary=sentinel
  [ "$os" = windows ] && binary=sentinel.exe
  for notice in "${notices[@]}"; do cp "$root/$notice" "$dir/$notice"; done
  chmod 755 "$dir/$binary"
  chmod 644 "${notices[@]/#/$dir/}"
  if [ "$os" = windows ]; then
    touch -d "@$epoch" "$dir"/*
    (cd "$dir" && TZ=UTC zip -X -q "$out/sentinel_${os}_${arch}.zip" "$binary" "${notices[@]}")
  else
    tar -C "$dir" --sort=name --owner=0 --group=0 --numeric-owner --mtime="@$epoch" \
      -cf - "$binary" "${notices[@]}" | gzip -n > "$out/sentinel_${os}_${arch}.tar.gz"
  fi
done

(cd "$out" && sha256sum sentinel_* > SHA256SUMS)
cat "$out/SHA256SUMS"
