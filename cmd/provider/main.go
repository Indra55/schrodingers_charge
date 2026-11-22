package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/Indra55/schrodingers_charge/internal/payment"
)

type provider struct {
	ledger ledger
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "HTTP listen address")
	dbPath := flag.String("db", "provider.db", "provider SQLite database")
	flag.Parse()

	if flag.NArg() != 0 {
		flag.Usage()
		log.Fatal("expected no positional arguments")
	}

	db, err := payment.OpenDB(*dbPath, schema)
	if err != nil {
		log.Fatalf("open provider database: %v", err)
	}

	p := provider{ledger: ledger{db: db}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /charges", p.createCharge)
	mux.HandleFunc("GET /charges/{id}", p.getCharge)

	server := http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("provider listening on %s", *addr)
	log.Fatal(server.ListenAndServe())
}

func (p provider) createCharge(w http.ResponseWriter, r *http.Request) {
	var requested payment.Charge
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&requested); err != nil || decoder.Decode(new(any)) != io.EOF {
		http.Error(w, "expected one charge JSON object", http.StatusBadRequest)
		return
	}
	if err := requested.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	inserted, err := p.ledger.record(r.Context(), requested.PaymentID, requested.AmountPaise)
	if err != nil {
		writeDBError(w, err)
		return
	}
	if inserted {
		writeCharge(w, http.StatusCreated, requested)
		return
	}

	amount, err := p.ledger.lookup(r.Context(), requested.PaymentID)
	if err != nil {
		writeDBError(w, err)
		return
	}
	if amount != requested.AmountPaise {
		http.Error(w, "payment ID already has a different amount", http.StatusConflict)
		return
	}
	writeCharge(w, http.StatusOK, requested)
}

func (p provider) getCharge(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	amount, err := p.ledger.lookup(r.Context(), id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		http.Error(w, "charge not found", http.StatusNotFound)
	case err != nil:
		writeDBError(w, err)
	default:
		writeCharge(w, http.StatusOK, payment.Charge{PaymentID: id, AmountPaise: amount})
	}
}

func writeDBError(w http.ResponseWriter, err error) {
	log.Printf("provider database: %v", err)
	http.Error(w, "provider database error", http.StatusInternalServerError)
}

func writeCharge(w http.ResponseWriter, status int, charge payment.Charge) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(charge); err != nil {
		log.Printf("write charge response: %v", err)
	}
}
