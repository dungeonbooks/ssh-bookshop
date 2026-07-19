package main

import (
	"context"
	"fmt"
	"os"
)

// checkShelf prints what Square says about every book, plus any extra ISBNs
// given on the command line. It only reads, so it can be pointed at production
// to answer "would the shop try to sell something it doesn't have?".
func checkShelf(extra []string) {
	priced, err := loadShop(catalog)
	if err != nil {
		fmt.Fprintln(os.Stderr, "square:", err)
		return
	}
	fmt.Printf("env=%s location=%s priced=%d/%d\n\n",
		env("SQUARE_ENVIRONMENT", "production"), sq.locationID, priced, len(catalog))

	fmt.Printf("%-15s %-28s %8s %7s %9s  %s\n", "ISBN", "TITLE", "PRICE", "STOCK", "SELLABLE", "SHOWS AS")
	for _, b := range catalog {
		shows := "sold out"
		if b.Sellable {
			shows = "add to cart"
		}
		price, stock := "-", "-"
		if b.Cents > 0 {
			price = usd(b.Cents)
		}
		if b.VariationID != "" {
			stock = fmt.Sprint(b.Stock)
		}
		fmt.Printf("%-15s %-28s %8s %7s %9v  %s\n",
			b.ISBN, truncate(b.BookTitle, 28), price, stock, b.Sellable, shows)
	}

	for _, isbn := range extra {
		ctx, cancel := context.WithTimeout(context.Background(), squareTimeout)
		defer cancel()
		id, cents, err := sq.lookup(ctx, isbn)
		if err != nil {
			fmt.Printf("\n%s: %v\n", isbn, err)
			continue
		}
		if id == "" {
			fmt.Printf("\n%s: not in the catalog\n", isbn)
			continue
		}
		qty, untracked, err := sq.stock(ctx, []string{id})
		if err != nil {
			fmt.Printf("\n%s: %v\n", isbn, err)
			continue
		}
		fmt.Printf("\n%s: %s stock=%d untracked=%v -> sellable=%v\n",
			isbn, usd(cents), qty[id], untracked[id], untracked[id] || qty[id] > 0)
	}
}
