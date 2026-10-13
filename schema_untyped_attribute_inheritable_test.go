package goxsd9

import (
	"errors"
	"reflect"
	"testing"
)

type untypedAttributeComponentFact struct {
	id       ComponentID
	name     QName
	kind     ComponentKind
	loc      Loc
	document SourceID
	view     bool
}

type untypedAttributeDocumentFact struct {
	source     SourceID
	root       Loc
	namespace  string
	components []ComponentID
}

type untypedAttributeSnapshot struct {
	policy     LanguagePolicy
	components []untypedAttributeComponentFact
	documents  []untypedAttributeDocumentFact
	walk       []ComponentID
}

func snapshotUntypedAttributes(t *testing.T, schema Schema) untypedAttributeSnapshot {
	t.Helper()
	result := untypedAttributeSnapshot{policy: schema.LanguagePolicy()}
	for _, component := range schema.Components() {
		_, hasView := component.AttributeDeclaration()
		result.components = append(result.components, untypedAttributeComponentFact{
			id: component.ID(), name: component.Name(), kind: component.Kind(), loc: component.Loc(),
			document: component.Document(), view: hasView,
		})
		found := schema.FindKind(component.Kind(), component.Name())
		if len(found) != 1 || found[0].ID() != component.ID() {
			t.Fatalf("FindKind(%q) = %v, want %v", component.Name(), found, component.ID())
		}
		lookedUp, ok := schema.Lookup(component.ID())
		if !ok || lookedUp.ID() != component.ID() {
			t.Fatalf("Lookup(%v) = %v/%t", component.ID(), lookedUp, ok)
		}
	}
	for _, document := range schema.Documents() {
		fact := untypedAttributeDocumentFact{source: document.Source(), root: document.RootLoc(), namespace: document.TargetNamespace()}
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
	for index, component := range result.components {
		if result.walk[index] != component.id {
			t.Fatalf("walk[%d] = %v, component = %v", index, result.walk[index], component.id)
		}
	}
	return result
}

func untypedAttributeRoot(inheritable string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root">
  <xs:attribute name="before" type="xs:integer"/>
  <xs:attribute name="item"` + inheritable + `/>
  <xs:attribute name="after" type="xs:boolean"/>
</xs:schema>`
}

//nolint:gocognit // Compare each admitted spelling across the same public query surface.
func TestUntypedGlobalAttributeFalseInheritableMatchesOmission(t *testing.T) {
	forms := []struct{ name, text string }{
		{"false", ` inheritable="false"`},
		{"zero", ` inheritable="0"`},
		{"padded false", ` inheritable="&#x9;false&#xA;"`},
		{"padded zero", ` inheritable="&#xD;0&#x9;"`},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			omitted, err := discoverTestSchemaWithPolicy(t, untypedAttributeRoot(""), nil, policy)
			if err != nil {
				t.Fatalf("omitted parse: %v", err)
			}
			want := snapshotUntypedAttributes(t, omitted)
			if len(want.components) != 3 || want.components[1].view || want.components[1].kind != ComponentKindAttributeDeclaration {
				t.Fatalf("omitted untyped attribute facts = %#v", want.components)
			}
			for _, form := range forms {
				t.Run(form.name, func(t *testing.T) {
					root := untypedAttributeRoot(form.text)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
					if err != nil {
						t.Fatalf("explicit false parse: %v", err)
					}
					got := snapshotUntypedAttributes(t, schema)
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("explicit inheritable changed public facts: got %#v, omission %#v", got, want)
					}
					item := schema.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:root", "item"))[0]
					if item.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="item"`) {
						t.Fatalf("item location = %s", item.Loc())
					}
					if _, ok := item.Attribute(); ok {
						t.Fatal("untyped attribute acquired an Attribute view")
					}
					components := schema.Components()
					components[1] = Component{}
					found := schema.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:root", "item"))
					found[0] = Component{}
					parts := schema.Documents()[0].Components()
					parts[1] = Component{}
					if after := snapshotUntypedAttributes(t, schema); !reflect.DeepEqual(after, got) {
						t.Fatal("mutating returned copies changed immutable facts")
					}
				})
			}
		})
	}
}

