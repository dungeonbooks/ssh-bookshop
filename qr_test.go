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
	trim := 4 - quietModules
	bm = bm[trim : len(bm)-trim]
	for i := range bm {
		bm[i] = bm[i][trim : len(bm[i])-trim]
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
			bottom := y+1 < len(bm) && bm[y+1][x]
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
// the code.
func TestQRQuietZone(t *testing.T) {
	lines := qrLines("https://square.link/u/AbCd1234")
	first := lines[0]
	if strings.Trim(first, "█") != "" {
		t.Errorf("first row should be all light (quiet zone), got %q", first)
	}
	for _, l := range lines {
		if !strings.HasPrefix(l, "██") || !strings.HasSuffix(l, "██") {
			t.Errorf("row lacks side quiet zone: %q", l)
		}
	}
}
