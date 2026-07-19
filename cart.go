package main

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// cartLine is a quantity of one catalog entry. Books are held by index rather
// than copied, so a price refresh reaches the cart too.
type cartLine struct {
	idx int
	qty int
}

// Checkout runs as a small state machine inside the cart tab.
type step int

const (
	stepCart   step = iota // reviewing line items
	stepPay                // showing the QR and waiting on Square
	stepDone               // Square says it's paid
	stepThanks             // the letter
)

// pollEvery is how often we ask Square whether the buyer has paid. They are
// filling in a card on another device, so this is minutes-scale work.
const pollEvery = 3 * time.Second

type checkoutMsg struct {
	out checkout
	err error
}

type paidMsg struct {
	paid bool
	err  error
}

// startCheckout builds the order on Square and returns a hosted link. The
// idempotency key is per attempt, so a retry after a network error doesn't
// create a second order.
func startCheckout(lines []cartLine, key string) tea.Cmd {
	return func() tea.Msg {
		if sq == nil {
			return checkoutMsg{err: fmt.Errorf("square is not configured")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), squareTimeout)
		defer cancel()
		out, err := sq.createLink(ctx, lines, key)
		return checkoutMsg{out: out, err: err}
	}
}

func pollPaid(orderID string) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(pollEvery)
		if sq == nil {
			return paidMsg{err: fmt.Errorf("square is not configured")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), squareTimeout)
		defer cancel()
		paid, err := sq.paid(ctx, orderID)
		return paidMsg{paid: paid, err: err}
	}
}

// --- cart maths ------------------------------------------------------------

func (m *model) addToCart(idx int) {
	// Only books Square can sell. Everything else links out to Bookshop, and
	// offering a cart we can't fulfil would be a lie.
	if catalog[idx].VariationID == "" {
		return
	}
	for i := range m.cart {
		if m.cart[i].idx == idx {
			m.cart[i].qty++
			return
		}
	}
	m.cart = append(m.cart, cartLine{idx: idx, qty: 1})
}

func (m *model) removeFromCart(idx int) {
	for i := range m.cart {
		if m.cart[i].idx != idx {
			continue
		}
		m.cart[i].qty--
		if m.cart[i].qty <= 0 {
			m.cart = append(m.cart[:i], m.cart[i+1:]...)
			if m.cartCursor >= len(m.cart) && m.cartCursor > 0 {
				m.cartCursor--
			}
		}
		return
	}
}

func (m model) cartCount() int {
	n := 0
	for _, l := range m.cart {
		n += l.qty
	}
	return n
}

func (m model) cartTotal() int64 {
	var cents int64
	for _, l := range m.cart {
		cents += catalog[l.idx].Cents * int64(l.qty)
	}
	return cents
}

// qtyInCart is what the shop's detail view shows, so the count is visible
// without switching tabs.
func (m model) qtyInCart(idx int) int {
	for _, l := range m.cart {
		if l.idx == idx {
			return l.qty
		}
	}
	return 0
}
