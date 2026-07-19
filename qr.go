package main

import (
	qrcode "github.com/skip2/go-qrcode"
)

// qrLines renders a URL as half-block rows. Each cell carries two module rows,
// so the code stays roughly square instead of doubling in height.
//
// Dark modules are the printed glyphs and light modules are left as spaces, so
// the terminal background shows through. This is what terminal.shop does, and it
// buys two things over painting the light modules instead. The quiet zone costs
// nothing: the empty screen around the code already is one, and an unbounded one
// rather than the four modules the spec asks for. And the odd module count stops
// mattering. A QR is always odd on a side, so half-block pairing leaves a
// trailing row, which as a light row is simply invisible here.
//
// The cost is that on a dark terminal this reads inverted, dark modules coming
// out bright. Scanners have handled that for years, and it is the arrangement
// terminal.shop takes real orders through.
//
// Returns nil if the URL won't encode, so the caller falls back to the plain
// link rather than a broken box.
func qrLines(url string) []string {
	q, err := qrcode.New(url, qrcode.Low)
	if err != nil {
		return nil
	}
	bm := q.Bitmap() // true = dark module; includes a 4-module quiet zone
	if len(bm) < 8 || len(bm[0]) < 8 {
		return nil
	}

	// Drop the library's quiet zone entirely; the terminal supplies it.
	bm = bm[4 : len(bm)-4]
	for i := range bm {
		bm[i] = bm[i][4 : len(bm[i])-4]
	}

	lines := make([]string, 0, (len(bm)+1)/2)
	for y := 0; y < len(bm); y += 2 {
		row := make([]rune, len(bm[y]))
		for x := range bm[y] {
			// A trailing odd row pairs against light, which prints as nothing.
			bottom := y+1 < len(bm) && bm[y+1][x]
			switch top := bm[y][x]; {
			case top && bottom:
				row[x] = '█'
			case top:
				row[x] = '▀'
			case bottom:
				row[x] = '▄'
			default:
				row[x] = ' '
			}
		}
		lines = append(lines, string(row))
	}
	return lines
}
