#!/usr/bin/env bash
# Tests for install.sh. Each case installs from a release built in a temporary
# directory and asserts the exit code, what was printed and what is on disk, so an
# installer that accepts a bad download (or installs nothing and says it did) is
# caught. `uname`, `cosign` and `gh` are replaced by small programs first in PATH: the
# script under test has no test switches of its own.
# INSTALL_SCRIPT points at another copy of the script, to run these cases against it.
set -uo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
script=${INSTALL_SCRIPT:-$here/install.sh}
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
total=0
failed=0

# A real cosign or gh on this machine would answer instead of "not installed": the
# cases decide by themselves whether one is there, so their directories are left out.
clean_path=
IFS=: read -r -a path_dirs <<< "$PATH"
for d in "${path_dirs[@]}"; do
  [ -x "$d/cosign" ] || [ -x "$d/cosign.exe" ] || [ -x "$d/gh" ] || [ -x "$d/gh.exe" ] ||
    clean_path=${clean_path:+$clean_path:}$d
done
PATH=$clean_path

sum() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$@"; else shasum -a 256 "$@"; fi
}

# new_release <dir> [binary text]: one release for linux/amd64 and darwin/arm64.
new_release() {
  rel=$1
  mkdir -p "$rel/build"
  printf '#!/bin/sh\necho "sentinel %s"\n' "${2:-v0.1.0 (test)}" > "$rel/build/sentinel"
  chmod 755 "$rel/build/sentinel"
  for target in linux_amd64 darwin_arm64; do
    tar -czf "$rel/sentinel_$target.tar.gz" -C "$rel/build" sentinel
  done
  (cd "$rel" && sum sentinel_*.tar.gz > SHA256SUMS)
  printf '{}\n' > "$rel/SHA256SUMS.sigstore.json"
  rm -rf "$rel/build"
}

# shims <os> <arch> [cosign exit status] [gh auth status exit] [gh verify exit]:
# programs that answer for the machine. An empty status leaves that program out.
shims() {
  bin=$work/bin.$total
  mkdir -p "$bin"
  printf '#!/bin/sh\ncase "$1" in -s) echo %s ;; -m) echo %s ;; esac\n' "$1" "$2" > "$bin/uname"
  chmod 755 "$bin/uname"
  if [ -n "${3:-}" ]; then
    printf '#!/bin/sh\necho "$@" >> "%s/cosign.calls"\nexit %s\n' "$bin" "$3" > "$bin/cosign"
    chmod 755 "$bin/cosign"
  fi
  if [ -n "${4:-}" ]; then
    printf '#!/bin/sh\necho "$@" >> "%s/gh.calls"\n[ "$1" = auth ] && exit %s\nexit %s\n' "$bin" "$4" "${5:-0}" > "$bin/gh"
    chmod 755 "$bin/gh"
  fi
}

# install <release dir> [VAR=value ...]  ->  sets out and code
install() {
  rel=$1
  shift
  home=$work/home.$total
  mkdir -p "$home"
  out=$(env HOME="$home" PATH="$bin:$PATH" SHELL=/bin/sh SENTINEL_DOWNLOAD_BASE="$rel" "$@" sh "$script" 2>&1)
  code=$?
}

check() {
  total=$((total + 1))
  if eval "$2"; then
    printf 'ok   %s\n' "$1"
  else
    failed=$((failed + 1))
    printf 'FAIL %s\n     exit %s\n     %s\n' "$1" "$code" "$(printf '%s' "$out" | head -5)"
  fi
}

installed() { [ -x "$home/.local/bin/sentinel" ]; }
nothing_installed() { [ ! -e "$home/.local/bin/sentinel" ] && ! ls "$home/.local/bin"/.sentinel.new.* >/dev/null 2>&1; }

new_release "$work/rel"

shims Linux x86_64
install "$work/rel"
check "installs the binary of this machine and says where" \
  '[ $code -eq 0 ] && installed && [[ $out == *"Installed sentinel v0.1.0 (test) in $home/.local/bin"* ]]'
check "says the signature was not checked when neither gh nor cosign is here" \
  '[[ $out == *"Signature not checked: sign in with gh auth login"* ]]'
check "puts the directory on the PATH of new terminals, and says how to use it now" \
  '[ "$(grep -cF "export PATH=\"$home/.local/bin:\$PATH\"" "$home/.profile")" = 1 ] &&
   [[ $out == *"Added $home/.local/bin to your PATH in $home/.profile."* ]] &&
   [[ $out == *"Open a new terminal to use sentinel"* ]]'

