package main

import (
	"time"

	"charm.land/log/v2"
	"charm.land/wish/v2"
	"github.com/charmbracelet/ssh"
)

// sessionLog replaces wish's logging middleware, which writes a line like
//
//	panat connect 74.102.100.8:57712 false [] 0 0 SSH-2.0-OpenSSH_10.0p2 Debian-7+deb13u4
//
// That is the visitor's address, the local account name their client sent, and
// a version string precise enough to name their distribution and package
// revision. journald persists it, so browsing a shelf left a record of who was
// looking and what they were running.
//
// The shop tells people it does not keep anything about them, and that has to
// be true in the logs and not only in the absence of a database. What is left
// here answers "is it up and is it rendering", which is what these lines were
// ever read for:
//
//   - key reports whether a public key was offered, which is the anonymous
//     browsing path getting used, and identifies nobody.
//   - term and size are what a rendering bug report needs.
//   - took separates a real visit from a scanner hanging up immediately.
//
// The cost is that abuse cannot be traced back to a source afterwards. That is
// acceptable because it was never how abuse gets handled here: the rate limiter
// bounds it as it happens, from buckets held in memory, and nothing about a
// bookshop justifies keeping an address log against a future incident.
func sessionLog() wish.Middleware {
	return func(next ssh.Handler) ssh.Handler {
		return func(s ssh.Session) {
			start := time.Now()
			pty, _, _ := s.Pty()

			log.Info("connect",
				"key", s.PublicKey() != nil,
				"term", pty.Term,
				"size", pty.Window.Width, "x", pty.Window.Height,
			)

			next(s)

			log.Info("disconnect", "took", time.Since(start).Round(time.Millisecond))
		}
	}
}
