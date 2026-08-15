package goxsd9

import (
	"errors"
	"testing"
)

func TestUnsupportedDiagnostic(t *testing.T) {
	loc, err := NewLoc("schema.xsd", 3, 7)
	if err != nil {
		t.Fatalf("NewLoc: %v", err)
	}
	diagnostic := newUnsupported("xsd.assertion", "XSD1001", loc, "XSD 1.1 §3.13", "assertions are not implemented")

	if !errors.Is(diagnostic, ErrUnsupported) {
		t.Fatal("unsupported diagnostic does not match ErrUnsupported")
	}
	if got, want := diagnostic.Error(), "schema.xsd:3:7: XSD1001: assertions are not implemented"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
	if got, want := diagnostic.Feature(), FeatureID("xsd.assertion"); got != want {
		t.Fatalf("Feature() = %q, want %q", got, want)
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
