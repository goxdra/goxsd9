package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit,funlen // Exercise completed facts through both public particle views.
func TestSchemaNamedPositiveIntegerDirectLocalFacts(t *testing.T) {
	for _, profile := range positiveIntegerPolicyProfiles() {
		for _, model := range []string{"choice", "sequence"} {
			t.Run(profile.name+"/"+model, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:complexType name="Record"><xs:` + model + ` minOccurs="0" maxOccurs="18446744073709551617"><xs:element name="first" type="r:Plain" minOccurs="0" maxOccurs="unbounded"/><xs:element name="omit" type="r:Empty" minOccurs="0" maxOccurs="0"/><xs:element name="second" type="r:Tight" minOccurs="18446744073709551616" maxOccurs="18446744073709551617"/><xs:element name="third" type="r:Empty"/><xs:element name="fourth" type="r:Base"/></xs:` + model + `></xs:complexType><xs:simpleType name="Tight"><xs:restriction base="r:Base"><xs:minInclusive value="3"/><xs:maxExclusive value="8"/></xs:restriction></xs:simpleType><xs:simpleType name="Base"><xs:restriction base="xs:positiveInteger"><xs:maxInclusive value="9"/></xs:restriction></xs:simpleType><xs:simpleType name="Plain"><xs:restriction base="xs:positiveInteger"/></xs:simpleType><xs:simpleType name="Empty"><xs:restriction base="r:Plain"/></xs:simpleType></xs:schema>`
				first, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				second, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil || !reflect.DeepEqual(first.Components(), second.Components()) {
					t.Fatalf("repeated components = %v", err)
				}
				owner := requireUnsignedLongParticleComplexType(t, first, "Record")
				var elements []ElementParticle
				switch particle := owner.Particle().(type) {
				case ChoiceParticle:
					if model != "choice" || particle.Occurrences().String() != "0/18446744073709551617" {
						t.Fatalf("choice = %v", particle.Occurrences())
					}
					for _, alternative := range particle.Alternatives() {
						elements = append(elements, requireUnsignedLongElementParticle(t, alternative))
					}
				case SequenceParticle:
					if model != "sequence" || particle.Occurrences().String() != "0/18446744073709551617" {
						t.Fatalf("sequence = %v", particle.Occurrences())
					}
					elements = particle.Elements()
				default:
					t.Fatalf("particle = %T", owner.Particle())
				}
				for index, want := range []struct {
					name, typ, occurrence, min, max, minLoc, maxLoc string
				}{
					{"first", "Plain", "0/unbounded", "1", "", "", ""},
					{"second", "Tight", "18446744073709551616/18446744073709551617", "3", "8", `value="3"`, `value="8"`},
					{"third", "Empty", "1/1", "1", "", "", ""},
					{"fourth", "Base", "1/1", "1", "9", "", `value="9"`},
				} {
					if len(elements) != 4 {
						t.Fatalf("elements = %d, want four ordered nonzero terms", len(elements))
					}
					element := elements[index]
					marker := `<xs:element name="` + want.name + `"`
					typeMarker := `type="r:` + want.typ + `"`
					wantType := mustTestQName(t, "urn:root", want.typ)
					if element.Name() != mustTestQName(t, "", want.name) || element.DeclaredType() != wantType || element.Loc() != elementReferenceTestAttributeLoc(t, root, marker) || element.Occurrences().String() != want.occurrence {
						t.Fatalf("%s facts = %s/%s/%s/%s", want.name, element.Name(), element.DeclaredType(), element.Loc(), element.Occurrences())
					}
					reference, ok := element.TypeReference()
					if !ok || !reference.IsNamed() || reference.Name() != wantType || reference.QName() != wantType || reference.Loc() != namedPositiveLocalTypeLoc(t, root, marker, typeMarker) {
						t.Fatalf("%s reference = %#v/%t", want.name, reference, ok)
					}
					target := first.FindKind(ComponentKindSimpleTypeDefinition, wantType)
					typeID, hasTypeID := element.TypeID()
					refID, hasRefID := reference.ComponentID()
					if len(target) != 1 || !hasTypeID || !hasRefID || typeID != target[0].ID() || refID != typeID {
						t.Fatalf("%s IDs = %v/%t %v/%t, target=%v", want.name, typeID, hasTypeID, refID, hasRefID, target)
					}
					bounds, ok := reference.IntegerBounds()
					minimum, hasMinimum := bounds.MinInclusiveFacet()
					if !ok || bounds.Version() != profile.version || !hasMinimum || minimum.Value().Canonical() != want.min {
						t.Fatalf("%s minimum = %#v/%t", want.name, minimum, hasMinimum)
					}
					wantMinLoc := Loc{}
					if want.minLoc != "" {
						wantMinLoc = elementReferenceTestAttributeLoc(t, root, want.minLoc)
					}
					if minimum.Loc() != wantMinLoc {
						t.Fatalf("%s minimum Loc = %s, want %s", want.name, minimum.Loc(), wantMinLoc)
					}
					maximum, hasMaximum := bounds.MaxExclusiveFacet()
					if want.typ == "Base" {
						maximum, hasMaximum = bounds.MaxInclusiveFacet()
					}
					if want.max == "" && hasMaximum || want.max != "" && (!hasMaximum || maximum.Value().Canonical() != want.max || maximum.Loc() != elementReferenceTestAttributeLoc(t, root, want.maxLoc)) {
						t.Fatalf("%s maximum = %#v/%t", want.name, maximum, hasMaximum)
					}
					copied := bounds.Bounds()
					copied[0] = IntegerBoundFacet{}
					value := minimum.Value()
					value.value.SetInt64(99)
					occurrenceMinimum := element.Occurrences().Minimum()
					occurrenceMinimum.value.SetInt64(99)
					repeated, _ := element.TypeReference()
					repeatedBounds, _ := repeated.IntegerBounds()
					repeatedMinimum, _ := repeatedBounds.MinInclusive()
					if repeatedMinimum.Canonical() != want.min || element.Occurrences().String() != want.occurrence {
						t.Fatalf("%s copied facts mutated schema", want.name)
					}
				}
				copied := first.Components()
				copied[0] = Component{}
				if len(first.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Record"))) != 1 {
					t.Fatal("component view mutated schema")
				}
			})
		}
	}
}

func namedPositiveLocalTypeLoc(t *testing.T, root, elementMarker, typeMarker string) Loc {
	t.Helper()
	element := strings.Index(root, elementMarker)
	if element < 0 {
		t.Fatalf("missing element marker %q", elementMarker)
	}
	typeOffset := strings.Index(root[element:], typeMarker)
	if typeOffset < 0 {
		t.Fatalf("missing type marker %q after %q", typeMarker, elementMarker)
	}
	return namedGroupLocAt(t, root, element+typeOffset)
}

//nolint:gocognit // Each excluded owner and particle shape needs a located public diagnostic.
func TestSchemaNamedPositiveIntegerLocalExclusions(t *testing.T) {
	for _, profile := range positiveIntegerPolicyProfiles() {
		for _, model := range []string{"choice", "sequence"} {
			for _, test := range []struct {
				name, body, marker string
			}{
				{"extension", `<xs:complexType name="Base"/><xs:complexType name="Record"><xs:complexContent><xs:extension base="r:Base"><xs:` + model + `><xs:element name="value" type="r:Alias"/></xs:` + model + `></xs:extension></xs:complexContent></xs:complexType>`, `type="r:Alias"`},
				{"inline owner", `<xs:element name="root"><xs:complexType><xs:` + model + `><xs:element name="value" type="r:Alias"/></xs:` + model + `></xs:complexType></xs:element>`, `type="r:Alias"`},
				{"anonymous derivative", `<xs:complexType name="Record"><xs:` + model + `><xs:element name="value"><xs:simpleType><xs:restriction base="r:Alias"/></xs:simpleType></xs:element></xs:` + model + `></xs:complexType>`, `<xs:simpleType>`},
				{"nested particle", `<xs:complexType name="Record"><xs:` + model + `><xs:sequence><xs:element name="value" type="r:Alias"/></xs:sequence></xs:` + model + `></xs:complexType>`, `<xs:sequence>`},
			} {
				t.Run(profile.name+"/"+model+"/"+test.name, func(t *testing.T) {
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `">` + test.body + `<xs:simpleType name="Alias"><xs:restriction base="xs:positiveInteger"/></xs:simpleType></xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertZeroSchema(t, schema)
					diagnostic := requireDiagnostic(t, err)
					wantLoc := elementReferenceTestAttributeLoc(t, root, test.marker)
					wantSpec := positiveIntegerFeatureSpecRef(t, FeatureSchemaSyntax, profile.version)
					if test.name == "nested particle" {
						wantLoc = namedGroupLocAt(t, root, strings.LastIndex(root, `<xs:sequence>`))
						wantSpec = schemaSyntaxSpecRefForVersion(XSDVersion10)
					}
					if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != wantSpec || len(diagnostic.Related()) != 0 || !errors.Is(err, ErrUnsupported) {
						t.Fatalf("diagnostic = %s related=%v spec=%s, want unsupported at %s spec=%s", diagnostic, diagnostic.Related(), diagnostic.SpecRef(), wantLoc, wantSpec)
					}
				})
			}
		}
	}
}

