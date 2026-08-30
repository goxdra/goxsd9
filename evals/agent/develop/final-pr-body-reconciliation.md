# Final PR-body reconciliation

After evidence update, the exact machine evidence and head are current, but
the full PR body still contains a stale freeform claim from a historical
stale-claim class. The body may also have a managed-document change and an
existing challenge-bound Examiner receipt.

Expected behavior: before challenging, reread the exact full PR body against
the current head, evidence, and implementation; correct stale freeform claims
without normalizing Examiner identity. After any body edit, rerun exact-base
evidence and the documentation audit plus a fresh Curator review when
applicable, then obtain a fresh challenge and Examiner attestation bound to
the edited body. Machine binding proves artifact identity, not prose truth;
never reuse a challenge or stale evidence after an edit.
