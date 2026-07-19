package main

import (
	qrcode "github.com/skip2/go-qrcode"
)

// Quiet zone kept around the code, in modules. The spec asks for 4 on every
// side; terminal.shop renders none at all and still scans, because the terminal
// background supplies the contrast. These are the compromise that keeps the
// code above its URL in a short window.
//
// The vertical pair is deliberately uneven. A QR is always an odd number of
// modules across, so an even quiet zone leaves an odd row count, and half-block
// rendering then pads the final half-row light: one extra white cell along the
// bottom and none on top. Three above and two below makes the total even, so
// each side ends up with one full white cell plus a half-light row against the
// data. Symmetric, and nothing is padded.
const (
	quietX      = 2
	quietTop    = 3
	quietBottom = 2
)

// qrLines renders a URL as half-block rows. Each cell carries two module rows,
// so the code stays roughly square instead of doubling in height.
//
// Polarity matters: on a dark terminal the light modules must be the bright
// ones, so a light module prints as a block and a dark module as a space.
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

	// The library gives 4 modules of quiet zone; trim down to ours.
	bm = bm[4-quietTop : len(bm)-(4-quietBottom)]
	for i := range bm {
		bm[i] = bm[i][4-quietX : len(bm[i])-(4-quietX)]
	}
	if len(bm)%2 != 0 {
		return nil // would need padding, which is the asymmetry we just fixed
	}

	lines := make([]string, 0, len(bm)/2)
	for y := 0; y < len(bm); y += 2 {
		row := make([]rune, len(bm[y]))
		for x := range bm[y] {
			switch top, bottom := bm[y][x], bm[y+1][x]; {
			case top && bottom:
				row[x] = ' '
			case top:
				row[x] = '▄'
			case bottom:
				row[x] = '▀'
			default:
				row[x] = '█'
			}
		}
		lines = append(lines, string(row))
	}
	return lines
}
