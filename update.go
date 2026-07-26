package main

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case copiedMsg:
		m.copied = false
		return m, nil

	case blinkMsg:
		m.cursorOn = !m.cursorOn
		if !m.ready {
			m.phase++
			if m.phase >= blinkPhases {
				m.ready = true
				return m, nil
			}
			return m, blinkTick()
		}
		// Keep blinking only while something is actually pending, so an idle
		// shop isn't redrawing itself forever.
		if m.tab == tabCart && m.step == stepPay {
			return m, blinkTick()
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case checkoutMsg:
		// Keep whatever Square just told us, so the shelf stops lying and a
		// retry has a chance of succeeding. Merged rather than replaced: an
		// earlier checkout's prices stay correct.
		if len(msg.fresh) > 0 {
			if m.fresh == nil {
				m.fresh = make(map[string]freshItem, len(msg.fresh))
			}
			for k, v := range msg.fresh {
				m.fresh[k] = v
			}
		}
		if msg.err != nil {
			m.checkoutErr = msg.err
			return m, nil
		}
		m.checkout = msg.out
		return m, pollPaid(msg.out.OrderID)

	case paidMsg:
		// Confirmation first, wherever they happen to be: the money has moved,
		// so an in-flight poll landing after they backed out still completes the
		// order rather than stranding a paid customer on the cart.
		if msg.paid {
			// Empty the cart the moment Square confirms, so the nav total goes
			// to zero on the order screen rather than lingering behind a letter
			// nobody has dismissed yet. The lines are kept for the receipt.
			m.placed, m.cart, m.cartCursor = m.cart, nil, 0
			m.step = stepDone
			return m, nil
		}
		// Otherwise stop once they have left checkout or the order is gone.
		// Without this the error path re-polls forever, and after esc then enter
		// clears m.checkout it re-polls an empty order id.
		if m.step != stepPay || m.checkout.OrderID == "" {
			return m, nil
		}
		// A failed poll is not a failed order. Keep waiting rather than telling
		// someone their payment did not go through.
		return m, pollPaid(m.checkout.OrderID)
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// any key skips the splash
	if !m.ready {
		m.ready = true
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			return m, tea.Quit
		}
		return m, nil
	}
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "tab":
		m.tab = (m.tab + 1) % 3
	case "shift+tab":
		m.tab = (m.tab + 2) % 3
	case "s":
		m.tab = tabShop
	case "a":
		m.tab = tabAccount
	case "c":
		m.tab = tabCart
	case "esc":
		m.back()
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "+", "=":
		m.changeQty(1)
	case "-", "_":
		m.changeQty(-1)
	case "enter":
		return m.advance()
	}
	return m, nil
}

// back steps out of checkout one screen at a time, and from the cart list out
// to the shop.
func (m *model) back() {
	switch {
	case m.tab == tabCart && m.step != stepCart:
		m.step = stepCart
		m.checkoutErr = nil
	case m.tab == tabCart:
		m.tab = tabShop
	}
}

// moveCursor walks whichever list the current tab is showing, by d rows. Both
// directions run through here so a bounds check can't be right going down and
// wrong coming back up.
func (m *model) moveCursor(d int) {
	switch m.tab {
	case tabShop:
		if next := m.cursor + d; next >= 0 && next < len(catalog) {
			m.cursor = next
			// The copied confirmation belongs to the book that was showing.
			m.copied = false
		}
	case tabAccount:
		if next := m.acct + d; next >= 0 && next < acctPageCount {
			m.acct = next
		}
	case tabCart:
		switch {
		case m.step == stepCart:
			if next := m.cartCursor + d; next >= 0 && next < len(m.cart) {
				m.cartCursor = next
			}
		case m.step == stepFulfil:
			// Two options rather than a list, so direction picks one outright.
			if d < 0 {
				m.fulfil = fulfilPickup
			} else {
				m.fulfil = fulfilShip
			}
		}
	}
}

// changeQty adds or removes one copy of whatever is under the cursor, on the
// shelf or in the cart.
func (m *model) changeQty(d int) {
	change := m.addToCart
	if d < 0 {
		change = m.removeFromCart
	}
	switch m.tab {
	case tabShop:
		change(m.cursor)
	case tabCart:
		if m.step == stepCart && m.cartCursor < len(m.cart) {
			change(m.cart[m.cartCursor].idx)
		}
	}
}

// advance is what enter does: copy a link on the shelf, or move checkout on a
// step. Checkout is the only place in the app that spends money, so the
// transitions are kept together rather than spread through the key switch.
func (m model) advance() (tea.Model, tea.Cmd) {
	switch {
	case m.tab == tabShop:
		// A server can't open a browser on someone else's machine, so
		// "open the link" means putting it on their clipboard.
		m.copied = true
		return m, tea.Batch(
			tea.SetClipboard(m.book(m.cursor).BuyURL()),
			tea.Tick(copiedFor, func(time.Time) tea.Msg { return copiedMsg{} }),
		)
	case m.tab == tabCart && m.step == stepCart && len(m.cart) > 0:
		m.step = stepFulfil
	case m.tab == tabCart && m.step == stepFulfil:
		m.step = stepPay
		m.checkoutErr = nil
		// The link from a previous attempt goes with the model field that held
		// it. Dropping the ID here and asking Square for another one left both
		// live, and only the newer one watched: the idempotency key is fresh
		// per attempt on purpose, so Square has no reason to collapse them.
		abandoned := m.checkout
		m.checkout = checkout{}
		ship, _ := m.shipping()
		if m.fulfil == fulfilPickup {
			ship = 0
		}
		return m, tea.Batch(
			discardLink(abandoned),
			startCheckout(m.snapshotCart(), m.fulfil, ship, newIdempotencyKey()),
			blinkTick(),
		)
	case m.tab == tabCart && m.step == stepDone:
		m.step = stepThanks
	case m.tab == tabCart && m.step == stepThanks:
		// Order's done: forget it and go back to the shelf.
		m.placed, m.cart, m.cartCursor, m.step = nil, nil, 0, stepCart
		m.checkout = checkout{}
		m.tab = tabShop
	}
	return m, nil
}
