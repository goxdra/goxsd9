package goxsd9

import (
	"errors"
	"testing"
)

func TestDiagnosticCodesAreUnique(t *testing.T) {
	definitions := []struct {
		name string
		code string
	}{
		{name: "InvalidIntegerLexicalCode", code: InvalidIntegerLexicalCode},
		{name: "InvalidDecimalLexicalCode", code: InvalidDecimalLexicalCode},
		{name: "InvalidXSDVersionCode", code: InvalidXSDVersionCode},
		{name: "InvalidTotalDigitsCode", code: InvalidTotalDigitsCode},
		{name: "InvalidFractionDigitsCode", code: InvalidFractionDigitsCode},
		{name: "diagnosticPrecisionDecimalLexicalCode", code: diagnosticPrecisionDecimalLexicalCode},
		{name: "diagnosticPrecisionDecimalConstructionCode", code: diagnosticPrecisionDecimalConstructionCode},
		{name: "InvalidDigitFacetCombinationCode", code: InvalidDigitFacetCombinationCode},
		{name: "InvalidDigitFacetRestrictionCode", code: InvalidDigitFacetRestrictionCode},
		{name: "DigitFacetValueViolationCode", code: DigitFacetValueViolationCode},
		{name: "InvalidDigitDatatypeCode", code: InvalidDigitDatatypeCode},
		{name: "InvalidXMLSyntaxCode", code: InvalidXMLSyntaxCode},
		{name: "InvalidSchemaRootCode", code: InvalidSchemaRootCode},
		{name: "UnsupportedSchemaSyntaxCode", code: UnsupportedSchemaSyntaxCode},
		{name: "SourceReadCode", code: SourceReadCode},
		{name: "SourceCloseCode", code: SourceCloseCode},
		{name: "SourceResolveCode", code: SourceResolveCode},
		{name: "SourceInvalidCode", code: SourceInvalidCode},
		{name: "MissingSchemaLocationCode", code: MissingSchemaLocationCode},
		{name: "invalidSchemaTargetNamespaceCode", code: invalidSchemaTargetNamespaceCode},
		{name: "invalidSchemaCompositionCode", code: invalidSchemaCompositionCode},
		{name: "invalidSchemaDeclarationNameCode", code: invalidSchemaDeclarationNameCode},
		{name: "diagnosticSchemaBridgeInvariantCode", code: diagnosticSchemaBridgeInvariantCode},
		{name: "diagnosticUnsupportedWithoutFeatureCode", code: diagnosticUnsupportedWithoutFeatureCode},
		{name: "diagnosticUnregisteredFeatureCode", code: diagnosticUnregisteredFeatureCode},
		{name: "diagnosticIntegerConstructionCode", code: diagnosticIntegerConstructionCode},
		{name: "diagnosticDecimalConstructionCode", code: diagnosticDecimalConstructionCode},
		{name: "diagnosticSyntaxNoReaderCode", code: diagnosticSyntaxNoReaderCode},
		{name: "diagnosticSyntaxEmptyTokenCode", code: diagnosticSyntaxEmptyTokenCode},
		{name: "diagnosticSyntaxUnsupportedTokenCode", code: diagnosticSyntaxUnsupportedTokenCode},
		{name: "diagnosticSyntaxAssertionFeatureCode", code: diagnosticSyntaxAssertionFeatureCode},
		{name: "diagnosticSyntaxFeatureCode", code: diagnosticSyntaxFeatureCode},
		{name: "diagnosticSyntaxUnclassifiedErrorCode", code: diagnosticSyntaxUnclassifiedErrorCode},
		{name: "diagnosticSchemaEmptySourceCode", code: diagnosticSchemaEmptySourceCode},
		{name: "diagnosticSchemaRepeatedSourceCode", code: diagnosticSchemaRepeatedSourceCode},
		{name: "diagnosticSchemaEmptyKindCode", code: diagnosticSchemaEmptyKindCode},
		{name: "diagnosticSchemaEmptyNameCode", code: diagnosticSchemaEmptyNameCode},
		{name: "diagnosticSyntaxDocumentNoRootCode", code: diagnosticSyntaxDocumentNoRootCode},
		{name: "diagnosticDigitRestrictionKindCode", code: diagnosticDigitRestrictionKindCode},
		{name: "diagnosticDigitRestrictionVersionCode", code: diagnosticDigitRestrictionVersionCode},
		{name: "diagnosticDigitTotalStateCode", code: diagnosticDigitTotalStateCode},
		{name: "diagnosticDigitFractionStateCode", code: diagnosticDigitFractionStateCode},
		{name: "diagnosticDigitValueConstructionCode", code: diagnosticDigitValueConstructionCode},
		{name: "diagnosticDigitEffectiveKindCode", code: diagnosticDigitEffectiveKindCode},
		{name: "diagnosticDigitEffectiveVersionCode", code: diagnosticDigitEffectiveVersionCode},
		{name: "diagnosticDigitIntegerFractionCode", code: diagnosticDigitIntegerFractionCode},
	}
	seen := make(map[string]string, len(definitions))
	for _, definition := range definitions {
		if previous, exists := seen[definition.code]; exists {
			t.Fatalf("diagnostic code %q is assigned to %s and %s", definition.code, previous, definition.name)
		}
		seen[definition.code] = definition.name
	}
}

