package main

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Palette — the grays are terminal.shop's exact ANSI 256 indices, so the shelf
// sits in the same tonal range. The accent stays dungeonbooks orange rather
// than their teal, and is used sparingly: hotkeys, links, the focused row.
var (
	accent = lipgloss.Color("#e08339") // --accent from taranat.com
	white  = lipgloss.Color("231")     // headings, selected text
	gray   = lipgloss.Color("102")     // body / secondary text
	dim    = lipgloss.Color("59")      // borders, rules, separators
	// Text on an accent background. taranat.com calls this --accent-ink, and it
	// is dark for a reason: white on #e08339 measures 2.4:1, under the 4.5:1
	// WCAG AA wants for normal text. This is 6.4:1.
	ink = lipgloss.Color("#161616")
)

var (
	boxDim   = lipgloss.NewStyle().Foreground(dim)
	hotkey   = lipgloss.NewStyle().Foreground(white)
	active   = lipgloss.NewStyle().Foreground(white).Bold(true) // wordmark, nav, hotkeys
	inactive = lipgloss.NewStyle().Foreground(gray)

	secHead = lipgloss.NewStyle().Foreground(white)
	// selItem is the single focused element: an accent block, like
	// terminal.shop's highlighted row. Dark text on it, not bright: see ink.
	selItem = lipgloss.NewStyle().Background(accent).Foreground(ink)
	romItem = lipgloss.NewStyle().Foreground(gray)
	navSep  = lipgloss.NewStyle().Foreground(dim)

	logoStyle = lipgloss.NewStyle().Foreground(accent).Bold(true)

	dTitle = lipgloss.NewStyle().Foreground(white)
	dLabel = lipgloss.NewStyle().Foreground(gray)
	dValue = lipgloss.NewStyle().Foreground(white)
	dLink  = lipgloss.NewStyle().Foreground(accent).Underline(true)
	dMonth = lipgloss.NewStyle().Foreground(accent).Bold(true)
	dBody  = lipgloss.NewStyle().Foreground(gray)

	promoStyle = lipgloss.NewStyle().Foreground(gray)
	ruleStyle  = lipgloss.NewStyle().Foreground(dim)
)

// Layout constants measured from terminal.shop.
const (
	maxContent    = 78 // wide content width, centered (terminal.shop)
	narrowContent = 48 // single-column content width
	twoColMin     = 80 // two columns at/above this terminal width
	leftCol       = 20 // product/menu column in two-column mode
	bodyMax       = 21 // body area height when the window has room to spare
	chromeH       = 8  // everything but the body: nav(3), 2 blanks, promo, rule, footer
	// Rows payView spends around the code on a screen that is showing one:
	// breadcrumb, blank, blank after the code, caption, url, status. Only valid
	// as the companion to a rendered QR, so it is what len(qr) is added to when
	// deciding whether the code fits. A screen that falls back to the bare link
	// does not spend these.
	qrChrome = 6
)

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

// sessionInfo is what the debug info page reports back, straight from the SSH
// session rather than guessed.
type sessionInfo struct {
	mode    string
	term    string
	user    string
	command string
}

type model struct {
	tab         tab
	cursor      int // index into catalog (shop)
	acct        int // index into account sub-pages
	cart        []cartLine
	placed      []cartLine // what was bought, kept for the receipt screens
	cartCursor  int
	step        step
	checkout    checkout
	checkoutErr error
	width       int
	height      int
	ready       bool // false while the splash shows
	cursorOn    bool // splash cursor visibility
	phase       int  // splash blink phases elapsed
	copied      bool // the current book's link was just copied
	fingerprint string
	sess        sessionInfo
	// fresh is price and stock re-read from Square during this session's
	// checkout. Kept here rather than written back to the package catalog,
	// which every other session is reading concurrently.
	fresh map[string]freshItem
}

func newModel(width, height int, fingerprint string) model {
	m := model{width: width, height: height, fingerprint: fingerprint, cursorOn: true}
	// Open on this month's pick — the thing someone connects to see.
	if i := featured(time.Now()); i >= 0 {
		m.cursor = i
	}
	return m
}

// book returns catalog entry i with anything this session re-read from Square
// laid over it.
func (m model) book(i int) Book {
	b := catalog[i]
	if f, ok := m.fresh[b.VariationID]; ok {
		b.Cents, b.Stock = f.cents, f.stock
		b.Tracked, b.Sellable = !f.untracked, f.sellable
	}
	return b
}

