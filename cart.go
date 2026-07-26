package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
)

// cartLine is a quantity of one catalog entry. Books are held by index rather
// than copied, so a price refresh reaches the cart too.
type cartLine struct {
	idx int
	qty int
}

// cartItem is a cart line frozen for handoff to a background command. Checkout
// runs off the UI goroutine, so it is given its own copy of everything it needs
// rather than an index into the shared catalog.
type cartItem struct {
	isbn        string
	variationID string
	title       string
	cents       int64
	qty         int
}

func (m model) snapshotCart() []cartItem {
	out := make([]cartItem, 0, len(m.cart))
	for _, l := range m.cart {
		b := m.book(l.idx)
		out = append(out, cartItem{
			isbn:        b.ISBN,
			variationID: b.VariationID,
			title:       b.BookTitle,
			cents:       b.Cents,
			qty:         l.qty,
		})
	}
	return out
}

// Checkout runs as a small state machine inside the cart tab.
type step int

const (
	stepCart   step = iota // reviewing line items
	stepFulfil             // pickup at the shop, or ship it
	stepPay                // showing the QR and waiting on Square
	stepDone               // Square says it's paid
	stepThanks             // the letter
)

// How the order reaches the buyer. Square's hosted page cannot offer the choice,
// so it is made here and baked into the order we hand it.
type fulfilment int

const (
	fulfilPickup fulfilment = iota
	fulfilShip
)

// pollEvery is how often we ask Square whether the buyer has paid. They are
// filling in a card on another device, so this is minutes-scale work.
const pollEvery = 3 * time.Second

type checkoutMsg struct {
	out   checkout
	fresh map[string]freshItem // what Square said just now, applied to the shelf
	err   error
}

type paidMsg struct {
	paid bool
	err  error
}

// newIdempotencyKey is fresh per checkout attempt. Square remembers keys well
// past the life of the link they created, so anything derived from the session
// or the order number collides the second time someone checks out: abandoning a
// checkout and starting again is the common case, and it must work.
//
// Nothing here retries automatically, so a per-attempt key costs us nothing:
// the protection idempotency exists for is against a retry we never send.
func newIdempotencyKey() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Fall back to the clock rather than a constant, which would collide
		// on the very next attempt.
		return fmt.Sprintf("t-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// startCheckout builds the order on Square and returns a hosted link.
func startCheckout(items []cartItem, how fulfilment, shipCents int64, key string) tea.Cmd {
	return func() tea.Msg {
		if sq == nil {
			return checkoutMsg{err: fmt.Errorf("square is not configured")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), squareTimeout)
		defer cancel()
		out, fresh, err := sq.createLink(ctx, items, how, shipCents, key)
		return checkoutMsg{out: out, fresh: fresh, err: err}
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
	b := m.book(idx)
	// Only books we can actually hand over. Everything else links out to
	// Bookshop, because offering a cart we can't fulfil would be a lie.
	if !b.Sellable {
		return
	}
	for i := range m.cart {
		if m.cart[i].idx != idx {
			continue
		}
		// Don't let someone stack up copies we don't have. Checkout would
		// refuse it anyway, and finding out then is a worse way to learn.
		// Untracked books have no count to cap against.
		if b.Tracked && m.cart[i].qty >= b.Stock {
			return
		}
		m.cart[i].qty++
		return
	}
	if b.Tracked && b.Stock < 1 {
		return
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
		cents += m.book(l.idx).Cents * int64(l.qty)
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

// discardLink deletes a payment link the shopper walked away from, so backing
// out of checkout and starting again does not leave two live links for one
// cart. Square links never expire, so the abandoned one stays payable: if the
// buyer had already copied or scanned it, paying it charges them against an
// order the shop is no longer watching, with whatever fulfilment they have
// since changed their mind about.
//
// Paid orders are left alone, checked the same way the sweeper checks them.
// Deleting a link cancels its order, and an order that was paid a moment ago is
// a real sale. An order we cannot read is not permission to cancel it either.
//
// Fire and forget: the shopper is already waiting on the new link, and tidying
// up the old one is never worth making them watch it happen.
//
// A failure is not free, though. This link is seconds old, so the sweeper will
// not consider it until it passes sweepAfter, and it stays live and payable for
// that whole day: exactly the window this is meant to close, reopened for one
// shopper. Accepted rather than retried because the alternative is holding up
// the pay screen on cleanup, and the sweeper does eventually get it.
func discardLink(c checkout) tea.Cmd {
	if c.LinkID == "" {
		return nil
	}
	return func() tea.Msg {
		if sq == nil {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), squareTimeout)
		defer cancel()
		if c.OrderID != "" {
			paid, err := sq.paid(ctx, c.OrderID)
			if err != nil || paid {
				return nil
			}
		}
		_ = sq.deleteLink(ctx, paymentLink{ID: c.LinkID, OrderID: c.OrderID})
		return nil
	}
}
