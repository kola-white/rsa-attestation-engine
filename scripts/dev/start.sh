#!/bin/bash
set -euo pipefail
# Preview: bash scripts/dev/start.sh --repo ROOT --branch feat/NAME
# Execute separately after review: add --execute --base FULL_SHA.
# Run a reviewed copy. The SHA is the caller's review acknowledgment, not an
# authenticated approval token. Keep other Git writers idle during execution.
scripts=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)
source "$scripts/git-guard.sh"

repo= branch= base= mode=preview mode_seen=
while [ "$#" -gt 0 ]; do
    case "$1" in
        --repo|--branch|--base)
            [ "$#" -ge 2 ] && [ -n "$2" ] || { dev_fail "missing value for $1"; exit 1; }
            case "$2" in --*) dev_fail "missing value for $1"; exit 1 ;; esac
            case "$1" in
                --repo) [ -z "$repo" ] || { dev_fail 'duplicate --repo'; exit 1; }; repo=$2 ;;
                --branch) [ -z "$branch" ] || { dev_fail 'duplicate --branch'; exit 1; }; branch=$2 ;;
                --base) [ -z "$base" ] || { dev_fail 'duplicate --base'; exit 1; }; base=$2 ;;
            esac
            shift 2 ;;
        --preview|--execute)
            [ -z "$mode_seen" ] || { dev_fail 'choose only one mode'; exit 1; }
            mode_seen=1; mode=${1#--}; shift ;;
        *) dev_fail "unknown argument: $1"; exit 1 ;;
    esac
done
[ -n "$repo" ] && [ -n "$branch" ] || {
    dev_fail 'required: --repo ROOT --branch feat/NAME [--execute --base FULL_SHA]'; exit 1;
}
if [ "$mode" = execute ]; then
    [[ "$base" =~ ^[0-9a-f]{40}$ || "$base" =~ ^[0-9a-f]{64}$ ]] || {
        dev_fail 'execution requires --base with the reviewed full commit SHA'; exit 1;
    }
else
    [ -z "$base" ] || { dev_fail '--base requires a separately invoked --execute'; exit 1; }
fi

# Refuse ambient redirection before running any Git command. Do not change
# configuration or permit lazy object retrieval from a partial-clone remote.
for name in GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY \
    GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_COMMON_DIR GIT_NAMESPACE \
    GIT_CONFIG GIT_CONFIG_COUNT GIT_CONFIG_PARAMETERS GIT_SHALLOW_FILE \
    GIT_REPLACE_REF_BASE; do
    eval 'value=${'"$name"'+set}'
    [ -z "$value" ] || { dev_fail "Git environment override: $name"; exit 1; }
done
export GIT_NO_LAZY_FETCH=1 GIT_TERMINAL_PROMPT=0 GIT_NO_REPLACE_OBJECTS=1
DEV_ROOT=$(cd "$repo" && pwd -P) || { dev_fail 'repository directory unavailable'; exit 1; }
scratch=$(mktemp -d "${TMPDIR:-/tmp}/cvera-start.XXXXXXXX")
trap 'rm -rf "$scratch"' EXIT

