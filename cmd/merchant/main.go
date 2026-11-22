package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Indra55/schrodingers_charge/internal/payment"
)

type merchant struct {
	store    store
	client   *http.Client
	provider string
	fault    string
	trace    func(string)
}

func main() {
	if err := run(os.Args[1:]); err != nil && !errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("merchant", flag.ContinueOnError)
	dbPath := flags.String("db", "merchant.db", "merchant SQLite database")
	provider := flags.String("provider", "http://127.0.0.1:8080", "provider base URL")
	addr := flags.String("addr", "127.0.0.1:3000", "browser demo listen address")
	timeout := flags.Duration("timeout", 500*time.Millisecond, "HTTP timeout per request")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Usage: merchant [flags] pay <id> <paise> | status <id> | reconcile | serve")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	args = flags.Args()
	if len(args) == 0 {
		flags.Usage()
		return fmt.Errorf("missing command")
	}

	var requested payment.Charge
	switch args[0] {
	case "pay":
		if len(args) != 3 {
			return fmt.Errorf("usage: merchant [flags] pay <id> <paise>")
		}
		amount, err := strconv.ParseInt(args[2], 10, 64)
		if err != nil {
			return fmt.Errorf("amount must be a positive integer in paise")
		}
		requested = payment.Charge{PaymentID: args[1], AmountPaise: amount}
		if err := requested.Validate(); err != nil {
			return err
		}
	case "status":
		if len(args) != 2 {
			return fmt.Errorf("usage: merchant [flags] status <id>")
		}
	case "reconcile", "serve":
		if len(args) != 1 {
			return fmt.Errorf("usage: merchant [flags] %s", args[0])
		}
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
	if *timeout <= 0 {
		return fmt.Errorf("HTTP timeout must be positive")
	}

	db, err := payment.OpenDB(*dbPath, schema)
	if err != nil {
		return fmt.Errorf("open merchant database: %w", err)
	}
	defer db.Close()

	m := merchant{
		store:    store{db: db},
		client:   &http.Client{Timeout: *timeout},
		provider: strings.TrimRight(*provider, "/"),
	}
	switch args[0] {
	case "pay":
		return m.pay(requested)
	case "status":
		return m.status(args[1])
	case "serve":
		return m.serve(*addr)
	default:
		return m.reconcile()
	}
}

func (m merchant) pay(charge payment.Charge) error {
	if err := m.store.savePending(charge); err != nil {
		return err
	}
	local, err := m.store.lookup(charge.PaymentID)
	if err != nil {
		return err
	}
	if local.AmountPaise != charge.AmountPaise {
		return fmt.Errorf("payment %s already has amount %d paise", charge.PaymentID, local.AmountPaise)
	}
	if local.status == "paid" {
		if m.trace != nil {
			m.record("Merchant already marked the payment paid.")
		} else {
			printStatus(local.Charge, local.status)
		}
		return nil
	}

	m.record("Merchant saved pending.")
	if err := m.send(charge); err != nil {
		return fmt.Errorf("%s pending (%d paise): %w", charge.PaymentID, charge.AmountPaise, err)
	}
	return m.markPaid(charge)
}

func (m merchant) status(id string) error {
	local, err := m.store.lookup(id)
	if err != nil {
		return err
	}
	printStatus(local.Charge, local.status)
	return nil
}

func (m merchant) markPaid(charge payment.Charge) error {
	if err := m.store.markPaid(charge.PaymentID); err != nil {
		return err
	}
	if m.trace != nil {
		m.record("Merchant saved paid.")
	} else {
		printStatus(charge, "paid")
	}
	return nil
}

func (m merchant) record(message string) {
	if m.trace != nil {
		m.trace(message)
	}
}
