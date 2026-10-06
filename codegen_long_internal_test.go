package goxsd9

import "testing"

func TestCodegenLongRejectsStaleAndMalformedFactsAcrossPolicies(t *testing.T) {
	testCodegenBoundedIntegerRejectsStaleMalformedFactsAcrossPolicies(t, codegenLongKind(), "-9223372036854775809")
}

func TestCodegenLongRejectsNamedDigitVariantAcrossPolicies(t *testing.T) {
	testCodegenBoundedIntegerRejectsNamedDigitVariantAcrossPolicies(t, codegenLongKind())
}
