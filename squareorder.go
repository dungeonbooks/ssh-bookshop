package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

type checkout struct {
	URL     string
	OrderID string
	LinkID  string // needed to delete the link; Square links never expire
}

// verifyCart is the last gate before someone is asked for money: it compares
// the cart against what Square says right now. Split out from createLink so the
// rules can be tested without creating an order.
func verifyCart(items []cartItem, fresh map[string]freshItem) error {
	_, err := verifyCartItems(items, fresh)
	return err
}

// cartError is a cart Square will not take money for as it stands: sold out,
// short, or repriced since the shelf was read. Typed so the API can tell a
// stale cart, which is the buyer's to fix, from Square being unreachable,
// which is not. isbn names the line that failed and cents is its price now,
// so a retry can be built without a second round trip.
type cartError struct {
	msg   string
	isbn  string
	cents int64
}

func (e *cartError) Error() string { return e.msg }

func verifyCartItems(items []cartItem, fresh map[string]freshItem) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		f, ok := fresh[it.variationID]
		switch {
		case !ok || !f.sellable:
			return nil, &cartError{msg: fmt.Sprintf("%s just sold out", it.title), isbn: it.isbn}
		case !f.untracked && f.stock < it.qty:
			return nil, &cartError{msg: fmt.Sprintf("only %d left of %s", f.stock, it.title), isbn: it.isbn, cents: f.cents}
		case f.cents != it.cents:
			// Better to send them back to a corrected shelf than to quote one
			// price and charge another. The fresh figures go back with the
			// error so the shelf updates and a retry isn't doomed to repeat.
			return nil, &cartError{msg: fmt.Sprintf("%s is now %s, not %s", it.title, usd(f.cents), usd(it.cents)), isbn: it.isbn, cents: f.cents}
		}
		out = append(out, map[string]any{
			"catalog_object_id": it.variationID,
			"quantity":          fmt.Sprint(it.qty),
		})
	}
	return out, nil
}

// freshItem is what Square says about a book right now, as opposed to at boot.
type freshItem struct {
	cents     int64
	stock     int
	untracked bool
	sellable  bool
}

// sellable decides whether the shop will take money for a book: stock we do not
// track is always sellable, tracked stock only while some remains. Square will
// sell past zero, so this is ours to enforce.
func sellable(untracked bool, qty int) bool {
	return untracked || qty > 0
}

// recheck re-reads price and stock for everything in the cart, keyed by
// variation id.
func (c *squareClient) recheck(ctx context.Context, items []cartItem) (map[string]freshItem, error) {
	out := map[string]freshItem{}
	var ids []string
	prices := map[string]int64{}
	for _, it := range items {
		id, cents, err := c.lookup(ctx, it.isbn)
		if err != nil {
			return nil, err
		}
		if id == "" {
			// Gone from the catalog entirely. Recorded as unsellable rather
			// than skipped, so the shelf that reads this back stops offering
			// the book instead of sending every retry into the same refusal.
			out[it.variationID] = freshItem{}
			continue
		}
		ids = append(ids, id)
		prices[id] = cents
	}

	qty, untracked, err := c.stock(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		out[id] = freshItem{
			cents:     prices[id],
			stock:     qty[id],
			untracked: untracked[id],
			sellable:  sellable(untracked[id], qty[id]),
		}
	}
	return out, nil
}

// createLink builds the buyer's cart on Square and returns a hosted checkout.
// Passing an order (rather than an ad hoc name and price) is what gets us
// itemisation, tax, and inventory for free.
func (c *squareClient) createLink(ctx context.Context, items []cartItem, how fulfilment, shipCents int64, idempotency string) (checkout, map[string]freshItem, error) {
	// Stock and price were read when the shop started, which could have been
	// days ago. Square charges the current catalog price and will sell past
	// zero, so both are re-checked here: this is the last moment before someone
	// is asked for money, and the only one where being wrong costs them.
	fresh, err := c.recheck(ctx, items)
	if err != nil {
		return checkout{}, nil, err
	}

	lineItems, err := verifyCartItems(items, fresh)
	if err != nil {
		return checkout{}, fresh, err
	}

	order := map[string]any{
		"location_id": c.locationID,
		"line_items":  lineItems,
	}
	opts := map[string]any{}

	// Square's hosted page cannot offer the choice, so the order says which it
	// is and the page only collects what that needs.
	if how == fulfilShip {
		order["service_charges"] = []map[string]any{{
			"name":              "Shipping",
			"amount_money":      map[string]any{"amount": shipCents, "currency": "USD"},
			"calculation_phase": "SUBTOTAL_PHASE",
		}}
		order["fulfillments"] = []map[string]any{{
			"type":  "SHIPMENT",
			"state": "PROPOSED",
		}}
		opts["ask_for_shipping_address"] = true
	} else {
		order["fulfillments"] = []map[string]any{{
			"type":           "PICKUP",
			"state":          "PROPOSED",
			"pickup_details": map[string]any{"schedule_type": "ASAP", "note": "Collect at the shop"},
		}}
	}

	body := map[string]any{
		"idempotency_key":  idempotency,
		"order":            order,
		"checkout_options": opts,
	}

	var out struct {
		PaymentLink struct {
			ID      string `json:"id"`
			URL     string `json:"url"`
			OrderID string `json:"order_id"`
		} `json:"payment_link"`
	}
	if err := c.call(ctx, http.MethodPost, "/v2/online-checkout/payment-links", body, &out); err != nil {
		return checkout{}, fresh, err
	}
	if out.PaymentLink.URL == "" {
		return checkout{}, fresh, fmt.Errorf("square returned no checkout url")
	}
	return checkout{
		URL:     out.PaymentLink.URL,
		OrderID: out.PaymentLink.OrderID,
		LinkID:  out.PaymentLink.ID,
	}, fresh, nil
}

// paid reports whether the order has been settled. The buyer pays on a page
// this process never sees, so polling is the only way to know.
func (c *squareClient) paid(ctx context.Context, orderID string) (bool, error) {
	var out struct {
		Order struct {
			State   string `json:"state"`
			Tenders []struct {
				ID string `json:"id"`
			} `json:"tenders"`
		} `json:"order"`
	}
	// Escaped because the API hands this straight from a URL: an id with a
	// slash or a question mark in it must not be able to reach some other
	// Square endpoint with the shop's token.
	if err := c.call(ctx, http.MethodGet, "/v2/orders/"+url.PathEscape(orderID), nil, &out); err != nil {
		return false, err
	}
	return out.Order.State == "COMPLETED" || len(out.Order.Tenders) > 0, nil
}

// link reads one payment link back, which is how a checkout id becomes the
// order id needed to check whether it was paid before deleting it.
func (c *squareClient) link(ctx context.Context, id string) (paymentLink, error) {
	var out struct {
		PaymentLink paymentLink `json:"payment_link"`
	}
	err := c.call(ctx, http.MethodGet, "/v2/online-checkout/payment-links/"+url.PathEscape(id), nil, &out)
	return out.PaymentLink, err
}
