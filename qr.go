package main

import (
	qrcode "github.com/skip2/go-qrcode"
)

// quietModules is the light margin kept around the code. The spec asks for 4;
// terminal.shop renders none at all and scans fine, because the terminal
// background supplies the contrast. 2 is the compromise: it survives a phone
// camera without costing 2 extra rows of a short window.
const quietModules = 2

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
	if len(bm) == 0 {
		return nil
	}

	trim := 4 - quietModules
	if trim < 0 || len(bm) <= 2*trim {
		trim = 0
	}
	bm = bm[trim : len(bm)-trim]
	for i := range bm {
		bm[i] = bm[i][trim : len(bm[i])-trim]
	}

	lines := make([]string, 0, (len(bm)+1)/2)
	for y := 0; y < len(bm); y += 2 {
		row := make([]rune, 0, len(bm[y]))
		for x := range bm[y] {
			top := bm[y][x]
			// An odd number of module rows leaves the last half empty, which
			// reads as a light module and keeps the quiet zone intact.
			bottom := false
			if y+1 < len(bm) {
				bottom = bm[y+1][x]
			}
			switch {
			case top && bottom:
				row = append(row, ' ')
			case top:
				row = append(row, '▄')
			case bottom:
				row = append(row, '▀')
			default:
				row = append(row, '█')
			}
		}
		lines = append(lines, string(row))
	}
	return lines
}
