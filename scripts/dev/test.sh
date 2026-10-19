#!/bin/bash
set -euo pipefail
# All Git mutations below target new, disposable repositories, never the caller's.
# Run: bash scripts/dev/test.sh (compatible with macOS Bash 3.2).
scripts=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)
scratch=$(mktemp -d "${TMPDIR:-/tmp}/cvera-dev-tests.XXXXXXXX")
trap 'rm -rf "$scratch"' EXIT
export GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null
export GIT_AUTHOR_NAME=Test GIT_AUTHOR_EMAIL=test@example.invalid
export GIT_COMMITTER_NAME=Test GIT_COMMITTER_EMAIL=test@example.invalid
export GIT_TERMINAL_PROMPT=0
count=0
test_count=0

new_repo() {
    count=$((count + 1))
    repo=$scratch/repo-$count
    git init -q "$repo"
    git -C "$repo" symbolic-ref HEAD refs/heads/feat/test
    git -C "$repo" remote add origin https://github.com/kola-white/rsa-attestation-engine.git
    mkdir -p "$repo/scripts/dev" "$repo/app"
    printf 'baseline\n' > "$repo/scripts/dev/tracked"
    printf 'application\n' > "$repo/app/tracked"
    git -C "$repo" add .
    git -C "$repo" -c commit.gpgsign=false commit -qm baseline
    base=$(git -C "$repo" rev-parse HEAD)
}
commit_all() {
    git -C "$repo" add -A
    git -C "$repo" -c commit.gpgsign=false commit -qm fixture
}
ignore_rules() {
    printf '%s\n' "$@" > "$repo/.gitignore"
    commit_all
    base=$(git -C "$repo" rev-parse HEAD)
}
scope() { bash "$scripts/scope.sh" --repo "$repo" --branch feat/test --base "$base" "$@"; }
expect() {
    local wanted=$1 label=$2 needle=$3 result=0
    shift 3
    "$@" > "$scratch/output" 2>&1 || result=$?
    if { [ "$wanted" = pass ] && [ "$result" -ne 0 ]; } ||
       { [ "$wanted" = fail ] && [ "$result" -eq 0 ]; } ||
       ! /usr/bin/grep -Fq -- "$needle" "$scratch/output"; then
        printf 'FAIL test: %s (exit %s)\n' "$label" "$result"
        cat "$scratch/output"
        exit 1
    fi
    test_count=$((test_count + 1))
    printf 'PASS test: %s\n' "$label"
}

new_repo
expect pass 'clean explicit baseline' 'clean baseline' scope --require-clean
printf 'committed\n' > "$repo/scripts/dev/committed"
commit_all
printf 'staged\n' > "$repo/scripts/dev/staged"
git -C "$repo" add scripts/dev/staged
printf 'unstaged\n' >> "$repo/scripts/dev/tracked"
printf 'untracked\n' > "$repo/scripts/dev/untracked"
expect pass 'allowed paths in all four states' 'Phase 1 scope' scope
expect fail 'dirty baseline rejected when required' 'baseline is not clean' scope --require-clean

for state in committed staged unstaged untracked; do
    new_repo
    case "$state" in
        committed) printf x >> "$repo/app/tracked"; commit_all ;;
        staged) printf x >> "$repo/app/tracked"; git -C "$repo" add app/tracked ;;
        unstaged) printf x >> "$repo/app/tracked" ;;
        untracked) printf x > "$repo/app/new" ;;
    esac
    expect fail "outside path: $state" 'outside Phase 1 scope' scope
done

for state in committed staged unstaged; do
    for direction in into outof; do
        new_repo
        if [ "$direction" = into ]; then
            mv "$repo/app/tracked" "$repo/scripts/dev/moved"
        else
            mv "$repo/scripts/dev/tracked" "$repo/app/moved"
        fi
        case "$state" in committed) commit_all ;; staged) git -C "$repo" add -A ;; esac
        expect fail "rename $direction scope: $state" 'outside Phase 1 scope' scope
    done
done

new_repo
odd=$'spaces\tand\nnewline "quote" \\ slash * glob'
printf x > "$repo/scripts/dev/$odd"
expect pass 'untracked unusual filename' 'Phase 1 scope' scope
git -C "$repo" add .
expect pass 'staged unusual filename' 'Phase 1 scope' scope
commit_all
printf y >> "$repo/scripts/dev/$odd"
expect pass 'committed and unstaged unusual filename' 'Phase 1 scope' scope
mv "$repo/scripts/dev/$odd" "$repo/scripts/dev/${odd} renamed"
git -C "$repo" add -A
expect pass 'rename between unusual allowed paths' 'Phase 1 scope' scope
printf x > "$repo/app/$odd"
expect fail 'outside unusual filename' 'outside Phase 1 scope' scope

# Exercise raw diff parsing for unauthorized literal paths in every diff layer.
for state in committed staged unstaged; do
    new_repo
    printf x > "$repo/app/$odd"
    commit_all
    base=$(git -C "$repo" rev-parse HEAD)
    printf y >> "$repo/app/$odd"
    case "$state" in committed) commit_all ;; staged) git -C "$repo" add -A ;; esac
    expect fail "outside unusual tracked filename: $state" 'outside Phase 1 scope' scope
done
for state in committed staged; do
    for direction in into outof; do
        new_repo
        printf x > "$repo/scripts/dev/$odd"
        printf y > "$repo/app/$odd"
        commit_all
        base=$(git -C "$repo" rev-parse HEAD)
        if [ "$direction" = into ]; then
            mv "$repo/app/$odd" "$repo/scripts/dev/${odd} destination"
        else
            mv "$repo/scripts/dev/$odd" "$repo/app/${odd} destination"
        fi
        case "$state" in committed) commit_all ;; staged) git -C "$repo" add -A ;; esac
        expect fail "unusual rename $direction scope: $state" 'outside Phase 1 scope' scope
    done
done

# A NUL status stream can contain either one path or a rename's two paths.
for state in untracked staged unstaged rename; do
    new_repo
    odd_status=$' \t\n"\\*? leading and trailing \n'
    printf x > "$repo/scripts/dev/$odd_status"
    case "$state" in
        staged) git -C "$repo" add -A ;;
        unstaged|rename)
            commit_all
            base=$(git -C "$repo" rev-parse HEAD)
            if [ "$state" = unstaged ]; then
                printf y >> "$repo/scripts/dev/$odd_status"
            else
                mv "$repo/scripts/dev/$odd_status" "$repo/scripts/dev/${odd_status} renamed"
                git -C "$repo" add -A
            fi ;;
    esac
    expect fail "unusual filename blocks clean baseline: $state" 'baseline is not clean' scope --require-clean
done

for state in untracked staged committed removed; do
    new_repo
    ln -s ../../app/tracked "$repo/scripts/dev/link"
    case "$state" in
        staged) git -C "$repo" add . ;;
        committed) commit_all ;;
        removed) commit_all; base=$(git -C "$repo" rev-parse HEAD); rm "$repo/scripts/dev/link" ;;
    esac
    expect fail "symlink: $state" 'FAIL:' scope
done
new_repo
ln -s ../../app/tracked "$repo/scripts/dev/link"
commit_all
base=$(git -C "$repo" rev-parse HEAD)
expect fail 'unchanged baseline symlink' 'symlink or submodule' scope

new_repo
ln -s "$scratch" "$repo/scripts/dev/linked-directory"
expect fail 'symlink directory' 'symlink' scope
new_repo
git -C "$repo" update-index --add --cacheinfo "160000,$base,scripts/dev/module"
expect fail 'staged submodule' 'symlink or submodule' scope
git -C "$repo" -c commit.gpgsign=false commit -qm module
expect fail 'committed submodule' 'symlink or submodule' scope
base=$(git -C "$repo" rev-parse HEAD)
expect fail 'unchanged baseline submodule' 'symlink or submodule' scope
new_repo
git init -q "$repo/scripts/dev/nested"
expect fail 'untracked nested repository' 'nested repository/submodule' scope

new_repo
git -C "$repo" symbolic-ref HEAD refs/heads/main
expect fail 'protected branch' 'protected branch' scope
new_repo
git -C "$repo" update-ref --no-deref HEAD "$base"
expect fail 'detached HEAD' 'detached HEAD' scope
new_repo
expect fail 'unexpected feature branch' 'unexpected branch' bash "$scripts/scope.sh" --repo "$repo" --branch feat/other --base "$base"
expect fail 'non-root repository path' 'repository root differs' bash "$scripts/scope.sh" --repo "$repo/scripts" --branch feat/test --base "$base"
git -C "$repo" remote set-url origin https://example.invalid/wrong.git
expect fail 'wrong repository identity' 'unexpected repository identity' scope

