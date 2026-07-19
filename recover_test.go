package main

import (
	"testing"

	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	wrecover "github.com/charmbracelet/wish/recover"
)

// TestRecoverCoversWholeChain pins the reason every middleware is passed to
// recover rather than just the shop handler. recover only guards the
// middlewares handed to it and calls the rest of the chain outside that guard,
// so wrapping bubbletea alone left a panic in the rate limiter or logger free to
// kill the server and every other session with it.
func TestRecoverCoversWholeChain(t *testing.T) {
	boom := func(wish.Middleware) wish.Middleware {
		return func(ssh.Handler) ssh.Handler {
			return func(ssh.Session) { panic("boom") }
		}
	}
	for _, position := range []string{"first", "last"} {
		t.Run("panic in the "+position+" middleware", func(t *testing.T) {
			calm := func(next ssh.Handler) ssh.Handler { return next }
			var mw wish.Middleware
			if position == "first" {
				mw = wrecover.Middleware(boom(nil), calm)
			} else {
				mw = wrecover.Middleware(calm, boom(nil))
			}
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panic escaped recover: %v", r)
				}
			}()
			// The session is never touched before the panic, so nil is fine.
			mw(func(ssh.Session) {})(nil)
		})
	}
}

// TestRecoverGuardIsScopedToWhatItWraps is the other half, and the reason the
// previous arrangement was wrong: recover guards only the middlewares handed to
// it, then calls the rest of the chain outside that guard. Anything not passed
// in is unprotected.
func TestRecoverGuardIsScopedToWhatItWraps(t *testing.T) {
	calm := func(next ssh.Handler) ssh.Handler { return next }
	mw := wrecover.Middleware(calm) // guards calm, and nothing else

	escaped := func() (escaped bool) {
		defer func() { escaped = recover() != nil }()
		// Stands in for the rest of the chain: recover calls this outside its
		// own guard, so a panic here is not caught.
		mw(func(ssh.Session) { panic("boom") })(nil)
		return
	}()

	if !escaped {
		t.Error("panic outside the wrapped set was caught; the scoping this guards against has changed")
	}
}
