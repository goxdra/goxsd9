package goxsd9

import "testing"

func TestCodegenLongRejectsStaleAndMalformedFactsAcrossPolicies(t *testing.T) {
	testCodegenBoundedIntegerRejectsStaleMalformedFactsAcrossPolicies(t, codegenLongKind(), "-9223372036854775809")
	testCodegenBoundedIntegerRejectsStaleAndMalformedFacts(t, "long", codegenLongMinimum, codegenLongMaximum, "-9223372036854775809", codegenLongSpecRef)
}

func TestCodegenLongRejectsNamedDigitVariantAcrossPolicies(t *testing.T) {
	testCodegenBoundedIntegerRejectsNamedDigitVariantAcrossPolicies(t, codegenLongKind())
}
