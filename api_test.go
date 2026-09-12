package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/dungeonbooks/ssh-bookshop/internal/shopapi"
)

// stubSquare is enough of Square for a checkout: a catalog keyed by ISBN, a
// stock count per variation, and the order and link calls checkout makes.
// Fields are mutated by tests to move the shelf out from under the API, which
// is the situation every 409 exists for.
type stubSquare struct {
	mu     sync.Mutex
	cents  map[string]int64 // by variation id
	stock  map[string]int   // by variation id; absent means untracked
	upc    map[string]string
	paid   bool
	links  int // payment links created
	orders map[string]bool
}

func (st *stubSquare) client(t *testing.T) *squareClient {
	t.Helper()
	return fakeSquare(t, map[string]func(http.ResponseWriter, *http.Request){
		"/v2/catalog/search-catalog-items": func(w http.ResponseWriter, r *http.Request) {
			var q struct {
				TextFilter string `json:"text_filter"`
			}
			json.NewDecoder(r.Body).Decode(&q)
			st.mu.Lock()
			defer st.mu.Unlock()
			id, ok := st.upc[q.TextFilter]
			if !ok {
				io.WriteString(w, `{"items":[]}`)
				return
			}
			fmt.Fprintf(w, `{"items":[{"item_data":{"variations":[{"id":%q,"item_variation_data":{"upc":%q,"price_money":{"amount":%d,"currency":"USD"}}}]}}]}`,
				id, q.TextFilter, st.cents[id])
		},
		"/v2/inventory/counts/batch-retrieve": func(w http.ResponseWriter, r *http.Request) {
			st.mu.Lock()
			defer st.mu.Unlock()
			var counts []string
			for id, n := range st.stock {
				counts = append(counts, fmt.Sprintf(`{"catalog_object_id":%q,"state":"IN_STOCK","quantity":"%d"}`, id, n))
			}
			io.WriteString(w, `{"counts":[`+strings.Join(counts, ",")+`]}`)
		},
		"/v2/online-checkout/payment-links": func(w http.ResponseWriter, r *http.Request) {
			st.mu.Lock()
			defer st.mu.Unlock()
			switch r.Method {
			case http.MethodPost:
				st.links++
				fmt.Fprintf(w, `{"payment_link":{"id":"LINK%d","url":"https://square.link/u/test%d","order_id":"ORD%d"}}`, st.links, st.links, st.links)
			case http.MethodGet:
				id := strings.TrimPrefix(r.URL.Path, "/v2/online-checkout/payment-links/")
				if id == "" || id == "MISSING" {
					io.WriteString(w, `{"errors":[{"code":"NOT_FOUND","detail":"no such link"}]}`)
					return
				}
				n := strings.TrimPrefix(id, "LINK")
				fmt.Fprintf(w, `{"payment_link":{"id":%q,"order_id":"ORD%s","created_at":"2026-01-01T00:00:00Z"}}`, id, n)
			case http.MethodDelete:
				io.WriteString(w, `{}`)
			}
		},
		"/v2/orders/": func(w http.ResponseWriter, r *http.Request) {
			st.mu.Lock()
			defer st.mu.Unlock()
			if strings.HasSuffix(r.URL.Path, "/MISSING") {
				io.WriteString(w, `{"errors":[{"code":"NOT_FOUND","detail":"no such order"}]}`)
				return
			}
			if st.paid {
				io.WriteString(w, `{"order":{"state":"COMPLETED","tenders":[{"id":"T1"}]}}`)
				return
			}
			io.WriteString(w, `{"order":{"state":"OPEN"}}`)
		},
	})
}

const (
	isbnStocked = "9780000000001"
	isbnGone    = "9780000000002"
)

// testShop is two books: one we carry with a tracked count of 4 at $30, one we
// do not carry at all, which links out to Bookshop.
func testShop(t *testing.T) (*apiShop, *stubSquare) {
	t.Helper()
	st := &stubSquare{
		cents:  map[string]int64{"VAR1": 3000},
		stock:  map[string]int{"VAR1": 4},
		upc:    map[string]string{isbnStocked: "VAR1"},
		orders: map[string]bool{},
	}
	books := []Book{
		{ISBN: isbnStocked, BookTitle: "Stocked", Author: "A", Collection: collBookClub, Month: "2026-09",
			WeightGrams: 500, Cents: 3000, VariationID: "VAR1", Stock: 4, Tracked: true, Sellable: true},
		{ISBN: isbnGone, BookTitle: "Gone", Author: "B", Collection: collBookClub, Month: "2026-08"},
	}
	return newAPIShop(books, st.client(t)), st
}