# The startup file follows the shell, and a second install adds nothing.
install "$work/rel" SHELL=/usr/bin/zsh
zsh_home=$home
check "writes the PATH line to .zshrc for zsh" '[ -f "$home/.zshrc" ] && [ ! -e "$home/.profile" ]'
out=$(env HOME="$zsh_home" PATH="$bin:$PATH" SHELL=/usr/bin/zsh SENTINEL_DOWNLOAD_BASE="$work/rel" sh "$script" 2>&1)
check "a second install does not add the line again" \
  '[ "$(grep -c "Added by the sentinel installer" "$zsh_home/.zshrc")" = 1 ]'
install "$work/rel" SHELL=/bin/bash
check "writes the PATH line to .bashrc for bash on Linux" '[ -f "$home/.bashrc" ] && [ ! -e "$home/.bash_profile" ]'
install "$work/rel" SENTINEL_NO_MODIFY_PATH=1
check "with SENTINEL_NO_MODIFY_PATH=1, touches no startup file and says so" \
  '[ ! -e "$home/.profile" ] && [[ $out == *"is not in your PATH. Add it"* ]]'
shims Darwin arm64
install "$work/rel" SHELL=/bin/bash
check "writes the PATH line to .bash_profile for bash on macOS" '[ -f "$home/.bash_profile" ] && [ ! -e "$home/.bashrc" ]'

shims Darwin arm64
install "$work/rel" SENTINEL_INSTALL_DIR="$work/custom dir/bin"
check "installs on macOS arm64 into the directory asked for, spaces included" \
  '[ $code -eq 0 ] && [ -x "$work/custom dir/bin/sentinel" ] && [ ! -e "$home/.local/bin/sentinel" ]'

shims Linux aarch64
install "$work/rel"
check "refuses a machine the release has no archive for, and installs nothing" \
  '[ $code -ne 0 ] && nothing_installed && [[ $out == *"could not download sentinel_linux_arm64.tar.gz"* ]]'

shims Linux riscv64
install "$work/rel"
check "refuses an architecture it does not know" \
  '[ $code -ne 0 ] && nothing_installed && [[ $out == *"no release for the riscv64 architecture"* ]]'

shims MINGW64_NT-10.0 x86_64
install "$work/rel"
check "sends Windows to install.ps1" \
  '[ $code -ne 0 ] && nothing_installed && [[ $out == *"install.ps1"* ]]'

# A download that is not the file the release lists.
new_release "$work/tampered"
printf 'x' >> "$work/tampered/sentinel_linux_amd64.tar.gz"
shims Linux x86_64
install "$work/tampered"
check "refuses an archive whose checksum does not match, and installs nothing" \
  '[ $code -ne 0 ] && nothing_installed && [[ $out == *"does not match SHA256SUMS. Nothing was installed."* ]]'

new_release "$work/unlisted"
grep -v linux_amd64 "$work/unlisted/SHA256SUMS" > "$work/unlisted/SHA256SUMS.new"
mv "$work/unlisted/SHA256SUMS.new" "$work/unlisted/SHA256SUMS"
shims Linux x86_64
install "$work/unlisted"
check "refuses an archive that SHA256SUMS does not list" \
  '[ $code -ne 0 ] && nothing_installed && [[ $out == *"SHA256SUMS does not list sentinel_linux_amd64.tar.gz"* ]]'

# An archive with the right checksum and no binary named sentinel in it.
mkdir -p "$work/empty/build"
echo hello > "$work/empty/build/README"
tar -czf "$work/empty/sentinel_linux_amd64.tar.gz" -C "$work/empty/build" README
(cd "$work/empty" && sum sentinel_linux_amd64.tar.gz > SHA256SUMS)
shims Linux x86_64
install "$work/empty"
check "refuses an archive without the binary" \
  '[ $code -ne 0 ] && nothing_installed && [[ $out == *"the archive has no sentinel binary"* ]]'

# Signature: checked when cosign is here, and its refusal stops the install.
shims Linux x86_64 0
install "$work/rel"
check "checks the signature of SHA256SUMS when cosign is installed" \
  '[ $code -eq 0 ] && installed && [[ $out == *"Signature checked"* ]] &&
   grep -q -- "verify-blob --bundle .*SHA256SUMS.sigstore.json --certificate-identity-regexp .*release.*yml@refs/tags/v.* --certificate-oidc-issuer https://token.actions.githubusercontent.com .*SHA256SUMS" "$bin/cosign.calls"'

