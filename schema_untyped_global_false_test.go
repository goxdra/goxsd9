package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

type untypedGlobalComponentFact struct {
	id       ComponentID
	name     QName
	kind     ComponentKind
	loc      Loc
	document SourceID
	hasView  bool
}

type untypedGlobalDocumentFact struct {
	source     SourceID
	root       Loc
	namespace  string
	components []ComponentID
}

type untypedGlobalSnapshot struct {
	policy     LanguagePolicy
	components []untypedGlobalComponentFact
	documents  []untypedGlobalDocumentFact
	walk       []ComponentID
}

func untypedGlobalFacts(t *testing.T, schema Schema) untypedGlobalSnapshot {
	t.Helper()
	result := untypedGlobalSnapshot{policy: schema.LanguagePolicy()}
	for _, component := range schema.Components() {
		_, hasView := component.ElementDeclaration()
		result.components = append(result.components, untypedGlobalComponentFact{
			id: component.ID(), name: component.Name(), kind: component.Kind(),
			loc: component.Loc(), document: component.Document(), hasView: hasView,
		})
		found := schema.FindKind(component.Kind(), component.Name())
		if len(found) != 1 || found[0].ID() != component.ID() {
			t.Fatalf("FindKind(%q) = %v, want component %v", component.Name(), found, component.ID())
		}
		lookedUp, ok := schema.Lookup(component.ID())
		if !ok || lookedUp.ID() != component.ID() {
			t.Fatalf("Lookup(%v) = %v/%t", component.ID(), lookedUp, ok)
		}
	}
	for _, document := range schema.Documents() {
		fact := untypedGlobalDocumentFact{source: document.Source(), root: document.RootLoc(), namespace: document.TargetNamespace()}
		for _, component := range document.Components() {
			fact.components = append(fact.components, component.ID())
		}
		result.documents = append(result.documents, fact)
	}
	if err := schema.Walk(func(component Component) error {
		result.walk = append(result.walk, component.ID())
		return nil
	}); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if len(result.walk) != len(result.components) {
		t.Fatalf("walk length = %d, components = %d", len(result.walk), len(result.components))
	}
	for index, component := range result.components {
		if result.walk[index] != component.id {
			t.Fatalf("walk[%d] = %v, component = %v", index, result.walk[index], component.id)
		}
	}
	return result
}

func untypedGlobalRoot(attributes string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" version="1.1">
  <xs:element name="before" type="xs:string"/>
  <xs:element name="item"` + attributes + `/>
  <xs:element name="after" type="xs:integer"/>
</xs:schema>`
}

func untypedGlobalPolicies() []struct {
	name   string
	policy LanguagePolicy
	ref    string
} {
	return []struct {
		name   string
		policy LanguagePolicy
		ref    string
	}{
		{"compatibility", Compatibility, "xsd11-structures#element-element"},
		{"strict10", Strict10, "xsd10-structures#element-element"},
		{"strict11", Strict11, "xsd11-structures#element-element"},
	}
}

func untypedGlobalConsumerFacts(t *testing.T, schema Schema) [2]Diagnostic {
	t.Helper()
	validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<item xmlns="urn:root"/>`)))
	validation := requireDiagnostic(t, validationErr)
	if validation.Class() != FailureUnsupported || validation.Code() != UnsupportedInstanceValidationCode ||
		validation.Loc() != mustTestLoc(t, "instance.xml", 1, 1) || !errors.Is(validationErr, errInstanceNoDeclaredType) {
		t.Fatalf("untyped instance diagnostic = %s", validation)
	}
	item := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:root", "item"))
	if len(item) != 1 || !reflect.DeepEqual(validation.Related(), []Loc{item[0].Loc()}) {
		t.Fatalf("untyped instance related = %v, want item location", validation.Related())
	}
	output, generationErr := GenerateGo(schema, "generated")
	generation := requireDiagnostic(t, generationErr)
	if output != nil || generation.Class() != FailureUnsupported || generation.Code() != diagnosticCodegenUnsupported ||
		generation.Loc() != item[0].Loc() || !errors.Is(generationErr, errCodegenUnsupported) {
		t.Fatalf("untyped generation = %q/%s", output, generation)
	}
	return [2]Diagnostic{validation, generation}
}

