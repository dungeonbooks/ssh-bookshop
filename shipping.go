package main

// USPS Media Mail, the rate books ship at: priced by weight alone, no distance
// and no dimensional weight, rounded up to the whole pound. Retail rates, which
// run above what Pirate Ship charges, so the difference covers packaging.
const (
	mediaMailFirstCents int64 = 413
	mediaMailAddlCents  int64 = 71
	packagingGrams            = 142 // mailer and padding, about 5oz
	gramsPerPound             = 454

	// Charged when any book in the cart has no recorded weight. Guessing a
	// weight means guessing a price, so fall back rather than invent one. Set
	// near the whole-shelf rate: a fallback should overcharge slightly, not
	// absurdly.
	flatShippingCents int64 = 800
)

// mediaMail returns what we charge to ship a parcel of the given weight: the
// USPS Media Mail rate rounded up to the whole dollar. USPS bills by the whole
// pound, so weight rounds up too.
//
// Whole dollars keep the shop free of decimals it does not need, the way
// terminal.shop prices everything. Rounding up rather than to nearest means the
// charge never lands under the postage.
func mediaMail(grams int) int64 {
	pounds := int64((grams + gramsPerPound - 1) / gramsPerPound)
	if pounds < 1 {
		pounds = 1
	}
	postage := mediaMailFirstCents + mediaMailAddlCents*(pounds-1)
	return wholeDollarsUp(postage)
}

// wholeDollarsUp rounds a charge up to the next whole dollar.
func wholeDollarsUp(cents int64) int64 {
	if r := cents % 100; r != 0 {
		cents += 100 - r
	}
	return cents
}

// shippingFor prices a cart. exact is false when a book is missing its weight,
// in which case the flat rate applies and the shelf needs its Ingram data.
func shippingFor(lines []cartLine, weight func(idx int) int) (cents int64, exact bool) {
	total := packagingGrams
	for _, l := range lines {
		g := weight(l.idx)
		if g <= 0 {
			return flatShippingCents, false
		}
		total += g * l.qty
	}
	return mediaMail(total), true
}
