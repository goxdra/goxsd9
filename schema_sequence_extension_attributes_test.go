package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func sequenceAttributeExtensionSchema(version, outer, members, uses, base string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" version="` + version + `" attributeFormDefault="qualified">
  <xs:element name="root" type="t:Derived"/>
  <xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Base"><xs:sequence` + outer + `>` + members + `</xs:sequence>` + uses + `</xs:extension></xs:complexContent></xs:complexType>
  <xs:element name="target" type="xs:integer"/>
  <xs:complexType name="Base">` + base + `</xs:complexType>
  <xs:simpleType name="NamedID"><xs:restriction base="xs:ID"/></xs:simpleType>
</xs:schema>`
}

//nolint:gocognit // Keep paired policy and immutable public fact assertions together.
func TestSequenceExtensionAttributePublicFacts(t *testing.T) {
	profiles := []struct {
		name, version string
		policy        LanguagePolicy
	}{
		{"compat10", "1.0", Compatibility}, {"compat11", "1.1", Compatibility},
		{"strict10", "1.0", Strict10}, {"strict10-label11", "1.1", Strict10},
		{"strict11", "1.1", Strict11}, {"strict11-label10", "1.0", Strict11},
	}
	members := `<xs:element name="local" type="xs:integer" minOccurs="2" maxOccurs="3"/><xs:element ref="t:target" minOccurs="0" maxOccurs="4"/>`
	uses := `<xs:attribute name="id" type="t:NamedID" use="required"/><xs:attribute name="flag" type="xs:boolean"/><xs:attribute name="gone" type="xs:decimal" use="prohibited"/>`
	base := `<xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent>`
	for _, profile := range profiles {
		t.Run(profile.name, func(t *testing.T) {
			root := sequenceAttributeExtensionSchema(profile.version, ` minOccurs="2" maxOccurs="5"`, members, uses, base)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			components := schema.Components()
			if len(components) != 5 || components[0].Name().Local() != "root" || components[1].Name().Local() != "Derived" || components[2].Name().Local() != "target" || components[3].Name().Local() != "Base" {
				t.Fatalf("component order: %v", components)
			}
			derived := requireTestComplexTypeDefinition(t, components[1], "Derived")
			if derived.Derivation() != ComplexTypeDerivationExtension || derived.DerivationLoc() != complexContentTestLoc(t, root, `<xs:extension`) || derived.BaseLoc() != complexContentTestLoc(t, root, `base="t:Base"`) {
				t.Fatalf("derivation: %v", derived)
			}
			baseRef, ok := derived.BaseReference()
			baseID, hasID := baseRef.ComponentID()
			if !ok || !hasID || baseID != components[3].ID() {
				t.Fatalf("base reference: %v", baseRef)
			}
			wildcard, ok := derived.AnyAttribute()
			if !ok || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" || wildcard.Loc() != complexContentTestLoc(t, root, `<xs:anyAttribute`) || wildcard.NamespaceLoc() != complexContentTestLoc(t, root, `namespace="##other"`) || wildcard.ProcessContentsLoc() != complexContentTestLoc(t, root, `processContents="lax"`) {
				t.Fatalf("inherited wildcard: %v", wildcard)
			}
			sequence, ok := derived.Particle().(SequenceParticle)
			if !ok || sequence.Occurrences().String() != "2/5" || sequence.Loc() != complexContentTestLoc(t, root, `<xs:sequence`) {
				t.Fatalf("sequence: %v", derived.Particle())
			}
			terms := sequence.Particles()
			if len(terms) != 2 || terms[0].Occurrences().String() != "2/3" || terms[1].Occurrences().String() != "0/4" {
				t.Fatalf("terms: %v", terms)
			}
			ref, ok := terms[1].(ElementReferenceParticle)
			if !ok || ref.TargetID() != components[2].ID() || ref.RefLoc() != complexContentTestLoc(t, root, `ref="t:target"`) {
				t.Fatalf("element ref: %v", terms[1])
			}
			attributeUses := derived.AttributeUses()
			if len(attributeUses) != 2 {
				t.Fatalf("uses: %v", attributeUses)
			}
			id, ok := attributeUses[0].(LocalAttributeUse)
			if !ok || id.Name() != mustTestQName(t, "urn:root", "id") || id.Use() != AttributeUseRequired || id.Loc() != complexContentTestLoc(t, root, `<xs:attribute name="id"`) || id.NameLoc() != complexContentTestLoc(t, root, `name="id"`) || id.UseLoc() != complexContentTestLoc(t, root, `use="required"`) || id.TypeLoc() != complexContentTestLoc(t, root, `type="t:NamedID"`) {
				t.Fatalf("ID use: %v", attributeUses[0])
			}
			idType, named := id.TypeID()
			if !named || idType != components[4].ID() {
				t.Fatalf("named ID: %v/%v", idType, named)
			}
			flag, ok := attributeUses[1].(LocalAttributeUse)
			if !ok || flag.Name().Local() != "flag" || flag.Use() != AttributeUseOptional || !flag.UseLoc().IsZero() {
				t.Fatalf("flag: %v", attributeUses[1])
			}
			attributeUses[0] = nil
			terms[0] = nil
			secondSequence, ok := derived.Particle().(SequenceParticle)
			if !ok || derived.AttributeUses()[0].Name().Local() != "id" || len(secondSequence.Particles()) != 2 {
				t.Fatal("view copy mutated schema")
			}
			again, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
				t.Fatalf("repeated parse: %v", err)
			}
		})
	}
}

//nolint:gocognit // Pair zero-range omission with both unsupported consumers.
func TestSequenceExtensionZeroAndProhibitedUse(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		edition := "xsd11"
		if policy == Strict10 {
			edition = "xsd10"
		}
		root := sequenceAttributeExtensionSchema("1.0", ` minOccurs="0" maxOccurs="0"`, `<xs:element ref="t:target"/>`, `<xs:attribute name="gone" type="xs:ID" use="prohibited"/>`, ``)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
		if err != nil {
			t.Fatalf("%s: %v", policy, err)
		}
		derived := requireTestComplexTypeDefinition(t, schema.Components()[1], "Derived")
		if derived.Particle() != nil || len(derived.AttributeUses()) != 0 || derived.Derivation() != ComplexTypeDerivationExtension {
			t.Fatalf("%s zero facts: %v", policy, derived)
		}
		validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
		if validationErr == nil || !errors.Is(validationErr, ErrUnsupported) {
			t.Fatalf("%s validation: %v", policy, validationErr)
		}
		validationDiagnostic := requireDiagnostic(t, validationErr)
		if validationDiagnostic.Class() != FailureUnsupported || validationDiagnostic.Code() != UnsupportedInstanceValidationCode || validationDiagnostic.Loc() != complexContentTestLoc(t, root, `<xs:extension`) || validationDiagnostic.SpecRef() != edition+"-structures#cvc-elt" || !errors.Is(validationErr, errInstanceComplexContentExtension) {
			t.Fatalf("%s validation diagnostic: %v", policy, validationErr)
		}
		output, generationErr := GenerateGo(schema, "generated")
		if generationErr == nil || output != nil || !errors.Is(generationErr, ErrUnsupported) {
			t.Fatalf("%s generation: %v", policy, generationErr)
		}
		generationDiagnostic := requireDiagnostic(t, generationErr)
		if generationDiagnostic.Class() != FailureUnsupported || generationDiagnostic.Code() != diagnosticCodegenUnsupported || generationDiagnostic.Loc() != complexContentTestLoc(t, root, `<xs:extension`) || generationDiagnostic.SpecRef() != edition+"-structures#cParticles" || !errors.Is(generationErr, errCodegenUnsupported) {
			t.Fatalf("%s generation diagnostic: %v", policy, generationErr)
		}
	}
}

//nolint:gocognit // Keep each policy and graph visibility fact paired.
func TestSequenceExtensionAttributeGraphVisibilityAndCycle(t *testing.T) {
	for _, graph := range []string{"forward", "included", "imported", "chameleon"} {
		for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
			t.Run(graph+"/"+string(policy), func(t *testing.T) {
				fixture := emptyComplexContentExtensionGraph(t, "1.0", graph)
				fixture.root = strings.Replace(fixture.root, `<xs:extension base="`+fixture.baseLexical+`"/>`, `<xs:extension base="`+fixture.baseLexical+`"><xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence><xs:attribute name="id" type="xs:ID" use="required"/></xs:extension>`, 1)
				schema, err := discoverTestSchemaWithPolicy(t, fixture.root, fixture.fixtures, policy)
				if err != nil {
					t.Fatalf("parse: %v", err)
				}
				derived := requireTestComplexTypeDefinition(t, schema.Components()[0], "Derived")
				baseRef, ok := derived.BaseReference()
				baseID, hasID := baseRef.ComponentID()
				if !ok || !hasID || baseID.Source() != fixture.baseSource || derived.BaseLoc() != complexContentTestLoc(t, fixture.root, `base="`+fixture.baseLexical+`"`) {
					t.Fatalf("base reference: %v", baseRef)
				}
				wildcard, ok := derived.AnyAttribute()
				if !ok || wildcard.Loc() != complexContentTestSourceLoc(t, fixture.baseSource, fixture.baseContents, `<xs:anyAttribute`) {
					t.Fatalf("wildcard: %v", wildcard)
				}
				if _, ok := derived.Particle().(SequenceParticle); !ok {
					t.Fatalf("particle: %T", derived.Particle())
				}
				uses := derived.AttributeUses()
				if len(uses) != 1 || uses[0].Name().Local() != "id" || uses[0].Loc() != complexContentTestLoc(t, fixture.root, `<xs:attribute name="id"`) {
					t.Fatalf("uses: %v", uses)
				}
			})
		}
	}
}

func TestSequenceExtensionAttributeReferenceAndZeroSemanticGate(t *testing.T) {
	root := sequenceAttributeExtensionSchema("1.1", "", `<xs:element ref="t:target"/>`, `<xs:attribute ref="t:global" use="required"/>`, ``)
	root = strings.Replace(root, `<xs:simpleType name="NamedID">`, `<xs:attribute name="global" type="xs:boolean"/><xs:simpleType name="NamedID">`, 1)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("parse attribute ref: %v", err)
	}
	derived := requireTestComplexTypeDefinition(t, schema.Components()[1], "Derived")
	uses := derived.AttributeUses()
	if len(uses) != 1 {
		t.Fatalf("reference uses: %v", uses)
	}
	ref, ok := uses[0].(AttributeReferenceUse)
	if !ok || ref.RefLoc() != complexContentTestLoc(t, root, `ref="t:global"`) || ref.TargetID() != schema.Components()[4].ID() || ref.Use() != AttributeUseRequired {
		t.Fatalf("reference use: %v", uses)
	}
	for _, test := range []struct {
		name, member, uses, code, marker, spec string
		cause                                  error
	}{
		{"bad element ref", `<xs:element ref="t:Missing"/>`, `<xs:attribute name="id" type="xs:ID"/>`, diagnosticSchemaElementReferenceUnresolvedCode, `ref="t:Missing"`, schemaElementReferenceSpecRef(XSDVersion11), errSchemaElementReferenceUnresolved},
		{"bad attribute type", `<xs:element ref="t:target"/>`, `<xs:attribute name="id" type="t:Missing"/>`, diagnosticSchemaAttributeTypeUnresolvedCode, `type="t:Missing"`, schemaAttributeTypeSpecRef(XSDVersion11), errSchemaAttributeTypeUnresolved},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := sequenceAttributeExtensionSchema("1.1", ` minOccurs="0" maxOccurs="0"`, test.member, test.uses, ``)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			if err == nil {
				t.Fatal("invalid zero sequence passed")
			}
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			if d.Class() != FailureInvalid || d.Code() != test.code || d.Loc() != complexContentTestLoc(t, root, test.marker) || d.SpecRef() != test.spec || !errors.Is(err, test.cause) {
				t.Fatalf("diagnostic: %v, related=%v", err, d.Related())
			}
		})
	}
}

//nolint:gocognit // The table covers alternate located exits at one boundary.
func TestSequenceExtensionAttributeFailureExits(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		version := XSDVersion11
		if policy == Strict10 {
			version = XSDVersion10
		}
		for _, test := range []struct {
			name, base, members, uses, marker, related, code, spec string
			class                                                  FailureClass
			cause                                                  error
		}{
			{"unresolved base", "Missing", `<xs:element name="value" type="xs:integer"/>`, `<xs:attribute name="flag" type="xs:boolean"/>`, `base="t:Missing"`, "", invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(version), FailureInvalid, errSchemaComplexTypeBaseUnresolved},
			{"wrong kind base", "NamedID", `<xs:element name="value" type="xs:integer"/>`, `<xs:attribute name="flag" type="xs:boolean"/>`, `base="t:NamedID"`, `<xs:simpleType name="NamedID"`, invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(version), FailureInvalid, errSchemaComplexTypeBaseWrongKind},
			{"duplicate use", "Base", `<xs:element name="value" type="xs:integer"/>`, `<xs:attribute name="flag" type="xs:boolean"/><xs:attribute name="flag" type="xs:integer"/>`, `<xs:attribute name="flag" type="xs:integer"`, `<xs:attribute name="flag" type="xs:boolean"`, diagnosticSchemaAttributeUseDuplicateCode, schemaAttributeUseSpecRef(version), FailureInvalid, errSchemaAttributeUseDuplicate},
			{"nonempty base", "Base", `<xs:element name="value" type="xs:integer"/>`, `<xs:attribute name="flag" type="xs:boolean"/>`, `base="t:Base"`, `<xs:sequence><xs:element name="baseValue"`, UnsupportedSchemaSyntaxCode, schemaComplexTypeExtensionSpecRef(version), FailureUnsupported, errSchemaComplexTypeBaseNonEmpty},
		} {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				base := ``
				if test.name == "nonempty base" {
					base = `<xs:sequence><xs:element name="baseValue" type="xs:integer"/></xs:sequence>`
				}
				root := sequenceAttributeExtensionSchema("1.0", "", test.members, test.uses, base)
				root = strings.Replace(root, `base="t:Base"`, `base="t:`+test.base+`"`, 1)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err == nil {
					t.Fatal("bad extension passed")
				}
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != test.class || d.Code() != test.code || d.Loc() != complexContentTestLoc(t, root, test.marker) || d.SpecRef() != test.spec || !errors.Is(err, test.cause) {
					t.Fatalf("diagnostic: %v related=%v", err, d.Related())
				}
				if test.related != "" && !slices.Contains(d.Related(), complexContentTestLoc(t, root, test.related)) {
					t.Fatalf("related: %v", d.Related())
				}
			})
		}
	}
}

//nolint:gocognit // Assert each excluded extension shape across language policies.
func TestSequenceExtensionAttributeExcludedShapes(t *testing.T) {
	attribute := `<xs:attribute name="flag" type="xs:boolean"/>`
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		version := XSDVersion11
		if policy == Strict10 {
			version = XSDVersion10
		}
		for _, test := range []struct {
			name, root, marker, spec string
			cause                    error
		}{
			{"choice with use", strings.ReplaceAll(sequenceAttributeExtensionSchema("1.0", "", `<xs:element name="value" type="xs:integer"/>`, attribute, ``), "xs:sequence", "xs:choice"), `<xs:attribute name="flag"`, schemaComplexTypeExtensionSpecRef(version), ErrUnsupported},
			{"nested sequence with use", sequenceAttributeExtensionSchema("1.0", "", `<xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence>`, attribute, ``), `<xs:sequence><xs:element`, schemaComplexParticleExtensionSpecRef(version), ErrUnsupported},
			{"attribute group after sequence", sequenceAttributeExtensionSchema("1.0", "", `<xs:element name="value" type="xs:integer"/>`, `<xs:attributeGroup ref="t:Attrs"/>`, ``), `<xs:attributeGroup`, schemaComplexTypeExtensionSpecRef(version), ErrUnsupported},
			{"value constraint", sequenceAttributeExtensionSchema("1.0", "", `<xs:element name="value" type="xs:integer"/>`, `<xs:attribute name="flag" type="xs:boolean" default="true"/>`, ``), `default="true"`, schemaComplexTypeExtensionSpecRef(version), ErrUnsupported},
		} {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, policy)
				if err == nil {
					t.Fatal("excluded form passed")
				}
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != complexContentTestLoc(t, test.root, test.marker) || d.SpecRef() != test.spec || !errors.Is(err, test.cause) {
					t.Fatalf("diagnostic: %v related=%v spec=%q want=%q loc=%s want=%s", err, d.Related(), d.SpecRef(), test.spec, d.Loc(), complexContentTestLoc(t, test.root, test.marker))
				}
			})
		}
	}
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root"><xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:Base"><xs:sequence/><xs:attribute name="flag" type="xs:boolean"/></xs:extension></xs:complexContent></xs:complexType></xs:element><xs:complexType name="Base"/></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err == nil {
		t.Fatal("inline owner passed")
	}
	assertZeroSchema(t, schema)
	d := requireDiagnostic(t, err)
	if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != complexContentTestLoc(t, root, `<xs:extension`) || d.SpecRef() != schemaComplexContentExtensionSpecRef(XSDVersion11) || !errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaSequenceExtensionAnonymousOwner) || !slices.Contains(d.Related(), complexContentTestLoc(t, root, `<xs:complexType>`)) {
		t.Fatalf("inline diagnostic: %v related=%v", err, d.Related())
	}
}

//nolint:gocognit // Pair effective ID cardinality with derivation-cycle diagnostics.
func TestSequenceExtensionAttributeIDCardinalityAndBaseCycle(t *testing.T) {
	uses := `<xs:attribute name="first" type="xs:ID"/><xs:attribute name="excluded" type="xs:ID" use="prohibited"/><xs:attribute name="second" type="xs:ID"/>`
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		root := sequenceAttributeExtensionSchema("1.0", "", `<xs:element name="value" type="xs:integer"/>`, uses, ``)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
		if policy != Strict10 {
			if err != nil {
				t.Fatalf("%s: %v", policy, err)
			}
			derived := requireTestComplexTypeDefinition(t, schema.Components()[1], "Derived")
			if got := derived.AttributeUses(); len(got) != 2 || got[0].Name().Local() != "first" || got[1].Name().Local() != "second" {
				t.Fatalf("%s uses: %v", policy, got)
			}
			continue
		}
		if err == nil {
			t.Fatal("Strict10 admitted two effective IDs")
		}
		assertZeroSchema(t, schema)
		d := requireDiagnostic(t, err)
		if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaAttributeUseIDDuplicateCode || d.Loc() != complexContentTestLoc(t, root, `<xs:attribute name="second"`) || d.SpecRef() != "xsd10-structures#cos-ct-props-correct" || !reflect.DeepEqual(d.Related(), []Loc{complexContentTestLoc(t, root, `<xs:attribute name="first"`)}) || !errors.Is(err, errSchemaAttributeUseIDDuplicate) {
			t.Fatalf("ID diagnostic: %v related=%v", err, d.Related())
		}
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		root := sequenceAttributeExtensionSchema("1.0", "", `<xs:element name="value" type="xs:integer"/>`, `<xs:attribute name="flag" type="xs:boolean"/>`, ``)
		root = strings.Replace(root, `base="t:Base"`, `base="t:Derived"`, 1)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
		if err == nil {
			t.Fatalf("%s admitted cycle", policy)
		}
		assertZeroSchema(t, schema)
		d := requireDiagnostic(t, err)
		version := XSDVersion11
		if policy == Strict10 {
			version = XSDVersion10
		}
		if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.Loc() != complexContentTestLoc(t, root, `base="t:Derived"`) || d.SpecRef() != schemaComplexTypeExtensionSpecRef(version) || !errors.Is(err, errSchemaComplexTypeBaseCycle) {
			t.Fatalf("cycle diagnostic: %v related=%v", err, d.Related())
		}
	}
}

func TestSequenceExtensionInlineAtomicUseAndConsumers(t *testing.T) {
	root := sequenceAttributeExtensionSchema("1.1", "", `<xs:element name="value" type="xs:integer"/>`, `<xs:attribute name="flag"><xs:simpleType><xs:restriction base="xs:boolean"/></xs:simpleType></xs:attribute>`, ``)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	derived := requireTestComplexTypeDefinition(t, schema.Components()[1], "Derived")
	uses := derived.AttributeUses()
	if len(uses) != 1 {
		t.Fatalf("uses: %v", uses)
	}
	local, ok := uses[0].(LocalAttributeUse)
	if !ok || local.Name().Local() != "flag" || local.TypeLoc() != complexContentTestLoc(t, root, `<xs:simpleType>`) {
		t.Fatalf("inline use: %v", uses[0])
	}
	reference, ok := local.TypeReference()
	if !ok || !reference.IsAnonymous() {
		t.Fatalf("inline reference: %v", reference)
	}
	validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
	if validationErr == nil {
		t.Fatal("validation accepted composed extension")
	}
	vd := requireDiagnostic(t, validationErr)
	instanceLoc, locErr := NewLoc("instance.xml", 1, 1)
	if locErr != nil {
		t.Fatalf("instance location: %v", locErr)
	}
	if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != instanceLoc || !slices.Contains(vd.Related(), complexContentTestLoc(t, root, `<xs:extension`)) || !errors.Is(validationErr, errInstanceComplexContentExtension) {
		t.Fatalf("validation: %v", validationErr)
	}
	output, generationErr := GenerateGo(schema, "generated")
	if generationErr == nil || output != nil {
		t.Fatal("generation accepted composed extension")
	}
	gd := requireDiagnostic(t, generationErr)
	if gd.Class() != FailureUnsupported || gd.Code() != diagnosticCodegenUnsupported || gd.Loc() != complexContentTestLoc(t, root, `<xs:attribute name="flag"`) || !errors.Is(generationErr, errCodegenUnsupported) {
		t.Fatalf("generation: %v", generationErr)
	}
}