func TestSchemaNamedPositiveIntegerAllMemberExcluded(t *testing.T) {
	for _, profile := range positiveIntegerPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:complexType name="Record"><xs:all><xs:element name="value" type="r:Alias"/></xs:all></xs:complexType><xs:simpleType name="Alias"><xs:restriction base="xs:positiveInteger"/></xs:simpleType></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			wantLoc := elementReferenceTestAttributeLoc(t, root, `type="r:Alias"`)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != schemaSyntaxSpecRefForVersion(profile.version) || len(diagnostic.Related()) != 0 || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("all diagnostic = %s related=%v spec=%s", diagnostic, diagnostic.Related(), diagnostic.SpecRef())
			}
		})
	}
}

//nolint:dupl,gocognit // Both signed integer boundary families need independent public boundary coverage.
func TestSchemaNamedPositiveIntegerLocalReferenceStaysOpaque(t *testing.T) {
	for _, profile := range positiveIntegerPolicyProfiles() {
		for _, model := range []string{"choice", "sequence"} {
			t.Run(profile.name+"/"+model, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:simpleType name="Alias"><xs:restriction base="xs:positiveInteger"/></xs:simpleType><xs:element name="target" type="r:Alias"/><xs:element name="root" type="r:Record"/><xs:complexType name="Record"><xs:` + model + `><xs:element ref="r:target"/></xs:` + model + `></xs:complexType></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				owner := requireUnsignedLongParticleComplexType(t, schema, "Record")
				var particle Particle
				switch model {
				case "choice":
					choice, ok := owner.Particle().(ChoiceParticle)
					if !ok || len(choice.Alternatives()) != 1 {
						t.Fatalf("choice = %T", owner.Particle())
					}
					particle = choice.Alternatives()[0]
				case "sequence":
					sequence, ok := owner.Particle().(SequenceParticle)
					if !ok || len(sequence.Particles()) != 1 {
						t.Fatalf("sequence = %T", owner.Particle())
					}
					particle = sequence.Particles()[0]
				}
				reference, ok := particle.(ElementReferenceParticle)
				target := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:root", "target"))
				wantLoc := elementReferenceTestAttributeLoc(t, root, `ref="r:target"`)
				if !ok || len(target) != 1 || reference.Ref() != mustTestQName(t, "urn:root", "target") || reference.TargetID() != target[0].ID() || reference.RefLoc() != wantLoc || reference.Occurrences().String() != "1/1" {
					t.Fatalf("ref = %#v/%t, target=%v", reference, ok, target)
				}
				output, generationErr := GenerateGo(schema, "generated")
				if len(output) != 0 || !errors.Is(generationErr, errCodegenUnsupported) {
					t.Fatalf("generation = %q/%v", output, generationErr)
				}
				generated := requireDiagnostic(t, generationErr)
				generationSpec := codegenDirectChoiceSpecReference(profile.version, codegenDirectChoiceElementChoiceReference)
				if model == "sequence" {
					generationSpec = codegenDirectSequenceSpecReference(profile.version, codegenDirectSequenceElementReference)
				}
				wantGenerationRelated := positiveIntegerLocs(t, root, []string{`<xs:element ref="r:target"`, `<xs:element name="target"`, `<xs:simpleType name="Alias"`, `base="xs:positiveInteger"`})
				if generated.Class() != FailureUnsupported || generated.Code() != diagnosticCodegenUnsupported || generated.Loc() != wantLoc || generated.SpecRef() != generationSpec || !reflect.DeepEqual(generated.Related(), wantGenerationRelated) {
					t.Fatalf("generation diagnostic = %s related=%v spec=%s, want loc=%s spec=%s", generated, generated.Related(), generated.SpecRef(), wantLoc, generationSpec)
				}
				validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"><target>1</target></root>`)))
				if !errors.Is(validationErr, ErrUnsupported) {
					t.Fatalf("validation = %v", validationErr)
				}
				validated := requireDiagnostic(t, validationErr)
				wantValidationLoc, err := NewLoc("instance.xml", 1, 1)
				if err != nil {
					t.Fatal(err)
				}
				wantValidationRelated := positiveIntegerLocs(t, root, []string{`<xs:element name="root"`, `<xs:complexType name="Record"`, `<xs:` + model + `>`, `<xs:element ref="r:target"`, `<xs:element name="target"`, `<xs:simpleType name="Alias"`})
				validationCause := errInstanceUnsupportedType
				if model == "sequence" {
					wantValidationRelated = positiveIntegerLocs(t, root, []string{`<xs:element name="root"`, `<xs:complexType name="Record"`, `<xs:` + model + `>`, `<xs:element ref="r:target"`, `ref="r:target"`})
					validationCause = errInstanceSequenceTarget
				}
				if validated.Class() != FailureUnsupported || validated.Code() != UnsupportedInstanceValidationCode || validated.Loc() != wantValidationLoc || validated.SpecRef() != instanceValidationSpecRef(profile.version) || !reflect.DeepEqual(validated.Related(), wantValidationRelated) || !errors.Is(validationErr, validationCause) {
					t.Fatalf("validation diagnostic = %s related=%v, want loc=%s related=%v", validated, validated.Related(), wantValidationLoc, wantValidationRelated)
				}
			})
		}
	}
}

