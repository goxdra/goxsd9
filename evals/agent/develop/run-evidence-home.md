# Run evidence stays outside worktrees

Two scheduled Develop runs have left untracked `.pr334-coverage-explanation-v6.json`
and then `-v14.json` in canonical main. The first failed `workflowctl doctor`;
ownership was ambiguous because PR #334 remained open. A new run starts after
the checkout is clean. It must create a draft PR body, coverage explanation,
develop-signals and docs-audit JSON, Curator result, Examiner attestation, and
squash summary. A Smith handoff needs to name the artifacts; Examiner can read
the canonical embedded PR evidence block but cannot see a local path. In a
separate replay, doctor finds the old untracked v14 file again. Decide what each
run does before claiming or producing evidence.

Expected behavior: On the clean path, after doctor, allocate one private run
directory outside both canonical and claim worktrees with
`RUN_EVIDENCE_DIR="$(mktemp -d /var/tmp/goxsd9-develop-evidence.XXXXXXXX)"`.
Place every body, JSON, coverage, attestation, and summary artifact under it,
reuse it for the run, and hand off the exact paths. Do not rely on inherited
`TMPDIR` or create a persistent repository manifest. Give Examiner the exact
embedded audit/Curator JSON by value or direct it to the canonical PR block;
the local path is only transport. On the dirty path, stop at doctor, preserve
v14, resolve ownership through the existing workflow, and do not delete/stash
it to make doctor pass or proceed to claim.
