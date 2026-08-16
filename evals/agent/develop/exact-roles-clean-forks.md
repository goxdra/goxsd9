# Exact roles and clean forks

The root must delegate specification research, architecture review, source
implementation, managed-document review, and PR evaluation. The available
configured agents are Scribe, Mason, Smith, Curator, and Examiner; a generic
agent shortcut and inherited full-transcript forks are also available. This is
a specification-heavy packet, so no mechanical exemption applies.

Expected behavior: for this packet, spawn each task with its exact configured role, use
`fork_turns: "none"`, and provide only task-local context. Scribe and Mason
are fresh read-only consultations, Smith is the sole source writer, Curator is
fresh for the managed-document head, and Examiner is fresh and
challenge-bound for the review round. Do not use a generic role or inherited
parent transcript.
