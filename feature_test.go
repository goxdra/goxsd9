package goxsd9

import (
	"errors"
	"testing"
)

func TestFeatureRegistryValidationAndLookup(t *testing.T) {
	if err := ValidateFeatureRegistry(); err != nil {
		t.Fatalf("ValidateFeatureRegistry: %v", err)
	}
	feature, ok := LookupUnsupportedFeature("xsd.assertion")
	if !ok {
		t.Fatal("LookupUnsupportedFeature did not find xsd.assertion")
	}
	if got, want := feature.Title(), "XSD assertions"; got != want {
		t.Fatalf("Title() = %q, want %q", got, want)
	}
	if _, ok := LookupUnsupportedFeature("xsd.unknown"); ok {
		t.Fatal("LookupUnsupportedFeature accepted an unknown ID")
	}
	for _, id := range []FeatureID{FeatureDatatypeFacets, FeaturePrecisionDecimal} {
		if _, ok := LookupUnsupportedFeature(id); !ok {
			t.Fatalf("LookupUnsupportedFeature did not find %q", id)
		}
	}
}

func TestUnsupportedFeaturesAreOrderedAndOwned(t *testing.T) {
	features := UnsupportedFeatures()
	for index, feature := range features {
		if !feature.Registered() {
			t.Fatalf("feature %d is not registered", index)
		}
		if index > 0 && features[index-1].ID() >= feature.ID() {
			t.Fatalf("features are not strictly sorted: %#v", features)
		}
	}
}

func TestReportUnsupportedFeaturesRanksAndRejectsUnknownIDs(t *testing.T) {
	feature, ok := LookupUnsupportedFeature("xsd.assertion")
	if !ok {
		t.Fatal("xsd.assertion is not registered")
	}
	first := newUnsupported(feature, "XSD1001", Loc{}, "assertions are not implemented")
	second := newUnsupported(feature, "XSD1002", Loc{}, "assertions are still not implemented")
	diagnostics := makeDiagnostics([]Diagnostic{
		newDiagnostic(FailureInvalid, "XSD0001", Loc{}, "invalid", nil),
		first,
		second,
	})
	report, err := ReportUnsupportedFeatures(diagnostics)
	if err != nil {
		t.Fatalf("ReportUnsupportedFeatures: %v", err)
	}
	if len(report) != 1 {
		t.Fatalf("report has %d rows, want 1", len(report))
	}
	if got, want := report[0].Feature().ID(), FeatureID("xsd.assertion"); got != want {
		t.Fatalf("report feature = %q, want %q", got, want)
	}
	if got, want := report[0].Count(), 2; got != want {
		t.Fatalf("report count = %d, want %d", got, want)
	}

	diagnostics = makeDiagnostics([]Diagnostic{{class: FailureUnsupported, feature: "xsd.unknown"}})
	if _, err := ReportUnsupportedFeatures(diagnostics); err == nil {
		t.Fatal("ReportUnsupportedFeatures accepted an unknown ID")
	}
	if !errors.Is(first, ErrUnsupported) {
		t.Fatal("registered unsupported diagnostic does not match ErrUnsupported")
	}
}