validate() {
    local root origins current operation path main remote exists=0
    root=$(dev_git rev-parse --show-toplevel) || { dev_fail 'not a Git worktree'; return 1; }
    root=$(cd "$root" && pwd -P) || return 1
    [ "$root" = "$DEV_ROOT" ] || { dev_fail 'expected repository root differs from Git root'; return 1; }
    origins=$(dev_git config --get-all remote.origin.url) || { dev_fail 'origin is missing'; return 1; }
    case "$origins" in
        https://github.com/kola-white/rsa-attestation-engine.git|git@github.com:kola-white/rsa-attestation-engine.git) ;;
        *) dev_fail 'unexpected repository identity (origin URL)'; return 1 ;;
    esac
    current=$(dev_git symbolic-ref --quiet HEAD) || { dev_fail 'detached HEAD'; return 1; }
    [ "$current" = refs/heads/main ] || { dev_fail "expected current branch main: $current"; return 1; }
    for operation in MERGE_HEAD CHERRY_PICK_HEAD REVERT_HEAD REBASE_HEAD \
        rebase-merge rebase-apply sequencer BISECT_START index.lock HEAD.lock \
        packed-refs.lock refs/heads/main.lock refs/remotes/origin/main.lock; do
        path=$(dev_git rev-parse --git-path "$operation") || return 1
        case "$path" in /*) ;; *) path=$DEV_ROOT/$path ;; esac
        if [ -e "$path" ] || [ -L "$path" ]; then
            dev_fail "Git operation or lock present: $operation"; return 1
        fi
    done
    case "$branch" in
        feat/?*|feature/?*) ;;
        *) dev_fail 'branch must use feat/ or feature/'; return 1 ;;
    esac
    dev_git check-ref-format "refs/heads/$branch" || { dev_fail 'invalid branch name'; return 1; }
    dev_git show-ref --verify --quiet "refs/heads/$branch" || exists=$?
    case "$exists" in
        0) dev_fail 'requested local branch already exists'; return 1 ;;
        1) ;;
        *) dev_fail 'cannot inspect requested branch'; return 1 ;;
    esac
    dev_git ls-files -v -z > "$scratch/index" || { dev_fail 'cannot inspect index'; return 1; }
    while IFS= read -r -d '' path; do
        case "$path" in
            H\ *) ;;
            *) dev_fail 'unsupported index flags (sparse/assume-unchanged/unmerged)'; return 1 ;;
        esac
    done < "$scratch/index"
    dev_git status --porcelain=v1 -z --untracked-files=all --ignore-submodules=none > "$scratch/status" || {
        dev_fail 'cannot inspect baseline status'; return 1;
    }
    [ ! -s "$scratch/status" ] || { dev_fail 'baseline is not clean'; return 1; }
    main=$(dev_git rev-parse --verify 'refs/heads/main^{commit}') || { dev_fail 'main has no commit'; return 1; }
    remote=$(dev_git rev-parse --verify 'refs/remotes/origin/main^{commit}') || { dev_fail 'origin/main is missing'; return 1; }
    [ "$main" = "$remote" ] || { dev_fail 'main and origin/main do not match'; return 1; }
    if [ -n "$base" ] && [ "$main" != "$base" ]; then
        dev_fail 'main no longer matches the reviewed SHA'; return 1
    fi
    validated_sha=$main
}

validate || exit 1
printf 'Repository: %s\nProposed branch: %s\nBase commit SHA: %s\n' "$DEV_ROOT" "$branch" "$validated_sha"
if [ "$mode" = preview ]; then
    printf 'Preview only; no Git changes made. Review, then invoke separately:\n'
    printf 'bash %q --repo %q --branch %q --execute --base %q\n' "$scripts/start.sh" "$DEV_ROOT" "$branch" "$validated_sha"
    exit 0
fi
# Recheck immediately before the sole mutation. Disable hooks and automatic
# tracking/submodule behavior for this command only; never persist config.
validate || exit 1
dev_git -c core.hooksPath=/dev/null -c submodule.recurse=false \
    switch --no-track -c "$branch" "$base" || {
    dev_fail 'branch creation failed; inspect repository state before retrying'; exit 1;
}
result_branch=$(dev_git symbolic-ref --quiet HEAD) || { dev_fail 'cannot inspect resulting branch'; exit 1; }
result_sha=$(dev_git rev-parse --verify HEAD) || { dev_fail 'cannot inspect resulting commit'; exit 1; }
[ "$result_branch" = "refs/heads/$branch" ] && [ "$result_sha" = "$base" ] || {
    dev_fail 'unexpected result; inspect repository state before retrying'; exit 1;
}
printf 'Created branch: %s\nCommit SHA: %s\n' "$branch" "$result_sha"
