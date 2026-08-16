#!/usr/bin/env bash
# Renders each ADRs/*.md to a standalone HTML page with its ```mermaid
# blocks rendered as SVG images (linked, not inlined). Uses mmdc
# (mermaid-cli) to extract/render diagrams and rewrite the markdown to
# reference them, then `marked` to render the rest of the markdown. Both
# run via npx — no extra tools to install.
#
# Output: ADRs/html/<adr-basename>/index.html, with the diagram SVGs
# alongside it in the same folder.
set -euo pipefail

if ! command -v npx >/dev/null 2>&1; then
	echo "error: npx not found (requires Node.js) — see https://nodejs.org" >&2
	exit 1
fi

adr_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/ADRs"

for md in "$adr_dir"/*.md; do
	[ -e "$md" ] || continue
	base="$(basename "$md" .md)"
	outdir="$adr_dir/html/$base"
	mkdir -p "$outdir"

	echo "Rendering $outdir/index.html"

	# mmdc extracts + renders mermaid blocks and rewrites the markdown to
	# reference the generated images (page-1.svg, page-2.svg, ...).
	npx -y @mermaid-js/mermaid-cli -q -i "$md" -o "$outdir/page.md"
	body="$(npx -y marked -i "$outdir/page.md")"
	rm -f "$outdir/page.md"

	cat >"$outdir/index.html" <<HTML
<!doctype html>
<html>
<head>
<meta charset="utf-8">
<title>${base}</title>
<style>
  body { max-width: 860px; margin: 2rem auto; padding: 0 1rem; font-family: -apple-system, sans-serif; line-height: 1.5; }
  code { background: #f0f0f0; padding: 0.1em 0.3em; border-radius: 3px; }
  pre code { display: block; padding: 0.75em; overflow-x: auto; }
  img { max-width: 100%; }
  img[alt="diagram"] { display: block; max-width: 480px; margin: 1.5rem auto; }
</style>
</head>
<body>
$body
</body>
</html>
HTML
done
