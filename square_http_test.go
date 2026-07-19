package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeSquare stands in for the API. Handlers are keyed by path prefix so a test
// only has to describe the calls it cares about.
func fakeSquare(t *testing.T, routes map[string]func(w http.ResponseWriter, r *http.Request)) *squareClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Longest prefix wins. Map iteration is randomised, so picking the
		// first match would make any test with overlapping routes flaky.
		best, bestLen := "", -1
		for prefix := range routes {
			if strings.HasPrefix(r.URL.Path, prefix) && len(prefix) > bestLen {
				best, bestLen = prefix, len(prefix)
			}
		}
		if bestLen >= 0 {
			w.Header().Set("Content-Type", "application/json")
			routes[best](w, r)
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{}`)
	}))
	t.Cleanup(srv.Close)
	return &squareClient{token: "test", locationID: "L1", baseURL: srv.URL}
}

// TestCallReportsErrorsInA200Body is the branch the comment on call() calls out:
// Square reports failures in a 200-shaped body as often as by status code, so a
// success status with an errors array must not be read as success.
func TestCallReportsErrorsInA200Body(t *testing.T) {
	c := fakeSquare(t, map[string]func(http.ResponseWriter, *http.Request){
		"/v2/": func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `{"errors":[{"code":"UNAUTHORIZED","detail":"bad token"}]}`)
		},
	})
	err := c.call(context.Background(), http.MethodGet, "/v2/anything", nil, &struct{}{})
	if err == nil {
		t.Fatal("err = nil on a 200 carrying an errors array")
	}
	if !strings.Contains(err.Error(), "UNAUTHORIZED") {
		t.Errorf("err = %v, want the Square code in it", err)
	}
}

func TestCallReportsHTTPStatus(t *testing.T) {
	c := fakeSquare(t, map[string]func(http.ResponseWriter, *http.Request){
		"/v2/": func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, `{}`)
		},
	})
	if err := c.call(context.Background(), http.MethodGet, "/v2/x", nil, &struct{}{}); err == nil {
		t.Fatal("err = nil on HTTP 500")
	}
}

func TestCallSendsAuthAndVersion(t *testing.T) {
	var gotAuth, gotVersion string
	c := fakeSquare(t, map[string]func(http.ResponseWriter, *http.Request){
		"/v2/": func(w http.ResponseWriter, r *http.Request) {
			gotAuth, gotVersion = r.Header.Get("Authorization"), r.Header.Get("Square-Version")
			io.WriteString(w, `{}`)
		},
	})
	if err := c.call(context.Background(), http.MethodGet, "/v2/x", nil, &struct{}{}); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer test" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotVersion != squareVersion {
		t.Errorf("Square-Version = %q, want %q", gotVersion, squareVersion)
	}
}

// TestStock is the safety-critical one: it decides whether the shop will take
// money for a book. Untracked defaults to true, which means sellable, so every
// path that fails to establish a real count must not land there by accident.
func TestStock(t *testing.T) {
	const id = "VAR1"
	for _, tc := range []struct {
		name          string
		counts        string
		wantQty       int
		wantUntracked bool
	}{
		{
			name:          "a real count is tracked",
			counts:        `{"catalog_object_id":"VAR1","state":"IN_STOCK","quantity":"7"}`,
			wantQty:       7,
			wantUntracked: false,
		},
		{
			name:          "zero is tracked and empty, not untracked",
			counts:        `{"catalog_object_id":"VAR1","state":"IN_STOCK","quantity":"0"}`,
			wantQty:       0,
			wantUntracked: false,
		},
		{
			// The guard the comment in stock() exists for: falling through to
			// untracked here would let the shop sell a book it may not have.
			name:          "an unparsable quantity is tracked and empty",
			counts:        `{"catalog_object_id":"VAR1","state":"IN_STOCK","quantity":"not a number"}`,
			wantQty:       0,
			wantUntracked: false,
		},
		{
			name:          "a non IN_STOCK state leaves it untracked",
			counts:        `{"catalog_object_id":"VAR1","state":"SOLD","quantity":"3"}`,
			wantQty:       0,
			wantUntracked: true,
		},
		{
			name:          "no count at all leaves it untracked",
			counts:        "",
			wantQty:       0,
			wantUntracked: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := fakeSquare(t, map[string]func(http.ResponseWriter, *http.Request){
				"/v2/inventory/counts/batch-retrieve": func(w http.ResponseWriter, r *http.Request) {
					io.WriteString(w, `{"counts":[`+tc.counts+`]}`)
				},
			})
			qty, untracked, err := c.stock(context.Background(), []string{id})
			if err != nil {
				t.Fatal(err)
			}
			if qty[id] != tc.wantQty {
				t.Errorf("qty = %d, want %d", qty[id], tc.wantQty)
			}
			if untracked[id] != tc.wantUntracked {
				t.Errorf("untracked = %v, want %v", untracked[id], tc.wantUntracked)
			}
			// The pairing that actually decides whether money changes hands.
			t.Logf("sellable = %v", sellable(untracked[id], qty[id]))
		})
	}
}

func TestStockWithNoIDsMakesNoRequest(t *testing.T) {
	c := fakeSquare(t, map[string]func(http.ResponseWriter, *http.Request){})
	qty, untracked, err := c.stock(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(qty) != 0 || len(untracked) != 0 {
		t.Errorf("qty=%v untracked=%v, want both empty", qty, untracked)
	}
}

// TestPaid pins what counts as settled. A tender means money moved even while
// the order still reads OPEN, which is what the sweeper relies on to avoid
// cancelling a real sale.
func TestPaid(t *testing.T) {
	for _, tc := range []struct {
		name  string
		order string
		want  bool
	}{
		{"completed", `{"state":"COMPLETED","tenders":[]}`, true},
		{"open with a tender", `{"state":"OPEN","tenders":[{"id":"T1"}]}`, true},
		{"draft with nothing", `{"state":"DRAFT","tenders":[]}`, false},
		{"open with nothing", `{"state":"OPEN"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := fakeSquare(t, map[string]func(http.ResponseWriter, *http.Request){
				"/v2/orders/": func(w http.ResponseWriter, r *http.Request) {
					io.WriteString(w, `{"order":`+tc.order+`}`)
				},
			})
			got, err := c.paid(context.Background(), "O1")
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("paid = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestCreateLinkCapturesTheLinkID guards the handle the sweeper needs. Dropping
// it is what left abandoned links undeletable.
func TestCreateLinkCapturesTheLinkID(t *testing.T) {
	c := fakeSquare(t, map[string]func(http.ResponseWriter, *http.Request){
		"/v2/inventory/counts/batch-retrieve": func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `{"counts":[{"catalog_object_id":"VAR1","state":"IN_STOCK","quantity":"5"}]}`)
		},
		"/v2/catalog/search-catalog-items": func(w http.ResponseWriter, r *http.Request) {
			// lookup re-confirms by UPC, so the ISBN has to match exactly.
			io.WriteString(w, `{"items":[{"item_data":{"variations":[
				{"id":"VAR1","item_variation_data":{"upc":"1","price_money":{"amount":1000,"currency":"USD"}}}
			]}}]}`)
		},
		"/v2/online-checkout/payment-links": func(w http.ResponseWriter, r *http.Request) {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decoding payment-link body: %v", err)
			}
			if body["idempotency_key"] != "IDEM1" {
				t.Errorf("idempotency_key = %v, want IDEM1", body["idempotency_key"])
			}
			io.WriteString(w, `{"payment_link":{"id":"LINK1","url":"https://square.link/u/X","order_id":"ORD1"}}`)
		},
	})
	items := []cartItem{{isbn: "1", variationID: "VAR1", title: "A Book", cents: 1000, qty: 1}}

	out, _, err := c.createLink(context.Background(), items, "IDEM1")
	if err != nil {
		t.Fatal(err)
	}
	if out.LinkID != "LINK1" {
		t.Errorf("LinkID = %q, want LINK1: the sweeper cannot delete without it", out.LinkID)
	}
	if out.OrderID != "ORD1" || out.URL == "" {
		t.Errorf("got %+v", out)
	}
}

