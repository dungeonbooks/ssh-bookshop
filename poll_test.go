package main

import (
	"errors"
	"testing"
)

func payingModel() model {
	m := newModel(100, 40, "k")
	m.ready = true
	m.tab, m.step = tabCart, stepPay
	m.checkout = checkout{URL: "https://square.link/u/E9zH3J5R", OrderID: "O1"}
	return m
}

// TestPollStopsAfterLeavingCheckout guards the loop that ran forever: the error
// path re-polled with no check on where the shopper had got to, so pressing esc
// during checkout left the session polling Square every few seconds until it
// disconnected.
func TestPollStopsAfterLeavingCheckout(t *testing.T) {
	m := payingModel()
	m.step = stepCart // esc

	_, cmd := m.Update(paidMsg{err: errors.New("square unreachable")})
	if cmd != nil {
		t.Error("kept polling after the shopper left checkout")
	}
}

// TestPollStopsWithoutAnOrder is the sharper version of the same bug: esc then
// enter clears m.checkout, so the next failed poll asked Square for order "".
func TestPollStopsWithoutAnOrder(t *testing.T) {
	m := payingModel()
	m.checkout = checkout{} // esc, then enter

	_, cmd := m.Update(paidMsg{err: errors.New("square unreachable")})
	if cmd != nil {
		t.Error("kept polling with an empty order id")
	}
}

// TestPollKeepsGoingWhileWaiting is the other side: a failed poll must not be
// mistaken for a failed order while the shopper is still on the QR.
func TestPollKeepsGoingWhileWaiting(t *testing.T) {
	for _, msg := range []paidMsg{{err: errors.New("timeout")}, {paid: false}} {
		_, cmd := payingModel().Update(msg)
		if cmd == nil {
			t.Errorf("%+v: stopped polling while still waiting for payment", msg)
		}
	}
}

// TestPaymentConfirmedWhereverTheyAre pins the ordering. Money has moved by the
// time paid is true, so a poll landing after the shopper backed out must still
// complete the order rather than stranding them on the cart having been charged.
func TestPaymentConfirmedWhereverTheyAre(t *testing.T) {
	m := payingModel()
	m.cart = []cartLine{{idx: 0, qty: 1}}
	m.step = stepCart // backed out, then the payment lands

	got, cmd := m.Update(paidMsg{paid: true})
	m2 := got.(model)
	if m2.step != stepDone {
		t.Errorf("step = %v, want stepDone: a paid order was dropped", m2.step)
	}
	if len(m2.cart) != 0 {
		t.Error("cart not emptied after payment")
	}
	if len(m2.placed) != 1 {
		t.Error("placed lines not kept for the receipt")
	}
	if cmd != nil {
		t.Error("kept polling after the order completed")
	}
}
