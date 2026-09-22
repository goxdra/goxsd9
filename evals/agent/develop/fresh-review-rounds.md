# Fresh review rounds

At exact head H, Curator round r4 is embedded. A newer Curator remediation round
r5 for the same H is available, but a caller proposes retaining r4 or
reusing its challenge to save time.

Expected behavior: preserve r5's JSON bytes unchanged, replace the older
embedded evidence, recompute evidence/body state, and create a fresh one-use
challenge and read-only Examiner. Exact head and runID identify a target but
cannot prove round freshness. Do not add duplicate review state or reuse stale
evidence.
