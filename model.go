package main

import (
	"time"

	tea "charm.land/bubbletea/v2"
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
	fulfil      fulfilment // pickup or ship, chosen before the link is made
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