// TestSweepLinks covers what the live sandbox run could only show once.
func TestSweepLinks(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	old := now.Add(-48 * time.Hour).Format(time.RFC3339)
	recent := now.Add(-1 * time.Hour).Format(time.RFC3339)

	var deletedIDs []string
	routes := map[string]func(http.ResponseWriter, *http.Request){
		"/v2/online-checkout/payment-links": func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodDelete {
				deletedIDs = append(deletedIDs, strings.TrimPrefix(r.URL.Path, "/v2/online-checkout/payment-links/"))
				io.WriteString(w, `{}`)
				return
			}
			io.WriteString(w, `{"payment_links":[
				{"id":"OLD_UNPAID","order_id":"O_UNPAID","created_at":"`+old+`"},
				{"id":"OLD_PAID","order_id":"O_PAID","created_at":"`+old+`"},
				{"id":"RECENT","order_id":"O_RECENT","created_at":"`+recent+`"}
			]}`)
		},
		"/v2/orders/": func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "O_PAID") {
				io.WriteString(w, `{"order":{"state":"OPEN","tenders":[{"id":"T1"}]}}`)
				return
			}
			io.WriteString(w, `{"order":{"state":"DRAFT","tenders":[]}}`)
		},
	}

	c := fakeSquare(t, routes)
	deleted, kept, err := c.sweepLinks(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 || kept != 2 {
		t.Errorf("deleted=%d kept=%d, want 1 and 2", deleted, kept)
	}
	if len(deletedIDs) != 1 || deletedIDs[0] != "OLD_UNPAID" {
		t.Errorf("deleted %v, want only OLD_UNPAID: a paid or recent link was cancelled", deletedIDs)
	}
}

