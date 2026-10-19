#!/bin/bash
set -euo pipefail
# Usage: bash scripts/dev/prepare.sh --repo ROOT --branch feat/NAME --base FULL_SHA [--full]
# Run reviewed scripts with trusted local verification commands and dependencies.
# Preparation only prints proposals; keep other Git/worktree writers idle.
scripts=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)
source "$scripts/git-guard.sh"

repo= branch= base= full=
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
        --full)
            [ -z "$full" ] || { dev_fail 'duplicate --full'; exit 1; }
            full=--full; shift ;;
        *) dev_fail "unknown argument: $1"; exit 1 ;;
    esac
done
[ -n "$repo" ] && [ -n "$branch" ] && [ -n "$base" ] || {
    dev_fail 'required: --repo ROOT --branch feat/NAME --base FULL_SHA [--full]'; exit 1;
}

# Prevent ambient Git redirection and implicit remote object retrieval, including
# in the unchanged scope/verification scripts. All scratch files stay outside
# the target repository even if the caller's TMPDIR points into it.
for name in GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY \
    GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_COMMON_DIR GIT_NAMESPACE \
    GIT_CONFIG GIT_CONFIG_COUNT GIT_CONFIG_PARAMETERS GIT_SHALLOW_FILE \
    GIT_REPLACE_REF_BASE; do
    eval 'value=${'"$name"'+set}'
    [ -z "$value" ] || { dev_fail "Git environment override: $name"; exit 1; }
done
export GIT_NO_LAZY_FETCH=1 GIT_TERMINAL_PROMPT=0 GIT_NO_REPLACE_OBJECTS=1
export TMPDIR=/tmp
bash "$scripts/scope.sh" --repo "$repo" --branch "$branch" --base "$base" || {
    dev_fail 'Phase 1 scope; preparation stopped'; exit 1;
}
DEV_ROOT=$(cd "$repo" && pwd -P)
head=$(dev_git rev-parse --verify 'HEAD^{commit}') || { dev_fail 'cannot resolve HEAD'; exit 1; }
scratch=$(mktemp -d /tmp/cvera-prepare.XXXXXXXX)
trap 'rm -rf "$scratch"' EXIT
verify_args=(--repo "$DEV_ROOT" --branch "$branch" --base "$base")
if [ -n "$full" ]; then verify_args+=(--full); fi
if ! bash "$scripts/verify.sh" "${verify_args[@]}" > "$scratch/verification" 2>&1; then
    cat "$scratch/verification" >&2
    dev_fail 'verification failed; preparation stopped'; exit 1
fi

# Disable rename detection so both original and destination paths are reported.
# Never put NUL streams in variables, split filenames, or use line-based Git
# output. %q renders every literal path on one terminal-safe, shell-escaped line.
enumerate_diff() {
    local layer=$1 status path
    shift
    dev_git diff --name-status -z --no-renames --no-ext-diff --no-textconv \
        --ignore-submodules=none "$@" -- > "$scratch/records" || {
        dev_fail "cannot enumerate $layer changes"; return 1;
    }
    layer_count=0
    : > "$scratch/$layer"
    while IFS= read -r -d '' status || [ -n "$status" ]; do
        case "$status" in A|D|M|T) ;; *) dev_fail "invalid $layer diff status"; return 1 ;; esac
        IFS= read -r -d '' path && [ -n "$path" ] || {
            dev_fail "incomplete $layer diff record"; return 1;
        }
        printf '  %s %q\n' "$status" "$path" >> "$scratch/$layer"
        layer_count=$((layer_count + 1))
    done < "$scratch/records"
}
enumerate_diff committed "$base" "$head"
committed=$layer_count
enumerate_diff staged --cached "$head"
staged=$layer_count
enumerate_diff unstaged
unstaged=$layer_count
dev_git ls-files --others --exclude-standard -z > "$scratch/records" || {
    dev_fail 'cannot enumerate non-ignored untracked files'; exit 1;
}
untracked=0
: > "$scratch/untracked"
while :; do
    path=
    if ! IFS= read -r -d '' path; then
        [ -z "$path" ] || { dev_fail 'incomplete untracked path record'; exit 1; }
        break
    fi
    [ -n "$path" ] && [[ "$path" = scripts/dev/?* ]] || {
        dev_fail 'invalid untracked path record'; exit 1;
    }
    printf '  ? %q\n' "$path" >> "$scratch/untracked"
    untracked=$((untracked + 1))
