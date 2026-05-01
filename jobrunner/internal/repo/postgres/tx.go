package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type postgresTx struct {
    tx  pgx.Tx
}

func (t *postgresTx) Commit(ctx context.Context) error {
	if err := t.tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit error: %w", err)
	}
	return nil
}

func (t *postgresTx) Rollback(ctx context.Context) error {
	if err := t.tx.Rollback(ctx); err != nil {
		return fmt.Errorf("rollback error: %w", err)
	}
	return nil
}

func (t *postgresTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
    rows, err := t.tx.Query(ctx, sql, args...)
    if err != nil {
        return nil, fmt.Errorf("query error in transaction: %w", err)
    }
    return rows, nil
}

func (t *postgresTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
    return t.tx.QueryRow(ctx, sql, args...)
}