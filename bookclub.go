package main

// catalog is the monthly Sci-Fi & Fantasy book club shelf, newest first, and
// the whole shop: ssh in, see what we're reading. ISBN is the edition we stock,
// so a cover lookup matches what is on the shelf.
//
// URL is where a reader should buy it: dungeonbooks.com while we have it, a
// bookshop.org affiliate link once we sell out.
var catalog = []Book{
	{ISBN: "9780316568654", BookTitle: "The Last Contract of Isako", Author: "Fonda Lee", Collection: collBookClub, Month: "2026-08",
		Format: "paperback", Pages: 528, WeightGrams: 576,
		Blurb: "Isako is a legendary swordswoman planning to walk out into the ice and die well. The last contract she takes puts her opposite Martim, the worst apprentice she ever trained, who has climbed to the top of a company that can buy a life or end one.",
		URL:   "https://www.dungeonbooks.com/product/the-last-contract-of-isako-by-fonda-lee-paperback-/GB5UWQBUBNLLCSREFBVH3636"},
	{ISBN: "9780593818947", BookTitle: "Daughter of Crows", Author: "Mark Lawrence", Collection: collBookClub, Month: "2026-07",
		Format: "hardcover", Pages: 410, WeightGrams: 553,
		Blurb: "The Academy of Kindness takes in a hundred girls a year and graduates three. One survivor has to exhume her own past. First in a new series from the author of the Broken Empire.",
		URL:   "https://www.dungeonbooks.com/product/daughter-of-crows-book-1-of-the-academy-of-kindness-by-mark-lawrence-hardcover-/UEY72ZWABXCFBC7HZYKSMDFZ"},
	{ISBN: "9781250376794", BookTitle: "Sublimation", Author: "Isabel J. Kim", Collection: collBookClub, Month: "2026-06",
		Format: "hardcover, signed", Pages: 368, WeightGrams: 567,
		Blurb: "When you emigrate, a copy of you stays behind. Rose left Korea at ten and never spoke to hers again, until a funeral calls her home and that other self decides to take her life back.",
		URL:   "https://www.dungeonbooks.com/product/sublimation-by-isabel-j-kim-hardcover-signed/3HRN5W3LAQUUIILNTU77D5QU"},
	{ISBN: "9781967967063", BookTitle: "Burn the Sea", Author: "Mona Tewari", Collection: collBookClub, Month: "2026-05",
		// Page count off the physical copy: neither Hardcover nor Ingram has one.
		Format: "paperback", Pages: 450, WeightGrams: 567,
		Blurb: "Abbakka trained to be her sister's blade, not to rule. When the Porcugi come back out of the sea demanding tribute, she has to learn statecraft and subterfuge in a hurry.",
		URL:   "https://www.dungeonbooks.com/product/burn-the-sea-by-mona-tewari-paperback-/6BLH3723VWADJVHV3XMDOEPG"},
	{ISBN: "9798991475273", BookTitle: "Blessed is the Rot", Author: "Sheri Singerling", Collection: collBookClub, Month: "2026-04",
		Format: "paperback", Pages: 303, WeightGrams: 390,
		Blurb: "Ashtin contained distortions for the Church until he defied it, lost his name, and became Fenrir, a bell ringer. When a distortion swallows his tower, the Church sends a surveyor to contain it.",
		URL:   "https://www.dungeonbooks.com/product/blessed-is-the-rot-by-sheri-singerling-paperback-/5XCYQAWHIVKO4CA3PFLFNQIA"},
	// The deluxe is the edition we stocked, so it is what Square prices and what
	// Bookshop is asked for. (taranat.com shows the paperback's cover instead,
	// because the deluxe's art is a 3/4 render; no covers here, so no conflict.)
	{ISBN: "9781250406811",
		BookTitle: "The Poet Empress", Author: "Shen Tao", Collection: collBookClub, Month: "2026-03",
		Format: "deluxe edition", Pages: 432, WeightGrams: 748,
		Blurb: "Wei Yin offers herself as concubine to a cruel prince to keep her family alive, and lands in a palace on the edge of civil war. To survive she becomes a poet, in a world where women are forbidden to read.",
	},
	{ISBN: "9781984820716", BookTitle: "The Tainted Cup", Author: "Robert Jackson Bennett", Collection: collBookClub, Month: "2026-02",
		Format: "paperback", Pages: 406, WeightGrams: 318,
		Blurb: "An impossible death on the Empire's frontier, where leviathan blood warps everything it touches. The detective Ana Dolabra solves it from inside her house, blindfolded, through her altered assistant Din.",
		URL:   "https://www.dungeonbooks.com/product/the-tainted-cup-book-1-of-3-ana-and-din-mysteries-by-robert-jackson-bennett-paperback-/554"},
	{ISBN: "9781250380968", BookTitle: "Saltcrop", Author: "Yume Kitasei", Collection: collBookClub, Month: "2026-01",
		Format: "hardcover", Pages: 376, WeightGrams: 581,
		Blurb: "The seas have swallowed the coastal cities and crops are failing worldwide. Two sisters sail out to find the third, who left a decade ago chasing a cure.",
		URL:   "https://www.dungeonbooks.com/product/saltcrop-by-yume-kitasei-hardcover-/SHDDSGUBEE7B2NOCEABTYGVK"},
}

// featured is the index of the pick the shop opens on: the newest one on the
// shelf, which is the top entry. Announcing a pick is what makes it current —
// we do that a little before its month starts — so adding it here features it,
// rather than it waiting on the calendar to roll over.
//
// This deliberately does not consult the date. Matching Month against today
// left a pick announced early unfeatured until the 1st, which is the window we
// most want it visible in. The cost is that a shelf nobody has updated keeps
// featuring its last pick instead of falling back to nothing, which beats a
// shop that opens on no book at all.
//
// -1 only when there is no shelf.
func featured() int {
	return featuredIn(catalog)
}

// featuredIn is featured() over any shelf, so the API can answer it for the
// shelf it was given rather than the package one.
func featuredIn(books []Book) int {
	if len(books) == 0 {
		return -1
	}
	return 0
}

// section is the list heading a book sits under. The featured pick is lifted
// out into its own "featured" section, the way terminal.shop separates its
// featured product from the rest of the shelf.
func section(i int) string {
	if i == featured() {
		return collFeatured
	}
	return catalog[i].Collection
}
