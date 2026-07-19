package main

// hyperlink marks the printed URL as clickable via OSC 8. The shop is on the far
// side of an SSH connection and cannot open a browser itself, so this and
// tea.SetClipboard are what make "enter" a real action. Terminals that do not
// support it show the URL unchanged, and it is always printed in full.
func hyperlink(url, text string) string {
	return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}