func TestUnsupportedDiagnostic(t *testing.T) {
	loc, err := NewLoc("schema.xsd", 3, 7)
	if err != nil {
		t.Fatalf("NewLoc: %v", err)
	}
	feature, ok := LookupUnsupportedFeature("xsd.assertion")
	if !ok {
		t.Fatal("xsd.assertion is not registered")
	}
	diagnostic := newUnsupported(feature, "XSD1001", loc, "assertions are not implemented")

	if !errors.Is(diagnostic, ErrUnsupported) {
		t.Fatal("unsupported diagnostic does not match ErrUnsupported")
	}
	if got, want := diagnostic.Error(), "schema.xsd:3:7: XSD1001: assertions are not implemented"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
	if got, want := diagnostic.Feature(), FeatureID("xsd.assertion"); got != want {
		t.Fatalf("Feature() = %q, want %q", got, want)
	}
	if got, want := diagnostic.SpecRef(), "xsd11-structures#cAssertions"; got != want {
		t.Fatalf("SpecRef() = %q, want %q", got, want)
	}
}

func TestUnsupportedDiagnosticRejectsUnregisteredFeature(t *testing.T) {
	diagnostic := newUnsupported(UnsupportedFeature{}, "XSD1001", Loc{}, "assertions are not implemented")
	if diagnostic.Class() != FailureInternal {
		t.Fatalf("Class() = %q, want %q", diagnostic.Class(), FailureInternal)
	}
	if diagnostic.Feature() != "" {
		t.Fatalf("Feature() = %q, want an empty ID", diagnostic.Feature())
	}
}

func TestGenericUnsupportedDiagnosticBecomesInternal(t *testing.T) {
	diagnostic := newDiagnostic(FailureUnsupported, "XSD1001", Loc{}, "placeholder", nil)
	if diagnostic.Class() != FailureInternal {
		t.Fatalf("Class() = %q, want %q", diagnostic.Class(), FailureInternal)
	}
	if diagnostic.Feature() != "" {
		t.Fatalf("Feature() = %q, want an empty ID", diagnostic.Feature())
	}
}

func TestDiagnosticsPreserveOrderAndOwnership(t *testing.T) {
	first := newDiagnostic(FailureInvalid, "XSD0001", Loc{}, "first", nil)
	second := newDiagnostic(FailureResolution, "XSD0002", Loc{}, "second", errors.New("offline"))
	input := []Diagnostic{first, second}
	diagnostics := makeDiagnostics(input)
	input[0] = second

	firstItem, ok := diagnostics.At(0)
	if !ok {
		t.Fatal("At(0) did not find a diagnostic")
	}
	if got, want := firstItem.Code(), "XSD0001"; got != want {
		t.Fatalf("At(0).Code() = %q, want %q", got, want)
	}
	all := diagnostics.All()
	all[0] = second
	firstItem, ok = diagnostics.At(0)
	if !ok {
		t.Fatal("At(0) did not find a diagnostic after copy mutation")
	}
	if got, want := firstItem.Code(), "XSD0001"; got != want {
		t.Fatalf("At(0).Code() after copy mutation = %q, want %q", got, want)
	}
	if !errors.Is(diagnostics, second.Unwrap()) {
		t.Fatal("aggregate does not expose nested cause")
	}
}
