// hr-assignment is an operator-only utility. It uses a separately supplied
// administrative DSN; application JWTs and API roles cannot invoke it.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kola-white/rsa-attestation-engine/apps/evt-api-go/internal/employeraccess"
)

func main() {
	if err := run(); err != nil {
		// Driver errors may include connection credentials or data. Report failure
		// without printing the DSN, SQL arguments, or a raw database error.
		fmt.Fprintln(os.Stderr, "assignment operation failed; verify reviewed inputs, schema and administrative privileges")
		os.Exit(1)
	}
}

func run() error {
	action := flag.String("action", "", "grant or revoke")
	principal := flag.String("principal", "", "canonical domain_users.id (grant)")
	employer := flag.String("employer", "", "exact reviewed employer identifier (grant)")
	assignment := flag.String("assignment", "", "grant instance UUID (revoke)")
	actor := flag.String("actor", "", "attributable approving/revoking operator")
	basis := flag.String("basis", "", "reviewed approval evidence reference (grant)")
	reason := flag.String("reason", "", "revocation reason (revoke)")
	flag.Parse()
	if flag.NArg() != 0 || *actor == "" {
		return fmt.Errorf("invalid arguments")
	}
	var id uuid.UUID
	switch *action {
	case "grant":
		if _, err := uuid.Parse(*principal); err != nil {
			return err
		}
		if *employer == "" || *basis == "" || *assignment != "" || *reason != "" {
			return fmt.Errorf("invalid grant arguments")
		}
	case "revoke":
		var err error
		id, err = uuid.Parse(*assignment)
		if err != nil {
			return err
		}
		if *reason == "" || *principal != "" || *employer != "" || *basis != "" {
			return fmt.Errorf("invalid revoke arguments")
		}
	default:
		return fmt.Errorf("invalid action")
	}
	dsn := os.Getenv("CVERA_HR_ADMIN_DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("administrative database URL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer conn.Close(context.Background())
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if *action == "grant" {
		id, err = employeraccess.Grant(ctx, tx, *principal, *employer, *actor, *basis)
	} else {
		err = employeraccess.Revoke(ctx, tx, id, *actor, *reason)
	}
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	fmt.Printf("%s committed: assignment_id=%s\n", *action, id)
	return nil
}
