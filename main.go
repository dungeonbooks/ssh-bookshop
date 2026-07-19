package main

import (
	"context"
	"errors"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/activeterm"
	"github.com/charmbracelet/wish/bubbletea"
	"github.com/charmbracelet/wish/logging"
	"github.com/muesli/termenv"
	gossh "golang.org/x/crypto/ssh"
)

func main() {
	host := env("HOST", "0.0.0.0")
	port := env("PORT", "23234")

	// Styles are rendered by lipgloss's default renderer, which probes this
	// process's stdout — a log file or pipe under a service manager, never a
	// TTY. Left to auto-detect it picks the no-color profile and strips every
	// style from output that is in fact going to a real terminal. activeterm
	// already refuses non-interactive sessions, so force color on.
	lipgloss.SetColorProfile(termenv.TrueColor)

	// Prices come from the shop's Square catalog. A failure here is survivable:
	// the shelf still opens, just without prices.
	if found, err := loadPrices(catalog); err != nil {
		log.Warn("square prices unavailable", "err", err, "priced", found)
	} else {
		log.Info("square prices loaded", "priced", found, "of", len(catalog))
	}

	s, err := wish.NewServer(
		wish.WithAddress(net.JoinHostPort(host, port)),
		wish.WithHostKeyPath(".ssh/id_ed25519"),
		// Accept any public key: anonymous browse is allowed. The key is still
		// captured per-session and becomes the account identity at checkout.
		wish.WithPublicKeyAuth(func(ssh.Context, ssh.PublicKey) bool { return true }),
		wish.WithMiddleware(
			bubbletea.Middleware(teaHandler),
			activeterm.Middleware(), // require a real interactive terminal
			logging.Middleware(),
		),
	)
	if err != nil {
		log.Fatal("could not create server", "err", err)
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	log.Info("starting ssh bookshop", "addr", net.JoinHostPort(host, port))
	go func() {
		if err := s.ListenAndServe(); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
			log.Error("server error", "err", err)
			done <- nil
		}
	}()

	<-done
	log.Info("stopping")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
		log.Error("shutdown error", "err", err)
	}
}

func teaHandler(s ssh.Session) (tea.Model, []tea.ProgramOption) {
	pty, _, _ := s.Pty()

	fp := "anonymous"
	mode := "anonymous"
	if pk := s.PublicKey(); pk != nil {
		fp = gossh.FingerprintSHA256(pk)
		mode = "ssh key"
	}
	log.Info("session", "user", s.User(), "fingerprint", fp)

	m := newModel(pty.Window.Width, pty.Window.Height, fp)
	m.sess = sessionInfo{
		mode:    mode,
		term:    pty.Term,
		user:    s.User(),
		command: strings.Join(s.Command(), " "),
	}
	return m, []tea.ProgramOption{tea.WithAltScreen()}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
