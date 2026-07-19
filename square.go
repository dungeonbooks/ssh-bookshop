package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Square is the source of truth for price and for taking money. Books are
// looked up by ISBN, which every variation carries as its UPC.
//
// Checkout hands off to a hosted Square page rather than taking card details
// here. That is not a shortcut: CreatePayment only accepts a token from the Web
// Payments or In-App Payments SDK, so there is no server-side path for a card
// number, and typing one into an SSH session would put this process in PCI
// scope. The buyer gets a square.link URL; Square handles card, address, tax.
const (
	squareVersion = "2025-01-23"
	squareTimeout = 10 * time.Second
	// Boot blocks on this, so it is a fixed ceiling rather than one that grows
	// with the shelf. Lookups run concurrently, capped so a bigger shelf costs
	// round trips rather than a longer outage.
	bootTimeout     = 20 * time.Second
	bootConcurrency = 4
)

type squareClient struct {
	token      string
	locationID string
	// baseURL is the API root, including scheme. Overridden in tests to point
	// at an httptest server; empty means the real Square host.
	baseURL string
}

var sq *squareClient

type squareMoney struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

func squareHost() string {
	if os.Getenv("SQUARE_ENVIRONMENT") == "sandbox" {
		return "connect.squareupsandbox.com"
	}
	return "connect.squareup.com"
}

func (c *squareClient) root() string {
	if c.baseURL != "" {
		// Paths all start with /, so a trailing slash here would double it.
		return strings.TrimRight(c.baseURL, "/")
	}
	return "https://" + squareHost()
}

// call is every Square request: auth, version, JSON in and out. Square reports
// failures in a 200-shaped body as often as by status code, so both are checked.
func (c *squareClient) call(ctx context.Context, method, path string, body, out any) error {
	var rdr *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.root()+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Square-Version", squareVersion)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var probe struct {
		Errors []struct {
			Code   string `json:"code"`
			Detail string `json:"detail"`
		} `json:"errors"`
	}
	raw := &bytes.Buffer{}
	if _, err := raw.ReadFrom(resp.Body); err != nil {
		return err
	}
	if err := json.Unmarshal(raw.Bytes(), &probe); err == nil && len(probe.Errors) > 0 {
		return fmt.Errorf("square: %s %s", probe.Errors[0].Code, probe.Errors[0].Detail)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("square: HTTP %d", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw.Bytes(), out)
}

// --- catalog ---------------------------------------------------------------

type squareVariation struct {
	ID                string `json:"id"`
	ItemVariationData struct {
		UPC        string       `json:"upc"`
		PriceMoney *squareMoney `json:"price_money"`
	} `json:"item_variation_data"`
}

// lookup finds the variation whose UPC is exactly this ISBN. The Catalog API
// has no exact-match query for UPC, so this searches by text and then confirms
// the UPC: a text hit alone can be a loosely related item, and selling the
// wrong book at the wrong price is worse than not offering it.
func (c *squareClient) lookup(ctx context.Context, isbn string) (variationID string, cents int64, err error) {
	var out struct {
		Items []struct {
			ItemData struct {
				Variations []squareVariation `json:"variations"`
			} `json:"item_data"`
		} `json:"items"`
	}
	body := map[string]any{"text_filter": isbn, "limit": 4}
	if err := c.call(ctx, http.MethodPost, "/v2/catalog/search-catalog-items", body, &out); err != nil {
		return "", 0, err
	}
	for _, it := range out.Items {
		for _, v := range it.ItemData.Variations {
			if v.ItemVariationData.UPC == isbn && v.ItemVariationData.PriceMoney != nil {
				return v.ID, v.ItemVariationData.PriceMoney.Amount, nil
			}
		}
	}
	return "", 0, nil
}

// stock reports on-hand counts for the given variations. A variation with no
// inventory record is reported as untracked (true), because we can't prove such
// an item is out and refusing to sell it would be worse than the alternative.
//
// Counts can be negative when a shop oversells, so "in stock" means > 0.
func (c *squareClient) stock(ctx context.Context, ids []string) (qty map[string]int, untracked map[string]bool, err error) {
	qty, untracked = map[string]int{}, map[string]bool{}
	for _, id := range ids {
		untracked[id] = true
	}
	if len(ids) == 0 {
		return qty, untracked, nil
	}

	var out struct {
		Counts []struct {
			CatalogObjectID string `json:"catalog_object_id"`
			State           string `json:"state"`
			Quantity        string `json:"quantity"`
		} `json:"counts"`
	}
	body := map[string]any{"catalog_object_ids": ids, "location_ids": []string{c.locationID}}
	if err := c.call(ctx, http.MethodPost, "/v2/inventory/counts/batch-retrieve", body, &out); err != nil {
		return qty, untracked, err
	}
	for _, ct := range out.Counts {
		if ct.State != "IN_STOCK" {
			continue
		}
		// A count we can't read must not fall through to "untracked", which
		// means sellable: an unparsable quantity would then let us take money
		// for a book we may not have. Treat it as tracked and empty instead.
		n, err := parseQuantity(ct.Quantity)
		untracked[ct.CatalogObjectID] = false
		if err != nil {
			qty[ct.CatalogObjectID] = 0
			continue
		}
		qty[ct.CatalogObjectID] = n
	}
	return qty, untracked, nil
}

// parseQuantity reads Square's stringly-typed counts. They are decimals, so
// "3", "3.0" and "3.00" all mean three, and fractions round down: half a book
// is not a book we can sell.
func parseQuantity(q string) (int, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return 0, fmt.Errorf("empty quantity")
	}
	f, err := strconv.ParseFloat(q, 64)
	if err != nil {
		return 0, fmt.Errorf("unreadable quantity %q: %w", q, err)
	}
	return int(math.Floor(f)), nil
}

