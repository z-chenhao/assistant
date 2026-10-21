# Computer Use

Configure the [durable Computer](computer.md) and JavaScript runtime. Code Mode
then discovers live Apps, Windows, Browsers and Tabs through the existing three
private Computer bridge Tools. Direct model calls to these Tools are rejected.
There is one Agent Loop and one ToolInvocation/Security/replay pipeline.

```js
const state = await cua.getState();
const app = await cua.getApp("com.example.Editor");
text(await app.documentation());
const observation = await app.getAXStateAndScreenshot();
text(observation.ax.state);
image(observation.screenshot.image);
```

Inventory failures appear by source in `getState().errors`; surviving sources
remain available. `getApp`, `getBrowser` and `getTab` bind existing resources.
`cua.computer.launchApp({app: bundleId})` and `createBrowserTab(browserId, url)`
are explicit mutations. `getApp({windowId})` binds a discovered window when an
App has multiple windows. Window IDs use native process/window identity, so
identical titles remain independently bindable and renaming does not change
the Surface identity. A Tab can be rebound by `{url}` when that URL has one
match. Ambiguous selectors fail rather than choosing an arbitrary target.

## Leases and observations

Bindings retain Computer ID, Instance epoch, Surface identity and the accepted
Code invocation's DesktopLease. They are not checkpoint state. `store()` rejects
Surface and Capability objects; store Bundle IDs, URLs and other semantic refs.
Instance reconstruction invalidates old bindings. A controlled approval/resource
suspension records a resume receipt and retains the physical DesktopLease.
Expired or abandoned active claims still follow the durable rebuild path. Old
Run/Code owners cannot write state, accept results or keep their transports live.

All Computer calls, including observations, acquire the same desktop conflict
key through ordinary ToolExecutor admission. They also retain the logical
`computer:<id>:desktop` key after allocation. Different Apps do not bypass the
read barrier. A mutation invalidates cached AX/pointer references before dispatch.
A lost result is `unknown`; the Host does not repeat it.

The first AX read and `full:true` return a full state. Later reads have
`kind:diff` and `baseRevision`; `state` is empty for no change and otherwise is a bounded contiguous line
patch: `@@ zeroBasedLine,removedLines,addedLines @@`, then `- ` removed lines and
`+ ` added lines. New AX observations invalidate old explicit ElementRefs.
Numeric indices always refer to the handle's latest AX state. Screenshots have
separate revisions and Artifact references. A screenshot-only read disables the AX walk and preserves native tokens; it
cannot retarget an old index to a changed tree. Browser element tokens also bind
CDP backend node identity, so replacing a node with identical text does not
reuse the old reference. Core operations retain a fixed ElementHandle while
waiting for actionability and reject a detached node instead of querying a
replacement. Explicit Playwright CSS locators remain dynamic. Coordinates require a valid screenshot and unchanged geometry.
Every action must be followed by observation before using element/pointer refs.
Observation references become actionable in the same SQLite transaction as the
Tool result and screenshot Artifact metadata. Failed output normalization leaves
the pending observation inert.

`performSecondaryAction` accepts only an advertised non-default action. The
native adapter uses Cua Driver's AX `click.action` path, not arbitrary command
execution. Native `selectText` uses bounded, exact-element keyboard selection for
one unique ASCII substring within the first 256 characters of an AX text field;
unsupported or ambiguous ranges fail closed. Browser text selection uses the
bound DOM element. Input acknowledgements preserve `verified`/`effect` facts;
`unverifiable` is not a claim of application success. Observe and read back the
application state after input, including Return/commit where the app requires it.

`waitForChange({sinceRevision,timeoutMs,full})` polls inside the Runtime, never
inside QuickJS. The deadline is 1–10,000 ms. `WAIT_TIMEOUT.details.observation`
contains the last bounded AX summary. Observe again after a timeout before
using element refs. Unchanged internal polls do not advance the delivered diff
base. It does not poll forever or retry input.

## Security and file transfer

This first adapter uses a conservative policy: every input, navigation,
launch/create and file transfer requires approval for that exact frozen nested
ToolInvocation. Reads and discovery are automatic. Consent for a Code cell,
import or Parent never grants upload, overwrite, private export or Child consent.
All mutations use `never` replay; reads use `safe`. A transport failure after
possible input becomes `UNKNOWN_SIDE_EFFECT` and is never retried automatically.

```js
const file = await cua.files.importArtifact({artifactRef: inputRef});
// file is {computerId, path}; path is relative to the controlled Guest root.
const artifact = await cua.files.exportArtifact({path: file.path});
// artifact.artifactId is the opaque ArtifactRef; pass it directly to Artifact tools.
return artifact;
```

The fixed Guest Transfer Root is `/Users/<configured-guest-user>/EriiTransfer`.
Only registered `imports/<name>` and `downloads/<name>` refs are accepted. No
Host path, arbitrary URL, external Guest path or script-provided ID authorizes
access. ArtifactStore enforces the current Agent/Task/Run scope. Imports,
overwrites, exports and page uploads are separate calls and approvals.

The Guest helper opens every path component with directory-relative descriptors
and `O_NOFOLLOW`. It refuses traversal, symlinks, hard links, directories and
special files before truncation. Binary bytes remain outside QuickJS and traces.
Transfers are bounded to 8 MiB. Downloads use generated names under `downloads/`;
page-chosen filenames cannot select a destination. FileChooser uploads receive
bytes from registered ComputerFileRefs, not Host or Guest filesystem paths.
File registration is retained for the owning Run or Task across cells and
Instance replacement. Another scope cannot turn a known path into authority.
New imports are limited to 256 visible files; overwrites require an existing
scoped registration.

## Playwright provisioning

