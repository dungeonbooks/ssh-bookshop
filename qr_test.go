package main

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
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
	bm = bm[4 : len(bm)-4] // the library's quiet zone, which we do not draw
	for i := range bm {
		bm[i] = bm[i][4 : len(bm[i])-4]
	}

	if got, want := len(lines), (len(bm)+1)/2; got != want {
		t.Fatalf("lines = %d, want %d", got, want)
	}
	for y := 0; y < len(bm); y += 2 {
		row := []rune(lines[y/2])
		if len(row) != len(bm[y]) {
			t.Fatalf("row %d width = %d, want %d", y/2, len(row), len(bm[y]))
		}
		for x := range bm[y] {
			top := bm[y][x]
			// An odd module count leaves the final row paired against light.
			bottom := y+1 < len(bm) && bm[y+1][x]
			// A dark module IS the printed half; light is left as background.
			gotTop := row[x] == '█' || row[x] == '▀'
			gotBottom := row[x] == '█' || row[x] == '▄'
			if gotTop != top {
				t.Fatalf("cell (%d,%d) top polarity wrong: rune %q, dark=%v", x, y, row[x], top)
			}
			if gotBottom != bottom {
				t.Fatalf("cell (%d,%d) bottom polarity wrong: rune %q, dark=%v", x, y, row[x], bottom)
			}
		}
	}
}

// TestQRNoDrawnMargin guards the margin the other way round from its
// predecessor. The quiet zone is no longer drawn: light modules are spaces, so
// the empty screen around the code supplies it, unbounded and for free.
//
// What that leaves to check is that the block is exactly the symbol and nothing
// more, on every side. A drawn margin would be pure waste now, and rows are the
// scarce thing on this screen. It cannot be checked by looking for light modules
// at the edges, because a QR has finder patterns in three of its corners and
// those are dark by definition: the first and last rows always carry glyphs.
// Compare against the module count instead.
func TestQRNoDrawnMargin(t *testing.T) {
	lines := qrLines("https://square.link/u/AbCd1234")
	q, err := qrcode.New("https://square.link/u/AbCd1234", qrcode.Low)
	if err != nil {
		t.Fatal(err)
	}
	modules := len(q.Bitmap()) - 8
	if got, want := len(lines), (modules+1)/2; got != want {
		t.Errorf("lines = %d, want %d: a margin is being drawn", got, want)
	}
	for i, l := range lines {
		if got := len([]rune(l)); got != modules {
			t.Errorf("row %d is %d cells, want %d: a side margin is being drawn", i, got, modules)
		}
	}
	// Light modules must be plain spaces. Anything else, a shaded block or a
	// non-breaking space, paints over the terminal background and takes the free
	// quiet zone with it.
	for i, l := range lines {
		for _, r := range l {
			if r != ' ' && r != '█' && r != '▀' && r != '▄' {
				t.Errorf("row %d has rune %q, want only spaces and half blocks", i, r)
				break
			}
		}
	}
}

// hasQR looks for the half-block runes, which only qrLines emits. Testing for
// '█' instead is what let this bug sit: the waiting-for-payment cursor is a full
// block too, so the assertion passed on screens with no code on them at all.
func hasQR(s string) bool { return strings.ContainsAny(s, "▀▄") }

// TestQRFitsCheckout guards the case that actually broke: the code needed a
// 30-row window and was dropped in silence below that. Rendering dark modules
// as the glyphs, so the terminal supplies the quiet zone, took a production
// code from 15 rows to 13 and brought it under the full frame. The frame is
// never given up, so it must be present every time the code is.
func TestQRFitsCheckout(t *testing.T) {
	const promo = "support independent bookstores"
	for _, tc := range []struct {
		url  string
		minH int
	}{
		{"https://square.link/u/E9zH3J5R", 28},
		{"https://sandbox.square.link/u/E9zH3J5R", 30}, // two rows taller
	} {
		for _, h := range []int{tc.minH, 32, 40, 51} {
			m := newModel(100, h, "k")
			m.ready = true
			m.tab, m.step = tabCart, stepPay
			m.checkout = checkout{URL: tc.url, OrderID: "O1"}
			out := ansi.ReplaceAllString(m.render(), "")
			if !hasQR(out) {
				t.Errorf("%s at height %d: no QR on the checkout screen", tc.url, h)
			}
			if !strings.Contains(out, promo) {
				t.Errorf("%s at height %d: frame missing, it is never given up", tc.url, h)
			}
			if !strings.Contains(out, tc.url) {
				t.Errorf("%s at height %d: url missing from the checkout screen", tc.url, h)
			}
			// The code must never cost more rows than the window has.
			if got := strings.Count(out, "\n") + 1; got > h {
				t.Errorf("%s at height %d: view is %d rows, overflows", tc.url, h, got)
			}
		}
	}
}

// TestQRFallsBackWhenTooShort pins the other half: below the fitting height the
// screen drops the code rather than overflowing or shedding the frame, and the
// URL always survives because it is the fallback for anyone who cannot scan.
func TestQRFallsBackWhenTooShort(t *testing.T) {
	const url = "https://square.link/u/E9zH3J5R"
	for _, h := range []int{8, 16, 24, 27} {
		m := newModel(100, h, "k")
		m.ready = true
		m.tab, m.step = tabCart, stepPay
		m.checkout = checkout{URL: url, OrderID: "O1"}
		out := ansi.ReplaceAllString(m.render(), "")
		if hasQR(out) {
			t.Errorf("height %d: QR rendered but cannot fit", h)
		}
		if !strings.Contains(out, url) {
			t.Errorf("height %d: url dropped, nothing left to check out with", h)
		}
	}
}

// TestQRDroppedWhenTooNarrow guards the clipping case. lipgloss truncates rather
// than overflowing, so a code wider than the content column loses its right-hand
// columns and still looks like a code while being impossible to scan. Better to
// show none. A production code is 25 cells; the content column is width-2.
func TestQRDroppedWhenTooNarrow(t *testing.T) {
	const url = "https://square.link/u/E9zH3J5R"
	if w := lipgloss.Width(qrLines(url)[0]); w != 25 {
		t.Fatalf("QR width = %d, want 25; the widths below assume it", w)
	}
	for _, tc := range []struct {
		w    int
		want bool
	}{
		{20, false}, {26, false}, {27, true}, {34, true}, {100, true},
	} {
		m := newModel(tc.w, 40, "k")
		m.ready = true
		m.tab, m.step = tabCart, stepPay
		m.checkout = checkout{URL: url, OrderID: "O1"}
		out := ansi.ReplaceAllString(m.render(), "")
		if got := hasQR(out); got != tc.want {
			t.Errorf("width %d: QR present = %v, want %v", tc.w, got, tc.want)
		}
		// Whatever is drawn, every row of it must be whole.
		for _, l := range strings.Split(out, "\n") {
			if !strings.ContainsAny(l, "▀▄") {
				continue
			}
			if w := lipgloss.Width(strings.TrimRight(l, " ")); w > tc.w {
				t.Errorf("width %d: QR row is %d cells, wider than the window", tc.w, w)
				break
			}
		}
	}
}
