# GC-16 research verification

The maintained runner drives Golden §§7.GC-16/8.17 through the production CLI,
Task/Root/Child, configured read-only MCP, Artifact, Reporter and Delivery paths.
The explicit scripted model proves transport and state mechanics. It does not
establish actual model research quality, internet-wide priority or L2 acceptance.

```sh
make build
python3 scripts/verify_gc16.py --binary bin/erii --evidence evidence/gc16.json
python3 scripts/verify_gc16.py --binary bin/erii --evidence evidence/gc16.json --validate
python3 scripts/prepare_release_eval.py --input evidence/gc16.json --evidence evidence/gc16-judge.json
python3 -m unittest discover -s scripts -p 'test_gc16.py'
go test -race -count=5 ./internal/llm/fake
```

`make verify` includes `gc16` in the communication shard, which already
provisions the MCP sandbox. `--variant base` is diagnostic and cannot prepare a
complete Judge packet set. The original lock binds the corpus and unchanged Golden
source. The supplemental `gc16-boundary-v1.lock.json` pins all 16 original
blocks from ERII-223's frozen manifest
`0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556`. Six variants include three wordings, reversed document order and two
20% irrelevant-document/search-hit variants, with seeds retained in receipts.

Launch creates isolated configuration, SQLite stores and a stdio corpus peer.
Doctor explicitly probes/selects the Fake profile and checks the source lock.
Drive sends the original natural-language question, without a Tool plan or Gold.
Main creates one Task. Root submits two Child calls in one exchange; their
persisted lifetimes overlap. One Child follows the secondary D2 lead to D1,
then D3 and D4; the other independently checks D5. The peer returns exact fixture
bytes and appends every read to an independent log, including duplicates or
unknown references. It does not synthesize answers, deduplicate calls, access
domain databases or contact the network. The example.test links identify the
synthetic sources; they are not fetched as real websites.

Root receives only the two-field Child final envelopes. It saves and publishes
one independent Markdown report; Main publishes that same reference to the
user. `artifact open` reopens the exact stored bytes. The report separates facts,
inferences and missing evidence, cites D1/D3/D4/D5 beside their claims, establishes
Atlas's 2024-03-01 primary date, distinguishes the 2024-06-15 rename from the
2025-01-10 fencing extension, and excludes Aster's declarative Step Cache. D2's
earlier speculation is only a discovery lead. The report limits conclusions to
this corpus and provides a next verification step for another reviewer.

Actual SIGKILLs after Child admission and after the committed Root report
exercise incomplete admission recovery and completed-call reuse. Both parallel
admissions can commit before the first signal; at least one is accepted, all
accepted Children remain queued and no source has been read. Restart retains
one Task/two Children, frozen Model/Capability snapshots, exact source reads and
one report. Duplicate ingress plus another drain leaves complete native state
and source logs unchanged. Receipts retain Runs, generations, ToolInvocations,
Child links/results, Approval (empty), Automation (empty), Artifact/publication,
Inbox/Reporter/Delivery, timestamps, source logs, binary/source identity and both
killed-process exit codes. Cleanup joins owned processes and removes private
state, with failed receipts retained on errors.

Python regressions reject changed primary dates/claims/citations, missing
fact/inference separation and use of a secondary lead as proof. Go regressions
require both evidence paths, walk all discovered official links and refuse to
publish a definitive report when any source/claim is absent or unsupported.
CLI `--validate`, CI communication collection and Judge preparation share one
strict consumer. It recomputes complete native assertions, locks, exact ordered
six-variant inventory, restart/replay and cleanup. Run/Tool/generation/Delivery/
conversation IDs are unique within each execution and disjoint across variants;
relabeling a copied execution cannot satisfy the inventory. Source full SHA and
working-byte hashes, binary SHA256/build information and freshly observed
platform/Python/Node/Go identity must match. Only the incidental executable path
is omitted from build information.

The consumer joins model-emitted calls to accepted Tool parameters, Child links
to WorkerResult and final generation/source bytes, and report save/publish to
CLI-reopened bytes, publication owners and delivered messages. This closes the
previous hole where empty persisted Child results still passed the oracle.
Twenty-six actual native corruption probes exercise these joins, old epochs,
capability/model drift, attempts/unknown status, source logs, crash cuts and
replay. Unit regressions additionally reject typed identity/cleanup drift,
missing/reordered/copied executions, forged flags and resealed manifest metadata.

The public JSON is a small size/hash-bound manifest. Full native bytes remain in
a hidden compressed sibling and per-variant files locally; summary alone is not
proof. Path traversal, symlinks, wrong sizes/hashes and changed summaries reject.
Six allowlisted Judge previews remain explicitly ineligible; scores and winner
stay null. The communication topology, timeouts, resource limits, one six-variant
GC-16 gate invocation and all existing protected five-run gates remain unchanged.

`unscripted_cases.py freeze --case GC-16 --variant base` accepts an explicit
operator-prepared production profile. It sends only the original question and
clock, never the hidden corpus/Gold or a Tool plan. `verify_unscripted_cases.py`
drives that fragment through the real CLI/bridge with a loopback peer and keeps
it ungraded. This is an input fragment: the connected research-source/Child/
Artifact adapter remains due. No credential is borrowed or created implicitly.

Read-only research requests no cancellation, approved external effect or
unknown outcome. This Case checks that Approval/Automation remain empty;
dedicated cancellation/epoch/unknown conformance gates remain required. Their
PASS must not be inferred from this Case's report.

Actual unscripted research, longer contexts, remaining variants, all required
dimension scores and protected acceptance, identical-condition local/account
comparison and current L2/named human review remain due. The release completion
gate stays nonzero. These gaps stay with parent ERII-223; `semantic_score=null`,
`eligible_for_grading=false` and `release_ready=false` are mandatory. This fixture adds no production Tool, schema, permission,
provider fallback or replay semantics.
