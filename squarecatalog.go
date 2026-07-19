package main

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
)

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

	priced, err = c.loadInto(ctx, books)
	if usable(priced, err) {
		sq = c
	}
	return priced, err
}

// usable reports whether a load left a client worth keeping, which is not the
// same as the load having gone perfectly. A clean run qualifies, and so does a
// partial one that priced something: that shelf still browses and still sells
// what it priced. Only a load that priced nothing leaves sq nil, so callers
// check sq rather than treating any error as fatal.
func usable(priced int, err error) bool {
	return err == nil || priced > 0
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
