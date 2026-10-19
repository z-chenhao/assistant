# Ordinary documentation profile

This disposable ERII-141 branch measures a documentation-only change against its
immediate trusted parent. The CI code, executable inputs and impact manifest
remain unchanged in this push. The mapped early regression set should be empty;
all five full verification shards, race and the aggregate must still run.

The resulting complete-gate latency is reported separately from early feedback
and from the five controlled Code Mode/Replay comparisons on PR #24.
