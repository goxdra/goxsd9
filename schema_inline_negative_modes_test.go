package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func inlineNegativeModeSchema(body, attributes string) string {
	return inlineNegativeSkipSchema(` targetNamespace="urn:root"`, body, inlineNegativeSkipWildcard(attributes))
}

func TestInlineNegativeAttributeModesGraphResolutionPrecedesAdmission(t *testing.T) {
	for _, mode := range []string{"", ` processContents="strict"`, ` processContents="lax"`} {
		root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:include schemaLocation="missing.xsd"/><xs:element name="root"><xs:complexType><xs:sequence/><xs:anyAttribute notNamespace="##local"` + mode + `/></xs:complexType></xs:element></xs:schema>`
		source, err := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
		if err != nil {
			t.Fatalf("root source: %v", err)
		}
		cause := errors.New("resolver unavailable")
		resolver := &discoveryResolver{failures: map[string]error{"missing.xsd": cause}}
		schema, err := ParseSchemaWithPolicy(source, resolver, Strict11)
		assertZeroSchema(t, schema)
		d := requireDiagnostic(t, err)
		if d.Class() != FailureResolution || d.Code() != SourceResolveCode || d.Loc() != anyAttributeTestLoc(root, `<xs:include`) ||
			len(d.Related()) != 0 || d.SpecRef() != "" || !errors.Is(err, cause) || len(resolver.calls) != 1 {
			t.Fatalf("resolution diagnostic = %s, related=%v, calls=%v, cause=%v", d, d.Related(), resolver.calls, err)
		}
	}
}

func TestInlineNegativeAttributeModesExplicitFalseMixed(t *testing.T) {
	for _, mixed := range []string{"false", "0"} {
		for _, mode := range []string{"", ` processContents="strict"`, ` processContents="lax"`} {
			root := inlineNegativeModeSchema(`<xs:sequence/>`, ` notNamespace="##local"`+mode)
			root = strings.Replace(root, `<xs:complexType>`, `<xs:complexType mixed="`+mixed+`">`, 1)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			if err != nil {
				t.Fatalf("mixed=%s mode=%q: %v", mixed, mode, err)
			}
			definition := inlineNegativeModeDefinition(t, schema)
			if definition.Loc() != anyAttributeTestLoc(root, `<xs:complexType`) ||
				inlineNegativeSkipWildcardFacts(t, definition).Loc() != anyAttributeTestLoc(root, `<xs:anyAttribute`) {
				t.Fatalf("mixed=%s mode=%q definition = %#v", mixed, mode, definition)
			}
		}
	}
}

func TestInlineNegativeAttributeModesMixedTrueRemainsUnsupported(t *testing.T) {
	for _, mode := range []string{"", ` processContents="strict"`, ` processContents="lax"`} {
		root := inlineNegativeModeSchema(`<xs:sequence/>`, ` notNamespace="##local"`+mode)
		root = strings.Replace(root, `<xs:complexType>`, `<xs:complexType mixed="true">`, 1)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
		assertZeroSchema(t, schema)
		d := requireDiagnostic(t, err)
		if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != anyAttributeTestLoc(root, `mixed=`) ||
			d.SpecRef() != "xsd11-structures#cSchemaDocument" || len(d.Related()) != 0 || !errors.Is(err, ErrUnsupported) {
			t.Fatalf("mixed=true %q diagnostic = %s, related=%v, cause=%v", mode, d, d.Related(), err)
		}
	}
}

func inlineNegativeModeDefinition(t *testing.T, schema Schema) ComplexTypeDefinition {
	t.Helper()
	components := schema.Components()
	if len(components) != 1 {
		t.Fatalf("components = %v", components)
	}
	declaration, ok := components[0].ElementDeclaration()
	if !ok {
		t.Fatalf("component = %#v", components[0])
	}
	definition, ok := declaration.InlineComplexType()
	if !ok {
		t.Fatalf("inline type = %#v", declaration)
	}
	return definition
}

//nolint:gocognit // The mode and exclusion products verify the same public component contract.
func TestInlineNegativeAttributeModesPublicFacts(t *testing.T) {
	modes := []struct {
		name, attribute, effective string
		present                    bool
	}{
		{"omitted", "", "strict", false},
		{"strict", ` processContents="strict"`, "strict", true},
		{"lax", ` processContents="lax"`, "lax", true},
		{"skip", ` processContents="skip"`, "skip", true},
	}
	exclusions := []struct {
		name, written string
		resolved      []string
	}{
		{"uri", "urn:z urn:a urn:z", []string{"urn:a", "urn:z"}},
		{"local", "##local", []string{""}},
		{"target", "##targetNamespace", []string{"urn:root"}},
		{"mixed", "urn:z ##targetNamespace ##local urn:z", []string{"", "urn:root", "urn:z"}},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, mode := range modes {
			for _, exclusion := range exclusions {
				t.Run(string(policy)+"/"+mode.name+"/"+exclusion.name, func(t *testing.T) {
					root := inlineNegativeModeSchema(`<xs:sequence minOccurs="1" maxOccurs="1"/>`,
						` notNamespace="`+exclusion.written+`"`+mode.attribute)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
					if err != nil {
						t.Fatalf("parse: %v", err)
					}
					definition := inlineNegativeModeDefinition(t, schema)
					sequence, ok := definition.Particle().(SequenceParticle)
					if !ok || !sequence.Occurrences().IsDefault() || sequence.Occurrences().String() != "1/1" ||
						sequence.Loc() != anyAttributeTestLoc(root, `<xs:sequence`) || len(sequence.Particles()) != 0 {
						t.Fatalf("sequence = %#v", definition.Particle())
					}
					if definition.Loc() != anyAttributeTestLoc(root, `<xs:complexType`) || len(definition.AttributeUses()) != 0 {
						t.Fatalf("definition = %#v", definition)
					}
					wildcard := inlineNegativeSkipWildcardFacts(t, definition)
					processLoc := Loc{}
					if mode.present {
						processLoc = anyAttributeTestLoc(root, `processContents=`)
					}
					constraint := wildcard.NamespaceConstraint()
					if wildcard.Loc() != anyAttributeTestLoc(root, `<xs:anyAttribute`) ||
						wildcard.ProcessContents() != mode.effective || wildcard.ProcessContentsLoc() != processLoc ||
						constraint.Variety() != WildcardNamespaceConstraintNot || constraint.LexicalForm() != exclusion.written ||
						constraint.Loc() != anyAttributeTestLoc(root, `notNamespace=`) ||
						wildcard.Namespace() != exclusion.written || wildcard.NamespaceLoc() != constraint.Loc() ||
						!reflect.DeepEqual(constraint.Namespaces(), exclusion.resolved) || wildcard.QNameConstraint().Present() ||
						!wildcard.QNameConstraint().Loc().IsZero() || len(wildcard.QNameConstraint().NamespaceBindings()) != 0 {
						t.Fatalf("wildcard = %#v", wildcard)
					}
					copied := constraint.Namespaces()
					copied[0] = "changed"
					if !reflect.DeepEqual(inlineNegativeSkipWildcardFacts(t, definition).NamespaceConstraint().Namespaces(), exclusion.resolved) {
						t.Fatal("wildcard exclusions exposed mutable storage")
					}
					walked := []ComponentID{}
					if walkErr := schema.Walk(func(component Component) error { walked = append(walked, component.ID()); return nil }); walkErr != nil {
						t.Fatalf("walk: %v", walkErr)
					}
					if !reflect.DeepEqual(walked, []ComponentID{schema.Components()[0].ID()}) {
						t.Fatalf("walk order = %v", walked)
					}
					again, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
					if err != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
						t.Fatalf("repeated parse = %v, %v", again.Components(), err)
					}
				})
			}
		}
	}
}

func TestInlineNegativeAttributeModesConsumersReject(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, mode := range []string{"", ` processContents="strict"`, ` processContents="lax"`, ` processContents="skip"`} {
			root := inlineNegativeModeSchema(`<xs:sequence/>`, ` notNamespace="##local"`+mode)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatalf("parse %s %q: %v", policy, mode, err)
			}
			instanceErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
			instance := requireDiagnostic(t, instanceErr)
			wantRelated := []Loc{
				anyAttributeTestLoc(root, `<xs:element`),
				anyAttributeTestLoc(root, `<xs:complexType`),
				anyAttributeTestLoc(root, `<xs:sequence`),
				anyAttributeTestLoc(root, `<xs:anyAttribute`),
			}
			if instance.Class() != FailureUnsupported || instance.Code() != UnsupportedInstanceValidationCode ||
				instance.Loc() != mustTestLoc(t, "instance.xml", 1, 1) || instance.SpecRef() != instanceValidationSpecRef(XSDVersion11) ||
				!reflect.DeepEqual(instance.Related(), wantRelated) || !errors.Is(instanceErr, errInstanceAttributes) {
				t.Fatalf("validation = %s, related = %v", instance, instance.Related())
			}
			output, generationErr := GenerateGo(schema, "generated")
			generation := requireDiagnostic(t, generationErr)
			if output != nil || generation.Class() != FailureUnsupported || generation.Code() != diagnosticCodegenUnsupported ||
				generation.Loc() != anyAttributeTestLoc(root, `<xs:anyAttribute`) || generation.SpecRef() != schemaElementTypeSpecRef(XSDVersion11) ||
				!reflect.DeepEqual(generation.Related(), wantRelated[:3]) || !errors.Is(generationErr, errCodegenUnsupported) {
				t.Fatalf("generation = %q, %s, related = %v", output, generation, generation.Related())
			}
		}
	}
}

//nolint:gocognit // Each alternate exit must retain its code, source location, and cause.
func TestInlineNegativeAttributeModesAlternateExits(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, mode := range []struct{ name, attribute string }{
			{"omitted", ""}, {"strict", ` processContents="strict"`}, {"lax", ` processContents="lax"`},
		} {
			for _, qname := range []string{` notQName=""`, ` notQName="##defined"`, ` notQName="xs:string"`} {
				root := inlineNegativeModeSchema(`<xs:sequence/>`, ` notNamespace="##local"`+mode.attribute+qname)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != anyAttributeTestLoc(root, `<xs:anyAttribute`) ||
					d.SpecRef() != schemaAnyAttributeSpecRef(XSDVersion11) || len(d.Related()) != 0 || !errors.Is(err, errSchemaAnyAttributeUnsupported) {
					t.Fatalf("%s %s %s diagnostic = %s, related=%v, cause=%v", policy, mode.name, qname, d, d.Related(), err)
				}
			}
		}
	}
	forms := []struct {
		name, body, attributes, marker, code, spec string
		policy                                     LanguagePolicy
		class                                      FailureClass
		cause                                      error
		related                                    string
	}{
		{"strict10 omitted", `<xs:sequence/>`, ` notNamespace="##local"`, `notNamespace=`, UnsupportedSchemaSyntaxCode, "xsd11-structures#cSchemaDocument", Strict10, FailureUnsupported, errLanguagePolicyMismatch, ""},
		{"strict10 lax", `<xs:sequence/>`, ` notNamespace="##local" processContents="lax"`, `notNamespace=`, UnsupportedSchemaSyntaxCode, "xsd11-structures#cSchemaDocument", Strict10, FailureUnsupported, errLanguagePolicyMismatch, ""},
		{"bad process", `<xs:sequence/>`, ` notNamespace="##local" processContents="bogus"`, `processContents=`, invalidSchemaCompositionCode, "", Strict11, FailureInvalid, nil, ""},
		{"duplicate process", `<xs:sequence/>`, ` notNamespace="##local" processContents="strict" processContents="lax"`, `processContents="lax"`, InvalidXMLSyntaxCode, "", Strict11, FailureInvalid, nil, ""},
		{"empty negative", `<xs:sequence/>`, ` notNamespace=" " processContents="lax"`, `notNamespace=`, invalidSchemaCompositionCode, "", Strict11, FailureInvalid, nil, ""},
		{"invalid negative", `<xs:sequence/>`, ` notNamespace="##bogus" processContents="strict"`, `notNamespace=`, invalidSchemaCompositionCode, "", Strict11, FailureInvalid, nil, ""},
		{"conflicting namespace", `<xs:sequence/>`, ` notNamespace="##local" namespace="##any" processContents="lax"`, `namespace=`, invalidSchemaCompositionCode, "", Strict11, FailureInvalid, nil, ""},
		{"duplicate negative", `<xs:sequence/>`, ` notNamespace="##local" notNamespace="urn:z" processContents="strict"`, `notNamespace="urn:z"`, InvalidXMLSyntaxCode, "", Strict11, FailureInvalid, nil, ""},
		{"invalid occurrence", `<xs:sequence minOccurs="-1"/>`, ` notNamespace="##local" processContents="lax"`, `minOccurs=`, invalidSchemaCompositionCode, "xsd11-datatypes#nonNegativeInteger", Strict11, FailureInvalid, nil, ""},
		{"optional occurrence", `<xs:sequence minOccurs="0"/>`, ` notNamespace="##local" processContents="lax"`, `<xs:anyAttribute`, UnsupportedSchemaSyntaxCode, schemaAnyAttributeXSD11SpecRef, Strict11, FailureUnsupported, errSchemaAnyAttributeUnsupported, ""},
		{"repeat occurrence", `<xs:sequence maxOccurs="2"/>`, ` notNamespace="##local" processContents="strict"`, `<xs:anyAttribute`, UnsupportedSchemaSyntaxCode, schemaAnyAttributeXSD11SpecRef, Strict11, FailureUnsupported, errSchemaAnyAttributeUnsupported, ""},
		{"nonempty sequence", `<xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence>`, ` notNamespace="##local" processContents="lax"`, `<xs:anyAttribute`, UnsupportedSchemaSyntaxCode, schemaAnyAttributeXSD11SpecRef, Strict11, FailureUnsupported, errSchemaAnyAttributeUnsupported, ""},
		{"attribute use", `<xs:sequence/><xs:attribute name="a" type="xs:string"/>`, ` notNamespace="##local" processContents="strict"`, `<xs:anyAttribute`, UnsupportedSchemaSyntaxCode, schemaAnyAttributeXSD11SpecRef, Strict11, FailureUnsupported, errSchemaAnyAttributeUnsupported, ""},
		{"choice", `<xs:choice/>`, ` notNamespace="##local" processContents="lax"`, `<xs:anyAttribute`, UnsupportedSchemaSyntaxCode, schemaAnyAttributeXSD11SpecRef, Strict11, FailureUnsupported, errSchemaAnyAttributeUnsupported, ""},
		{"qname membership before owner", `<xs:choice/>`, ` notNamespace="` + testXSDNamespace + `" processContents="strict" notQName="xs:string"`, `notQName=`, invalidSchemaCompositionCode, schemaWildcardQNameNamespaceXSD11SpecRef, Strict11, FailureInvalid, errSchemaWildcardQNameNamespace, `notNamespace=`},
	}
	for _, tc := range forms {
		t.Run(tc.name, func(t *testing.T) {
			root := inlineNegativeModeSchema(tc.body, tc.attributes)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, tc.policy)
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			var related []Loc
			if tc.related != "" {
				related = []Loc{anyAttributeTestLoc(root, tc.related)}
			}
			if d.Class() != tc.class || d.Code() != tc.code || d.Loc() != anyAttributeTestLoc(root, tc.marker) ||
				d.SpecRef() != tc.spec || !reflect.DeepEqual(d.Related(), related) || tc.cause != nil && !errors.Is(err, tc.cause) {
				t.Fatalf("diagnostic = %s, related=%v, cause=%v", d, d.Related(), err)
			}
		})
	}
}

//nolint:gocognit // Exercise chameleon adoption, imports, cycles, and ordered public walks together.
func TestInlineNegativeAttributeModesGraphProvenance(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:include schemaLocation="included.xsd"/><xs:include schemaLocation="included.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`
	included := inlineNegativeSkipSchema("", `<xs:sequence/>`, inlineNegativeSkipWildcard(` notNamespace="##targetNamespace ##local"`))
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:import namespace="urn:root" schemaLocation="root.xsd"/><xs:element name="other"><xs:complexType><xs:sequence/><xs:anyAttribute notNamespace="urn:z ##targetNamespace" processContents="lax"/></xs:complexType></xs:element></xs:schema>`
	fixtures := map[string]discoveryFixture{
		"root.xsd": {id: "root.xsd", contents: root}, "included.xsd": {id: "included.xsd", contents: included}, "other.xsd": {id: "other.xsd", contents: other},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		var previous []Component
		for run := 0; run < 2; run++ {
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy)
			if err != nil {
				t.Fatalf("graph %s run %d: %v", policy, run, err)
			}
			documents := schema.Documents()
			components := schema.Components()
			if len(documents) != 3 || documents[0].Source() != "root.xsd" || documents[1].Source() != "included.xsd" || documents[2].Source() != "other.xsd" || len(components) != 2 {
				t.Fatalf("graph documents/components = %v / %v", documents, components)
			}
			walked := []ComponentID{}
			if err := schema.Walk(func(component Component) error { walked = append(walked, component.ID()); return nil }); err != nil {
				t.Fatalf("walk: %v", err)
			}
			if !reflect.DeepEqual(walked, []ComponentID{components[0].ID(), components[1].ID()}) {
				t.Fatalf("walk order = %v", walked)
			}
			for index, want := range []struct {
				source   SourceID
				contents string
				name     QName
				mode     string
				loc      Loc
				resolved []string
			}{
				{"included.xsd", included, mustTestQName(t, "urn:root", "root"), "strict", Loc{}, []string{"", "urn:root"}},
				{"other.xsd", other, mustTestQName(t, "urn:other", "other"), "lax", anyAttributeTestLocForSource("other.xsd", other, `processContents=`), []string{"urn:other", "urn:z"}},
			} {
				component := components[index]
				declaration, ok := component.ElementDeclaration()
				if !ok || component.Name() != want.name || component.Document() != want.source || component.Loc() != anyAttributeTestLocForSource(want.source, want.contents, `<xs:element`) {
					t.Fatalf("component %d = %#v", index, component)
				}
				definition, ok := declaration.InlineComplexType()
				if !ok || definition.Loc() != anyAttributeTestLocForSource(want.source, want.contents, `<xs:complexType`) {
					t.Fatalf("inline %d = %#v", index, definition)
				}
				wildcard := inlineNegativeSkipWildcardFacts(t, definition)
				if wildcard.ProcessContents() != want.mode || wildcard.ProcessContentsLoc() != want.loc ||
					wildcard.Loc() != anyAttributeTestLocForSource(want.source, want.contents, `<xs:anyAttribute`) ||
					wildcard.NamespaceLoc() != anyAttributeTestLocForSource(want.source, want.contents, `notNamespace=`) ||
					!reflect.DeepEqual(wildcard.NamespaceConstraint().Namespaces(), want.resolved) {
					t.Fatalf("wildcard %d = %#v", index, wildcard)
				}
			}
			if run > 0 && !reflect.DeepEqual(previous, components) {
				t.Fatal("repeated graph changed order or component facts")
			}
			previous = components
		}
	}
}

//nolint:gocognit // Keep each excluded owner and reference shape in one mode matrix.
func TestInlineNegativeAttributeModesOwnerAndReferenceBoundaries(t *testing.T) {
	for _, mode := range []string{"", ` processContents="strict"`, ` processContents="lax"`} {
		wildcard := inlineNegativeSkipWildcard(` notNamespace="##local"` + mode)
		for _, tc := range []struct {
			name, root, marker, spec string
			cause                    error
		}{
			{"inline without sequence", inlineNegativeSkipSchema(` targetNamespace="urn:root"`, "", wildcard), `<xs:anyAttribute`, schemaAnyAttributeXSD11SpecRef, errSchemaAnyAttributeUnsupported},
			{"local inline", `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:complexType name="Owner"><xs:sequence><xs:element name="local"><xs:complexType><xs:sequence/>` + wildcard + `</xs:complexType></xs:element></xs:sequence></xs:complexType></xs:schema>`, `<xs:anyAttribute`, schemaAnyAttributeXSD11SpecRef, errSchemaAnyAttributeUnsupported},
			{"attribute ref", inlineNegativeSkipSchema(` xmlns:t="urn:root" targetNamespace="urn:root"`, `<xs:sequence/><xs:attribute ref="t:missing"/>`, wildcard), `<xs:anyAttribute`, schemaAnyAttributeXSD11SpecRef, errSchemaAnyAttributeUnsupported},
			{"element ref", inlineNegativeSkipSchema(` xmlns:t="urn:root" targetNamespace="urn:root"`, `<xs:sequence><xs:element ref="t:missing"/></xs:sequence>`, wildcard), `<xs:anyAttribute`, schemaAnyAttributeXSD11SpecRef, errSchemaAnyAttributeUnsupported},
			{"named extension", `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root"><xs:complexType name="Base"/><xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Base"><xs:sequence/>` + wildcard + `</xs:extension></xs:complexContent></xs:complexType></xs:schema>`, `<xs:anyAttribute`, schemaComplexTypeExtensionSpecRef(XSDVersion11), errSchemaAnyAttributeUnsupported},
		} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				schema, err := discoverTestSchemaWithPolicy(t, tc.root, nil, Strict11)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != anyAttributeTestLoc(tc.root, tc.marker) ||
					d.SpecRef() != tc.spec || len(d.Related()) != 0 || !errors.Is(err, tc.cause) {
					t.Fatalf("diagnostic = %s, related=%v, cause=%v", d, d.Related(), err)
				}
			})
		}
		root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:complexType name="Named"><xs:sequence/>` + wildcard + `</xs:complexType></xs:schema>`
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
		if err != nil {
			t.Fatalf("preexisting direct named support %q: %v", mode, err)
		}
		components := schema.Components()
		if len(components) != 1 {
			t.Fatalf("named components = %v", components)
		}
		definition, ok := components[0].ComplexType()
		if !ok {
			t.Fatalf("named owner = %#v", components[0])
		}
		attribute := inlineNegativeSkipWildcardFacts(t, definition)
		wantMode := "strict"
		if mode == ` processContents="lax"` {
			wantMode = "lax"
		}
		if attribute.ProcessContents() != wantMode ||
			attribute.Loc() != anyAttributeTestLoc(root, `<xs:anyAttribute`) {
			t.Fatalf("named direct wildcard = %#v", attribute)
		}
	}
}

