package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// Square is the source of truth for price and for taking money. Books are
// looked up by ISBN, which every variation carries as its UPC.
//
// Checkout hands off to a hosted Square page rather than taking card details
// here. That is not a shortcut: CreatePayment only accepts a token from the Web
// Payments or In-App Payments SDK, so there is no server-side path for a card
// number, and typing one into an SSH session would put this process in PCI
// scope. The buyer gets a square.link URL; Square handles card, address, tax.
const (
	squareVersion = "2025-01-23"
	squareTimeout = 10 * time.Second
	// Boot blocks on this, so it is a fixed ceiling rather than one that grows
	// with the shelf. Lookups run concurrently, capped so a bigger shelf costs
	// round trips rather than a longer outage.
	bootTimeout     = 20 * time.Second
	bootConcurrency = 4
)

type squareClient struct {
	token      string
	locationID string
	// baseURL is the API root, including scheme. Overridden in tests to point
	// at an httptest server; empty means the real Square host.
	baseURL string
}

var sq *squareClient

type squareMoney struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

func squareHost() string {
	if os.Getenv("SQUARE_ENVIRONMENT") == "sandbox" {
		return "connect.squareupsandbox.com"
	}
	return "connect.squareup.com"
}

func (c *squareClient) root() string {
	if c.baseURL != "" {
		// Paths all start with /, so a trailing slash here would double it.
		return strings.TrimRight(c.baseURL, "/")
	}
	return "https://" + squareHost()
}

// call is every Square request: auth, version, JSON in and out. Square reports
// failures in a 200-shaped body as often as by status code, so both are checked.
func (c *squareClient) call(ctx context.Context, method, path string, body, out any) error {
	var rdr *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.root()+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Square-Version", squareVersion)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var probe struct {
		Errors []struct {
			Code   string `json:"code"`
			Detail string `json:"detail"`
		} `json:"errors"`
	}
	raw := &bytes.Buffer{}
	if _, err := raw.ReadFrom(resp.Body); err != nil {
		return err
	}
	if err := json.Unmarshal(raw.Bytes(), &probe); err == nil && len(probe.Errors) > 0 {
		return fmt.Errorf("square: %s %s", probe.Errors[0].Code, probe.Errors[0].Detail)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("square: HTTP %d", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw.Bytes(), out)
}
