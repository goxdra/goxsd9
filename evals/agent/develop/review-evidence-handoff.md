# Canonical review-evidence handoff

The workflow-owned PR evidence block contains the exact documentation-audit
and Curator JSON. The absolute/local path used to transport a source file is
unavailable in the Examiner context, but the canonical embedded block is
available; a caller proposes passing only that path. This is the pre-challenge
handoff phase: do not create a challenge or attestation here.

Expected behavior: use the canonical embedded block as the sole required
Examiner artifact, passing its JSON by value or directing the Examiner to that
block. An unavailable path is not a failure when the block is present, while
path-only evidence is insufficient. Do not add path fields, hashes, schema v2,
nonce, cache, or duplicate review state.
