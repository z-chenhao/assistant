# Artifact storage and operations (ERII-14)

Artifacts reopen through opaque `artifact:` references. Only the Store resolves
references to storage keys. Files live beneath `app.data_root/artifacts`;
metadata, references, publications and tombstones live in state.db schema v11.
The forward migration preserves schema v10 data and frozen Run capabilities.

This installation has one local user principal (`local`), tied to its data root
and OS account. This is not a multi-user server or a bearer-reference API. The
Host constructs user/Agent/Task/Run scope. The CLI acts as that local user;
models never choose scope. Cross-user and cross-Agent access fail. Worker
outputs are immutable, Run-owned and readable within the same Task. Main
inputs require explicit grants; Task create/append validate and pin accessible
refs. Same Task visibility does not inject any file contents into context.

## CLI boundary

```sh
printf 'report text' | bin/erii artifact save report.md text/markdown
bin/erii artifact describe ARTIFACT_REF
bin/erii artifact open ARTIFACT_REF > ./download.md
bin/erii artifact delete ARTIFACT_REF VERSION
bin/erii artifact cleanup
bin/erii artifact cleanup apply
```

`save` streams stdin and accepts a display name and MIME type. It accepts no
source path or URL. `open` verifies size/SHA-256 before emitting bytes to stdout.
`describe` returns metadata without paths, storage keys, owner IDs or download
URLs. The local user may inspect expired/deleted metadata for audit; content
reads and all Worker operations reject expired/deleted refs. Active/paused Task
ownership or explicit Task attachment suspends expiry; the frozen per-object
retention duration runs for at least that duration after the last associated
Task ends. Descriptors omit expiry while a Task protects the object. Delete checks
version and references, then tombstones metadata. Recovery unlinks deleted
bytes on the next startup; cleanup apply also unlinks after commit. Audit
metadata remains. Referenced objects cannot be deleted.

`chat send-parts [REQUEST_KEY]` reads a JSON array on stdin, bounded to 16 KiB.
Text uses `{ "type": "text", "text": "..." }`; image/audio/video/file parts use
`{ "type": "file", "artifact_ref": "..." }` with their semantic type. The Host
validates and pins each input before accepting it. Large files are uploaded
through `artifact save`, then passed by reference. The current text Provider
receives reference hints and can request bounded text reads. Reference transport
does not establish visual understanding. [M6 fixtures](m6-evals.md) exercise
image/source-region and media handling with explicit Fakes; [Computer Use](computer-use.md)
provides scoped file transfer. Those paths have separate proof limits.

## Tool contract

The consolidated Artifact builtins use the existing Tool Registry, frozen Capability Snapshot,
ToolInvocation, Executor, claim/epoch fence and error normalization. Replay
policy is `safe`, timeout is 10 seconds, result limit is 64 KiB, risk is low.
Only the Host executes them. No direct Tool bypass or separate invocation
pipeline exists. Main and Worker share these capabilities; Workers still lack
Task controls and user delivery.

| Tool | Arguments | Result | Permission |
| --- | --- | --- | --- |
| `builtin__artifact__manage` (`operation: save`) | `name`, `mediaType`, `text` (8192 characters; 16 KiB total call cap) | opaque ref in `artifact_refs` | `artifact_write` (local immutable write) |
| `builtin__artifact__manage` (`operation: read`) | `artifactRef`, optional byte `offset`, `limit` 1–4096 | bounded text range; binary content returns a ref | `artifact_write` |
| `builtin__artifact__manage` (`operation: describe`) | `artifactRef` | name, MIME, hash, size, dates, version | `artifact_write` |
| `builtin__artifact__manage` (`operation: publish`) | `artifactRefs` (0–127) | selected refs | `artifact_write` |
| `builtin__artifact__search` | optional `query`, `limit` 1–50 (default 50) | accessible descriptors by Unicode-lowercased name/MIME substring, newest first; no query browses | `read_only` |

Unknown fields, ownership/Run IDs, URLs and path inputs are rejected. Inaccessible,
forged, expired, deleted or corrupt references return `NOT_FOUND`; invalid
names/MIME/ranges return `INVALID_ARGUMENT`; quota/size failures return
`RESOURCE_LIMIT`. Storage failures do not become successful results. Byte ranges
must contain valid UTF-8; callers can adjust boundaries when a character is cut.
Binary media remain references. This API does not base64 encode media.

