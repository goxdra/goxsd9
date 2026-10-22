package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func sequenceInheritableUses() string {
	return `<xs:attribute name="first" type="xs:ID" inheritable="false" use="required"/>` +
		`<xs:attribute name="second" type="t:NamedID" inheritable=" 0 "/>` +
		`<xs:attribute name="gone" type="xs:ID" inheritable="false" use="prohibited"/>`
}

//nolint:gocognit // Compare ordered public facts, locations, and copied views.
func TestSequenceExtensionFalseInheritableOrderedFacts(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, explicit := range []bool{false, true} {
			uses := sequenceInheritableUses()
			if !explicit {
				uses = strings.ReplaceAll(uses, ` inheritable="false"`, "")
				uses = strings.ReplaceAll(uses, ` inheritable=" 0 "`, "")
			}
			root := sequenceAttributeExtensionSchema("1.0", ` minOccurs="2" maxOccurs="5"`, `<xs:element ref="t:target" minOccurs="0" maxOccurs="3"/>`, uses, ``)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatalf("%s explicit=%t: %v", policy, explicit, err)
			}
			derived := requireTestComplexTypeDefinition(t, schema.Components()[1], "Derived")
			if derived.Derivation() != ComplexTypeDerivationExtension || derived.DerivationLoc() != complexContentTestLoc(t, root, `<xs:extension`) || derived.BaseLoc() != complexContentTestLoc(t, root, `base="t:Base"`) {
				t.Fatalf("derivation facts = %#v", derived)
			}
			base, ok := derived.BaseReference()
			baseID, hasID := base.ComponentID()
			if !ok || !hasID || baseID != schema.Components()[3].ID() {
				t.Fatalf("base reference = %#v", base)
			}
			sequence, ok := derived.Particle().(SequenceParticle)
			if !ok || sequence.Occurrences().String() != "2/5" || sequence.Loc() != complexContentTestLoc(t, root, `<xs:sequence`) {
				t.Fatalf("sequence = %#v", derived.Particle())
			}
			terms := sequence.Particles()
			if len(terms) != 1 {
				t.Fatalf("terms = %#v", terms)
			}
			ref, ok := terms[0].(ElementReferenceParticle)
			if !ok || ref.Occurrences().String() != "0/3" || ref.RefLoc() != complexContentTestLoc(t, root, `ref="t:target"`) || ref.TargetID() != schema.Components()[2].ID() {
				t.Fatalf("terms = %#v", terms)
			}
			attributeUses := derived.AttributeUses()
			if len(attributeUses) != 2 || attributeUses[0].Name().Local() != "first" || attributeUses[1].Name().Local() != "second" {
				t.Fatalf("ordered uses = %#v", attributeUses)
			}
			for index, name := range []string{"first", "second"} {
				use, ok := attributeUses[index].(LocalAttributeUse)
				if !ok || use.Name() != mustTestQName(t, "urn:root", name) || use.Loc() != complexContentTestLoc(t, root, `<xs:attribute name="`+name+`"`) ||
					use.NameLoc() != complexContentTestLoc(t, root, `name="`+name+`"`) {
					t.Fatalf("use %d facts = %#v", index, attributeUses[index])
				}
				if index == 0 && (use.DeclaredType() != mustTestQName(t, testXSDNamespace, "ID") || use.TypeLoc() != complexContentTestLoc(t, root, `type="xs:ID"`)) {
					t.Fatalf("built-in ID = %#v", use)
				}
				if index == 1 {
					id, hasID := use.TypeID()
					if use.DeclaredType() != mustTestQName(t, "urn:root", "NamedID") || use.TypeLoc() != complexContentTestLoc(t, root, `type="t:NamedID"`) || !hasID || id != schema.Components()[4].ID() {
						t.Fatalf("named ID = %#v", use)
					}
				}
				if index == 0 && (use.Use() != AttributeUseRequired || use.UseLoc() != complexContentTestLoc(t, root, `use="required"`)) {
					t.Fatalf("required use = %#v", use)
				}
				if index == 1 && (use.Use() != AttributeUseOptional || !use.UseLoc().IsZero()) {
					t.Fatalf("optional use = %#v", use)
				}
			}
			attributeUses[0] = nil
			terms[0] = nil
			if derived.AttributeUses()[0] == nil || sequence.Particles()[0] == nil {
				t.Fatal("returned slices changed completed facts")
			}
		}
	}
}