for operation in MERGE_HEAD CHERRY_PICK_HEAD REVERT_HEAD REBASE_HEAD rebase-merge rebase-apply sequencer BISECT_START index.lock; do
    new_repo
    touch "$repo/.git/$operation"
    expect fail "in-progress operation: $operation" 'Git operation or lock' scope
done
new_repo
expect fail 'missing base argument' 'required:' bash "$scripts/scope.sh" --repo "$repo" --branch feat/test
expect fail 'symbolic base refused' 'full commit SHA' bash "$scripts/scope.sh" --repo "$repo" --branch feat/test --base HEAD
expect fail 'unknown SHA refused' 'base is not a commit' bash "$scripts/scope.sh" --repo "$repo" --branch feat/test --base 0000000000000000000000000000000000000000
printf x >> "$repo/scripts/dev/tracked"
commit_all
future=$(git -C "$repo" rev-parse HEAD)
git -C "$repo" update-ref refs/heads/feat/test "$base"
expect fail 'non-ancestor base refused' 'base is not an ancestor' bash "$scripts/scope.sh" --repo "$repo" --branch feat/test --base "$future"
expect fail 'Git environment override' 'Git environment override' env GIT_INDEX_FILE="$scratch/alternate-index" bash "$scripts/scope.sh" --repo "$repo" --branch feat/test --base "$base"

# Exercise the shared guard directly, including overrides set to an empty value.
new_repo
for variable in GIT_CONFIG GIT_REPLACE_REF_BASE; do
    for value in '' /dev/null; do
        expect fail "shared guard rejects $variable=${value:-<empty>}" "Git environment override: $variable" \
            env "$variable=$value" bash -c 'source "$1"; dev_guard "$2" feat/test require-clean' \
            bash "$scripts/git-guard.sh" "$repo"
    done
done

for flag in --assume-unchanged --skip-worktree; do
    new_repo
    git -C "$repo" update-index "$flag" app/tracked
    printf x >> "$repo/app/tracked"
    expect fail "hidden changes: $flag" 'unsupported index flags' scope
done
new_repo
printf 'allow=**\n' > "$repo/scripts/dev/scope-policy.conf"
printf 'scripts/dev/**\napp/**\n' > "$repo/scripts/dev/scope-policy.txt"
printf x >> "$repo/app/tracked"
expect fail 'proposed scope policies cannot widen scope' 'outside Phase 1 scope' scope
expect fail 'unknown authorization flag refused' 'unknown argument' scope --allow-symlinks
new_repo
printf 'scripts/dev/hidden\n' > "$repo/.git/info/exclude"
printf x > "$repo/scripts/dev/hidden"
expect pass 'ignored file inside scope' 'Phase 1 scope' scope
expect pass 'ignored files do not prevent clean baseline' 'clean baseline' scope --require-clean
new_repo
printf '*\n' > "$repo/.git/info/exclude"
printf x > "$repo/"$'\n'
expect pass 'ignored newline-only filename permits clean baseline' 'clean baseline' scope --require-clean

new_repo
ignore_rules 'node_modules/' 'scripts/dev/cache/'
mkdir -p "$repo/node_modules/dependency" "$repo/scripts/dev/cache"
printf x > "$repo/node_modules/dependency/index.js"
printf x > "$repo/scripts/dev/cache/"$'ignored\tartifact\nfile'
expect pass 'ignored dependencies and cache do not block scope' 'Phase 1 scope' scope
expect pass 'ignored dependencies and cache permit clean baseline' 'clean baseline' scope --require-clean
printf x > "$repo/app/unauthorized"
expect fail 'non-ignored unauthorized file among ignored artifacts' 'outside Phase 1 scope' scope
expect fail 'non-ignored untracked file blocks clean baseline' 'baseline is not clean' scope --require-clean

# Ignore patterns never suppress checks on files already tracked at the base.
for state in committed staged unstaged; do
    new_repo
    ignore_rules 'app/tracked'
    printf x >> "$repo/app/tracked"
    case "$state" in committed) commit_all ;; staged) git -C "$repo" add -u ;; esac
    expect fail "tracked outside change matching ignore rule: $state" 'outside Phase 1 scope' scope
done
new_repo
ignore_rules 'scripts/dev/tracked'
printf x >> "$repo/scripts/dev/tracked"
expect pass 'existing tracked allowed file matching ignore rule' 'Phase 1 scope' scope
expect fail 'ignored-pattern tracked modification blocks clean baseline' 'baseline is not clean' scope --require-clean
git -C "$repo" add -u
expect pass 'previously tracked ignored-pattern staged modification allowed' 'Phase 1 scope' scope
commit_all
expect pass 'previously tracked ignored-pattern committed modification allowed' 'Phase 1 scope' scope
rm "$repo/scripts/dev/tracked"
git -C "$repo" add -u
expect pass 'previously tracked ignored-pattern deletion allowed' 'Phase 1 scope' scope

new_repo
ignore_rules 'scripts/dev/cache/*' '!scripts/dev/cache/keep'
mkdir -p "$repo/scripts/dev/cache"
printf x > "$repo/scripts/dev/cache/keep"
git -C "$repo" add -- scripts/dev/cache/keep
expect pass 'negated ignore pattern permits staged addition' 'Phase 1 scope' scope
printf x > "$repo/scripts/dev/cache/"$' ignored\tartifact\n'
git -C "$repo" add -f -- scripts/dev/cache
expect fail 'mixed additions reject ignored unusual filename' 'ignored artifact added to change set' scope

new_repo
ignore_rules 'scripts/dev/cache/' 'node_modules/'
mkdir -p "$repo/scripts/dev/cache" "$repo/node_modules/dependency"
printf x > "$repo/scripts/dev/cache/"$'artifact\twith\nnewline'
printf x > "$repo/node_modules/dependency/index.js"
printf x > "$repo/scripts/dev/new"
# Fixture-only staging verifies ordinary Git staging excludes workspace artifacts.
git -C "$repo" add -A
git -C "$repo" ls-files --cached -z -- scripts/dev/cache node_modules > "$scratch/index-artifacts"
if [ -s "$scratch/index-artifacts" ]; then
    printf 'FAIL: ordinary staging included ignored artifacts\n'; exit 1
fi
printf 'PASS test: ordinary staging excludes ignored workspace artifacts\n'
test_count=$((test_count + 1))
expect pass 'staged change set excludes ignored artifacts' 'Phase 1 scope' scope
git -C "$repo" add -f -- scripts/dev/cache
expect fail 'force-added ignored artifact inside scope rejected' 'ignored artifact added to change set' scope
git -C "$repo" -c commit.gpgsign=false commit -qm forced-artifact
expect fail 'committed ignored addition inside scope rejected' 'ignored artifact added to change set' scope
new_repo
ignore_rules 'app/ignored'
printf x > "$repo/app/ignored"
git -C "$repo" add -f -- app/ignored
expect fail 'force-added ignored artifact outside scope rejected' 'outside Phase 1 scope' scope

# Standard global excludes are honored alongside .gitignore and info/exclude.
new_repo
printf 'app/global-artifact\n' > "$scratch/global-ignore"
git -C "$repo" config core.excludesFile "$scratch/global-ignore"
printf x > "$repo/app/global-artifact"
expect pass 'configured global ignore semantics' 'clean baseline' scope --require-clean

# Inject Git errors without touching production repositories or invoking hooks.
new_repo
mkdir -p "$scratch/bin"
real_git=$(command -v git)
cat > "$scratch/bin/git" <<'GIT'
#!/bin/bash
for argument in "$@"; do
    if [ "$argument" = status ] && [ "${CVERA_TEST_REQUIRE_STATUS_NUL:-}" = 1 ]; then
        case " $* " in
            *' -z '*) ;;
            *) printf 'status must use -z\n' >&2; exit 128 ;;
        esac
    fi
    if [ "$argument" = "$CVERA_TEST_FAIL_GIT_COMMAND" ]; then
        printf 'injected Git command error\n' >&2
        exit 128
    fi
