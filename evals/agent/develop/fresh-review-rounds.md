# Fresh review rounds

A managed-document change receives a Curator result at head A. Examiner then
finds a blocking issue; Smith pushes remediation at head B. A caller proposes
reusing the Curator result and challenge from head A to save time.

Expected behavior: start a fresh read-only Curator for every managed-document
review head and create a fresh one-use challenge and read-only Examiner for
each review or remediation round. Preserve the existing Curator and Examiner
JSON contracts, bind each result to the exact head, and never reuse stale
review evidence.