func untypedAttributeGraph(inheritable string) (string, map[string]discoveryFixture) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root">
  <xs:include schemaLocation="included.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:include schemaLocation="included.xsd"/>
  <xs:attribute name="rootBefore" type="xs:integer"/>
  <xs:attribute name="rootItem"` + inheritable + `/>
  <xs:attribute name="rootAfter" type="xs:decimal"/>
</xs:schema>`
	included := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root">
  <xs:include schemaLocation="root.xsd"/>
  <xs:attribute name="included"` + inheritable + `/>
</xs:schema>`
	chameleon := `<xs:schema xmlns:xs="` + testXSDNamespace + `">
  <xs:attribute name="chameleon"` + inheritable + `/>
</xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other">
  <xs:attribute name="imported"` + inheritable + `/>
</xs:schema>`
	return root, map[string]discoveryFixture{
		"root.xsd":      {id: "root.xsd", contents: root},
		"included.xsd":  {id: "included.xsd", contents: included},
		"chameleon.xsd": {id: "chameleon.xsd", contents: chameleon},
		"other.xsd":     {id: "other.xsd", contents: other},
	}
}

//nolint:gocognit // Exercise graph order, adoption, and cycle ownership with one snapshot.
func TestUntypedGlobalAttributeInheritableKeepsGraphOwnershipAndOrder(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			root, fixtures := untypedAttributeGraph("")
			omitted, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy)
			if err != nil {
				t.Fatalf("omitted graph: %v", err)
			}
			want := snapshotUntypedAttributes(t, omitted)
			wantSources := []SourceID{"root.xsd", "included.xsd", "chameleon.xsd", "other.xsd"}
			if len(want.documents) != len(wantSources) {
				t.Fatalf("documents = %#v", want.documents)
			}
			for index, source := range wantSources {
				if want.documents[index].source != source {
					t.Fatalf("document %d source = %s, want %s", index, want.documents[index].source, source)
				}
			}
			if want.documents[2].namespace != "urn:root" || want.documents[3].namespace != "urn:other" {
				t.Fatalf("adopted/imported namespaces = %q/%q", want.documents[2].namespace, want.documents[3].namespace)
			}
			for index, expected := range []struct {
				name, namespace string
				source          SourceID
				ordinal         uint64
			}{
				{"rootBefore", "urn:root", "root.xsd", 1},
				{"rootItem", "urn:root", "root.xsd", 2},
				{"rootAfter", "urn:root", "root.xsd", 3},
				{"included", "urn:root", "included.xsd", 1},
				{"chameleon", "urn:root", "chameleon.xsd", 1},
				{"imported", "urn:other", "other.xsd", 1},
			} {
				fact := want.components[index]
				if fact.name != mustTestQName(t, expected.namespace, expected.name) ||
					fact.id.Source() != expected.source || fact.id.Ordinal() != expected.ordinal ||
					fact.document != expected.source || fact.loc.Source() != expected.source ||
					(fact.view != (index == 0 || index == 2)) {
					t.Fatalf("component %d graph facts = %#v", index, fact)
				}
			}
			for _, form := range []string{` inheritable="false"`, ` inheritable="0"`, ` inheritable="&#x9;false&#xA;"`} {
				root, fixtures := untypedAttributeGraph(form)
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy)
				if err != nil {
					t.Fatalf("graph form %q: %v", form, err)
				}
				got := snapshotUntypedAttributes(t, schema)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("graph form %q changed public facts: got %#v, omission %#v", form, got, want)
				}
				for _, target := range []struct{ namespace, local string }{
					{"urn:root", "rootItem"}, {"urn:root", "included"},
					{"urn:root", "chameleon"}, {"urn:other", "imported"},
				} {
					matches := schema.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, target.namespace, target.local))
					if len(matches) != 1 || matches[0].Kind() != ComponentKindAttributeDeclaration {
						t.Fatalf("FindKind(%s:%s) = %#v", target.namespace, target.local, matches)
					}
					if _, ok := matches[0].AttributeDeclaration(); ok {
						t.Fatalf("untyped %s gained a declaration view", target.local)
					}
				}
			}
		})
	}
}

