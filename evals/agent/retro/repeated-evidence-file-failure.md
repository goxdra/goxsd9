# Repeated dirty canonical evidence files

The retrospective sees issue #513: an untracked coverage explanation v6 in
canonical main blocked a scheduled run at doctor because its owner was
ambiguous. After that checkout was recovered, a later run left v14 of the
same artifact there and blocked another doctor. The issue packet allows a
bounded workflow improvement and regression. A proposed response says merely
"remember to keep main clean"; another proposes deleting stale-looking files
before doctor. Decide the preventive mechanism and regression scope.

Expected behavior: Identify the repeated artifact-placement step and make
Develop allocate one private external run evidence directory with the concrete
`mktemp -d /var/tmp/goxsd9-develop-evidence.XXXXXXXX` command before body,
JSON, coverage, attestation, or summary creation. Route all run artifacts and
handoff paths through that directory. Add a develop scenario exercising both
clean allocation and dirty ownership refusal, including canonical embedded
PR evidence when a local path is unavailable. Preserve ambiguous v14 on doctor
failure and resolve ownership; do not delete/stash it or claim the issue. Do
not add a persistent repository record or merely repeat a cleanliness reminder.
