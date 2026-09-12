package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"charm.land/log/v2"

	"github.com/dungeonbooks/ssh-bookshop/internal/shopapi"
)

// The HTTP API is the shop for agents: the same shelf, the same Square
// checkout, without a terminal in the way. It goes as far as a shopper can go
// without a card, which is the hosted checkout URL, and stops there.
//
// There is no authentication because there is nothing to authenticate. The
// shop has no accounts, and the only state an order has lives in Square under
// an id that reveals nothing but whether it was paid.
const (
	// maxQtyPerLine is a shop limit rather than a stock one: checkout already
	// refuses more copies than are on the shelf, but an untracked book has no
	// count to refuse against, and nobody buys eleven of anything here.
	maxQtyPerLine = 10

	// maxBody is generous for a request that is a few ISBNs and a word.
	maxBody = 4 << 10

	// apiTimeout bounds one request end to end. Longer than squareTimeout,
	// because a checkout makes several Square calls in a row.
	apiTimeout = 30 * time.Second

	shippingNote = "USPS Media Mail, priced by weight at checkout. Pickup at the shop is free."
	totalNote    = "Total before tax. Square collects the address and tax on the checkout page."
)

// apiShop is the API's view of the shelf. Like a TUI session it keeps what
// Square said during its checkouts as an overlay rather than writing back to
// the shelf that every SSH session is reading concurrently; unlike a session
// the overlay is shared by every request, so it is locked.
type apiShop struct {
	books []Book
	sq    *squareClient
	// lim is shared by the HTTP and SSH transports, so a checkout is a
	// checkout whichever way it arrives.
	lim *apiLimiter

	mu    sync.RWMutex
	fresh map[string]freshItem
	asOf  time.Time
}

func newAPIShop(books []Book, sq *squareClient) *apiShop {
	return &apiShop{books: books, sq: sq, lim: newAPILimiter(), fresh: map[string]freshItem{}, asOf: time.Now().UTC()}
}

// book returns shelf entry i with anything the API has re-read from Square laid
// over it, the same way model.book does for a session.
func (s *apiShop) book(i int) Book {
	b := s.books[i]
	s.mu.RLock()
	f, ok := s.fresh[b.VariationID]
	s.mu.RUnlock()
	if ok {
		b.Cents, b.Stock = f.cents, f.stock
		b.Tracked, b.Sellable = !f.untracked, f.sellable
	}
	return b
}

// remember keeps what a checkout just learned, merged rather than replaced so
// an earlier checkout's prices stay correct.
func (s *apiShop) remember(fresh map[string]freshItem) {
	if len(fresh) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range fresh {
		s.fresh[k] = v
	}
	s.asOf = time.Now().UTC()
}

func (s *apiShop) find(isbn string) (int, bool) {
	for i, b := range s.books {
		if b.ISBN == isbn {
			return i, true
		}
	}
	return -1, false
}

func (s *apiShop) view(i int) shopapi.Book {
	b := s.book(i)
	v := shopapi.Book{
		ISBN:       b.ISBN,
		Title:      b.BookTitle,
		Author:     b.Author,
		Collection: b.Collection,
		Month:      b.Month,
		Featured:   i == featuredIn(s.books),
		Format:     b.Format,
		Pages:      b.Pages,
		Blurb:      b.Blurb,
		PriceCents: b.Cents,
		Sellable:   b.Sellable,
		Tracked:    b.Tracked,
	}
	if b.Tracked {
		stock := b.Stock
		v.Stock = &stock
	}
	// Only one way to buy a book: ours while we have it, Bookshop's once we
	// do not. A sellable book's buy URL is the checkout endpoint.
	if !b.Sellable {
		v.BuyURL = b.BuyURL()
	}
	return v
}

func (s *apiShop) shelf() shopapi.Shelf {
	out := shopapi.Shelf{
		Books:    make([]shopapi.Book, 0, len(s.books)),
		Shipping: shopapi.Shipping{Region: "US", Note: shippingNote},
	}
	for i := range s.books {
		out.Books = append(out.Books, s.view(i))
	}
	s.mu.RLock()
	out.PricesAsOf = s.asOf
	s.mu.RUnlock()
	return out
}

func apiErr(status int, format string, args ...any) *shopapi.Error {
	return &shopapi.Error{Status: status, Message: fmt.Sprintf(format, args...)}
}

