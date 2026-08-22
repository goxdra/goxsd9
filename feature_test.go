package goxsd9

import (
	"errors"
	"testing"
)

func TestFeatureRegistryValidationAndLookup(t *testing.T) {
	if err := ValidateFeatureRegistry(); err != nil {
		t.Fatalf("ValidateFeatureRegistry: %v", err)
	}
	feature, found := LookupUnsupportedFeature("xsd.assertion")
	if !found {
		t.Fatal("LookupUnsupportedFeature did not find xsd.assertion")
	}
	if got, want := feature.Title(), "XSD assertions"; got != want {
		t.Fatalf("Title() = %q, want %q", got, want)
	}
	if _, unknownFound := LookupUnsupportedFeature("xsd.unknown"); unknownFound {
		t.Fatal("LookupUnsupportedFeature accepted an unknown ID")
	}
	for _, id := range []FeatureID{FeatureCodegen, FeatureDatatypeFacets, FeatureInstanceSyntax, FeatureInstanceValidation, FeaturePrecisionDecimal} {
		if _, featureFound := LookupUnsupportedFeature(id); !featureFound {
			t.Fatalf("LookupUnsupportedFeature did not find %q", id)
		}
	}
	codegen, found := LookupUnsupportedFeature(FeatureCodegen)
	if !found || codegen.Title() != "Go code generation outside supported scalar declarations" {
		t.Fatalf("codegen feature title = %q, want stable title", codegen.Title())
	}
	if references := codegen.References(); len(references) != 10 ||
		references[0].Source() != "xsd10-structures#Simple_Type_Definitions" ||
		references[1].Source() != "xsd10-structures#Element_Declaration_details" ||
		references[2].Source() != "xsd10-structures#cParticles" ||
		references[3].Source() != "xsd10-structures#element-choice" ||
		references[4].Source() != "xsd10-structures#Particle_details" ||
		references[5].Source() != "xsd11-structures#Simple_Type_Definition" ||
		references[6].Source() != "xsd11-structures#Element_Declaration_details" ||
		references[7].Source() != "xsd11-structures#cParticles" ||
		references[8].Source() != "xsd11-structures#element-choice" ||
		references[9].Source() != "xsd11-structures#Particle_details" {
		t.Fatalf("codegen feature references = %#v, want scalar and direct-choice sections for XSD 1.0 and 1.1", references)
	}
	validationFeature, found := LookupUnsupportedFeature(FeatureInstanceValidation)
	if !found {
		t.Fatal("LookupUnsupportedFeature did not find instance validation")
	}
	references := validationFeature.References()
	if len(references) != 2 || references[0].Source() != "xsd10-structures#cvc-elt" || references[1].Source() != "xsd11-structures#cvc-elt" {
		t.Fatalf("instance validation references = %#v, want XSD 1.0 and 1.1 cvc-elt", references)
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
