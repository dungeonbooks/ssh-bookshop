# ssh-bookshop, for coding agents

A Go SSH bookshop (Charm's Wish and Bubble Tea) with a Square checkout, plus an
HTTP API and a CLI so other agents can buy from it. This file is for agents
working on the code. Agents buying from the shop want `deploy/site/llms.txt`.

## Build and test

```sh
go build ./...                      # server in ., CLI in ./cmd/dungeon
go test -count=1 -race ./...        # what CI runs; -race is the point
gofmt -l . && go vet ./...          # CI fails on either
```

Run the server locally with `go run .` and a `.env` holding sandbox Square
credentials (`SQUARE_ENVIRONMENT=sandbox`, `SQUARE_ACCESS_TOKEN`). It listens
for SSH on 23234 and serves the API on 127.0.0.1:8080. `scripts/seed-sandbox.sh`
fills a sandbox catalog with the shelf.

## Layout

- `main.go`: Wish server and middleware chain. Middleware runs bottom-up.
- `model.go`, `update.go`, `render.go`, `shop.go`, `checkout.go`, `nav.go`,
  `account.go`: the TUI.
- `cart.go`, `shipping.go`: cart maths and Media Mail pricing, no UI.
- `square.go`, `squarecatalog.go`, `squareorder.go`, `sweep.go`: everything
  that talks to Square. `createLink` is the only place money is asked for.
- `api.go`, `api_limit.go`: the HTTP API. `command.go`: the same over SSH.
- `internal/shopapi`: wire types and client shared by server and CLI.
- `cmd/dungeon`: the CLI. `skill.md` beside it is embedded and also copied to
  `deploy/site/skill.md`; a test checks the copies match.
- `deploy/`: Caddyfile, systemd unit, runbook, and the static site including
  the agent-facing files.

## Rules that are not obvious from the code

- The shelf (`catalog`) is read by every session concurrently. Never write to
  it after boot; sessions and the API keep their own overlays of what Square
  said (`model.fresh`, `apiShop.fresh`).
- Square payment links never expire. Anything that creates one must either
  watch it or make it deletable; the sweeper is the backstop, not the plan.
- Logs never carry an address, key, or account name. The site says the shop
  stores nothing and that has to be true of journald too.
- `openapi.json` is hand-written and a test holds it to `apiRoutes`. Add a
  route, document it.
- Prose in this repo avoids em dashes.