//nolint:gocognit // Every admitted lexical form is compared against the same public omission facts and both consumers.
func TestUntypedGlobalFalseFlagsMatchOmission(t *testing.T) {
	forms := []struct{ name, attributes string }{
		{"abstract false", ` abstract="false"`},
		{"abstract zero", ` abstract="0"`},
		{"abstract padded false", ` abstract="&#x9;false&#xA;"`},
		{"abstract padded zero", ` abstract="&#xD;0&#x9;"`},
		{"nillable false", ` nillable="false"`},
		{"nillable zero", ` nillable="0"`},
		{"nillable padded false", ` nillable="&#x9;false&#xA;"`},
		{"nillable padded zero", ` nillable="&#xD;0&#x9;"`},
		{"both", ` abstract="false" nillable="0"`},
		{"both padded", ` nillable="&#xA;false&#x9;" abstract="&#xD;0&#xA;"`},
	}
	for _, policy := range untypedGlobalPolicies() {
		t.Run(policy.name, func(t *testing.T) {
			omitted, err := discoverTestSchemaWithPolicy(t, untypedGlobalRoot(""), nil, policy.policy)
			if err != nil {
				t.Fatalf("omitted parse: %v", err)
			}
			wantFacts := untypedGlobalFacts(t, omitted)
			wantConsumers := untypedGlobalConsumerFacts(t, omitted)
			for _, form := range forms {
				t.Run(form.name, func(t *testing.T) {
					root := untypedGlobalRoot(form.attributes)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
					if err != nil {
						t.Fatalf("parse explicit false: %v", err)
					}
					got := untypedGlobalFacts(t, schema)
					if !reflect.DeepEqual(got, wantFacts) {
						t.Fatalf("explicit flags changed public facts: got %#v, omission %#v", got, wantFacts)
					}
					if got.components[1].hasView || got.components[1].kind != ComponentKindElementDeclaration {
						t.Fatalf("untyped component has unexpected view/kind: %#v", got.components[1])
					}
					components := schema.Components()
					components[1] = Component{}
					find := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:root", "item"))
					find[0] = Component{}
					documents := schema.Documents()
					documents[0].Components()[1] = Component{}
					if after := untypedGlobalFacts(t, schema); !reflect.DeepEqual(after, got) {
						t.Fatal("mutating returned slices changed completed schema")
					}
					consumers := untypedGlobalConsumerFacts(t, schema)
					for index := range consumers {
						if consumers[index].Class() != wantConsumers[index].Class() ||
							consumers[index].Code() != wantConsumers[index].Code() ||
							consumers[index].Loc() != wantConsumers[index].Loc() ||
							consumers[index].SpecRef() != wantConsumers[index].SpecRef() ||
							!reflect.DeepEqual(consumers[index].Related(), wantConsumers[index].Related()) {
							t.Fatalf("consumer %d changed: got %s, omission %s", index, consumers[index], wantConsumers[index])
						}
					}
				})
			}
		})
	}
}