done
exec "$CVERA_TEST_REAL_GIT" "$@"
GIT
chmod +x "$scratch/bin/git"
for command in diff ls-files ls-tree check-ignore status; do
    expect fail "Git command errors fail closed: $command" 'injected Git command error' \
        env PATH="$scratch/bin:$PATH" CVERA_TEST_REAL_GIT="$real_git" CVERA_TEST_FAIL_GIT_COMMAND="$command" \
        bash "$scripts/scope.sh" --repo "$repo" --branch feat/test --base "$base" --require-clean
done
expect pass 'clean baseline explicitly requests NUL status' 'clean baseline' \
    env PATH="$scratch/bin:$PATH" CVERA_TEST_REAL_GIT="$real_git" CVERA_TEST_REQUIRE_STATUS_NUL=1 \
    bash "$scripts/scope.sh" --repo "$repo" --branch feat/test --base "$base" --require-clean
new_repo
mv "$repo/app/tracked" "$repo/app/deleted"
git -C "$repo" add -A
expect fail 'outside deletion' 'outside Phase 1 scope' scope
new_repo
git -C "$repo" remote set-url origin git@github.com:kola-white/rsa-attestation-engine.git
expect pass 'SSH origin identity' 'Phase 1 scope' scope

printf 'PASS: %s Phase 1 tests completed in isolated temporary repositories\n' "$test_count"

# Phase 2 uses real Git, scope and gofmt, with strict offline command doubles
# for vet/test/npm. No application dependencies or network access are needed.
test_count=0
verify_bin=$scratch/verify-bin
mkdir -p "$verify_bin"
export CVERA_TEST_REAL_GOFMT=$(command -v gofmt)
cat > "$verify_bin/gofmt" <<'FMT'
#!/bin/bash
printf 'gofmt\n' >> "$CVERA_TEST_LOG"
[ "${CVERA_TEST_FAIL:-}" != gofmt ] || exit 1
exec "$CVERA_TEST_REAL_GOFMT" "$@"
FMT
cat > "$verify_bin/go" <<'GO'
#!/bin/bash
printf 'go %s\n' "$*" >> "$CVERA_TEST_LOG"
[ "$PWD" = "$CVERA_TEST_REPO/apps/evt-api-go" ] || { echo 'wrong Go directory'; exit 1; }
[ "$GOPROXY:$GOSUMDB:$GOTOOLCHAIN:$GOFLAGS" = off:off:local:-mod=readonly ] || exit 1
case "$*" in 'vet ./...'|'test -mod=readonly ./...') ;; *) exit 1 ;; esac
[ "${CVERA_TEST_FAIL:-}" != "$1" ] || exit 1
GO
cat > "$verify_bin/npm" <<'NPM'
#!/bin/bash
printf 'npm %s\n' "$*" >> "$CVERA_TEST_LOG"
[ "$PWD" = "$CVERA_TEST_REPO" ] || { echo 'wrong npm directory'; exit 1; }
[ "$*" = 'run test:ci' ] || exit 1
[ "$npm_config_offline:$npm_config_update_notifier" = true:false ] || exit 1
[ "${CVERA_TEST_FAIL:-}" != npm ] || exit 1
NPM
chmod +x "$verify_bin/gofmt" "$verify_bin/go" "$verify_bin/npm"
verify_repo() {
    new_repo
    repo=$(cd "$repo" && pwd -P)
    mkdir -p "$repo/apps/evt-api-go"
    # Existing unformatted files must never be checked or rewritten.
    printf 'package baseline;var X=1\n' > "$repo/scripts/dev/baseline.go"
    printf 'package baseline;var X=1\n' > "$repo/app/baseline.go"
    commit_all
    base=$(git -C "$repo" rev-parse HEAD)
    verify_failure=
}
verify_args() {
    : > "$scratch/verify-log"
    env PATH="$verify_bin:$PATH" CVERA_TEST_LOG="$scratch/verify-log" \
        CVERA_TEST_REPO="$repo" CVERA_TEST_FAIL="$verify_failure" \
        bash "$scripts/verify.sh" "$@"
}
verify() { verify_args --repo "$repo" --branch feat/test --base "$base" "$@"; }
unchanged() { cmp "$@" && printf 'unchanged\n'; }
log_is() {
    printf '%s\n' "$@" > "$scratch/expected-log"
    if [ "$#" -eq 0 ]; then : > "$scratch/expected-log"; fi
    if ! cmp -s "$scratch/expected-log" "$scratch/verify-log"; then
        printf 'FAIL: unexpected verification commands\n'
        cat "$scratch/verify-log"
        exit 1
    fi
}

verify_repo
expect pass 'default mode and Go working directory' 'verification complete (default)' verify
log_is 'go vet ./...' 'go test -mod=readonly ./...'
expect pass 'full mode and npm root directory' 'verification complete (full)' verify --full
log_is 'go vet ./...' 'go test -mod=readonly ./...' 'npm run test:ci'
verify_failure=npm
expect pass 'default mode never invokes npm' 'verification complete (default)' verify
log_is 'go vet ./...' 'go test -mod=readonly ./...'
verify_failure=
# The caller need not run from the repository root.
(cd "$scratch" && verify --full) > "$scratch/caller-output" 2>&1
expect pass 'independent of caller directory' 'verification complete' cat "$scratch/caller-output"
for arg in --repo --branch --base; do
    expect fail "verify missing option value: $arg" 'missing value' verify_args "$arg"
    log_is
    expect fail "verify empty option value: $arg" 'missing value' verify_args "$arg" ''
    log_is
done
expect fail 'verify missing required arguments' 'required:' verify_args
expect fail 'verify option used as value' 'missing value' verify_args --repo --full
expect fail 'verify unknown flag' 'unknown argument' verify --install
expect fail 'verify symbolic base' 'full commit SHA' verify_args --repo "$repo" --branch feat/test --base HEAD
expect fail 'verify unknown base' 'base is not a commit' verify_args --repo "$repo" --branch feat/test --base 0000000000000000000000000000000000000000
expect fail 'verify wrong feature branch' 'unexpected branch' verify_args --repo "$repo" --branch feat/other --base "$base"
expect fail 'verify non-root directory' 'repository root differs' verify_args --repo "$repo/scripts" --branch feat/test --base "$base"
log_is
git -C "$repo" symbolic-ref HEAD refs/heads/main
expect fail 'verify protected branch' 'protected branch' verify
log_is
verify_repo
printf x >> "$repo/app/tracked"
expect fail 'verify scope runs before tools' 'outside Phase 1 scope' verify --full
log_is
verify_repo
git -C "$repo" remote set-url origin https://example.invalid/wrong.git
expect fail 'verify origin guard' 'unexpected repository identity' verify
log_is

for failure in vet test npm; do
    verify_repo
    verify_failure=$failure
    case "$failure" in
        vet) label='go vet ./...' ;;
        test) label='go test -mod=readonly ./...' ;;
        npm) label='npm run test:ci' ;;
    esac
    expect fail "verification stops on $failure failure" "FAIL: $label" verify --full
    case "$failure" in
        vet) log_is 'go vet ./...' ;;
        test) log_is 'go vet ./...' 'go test -mod=readonly ./...' ;;
        npm) log_is 'go vet ./...' 'go test -mod=readonly ./...' 'npm run test:ci' ;;
    esac
done
verify_repo
rmdir "$repo/apps/evt-api-go"
expect fail 'missing Go directory stops verification' 'FAIL: go vet ./...' verify --full
log_is

for state in committed staged unstaged untracked; do
    verify_repo
    file=$repo/scripts/dev/$' changed\tfile\n.go'
    if [ "$state" = unstaged ]; then
        printf 'package example\n' > "$file"
        commit_all
        base=$(git -C "$repo" rev-parse HEAD)
    fi
    printf 'package example;var X=1\n' > "$file"
    case "$state" in committed) commit_all ;; staged) git -C "$repo" add -A ;; esac
    cp "$file" "$scratch/original-go"
    expect fail "formatting detects $state Go content and unusual path" 'FAIL: Go formatting' verify --full
    log_is gofmt
    expect pass "formatting preserves $state source" 'unchanged' unchanged "$file" "$scratch/original-go"
