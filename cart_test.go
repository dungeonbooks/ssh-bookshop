package main

import "testing"

// removeFromCart both drops the line and pulls the cursor back with it. Getting
// the second part wrong strands the cursor past the end of the slice, which the
// cart screen then indexes.
func TestRemoveFromCart(t *testing.T) {
	// Three lines so a removal from the middle can be told from one off the end.
	fresh := func() model {
		m := newModel(100, 30, "fp")
		m.cart = []cartLine{{idx: 0, qty: 2}, {idx: 1, qty: 1}, {idx: 2, qty: 1}}
		return m
	}

	t.Run("a quantity above one just decrements", func(t *testing.T) {
		m := fresh()
		m.removeFromCart(0)
		if len(m.cart) != 3 {
			t.Fatalf("cart has %d lines, want 3: the line was dropped too early", len(m.cart))
		}
		if m.cart[0].qty != 1 {
			t.Errorf("qty = %d, want 1", m.cart[0].qty)
		}
	})

	t.Run("the last copy drops the line", func(t *testing.T) {
		m := fresh()
		m.removeFromCart(1)
		if len(m.cart) != 2 {
			t.Fatalf("cart has %d lines, want 2", len(m.cart))
		}
		for _, l := range m.cart {
			if l.idx == 1 {
				t.Error("book 1 is still in the cart")
			}
		}
	})

	t.Run("the cursor follows a line removed from under it", func(t *testing.T) {
		m := fresh()
		m.cartCursor = 2 // on the last line
		m.removeFromCart(2)
		if m.cartCursor != 1 {
			t.Errorf("cartCursor = %d, want 1: it is past the end of a 2-line cart", m.cartCursor)
		}
	})

	t.Run("emptying the cart leaves the cursor at zero", func(t *testing.T) {
		m := newModel(100, 30, "fp")
		m.cart = []cartLine{{idx: 0, qty: 1}}
		m.removeFromCart(0)
		if len(m.cart) != 0 {
			t.Fatalf("cart has %d lines, want 0", len(m.cart))
		}
		if m.cartCursor != 0 {
			t.Errorf("cartCursor = %d, want 0", m.cartCursor)
		}
	})

	t.Run("a book that is not in the cart changes nothing", func(t *testing.T) {
		m := fresh()
		m.removeFromCart(5)
		if len(m.cart) != 3 {
			t.Errorf("cart has %d lines, want 3", len(m.cart))
		}
	})
}
