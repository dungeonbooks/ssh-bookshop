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
	"github.com/charmbracelet/wish/ratelimiter"
	// Aliased: the package name shadows the recover builtin.
	wrecover "github.com/charmbracelet/wish/recover"
	"github.com/muesli/termenv"
	gossh "golang.org/x/crypto/ssh"
)

func main() {
	loadDotEnv(".env")

	// -check reports what Square says about the shelf and exits. Read-only, so
	// it is safe to point at production.
	if len(os.Args) > 1 && os.Args[1] == "-check" {
		checkShelf(os.Args[2:])
		return
	}
	// -sweep deletes abandoned payment links once and exits. The server does
	// this on a timer too; this is for running it by hand.
	if len(os.Args) > 1 && os.Args[1] == "-sweep" {
		sweepShelf()
		return
	}

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
	if found, err := loadShop(catalog); err != nil {
		log.Warn("square unavailable, browsing only", "err", err, "priced", found)
	} else {
		log.Info("square ready", "env", env("SQUARE_ENVIRONMENT", "production"),
			"priced", found, "of", len(catalog), "location", sq.locationID)
	}

	// A fixed host key from the environment where storage is ephemeral; a
	// generated one on disk for local development. See hostKeyPEM for why not a
	// Railway volume.
	hostKey, err := hostKeyPEM()
	if err != nil {
		log.Fatal("bad host key", "err", err)
	}
	var hostKeyOpt ssh.Option
	switch {
	case hostKey != nil:
		hostKeyOpt = wish.WithHostKeyPEM(hostKey)
		log.Info("host key loaded from SSH_HOST_KEY")

	// True in the image and on any Railway deployment, false during `go run .`.
	// Otherwise a missing key is merely a warning, and the shop would boot
	// happily on a fresh key after every deploy, greeting each returning visitor
	// with REMOTE HOST IDENTIFICATION HAS CHANGED. That reads as a compromised
	// shop, so refuse to start instead.
	case ephemeralStorage():
		log.Fatal("SSH_HOST_KEY is required in this environment",
			"why", "storage is ephemeral, so a generated key would change on every deploy")

	default:
		hostKeyOpt = wish.WithHostKeyPath(".ssh/id_ed25519")
		log.Warn("SSH_HOST_KEY unset, using .ssh/id_ed25519",
			"note", "fine locally; on ephemeral storage this changes every deploy")
	}

	s, err := wish.NewServer(
		wish.WithAddress(net.JoinHostPort(host, port)),
		hostKeyOpt,
		// Accept any public key: anonymous browse is allowed. The key is still
		// captured per-session and becomes the account identity at checkout.
		wish.WithPublicKeyAuth(func(ssh.Context, ssh.PublicKey) bool { return true }),
		// The whole chain goes inside recover, not just the shop: a panic in any
		// of these would otherwise take the server down and every other
		// shopper's connection with it. Both wish and recover call the last
		// entry first, so this reads bottom-up: rate limit, log, require a
		// terminal, then run the shop.
		wish.WithMiddleware(
			wrecover.Middleware(
				bubbletea.Middleware(teaHandler),
				activeterm.Middleware(), // require a real interactive terminal
				logging.Middleware(),
				ratelimiter.Middleware(connectionLimiter()),
			),
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

	// Abandoned checkouts leave a live payment link behind, and Square's never
	// expire. Nothing younger than a day is touched, so a shopper still deciding
	// is safe.
	sweepCtx, stopSweep := context.WithCancel(context.Background())
	defer stopSweep()
	go sweepPeriodically(sweepCtx)

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
