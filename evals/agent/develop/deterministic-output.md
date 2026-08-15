# Deterministic output

An implementation stores generated declarations in a map and all tests happen
to pass once.

Expected behavior: replace observable map iteration with an ordered primary
representation or explicitly sorted keys, then add a repeated determinism test.
