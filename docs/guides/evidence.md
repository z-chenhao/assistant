# Local-first verification evidence

Use `scripts/evidence.py` for repository-private evidence collection. It extends
the existing Python harness and `ci_github.py` transport. It does not change
Golden Case scoring, required assertions, five-run series, replay/report formats,
independent review or final release evidence. ERII-134/141's complete suite and
join consumers continue to read their original files and paths.

## Collect and retain

Choose the registered durable evidence root outside any disposable checkout.
For this workspace it is `/Users/zhuchenhao/Projects/erii/evidence/agent-delivery`.
Other installations must supply their own absolute root. Never put credentials
in commands, profiles, logs or manifests. Profile names describe the environment;
they are not credentials or a request to load another person's account.

Run the existing maintained checks with `ci_step.py`, which records the real
command, exit code, source SHA, timing and cleanup. Point collection at **one
current run's evidence**, not the repository, home directory, or all historical
logs. Preserve failed runs too. Example (set `ROOT`, `ISSUE`, `RUN` and `PROFILE`
to the actual registered root and current identities first):

```sh
SHA=$(git rev-parse HEAD)
python3 scripts/ci_step.py --name evidence-tests --phase test --evidence evidence/current/steps -- python3 -m unittest discover -s scripts -p 'test_evidence.py'
python3 scripts/evidence.py collect --root "$ROOT" --source evidence/current --issue "$ISSUE" --repo z-chenhao/erii --sha "$SHA" --run "$RUN" --attempt 1 --record evidence/current/steps/evidence-tests.json --profile "$PROFILE"
```

Collection copies regular files under
`ISSUE/owner/repository/FULL_SHA/RUN/ATTEMPT/raw/`. `manifest.json` records relative
paths, byte sizes, SHA256, commands/exit codes, profile, timestamp and collector
Python/host versions. Command records can also include test-tool/profile versions.
Collect reports and their referenced Artifact directories together. A nonzero
command exit remains nonzero evidence; collection never upgrades its verdict.

Dependencies (`node_modules`), `.git`, `.cache`, `ms-playwright`, `downloads`,
`ci-artifacts` and prior archives (`.zip`, `.tar`, `.tar.gz`, `.tgz`) are excluded
and listed. Symlinks fail closed. Required original archives can be retained with
`--include relative/file.zip`; the file must exist and is hash-bound like every
other file. Inspect exclusions against the final evidence contract before
handoff. Logs from this run remain complete. Historical logs must stay in their
own run directories, never recursively bundled into the current run.

Collection holds a per-run lock, commits each checked file then the manifest,
and resumes an interrupted copy. Identical repeated calls reuse the manifest.
Content, profile or command drift in a completed identity fails; use a new
run/attempt. Existing corrupt bytes fail rather than silently replacing evidence.

## Same-host handoff and export

Keep the printed manifest path and SHA256 in the review handoff. The digest must
come from that trusted handoff, not from the manifest being checked. Substitute
those values for `MANIFEST` and `HASH` below:

```sh
python3 scripts/evidence.py verify --manifest "$MANIFEST" --manifest-sha256 "$HASH"
python3 scripts/evidence.py receipt --manifest "$MANIFEST" --manifest-sha256 "$HASH" --output evidence/handoff.json
# Only when the existing final/release consumer requires a full export:
python3 scripts/evidence.py export --manifest "$MANIFEST" --manifest-sha256 "$HASH" --purpose 'Required final evidence for CONSUMER'
```

The receipt is a small location/identity/hash manifest suitable for one online
attachment. It does not copy all raw file rows or upload anything. Publish the
receipt once with the existing delivery path; keep the returned provider object
ID/size in the handoff. Repeat handoffs reference that same object. Sum **all**
ordinary attachments in the run; the policy limit is 1 MiB, not 1 MiB per file.
The collector rejects a receipt exceeding that limit. Never infer an upload from
a staged file, command count, or metadata estimate.

Reviewer verifies the manifest's trusted hash and every local raw file, reads
supporting evidence without writing it, and still uses a clean exact-SHA checkout,
independent code inspection and relevant maintained tests. This is same-host
reuse, not a remote path or expanded access permission. If evidence is missing,
retrieve only indispensable files; missing required proof stays inconclusive.