done
verify_repo
printf 'package example;var X=1\n' > "$repo/scripts/dev/staged.go"
git -C "$repo" add -A
printf 'package example\n' > "$repo/scripts/dev/staged.go"
expect fail 'unstaged fix cannot conceal bad index formatting' 'Go formatting (index)' verify
log_is gofmt
verify_repo
printf 'package example\n' > "$repo/scripts/dev/changed.go"
git -C "$repo" add -A
printf 'package example;var X=1\n' > "$repo/scripts/dev/changed.go"
expect fail 'working formatting checked after formatted index' 'Go formatting (worktree)' verify
log_is gofmt gofmt
verify_repo
printf 'package example\n' > "$repo/scripts/dev/good.go"
expect pass 'formatted new Go file passes' 'verification complete' verify
log_is gofmt 'go vet ./...' 'go test -mod=readonly ./...'
verify_failure=gofmt
expect fail 'gofmt command error stops verification' 'FAIL: Go formatting' verify
log_is gofmt
verify_failure=
printf 'not Go syntax\n' > "$repo/scripts/dev/good.go"
expect fail 'Go parse error stops verification' 'FAIL: Go formatting' verify
log_is gofmt
verify_repo
rm "$repo/scripts/dev/baseline.go"
expect pass 'deleted Go file excluded' 'verification complete' verify
log_is 'go vet ./...' 'go test -mod=readonly ./...'
verify_repo
mv "$repo/scripts/dev/baseline.go" "$repo/scripts/dev/renamed.go"
git -C "$repo" add -A
expect fail 'renamed Go file checked as addition' 'FAIL: Go formatting' verify
log_is gofmt
verify_repo
printf 'scripts/dev/ignored.go\n' > "$repo/.git/info/exclude"
printf 'not Go\n' > "$repo/scripts/dev/ignored.go"
printf 'not Go\n' > "$repo/scripts/dev/plain.txt"
expect pass 'ignored and non-Go files excluded' 'verification complete' verify
log_is 'go vet ./...' 'go test -mod=readonly ./...'

printf 'PASS: %s Phase 2 tests completed with isolated repositories and offline command doubles\n' "$test_count"

# Phase 3: only disposable repositories may be passed to start.sh.
test_count=0
start_repo() {
    new_repo
    git -C "$repo" symbolic-ref HEAD refs/heads/main
    git -C "$repo" update-ref refs/heads/main "$base"
    git -C "$repo" update-ref refs/remotes/origin/main "$base"
}
start() { bash "$scripts/start.sh" --repo "$repo" --branch feat/new "$@"; }
snapshot_start() {
    # Include index, config, refs, reflogs and worktree bytes, even ignored files.
    find "$repo" -type f -exec shasum -a 256 {} + | LC_ALL=C sort
}
start_unchanged() {
    local result=0
    snapshot_start > "$scratch/start-before"
    "$@" || result=$?
    snapshot_start > "$scratch/start-after"
    if ! cmp -s "$scratch/start-before" "$scratch/start-after"; then
        printf 'FAIL: start unexpectedly mutated repository\n'; exit 1
    fi
    return "$result"
}
assert_start_state() {
    [ "$(git -C "$repo" symbolic-ref HEAD)" = "refs/heads/$1" ] &&
        [ "$(git -C "$repo" rev-parse HEAD)" = "$base" ] &&
        [ "$(git -C "$repo" rev-parse refs/heads/main)" = "$base" ] &&
        [ "$(git -C "$repo" rev-parse refs/remotes/origin/main)" = "$base" ] &&
        printf 'expected branch and SHA\n'
}

start_repo
expect pass 'start defaults to preview without writes' "Base commit SHA: $base" start_unchanged start
expect pass 'preview prints separate execution invocation' "--execute --base $base" start_unchanged start --preview
expect pass 'preview keeps main and refs unchanged' 'expected branch and SHA' assert_start_state main
expect pass 'start executes from reviewed SHA' 'Created branch: feat/new' start --execute --base "$base"
cp "$scratch/output" "$scratch/start-result"
expect pass 'execution reports resulting SHA' "Commit SHA: $base" cat "$scratch/start-result"
expect pass 'execution checks out requested branch at base' 'expected branch and SHA' assert_start_state feat/new
start_repo
expect pass 'feature prefix execution' 'Created branch: feature/new' bash "$scripts/start.sh" --repo "$repo" --branch feature/new --execute --base "$base"

start_repo
for args in --repo --branch --base; do
    expect fail "start missing value $args" 'missing value' start_unchanged bash "$scripts/start.sh" "$args"
    expect fail "start empty value $args" 'missing value' start_unchanged bash "$scripts/start.sh" "$args" ''
done
expect fail 'start missing required args' 'required:' start_unchanged bash "$scripts/start.sh"
expect fail 'start option as value' 'missing value' start_unchanged bash "$scripts/start.sh" --repo --execute
expect fail 'start execution needs reviewed SHA' 'reviewed full commit SHA' start_unchanged start --execute
expect fail 'start symbolic SHA' 'reviewed full commit SHA' start_unchanged start --execute --base HEAD
expect fail 'start abbreviated SHA' 'reviewed full commit SHA' start_unchanged start --execute --base "${base:0:12}"
expect fail 'start preview cannot imply execution' 'separately invoked' start_unchanged start --base "$base"
expect fail 'start conflicting modes' 'choose only one mode' start_unchanged start --preview --execute --base "$base"
expect fail 'start duplicate arguments' 'duplicate --branch' start_unchanged start --branch feat/other
expect fail 'start unknown flag' 'unknown argument' start_unchanged start --force
for invalid in main bugfix/a feat/ feature/ feat/a..b 'feat/a b' 'feat/a~b' 'feat/a^b' 'feat/a:b' 'feat/a?b' 'feat/a*b' 'feat/a[b' 'feat/a\b' feat/.hidden feat/a.lock feat/a/ feat/a//b 'feat/a@{b' $'feat/a\nb'; do
    expect fail "start rejects invalid branch: $invalid" 'FAIL:' start_unchanged \
        bash "$scripts/start.sh" --repo "$repo" --branch "$invalid" --execute --base "$base"
done

for mode in preview execute; do
    start_repo
    start_mode() {
        if [ "$mode" = execute ]; then start --execute --base "$base"; else start; fi
    }
    git -C "$repo" update-ref refs/heads/feat/new "$base"
    expect fail "start existing branch: $mode" 'already exists' start_unchanged start_mode
    start_repo
    git -C "$repo" update-ref --no-deref HEAD "$base"
    expect fail "start detached HEAD: $mode" 'detached HEAD' start_unchanged start_mode
    start_repo
    git -C "$repo" symbolic-ref HEAD refs/heads/feat/test
    expect fail "start wrong current branch: $mode" 'expected current branch main' start_unchanged start_mode
    start_repo
    git -C "$repo" remote set-url origin https://example.invalid/wrong.git
    expect fail "start wrong identity: $mode" 'unexpected repository identity' start_unchanged start_mode
    start_repo
    git -C "$repo" remote remove origin
    expect fail "start missing origin: $mode" 'origin is missing' start_unchanged start_mode
    start_repo
    git -C "$repo" update-ref -d refs/remotes/origin/main
    expect fail "start missing origin/main: $mode" 'origin/main is missing' start_unchanged start_mode
    for state in staged unstaged untracked; do
        start_repo
        case "$state" in
            staged) printf x >> "$repo/app/tracked"; git -C "$repo" add -u ;;
            unstaged) printf x >> "$repo/app/tracked" ;;
            untracked) printf x > "$repo/"$' odd\tfile\n' ;;
        esac
        expect fail "start dirty $state: $mode" 'baseline is not clean' start_unchanged start_mode
    done
    for operation in MERGE_HEAD CHERRY_PICK_HEAD REVERT_HEAD REBASE_HEAD rebase-merge rebase-apply sequencer BISECT_START index.lock HEAD.lock packed-refs.lock; do
        start_repo
        touch "$repo/.git/$operation"
        expect fail "start active operation $operation: $mode" 'Git operation or lock' start_unchanged start_mode
    done
    for flag in --assume-unchanged --skip-worktree; do
        start_repo
        git -C "$repo" update-index "$flag" app/tracked
        printf x >> "$repo/app/tracked"
        expect fail "start hidden changes $flag: $mode" 'unsupported index flags' start_unchanged start_mode
    done
done

start_repo
expect fail 'start subdirectory root rejected' 'repository root differs' start_unchanged \
    bash "$scripts/start.sh" --repo "$repo/scripts" --branch feat/new
