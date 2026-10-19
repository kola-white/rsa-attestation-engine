#!/bin/bash
set -euo pipefail
# Preview: bash scripts/dev/sync.sh --repo ROOT
# Execute separately after review: add --execute --target FULL_SHA.
# origin/main is local metadata. Fetching is a separate, manually approved task.
# Run a reviewed copy with trusted local Git configuration. Keep concurrent Git
# and worktree writers idle: revalidation does not provide a transaction.
scripts=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)
source "$scripts/git-guard.sh"

repo= target= mode=preview mode_seen=
while [ "$#" -gt 0 ]; do
    case "$1" in
        --repo|--target)
            [ "$#" -ge 2 ] && [ -n "$2" ] || { dev_fail "missing value for $1"; exit 1; }
            case "$2" in --*) dev_fail "missing value for $1"; exit 1 ;; esac
            case "$1" in
                --repo) [ -z "$repo" ] || { dev_fail 'duplicate --repo'; exit 1; }; repo=$2 ;;
                --target) [ -z "$target" ] || { dev_fail 'duplicate --target'; exit 1; }; target=$2 ;;
            esac
            shift 2 ;;
        --preview|--execute)
            [ -z "$mode_seen" ] || { dev_fail 'choose only one mode'; exit 1; }
            mode_seen=1; mode=${1#--}; shift ;;
        *) dev_fail "unknown argument: $1"; exit 1 ;;
    esac
done
[ -n "$repo" ] || { dev_fail 'required: --repo ROOT [--execute --target FULL_SHA]'; exit 1; }
if [ "$mode" = execute ]; then
    [[ "$target" =~ ^[0-9a-f]{40}$ || "$target" =~ ^[0-9a-f]{64}$ ]] || {
        dev_fail 'execution requires --target with the approved full commit SHA'; exit 1;
    }
else
    [ -z "$target" ] || { dev_fail '--target requires a separately invoked --execute'; exit 1; }
fi

for name in GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY \
    GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_COMMON_DIR GIT_NAMESPACE \
    GIT_CONFIG GIT_CONFIG_COUNT GIT_CONFIG_PARAMETERS GIT_SHALLOW_FILE \
    GIT_REPLACE_REF_BASE; do
    eval 'value=${'"$name"'+set}'
    [ -z "$value" ] || { dev_fail "Git environment override: $name"; exit 1; }
done
export GIT_NO_LAZY_FETCH=1 GIT_TERMINAL_PROMPT=0 GIT_NO_REPLACE_OBJECTS=1
# Keep Git and platform helper temporary files outside the target as well.
export TMPDIR=/tmp
# Deny all Git transports as defense in depth, including older Git versions
# without GIT_NO_LAZY_FETCH. Settings are command-scoped, never persisted.
export GIT_ALLOW_PROTOCOL=
dev_git() {
    command git --no-optional-locks -c core.fsmonitor=false -c core.untrackedCache=false \
        -c core.hooksPath=/dev/null -c submodule.recurse=false \
        -c fetch.recurseSubmodules=false -c protocol.allow=never \
        -c merge.verifySignatures=false -c commit.gpgsign=false \
        -c gc.auto=0 -c maintenance.auto=false -C "$DEV_ROOT" "$@"
}
DEV_ROOT=$(cd "$repo" && pwd -P) || { dev_fail 'repository directory unavailable'; exit 1; }
scratch=$(mktemp -d /tmp/cvera-sync.XXXXXXXX)
trap 'rm -rf "$scratch"' EXIT