shims Linux x86_64 1
install "$work/rel"
check "refuses a release whose signature cosign rejects, and installs nothing" \
  '[ $code -ne 0 ] && nothing_installed && [[ $out == *"the signature of SHA256SUMS is not valid. Nothing was installed."* ]]'

new_release "$work/unsigned"
rm "$work/unsigned/SHA256SUMS.sigstore.json"
shims Linux x86_64 0
install "$work/unsigned"
check "refuses a release with no signature when cosign is installed" \
  '[ $code -ne 0 ] && nothing_installed && [[ $out == *"the release has no signature for SHA256SUMS"* ]]'

shims Linux x86_64
install "$work/rel" SENTINEL_REQUIRE_SIGNATURE=1
check "with SENTINEL_REQUIRE_SIGNATURE=1, refuses to install with nothing to check it" \
  '[ $code -ne 0 ] && nothing_installed && [[ $out == *"no signed-in gh or cosign to check it. Nothing was installed."* ]]'

# GitHub's attestation of the archive: checked by a signed-in gh, preferred to cosign.
shims Linux x86_64 "" 0 0
install "$work/rel"
check "checks the attestation of the archive with a signed-in gh" \
  '[ $code -eq 0 ] && installed && [[ $out == *"Signature checked with GitHub"* ]] &&
   grep -q -- "attestation verify .*sentinel_linux_amd64.tar.gz --repo Chisu-Technologies/sentinel-dev --cert-identity-regex .*release.*yml@refs/tags/v.* --cert-oidc-issuer https://token.actions.githubusercontent.com --deny-self-hosted-runners" "$bin/gh.calls"'

shims Linux x86_64 "" 0 1
install "$work/rel"
check "refuses an archive GitHub has no valid attestation for, and installs nothing" \
  '[ $code -ne 0 ] && nothing_installed && [[ $out == *"GitHub has no valid attestation"*"Nothing was installed."* ]]'

shims Linux x86_64 "" 1 0
install "$work/rel"
check "a gh that is not signed in is not asked, and the install goes on" \
  '[ $code -eq 0 ] && installed && [[ $out == *"Signature not checked"* ]] && ! grep -q attestation "$bin/gh.calls" 2>/dev/null'

shims Linux x86_64 1 0 0
install "$work/rel"
check "a signed-in gh is preferred to cosign" '[ $code -eq 0 ] && installed && [ ! -e "$bin/cosign.calls" ]'

shims Linux x86_64 0 1 0
install "$work/rel"
check "without a gh session, cosign checks the signature" \
  '[ $code -eq 0 ] && installed && [[ $out == *"Signature checked: built by"* ]] && [ -e "$bin/cosign.calls" ]'

shims Linux x86_64 "" 0 0
install "$work/rel" SENTINEL_REQUIRE_SIGNATURE=1
check "with SENTINEL_REQUIRE_SIGNATURE=1, a signed-in gh is enough" '[ $code -eq 0 ] && installed'

# What the variables accept.
shims Linux x86_64
install "http://example.invalid/releases"
check "refuses a download address that is not https" \
  '[ $code -ne 0 ] && nothing_installed && [[ $out == *"SENTINEL_DOWNLOAD_BASE must use https."* ]]'

for bad in "latest" "1.2.3" "v1.2.3/../../x" "v1.2.3 x" 'v1.2.3;x'; do
  install "$work/rel" SENTINEL_VERSION="$bad"
  check "refuses the version '$bad', which is not a tag" \
    '[ $code -ne 0 ] && nothing_installed && [[ $out == *"SENTINEL_VERSION must be a tag such as v0.1.0."* ]]'
done

# Installing again replaces the binary and leaves nothing else behind.
shims Linux x86_64
install "$work/rel"
first_home=$home
new_release "$work/rel2" "v0.2.0 (test)"
out=$(env HOME="$first_home" PATH="$bin:$PATH" SENTINEL_DOWNLOAD_BASE="$work/rel2" sh "$script" 2>&1)
code=$?
home=$first_home
check "installing again replaces the binary and leaves one file" \
  '[ $code -eq 0 ] && [[ $out == *"Installed sentinel v0.2.0 (test)"* ]] && [ "$(ls -A "$home/.local/bin")" = sentinel ]'

printf '\n%s of %s checks failed\n' "$failed" "$total"
[ "$failed" -eq 0 ]