expect fail 'start Git environment redirection' 'Git environment override' start_unchanged \
    env GIT_INDEX_FILE="$scratch/alternate-index" bash "$scripts/start.sh" --repo "$repo" --branch feat/new
expect fail 'start nonexistent root' 'repository directory unavailable' start_unchanged \
    bash "$scripts/start.sh" --repo "$scratch/missing" --branch feat/new
mkdir "$scratch/not-a-repository"
expect fail 'start non-repository root' 'not a Git worktree' start_unchanged \
    bash "$scripts/start.sh" --repo "$scratch/not-a-repository" --branch feat/new
start_repo
expect pass 'preview before stale SHA scenario' "$base" start_unchanged start
reviewed=$base
printf x >> "$repo/app/tracked"
commit_all
expect fail 'start moved main with stale remote' 'main and origin/main do not match' start_unchanged start --execute --base "$reviewed"
git -C "$repo" update-ref refs/remotes/origin/main HEAD
expect fail 'start both refs advanced after preview' 'reviewed SHA' start_unchanged start --execute --base "$reviewed"
expect fail 'start unknown full SHA' 'reviewed SHA' start_unchanged start --execute --base 0000000000000000000000000000000000000000
start_repo
# Use a distinct commit to model an independently moved tracking ref.
ahead=$(printf 'remote advance\n' | git -C "$repo" -c commit.gpgsign=false commit-tree "$base^{tree}" -p "$base")
git -C "$repo" update-ref refs/remotes/origin/main "$ahead"
expect fail 'start tracking ref mismatch' 'main and origin/main do not match' start_unchanged start --execute --base "$base"

start_repo
ignore_rules 'node_modules/' 'scripts/dev/cache/'
git -C "$repo" update-ref refs/remotes/origin/main "$base"
mkdir -p "$repo/node_modules/pkg" "$repo/scripts/dev/cache"
printf x > "$repo/node_modules/pkg/index.js"
printf x > "$repo/scripts/dev/cache/"$' odd\tartifact\n'
expect pass 'start preview ignores workspace artifacts' 'Preview only' start_unchanged start
expect pass 'start execution ignores workspace artifacts' 'Created branch: feat/new' start --execute --base "$base"
expect pass 'start ignored artifact preserved' 'x' cat "$repo/node_modules/pkg/index.js"

# Inject failures into each critical Git command, including the sole mutation.
for command in config symbolic-ref rev-parse check-ref-format show-ref ls-files status switch; do
    start_repo
    expect fail "start Git failure: $command" 'FAIL:' start_unchanged \
        env PATH="$scratch/bin:$PATH" CVERA_TEST_REAL_GIT="$real_git" CVERA_TEST_FAIL_GIT_COMMAND="$command" \
        bash "$scripts/start.sh" --repo "$repo" --branch feat/new --execute --base "$base"
done
start_repo
expect pass 'start clean check requests NUL status' 'Preview only' start_unchanged \
    env PATH="$scratch/bin:$PATH" CVERA_TEST_REAL_GIT="$real_git" CVERA_TEST_REQUIRE_STATUS_NUL=1 \
    bash "$scripts/start.sh" --repo "$repo" --branch feat/new

# Hooks and tracking defaults must not cause extra writes during creation.
start_repo
git -C "$repo" config branch.autoSetupMerge always
git -C "$repo" config branch.autoSetupRebase always
printf '#!/bin/sh\nprintf hook > "%s/hook-ran"\n' "$repo" > "$repo/.git/hooks/post-checkout"
chmod +x "$repo/.git/hooks/post-checkout"
cp "$repo/.git/config" "$scratch/start-config"
expect pass 'start disables checkout hooks' 'Created branch: feat/new' start --execute --base "$base"
expect pass 'start leaves Git config unchanged' 'unchanged' unchanged "$repo/.git/config" "$scratch/start-config"
if [ -e "$repo/hook-ran" ]; then printf 'FAIL: checkout hook ran\n'; exit 1; fi
expect pass 'start hooked repository resulting state' 'expected branch and SHA' assert_start_state feat/new
start_repo
(cd "$scratch" && start) > "$scratch/start-caller" 2>&1
expect pass 'start independent of caller directory' 'Preview only' cat "$scratch/start-caller"

# An error during the final revalidation must stop before branch creation.
start_repo
mkdir -p "$scratch/start-late-bin"
cat > "$scratch/start-late-bin/git" <<'LATE'
#!/bin/bash
for argument in "$@"; do
    case "$argument" in
        status)
            if [ -e "$CVERA_TEST_STATUS_SEEN" ]; then
                printf 'injected final validation failure\n' >&2; exit 128
            fi
            touch "$CVERA_TEST_STATUS_SEEN" ;;
        switch) printf 'FAIL: mutation after failed validation\n' >&2; exit 128 ;;
    esac
done
exec "$CVERA_TEST_REAL_GIT" "$@"
LATE
chmod +x "$scratch/start-late-bin/git"
expect fail 'start revalidates before mutation' 'cannot inspect baseline status' start_unchanged \
    env PATH="$scratch/start-late-bin:$PATH" CVERA_TEST_REAL_GIT="$real_git" CVERA_TEST_STATUS_SEEN="$scratch/start-status-seen" \
    bash "$scripts/start.sh" --repo "$repo" --branch feat/new --execute --base "$base"
expect pass 'start failed final validation leaves main' 'expected branch and SHA' assert_start_state main

# A valid ref name may still conflict with a parent branch in the ref store.
start_repo
git -C "$repo" update-ref refs/heads/feature "$base"
expect fail 'start branch namespace conflict fails without writes' 'branch creation failed' start_unchanged \
    bash "$scripts/start.sh" --repo "$repo" --branch feature/new --execute --base "$base"

printf 'PASS: %s Phase 3 tests completed in isolated temporary repositories\n' "$test_count"

# Phase 4: actual scope and verification scripts; Go/npm remain offline doubles.
# The Git double permits only read commands and injects enumeration failures
# specifically into preparation, after scope and verification have succeeded.
test_count=0
prepare_bin=$scratch/prepare-bin
mkdir -p "$prepare_bin"
cat > "$prepare_bin/git" <<'PREPARE_GIT'
#!/bin/bash
set -eu
[ "$GIT_NO_LAZY_FETCH:$GIT_NO_REPLACE_OBJECTS:$GIT_TERMINAL_PROMPT" = 1:1:0 ] || exit 128
args=("$@")
while [ "$#" -gt 0 ]; do
    case "$1" in
        --no-optional-locks) shift ;;
        -c|-C) shift 2 ;;
        *) break ;;
    esac
done
command=$1
case "$command" in
    rev-parse|symbolic-ref|merge-base|ls-files|ls-tree|diff|check-ignore) ;;
    config) [ "${2:-}" = --get-all ] || { echo 'FAIL: configuration write attempted'; exit 128; } ;;
    *) echo "FAIL: unexpected Git command: $command"; exit 128 ;;
esac
if [ "$command" = diff ]; then
    case " $* " in
        *' --name-status '*)
            layer=committed
            case " $* " in
                *' --cached '*) layer=staged ;;
                *' --ignore-submodules=none -- '*) layer=unstaged ;;
            esac
            if [ "${CVERA_TEST_ENUM_FAIL:-}" = "$layer" ]; then
                echo 'injected preparation enumeration failure' >&2; exit 128
            fi
            if [ "${CVERA_TEST_ENUM_BAD:-}" = "$layer" ]; then
                printf 'M\000scripts/dev/incomplete'; exit 0
            fi
            if [ "$layer" = unstaged ]; then : > "$CVERA_TEST_ENUM_READY"; fi ;;
    esac
fi
if [ "$command" = ls-files ] && [ -e "$CVERA_TEST_ENUM_READY" ]; then
    if [ "${CVERA_TEST_ENUM_FAIL:-}" = untracked ]; then
        echo 'injected preparation enumeration failure' >&2; exit 128
    fi
    if [ "${CVERA_TEST_ENUM_BAD:-}" = untracked ]; then
        printf 'scripts/dev/incomplete'; exit 0
    fi
