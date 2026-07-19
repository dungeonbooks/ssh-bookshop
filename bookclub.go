package main

import "time"

// catalog is the monthly Sci-Fi & Fantasy book club shelf, newest first, and
// the whole shop: ssh in, see what we're reading. ISBN is the edition we stock,
// so a cover lookup matches what is on the shelf.
//
// URL is where a reader should buy it: dungeonbooks.com while we have it, a
// bookshop.org affiliate link once we sell out.
var catalog = []Book{
	{ISBN: "9780593818947", BookTitle: "Daughter of Crows", Author: "Mark Lawrence", Collection: collBookClub, Month: "2026-07",
		URL: "https://www.dungeonbooks.com/product/daughter-of-crows-book-1-of-the-academy-of-kindness-by-mark-lawrence-hardcover-/UEY72ZWABXCFBC7HZYKSMDFZ"},
	{ISBN: "9781250376794", BookTitle: "Sublimation", Author: "Isabel J. Kim", Collection: collBookClub, Month: "2026-06",
		URL: "https://www.dungeonbooks.com/product/sublimation-by-isabel-j-kim-hardcover-signed/3HRN5W3LAQUUIILNTU77D5QU"},
	{ISBN: "9781967967063", BookTitle: "Burn the Sea", Author: "Mona Tewari", Collection: collBookClub, Month: "2026-05",
		URL: "https://www.dungeonbooks.com/product/burn-the-sea-by-mona-tewari-paperback-/6BLH3723VWADJVHV3XMDOEPG"},
	{ISBN: "9798991475273", BookTitle: "Blessed is the Rot", Author: "Sheri Singerling", Collection: collBookClub, Month: "2026-04",
		URL: "https://www.dungeonbooks.com/product/blessed-is-the-rot-by-sheri-singerling-paperback-/5XCYQAWHIVKO4CA3PFLFNQIA"},
	{ISBN: "9781250406828", BookTitle: "The Poet Empress", Author: "Shen Tao", Collection: collBookClub, Month: "2026-03",
		URL: "https://bookshop.org/a/108216/9781250406811"},
	{ISBN: "9781984820716", BookTitle: "The Tainted Cup", Author: "Robert Jackson Bennett", Collection: collBookClub, Month: "2026-02",
		URL: "https://www.dungeonbooks.com/product/the-tainted-cup-book-1-of-3-ana-and-din-mysteries-by-robert-jackson-bennett-paperback-/554"},
	{ISBN: "9781250380968", BookTitle: "Saltcrop", Author: "Yume Kitasei", Collection: collBookClub, Month: "2026-01",
		URL: "https://www.dungeonbooks.com/product/saltcrop-by-yume-kitasei-hardcover-/SHDDSGUBEE7B2NOCEABTYGVK"},
}

// monthLabel turns "2026-07" into "July 2026". Returns "" for anything that
// isn't that shape, so callers fall back rather than print a raw key.
func monthLabel(month string) string {
	t, err := time.Parse("2006-01", month)
	if err != nil {
		return ""
	}
	return t.Format("January 2006")
}

// blurb stands in for per-book copy we don't have yet. The month is the point
// of the shelf, so lead with it.
func blurb(month string) string {
	if m := monthLabel(month); m != "" {
		return "Our book club pick for " + m + ". We meet monthly at the shop in Jersey City to talk about it."
	}
	return "A Dungeon Books book club pick. We meet monthly at the shop in Jersey City."
}

func init() {
	for i := range catalog {
		if catalog[i].Blurb == "" {
			catalog[i].Blurb = blurb(catalog[i].Month)
		}
	}
}

// featured is the index of this month's pick, or -1 when the shelf has not
// caught up to the calendar yet. Resolved per call rather than at init so a
// long-running server rolls over at the month boundary on its own.
func featured(now time.Time) int {
	this := now.Format("2006-01")
	for i, b := range catalog {
		if b.Month == this {
			return i
		}
	}
	return -1
}

// section is the list heading a book sits under. This month's pick is lifted
// out into its own "featured" section, the way terminal.shop separates its
// featured product from the rest of the shelf.
func section(i int) string {
	if i == featured(time.Now()) {
		return collFeatured
	}
	return catalog[i].Collection
}
