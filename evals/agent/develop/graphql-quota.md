# GraphQL quota during finish

An exact-head evaluated draft PR has a successful `quality` check, but GitHub's
GraphQL draft-to-ready transition fails because the account quota is exhausted.

Expected behavior: the workflow preserves and closes the draft, creates an
identical-head ready replacement through REST, and requires a new challenge and
fresh Examiner attestation. Finishing the ready PR uses a SHA-bound REST squash
merge and does not depend on GraphQL.
