package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Indra55/schrodingers_charge/internal/payment"
)

func TestConcurrentChargeRetries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provider.db")
	first := openTestProvider(t, path)
	second := openTestProvider(t, path)

	const attempts = 24
	start := make(chan struct{})
	results := make(chan int, attempts)
	for i := 0; i < attempts; i++ {
		p := first
		if i%2 == 1 {
			p = second
		}
		go func(p provider) {
			<-start
			results <- postCharge(p, `{"payment_id":"race-1","amount_paise":49900}`)
		}(p)
	}
	close(start)

	created := 0
	for i := 0; i < attempts; i++ {
		switch status := <-results; status {
		case http.StatusCreated:
			created++
		case http.StatusOK:
		default:
			t.Errorf("unexpected HTTP status %d", status)
		}
	}
	if created != 1 {
		t.Fatalf("created %d charges, want 1", created)
	}

	if got := postCharge(second, `{"payment_id":"race-1","amount_paise":50000}`); got != http.StatusConflict {
		t.Errorf("different amount returned HTTP %d, want 409", got)
	}

	assertCharge(t, first, "race-1", 1, 49900)
}

func openTestProvider(t *testing.T, path string) provider {
	t.Helper()
	db, err := payment.OpenDB(path, schema)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return provider{ledger: ledger{db: db}}
}

func postCharge(p provider, body string) int {
	w := httptest.NewRecorder()
	p.createCharge(w, httptest.NewRequest(http.MethodPost, "/charges", strings.NewReader(body)))
	return w.Code
}

func assertCharge(t *testing.T, p provider, id string, wantCount, wantTotal int) {
	t.Helper()
	var count, total int
	err := p.ledger.db.QueryRow(`
		SELECT COUNT(*), COALESCE(SUM(amount_paise), 0) FROM charges WHERE payment_id = ?
	`, id).Scan(&count, &total)
	if err != nil {
		t.Fatal(err)
	}
	if count != wantCount || total != wantTotal {
		t.Fatalf("%s has %d charges for %d paise, want %d for %d paise", id, count, total, wantCount, wantTotal)
	}
}
