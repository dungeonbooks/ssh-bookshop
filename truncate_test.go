package main

import (
	"strings"
	"testing"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
)

// truncate is what keeps a title inside its column, so the one thing it must
// never do is return something wider than it was asked for.
func TestTruncateNeverExceedsItsWidth(t *testing.T) {
	for _, s := range []string{
		"The Pragmatic Programmer",
		"Gödel, Escher, Bach",   // combining and accented runes
		"三体",                    // wide runes, two cells each
		"Daughter of Crows 🐦‍⬛", // emoji with a zero-width joiner
		"a",
		"",
	} {
		for w := 1; w <= 30; w++ {
			got := truncate(s, w)
			if lipgloss.Width(got) > w {
				t.Errorf("truncate(%q, %d) = %q, %d cells wide", s, w, got, lipgloss.Width(got))
			}
		}
	}
}

// Byte slicing a string measured in display cells is how the old version cut
// multibyte titles apart. Nothing that comes back may be invalid UTF-8.
func TestTruncateKeepsRunesWhole(t *testing.T) {
	const title = "Gödel, Escher, Bach"
	for w := 1; w <= len(title); w++ {
		got := truncate(title, w)
		if !utf8.ValidString(got) {
			t.Fatalf("truncate(%q, %d) = %q, which is not valid UTF-8", title, w, got)
		}
	}
}

// Anything short enough is left alone: no ellipsis on a title that already fits.
func TestTruncateLeavesShortStringsAlone(t *testing.T) {
	if got := truncate("short", 20); got != "short" {
		t.Errorf("truncate = %q, want it untouched", got)
	}
	if got := truncate("exactly ten", 11); got != "exactly ten" {
		t.Errorf("truncate = %q, want it untouched at exactly the width", got)
	}
	if got := truncate("just over", 8); !strings.HasSuffix(got, "…") {
		t.Errorf("truncate = %q, want an ellipsis marking the cut", got)
	}
}

// The focused row is styled to a fixed width so the highlight spans the column.
// Sizing it from a constant rather than the column being drawn clipped the
// selected book in one-column mode while its neighbours ran on to full width.
func TestSelectedRowMatchesTheColumnWidth(t *testing.T) {
	m := newModel(60, 30, "fp") // under twoColMin, so one column
	m.ready = true
	cw, _, _, twoCol := m.dims()
	if twoCol {
		t.Fatal("want a one-column layout for this test")
	}

	list := ansi.ReplaceAllString(m.productList(12, cw), "")
	for _, line := range strings.Split(list, "\n") {
		if w := lipgloss.Width(line); w > cw {
			t.Fatalf("row %q is %d cells in a %d column", line, w, cw)
		}
	}

	// The selected row should fill the column rather than stopping at leftCol.
	var widest int
	for _, line := range strings.Split(list, "\n") {
		if w := lipgloss.Width(line); w > widest {
			widest = w
		}
	}
	if widest <= leftCol {
		t.Errorf("widest row is %d cells, still pinned near leftCol (%d) in a %d column",
			widest, leftCol, cw)
	}
}
