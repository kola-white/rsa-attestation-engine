#!/bin/bash
# Source this library, then call dev_guard REPO EXPECTED_BRANCH [require-clean].
# Callers must check its return status. No Git writes or network access occur.

dev_fail() { printf 'FAIL: %s\n' "$*" >&2; return 1; }
dev_pass() { printf 'PASS: %s\n' "$*"; }
dev_git() { command git --no-optional-locks -c core.fsmonitor=false -c core.untrackedCache=false -C "$DEV_ROOT" "$@"; }

dev_guard() {
    local expected_root=$1 expected_branch=$2 cleanliness=${3:-allow-dirty}
    local name value root branch origins operation path flags
    for name in GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY \
        GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_COMMON_DIR GIT_NAMESPACE \
        GIT_CONFIG GIT_CONFIG_COUNT GIT_CONFIG_PARAMETERS GIT_SHALLOW_FILE \
        GIT_REPLACE_REF_BASE; do
        eval 'value=${'"$name"'+set}'
        [ -z "$value" ] || { dev_fail "Git environment override: $name"; return 1; }
    done
    DEV_ROOT=$(cd "$expected_root" && pwd -P) || { dev_fail 'repository directory unavailable'; return 1; }
    root=$(dev_git rev-parse --show-toplevel 2>/dev/null) || { dev_fail 'not a Git worktree'; return 1; }
    root=$(cd "$root" && pwd -P) || return 1
    [ "$root" = "$DEV_ROOT" ] || { dev_fail 'expected repository root differs from Git root'; return 1; }
    # This URL guard prevents accidental repository confusion; local origin
    # metadata is editable and does not authenticate repository contents.
    origins=$(dev_git config --get-all remote.origin.url) || { dev_fail 'origin is missing'; return 1; }
    case "$origins" in
        https://github.com/kola-white/rsa-attestation-engine.git|git@github.com:kola-white/rsa-attestation-engine.git) ;;
        *) dev_fail 'unexpected repository identity (origin URL)'; return 1 ;;
    esac
    dev_pass 'repository root and origin identity'
    branch=$(dev_git symbolic-ref --quiet --short HEAD) || { dev_fail 'detached HEAD'; return 1; }
    case "$branch" in
        main|master|develop|development|staging|production|release|release/*)
            dev_fail "protected branch: $branch"; return 1 ;;
    esac
    case "$expected_branch" in
        feat/?*|feature/?*) ;;
        *) dev_fail 'expected branch must use feat/ or feature/'; return 1 ;;
    esac
    [ "$branch" = "$expected_branch" ] || { dev_fail "unexpected branch: $branch"; return 1; }
    dev_git rev-parse --verify HEAD >/dev/null 2>&1 || { dev_fail 'repository has no HEAD commit'; return 1; }
    dev_pass "expected feature branch: $branch"
    for operation in MERGE_HEAD CHERRY_PICK_HEAD REVERT_HEAD REBASE_HEAD \
        rebase-merge rebase-apply sequencer BISECT_START index.lock; do
        path=$(dev_git rev-parse --git-path "$operation") || return 1
        case "$path" in /*) ;; *) path=$DEV_ROOT/$path ;; esac
        if [ -e "$path" ] || [ -L "$path" ]; then
            dev_fail "Git operation or lock present: $operation"; return 1
        fi
    done
    # These index flags can conceal working-tree changes from diff/status.
    flags=$(mktemp "${TMPDIR:-/tmp}/cvera-index.XXXXXXXX") || return 1
    if ! dev_git ls-files -v -z > "$flags"; then rm -f "$flags"; return 1; fi
    while IFS= read -r -d '' path; do
        case "$path" in
            H\ *) ;;
            *) rm -f "$flags"; dev_fail 'unsupported index flags (sparse/assume-unchanged/unmerged)'; return 1 ;;
        esac
    done < "$flags"
    rm -f "$flags"
    dev_pass 'no in-progress Git operation or hidden index entries'
    case "$cleanliness" in
        require-clean)
            # Standard Git semantics: ignored workspace artifacts are not changes.
            # Keep NUL-delimited records in a file: Bash variables cannot hold
            # NUL bytes. Any output means dirty, so no path parsing is needed.
            flags=$(mktemp "${TMPDIR:-/tmp}/cvera-status.XXXXXXXX") || return 1
            if ! dev_git status --porcelain=v1 -z --untracked-files=all --ignore-submodules=none > "$flags"; then
                rm -f "$flags"; dev_fail 'cannot inspect baseline status'; return 1
            fi
            if [ -s "$flags" ]; then
                rm -f "$flags"; dev_fail 'baseline is not clean'; return 1
            fi
            rm -f "$flags"
            dev_pass 'clean baseline' ;;
        allow-dirty) ;;
        *) dev_fail 'unknown cleanliness mode'; return 1 ;;
    esac
}
