// dungeon is the shop for a terminal that is not a person: the same shelf and
// the same Square checkout as ssh shop.dungeonbooks.com, as commands an agent
// can run and read. It is a thin client for the HTTP API; every shape it
// prints with --json is the API's, unchanged.
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/dungeonbooks/ssh-bookshop/internal/shopapi"
)

// version is set by the release build with -ldflags "-X main.version=...".
var version = "dev"

//go:embed skill.md
var skill string

// Exit codes are the protocol: 0 done, 1 the shop refused or failed, 2 the
// command was malformed, 3 the order is still waiting for payment.
const (
	exitOK      = 0
	exitError   = 1
	exitUsage   = 2
	exitWaiting = 3
)

const (
	pollEvery   = 3 * time.Second
	waitDefault = time.Hour
)

const rootUsage = `dungeon: dungeon books, from the terminal

usage: dungeon [--json] [--api <url>] <command> [args]

commands:
  (none)             this month's pick and how to buy it
  books              the shelf
  books <isbn>       one book
  buy <isbn>[:qty]... --pickup|--ship
                     build a checkout, print the URL, wait for payment
  order <order_id>   whether an order has been paid
  cancel <checkout_id>
                     abandon an unpaid checkout
  skill              print the agent skill for this tool
  version

flags:
  --json             machine output: the API's JSON, unchanged
  --api <url>        API base (default %s, or $DUNGEON_API)

exit codes: 0 done, 1 error, 2 bad usage, 3 waiting or timed out

Payment happens on a Square page in a browser. buy prints the URL and waits.
`

const buyUsage = `usage: dungeon buy <isbn>[:qty]... (--pickup | --ship) [flags]

  --pickup           collect at the shop, 115 Brunswick St, Jersey City
  --ship             USPS Media Mail, US only, priced by weight
  --no-wait          print the checkout and exit 0 without waiting for payment
  --wait <duration>  how long to wait for payment (default 1h)
  --key <string>     idempotency key: a retry with the same key gets the same link
  --keep             on ctrl-c, leave the checkout live instead of cancelling it

Prints the checkout URL first, on its own line, then waits, polling every few
seconds. Exits 0 once paid, 3 if the wait runs out, 1 if the shop refuses.
`

