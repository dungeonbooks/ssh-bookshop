package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Palette — terminal-orange on black, the terminal.shop visual language.
var (
	orange = lipgloss.Color("#FF5C00")
	white  = lipgloss.Color("#FFFFFF")
	gray   = lipgloss.Color("#8A8A8A")
	dim    = lipgloss.Color("#4A4A4A")
	green  = lipgloss.Color("#3FB950")
	blue   = lipgloss.Color("#58A6FF")
)

var (
	boxDim   = lipgloss.NewStyle().Foreground(dim)
	hotkey   = lipgloss.NewStyle().Foreground(orange).Bold(true)
	active   = lipgloss.NewStyle().Foreground(white).Bold(true)
	inactive = lipgloss.NewStyle().Foreground(gray)

	secHead = lipgloss.NewStyle().Foreground(orange)
	selItem = lipgloss.NewStyle().Foreground(orange).Bold(true)
	romItem = lipgloss.NewStyle().Foreground(gray)

	dTitle = lipgloss.NewStyle().Foreground(white).Bold(true)
	dLabel = lipgloss.NewStyle().Foreground(gray)
	dValue = lipgloss.NewStyle().Foreground(white)
	dLink  = lipgloss.NewStyle().Foreground(blue).Underline(true)
	dFree  = lipgloss.NewStyle().Foreground(green).Bold(true)
	dBody  = lipgloss.NewStyle().Foreground(lipgloss.Color("#C9C9C9"))

	promoStyle = lipgloss.NewStyle().Foreground(gray)
	ruleStyle  = lipgloss.NewStyle().Foreground(dim)
)

const (
	contentW = 86 // centered content column, like terminal.shop
	leftW    = 30 // product list column
	gapW     = 3
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
	cursor      int // index into catalog
	cart        []Book
	width       int
	height      int
	fingerprint string
}

func newModel(width, height int, fingerprint string) model {
	return model{width: width, height: height, fingerprint: fingerprint}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
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
			if m.tab == tabShop && m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.tab == tabShop && m.cursor < len(catalog)-1 {
				m.cursor++
			}
		case "+", "enter":
			if m.tab == tabShop && !catalog[m.cursor].Free {
				m.cart = append(m.cart, catalog[m.cursor])
			}
		}
	}
	return m, nil
}

func (m model) View() string {
	bodyH := m.height - 3 /*nav*/ - 4 /*promo+rule+footer+blank*/
	if bodyH < 6 {
		bodyH = 6
	}

	var left, right string
	switch m.tab {
	case tabCart:
		left, right = m.pageMenu(), m.cartView()
	case tabAccount:
		left, right = m.pageMenu(), m.accountView()
	default:
		left, right = m.productList(bodyH), m.detailView()
	}

	body := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(leftW).Render(left),
		strings.Repeat(" ", gapW),
		lipgloss.NewStyle().Width(contentW-leftW-gapW).Render(right),
	)

	promo := promoStyle.Render("every order supports independent bookstores")
	rule := ruleStyle.Render(strings.Repeat("─", contentW))

	stack := lipgloss.JoinVertical(lipgloss.Left,
		m.nav(),
		"",
		body,
		"",
		lipgloss.PlaceHorizontal(contentW, lipgloss.Center, promo),
		rule,
		m.footer(),
	)
	// center the whole content column in the terminal
	return lipgloss.NewStyle().Width(m.width).Align(lipgloss.Center).Render(stack)
}

// nav draws the boxed cell bar: logo | s shop | a account | c cart [n]
func (m model) nav() string {
	type cell struct {
		hot, label string
		on, logo   bool
	}
	cells := []cell{
		{label: "dungeonbooks", logo: true},
		{hot: "s", label: "shop", on: m.tab == tabShop},
		{hot: "a", label: "account", on: m.tab == tabAccount},
	}
	cartLabel := "cart"
	if len(m.cart) > 0 {
		cartLabel = fmt.Sprintf("cart [%d]", len(m.cart))
	}
	cells = append(cells, cell{hot: "c", label: cartLabel, on: m.tab == tabCart})

	plains := make([]string, len(cells))  // plain text for width math
	styled := make([]string, len(cells))  // colored text
	for i, c := range cells {
		var plain, col string
		if c.logo {
			plain = c.label
			col = hotkey.Render(c.label)
		} else {
			plain = c.hot + " " + c.label
			st := inactive
			if c.on {
				st = active
			}
			col = hotkey.Render(c.hot) + " " + st.Render(c.label)
		}
		plains[i] = "  " + plain + "  "
		styled[i] = "  " + col + "  "
	}

	var top, mid, bot strings.Builder
	top.WriteString("┌")
	bot.WriteString("└")
	mid.WriteString(boxDim.Render("│"))
	for i := range cells {
		w := lipgloss.Width(plains[i])
		top.WriteString(strings.Repeat("─", w))
		bot.WriteString(strings.Repeat("─", w))
		mid.WriteString(styled[i])
		mid.WriteString(boxDim.Render("│"))
		if i < len(cells)-1 {
			top.WriteString("┬")
			bot.WriteString("┴")
		}
	}
	top.WriteString("┐")
	bot.WriteString("┘")
	return lipgloss.JoinVertical(lipgloss.Left,
		boxDim.Render(top.String()),
		mid.String(),
		boxDim.Render(bot.String()),
	)
}