//nolint:gocognit // These lexical exits share one diagnostic boundary and must retain their precedence.
func TestUntypedGlobalBooleanDiagnosticExits(t *testing.T) {
	cases := []struct {
		name, attributes, marker string
		class                    FailureClass
	}{
		{"abstract true", ` abstract="true"`, `abstract="true"`, FailureUnsupported},
		{"abstract one", ` abstract="1"`, `abstract="1"`, FailureUnsupported},
		{"nillable true", ` nillable="true"`, `nillable="true"`, FailureUnsupported},
		{"nillable one", ` nillable="1"`, `nillable="1"`, FailureUnsupported},
		{"both true precedence", ` abstract="true" nillable="1"`, `abstract="true"`, FailureUnsupported},
		{"both reverse precedence", ` nillable="1" abstract="true"`, `nillable="1"`, FailureUnsupported},
		{"invalid abstract", ` abstract="maybe"`, `abstract="maybe"`, FailureInvalid},
		{"invalid nillable", ` nillable="00"`, `nillable="00"`, FailureInvalid},
		{"invalid empty", ` abstract=""`, `abstract=""`, FailureInvalid},
		{"invalid XML whitespace", ` nillable="false true"`, `nillable="false true"`, FailureInvalid},
		{"invalid beats true", ` abstract="true" nillable="maybe"`, `nillable="maybe"`, FailureInvalid},
	}
	for _, policy := range untypedGlobalPolicies() {
		for _, test := range cases {
			t.Run(policy.name+"/"+test.name, func(t *testing.T) {
				root := untypedGlobalRoot(test.attributes)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
				if err == nil || len(schema.Components()) != 0 || len(schema.Documents()) != 0 {
					t.Fatalf("excluded flags returned schema: %v/%v", schema.Components(), err)
				}
				d := requireDiagnostic(t, err)
				wantLoc := schemaMixedComplexLoc(t, root, test.marker)
				if d.Class() != test.class || d.Loc() != wantLoc || len(d.Related()) != 0 {
					t.Fatalf("diagnostic = %s, want %s at %s", d, test.class, wantLoc)
				}
				if test.class == FailureUnsupported {
					version := XSDVersion11
					if policy.policy == Strict10 {
						version = XSDVersion10
					}
					wantRef := newSchemaSyntaxUnsupportedForVersion(Loc{}, "", version).SpecRef()
					if d.Code() != UnsupportedSchemaSyntaxCode || d.SpecRef() != wantRef || d.Unwrap() != nil || !errors.Is(err, ErrUnsupported) {
						t.Fatalf("unsupported diagnostic = %s, want code, edition ref, and unsupported classification", d)
					}
					return
				}
				if d.Code() != invalidSchemaCompositionCode || d.SpecRef() != "" || errors.Is(err, ErrUnsupported) ||
					!strings.Contains(d.Message(), "invalid boolean value") {
					t.Fatalf("malformed Boolean diagnostic = %s", d)
				}
				lexical := requireDiagnostic(t, d.Unwrap())
				if lexical.Class() != FailureInvalid || lexical.Code() != invalidSchemaCompositionCode || lexical.Loc() != wantLoc ||
					lexical.Unwrap() != nil || !strings.Contains(lexical.Message(), "invalid boolean value") {
					t.Fatalf("malformed Boolean lexical cause = %s", lexical)
				}
			})
		}
	}
}

func untypedGlobalGraph(flags string) (string, map[string]discoveryFixture) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="1.0">
  <xs:include schemaLocation="included.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:element name="item"` + flags + `/>
  <xs:element name="forward" type="t:Later"/>
  <xs:simpleType name="Later"><xs:restriction base="xs:integer"/></xs:simpleType>
</xs:schema>`
	included := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root">
  <xs:include schemaLocation="root.xsd"/>
  <xs:element name="included"` + flags + `/>
</xs:schema>`
	chameleon := `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:element name="chameleon"` + flags + `/>
</xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other">
  <xs:element name="imported"` + flags + `/>
</xs:schema>`
	return root, map[string]discoveryFixture{
		"root.xsd":      {id: "root.xsd", contents: root},
		"included.xsd":  {id: "included.xsd", contents: included},
		"chameleon.xsd": {id: "chameleon.xsd", contents: chameleon},
		"other.xsd":     {id: "other.xsd", contents: other},
	}
}