type cli struct {
	api    *shopapi.Client
	json   bool
	stdout io.Writer
	stderr io.Writer
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("dungeon", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	asJSON := fs.Bool("json", false, "")
	apiURL := fs.String("api", "", "")
	help := fs.Bool("help", false, "")
	fs.BoolVar(help, "h", false, "")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(stderr, rootUsage, shopapi.DefaultBaseURL)
		return exitUsage
	}
	base := *apiURL
	if base == "" {
		base = os.Getenv("DUNGEON_API")
	}
	if base == "" {
		base = shopapi.DefaultBaseURL
	}
	c := &cli{
		api:    &shopapi.Client{BaseURL: base, UserAgent: "dungeon/" + version},
		json:   *asJSON,
		stdout: stdout,
		stderr: stderr,
	}

	rest := fs.Args()
	if *help {
		fmt.Fprintf(stdout, rootUsage, shopapi.DefaultBaseURL)
		return exitOK
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if len(rest) == 0 {
		return c.pick(ctx)
	}
	// --help after a subcommand, for the ones without flags of their own;
	// buy parses its own and answers with its own usage.
	if rest[0] != "buy" && wantsHelp(rest[1:]) {
		fmt.Fprintf(stdout, rootUsage, shopapi.DefaultBaseURL)
		return exitOK
	}
	switch rest[0] {
	case "books":
		switch len(rest) {
		case 1:
			return c.books(ctx)
		case 2:
			return c.book(ctx, rest[1])
		}
	case "buy":
		return c.buy(ctx, rest[1:])
	case "order":
		if len(rest) == 2 {
			return c.order(ctx, rest[1])
		}
	case "cancel":
		if len(rest) == 2 {
			return c.cancel(ctx, rest[1])
		}
	case "skill":
		io.WriteString(stdout, skill)
		return exitOK
	case "version":
		fmt.Fprintln(stdout, "dungeon", version)
		return exitOK
	case "help":
		fmt.Fprintf(stdout, rootUsage, shopapi.DefaultBaseURL)
		return exitOK
	}
	fmt.Fprintf(stderr, rootUsage, shopapi.DefaultBaseURL)
	return exitUsage
}

// usage reports a malformed command. Machine mode stays machine-readable:
// the message goes out as JSON and the usage text is left to --help.
func (c *cli) usage(msg, text string) int {
	if c.json {
		emit(c.stderr, map[string]string{"error": msg})
	} else {
		fmt.Fprintln(c.stderr, msg)
		fmt.Fprintln(c.stderr)
		io.WriteString(c.stderr, text)
	}
	return exitUsage
}

func wantsHelp(args []string) bool {
	for _, a := range args {
		if a == "--help" || a == "-h" || a == "help" {
			return true
		}
	}
	return false
}

// fail reports an error the way the caller asked for output: JSON stays JSON.
// A refusal the API explained (4xx) is the caller's to fix, so it exits 2;
// anything else is 1.
func (c *cli) fail(err error) int {
	var e *shopapi.Error
	if errors.As(err, &e) {
		if c.json {
			emit(c.stderr, e)
		} else {
			fmt.Fprintln(c.stderr, e.Message)
			if e.BuyURL != "" {
				fmt.Fprintln(c.stderr, "buy it at", e.BuyURL)
			}
		}
		if e.Status >= 400 && e.Status < 500 && e.Status != 429 {
			return exitUsage
		}
		return exitError
	}
	if c.json {
		emit(c.stderr, map[string]string{"error": err.Error()})
	} else {
		fmt.Fprintln(c.stderr, err)
	}
	return exitError
}

func (c *cli) pick(ctx context.Context) int {
	shelf, err := c.api.Books(ctx)
	if err != nil {
		return c.fail(err)
	}
	for _, b := range shelf.Books {
		if b.Featured {
			if c.json {
				emit(c.stdout, b)
				return exitOK
			}
			fmt.Fprintf(c.stdout, "this month's pick: %s\n\n", b.Title)
			c.printBook(b)
			return exitOK
		}
	}
	return c.fail(errors.New("no featured pick on the shelf"))
}

func (c *cli) books(ctx context.Context) int {
	shelf, err := c.api.Books(ctx)
	if err != nil {
		return c.fail(err)
	}
	if c.json {
		emit(c.stdout, shelf)
		return exitOK
	}
	section := ""
	for _, b := range shelf.Books {
		s := b.Collection
		if b.Featured {
			s = "featured"
		}
		if s != section {
			if section != "" {
				fmt.Fprintln(c.stdout)
			}
			fmt.Fprintf(c.stdout, "~ %s ~\n", s)
			section = s
		}
		fmt.Fprintf(c.stdout, "  %s  %s, %s  %s\n", b.ISBN, b.Title, b.Author, status(b))
	}
	fmt.Fprintf(c.stdout, "\n%s\n", shelf.Shipping.Note)
	return exitOK
}

func (c *cli) book(ctx context.Context, isbn string) int {
	b, err := c.api.Book(ctx, isbn)
	if err != nil {
		return c.fail(err)
	}
	if c.json {
		emit(c.stdout, b)
		return exitOK
	}
	c.printBook(b)
	return exitOK
}

func (c *cli) printBook(b shopapi.Book) {
	attrs := []string{b.Author}
	if b.Format != "" {
		attrs = append(attrs, b.Format)
	}
	if b.Pages > 0 {
		attrs = append(attrs, fmt.Sprintf("%d pages", b.Pages))
	}
	fmt.Fprintln(c.stdout, b.Title)
	fmt.Fprintln(c.stdout, strings.Join(attrs, " | "))
	fmt.Fprintln(c.stdout, status(b))
	fmt.Fprintln(c.stdout)
	fmt.Fprintln(c.stdout, b.Blurb)
	fmt.Fprintln(c.stdout)
	if b.Sellable {
		fmt.Fprintf(c.stdout, "buy: dungeon buy %s --pickup   or   dungeon buy %s --ship\n", b.ISBN, b.ISBN)
	} else {
		fmt.Fprintf(c.stdout, "buy: %s\n", b.BuyURL)
	}
}

// status is the price and stock in one short phrase.
func status(b shopapi.Book) string {
	if !b.Sellable {
		return "sold out here, " + b.BuyURL
	}
	s := usd(b.PriceCents)
	if b.Stock != nil && *b.Stock <= 3 {
		s += fmt.Sprintf("  only %d left", *b.Stock)
	}
	return s
}

func (c *cli) buy(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("buy", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	pickup := fs.Bool("pickup", false, "")
	ship := fs.Bool("ship", false, "")
	noWait := fs.Bool("no-wait", false, "")
	wait := fs.Duration("wait", waitDefault, "")
	key := fs.String("key", "", "")
	keep := fs.Bool("keep", false, "")
	help := fs.Bool("help", false, "")
	fs.BoolVar(help, "h", false, "")
	// Flags may follow the ISBNs, so parse, then treat what is left as
	// positionals and parse again in case flags trail them.
	var isbns []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			return c.usage(err.Error(), buyUsage)
		}
		rest = fs.Args()
		if len(rest) == 0 {
			break
		}
		isbns = append(isbns, rest[0])
		rest = rest[1:]
	}
	if *help {
		io.WriteString(c.stdout, buyUsage)
		return exitOK
	}
	switch {
	case len(isbns) == 0:
		return c.usage("buy needs at least one isbn", buyUsage)
	case *pickup == *ship:
		return c.usage("choose one of --pickup or --ship", buyUsage)
	}
	req := shopapi.CheckoutRequest{Fulfilment: shopapi.FulfilPickup, IdempotencyKey: *key}
	if *ship {
		req.Fulfilment = shopapi.FulfilShip
	}
	for _, a := range isbns {
		isbn, q, hasQty := strings.Cut(a, ":")
		qty := 1
		if hasQty {
			n, err := strconv.Atoi(q)
			if err != nil || n < 1 {
				return c.usage("bad quantity in "+a, buyUsage)
			}
			qty = n
		}
		req.Items = append(req.Items, shopapi.Item{ISBN: isbn, Qty: qty})
	}

	out, err := c.api.Checkout(ctx, req)
	if err != nil {
		return c.fail(err)
	}
	if c.json {
		emit(c.stdout, out)
	} else {
		// The URL first and on its own line, so a caller that captures output
		// has it even if it kills the wait.
		fmt.Fprintln(c.stdout, out.CheckoutURL)
		fmt.Fprintf(c.stdout, "order %s\n", out.OrderID)
		if out.ShippingCents > 0 {
			fmt.Fprintf(c.stdout, "books %s + shipping %s = %s before tax\n", usd(out.SubtotalCents), usd(out.ShippingCents), usd(out.TotalCents))
		} else {
			fmt.Fprintf(c.stdout, "total %s before tax, pick up at the shop\n", usd(out.TotalCents))
		}
	}
	if *noWait {
		return exitOK
	}

	if !c.json {
		fmt.Fprintln(c.stdout, "waiting for payment (ctrl-c to abandon)")
	}
	deadline := time.Now().Add(*wait)
	for {
		select {
		case <-ctx.Done():
			// Interrupted. The link would otherwise stay payable for a day,
			// against an order nobody is watching; tidy it unless asked not to.
			if *keep {
				fmt.Fprintf(c.stderr, "left checkout %s live; pay at %s or cancel with: dungeon cancel %s\n", out.CheckoutID, out.CheckoutURL, out.CheckoutID)
				return exitWaiting
			}
			bg, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := c.api.Cancel(bg, out.CheckoutID); err != nil {
				fmt.Fprintf(c.stderr, "could not cancel checkout %s: %v\n", out.CheckoutID, err)
				return exitError
			}
			fmt.Fprintln(c.stderr, "abandoned; the checkout link no longer works")
			return exitError
		case <-time.After(min(pollEvery, max(time.Until(deadline), 0))):
			// Bounded by the deadline, so a short --wait is honoured rather
			// than rounded up to the poll interval.
		}
		o, err := c.api.Order(ctx, out.OrderID)
		if err != nil {
			if ctx.Err() != nil {
				continue // the select above will handle the interrupt
			}
			// A failed poll is not a failed order. Keep waiting, but not
			// past the deadline: an API that is down for an hour must still
			// hand control back.
			if time.Now().After(deadline) {
				return c.stillWaiting(out.OrderID, *wait, shopapi.Order{OrderID: out.OrderID, State: shopapi.StateAwaitingPayment})
			}
			continue
		}
		if o.State == shopapi.StatePaid {
			if c.json {
				emit(c.stdout, o)
			} else {
				fmt.Fprintln(c.stdout, "paid. thank you for ordering from dungeon books.")
			}
			return exitOK
		}
		if time.Now().After(deadline) {
			return c.stillWaiting(out.OrderID, *wait, o)
		}
	}
}

