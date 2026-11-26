package goxsd9

import (
	"errors"
	"testing"
)

func notationPolicies() []struct {
	name   string
	policy LanguagePolicy
} {
	return []struct {
		name   string
		policy LanguagePolicy
	}{
		{name: "compatibility", policy: Compatibility},
		{name: "strict10", policy: Strict10},
		{name: "strict11", policy: Strict11},
	}
}

//nolint:gocognit // Each case checks the complete public identifier-presence contract.
func TestNotationIdentifierPresenceAndCopiedFacts(t *testing.T) {
	cases := []struct {
		name       string
		attributes string
		public     string
		publicLoc  string
		system     string
		systemLoc  string
	}{
		{"system only", `system="urn:system"`, "", "", "urn:system", `system="urn:system"`},
		{"empty system only", `system=""`, "", "", "", `system=""`},
		{"empty public only", `public=""`, "", `public=""`, "", ""},
		{"whitespace public only", `public=" &#9; &#10; "`, "", `public="`, "", ""},
		{"public only", `public=" alpha&#9;  beta "`, "alpha beta", `public="`, "", ""},
		{"both", `public="name" system=" urn:system "`, "name", `public="name"`, "urn:system", `system="`},
	}
	for _, policy := range notationPolicies() {
		for _, tc := range cases {
			t.Run(policy.name+"/"+tc.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:n">
  <xs:notation name="first" ` + tc.attributes + `/>
</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
				if err != nil {
					t.Fatalf("parse: %v", err)
				}
				components := schema.Components()
				if len(components) != 1 {
					t.Fatalf("components = %d, want 1", len(components))
				}
				component := components[0]
				notation, ok := component.Notation()
				if !ok || component.Kind() != ComponentKindNotationDeclaration || notation.Name() != mustTestQName(t, "urn:n", "first") || notation.ID() != component.ID() || component.ID().Ordinal() != 1 {
					t.Fatalf("notation identity = %#v, found=%t", component, ok)
				}
				if got, want := notation.Loc(), complexContentTestLoc(t, root, `<xs:notation`); got != want {
					t.Fatalf("declaration Loc = %s, want %s", got, want)
				}
				if got := notation.Public(); got != tc.public {
					t.Fatalf("public = %q, want %q", got, tc.public)
				}
				wantPublicLoc := Loc{}
				if tc.publicLoc != "" {
					wantPublicLoc = complexContentTestLoc(t, root, tc.publicLoc)
				}
				if got := notation.PublicLoc(); got != wantPublicLoc {
					t.Fatalf("public Loc = %s, want %s", got, wantPublicLoc)
				}
				gotSystem, hasSystem := notation.System()
				wantSystemLoc := Loc{}
				if tc.systemLoc != "" {
					wantSystemLoc = complexContentTestLoc(t, root, tc.systemLoc)
				}
				gotSystemLoc, hasSystemLoc := notation.SystemLoc()
				if gotSystem != tc.system || hasSystem != (tc.systemLoc != "") || gotSystemLoc != wantSystemLoc || hasSystemLoc != hasSystem {
					t.Fatalf("system = (%q,%t) at (%s,%t), want (%q,%t) at %s", gotSystem, hasSystem, gotSystemLoc, hasSystemLoc, tc.system, tc.systemLoc != "", wantSystemLoc)
				}
				components[0] = Component{}
				found := schema.FindKind(ComponentKindNotationDeclaration, notation.Name())
				if len(found) != 1 || found[0].ID() != notation.ID() {
					t.Fatalf("FindKind after slice mutation = %#v", found)
				}
				found[0] = Component{}
				lookedUp, exists := schema.Lookup(notation.ID())
				if !exists {
					t.Fatal("Lookup lost notation")
				}
				copyView, exists := lookedUp.Notation()
				if !exists || copyView.Public() != tc.public || copyView.PublicLoc() != wantPublicLoc {
					t.Fatal("Lookup changed copied identifier facts")
				}
			})
		}
	}
}

