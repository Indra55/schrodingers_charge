package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/Indra55/schrodingers_charge/internal/payment"
)

func (m merchant) send(charge payment.Charge) error {
	body, err := json.Marshal(charge)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, m.provider+"/charges", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return m.confirm(req, charge)
}

func (m merchant) confirm(req *http.Request, expected payment.Charge) error {
	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("provider POST returned %s", resp.Status)
	}

	var confirmed payment.Charge
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 4096))
	if err := decoder.Decode(&confirmed); err != nil {
		return fmt.Errorf("decode provider charge: %w", err)
	}
	if confirmed != expected {
		return fmt.Errorf("provider returned a different payment ID or amount")
	}
	return nil
}
