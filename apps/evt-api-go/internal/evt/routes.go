package evt

import (
	"github.com/gin-gonic/gin"

	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/auth"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/db"
)

type Module struct {
	Candidate *CandidateHandlers
	Employer  *EmployerHandlers
	Recruiter *RecruiterHandlers
	Internal  *InternalHandlers
}

func NewModule(d *db.DB) *Module {
	repo := &Repo{DB: d}

	return &Module{
		Candidate: &CandidateHandlers{DB: d, Repo: repo},
		Employer:  &EmployerHandlers{DB: d, Repo: repo},
		Recruiter: &RecruiterHandlers{DB: d, Repo: repo},
		Internal:  &InternalHandlers{DB: d, Repo: repo},
	}
}

// Register mounts Phase-2 EVT routes under the provided RouterGroup.
// In router.go you call:
//
//   v1 := evtEngine.Group("/v1")
//   evtModule.Register(v1)
//
func (m *Module) Register(v1 *gin.RouterGroup) {
	// -----------------------------
	// /v1 — Requestor (Candidate)
	// -----------------------------
	req := v1.Group("", auth.GinRequireRole(auth.RoleRequestor))
	{
		req.GET("/requests", m.Candidate.List)
		req.GET("/requests/:request_id", m.Candidate.Get)

		req.POST("/requests", m.Candidate.CreateDraft)
		req.PATCH("/requests/:request_id", m.Candidate.PatchDraft)

		req.POST("/requests/:request_id/submit", m.Candidate.Submit)
		req.POST("/requests/:request_id/cancel", m.Candidate.Cancel)
	}

	// ------------------------------------
	// /v1/employer — Employer HR Reviewer
	// ------------------------------------
	hr := v1.Group("/employer", auth.GinRequireRole(auth.RoleHRReviewer))
	{
		// Optional queue + detail endpoints; uncomment if implemented.
		hr.GET("/requests", m.Employer.List)
		hr.GET("/requests/:request_id", m.Employer.Get)

		hr.POST("/requests/:request_id/attest", m.Employer.Attest)
	}

	// -----------------------------
	// /v1/internal — Cvera Internal
	// -----------------------------
	intg := v1.Group("/internal", auth.GinRequireRole(auth.RoleCvera))
	{
		intg.POST("/requests/:request_id/verify", m.Internal.Verify)
		intg.POST("/requests/:request_id/close", m.Internal.Close)
	}

	// -----------------------------
	// /v1/tokens — Recruiter
	// -----------------------------
	tok := v1.Group("/tokens", auth.GinRequireRole(auth.RoleRecruiter))
	{
		tok.POST("/:request_id/consume", m.Recruiter.Consume)
	}
}