//nolint:gocognit // The graph cases share the same public namespace and order checks.
func TestSystemOnlyNotationGraphNamespaceAndOrder(t *testing.T) {
	const notation = `<xs:notation name="late" system=" urn:late "/>`
	for _, policy := range notationPolicies() {
		for _, graph := range []struct {
			name       string
			root       string
			child      string
			namespace  string
			source     SourceID
			componentN int
		}{
			{"same document", `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:owner"><xs:element name="early"/>` + notation + `</xs:schema>`, "", "urn:owner", "root.xsd", 2},
			{"include", `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:owner"><xs:include schemaLocation="child.xsd"/><xs:element name="early"/></xs:schema>`, `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:owner">` + notation + `</xs:schema>`, "urn:owner", "child.xsd", 2},
			{"import", `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:owner"><xs:import namespace="urn:other" schemaLocation="child.xsd"/><xs:element name="early"/></xs:schema>`, `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other">` + notation + `</xs:schema>`, "urn:other", "child.xsd", 2},
			{"chameleon include", `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:owner"><xs:include schemaLocation="child.xsd"/><xs:element name="early"/></xs:schema>`, `<xs:schema xmlns:xs="` + testXSDNamespace + `">` + notation + `</xs:schema>`, "urn:owner", "child.xsd", 2},
		} {
			t.Run(policy.name+"/"+graph.name, func(t *testing.T) {
				var fixtures map[string]discoveryFixture
				if graph.child != "" {
					fixtures = map[string]discoveryFixture{"child.xsd": {id: "child.xsd", contents: graph.child}}
				}
				schema, err := discoverTestSchemaWithPolicy(t, graph.root, fixtures, policy.policy)
				if err != nil {
					t.Fatalf("parse: %v", err)
				}
				components := schema.Components()
				if len(components) != graph.componentN || components[0].Name() != mustTestQName(t, "urn:owner", "early") || components[1].Kind() != ComponentKindNotationDeclaration {
					t.Fatalf("component discovery/lexical order = %#v", components)
				}
				name := mustTestQName(t, graph.namespace, "late")
				found := schema.FindKind(ComponentKindNotationDeclaration, name)
				if len(found) != 1 || found[0].ID() != components[1].ID() || found[0].Document() != graph.source {
					t.Fatalf("FindKind(%s) = %#v", name, found)
				}
				view, ok := found[0].Notation()
				contents := graph.root
				if graph.child != "" {
					contents = graph.child
				}
				if !ok || !view.PublicLoc().IsZero() || view.Public() != "" || view.Loc() != complexContentTestSourceLoc(t, graph.source, contents, `<xs:notation`) {
					t.Fatalf("notation facts = %#v, found=%t", view, ok)
				}
				if got, present := view.System(); !present || got != "urn:late" {
					t.Fatalf("system = (%q,%t)", got, present)
				}
				if got, present := view.SystemLoc(); !present || got != complexContentTestSourceLoc(t, graph.source, contents, `system="`) {
					t.Fatalf("system Loc = (%s,%t)", got, present)
				}
				var walked []ComponentID
				if err := schema.Walk(func(component Component) error {
					walked = append(walked, component.ID())
					return nil
				}); err != nil {
					t.Fatalf("Walk: %v", err)
				}
				if len(walked) != 2 || walked[0] != components[0].ID() || walked[1] != components[1].ID() {
					t.Fatalf("walk order = %#v", walked)
				}
			})
		}
	}
}

