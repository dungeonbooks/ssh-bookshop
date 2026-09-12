// Package shopapi is the wire format of the shop's HTTP API, shared by the
// server and the dungeon CLI so there is one definition of every shape.
//
// The API exists so an agent can do everything a shopper does up to the
// moment money changes hands: read the shelf, price an order, choose pickup or
// shipping, and get the hosted Square checkout URL to hand to a human. Payment
// itself stays on Square's page, so nothing here carries a card or an address.
package shopapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Fulfilment is how the order reaches the buyer. Square's hosted page cannot
// offer the choice, so it has to be made before the order exists.
const (
	FulfilPickup = "pickup"
	FulfilShip   = "ship"
)

// Order states. There are two because the shop only ever asks Square one
// question about an order: has it been paid.
const (
	StateAwaitingPayment = "awaiting_payment"
	StatePaid            = "paid"
)

// Book is one shelf entry. Stock is only present when Square keeps a count for
// the book; BuyURL is only present for a book the shop does not carry, so a
// reader never sees two ways to buy the same book.
type Book struct {
	ISBN       string `json:"isbn"`
	Title      string `json:"title"`
	Author     string `json:"author"`
	Collection string `json:"collection"`
	Month      string `json:"month,omitempty"`
	Featured   bool   `json:"featured"`
	Format     string `json:"format,omitempty"`
	Pages      int    `json:"pages,omitempty"`
	Blurb      string `json:"blurb"`
	PriceCents int64  `json:"price_cents"`
	Sellable   bool   `json:"sellable"`
	Tracked    bool   `json:"tracked"`
	Stock      *int   `json:"stock,omitempty"`
	BuyURL     string `json:"buy_url,omitempty"`
}

// Shelf is the whole shop.
type Shelf struct {
	Books      []Book    `json:"books"`
	Shipping   Shipping  `json:"shipping"`
	PricesAsOf time.Time `json:"prices_as_of"`
}

// Shipping is the one-line rule, for agents that want to warn a buyer before
// they ask for a checkout.
type Shipping struct {
	Region string `json:"region"`
	Note   string `json:"note"`
}

// Item is one line of a checkout request.
type Item struct {
	ISBN string `json:"isbn"`
	Qty  int    `json:"qty"`
}

// CheckoutRequest builds an order. IdempotencyKey is optional: when given, a
// retry of the same request returns the same link rather than a second live
// one. When absent the server mints one per request.
type CheckoutRequest struct {
	Items          []Item `json:"items"`
	Fulfilment     string `json:"fulfilment"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

// Checkout is a hosted Square checkout, ready for a human. Totals are before
// tax: Square collects the address and works out tax on its own page.
type Checkout struct {
	OrderID       string `json:"order_id"`
	CheckoutID    string `json:"checkout_id"`
	CheckoutURL   string `json:"checkout_url"`
	Fulfilment    string `json:"fulfilment"`
	SubtotalCents int64  `json:"subtotal_cents"`
	ShippingCents int64  `json:"shipping_cents"`
	ShippingExact bool   `json:"shipping_exact"`
	TotalCents    int64  `json:"total_cents"`
	Note          string `json:"note"`
}

// Order is what polling returns.
type Order struct {
	OrderID string `json:"order_id"`
	State   string `json:"state"`
}

// Error is the body of every non-2xx response. BuyURL is set when the book is
// on the shelf but not for sale here, so the agent can still send someone
// somewhere. PriceCents is set when the price moved, so a retry can use it.
type Error struct {
	Status     int    `json:"-"`
	Message    string `json:"error"`
	BuyURL     string `json:"buy_url,omitempty"`
	PriceCents int64  `json:"price_cents,omitempty"`
}

func (e *Error) Error() string { return e.Message }

// Client talks to the API. The zero value is not usable; set BaseURL.
type Client struct {
	BaseURL   string
	UserAgent string
	HTTP      *http.Client
}

// DefaultBaseURL is where the shop's API lives.
const DefaultBaseURL = "https://api.dungeonbooks.com"

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// do sends one request and decodes the response. An HTTP error status comes
// back as *Error so callers can read the message and the extra fields.
func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		e := &Error{Status: resp.StatusCode}
		if json.Unmarshal(raw, e) != nil || e.Message == "" {
			e.Message = fmt.Sprintf("HTTP %d from %s", resp.StatusCode, c.BaseURL)
		}
		return e
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// Books returns the shelf.
func (c *Client) Books(ctx context.Context) (Shelf, error) {
	var s Shelf
	err := c.do(ctx, http.MethodGet, "/v1/books", nil, &s)
	return s, err
}

// Book returns one shelf entry.
func (c *Client) Book(ctx context.Context, isbn string) (Book, error) {
	var b Book
	err := c.do(ctx, http.MethodGet, "/v1/books/"+url.PathEscape(isbn), nil, &b)
	return b, err
}

// Checkout builds an order on Square and returns the hosted checkout.
func (c *Client) Checkout(ctx context.Context, req CheckoutRequest) (Checkout, error) {
	var out Checkout
	err := c.do(ctx, http.MethodPost, "/v1/checkout", req, &out)
	return out, err
}

// Order reports whether an order has been paid.
func (c *Client) Order(ctx context.Context, orderID string) (Order, error) {
	var o Order
	err := c.do(ctx, http.MethodGet, "/v1/orders/"+url.PathEscape(orderID), nil, &o)
	return o, err
}

// Cancel abandons a checkout that has not been paid.
func (c *Client) Cancel(ctx context.Context, checkoutID string) error {
	return c.do(ctx, http.MethodDelete, "/v1/checkout/"+url.PathEscape(checkoutID), nil, nil)
}

// IsStatus reports whether err is an API error with the given status.
func IsStatus(err error, status int) bool {
	var e *Error
	return errors.As(err, &e) && e.Status == status
}
