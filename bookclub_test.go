package main

import "testing"

// The shelf leads the calendar: a pick is featured from the moment it is added,
// not from the 1st of its month, because that early window is exactly when we
// want the announcement visible.
func TestFeaturedIsNewestPick(t *testing.T) {
	i := featured()
	if i != 0 {
		t.Fatalf("featured() = %d, want 0 (the newest pick)", i)
	}
	if got := section(i); got != collFeatured {
		t.Errorf("section(%d) = %q, want %q", i, got, collFeatured)
	}
	for j := 1; j < len(catalog); j++ {
		if got := section(j); got == collFeatured {
			t.Errorf("section(%d) = %q, want only the newest pick featured", j, got)
		}
	}
}

// Each club is contiguous and newest first, and the Sci-Fi & Fantasy club
// leads: featured() returning index 0 depends on all three.
func TestCatalogIsNewestFirstPerClub(t *testing.T) {
	if catalog[0].Collection != collBookClub {
		t.Fatalf("catalog[0] is in %q, want the %q club first", catalog[0].Collection, collBookClub)
	}
	seen := map[string]bool{catalog[0].Collection: true}
	for i := 1; i < len(catalog); i++ {
		prev, cur := catalog[i-1], catalog[i]
		if cur.Collection != prev.Collection {
			if seen[cur.Collection] {
				t.Errorf("catalog[%d] starts %q a second time; keep each club together", i, cur.Collection)
			}
			seen[cur.Collection] = true
			continue
		}
		if prev.Month <= cur.Month {
			t.Errorf("catalog[%d] (%s) is not newer than catalog[%d] (%s) in %q",
				i-1, prev.Month, i, cur.Month, cur.Collection)
		}
	}
}

// A pick is keyed by ISBN everywhere: Square lookups, the API, checkout.
func TestCatalogISBNsAreUnique(t *testing.T) {
	seen := map[string]int{}
	for i, b := range catalog {
		if j, ok := seen[b.ISBN]; ok {
			t.Errorf("catalog[%d] and catalog[%d] share ISBN %s", j, i, b.ISBN)
		}
		seen[b.ISBN] = i
	}
}