func do(t *testing.T, h http.Handler, method, path string, body any) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rdr)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("%s %s: body is not JSON: %v\n%s", method, path, err, rec.Body.String())
		}
	}
	return rec, out
}

func TestShelfView(t *testing.T) {
	s, _ := testShop(t)
	h := s.handler()

	rec, out := do(t, h, http.MethodGet, "/v1/books", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	books := out["books"].([]any)
	if len(books) != 2 {
		t.Fatalf("got %d books", len(books))
	}
	stocked := books[0].(map[string]any)
	if stocked["featured"] != true || stocked["stock"] != 4.0 || stocked["sellable"] != true {
		t.Errorf("stocked = %v", stocked)
	}
	if _, has := stocked["buy_url"]; has {
		t.Error("a sellable book must not carry a buy_url: the checkout is how you buy it")
	}
	gone := books[1].(map[string]any)
	if gone["sellable"] != false || !strings.Contains(gone["buy_url"].(string), "bookshop.org") {
		t.Errorf("gone = %v", gone)
	}
	if _, has := gone["stock"]; has {
		t.Error("an untracked book must not report a stock count")
	}

	rec, _ = do(t, h, http.MethodGet, "/v1/books/"+isbnGone, nil)
	if rec.Code != http.StatusOK {
		t.Errorf("one book: status = %d", rec.Code)
	}
	rec, _ = do(t, h, http.MethodGet, "/v1/books/000", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown isbn: status = %d", rec.Code)
	}
	rec, _ = do(t, h, http.MethodGet, "/nope", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown route: status = %d", rec.Code)
	}
}

func TestCheckoutShips(t *testing.T) {
	s, st := testShop(t)
	h := s.handler()

	rec, out := do(t, h, http.MethodPost, "/v1/checkout", shopapi.CheckoutRequest{
		Items: []shopapi.Item{{ISBN: isbnStocked, Qty: 2}}, Fulfilment: shopapi.FulfilShip,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %v", rec.Code, out)
	}
	if out["checkout_url"] != "https://square.link/u/test1" || out["order_id"] != "ORD1" || out["checkout_id"] != "LINK1" {
		t.Errorf("checkout = %v", out)
	}
	wantShip := mediaMail(2*500 + packagingGrams)
	if out["subtotal_cents"] != 6000.0 || out["shipping_cents"] != float64(wantShip) || out["total_cents"] != float64(6000+wantShip) {
		t.Errorf("totals = %v, want shipping %d", out, wantShip)
	}
	if out["shipping_exact"] != true {
		t.Error("shipping_exact = false with a known weight")
	}
	if st.links != 1 {
		t.Errorf("links created = %d", st.links)
	}
}

func TestCheckoutPickupIsFree(t *testing.T) {
	s, _ := testShop(t)
	_, out := do(t, s.handler(), http.MethodPost, "/v1/checkout", shopapi.CheckoutRequest{
		Items: []shopapi.Item{{ISBN: isbnStocked, Qty: 1}}, Fulfilment: shopapi.FulfilPickup,
	})
	if out["shipping_cents"] != 0.0 || out["total_cents"] != 3000.0 {
		t.Errorf("pickup totals = %v", out)
	}
}

// TestCheckoutRefusals is every way a request is turned away before or at the
// point of asking Square for a link. None of them may create one.
func TestCheckoutRefusals(t *testing.T) {
	for _, tc := range []struct {
		name   string
		req    any
		status int
		want   string
	}{
		{"no fulfilment", shopapi.CheckoutRequest{Items: []shopapi.Item{{ISBN: isbnStocked, Qty: 1}}}, 400, "fulfilment is required"},
		{"bad fulfilment", shopapi.CheckoutRequest{Items: []shopapi.Item{{ISBN: isbnStocked, Qty: 1}}, Fulfilment: "drone"}, 400, "drone"},
		{"no items", shopapi.CheckoutRequest{Fulfilment: "pickup"}, 400, "items is empty"},
		{"zero qty", shopapi.CheckoutRequest{Items: []shopapi.Item{{ISBN: isbnStocked, Qty: 0}}, Fulfilment: "pickup"}, 400, "at least 1"},
		{"unknown isbn", shopapi.CheckoutRequest{Items: []shopapi.Item{{ISBN: "1", Qty: 1}}, Fulfilment: "pickup"}, 404, "not on the shelf"},
		{"too many", shopapi.CheckoutRequest{Items: []shopapi.Item{{ISBN: isbnStocked, Qty: 11}}, Fulfilment: "pickup"}, 400, "at most 10"},
		{"duplicates merge before the cap", shopapi.CheckoutRequest{Items: []shopapi.Item{{ISBN: isbnStocked, Qty: 6}, {ISBN: isbnStocked, Qty: 6}}, Fulfilment: "pickup"}, 400, "at most 10"},
		{"a sum that would overflow", shopapi.CheckoutRequest{Items: []shopapi.Item{{ISBN: isbnStocked, Qty: 6}, {ISBN: isbnStocked, Qty: math.MaxInt}}, Fulfilment: "pickup"}, 400, "at most 10"},
		{"not carried", shopapi.CheckoutRequest{Items: []shopapi.Item{{ISBN: isbnGone, Qty: 1}}, Fulfilment: "pickup"}, 409, "sold out here"},
		{"not json", "nonsense", 400, "bad request body"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, st := testShop(t)
			rec, out := do(t, s.handler(), http.MethodPost, "/v1/checkout", tc.req)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d: %v", rec.Code, tc.status, out)
			}
			if msg, _ := out["error"].(string); !strings.Contains(msg, tc.want) {
				t.Errorf("error = %q, want it to mention %q", msg, tc.want)
			}
			if tc.name == "not carried" && !strings.Contains(out["buy_url"].(string), "bookshop.org") {
				t.Errorf("a book we do not carry should still say where to buy it: %v", out)
			}
			if st.links != 0 {
				t.Errorf("a refused checkout created %d link(s)", st.links)
			}
		})
	}
}