type blinkMsg struct{}

// copiedMsg clears the "link copied" confirmation so the tip returns to telling
// you what enter does.
type copiedMsg struct{}

const copiedFor = 2 * time.Second

// blinkPeriod is one on or off phase, so two full blinks take 4 of them.
const (
	blinkPeriod = 600 * time.Millisecond
	blinkPhases = 4
)

func blinkTick() tea.Cmd {
	return tea.Tick(blinkPeriod, func(time.Time) tea.Msg { return blinkMsg{} })
}

func (m model) Init() tea.Cmd {
	// wordmark splash on connect: the cursor blinks twice, then the shop loads
	return blinkTick()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case copiedMsg:
		m.copied = false
		return m, nil

	case blinkMsg:
		m.cursorOn = !m.cursorOn
		if !m.ready {
			m.phase++
			if m.phase >= blinkPhases {
				m.ready = true
				return m, nil
			}
			return m, blinkTick()
		}
		// Keep blinking only while something is actually pending, so an idle
		// shop isn't redrawing itself forever.
		if m.tab == tabCart && m.step == stepPay {
			return m, blinkTick()
		}
		return m, nil

	case tea.KeyMsg:
		// any key skips the splash
		if !m.ready {
			m.ready = true
			if msg.String() == "ctrl+c" || msg.String() == "q" {
				return m, tea.Quit
			}
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "tab":
			m.tab = (m.tab + 1) % 3
		case "shift+tab":
			m.tab = (m.tab + 2) % 3
		case "s":
			m.tab = tabShop
		case "a":
			m.tab = tabAccount
		case "c":
			m.tab = tabCart
		case "esc":
			// Back out of checkout one step at a time; from the cart list, out
			// to the shop.
			switch {
			case m.tab == tabCart && m.step != stepCart:
				m.step = stepCart
				m.checkoutErr = nil
			case m.tab == tabCart:
				m.tab = tabShop
			}
		case "up", "k":
			switch m.tab {
			case tabShop:
				if m.cursor > 0 {
					m.cursor--
					m.copied = false
				}
			case tabAccount:
				if m.acct > 0 {
					m.acct--
				}
			case tabCart:
				if m.step == stepCart && m.cartCursor > 0 {
					m.cartCursor--
				}
			}
		case "down", "j":
			switch m.tab {
			case tabShop:
				if m.cursor < len(catalog)-1 {
					m.cursor++
					m.copied = false
				}
			case tabAccount:
				if m.acct < acctPageCount-1 {
					m.acct++
				}
			case tabCart:
				if m.step == stepCart && m.cartCursor < len(m.cart)-1 {
					m.cartCursor++
				}
			}
		case "+", "=":
			switch m.tab {
			case tabShop:
				m.addToCart(m.cursor)
			case tabCart:
				if m.step == stepCart && m.cartCursor < len(m.cart) {
					m.addToCart(m.cart[m.cartCursor].idx)
				}
			}
		case "-", "_":
			switch m.tab {
			case tabShop:
				m.removeFromCart(m.cursor)
			case tabCart:
				if m.step == stepCart && m.cartCursor < len(m.cart) {
					m.removeFromCart(m.cart[m.cartCursor].idx)
				}
			}
		case "enter":
			switch {
			case m.tab == tabShop:
				// A server can't open a browser on someone else's machine, so
				// "open the link" means putting it on their clipboard.
				m.copied = true
				return m, tea.Batch(
					tea.SetClipboard(m.book(m.cursor).BuyURL()),
					tea.Tick(copiedFor, func(time.Time) tea.Msg { return copiedMsg{} }),
				)
			case m.tab == tabCart && m.step == stepCart && len(m.cart) > 0:
				m.step = stepPay
				m.checkoutErr = nil
				m.checkout = checkout{}
				return m, tea.Batch(startCheckout(m.snapshotCart(), newIdempotencyKey()), blinkTick())
			case m.tab == tabCart && m.step == stepDone:
				m.step = stepThanks
			case m.tab == tabCart && m.step == stepThanks:
				// Order's done: forget it and go back to the shelf.
				m.placed, m.cart, m.cartCursor, m.step = nil, nil, 0, stepCart
				m.checkout = checkout{}
				m.tab = tabShop
			}
		}

	case checkoutMsg:
		// Keep whatever Square just told us, so the shelf stops lying and a
		// retry has a chance of succeeding. Merged rather than replaced: an
		// earlier checkout's prices stay correct.
		if len(msg.fresh) > 0 {
			if m.fresh == nil {
				m.fresh = make(map[string]freshItem, len(msg.fresh))
			}
			for k, v := range msg.fresh {
				m.fresh[k] = v
			}
		}
		if msg.err != nil {
			m.checkoutErr = msg.err
			return m, nil
		}
		m.checkout = msg.out
		return m, pollPaid(msg.out.OrderID)

	case paidMsg:
		// Confirmation first, wherever they happen to be: the money has moved,
		// so an in-flight poll landing after they backed out still completes the
		// order rather than stranding a paid customer on the cart.
		if msg.paid {
			// Empty the cart the moment Square confirms, so the nav total goes
			// to zero on the order screen rather than lingering behind a letter
			// nobody has dismissed yet. The lines are kept for the receipt.
			m.placed, m.cart, m.cartCursor = m.cart, nil, 0
			m.step = stepDone
			return m, nil
		}
		// Otherwise stop once they have left checkout or the order is gone.
		// Without this the error path re-polls forever, and after esc then enter
		// clears m.checkout it re-polls an empty order id.
		if m.step != stepPay || m.checkout.OrderID == "" {
			return m, nil
		}
		// A failed poll is not a failed order. Keep waiting rather than telling
		// someone their payment did not go through.
		return m, pollPaid(m.checkout.OrderID)
	}
	return m, nil
}

