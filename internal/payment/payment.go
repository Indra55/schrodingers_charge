package payment

import (
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	_ "modernc.org/sqlite"
)

type Charge struct {
	PaymentID   string `json:"payment_id"`
	AmountPaise int64  `json:"amount_paise"`
}

var paymentIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func (c Charge) Validate() error {
	if !paymentIDPattern.MatchString(c.PaymentID) {
		return fmt.Errorf("payment ID must contain 1–128 letters, digits, hyphens, or underscores")
	}
	if c.AmountPaise <= 0 {
		return fmt.Errorf("amount must be a positive number of paise")
	}
	return nil
}

func OpenDB(path, schema string) (*sql.DB, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	uriPath := filepath.ToSlash(absolutePath)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	dsn := url.URL{Scheme: "file", Path: uriPath, RawQuery: "_pragma=busy_timeout(5000)"}

	db, err := sql.Open("sqlite", dsn.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
