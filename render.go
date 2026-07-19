package main

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// View is what bubbletea renders. The work is in render(); this only wraps it,
// which also keeps every test asserting on plain strings.
func (m model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

func (m model) render() string {
	if !m.ready {
		// A space when the cursor is off, so the wordmark never shifts.
		cursor := " "
		if m.cursorOn {
			cursor = logoStyle.Render("█")
		}
		splash := active.Render("dungeonbooks") + cursor
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, splash)
	}

	cw, rightW, bodyH, twoCol := m.dims()

	// Build the body content, clamped to the fixed body-area height (bodyH).
	var body string
	switch {
	case m.tab == tabCart:
		body = clampLines(m.cartView(cw, bodyH), bodyH)

	case twoCol:
		var left, right string
		if m.tab == tabAccount {
			pages := m.acctPages(rightW)
			left, right = m.accountMenu(pages, leftCol), pages[m.acct].body
		} else {
			left, right = m.productList(bodyH, leftCol), m.detailView(rightW)
		}
		body = lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().Width(leftCol).Render(clampLines(left, bodyH)),
			lipgloss.NewStyle().Width(rightW).Render(clampLines(right, bodyH)),
		)

	default: // single column: stack list/menu above detail/page, split bodyH
		listRows := bodyH / 2
		if listRows < 4 {
			listRows = 4
		}
		detBudget := bodyH - listRows - 1
		if detBudget < 3 {
			detBudget = 3
		}
		var top, bottom string
		if m.tab == tabAccount {
			pages := m.acctPages(cw)
			top, bottom = m.accountMenu(pages, cw), pages[m.acct].body
		} else {
			top, bottom = m.productList(listRows, cw), m.detailView(cw)
		}
		body = lipgloss.JoinVertical(lipgloss.Left,
			clampLines(top, listRows), "", clampLines(bottom, detBudget))
	}

	// fixed-height body area pins the footer at a stable row (terminal.shop)
	bodyArea := lipgloss.NewStyle().Width(cw).Height(bodyH).Render(clampLines(body, bodyH))

	promo := lipgloss.PlaceHorizontal(cw, lipgloss.Center, promoStyle.Render("support independent bookstores"))
	rule := ruleStyle.Render(strings.Repeat("─", cw))

	stack := lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.PlaceHorizontal(cw, lipgloss.Center, m.nav(cw, twoCol)),
		"",
		bodyArea,
		"",
		promo,
		rule,
		m.footer(cw),
	)
	// center within height-1 so a row is always reserved at the bottom, matching
	// terminal.shop (its block never touches the last line); top-align if taller
	avail := m.height - 1
	vpos := lipgloss.Center
	if lipgloss.Height(stack) >= avail {
		vpos = lipgloss.Top
	}
	return lipgloss.Place(m.width, avail, lipgloss.Center, vpos, stack)
}

// clampLines truncates a rendered block to at most n lines so it fits the
// vertical budget (lipgloss has no vertical truncation of its own).
func clampLines(s string, n int) string {
	if n < 1 {
		n = 1
	}
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

func (m model) footer(cw int) string {
	// Hints are per screen, as terminal.shop's are: only what works here.
	var keys string
	switch {
	case m.tab == tabShop:
		keys = fk("↑/↓", "books") + fk("+/-", "qty") + fk("c", "cart") + fk("q", "quit")
	case m.tab == tabAccount:
		keys = fk("↑/↓", "navigate") + fk("q", "quit")
	case m.step == stepCart && len(m.cart) > 0:
		keys = fk("esc", "back") + fk("↑/↓", "items") + fk("+/-", "qty") + fk("enter", "checkout")
	case m.step == stepFulfil:
		keys = fk("esc", "back") + fk("↑/↓", "choose") + fk("enter", "continue")
	case m.step == stepPay:
		keys = fk("esc", "back") + fk("q", "quit")
	case m.step == stepDone, m.step == stepThanks:
		keys = fk("enter", "done")
	default:
		keys = fk("esc", "back") + fk("q", "quit")
	}
	return lipgloss.PlaceHorizontal(cw, lipgloss.Center, strings.TrimRight(keys, " "))
}

// fk renders one footer hint. terminal.shop keeps the whole footer accent-free:
// the key is bold white, the label gray.
func fk(k, label string) string {
	return active.Render(k) + " " + inactive.Render(label) + "   "
}

// truncate clips to w display cells, marking the cut with an ellipsis.
func truncate(s string, w int) string {
	if w < 1 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	// Spend display cells, not runes. A wide rune costs two, so a rune count
	// overshoots the column on CJK and emoji while cutting ASCII short. One
	// cell is held back for the ellipsis.
	var b strings.Builder
	used := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if used+rw > w-1 {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	return b.String() + "…"
}