//nolint:gocognit // Keep both consumers independently rejected for surviving named locals.
func TestSchemaNamedPositiveIntegerLocalConsumers(t *testing.T) {
	for _, profile := range positiveIntegerPolicyProfiles() {
		for _, model := range []string{"choice", "sequence"} {
			t.Run(profile.name+"/"+model, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:element name="root" type="r:Record"/><xs:complexType name="Record"><xs:` + model + `><xs:element name="value" type="r:Alias"/></xs:` + model + `></xs:complexType><xs:simpleType name="Alias"><xs:restriction base="xs:positiveInteger"/></xs:simpleType></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				wantElementLoc := elementReferenceTestAttributeLoc(t, root, `<xs:element name="value"`)
				output, err := GenerateGo(schema, "generated")
				if len(output) != 0 || !errors.Is(err, errCodegenUnsupported) {
					t.Fatalf("generation = %q/%v", output, err)
				}
				generated := requireDiagnostic(t, err)
				if generated.Class() != FailureUnsupported || generated.Code() != diagnosticCodegenUnsupported || generated.Loc() != wantElementLoc || generated.SpecRef() != schemaSimpleTypeSpecRef(profile.version) || !reflect.DeepEqual(generated.Related(), positiveIntegerLocs(t, root, []string{`<xs:simpleType name="Alias"`, `base="xs:positiveInteger"`})) {
					t.Fatalf("generation diagnostic = %s related=%v spec=%s, want %s", generated, generated.Related(), generated.SpecRef(), schemaSimpleTypeSpecRef(profile.version))
				}
				validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"><value xmlns="">1</value></root>`)))
				if !errors.Is(validationErr, errInstanceUnsupportedType) {
					t.Fatalf("validation = %v", validationErr)
				}
				validated := requireDiagnostic(t, validationErr)
				wantValidationLoc := wantElementLoc
				if model == "sequence" {
					wantValidationLoc, err = NewLoc("instance.xml", 1, 1)
					if err != nil {
						t.Fatal(err)
					}
				}
				wantRelated := positiveIntegerLocs(t, root, []string{`<xs:element name="root"`, `<xs:complexType name="Record"`, `<xs:` + model + `>`, `<xs:element name="value"`, `<xs:simpleType name="Alias"`})
				if validated.Class() != FailureUnsupported || validated.Code() != UnsupportedInstanceValidationCode || validated.Loc() != wantValidationLoc || validated.SpecRef() != instanceValidationSpecRef(profile.version) || !reflect.DeepEqual(validated.Related(), wantRelated) {
					t.Fatalf("validation diagnostic = %s related=%v", validated, validated.Related())
				}
			})
		}
	}
}

