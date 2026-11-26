package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func inlineNegativeSkipSchema(namespace, body, wildcard string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `"` + namespace + `><xs:element name="root"><xs:complexType>` + body + wildcard + `</xs:complexType></xs:element></xs:schema>`
}

func inlineNegativeSkipWildcard(attributes string) string {
	return `<xs:anyAttribute` + attributes + `/>`
}

//nolint:gocognit // Compare every admitted lexical form through immutable public queries.
func TestInlineNegativeSkipEmptyQNameFacts(t *testing.T) {
	forms := []struct {
		name, attribute string
		present         bool
	}{
		{"absent", "", false},
		{"empty", ` notQName=""`, true},
		{"xml whitespace", ` notQName=" &#x9;&#xA;&#xD; "`, true},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, form := range forms {
			t.Run(string(policy)+"/"+form.name, func(t *testing.T) {
				root := inlineNegativeSkipSchema(` xmlns:p="urn:p" targetNamespace="urn:root"`,
					`<xs:sequence minOccurs="1" maxOccurs="1"/>`,
					inlineNegativeSkipWildcard(` notNamespace="urn:z ##targetNamespace ##local urn:z" processContents="skip"`+form.attribute))
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err != nil {
					t.Fatalf("parse: %v", err)
				}
				components := schema.Components()
				if len(components) != 1 || components[0].Name() != mustTestQName(t, "urn:root", "root") {
					t.Fatalf("components = %v", components)
				}
				declaration, ok := components[0].ElementDeclaration()
				if !ok || declaration.Loc() != anyAttributeTestLoc(root, `<xs:element`) {
					t.Fatalf("element = %#v", declaration)
				}
				inline, ok := declaration.InlineComplexType()
				if !ok || inline.Loc() != anyAttributeTestLoc(root, `<xs:complexType`) || len(inline.AttributeUses()) != 0 {
					t.Fatalf("inline = %#v", inline)
				}
				sequence, ok := inline.Particle().(SequenceParticle)
				if !ok || sequence.Occurrences().String() != "1/1" || !sequence.Occurrences().IsDefault() ||
					sequence.Loc() != anyAttributeTestLoc(root, `<xs:sequence`) || len(sequence.Particles()) != 0 {
					t.Fatalf("sequence = %#v", inline.Particle())
				}
				wildcard := inlineNegativeSkipWildcardFacts(t, inline)
				wantNamespaces := []string{"", "urn:root", "urn:z"}
				if wildcard.Loc() != anyAttributeTestLoc(root, `<xs:anyAttribute`) ||
					wildcard.ProcessContents() != "skip" || wildcard.ProcessContentsLoc() != anyAttributeTestLoc(root, `processContents=`) ||
					wildcard.NamespaceConstraint().Variety() != WildcardNamespaceConstraintNot ||
					wildcard.NamespaceConstraint().LexicalForm() != "urn:z ##targetNamespace ##local urn:z" ||
					wildcard.NamespaceConstraint().Loc() != anyAttributeTestLoc(root, `notNamespace=`) ||
					!reflect.DeepEqual(wildcard.NamespaceConstraint().Namespaces(), wantNamespaces) {
					t.Fatalf("wildcard = %#v", wildcard)
				}
				qname := wildcard.QNameConstraint()
				wantLoc := Loc{}
				if form.present {
					wantLoc = anyAttributeTestLoc(root, `notQName=`)
				}
				if qname.Present() != form.present || qname.Loc() != wantLoc || qname.LexicalForm() != "" ||
					len(qname.Tokens()) != 0 || len(qname.Names()) != 0 {
					t.Fatalf("QName constraint = %#v", qname)
				}
				if form.present {
					bindings := qname.NamespaceBindings()
					if len(bindings) == 0 {
						t.Fatal("missing captured namespace bindings")
					}
					original := append([]IdentityNamespaceBinding(nil), bindings...)
					bindings[0].Namespace = "changed"
					if !reflect.DeepEqual(inlineNegativeSkipWildcardFacts(t, inline).QNameConstraint().NamespaceBindings(), original) {
						t.Fatal("namespace bindings exposed mutable storage")
					}
				}
				if !form.present && len(qname.NamespaceBindings()) != 0 {
					t.Fatalf("absent QName bindings = %v", qname.NamespaceBindings())
				}
				namespaces := wildcard.NamespaceConstraint().Namespaces()
				namespaces[0] = "changed"
				if !reflect.DeepEqual(inlineNegativeSkipWildcardFacts(t, inline).NamespaceConstraint().Namespaces(), wantNamespaces) {
					t.Fatal("namespace exclusions exposed mutable storage")
				}
				walked := []ComponentID{}
				if walkErr := schema.Walk(func(component Component) error { walked = append(walked, component.ID()); return nil }); walkErr != nil {
					t.Fatalf("walk: %v", walkErr)
				}
				if !reflect.DeepEqual(walked, []ComponentID{declaration.ID()}) {
					t.Fatalf("walk order = %v", walked)
				}
				again, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err != nil || !reflect.DeepEqual(again.Components(), components) {
					t.Fatalf("repeated build = %v, %v", again.Components(), err)
				}
			})
		}
	}
}

