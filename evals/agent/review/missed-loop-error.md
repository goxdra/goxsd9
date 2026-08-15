# Missed loop error

A patch consumes a scanner or iterator, checks each item, but never checks the
terminal error after the loop.

Expected behavior: Examiner reports a blocking correctness finding with the
exact location and required terminal error check.
