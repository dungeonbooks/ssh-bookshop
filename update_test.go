package main

import (
	"errors"
	"testing"
)

// The checkout steps are the only path in the shop that takes money, and until
// now the only thing holding them in place was that they rendered. These drive
// the transitions directly.
//
// advance returns a tea.Cmd on the steps that talk to Square. The command is
// never run here, so nothing reaches the network: what matters is the state the
// buyer is left in, and whether a command was handed back at all.

// stockedShelf marks the shelf as a Square load would. The static catalog
// carries no price, stock or Sellable flag, so until Square fills those in
// addToCart correctly refuses everything: a test that skips this passes for the
// wrong reason. Books share the empty VariationID before Square assigns one, so
// a single entry covers the shelf.
func stockedShelf() map[string]freshItem {
	return map[string]freshItem{"": {cents: 3000, stock: 5, sellable: true}}
}

func cartModel(t *testing.T) model {
	t.Helper()
	m := newModel(100, 30, "fp")
	m.ready = true
	m.tab = tabCart
	m.fresh = stockedShelf()
	m.cart = []cartLine{{idx: 0, qty: 1}}
	return m
}

func TestAdvanceWalksCheckout(t *testing.T) {
	m := cartModel(t)

	mm, cmd := m.advance()
	m = mm.(model)
	if m.step != stepFulfil {
		t.Fatalf("step = %d after the cart, want stepFulfil", m.step)
	}
	if cmd != nil {
		t.Error("the cart step asked Square for something; nothing should be ordered yet")
	}

	mm, cmd = m.advance()
	m = mm.(model)
	if m.step != stepPay {
		t.Fatalf("step = %d after choosing fulfilment, want stepPay", m.step)
	}
	if cmd == nil {
		t.Error("reaching pay returned no command, so no order is being created")
	}

	// Square confirms out of band, so the buyer cannot walk from pay to done by
	// pressing enter. Only a paidMsg moves them.
	before := m.step
	mm, _ = m.advance()
	if got := mm.(model).step; got != before {
		t.Errorf("enter on the pay screen moved to step %d; only Square should", got)
	}
}

// An empty cart must not open checkout. Square would reject the order anyway,
// and finding out at the QR is a worse way to learn.
func TestAdvanceRefusesAnEmptyCart(t *testing.T) {
	m := newModel(100, 30, "fp")
	m.ready = true
	m.tab = tabCart

	mm, cmd := m.advance()
	if got := mm.(model).step; got != stepCart {
		t.Errorf("step = %d on an empty cart, want to stay on stepCart", got)
	}
	if cmd != nil {
		t.Error("an empty cart still asked Square for an order")
	}
}

// The thank-you screen is the reset: the order is done, so the next visitor to
// this session starts clean rather than inheriting a paid cart.
func TestAdvanceFromThanksClearsTheOrder(t *testing.T) {
	m := cartModel(t)
	m.step = stepThanks
	m.placed = []cartLine{{idx: 0, qty: 1}}
	m.cart = nil
	m.checkout = checkout{URL: "https://square.link/u/x", OrderID: "O1", LinkID: "L1"}
	m.cartCursor = 3

	mm, _ := m.advance()
	m = mm.(model)
	switch {
	case m.step != stepCart:
		t.Errorf("step = %d, want stepCart", m.step)
	case m.tab != tabShop:
		t.Errorf("tab = %d, want the shelf", m.tab)
	case m.placed != nil:
		t.Error("the receipt lines survived the reset")
	case m.cart != nil:
		t.Error("the cart survived the reset")
	case m.cartCursor != 0:
		t.Errorf("cartCursor = %d, want 0", m.cartCursor)
	case m.checkout.OrderID != "":
		t.Error("the finished order survived the reset, so the next one starts dirty")
	}
}

// esc unwinds checkout a screen at a time rather than dumping the buyer out of
// a cart they spent time filling.
func TestBackUnwindsOneStepAtATime(t *testing.T) {
	m := cartModel(t)
	m.step = stepPay
	m.checkoutErr = errors.New("square said no")

	m.back()
	if m.step != stepCart {
		t.Fatalf("step = %d, want stepCart", m.step)
	}
	if m.tab != tabCart {
		t.Error("esc left the cart tab as well as the step")
	}
	if m.checkoutErr != nil {
		t.Error("a stale checkout error followed the buyer back to the cart")
	}

	// Already at the cart list, so the next esc leaves for the shelf.
	m.back()
	if m.tab != tabShop {
		t.Errorf("tab = %d, want the shelf", m.tab)
	}
}

// changeQty is the +/- pair. On the shelf it acts on the highlighted book; in
// the cart, on the highlighted line.
func TestChangeQty(t *testing.T) {
	t.Run("on the shelf it adds and removes the highlighted book", func(t *testing.T) {
		m := newModel(100, 30, "fp")
		m.ready = true
		m.tab, m.cursor = tabShop, 0
		m.fresh = stockedShelf()

		m.changeQty(1)
		if m.cartCount() != 1 {
			t.Fatalf("cart holds %d, want 1", m.cartCount())
		}
		m.changeQty(-1)
		if m.cartCount() != 0 {
			t.Errorf("cart holds %d after removing, want 0", m.cartCount())
		}
	})

	t.Run("in the cart it acts on the line under the cursor", func(t *testing.T) {
		m := cartModel(t)
		m.cart = []cartLine{{idx: 0, qty: 1}, {idx: 1, qty: 1}}
		m.cartCursor = 1

		m.changeQty(1)
		if m.cart[1].qty != 2 {
			t.Errorf("line 1 qty = %d, want 2: the wrong line moved", m.cart[1].qty)
		}
		if m.cart[0].qty != 1 {
			t.Errorf("line 0 qty = %d, want it untouched", m.cart[0].qty)
		}
	})

	t.Run("it does nothing on the later checkout screens", func(t *testing.T) {
		m := cartModel(t)
		m.step = stepPay

		m.changeQty(1)
		if m.cartCount() != 1 {
			t.Errorf("cart changed to %d while the order was already with Square", m.cartCount())
		}
	})
}
