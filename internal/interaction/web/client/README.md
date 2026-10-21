# Erii Web components

This is a locally bundled presentation layer for the existing Go Web server.
The conversation layout follows the requested ChatGPT-style neutral sidebar,
centered thread and rounded composer. React enhances the escaped server HTML;
the existing forms and semantic fallback remain usable without JavaScript.

## Pinned source and changes

| Source | Revision | Reused code or asset | Local changes |
| --- | --- | --- | --- |
| [ad-agent](https://github.com/z-chenhao/ad-agent) | `e43216ffa7bd686b496a15f1d8a99ac180857caa` | `web/src/components/ui/{button,card,textarea}.tsx`, `web/src/lib/utils.ts`, neutral style tokens | Component source preserved; Erii supplies layout, routes and content |
| [OpenIntelligentUI](https://github.com/CopilotKit/OpenIntelligentUI) | `8391a94c1c3939c679e40ddd7bfc08a9ef690e70` | `apps/app/src/components/generative-ui/table-catalog.tsx` native `DataTable` and Zod props | Remove the CopilotKit catalog registration/import; retain validation and accessible table; map the existing UISpec table through a private adapter |
| [eri](https://github.com/z-chenhao/eri) | `f4dcf11f29d17a2be517acb2a50dad08318edc40` | `assets/brand/eri-icon-192.png`, `eri-favicon-32.png` | Original bytes; the same character is the logo and default agent avatar |

Upstream licenses and artwork provenance are in `licenses/`. The binary embeds
the required MIT/Apache-2.0 license texts and serves them at `/licenses.txt`.
There are no paid frontend services. Pinned runtime dependencies are React,
React DOM, Zod and the ad-agent component helpers. Tailwind and esbuild compile
checked-in local assets; they do not run as product services.

OpenIntelligentUI's native component is integrated here. Its Python agent,
CopilotKit/AG-UI transport, model routing, browser-key flow and generated
HTML/JavaScript sandbox are not integrated. Erii's frozen ModelSnapshot,
SecretStore, Actor, approval and controlled UISpec v1 contracts remain the owners
of those responsibilities. Enabling arbitrary generated code requires a separate
architecture/security decision.

The client accepts only supported UISpec v1 components. Invalid/unknown versions,
components and unsupported table dimensions retain the server-rendered result.
Empty or failed renders use authoritative semantic text from the Host, never a
replacement fallback supplied inside a projection. There is no HTML evaluator,
dynamic component import, remote asset or browser credential input.

## Build and verify

```sh
npm ci --prefix internal/interaction/web/client --ignore-scripts
npm run build --prefix internal/interaction/web/client
make build
npm run check --prefix internal/interaction/web/client
npm run lint --prefix internal/interaction/web/client
python3 scripts/verify_channels.py --binary bin/personal-agent --evidence evidence/components.json --ui-components-only
```

Commit component source, lockfile and generated assets together. `make test`
rejects stale embedded assets; CI installs the same pinned dependencies. Go-only
builds consume those checked-in assets and need no frontend server or network.
The maintained channel browser action proves actual React composition, native
table cells and fallback, frozen approval fields, artwork loading, responsive
layout, CSP-compatible execution and local-only frontend requests. Its complete
action also drives model setup, file upload and actual message submission.