done < "$scratch/records"

# Revalidate scope after verification, which executes project-owned code. This
# also catches branch movement and newly out-of-scope files before printing.
bash "$scripts/scope.sh" --repo "$DEV_ROOT" --branch "$branch" --base "$base" > "$scratch/final-scope" 2>&1 || {
    cat "$scratch/final-scope" >&2; dev_fail 'scope changed during preparation'; exit 1;
}
current_head=$(dev_git rev-parse --verify 'HEAD^{commit}') || { dev_fail 'cannot resolve current HEAD'; exit 1; }
[ "$head" = "$current_head" ] || { dev_fail 'HEAD changed during preparation'; exit 1; }

printf '\nPreparation proposal (Terminal only)\nRepository: %q\nBranch: %q\n' "$DEV_ROOT" "$branch"
printf 'Base SHA: %s\nCurrent HEAD SHA: %s\n' "$base" "$head"
printf 'Change summary: committed=%s staged=%s unstaged=%s non-ignored-untracked=%s\n' \
    "$committed" "$staged" "$unstaged" "$untracked"
title='chore(dev): update developer workflow automation'
printf '\nProposed conventional commit message:\n%s\n' "$title"
printf '\nProposed PR title:\n%s\n\nProposed PR description:\n' "$title"
printf 'Purpose and scope\nUpdate developer workflow automation within scripts/dev/. Review and refine this proposed purpose against the listed changes.\n'
printf 'Base SHA: %s\nCurrent HEAD SHA: %s\n' "$base" "$head"
printf '\nFiles changed\n'
for layer in committed staged unstaged untracked; do
    printf '%s:\n' "$layer"
    if [ -s "$scratch/$layer" ]; then cat "$scratch/$layer"; else printf '  (none)\n'; fi
done
printf 'Paths use Bash shell escaping; A=added, D=deleted, M=modified, T=type changed, ?=untracked.\n'
printf 'Counts are per layer; paths may repeat. Renames are deletion + addition.\n'
printf '\nVerification evidence\nPASS: existing scope checker before and after verification.\n'
printf 'PASS: Go formatting of added/modified index and working files.\nPASS: go vet ./...\nPASS: go test -mod=readonly ./...\n'
if [ -n "$full" ]; then
    printf 'PASS: npm run test:ci (--full, offline mode).\n'
else
    printf 'Default Go verification; npm run test:ci was not run (use --full).\n'
fi
printf '\nExplicit limitations\n'
printf '%s\n' \
    '- Generic proposed title and purpose require human refinement; file contents are not semantically reviewed.' \
    '- Local checks use the working tree; staged and historical snapshots are not independently built or tested.' \
    '- Verification runs trusted project commands with existing dependencies and offline settings; it is not a sandbox.' \
    '- This is a non-atomic local snapshot. Keep other writers idle and rerun after any change.' \
    '- No remote status was queried. GitHub CI and deployment checks remain authoritative.' \
    '- No staging, commit, push, branch, PR, merge, Git configuration change, or dependency installation is performed by preparation.'
printf '\nHuman review checklist\n'
printf '%s\n' \
    '- [ ] Confirm purpose, scope, proposed commit message and PR title.' \
    '- [ ] Review every listed path and diff, including deletions and all dirty layers.' \
    '- [ ] Confirm the intended final commit contents; rerun verification after changes.' \
    '- [ ] Assess verification coverage and explicit limitations; run --full when appropriate.' \
    '- [ ] Confirm authoritative GitHub CI and deployment checks before merge or deployment.'
