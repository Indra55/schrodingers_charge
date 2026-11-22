package main

import (
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestMerchantRecovery(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "merchant")
	if output, err := exec.Command("go", "build", "-o", binary, "../merchant").CombinedOutput(); err != nil {
		t.Fatalf("build merchant: %v\n%s", err, output)
	}

	t.Run("committed before timeout", func(t *testing.T) {
		p := openTestProvider(t, filepath.Join(dir, "delayed.db"))
		p.responseDelay = time.Minute
		server := httptest.NewServer(p.handler())
		defer server.Close()
		db := filepath.Join(dir, "timeout-merchant.db")

		runMerchant(t, binary, db, server.URL, 1, "Client.Timeout", "-timeout", "1s", "pay", "demo-1", "49900")
		runMerchant(t, binary, db, server.URL, 0, "demo-1 pending (49900 paise)", "status", "demo-1")
		assertCharge(t, p, "demo-1", 1, 49900)

		runMerchant(t, binary, db, server.URL, 0, "demo-1 paid (49900 paise)", "reconcile")
		runMerchant(t, binary, db, server.URL, 0, "demo-1 paid (49900 paise)", "status", "demo-1")
		runMerchant(t, binary, db, server.URL, 0, "no pending payments", "reconcile")
		assertCharge(t, p, "demo-1", 1, 49900)
	})

	t.Run("retry only after a missing charge", func(t *testing.T) {
		p := openTestProvider(t, filepath.Join(dir, "available.db"))
		handler := p.handler()
		var available atomic.Bool
		var posts atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				posts.Add(1)
			}
			if !available.Load() {
				http.Error(w, "unavailable", http.StatusServiceUnavailable)
				return
			}
			handler.ServeHTTP(w, r)
		}))
		defer server.Close()

		db := filepath.Join(dir, "missing-merchant.db")
		runMerchant(t, binary, db, server.URL, 1, "provider POST returned 503", "pay", "demo-2", "49900")
		runMerchant(t, binary, db, server.URL, 1, "provider GET returned 503", "reconcile")
		if got := posts.Load(); got != 1 {
			t.Fatalf("sent %d POST requests after a failed lookup, want 1", got)
		}
		assertCharge(t, p, "demo-2", 0, 0)

		available.Store(true)
		runMerchant(t, binary, db, server.URL, 0, "demo-2 paid (49900 paise)", "reconcile")
		runMerchant(t, binary, db, server.URL, 0, "no pending payments", "reconcile")
		if got := posts.Load(); got != 2 {
			t.Fatalf("sent %d POST requests, want 2", got)
		}
		assertCharge(t, p, "demo-2", 1, 49900)
	})
}

func runMerchant(t *testing.T, binary, db, provider string, wantExit int, wantOutput string, args ...string) {
	t.Helper()
	flags := []string{"-db", db, "-provider", provider}
	output, err := exec.Command(binary, append(flags, args...)...).CombinedOutput()
	exit := 0
	if err != nil {
		process, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("run merchant %v: %v", args, err)
		}
		exit = process.ExitCode()
	}
	if exit != wantExit || !strings.Contains(string(output), wantOutput) {
		t.Fatalf("merchant %v exited %d with %q, want exit %d and %q", args, exit, output, wantExit, wantOutput)
	}
}
