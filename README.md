# dungeonbooks ssh-bookshop

`ssh` in, see what our book club is reading. One shelf: the monthly Sci-Fi &
Fantasy picks, newest first, with the current month starred. Inspired by
[`terminal.shop`](https://www.terminal.shop) (`ssh terminal.shop`).

## How it works

- **[charmbracelet/wish](https://github.com/charmbracelet/wish)** runs the SSH
  server. No OpenSSH involved. The `bubbletea` middleware gives each session its
  own [Bubble Tea](https://github.com/charmbracelet/bubbletea) TUI wired to the
  PTY; `activeterm` requires a real terminal; `logging` logs connects.
- **Identity = SSH public key.** Any key connects (anonymous browse). The key's
  SHA256 fingerprint is captured per session (`main.go:teaHandler`) and is what
  you'd associate with an account at first purchase — same model as terminal.shop.
- **No inventory, no payment.** "Buy" hands off to a URL. Books we stock link to
  dungeonbooks.com (a sale beats a commission); once a pick sells out it falls
  back to `https://bookshop.org/a/{AffiliateID}/{isbn}`. The affiliate ID
  (`108216`) and URL shape match marty's `BookshopClient.get_buy_url`
  (`marty/src/tools/external/bookshop.py`).
- **This month is featured.** `featured()` matches a pick's `Month` against the
  current date, so the shelf opens on it and marks it with a star. Resolved per
  call, not at startup, so a long-running server rolls over on its own.

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
- `catalog.go` — `Book` type, buy-link resolution, affiliate helper.
- `bookclub.go` — the shelf itself, plus month formatting and `featured()`.

## Extending

- **Catalog → DB / API.** Replace the hardcoded `catalog` slice with a query or
  a feed fetch; keep a bookshop.org ISBN validation step for anything we don't
  stock ourselves.
- **Accounts.** Persist `fingerprint -> account` (Postgres/SQLite) to remember
  carts and order history across sessions.
- **DRM-free delivery.** Wish ships an `scp` middleware — free/owned titles could
  be pulled with `scp ssh://books/<isbn>.pdf .` over the same connection.
- **Paid checkout.** For non-affiliate sales, render a Stripe payment link the
  user opens in a browser to stay out of PCI scope.