//nolint:gocognit // Graph discovery, visibility, cycles, and copied public facts share one omission comparison.
func TestUntypedGlobalFalseFlagsKeepGraphFacts(t *testing.T) {
	for _, policy := range untypedGlobalPolicies() {
		t.Run(policy.name, func(t *testing.T) {
			root, fixtures := untypedGlobalGraph("")
			omitted, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy.policy)
			if err != nil {
				t.Fatalf("omitted graph: %v", err)
			}
			want := untypedGlobalFacts(t, omitted)
			wantConsumers := untypedGlobalConsumerFacts(t, omitted)
			wantSources := []SourceID{"root.xsd", "included.xsd", "chameleon.xsd", "other.xsd"}
			if len(want.documents) != len(wantSources) {
				t.Fatalf("document count = %d, want %d", len(want.documents), len(wantSources))
			}
			for index, source := range wantSources {
				if want.documents[index].source != source {
					t.Fatalf("document %d = %s, want %s", index, want.documents[index].source, source)
				}
			}
			if want.documents[2].namespace != "urn:root" || want.documents[3].namespace != "urn:other" {
				t.Fatalf("chameleon/import namespaces = %q/%q", want.documents[2].namespace, want.documents[3].namespace)
			}
			for _, flags := range []string{` abstract="false"`, ` nillable="0"`, ` abstract="&#x9;false&#xA;" nillable="&#xD;0&#x9;"`} {
				root, fixtures := untypedGlobalGraph(flags)
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy.policy)
				if err != nil {
					t.Fatalf("flagged graph %q: %v", flags, err)
				}
				got := untypedGlobalFacts(t, schema)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("flags %q changed graph facts: got %#v, want %#v", flags, got, want)
				}
				for _, name := range []struct{ namespace, local string }{
					{"urn:root", "item"}, {"urn:root", "included"},
					{"urn:root", "chameleon"}, {"urn:other", "imported"},
				} {
					matches := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, name.namespace, name.local))
					if len(matches) != 1 {
						t.Fatalf("FindKind(%s:%s) = %d, want 1", name.namespace, name.local, len(matches))
					}
					if _, ok := matches[0].ElementDeclaration(); ok {
						t.Fatalf("untyped graph element %s gained a declaration view", name.local)
					}
				}
				consumers := untypedGlobalConsumerFacts(t, schema)
				for index := range consumers {
					if consumers[index].Code() != wantConsumers[index].Code() ||
						consumers[index].Loc() != wantConsumers[index].Loc() ||
						consumers[index].SpecRef() != wantConsumers[index].SpecRef() ||
						!reflect.DeepEqual(consumers[index].Related(), wantConsumers[index].Related()) {
						t.Fatalf("graph consumer %d changed: got %s, omission %s", index, consumers[index], wantConsumers[index])
					}
				}
			}
		})
	}
}

