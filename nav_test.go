package main

import "testing"

// The nav's rules have to line up with the body beneath them, so the cells must
// fill the content width exactly rather than approximately. That arithmetic used
// to live inside nav(), where a test could only reach it through rendered output
// and styling.

// shopLabels are the real cell widths on the shop tab: the wordmark, two plain
// hotkey cells, and the cart with a price and count in it.
var shopLabels = []int{12, 6, 9, 14}

// occupied is what the cells actually consume: their own widths plus the box
// bar between each pair and one at either end.
func occupied(widths []int) int {
	n := len(widths) + 1
	for _, w := range widths {
		n += w
	}
	return n
}

// minWidth is the narrowest cw that fits: every cell at its label plus single
// padding. Below it nav() falls back to the plain line and never calls here.
func minWidth(labels []int) int {
	cw := len(labels) + 1
	for _, lw := range labels {
		cw += lw + 2
	}
	return cw
}

func TestNavWidthsFillExactly(t *testing.T) {
	for cw := minWidth(shopLabels); cw <= 200; cw++ {
		if got := occupied(navWidths(shopLabels, cw, false)); got != cw {
			t.Fatalf("cw=%d: cells occupy %d", cw, got)
		}
	}
}

// The leftover is dealt out from the left one column at a time, so the widest
// cell is not the one that absorbs it all.
func TestNavWidthsSpreadLeftoverFromTheLeft(t *testing.T) {
	cw := minWidth(shopLabels) + 2 // exactly two spare columns
	got := navWidths(shopLabels, cw, false)
	want := []int{15, 9, 11, 16}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("cw=%d: widths %v, want %v", cw, got, want)
		}
	}
}

// Two-column mode is the whole reason this alignment exists: the divider after
// the logo has to land on the detail column edge so the nav's first joint sits
// over the split below it. cw is always 78 there, since twoCol needs a window of
// at least 80 and the clamp to m.width-2 can never bite.
func TestNavWidthsAlignLogoToDetailColumn(t *testing.T) {
	got := navWidths(shopLabels, 78, true)
	if got[0] != leftCol-1 {
		t.Fatalf("logo cell = %d, want %d so the divider lands on leftCol", got[0], leftCol-1)
	}
	if occupied(got) != 78 {
		t.Fatalf("aligning the logo changed the total to %d, want 78", occupied(got))
	}
}

// Aligning the logo moves columns onto the cells after it, so it must not drive
// any of them negative: strings.Repeat panics on a negative count, and it would
// panic inside a render, taking the session with it.
func TestNavWidthsNeverGoNegative(t *testing.T) {
	for cw := minWidth(shopLabels); cw <= 200; cw++ {
		for _, w := range navWidths(shopLabels, cw, true) {
			if w < 0 {
				t.Fatalf("cw=%d: negative cell width %d", cw, w)
			}
		}
	}
}

// A lone logo has no neighbours to trade columns with, so alignment is skipped
// rather than looping forever looking for one.
func TestNavWidthsSingleCellIgnoresAlignment(t *testing.T) {
	got := navWidths([]int{12}, 20, true)
	if occupied(got) != 20 {
		t.Fatalf("single cell occupies %d, want 20", occupied(got))
	}
}

// nav() only calls this after fits() passes, so an empty nav is unreachable
// today. Guarded anyway because the leftover loop divides by the cell count.
func TestNavWidthsEmpty(t *testing.T) {
	if got := navWidths(nil, 80, true); got != nil {
		t.Fatalf("navWidths(nil) = %v, want nil", got)
	}
}
