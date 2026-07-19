package main

import (
	"fmt"
	"strings"
)

// AffiliateID is dungeonbooks' Bookshop.org affiliate identifier (matches
// marty's BOOKSHOP_AFFILIATE_ID). Affiliate links pay a commission; every ISBN
// below was validated against bookshop.org/book/{isbn} (308 = live).
const AffiliateID = "108216"

func affiliate(isbn string) string {
	return "https://bookshop.org/a/" + AffiliateID + "/" + isbn
}

// Book is one catalog entry. Affiliate model: no inventory, no payment — "buy"
// hands off to Bookshop.org, except books we stock ourselves, which carry their
// own dungeonbooks.com URL. Free titles are openly licensed and read directly.
type Book struct {
	ISBN        string
	BookTitle   string
	Author      string
	Year        int
	Publisher   string
	Collection  string
	Blurb       string
	Free        bool
	DownloadURL string
	URL         string // overrides the affiliate link when we sell it ourselves
	Month       string // book club pick month, "2026-07"
	Format      string // "hardcover", "paperback" — the edition we stock
	Pages       int    // 0 when unknown
	Cents       int64  // price from Square, 0 when we don't carry it
	VariationID string // Square catalog variation, needed to build an order
	Stock       int    // on-hand at the shop; can go negative when oversold
	Tracked     bool   // Square keeps a count for this book, so Stock means something
	Sellable    bool   // in the catalog and either in stock or not inventoried
	ListCents   int64  // publisher list price, for books we don't stock
}

// lowStock is where a count stops being reassuring and starts being useful.
const lowStock = 3

// stockNote warns when the shelf is nearly empty. Only for books Square keeps a
// count for: an untracked book reads as zero, and "only 0 left" beside an add
// button would be nonsense. Silent above the threshold, because a count that
// appears on everything is just decoration, and manufacturing urgency out of a
// number nobody checked is how shops end up lying.
func (b Book) stockNote() string {
	if !b.Tracked || !b.Sellable || b.Stock > lowStock {
		return ""
	}
	return fmt.Sprintf("   only %d left", b.Stock)
}

// Price is what to show. Square is authoritative for anything on our shelf;
// otherwise fall back to list price, which is what Bookshop.org charges.
func (b Book) Price() int64 {
	if b.Cents > 0 {
		return b.Cents
	}
	return b.ListCents
}

// attrs is the pipe-joined line under the title: author, then whatever else we
// actually know. Anything missing is left out rather than shown empty.
func (b Book) attrs() []string {
	out := []string{b.Author}
	if b.Format != "" {
		out = append(out, b.Format)
	}
	if b.Pages > 0 {
		out = append(out, fmt.Sprintf("%d pages", b.Pages))
	}
	return out
}

// BuyURL is where to send someone who wants this book.
//
// While we have it, our own store: a sale beats an affiliate commission. Once
// we don't, our product page is a dead end, so it falls back to Bookshop. A
// hand-picked Bookshop link wins there, because it can point at the edition
// they actually stock when ours is keyed to a different one.
func (b Book) BuyURL() string {
	if !b.Sellable && !strings.Contains(b.URL, "bookshop.org") {
		return affiliate(b.ISBN)
	}
	if b.URL != "" {
		return b.URL
	}
	return affiliate(b.ISBN)
}

// BuyLabel names the destination so the link is never a surprise.
func (b Book) BuyLabel() string {
	if strings.Contains(b.BuyURL(), "dungeonbooks.com") {
		return "buy at dungeonbooks.com:"
	}
	return "buy on bookshop.org:"
}

const (
	collBookClub = "book club"
	collFeatured = "featured"
)