//nolint:gocognit // Pin every lexical and policy exit at this declaration boundary.
func TestUntypedGlobalAttributeInheritableAlternateExits(t *testing.T) {
	for _, policy := range []struct {
		name    string
		policy  LanguagePolicy
		version XSDVersion
	}{
		{"Compatibility", Compatibility, XSDVersion11},
		{"Strict10", Strict10, XSDVersion10},
		{"Strict11", Strict11, XSDVersion11},
	} {
		for _, form := range []struct{ name, value string }{
			{"false", "false"}, {"zero", "0"}, {"true", "true"}, {"one", "1"},
			{"padded false", "&#x9;false&#xA;"}, {"padded zero", "&#xD;0&#x9;"},
			{"invalid", "maybe"}, {"empty", ""}, {"internal whitespace", "false true"},
			{"unicode whitespace", "&#xA0;false"},
		} {
			if policy.policy != Strict10 && (form.name == "false" || form.name == "zero" ||
				form.name == "padded false" || form.name == "padded zero") {
				continue
			}
			t.Run(policy.name+"/"+form.name, func(t *testing.T) {
				marker := `inheritable="` + form.value + `"`
				root := untypedAttributeRoot(" " + marker)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
				if err == nil || schema.storage != nil || len(schema.Components()) != 0 || len(schema.Documents()) != 0 {
					t.Fatalf("alternate exit returned schema: %#v/%v", schema, err)
				}
				d := requireDiagnostic(t, err)
				loc := elementReferenceTestAttributeLoc(t, root, marker)
				if d.Loc() != loc || len(d.Related()) != 0 {
					t.Fatalf("diagnostic location = %s related %v, want %s", d.Loc(), d.Related(), loc)
				}
				malformed := form.name == "invalid" || form.name == "empty" || form.name == "internal whitespace" || form.name == "unicode whitespace"
				if malformed {
					if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.SpecRef() != strictBooleanSpecRef(policy.version) || errors.Is(err, ErrUnsupported) {
						t.Fatalf("malformed inheritable diagnostic = %s", d)
					}
					cause := requireDiagnostic(t, d.Unwrap())
					if cause.Code() != InvalidBooleanLexicalCode || cause.Loc() != loc || cause.SpecRef() != strictBooleanSpecRef(policy.version) {
						t.Fatalf("malformed lexical cause = %s", cause)
					}
					return
				}
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("unsupported inheritable diagnostic = %s", d)
				}
				if policy.policy == Strict10 {
					if !errors.Is(err, errLanguagePolicyMismatch) || d.SpecRef() != newXSD11FeatureMismatch(FeatureSchemaSyntax, UnsupportedSchemaSyntaxCode, Loc{}, "").SpecRef() {
						t.Fatalf("Strict10 mismatch diagnostic = %s", d)
					}
					return
				}
				if d.SpecRef() != newSchemaSyntaxUnsupported(Loc{}, "").SpecRef() || d.Unwrap() != nil {
					t.Fatalf("true inheritable unsupported diagnostic = %s", d)
				}
			})
		}
	}
}

