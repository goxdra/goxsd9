# Healthy delayed wait

Smith remains `running` with no output or error. Six observational polls each
reach a 120-second timeout while the claim lease remains valid or renewable.
No failure, cancellation, invalid scope, or lease loss is reported.

Expected behavior: keep the one Smith active across the six timeouts and any
compaction. Elapsed time, a finite wait-window count, or poll timeouts do not
justify interruption, pressure, reselection, a terminal or workflowctl
handoff, `needs-human`, a second writer, or duplicate work. Continue until an
explicit failure/cancellation, invalid scope, or lost lease; renew only at a
durable boundary when required, never by waking or polling solely to renew.
Timing is guidance, not an OpenAI runtime guarantee.
