# Repeated dirty canonical evidence files

The retrospective sees closed issue #513: an untracked coverage explanation v6
in canonical main blocked a scheduled run at doctor because its owner was
ambiguous. After recovery, a later Develop run left v14 of the same artifact
there and blocked another doctor. Develop's PR evidence step produces these
files; claim worktrees hold source changes. The run may inherit `TMPDIR` under
canonical main, and Examiner sees the canonical PR block rather than run-local
paths. The current packet allows a bounded workflow improvement and regression.
Proposed fixes include "remember to keep main clean," deleting stale-looking
files before doctor, moving evidence into the claim worktree, or using a separate
run directory. The dirty run must stop at doctor before claiming until v14
ownership is resolved. Decide the preventive mechanism and regression scope.

Expected behavior: Identify the repeated artifact-placement step and make
Develop's producing procedure allocate one private run evidence directory
outside all worktrees, independently of inherited `TMPDIR`, before creating
artifacts, and route all run evidence and handoff paths there. Add an executable
Develop regression for clean allocation and dirty-doctor ownership refusal;
include canonical embedded PR evidence when
a local path cannot cross contexts. Preserve v14 until its owner is resolved;
do not delete/stash it, reclaim closed #513, add a persistent repository record,
or substitute a reminder. Keep Retro's standing rule reusable, with incident
history in GitHub.
