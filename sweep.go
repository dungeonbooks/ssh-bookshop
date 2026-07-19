package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/charmbracelet/log"
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
// trail. Returns how many were deleted and how many were kept.
func (c *squareClient) sweepLinks(ctx context.Context, now time.Time) (deleted, kept int, err error) {
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
			return deleted, kept, err
		}

		for _, l := range page.PaymentLinks {
			// A missing or unparsable created_at leaves the zero time, which
			// would look older than anything and delete the link on sight.
			if l.CreatedAt.IsZero() || now.Sub(l.CreatedAt) < sweepAfter {
				kept++
				continue
			}
			// Never cancel an order someone paid for.
			if l.OrderID != "" {
				isPaid, err := c.paid(ctx, l.OrderID)
				if err != nil {
					return deleted, kept, fmt.Errorf("check order %s: %w", l.OrderID, err)
				}
				if isPaid {
					kept++
					continue
				}
			}
			if err := c.call(ctx, http.MethodDelete, "/v2/online-checkout/payment-links/"+url.PathEscape(l.ID), nil, nil); err != nil {
				return deleted, kept, fmt.Errorf("delete link %s: %w", l.ID, err)
			}
			deleted++
		}

		if page.Cursor == "" {
			return deleted, kept, nil
		}
		cursor = page.Cursor
	}
}

// sweepShelf is the -sweep command: run the sweep once and report.
func sweepShelf() {
	if _, err := loadShop(catalog); err != nil {
		fmt.Println("square:", err)
		return
	}
	if sq == nil {
		fmt.Println("square: no client")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	deleted, kept, err := sq.sweepLinks(ctx, time.Now())
	fmt.Printf("deleted=%d kept=%d\n", deleted, kept)
	if err != nil {
		fmt.Println("stopped early:", err)
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
				log.Warn("link sweep stopped early", "err", err, "deleted", deleted, "kept", kept)
				continue
			}
			if deleted > 0 {
				log.Info("swept abandoned payment links", "deleted", deleted, "kept", kept)
			}
		}
	}
}