// checkout is POST /v1/checkout without the HTTP around it, so the SSH command
// path can share it. Everything before createLink is validation the TUI does
// by construction: its cart can only hold books from the shelf, in quantities
// the stepper allowed.
func (s *apiShop) checkout(ctx context.Context, req shopapi.CheckoutRequest) (shopapi.Checkout, *shopapi.Error) {
	var how fulfilment
	switch req.Fulfilment {
	case shopapi.FulfilPickup:
		how = fulfilPickup
	case shopapi.FulfilShip:
		how = fulfilShip
	case "":
		return shopapi.Checkout{}, apiErr(http.StatusBadRequest, "fulfilment is required: %q or %q", shopapi.FulfilPickup, shopapi.FulfilShip)
	default:
		return shopapi.Checkout{}, apiErr(http.StatusBadRequest, "fulfilment must be %q or %q, not %q", shopapi.FulfilPickup, shopapi.FulfilShip, req.Fulfilment)
	}
	if len(req.Items) == 0 {
		return shopapi.Checkout{}, apiErr(http.StatusBadRequest, "items is empty")
	}
	// Before the shelf is consulted, so an unconfigured Square says so rather
	// than presenting as every book having sold out.
	if s.sq == nil {
		return shopapi.Checkout{}, apiErr(http.StatusServiceUnavailable, "square is not configured")
	}

	// Merge duplicate lines so two entries for one ISBN do not become two
	// Square line items that each pass the stock check on their own.
	var lines []cartLine
	for _, it := range req.Items {
		if it.Qty < 1 {
			return shopapi.Checkout{}, apiErr(http.StatusBadRequest, "qty for %s must be at least 1", it.ISBN)
		}
		idx, ok := s.find(it.ISBN)
		if !ok {
			return shopapi.Checkout{}, apiErr(http.StatusNotFound, "%s is not on the shelf", it.ISBN)
		}
		tooMany := apiErr(http.StatusBadRequest, "at most %d copies of %s per order", maxQtyPerLine, s.books[idx].BookTitle)
		// Checked as headroom rather than after adding, so two quantities
		// that are each in range cannot overflow the sum past the cap.
		if it.Qty > maxQtyPerLine {
			return shopapi.Checkout{}, tooMany
		}
		merged := false
		for i := range lines {
			if lines[i].idx == idx {
				if it.Qty > maxQtyPerLine-lines[i].qty {
					return shopapi.Checkout{}, tooMany
				}
				lines[i].qty += it.Qty
				merged = true
			}
		}
		if !merged {
			lines = append(lines, cartLine{idx: idx, qty: it.Qty})
		}
	}
	items := make([]cartItem, 0, len(lines))
	var subtotal int64
	for _, l := range lines {
		b := s.book(l.idx)
		// Only a book Square has never priced is refused here. One that has
		// sold out since goes on to the recheck, which is what notices it
		// coming back into stock; refusing on the overlay alone would make a
		// sellout permanent for the life of the process.
		if b.VariationID == "" {
			e := apiErr(http.StatusConflict, "%s is sold out here", b.BookTitle)
			e.BuyURL = b.BuyURL()
			return shopapi.Checkout{}, e
		}
		items = append(items, cartItem{isbn: b.ISBN, variationID: b.VariationID, title: b.BookTitle, cents: b.Cents, qty: l.qty})
		subtotal += b.Cents * int64(l.qty)
	}

	var ship int64
	exact := true
	if how == fulfilShip {
		ship, exact = shippingFor(lines, func(i int) int { return s.books[i].WeightGrams })
	}
	key := req.IdempotencyKey
	if key == "" {
		key = newIdempotencyKey()
	}

	out, fresh, err := s.sq.createLink(ctx, items, how, ship, key)
	s.remember(fresh)
	if err != nil {
		var ce *cartError
		if errors.As(err, &ce) {
			e := apiErr(http.StatusConflict, "%s", ce.msg)
			e.PriceCents = ce.cents
			// The overlay was just updated from what Square said, so if the
			// book is no longer for sale here the refusal says where else.
			if i, ok := s.find(ce.isbn); ok {
				if b := s.book(i); !b.Sellable {
					e.BuyURL = b.BuyURL()
				}
			}
			return shopapi.Checkout{}, e
		}
		return shopapi.Checkout{}, apiErr(http.StatusBadGateway, "%s", err)
	}
	return shopapi.Checkout{
		OrderID:       out.OrderID,
		CheckoutID:    out.LinkID,
		CheckoutURL:   out.URL,
		Fulfilment:    req.Fulfilment,
		SubtotalCents: subtotal,
		ShippingCents: ship,
		ShippingExact: exact,
		TotalCents:    subtotal + ship,
		Note:          totalNote,
	}, nil
}

// order is GET /v1/orders/{id}.
func (s *apiShop) order(ctx context.Context, id string) (shopapi.Order, *shopapi.Error) {
	if s.sq == nil {
		return shopapi.Order{}, apiErr(http.StatusServiceUnavailable, "square is not configured")
	}
	paid, err := s.sq.paid(ctx, id)
	if err != nil {
		return shopapi.Order{}, squareErr(err, "order %s", id)
	}
	o := shopapi.Order{OrderID: id, State: shopapi.StateAwaitingPayment}
	if paid {
		o.State = shopapi.StatePaid
	}
	return o, nil
}

