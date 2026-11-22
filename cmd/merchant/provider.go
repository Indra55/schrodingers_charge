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
	_, err = m.request(http.MethodPost, "/charges", bytes.NewReader(body), charge)
	return err
}

func (m merchant) lookup(charge payment.Charge) (bool, error) {
	return m.request(http.MethodGet, "/charges/"+charge.PaymentID, nil, charge)
}

func (m merchant) request(method, path string, body io.Reader, expected payment.Charge) (bool, error) {
	req, err := http.NewRequest(method, m.provider+path, body)
	if err != nil {
		return false, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if method == http.MethodGet && resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode != http.StatusOK &&
		!(method == http.MethodPost && resp.StatusCode == http.StatusCreated) {
		return false, fmt.Errorf("provider %s returned %s", method, resp.Status)
	}

	var confirmed payment.Charge
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 4096))
	if err := decoder.Decode(&confirmed); err != nil {
		return false, fmt.Errorf("decode provider charge: %w", err)
	}
	if confirmed != expected {
		return false, fmt.Errorf("provider returned a different payment ID or amount")
	}
	return true, nil
}
