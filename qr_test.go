package main

import (
	"strings"
	"testing"

	qrcode "github.com/skip2/go-qrcode"
)

// TestQRMatchesBitmap checks the half-block renderer against the library's own
// bitmap: every module must land in the right half of the right cell. A QR that
// merely looks like a QR is worthless if it doesn't scan.
func TestQRMatchesBitmap(t *testing.T) {
	const url = "https://square.link/u/AbCd1234"
	lines := qrLines(url)
	if len(lines) == 0 {
		t.Fatal("no lines")
	}

	q, err := qrcode.New(url, qrcode.Low)
	if err != nil {
		t.Fatal(err)
	}
	bm := q.Bitmap()
	bm = bm[4-quietTop : len(bm)-(4-quietBottom)]
	for i := range bm {
		bm[i] = bm[i][4-quietX : len(bm[i])-(4-quietX)]
	}

	if len(bm)%2 != 0 {
		t.Fatalf("module rows = %d, want even so no half-row is padded", len(bm))
	}
	if got, want := len(lines), len(bm)/2; got != want {
		t.Fatalf("lines = %d, want %d", got, want)
	}
	for y := 0; y < len(bm); y += 2 {
		row := []rune(lines[y/2])
		if len(row) != len(bm[y]) {
			t.Fatalf("row %d width = %d, want %d", y/2, len(row), len(bm[y]))
		}
		for x := range bm[y] {
			top := bm[y][x]
			bottom := bm[y+1][x]
			// dark module must NOT be a bright half
			gotTop := row[x] == '█' || row[x] == '▀'
			gotBottom := row[x] == '█' || row[x] == '▄'
			if gotTop == top {
				t.Fatalf("cell (%d,%d) top polarity wrong: rune %q, dark=%v", x, y, row[x], top)
			}
			if gotBottom == bottom {
				t.Fatalf("cell (%d,%d) bottom polarity wrong: rune %q, dark=%v", x, y, row[x], bottom)
			}
		}
	}
}

// TestQRQuietZone guards the margin: too little and phone cameras stop finding
// the code. The first and last rows must both be a full light cell, so the code
// is not lopsided.
func TestQRQuietZone(t *testing.T) {
	lines := qrLines("https://square.link/u/AbCd1234")
	for _, idx := range []int{0, len(lines) - 1} {
		if strings.Trim(lines[idx], "█") != "" {
			t.Errorf("row %d should be all light (quiet zone), got %q", idx, lines[idx])
		}
	}
	// and exactly one such row at each end, or the margin is uneven
	if strings.Trim(lines[1], "█") == "" {
		t.Error("two full light rows at the top: quiet zone is lopsided")
	}
	if strings.Trim(lines[len(lines)-2], "█") == "" {
		t.Error("two full light rows at the bottom: quiet zone is lopsided")
	}
	for _, l := range lines {
		if !strings.HasPrefix(l, "██") || !strings.HasSuffix(l, "██") {
			t.Errorf("row lacks side quiet zone: %q", l)
		}
	}
}

// TestQRFitsCheckout guards the case that actually broke: a sandbox payment URL
// is longer than a production one, so its QR is taller, and the checkout screen
// silently fell back to the bare link. Both must render the code.
func TestQRFitsCheckout(t *testing.T) {
	for _, url := range []string{
		"https://square.link/u/E9zH3J5R",
		"https://sandbox.square.link/u/E9zH3J5R",
	} {
		m := newModel(100, 40, "k")
		m.ready = true
		m.tab, m.step = tabCart, stepPay
		m.checkout = checkout{URL: url, OrderID: "O1"}
		out := ansi.ReplaceAllString(m.View(), "")
		if !strings.Contains(out, "█") {
			t.Errorf("%s: no QR rendered on the checkout screen", url)
		}
		if !strings.Contains(out, url) {
			t.Errorf("%s: url missing from the checkout screen", url)
		}
	}
}
