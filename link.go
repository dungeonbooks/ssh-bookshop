package main

import "encoding/base64"

// The shop runs on the far side of an SSH connection, so it cannot launch a
// browser for the person using it. Two terminal escape sequences close the gap:
//
//	OSC 52 writes the URL to their clipboard, so enter is a real action.
//	OSC 8 marks the printed URL as a hyperlink, so it is click-to-open.
//
// Support varies by terminal. Both degrade to doing nothing visible, and the
// URL is printed in full either way, so nobody is left without a way to buy.

// osc52 writes s to the client's clipboard. Emitted from the view while the
// copied state is showing: rewriting the same value each frame is harmless, and
// bubbletea v1 has no clipboard command to hang this off instead.
func osc52(s string) string {
	return "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(s)) + "\x07"
}

// hyperlink wraps text in OSC 8 so terminals that support it make it clickable.
func hyperlink(url, text string) string {
	return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}