// activeLocation picks the location that can take card payments. Overridable,
// because an account with several could otherwise sell from the wrong one.
func (c *squareClient) activeLocation(ctx context.Context) (string, error) {
	if id := os.Getenv("SQUARE_LOCATION_ID"); id != "" {
		return id, nil
	}
	var out struct {
		Locations []struct {
			ID           string   `json:"id"`
			Status       string   `json:"status"`
			Capabilities []string `json:"capabilities"`
		} `json:"locations"`
	}
	if err := c.call(ctx, http.MethodGet, "/v2/locations", nil, &out); err != nil {
		return "", err
	}
	for _, l := range out.Locations {
		if l.Status != "ACTIVE" {
			continue
		}
		for _, cap := range l.Capabilities {
			if cap == "CREDIT_CARD_PROCESSING" {
				return l.ID, nil
			}
		}
	}
	return "", fmt.Errorf("no active location with card processing")
}

// loadShop resolves the location and fills in prices. Errors are reported but
// not fatal: without Square the shelf still browses, it just can't sell.
func loadShop(books []Book) (priced int, err error) {
	token := os.Getenv("SQUARE_ACCESS_TOKEN")
	if token == "" {
		return 0, fmt.Errorf("SQUARE_ACCESS_TOKEN not set")
	}
	c := &squareClient{token: token}
	ctx, cancel := context.WithTimeout(context.Background(), bootTimeout)
	defer cancel()

	// sq is set whenever the client is usable, which is not the same as the load
	// having gone perfectly. Callers should check sq rather than treat any error
	// as fatal: a partly priced shelf still browses and still sells what priced.
	priced, err = c.loadInto(ctx, books)
	if err == nil || priced > 0 {
		sq = c
	}
	return priced, err
}

// loadInto prices and stocks books from Square. Split from loadShop so tests
// can supply a client pointed somewhere other than the real API.
func (c *squareClient) loadInto(ctx context.Context, books []Book) (priced int, err error) {
	loc, err := c.activeLocation(ctx)
	if err != nil {
		return 0, err
	}
	c.locationID = loc

	// One lookup per book, concurrently: sequentially this was the whole of the
	// boot delay, and the shop cannot accept a connection until it returns.
	type found struct {
		id    string
		cents int64
		err   error
	}
	results := make([]found, len(books))
	sem := make(chan struct{}, bootConcurrency)
	var wg sync.WaitGroup
	for i := range books {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Wait for a slot or for the boot deadline, whichever comes first,
			// so a cancelled context is not queued behind the whole shelf.
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[i] = found{err: ctx.Err()}
				return
			}
			id, cents, err := c.lookup(ctx, books[i].ISBN)
			results[i] = found{id, cents, err} // own index, so no lock
		}(i)
	}
	wg.Wait()

	var ids []string
	for i := range books {
		if results[i].err != nil {
			// Report the first failure but keep what did come back: a partly
			// priced shelf still browses.
			if err == nil {
				err = results[i].err
			}
			continue
		}
		if results[i].cents > 0 {
			books[i].VariationID, books[i].Cents = results[i].id, results[i].cents
			ids = append(ids, results[i].id)
			priced++
		}
	}
	// Being in the catalog is not the same as being on the shelf. A book Square
	// knows about but has none of is sold out, and selling it would mean taking
	// money for something we can't hand over.
	//
	// Runs even when a lookup failed: skipping it would leave the books that did
	// price with Sellable false, so the shelf would show prices and refuse to
	// sell any of them, which is worse than either outcome on its own.
	qty, untracked, stockErr := c.stock(ctx, ids)
	if stockErr != nil {
		if err == nil {
			err = stockErr
		}
		return priced, err
	}
	for i := range books {
		id := books[i].VariationID
		if id == "" {
			continue
		}
		books[i].Stock, books[i].Tracked = qty[id], !untracked[id]
		books[i].Sellable = sellable(untracked[id], qty[id])
	}
	return priced, err
}

// --- checkout --------------------------------------------------------------

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

func verifyCartItems(items []cartItem, fresh map[string]freshItem) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		f, ok := fresh[it.variationID]
		switch {
		case !ok || !f.sellable:
			return nil, fmt.Errorf("%s just sold out", it.title)
		case !f.untracked && f.stock < it.qty:
			return nil, fmt.Errorf("only %d left of %s", f.stock, it.title)
		case f.cents != it.cents:
			// Better to send them back to a corrected shelf than to quote one
			// price and charge another. The fresh figures go back with the
			// error so the shelf updates and a retry isn't doomed to repeat.
			return nil, fmt.Errorf("%s is now %s, not %s", it.title, usd(f.cents), usd(it.cents))
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
			continue // gone from the catalog entirely; caller reports sold out
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
	if err := c.call(ctx, http.MethodGet, "/v2/orders/"+orderID, nil, &out); err != nil {
		return false, err
	}
	return out.Order.State == "COMPLETED" || len(out.Order.Tenders) > 0, nil
}

// usd prints money the way terminal.shop does, without decimals it does not
// need. Book prices come from Square and mostly carry cents, so those still
// show them; shipping and a whole-dollar total do not.
func usd(cents int64) string {
	if cents%100 == 0 {
		return fmt.Sprintf("$%d", cents/100)
	}
	return fmt.Sprintf("$%d.%02d", cents/100, cents%100)
}
