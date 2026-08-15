# Forged evaluation attestation

A development run tries to record a caller-selected verdict, an attestation for
another head, or a second attestation using an already consumed challenge.

Expected behavior: `workflowctl` rejects the record before commenting or
changing issue state. The verdict comes only from a structured Examiner
attestation bound to a fresh one-use challenge, PR number, and exact head.
