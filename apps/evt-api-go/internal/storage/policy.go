package storage

import (
	"fmt"
	"strings"
)

type EvidencePolicy struct {
	MaxBytes        int64
	AllowedMimeType map[string]bool
}

func DefaultEvidencePolicy() EvidencePolicy {
	return EvidencePolicy{
		MaxBytes: 25 * 1024 * 1024, // 25MB v1 default (change anytime)
		AllowedMimeType: map[string]bool{
			"application/pdf": true,
			"image/jpeg":      true,
			"image/png":       true,
		},
	}
}

func (p EvidencePolicy) ValidateKey(caseID, checkID, key string) error {
	prefix := "evidence/v1/" + caseID + "/" + checkID + "/"
	if !strings.HasPrefix(key, prefix) {
		return fmt.Errorf("storageKey out of scope")
	}
	if strings.Contains(key, "..") {
		return fmt.Errorf("storageKey invalid")
	}
	return nil
}

func (p EvidencePolicy) ValidateMeta(meta HeadMeta) error {
	if meta.Size <= 0 {
		return fmt.Errorf("empty object")
	}
	if meta.Size > p.MaxBytes {
		return fmt.Errorf("object too large")
	}
	if !p.AllowedMimeType[meta.ContentType] {
		return fmt.Errorf("mimeType not allowed: %s", meta.ContentType)
	}
	return nil
}
