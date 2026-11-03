# Steward Web

[Web feature and usage](../docs/features/channels.md) ·
[Frontend architecture and provenance](../docs/designs/channels.md#web-components-steward-web-components) ·
[Browser checks and build](../docs/test-plans/channels.md#web-components-build-and-verify)

This file is a navigation entry. Component and artwork license notices remain in web/licenses.

The product is Steward and the agent is Ghost. The CLI is `steward`, environment variables use `STEWARD_*`, and the Go module matches the repository: `github.com/z-chenhao/assistant`. [Visual rules](DESIGN.md) · [Reconstruction scope](BRIEF.md).

Ghost atlas sources are in `web/artwork/` (build-only). From the repository root, run `python3 web/artwork/bake-server.py`, open `http://127.0.0.1:8789/bake.html`, then run `python3 web/artwork/encode-frames.py` with `cwebp` installed. The product only serves the resulting PNG/WebP assets; it never loads the build mesh or server.
