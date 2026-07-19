package main

import (
	"regexp"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

var ansi = regexp.MustCompile("\x1b\\[[0-9;]*[a-zA-Z]")

// TestRenderDemo prints the TUI at a fixed size with ANSI stripped, so the
// layout can be eyeballed without a live terminal. Run: go test -run Render -v
func TestRenderDemo(t *testing.T) {
	m := newModel(100, 28, "SHA256:bzXOMB8w3vPQ77GthA40xEWGjr3uAkphFsqfVOqxMQQ")
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 28})
	m = mm.(model)
	// move selection down to a paid title so the detail shows a buy link
	for i := 0; i < 2; i++ {
		mm, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = mm.(model)
	}
	t.Log("\n" + ansi.ReplaceAllString(m.View(), ""))
}

// TestAcctPageCount pins the constant to the real menu, since navigation bounds
// the cursor with it and a stale value would strand the last page.
func TestAcctPageCount(t *testing.T) {
	m := newModel(100, 30, "k")
	if got := len(m.acctPages(40)); got != acctPageCount {
		t.Fatalf("acctPageCount = %d, but acctPages returns %d", acctPageCount, got)
	}
}
