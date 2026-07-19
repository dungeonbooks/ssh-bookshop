package main

// navWidths sizes the nav's cells to fill cw exactly. Each cell takes its label
// plus a column of padding either side, and the leftover is dealt out one column
// at a time from the left, so the row is always flush with the body below it.
//
// alignLogo pins the first cell so the divider after it lands on the detail
// column edge, which is what keeps the nav's first joint sitting over the
// two-column split. The columns that moves off the logo cell are taken from, or
// handed back to, the cells after it, so the total is unchanged.
//
// labels are the cells' rendered widths without padding. The returned slice is
// parallel to it.
func navWidths(labels []int, cw int, alignLogo bool) []int {
	n := len(labels)
	if n == 0 {
		return nil
	}

	widths := make([]int, n)
	used := n + 1 // the box bars, one between each cell and one at either end
	for i, lw := range labels {
		widths[i] = lw + 2
		used += widths[i]
	}
	for i := 0; used < cw; i = (i + 1) % n {
		widths[i]++
		used++
	}

	// A lone logo cell has nothing to trade columns with, so leave it be.
	if alignLogo && n > 1 {
		delta := widths[0] - (leftCol - 1)
		widths[0] = leftCol - 1
		for i := 1; delta != 0; i++ {
			if i >= n {
				i = 1
			}
			if delta > 0 {
				widths[i]++
				delta--
			} else {
				widths[i]--
				delta++
			}
		}
	}
	return widths
}
