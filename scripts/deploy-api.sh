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

echo "API deployment command completed for revision ${DEPLOY_SHA}."
echo "Post-deployment runtime verification belongs to Phase 5."
