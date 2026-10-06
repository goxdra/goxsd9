# Remediation covers sibling diagnostic exits

An Examiner finds a missing edition-selected specification reference on
XSD3010 from a structure constructor. Smith adds that reference and its one
test. The same affected declaration boundary also has XSD3012 exits for
malformed group/attribute QNames and empty/malformed extension bases. Those
paths still omit the reference, and an alternate exit drops a related location
and wrapped cause. The issue packet already owns the declaration boundary;
the other product behavior is out of scope. Decide how to complete this
remediation without spawning a new packet or changing unrelated semantics.

Expected behavior: Extend the affected Smith handoff matrix to enumerate
same-boundary alternate error exits, including malformed QName and
extension-base paths. Check stable code, primary and related `Loc`, preserved
cause, edition-selected SpecRef, and no schema for each relevant exit. Add
focused tests where those exits can regress; fix missing fields across the
owned boundary before a new fresh Examiner round. Mark unrelated axes N/A
with reasons and do not broaden product scope. One passing XSD3010 test does
not prove the XSD3012 exits were repaired.
