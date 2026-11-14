package evt

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/auth"
)

// EmployerAccess IDs are opaque existing identifiers, not necessarily UUIDs.
// There is no authoritative name field in the current employer registry.
type EmployerAccess struct {
	EmployerID string `json:"employer_id"`
}

func assignedEmployers(ctx context.Context, tx pgx.Tx, principal string) ([]EmployerAccess, error) {
	items := make([]EmployerAccess, 0)
	if _, err := uuid.Parse(principal); err != nil {
		return items, nil
	}
	// Reuse the existing SECURITY DEFINER checker: current principal role/status,
	// explicit unrevoked assignment, and the established revocation lock protocol.
	// Runtime needs only its existing config SELECT and checker EXECUTE grants.
	rows, err := tx.Query(ctx, `SELECT employer_id FROM evt_employer_config
		WHERE check_employer_hr_assignment($1, employer_id) ORDER BY employer_id`, principal)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item EmployerAccess
		if err := rows.Scan(&item.EmployerID); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// Employers is discovery, never an authorization receipt for a later operation.
func (h *EmployerHandlers) Employers(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	claims := auth.MustClaims(c)
	if claims == nil {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	var items []EmployerAccess
	err := h.DB.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		items, err = assignedEmployers(ctx, tx, claims.Sub)
		return err
	})
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "employer_access_unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}