// productList renders the catalog grouped by collection, windowed to fit.
func (m model) productList(maxRows int) string {
	type row struct {
		text     string
		bookIdx  int // -1 for section header
	}
	rows := []row{}
	lastColl := ""
	for i, b := range catalog {
		if b.Collection != lastColl {
			rows = append(rows, row{text: secHead.Render("~ " + strings.ToLower(b.Collection) + " ~"), bookIdx: -1})
			lastColl = b.Collection
		}
		name := b.BookTitle
		if w := leftW - 2; lipgloss.Width(name) > w {
			name = name[:w-1] + "…"
		}
		if i == m.cursor {
			rows = append(rows, row{text: selItem.Render("› " + name), bookIdx: i})
		} else {
			rows = append(rows, row{text: romItem.Render("  " + name), bookIdx: i})
		}
	}

	// find the display row of the cursor and window around it
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
		sb.WriteString(r.text + "\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

func (m model) detailView() string {
	b := catalog[m.cursor]
	w := contentW - leftW - gapW
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
		fmt.Fprintln(&sb, dLink.Render(b.DownloadURL))
	} else {
		fmt.Fprintln(&sb, dLabel.Render("buy on bookshop.org:"))
		fmt.Fprintln(&sb, dLink.Render(b.BuyURL()))
	}
	return sb.String()
}

func (m model) pageMenu() string {
	rows := []struct {
		t tab
	}{{tabShop}, {tabAccount}, {tabCart}}
	var sb strings.Builder
	for _, r := range rows {
		if r.t == m.tab {
			sb.WriteString(selItem.Render("› "+r.t.String()) + "\n")
		} else {
			sb.WriteString(romItem.Render("  "+r.t.String()) + "\n")
		}
	}
	return sb.String()
}

func (m model) cartView() string {
	w := contentW - leftW - gapW
	if len(m.cart) == 0 {
		return dBody.Width(w).Render("Your cart is empty.\n\nIn the shop, press + to add the highlighted book.")
	}
	var sb strings.Builder
	fmt.Fprintln(&sb, dTitle.Render(fmt.Sprintf("cart · %d item(s)", len(m.cart))))
	fmt.Fprintln(&sb)
	for i, b := range m.cart {
		fmt.Fprintf(&sb, "%s %s\n", dLabel.Render(fmt.Sprintf("%d.", i+1)), dValue.Width(w-4).Render(b.BookTitle))
		fmt.Fprintln(&sb, "   "+dLink.Render(b.BuyURL()))
	}
	fmt.Fprintln(&sb)
	fmt.Fprintln(&sb, dBody.Width(w).Render("Open these links in a browser to check out via Bookshop.org."))
	return sb.String()
}

func (m model) accountView() string {
	w := contentW - leftW - gapW
	var sb strings.Builder
	fmt.Fprintln(&sb, dTitle.Render("account"))
	fmt.Fprintln(&sb)
	fmt.Fprintln(&sb, dBody.Width(w).Render("Your SSH public key is your identity here. No password, no signup form. The same key always maps to the same account."))
	fmt.Fprintln(&sb)
	fmt.Fprintln(&sb, dLabel.Render("key fingerprint:"))
	fmt.Fprintln(&sb, dValue.Width(w).Render(m.fingerprint))
	return sb.String()
}

func (m model) footer() string {
	var keys string
	if m.tab == tabShop {
		keys = fk("↑/↓", "products") + fk("+", "add") + fk("c", "cart") + fk("s/a", "shop/account") + fk("q", "quit")
	} else {
		keys = fk("s/a/c", "shop/account/cart") + fk("tab", "cycle") + fk("q", "quit")
	}
	return lipgloss.PlaceHorizontal(contentW, lipgloss.Left, keys)
}

func fk(k, label string) string {
	return hotkey.Render(k) + " " + inactive.Render(label) + "   "
}
