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

// Newest-first ordering is the whole basis for featured() returning index 0.
func TestCatalogIsNewestFirst(t *testing.T) {
	for i := 1; i < len(catalog); i++ {
		if catalog[i-1].Month <= catalog[i].Month {
			t.Errorf("catalog[%d] (%s) is not newer than catalog[%d] (%s)",
				i-1, catalog[i-1].Month, i, catalog[i].Month)
		}
	}
}
