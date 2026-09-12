package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dungeonbooks/ssh-bookshop/internal/shopapi"
)

// fakeAPI is the server side of the shapes in shopapi, enough to drive every
// command once.
func fakeAPI(t *testing.T, paid bool) *httptest.Server {
	t.Helper()
	stock := 2
	shelf := shopapi.Shelf{Books: []shopapi.Book{
		{ISBN: "111", Title: "Pick", Author: "A", Collection: "book club", Featured: true, PriceCents: 1999, Sellable: true, Tracked: true, Stock: &stock},
		{ISBN: "222", Title: "Gone", Author: "B", Collection: "book club", BuyURL: "https://bookshop.org/a/108216/222"},
	}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/books", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(shelf)
	})
	mux.HandleFunc("GET /v1/books/{isbn}", func(w http.ResponseWriter, r *http.Request) {
		for _, b := range shelf.Books {
			if b.ISBN == r.PathValue("isbn") {
				json.NewEncoder(w).Encode(b)
				return
			}
		}
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(shopapi.Error{Message: "not on the shelf"})
	})
	mux.HandleFunc("POST /v1/checkout", func(w http.ResponseWriter, r *http.Request) {
		var req shopapi.CheckoutRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Items[0].ISBN == "222" {
			w.WriteHeader(409)
			json.NewEncoder(w).Encode(shopapi.Error{Message: "Gone is sold out here", BuyURL: "https://bookshop.org/a/108216/222"})
			return
		}
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(shopapi.Checkout{OrderID: "ORD1", CheckoutID: "LINK1", CheckoutURL: "https://square.link/u/x",
			Fulfilment: req.Fulfilment, SubtotalCents: 1999, ShippingCents: 500, TotalCents: 2499})
	})
	mux.HandleFunc("GET /v1/orders/{id}", func(w http.ResponseWriter, r *http.Request) {
		o := shopapi.Order{OrderID: r.PathValue("id"), State: shopapi.StateAwaitingPayment}
		if paid {
			o.State = shopapi.StatePaid
		}
		json.NewEncoder(w).Encode(o)
	})
	mux.HandleFunc("DELETE /v1/checkout/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestCommands(t *testing.T) {
	srv := fakeAPI(t, false)
	try := func(args ...string) (int, string, string) {
		var so, se bytes.Buffer
		code := run(append([]string{"--api", srv.URL}, args...), &so, &se)
		return code, so.String(), se.String()
	}

	if code, out, _ := try(); code != exitOK || !strings.Contains(out, "this month's pick: Pick") {
		t.Errorf("pick: %d %q", code, out)
	}
	if code, out, _ := try("books"); code != exitOK || !strings.Contains(out, "~ featured ~") || !strings.Contains(out, "sold out here") || !strings.Contains(out, "only 2 left") {
		t.Errorf("books: %d %q", code, out)
	}
	if code, out, _ := try("--json", "books"); code != exitOK || !strings.HasPrefix(out, "{") {
		t.Errorf("books --json: %d %q", code, out)
	}
	if code, out, _ := try("books", "111"); code != exitOK || !strings.Contains(out, "dungeon buy 111 --pickup") {
		t.Errorf("book: %d %q", code, out)
	}
	if code, _, se := try("books", "999"); code != exitUsage || !strings.Contains(se, "not on the shelf") {
		t.Errorf("missing book: %d %q", code, se)
	}
	if code, out, _ := try("buy", "111", "--ship", "--no-wait"); code != exitOK || !strings.HasPrefix(out, "https://square.link/u/x\n") || !strings.Contains(out, "books $19.99 + shipping $5 = $24.99") {
		t.Errorf("buy: %d %q", code, out)
	}
	if code, _, se := try("buy", "111"); code != exitUsage || !strings.Contains(se, "--pickup | --ship") {
		t.Errorf("buy without fulfilment: %d %q", code, se)
	}
	if code, _, se := try("buy", "111", "--pickup", "--ship"); code != exitUsage || !strings.Contains(se, "usage") {
		t.Errorf("buy with both: %d %q", code, se)
	}
	if code, _, se := try("--json", "buy", "111", "--pickup", "--ship"); code != exitUsage || !strings.HasPrefix(se, "{") || strings.Contains(se, "usage:") {
		t.Errorf("buy with both, json mode: %d %q (want a JSON error, no usage text)", code, se)
	}
	if code, _, se := try("--json", "buy", "111:x", "--pickup"); code != exitUsage || !strings.HasPrefix(se, "{") {
		t.Errorf("bad qty, json mode: %d %q", code, se)
	}
	if code, _, se := try("buy", "222", "--pickup"); code != exitUsage || !strings.Contains(se, "buy it at https://bookshop.org") {
		t.Errorf("buy sold out: %d %q", code, se)
	}
	if code, out, _ := try("order", "ORD1"); code != exitWaiting || !strings.Contains(out, "awaiting payment") {
		t.Errorf("order: %d %q", code, out)
	}
	if code, out, _ := try("cancel", "LINK1"); code != exitOK || !strings.Contains(out, "cancelled") {
		t.Errorf("cancel: %d %q", code, out)
	}
	if code, out, _ := try("books", "--help"); code != exitOK || !strings.Contains(out, "usage: dungeon") {
		t.Errorf("books --help: %d %q", code, out)
	}
	if code, out, _ := try("buy", "--help"); code != exitOK || !strings.Contains(out, "usage: dungeon buy") {
		t.Errorf("buy --help: %d %q", code, out)
	}
	if code, out, _ := try("skill"); code != exitOK || !strings.Contains(out, "name: dungeon-books") {
		t.Errorf("skill: %d %q", code, out)
	}
	if code, _, se := try("dance"); code != exitUsage || !strings.Contains(se, "usage:") {
		t.Errorf("unknown: %d %q", code, se)
	}
}

// TestBuyHonoursAShortWait: --wait shorter than the poll interval must still
// return when it runs out, not after the interval.
func TestBuyHonoursAShortWait(t *testing.T) {
	srv := fakeAPI(t, false)
	var so, se bytes.Buffer
	start := time.Now()
	code := run([]string{"--api", srv.URL, "buy", "111", "--pickup", "--wait", "300ms"}, &so, &se)
	if code != exitWaiting {
		t.Fatalf("code %d, stdout %q stderr %q", code, so.String(), se.String())
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("took %s for a 300ms wait", took)
	}
	if !strings.Contains(so.String(), "still waiting") {
		t.Errorf("stdout %q", so.String())
	}
}

func TestBuyWaitsForPayment(t *testing.T) {
	srv := fakeAPI(t, true)
	var so, se bytes.Buffer
	code := run([]string{"--api", srv.URL, "--json", "buy", "111", "--pickup"}, &so, &se)
	if code != exitOK {
		t.Fatalf("code %d, stderr %q", code, se.String())
	}
	// Two JSON documents: the checkout, then the paid order.
	dec := json.NewDecoder(&so)
	var checkout shopapi.Checkout
	var order shopapi.Order
	if err := dec.Decode(&checkout); err != nil || checkout.CheckoutURL == "" {
		t.Fatalf("first document: %v %+v", err, checkout)
	}
	if err := dec.Decode(&order); err != nil || order.State != shopapi.StatePaid {
		t.Fatalf("second document: %v %+v", err, order)
	}
}
