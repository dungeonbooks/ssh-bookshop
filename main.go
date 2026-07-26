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

	tea "charm.land/bubbletea/v2"
	"charm.land/log/v2"
	"charm.land/wish/v2"
	"charm.land/wish/v2/activeterm"
	"charm.land/wish/v2/bubbletea"
	"charm.land/wish/v2/logging"
	"charm.land/wish/v2/ratelimiter"
	"github.com/charmbracelet/ssh"
	// Aliased: the package name shadows the recover builtin.
	wrecover "charm.land/wish/v2/recover"
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
		// Accept any public key. Nothing is checked against it: it gives the
		// rate limiter a per-visitor bucket and the account page something to
		// show. Checkout never looks at it, because a Square payment link
		// collects whatever it needs on Square's side.
		wish.WithPublicKeyAuth(func(ssh.Context, ssh.PublicKey) bool { return true }),
		// Let someone in who offers no key at all. Publickey alone turns a
		// keyless client away at the door with "Permission denied (publickey)",
		// which is a poor greeting for a shop that does not need to know who
		// you are to show you a shelf. It also asks visitors to hand over a
		// public key before they have any reason to trust us, and public keys
		// are correlatable: GitHub publishes everyone's.
		//
		// The handler prompts for nothing and always succeeds, so a keyless
		// client falls through to it and browses as anonymous. teaHandler
		// already treats a nil public key that way, and since checkout does not
		// consult the key either, an anonymous visitor can buy as well as
		// browse. The rate limiter falls back to keying on the address, which
		// is the one thing that changes for them.
		wish.WithKeyboardInteractiveAuth(
			func(ssh.Context, gossh.KeyboardInteractiveChallenge) bool { return true },
		),
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
	// Deliberately not logging the fingerprint. Nothing here needs it: the rate
	// limiter keeps its own buckets in memory, there are no orders to trace
	// back, and no support workflow that identifies a returning visitor. A
	// public key is correlatable against GitHub, so writing one to the journal
	// turns a browse into a retained record of who was looking. mode is the
	// part that is actually useful and identifies nobody.
	log.Info("session", "user", s.User(), "mode", mode)

	m := newModel(pty.Window.Width, pty.Window.Height, fp)
	m.sess = sessionInfo{
		mode:    mode,
		term:    pty.Term,
		user:    s.User(),
		command: strings.Join(s.Command(), " "),
	}
	return m, nil
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
