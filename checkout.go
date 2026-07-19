package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// breadcrumb is the checkout step indicator: only the current step is bright,
// and the whole thing disappears once the order is placed.
func (m model) breadcrumb() string {
	steps := []struct {
		label string
		at    step
	}{{"cart", stepCart}, {"delivery", stepFulfil}, {"checkout", stepPay}}
	parts := make([]string, 0, len(steps))
	for _, s := range steps {
		if s.at == m.step {
			parts = append(parts, dValue.Render(s.label))
		} else {
			parts = append(parts, dBody.Render(s.label))
		}
	}
	return " " + strings.Join(parts, dBody.Render(" / "))
}

func (m model) cartView(w, h int) string {
	switch m.step {
	case stepFulfil:
		return m.fulfilView(w)
	case stepPay:
		return m.payView(w, h)
	case stepDone:
		return m.doneView(w)
	case stepThanks:
		return m.thanksView(w)
	}

	if len(m.cart) == 0 {
		return lipgloss.PlaceHorizontal(w, lipgloss.Center, romItem.Render("your cart is empty"))
	}

	var sb strings.Builder
	fmt.Fprintln(&sb, m.breadcrumb())
	fmt.Fprintln(&sb)
	for i, l := range m.cart {
		fmt.Fprint(&sb, m.cartRow(w, i, l))
	}
	fmt.Fprintf(&sb, " %s\n", dBody.Render("total "+usd(m.cartTotal())))
	return sb.String()
}

// cartRow is one boxed line item. Focus shows as border brightness, and the
// quantity controls swap in for spaces of the same width so nothing shifts
// as the cursor moves.
func (m model) cartRow(w int, i int, l cartLine) string {
	b := m.book(l.idx)
	border := boxDim
	minus, plus := " ", " "
	if i == m.cartCursor {
		border = lipgloss.NewStyle().Foreground(white)
		minus, plus = "-", "+"
	}

	// One space of margin, then the box. content is what fits between the
	// borders and their padding.
	boxW := w - 2
	if boxW < 28 {
		boxW = 28
	}
	content := boxW - 4

	// The price is deliberately dim and the quantity bright: the number you
	// might change is the one worth looking at.
	right := dBody.Render(minus+" ") + dValue.Render(fmt.Sprint(l.qty)) +
		dBody.Render(" "+plus+"  "+usd(b.Cents*int64(l.qty)))
	rightW := lipgloss.Width(right)

	name := truncate(b.BookTitle, content-rightW-1)
	gap := content - lipgloss.Width(name) - rightW
	if gap < 1 {
		gap = 1
	}
	attrs := truncate(strings.Join(b.attrs(), " | "), content)

	line := func(body string, bodyW int) string {
		return " " + border.Render("│") + " " + body +
			strings.Repeat(" ", max(0, content-bodyW)) + " " + border.Render("│")
	}
	rule := func(l, r string) string {
		return " " + border.Render(l+strings.Repeat("─", boxW-2)+r)
	}

	return rule("┌", "┐") + "\n" +
		line(dValue.Render(name)+strings.Repeat(" ", gap)+right, lipgloss.Width(name)+gap+rightW) + "\n" +
		line(dBody.Render(attrs), lipgloss.Width(attrs)) + "\n" +
		rule("└", "┘") + "\n"
}

// shipping prices the cart at Media Mail, or falls back to the flat rate when a
// book has no recorded weight.
func (m model) shipping() (cents int64, exact bool) {
	return shippingFor(m.cart, func(i int) int { return m.book(i).WeightGrams })
}

// fulfilView is the choice Square's hosted page cannot offer, so it has to be
// made before the order exists.
func (m model) fulfilView(w int) string {
	var sb strings.Builder
	fmt.Fprintln(&sb, m.breadcrumb())
	fmt.Fprintln(&sb)

	opt := func(sel bool, label, detail string) string {
		marker, st := "  ", romItem
		if sel {
			marker, st = dValue.Render("> "), dValue
		}
		return " " + marker + st.Render(label) + "\n     " + dBody.Render(detail)
	}
	fmt.Fprintln(&sb, opt(m.fulfil == fulfilPickup, "pick up at the shop",
		"115 Brunswick St, Jersey City. We'll email when it's ready."))
	fmt.Fprintln(&sb)
	ship, exact := m.shipping()
	note := fmt.Sprintf("%s, US only. 6-12 business days.", usd(ship))
	if !exact {
		note = fmt.Sprintf("%s flat, US only. 6-12 business days.", usd(ship))
	}
	fmt.Fprintln(&sb, opt(m.fulfil == fulfilShip, "ship it to me", note))
	fmt.Fprintln(&sb)

	total := m.cartTotal()
	if m.fulfil == fulfilShip {
		ship, _ := m.shipping()
		fmt.Fprintf(&sb, " %s\n", dBody.Render(fmt.Sprintf("books %s  +  shipping %s", usd(total), usd(ship))))
		total += ship
	}
	fmt.Fprintf(&sb, " %s %s\n", dLabel.Render("total"), dValue.Render(usd(total)))
	return sb.String()
}

