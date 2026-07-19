#!/usr/bin/env bash
# Seed the Square sandbox catalog with the book club shelf, so checkout can be
# exercised end to end without touching the real shop.
#
# The ISBN goes in as the variation's UPC, because that is how the shop's real
# catalog is keyed and how lookup() finds a book. Prices mirror production.
#
# The Poet Empress is deliberately left out: it is sold out in the real shop, so
# omitting it here keeps the sold-out path testable.
#
#   ./scripts/seed-sandbox.sh
set -euo pipefail

cd "$(dirname "$0")/.."
# shellcheck disable=SC1091
set -a && . ./.env && set +a

if [ "${SQUARE_ENVIRONMENT:-}" != "sandbox" ]; then
    echo "refusing to seed: SQUARE_ENVIRONMENT is '${SQUARE_ENVIRONMENT:-unset}', not sandbox" >&2
    exit 1
fi

host=connect.squareupsandbox.com

# isbn|title|cents
books=(
    "9780593818947|Daughter of Crows by Mark Lawrence (Hardcover)|3000"
    "9781250376794|Sublimation by Isabel J. Kim (Hardcover) SIGNED|2899"
    "9781967967063|Burn the Sea by Mona Tewari (Paperback)|1995"
    "9798991475273|Blessed Is the Rot by Sheri Singerling (Paperback)|1499"
    "9781984820716|The Tainted Cup by Robert Jackson Bennett (Paperback)|2000"
    "9781250380968|Saltcrop by Yume Kitasei (Hardcover)|3099"
)

objects=""
for row in "${books[@]}"; do
    IFS='|' read -r isbn title cents <<<"$row"
    [ -n "$objects" ] && objects+=","
    objects+=$(
        cat <<JSON
{
  "type": "ITEM",
  "id": "#item_$isbn",
  "present_at_all_locations": true,
  "item_data": {
    "name": "$title",
    "variations": [{
      "type": "ITEM_VARIATION",
      "id": "#var_$isbn",
      "present_at_all_locations": true,
      "item_variation_data": {
        "item_id": "#item_$isbn",
        "name": "Regular",
        "pricing_type": "FIXED_PRICING",
        "upc": "$isbn",
        "price_money": {"amount": $cents, "currency": "USD"}
      }
    }]
  }
}
JSON
    )
done

curl -sS -X POST "https://$host/v2/catalog/batch-upsert" \
    -H "Authorization: Bearer $SQUARE_ACCESS_TOKEN" \
    -H "Content-Type: application/json" \
    -H "Square-Version: 2025-01-23" \
    -d "{\"idempotency_key\": \"seed-$(date +%s)\", \"batches\": [{\"objects\": [$objects]}]}" |
    python3 -c "
import json, sys
d = json.load(sys.stdin)
if 'errors' in d:
    print('failed:', json.dumps(d['errors'], indent=2), file=sys.stderr)
    raise SystemExit(1)
print(f\"seeded {len(d.get('objects', []))} objects\")
"