fi
exec "$CVERA_TEST_REAL_GIT" "${args[@]}"
PREPARE_GIT
chmod +x "$prepare_bin/git"
prepare_enum_fail= prepare_enum_bad=
prepare_args() {
    : > "$scratch/verify-log"
    # Remove only an outside-repository test-double marker from the last call.
    rm -f "$scratch/prepare-enum-ready"
    env PATH="$prepare_bin:$verify_bin:$PATH" CVERA_TEST_REAL_GIT="$real_git" \
        CVERA_TEST_LOG="$scratch/verify-log" CVERA_TEST_REPO="$repo" \
        CVERA_TEST_FAIL="$verify_failure" CVERA_TEST_ENUM_FAIL="$prepare_enum_fail" \
        CVERA_TEST_ENUM_BAD="$prepare_enum_bad" CVERA_TEST_ENUM_READY="$scratch/prepare-enum-ready" \
        bash "$scripts/prepare.sh" "$@"
}
prepare() { prepare_args --repo "$repo" --branch feat/test --base "$base" "$@"; }
prepare_unchanged() {
    local result=0
    snapshot_start > "$scratch/prepare-before"
    "$@" || result=$?
    snapshot_start > "$scratch/prepare-after"
    if ! cmp -s "$scratch/prepare-before" "$scratch/prepare-after"; then
        printf 'FAIL: preparation mutated repository\n'; exit 1
    fi
    return "$result"
}
no_proposal() {
    if /usr/bin/grep -Fq 'Proposed conventional commit message:' "$scratch/output"; then
        printf 'FAIL: proposal printed after preparation failure\n'; exit 1
    fi
}
output_has() { expect pass "$1" "$2" cat "$scratch/prepare-result"; }
output_lacks() {
    if /usr/bin/grep -Fq -- "$2" "$scratch/prepare-result"; then
        printf 'FAIL: %s\n' "$1"; exit 1
    fi
    test_count=$((test_count + 1))
    printf 'PASS test: %s\n' "$1"
}

verify_repo
expect pass 'prepare default verification and no repository writes' \
    'Change summary: committed=0 staged=0 unstaged=0 non-ignored-untracked=0' prepare_unchanged prepare
log_is 'go vet ./...' 'go test -mod=readonly ./...'
cp "$scratch/output" "$scratch/prepare-result"
for section in 'Purpose and scope' 'Files changed' 'Verification evidence' 'Explicit limitations' 'Human review checklist'; do
    output_has "prepare PR section: $section" "$section"
done
output_has 'prepare full base SHA' "Base SHA: $base"
output_has 'prepare current HEAD SHA' "Current HEAD SHA: $base"
output_has 'prepare conventional message' 'chore(dev): update developer workflow automation'
output_has 'prepare PR title' 'Proposed PR title:'
output_has 'prepare default limitation' 'npm run test:ci was not run'
output_has 'prepare authority statement' 'GitHub CI and deployment checks remain authoritative'
output_has 'prepare snapshot limitation' 'staged and historical snapshots are not independently built or tested'
expect pass 'prepare full verification without writes' 'PASS: npm run test:ci (--full, offline mode)' prepare_unchanged prepare --full
log_is 'go vet ./...' 'go test -mod=readonly ./...' 'npm run test:ci'
(cd "$scratch" && prepare_unchanged prepare) > "$scratch/prepare-caller" 2>&1
expect pass 'prepare independent of caller directory' 'Proposed PR description:' cat "$scratch/prepare-caller"

for option in --repo --branch --base; do
    expect fail "prepare missing value: $option" 'missing value' prepare_unchanged prepare_args "$option"
    log_is; no_proposal
    expect fail "prepare empty value: $option" 'missing value' prepare_unchanged prepare_args "$option" ''
    log_is; no_proposal
    expect fail "prepare duplicate option: $option" "duplicate $option" prepare_unchanged prepare "$option" duplicate
    log_is; no_proposal
done
expect fail 'prepare missing required arguments' 'required:' prepare_unchanged prepare_args
log_is; no_proposal
expect fail 'prepare flag as value' 'missing value' prepare_unchanged prepare_args --repo --full
log_is; no_proposal
expect fail 'prepare unknown flag' 'unknown argument' prepare_unchanged prepare --execute
log_is; no_proposal
expect fail 'prepare duplicate full' 'duplicate --full' prepare_unchanged prepare --full --full
log_is; no_proposal
for invalid in HEAD "${base:0:12}" 0000000000000000000000000000000000000000; do
    expect fail "prepare invalid base: $invalid" 'FAIL:' prepare_unchanged prepare_args --repo "$repo" --branch feat/test --base "$invalid"
    log_is; no_proposal
done
expect fail 'prepare wrong branch' 'unexpected branch' prepare_unchanged prepare_args --repo "$repo" --branch feat/other --base "$base"
log_is; no_proposal
expect fail 'prepare wrong root' 'repository root differs' prepare_unchanged prepare_args --repo "$repo/scripts" --branch feat/test --base "$base"
log_is; no_proposal
expect fail 'prepare environment override rejected' 'Git environment override' prepare_unchanged \
    env GIT_CONFIG=/dev/null bash "$scripts/prepare.sh" --repo "$repo" --branch feat/test --base "$base"
no_proposal

for state in committed staged unstaged untracked; do
    verify_repo
    case "$state" in
        committed) printf x >> "$repo/app/tracked"; commit_all ;;
        staged) printf x >> "$repo/app/tracked"; git -C "$repo" add -u ;;
        unstaged) printf x >> "$repo/app/tracked" ;;
        untracked) printf x > "$repo/app/new" ;;
    esac
    expect fail "prepare scope rejects $state before verification" 'outside Phase 1 scope' prepare_unchanged prepare --full
    log_is; no_proposal
done
for failure in vet test npm; do
    verify_repo
    verify_failure=$failure
    expect fail "prepare stops on verification $failure" 'verification failed; preparation stopped' prepare_unchanged prepare --full
    case "$failure" in
        vet) log_is 'go vet ./...' ;;
        test) log_is 'go vet ./...' 'go test -mod=readonly ./...' ;;
        npm) log_is 'go vet ./...' 'go test -mod=readonly ./...' 'npm run test:ci' ;;
    esac
    no_proposal
done
verify_repo
printf 'package example;var X=1\n' > "$repo/scripts/dev/unformatted.go"
expect fail 'prepare stops on formatting failure' 'Go formatting' prepare_unchanged prepare
log_is gofmt; no_proposal

# Check exact counts and escaped literal paths, including overlap across layers,
# net-reverted committed work, deletions and renames. Ignored artifacts stay out.
verify_repo
ignore_rules 'scripts/dev/cache/' 'node_modules/' 'scripts/dev/tracked'
odd_prepare=$' leading \tline\n"quote" \\ *? [glob] \033 tail '
printf x > "$repo/scripts/dev/$odd_prepare"
printf 'committed\n' > "$repo/scripts/dev/committed"
printf 'changed\n' >> "$repo/scripts/dev/tracked"
commit_all
head=$(git -C "$repo" rev-parse HEAD)
mv "$repo/scripts/dev/$odd_prepare" "$repo/scripts/dev/${odd_prepare} renamed"
git -C "$repo" add -A
# Restore tracked to base in the index: the committed and staged changes must
# both remain visible even when their combined change cancels out.
git -C "$repo" show "$base:scripts/dev/tracked" > "$repo/scripts/dev/tracked"
git -C "$repo" add -u
printf y >> "$repo/scripts/dev/tracked"
rm "$repo/scripts/dev/committed"
printf new > "$repo/scripts/dev/${odd_prepare} untracked"
mkdir -p "$repo/scripts/dev/cache" "$repo/node_modules/pkg"
printf secret > "$repo/scripts/dev/cache/hidden"
printf secret > "$repo/node_modules/pkg/hidden"
expect pass 'prepare accurate mixed-layer counts and read-only snapshots' \
    'Change summary: committed=3 staged=3 unstaged=2 non-ignored-untracked=1' prepare_unchanged prepare
