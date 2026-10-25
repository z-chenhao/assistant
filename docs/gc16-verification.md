# GC-16 research verification

The maintained runner drives Golden §§7.GC-16/8.17 through the production CLI,
Task/Root/Child, configured read-only MCP, Artifact, Reporter and Delivery paths.
The explicit scripted model proves transport and state mechanics. It does not
establish actual model research quality, internet-wide priority or L2 acceptance.

```sh
make build
python3 scripts/verify_gc16.py --binary bin/erii --evidence evidence/gc16.json
python3 scripts/prepare_release_eval.py --input evidence/gc16.json --evidence evidence/gc16-judge.json
python3 -m unittest discover -s scripts -p 'test_gc16.py'
go test -race -count=5 ./internal/llm/fake
```

`make verify` includes `gc16` in the communication shard, which already
provisions the MCP sandbox. `--variant base` is diagnostic and cannot prepare a
complete Judge packet set. The lock binds the corpus and unchanged Golden
source. Six variants include three wordings, reversed document order and two
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
Judge preparation recomputes the complete native assertions, variant lock,
restart/replay and cleanup before allowlisting visible fixture/conversation/report
content. Six previews are explicitly ineligible; scores and winner remain null.

Actual unscripted research, longer contexts, remaining variants, all required
dimension scores and protected acceptance, identical-condition local/account
comparison and current L2/named human review remain due. The release completion
gate stays nonzero. This fixture adds no production Tool, schema, permission,
provider fallback or replay semantics.