//nolint:gocognit // Exercise graph visibility and identity under the admitted spelling.
func TestSequenceExtensionFalseInheritableGraph(t *testing.T) {
	for _, graph := range []string{"forward", "included", "imported", "chameleon"} {
		for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
			t.Run(graph+"/"+string(policy), func(t *testing.T) {
				fixture := emptyComplexContentExtensionGraph(t, "1.0", graph)
				fixture.root = strings.Replace(fixture.root, `<xs:extension base="`+fixture.baseLexical+`"/>`, `<xs:extension base="`+fixture.baseLexical+`"><xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence><xs:attribute name="id" type="xs:ID" inheritable="false"/></xs:extension>`, 1)
				schema, err := discoverTestSchemaWithPolicy(t, fixture.root, fixture.fixtures, policy)
				if err != nil {
					t.Fatalf("parse graph: %v", err)
				}
				derived := requireTestComplexTypeDefinition(t, schema.Components()[0], "Derived")
				base, ok := derived.BaseReference()
				baseID, hasID := base.ComponentID()
				if !ok || !hasID || baseID.Source() != fixture.baseSource || derived.BaseLoc() != complexContentTestLoc(t, fixture.root, `base="`+fixture.baseLexical+`"`) {
					t.Fatalf("base provenance = %#v", base)
				}
				uses := derived.AttributeUses()
				if len(uses) != 1 || uses[0].Name().Local() != "id" || uses[0].Loc() != complexContentTestLoc(t, fixture.root, `<xs:attribute name="id"`) {
					t.Fatalf("uses = %#v", uses)
				}
			})
		}
	}
}

