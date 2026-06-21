#!/usr/bin/env bash
# Rebuild the embedded SPA (fbembed/dist.zip) from the frontend.
#
# Run from the fbembed package dir (go generate does this):
#   go generate ./fbembed
#
# Produces a trimmed archive: raw .js are dropped (File Browser's static
# handler only serves the pre-gzipped .js.gz), as are non-JS .gz (non-JS
# assets are served raw) and stray .gitkeep/.map files.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd "$here/.." && pwd)"

echo "fbembed: building frontend…"
( cd "$root/frontend" && pnpm install --frozen-lockfile && pnpm run build )

stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT
cp -r "$root/frontend/dist/." "$stage/"

( cd "$stage"
  find . -name '*.js' -delete
  find . -name '*.gz' ! -name '*.js.gz' -delete
  find . -name '.gitkeep' -delete
  find . -name '*.map' -delete
)

rm -f "$here/dist.zip"
( cd "$stage" && zip -X -q -r -9 "$here/dist.zip" . )
echo "fbembed: wrote dist.zip ($(du -h "$here/dist.zip" | cut -f1))"
