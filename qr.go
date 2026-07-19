package main

import (
	qrcode "github.com/skip2/go-qrcode"
)

// qrLines renders a URL as half-block rows, two module rows per cell so the code
// stays roughly square. Returns nil if the URL won't encode, leaving the caller
// to fall back to the plain link.
//
// Dark modules are the glyphs and light ones are spaces, as terminal.shop does.
// That makes the quiet zone free and unbounded (the empty screen supplies it) and
// makes the trailing row of an odd module count invisible rather than padding.
// It reads inverted on a dark terminal, which scanners have long handled.
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