// TestCheckoutStaleShelf moves Square out from under the shelf between boot and
// checkout. The 409 has to carry what Square says now, and the shelf has to
// say it too afterwards, so the next request is not doomed to repeat.
func TestCheckoutStaleShelf(t *testing.T) {
	t.Run("repriced", func(t *testing.T) {
		s, st := testShop(t)
		st.cents["VAR1"] = 3500
		h := s.handler()
		rec, out := do(t, h, http.MethodPost, "/v1/checkout", shopapi.CheckoutRequest{
			Items: []shopapi.Item{{ISBN: isbnStocked, Qty: 1}}, Fulfilment: "pickup",
		})
		if rec.Code != http.StatusConflict || out["price_cents"] != 3500.0 {
			t.Fatalf("status = %d, body = %v", rec.Code, out)
		}
		_, shelf := do(t, h, http.MethodGet, "/v1/books/"+isbnStocked, nil)
		if shelf["price_cents"] != 3500.0 {
			t.Errorf("shelf still says %v after Square said 3500", shelf["price_cents"])
		}
	})
	t.Run("short", func(t *testing.T) {
		s, st := testShop(t)
		st.stock["VAR1"] = 1
		rec, out := do(t, s.handler(), http.MethodPost, "/v1/checkout", shopapi.CheckoutRequest{
			Items: []shopapi.Item{{ISBN: isbnStocked, Qty: 2}}, Fulfilment: "pickup",
		})
		if rec.Code != http.StatusConflict || !strings.Contains(out["error"].(string), "only 1 left") {
			t.Fatalf("status = %d, body = %v", rec.Code, out)
		}
	})
	t.Run("removed from the catalog", func(t *testing.T) {
		s, st := testShop(t)
		delete(st.upc, isbnStocked)
		h := s.handler()
		rec, out := do(t, h, http.MethodPost, "/v1/checkout", shopapi.CheckoutRequest{
			Items: []shopapi.Item{{ISBN: isbnStocked, Qty: 1}}, Fulfilment: "pickup",
		})
		if rec.Code != http.StatusConflict || !strings.Contains(out["error"].(string), "sold out") {
			t.Fatalf("status = %d, body = %v", rec.Code, out)
		}
		_, shelf := do(t, h, http.MethodGet, "/v1/books/"+isbnStocked, nil)
		if shelf["sellable"] != false {
			t.Error("shelf still offers a book Square no longer lists, so every retry would hit the same 409")
		}
	})
	t.Run("sold out", func(t *testing.T) {
		s, st := testShop(t)
		st.stock["VAR1"] = 0
		h := s.handler()
		rec, _ := do(t, h, http.MethodPost, "/v1/checkout", shopapi.CheckoutRequest{
			Items: []shopapi.Item{{ISBN: isbnStocked, Qty: 1}}, Fulfilment: "pickup",
		})
		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d", rec.Code)
		}
		_, shelf := do(t, h, http.MethodGet, "/v1/books/"+isbnStocked, nil)
		if shelf["sellable"] != false {
			t.Error("shelf still sellable after Square reported zero")
		}
	})
}