//nolint:gocognit // Resolve named local types across ordered graph discovery and cycles.
func TestSchemaNamedPositiveIntegerLocalGraph(t *testing.T) {
	for _, profile := range positiveIntegerPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:f="urn:foreign" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:include schemaLocation="included.xsd"/><xs:include schemaLocation="included.xsd"/><xs:include schemaLocation="chameleon.xsd"/><xs:import namespace="urn:foreign" schemaLocation="foreign.xsd"/><xs:complexType name="First"><xs:choice><xs:element name="one" type="r:Forward"/></xs:choice></xs:complexType><xs:complexType name="Second"><xs:sequence><xs:element name="two" type="f:Imported"/></xs:sequence></xs:complexType><xs:simpleType name="Forward"><xs:restriction base="xs:positiveInteger"/></xs:simpleType></xs:schema>`
			fixtures := map[string]discoveryFixture{
				"root.xsd":      {id: "root.xsd", contents: root},
				"included.xsd":  {id: "included.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:include schemaLocation="root.xsd"/><xs:complexType name="Included"><xs:choice><xs:element name="three" type="r:Forward" xmlns:r="urn:root"/></xs:choice></xs:complexType></xs:schema>`},
				"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="` + string(profile.version) + `"><xs:simpleType name="Adopted"><xs:restriction base="xs:positiveInteger"/></xs:simpleType><xs:complexType name="Chameleon"><xs:sequence><xs:element name="four" type="r:Adopted" xmlns:r="urn:root"/></xs:sequence></xs:complexType></xs:schema>`},
				"foreign.xsd":   {id: "foreign.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:foreign" version="` + string(profile.version) + `"><xs:simpleType name="Imported"><xs:restriction base="xs:positiveInteger"><xs:minInclusive value="4"/></xs:restriction></xs:simpleType></xs:schema>`},
			}
			first, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil || !reflect.DeepEqual(first.Components(), second.Components()) || !reflect.DeepEqual(first.Documents(), second.Documents()) {
				t.Fatalf("repeated graph differed: %v", err)
			}
			if len(first.Documents()) != 4 {
				t.Fatalf("documents = %d, want four interned documents", len(first.Documents()))
			}
			for _, want := range []struct {
				owner, member, typ, namespace, source, minimum string
			}{
				{"First", "one", "Forward", "urn:root", "root.xsd", "1"},
				{"Second", "two", "Imported", "urn:foreign", "root.xsd", "4"},
				{"Included", "three", "Forward", "urn:root", "included.xsd", "1"},
				{"Chameleon", "four", "Adopted", "urn:root", "chameleon.xsd", "1"},
			} {
				owner := requireUnsignedLongParticleComplexType(t, first, want.owner)
				var element ElementParticle
				switch particle := owner.Particle().(type) {
				case ChoiceParticle:
					if len(particle.Alternatives()) != 1 {
						t.Fatalf("%s alternatives = %d", want.owner, len(particle.Alternatives()))
					}
					element = requireUnsignedLongElementParticle(t, particle.Alternatives()[0])
				case SequenceParticle:
					if len(particle.Elements()) != 1 {
						t.Fatalf("%s elements = %d", want.owner, len(particle.Elements()))
					}
					element = particle.Elements()[0]
				default:
					t.Fatalf("%s particle = %T", want.owner, owner.Particle())
				}
				name := mustTestQName(t, want.namespace, want.typ)
				reference, ok := element.TypeReference()
				target := first.FindKind(ComponentKindSimpleTypeDefinition, name)
				id, hasID := element.TypeID()
				bounds, hasBounds := reference.IntegerBounds()
				minimum, hasMinimum := bounds.MinInclusive()
				if !ok || len(target) != 1 || !hasID || id != target[0].ID() || reference.Name() != name || reference.Loc().Source() != SourceID(want.source) || element.Loc().Source() != SourceID(want.source) || element.Name().Local() != want.member || !hasBounds || !hasMinimum || minimum.Canonical() != want.minimum {
					t.Fatalf("%s facts = %#v/%t, target=%v, min=%s", want.owner, reference, ok, target, minimum.Canonical())
				}
			}
		})
	}
}

//nolint:dupl,gocognit // Both signed integer boundary families require independent located diagnostic coverage.
func TestSchemaNamedPositiveIntegerLocalDiagnosticExits(t *testing.T) {
	for _, profile := range positiveIntegerPolicyProfiles() {
		for _, model := range []string{"choice", "sequence"} {
			for _, test := range []struct {
				name, element, definitions, marker string
				related                            []string
				class                              FailureClass
				code                               string
				cause                              error
				spec                               func(XSDVersion) string
				lexicalCause                       bool
			}{
				{"malformed occurrence", `<xs:element name="value" type="r:Alias" minOccurs="0" maxOccurs="many"/>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:positiveInteger"/></xs:simpleType>`, `maxOccurs="many"`, nil, FailureInvalid, invalidSchemaCompositionCode, nil, schemaParticleOccurrenceDatatypeSpecRef, true},
				{"reversed occurrence", `<xs:element name="value" type="r:Alias" minOccurs="2" maxOccurs="1"/>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:positiveInteger"/></xs:simpleType>`, `<xs:element name="value"`, []string{`minOccurs="2"`, `maxOccurs="1"`}, FailureInvalid, invalidSchemaCompositionCode, errParticleOccurrenceMinimumExceedsMaximum, schemaParticleCorrectSpecRef, false},
				{"unresolved", `<xs:element name="value" type="r:Missing" minOccurs="0" maxOccurs="0"/>`, "", `type="r:Missing"`, nil, FailureInvalid, diagnosticSchemaElementTypeUnresolvedCode, errSchemaElementTypeUnresolved, schemaElementTypeSpecRef, false},
				{"wrong kind", `<xs:element name="value" type="r:Wrong" minOccurs="0" maxOccurs="0"/>`, `<xs:element name="Wrong" type="xs:positiveInteger"/>`, `type="r:Wrong"`, []string{`<xs:element name="Wrong"`}, FailureInvalid, diagnosticSchemaElementTypeWrongKindCode, errSchemaElementTypeWrongKind, schemaElementTypeSpecRef, false},
				{"invalid interval", `<xs:element name="value" type="r:Bad" minOccurs="0" maxOccurs="0"/>`, `<xs:simpleType name="Bad"><xs:restriction base="xs:positiveInteger"><xs:maxInclusive value="0"/></xs:restriction></xs:simpleType>`, `value="0"`, nil, FailureInvalid, InvalidBoundRestrictionCode, errInvalidBoundRestriction, positiveIntegerBoundRestrictionSpecRef, false},
				{"unsupported facet", `<xs:element name="value" type="r:Bad" minOccurs="0" maxOccurs="0"/>`, `<xs:simpleType name="Bad"><xs:restriction base="xs:positiveInteger"><xs:pattern value="[0-9]+"/></xs:restriction></xs:simpleType>`, `<xs:pattern`, nil, FailureUnsupported, UnsupportedDatatypeFacetCode, ErrUnsupported, nil, false},
				{"cycle", `<xs:element name="value" type="r:One" minOccurs="0" maxOccurs="0"/>`, `<xs:simpleType name="One"><xs:restriction base="r:Two"/></xs:simpleType><xs:simpleType name="Two"><xs:restriction base="r:One"/></xs:simpleType>`, `base="r:Two"`, []string{`base="r:One"`}, FailureInvalid, diagnosticSchemaSimpleTypeCycleCode, errSchemaSimpleTypeBaseCycle, schemaSimpleTypeSpecRef, false},
			} {
				t.Run(profile.name+"/"+model+"/"+test.name, func(t *testing.T) {
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:complexType name="Record"><xs:` + model + `>` + test.element + `</xs:` + model + `></xs:complexType>` + test.definitions + `</xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertZeroSchema(t, schema)
					diagnostic := requireDiagnostic(t, err)
					wantLoc := elementReferenceTestAttributeLoc(t, root, test.marker)
					wantRelated := positiveIntegerLocs(t, root, test.related)
					wantSpec := positiveIntegerFeatureSpecRef(t, FeatureDatatypeFacets, profile.version)
					if test.spec != nil {
						wantSpec = test.spec(profile.version)
					}
					if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != wantSpec || !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
						t.Fatalf("diagnostic = %s related=%v spec=%s, want %s at %s related=%v spec=%s", diagnostic, diagnostic.Related(), diagnostic.SpecRef(), test.code, wantLoc, wantRelated, wantSpec)
					}
					if test.cause != nil && !errors.Is(err, test.cause) {
						t.Fatalf("diagnostic lost %v: %v", test.cause, err)
					}
					if test.lexicalCause {
						var inner Diagnostic
						if !errors.As(errors.Unwrap(diagnostic), &inner) || inner.Code() != InvalidIntegerLexicalCode || inner.Loc() != wantLoc {
							t.Fatalf("malformed occurrence lost lexical cause: %v", errors.Unwrap(diagnostic))
						}
					}
				})
			}
		}
	}
}

