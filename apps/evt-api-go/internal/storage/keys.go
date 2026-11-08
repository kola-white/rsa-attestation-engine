package storage

import (
	"path"
	"regexp"
	"strings"
)

var fileNameSafe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// BuildStorageKey builds a safe object key under a controlled prefix.
// This MUST match whatever policy you enforce in /v1/evidence/commit.
func BuildStorageKey(caseID, checkID, uploadID, originalName string) string {
	base := sanitizeFileName(originalName)
	return path.Join("cases", caseID, "checks", checkID, "evidence", uploadID, base)
}

func sanitizeFileName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "file"
	}
	s = fileNameSafe.ReplaceAllString(s, "_")
	if len(s) > 120 {
		s = s[:120]
	}
	return s
}

// BuildSealedEvidenceKey keeps failed-write objects attributable to their case.
// The nonce is server-generated and this namespace is never presigned for PUT.
// Inputs come from the persisted validated case/check and UUID upload identity.
func BuildSealedEvidenceKey(caseID, checkID, uploadID, nonce string) string {
	return path.Join("sealed-evidence", caseID, checkID, uploadID, nonce)
}
