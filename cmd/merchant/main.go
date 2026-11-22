package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/Indra55/schrodingers_charge/internal/payment"
)

func main() {
	if err := run(os.Args[1:]); err != nil && !errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("merchant", flag.ContinueOnError)
	dbPath := flags.String("db", "merchant.db", "merchant SQLite database")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Usage: merchant [flags] status <id>")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	args = flags.Args()
	if len(args) != 2 || args[0] != "status" {
		return fmt.Errorf("usage: merchant [flags] status <id>")
	}

	db, err := payment.OpenDB(*dbPath, schema)
	if err != nil {
		return fmt.Errorf("open merchant database: %w", err)
	}
	defer db.Close()

	s := store{db: db}
	local, err := s.lookup(args[1])
	if err != nil {
		return err
	}
	printStatus(local.Charge, local.status)
	return nil
}
