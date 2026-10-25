# Reserved-slot Drain diagnosis

ERII-372 retains the original failure on source
`db2b3d3069f3c35d155d52cfe53e83398f2e1dee`, CI run `40171915283`, job
`121104343523`, artifact `12590877542` (SHA256
`b09370a6afd327eba0ec18076d0d4b62982287cb7139563d9fb0ada3e4440505`).
The native test failed after 12.43 seconds at its Drain join with
`context deadline exceeded`. Its four-Root concurrency and urgent Main assertions
had already passed. The archive has no phase timings or CPU profile, so its exact
execution remains **inconclusive**. Later green CI does not explain that failure.

## Confirmed mechanism and change

`Runs.Next` repeatedly scanned runnable/active Runs with `DB.QueryRowContext`.
Each scan compiled the same large SQL expressions again. Linux arm64 and offline
Linux amd64 race profiles attributed about 53–55% of sampled test CPU to SQLite
compilation; these totals include other queries and fixture setup. They are not
an attribution of the archived CI failure.

Store now prepares the two fixed scheduling scans once. Each scan still binds
fresh time/limit arguments and reads current rows. SQL predicates, ordering,
transaction admission, fencing and recovery are unchanged. `database/sql` owns
statement closure and repreparation after connection replacement. The original
reserved-slot test retains its ten-second context, three-second Main window,
four held Roots, eight Tasks/reports and Drain assertion. It logs phase times and
failure residue without changing its verdict.

`TestSchedulerScanCompilationDiagnostic` provides a bounded counterfactual through
the actual driver: sixteen alternating pairs, two queries per mode, identical
database/rows/SQL/arguments/process, comparing statement reuse with the former
compilation path. Both paths must return the same Main and active count. Durations
are observations, not assertions. CPU labels distinguish the paths. On the initial
macOS arm64 single-CPU race probe, 32 reused queries took 12.1 ms and 32 recompiling
queries took 231.1 ms. Sampling captured 190 ms in the recompiling path and too few
samples to characterize the reused path. This proves avoidable scan cost; it does
not reproduce the original deadline failure or establish a general speedup.

Run the maintained diagnostic with the ordinary Go profiler:

```sh
mkdir -p evidence/reserved-slot
GOMAXPROCS=1 go test -race -count=1 -timeout=15m ./internal/agent/sqlite \
  -run '^TestSchedulerScanCompilationDiagnostic$' -v \
  -cpuprofile "$PWD/evidence/reserved-slot/scans.cpu" \
  -o "$PWD/evidence/reserved-slot/scans.test"
go tool pprof -top -cum -tagfocus=scheduler_scan=recompiled \
  evidence/reserved-slot/scans.test evidence/reserved-slot/scans.cpu
```

Use `scheduler_scan=reused` for the other path. The diagnostic owns only temporary
fixture databases; the ordinary race inventory owns test discovery and process
cleanup. It does not replace the original journey or full required CI.

## Observations and remaining limits

The earlier candidate `f8a7497fcbc584cacceacd33c40693969184f8d6` passed the complete
native inventory (57 packages, 240 SQLite roots) and exact-SHA Ubuntu race parts
and join (`40174355044`, target 3.24 seconds). Its Task CLI and scheduling harnesses
also passed, including fourteen kill/restart boundaries, saturated descendants,
independent work, cancellation and cleanup. Five offline Linux amd64 emulation
samples averaged 4.112 seconds before and 3.494 seconds after, with order/cache/
thermal noise. Full logs/profiles remain in the registered durable evidence root,
under `ERII-372/z-chenhao/erii/f8a7497fcbc584cacceacd33c40693969184f8d6/`.

That candidate's aggregate CI failed. Audit identity drift caused Python
compatibility failure; the four deferred M6 cells depend on that job, explaining
their skipped/missing inventory. ERII-373 supplies the missing original ledger
seals; this consumer must explicitly refresh affected identities. Historical
observations remain historical and no release claim follows from a refresh.
GC-02 verification, native validation and cleanup returned zero, then GitHub's
CreateArtifact service timed out; the missing artifact still failed the gate.

New source revisions require their own complete CI receipts and independent
acceptance review. No deadline, resource limit, assertion, inventory, cleanup,
security/replay or release gate is waived. `release_ready=false`.
