package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Palette — mostly grayscale (terminal.shop's restraint) with the dungeonbooks
// orange used sparingly: hotkeys, links, and the one focused element.
var (
	accent = lipgloss.Color("#FF5C00")
	white  = lipgloss.Color("#EEEEEE")
	gray   = lipgloss.Color("#8A8A8A") // body / secondary text
	dim    = lipgloss.Color("#5F5F5F") // borders, rules, separators
	black  = lipgloss.Color("#0B0B0B")
)

var (
	boxDim   = lipgloss.NewStyle().Foreground(dim)
	hotkey   = lipgloss.NewStyle().Foreground(accent).Bold(true)
	active   = lipgloss.NewStyle().Foreground(white).Bold(true)
	inactive = lipgloss.NewStyle().Foreground(gray)

	secHead = lipgloss.NewStyle().Foreground(dim)
	// selItem is the single focused element: an accent block, like terminal.shop.
	selItem = lipgloss.NewStyle().Background(accent).Foreground(black).Bold(true)
	romItem = lipgloss.NewStyle().Foreground(gray)
	navSep  = lipgloss.NewStyle().Foreground(dim)

	logoStyle = lipgloss.NewStyle().Foreground(accent).Bold(true)

	dTitle = lipgloss.NewStyle().Foreground(white).Bold(true)
	dLabel = lipgloss.NewStyle().Foreground(gray)
	dValue = lipgloss.NewStyle().Foreground(white)
	dLink  = lipgloss.NewStyle().Foreground(accent).Underline(true)
	dFree  = lipgloss.NewStyle().Foreground(accent).Bold(true)
	dBody  = lipgloss.NewStyle().Foreground(lipgloss.Color("#B0B0B0"))

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

type model struct {
	tab         tab
	cursor      int // index into catalog (shop)
	acct        int // index into account sub-pages
	cart        []Book
	width       int
	height      int
	ready       bool // false while the splash shows
	fingerprint string
}

func newModel(width, height int, fingerprint string) model {
	return model{width: width, height: height, fingerprint: fingerprint}
}

type readyMsg struct{}

func (m model) Init() tea.Cmd {
	// brief wordmark splash on connect, like terminal.shop
	return tea.Tick(700*time.Millisecond, func(time.Time) tea.Msg { return readyMsg{} })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case readyMsg:
		m.ready = true
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
		case "up", "k":
			switch m.tab {
			case tabShop:
				if m.cursor > 0 {
					m.cursor--
				}
			case tabAccount:
				if m.acct > 0 {
					m.acct--
				}
			}
		case "down", "j":
			switch m.tab {
			case tabShop:
				if m.cursor < len(catalog)-1 {
					m.cursor++
				}
			case tabAccount:
				if m.acct < acctPageCount-1 {
					m.acct++
				}
			}
		case "+", "enter":
			if m.tab == tabShop && !catalog[m.cursor].Free {
				m.cart = append(m.cart, catalog[m.cursor])
			}
		}
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

func (m model) View() string {
	if !m.ready {
		splash := lipgloss.JoinVertical(lipgloss.Center,
			logoStyle.Render("dungeonbooks"),
			"",
			promoStyle.Render("a bookstore over ssh"),
		)
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, splash)
	}

	cw, rightW, bodyH, twoCol := m.dims()

	// Build the body content, clamped to the fixed body-area height (bodyH).
	var body string
	switch {
	case m.tab == tabCart:
		body = clampLines(m.cartView(cw), bodyH)

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

	promo := lipgloss.PlaceHorizontal(cw, lipgloss.Center, promoStyle.Render("every order supports independent bookstores"))
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
	cartLabel := fmt.Sprintf("cart [%d]", len(m.cart))
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
	// top bar is all white (no accent); inactive labels are gray
	styled := func(c cell) string {
		if c.logo {
			return active.Render(c.label)
		}
		st := inactive
		if c.on {
			st = active
		}
		return active.Render(c.hot) + " " + st.Render(c.label)
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
	cartLabel := "cart"
	if len(m.cart) > 0 {
		cartLabel = fmt.Sprintf("cart [%d]", len(m.cart))
	}
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
		if b.Collection != lastColl {
			rows = append(rows, row{text: " " + secHead.Render("~ "+strings.ToLower(b.Collection)+" ~"), bookIdx: -1})
			lastColl = b.Collection
		}
		name := b.BookTitle
		if lipgloss.Width(name) > maxw {
			name = name[:maxw-1] + "…"
		}
		if i == m.cursor {
			rows = append(rows, row{text: selItem.Render(" " + name), bookIdx: i})
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
	b := catalog[m.cursor]
	var sb strings.Builder
	fmt.Fprintln(&sb, dTitle.Width(w).Render(b.BookTitle))
	fmt.Fprintln(&sb)
	fmt.Fprintf(&sb, "%s %s\n", dLabel.Render("author     "), dValue.Render(b.Author))
	fmt.Fprintf(&sb, "%s %s, %d\n", dLabel.Render("publisher  "), dValue.Render(b.Publisher), b.Year)
	fmt.Fprintf(&sb, "%s %s\n", dLabel.Render("collection "), secHead.Render(b.Collection))
	if b.Free {
		fmt.Fprintf(&sb, "%s %s\n", dLabel.Render("price      "), dFree.Render("FREE · openly licensed"))
	} else {
		fmt.Fprintf(&sb, "%s %s\n", dLabel.Render("isbn       "), dValue.Render(b.ISBN))
	}
	fmt.Fprintln(&sb)
	fmt.Fprintln(&sb, dBody.Width(w).Render(b.Blurb))
	fmt.Fprintln(&sb)
	if b.Free {
		fmt.Fprintln(&sb, dLabel.Render("read free:"))
		fmt.Fprint(&sb, dLink.Render(b.DownloadURL))
	} else {
		fmt.Fprintln(&sb, dLabel.Render("buy on bookshop.org:"))
		fmt.Fprint(&sb, dLink.Render(b.BuyURL()))
	}
	return sb.String()
}

func (m model) cartView(w int) string {
	if len(m.cart) == 0 {
		return dBody.Width(w).Render("Your cart is empty. In the shop, press + to add the highlighted book.")
	}
	var sb strings.Builder
	fmt.Fprintln(&sb, dTitle.Render(fmt.Sprintf("cart · %d item(s)", len(m.cart))))
	fmt.Fprintln(&sb)
	for i, b := range m.cart {
		fmt.Fprintf(&sb, "%s %s\n", dLabel.Render(fmt.Sprintf("%d.", i+1)), dValue.Width(w-4).Render(b.BookTitle))
		fmt.Fprintf(&sb, "   %s\n", dLink.Render(b.BuyURL()))
	}
	fmt.Fprintln(&sb)
	fmt.Fprint(&sb, dBody.Width(w).Render("Open these links in a browser to check out via Bookshop.org."))
	return sb.String()
}

type acctPage struct {
	title string
	body  string
}

// acctPages builds the account sub-pages. Everything here is real for this app.
// acctPageCount is the menu length. Kept alongside acctPages so navigation can
// bound the cursor without rendering every page body just to count them.
const acctPageCount = 3

func (m model) acctPages(w int) []acctPage {
	return []acctPage{
		{"faq", m.pgFAQ(w)},
		{"order history", m.pgOrders(w)},
		{"about", m.pgAbout(w)},
	}
}

func (m model) accountMenu(pages []acctPage) string {
	var sb strings.Builder
	for i, p := range pages {
		if i == m.acct {
			sb.WriteString(selItem.Render("› " + p.title))
		} else {
			sb.WriteString(romItem.Render("  " + p.title))
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

func (m model) pgFAQ(w int) string {
	var sb strings.Builder
	fmt.Fprintln(&sb, dTitle.Render("faq"))
	fmt.Fprintln(&sb)
	fmt.Fprintln(&sb, dLabel.Render("how do i buy a book?"))
	fmt.Fprintln(&sb, dBody.Width(w).Render("\"buy\" opens a Bookshop.org link. Bookshop supports independent bookstores; dungeonbooks earns a small affiliate commission. No inventory, no card details here."))
	fmt.Fprintln(&sb)
	fmt.Fprintln(&sb, dLabel.Render("what are the free titles?"))
	fmt.Fprintln(&sb, dBody.Width(w).Render("Some books are openly licensed and link straight to the full text."))
	fmt.Fprintln(&sb)
	fmt.Fprintln(&sb, dLabel.Render("why ssh?"))
	fmt.Fprint(&sb, dBody.Width(w).Render("A bookstore you browse from any terminal. Your SSH key is your account, so there is no password or signup."))
	return sb.String()
}

func (m model) pgOrders(w int) string {
	var sb strings.Builder
	fmt.Fprintln(&sb, dTitle.Render("order history"))
	fmt.Fprintln(&sb)
	fmt.Fprintln(&sb, dBody.Width(w).Render("Orders are placed on Bookshop.org, not here. dungeonbooks earns a small commission on each sale and never sees your cart or card."))
	fmt.Fprintln(&sb)
	fmt.Fprint(&sb, romItem.Render("no orders on file"))
	return sb.String()
}

func (m model) pgAbout(w int) string {
	var sb strings.Builder
	fmt.Fprintln(&sb, dTitle.Render("about"))
	fmt.Fprintln(&sb)
	fmt.Fprintln(&sb, dBody.Width(w).Render("dungeonbooks is an independent science fiction, fantasy, and RPG bookstore in Jersey City, NJ. This is the same shop, browsable over SSH."))
	fmt.Fprintln(&sb)
	fmt.Fprintln(&sb, dBody.Width(w).Render("Built with Wish and Bubble Tea."))
	fmt.Fprint(&sb, dLink.Render("github.com/dungeonbooks/ssh-bookshop"))
	return sb.String()
}

func (m model) footer(cw int) string {
	var keys string
	switch m.tab {
	case tabShop:
		keys = fk("↑/↓", "products") + fk("enter", "add") + fk("c", "cart") + fk("q", "quit")
	case tabAccount:
		keys = fk("↑/↓", "navigate") + fk("q", "quit")
	default:
		keys = fk("q", "quit")
	}
	return lipgloss.PlaceHorizontal(cw, lipgloss.Center, strings.TrimRight(keys, " "))
}

func fk(k, label string) string {
	return hotkey.Render(k) + " " + inactive.Render(label) + "   "
}
