package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	Pool *pgxpool.Pool
}

func (d *DB) WithTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := d.Pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.ReadCommitted,
	})
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx) // no-op if committed
	}()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")

func NowUTC() time.Time { return time.Now().UTC() }
