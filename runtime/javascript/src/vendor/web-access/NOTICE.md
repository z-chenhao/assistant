# Pinned Pi Web Access core

Source: https://github.com/nicobailon/pi-web-access
Package: `pi-web-access@0.39.0`, MIT, copyright Nico Bailon.
The original LICENSE is retained alongside the source.

`upstream.json` records SHA-256 hashes of the original package's top-level
TypeScript files, package manifest and license. `adaptations.json` records the
current hashes and reasons for the three patched source files. The provenance
unit test rejects unrecorded edits or additions. Refresh both manifests when
reviewing an upstream update; never treat an upstream update as automatically
approved.

Steward loads `web-tool-core.ts`, not Pi's extension entrypoint, TUI, curator
server or standalone MCP host. Unmodified adapters, extraction, routing, filters,
content finding and evidence construction are reused directly. Host adaptations:

- `utils.ts`: current-account requests use the fenced, budgeted Steward Provider
  Bridge. Account tokens remain outside Node.
- `ssrf-protection.ts`: retain upstream preflight and pin public-page DNS at the
  actual connection, including configured endpoint/redirect policies.
- `extract.ts`: decode/resize images through Steward's bounded Go media adapter;
  retain complete extracted PDF Markdown in scoped result state.

The sibling `web-access*.mjs` files are Steward-owned integration code. They
replace Pi session storage with invocation scratch roots and scoped Artifacts,
export GitHub snapshots, accept local videos only from granted Artifacts, and
hand summary-review drafts back to the user. They do not import Pi/Codex
credential files, browser profiles or mutable session state. Only the current
frozen Steward model is exposed for page answers/summaries. Standalone upstream
providers require explicit SecretStore environment references. Browser-cookie
providers and Pi's interactive curator are not wired into Steward.

Dependency versions are pinned by Steward's package-lock.json. In particular,
Undici is patched to 8.12.4 rather than retaining the vulnerable minimum declared
by upstream. The installed npm dependency trees retain their original notices.