Save and Tool output publication commit metadata with the normalized Tool result.
Runtime idempotency uses the accepted invocation ID, never model-supplied keys.
Large text exceeding the frozen Tool output limit and internal provider binary
streams become Artifacts before any result event, checkpoint or next generation.
Small Tool outputs retain their existing representation. Only an accepted
`publish` invocation can select final outputs; provider result fields cannot
select them. Binary consumption and publication validation keep the same frozen
Tool deadline. The Executor closes returned closable streams, including on
expiry/cancellation, and joins cancellation cleanup before returning. Failed
normalization rolls back metadata, pins and publications before committing the
Tool error. An external-effect stream timeout remains `unknown` and never
retries the effect. Authoritative Query receipts use that same atomic
normalization for reclaimed running calls and terminal `unknown` calls. The
caller, persisted result, reconciliation proof and replay expose the committed
ref. The frozen Query deadline includes receipt consumption and stream closure.
Unusable receipts retain `unknown`; storage errors propagate without settling or
redispatching the effect. Query-only reconciliation never revives the old Run.
A published reference is a resource address; it grants no external upload permission or approval.

`publish` explicitly adds selected final outputs for that Run; it is idempotent
and additive. Intermediate saves are excluded. The union of final selections is bounded to
127 refs, leaving one text part in the 128-part Conversation envelope. A call
exceeding that aggregate limit returns `RESOURCE_LIMIT` without changing the
selection. Root results carry only the
Root's explicit publications in Task updates; Main explicitly selects outputs
for final Conversation parts, preserving image/audio/video semantic types and
using file parts for other MIME types. Direct Child admission and result handoff are covered by [Child operations](children.md).
Changing content creates a new ref; no overwrite operation exists.

## Recovery, quota and retention

Defaults: 30 days, 1 GiB per local user, 64 MiB per object, 100 expiry candidates
per cleanup batch. `artifact` configuration sets positive bounds; quota must
cover max object size. Quota counts all live metadata, including expired objects
until deletion. The immediate state.db write transaction serializes quota,
object writes and orphan recovery across processes. This favors correctness
and bounded local storage over throughput; one large upload holds the write
lock, and lock contention can return a storage failure rather than retrying an
unreadable input stream.

The Store writes a fresh exclusive temporary file, hashes and syncs it, verifies
the written bytes, atomically renames and syncs the directory, then inserts
metadata. A process death before metadata commit leaves an orphan; startup
removes unreferenced temp/final objects under the same write lock, so a live
writer cannot be collected. A committed Tool result is reused after restart.
Filesystem access uses `os.Root`, generated flat keys, regular-file checks and
no caller-controlled paths. A symlink as the Artifact root fails startup;
symlink objects and path escapes fail closed. The configured data root and OS
account remain trusted deployment boundaries.

Startup always performs orphan recovery first. Cleanup of committed metadata defaults to dry-run. It reports expired, unreferenced candidates and
orphan counts without mutating them. Apply tombstones a bounded batch, commits,
and then removes bytes. Active/paused Task outputs are protected from automatic
cleanup. References to input history, Tool results, Tasks and explicit outputs
pin content. These audit references are durable; retention does not silently
release them. Future history pruning must release its references in the same
transaction before storage can be reclaimed. After Task protection ends, effective expiry rejects content reads.

## Verification

For focused receipt recovery, add `--recovery-only` to the command below. It
launches approved Worker effects, kills after the effect or terminal unknown
commit, reconciles through the CLI, verifies bytes/hash and caller readback,
replays without a second effect, and removes all disposable state.

`python3 scripts/verify_artifacts.py --binary bin/erii --evidence
evidence/artifacts.json` owns Launch/Doctor/Drive/Evidence/Cleanup, using the
explicit `artifacts` model and binary/large-output Tool Fakes. It drives actual
CLI save/open/delete, large input refs, Task output publication, eight concurrent
CLI writers, forged/traversal refs and six SIGKILL boundaries. `make verify`
repeats it five times. Go contract tests add cross-user/Agent/Task/Run checks,
child-shaped writers, symlinks/tampering, quota, expiry/reference/dry-run checks,
migration rollback, failed Tool commit and stale claim fencing. Race detection
is separate (`make test-race`).

GC-15's versioned fragment preserves source hashes and reopens `visa_check.md`
through Task → Root → Main final delivery. Synthetic PNGs carry fixture fields
in metadata; they are binary transport assets, not OCR evidence. Scripted text
is not semantically graded; this Artifact fragment keeps the full case
`not_applicable`. [M6 evidence](m6-evals.md) covers additional image/privacy
mechanics without establishing model vision quality. Original M1/M2 fixtures and locks stay unchanged.
No Sidecar exists yet; JavaScript Durable Replay and Computer transfer are not
claimed by this slice.

`manage` accepts only fields for its selected operation. `publish` selects the
current Run's final outputs; it does not send a message or upload a file. Main's
final AssistantMessage passes those semantic references to the existing Delivery
Composer and versioned UISpec (including Web's OpenIntelligentUI table). Workers
return only to their direct Parent. Neither tool accepts ownership or filesystem
paths. Search filters before counting accessible matches, so older matches are
not hidden behind 50 unrelated files.

The five old Direct Tool names are removed. See
[ERII-192 compatibility](../adr/005-consolidated-tool-surface.md) before upgrading
an installation with active Runs. The authenticated `artifact` CLI is unchanged.
