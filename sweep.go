package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"charm.land/log/v2"
)

// Square payment links never expire, so every abandoned checkout leaves a live
// link and a DRAFT order behind for good. Deleting one cancels its order, which
// is why nothing recent is touched: a shopper still deciding would get an error
// on a page we already sent them to.
const (
	sweepAfter    = 24 * time.Hour
	sweepEvery    = 6 * time.Hour
	sweepPageSize = 100
)

type paymentLink struct {
	ID        string    `json:"id"`
	OrderID   string    `json:"order_id"`
	CreatedAt time.Time `json:"created_at"`
}

// sweepLinks deletes unpaid payment links older than sweepAfter. Paid ones are
// left alone: their order is a real sale, and the link is the buyer's receipt
// trail. Returns how many were deleted and how many are still live, so
// deleted+kept is every link walked: a link that could not be read or could not
// be deleted is still on the shelf and counts as kept, with the reason in err.
//
// A link that will not go is reported but does not stop the run. Returning on
// the first failure meant one undeletable link wedged the sweep at that point
// in the page, every six hours, for good: everything behind it stayed live
// however old it got.
func (c *squareClient) sweepLinks(ctx context.Context, now time.Time) (deleted, kept int, err error) {
	var failures []error
	cursor := ""
	for {
		q := url.Values{"limit": {strconv.Itoa(sweepPageSize)}}
		if cursor != "" {
			// Square cursors are base64-ish: an unescaped + arrives as a space.
			q.Set("cursor", cursor)
		}
		path := "/v2/online-checkout/payment-links?" + q.Encode()
		var page struct {
			PaymentLinks []paymentLink `json:"payment_links"`
			Cursor       string        `json:"cursor"`
		}
		if err := c.call(ctx, http.MethodGet, path, nil, &page); err != nil {
			// Without a page there is nothing to walk and no cursor to follow,
			// so this one really does end the run.
			return deleted, kept, errors.Join(append(failures, err)...)
		}

		for _, l := range page.PaymentLinks {
			// A missing or unparsable created_at leaves the zero time, which
			// would look older than anything and delete the link on sight.
			if l.CreatedAt.IsZero() || now.Sub(l.CreatedAt) < sweepAfter {
				kept++
				continue
			}
			// Never cancel an order someone paid for. An order we cannot read is
			// not permission to delete it either: leave it for the next run.
			if l.OrderID != "" {
				isPaid, err := c.paid(ctx, l.OrderID)
				if err != nil {
					// Counted as kept because it is still live. The pair
					// describes the shelf after the run rather than the reason
					// for each outcome, so deleted+kept stays every link
					// walked; why it survived is in failures.
					kept++
					failures = append(failures, fmt.Errorf("check order %s: %w", l.OrderID, err))
					continue
				}
				if isPaid {
					kept++
					continue
				}
			}
			if err := c.deleteLink(ctx, l); err != nil {
				kept++
				failures = append(failures, err)
				continue
			}
			deleted++
		}

		if page.Cursor == "" {
			return deleted, kept, errors.Join(failures...)
		}
		cursor = page.Cursor
	}
}

// deleteLink removes a payment link, cancelling its order first if Square will
// not take the delete on its own.
//
// A shipping checkout is created with a SHIPMENT fulfilment and no
// shipment_details, because the buyer fills the address in on Square's page.
// Square accepts that at creation and then rejects every later write to the
// order for want of the missing field, and deleting a link writes to its order,
// so the link cannot be deleted while the order is live. Cancelling the order
// first is what lets the delete through.
func (c *squareClient) deleteLink(ctx context.Context, l paymentLink) error {
	path := "/v2/online-checkout/payment-links/" + url.PathEscape(l.ID)
	err := c.call(ctx, http.MethodDelete, path, nil, nil)
	if err == nil || l.OrderID == "" {
		return err
	}
	if cancelErr := c.cancelOrder(ctx, l.OrderID); cancelErr != nil {
		// Joined rather than one wrapped and one formatted: either the delete
		// or the cancel can be the root cause, so both have to stay reachable
		// through errors.Is and errors.As.
		return fmt.Errorf("delete link %s (cancelling order %s): %w",
			l.ID, l.OrderID, errors.Join(err, cancelErr))
	}
	if err := c.call(ctx, http.MethodDelete, path, nil, nil); err != nil {
		return fmt.Errorf("delete link %s after cancelling order %s: %w", l.ID, l.OrderID, err)
	}
	return nil
}

// cancelOrder cancels an order so that its payment link can be deleted. Only
// ever called for a link the sweep has already established is old and unpaid.
//
// Square will not cancel an order while a fulfilment is still live, so each one
// is cancelled in the same write. A SHIPMENT fulfilment additionally needs the
// shipment_details it was never given, and a placeholder recipient satisfies
// that: the order is being cancelled, so nobody reads the name.
func (c *squareClient) cancelOrder(ctx context.Context, orderID string) error {
	var got struct {
		Order struct {
			Version      int `json:"version"`
			Fulfillments []struct {
				UID  string `json:"uid"`
				Type string `json:"type"`
			} `json:"fulfillments"`
		} `json:"order"`
	}
	path := "/v2/orders/" + url.PathEscape(orderID)
	if err := c.call(ctx, http.MethodGet, path, nil, &got); err != nil {
		return err
	}

	fulfilments := make([]map[string]any, 0, len(got.Order.Fulfillments))
	for _, f := range got.Order.Fulfillments {
		cancelled := map[string]any{"uid": f.UID, "type": f.Type, "state": "CANCELED"}
		if f.Type == "SHIPMENT" {
			cancelled["shipment_details"] = map[string]any{
				"recipient": map[string]any{"display_name": "cancelled"},
			}
		}
		fulfilments = append(fulfilments, cancelled)
	}

	return c.call(ctx, http.MethodPut, path, map[string]any{
		"idempotency_key": "sweep-cancel-" + orderID,
		"order": map[string]any{
			"version":      got.Order.Version,
			"state":        "CANCELED",
			"fulfillments": fulfilments,
		},
	}, nil)
}

// sweepShelf is the -sweep command: run the sweep once and report.
func sweepShelf() {
	if _, err := loadShop(catalog); err != nil {
		// A failed price lookup says nothing about whether links can be swept.
		fmt.Println("square:", err)
	}
	if sq == nil {
		fmt.Println("square: unavailable, nothing swept")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	deleted, kept, err := sq.sweepLinks(ctx, time.Now())
	fmt.Printf("deleted=%d kept=%d\n", deleted, kept)
	if err != nil {
		fmt.Println("left behind:", err)
	}
}

// sweepPeriodically runs the sweep in the background for the life of the
// process. Errors are logged and retried next tick rather than killing the shop:
// tidying up is never worth an outage.
func sweepPeriodically(ctx context.Context) {
	if sq == nil {
		log.Warn("no square client, not sweeping payment links")
		return
	}
	t := time.NewTicker(sweepEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c, cancel := context.WithTimeout(ctx, 5*time.Minute)
			deleted, kept, err := sq.sweepLinks(c, time.Now())
			cancel()
			if err != nil {
				// The sweep finished; these are the links it could not remove.
				log.Warn("some payment links could not be swept", "err", err, "deleted", deleted, "kept", kept)
				continue
			}
			if deleted > 0 {
				log.Info("swept abandoned payment links", "deleted", deleted, "kept", kept)
			}
		}
	}
}