//nolint:gocognit // Same-boundary exits must retain their established diagnostics.
func TestSequenceExtensionFalseInheritableAlternateExits(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, tc := range []struct {
			name, members, uses, base, marker, related, code, spec string
			class                                                  FailureClass
			cause                                                  error
		}{
			{"missing child", `<xs:element ref="t:Missing"/>`, sequenceInheritableUses(), "", `ref="t:Missing"`, "", diagnosticSchemaElementReferenceUnresolvedCode, schemaElementReferenceSpecRef(XSDVersion11), FailureInvalid, errSchemaElementReferenceUnresolved},
			{"child before attribute", `<xs:element ref="t:Missing"/>`, `<xs:attribute name="first" type="t:Missing" inheritable="false"/>`, "", `ref="t:Missing"`, "", diagnosticSchemaElementReferenceUnresolvedCode, schemaElementReferenceSpecRef(XSDVersion11), FailureInvalid, errSchemaElementReferenceUnresolved},
			{"missing attribute type", `<xs:element ref="t:target"/>`, `<xs:attribute name="first" type="t:Missing" inheritable="false"/>`, "", `type="t:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(XSDVersion11), FailureInvalid, errSchemaAttributeTypeUnresolved},
			{"first use before later use", `<xs:element ref="t:target"/>`, `<xs:attribute name="first" type="t:MissingOne" inheritable="false"/><xs:attribute name="second" type="t:MissingTwo"/>`, "", `type="t:MissingOne"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(XSDVersion11), FailureInvalid, errSchemaAttributeTypeUnresolved},
			{"attribute before base", `<xs:element ref="t:target"/>`, `<xs:attribute name="first" type="t:Missing" inheritable="false"/>`, "Missing", `type="t:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(XSDVersion11), FailureInvalid, errSchemaAttributeTypeUnresolved},
			{"unsupported attribute type", `<xs:element ref="t:target"/>`, `<xs:attribute name="first" type="xs:QName" inheritable="false"/>`, "", `type="xs:QName"`, "", UnsupportedSchemaSyntaxCode, schemaAttributeTypeSpecRef(XSDVersion11), FailureUnsupported, errSchemaAttributeUseUnsupported},
			{"duplicate use", `<xs:element ref="t:target"/>`, `<xs:attribute name="first" type="xs:ID" inheritable="false"/><xs:attribute name="first" type="xs:boolean"/>`, "", `<xs:attribute name="first" type="xs:boolean"`, `<xs:attribute name="first" type="xs:ID"`, diagnosticSchemaAttributeUseDuplicateCode, schemaAttributeUseSpecRef(XSDVersion11), FailureInvalid, errSchemaAttributeUseDuplicate},
			{"unresolved base", `<xs:element ref="t:target"/>`, sequenceInheritableUses(), "Missing", `base="t:Missing"`, "", invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(XSDVersion11), FailureInvalid, errSchemaComplexTypeBaseUnresolved},
			{"base cycle", `<xs:element ref="t:target"/>`, sequenceInheritableUses(), "Derived", `base="t:Derived"`, "", invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(XSDVersion11), FailureInvalid, errSchemaComplexTypeBaseCycle},
			{"nonempty base", `<xs:element ref="t:target"/>`, sequenceInheritableUses(), "nonempty", `base="t:Base"`, `<xs:sequence><xs:element name="baseValue"`, UnsupportedSchemaSyntaxCode, schemaComplexTypeExtensionSpecRef(XSDVersion11), FailureUnsupported, errSchemaComplexTypeBaseNonEmpty},
		} {
			t.Run(string(policy)+"/"+tc.name, func(t *testing.T) {
				base := ""
				if tc.base == "nonempty" {
					base = `<xs:sequence><xs:element name="baseValue" type="xs:integer"/></xs:sequence>`
				}
				root := sequenceAttributeExtensionSchema("1.0", "", tc.members, tc.uses, base)
				if tc.base == "Missing" || tc.base == "Derived" {
					root = strings.Replace(root, `base="t:Base"`, `base="t:`+tc.base+`"`, 1)
				}
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				wantRelated := []Loc(nil)
				if tc.related != "" {
					wantRelated = []Loc{complexContentTestLoc(t, root, tc.related)}
				}
				if tc.name == "nonempty base" {
					wantRelated = []Loc{complexContentTestLoc(t, root, `<xs:complexType name="Base"`), complexContentTestLoc(t, root, tc.related)}
				}
				if d.Class() != tc.class || d.Code() != tc.code || d.Loc() != complexContentTestLoc(t, root, tc.marker) || d.SpecRef() != tc.spec || !reflect.DeepEqual(d.Related(), wantRelated) || !errors.Is(err, tc.cause) {
					t.Fatalf("alternate exit = %v, related=%v", err, d.Related())
				}
			})
		}
	}
}

//nolint:gocognit // Pair each effective-use form with both consumer boundaries.
func TestSequenceExtensionFalseInheritableConsumers(t *testing.T) {
	for _, uses := range []string{`<xs:attribute name="id" type="xs:ID" inheritable="false"/>`, `<xs:attribute name="gone" type="xs:ID" inheritable="false" use="prohibited"/>`} {
		root := sequenceAttributeExtensionSchema("1.1", ` minOccurs="0" maxOccurs="0"`, `<xs:element ref="t:target"/>`, uses, "")
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		derived := requireTestComplexTypeDefinition(t, schema.Components()[1], "Derived")
		if derived.Particle() != nil {
			t.Fatalf("0/0 sequence = %#v", derived.Particle())
		}
		if strings.Contains(uses, `use="prohibited"`) && len(derived.AttributeUses()) != 0 {
			t.Fatalf("prohibited use = %#v", derived.AttributeUses())
		}
		want := complexContentTestLoc(t, root, `<xs:extension`)
		if len(derived.AttributeUses()) != 0 {
			want = complexContentTestLoc(t, root, `<xs:attribute name="id"`)
		}
		validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
		validation := requireDiagnostic(t, validationErr)
		if validation.Class() != FailureUnsupported || validation.Code() != UnsupportedInstanceValidationCode || validation.Loc() != want || !errors.Is(validationErr, ErrUnsupported) {
			t.Fatalf("validation = %v", validationErr)
		}
		output, generationErr := GenerateGo(schema, "generated")
		generation := requireDiagnostic(t, generationErr)
		if output != nil || generation.Class() != FailureUnsupported || generation.Code() != diagnosticCodegenUnsupported || generation.Loc() != want || !errors.Is(generationErr, ErrUnsupported) {
			t.Fatalf("generation = %q/%v", output, generationErr)
		}
	}
}
