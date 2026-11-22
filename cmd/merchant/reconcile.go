package main

import (
	"errors"
	"fmt"

	"github.com/Indra55/schrodingers_charge/internal/payment"
)

func (m merchant) reconcile() error {
	pending, err := m.store.pending()
	if err != nil {
		return fmt.Errorf("list pending payments: %w", err)
	}
	if len(pending) == 0 {
		fmt.Println("no pending payments")
		return nil
	}

	var failures []error
	for _, charge := range pending {
		if err := m.reconcileCharge(charge); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (m merchant) reconcileCharge(charge payment.Charge) error {
	found, err := m.lookup(charge)
	if err == nil && !found {
		err = m.send(charge)
	}
	if err != nil {
		return fmt.Errorf("%s pending (%d paise): %w", charge.PaymentID, charge.AmountPaise, err)
	}
	return m.markPaid(charge)
}
