package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// navWidths sizes the nav's cells to fill cw exactly. Each cell takes its label
// plus a column of padding either side, and the leftover is dealt out one column
// at a time from the left, so the row is always flush with the body below it.
//
// cw has to be wide enough for every label at that minimum padding. Below it
// there is no leftover to deal out and the cells returned overrun cw rather than
// shrinking to meet it, so the caller checks first: nav() drops to a plain line
// when fits() fails, and the case never reaches here.
//
// alignLogo pins the first cell so the divider after it lands on the detail
// column edge, which is what keeps the nav's first joint sitting over the
// two-column split. The columns that move off the logo cell are taken from, or
// handed back to, the cells after it, so the total is unchanged.
//
// labels are the cells' rendered widths without padding. The returned slice is
// parallel to it.
func navWidths(labels []int, cw int, alignLogo bool) []int {
	n := len(labels)
	if n == 0 {
		return nil
	}

	widths := make([]int, n)
	used := n + 1 // the box bars, one between each cell and one at either end
	for i, lw := range labels {
		widths[i] = lw + 2
		used += widths[i]
	}
	for i := 0; used < cw; i = (i + 1) % n {
		widths[i]++
		used++
	}

	// A lone logo cell has nothing to trade columns with, so leave it be.
	if alignLogo && n > 1 {
		delta := widths[0] - (leftCol - 1)
		widths[0] = leftCol - 1
		for i := 1; delta != 0; i++ {
			if i >= n {
				i = 1
			}
			if delta > 0 {
				widths[i]++
				delta--
			} else {
				widths[i]--
				delta++
			}
		}
	}
	return widths
}

type tab int

const (
	tabShop tab = iota
	tabAccount
	tabCart
)

func (t tab) String() string {
	switch t {
	case tabAccount:
		return "account"
	case tabCart:
		return "cart"
	default:
		return "shop"
	}
}

// nav draws the boxed cell bar. Cells stretch to fill cw so the bar spans the
// same width as the body below it (terminal.shop's behavior). When too narrow,
// the logo is dropped, then it collapses to a plain line.
func (m model) nav(cw int, twoCol bool) string {
	type cell struct {
		hot, label string
		on, logo   bool
	}
	cartLabel := fmt.Sprintf("cart %s [%d]", usd(m.cartTotal()), m.cartCount())
	mk := func(withLogo bool) []cell {
		cs := []cell{}
		if withLogo {
			cs = append(cs, cell{label: "dungeonbooks", logo: true})
		}
		return append(cs,
			cell{hot: "s", label: "shop", on: m.tab == tabShop},
			cell{hot: "a", label: "account", on: m.tab == tabAccount},
			cell{hot: "c", label: cartLabel, on: m.tab == tabCart},
		)
	}
	plain := func(c cell) string {
		if c.logo {
			return c.label
		}
		return c.hot + " " + c.label
	}
	// Only the wordmark is bold. The focused tab goes entirely white; the rest
	// keep a white hotkey over a gray label. In the cart cell the money stays
	// white either way, as terminal.shop's does.
	styled := func(c cell) string {
		if c.logo {
			return active.Render(c.label)
		}
		st := inactive
		if c.on {
			st = hotkey
		}
		label := st.Render(c.label)
		if c.hot == "c" {
			label = st.Render("cart ") + hotkey.Render(usd(m.cartTotal())) +
				st.Render(fmt.Sprintf(" [%d]", m.cartCount()))
		}
		return hotkey.Render(c.hot) + " " + label
	}
	fits := func(cells []cell) bool {
		need := len(cells) + 1 // box bars
		for _, c := range cells {
			need += lipgloss.Width(plain(c)) + 2 // min 1 pad each side
		}
		return need <= cw
	}

	var cells []cell
	switch {
	case fits(mk(true)):
		cells = mk(true)
	case fits(mk(false)):
		cells = mk(false)
	default:
		return m.navPlain(cw)
	}

	n := len(cells)
	labels := make([]int, n)
	for i, c := range cells {
		labels[i] = lipgloss.Width(plain(c))
	}
	widths := navWidths(labels, cw, twoCol && cells[0].logo)

	var top, mid, bot strings.Builder
	top.WriteString("┌")
	bot.WriteString("└")
	mid.WriteString(boxDim.Render("│"))
	for i, c := range cells {
		w := widths[i]
		top.WriteString(strings.Repeat("─", w))
		bot.WriteString(strings.Repeat("─", w))
		p := plain(c)
		padL := (w - lipgloss.Width(p)) / 2
		padR := w - lipgloss.Width(p) - padL
		mid.WriteString(strings.Repeat(" ", padL))
		mid.WriteString(styled(c))
		mid.WriteString(strings.Repeat(" ", padR))
		mid.WriteString(boxDim.Render("│"))
		if i < n-1 {
			top.WriteString("┬")
			bot.WriteString("┴")
		}
	}
	top.WriteString("┐")
	bot.WriteString("┘")
	return lipgloss.JoinVertical(lipgloss.Left,
		boxDim.Render(top.String()), mid.String(), boxDim.Render(bot.String()),
	)
}

func (m model) navPlain(cw int) string {
	seg := func(hot, label string, on bool) string {
		st := inactive
		if on {
			st = active
		}
		return active.Render(hot) + " " + st.Render(label)
	}
	// Same shape as the boxed nav, so the two layouts agree.
	cartLabel := fmt.Sprintf("cart %s [%d]", usd(m.cartTotal()), m.cartCount())
	sep := navSep.Render(" · ")
	line := seg("s", "shop", m.tab == tabShop) + sep +
		seg("a", "acct", m.tab == tabAccount) + sep +
		seg("c", cartLabel, m.tab == tabCart)
	return lipgloss.PlaceHorizontal(cw, lipgloss.Center, line)
}
