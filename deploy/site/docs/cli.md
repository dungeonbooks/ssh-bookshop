# dungeon CLI

The shop as commands. A thin client for the HTTP API: with `--json`, every
command prints the API's response unchanged, so the API reference describes
the shapes.

## Install

```sh
go install github.com/dungeonbooks/ssh-bookshop/cmd/dungeon@latest
```

Needs Go 1.26 or later. No config file, no login.

## Commands

```
dungeon                       this month's pick and how to buy it
dungeon books                 the shelf
dungeon books <isbn>          one book
dungeon buy <isbn>[:qty]... --pickup|--ship
                              build a checkout, print the URL, wait for payment
dungeon order <order_id>      whether an order has been paid
dungeon cancel <checkout_id>  abandon an unpaid checkout
dungeon skill                 print the agent skill for this tool
dungeon version
```

Global flags: `--json` for machine output, `--api <url>` or `$DUNGEON_API` to
point somewhere other than `https://api.dungeonbooks.com`.

`--help` works at every level and is the complete documentation.

## buy

```
dungeon buy 9780316568654 --ship
dungeon buy 9780316568654:2 9780593818947 --pickup --no-wait
```

Prints the checkout URL first, on its own line, so a caller that captures
output has it even if it kills the wait. Then polls until Square reports
payment.

- `--no-wait`: print the checkout and return at once.
- `--wait 30m`: how long to wait (default one hour).
- `--key <string>`: idempotency key; a retry with the same key gets the same link.
- `--keep`: on ctrl-c, leave the checkout live instead of cancelling it.

## Exit codes

| code | meaning |
|---|---|
| 0 | done, or paid |
| 1 | the shop refused, or something failed |
| 2 | bad usage, or a request the API turned down (400, 404, 409) |
| 3 | waiting: the order is not paid yet, or the wait ran out |

## For agents

`dungeon skill` prints a skill file; save it as `.claude/skills/dungeon-books/SKILL.md`
or the equivalent for your tool. The same file is at
`https://shop.dungeonbooks.com/skill.md`.

Never choose pickup or shipping for a person. Ask.
