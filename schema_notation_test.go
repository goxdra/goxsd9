package goxsd9

import (
	"errors"
	"testing"
)

//nolint:gocognit // Keep the complete immutable notation view contract together.
func TestSchemaNotationViewExposesNormalizedIdentifiers(t *testing.T) {
	for _, test := range []struct {
		name   string
		policy LanguagePolicy
	}{
		{name: "XSD 1.0", policy: Strict10},
		{name: "XSD 1.1", policy: Strict11},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test">
  <xs:notation
    name="first"
    public="  public   identifier  "
    system="  urn:system  "/>
  <xs:notation name="second" public="second"/>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, test.policy)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}
			components := schema.Components()
			if got, want := len(components), 2; got != want {
				t.Fatalf("component count = %d, want %d", got, want)
			}

			first := components[0]
			if got, want := first.Kind(), ComponentKindNotationDeclaration; got != want {
				t.Fatalf("first kind = %q, want %q", got, want)
			}
			notation, ok := first.Notation()
			if !ok {
				t.Fatal("first notation view is missing")
			}
			if alias, aliasOK := first.NotationDeclaration(); !aliasOK || alias.Public() != notation.Public() {
				t.Fatal("notation accessor alias does not expose the same declaration")
			}
			if notation.Component().ID() != first.ID() || notation.ID() != first.ID() {
				t.Fatal("notation view does not delegate component identity")
			}
			if notation.Name() != first.Name() || notation.Loc() != first.Loc() {
				t.Fatal("notation view does not delegate component name and location")
			}
			if got, want := notation.Public(), "public identifier"; got != want {
				t.Fatalf("public = %q, want %q", got, want)
			}
			if got, want := notation.PublicLoc(), mustTestLoc(t, "root.xsd", 4, 5); got != want {
				t.Fatalf("public location = %s, want %s", got, want)
			}
			system, systemPresent := notation.System()
			if !systemPresent || system != "urn:system" {
				t.Fatalf("system = (%q, %t), want (urn:system, true)", system, systemPresent)
			}
			if systemLoc, locPresent := notation.SystemLoc(); !locPresent || systemLoc != mustTestLoc(t, "root.xsd", 5, 5) {
				t.Fatalf("system location = (%s, %t), want root.xsd:5:5,true", systemLoc, locPresent)
			}

			second, ok := components[1].Notation()
			if !ok {
				t.Fatal("second notation view is missing")
			}
			if got, want := second.Public(), "second"; got != want {
				t.Fatalf("second public = %q, want %q", got, want)
			}
			if _, present := second.System(); present {
				t.Fatal("public-only notation unexpectedly has a system identifier")
			}
			if _, present := second.SystemLoc(); present {
				t.Fatal("public-only notation unexpectedly has a system location")
			}
			if got, want := components[0].ID().Ordinal(), uint64(1); got != want {
				t.Fatalf("first ordinal = %d, want %d", got, want)
			}
			if got, want := components[1].ID().Ordinal(), uint64(2); got != want {
				t.Fatalf("second ordinal = %d, want %d", got, want)
			}
		})
	}
}

func TestSchemaNotationPreservesPresentEmptySystemIdentifier(t *testing.T) {
	for _, test := range []struct {
		name   string
		policy LanguagePolicy
	}{
		{name: "XSD 1.0", policy: Strict10},
		{name: "XSD 1.1", policy: Strict11},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:notation
    name="item"
    public="public"
    system=""/>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, test.policy)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}
			notation, ok := schema.Components()[0].Notation()
			if !ok {
				t.Fatal("notation view is missing")
			}
			if system, present := notation.System(); !present || system != "" {
				t.Fatalf("system = (%q, %t), want (empty,true)", system, present)
			}
			if systemLoc, present := notation.SystemLoc(); !present || systemLoc != mustTestLoc(t, "root.xsd", 5, 5) {
				t.Fatalf("system location = (%s, %t), want root.xsd:5:5,true", systemLoc, present)
			}
		})
	}
}

