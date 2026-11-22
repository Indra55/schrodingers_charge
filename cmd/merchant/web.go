package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/Indra55/schrodingers_charge/internal/payment"
)

const demoAmount = 49900

var demoID = regexp.MustCompile(`^[a-f0-9]{8}(-[a-f0-9]{4}){3}-[a-f0-9]{12}$`)

type demoState struct {
	MerchantStatus string   `json:"merchant_status"`
	ProviderCount  int      `json:"provider_count"`
	Events         []string `json:"events"`
	Error          string   `json:"error,omitempty"`
}

func (m merchant) serve(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "docs/index.html")
	})
	mux.HandleFunc("GET /healthz", m.health)
	mux.HandleFunc("GET /api/demo/{id}", m.demo)
	mux.HandleFunc("POST /api/demo/{id}/{action}", m.demo)

	server := http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	log.Printf("browser demo listening on %s", addr)
	return server.ListenAndServe()
}

func (m merchant) health(w http.ResponseWriter, r *http.Request) {
	resp, err := m.providerGET(r.Context(), "/healthz")
	if err != nil {
		http.Error(w, "provider unavailable", http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		http.Error(w, "provider unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := m.store.db.PingContext(r.Context()); err != nil {
		http.Error(w, "merchant database unavailable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (m merchant) demo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	id := r.PathValue("id")
	if !demoID.MatchString(id) {
		http.Error(w, "invalid demo payment ID", http.StatusBadRequest)
		return
	}

	state := demoState{Events: []string{}}
	var traceLock sync.Mutex
	m.trace = func(message string) {
		traceLock.Lock()
		state.Events = append(state.Events, message)
		traceLock.Unlock()
	}

	charge := payment.Charge{PaymentID: id, AmountPaise: demoAmount}
	action := r.PathValue("action")
	var actionErr error
	switch action {
	case "":
	case "pay":
		var input struct {
			Scenario string `json:"scenario"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
		if err := decoder.Decode(&input); err != nil || decoder.Decode(new(any)) != io.EOF {
			http.Error(w, "invalid demo scenario", http.StatusBadRequest)
			return
		}
		switch input.Scenario {
		case "normal":
		case "offline":
			m.fault = "before-commit"
		case "timeout":
			m.fault = "after-commit"
		default:
			http.Error(w, "unknown demo scenario", http.StatusBadRequest)
			return
		}
		actionErr = m.pay(charge)
	case "reconcile", "race":
		local, err := m.store.lookup(id)
		switch {
		case errors.Is(err, errNotFound):
			http.Error(w, "send a payment first", http.StatusConflict)
			return
		case err != nil:
			actionErr = err
		case local.status == "paid":
			m.record("Already paid. Nothing to reconcile.")
		case action == "reconcile":
			actionErr = m.reconcileCharge(local.Charge)
		default:
			actionErr = m.race(local.Charge)
		}
	default:
		http.NotFound(w, r)
		return
	}

	status, err := m.demoStatus(id)
	if err != nil {
		log.Printf("read demo payment: %v", err)
		http.Error(w, "merchant database unavailable", http.StatusBadGateway)
		return
	}
	count, err := m.providerCount(r.Context(), id)
	if err != nil {
		log.Printf("read demo ledger: %v", err)
		http.Error(w, "provider ledger unavailable", http.StatusBadGateway)
		return
	}
	state.MerchantStatus = status
	state.ProviderCount = count

	code := http.StatusOK
	if actionErr != nil {
		if status == "pending" && expectedDemoFault(m.fault, actionErr, count) {
			m.record("Merchant kept the payment pending.")
		} else {
			log.Printf("demo %s: %v", action, actionErr)
			state.Error = "The merchant could not complete the action. Check the payment state and try again."
			code = http.StatusBadGateway
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(state)
}

func expectedDemoFault(fault string, err error, count int) bool {
	switch fault {
	case "after-commit":
		return count == 1 && errors.Is(err, context.DeadlineExceeded)
	case "before-commit":
		var status providerStatusError
		return count == 0 && errors.As(err, &status) && status.code == http.StatusServiceUnavailable
	default:
		return false
	}
}

func (m merchant) demoStatus(id string) (string, error) {
	local, err := m.store.lookup(id)
	if errors.Is(err, errNotFound) {
		return "none", nil
	}
	return local.status, err
}

func (m merchant) providerGET(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.provider+path, nil)
	if err != nil {
		return nil, err
	}
	return m.client.Do(req)
}

func (m merchant) providerCount(ctx context.Context, id string) (int, error) {
	resp, err := m.providerGET(ctx, "/demo/charges/"+id+"/count")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("provider ledger returned %s", resp.Status)
	}
	var count int
	err = json.NewDecoder(io.LimitReader(resp.Body, 64)).Decode(&count)
	return count, err
}

func (m merchant) race(charge payment.Charge) error {
	workers := make([]merchant, 0, 2)
	for _, name := range []string{"Worker A", "Worker B"} {
		worker := m
		worker.trace = func(message string) { m.record(name + " · " + message) }
		found, err := worker.lookup(charge)
		if err != nil {
			return err
		}
		if found {
			return m.markPaid(charge)
		}
		workers = append(workers, worker)
	}

	results := make(chan error, len(workers))
	for _, worker := range workers {
		go func() { results <- worker.send(charge) }()
	}
	first, second := <-results, <-results
	if first != nil && second != nil {
		return errors.Join(first, second)
	}
	return m.markPaid(charge)
}
