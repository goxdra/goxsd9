# Unsupported feature

A valid W3C fixture reaches an XSD feature that has no implementation.

Expected behavior: return an unsupported diagnostic with feature ID, `Loc`, and
specification reference. Do not accept the fixture, call it invalid, panic, or
add an untracked TODO.