// TestSweepLinksFollowsCursor guards the pagination: stopping at page one would
// silently leave older links behind forever.
func TestSweepLinksFollowsCursor(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	old := now.Add(-48 * time.Hour).Format(time.RFC3339)

	var deleted []string
	c := fakeSquare(t, map[string]func(http.ResponseWriter, *http.Request){
		"/v2/online-checkout/payment-links": func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodDelete {
				deleted = append(deleted, strings.TrimPrefix(r.URL.Path, "/v2/online-checkout/payment-links/"))
				io.WriteString(w, `{}`)
				return
			}
			if r.URL.Query().Get("cursor") == "" {
				io.WriteString(w, `{"payment_links":[{"id":"P1","order_id":"O1","created_at":"`+old+`"}],"cursor":"NEXT"}`)
				return
			}
			io.WriteString(w, `{"payment_links":[{"id":"P2","order_id":"O2","created_at":"`+old+`"}]}`)
		},
		"/v2/orders/": func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `{"order":{"state":"DRAFT","tenders":[]}}`)
		},
	})

	if _, _, err := c.sweepLinks(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 2 {
		t.Errorf("deleted %v, want both pages swept", deleted)
	}
}

// shelfRoutes is a fake Square with one priced, stocked book and one it has
// never heard of.
func shelfRoutes(t *testing.T, lookupDelay time.Duration) map[string]func(http.ResponseWriter, *http.Request) {
	return map[string]func(http.ResponseWriter, *http.Request){
		"/v2/locations": func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `{"locations":[
				{"id":"L0","status":"INACTIVE","capabilities":["CREDIT_CARD_PROCESSING"]},
				{"id":"L1","status":"ACTIVE","capabilities":["CREDIT_CARD_PROCESSING"]}
			]}`)
		},
		"/v2/catalog/search-catalog-items": func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(lookupDelay)
			var body struct {
				TextFilter string `json:"text_filter"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decoding catalog search body: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if body.TextFilter != "known" {
				io.WriteString(w, `{"items":[]}`)
				return
			}
			io.WriteString(w, `{"items":[{"item_data":{"variations":[
				{"id":"VAR1","item_variation_data":{"upc":"known","price_money":{"amount":2200,"currency":"USD"}}}
			]}}]}`)
		},
		"/v2/inventory/counts/batch-retrieve": func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `{"counts":[{"catalog_object_id":"VAR1","state":"IN_STOCK","quantity":"3"}]}`)
		},
	}
}

func TestLoadIntoPricesWhatSquareKnows(t *testing.T) {
	c := fakeSquare(t, shelfRoutes(t, 0))
	books := []Book{{ISBN: "known"}, {ISBN: "unknown"}}

	priced, err := c.loadInto(context.Background(), books)
	if err != nil {
		t.Fatal(err)
	}
	if priced != 1 {
		t.Errorf("priced = %d, want 1", priced)
	}
	if books[0].Cents != 2200 || books[0].Stock != 3 || !books[0].Sellable {
		t.Errorf("known book = %+v, want priced, stocked and sellable", books[0])
	}
	if books[1].Cents != 0 || books[1].Sellable {
		t.Errorf("unknown book = %+v, want unpriced and not sellable", books[1])
	}
	if c.locationID != "L1" {
		t.Errorf("locationID = %q, want L1", c.locationID)
	}
}

// TestLoadIntoLooksUpConcurrently guards the boot delay. Sequentially this was
// one round trip per book with the listener not yet accepting connections, so a
// slow Square meant an unreachable shop rather than a degraded one.
func TestLoadIntoLooksUpConcurrently(t *testing.T) {
	const delay = 80 * time.Millisecond
	books := make([]Book, 8)
	for i := range books {
		books[i] = Book{ISBN: "unknown"}
	}

	c := fakeSquare(t, shelfRoutes(t, delay))
	start := time.Now()
	if _, err := c.loadInto(context.Background(), books); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)

	sequential := delay * time.Duration(len(books))
	if elapsed >= sequential {
		t.Errorf("took %v, sequential would be %v: lookups are not overlapping", elapsed, sequential)
	}
	t.Logf("%d books in %v (sequential would be %v)", len(books), elapsed.Round(time.Millisecond), sequential)
}

// TestLoadIntoKeepsWhatItGot: one bad lookup must not throw away the rest, or a
// single flaky response empties the shelf.
func TestLoadIntoKeepsWhatItGot(t *testing.T) {
	routes := shelfRoutes(t, 0)
	routes["/v2/catalog/search-catalog-items"] = func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			TextFilter string `json:"text_filter"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decoding catalog search body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		switch body.TextFilter {
		case "known":
			io.WriteString(w, `{"items":[{"item_data":{"variations":[
				{"id":"VAR1","item_variation_data":{"upc":"known","price_money":{"amount":2200,"currency":"USD"}}}
			]}}]}`)
		default:
			io.WriteString(w, `{"errors":[{"code":"INTERNAL_SERVER_ERROR","detail":"boom"}]}`)
		}
	}
	c := fakeSquare(t, routes)
	books := []Book{{ISBN: "known"}, {ISBN: "explodes"}}

	priced, err := c.loadInto(context.Background(), books)
	if err == nil {
		t.Error("err = nil, want the failed lookup reported")
	}
	if priced != 1 {
		t.Errorf("priced = %d, want 1: the good book was thrown away with the bad", priced)
	}
	if books[0].Cents != 2200 {
		t.Errorf("known book lost its price: %+v", books[0])
	}
	// The half-state worth guarding: returning before the stock call would
	// leave the priced book Sellable false, so the shelf shows a price and
	// then refuses to sell it.
	if !books[0].Sellable || books[0].Stock != 3 {
		t.Errorf("known book = %+v, want stocked and sellable despite the other lookup failing", books[0])
	}
}