Export creates one ZIP under the durable run's `export/`, records its purpose,
actual size and SHA256 and reuses it on repeated calls. It includes the original
manifest and complete selected raw tree. It never uploads automatically. Large
mandatory exports require an explicit size/use entry and the existing bulk
transfer approval policy; do not omit mandatory evidence to meet the routine
budget. Old consumers can unzip and use `raw/` as their evidence directory.

## CI metadata before bytes

```sh
python3 scripts/evidence.py ci-list --repo z-chenhao/erii --run RUN_ID --attempt ATTEMPT --output evidence/ci-metadata.json
python3 scripts/evidence.py ci-fetch --root "$ROOT" --metadata evidence/ci-metadata.json --id ARTIFACT_ID
```

`ci-list` reads the exact run attempt and paginated artifact metadata. It retains
only existing `evidence-…-FULL_SHA-ATTEMPT` names. Inspect name, immutable ID,
size, digest and expiry before selecting one ID. `ci-fetch` checks the run/SHA/
attempt binding, requires an authoritative SHA256 and fetches only that archive.
It uses the existing authenticated `gh api` GET boundary. It never extracts ZIP
paths or downloads executables for execution.

Verified immutable archives live at `ROOT/ci-artifacts/owner/repository/ID/`, with
identity/hash receipts. A warm call verifies bytes and transfers zero payload.
A different run attempt must use its own matching metadata/artifact. A missing,
expired uncached, digest-less or corrupt artifact fails closed. A verified local
copy remains useful after remote expiry. Refresh `ci-list` when diagnosing a
missing/expired object; no automatic switch to another attempt is allowed.

Each binary request gets a durable transfer journal and unique partial file.
There is no transparent binary retry. Received payload bytes are measured;
interrupted requests are marked incomplete/failed. SIGKILL may leave the journal
incomplete; its named partial file retains received bytes for diagnosis. The next
call starts a new request unless the completed archive already verifies, in which
case it repairs the receipt without another download. Metadata/HTTP overhead is
not counted as archive payload. `size_in_bytes` is a metadata estimate until the
archive is received and measured.

The default per-object download ceiling is 1 MiB. Over-budget objects fail before
download. Choose local/smaller evidence first; only after authorized indispensable
bulk transfer use `--max-bytes EXACT_APPROVED_SIZE`. This flag records a technical
ceiling and does not grant approval. Missing evidence is not a passing result.

The historical `ci_compare.py` collector now requires `ERII_EVIDENCE_ROOT`, uses
this verified cache and selects only canonical shard archives; fast/race/part
archives cannot supply its structural suite reports. Comparisons retain their
original complete-sample rules. Current CI's mandatory part/join uploads retain
14-day retention: ephemeral hosted runners have no access to
the registered host root, and those uploads are the existing complete-evidence
contract. They are not routine Builder/Reviewer attachments. Communication parts
now recompute their native oracles locally and upload
compact source/fixture/assertion/cleanup manifests for the required join; see
[communication operations](../ci.md#communication-budget-and-evidence). Retain
their full local native bytes for independent recomputation. This deterministic
completion record cannot certify a Golden Case or release. Other joins retain
the raw input reports their existing consumers require.

## Reproduce and clean up

```sh
python3 -m unittest discover -s scripts -p 'test_evidence.py'
make verify-evidence EVIDENCE_ROOT=/absolute/durable/fresh-fixture-root
```

Use a fresh fixture root per harness invocation. Launch creates a private author
tree and executable `gh` Fake; Doctor validates identities and required records;
Drive invokes the actual CLI in separate author/reader processes; Evidence records
objects, payload bytes, cold/warm results, command exits and export size; Cleanup
removes only the disposable author/Fake trees. The report explicitly distinguishes
Fake payloads, live transport and staged versus uploaded receipts. Unit regressions
cover corrupt manifests/files, missing/expired artifacts, attempt changes, repeated
calls, interruption recovery and existing comparison consumers.

Retain durable raw data, receipts, partial transfers and required exports through
issue completion **and** all review/repair resolution. Worktree cleanup must not
remove them. There is no automatic garbage collection or remote deletion. After
resolution the owner can archive/remove the exact issue's directories according
to retention policy; shared immutable CI archives may serve multiple issues, so
check references before deletion. Never delete failed evidence to save bandwidth.

GitHub references: [selective artifact download](https://docs.github.com/en/actions/how-tos/manage-workflow-runs/download-workflow-artifacts)
and [artifact metadata and download API](https://docs.github.com/en/rest/actions/artifacts).
