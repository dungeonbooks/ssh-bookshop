package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

type acctPage struct {
	title string
	body  string
}

// acctPageCount is the menu length. Kept alongside acctPages so navigation can
// bound the cursor without rendering every page body just to count them.
const acctPageCount = 4

// acctPages mirrors terminal.shop's account menu, minus the pages that would
// need accounts we don't have yet (subscriptions, tokens, apps, addresses).
func (m model) acctPages(w int) []acctPage {
	return []acctPage{
		{"order history", m.pgOrders(w)},
		{"faq", m.pgFAQ(w)},
		{"about", m.pgAbout(w)},
		{"debug info", m.pgDebug(w)},
	}
}

// accountMenu draws the page list. w is the column it is being drawn into, so
// the highlight matches it rather than assuming the two-column sidebar.
func (m model) accountMenu(pages []acctPage, w int) string {
	var sb strings.Builder
	for i, p := range pages {
		// One budget for both rows, computed once. The leading space is drawn
		// inside the highlight, so the title gets two less than the column: one
		// for the space, one for the style width. Selected and unselected rows
		// have to agree, and the surest way is for there to be one number.
		//
		// An over-long row does not clip here, it wraps, taking the whole menu
		// a line further down with it.
		row := " " + truncate(p.title, w-2)
		if i == m.acct {
			sb.WriteString(selItem.Width(w - 1).Render(row))
		} else {
			sb.WriteString(romItem.Render(row))
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

func (m model) pgFAQ(w int) string {
	qa := []struct{ q, a string }{
		{"help, i have a question about my order!",
			"come by the shop and ask at the counter, or email hello@dungeonbooks.com"},
		{"what is the book club?",
			"we read one science fiction or fantasy novel a month. we meet at the shop in jersey city to talk about it."},
		{"can i join the book club?",
			"yes. check dungeonbooks.com for the next meeting. you don't have to finish the book."},
	}
	var sb strings.Builder
	for i, p := range qa {
		if i > 0 {
			fmt.Fprintln(&sb)
		}
		fmt.Fprintln(&sb, dValue.Width(w).Render(p.q))
		fmt.Fprintln(&sb, dBody.Width(w).Render(p.a))
	}
	return strings.TrimRight(sb.String(), "\n")
}

// pgOrders stays empty because nothing keeps orders between sessions yet. Square
// holds them; the shop has no identity to look them up by.
func (m model) pgOrders(w int) string {
	return lipgloss.PlaceHorizontal(w, lipgloss.Center, romItem.Render("no orders found"))
}

func (m model) pgDebug(w int) string {
	var sb strings.Builder
	fmt.Fprintln(&sb, dValue.Render("connected with an ssh key"))
	fmt.Fprintln(&sb)
	fmt.Fprintln(&sb, dValue.Render("session"))
	kv := func(k, v string) {
		// A value that would wrap goes on its own indented line, so it never
		// wraps back to column zero and breaks the block.
		if lipgloss.Width(k)+lipgloss.Width(v)+3 > w {
			fmt.Fprintf(&sb, "  %s\n    %s\n", dLabel.Render(k+":"), dValue.Render(v))
			return
		}
		fmt.Fprintf(&sb, "  %s %s\n", dLabel.Render(k+":"), dValue.Render(v))
	}
	kv("mode", m.sess.mode)
	kv("fingerprint", strings.TrimPrefix(m.fingerprint, "SHA256:"))
	kv("term", m.sess.term)
	kv("size", fmt.Sprintf("%dx%d", m.width, m.height))
	kv("command", orNone(m.sess.command))
	fmt.Fprintln(&sb)
	fmt.Fprintln(&sb, dValue.Render("account"))
	kv("user", orNone(m.sess.user))
	kv("cart", fmt.Sprintf("%d item(s)", len(m.cart)))
	return strings.TrimRight(sb.String(), "\n")
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func (m model) pgAbout(w int) string {
	var sb strings.Builder
	fmt.Fprintln(&sb, dBody.Width(w).Render("an independent science fiction, fantasy, and role-playing game bookstore in jersey city, new jersey."))
	fmt.Fprintln(&sb)
	fmt.Fprintln(&sb, dValue.Render("made by @ptaranat"))
	fmt.Fprintln(&sb)
	fmt.Fprint(&sb, dBody.Render("inspired by terminal.shop"))
	return sb.String()
}