//nolint:gocognit // Check copied public facts and the written empty sequence across both admitting policies.
func TestInlineNegativeSkipEmptySequenceFacts(t *testing.T) {
	forms := []struct {
		name, namespace, negative, lexical string
		want                               []string
	}{
		{"local", ` targetNamespace="urn:root"`, `##local`, `##local`, []string{""}},
		{"target", ` targetNamespace="urn:root"`, `##targetNamespace`, `##targetNamespace`, []string{"urn:root"}},
		{"literals", ` targetNamespace="urn:root"`, `urn:z urn:a urn:z`, `urn:z urn:a urn:z`, []string{"urn:a", "urn:z"}},
		{"mixed", ` targetNamespace="urn:root"`, `urn:z ##targetNamespace ##local urn:a`, `urn:z ##targetNamespace ##local urn:a`, []string{"", "urn:a", "urn:root", "urn:z"}},
		{"no-target", "", `##targetNamespace`, `##targetNamespace`, []string{""}},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, form := range forms {
			t.Run(string(policy)+"/"+form.name, func(t *testing.T) {
				wildcard := inlineNegativeSkipWildcard(` notNamespace="` + form.negative + `" processContents="skip"`)
				root := inlineNegativeSkipSchema(form.namespace, `<xs:sequence><xs:annotation><xs:documentation><opaque>text</opaque></xs:documentation></xs:annotation></xs:sequence>`, wildcard)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err != nil {
					t.Fatalf("parse inline negative skip: %v", err)
				}
				components := schema.Components()
				ownerNamespace := "urn:root"
				if form.namespace == "" {
					ownerNamespace = ""
				}
				if len(components) != 1 || components[0].Kind() != ComponentKindElementDeclaration || components[0].Name() != mustTestQName(t, ownerNamespace, "root") {
					t.Fatalf("components = %v", components)
				}
				declaration, ok := components[0].ElementDeclaration()
				if !ok || declaration.ID().Source() != "root.xsd" || declaration.ID().Ordinal() != 1 || declaration.Loc() != anyAttributeTestLoc(root, `<xs:element`) {
					t.Fatalf("element declaration = %#v", declaration)
				}
				if owner, found := schema.Lookup(declaration.ID()); !found || !reflect.DeepEqual(owner, components[0]) {
					t.Fatalf("element owner lookup = %#v, %v", owner, found)
				}
				inline, ok := declaration.InlineComplexType()
				if !ok || !inline.ID().IsZero() || !inline.Name().IsZero() || inline.Loc() != anyAttributeTestLoc(root, `<xs:complexType`) {
					t.Fatalf("anonymous type = %#v", inline)
				}
				if nodeID, present := inline.NodeID(); !present || nodeID.Source() != declaration.ID().Source() || nodeID.Ordinal() != 1 {
					t.Fatalf("anonymous type node ID = %v, %v", nodeID, present)
				}
				if len(schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, ownerNamespace, "root"))) != 0 {
					t.Fatal("anonymous type appeared as a named component")
				}
				if _, hasBase := inline.BaseReference(); hasBase || len(inline.AttributeUses()) != 0 {
					t.Fatalf("inline effective base/uses = %#v / %v", inline, inline.AttributeUses())
				}
				sequence, ok := inline.Particle().(SequenceParticle)
				if !ok || sequence.Loc() != anyAttributeTestLoc(root, `<xs:sequence`) || !sequence.Occurrences().IsDefault() || len(sequence.Particles()) != 0 || len(sequence.Elements()) != 0 {
					t.Fatalf("written empty sequence = %#v", inline.Particle())
				}
				attribute, ok := inline.AnyAttribute()
				if !ok {
					t.Fatal("inline attribute wildcard absent")
				}
				constraint := attribute.NamespaceConstraint()
				if constraint.Variety() != WildcardNamespaceConstraintNot || constraint.LexicalForm() != form.lexical ||
					!reflect.DeepEqual(constraint.Namespaces(), form.want) || attribute.Namespace() != form.lexical ||
					constraint.Loc() != anyAttributeTestLoc(root, `notNamespace=`) || attribute.NamespaceLoc() != constraint.Loc() ||
					attribute.Loc() != anyAttributeTestLoc(root, `<xs:anyAttribute`) || attribute.ProcessContents() != "skip" ||
					attribute.ProcessContentsLoc() != anyAttributeTestLoc(root, `processContents=`) || attribute.QNameConstraint().Present() {
					t.Fatalf("wildcard = %#v, effective = %v", attribute, constraint.Namespaces())
				}
				copied := constraint.Namespaces()
				copied[0] = "mutated"
				if !reflect.DeepEqual(inlineNegativeSkipWildcardFacts(t, inline).NamespaceConstraint().Namespaces(), form.want) {
					t.Fatal("wildcard query exposed mutable exclusions")
				}
				walked := []ComponentID{}
				if walkErr := schema.Walk(func(component Component) error { walked = append(walked, component.ID()); return nil }); walkErr != nil {
					t.Fatalf("walk: %v", walkErr)
				}
				if !reflect.DeepEqual(walked, []ComponentID{declaration.ID()}) || !reflect.DeepEqual(schema.Components(), components) {
					t.Fatalf("schema order changed: %v", walked)
				}
				again, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err != nil || !reflect.DeepEqual(again.Components(), components) {
					t.Fatalf("repeated build = %v, %v", again.Components(), err)
				}
			})
		}
	}
}