//nolint:gocognit // Each excluded shape must fail before publishing any schema.
func TestUntypedGlobalFalseFlagsKeepShapeExclusions(t *testing.T) {
	cases := []struct {
		name, root, marker string
		class              FailureClass
		code               string
	}{
		{
			name: "substitution group",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root">
  <xs:element name="head" type="xs:string"/>
  <xs:element name="item" abstract="false" nillable="0" substitutionGroup="t:head"/>
</xs:schema>`,
			marker: `substitutionGroup="t:head"`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode,
		},
		{
			name: "block",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:element name="item" abstract="false" nillable="0" block="extension"/>
</xs:schema>`,
			marker: `block="extension"`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode,
		},
		{
			name: "child alternative",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:element name="item" abstract="false" nillable="0"><xs:alternative type="xs:string"/></xs:element>
</xs:schema>`,
			marker: `<xs:alternative`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode,
		},
		{
			name: "local without type",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root">
  <xs:complexType name="Container"><xs:sequence><xs:element name="local" nillable="false"/></xs:sequence></xs:complexType>
</xs:schema>`,
			marker: `<xs:element name="local"`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode,
		},
		{
			name: "local ref flag",
			root: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root">
  <xs:element name="item" abstract="false" nillable="0"/>
  <xs:complexType name="Container"><xs:sequence><xs:element ref="t:item" nillable="false"/></xs:sequence></xs:complexType>
</xs:schema>`,
			marker: `nillable="false"/></xs:sequence>`, class: FailureInvalid, code: invalidSchemaCompositionCode,
		},
	}
	for _, policy := range untypedGlobalPolicies() {
		for _, test := range cases {
			t.Run(policy.name+"/"+test.name, func(t *testing.T) {
				schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, policy.policy)
				if err == nil || len(schema.Components()) != 0 || len(schema.Documents()) != 0 {
					t.Fatalf("excluded shape returned schema: %v/%v", schema.Components(), err)
				}
				d := requireDiagnostic(t, err)
				wantLoc := schemaMixedComplexLoc(t, test.root, test.marker)
				if d.Class() != test.class || d.Code() != test.code || d.Loc() != wantLoc || len(d.Related()) != 0 {
					t.Fatalf("diagnostic = %s, want %s/%s at %s with no related locations", d, test.class, test.code, wantLoc)
				}
				if test.class == FailureUnsupported && !errors.Is(err, ErrUnsupported) {
					t.Fatalf("unsupported shape lost classification: %v", err)
				}
			})
		}
	}
}

//nolint:gocognit // Alternate exits at the same declaration boundary retain their first diagnostic.
func TestUntypedGlobalBooleanPrecedenceWithExcludedShapes(t *testing.T) {
	cases := []struct {
		name, element, marker string
		class                 FailureClass
	}{
		{"true before child", `<xs:element name="item" abstract="true"><xs:alternative type="xs:string"/></xs:element>`, `abstract="true"`, FailureUnsupported},
		{"malformed before child", `<xs:element name="item" abstract="maybe"><xs:alternative type="xs:string"/></xs:element>`, `abstract="maybe"`, FailureInvalid},
		{"true before substitution", `<xs:element name="item" abstract="true" substitutionGroup="t:before"/>`, `abstract="true"`, FailureUnsupported},
		{"malformed before substitution", `<xs:element name="item" nillable="maybe" substitutionGroup="t:before"/>`, `nillable="maybe"`, FailureInvalid},
	}
	for _, policy := range untypedGlobalPolicies() {
		for _, test := range cases {
			t.Run(policy.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root">
  <xs:element name="before" type="xs:string"/>
  ` + test.element + `
</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
				if err == nil || len(schema.Components()) != 0 || len(schema.Documents()) != 0 {
					t.Fatalf("excluded precedence returned schema: %v/%v", schema.Components(), err)
				}
				d := requireDiagnostic(t, err)
				wantLoc := schemaMixedComplexLoc(t, root, test.marker)
				if policy.policy == Strict10 && test.name == "true before child" {
					wantLoc = schemaMixedComplexLoc(t, root, `<xs:alternative`)
					wantSpec := newSchemaSyntaxUnsupportedForVersion(Loc{}, "", XSDVersion11).SpecRef()
					if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.SpecRef() != wantSpec ||
						d.Loc() != wantLoc || len(d.Related()) != 0 || !errors.Is(err, errLanguagePolicyMismatch) {
						t.Fatalf("strict10 alternative mismatch = %s, want XSD 1.1 at %s", d, wantLoc)
					}
					return
				}
				if d.Class() != test.class || d.Loc() != wantLoc || len(d.Related()) != 0 {
					t.Fatalf("precedence diagnostic = %s, want %s at %s", d, test.class, wantLoc)
				}
				if test.class == FailureUnsupported {
					version := XSDVersion11
					if policy.policy == Strict10 {
						version = XSDVersion10
					}
					if d.Code() != UnsupportedSchemaSyntaxCode || d.SpecRef() != newSchemaSyntaxUnsupportedForVersion(Loc{}, "", version).SpecRef() ||
						!errors.Is(err, ErrUnsupported) {
						t.Fatalf("precedence unsupported diagnostic = %s", d)
					}
					return
				}
				if d.Code() != invalidSchemaCompositionCode || d.SpecRef() != "" || !strings.Contains(d.Message(), "invalid boolean value") {
					t.Fatalf("precedence invalid diagnostic = %s", d)
				}
				lexical := requireDiagnostic(t, d.Unwrap())
				if lexical.Code() != invalidSchemaCompositionCode || lexical.Loc() != wantLoc || lexical.Unwrap() != nil {
					t.Fatalf("precedence lexical cause = %s", lexical)
				}
			})
		}
	}
}