func TestInlineNegativeAttributeModesExtensionDependencyPrecedence(t *testing.T) {
	for _, mode := range []string{"", ` processContents="strict"`, ` processContents="lax"`} {
		for _, tc := range []struct {
			name, baseRef, base, marker, related string
			class                                FailureClass
			code                                 string
			cause                                error
		}{
			{"missing", "t:Missing", "", `base="t:Missing"`, "", FailureInvalid, invalidSchemaCompositionCode, errSchemaComplexTypeBaseUnresolved},
			{"final", "t:Base", `<xs:complexType name="Base" final="extension"/>`, `base="t:Base"`, `final="extension"`, FailureInvalid, invalidSchemaCompositionCode, errSchemaComplexTypeBaseUnsupported},
			{"valid excluded derivation", "t:Base", `<xs:complexType name="Base"/>`, `<xs:anyAttribute`, "", FailureUnsupported, UnsupportedSchemaSyntaxCode, errSchemaAnyAttributeUnsupported},
		} {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				owner := `<xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="` + tc.baseRef + `"><xs:sequence/><xs:anyAttribute notNamespace="##local"` + mode + `/></xs:extension></xs:complexContent></xs:complexType></xs:element>`
				root := namedRestrictionSchema("1.1", "urn:t", owner+tc.base)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				var related []Loc
				if tc.related != "" {
					related = []Loc{anyAttributeTestLoc(root, tc.related)}
				}
				if d.Class() != tc.class || d.Code() != tc.code || d.Loc() != anyAttributeTestLoc(root, tc.marker) ||
					!reflect.DeepEqual(d.Related(), related) || d.SpecRef() != schemaComplexTypeExtensionSpecRef(XSDVersion11) || !errors.Is(err, tc.cause) {
					t.Fatalf("dependency diagnostic = %s, related=%v, cause=%v", d, d.Related(), err)
				}
			})
		}
	}
}
