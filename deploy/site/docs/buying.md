# How buying works

Dungeon Books is a bookstore at 115 Brunswick St, Jersey City, NJ. The shelf
here is the monthly science fiction and fantasy book club picks, newest first.
The current pick is marked featured.

## What can be bought

A book is `sellable` when we have it on the shelf. Its price is in cents, from
our point of sale, and `stock` is present when we keep a count for it. A book
we no longer carry is not sellable and carries a `buy_url` to Bookshop.org
instead; that link is an affiliate link, and the commission supports the shop.

## Fulfilment

Every order chooses one, before the order exists:

- `pickup`: collect at the shop. Free. We email when it is ready.
- `ship`: USPS Media Mail, US only. Priced by weight, rounded up to the whole
  dollar; a flat $8 when a book is missing its weight. 6 to 12 business days.

Never guess this for a person. Ask.

## Payment

Payment happens on a hosted Square page. The API and CLI build the order and
return a `checkout_url`; a human opens it, enters card and address, and Square
collects tax there. Totals from the API are before tax. Nothing here takes or
stores a card.

An unpaid checkout stays open for a day, then is swept. Cancel it sooner with
the API or `dungeon cancel` if the person changes their mind.

## After payment

Poll the order until its state is `paid`. Square emails the receipt. Shipped
orders leave Jersey City within 3 to 5 business days, with a tracking number
by email. Pickup orders are set aside at the counter and we email when ready.

Questions about an order: hello@dungeonbooks.com.
