package feature

import "testing"

func TestValidateRegistry(t *testing.T) {
	if err := ValidateRegistry(); err != nil {
		t.Fatalf("ValidateRegistry: %v", err)
	}
}

func TestValidateDefinitionsRejectsDuplicateUnsortedAndMalformedIDs(t *testing.T) {
	valid := definition{
		id:    "xsd.assertion",
		title: "XSD assertions",
		references: []Reference{
			{version: "1.1", source: "xsd11-structures#cAssertions"},
		},
	}
	tests := []struct {
		name        string
		definitions []definition
	}{
		{name: "duplicate", definitions: []definition{valid, valid}},
		{name: "unsorted", definitions: []definition{
			{id: "xsd.zeta", title: "Zeta", references: valid.references},
			{id: "xsd.alpha", title: "Alpha", references: valid.references},
		}},
		{name: "empty", definitions: []definition{{title: valid.title, references: valid.references}}},
		{name: "bare namespace", definitions: []definition{{id: "xsd", title: valid.title, references: valid.references}}},
		{name: "wrong namespace", definitions: []definition{{id: "xpath.assertion", title: valid.title, references: valid.references}}},
		{name: "uppercase", definitions: []definition{{id: "xsd.Assertion", title: valid.title, references: valid.references}}},
		{name: "repeated dot", definitions: []definition{{id: "xsd..assertion", title: valid.title, references: valid.references}}},
		{name: "leading hyphen", definitions: []definition{{id: "xsd.-assertion", title: valid.title, references: valid.references}}},
		{name: "trailing hyphen", definitions: []definition{{id: "xsd.assertion-", title: valid.title, references: valid.references}}},
		{name: "whitespace", definitions: []definition{{id: "xsd.assertion name", title: valid.title, references: valid.references}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateDefinitions(test.definitions); err == nil {
				t.Fatal("validateDefinitions accepted invalid definitions")
			}
		})
	}
}

func TestAllAndLookupPreserveStableOrderAndOwnership(t *testing.T) {
	features := All()
	if len(features) != len(registry) {
		t.Fatalf("All() returned %d features, want %d", len(features), len(registry))
	}
	for index, feature := range features {
		if !feature.Registered() {
			t.Fatalf("feature %d is not registered", index)
		}
		if index > 0 && features[index-1].ID() >= feature.ID() {
			t.Fatalf("features are not strictly sorted: %#v", features)
		}
		found, ok := Lookup(feature.ID())
		if !ok || found.ID() != feature.ID() {
			t.Fatalf("Lookup(%q) = (%#v, %t)", feature.ID(), found, ok)
		}
	}
	if _, ok := Lookup("xsd.unknown"); ok {
		t.Fatal("Lookup accepted an unknown feature ID")
	}

	references := features[0].References()
	references[0] = Reference{version: "1.0", source: "wrong#reference"}
	if got, want := features[0].SpecRef(), "xsd11-structures#cAssertions"; got != want {
		t.Fatalf("SpecRef() after reference mutation = %q, want %q", got, want)
	}
}

func TestReferenceExposesGenericVersionWithXSDCompatibilityAlias(t *testing.T) {
	reference := Reference{version: "1.0"}
	if got, want := reference.Version(), "1.0"; got != want {
		t.Fatalf("Version() = %q, want %q", got, want)
	}
	if got, want := reference.XSDVersion(), "1.0"; got != want {
		t.Fatalf("XSDVersion() = %q, want %q", got, want)
	}
}
