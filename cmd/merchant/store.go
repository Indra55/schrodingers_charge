package main

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/Indra55/schrodingers_charge/internal/payment"
)

const schema = `
CREATE TABLE IF NOT EXISTS payments (
    payment_id TEXT NOT NULL PRIMARY KEY,
    amount_paise INTEGER NOT NULL CHECK (amount_paise > 0),
    status TEXT NOT NULL CHECK (status IN ('pending', 'paid'))
);
`

var errNotFound = errors.New("not found")

type localPayment struct {
	payment.Charge
	status string
}

type store struct {
	db *sql.DB
}

func (s store) savePending(charge payment.Charge) error {
	_, err := s.db.Exec(`
		INSERT INTO payments (payment_id, amount_paise, status) VALUES (?, ?, 'pending')
		ON CONFLICT (payment_id) DO NOTHING
	`, charge.PaymentID, charge.AmountPaise)
	if err != nil {
		return fmt.Errorf("save payment: %w", err)
	}
	return nil
}

func (s store) lookup(id string) (localPayment, error) {
	var local localPayment
	local.PaymentID = id
	err := s.db.QueryRow("SELECT amount_paise, status FROM payments WHERE payment_id = ?", id).
		Scan(&local.AmountPaise, &local.status)
	if errors.Is(err, sql.ErrNoRows) {
		return local, fmt.Errorf("payment %q %w", id, errNotFound)
	}
	if err != nil {
		return local, fmt.Errorf("read payment %q: %w", id, err)
	}
	return local, nil
}

func printStatus(charge payment.Charge, status string) {
	fmt.Printf("%s %s (%d paise)\n", charge.PaymentID, status, charge.AmountPaise)
}

func (s store) markPaid(id string) error {
	_, err := s.db.Exec("UPDATE payments SET status = 'paid' WHERE payment_id = ?", id)
	if err != nil {
		return fmt.Errorf("save paid status for %s: %w", id, err)
	}
	return nil
}

func (s store) pending() ([]payment.Charge, error) {
	rows, err := s.db.Query(`
		SELECT payment_id, amount_paise FROM payments
		WHERE status = 'pending' ORDER BY payment_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pending []payment.Charge
	for rows.Next() {
		var charge payment.Charge
		if err := rows.Scan(&charge.PaymentID, &charge.AmountPaise); err != nil {
			return nil, err
		}
		pending = append(pending, charge)
	}
	return pending, rows.Err()
}
