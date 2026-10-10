package goxsd9

import "testing"

func TestGenerateGoNamedLongRejectsStaleEnumerationFacetsAcrossPolicies(t *testing.T) {
	testGenerateGoNamedBoundedIntegerRejectsStaleEnumerationFacets(t, "long", codegenLongMinimum, "9223372036854775808", codegenLongSpecRef)
}