The only delayed Capability is Tab `playwright`. Read `capabilities.list()` before
`get("playwright")`. Its documentation describes CSS locators, bounded navigation
waits, download and chooser upload. Every method creates a normal nested
ToolInvocation. Navigation/reload expires the Capability even at the same URL.
CSS resolution and actionability waits stay scoped to the bound document. A
matching control in a replacement document cannot receive the pending input;
the call fails with `STALE_SURFACE_HANDLE`. CSS may follow element replacement
within the same document, while core ElementRefs remain tied to one DOM node.
There is no evaluate, dynamic import, arbitrary browser endpoint or filesystem
API in the model facade.

Install Node 24.21.0 and this repository's `guest-browser/package-lock.json` in the
clean Guest. Run `npm ci --ignore-scripts` there. The pinned Playwright Core
1.64.1 dependency uses Apache-2.0 and adds no Go dependency or Host runtime.
Place `internal/computer/macosvm/transfer.py` beside Playwright Core's `index.mjs`
as `erii-transfer.py`. Provision one isolated Chromium profile with CDP listening
only on Guest loopback. The adapter binds that existing browser; discovery does
not launch it or attach to a Host user's browser. Add trusted configuration:

```yaml
computer:
  # existing durable Lume/Guest configuration ...
  browser:
    node: /opt/erii/node/bin/node
    module: /opt/erii/browser/node_modules/playwright-core/index.mjs
    port: 9222
```

Absent browser configuration reports no managed Browsers. A configured browser
failure is an Inventory error and does not suppress native Apps or Windows.
The module version is checked before CDP attachment; configuration and bridge
resource versions are frozen in Capability Snapshots. Reprovisioning affects new
Runs; mismatched snapshots fail closed.

## Output and verification

Nested observations, actions, capability calls, files and errors stay in the
Tool/Code ledger. Only `text()`, `image()` and `return` select model context.
`image(screenshot.image)` selects its scoped Artifact. Image-capable snapshots
receive bounded JPEGs resolved in the Host only at Provider dispatch; the
persisted request keeps references. Text-only snapshots receive reference hints. Existing user-input media projections
keep their text-only reference shape; only explicit Code Mode image outputs gain
the dispatch-time image field, preserving pre-ERII-25 accepted requests.
No screenshot Base64 crosses QuickJS or enters SQLite, RunTrace or errors.
To activate image input normally, declare `capabilities.image_input:true`, run
`model test`, then `model select`. The Host verifies a synthetic image before
admitting that capability. Undeclared or unverified image input cannot enter a
new snapshot. Text-only profiles keep the reference fallback.

Offline Launch/Doctor/Drive/Evidence/Cleanup:

```sh
npm ci --prefix guest-browser --ignore-scripts
node guest-browser/node_modules/playwright-core/cli.js install chromium
make test
python3 scripts/verify_computer_use.py --binary bin/erii --evidence evidence/computer-use.json
npm test --prefix guest-browser
python3 scripts/verify_local_provider.py --binary bin/erii --evidence evidence/local-provider.json
```

`make verify` runs the Computer Use CLI contract five times. `make test` includes
real Chromium against a local synthetic website and the actual openat transfer
helper's traversal/symlink/hardlink/FIFO regressions. No online account or website
is used by tests. Browser binaries are installed separately before offline tests;
on Ubuntu use `install --with-deps chromium`.

For the previously authorized isolated Lume test root, run:

```sh
python3 scripts/verify_computer_use.py --binary bin/erii --live-config TEST_ROOT/product-live.json --live-root TEST_ROOT --evidence evidence/computer-use-live.json
```

This command owns the foreground Lume server, compiles the project-local AppKit
fixture, installs it only into a disposable Guest clone, and drives it through
the production CLI/Code Mode/ToolExecutor/SSH/Cua adapter. It records screenshot,
AX and nested invocation evidence and checks checkpoint/release. Guest/browser
compatibility is distinct from offline conformance and Golden Case scoring.

To add the pinned browser runtime to that disposable Guest checkpoint and then
exercise the full Guest browser path, use its retained `erii25-cua-use-*` root:

```sh
python3 scripts/verify_computer_use.py --binary bin/erii --live-config TEST_ROOT/product-live.json --live-root TEST_ROOT --resume-root TEST_ROOT/erii25-cua-use-INSTANCE --native-actions-only --provision-browser --evidence evidence/computer-use-native-live.json
python3 scripts/verify_computer_use.py --binary bin/erii --live-config TEST_ROOT/product-live.json --live-root TEST_ROOT --resume-root TEST_ROOT/erii25-cua-use-INSTANCE --live-browser --evidence evidence/computer-use-browser-live.json
```

Provisioning downloads checksum-verified Node and locked Playwright/Chromium into
an issue-owned Guest directory. Guest LaunchAgents start the synthetic local site
and isolated headless browser at the next login. Doctor explicitly bootstraps
those owned fixture services if the Guest session has not loaded them, then
checks CDP and the local site before product discovery. The browser drive reads back
locator fill/click state, exact downloaded Artifact bytes, chooser upload state,
and navigation capability expiry. Each retained-root drive uses a fresh ingress
key so a repeated command cannot pass by reusing an old completed Run. Evidence
includes nested frozen approvals, AX results, screenshot hashes and checkpointed
cleanup. Screenshot PNGs stay in the owned test root's `evidence/` directory.

To collect every recorded screenshot from a completed live report without
starting a VM or sending input:

```sh
python3 scripts/verify_computer_use.py --binary bin/erii --live-root TEST_ROOT --resume-root TEST_ROOT/erii25-cua-use-INSTANCE --collect-observations evidence/computer-use-native-live.json --evidence evidence/computer-use-native-live.json
```

Collection reads each opaque ref through the Artifact CLI and records the PNG
filename and SHA-256 beside the nested AX/action evidence.