// payView hands the card details to Square: a QR of the hosted checkout, the
// URL to copy, and a quiet note that we're watching for the payment.
func (m model) payView(w, h int) string {
	var sb strings.Builder
	fmt.Fprintln(&sb, m.breadcrumb())
	fmt.Fprintln(&sb)

	if m.checkoutErr != nil {
		fmt.Fprintln(&sb, lipgloss.PlaceHorizontal(w, lipgloss.Center, dValue.Render("checkout failed")))
		fmt.Fprintln(&sb)
		fmt.Fprintln(&sb, lipgloss.PlaceHorizontal(w, lipgloss.Center, dBody.Render(m.checkoutErr.Error())))
		return sb.String()
	}
	if m.checkout.URL == "" {
		return sb.String() + lipgloss.PlaceHorizontal(w, lipgloss.Center, dBody.Render("building your order…"))
	}

	// The URL is the fallback for anyone who can't scan, so it is never what
	// gets clipped. The QR is dropped instead when the window is too short
	// for both, or too narrow to draw a full row: lipgloss clips rather than
	// overflows, and a code missing its right-hand columns still looks like a
	// code while being impossible to scan.
	qr := qrLines(m.checkout.URL)
	if len(qr) > 0 && len(qr)+qrChrome <= h && lipgloss.Width(qr[0]) <= w {
		for _, line := range qr {
			fmt.Fprintln(&sb, lipgloss.PlaceHorizontal(w, lipgloss.Center, dValue.Render(line)))
		}
		fmt.Fprintln(&sb)
	}
	fmt.Fprintln(&sb, lipgloss.PlaceHorizontal(w, lipgloss.Center, dBody.Render("scan or copy to check out")))
	fmt.Fprintln(&sb, lipgloss.PlaceHorizontal(w, lipgloss.Center,
		hyperlink(m.checkout.URL, dLink.Render(m.checkout.URL))))
	// A cursor rather than an ellipsis: it shows the shop is still watching.
	cursor := " "
	if m.cursorOn {
		cursor = logoStyle.Render("█")
	}
	fmt.Fprint(&sb, lipgloss.PlaceHorizontal(w, lipgloss.Center,
		dBody.Render("waiting for payment ")+cursor))
	return sb.String()
}

func (m model) doneView(w int) string {
	var sb strings.Builder
	fmt.Fprintln(&sb, dValue.Render(" order complete!"))
	fmt.Fprintln(&sb)
	for _, l := range m.placed {
		fmt.Fprintf(&sb, " %s\n", dBody.Render(fmt.Sprintf("%s (x%d)", catalog[l.idx].BookTitle, l.qty)))
	}
	fmt.Fprintln(&sb)
	fmt.Fprint(&sb, dBody.Render(" press ")+dValue.Render("enter")+dBody.Render(" to continue"))
	return sb.String()
}

// thanksView breaks the lowercase house style on purpose: it's a letter.
func (m model) thanksView(w int) string {
	// PaddingLeft rather than a prefix, so every wrapped line lands on the same
	// column as the rest of the body instead of only the first.
	para := lipgloss.NewStyle().Foreground(gray).Width(w).PaddingLeft(1)
	var sb strings.Builder
	fmt.Fprintln(&sb, para.Render("Thank you for ordering from Dungeon Books."))
	fmt.Fprintln(&sb)
	if m.fulfil == fulfilShip {
		fmt.Fprintln(&sb, para.Render("Your books ship from Jersey City within 3-5 business days, and usually arrive 3-7 after that. Square has emailed you a receipt, and we'll send a tracking number once they're on their way."))
	} else {
		fmt.Fprintln(&sb, para.Render("Your books are set aside at the shop in Jersey City, 115 Brunswick St. Square has emailed you a receipt, and we'll email again once they're ready to collect."))
	}
	fmt.Fprintln(&sb)
	fmt.Fprintln(&sb, para.Render("If you're reading the book club pick, come argue about it with us at the end of the month."))
	fmt.Fprintln(&sb)
	// The signature is the one bright line: it reads as a hand rather than a
	// system message.
	sig := lipgloss.NewStyle().Foreground(white).Width(w).PaddingLeft(1)
	fmt.Fprintln(&sb, sig.Render("Carrie and Panat"))
	fmt.Fprint(&sb, para.Render("Dungeon Books"))
	return sb.String()
}