// dims derives the responsive layout from the window size, matching
// terminal.shop: content is 78 cols (two columns) at width >= 80, otherwise 48
// (single column); both centered. bodyH is the fixed body-area height that pins
// the footer, shrinking only when the window is too short.
func (m model) dims() (cw, rightW, bodyH int, twoCol bool) {
	twoCol = m.width >= twoColMin
	if twoCol {
		cw = maxContent
	} else {
		cw = narrowContent
	}
	if cw > m.width-2 {
		cw = m.width - 2
	}
	if cw < 16 {
		cw = 16
	}
	if twoCol {
		rightW = cw - leftCol
	} else {
		rightW = cw
	}
	bodyH = bodyMax
	// reserve one blank row at the bottom so the footer never sits on the last
	// line when compressed (terminal.shop caps its block at height-1)
	if avail := m.height - chromeH - 1; bodyH > avail {
		bodyH = avail
	}
	if bodyH < 4 {
		bodyH = 4
	}
	return
}

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
			left, right = m.accountMenu(pages), pages[m.acct].body
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
			top, bottom = m.accountMenu(pages), pages[m.acct].body
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

	// widths: each cell gets its label + min padding, then leftover spread evenly
	n := len(cells)
	widths := make([]int, n)
	used := n + 1
	for i, c := range cells {
		widths[i] = lipgloss.Width(plain(c)) + 2
		used += widths[i]
	}
	for i := 0; used < cw; i = (i + 1) % n {
		widths[i]++
		used++
	}

	// In two-column mode, size the logo cell so the divider after it lands on the
	// detail column edge (leftCol): divider sits at col 1+widths[0], align to leftCol.
	if twoCol && n > 1 && cells[0].logo {
		delta := widths[0] - (leftCol - 1) // columns to move off the logo cell
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
		name := b.BookTitle
		if lipgloss.Width(name) > maxw {
			name = name[:maxw-1] + "…"
		}
		if i == m.cursor {
			// Width inside the style so the highlight spans the whole column,
			// as terminal.shop's does, rather than hugging the text.
			rows = append(rows, row{text: selItem.Width(leftCol - 1).Render(" " + name), bookIdx: i})
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

// breadcrumb is the checkout step indicator: only the current step is bright,
// and the whole thing disappears once the order is placed.
func (m model) breadcrumb() string {
	steps := []struct {
		label string
		at    step
	}{{"cart", stepCart}, {"checkout", stepPay}}
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
	fmt.Fprintln(&sb, para.Render("Your books are set aside at the shop in Jersey City. Square has emailed you a receipt, and we'll be in touch about pickup or shipping."))
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

func (m model) accountMenu(pages []acctPage) string {
	var sb strings.Builder
	for i, p := range pages {
		if i == m.acct {
			sb.WriteString(selItem.Width(leftCol - 1).Render(" " + p.title))
		} else {
			sb.WriteString(romItem.Render(" " + p.title))
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
	r := []rune(s)
	if len(r) > w-1 {
		r = r[:w-1]
	}
	return string(r) + "…"
}
