package evt

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/auth"
)

// Only database transport is stubbed. SQL authorization behavior is covered by
// TestEmployerDiscoveryAssignmentsAndRevocation against the existing DB fixture.
type employerRows struct {
	pgx.Rows
	ids              []string
	next             int
	scanErr, rowsErr error
	closed           bool
}

func (r *employerRows) Next() bool { return r.next < len(r.ids) }
func (r *employerRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	*dest[0].(*string) = r.ids[r.next]
	r.next++
	return nil
}
func (r *employerRows) Err() error { return r.rowsErr }
func (r *employerRows) Close()     { r.closed = true }

type employerTx struct {
	pgx.Tx
	rows      *employerRows
	err       error
	principal any
}

func (tx *employerTx) Query(_ context.Context, _ string, args ...any) (pgx.Rows, error) {
	tx.principal = args[0]
	return tx.rows, tx.err
}

func TestAssignedEmployersPreservesOpaqueIDsAndEmptyArray(t *testing.T) {
	principal := "11111111-1111-4111-8111-111111111111"
	for _, ids := range [][]string{nil, {"cei-uat-sim-001", "019a5ace-3b00-7000-8000-000000000001", "legacy/name & division"}} {
		tx := &employerTx{rows: &employerRows{ids: ids}}
		items, err := assignedEmployers(context.Background(), tx, principal)
		if err != nil || items == nil || tx.principal != principal || !tx.rows.closed {
			t.Fatalf("resolution: %+v %v", items, err)
		}
		got := make([]string, 0)
		for _, item := range items {
			got = append(got, item.EmployerID)
		}
		if len(ids) != len(got) || (len(ids) > 0 && !reflect.DeepEqual(ids, got)) {
			t.Fatalf("identifier changed: %v", got)
		}
		if len(ids) == 0 {
			b, _ := json.Marshal(items)
			if string(b) != "[]" {
				t.Fatal("empty result must be []")
			}
		}
	}
	items, err := assignedEmployers(context.Background(), nil, "not-a-principal-uuid")
	if err != nil || len(items) != 0 {
		t.Fatal("malformed principal must not query database")
	}
}

func TestAssignedEmployersPropagatesDatabaseFailure(t *testing.T) {
	failure := errors.New("database unavailable")
	for _, tx := range []*employerTx{
		{err: failure}, {rows: &employerRows{ids: []string{"legacy"}, scanErr: failure}},
		{rows: &employerRows{rowsErr: failure}},
	} {
		_, err := assignedEmployers(context.Background(), tx, "11111111-1111-4111-8111-111111111111")
		if !errors.Is(err, failure) {
			t.Fatal("database error hidden", err)
		}
	}
}

func TestEmployerDiscoveryRequiresAuthenticatedHRRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(auth.GinRequireAuth("test-secret", "test-issuer", "test-audience"))
	m := &Module{Employer: &EmployerHandlers{}}
	m.Register(engine.Group("/v1"))
	for _, role := range []auth.Role{"", auth.RoleRequestor, auth.RoleRecruiter, auth.RoleCvera} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/v1/employer/employers?principal_id=someone-else", nil)
		want := 401
		if role != "" {
			token, err := auth.MintAccessTokenHS256("test-secret", "test-issuer", "test-audience", "11111111-1111-4111-8111-111111111111", "", []auth.Role{role}, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			r.Header.Set("Authorization", "Bearer "+token)
			want = 403
		}
		engine.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("role %q: %d %s", role, w.Code, w.Body.String())
		}
	}
}
