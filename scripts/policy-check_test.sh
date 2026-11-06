#!/usr/bin/env bash
# Tests for policy-check.sh: builds throwaway repositories and asserts the gate's exit
# code AND the reason it prints, so a gate that fails for the wrong reason (or always
# passes) is caught. POLICY_SCRIPT points at another copy of the script, to run these
# cases against an older version.
set -uo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
script=${POLICY_SCRIPT:-$here/policy-check.sh}
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
total=0
failed=0
big=$((6 * 1024 * 1024))

g() { git -C "$repo" -c user.name=test -c user.email=test@example.invalid -c commit.gpgsign=false "$@"; }

# A clean repository with one signed base commit, tagged `base`.
new_repo() {
  repo=$work/$1
  mkdir -p "$repo/scripts"
  git -C "$repo" init -q
  cp "$script" "$repo/scripts/policy-check.sh"
  echo MIT > "$repo/LICENSE"
  printf '* text=auto eol=lf\n\n*.png binary\n' > "$repo/.gitattributes"
  g add -A
  g commit -q -s -m base
  g tag base
}

commit() { g add -A && g commit -q -s -m "$1"; }

# run_gate push|pr  ->  sets out and code
run_gate() {
  if [ "$1" = pr ]; then
    out=$(cd "$repo" && env GITHUB_EVENT_NAME=pull_request BASE_SHA="$(git rev-parse base)" \
      HEAD_SHA="$(git rev-parse HEAD)" bash scripts/policy-check.sh 2>&1)
  else
    out=$(cd "$repo" && env -u GITHUB_EVENT_NAME -u BASE_SHA -u HEAD_SHA bash scripts/policy-check.sh 2>&1)
  fi
  code=$?
}

# expect <name> <exit code> <text the output must contain>
expect() {
  total=$((total + 1))
  if [ "$code" -eq "$2" ] && grep -qF -- "$3" <<< "$out"; then
    echo "ok   - $1"
  else
    failed=$((failed + 1))
    echo "FAIL - $1 (exit $code, wanted $2 with \"$3\")"
    sed 's/^/       | /' <<< "$out"
  fi
}

new_repo clean
run_gate push; expect "clean tree passes" 0 "policy-check: ok"

new_repo clean_pr
echo hello > "$repo/notes.txt"; commit "add notes"
run_gate pr; expect "clean signed pull request passes" 0 "policy-check: ok"

new_repo comment_only
printf '# we never use export-ignore or filter=lfs here\n' >> "$repo/.gitattributes"
commit "comment"
run_gate pr; expect "comment that only mentions export-ignore passes" 0 "policy-check: ok"

new_repo no_attrs
g rm -q .gitattributes; commit "drop attributes"
run_gate pr; expect "missing .gitattributes passes and says so" 0 "attribute check skipped"

new_repo big_tree
head -c "$big" /dev/zero > "$repo/big.bin"; commit "add big"
run_gate push; expect "6 MiB tracked file fails" 1 "big.bin is $big bytes"

new_repo big_history
head -c "$big" /dev/zero > "$repo/big.bin"; commit "add big"
g rm -q big.bin; commit "remove big"
run_gate push; expect "blob deleted again passes outside a pull request (tree only)" 0 "policy-check: ok"
run_gate pr; expect "6 MiB blob added and deleted inside the pull request fails" 1 "in the commits of this pull request"

new_repo nested_export
mkdir "$repo/sub"; printf '*.md export-ignore\n' > "$repo/sub/.gitattributes"; commit "nested"
run_gate pr; expect "export-ignore in a nested .gitattributes fails" 1 "sub/.gitattributes"

new_repo root_subst
printf '*.txt export-subst\n' >> "$repo/.gitattributes"; commit "subst"
run_gate pr; expect "export-subst in the root .gitattributes fails" 1 "export-subst"

new_repo lfs_filter
printf '*.bin filter=lfs diff=lfs merge=lfs -text\n' >> "$repo/.gitattributes"; commit "lfs"
run_gate pr; expect "filter= fails" 1 "export-ignore, export-subst or filter"

new_repo unsigned
echo hello > "$repo/notes.txt"; g add -A; g commit -q -m "no sign-off"
run_gate pr; expect "commit without Signed-off-by fails" 1 "has no Signed-off-by line"

new_repo no_license
g rm -q LICENSE; commit "drop license"
run_gate push; expect "missing LICENSE fails" 1 "LICENSE is missing"

new_repo exact_cap
head -c $((5 * 1024 * 1024)) /dev/zero > "$repo/sub.bin"; commit "exactly 5 MiB"
run_gate pr; expect "a blob of exactly 5 MiB passes" 0 "policy-check: ok"

new_repo big_nested
mkdir -p "$repo/assets/deep"; head -c "$big" /dev/zero > "$repo/assets/deep/big.bin"; commit "nested big"
run_gate pr; expect "6 MiB blob in a subdirectory fails" 1 "assets/deep/big.bin"

new_repo unsigned_merge
g checkout -q -b side base
echo side > "$repo/side.txt"; commit "side"
g checkout -q -
echo main > "$repo/main.txt"; commit "main"
g merge -q --no-ff -m "merge side" side
run_gate pr; expect "an unsigned merge commit between signed commits passes" 0 "policy-check: ok"

new_repo no_base
out=$(cd "$repo" && env -u BASE_SHA GITHUB_EVENT_NAME=pull_request HEAD_SHA="$(git rev-parse HEAD)"   bash scripts/policy-check.sh 2>&1); code=$?
expect "pull request without BASE_SHA fails" 1 "BASE_SHA is required"

# A git that fails must fail the gate, never print ok.
new_repo git_broken
out=$(cd "$repo" && env -u GITHUB_EVENT_NAME GIT_DIR="$work/does-not-exist" bash scripts/policy-check.sh 2>&1); code=$?
total=$((total + 1))
if [ "$code" -ne 0 ] && ! grep -qF "policy-check: ok" <<< "$out"; then
  echo "ok   - a failing git fails the gate"
else
  failed=$((failed + 1))
  echo "FAIL - a failing git fails the gate (exit $code)"
  sed 's/^/       | /' <<< "$out"
fi

echo "$((total - failed))/$total passed"
[ "$failed" -eq 0 ]
