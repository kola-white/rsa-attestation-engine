#!/bin/bash
set -euo pipefail

# Usage: bash scripts/dev/scope.sh --repo ROOT --branch feat/NAME --base FULL_SHA [--require-clean]
# No policy file or environment variable can widen Phase 1's allowlist.
# Run a reviewed copy: editable shell code cannot authenticate itself.
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)/git-guard.sh"

repo= branch= base= cleanliness=allow-dirty
while [ "$#" -gt 0 ]; do
    case "$1" in
        --repo|--branch|--base)
            [ "$#" -ge 2 ] || { dev_fail "missing value for $1"; exit 1; }
            case "$1" in --repo) repo=$2 ;; --branch) branch=$2 ;; --base) base=$2 ;; esac
            shift 2 ;;
        --require-clean) cleanliness=require-clean; shift ;;
        *) dev_fail "unknown argument: $1"; exit 1 ;;
    esac
done
[ -n "$repo" ] && [ -n "$branch" ] && [ -n "$base" ] || {
    dev_fail 'required: --repo ROOT --branch feat/NAME --base FULL_SHA [--require-clean]'; exit 1;
}
dev_guard "$repo" "$branch" "$cleanliness" || exit 1
[[ "$base" =~ ^[0-9a-fA-F]{40}$ || "$base" =~ ^[0-9a-fA-F]{64}$ ]] || {
    dev_fail 'base must be a full commit SHA'; exit 1;
}
resolved=$(dev_git rev-parse --verify "$base^{commit}" 2>/dev/null) || { dev_fail 'base is not a commit'; exit 1; }
[ "$resolved" = "$base" ] || { dev_fail 'base must name the commit itself'; exit 1; }
dev_git merge-base --is-ancestor "$base" HEAD || { dev_fail 'base is not an ancestor of HEAD'; exit 1; }
dev_pass "explicit base commit: $base"

scratch=$(mktemp -d "${TMPDIR:-/tmp}/cvera-scope.XXXXXXXX")
trap 'rm -rf "$scratch"' EXIT

check_path() {
    local path=$1 ancestor
    case "$path" in
        scripts/dev/?*) ;;
        *) printf 'FAIL: outside Phase 1 scope: %q\n' "$path" >&2; return 1 ;;
    esac
    # Git paths are canonical; also check every existing ancestor for symlinks
    # and nested repositories, including untracked repositories Git collapses.
    ancestor=$path
    while [ -n "$ancestor" ]; do
        if [ -L "$DEV_ROOT/$ancestor" ]; then
            printf 'FAIL: symlink: %q\n' "$ancestor" >&2; return 1
        fi
        if [ -e "$DEV_ROOT/$ancestor/.git" ]; then
            printf 'FAIL: nested repository/submodule: %q\n' "$ancestor" >&2; return 1
        fi
        case "$ancestor" in */*) ancestor=${ancestor%/*} ;; *) break ;; esac
    done
}

check_mode() {
    case "$1" in
        120000|160000) dev_fail 'symlink or submodule in Git entries'; return 1 ;;
    esac
}

# Fail closed even for unchanged symlinks/gitlinks inherited from the base.
for tree in "$base" HEAD; do
    dev_git ls-tree -r -z "$tree" > "$scratch/tree"
    while IFS= read -r -d '' entry; do check_mode "${entry%% *}"; done < "$scratch/tree"
done
dev_git ls-files --stage -z > "$scratch/index"
while IFS= read -r -d '' entry; do check_mode "${entry%% *}"; done < "$scratch/index"
dev_pass 'base, HEAD, and index contain no symlinks or submodules'

check_diff() {
    local header path oldmode newmode rest
    dev_git diff --raw -z --no-abbrev --no-renames --no-ext-diff --no-textconv --ignore-submodules=none "$@" -- > "$scratch/diff" || {
        dev_fail 'cannot enumerate Git changes'; return 1;
    }
    # --raw -z emits a NUL-terminated ASCII header, then one NUL-terminated
    # literal path. Split only the header on spaces; never split/trim the path.
    # --no-renames prevents the two-path R/C format: a rename is D + A, so both
    # its original and destination paths are checked, even with tabs/newlines.
    while IFS= read -r -d '' header; do
        IFS= read -r -d '' path || { dev_fail 'incomplete Git diff record'; return 1; }
        IFS=' ' read -r oldmode newmode rest <<< "${header#:}"
        check_mode "$oldmode" && check_mode "$newmode" && check_path "$path" || return 1
    done < "$scratch/diff"
}

reject_ignored_additions() {
    local result=0 path
    # Only A entries are fed to check-ignore, never existing tracked M/D paths.
    # --no-index is necessary because newly staged files are already in the
    # index, where check-ignore would otherwise suppress matching results.
    # Existing tracked files remain subject to diff checks even if now ignored.
    # Newly introduced artifacts must not enter the proposed change set, even
    # through force-add inside scripts/dev/. No staging is performed here.
    dev_git diff --name-only -z --diff-filter=A --no-renames --no-ext-diff --no-textconv "$@" -- > "$scratch/additions" || {
        dev_fail 'cannot enumerate added paths'; return 1;
    }
    dev_git check-ignore --no-index --stdin -z < "$scratch/additions" > "$scratch/ignored" || result=$?
    case "$result" in
        0|1) ;; # check-ignore returns 1 when no paths match.
        *) dev_fail 'cannot evaluate ignore rules for added paths'; return 1 ;;
    esac
    if IFS= read -r -d '' path < "$scratch/ignored"; then
        printf 'FAIL: ignored artifact added to change set: %q\n' "$path" >&2; return 1
    fi
}

check_diff "$base" HEAD
reject_ignored_additions "$base" HEAD
dev_pass 'committed paths and modes'
check_diff --cached HEAD
reject_ignored_additions --cached HEAD
dev_pass 'staged paths and modes'
check_diff
dev_pass 'unstaged paths and modes'
# Ignore only untracked workspace artifacts; tracked diffs above are unfiltered.
dev_git ls-files --others --exclude-standard -z > "$scratch/untracked" || { dev_fail 'cannot enumerate untracked paths'; exit 1; }
while IFS= read -r -d '' path; do check_path "$path"; done < "$scratch/untracked"
dev_pass 'non-ignored untracked paths'
dev_pass 'Phase 1 scope: scripts/dev/ only; symlinks and submodules forbidden'
