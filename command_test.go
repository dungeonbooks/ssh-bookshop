package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dungeonbooks/ssh-bookshop/internal/shopapi"
)

func run(t *testing.T, s *apiShop, args ...string) (code int, out map[string]any, stderr string) {
	t.Helper()
	var so, se bytes.Buffer
	code = runCommand(context.Background(), s, "test-key", args, &so, &se)
	if so.Len() > 0 {
		if err := json.Unmarshal(so.Bytes(), &out); err != nil {
			t.Fatalf("%v: stdout is not JSON: %v\n%s", args, err, so.String())
		}
	}
	return code, out, se.String()
}

func TestCommandMode(t *testing.T) {
	s, st := testShop(t)

	if code, out, _ := run(t, s, "books"); code != exitOK || len(out["books"].([]any)) != 2 {
		t.Errorf("books: code %d, %v", code, out)
	}
	if code, out, _ := run(t, s, "books", isbnStocked); code != exitOK || out["isbn"] != isbnStocked {
		t.Errorf("books isbn: code %d, %v", code, out)
	}
	if code, _, se := run(t, s, "books", "000"); code != exitUsage || !strings.Contains(se, "not on the shelf") {
		t.Errorf("unknown isbn: code %d, stderr %q", code, se)
	}

	code, out, _ := run(t, s, "buy", isbnStocked+":2", "--ship")
	if code != exitOK || out["checkout_url"] == "" || out["subtotal_cents"] != 6000.0 {
		t.Errorf("buy: code %d, %v", code, out)
	}
	if code, _, se := run(t, s, "buy", isbnStocked); code != exitUsage || !strings.Contains(se, "fulfilment") {
		t.Errorf("buy without fulfilment: code %d, stderr %q", code, se)
	}
	if code, _, se := run(t, s, "buy", "--pickup"); code != exitUsage || !strings.Contains(se, "at least one isbn") {
		t.Errorf("buy nothing: code %d, stderr %q", code, se)
	}
	if code, _, _ := run(t, s, "buy", isbnStocked+":x", "--pickup"); code != exitUsage {
		t.Errorf("bad qty: code %d", code)
	}

	if code, out, _ := run(t, s, "order", "ORD1"); code != exitWaiting || out["state"] != shopapi.StateAwaitingPayment {
		t.Errorf("order unpaid: code %d, %v", code, out)
	}
	st.paid = true
	if code, _, _ := run(t, s, "order", "ORD1"); code != exitOK {
		t.Errorf("order paid: code %d", code)
	}
	st.paid = false
	if code, out, _ := run(t, s, "cancel", "LINK1"); code != exitOK || out["cancelled"] != "LINK1" {
		t.Errorf("cancel: code %d, %v", code, out)
	}

	if code, _, se := run(t, s, "dance"); code != exitUsage || !strings.Contains(se, "usage:") || !strings.HasPrefix(se, "{") {
		t.Errorf("unknown command: code %d, stderr %q (want JSON carrying the usage)", code, se)
	}
	// A refusal from the shelf is the caller's to fix, so it exits 2 as in the
	// CLI, not 1.
	if code, _, se := run(t, s, "buy", isbnGone, "--pickup"); code != exitUsage || !strings.Contains(se, "bookshop.org") {
		t.Errorf("buy a book we do not carry: code %d, stderr %q", code, se)
	}
}

// TestCommandBuyIsThrottled: the SSH path spends the same checkout budget as
// HTTP, otherwise it would be the way around the limit.
func TestCommandBuyIsThrottled(t *testing.T) {
	s, st := testShop(t)
	var limited int
	for i := 0; i < checkoutAddrBurst+2; i++ {
		if code, _, se := run(t, s, "buy", isbnStocked, "--pickup"); code == exitError && strings.Contains(se, "too many") {
			limited++
		}
	}
	if limited != 2 || st.links != checkoutAddrBurst {
		t.Errorf("limited %d, links %d; want 2 and %d", limited, st.links, checkoutAddrBurst)
	}
}

func TestParseBuy(t *testing.T) {
	req, err := parseBuy([]string{"--key", "k1", "111:3", "222", "--pickup"})
	if err != nil {
		t.Fatal(err)
	}
	if req.IdempotencyKey != "k1" || req.Fulfilment != shopapi.FulfilPickup || len(req.Items) != 2 ||
		req.Items[0].Qty != 3 || req.Items[1].Qty != 1 {
		t.Errorf("parsed %+v", req)
	}
	if _, err := parseBuy([]string{"111", "--drone"}); err == nil {
		t.Error("accepted an unknown flag")
	}
	if _, err := parseBuy([]string{"111", "--pickup", "--ship"}); err == nil {
		t.Error("accepted both fulfilments; the last one would silently win")
	}
}
