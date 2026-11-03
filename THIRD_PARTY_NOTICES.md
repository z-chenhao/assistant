# Third-party notices

Steward's own code is licensed under [Apache-2.0](LICENSE). This does not
replace the licenses of incorporated code, fonts or dependencies.

| Component | Source notices | Distributed notices |
| --- | --- | --- |
| Go standard library and linked modules | The pinned toolchain and `go.mod` / `go.sum`; upstream module license files | `licenses/go/` and `licenses/go-toolchain.txt` |
| Web components, React and other bundled UI dependencies | [UI notices](web/licenses/) and the locked web dependencies | `licenses/web.txt`; also embedded and served at `/licenses.txt` |
| Self-hosted fonts | SIL OFL notices in [web/licenses](web/licenses/) | Included in `licenses/web.txt` and the embedded UI notices |
| Provider Bridge and QuickJS-WASI dependencies | Their locked npm packages | Their original notices remain inside the installed `node_modules` trees |
| Vendored JavaScript VM/prelude | [MIT license](runtime/javascript/src/vendor/LICENSE) and [provenance](runtime/javascript/src/vendor/NOTICE.md) | The same files in `runtime/javascript/src/vendor/` |
| Steward / Ghost artwork | [Current artwork and provenance](web/licenses/steward-artwork.md) | Included in `licenses/web.txt` |

Source installation and native packaging retain `LICENSE`, `NOTICE`, this file
and the linked Go dependency notices. Dependency updates must retain upstream
notices and be reviewed for license compatibility; a passing vulnerability scan
is not a license review. The project license grants no trademark rights.
