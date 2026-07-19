package main

import "strings"

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
}

// BuyURL prefers our own store: a sale beats an affiliate commission.
func (b Book) BuyURL() string {
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
