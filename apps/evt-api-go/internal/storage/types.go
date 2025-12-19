package storage

type EvidenceFileInit struct {
	Name     string `json:"name"`
	MimeType string `json:"mimeType"`
	Size     int64  `json:"size"`
}

type EvidenceInitRequest struct {
	CaseID  string            `json:"caseId"`
	CheckID string            `json:"checkId"`
	Files   []EvidenceFileInit `json:"files"`
}

type EvidenceInitItem struct {
	Name            string            `json:"name"`
	MimeType        string            `json:"mimeType"`
	Size            int64             `json:"size"`
	StorageKey      string            `json:"storageKey"`
	UploadURL       string            `json:"uploadUrl"`
	Method          string            `json:"method"` // "PUT"
	RequiredHeaders map[string]string `json:"requiredHeaders,omitempty"`
}

type EvidenceInitResponse struct {
	CaseID           string             `json:"caseId"`
	CheckID          string             `json:"checkId"`
	Items            []EvidenceInitItem  `json:"items"`
	ExpiresInSeconds int64              `json:"expiresInSeconds"`
}

type EvidenceCommitItem struct {
	StorageKey string `json:"storageKey"`
	MimeType   string `json:"mimeType,omitempty"`
	Size       int64  `json:"size,omitempty"`
	SHA256     string `json:"sha256,omitempty"` // optional in v1
}

type EvidenceCommitRequest struct {
	CaseID  string               `json:"caseId"`
	CheckID string               `json:"checkId"`
	Items   []EvidenceCommitItem `json:"items"`
}

type EvidenceCommitResponse struct {
	CaseID  string `json:"caseId"`
	CheckID string `json:"checkId"`
	Count   int    `json:"count"`
}
