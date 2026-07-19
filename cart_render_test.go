package main

import (
	"regexp"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestCartRender fills a cart with fake Square data and prints the view, so the
// box layout can be checked without a live catalog or a real order.
func TestCartRender(t *testing.T) {
	defer restoreCatalog(catalog[0], catalog[1])
	catalog[0].Cents, catalog[0].VariationID, catalog[0].Sellable = 3000, "VAR0", true
	catalog[1].Cents, catalog[1].VariationID, catalog[1].Sellable = 2899, "VAR1", true

	m := newModel(100, 30, "SHA256:test")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = mm.(model)
	m.ready = true
	m.addToCart(0)
	m.addToCart(0)
	m.addToCart(1)
	m.tab = tabCart

	t.Log("\n" + ansi.ReplaceAllString(m.render(), ""))

	if got := m.cartCount(); got != 3 {
		t.Errorf("cartCount = %d, want 3", got)
	}
	if got := m.cartTotal(); got != 3000*2+2899 {
		t.Errorf("cartTotal = %d", got)
	}
}

// TestCartRowWidthsStable checks the terminal.shop detail: focusing a row swaps
// the +/- glyphs in for spaces, so nothing shifts as the cursor moves.
func TestCartRowWidthsStable(t *testing.T) {
	defer restoreCatalog(catalog[0])
	catalog[0].Cents, catalog[0].VariationID, catalog[0].Sellable = 3000, "VAR0", true
	m := newModel(100, 30, "k")
	m.addToCart(0)

	strip := func(s string) string { return ansi.ReplaceAllString(s, "") }
	m.cartCursor = 0
	focused := strip(m.cartRow(58, 0, m.cart[0]))
	m.cartCursor = 1 // nothing selected
	unfocused := strip(m.cartRow(58, 0, m.cart[0]))

	fl, ul := regexp.MustCompile(`\n`).Split(focused, -1), regexp.MustCompile(`\n`).Split(unfocused, -1)
	for i := range fl {
		if len([]rune(fl[i])) != len([]rune(ul[i])) {
			t.Errorf("line %d width differs: focused %q vs unfocused %q", i, fl[i], ul[i])
		}
	}
}

// TestPayRender shows the QR handoff with a stand-in URL, so the layout can be
// checked without creating a real payment link.
func TestPayRender(t *testing.T) {
	defer restoreCatalog(catalog[0])
	catalog[0].Cents, catalog[0].VariationID, catalog[0].Sellable = 3000, "VAR0", true
	m := newModel(100, 40, "k")
	m.ready = true
	m.addToCart(0)
	m.tab, m.step = tabCart, stepPay
	m.checkout = checkout{URL: "https://square.link/u/AbCd1234", OrderID: "ORDER123"}
	t.Log("\n" + ansi.ReplaceAllString(m.render(), ""))
}

func TestThanksRender(t *testing.T) {
	m := newModel(100, 30, "k")
	m.ready = true
	m.tab, m.step = tabCart, stepThanks
	t.Log("\n" + ansi.ReplaceAllString(m.render(), ""))
}

// restoreCatalog puts the shared shelf back after a test edits it, so the suite
// doesn't depend on the order tests happen to run in.
func restoreCatalog(books ...Book) {
	for _, b := range books {
		for i := range catalog {
			if catalog[i].ISBN == b.ISBN {
				catalog[i] = b
			}
		}
	}
}
