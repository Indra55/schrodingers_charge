package main

import (
	"context"
	"database/sql"
)

const schema = `
CREATE TABLE IF NOT EXISTS charges (
    payment_id TEXT NOT NULL PRIMARY KEY,
    amount_paise INTEGER NOT NULL CHECK (amount_paise > 0)
);
`

type ledger struct {
	db *sql.DB
}

func (l ledger) record(ctx context.Context, id string, amount int64) (bool, error) {
	result, err := l.db.ExecContext(ctx, `
		INSERT INTO charges (payment_id, amount_paise) VALUES (?, ?)
		ON CONFLICT (payment_id) DO NOTHING
	`, id, amount)
	if err != nil {
		return false, err
	}
	inserted, err := result.RowsAffected()
	return inserted == 1, err
}

func (l ledger) lookup(ctx context.Context, id string) (int64, error) {
	var amount int64
	err := l.db.QueryRowContext(ctx,
		"SELECT amount_paise FROM charges WHERE payment_id = ?", id,
	).Scan(&amount)
	return amount, err
}
