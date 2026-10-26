# CI comparison after the ERII-207 delivery

This receipt is **not performance-certified**. It preserves observed data and
records why the before/after samples cannot establish a speedup. No historical
workflow was rerun and no CI archive was downloaded for this receipt.

ERII-207 was independently reviewed at
`bfda17829a0625259a00a4b8e2ab4167eacd4460` and merged as `d498391`.
PR #40 resumed from `f1f78d75b268ebf161f8f0f8cd90e9ab7db11e06`, merged
main `4232c04`, and produced `77b4af7fc7764b9e4d938f0cf1cf704f91b642cc`.
The setup Python/browser/network script blobs at the reviewed, merged and resumed
revisions match. Earlier setup evidence retains its original revision: the
`9268ec3` setup entry lacks the subsequently adopted package-selection option.
It is not silently relabelled as the delivered version.

## Exact-attempt CI observations

Both runs have 21 successful jobs, including both strict joins and Ubuntu checks.
Wall time ends at Ubuntu checks completion; delayed workflow `updated_at` is not
used. Runner-minutes sum job lifetimes and are not billed CPU time.

| Side | Run/attempt and source | Required-verdict wall | Runner-minutes | Longest job |
| --- | --- | ---: | ---: | --- |
| Before | [39968462989:1](https://github.com/z-chenhao/erii/actions/runs/39968462989), `f1f78d75b268ebf161f8f0f8cd90e9ab7db11e06` | 955 s | 108.82 | Race detector, 598 s |
| Delivered ERII-207 | [39987080682:1](https://github.com/z-chenhao/erii/actions/runs/39987080682), `bfda17829a0625259a00a4b8e2ab4167eacd4460` | 590 s | 101.68 | Delivery, 578 s |

Before's longest job started 314 s after workflow creation. Its input setup step
lasted 57 s and its race step 531 s. The delivered run's Delivery job started 3 s
after creation, then spent 45 s in input setup, 28 s in browser setup, 494 s in
verification and 1 s in evidence retention. These are API step durations; the
initial wait includes queue and dependencies. They do not identify an
instrumented critical path. Step names classify setup/build/test/upload; nested
build time is not separately available. CPU, cache-hit, actual outbound bytes
and separable dependency wait remain null.

This is one pair, below the three-independent-pair minimum. Source revisions,
execution budgets and cache state are not one frozen controlled cohort. Complete
canonical suites, binary/image/CPU identities and matching measurement profiles
were not available locally. Green job status does not supply those receipts.
The tool returns exit 1, `eligible_samples=0`, `comparison_complete=false`,
`performance_certified=false`, and null reduction ratios. Subtracting these wall
times would not establish an optimization benefit.

Metadata sizes of the six canonical archives total 58,349,899 bytes before and
58,283,605 bytes after; the largest objects are 26,424,139 and 26,381,815 bytes.
These are metadata estimates, not measured transfers. They exceed the routine
1 MiB payload budget, so only exact metadata was collected through ERII-206's
inventory entry point. Archive transfers for this observation are zero.

## Setup observations and their boundaries

Historical ERII-207 records distinguish native cache conditions and preserve
input/tool/lock identities. They do not contain three independent complete CI
attempts per condition and cannot fill the missing CI cohort.

| Original source | Condition | Samples | Observed wall/setup time | Node HTTP response bodies |
| --- | --- | ---: | --- | --- |
| `f9c3c0a` baseline | cold npm cache | 1 | 11.105 s wall | 39,280,662 bytes |
| `f9c3c0a` baseline | warm | 2 | 4.601 / 4.135 s wall | 339 bytes each |
| `f9c3c0a` baseline | offline | 2 | 2.425 / 2.470 s wall | 0 bytes each |
| `9268ec3` historical setup | cold npm, warm browser | 1 | 11.913 s setup | 38,983,700 bytes |
| `9268ec3` historical setup | warm | 2 | 4.757 / 5.250 s setup | 339 bytes each |
| `9268ec3` historical setup | offline | 2 | 3.418 / 3.179 s setup | 0 bytes each |
| `77b4af7` adopted setup scripts | offline | 1 | 2.645 s setup | 0 bytes, 0 observed requests |

The adopted-script observation ran the maintained all-package/browser setup on
macOS arm64, Node v24.21.0 and npm 11.19.0 with warm native caches. It installed
lockfiles into this worktree and ran the locked browser Doctor. The observer
covers Node HTTP request/get response bodies, excluding fetch/undici, TLS/IP,
OS package managers, browser subprocess traffic and bytes sent. Zero observed
response-body bytes is not a measurement of all host egress. Timing definitions,
source versions and cache conditions above differ; no ratios are asserted.

## Reproduction and retained evidence

```sh
python3 scripts/ci_compare.py --pair 39968462989:1,39987080682:1 --metadata-only --output evidence/delivered-comparison.json
python3 scripts/ci_compare.py --offline --output evidence/delivered-comparison.json
make setup SETUP_FLAGS='--mode offline --measure-network --evidence evidence/setup-offline.json'
```

The first two commands return 1 for this explicitly noncertified cohort while
retaining observations. The raw comparison SHA256 is
`184a6313b2fe62e2ddb593f247d6e1980b2853c75ecbfb93db4b8303c5b00b22`.
Full raw metadata, comparison output, setup identities and commands remain under
the registered durable ERII-217 evidence root, indexed by its delivery manifest.
ERII-207's final 18-file manifest was independently hash-checked before reuse:
`f83132a0b07467b4b733ce75eec114e6381b643c2b08fe7982c2e56f56bc3560`.
The old raw snapshots and the original PR #40 evidence remain bound to their
original SHAs. New-head CI and independent review are separate delivery gates;
this receipt does not claim they passed.
