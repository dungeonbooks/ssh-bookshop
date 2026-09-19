# The edge Worker

A Cloudflare Worker bound to one path, `/llms.txt`, on `dungeonbooks.com` and
`www.dungeonbooks.com`, the Square Online storefront. It exists because an
agent told to buy from `dungeonbooks.com` probes that path first, gets a 404,
and never learns that `docs.dungeonbooks.com` or the shop exist. Square's
templates cannot be edited past its own editor, so this runs at the edge.

The body is `llms.txt` in this directory: the storefront for humans, the docs
site for everything documented, the shop's API and CLI for agents. It is
bundled at deploy time (the `Text` rule in `wrangler.jsonc`), so the Worker
needs nothing from any origin. Anything else, and any exception, falls
through to the origin unchanged.

## It is dormant

`dungeonbooks.com` and `www.dungeonbooks.com` are proxied records in our
Cloudflare zone, but Square serves them through Cloudflare for SaaS, and a
SaaS provider's custom hostname outranks the zone's own DNS record for the
same name (Cloudflare's "hostname priority" rules). So our zone's Workers,
rules and everything else are skipped for those two hostnames; the request
goes straight to Square's zone. The certificates show it: `www` and the apex
serve single-name certificates issued for Square's setup, while
`api.dungeonbooks.com` serves our zone's wildcard. Confirmed 2026-09-19 by
deploying with the routes bound and watching `wrangler tail` stay silent
while Square answered every request.

Nothing at the edge can change that while Square holds the hostname. The
Worker stays deployed with its routes so that the moment the hostname is
ours again (the hosting migration, when Square's custom hostname goes away),
`/llms.txt` answers without a further deploy. Until then the storefront's
only levers are content edits in the Square editor: a footer link to
`docs.dungeonbooks.com`, head code if the plan allows it.

## Run

```sh
pnpm install
pnpm check          # wrangler types, tsc
pnpm dev            # http://127.0.0.1:8787/llms.txt
pnpm deploy         # needs `wrangler login`; binds the routes in wrangler.jsonc
```

## baseline/

Square's output captured on 2026-09-19 before any of this: `robots.square.txt`,
`sitemap.square.xml`, and `urls.csv`, every sitemap URL with its status code
(1807, all 200). The CSV is the input for the redirect map at the hosting
migration, when Square's `/product/<slug>/<id>` shapes stop existing.