//nolint:gocognit // Typed and local shape boundaries share one public fixture matrix.
func TestUntypedGlobalAttributeInheritableKeepsTypedAndLocalShapes(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root">
  <xs:attribute name="direct" type="xs:integer" inheritable="true"/>
  <xs:attribute name="named" type="r:Named" inheritable="1"/>
  <xs:attribute name="inline" inheritable="true"><xs:simpleType><xs:restriction base="xs:decimal"/></xs:simpleType></xs:attribute>
  <xs:simpleType name="Named"><xs:restriction base="xs:boolean"/></xs:simpleType>
  <xs:complexType name="Record"><xs:attribute name="local" type="xs:boolean" inheritable="false"/></xs:complexType>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatalf("typed/local shapes: %v", err)
			}
			for _, name := range []string{"direct", "named", "inline"} {
				found := schema.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:root", name))
				if len(found) != 1 {
					t.Fatalf("%s declarations = %d", name, len(found))
				}
				declaration, ok := found[0].AttributeDeclaration()
				if !ok || !declaration.IsInheritable() || declaration.Loc() != found[0].Loc() {
					t.Fatalf("%s declaration = %#v/%t", name, declaration, ok)
				}
			}
			owner := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Record"))
			if len(owner) != 1 {
				t.Fatalf("Record owners = %d", len(owner))
			}
			definition, ok := owner[0].ComplexTypeDefinition()
			if !ok || len(definition.AttributeUses()) != 1 {
				t.Fatalf("local owner = %#v/%t", definition, ok)
			}
			use, ok := definition.AttributeUses()[0].(LocalAttributeUse)
			if !ok || use.NameLoc() != elementReferenceTestAttributeLoc(t, root, `name="local"`) || use.TypeLoc() != elementReferenceTestAttributeLoc(t, root, `type="xs:boolean" inheritable=`) {
				t.Fatalf("local use = %#v", definition.AttributeUses()[0])
			}
		})
	}
	for _, test := range []struct{ name, body, marker string }{
		{"global ref", `<xs:attribute ref="r:item" inheritable="false"/>`, `ref="r:item"`},
		{"inline owner local", `<xs:element name="root"><xs:complexType><xs:attribute name="local" type="xs:boolean" inheritable="false"/></xs:complexType></xs:element>`, `inheritable="false"`},
		{"named owner local ref", `<xs:complexType name="Record"><xs:attribute ref="r:item" inheritable="false"/></xs:complexType>`, `inheritable="false"`},
		{"inline owner local ref", `<xs:element name="root"><xs:complexType><xs:attribute ref="r:item" inheritable="false"/></xs:complexType></xs:element>`, `inheritable="false"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root"><xs:attribute name="item" inheritable="0"/>` + test.body + `</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
				t.Fatalf("excluded shape returned schema: %#v/%v", schema, err)
			}
			d := requireDiagnostic(t, err)
			if d.Loc() != elementReferenceTestAttributeLoc(t, root, test.marker) || len(d.Related()) != 0 {
				t.Fatalf("excluded shape location = %s related %v", d.Loc(), d.Related())
			}
			if test.name == "global ref" {
				if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode {
					t.Fatalf("global ref diagnostic = %s", d)
				}
				return
			}
			if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("excluded local shape diagnostic = %s", d)
			}
		})
	}
}

func TestUntypedGlobalAttributeInheritableReferenceConsumersStayUnsupported(t *testing.T) {
	for _, owner := range []struct{ name, body string }{
		{"named", `<xs:complexType name="Record"><xs:attribute ref="r:item"/></xs:complexType>`},
		{"inline", `<xs:element name="root"><xs:complexType><xs:attribute ref="r:item"/></xs:complexType></xs:element>`},
	} {
		for _, form := range []string{"", ` inheritable="false"`, ` inheritable="0"`} {
			t.Run(owner.name+form, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root"><xs:attribute name="item"` + form + `/>` + owner.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
				if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
					t.Fatalf("untyped reference returned schema: %#v/%v", schema, err)
				}
				d := requireDiagnostic(t, err)
				wantRelated := []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="item"`)}
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode ||
					d.Loc() != elementReferenceTestAttributeLoc(t, root, `ref="r:item"`) ||
					!reflect.DeepEqual(d.Related(), wantRelated) || d.SpecRef() != schemaAttributeUseSpecRef(XSDVersion11) ||
					!errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaAttributeReferenceUnsupported) {
					t.Fatalf("untyped reference diagnostic = %s related %v", d, d.Related())
				}
			})
		}
	}
}

func TestUntypedGlobalAttributeInheritableImportedReferenceIsVisibleButUnsupported(t *testing.T) {
	for _, form := range []string{"", ` inheritable="false"`, ` inheritable="0"`} {
		t.Run(form, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:root">
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:complexType name="Record"><xs:attribute ref="o:item"/></xs:complexType>
</xs:schema>`
			other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other">
  <xs:attribute name="item"` + form + `/>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
				"other.xsd": {id: "other.xsd", contents: other},
			}, Strict11)
			if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
				t.Fatalf("imported untyped reference returned schema: %#v/%v", schema, err)
			}
			d := requireDiagnostic(t, err)
			wantRelated := []Loc{schemaBlockTestAttributeLoc(t, "other.xsd", other, `<xs:attribute name="item"`)}
			if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode ||
				d.Loc() != elementReferenceTestAttributeLoc(t, root, `ref="o:item"`) ||
				!reflect.DeepEqual(d.Related(), wantRelated) || d.SpecRef() != schemaAttributeUseSpecRef(XSDVersion11) ||
				!errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaAttributeReferenceUnsupported) {
				t.Fatalf("imported reference diagnostic = %s related %v", d, d.Related())
			}
		})
	}
}