log_is 'go vet ./...' 'go test -mod=readonly ./...'
cp "$scratch/output" "$scratch/prepare-result"
output_has 'prepare distinguishes HEAD from base' "Current HEAD SHA: $head"
output_has 'prepare committed addition literal odd filename' "$(printf '  A %q' "scripts/dev/$odd_prepare")"
output_has 'prepare staged rename original deletion' "$(printf '  D %q' "scripts/dev/$odd_prepare")"
output_has 'prepare staged rename destination addition' "$(printf '  A %q' "scripts/dev/${odd_prepare} renamed")"
output_has 'prepare untracked literal odd filename' "$(printf '  ? %q' "scripts/dev/${odd_prepare} untracked")"
output_has 'prepare unstaged deletion' '  D scripts/dev/committed'
output_has 'prepare tracked file matching ignore rule' '  M scripts/dev/tracked'
output_lacks 'prepare omits ignored in-scope artifact' 'scripts/dev/cache/hidden'
output_lacks 'prepare omits ignored dependency artifact' 'node_modules/pkg/hidden'
# TMPDIR inside the target must not place temporary artifacts there.
prepare_bad_tmpdir() { (export TMPDIR="$repo/scripts/dev"; prepare_unchanged prepare); }
expect pass 'prepare keeps scratch outside repository with hostile TMPDIR' 'Proposed PR title:' prepare_bad_tmpdir

for layer in committed staged unstaged untracked; do
    verify_repo
    prepare_enum_fail=$layer
    expect fail "prepare enumeration command failure: $layer" 'cannot enumerate' prepare_unchanged prepare
    log_is 'go vet ./...' 'go test -mod=readonly ./...'
    no_proposal
    prepare_enum_fail=
    prepare_enum_bad=$layer
    expect fail "prepare incomplete enumeration record: $layer" 'incomplete' prepare_unchanged prepare
    no_proposal
    prepare_enum_bad=
done

printf 'PASS: %s Phase 4 tests completed in isolated repositories with offline command doubles\n' "$test_count"

# Phase 5: local-only synchronization. Every invocation uses an offline Git
# allowlist double; unexpected commands fail before reaching real Git.
test_count=0
sync_bin=$scratch/sync-bin
mkdir -p "$sync_bin"
cat > "$sync_bin/git" <<'SYNC_GIT'
#!/bin/bash
set -eu
[ "$GIT_NO_LAZY_FETCH:$GIT_NO_REPLACE_OBJECTS:$GIT_TERMINAL_PROMPT" = 1:1:0 ] || exit 128
[ "${GIT_ALLOW_PROTOCOL-unset}" = '' ] || exit 128
args=("$@")
while [ "$#" -gt 0 ]; do
    case "$1" in
        --no-optional-locks) shift ;;
        -c|-C) shift 2 ;;
        *) break ;;
    esac
done
command=$1
printf '%s\n' "$command" >> "$CVERA_TEST_SYNC_LOG"
case "$command" in
    rev-parse|symbolic-ref|merge-base|ls-files|status) ;;
    config)
        case "${2:-}" in --get-all|--get-regexp) ;; *) exit 128 ;; esac ;;
    merge)
        case " ${args[*]} " in *' core.hooksPath=/dev/null '*) ;; *) exit 128 ;; esac
        case " ${args[*]} " in *' submodule.recurse=false '*) ;; *) exit 128 ;; esac
        case " $* " in *' --ff-only --no-edit --no-stat --no-autostash --no-overwrite-ignore '*) ;; *) exit 128 ;; esac
        [ "${CVERA_TEST_SYNC_FAKE_MERGE:-}" != 1 ] || exit 0 ;;
    *) printf 'FAIL: unexpected Git command: %s\n' "$command"; exit 128 ;;
esac
if [ "${CVERA_TEST_SYNC_FAIL:-}" = "$command" ]; then
    printf 'injected sync Git failure\n' >&2; exit 128
fi
if [ "$command" = status ]; then
    case " $* " in *' -z '*) ;; *) exit 128 ;; esac
    if [ -e "$CVERA_TEST_SYNC_SEEN" ]; then
        case "${CVERA_TEST_SYNC_LATE:-}" in
            fail) printf 'injected final validation failure\n' >&2; exit 128 ;;
            stale) "$CVERA_TEST_REAL_GIT" -C "$CVERA_TEST_SYNC_REPO" update-ref refs/remotes/origin/main "$CVERA_TEST_SYNC_BASE" ;;
            dirty) printf changed >> "$CVERA_TEST_SYNC_REPO/app/tracked" ;;
        esac
    fi
    : > "$CVERA_TEST_SYNC_SEEN"
fi
exec "$CVERA_TEST_REAL_GIT" "${args[@]}"
SYNC_GIT
chmod +x "$sync_bin/git"
sync_fail= sync_late= sync_fake_merge=
sync_repo() {
    start_repo
    # Create the target on a separate branch; main stays at the baseline.
    git -C "$repo" checkout -qb fixture-target
    printf 'merged work\n' >> "$repo/app/tracked"
    commit_all
    target=$(git -C "$repo" rev-parse HEAD)
    git -C "$repo" checkout -q main
    git -C "$repo" update-ref refs/remotes/origin/main "$target"
}
sync_args() {
    : > "$scratch/sync-log"
    rm -f "$scratch/sync-seen"
    env PATH="$sync_bin:$PATH" CVERA_TEST_REAL_GIT="$real_git" \
        CVERA_TEST_SYNC_LOG="$scratch/sync-log" CVERA_TEST_SYNC_SEEN="$scratch/sync-seen" \
        CVERA_TEST_SYNC_FAIL="$sync_fail" CVERA_TEST_SYNC_LATE="$sync_late" \
        CVERA_TEST_SYNC_FAKE_MERGE="$sync_fake_merge" \
        CVERA_TEST_SYNC_REPO="$repo" CVERA_TEST_SYNC_BASE="$base" \
        bash "$scripts/sync.sh" "$@"
}
sync_main() { sync_args --repo "$repo" "$@"; }
sync_unchanged() {
    local result=0
    snapshot_start > "$scratch/sync-before"
    "$@" || result=$?
    snapshot_start > "$scratch/sync-after"
    if ! cmp -s "$scratch/sync-before" "$scratch/sync-after"; then
        printf 'FAIL: sync unexpectedly mutated repository\n'; exit 1
    fi
    return "$result"
}
sync_no_merge() {
    if /usr/bin/grep -Fxq merge "$scratch/sync-log"; then
        printf 'FAIL: unnecessary or unsafe merge attempted\n'; exit 1
    fi
}
assert_sync_state() {
    [ "$(git -C "$repo" symbolic-ref HEAD)" = refs/heads/main ] &&
        [ "$(git -C "$repo" rev-parse HEAD)" = "$target" ] &&
        [ "$(git -C "$repo" rev-parse refs/heads/main)" = "$target" ] &&
        [ "$(git -C "$repo" rev-parse refs/remotes/origin/main)" = "$target" ] &&
        [ -z "$(git -C "$repo" status --porcelain --untracked-files=all)" ] &&
        printf 'main and HEAD equal target, clean\n'
}

sync_repo
expect pass 'sync default preview has current SHA and no writes' "Current main SHA: $base" sync_unchanged sync_main
sync_no_merge
expect pass 'sync preview prints exact target and execution command' "--execute --target $target" sync_unchanged sync_main --preview
sync_no_merge
(cd "$scratch" && sync_unchanged sync_main) > "$scratch/sync-caller" 2>&1
expect pass 'sync independent of caller directory' 'Preview only' cat "$scratch/sync-caller"
sync_inside_tmpdir() { (export TMPDIR="$repo/scripts/dev"; sync_unchanged sync_main); }
expect pass 'sync hostile TMPDIR does not cause repository writes' 'Preview only' sync_inside_tmpdir
expect pass 'sync approved fast-forward succeeds' 'Synchronized local main and HEAD' sync_main --execute --target "$target"
expect pass 'sync resulting refs and worktree' 'main and HEAD equal target' assert_sync_state
expect pass 'sync already synchronized preview has no writes' 'no update needed' sync_unchanged sync_main
sync_no_merge
expect pass 'sync already synchronized execution has no writes' 'no update needed' sync_unchanged sync_main --execute --target "$target"
sync_no_merge
expect fail 'sync already synchronized still requires matching approval' 'approved target SHA' sync_unchanged sync_main --execute --target "$base"

for option in --repo --target; do
    expect fail "sync missing value $option" 'missing value' sync_unchanged sync_args "$option"
    expect fail "sync empty value $option" 'missing value' sync_unchanged sync_args "$option" ''
    expect fail "sync option as value $option" 'missing value' sync_unchanged sync_args "$option" --execute
