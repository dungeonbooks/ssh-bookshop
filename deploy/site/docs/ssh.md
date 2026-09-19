# The SSH shop

```
ssh shop.dungeonbooks.com
```

No port flag, no client software. Any key connects, and no key connects too:
without one you browse anonymously and can still buy. The shop never sees your
private key; SSH proves you hold it by signing a challenge.

Host key fingerprint, to check on first connect:

```
SHA256:EXv5X+C23tnnR0ftxRP/Iq+3z3rL1ezcpbAy9S+zXLg
```

Keys inside: arrows or `j`/`k` move, `enter` opens, `+`/`-` change quantity,
`c` cart, `s` shop, `a` account, `q` quits. Checkout shows a QR code and a URL
for Square's hosted page; the shop watches for the payment and says thank you.

## Command mode

The API is also reachable over SSH, for anyone with a client and no browser.
Send a command and the shop answers in JSON and exits, no terminal needed:

```sh
ssh shop.dungeonbooks.com books
ssh shop.dungeonbooks.com books 9780316568654
ssh shop.dungeonbooks.com buy 9780316568654:2 --ship
ssh shop.dungeonbooks.com order <order_id>
ssh shop.dungeonbooks.com cancel <checkout_id>
```

Same shapes and same exit codes as the dungeon CLI: 0 done, 1 something
failed or you were rate limited, 2 malformed or a request the shop turned
down (400, 404, 409), 3 the order is still waiting. Errors are JSON on stderr. `buy` returns the checkout at once; poll
with `order`. Checkouts share the API's rate limit: a burst of three, then one
every five seconds.

Automation should prefer the HTTP API. Sandboxes often have no SSH client, no
key, and a host-key prompt that blocks a script. If you must script SSH, pin
the host key rather than accepting whatever is offered on first connection:
this line is the key behind the fingerprint above, ready for `known_hosts`.

```
shop.dungeonbooks.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIITy6szP54vkrtWfCrSyQ6uFYL9JGjRWiqgzK3yVm9ux
```

## What is stored

Nothing. There is no database. The logs do not record your key, address, or
what you bought, and Square handles the card. Rate limiting works from
in-memory buckets and forgets you when you leave.
