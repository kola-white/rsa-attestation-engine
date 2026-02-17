package evt

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/auth"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/db"
)

type RecruiterListHandlers struct {
	DB   *db.DB
	Repo *Repo
}

type RecruiterTrustMode string

const (
	TrustAny            RecruiterTrustMode = "any"
	TrustTrustedOnly    RecruiterTrustMode = "trusted_only"
	TrustIncludeUntrust RecruiterTrustMode = "include_untrusted"
)

type RecruiterSort string

const (
	SortMostRecent RecruiterSort = "most_recent"
)

type SignatureBadge string

const (
	SigVerified SignatureBadge = "verified"
	SigInvalid  SignatureBadge = "invalid"
	SigUnknown  SignatureBadge = "unknown"
)

type TrustBadge string

const (
	TrustTrusted   TrustBadge = "trusted"
	TrustUntrusted TrustBadge = "untrusted"
	TrustUnknown   TrustBadge = "unknown"
)

type CandidateRowSnapshot struct {
	CandidateID string `json:"candidate_id"`
	Subject     struct {
		FullName   string  `json:"full_name"`
		EmployeeID *string `json:"employee_id,omitempty"`
	} `json:"subject"`
	PrimaryEmployment struct {
		IssuerName string  `json:"issuer_name"`
		Title      string  `json:"title"`
		StartDate  string  `json:"start_date"`
		EndDate    *string `json:"end_date"`
	} `json:"primary_employment"`
	PrimaryEVT struct {
		EVTID string `json:"evt_id"`
	} `json:"primary_evt"`
	Badges struct {
		Signature SignatureBadge `json:"signature"`
		Trust     TrustBadge     `json:"trust"`
	} `json:"badges"`
	UpdatedAt string `json:"updated_at"`
}

type RecruiterListResp struct {
	Items      []CandidateRowSnapshot `json:"items"`
	NextCursor *string                `json:"next_cursor,omitempty"`
}

type recruiterCursor struct {
	UpdatedAt time.Time `json:"updated_at"`
	RequestID string    `json:"request_id"`
}

func encodeCursor(c recruiterCursor) (string, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func decodeCursor(s string) (*recruiterCursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	var out recruiterCursor
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func parseCSVOrRepeated(c *gin.Context, key string) []string {
	raw := c.QueryArray(key)
	if len(raw) == 0 {
		v := strings.TrimSpace(c.Query(key))
		if v == "" {
			return nil
		}
		raw = []string{v}
	}

	var out []string
	for _, r := range raw {
		for _, part := range strings.Split(r, ",") {
			p := strings.TrimSpace(part)
			if p != "" {
				out = append(out, p)
			}
		}
	}

	seen := map[string]struct{}{}
	uniq := make([]string, 0, len(out))
	for _, s := range out {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		uniq = append(uniq, s)
	}
	return uniq
}

func (h *RecruiterListHandlers) ListCandidates(c *gin.Context) {
	claims := auth.MustClaims(c)
	if claims == nil {
		return
	}
	if !auth.HasRole(claims, auth.RoleRecruiter) {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	search := strings.TrimSpace(c.Query("search"))

	trustMode := RecruiterTrustMode(strings.TrimSpace(c.DefaultQuery("trust_mode", string(TrustAny))))
	if trustMode != TrustAny && trustMode != TrustTrustedOnly && trustMode != TrustIncludeUntrust {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid_trust_mode"})
		return
	}

	sort := RecruiterSort(strings.TrimSpace(c.DefaultQuery("sort", string(SortMostRecent))))
	if sort != SortMostRecent {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid_sort"})
		return
	}

	sigRaw := parseCSVOrRepeated(c, "signature_status")
	if len(sigRaw) == 0 {
		sigRaw = []string{string(SigVerified), string(SigInvalid), string(SigUnknown)}
	}

	signatures := make([]SignatureBadge, 0, len(sigRaw))
	for _, s := range sigRaw {
		v := SignatureBadge(s)
		if v != SigVerified && v != SigInvalid && v != SigUnknown {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_signature_status"})
			return
		}
		signatures = append(signatures, v)
	}

	companyIDs := parseCSVOrRepeated(c, "company_ids")

	limit := 25
	if ls := strings.TrimSpace(c.Query("limit")); ls != "" {
		n, err := strconv.Atoi(ls)
		if err != nil || n < 1 || n > 100 {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid_limit"})
			return
		}
		limit = n
	}

	var cur *recruiterCursor
	if cs := strings.TrimSpace(c.Query("cursor")); cs != "" {
		parsed, err := decodeCursor(cs)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_cursor"})
			return
		}
		cur = parsed
	}

	recruiterPersonID := claims.Sub

	var (
		items []CandidateRowSnapshot
		next  *string
	)

	err := h.DB.WithTx(c.Request.Context(), func(tx pgx.Tx) error {
		rows, nextCursor, err := h.Repo.RecruiterCandidateList(
			c.Request.Context(),
			tx,
			recruiterPersonID,
			search,
			trustMode,
			signatures,
			companyIDs,
			limit,
			cur,
		)
		if err != nil {
			return err
		}
		items = rows
		next = nextCursor
		return nil
	})

	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "list_failed"})
		return
	}

	c.JSON(http.StatusOK, RecruiterListResp{Items: items, NextCursor: next})
}

func (h *RecruiterListHandlers) FilterOptions(c *gin.Context) {
	claims := auth.MustClaims(c)
	if claims == nil {
		return
	}
	if !auth.HasRole(claims, auth.RoleRecruiter) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	recruiterPersonID := claims.Sub

	type companyOpt struct {
		CompanyID string `json:"company_id"`
		Name      string `json:"name"`
	}

	type resp struct {
		Companies              []companyOpt `json:"companies"`
		SignatureStatusOptions []string     `json:"signature_status_options"`
		TrustModeOptions       []string     `json:"trust_mode_options"`
	}

	var companies []companyOpt

	err := h.DB.WithTx(c.Request.Context(), func(tx pgx.Tx) error {
		opts, err := h.Repo.RecruiterCompanyOptions(c.Request.Context(), tx, recruiterPersonID)
		if err != nil {
			return err
		}
		companies = make([]companyOpt, 0, len(opts))
		for _, o := range opts {
			companies = append(companies, companyOpt{CompanyID: o.CompanyID, Name: o.Name})
		}
		return nil
	})

	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "options_failed"})
		return
	}

	c.JSON(http.StatusOK, resp{
		Companies:              companies,
		SignatureStatusOptions: []string{string(SigVerified), string(SigInvalid), string(SigUnknown)},
		TrustModeOptions:       []string{string(TrustAny), string(TrustTrustedOnly), string(TrustIncludeUntrust)},
	})
}
