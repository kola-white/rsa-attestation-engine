package httpapi

import (
	"encoding/json"
	"log"
	"net/http"
)

type VerificationOutcome struct {
	Signature string `json:"signature"` // "verified" | "invalid" | "unknown"
	Trust     string `json:"trust"`     // "trusted" | "untrusted" | "unknown"
	Why       *struct {
		Summary string `json:"summary"`
		Code    string `json:"code"`
	} `json:"why,omitempty"`
	Checks *struct {
		ValidityWindow string `json:"validity_window"` // "valid_now" | "not_valid_now" | "unknown"
		Revocation     string `json:"revocation"`      // "not_revoked" | "revoked" | "unknown"
	} `json:"checks,omitempty"`
}

type verifyOutcomeRequest struct {
	JWSCompact string `json:"jwsCompact"`
}

func (s *Server) HandleVerificationOutcome(w http.ResponseWriter, r *http.Request) {
	// Hard method guard (ServeMux pattern-match is not enough on all paths)
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Limit body size defensively (no huge payloads)
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB

	var req verifyOutcomeRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if req.JWSCompact == "" {
		writeErr(w, http.StatusBadRequest, "missing_jws_compact")
		return
	}

	// ✅ IMPORTANT:
	// This endpoint is the boundary: iOS should not run Node verification.
	// Wire your real verifier here when ready.
	//
	// For now, return a safe locked outcome that keeps the app deterministic,
	// without claiming verification succeeded.
	out := VerificationOutcome{
		Signature: "unknown",
		Trust:     "unknown",
		Why: &struct {
			Summary string `json:"summary"`
			Code    string `json:"code"`
		}{
			Summary: "Verification is not configured on the API yet.",
			Code:    "SERVER_VERIFIER_UNCONFIGURED",
		},
		Checks: &struct {
			ValidityWindow string `json:"validity_window"`
			Revocation     string `json:"revocation"`
		}{
			ValidityWindow: "unknown",
			Revocation:     "unknown",
		},
	}

	// Dev-only log: do not log the JWS
	log.Printf("[verify:outcome] jws_len=%d signature=%s trust=%s why=%s",
		len(req.JWSCompact),
		out.Signature,
		out.Trust,
		func() string {
			if out.Why == nil {
				return ""
			}
			return out.Why.Code
		}(),
	)

	writeJSON(w, http.StatusOK, out)
}