//nolint:gocognit // Keep composition and ordered discovery assertions together.
func TestSchemaNotationDeclarationsComposeInDiscoveryOrder(t *testing.T) {
	for _, test := range []struct {
		name   string
		policy LanguagePolicy
	}{
		{name: "XSD 1.0", policy: Strict10},
		{name: "XSD 1.1", policy: Strict11},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test">
  <xs:include schemaLocation="child.xsd"/>
  <xs:notation name="root" public="root-public"/>
</xs:schema>`
			fixtures := map[string]discoveryFixture{
				"child.xsd": {
					id: "child.xsd",
					contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test">
  <xs:notation name="child" public="child-public" system="urn:child"/>
</xs:schema>`,
				},
			}
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, test.policy)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}
			components := schema.Components()
			if got, want := len(components), 2; got != want {
				t.Fatalf("component count = %d, want %d", got, want)
			}
			if components[0].Document() != "root.xsd" || components[1].Document() != "child.xsd" {
				t.Fatalf("component discovery order = %q, %q; want root.xsd, child.xsd", components[0].Document(), components[1].Document())
			}
			rootNotation, ok := components[0].Notation()
			if !ok || rootNotation.Public() != "root-public" {
				t.Fatalf("root notation = (%q, %t), want root-public,true", rootNotation.Public(), ok)
			}
			childNotation, ok := components[1].Notation()
			if !ok || childNotation.Public() != "child-public" {
				t.Fatalf("child notation = (%q, %t), want child-public,true", childNotation.Public(), ok)
			}
			if system, present := childNotation.System(); !present || system != "urn:child" {
				t.Fatalf("child system = (%q, %t), want urn:child,true", system, present)
			}
			name := mustTestQName(t, "urn:test", "child")
			found := schema.FindKind(ComponentKindNotationDeclaration, name)
			if len(found) != 1 || found[0].ID() != components[1].ID() {
				t.Fatalf("FindKind(child) = %#v, want child component", found)
			}
		})
	}
}

