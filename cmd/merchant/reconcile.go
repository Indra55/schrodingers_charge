package main

import (
	"errors"
	"fmt"
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
		found, err := m.lookup(charge)
		if err != nil {
			failures = append(failures, fmt.Errorf("%s pending (%d paise): %w",
				charge.PaymentID, charge.AmountPaise, err))
			continue
		}
		if !found {
			fmt.Printf("%s pending (%d paise)\n", charge.PaymentID, charge.AmountPaise)
			continue
		}
		if err := m.markPaid(charge); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}
