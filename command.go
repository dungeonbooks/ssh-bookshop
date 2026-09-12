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
// Exit codes are the protocol: 0 done, 1 the shop refused or failed, 2 the
// command was malformed, 3 the order is still waiting for payment.
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
			code := runCommand(sess.Context(), s, args, sess, sess.Stderr())
			_ = sess.Exit(code)
		}
	}
}

// runCommand is the whole of command mode without the session around it, so
// it can be tested with a writer.
func runCommand(ctx context.Context, s *apiShop, args []string, stdout, stderr io.Writer) int {
	fail := func(e *shopapi.Error) int {
		emit(stderr, e)
		if e.Status == http.StatusBadRequest || e.Status == http.StatusNotFound {
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

	io.WriteString(stderr, commandUsage)
	return exitUsage
}

// parseBuy reads `<isbn>[:qty]... --pickup|--ship [--key k]` in any order.
func parseBuy(args []string) (shopapi.CheckoutRequest, error) {
	var req shopapi.CheckoutRequest
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--pickup":
			req.Fulfilment = shopapi.FulfilPickup
		case a == "--ship":
			req.Fulfilment = shopapi.FulfilShip
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