// cancel is DELETE /v1/checkout/{id}: the API's discardLink, with the same
// refusal to cancel an order that has been paid or cannot be read.
func (s *apiShop) cancel(ctx context.Context, checkoutID string) *shopapi.Error {
	if s.sq == nil {
		return apiErr(http.StatusServiceUnavailable, "square is not configured")
	}
	l, err := s.sq.link(ctx, checkoutID)
	if err != nil {
		return squareErr(err, "checkout %s", checkoutID)
	}
	if l.OrderID != "" {
		paid, err := s.sq.paid(ctx, l.OrderID)
		if err != nil {
			return squareErr(err, "order %s", l.OrderID)
		}
		if paid {
			return apiErr(http.StatusConflict, "order %s is already paid", l.OrderID)
		}
	}
	if err := s.sq.deleteLink(ctx, l); err != nil {
		return apiErr(http.StatusBadGateway, "%s", err)
	}
	return nil
}

// squareErr maps a Square failure onto a status. NOT_FOUND is the one worth
// telling apart: an id the caller made up is their problem, anything else is
// between us and Square.
func squareErr(err error, format string, args ...any) *shopapi.Error {
	if strings.Contains(err.Error(), "NOT_FOUND") {
		return apiErr(http.StatusNotFound, format+" not found", args...)
	}
	return apiErr(http.StatusBadGateway, "%s", err)
}

// --- HTTP ------------------------------------------------------------------

// handler is the whole API. Rate limiting wraps everything; the checkout
// route gets a second, tighter limit because it creates orders on Square.
func (s *apiShop) handler() http.Handler {
	lim := s.lim
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/books", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, s.shelf())
	})
	mux.HandleFunc("GET /v1/books/{isbn}", func(w http.ResponseWriter, r *http.Request) {
		i, ok := s.find(r.PathValue("isbn"))
		if !ok {
			writeErr(w, apiErr(http.StatusNotFound, "%s is not on the shelf", r.PathValue("isbn")))
			return
		}
		writeJSON(w, http.StatusOK, s.view(i))
	})
	mux.HandleFunc("POST /v1/checkout", func(w http.ResponseWriter, r *http.Request) {
		if !lim.allowCheckout(clientKey(r)) {
			w.Header().Set("Retry-After", "5")
			writeErr(w, apiErr(http.StatusTooManyRequests, "too many checkouts, slow down"))
			return
		}
		var req shopapi.CheckoutRequest
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
		if err := dec.Decode(&req); err != nil {
			writeErr(w, apiErr(http.StatusBadRequest, "bad request body: %s", err))
			return
		}
		// One object and nothing after it. A valid request followed by
		// garbage is a malformed request, not a request.
		if dec.Decode(&struct{}{}) != io.EOF {
			writeErr(w, apiErr(http.StatusBadRequest, "bad request body: trailing data after the request"))
			return
		}
		out, e := s.checkout(r.Context(), req)
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, http.StatusCreated, out)
	})
	mux.HandleFunc("GET /v1/orders/{id}", func(w http.ResponseWriter, r *http.Request) {
		out, e := s.order(r.Context(), r.PathValue("id"))
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	mux.HandleFunc("DELETE /v1/checkout/{id}", func(w http.ResponseWriter, r *http.Request) {
		if e := s.cancel(r.Context(), r.PathValue("id")); e != nil {
			writeErr(w, e)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, apiErr(http.StatusNotFound, "no such route; see https://shop.dungeonbooks.com/llms.txt"))
	})

	// The log sits outside the timeout so a request that timed out is logged
	// with the 503 it got, not the 200 the inner handler never sent.
	return accessLog(http.TimeoutHandler(lim.limit(mux), apiTimeout, `{"error":"timed out"}`))
}

// apiRoutes is every pattern the handler serves, in one place so a test can
// check the OpenAPI document against it.
var apiRoutes = []string{
	"GET /v1/books",
	"GET /v1/books/{isbn}",
	"POST /v1/checkout",
	"GET /v1/orders/{id}",
	"DELETE /v1/checkout/{id}",
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func writeErr(w http.ResponseWriter, e *shopapi.Error) {
	writeJSON(w, e.Status, e)
}

// accessLog is sessionLog for HTTP: enough to know the API is up and what it
// is being asked, and nothing that identifies who is asking.
func accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Info("api", "method", r.Method, "path", r.URL.Path, "status", rec.status,
			"took", time.Since(start).Round(time.Millisecond))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// clientKey is who is asking, for the rate limiter only; it is never logged.
//
// The listener is loopback-only, so the one thing that can set X-Client-IP is
// Caddy in front of it, which fills it from the real client address after
// honouring its trusted proxies. Without the header this is a direct
// connection, and the remote address is the client.
func clientKey(r *http.Request) string {
	if ip := net.ParseIP(strings.TrimSpace(r.Header.Get("X-Client-IP"))); ip != nil {
		return addrKey(&net.TCPAddr{IP: ip})
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil {
		return addrKey(&net.TCPAddr{IP: ip})
	}
	return host
}

// loopbackAddr reports whether addr binds only to this machine.
func loopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// startAPI serves the API on addr in the background. A listener that fails
// reports on failed, the same channel the SSH server uses, so the process
// stops and systemd restarts it rather than running on with the agent side
// silently missing.
func startAPI(addr string, s *apiShop, failed chan<- os.Signal) *http.Server {
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      apiTimeout + 5*time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		log.Info("starting api", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("api error", "err", err)
			failed <- nil
		}
	}()
	return srv
}
