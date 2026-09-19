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
- **Square is the till.** Prices and stock come from the shop's Square catalog
  at boot, keyed by ISBN as the variation's UPC, and are re-read at checkout.
  Checkout builds the order on Square and hands the buyer a hosted payment
  link as a QR and a URL; the shop polls until it is paid. No card ever passes
  through this process. A pick we no longer carry links out to
  `https://bookshop.org/a/{AffiliateID}/{isbn}` instead; the affiliate ID
  (`108216`) and URL shape match marty's `BookshopClient.get_buy_url`.
- **The newest pick is featured.** `featured()` returns the top of the shelf, so
  the shop opens on it and marks it with a star. Announcing a pick is what makes
  it current, and that happens a little before its month starts, so adding it to
  `catalog` features it right away instead of it waiting for the 1st. Ordering
  the shelf newest-first is therefore load-bearing, not cosmetic.

## Run

```sh
go run .            # listens on 0.0.0.0:23234 (override with HOST/PORT)
# in another terminal:
ssh -p 23234 localhost
```

A host key is generated at `.ssh/id_ed25519` on first run.

Keys: `↑/↓` move · `enter`/`l` open · `h`/`esc` back · `/` filter · `q` quit.

## For agents

The same shelf and checkout are reachable without a terminal:

- **HTTP API** at `api.dungeonbooks.com`: list books, build a checkout, poll an
  order. No auth. `api.go`, contract in `deploy/site/openapi.json`.
- **`dungeon` CLI**, a thin client for it with `--json` everywhere:
  `go install github.com/dungeonbooks/ssh-bookshop/cmd/dungeon@latest`.
- **SSH command mode**: `ssh shop.dungeonbooks.com books` answers in JSON and
  exits. `command.go`.
- **Discovery**: `shop.dungeonbooks.com/llms.txt` links the docs, the OpenAPI
  document, and a skill file agents can install. The same docs are pages at
  [docs.dungeonbooks.com/agents](https://docs.dungeonbooks.com/agents), built
  from `deploy/site/docs` by the policies repo.

All of it stops at the hosted Square checkout URL; a human pays there.

## Deploy

`deploy/` has the systemd unit and the runbook: why this needs a plain VM rather
than managed hosting, how port 22 is freed by moving admin sshd to the tailnet,
and why the host key is permanent once anyone has connected.

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
- **Card on file.** A first purchase through the hosted link can seed a stored
  card on Square, after which repeat purchases need no link and no QR. Spiked
  and proven in sandbox; needs an identity for the HTTP and CLI paths.