func (c *cli) stillWaiting(orderID string, waited time.Duration, o shopapi.Order) int {
	if c.json {
		emit(c.stdout, o)
	} else {
		fmt.Fprintf(c.stdout, "still waiting after %s; check later with: dungeon order %s\n", waited, orderID)
	}
	return exitWaiting
}

func (c *cli) order(ctx context.Context, id string) int {
	o, err := c.api.Order(ctx, id)
	if err != nil {
		return c.fail(err)
	}
	if c.json {
		emit(c.stdout, o)
	} else {
		fmt.Fprintln(c.stdout, strings.ReplaceAll(o.State, "_", " "))
	}
	if o.State == shopapi.StatePaid {
		return exitOK
	}
	return exitWaiting
}

func (c *cli) cancel(ctx context.Context, id string) int {
	if err := c.api.Cancel(ctx, id); err != nil {
		return c.fail(err)
	}
	if c.json {
		emit(c.stdout, map[string]string{"cancelled": id})
	} else {
		fmt.Fprintln(c.stdout, "cancelled")
	}
	return exitOK
}

func usd(cents int64) string {
	if cents%100 == 0 {
		return fmt.Sprintf("$%d", cents/100)
	}
	return fmt.Sprintf("$%d.%02d", cents/100, cents%100)
}

func emit(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
