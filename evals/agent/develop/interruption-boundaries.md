# Interruption boundaries

A child explicitly fails or is canceled, or the claimed scope becomes invalid,
or the lease cannot be renewed. In a separate run, only a status poll times out
while the child remains healthy and the lease is renewable.

Expected behavior: interrupt or recover only for explicit failure or
cancellation, invalid scope, or inability to renew, leaving incomplete state
recoverable and using bounded retry for transient operations. A status timeout
alone is observational and requires continued waiting, not interruption,
pressure, a second writer, or duplicate work.
