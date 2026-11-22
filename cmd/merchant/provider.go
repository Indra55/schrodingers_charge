package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/Indra55/schrodingers_charge/internal/payment"
)

type providerStatusError struct {
	method string
	status string
	code   int
}

func (e providerStatusError) Error() string {
	return fmt.Sprintf("provider %s returned %s", e.method, e.status)
}

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
		if m.fault != "" {
			req.Header.Set("X-Demo-Fault", m.fault)
		}
	}

	label := method + " /charges"
	if method == http.MethodGet {
		label += "/{id}"
	}
	if m.trace != nil {
		m.record("request", label, fmt.Sprintf("Merchant sending %s %s", method, path))
	}
	resp, err := m.client.Do(req)
	if err != nil {
		if m.trace != nil {
			outcome := "request failed"
			if errors.Is(err, context.DeadlineExceeded) {
				outcome = "timeout"
			}
			m.record("fault", outcome, fmt.Sprintf("%s %s → %s", method, path, outcome))
		}
		return false, err
	}
	defer resp.Body.Close()
	if m.trace != nil {
		m.record("response", fmt.Sprintf("HTTP %d", resp.StatusCode), fmt.Sprintf("%s %s → HTTP %d", method, path, resp.StatusCode))
	}

	if method == http.MethodGet && resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode != http.StatusOK &&
		!(method == http.MethodPost && resp.StatusCode == http.StatusCreated) {
		return false, providerStatusError{method: method, status: resp.Status, code: resp.StatusCode}
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
