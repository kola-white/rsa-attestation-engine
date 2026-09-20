#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
API_DIR="${REPO_ROOT}/apps/evt-api-go"

if [[ $# -ne 1 ]]; then
  echo "Usage: $0 <40-character-git-sha>" >&2
  exit 2
fi

DEPLOY_SHA="$1"

if [[ ! "${DEPLOY_SHA}" =~ ^[0-9a-f]{40}$ ]]; then
  echo "ERROR: deployment revision must be a full 40-character lowercase Git SHA" >&2
  exit 2
fi

cd "${REPO_ROOT}"

if [[ -n "$(git status --porcelain)" ]]; then
  echo "ERROR: repository worktree is not clean; refusing deployment" >&2
  exit 1
fi

echo "Fetching authoritative origin/main..."
git fetch origin main:refs/remotes/origin/main

if ! git cat-file -e "${DEPLOY_SHA}^{commit}" 2>/dev/null; then
  echo "ERROR: deployment revision does not exist: ${DEPLOY_SHA}" >&2
  exit 1
fi

ORIGIN_MAIN_SHA="$(git rev-parse origin/main)"

if [[ "${DEPLOY_SHA}" != "${ORIGIN_MAIN_SHA}" ]]; then
  echo "ERROR: deployment revision does not match current origin/main" >&2
  echo "Requested:   ${DEPLOY_SHA}" >&2
  echo "origin/main: ${ORIGIN_MAIN_SHA}" >&2
  exit 1
fi

echo "Checking out exact deployment revision..."
git checkout --detach "${DEPLOY_SHA}"

ACTUAL_SHA="$(git rev-parse HEAD)"

if [[ "${ACTUAL_SHA}" != "${DEPLOY_SHA}" ]]; then
  echo "ERROR: checked-out revision does not match requested deployment revision" >&2
  echo "Expected: ${DEPLOY_SHA}" >&2
  echo "Actual:   ${ACTUAL_SHA}" >&2
  exit 1
fi

echo "Building API for revision ${DEPLOY_SHA}..."
cd "${API_DIR}"
BUILD_SHA="${DEPLOY_SHA}" docker compose build api

echo "Recreating API container only..."
docker compose up -d --no-deps api

HEALTH_URL="http://127.0.0.1:8080/healthz"
VERSION_URL="http://127.0.0.1:8080/version"
READINESS_ATTEMPTS=12
READINESS_INTERVAL_SECONDS=5

echo "Waiting for API readiness..."

API_READY=false

for ((attempt = 1; attempt <= READINESS_ATTEMPTS; attempt++)); do
  if HEALTH_JSON="$(curl --fail --silent --show-error "${HEALTH_URL}" 2>/dev/null)"; then
    if HEALTH_JSON="${HEALTH_JSON}" python3 - <<'PY'
import json
import os
import sys

try:
    payload = json.loads(os.environ["HEALTH_JSON"])
except (json.JSONDecodeError, KeyError):
    sys.exit(1)

sys.exit(0 if payload.get("ok") == "true" else 1)
PY
    then
      API_READY=true
      echo "API readiness accepted on attempt ${attempt}/${READINESS_ATTEMPTS}."
      break
    fi
  fi

  if (( attempt < READINESS_ATTEMPTS )); then
    echo "API not ready on attempt ${attempt}/${READINESS_ATTEMPTS}; retrying in ${READINESS_INTERVAL_SECONDS}s..."
    sleep "${READINESS_INTERVAL_SECONDS}"
  fi
done

if [[ "${API_READY}" != "true" ]]; then
  echo "ERROR: API did not become ready within the bounded readiness window" >&2
  echo "Final health probe:" >&2
  curl --fail --silent --show-error "${HEALTH_URL}" >&2 || true
  exit 1
fi

echo "Verifying deployed API revision..."

if ! VERSION_JSON="$(curl --fail --silent --show-error "${VERSION_URL}")"; then
  echo "ERROR: unable to retrieve deployed API revision" >&2
  exit 1
fi

ACTUAL_RUNTIME_SHA="$(
  VERSION_JSON="${VERSION_JSON}" python3 - <<'PY'
import json
import os
import sys

try:
    payload = json.loads(os.environ["VERSION_JSON"])
except (json.JSONDecodeError, KeyError):
    sys.exit(1)

git_sha = payload.get("git_sha")

if not isinstance(git_sha, str):
    sys.exit(1)

print(git_sha)
PY
)" || {
  echo "ERROR: deployed API returned an invalid version response" >&2
  exit 1
}

if [[ "${ACTUAL_RUNTIME_SHA}" != "${DEPLOY_SHA}" ]]; then
  echo "ERROR: deployed API revision does not match requested deployment revision" >&2
  echo "Expected: ${DEPLOY_SHA}" >&2
  echo "Actual:   ${ACTUAL_RUNTIME_SHA}" >&2
  exit 1
fi

echo "API revision accepted: ${ACTUAL_RUNTIME_SHA}"
echo "API deployment accepted for revision ${DEPLOY_SHA}."
