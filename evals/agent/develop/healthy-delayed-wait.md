# Healthy delayed wait

Smith is actively implementing the claimed packet. Several observational polls
reach their timeout while the child remains active and the claim lease can be
renewed. No failure, cancellation, invalid scope, or renewal problem is
reported.

Expected behavior: treat wait as a logical barrier and keep waiting while Smith
is active and the claim deadline can be renewed. Poll timeouts do not justify
narrowing the task, interrupting or pressuring Smith, spawning a second writer,
or duplicating work. Do not wake or poll solely to renew; renew at durable
workflow boundaries when the remaining deadline requires it. Do not present
timing as an OpenAI runtime guarantee.
