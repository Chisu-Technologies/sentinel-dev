#!/usr/bin/env bash
# Repository policy gate. Runs in CI (policy.yml) and locally from any directory.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."
fail=0
# Every git listing is written to a file before it is read. Read straight from a
# process substitution, a failing git would hand the loops zero lines and the gate
# would print ok: set -e cannot see the exit code of <(...).
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
problem() { echo "FAIL: $*" >&2; fail=1; }

[ -f LICENSE ] || problem "LICENSE is missing"

# 5 MiB cap on every tracked file in the working tree. The history of a pull request
# is checked separately below, because a blob added and deleted again stays public.
max=$((5 * 1024 * 1024))
git ls-files -z > "$tmp/files"
while IFS= read -r -d '' f; do
  [ -f "$f" ] || continue
  size=$(wc -c < "$f")
  [ "$size" -le "$max" ] || problem "$f is $size bytes (limit $max)"
done < "$tmp/files"

# The Anthropic directory validator rejects archives that rely on these attributes.
# Every tracked .gitattributes counts, at any depth, and so does `filter=` (LFS), which
# the validator cannot resolve either. Comment lines are ignored; the first field of a
# line is the path pattern, so only the attributes after it are matched.
attr_files=0
git ls-files -z -- ':(glob)**/.gitattributes' > "$tmp/attrs"
while IFS= read -r -d '' a; do
  [ -f "$a" ] || continue
  attr_files=$((attr_files + 1))
  hits=$(tr -d '\r' < "$a" | awk '
    { sub(/^[ \t]+/, "") }
    /^#/ { next }
    { for (i = 2; i <= NF; i++)
        if ($i ~ /^[-!]?(export-ignore|export-subst|filter)(=|$)/) { print FNR ": " $0; next } }')
  if [ -n "$hits" ]; then
    echo "$a:" >&2
    echo "$hits" >&2
    problem "$a uses export-ignore, export-subst or filter"
  fi
done < "$tmp/attrs"
# Having none is allowed: there is nothing to forbid. Say so rather than pass silently.
[ "$attr_files" -gt 0 ] || echo "policy-check: no tracked .gitattributes, attribute check skipped"

# Pull request only, where the range is known: DCO sign-off and blobs new in the range.
# A push to main is not walked; the pull request that brought the commits was.
if [ "${GITHUB_EVENT_NAME:-}" = "pull_request" ]; then
  : "${BASE_SHA:?BASE_SHA is required on pull_request}"
  : "${HEAD_SHA:?HEAD_SHA is required on pull_request}"

  # Merge commits are exempt; every other commit needs a non-empty Signed-off-by line.
  # It is not matched against the author.
  commits=$(git rev-list --no-merges "$BASE_SHA..$HEAD_SHA")
  for c in $commits; do
    git log -1 --format=%B "$c" | grep -qE '^Signed-off-by: .+' \
      || problem "commit $c has no Signed-off-by line"
  done

  # A blob over the cap that any commit of the pull request introduced stays in public
  # history even if a later commit deletes it, so the final tree is not enough.
  objects=$(git rev-list --objects "$BASE_SHA..$HEAD_SHA")
  if [ -n "$objects" ]; then
    git cat-file --batch-check='%(objecttype) %(objectsize) %(rest)' <<< "$objects" > "$tmp/sizes"
    while read -r type size path; do
      [ "$type" = blob ] || continue
      [ "$size" -le "$max" ] || problem "blob ${path:-<unnamed>} is $size bytes (limit $max) in the commits of this pull request"
    done < "$tmp/sizes"
  fi
fi

if [ "$fail" -ne 0 ]; then exit 1; fi
echo "policy-check: ok"