func inlineNegativeSkipWildcardFacts(t *testing.T, definition ComplexTypeDefinition) AnyAttribute {
	t.Helper()
	attribute, ok := definition.AnyAttribute()
	if !ok {
		t.Fatal("wildcard absent on repeated query")
	}
	return attribute
}

func TestInlineNegativeSkipExplicitDefaultBounds(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		root := inlineNegativeSkipSchema("", `<xs:sequence minOccurs="1" maxOccurs="1"/>`, inlineNegativeSkipWildcard(` notNamespace="##targetNamespace" processContents="skip"`))
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
		if err != nil {
			t.Fatalf("parse explicit default bounds: %v", err)
		}
		declaration, ok := schema.Components()[0].ElementDeclaration()
		if !ok {
			t.Fatal("no element declaration")
		}
		inline, ok := declaration.InlineComplexType()
		if !ok {
			t.Fatal("no inline complex type")
		}
		sequence, ok := inline.Particle().(SequenceParticle)
		if !ok || !sequence.Occurrences().IsDefault() || sequence.Loc() != anyAttributeTestLoc(root, `<xs:sequence`) || len(sequence.Particles()) != 0 {
			t.Fatalf("explicit default sequence = %#v", inline.Particle())
		}
	}
}

func TestInlineNegativeSkipNamedDirectOwnerStillQueryable(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:complexType name="Named"><xs:sequence/><xs:anyAttribute notNamespace="##local" processContents="skip" notQName=""/></xs:complexType></xs:schema>`
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
		if err != nil {
			t.Fatalf("named direct owner: %v", err)
		}
		components := schema.Components()
		if len(components) != 1 {
			t.Fatalf("components = %v", components)
		}
		definition, ok := components[0].ComplexType()
		if !ok || components[0].Name() != mustTestQName(t, "urn:root", "Named") {
			t.Fatalf("named owner = %#v", components[0])
		}
		wildcard := inlineNegativeSkipWildcardFacts(t, definition)
		if wildcard.ProcessContents() != "skip" || !reflect.DeepEqual(wildcard.NamespaceConstraint().Namespaces(), []string{""}) ||
			!wildcard.QNameConstraint().Present() || wildcard.QNameConstraint().Loc() != anyAttributeTestLoc(root, `notQName=`) {
			t.Fatalf("named direct wildcard = %#v", wildcard)
		}
	}
}

func TestInlineNegativeSkipConsumersReject(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, qname := range []string{` notQName=""`, ` notQName=" &#x9;&#xA;&#xD; "`} {
			root := inlineNegativeSkipSchema(` targetNamespace="urn:root"`, `<xs:sequence/>`, inlineNegativeSkipWildcard(` notNamespace="##local" processContents="skip"`+qname))
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			instanceErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
			instance := requireDiagnostic(t, instanceErr)
			wantValidationRelated := []Loc{
				anyAttributeTestLoc(root, `<xs:element`),
				anyAttributeTestLoc(root, `<xs:complexType`),
				anyAttributeTestLoc(root, `<xs:sequence`),
				anyAttributeTestLoc(root, `<xs:anyAttribute`),
			}
			if instance.Class() != FailureUnsupported || instance.Code() != UnsupportedInstanceValidationCode ||
				instance.Loc() != mustTestLoc(t, "instance.xml", 1, 1) || instance.SpecRef() != instanceValidationSpecRef(XSDVersion11) ||
				!reflect.DeepEqual(instance.Related(), wantValidationRelated) ||
				!errors.Is(instanceErr, errInstanceAttributes) {
				t.Fatalf("validation = %s, related = %v", instance, instance.Related())
			}
			output, generationErr := GenerateGo(schema, "generated")
			generation := requireDiagnostic(t, generationErr)
			wantGenerationRelated := wantValidationRelated[:3]
			if output != nil || generation.Class() != FailureUnsupported || generation.Code() != diagnosticCodegenUnsupported ||
				generation.Loc() != anyAttributeTestLoc(root, `<xs:anyAttribute`) ||
				!reflect.DeepEqual(generation.Related(), wantGenerationRelated) ||
				generation.SpecRef() != schemaElementTypeSpecRef(XSDVersion11) ||
				!errors.Is(generationErr, errCodegenUnsupported) {
				t.Fatalf("generation = (%q, %s), related = %v", output, generation, generation.Related())
			}
		}
	}
}

func TestInlineNegativeSkipExcludedShapes(t *testing.T) {
	type form struct {
		name, body, wildcard, marker, code, spec string
		policy                                   LanguagePolicy
		class                                    FailureClass
		cause                                    error
	}
	base := inlineNegativeSkipWildcard(` notNamespace="##local" processContents="skip" notQName=""`)
	forms := []form{
		{"no sequence", "", base, `<xs:anyAttribute`, UnsupportedSchemaSyntaxCode, schemaAnyAttributeSpecRef(XSDVersion11), Strict11, FailureUnsupported, errSchemaAnyAttributeUnsupported},
		{"nonempty sequence", `<xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence>`, base, `<xs:anyAttribute`, UnsupportedSchemaSyntaxCode, schemaAnyAttributeSpecRef(XSDVersion11), Strict11, FailureUnsupported, errSchemaAnyAttributeUnsupported},
		{"choice", `<xs:choice/>`, base, `<xs:anyAttribute`, UnsupportedSchemaSyntaxCode, schemaAnyAttributeSpecRef(XSDVersion11), Strict11, FailureUnsupported, errSchemaAnyAttributeUnsupported},
		{"zero bounds", `<xs:sequence minOccurs="0" maxOccurs="0"/>`, base, `<xs:anyAttribute`, UnsupportedSchemaSyntaxCode, schemaAnyAttributeSpecRef(XSDVersion11), Strict11, FailureUnsupported, errSchemaAnyAttributeUnsupported},
		{"optional bounds", `<xs:sequence minOccurs="0" maxOccurs="1"/>`, base, `<xs:anyAttribute`, UnsupportedSchemaSyntaxCode, schemaAnyAttributeSpecRef(XSDVersion11), Strict11, FailureUnsupported, errSchemaAnyAttributeUnsupported},
		{"repeated bounds", `<xs:sequence minOccurs="2" maxOccurs="2"/>`, base, `<xs:anyAttribute`, UnsupportedSchemaSyntaxCode, schemaAnyAttributeSpecRef(XSDVersion11), Strict11, FailureUnsupported, errSchemaAnyAttributeUnsupported},
		{"unbounded bounds", `<xs:sequence maxOccurs="unbounded"/>`, base, `<xs:anyAttribute`, UnsupportedSchemaSyntaxCode, schemaAnyAttributeSpecRef(XSDVersion11), Strict11, FailureUnsupported, errSchemaAnyAttributeUnsupported},
		{"attribute use", `<xs:sequence/><xs:attribute name="a" type="xs:string"/>`, base, `<xs:anyAttribute`, UnsupportedSchemaSyntaxCode, schemaAnyAttributeSpecRef(XSDVersion11), Strict11, FailureUnsupported, errSchemaAnyAttributeUnsupported},
		{"strict mode", `<xs:sequence/>`, inlineNegativeSkipWildcard(` notNamespace="##local"`), `<xs:anyAttribute`, UnsupportedSchemaSyntaxCode, schemaAnyAttributeSpecRef(XSDVersion11), Strict11, FailureUnsupported, errSchemaAnyAttributeUnsupported},
		{"lax mode", `<xs:sequence/>`, inlineNegativeSkipWildcard(` notNamespace="##local" processContents="lax"`), `<xs:anyAttribute`, UnsupportedSchemaSyntaxCode, schemaAnyAttributeSpecRef(XSDVersion11), Strict11, FailureUnsupported, errSchemaAnyAttributeUnsupported},
		{"qname", `<xs:sequence/>`, inlineNegativeSkipWildcard(` notNamespace="##local" processContents="skip" notQName="##defined"`), `<xs:anyAttribute`, UnsupportedSchemaSyntaxCode, schemaAnyAttributeSpecRef(XSDVersion11), Strict11, FailureUnsupported, errSchemaAnyAttributeUnsupported},
		{"explicit qname", `<xs:sequence/>`, inlineNegativeSkipWildcard(` notNamespace="##local" processContents="skip" notQName="xs:string"`), `<xs:anyAttribute`, UnsupportedSchemaSyntaxCode, schemaAnyAttributeSpecRef(XSDVersion11), Strict11, FailureUnsupported, errSchemaAnyAttributeUnsupported},
		{"sibling qname", `<xs:sequence/>`, inlineNegativeSkipWildcard(` notNamespace="##local" processContents="skip" notQName="##definedSibling"`), `notQName=`, invalidSchemaCompositionCode, "", Strict11, FailureInvalid, nil},
		{"malformed qname", `<xs:sequence/>`, inlineNegativeSkipWildcard(` notNamespace="##local" processContents="skip" notQName="bad:q:name"`), `notQName=`, invalidSchemaConditionalCode, schemaWildcardQNameDatatypeXSD11SpecRef, Strict11, FailureInvalid, nil},
		{"unbound qname", `<xs:sequence/>`, inlineNegativeSkipWildcard(` notNamespace="##local" processContents="skip" notQName="missing:item"`), `notQName=`, invalidSchemaConditionalCode, schemaWildcardQNameDatatypeXSD11SpecRef, Strict11, FailureInvalid, nil},
		{"unicode whitespace qname", `<xs:sequence/>`, inlineNegativeSkipWildcard(" notNamespace=\"##local\" processContents=\"skip\" notQName=\"\u00a0\""), `notQName=`, invalidSchemaConditionalCode, schemaWildcardQNameDatatypeXSD11SpecRef, Strict11, FailureInvalid, nil},
		{"qname outside negative", `<xs:sequence/>`, inlineNegativeSkipWildcard(` notNamespace="` + testXSDNamespace + `" processContents="skip" notQName="xs:string"`), `notQName=`, invalidSchemaCompositionCode, schemaWildcardQNameNamespaceXSD11SpecRef, Strict11, FailureInvalid, errSchemaWildcardQNameNamespace},
		{"positive", `<xs:sequence/>`, inlineNegativeSkipWildcard(` namespace="##other" processContents="skip"`), `<xs:anyAttribute`, UnsupportedSchemaSyntaxCode, schemaAnyAttributeSpecRef(XSDVersion11), Strict11, FailureUnsupported, errSchemaAnyAttributeUnsupported},
		{"strict10", `<xs:sequence/>`, base, `notNamespace=`, UnsupportedSchemaSyntaxCode, "xsd11-structures#cSchemaDocument", Strict10, FailureUnsupported, errLanguagePolicyMismatch},
		{"empty negative", `<xs:sequence/>`, inlineNegativeSkipWildcard(` notNamespace=" " processContents="skip"`), `notNamespace=`, invalidSchemaCompositionCode, "", Strict11, FailureInvalid, nil},
		{"malformed negative", `<xs:sequence/>`, inlineNegativeSkipWildcard(` notNamespace="##bogus" processContents="skip"`), `notNamespace=`, invalidSchemaCompositionCode, "", Strict11, FailureInvalid, nil},
		{"forbidden negative", `<xs:sequence/>`, inlineNegativeSkipWildcard(` notNamespace="##any" processContents="skip"`), `notNamespace=`, invalidSchemaCompositionCode, "", Strict11, FailureInvalid, nil},
		{"namespace conflict", `<xs:sequence/>`, inlineNegativeSkipWildcard(` notNamespace="##local" namespace="##any" processContents="skip"`), `namespace=`, invalidSchemaCompositionCode, "", Strict11, FailureInvalid, nil},
		{"duplicate negative", `<xs:sequence/>`, inlineNegativeSkipWildcard(` notNamespace="##local" notNamespace="urn:a" processContents="skip"`), `notNamespace="urn:a"`, InvalidXMLSyntaxCode, "", Strict11, FailureInvalid, nil},
		{"bad mode", `<xs:sequence/>`, inlineNegativeSkipWildcard(` notNamespace="##local" processContents="bad"`), `processContents=`, invalidSchemaCompositionCode, "", Strict11, FailureInvalid, nil},
		{"invalid bounds", `<xs:sequence minOccurs="-1"/>`, base, `minOccurs=`, invalidSchemaCompositionCode, "xsd11-datatypes#nonNegativeInteger", Strict11, FailureInvalid, nil},
		{"invalid child", `<xs:sequence><xs:attribute name="a" type="xs:string"/></xs:sequence>`, base, `<xs:attribute`, invalidSchemaCompositionCode, "", Strict11, FailureInvalid, nil},
		{"mixed content", `<xs:sequence/>`, base, `mixed=`, UnsupportedSchemaSyntaxCode, "xsd11-structures#cSchemaDocument", Strict11, FailureUnsupported, ErrUnsupported},
	}
	for _, tc := range forms {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(tc.wildcard, "notQName=") {
				tc.wildcard = strings.Replace(tc.wildcard, `/>`, ` notQName=""/>`, 1)
			}
			root := inlineNegativeSkipSchema(` targetNamespace="urn:root"`, tc.body, tc.wildcard)
			if tc.name == "mixed content" {
				root = strings.Replace(root, `<xs:complexType>`, `<xs:complexType mixed="true">`, 1)
			}
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, tc.policy)
			assertZeroSchema(t, schema)
			got := requireDiagnostic(t, err)
			var related []Loc
			if tc.name == "qname outside negative" {
				related = []Loc{anyAttributeTestLoc(root, `notNamespace=`)}
			}
			if got.Class() != tc.class || got.Code() != tc.code || got.Loc() != anyAttributeTestLoc(root, tc.marker) ||
				got.SpecRef() != tc.spec || !reflect.DeepEqual(got.Related(), related) || tc.cause != nil && !errors.Is(err, tc.cause) {
				t.Fatalf("diagnostic = %s, class=%s code=%s loc=%s wantLoc=%s spec=%q related=%v cause=%v", got, got.Class(), got.Code(), got.Loc(), anyAttributeTestLoc(root, tc.marker), got.SpecRef(), got.Related(), err)
			}
		})
	}
}

//nolint:gocognit // One graph covers include adoption, import, a cycle, and stable discovery order.
func TestInlineNegativeSkipGraphProvenance(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:include schemaLocation="included.xsd"/><xs:include schemaLocation="included.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`
	included := inlineNegativeSkipSchema("", `<xs:sequence/>`, inlineNegativeSkipWildcard(` notNamespace="##targetNamespace urn:z ##local" processContents="skip" notQName=""`))
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:import namespace="urn:root" schemaLocation="root.xsd"/><xs:element name="other"><xs:complexType><xs:sequence/><xs:anyAttribute notNamespace="##targetNamespace" processContents="skip" notQName=" &#x9;&#xA;&#xD; "/></xs:complexType></xs:element></xs:schema>`
	fixtures := map[string]discoveryFixture{
		"root.xsd":     {id: "root.xsd", contents: root},
		"included.xsd": {id: "included.xsd", contents: included},
		"other.xsd":    {id: "other.xsd", contents: other},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		var previous []Component
		for run := 0; run < 2; run++ {
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy)
			if err != nil {
				t.Fatalf("graph %s run %d: %v", policy, run, err)
			}
			documents := schema.Documents()
			if len(documents) != 3 || documents[0].Source() != "root.xsd" || documents[1].Source() != "included.xsd" || documents[2].Source() != "other.xsd" {
				t.Fatalf("graph documents = %v", documents)
			}
			components := schema.Components()
			if len(components) != 2 {
				t.Fatalf("graph components = %v", components)
			}
			walked := []ComponentID{}
			if err := schema.Walk(func(component Component) error { walked = append(walked, component.ID()); return nil }); err != nil {
				t.Fatalf("graph walk: %v", err)
			}
			if !reflect.DeepEqual(walked, []ComponentID{components[0].ID(), components[1].ID()}) {
				t.Fatalf("graph walk order = %v", walked)
			}
			for index, want := range []struct {
				name       QName
				source     SourceID
				contents   string
				namespaces []string
			}{
				{mustTestQName(t, "urn:root", "root"), "included.xsd", included, []string{"", "urn:root", "urn:z"}},
				{mustTestQName(t, "urn:other", "other"), "other.xsd", other, []string{"urn:other"}},
			} {
				component := components[index]
				declaration, ok := component.ElementDeclaration()
				if !ok || component.Name() != want.name || component.Document() != want.source ||
					component.Loc() != anyAttributeTestLocForSource(want.source, want.contents, `<xs:element`) {
					t.Fatalf("graph component %d = %v", index, component)
				}
				inline, ok := declaration.InlineComplexType()
				if !ok || inline.Loc() != anyAttributeTestLocForSource(want.source, want.contents, `<xs:complexType`) {
					t.Fatalf("graph inline type %d = %#v", index, inline)
				}
				if nodeID, present := inline.NodeID(); !present || nodeID.Source() != component.ID().Source() || nodeID.Ordinal() != 1 || !inline.ID().IsZero() {
					t.Fatalf("graph inline identity %d = %v, %v", index, nodeID, present)
				}
				attribute := inlineNegativeSkipWildcardFacts(t, inline)
				if attribute.Loc() != anyAttributeTestLocForSource(want.source, want.contents, `<xs:anyAttribute`) ||
					attribute.NamespaceLoc() != anyAttributeTestLocForSource(want.source, want.contents, `notNamespace=`) ||
					!reflect.DeepEqual(attribute.NamespaceConstraint().Namespaces(), want.namespaces) || attribute.ProcessContents() != "skip" ||
					!attribute.QNameConstraint().Present() || attribute.QNameConstraint().Loc() != anyAttributeTestLocForSource(want.source, want.contents, `notQName=`) ||
					attribute.QNameConstraint().LexicalForm() != "" || len(attribute.QNameConstraint().Tokens()) != 0 || len(attribute.QNameConstraint().Names()) != 0 {
					t.Fatalf("graph wildcard %d = %#v", index, attribute)
				}
			}
			if run == 0 {
				previous = components
				continue
			}
			if !reflect.DeepEqual(previous, components) {
				t.Fatal("repeated graph changed component facts or order")
			}
		}
	}
}

func TestInlineNegativeSkipGraphResolutionFailure(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		body := `<xs:include schemaLocation="missing.xsd"/><xs:element name="root"><xs:complexType><xs:sequence/><xs:anyAttribute notNamespace="##local" processContents="skip"/></xs:complexType></xs:element>`
		root := `<xs:schema xmlns:xs="` + testXSDNamespace + `">` + body + `</xs:schema>`
		source, err := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
		if err != nil {
			t.Fatalf("root source: %v", err)
		}
		cause := errors.New("resolver unavailable")
		resolver := &discoveryResolver{failures: map[string]error{"missing.xsd": cause}}
		schema, err := ParseSchemaWithPolicy(source, resolver, policy)
		assertZeroSchema(t, schema)
		d := requireDiagnostic(t, err)
		if d.Class() != FailureResolution || d.Code() != SourceResolveCode || d.Loc() != anyAttributeTestLoc(root, `<xs:include`) ||
			len(d.Related()) != 0 || d.SpecRef() != "" || !errors.Is(err, cause) || len(resolver.calls) != 1 {
			t.Fatalf("resolution diagnostic = %s, related=%v, calls=%v, cause=%v", d, d.Related(), resolver.calls, err)
		}
	}
}

func TestInlineNegativeSkipReferenceShapeExclusions(t *testing.T) {
	wildcard := inlineNegativeSkipWildcard(` notNamespace="##local" processContents="skip" notQName=""`)
	for _, tc := range []struct {
		name, body, marker, spec string
		cause                    error
	}{
		{"attribute ref", `<xs:sequence/><xs:attribute ref="t:missing"/>` + wildcard, `<xs:anyAttribute`, schemaAnyAttributeSpecRef(XSDVersion11), errSchemaAnyAttributeUnsupported},
		{"attribute group ref", `<xs:sequence/><xs:attributeGroup ref="t:missing"/>` + wildcard, `<xs:attributeGroup`, "xsd10-structures#schema-document", nil},
		{"element ref", `<xs:sequence><xs:element ref="t:missing"/></xs:sequence>` + wildcard, `<xs:anyAttribute`, schemaAnyAttributeSpecRef(XSDVersion11), errSchemaAnyAttributeUnsupported},
		{"local inline", `<xs:complexType name="Owner"><xs:sequence><xs:element name="local"><xs:complexType><xs:sequence/>` + wildcard + `</xs:complexType></xs:element></xs:sequence></xs:complexType>`, `<xs:anyAttribute`, schemaAnyAttributeSpecRef(XSDVersion11), errSchemaAnyAttributeUnsupported},
	} {
		root := inlineNegativeSkipSchema(` xmlns:t="urn:root" targetNamespace="urn:root"`, tc.body, "")
		if tc.name == "local inline" {
			root = `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root">` + tc.body + `</xs:schema>`
		}
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
		assertZeroSchema(t, schema)
		d := requireDiagnostic(t, err)
		if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != anyAttributeTestLoc(root, tc.marker) ||
			d.SpecRef() != tc.spec || len(d.Related()) != 0 || tc.cause != nil && !errors.Is(err, tc.cause) {
			t.Fatalf("%s diagnostic = %s, related=%v, cause=%v", tc.name, d, d.Related(), err)
		}
	}
}