//nolint:gocognit // Keep every invalid notation diagnostic dimension together.
func TestSchemaNotationRejectsInvalidIdentifiers(t *testing.T) {
	tests := []struct {
		name  string
		root  string
		loc   Loc
		cause error
		extra func(t *testing.T, diagnostic Diagnostic)
	}{
		{
			name: "missing public",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:notation name="item"/>
</xs:schema>`,
			loc:   mustTestLoc(t, "root.xsd", 2, 3),
			cause: errSchemaNotationPublic,
		},
		{
			name: "empty public",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:notation
    name="item"
    public="  "/>
</xs:schema>`,
			loc:   mustTestLoc(t, "root.xsd", 4, 5),
			cause: errSchemaNotationPublic,
		},
		{
			name: "malformed system",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:notation
    name="item"
    public="public"
    system="http://[bad"/>
</xs:schema>`,
			loc:   mustTestLoc(t, "root.xsd", 5, 5),
			cause: errSchemaNotationSystem,
			extra: func(t *testing.T, diagnostic Diagnostic) {
				var uriDiagnostic Diagnostic
				if !errors.As(diagnostic.Unwrap(), &uriDiagnostic) {
					t.Fatal("notation diagnostic did not preserve the anyURI diagnostic")
				}
				if uriDiagnostic.Code() != invalidSchemaCompositionCode || uriDiagnostic.Loc() != diagnostic.Loc() {
					t.Fatalf("preserved anyURI diagnostic = %s, want composition at %s", uriDiagnostic, diagnostic.Loc())
				}
			},
		},
	}
	for _, policy := range []struct {
		name    string
		value   LanguagePolicy
		specRef string
	}{
		{name: "XSD 1.0", value: Strict10, specRef: schemaNotationXSD10SpecRef},
		{name: "XSD 1.1", value: Strict11, specRef: schemaNotationXSD11SpecRef},
	} {
		for _, test := range tests {
			t.Run(policy.name+"/"+test.name, func(t *testing.T) {
				var first Diagnostic
				for iteration := 0; iteration < 3; iteration++ {
					schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, policy.value)
					if err == nil || schema.storage != nil || len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
						t.Fatal("invalid notation was accepted or returned a partial schema")
					}
					diagnostic := requireDiagnostic(t, err)
					if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaNotationCode {
						t.Fatalf("diagnostic = %s, want invalid notation diagnostic", diagnostic)
					}
					if diagnostic.Loc() != test.loc {
						t.Fatalf("diagnostic location = %s, want %s", diagnostic.Loc(), test.loc)
					}
					if diagnostic.SpecRef() != policy.specRef {
						t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), policy.specRef)
					}
					if !errors.Is(err, test.cause) {
						t.Fatalf("diagnostic does not preserve cause %v: %v", test.cause, err)
					}
					if test.extra != nil {
						test.extra(t, diagnostic)
					}
					if iteration == 0 {
						first = diagnostic
						continue
					}
					assertSameSchemaDiagnostic(t, first, diagnostic)
				}
			})
		}
	}
}

//nolint:gocognit // Keep both common-identifier diagnostic dimensions together.
func TestSchemaNotationValidatesCommonIdentifiers(t *testing.T) {
	tests := []struct {
		name string
		root string
		code string
		loc  Loc
	}{
		{
			name: "malformed name",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:notation
    name="bad:name"
    public="public"/>
</xs:schema>`,
			code: invalidSchemaDeclarationNameCode,
			loc:  mustTestLoc(t, "root.xsd", 3, 5),
		},
		{
			name: "malformed id",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:notation
    name="item"
    public="public"
    id="bad:id"/>
</xs:schema>`,
			code: invalidSchemaCompositionCode,
			loc:  mustTestLoc(t, "root.xsd", 5, 5),
		},
	}
	for _, policy := range []LanguagePolicy{Strict10, Strict11} {
		for _, test := range tests {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, policy)
				if err == nil || schema.storage != nil || len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
					t.Fatal("invalid notation was accepted or returned a partial schema")
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code {
					t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, test.code)
				}
				if diagnostic.Loc() != test.loc {
					t.Fatalf("diagnostic location = %s, want %s", diagnostic.Loc(), test.loc)
				}
				if len(diagnostic.Related()) != 0 {
					t.Fatalf("diagnostic related locations = %v, want none", diagnostic.Related())
				}
				if cause := diagnostic.Unwrap(); cause != nil {
					t.Fatalf("diagnostic cause = %v, want nil", cause)
				}
			})
		}
	}
}

func TestSchemaNotationDuplicateAttributeUsesXMLSyntaxDiagnostic(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:notation name="item" public="first"
    public="later"/>
</xs:schema>`
	for _, policy := range []LanguagePolicy{Strict10, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err == nil || schema.storage != nil {
				t.Fatal("duplicate notation attribute was accepted or returned a partial schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != InvalidXMLSyntaxCode {
				t.Fatalf("diagnostic = %s, want XML syntax diagnostic", diagnostic)
			}
			if got, want := diagnostic.Loc(), mustTestLoc(t, "root.xsd", 3, 5); got != want {
				t.Fatalf("diagnostic location = %s, want %s", got, want)
			}
		})
	}
}

func TestSchemaNotationInputAndQueriesAreImmutable(t *testing.T) {
	name := mustTestQName(t, "urn:test", "item")
	componentLoc := mustTestLoc(t, "input.xsd", 2, 3)
	publicLoc := mustTestLoc(t, "input.xsd", 2, 30)
	systemLoc := mustTestLoc(t, "input.xsd", 2, 50)
	inputNotation := &schemaNotationInput{
		public:    "before",
		publicLoc: publicLoc,
		system:    "urn:before",
		systemLoc: systemLoc,
		hasSystem: true,
	}
	schema, err := newSchema([]schemaDocumentInput{{
		source:  "input.xsd",
		rootLoc: mustTestLoc(t, "input.xsd", 1, 1),
		declarations: []schemaComponentInput{{
			kind:     ComponentKindNotationDeclaration,
			name:     name,
			loc:      componentLoc,
			notation: inputNotation,
		}},
	}})
	if err != nil {
		t.Fatalf("newSchema: %v", err)
	}
	inputNotation.public = "after"
	inputNotation.publicLoc = Loc{}
	inputNotation.system = "urn:after"
	inputNotation.systemLoc = Loc{}
	inputNotation.hasSystem = false

	component := schema.Components()[0]
	notation, ok := component.Notation()
	if !ok {
		t.Fatal("notation view is missing")
	}
	if notation.Public() != "before" || notation.PublicLoc() != publicLoc {
		t.Fatalf("input mutation changed public facts: %q at %s", notation.Public(), notation.PublicLoc())
	}
	if system, present := notation.System(); !present || system != "urn:before" {
		t.Fatalf("input mutation changed system facts: %q, %t", system, present)
	}
	if systemLocation, present := notation.SystemLoc(); !present || systemLocation != systemLoc {
		t.Fatalf("input mutation changed system location: %s, %t", systemLocation, present)
	}

	components := schema.Components()
	components[0] = Component{}
	if found := schema.FindKind(ComponentKindNotationDeclaration, name); len(found) != 1 {
		t.Fatal("component query slice mutation changed schema contents")
	}
	found := schema.Find(name)
	found[0] = Component{}
	queried, queriedOK := schema.Lookup(component.ID())
	if !queriedOK {
		t.Fatal("Lookup lost notation after query mutation")
	}
	if _, notationOK := queried.Notation(); !notationOK {
		t.Fatal("Lookup returned a notation without its immutable facts")
	}
}
