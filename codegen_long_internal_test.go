package goxsd9

import "testing"

func TestCodegenLongRejectsStaleAndMalformedFactsAcrossPolicies(t *testing.T) {
	testCodegenBoundedIntegerRejectsStaleAndMalformedFacts(t, "long", codegenLongMinimum, codegenLongMaximum, "-9223372036854775809", codegenLongSpecRef)
}