validate() {
    local root origins current operation path ref code raw main remote head
    root=$(dev_git rev-parse --show-toplevel) || { dev_fail 'not a Git worktree'; return 1; }
    root=$(cd "$root" && pwd -P) || return 1
    [ "$root" = "$DEV_ROOT" ] || { dev_fail 'expected repository root differs from Git root'; return 1; }
    origins=$(dev_git config --get-all remote.origin.url) || { dev_fail 'origin is missing'; return 1; }
    # Identity convention matches git-guard.sh; editable metadata is not proof
    # that the commit contents came from GitHub.
    case "$origins" in
        https://github.com/kola-white/rsa-attestation-engine.git|git@github.com:kola-white/rsa-attestation-engine.git) ;;
        *) dev_fail 'unexpected repository identity (origin URL)'; return 1 ;;
    esac
    # Checkout/status filters (for example an LFS smudge process) can perform
    # their own network access outside Git's transport restrictions. Fail closed
    # rather than invoking any configured clean, smudge, or process command.
    code=0
    dev_git config --get-regexp '^filter\..*\.(clean|smudge|process)$' > "$scratch/filters" || code=$?
    case "$code" in
        1) ;;
        0) dev_fail 'external Git filters are unsupported for offline synchronization'; return 1 ;;
        *) dev_fail 'cannot inspect external Git filters'; return 1 ;;
    esac
    current=$(dev_git symbolic-ref --quiet HEAD) || { dev_fail 'detached HEAD'; return 1; }
    [ "$current" = refs/heads/main ] || { dev_fail "expected current branch main: $current"; return 1; }
    for operation in MERGE_HEAD CHERRY_PICK_HEAD REVERT_HEAD REBASE_HEAD \
        rebase-merge rebase-apply sequencer BISECT_START index.lock HEAD.lock \
        packed-refs.lock refs/heads/main.lock refs/remotes/origin/main.lock; do
        path=$(dev_git rev-parse --git-path "$operation") || { dev_fail 'cannot inspect Git operation paths'; return 1; }
        case "$path" in /*) ;; *) path=$DEV_ROOT/$path ;; esac
        if [ -e "$path" ] || [ -L "$path" ]; then
            dev_fail "Git operation or lock present: $operation"; return 1
        fi
    done
    # Neither source ref may redirect to another ref or point to a tag object.
    for ref in refs/heads/main refs/remotes/origin/main; do
        code=0
        dev_git symbolic-ref --quiet "$ref" > /dev/null || code=$?
        case "$code" in
            1) ;;
            0) dev_fail "unexpected symbolic ref: $ref"; return 1 ;;
            *) dev_fail "cannot inspect ref: $ref"; return 1 ;;
        esac
        raw=$(dev_git rev-parse --verify "$ref") || { dev_fail "missing ref: $ref"; return 1; }
        head=$(dev_git rev-parse --verify "$ref^{commit}") || { dev_fail "ref has no commit: $ref"; return 1; }
        [ "$raw" = "$head" ] || { dev_fail "ref is not a direct commit: $ref"; return 1; }
    done
    dev_git ls-files -v -z > "$scratch/index" || { dev_fail 'cannot inspect index'; return 1; }
    while :; do
        path=
        if ! IFS= read -r -d '' path; then
            [ -z "$path" ] || { dev_fail 'incomplete index record'; return 1; }
            break
        fi
        case "$path" in
            H\ ?*) ;;
            *) dev_fail 'unsupported index flags (sparse/assume-unchanged/unmerged)'; return 1 ;;
        esac
    done < "$scratch/index"
    dev_git status --porcelain=v1 -z --untracked-files=all --ignore-submodules=none > "$scratch/status" || {
        dev_fail 'cannot inspect baseline status'; return 1;
    }
    [ ! -s "$scratch/status" ] || { dev_fail 'baseline is not clean'; return 1; }
    main=$(dev_git rev-parse --verify 'refs/heads/main^{commit}') || { dev_fail 'main has no commit'; return 1; }
    remote=$(dev_git rev-parse --verify 'refs/remotes/origin/main^{commit}') || { dev_fail 'origin/main is missing'; return 1; }
    head=$(dev_git rev-parse --verify 'HEAD^{commit}') || { dev_fail 'cannot inspect HEAD'; return 1; }
    [ "$head" = "$main" ] || { dev_fail 'HEAD and main do not match'; return 1; }
    if [ -n "$target" ] && [ "$remote" != "$target" ]; then
        dev_fail 'origin/main no longer matches the approved target SHA'; return 1
    fi
    dev_git merge-base --is-ancestor "$main" "$remote" || {
        dev_fail 'main is not an ancestor of target, or ancestry check failed (divergence/rollback)'; return 1;
    }
    validated_main=$main validated_target=$remote
}

validate || exit 1
printf 'Repository: %s\nCurrent main SHA: %s\nLocally recorded origin/main target SHA: %s\n' \
    "$DEV_ROOT" "$validated_main" "$validated_target"
printf 'No fetch performed; origin/main freshness is not guaranteed.\n'
if [ "$validated_main" = "$validated_target" ]; then
    printf 'Already synchronized; no update needed.\n'; exit 0
fi
if [ "$mode" = preview ]; then
    printf 'Preview only; no Git changes made. Review, then invoke separately:\n'
    printf 'bash %q --repo %q --execute --target %q\n' "$scripts/sync.sh" "$DEV_ROOT" "$validated_target"
    exit 0
fi
# Revalidate immediately before the sole mutation, using the approved immutable
# SHA rather than a mutable ref. This cannot exclude concurrent Git writers.
validate || exit 1
if [ "$validated_main" = "$target" ]; then
    printf 'Already synchronized; no update needed.\n'; exit 0
fi
dev_git merge --ff-only --no-edit --no-stat --no-autostash --no-overwrite-ignore "$target" || {
    dev_fail 'fast-forward failed; inspect repository state before retrying'; exit 1;
}
result_branch=$(dev_git symbolic-ref --quiet HEAD) || { dev_fail 'cannot inspect resulting branch'; exit 1; }
result_main=$(dev_git rev-parse --verify 'refs/heads/main^{commit}') || { dev_fail 'cannot inspect resulting main'; exit 1; }
result_head=$(dev_git rev-parse --verify 'HEAD^{commit}') || { dev_fail 'cannot inspect resulting HEAD'; exit 1; }
[ "$result_branch" = refs/heads/main ] && [ "$result_main" = "$target" ] && [ "$result_head" = "$target" ] || {
    dev_fail 'unexpected result; inspect repository state before retrying'; exit 1;
}
printf 'Synchronized local main and HEAD to approved target: %s\n' "$target"
