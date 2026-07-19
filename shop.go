package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// productList renders the catalog grouped by collection, windowed to maxRows.
func (m model) productList(maxRows, colW int) string {
	type row struct {
		text    string
		bookIdx int
	}
	maxw := colW - 3 // 1 leading space + 2-col gutter before the detail column
	rows := []row{}
	lastColl := ""
	for i, b := range catalog {
		if coll := section(i); coll != lastColl {
			if lastColl != "" {
				rows = append(rows, row{text: "", bookIdx: -1})
			}
			rows = append(rows, row{text: " " + secHead.Render("~ "+strings.ToLower(coll)+" ~"), bookIdx: -1})
			lastColl = coll
		}
		name := truncate(b.BookTitle, maxw)
		if i == m.cursor {
			// Width inside the style so the highlight spans the whole column,
			// as terminal.shop's does, rather than hugging the text. Sized from
			// the column being drawn: in one-column mode that is the full
			// content width, and a fixed leftCol would clip the focused row
			// while its neighbours ran on.
			rows = append(rows, row{text: selItem.Width(colW - 1).Render(" " + name), bookIdx: i})
		} else {
			rows = append(rows, row{text: romItem.Render(" " + name), bookIdx: i})
		}
	}

	cursorRow := 0
	for ri, r := range rows {
		if r.bookIdx == m.cursor {
			cursorRow = ri
		}
	}
	start := 0
	if len(rows) > maxRows {
		start = cursorRow - maxRows/2
		if start < 0 {
			start = 0
		}
		if start+maxRows > len(rows) {
			start = len(rows) - maxRows
		}
	}
	end := start + maxRows
	if end > len(rows) {
		end = len(rows)
	}

	var sb strings.Builder
	for _, r := range rows[start:end] {
		sb.WriteString(r.text)
		sb.WriteByte('\n')
	}
	return strings.TrimRight(sb.String(), "\n")
}

func (m model) detailView(w int) string {
	b := m.book(m.cursor)
	var sb strings.Builder
	// terminal.shop's detail shape: name, attributes on one pipe-joined line,
	// then the single number that matters, then the description.
	fmt.Fprintln(&sb, dTitle.Width(w).Render(b.BookTitle))

	fmt.Fprintln(&sb, dLabel.Width(w).Render(strings.Join(b.attrs(), " | ")))
	fmt.Fprintln(&sb)

	// The price, where terminal.shop puts it. For a book we no longer have, the
	// figure is Bookshop's current price rather than a former one of ours, so
	// it is not struck through: that would read as a discount. The status says
	// plainly that the shelf is empty.
	if p := b.Price(); p > 0 {
		fmt.Fprintln(&sb, dMonth.Render(usd(p))+dBody.Render(b.stockNote()))
	} else {
		fmt.Fprintln(&sb, dLabel.Render("price unavailable"))
	}
	fmt.Fprintln(&sb)
	fmt.Fprintln(&sb, dBody.Width(w).Render(b.Blurb))
	fmt.Fprintln(&sb)
	fmt.Fprint(&sb, m.action(w, b))
	return sb.String()
}

// action is what you can do with the book in front of you. Books we stock get
// terminal.shop's bare quantity stepper; the rest can only be linked out to,
// so they show the link itself.
func (m model) action(w int, b Book) string {
	if b.Sellable {
		// "- " and " +" dim, the count bright: same as their stepper.
		return dBody.Render("- ") + dValue.Render(fmt.Sprintf(" %d ", m.qtyInCart(m.cursor))) + dBody.Render(" +")
	}
	// The chip states the shelf status; the hint beside it says what the one
	// available action does. Its background starts flush with the title and
	// description above, so the accent block lines up with the column.
	chip := lipgloss.NewStyle().Background(accent).Foreground(ink)
	// The shop name carries the link rather than printing the affiliate URL,
	// which is long and ugly on a book page. Two ways to reach it: click it
	// (OSC 8) or press enter to copy it.
	tip := dValue.Render("enter") + dBody.Render(" to buy on ") +
		hyperlink(b.BuyURL(), dLink.Render("bookshop.org"))
	if m.copied {
		tip = dBody.Render("link copied to clipboard")
	}
	return chip.Render(" sold out ") + "  " + tip
}
