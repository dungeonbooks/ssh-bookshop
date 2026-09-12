---
name: dungeon-books
description: Browse and buy books from Dungeon Books, an independent science fiction, fantasy, and RPG bookstore in Jersey City, using the dungeon CLI. Payment happens on a Square page the human opens; the CLI hands you the link.
---

# Dungeon Books

`dungeon` is on PATH. Navigate with `--help`; do not pre-map the tree.

```
dungeon --help          # root: every command
dungeon buy --help      # one command: flags and exit codes
```

The purchase flow is: `dungeon books` to see the shelf, `dungeon buy <isbn>
--pickup` or `--ship` to build a checkout, then show the human the printed URL.
The command waits and exits 0 once Square reports payment. Pass `--json` for
machine output and `--no-wait` to get the link and return at once.

Never invent a fulfilment choice. Ask the human whether they want pickup in
Jersey City or US shipping before running `buy`. Shipping is US only.

If `dungeon` is not installed: `go install github.com/dungeonbooks/ssh-bookshop/cmd/dungeon@latest`,
or call the HTTP API directly, documented at https://shop.dungeonbooks.com/llms.txt