//nolint:dupl,gocognit // Both signed integer boundary families require independent omission coverage.
func TestSchemaNamedPositiveIntegerLocalZeroOmissionConsumers(t *testing.T) {
	for _, profile := range positiveIntegerPolicyProfiles() {
		for _, model := range []string{"choice", "sequence"} {
			t.Run(profile.name+"/"+model, func(t *testing.T) {
				prefix := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:simpleType name="Alias"><xs:restriction base="xs:positiveInteger"/></xs:simpleType><xs:element name="root" type="r:Record"/><xs:complexType name="Record"><xs:` + model + `>`
				suffix := `<xs:element name="value" type="xs:integer"/></xs:` + model + `></xs:complexType></xs:schema>`
				omitted := `<xs:element name="omitted" type="r:Alias" minOccurs="0" maxOccurs="0"/>`
				for _, body := range []string{prefix + suffix, prefix + omitted + suffix} {
					schema, err := discoverTestSchemaWithPolicy(t, body, nil, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					owner := requireUnsignedLongParticleComplexType(t, schema, "Record")
					switch particle := owner.Particle().(type) {
					case ChoiceParticle:
						if len(particle.Alternatives()) != 1 || requireUnsignedLongElementParticle(t, particle.Alternatives()[0]).Name().Local() != "value" {
							t.Fatalf("choice alternatives = %v", particle.Alternatives())
						}
					case SequenceParticle:
						if len(particle.Elements()) != 1 || particle.Elements()[0].Name().Local() != "value" {
							t.Fatalf("sequence elements = %v", particle.Elements())
						}
					default:
						t.Fatalf("particle = %T", owner.Particle())
					}
					if err := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"><value xmlns="">2</value></root>`))); err != nil {
						t.Fatalf("validation after omission = %v", err)
					}
					output, generationErr := GenerateGo(schema, "generated")
					if len(output) != 0 || !errors.Is(generationErr, errCodegenUnsupported) {
						t.Fatalf("generation after omission = %q/%v", output, generationErr)
					}
					generation := requireDiagnostic(t, generationErr)
					if generation.Code() != diagnosticCodegenUnsupported || generation.Class() != FailureUnsupported || generation.Loc() != elementReferenceTestAttributeLoc(t, body, `<xs:simpleType name="Alias"`) || generation.SpecRef() != schemaSimpleTypeSpecRef(profile.version) {
						t.Fatalf("generation diagnostic = %s", generation)
					}
				}
			})
		}
	}
}

func TestSchemaNamedPositiveIntegerLocalPolicyBeforeOmission(t *testing.T) {
	for _, model := range []string{"choice", "sequence"} {
		t.Run(model, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="1.0"><xs:complexType name="Record"><xs:` + model + `><xs:element name="value" type="r:Alias" targetNamespace="urn:root" minOccurs="0" maxOccurs="0"/></xs:` + model + `></xs:complexType><xs:simpleType name="Alias"><xs:restriction base="xs:positiveInteger"/></xs:simpleType></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			wantLoc := elementReferenceTestAttributeLoc(t, root, `targetNamespace="urn:root" minOccurs`)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != "xsd11-structures#cSchemaDocument" || len(diagnostic.Related()) != 0 || !errors.Is(err, errLanguagePolicyMismatch) {
				t.Fatalf("policy diagnostic = %s related=%v spec=%s", diagnostic, diagnostic.Related(), diagnostic.SpecRef())
			}
		})
	}
}
