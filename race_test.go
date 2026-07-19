package main

import (
	"sync"
	"testing"
)

// TestNoCatalogRace runs the two things a real deployment does at once: one
// session applying what Square said at checkout, another rendering the shelf.
// Both used to touch the package-level catalog, one writing and one reading.
//
// The suite missed it for a long time because every other test builds a single
// model on the test goroutine, and the detector only reports races it observes.
func TestNoCatalogRace(t *testing.T) {
	fresh := map[string]freshItem{
		catalog[0].VariationID: {cents: 1234, stock: 5, sellable: true},
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { // checking out
		defer wg.Done()
		m := newModel(100, 40, "a")
		m.ready, m.tab, m.step = true, tabCart, stepPay
		for i := 0; i < 200; i++ {
			m.Update(checkoutMsg{out: checkout{URL: "https://square.link/u/X", OrderID: "O1"}, fresh: fresh})
		}
	}()
	go func() { // browsing
		defer wg.Done()
		m := newModel(100, 40, "b")
		m.ready = true
		for i := 0; i < 200; i++ {
			_ = m.render()
		}
	}()
	wg.Wait()
}

// TestFreshPricesAreSessionLocal is the behavioural half: one session's checkout
// must not rewrite what another session sees.
func TestFreshPricesAreSessionLocal(t *testing.T) {
	before := catalog[0].Cents

	m := newModel(100, 40, "a")
	m.tab, m.step = tabCart, stepPay
	got, _ := m.Update(checkoutMsg{
		out:   checkout{URL: "https://square.link/u/X", OrderID: "O1"},
		fresh: map[string]freshItem{catalog[0].VariationID: {cents: before + 500, stock: 3, sellable: true}},
	})
	updated := got.(model)

	if catalog[0].Cents != before {
		t.Errorf("shared catalog was mutated: %d -> %d", before, catalog[0].Cents)
	}
	if updated.book(0).Cents != before+500 {
		t.Errorf("session price = %d, want %d", updated.book(0).Cents, before+500)
	}
	if other := newModel(100, 40, "b"); other.book(0).Cents != before {
		t.Errorf("another session saw %d, want the catalog price %d", other.book(0).Cents, before)
	}
}
