package main

import (
	"strings"
	"testing"
)

// TestCheckoutRejectsStale walks the guard that runs immediately before anyone
// is asked for money. Each case is a way the shelf can be wrong by the time
// someone checks out; none of them may reach Square as an order.
func TestCheckoutRejectsStale(t *testing.T) {
	items := []cartItem{{isbn: "9780000000001", variationID: "VAR0", title: "A Book", cents: 3000, qty: 2}}

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
			err := verifyCart(items, map[string]freshItem{"VAR0": tc.fresh})
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
	err := verifyCart([]cartItem{{variationID: "VAR0", title: "A Book", cents: 3000, qty: 1}}, map[string]freshItem{})
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

// TestBuyURLFallsBackWhenSoldOut covers the case that only shows up after a
// book sells out mid-session: our own product page becomes a dead end, and the
// chip already promises Bookshop, so the link has to actually go there.
func TestBuyURLFallsBackWhenSoldOut(t *testing.T) {
	ours := "https://www.dungeonbooks.com/product/x/ABC"
	theirs := "https://bookshop.org/a/108216/9781250406811"

	cases := []struct {
		name string
		b    Book
		want string
	}{
		{"in stock uses our store", Book{ISBN: "9780000000001", URL: ours, Sellable: true}, ours},
		{"sold out leaves our store", Book{ISBN: "9780000000001", URL: ours, Sellable: false}, "bookshop.org/a/108216/9780000000001"},
		{"hand-picked bookshop link is kept", Book{ISBN: "9781250406828", URL: theirs, Sellable: false}, theirs},
		{"no url at all", Book{ISBN: "9780000000002", Sellable: true}, "bookshop.org/a/108216/9780000000002"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.b.BuyURL(); !strings.Contains(got, tc.want) {
				t.Fatalf("got %q, want it to contain %q", got, tc.want)
			}
		})
	}
}

// TestParseQuantity covers Square's stringly-typed counts. The failure mode
// that matters is an unreadable one being treated as "no inventory tracking",
// which would mark a book sellable.
func TestParseQuantity(t *testing.T) {
	ok := map[string]int{"3": 3, "3.0": 3, "3.00": 3, "0": 0, "-3": -3, " 12 ": 12, "2.9": 2}
	for in, want := range ok {
		got, err := parseQuantity(in)
		if err != nil || got != want {
			t.Errorf("parseQuantity(%q) = %d, %v; want %d, nil", in, got, err, want)
		}
	}
	for _, in := range []string{"", "  ", "lots", "3,000"} {
		if _, err := parseQuantity(in); err == nil {
			t.Errorf("parseQuantity(%q) accepted an unreadable count", in)
		}
	}
}

// TestAddToCartRespectsStock: the cart must not accept more copies than the
// shelf holds. Checkout would refuse it anyway, but finding out at the payment
// screen is a worse way to learn.
func TestAddToCartRespectsStock(t *testing.T) {
	orig := catalog[0]
	defer func() { catalog[0] = orig }()

	set := func(stock int, tracked, sellable bool) {
		catalog[0].VariationID, catalog[0].Cents = "VAR0", 3000
		catalog[0].Stock, catalog[0].Tracked, catalog[0].Sellable = stock, tracked, sellable
	}

	t.Run("stops at the last copy", func(t *testing.T) {
		set(2, true, true)
		var m model
		for i := 0; i < 5; i++ {
			m.addToCart(0)
		}
		if got := m.qtyInCart(0); got != 2 {
			t.Fatalf("cart holds %d of 2 in stock", got)
		}
	})

	t.Run("untracked has no cap", func(t *testing.T) {
		set(0, false, true)
		var m model
		for i := 0; i < 3; i++ {
			m.addToCart(0)
		}
		if got := m.qtyInCart(0); got != 3 {
			t.Fatalf("untracked book capped at %d, should be uncapped", got)
		}
	})

	t.Run("zero stock never enters the cart", func(t *testing.T) {
		set(0, true, true) // sellable flag stale, count says otherwise
		var m model
		m.addToCart(0)
		if got := m.qtyInCart(0); got != 0 {
			t.Fatalf("added %d copies of a book with none in stock", got)
		}
	})

	t.Run("sold out never enters the cart", func(t *testing.T) {
		set(-3, true, false)
		var m model
		m.addToCart(0)
		if got := m.qtyInCart(0); got != 0 {
			t.Fatalf("added %d copies of a sold out book", got)
		}
	})
}
