package main

import (
	"strings"
	"testing"
)

// TestCheckoutRejectsStale walks the guard that runs immediately before anyone
// is asked for money. Each case is a way the shelf can be wrong by the time
// someone checks out; none of them may reach Square as an order.
func TestCheckoutRejectsStale(t *testing.T) {
	orig := catalog[0]
	defer func() { catalog[0] = orig }()
	catalog[0].VariationID, catalog[0].Cents, catalog[0].Sellable = "VAR0", 3000, true

	lines := []cartLine{{idx: 0, qty: 2}}

	cases := []struct {
		name  string
		fresh freshItem
		want  string
	}{
		{"sold out since boot", freshItem{cents: 3000, stock: 0, sellable: false}, "just sold out"},
		{"fewer left than wanted", freshItem{cents: 3000, stock: 1, sellable: true}, "only 1 left"},
		{"price changed", freshItem{cents: 3500, stock: 9, sellable: true}, "is now $35.00"},
		{"untracked is fine", freshItem{cents: 3000, stock: 0, untracked: true, sellable: true}, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := verifyCart(lines, map[string]freshItem{"VAR0": tc.fresh})
			switch {
			case tc.want == "" && err != nil:
				t.Fatalf("rejected a valid cart: %v", err)
			case tc.want == "":
			case err == nil:
				t.Fatalf("accepted a stale cart, wanted %q", tc.want)
			case !strings.Contains(err.Error(), tc.want):
				t.Fatalf("got %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

// A cart holding a book Square no longer lists must not check out.
func TestCheckoutRejectsMissing(t *testing.T) {
	orig := catalog[0]
	defer func() { catalog[0] = orig }()
	catalog[0].VariationID, catalog[0].Cents, catalog[0].Sellable = "VAR0", 3000, true

	err := verifyCart([]cartLine{{idx: 0, qty: 1}}, map[string]freshItem{})
	if err == nil {
		t.Fatal("accepted a book Square no longer lists")
	}
}

// TestStockNote pins when the shelf warns about running low. The untracked case
// is the one that matters: those books report a count of zero, and without the
// guard every one of them would advertise "only 0 left".
func TestStockNote(t *testing.T) {
	cases := []struct {
		name string
		b    Book
		want string
	}{
		{"plenty", Book{Tracked: true, Sellable: true, Stock: 18}, ""},
		{"at threshold", Book{Tracked: true, Sellable: true, Stock: 3}, "only 3 left"},
		{"two", Book{Tracked: true, Sellable: true, Stock: 2}, "only 2 left"},
		{"one", Book{Tracked: true, Sellable: true, Stock: 1}, "only 1 left"},
		{"untracked reads as zero", Book{Tracked: false, Sellable: true, Stock: 0}, ""},
		{"sold out says nothing here", Book{Tracked: true, Sellable: false, Stock: 0}, ""},
		{"oversold says nothing here", Book{Tracked: true, Sellable: false, Stock: -3}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.b.stockNote()
			if tc.want == "" && got != "" {
				t.Fatalf("got %q, want nothing", got)
			}
			if tc.want != "" && !strings.Contains(got, tc.want) {
				t.Fatalf("got %q, want it to mention %q", got, tc.want)
			}
		})
	}
}
