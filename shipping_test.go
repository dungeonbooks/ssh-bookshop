package main

import "testing"

// TestMediaMail pins the rate against USPS: $4.13 for the first pound, $0.71 for
// each additional, weight rounded up to the whole pound, and the charge rounded
// up to the whole dollar so the shop shows no decimals.
func TestMediaMail(t *testing.T) {
	for _, tc := range []struct {
		grams int
		want  int64
	}{
		// Postage rounded up to the dollar: 4.13 -> 5, 4.84 -> 5, 5.55 -> 6.
		{1, 500},     // a fraction of a pound still pays for one
		{454, 500},   // exactly a pound
		{455, 500},   // a gram over costs a whole band, 4.84 -> 5
		{908, 500},   // two pounds
		{909, 600},   // three, 5.55 -> 6
		{4540, 1100}, // ten pounds: 10.52 -> 11
	} {
		if got := mediaMail(tc.grams); got != tc.want {
			t.Errorf("mediaMail(%dg) = %d, want %d", tc.grams, got, tc.want)
		}
	}
}

func TestShippingForCart(t *testing.T) {
	// Real Ingram weights: Daughter of Crows and Sublimation.
	weights := map[int]int{0: 553, 1: 567, 2: 0} // book 2 has no Ingram data yet

	t.Run("one book", func(t *testing.T) {
		got, exact := shippingFor([]cartLine{{idx: 0, qty: 1}}, func(i int) int { return weights[i] })
		if !exact {
			t.Fatal("exact = false, want a computed rate")
		}
		if want := mediaMail(553 + packagingGrams); got != want {
			t.Errorf("got %d, want %d", got, want)
		}
	})

	t.Run("quantity counts", func(t *testing.T) {
		got, _ := shippingFor([]cartLine{{idx: 0, qty: 3}}, func(i int) int { return weights[i] })
		if want := mediaMail(553*3 + packagingGrams); got != want {
			t.Errorf("got %d, want %d: quantity is not being multiplied", got, want)
		}
	})

	// The case that decides whether we can trust the number at all.
	t.Run("a missing weight falls back to flat", func(t *testing.T) {
		got, exact := shippingFor([]cartLine{{idx: 0, qty: 1}, {idx: 2, qty: 1}}, func(i int) int { return weights[i] })
		if exact {
			t.Error("exact = true, but a book had no weight")
		}
		if got != flatShippingCents {
			t.Errorf("got %d, want the flat rate %d", got, flatShippingCents)
		}
	})

	t.Run("empty cart still pays for the parcel", func(t *testing.T) {
		got, exact := shippingFor(nil, func(int) int { return 0 })
		if !exact || got != mediaMail(packagingGrams) {
			t.Errorf("got %d exact=%v", got, exact)
		}
	})
}

// TestShelfWeightsPresent is the reminder: until every book carries its Ingram
// weight, real carts fall back to the flat rate.
func TestShelfWeightsPresent(t *testing.T) {
	var missing []string
	for _, b := range catalog {
		if b.WeightGrams <= 0 {
			missing = append(missing, b.ISBN)
		}
	}
	if len(missing) > 0 {
		t.Skipf("%d of %d books have no WeightGrams yet, so shipping falls back to flat: %v",
			len(missing), len(catalog), missing)
	}
}
