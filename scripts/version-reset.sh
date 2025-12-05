#!/usr/bin/env bash
set -euo pipefail

# Version Reset Helper Script
# ---------------------------
# What this does:
# 1. Ensures working tree is clean
# 2. Creates a backup branch for current state
# 3. Creates an annotated tag for the "pre-wedge" state

PRE_WEDGE_TAG="v0.3.0-pre-wedge"
BACKUP_BRANCH="pre-wedge-backup"

echo "== Attested Identity Version Reset Helper =="

# 1. Ensure working tree is clean
if ! git diff --quiet || ! git diff --cached --quiet; then
  echo "ERROR: Your working tree has uncommitted changes."
  echo "Please commit or stash them before running this script."
  exit 1
fi

CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD)

echo "Working tree is clean."
echo "Current branch: ${CURRENT_BRANCH}"

# 2. Create backup branch at current HEAD
if git show-ref --verify --quiet "refs/heads/${BACKUP_BRANCH}"; then
  echo "Backup branch '${BACKUP_BRANCH}' already exists, skipping branch creation."
else
  echo "Creating backup branch '${BACKUP_BRANCH}' at current HEAD..."
  git branch "${BACKUP_BRANCH}"
  echo "Backup branch '${BACKUP_BRANCH}' created."
fi

# 3. Create an annotated pre-wedge tag
if git show-ref --tags --verify --quiet "refs/tags/${PRE_WEDGE_TAG}"; then
  echo "Tag '${PRE_WEDGE_TAG}' already exists, not recreating."
else
  echo "Creating annotated tag '${PRE_WEDGE_TAG}' at current HEAD..."
  git tag -a "${PRE_WEDGE_TAG}" -m "Pre-wedge general attestation MVP (0.3-dev era)"
  echo "Tag '${PRE_WEDGE_TAG}' created."
fi

echo
echo "Next steps:"
echo "1) Push the backup branch and tag to origin (already done)."
echo
echo "2) Manually update CHANGELOG.md to:"
echo "   - Clarify [0.3-dev] / [0.3.0-pre-wedge] as the pre-wedge experimental state."
echo "   - Add a new [0.1.0-evt] section for the upcoming EVT MVP work."
echo
echo "3) After that, you can safely:"
echo "   - Restructure the repo for EVT (Phase 1)."
echo "   - Commit those changes."
echo "   - Tag the new EVT-based state later as: v0.1.0-evt"
echo
echo "You now have a safe restore point:"
echo "  branch: ${BACKUP_BRANCH}"
echo "  tag:    ${PRE_WEDGE_TAG}"