//nolint:gocognit // Every alternate identifier failure verifies its public diagnostic contract.
func TestNotationIdentifierAlternateFailures(t *testing.T) {
	for _, policy := range notationPolicies() {
		for _, tc := range []struct {
			name, attributes, marker string
			cause                    error
		}{
			{"neither", "", `<xs:notation`, errSchemaNotationPublic},
			{"illegal public", `public="bad["`, `public="`, errSchemaNotationPublic},
			{"invalid system only", `system="http://[bad"`, `system="`, errSchemaNotationSystem},
			{"invalid system with public", `public="ok" system="http://[bad"`, `system="`, errSchemaNotationSystem},
		} {
			t.Run(policy.name+"/"+tc.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:notation name="n" ` + tc.attributes + `/></xs:schema>`
				var previous Diagnostic
				for repeat := 0; repeat < 3; repeat++ {
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
					if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
						t.Fatal("invalid notation yielded a schema")
					}
					diagnostic := requireDiagnostic(t, err)
					if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaNotationCode || diagnostic.Loc() != complexContentTestLoc(t, root, tc.marker) || diagnostic.SpecRef() != schemaNotationSpecRef(idAllVersion(policy.policy)) || !errors.Is(err, tc.cause) {
						t.Fatalf("diagnostic = %s, want located notation failure with %v", diagnostic, tc.cause)
					}
					if len(diagnostic.Related()) != 0 {
						t.Fatalf("related = %v, want none", diagnostic.Related())
					}
					if errors.Is(err, errSchemaNotationSystem) {
						var uriDiagnostic Diagnostic
						if !errors.As(diagnostic.Unwrap(), &uriDiagnostic) || uriDiagnostic.Code() != invalidSchemaCompositionCode || uriDiagnostic.Loc() != diagnostic.Loc() {
							t.Fatalf("system cause = %v, want located anyURI diagnostic", diagnostic.Unwrap())
						}
					}
					if repeat > 0 {
						assertSameSchemaDiagnostic(t, previous, diagnostic)
					}
					previous = diagnostic
				}
			})
		}
	}
}

//nolint:gocognit // The four notation-reference shapes must retain their distinct diagnostics.
func TestSystemOnlyNotationDoesNotWidenReferenceShapes(t *testing.T) {
	for _, policy := range notationPolicies() {
		for _, tc := range []struct {
			name, body, marker, code string
			class                    FailureClass
		}{
			{"direct builtin", `<xs:element name="root" type="xs:NOTATION"/>`, `type="xs:NOTATION"`, UnsupportedSchemaSyntaxCode, FailureUnsupported},
			{"named builtin base", `<xs:simpleType name="N"><xs:restriction base="xs:NOTATION"/></xs:simpleType><xs:element name="root" type="t:N"/>`, `base="xs:NOTATION"`, UnsupportedSchemaSyntaxCode, FailureUnsupported},
			{"inline builtin base", `<xs:element name="root"><xs:simpleType><xs:restriction base="xs:NOTATION"/></xs:simpleType></xs:element>`, `base="xs:NOTATION"`, UnsupportedSchemaSyntaxCode, FailureUnsupported},
			{"notation as type ref", `<xs:element name="root" type="t:n"/>`, `type="t:n"`, diagnosticSchemaElementTypeWrongKindCode, FailureInvalid},
			{"attribute builtin", `<xs:attribute name="value" type="xs:NOTATION"/>`, `type="xs:NOTATION"`, UnsupportedSchemaSyntaxCode, FailureUnsupported},
		} {
			t.Run(policy.name+"/"+tc.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:t" targetNamespace="urn:t"><xs:notation name="n" system="urn:n"/>` + tc.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
				if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
					t.Fatal("notation reference yielded a schema")
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != tc.class || diagnostic.Code() != tc.code || diagnostic.Loc() != complexContentTestLoc(t, root, tc.marker) {
					t.Fatalf("diagnostic = %s, want %s/%s at %s", diagnostic, tc.class, tc.code, tc.marker)
				}
				if tc.name == "notation as type ref" {
					if !errors.Is(err, errSchemaElementTypeWrongKind) || len(diagnostic.Related()) != 1 || diagnostic.Related()[0] != complexContentTestLoc(t, root, `<xs:notation`) {
						t.Fatalf("wrong-kind cause/related = %v, %v", err, diagnostic.Related())
					}
				}
			})
		}
	}
}

//nolint:gocognit // Each policy checks the full consumer diagnostic and nil output.
func TestSystemOnlyNotationGenerationStillRejectsWithNilOutput(t *testing.T) {
	for _, policy := range notationPolicies() {
		t.Run(policy.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:t"><xs:notation name="n" system="urn:n"/><xs:element name="root" type="xs:integer"/></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			output, err := GenerateGo(schema, "generated")
			if err == nil || output != nil {
				t.Fatalf("GenerateGo = (%d bytes, %v), want nil unsupported", len(output), err)
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != "GOXSD9029" || diagnostic.Loc() != complexContentTestLoc(t, root, `<xs:notation`) {
				t.Fatalf("GenerateGo diagnostic = %s", diagnostic)
			}
			if !errors.Is(err, errCodegenUnsupported) || len(diagnostic.Related()) != 0 {
				t.Fatalf("GenerateGo cause/related = %v, %v", err, diagnostic.Related())
			}
		})
	}
}

