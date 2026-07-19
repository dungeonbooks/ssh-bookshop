package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
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
)

type squareClient struct {
	token      string
	locationID string
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

	req, err := http.NewRequestWithContext(ctx, method, "https://"+squareHost()+path, rdr)
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
	ctx, cancel := context.WithTimeout(context.Background(), squareTimeout*time.Duration(len(books)+1))
	defer cancel()

	loc, err := c.activeLocation(ctx)
	if err != nil {
		return 0, err
	}
	c.locationID = loc

	for i := range books {
		id, cents, err := c.lookup(ctx, books[i].ISBN)
		if err != nil {
			return priced, err
		}
		if cents > 0 {
			books[i].VariationID, books[i].Cents = id, cents
			priced++
		}
	}
	sq = c
	return priced, nil
}

// --- checkout --------------------------------------------------------------

type checkout struct {
	URL     string
	OrderID string
}

// createLink builds the buyer's cart on Square and returns a hosted checkout.
// Passing an order (rather than an ad hoc name and price) is what gets us
// itemisation, tax, and inventory for free.
func (c *squareClient) createLink(ctx context.Context, lines []cartLine, idempotency string) (checkout, error) {
	items := make([]map[string]any, 0, len(lines))
	for _, l := range lines {
		b := catalog[l.idx]
		if b.VariationID == "" {
			return checkout{}, fmt.Errorf("%s is not in the square catalog", b.BookTitle)
		}
		items = append(items, map[string]any{
			"catalog_object_id": b.VariationID,
			"quantity":          fmt.Sprint(l.qty),
		})
	}

	body := map[string]any{
		"idempotency_key": idempotency,
		"order": map[string]any{
			"location_id": c.locationID,
			"line_items":  items,
		},
		"checkout_options": map[string]any{
			"ask_for_shipping_address": true,
		},
	}

	var out struct {
		PaymentLink struct {
			URL     string `json:"url"`
			OrderID string `json:"order_id"`
		} `json:"payment_link"`
	}
	if err := c.call(ctx, http.MethodPost, "/v2/online-checkout/payment-links", body, &out); err != nil {
		return checkout{}, err
	}
	if out.PaymentLink.URL == "" {
		return checkout{}, fmt.Errorf("square returned no checkout url")
	}
	return checkout{URL: out.PaymentLink.URL, OrderID: out.PaymentLink.OrderID}, nil
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

func usd(cents int64) string {
	return fmt.Sprintf("$%d.%02d", cents/100, cents%100)
}