done
expect fail 'sync missing root' 'required:' sync_unchanged sync_args
expect fail 'sync duplicate repo' 'duplicate --repo' sync_unchanged sync_main --repo "$repo"
expect fail 'sync duplicate target' 'duplicate --target' sync_unchanged sync_main --execute --target "$target" --target "$target"
expect fail 'sync conflicting modes' 'choose only one mode' sync_unchanged sync_main --preview --execute
expect fail 'sync duplicate mode' 'choose only one mode' sync_unchanged sync_main --execute --execute
expect fail 'sync unknown argument' 'unknown argument' sync_unchanged sync_main --fetch
expect fail 'sync target requires execution' 'separately invoked' sync_unchanged sync_main --target "$target"
for invalid in '' HEAD "${target:0:12}" 0000000000000000000000000000000000000000; do
    if [ -z "$invalid" ]; then
        expect fail 'sync execute requires target' 'full commit SHA' sync_unchanged sync_main --execute
    else
        expect fail "sync invalid approval $invalid" 'FAIL:' sync_unchanged sync_main --execute --target "$invalid"
    fi
done

for mode in preview execute; do
    sync_mode() {
        if [ "$mode" = execute ]; then sync_main --execute --target "$target"; else sync_main; fi
    }
    for state in staged unstaged untracked; do
        sync_repo
        case "$state" in
            staged) printf x >> "$repo/app/tracked"; git -C "$repo" add -u ;;
            unstaged) printf x >> "$repo/app/tracked" ;;
            untracked) printf x > "$repo/"$' odd\tfile\n' ;;
        esac
        expect fail "sync dirty $state: $mode" 'baseline is not clean' sync_unchanged sync_mode
        sync_no_merge
    done
    for branch in feat/test master; do
        sync_repo
        git -C "$repo" checkout -q -B "$branch" "$base"
        expect fail "sync unexpected branch $branch: $mode" 'expected current branch main' sync_unchanged sync_mode
    done
    sync_repo
    git -C "$repo" checkout -q --detach
    expect fail "sync detached: $mode" 'detached HEAD' sync_unchanged sync_mode
    sync_repo
    git -C "$repo" remote set-url origin https://example.invalid/wrong.git
    expect fail "sync wrong identity: $mode" 'unexpected repository identity' sync_unchanged sync_mode
    sync_repo
    git -C "$repo" remote remove origin
    expect fail "sync missing origin: $mode" 'origin is missing' sync_unchanged sync_mode
    for ref in refs/heads/main refs/remotes/origin/main; do
        sync_repo
        git -C "$repo" update-ref -d "$ref"
        expect fail "sync missing ref $ref: $mode" 'FAIL:' sync_unchanged sync_mode
    done
    sync_repo
    git -C "$repo" symbolic-ref refs/remotes/origin/main refs/heads/fixture-target
    expect fail "sync symbolic tracking ref: $mode" 'unexpected symbolic ref' sync_unchanged sync_mode
    for operation in MERGE_HEAD CHERRY_PICK_HEAD REVERT_HEAD REBASE_HEAD rebase-merge rebase-apply sequencer BISECT_START index.lock HEAD.lock packed-refs.lock refs/heads/main.lock refs/remotes/origin/main.lock; do
        sync_repo
        touch "$repo/.git/$operation"
        expect fail "sync active operation $operation: $mode" 'Git operation or lock' sync_unchanged sync_mode
    done
    for flag in --assume-unchanged --skip-worktree; do
        sync_repo
        git -C "$repo" update-index "$flag" app/tracked
        expect fail "sync unsupported index $flag: $mode" 'unsupported index flags' sync_unchanged sync_mode
    done
    sync_repo
    printf 'local divergence\n' > "$repo/local"
    commit_all
    expect fail "sync divergence: $mode" 'not an ancestor' sync_unchanged sync_mode
    sync_repo
    # Local main ahead of the tracking ref must never be rolled back.
    git -C "$repo" checkout -q fixture-target
    git -C "$repo" branch -f main "$target"
    git -C "$repo" checkout -q main
    git -C "$repo" update-ref refs/remotes/origin/main "$base"
    target=$base
    expect fail "sync rollback: $mode" 'not an ancestor' sync_unchanged sync_mode
done

sync_repo
expect fail 'sync root must be top-level' 'repository root differs' sync_unchanged sync_args --repo "$repo/scripts"
for variable in GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_COMMON_DIR GIT_NAMESPACE GIT_CONFIG GIT_CONFIG_COUNT GIT_CONFIG_PARAMETERS GIT_SHALLOW_FILE GIT_REPLACE_REF_BASE; do
    expect fail "sync rejects environment $variable" 'Git environment override' sync_unchanged \
        env "$variable=" bash "$scripts/sync.sh" --repo "$repo"
done
expect fail 'sync stale approval' 'approved target SHA' sync_unchanged sync_main --execute --target "$base"
git -C "$repo" remote set-url origin git@github.com:kola-white/rsa-attestation-engine.git
expect pass 'sync accepts SSH identity without network' 'Preview only' sync_unchanged sync_main

for command in config symbolic-ref rev-parse ls-files status merge-base merge; do
    sync_repo
    sync_fail=$command
    expect fail "sync Git command failure $command" 'FAIL:' sync_unchanged sync_main --execute --target "$target"
    sync_fail=
done
sync_repo
sync_fake_merge=1
expect fail 'sync detects successful merge without expected result' 'unexpected result' sync_unchanged sync_main --execute --target "$target"
sync_fake_merge=
for late in fail stale dirty; do
    sync_repo
    sync_late=$late
    expect fail "sync revalidates immediately before mutation: $late" 'FAIL:' sync_main --execute --target "$target"
    sync_no_merge
    expect pass "sync failed revalidation leaves main: $late" "$base" git -C "$repo" rev-parse refs/heads/main
    sync_late=
done

# Ignored artifacts survive normal updates, and are protected against target
# collisions. Also exercise a real failed ff-only operation rather than a stub.
sync_repo
printf 'node_modules/\n' > "$repo/.git/info/exclude"
mkdir -p "$repo/node_modules/pkg"
printf artifact > "$repo/node_modules/pkg/index.js"
expect pass 'sync ignored artifacts permit preview without writes' 'Preview only' sync_unchanged sync_main
expect pass 'sync ignored artifacts permit execution' 'Synchronized local main' sync_main --execute --target "$target"
expect pass 'sync ignored artifact preserved' 'artifact' cat "$repo/node_modules/pkg/index.js"
sync_repo
git -C "$repo" checkout -q fixture-target
printf incoming > "$repo/collision"
commit_all
target=$(git -C "$repo" rev-parse HEAD)
git -C "$repo" checkout -q main
git -C "$repo" update-ref refs/remotes/origin/main "$target"
printf 'collision\n' > "$repo/.git/info/exclude"
printf artifact > "$repo/collision"
expect fail 'sync actual fast-forward failure protects ignored collision' 'fast-forward failed' sync_main --execute --target "$target"
expect pass 'sync failed ff retains main' "$base" git -C "$repo" rev-parse refs/heads/main
expect pass 'sync failed ff retains ignored content' 'artifact' cat "$repo/collision"

sync_repo
git -C "$repo" config merge.autoStash true
git -C "$repo" config submodule.recurse true
git -C "$repo" config maintenance.auto true
git -C "$repo" config merge.verifySignatures true
printf '#!/bin/sh\nprintf hook > "%s/hook-ran"\n' "$repo" > "$repo/.git/hooks/post-merge"
chmod +x "$repo/.git/hooks/post-merge"
cp "$repo/.git/config" "$scratch/sync-config"
expect pass 'sync disables hooks and overrides unwanted defaults' 'Synchronized local main' sync_main --execute --target "$target"
expect pass 'sync config remains unchanged' 'unchanged' unchanged "$repo/.git/config" "$scratch/sync-config"
if [ -e "$repo/hook-ran" ]; then printf 'FAIL: sync hook ran\n'; exit 1; fi
expect pass 'sync with configured defaults reaches clean target' 'main and HEAD equal target' assert_sync_state

for filter in clean smudge process; do
    sync_repo
    git -C "$repo" config "filter.offline-test.$filter" "touch '$repo/filter-ran'"
    expect fail "sync rejects external filter $filter before invocation" 'external Git filters' sync_unchanged sync_main --execute --target "$target"
    sync_no_merge
    if [ -e "$repo/filter-ran" ]; then printf 'FAIL: external filter ran\n'; exit 1; fi
done

printf 'PASS: %s Phase 5 tests completed in isolated repositories with offline command doubles\n' "$test_count"
