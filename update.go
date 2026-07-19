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
			// Back out of checkout one step at a time; from the cart list, out
			// to the shop.
			switch {
			case m.tab == tabCart && m.step != stepCart:
				m.step = stepCart
				m.checkoutErr = nil
			case m.tab == tabCart:
				m.tab = tabShop
			}
		case "up", "k":
			switch m.tab {
			case tabShop:
				if m.cursor > 0 {
					m.cursor--
					m.copied = false
				}
			case tabAccount:
				if m.acct > 0 {
					m.acct--
				}
			case tabCart:
				switch {
				case m.step == stepCart && m.cartCursor > 0:
					m.cartCursor--
				case m.step == stepFulfil:
					m.fulfil = fulfilPickup
				}
			}
		case "down", "j":
			switch m.tab {
			case tabShop:
				if m.cursor < len(catalog)-1 {
					m.cursor++
					m.copied = false
				}
			case tabAccount:
				if m.acct < acctPageCount-1 {
					m.acct++
				}
			case tabCart:
				switch {
				case m.step == stepCart && m.cartCursor < len(m.cart)-1:
					m.cartCursor++
				case m.step == stepFulfil:
					m.fulfil = fulfilShip
				}
			}
		case "+", "=":
			switch m.tab {
			case tabShop:
				m.addToCart(m.cursor)
			case tabCart:
				if m.step == stepCart && m.cartCursor < len(m.cart) {
					m.addToCart(m.cart[m.cartCursor].idx)
				}
			}
		case "-", "_":
			switch m.tab {
			case tabShop:
				m.removeFromCart(m.cursor)
			case tabCart:
				if m.step == stepCart && m.cartCursor < len(m.cart) {
					m.removeFromCart(m.cart[m.cartCursor].idx)
				}
			}
		case "enter":
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
				m.checkout = checkout{}
				ship, _ := m.shipping()
				if m.fulfil == fulfilPickup {
					ship = 0
				}
				return m, tea.Batch(
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
		}

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
