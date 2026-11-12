#!/bin/sh
# Installs the sentinel command for the current user, on Linux and macOS.
#
#   curl -fsSL https://raw.githubusercontent.com/Chisu-Technologies/sentinel-dev/main/scripts/install.sh | sh
#
# It asks for no password, never runs sudo, and writes one file: the binary, in
# SENTINEL_INSTALL_DIR. The archive is checked against the SHA256SUMS of its
# release before anything is installed. On Windows, use install.ps1.
#
#   SENTINEL_VERSION            a release tag such as v0.1.0. Empty: the latest release.
#   SENTINEL_INSTALL_DIR        where the binary goes. Empty: $HOME/.local/bin.
#   SENTINEL_DOWNLOAD_BASE      where releases are read from: an https address that
#                               mirrors the release pages, or a directory that holds
#                               the files of one release. Empty: GitHub.
#   SENTINEL_REQUIRE_SIGNATURE  1: stop unless the signature of SHA256SUMS was checked.
set -eu

REPO="Chisu-Technologies/sentinel-dev"
# Who may have signed a release: the release workflow of this repository, run for a
# version tag. Nothing else.
SIGNER_PATTERN="^https://github\\.com/${REPO}/\\.github/workflows/release\\.yml@refs/tags/v[0-9]+\\.[0-9]+\\.[0-9]+\$"
SIGNER_ISSUER="https://token.actions.githubusercontent.com"

say() { printf '%s\n' "$*"; }
fail() {
  printf 'install: %s\n' "$*" >&2
  exit 1
}
have() { command -v "$1" >/dev/null 2>&1; }

# platform prints "<os> <arch>" as the release names them.
platform() {
  os=$(uname -s)
  arch=$(uname -m)
  case "$os" in
    Linux) os=linux ;;
    Darwin) os=darwin ;;
    *) fail "this script installs on Linux and macOS, not on $os. On Windows, use install.ps1." ;;
  esac
  case "$arch" in
    x86_64 | amd64) arch=amd64 ;;
    aarch64 | arm64) arch=arm64 ;;
    *) fail "there is no release for the $arch architecture." ;;
  esac
  say "$os $arch"
}

# fetch <file name> <destination>: one file of the release.
fetch() {
  case "$base" in
    https://*)
      if [ -n "$version" ]; then
        url="$base/download/$version/$1"
      else
        url="$base/latest/download/$1"
      fi
      if have curl; then
        curl --proto '=https' --tlsv1.2 -fsSL --retry 3 -o "$2" "$url"
      elif have wget; then
        wget --https-only -q -O "$2" "$url"
      else
        fail "curl or wget is needed to download the release."
      fi
      ;;
    *)
      cp "$base/$1" "$2"
      ;;
  esac
}

sha256_of() {
  if have sha256sum; then
    sha256sum "$1" | cut -d ' ' -f 1
  elif have shasum; then
    shasum -a 256 "$1" | cut -d ' ' -f 1
  else
    fail "sha256sum or shasum is needed to check the download."
  fi
}

# check_signature: verifies SHA256SUMS against the signature of the release, when
# cosign is here to do it. The checksum alone only says the download is the file the
# release lists; the signature says who built the release.
check_signature() {
  if ! have cosign; then
    if [ "${SENTINEL_REQUIRE_SIGNATURE:-}" = 1 ]; then
      fail "SENTINEL_REQUIRE_SIGNATURE is set and cosign is not installed. Nothing was installed."
    fi
    say "Signature not checked: cosign is not installed."
    return 0
  fi
  if ! fetch SHA256SUMS.sigstore.json "$tmp/SHA256SUMS.sigstore.json" 2>/dev/null; then
    fail "the release has no signature for SHA256SUMS."
  fi
  if ! cosign verify-blob \
    --bundle "$tmp/SHA256SUMS.sigstore.json" \
    --certificate-identity-regexp "$SIGNER_PATTERN" \
    --certificate-oidc-issuer "$SIGNER_ISSUER" \
    "$tmp/SHA256SUMS" >/dev/null 2>&1; then
    fail "the signature of SHA256SUMS is not valid. Nothing was installed."
  fi
  say "Signature checked: built by the release workflow of $REPO."
}

main() {
  version=${SENTINEL_VERSION:-}
  dir=${SENTINEL_INSTALL_DIR:-${HOME:?HOME is not set}/.local/bin}
  base=${SENTINEL_DOWNLOAD_BASE:-https://github.com/$REPO/releases}
  base=${base%/}
  case "$base" in
    https://*) ;;
    http://*) fail "SENTINEL_DOWNLOAD_BASE must use https." ;;
    *) [ -d "$base" ] || fail "SENTINEL_DOWNLOAD_BASE is not an https address or a directory." ;;
  esac
  # The version becomes part of an address, so it is a tag and nothing more.
  case "$version" in
    *[!A-Za-z0-9.+-]*) fail "SENTINEL_VERSION must be a tag such as v0.1.0." ;;
    "" | v[0-9]*.[0-9]*.[0-9]*) ;;
    *) fail "SENTINEL_VERSION must be a tag such as v0.1.0." ;;
  esac

  # Assigned first: the failure of a command substitution inside `set --` is lost.
  target=$(platform) || exit 1
  set -- $target
  asset="sentinel_$1_$2.tar.gz"

  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT INT TERM

  fetch SHA256SUMS "$tmp/SHA256SUMS" || fail "could not read SHA256SUMS of the release."
  fetch "$asset" "$tmp/$asset" || fail "could not download $asset."
  check_signature

  want=$(awk -v name="$asset" '$2 == name || $2 == "*" name { print $1 }' "$tmp/SHA256SUMS")
  [ -n "$want" ] || fail "SHA256SUMS does not list $asset."
  got=$(sha256_of "$tmp/$asset")
  [ "$want" = "$got" ] || fail "the checksum of $asset does not match SHA256SUMS. Nothing was installed."

  mkdir "$tmp/unpacked"
  tar -xzf "$tmp/$asset" -C "$tmp/unpacked" sentinel || fail "the archive has no sentinel binary."
  [ -f "$tmp/unpacked/sentinel" ] && [ ! -L "$tmp/unpacked/sentinel" ] ||
    fail "the archive has no sentinel binary."

  mkdir -p "$dir"
  # Written beside its destination and renamed, so an interrupted install leaves the
  # previous binary whole.
  cp "$tmp/unpacked/sentinel" "$dir/.sentinel.new.$$"
  chmod 755 "$dir/.sentinel.new.$$"
  mv -f "$dir/.sentinel.new.$$" "$dir/sentinel"

  say "Installed $("$dir/sentinel" version 2>/dev/null || say sentinel) in $dir"
  case ":${PATH:-}:" in
    *":$dir:"*) ;;
    *) say "$dir is not in your PATH. Add it, or run $dir/sentinel." ;;
  esac
}

# Everything above only defines functions: a download cut short runs nothing.
main "$@"
