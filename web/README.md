# Steward Web

Go owns authenticated HTML, forms, routes and live updates. `web/src/` contains
CSS and the React/JavaScript enhancements. `internal/interaction/web/assets/`
contains the shipped fonts, artwork and generated bundles embedded by Go.

```sh
npm ci --prefix web --ignore-scripts
npm run build --prefix web
npm run check --prefix web
npm run lint --prefix web
```

Edit sources, then commit the generated bundles with them. CI rejects stale
outputs. `src/foundation.css` owns shared tokens/fonts; the other CSS modules own
their respective views. Setting layout and controls follow [DESIGN.md](../DESIGN.md). Current design and behavior live in the
[brand design](../docs/designs/branding.md) and
[channel design](../docs/designs/channels.md), not separate reconstruction briefs.

## Artwork sources and derived files

`artwork/manifest.json` records the two source cutouts and four active animation
atlases. Both themes and both frame sizes are used dynamically; they are not
retired variants. Sources under `artwork/source/` are not embedded or served.

After changing a production atlas, export README/favicon images from its first
frame with the pinned Chromium installed by `make setup`:

```sh
node web/artwork/icons.mjs
node web/artwork/icons.mjs --check
```

To rebuild animation frames, run `python3 web/artwork/bake-server.py`, open
`http://127.0.0.1:8789/bake.html` locally, then run
`python3 web/artwork/encode-frames.py` (requires `cwebp`). Inspect the result,
update the atlas hashes in the manifest, and export the static icons. This
optional artwork pipeline is not needed for ordinary builds.

[License notices](licenses/) preserve incorporated components and fonts.
The Go module remains `github.com/z-chenhao/assistant` for import compatibility;
the canonical repository is [z-chenhao/steward](https://github.com/z-chenhao/steward).
