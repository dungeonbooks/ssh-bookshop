package main

import "charm.land/lipgloss/v2"

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