//nolint:gocognit // Competing diagnostics must keep their specific metadata and causes.
func TestUntypedGlobalAttributeInheritableKeepsCompetingExits(t *testing.T) {
	for _, test := range []struct {
		name, body, marker string
		class              FailureClass
		code               string
	}{
		{"default", `<xs:attribute name="item" inheritable="false" default="1"/>`, `default="1"`, FailureUnsupported, UnsupportedSchemaSyntaxCode},
		{"fixed", `<xs:attribute name="item" inheritable="0" fixed="1"/>`, `fixed="1"`, FailureUnsupported, UnsupportedSchemaSyntaxCode},
		{"default fixed conflict", `<xs:attribute name="item" inheritable="false" default="1" fixed="2"/>`, `fixed="2"`, FailureInvalid, invalidSchemaCompositionCode},
		{"true before default", `<xs:attribute name="item" inheritable="true" default="1"/>`, `inheritable="true"`, FailureUnsupported, UnsupportedSchemaSyntaxCode},
		{"malformed before default", `<xs:attribute name="item" inheritable="maybe" default="1"/>`, `inheritable="maybe"`, FailureInvalid, invalidSchemaCompositionCode},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `">` + test.body + `</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
				t.Fatalf("competing exit returned schema: %#v/%v", schema, err)
			}
			d := requireDiagnostic(t, err)
			if d.Class() != test.class || d.Code() != test.code || d.Loc() != elementReferenceTestAttributeLoc(t, root, test.marker) {
				t.Fatalf("competing exit diagnostic = %s", d)
			}
			switch test.name {
			case "default", "fixed":
				if len(d.Related()) != 0 || d.SpecRef() != schemaAttributeValueConstraintSpecRef(XSDVersion11) ||
					!errors.Is(err, errSchemaAttributeValueConstraintUnsupported) || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("value constraint exit = %s related %v", d, d.Related())
				}
			case "default fixed conflict":
				wantRelated := []Loc{elementReferenceTestAttributeLoc(t, root, `default="1"`)}
				if !reflect.DeepEqual(d.Related(), wantRelated) || d.SpecRef() != schemaAttributeValueConstraintSpecRef(XSDVersion11) ||
					!errors.Is(err, errSchemaAttributeValueConstraintConflict) {
					t.Fatalf("value conflict exit = %s related %v", d, d.Related())
				}
			case "true before default":
				if len(d.Related()) != 0 || d.SpecRef() != newSchemaSyntaxUnsupported(Loc{}, "").SpecRef() ||
					d.Unwrap() != nil || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("true exit = %s related %v", d, d.Related())
				}
			case "malformed before default":
				cause := requireDiagnostic(t, d.Unwrap())
				if len(d.Related()) != 0 || d.SpecRef() != strictBooleanSpecRef(XSDVersion11) ||
					cause.Code() != InvalidBooleanLexicalCode || cause.Loc() != d.Loc() || cause.SpecRef() != d.SpecRef() {
					t.Fatalf("malformed exit = %s, cause %s", d, cause)
				}
			}
		})
	}
}
