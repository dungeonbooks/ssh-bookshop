# API reference

Base URL: `https://api.dungeonbooks.com`. JSON in and out. No authentication.
Errors are `{"error": "..."}` with a status that says whose problem it is:
400 and 404 are the request's, 409 is the shelf having moved, 429 is too fast,
502 and 503 are between us and Square.

Rate limits are per client address. Reads are generous; checkout allows a
burst of three then one every five seconds, because each one creates an order.

The contract as OpenAPI 3.1: `https://shop.dungeonbooks.com/openapi.json`.

## GET /v1/books

The shelf.

```json
{
  "books": [
    {
      "isbn": "9780316568654",
      "title": "The Last Contract of Isako",
      "author": "Fonda Lee",
      "collection": "book club",
      "month": "2026-08",
      "featured": true,
      "format": "paperback",
      "pages": 528,
      "blurb": "...",
      "price_cents": 1999,
      "sellable": true,
      "tracked": true,
      "stock": 4
    },
    {
      "isbn": "9781250406811",
      "title": "The Poet Empress",
      "author": "Shen Tao",
      "collection": "book club",
      "month": "2026-03",
      "featured": false,
      "blurb": "...",
      "price_cents": 0,
      "sellable": false,
      "tracked": false,
      "buy_url": "https://bookshop.org/a/108216/9781250406811"
    }
  ],
  "shipping": {
    "region": "US",
    "note": "USPS Media Mail, priced by weight at checkout. Pickup at the shop is free."
  },
  "prices_as_of": "2026-09-09T13:02:11Z"
}
```

`stock` is only present when `tracked` is true. `buy_url` is only present when
`sellable` is false: there is exactly one way to buy each book. Prices are
re-read from the point of sale on every checkout, so `prices_as_of` moves.

## GET /v1/books/{isbn}

One entry, same shape. 404 when the ISBN is not on the shelf.

## POST /v1/checkout

Build the order and get the hosted checkout.

```json
{
  "items": [{"isbn": "9780316568654", "qty": 1}],
  "fulfilment": "ship",
  "idempotency_key": "optional-client-string"
}
```

`fulfilment` is required, `"pickup"` or `"ship"`. Quantities are capped at 10
per book. `idempotency_key` is optional: a retry with the same key returns the
same checkout rather than creating a second one. Send one if you might retry:
a 502 can mean the checkout was created and the reply was lost, and retrying
with the same key is how you get it back rather than leaving it to the sweeper.

Response, 201:

```json
{
  "order_id": "CAISENgvlJ6jLWAzERDzjyHVybY",
  "checkout_id": "ABCDEF",
  "checkout_url": "https://square.link/u/abc123",
  "fulfilment": "ship",
  "subtotal_cents": 1999,
  "shipping_cents": 500,
  "shipping_exact": true,
  "total_cents": 2499,
  "note": "Total before tax. Square collects the address and tax on the checkout page."
}
```

Hand `checkout_url` to the person. Then poll the order.

Refusals:

- 400: missing fulfilment, empty items, bad quantity, over the cap.
- 404: an ISBN not on the shelf.
- 409: the shelf moved. `"X just sold out"`, `"only 2 left of X"`, or
  `"X is now $35, not $30"`. When the price moved, `price_cents` carries the
  new one. When the book is not carried at all, `buy_url` says where to send
  the person instead.
- 503: the point of sale is not configured; browsing still works.

## GET /v1/orders/{order_id}

```json
{"order_id": "CAISENgvlJ6jLWAzERDzjyHVybY", "state": "awaiting_payment"}
```

`state` is `awaiting_payment` or `paid`. Poll every few seconds; give up after
an hour and tell the person the order id. 404 for an id we do not know.

## DELETE /v1/checkout/{checkout_id}

Abandon an unpaid checkout: the link stops working and the order is cancelled.
204 on success, 409 if it has already been paid, 404 if unknown.

## A whole purchase

```sh
curl -s https://api.dungeonbooks.com/v1/books | jq '.books[] | select(.sellable) | {isbn, title, price_cents}'

curl -s -X POST https://api.dungeonbooks.com/v1/checkout \
  -H 'content-type: application/json' \
  -d '{"items":[{"isbn":"9780316568654","qty":1}],"fulfilment":"pickup"}'
# show the person checkout_url, then:

curl -s https://api.dungeonbooks.com/v1/orders/<order_id>
```
