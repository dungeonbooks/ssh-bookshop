package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"charm.land/wish/v2"
	"github.com/charmbracelet/ssh"

	"github.com/dungeonbooks/ssh-bookshop/internal/shopapi"
)

// Command mode is the API over SSH: `ssh shop.dungeonbooks.com books` prints
// the shelf as JSON and exits, no terminal required. Same shelf, same
// checkout, same shapes as the HTTP API; only the transport differs. It exists
// because the SSH key is a free identity and the hostname is already the
// product, not because agents should prefer it: most sandboxes have no SSH
// client, so the HTTP API is the path that has to work.
//
// Exit codes are the protocol, the same as the CLI's: 0 done, 1 something
// failed, 2 the command was malformed or the shop turned the request down, 3
// the order is still waiting for payment.
const (
	exitOK      = 0
	exitError   = 1
	exitUsage   = 2
	exitWaiting = 3
)

const commandUsage = `usage:
  books                      the shelf
  books <isbn>               one book
  buy <isbn>[:qty]... --pickup|--ship [--key <idempotency-key>]
                             build a checkout; prints the URL for a human to pay
  order <order_id>           exit 0 once paid, 3 while waiting
  cancel <checkout_id>       abandon an unpaid checkout

Everything is JSON on stdout, errors are JSON on stderr.
The HTTP API and the dungeon CLI do the same things: https://shop.dungeonbooks.com/llms.txt
`

// commandMode answers a session that arrived with a command and passes the
// rest on to the terminal check and the shop.
func commandMode(s *apiShop) wish.Middleware {
	return func(next ssh.Handler) ssh.Handler {
		return func(sess ssh.Session) {
			args := sess.Command()
			if len(args) == 0 {
				next(sess)
				return
			}
			// A deadline, as the HTTP side has, so a stalled Square call cannot
			// hold a session open for good.
			ctx, cancel := context.WithTimeout(sess.Context(), apiTimeout)
			defer cancel()
			code := runCommand(ctx, s, sessionKey(sess), args, sess, sess.Stderr())
			_ = sess.Exit(code)
		}
	}
}

// runCommand is the whole of command mode without the session around it, so
// it can be tested with a writer. key identifies the caller to the checkout
// limiter: the SSH fingerprint, or the address for a keyless session.
func runCommand(ctx context.Context, s *apiShop, key string, args []string, stdout, stderr io.Writer) int {
	// Same mapping as the CLI: a request the shop turned down is the
	// caller's to fix, rate limiting and outages are not.
	fail := func(e *shopapi.Error) int {
		emit(stderr, e)
		if e.Status >= 400 && e.Status < 500 && e.Status != http.StatusTooManyRequests {
			return exitUsage
		}
		return exitError
	}

	switch args[0] {
	case "books":
		switch len(args) {
		case 1:
			emit(stdout, s.shelf())
			return exitOK
		case 2:
			i, ok := s.find(args[1])
			if !ok {
				return fail(apiErr(http.StatusNotFound, "%s is not on the shelf", args[1]))
			}
			emit(stdout, s.view(i))
			return exitOK
		}

	case "buy":
		req, err := parseBuy(args[1:])
		if err != nil {
			return fail(apiErr(http.StatusBadRequest, "%s", err))
		}
		// The connection limiter admits sessions; this is the checkout
		// budget, shared with HTTP, because each call creates an order.
		if !s.lim.allowCheckout(key) {
			return fail(apiErr(http.StatusTooManyRequests, "too many checkouts, slow down"))
		}
		out, e := s.checkout(ctx, req)
		if e != nil {
			return fail(e)
		}
		emit(stdout, out)
		return exitOK

	case "order":
		if len(args) == 2 {
			out, e := s.order(ctx, args[1])
			if e != nil {
				return fail(e)
			}
			emit(stdout, out)
			if out.State == shopapi.StatePaid {
				return exitOK
			}
			return exitWaiting
		}

	case "cancel":
		if len(args) == 2 {
			if e := s.cancel(ctx, args[1]); e != nil {
				return fail(e)
			}
			emit(stdout, map[string]string{"cancelled": args[1]})
			return exitOK
		}
	}

	// JSON like every other error, with the usage inside it, so a caller
	// that parses stderr never meets a bare block of text.
	emit(stderr, struct {
		Error string `json:"error"`
		Usage string `json:"usage"`
	}{"unknown command or wrong arguments", commandUsage})
	return exitUsage
}

// parseBuy reads `<isbn>[:qty]... --pickup|--ship [--key k]` in any order.
func parseBuy(args []string) (shopapi.CheckoutRequest, error) {
	var req shopapi.CheckoutRequest
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--pickup" || a == "--ship":
			// Two flags is an ambiguity, not a preference for the last one.
			if req.Fulfilment != "" {
				return req, fmt.Errorf("choose one of --pickup or --ship")
			}
			req.Fulfilment = shopapi.FulfilPickup
			if a == "--ship" {
				req.Fulfilment = shopapi.FulfilShip
			}
		case a == "--key":
			if i+1 >= len(args) {
				return req, fmt.Errorf("--key needs a value")
			}
			i++
			req.IdempotencyKey = args[i]
		case strings.HasPrefix(a, "--key="):
			req.IdempotencyKey = strings.TrimPrefix(a, "--key=")
		case strings.HasPrefix(a, "-"):
			return req, fmt.Errorf("unknown flag %s", a)
		default:
			isbn, q, hasQty := strings.Cut(a, ":")
			qty := 1
			if hasQty {
				n, err := strconv.Atoi(q)
				if err != nil || n < 1 {
					return req, fmt.Errorf("bad quantity in %s", a)
				}
				qty = n
			}
			req.Items = append(req.Items, shopapi.Item{ISBN: isbn, Qty: qty})
		}
	}
	if len(req.Items) == 0 {
		return req, fmt.Errorf("buy needs at least one isbn")
	}
	return req, nil
}

func emit(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
