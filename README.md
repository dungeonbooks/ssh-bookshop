# dungeonbooks ssh-bookshop

`ssh` in, browse a curated shelf of seminal computer science books and the Stripe
Press collection, buy via Bookshop.org affiliate links. Inspired by
[`terminal.shop`](https://www.terminal.shop) (`ssh terminal.shop`).

## How it works

- **[charmbracelet/wish](https://github.com/charmbracelet/wish)** runs the SSH
  server. No OpenSSH involved. The `bubbletea` middleware gives each session its
  own [Bubble Tea](https://github.com/charmbracelet/bubbletea) TUI wired to the
  PTY; `activeterm` requires a real terminal; `logging` logs connects.
- **Identity = SSH public key.** Any key connects (anonymous browse). The key's
  SHA256 fingerprint is captured per session (`main.go:teaHandler`) and is what
  you'd associate with an account at first purchase — same model as terminal.shop.
- **Affiliate model, no inventory, no payment.** "Buy" hands off to
  `https://bookshop.org/a/{AffiliateID}/{isbn}`. The affiliate ID (`108216`) and
  URL shape match marty's `BookshopClient.get_buy_url`
  (`marty/src/tools/external/bookshop.py`).
- **Validated links.** Every ISBN in `catalog.go` was checked against
  `bookshop.org/book/{isbn}` (308 = live), the same way marty's client validates,
  so affiliate links don't 404.
- **Free titles** (SICP, OSTEP) are openly licensed and shown with a direct read
  link instead of a buy link.

## Run

```sh
go run .            # listens on 0.0.0.0:23234 (override with HOST/PORT)
# in another terminal:
ssh -p 23234 localhost
```

A host key is generated at `.ssh/id_ed25519` on first run.

Keys: `↑/↓` move · `enter`/`l` open · `h`/`esc` back · `/` filter · `q` quit.

## Files

- `main.go` — Wish server, middleware stack, per-session key capture.
- `model.go` — Bubble Tea model: list view + detail view.
- `catalog.go` — curated catalog, affiliate-link helper, validated ISBNs.

## Extending

- **Catalog → DB / API.** Replace the hardcoded `catalog` slice with a query or
  an OpenLibrary/Google Books fetch; keep the bookshop.org ISBN validation step.
- **Accounts.** Persist `fingerprint -> account` (Postgres/SQLite) to remember
  carts and order history across sessions.
- **DRM-free delivery.** Wish ships an `scp` middleware — free/owned titles could
  be pulled with `scp ssh://books/<isbn>.pdf .` over the same connection.
- **Paid checkout.** For non-affiliate sales, render a Stripe payment link the
  user opens in a browser to stay out of PCI scope.