func TestSystemOnlyNotationMalformedNameAndDuplicateAttribute(t *testing.T) {
	for _, policy := range notationPolicies() {
		for _, tc := range []struct {
			name, attrs, marker, code string
		}{
			{"malformed name", `name="bad:name" system="urn:n"`, `name="`, invalidSchemaDeclarationNameCode},
			{"missing name", `system="urn:n"`, `<xs:notation`, invalidSchemaDeclarationNameCode},
			{"duplicate system", `name="n" system="urn:first" system="urn:second"`, `system="urn:second"`, InvalidXMLSyntaxCode},
		} {
			t.Run(policy.name+"/"+tc.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:notation ` + tc.attrs + `/></xs:schema>`
				assertSystemOnlyNotationNameOrSyntaxFailure(t, root, policy.policy, tc.code, tc.marker)
			})
		}
	}
}

func assertSystemOnlyNotationNameOrSyntaxFailure(t *testing.T, root string, policy LanguagePolicy, code, marker string) {
	t.Helper()
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
	if err == nil || schema.storage != nil {
		t.Fatal("malformed notation yielded a schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != code || diagnostic.Loc() != complexContentTestLoc(t, root, marker) {
		t.Fatalf("diagnostic = %s, want %s at %s", diagnostic, code, marker)
	}
	if len(diagnostic.Related()) != 0 || diagnostic.SpecRef() != "" {
		t.Fatalf("related/spec = %v, %q, want none", diagnostic.Related(), diagnostic.SpecRef())
	}
	if diagnostic.Unwrap() != nil {
		t.Fatalf("syntax/name cause = %v, want nil", diagnostic.Unwrap())
	}
}

func TestSystemOnlyNotationAnnotationPayloadRemainsOpaque(t *testing.T) {
	for _, policy := range notationPolicies() {
		t.Run(policy.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:notation name="n" system="urn:n"><xs:annotation><xs:documentation><xs:notation name="not-a-declaration"/></xs:documentation></xs:annotation></xs:notation></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if len(schema.Components()) != 1 || schema.Components()[0].Kind() != ComponentKindNotationDeclaration {
				t.Fatalf("components = %#v, want one notation", schema.Components())
			}
		})
	}
}

func TestSystemOnlyNotationStrict10PolicyPrecedesAdmission(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xpathDefaultNamespace="##local"><xs:notation name="n" system="urn:n"/></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	if err == nil || schema.storage != nil {
		t.Fatal("XSD 1.1 root policy yielded a Strict10 schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != complexContentTestLoc(t, root, `xpathDefaultNamespace="`) || !errors.Is(err, errLanguagePolicyMismatch) {
		t.Fatalf("policy diagnostic = %s", diagnostic)
	}
}

func TestSystemOnlyNotationDoesNotHideLaterTypeResolution(t *testing.T) {
	for _, policy := range notationPolicies() {
		t.Run(policy.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:t" targetNamespace="urn:t"><xs:notation name="n" system="urn:n"/><xs:element name="root" type="t:Missing"/></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
			if err == nil || schema.storage != nil {
				t.Fatal("unresolved element type yielded a schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaElementTypeUnresolvedCode || diagnostic.Loc() != complexContentTestLoc(t, root, `type="t:Missing"`) || !errors.Is(err, errSchemaElementTypeUnresolved) {
				t.Fatalf("type-resolution diagnostic = %s", diagnostic)
			}
		})
	}
}

func FuzzNotationIdentifierAdmission(f *testing.F) {
	for _, seed := range []string{
		`system="urn:n"`,
		`public=""`,
		`public="a b" system="urn:n"`,
		`public="bad["`,
		`system="http://[bad"`,
		``,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, attributes string) {
		if len(attributes) > 4096 {
			t.Skip("bounded schema input")
		}
		root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:notation name="n" ` + attributes + `/></xs:schema>`
		for _, policy := range notationPolicies() {
			first, firstErr := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
			second, secondErr := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
			if (firstErr == nil) != (secondErr == nil) {
				t.Fatalf("%s: admission changed between parses: %v, %v", policy.name, firstErr, secondErr)
			}
			if firstErr != nil {
				assertNotationFuzzError(t, first, second, firstErr, secondErr)
				continue
			}
			assertNotationFuzzSuccess(t, first, second)
		}
	})
}

func assertNotationFuzzError(t *testing.T, first, second Schema, firstErr, secondErr error) {
	t.Helper()
	if first.storage != nil || second.storage != nil {
		t.Fatal("error returned a partial schema")
	}
	assertSameSchemaDiagnostic(t, requireDiagnostic(t, firstErr), requireDiagnostic(t, secondErr))
}

func assertNotationFuzzSuccess(t *testing.T, first, second Schema) {
	t.Helper()
	firstComponents := first.Components()
	secondComponents := second.Components()
	if len(firstComponents) != 1 || len(secondComponents) != 1 {
		t.Fatalf("component counts = %d, %d", len(firstComponents), len(secondComponents))
	}
	firstView, firstOK := firstComponents[0].Notation()
	secondView, secondOK := secondComponents[0].Notation()
	if !firstOK || !secondOK || firstView.Public() != secondView.Public() || firstView.PublicLoc() != secondView.PublicLoc() {
		t.Fatal("notation facts changed between parses")
	}
	firstSystem, firstPresent := firstView.System()
	secondSystem, secondPresent := secondView.System()
	if firstSystem != secondSystem || firstPresent != secondPresent {
		t.Fatal("system facts changed between parses")
	}
}
