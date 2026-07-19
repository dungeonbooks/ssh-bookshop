package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	hotkey   = lipgloss.NewStyle().Foreground(accent).Bold(true)
	active   = lipgloss.NewStyle().Foreground(white).Bold(true)
	inactive = lipgloss.NewStyle().Foreground(gray)

	secHead = lipgloss.NewStyle().Foreground(white)
	// selItem is the single focused element: an accent block, like terminal.shop.
	// Dark text on it, not bright: see ink.
	selItem = lipgloss.NewStyle().Background(accent).Foreground(ink)
	romItem = lipgloss.NewStyle().Foreground(gray)
	navSep  = lipgloss.NewStyle().Foreground(dim)

	logoStyle = lipgloss.NewStyle().Foreground(accent).Bold(true)

	dTitle = lipgloss.NewStyle().Foreground(white).Bold(true)
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
	cart        []Book
	width       int
	height      int
	ready       bool // false while the splash shows
	cursorOn    bool // splash cursor visibility
	phase       int  // splash blink phases elapsed
	copied      bool // the current book's link was just copied
	fingerprint string
	sess        sessionInfo
}

func newModel(width, height int, fingerprint string) model {
	m := model{width: width, height: height, fingerprint: fingerprint, cursorOn: true}
	// Open on this month's pick — the thing someone connects to see.
	if i := featured(time.Now()); i >= 0 {
		m.cursor = i
	}
	return m
}

type blinkMsg struct{}

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

	case blinkMsg:
		m.phase++
		m.cursorOn = !m.cursorOn
		if m.phase >= blinkPhases {
			m.ready = true
			return m, nil
		}
		return m, blinkTick()

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
					m.copied = false
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
					m.copied = false
				}
			case tabAccount:
				if m.acct < acctPageCount-1 {
					m.acct++
				}
			}
		case "enter":
			// A server can't open a browser on someone else's machine, so
			// "open the link" means putting it on their clipboard (OSC 52)
			// and leaving it clickable (OSC 8) where that is supported.
			if m.tab == tabShop {
				m.copied = true
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
	out := lipgloss.Place(m.width, avail, lipgloss.Center, vpos, stack)
	if m.copied && m.tab == tabShop {
		out = osc52(catalog[m.cursor].BuyURL()) + out
	}
	return out
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
	b := catalog[m.cursor]
	var sb strings.Builder
	// terminal.shop's detail shape: name, attributes on one pipe-joined line,
	// then the single number that matters, then the description.
	fmt.Fprintln(&sb, dTitle.Width(w).Render(b.BookTitle))

	fmt.Fprintln(&sb, dLabel.Width(w).Render(strings.Join(b.attrs(), " | ")))
	fmt.Fprintln(&sb)

	// The price, where terminal.shop puts it.
	if p := b.Price(); p > 0 {
		fmt.Fprintln(&sb, dMonth.Render(usd(p)))
	} else {
		fmt.Fprintln(&sb, dLabel.Render("price unavailable"))
	}
	fmt.Fprintln(&sb)
	fmt.Fprintln(&sb, dBody.Width(w).Render(b.Blurb))
	fmt.Fprintln(&sb)
	fmt.Fprint(&sb, m.action(w, b))
	return sb.String()
}

// action is the boxed call-to-action, like terminal.shop's "subscribe  enter".
func (m model) action(w int, b Book) string {
	label := "buy at dungeonbooks.com"
	if !strings.Contains(b.BuyURL(), "dungeonbooks.com") {
		label = "buy at bookshop.org"
	}
	if m.copied {
		label = "link copied to clipboard"
	}
	inner := w - 4
	if inner < len(label)+8 {
		inner = len(label) + 8
	}
	gap := inner - lipgloss.Width(label) - lipgloss.Width("enter")
	if gap < 1 {
		gap = 1
	}
	row := " " + dValue.Render(label) + strings.Repeat(" ", gap) + dLabel.Render("enter") + " "
	return boxDim.Render("┌"+strings.Repeat("─", inner+2)+"┐") + "\n" +
		boxDim.Render("│") + row + boxDim.Render("│") + "\n" +
		boxDim.Render("└"+strings.Repeat("─", inner+2)+"┘")
}

func (m model) cartView(w int) string {
	if len(m.cart) == 0 {
		return lipgloss.PlaceHorizontal(w, lipgloss.Center, romItem.Render("your cart is empty"))
	}
	var sb strings.Builder
	fmt.Fprintln(&sb, dTitle.Render(fmt.Sprintf("cart · %d item(s)", len(m.cart))))
	fmt.Fprintln(&sb)
	for i, b := range m.cart {
		fmt.Fprintf(&sb, "%s %s\n", dLabel.Render(fmt.Sprintf("%d.", i+1)), dValue.Width(w-4).Render(b.BookTitle))
		fmt.Fprintf(&sb, "   %s\n", hyperlink(b.BuyURL(), dLink.Render(b.BuyURL())))
	}
	fmt.Fprintln(&sb)
	fmt.Fprint(&sb, dBody.Width(w).Render("open these links in a browser to check out."))
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

// pgOrders is the empty state until checkout exists. terminal.shop centers the
// same message in the page rather than explaining itself.
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

// fk renders one footer hint. terminal.shop keeps the whole footer accent-free:
// the key is bold white, the label gray.
func fk(k, label string) string {
	return active.Render(k) + " " + inactive.Render(label) + "   "
}