func TestCheckoutWithoutSquare(t *testing.T) {
	s, _ := testShop(t)
	s.sq = nil
	rec, _ := do(t, s.handler(), http.MethodPost, "/v1/checkout", shopapi.CheckoutRequest{
		Items: []shopapi.Item{{ISBN: isbnStocked, Qty: 1}}, Fulfilment: "pickup",
	})
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d", rec.Code)
	}
	// Browsing still works, as it does in the TUI.
	if rec, _ := do(t, s.handler(), http.MethodGet, "/v1/books", nil); rec.Code != http.StatusOK {
		t.Errorf("shelf status = %d without square", rec.Code)
	}
}

func TestOrderState(t *testing.T) {
	s, st := testShop(t)
	h := s.handler()
	rec, out := do(t, h, http.MethodGet, "/v1/orders/ORD1", nil)
	if rec.Code != http.StatusOK || out["state"] != shopapi.StateAwaitingPayment {
		t.Errorf("unpaid: %d %v", rec.Code, out)
	}
	st.paid = true
	_, out = do(t, h, http.MethodGet, "/v1/orders/ORD1", nil)
	if out["state"] != shopapi.StatePaid {
		t.Errorf("paid: %v", out)
	}
	rec, _ = do(t, h, http.MethodGet, "/v1/orders/MISSING", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("missing order: status = %d", rec.Code)
	}
}

func TestCancel(t *testing.T) {
	s, st := testShop(t)
	h := s.handler()
	if rec, out := do(t, h, http.MethodDelete, "/v1/checkout/LINK1", nil); rec.Code != http.StatusNoContent {
		t.Errorf("cancel: %d %v", rec.Code, out)
	}
	if rec, _ := do(t, h, http.MethodDelete, "/v1/checkout/MISSING", nil); rec.Code != http.StatusNotFound {
		t.Errorf("cancel missing: %d", rec.Code)
	}
	st.paid = true
	if rec, _ := do(t, h, http.MethodDelete, "/v1/checkout/LINK1", nil); rec.Code != http.StatusConflict {
		t.Errorf("cancel paid: %d, a paid order must never be cancelled", rec.Code)
	}
}

// TestCheckoutRateLimit: the checkout bucket is the one that matters, because
// every call past it is an order on Square.
func TestCheckoutRateLimit(t *testing.T) {
	s, st := testShop(t)
	h := s.handler()
	req := shopapi.CheckoutRequest{Items: []shopapi.Item{{ISBN: isbnStocked, Qty: 1}}, Fulfilment: "pickup"}
	var limited int
	for i := 0; i < checkoutAddrBurst+3; i++ {
		rec, _ := do(t, h, http.MethodPost, "/v1/checkout", req)
		if rec.Code == http.StatusTooManyRequests {
			limited++
		}
	}
	if limited != 3 {
		t.Errorf("limited %d of %d, want 3", limited, checkoutAddrBurst+3)
	}
	if st.links != checkoutAddrBurst {
		t.Errorf("links created = %d, want %d", st.links, checkoutAddrBurst)
	}
}

// TestOrderIDIsEscaped: the id comes straight off a public URL, and it must
// not be able to steer the request at some other Square endpoint.
func TestOrderIDIsEscaped(t *testing.T) {
	var got string
	c := fakeSquare(t, map[string]func(http.ResponseWriter, *http.Request){
		"/v2/orders/": func(w http.ResponseWriter, r *http.Request) {
			got = r.RequestURI
			io.WriteString(w, `{"order":{"state":"OPEN"}}`)
		},
	})
	if _, err := c.paid(context.Background(), "a/b?x=1"); err != nil {
		t.Fatal(err)
	}
	if got != "/v2/orders/a%2Fb%3Fx=1" {
		t.Errorf("Square saw %q", got)
	}
}

func TestClientKey(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "127.0.0.1:4242"
	if got := clientKey(r); got != "127.0.0.1" {
		t.Errorf("direct = %q", got)
	}
	r.Header.Set("X-Client-IP", "203.0.113.9")
	if got := clientKey(r); got != "203.0.113.9" {
		t.Errorf("behind caddy = %q", got)
	}
	r.Header.Set("X-Client-IP", "2001:db8:1:2:3:4:5:6")
	if got := clientKey(r); got != "2001:db8:1:2::/64" {
		t.Errorf("v6 = %q, want the /64", got)
	}
}
