#!/bin/bash
set -euo pipefail
# Usage: bash scripts/dev/verify.sh --repo ROOT --branch feat/NAME --base FULL_SHA [--full]
# Default: scope, changed Go formatting, Go vet and tests. --full adds root test:ci.
# Run a reviewed copy, with dependencies/toolchain already available locally.
scripts=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)
source "$scripts/git-guard.sh"

repo= branch= base= mode=default
while [ "$#" -gt 0 ]; do
    case "$1" in
        --repo|--branch|--base)
            [ "$#" -ge 2 ] && [ -n "$2" ] || { dev_fail "missing value for $1"; exit 1; }
            case "$2" in --*) dev_fail "missing value for $1"; exit 1 ;; esac
            case "$1" in --repo) repo=$2 ;; --branch) branch=$2 ;; --base) base=$2 ;; esac
            shift 2 ;;
        --full) mode=full; shift ;;
        *) dev_fail "unknown argument: $1"; exit 1 ;;
    esac
done
[ -n "$repo" ] && [ -n "$branch" ] && [ -n "$base" ] || {
    dev_fail 'required: --repo ROOT --branch feat/NAME --base FULL_SHA [--full]'; exit 1;
}
# Scope owns the Phase 1 guards and explicit ancestor-SHA validation.
bash "$scripts/scope.sh" --repo "$repo" --branch "$branch" --base "$base" || {
    dev_fail 'Phase 1 scope'; exit 1;
}
DEV_ROOT=$(cd "$repo" && pwd -P)
scratch=$(mktemp -d "${TMPDIR:-/tmp}/cvera-verify.XXXXXXXX")
trap 'rm -rf "$scratch"' EXIT

format_check() {
    local layer=$1 path output
    shift
    dev_git diff --name-only -z --diff-filter=AM --no-renames --no-ext-diff --no-textconv "$@" -- > "$scratch/paths" || {
        dev_fail 'cannot enumerate Go formatting paths'; return 1;
    }
    if [ "$layer" = worktree ]; then
        dev_git ls-files --others --exclude-standard -z >> "$scratch/paths" || {
            dev_fail 'cannot enumerate new Go files'; return 1;
        }
    fi
    while IFS= read -r -d '' path; do
        case "$path" in *.go) ;; *) continue ;; esac
        if [ "$layer" = index ]; then
            dev_git show ":$path" > "$scratch/source.go" || {
                dev_fail 'cannot read indexed Go file'; return 1;
            }
        else
            cat "$DEV_ROOT/$path" > "$scratch/source.go" || {
                dev_fail 'cannot read working Go file'; return 1;
            }
        fi
        output=$(gofmt -l < "$scratch/source.go") || {
            printf 'FAIL: Go formatting (%s): %q\n' "$layer" "$path" >&2; return 1;
        }
        if [ -n "$output" ]; then
            printf 'FAIL: Go formatting (%s): %q\n' "$layer" "$path" >&2; return 1
        fi
    done < "$scratch/paths"
}
# Compare both snapshots with the explicit base; -z preserves arbitrary filenames.
# Deleted files are excluded. Renames appear as deletion + addition.
format_check index --cached "$base"
format_check worktree "$base"
dev_pass 'Go formatting: added/modified index and working files'

# Do not download modules, checksum data, or an automatic Go toolchain, and do
# not let ambient GOFLAGS enable module-file writes for vet.
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly
run_check() {
    local directory=$1 label=$2
    shift 2
    if (cd "$directory" && "$@"); then
        dev_pass "$label"
    else
        dev_fail "$label"; return 1
    fi
}
run_check "$DEV_ROOT/apps/evt-api-go" 'go vet ./...' go vet ./...
run_check "$DEV_ROOT/apps/evt-api-go" 'go test -mod=readonly ./...' go test -mod=readonly ./...
if [ "$mode" = full ]; then
    run_check "$DEV_ROOT" 'npm run test:ci' env npm_config_offline=true npm_config_update_notifier=false npm run test:ci
fi
dev_pass "verification complete ($mode)"
