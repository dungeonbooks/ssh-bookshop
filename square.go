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

// Square is the source of truth for what a book costs in the shop. We look up
// by ISBN because every variation carries it as its UPC.
//
// There is no exact-match query for UPC in the Catalog API, so this uses the
// text search and then confirms the UPC itself. A text hit alone is not enough:
// searching an ISBN can return a loosely related item, and quoting the wrong
// price is worse than quoting none.
const (
	squareVersion = "2025-01-23"
	squareTimeout = 8 * time.Second
)

type squareMoney struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

type squareVariation struct {
	ID                string `json:"id"`
	ItemVariationData struct {
		UPC        string       `json:"upc"`
		PriceMoney *squareMoney `json:"price_money"`
	} `json:"item_variation_data"`
}

type squareSearchResp struct {
	Items []struct {
		ItemData struct {
			Name       string            `json:"name"`
			Variations []squareVariation `json:"variations"`
		} `json:"item_data"`
	} `json:"items"`
	Errors []struct {
		Code   string `json:"code"`
		Detail string `json:"detail"`
	} `json:"errors"`
}

func squareHost() string {
	if os.Getenv("SQUARE_ENVIRONMENT") == "sandbox" {
		return "connect.squareupsandbox.com"
	}
	return "connect.squareup.com"
}

// priceFor returns the price in cents for an ISBN, or 0 if the shop has no
// matching item. A miss is normal: it means we don't carry that edition.
func priceFor(ctx context.Context, token, isbn string) (int64, error) {
	body, _ := json.Marshal(map[string]any{"text_filter": isbn, "limit": 4})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://"+squareHost()+"/v2/catalog/search-catalog-items", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Square-Version", squareVersion)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var out squareSearchResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, fmt.Errorf("square: %w", err)
	}
	if len(out.Errors) > 0 {
		return 0, fmt.Errorf("square: %s %s", out.Errors[0].Code, out.Errors[0].Detail)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("square: HTTP %d", resp.StatusCode)
	}

	for _, it := range out.Items {
		for _, v := range it.ItemData.Variations {
			// The UPC check is what makes this a lookup rather than a guess.
			if v.ItemVariationData.UPC == isbn && v.ItemVariationData.PriceMoney != nil {
				return v.ItemVariationData.PriceMoney.Amount, nil
			}
		}
	}
	return 0, nil
}

// loadPrices fills in every book's price from Square. Failures are per-book and
// non-fatal: a book with no price simply shows none, and the shop still opens.
func loadPrices(books []Book) (found int, err error) {
	token := os.Getenv("SQUARE_ACCESS_TOKEN")
	if token == "" {
		return 0, fmt.Errorf("SQUARE_ACCESS_TOKEN not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), squareTimeout*time.Duration(len(books)))
	defer cancel()

	for i := range books {
		cents, err := priceFor(ctx, token, books[i].ISBN)
		if err != nil {
			return found, err
		}
		if cents > 0 {
			books[i].Cents = cents
			found++
		}
	}
	return found, nil
}

func usd(cents int64) string {
	return fmt.Sprintf("$%d.%02d", cents/100, cents%100)
}
