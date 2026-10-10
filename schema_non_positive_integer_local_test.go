package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit // Compare the completed public facts for both direct particle models.
func TestSchemaNonPositiveIntegerDirectLocalFacts(t *testing.T) {
	for _, profile := range nonPositiveIntegerPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root" elementFormDefault="qualified" version="` + string(profile.version) + `">
<xs:complexType name="Choice"><xs:choice minOccurs="0" maxOccurs="unbounded"><xs:element name="first" type="xs:nonPositiveInteger" form="qualified" minOccurs="0" maxOccurs="18446744073709551616"/><xs:element name="omit" type="xs:nonPositiveInteger" minOccurs="0" maxOccurs="0"/><xs:element name="last" type="xs:nonPositiveInteger" form="unqualified"/></xs:choice></xs:complexType>
<xs:complexType name="Sequence"><xs:sequence minOccurs="2" maxOccurs="18446744073709551617"><xs:element name="first" type="xs:nonPositiveInteger" form="qualified" minOccurs="0" maxOccurs="unbounded"/><xs:element name="omit" type="xs:nonPositiveInteger" minOccurs="0" maxOccurs="0"/><xs:element name="last" type="xs:nonPositiveInteger" form="unqualified"/></xs:sequence></xs:complexType>
</xs:schema>`
			first, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil || !reflect.DeepEqual(first.Components(), second.Components()) {
				t.Fatalf("repeated components differ: %v", err)
			}
			if matches := first.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, testXSDNamespace, "nonPositiveInteger")); len(matches) != 0 {
				t.Fatalf("built-in type gained %d synthetic components", len(matches))
			}
			var walked []string
			if err := first.Walk(func(component Component) error {
				walked = append(walked, component.Name().Local())
				return nil
			}); err != nil || !reflect.DeepEqual(walked, []string{"Choice", "Sequence"}) {
				t.Fatalf("walk = %v/%v", walked, err)
			}
			for _, want := range []struct {
				owner, model, outer, firstRange string
			}{
				{"Choice", "choice", "0/unbounded", "0/18446744073709551616"},
				{"Sequence", "sequence", "2/18446744073709551617", "0/unbounded"},
			} {
				definition := requireUnsignedLongParticleComplexType(t, first, want.owner)
				var elements []ElementParticle
				switch particle := definition.Particle().(type) {
				case ChoiceParticle:
					if want.model != "choice" || particle.Occurrences().String() != want.outer {
						t.Fatalf("%s choice range = %s", want.owner, particle.Occurrences())
					}
					for _, alternative := range particle.Alternatives() {
						elements = append(elements, requireUnsignedLongElementParticle(t, alternative))
					}
				case SequenceParticle:
					if want.model != "sequence" || particle.Occurrences().String() != want.outer {
						t.Fatalf("%s sequence range = %s", want.owner, particle.Occurrences())
					}
					elements = particle.Elements()
				default:
					t.Fatalf("%s particle = %T", want.owner, definition.Particle())
				}
				if len(elements) != 2 {
					t.Fatalf("%s has %d elements, want ordered first/last after 0/0", want.owner, len(elements))
				}
				for index, element := range elements {
					name, namespace, occurrence := "first", "urn:root", want.firstRange
					if index == 1 {
						name, namespace, occurrence = "last", "", "1/1"
					}
					ownerMarker := `<xs:complexType name="` + want.owner + `"`
					wantElementLoc := unsignedLongParticleTokenLocAfter(t, root, ownerMarker, `<xs:element name="`+name+`"`)
					wantTypeLoc := nonPositiveIntegerLocalTypeLoc(t, root, ownerMarker, name)
					if element.Name() != mustTestQName(t, namespace, name) || element.Loc() != wantElementLoc || element.Occurrences().String() != occurrence {
						t.Fatalf("%s child %d = %s/%s/%s", want.owner, index, element.Name(), element.Loc(), element.Occurrences())
					}
					assertNonPositiveIntegerDirectLocalReference(t, element, wantTypeLoc, profile.version)
					minimum := element.Occurrences().Minimum()
					if minimum.value != nil {
						minimum.value.SetInt64(99)
					}
					maximum, finite := element.Occurrences().Maximum().Finite()
					if finite && maximum.value != nil {
						maximum.value.SetInt64(99)
					}
					if element.Occurrences().String() != occurrence {
						t.Fatalf("mutating copied occurrence changed %s child to %s", want.owner, element.Occurrences())
					}
				}
			}
			copied := first.Components()
			copied[0] = Component{}
			if len(first.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Choice"))) != 1 {
				t.Fatal("mutating copied component slice changed schema")
			}
		})
	}
}

func nonPositiveIntegerLocalTypeLoc(t *testing.T, root, ownerMarker, name string) Loc {
	t.Helper()
	owner := strings.Index(root, ownerMarker)
	if owner < 0 {
		t.Fatalf("missing owner marker %q", ownerMarker)
	}
	element := strings.Index(root[owner:], `<xs:element name="`+name+`"`)
	if element < 0 {
		t.Fatalf("missing local element %q after %q", name, ownerMarker)
	}
	start := owner + element
	typeOffset := strings.Index(root[start:], `type="xs:nonPositiveInteger"`)
	if typeOffset < 0 {
		t.Fatalf("missing nonPositiveInteger type after local element %q", name)
	}
	return namedGroupLocAt(t, root, start+typeOffset)
}

//nolint:dupl // Mirror the neighboring built-in's public facts and consumer boundary.
func assertNonPositiveIntegerDirectLocalReference(t *testing.T, element ElementParticle, wantTypeLoc Loc, version XSDVersion) {
	t.Helper()
	wantType := mustTestQName(t, testXSDNamespace, "nonPositiveInteger")
	reference, ok := element.TypeReference()
	if !ok || !reference.IsBuiltin() || reference.Name() != wantType || reference.QName() != wantType || reference.Loc() != wantTypeLoc || element.DeclaredType() != wantType {
		t.Fatalf("type reference = %#v/%t, declared %s, want built-in at %s", reference, ok, element.DeclaredType(), wantTypeLoc)
	}
	id, present := element.TypeID()
	refID, refPresent := reference.ComponentID()
	if present || refPresent || !id.IsZero() || !refID.IsZero() {
		t.Fatalf("built-in gained ID: %v/%t, %v/%t", id, present, refID, refPresent)
	}
	bounds, ok := reference.IntegerBounds()
	maximum, hasMaximum := bounds.MaxInclusiveFacet()
	if !ok || bounds.Version() != version || !hasMaximum || maximum.Value().Canonical() != "0" || !maximum.Loc().IsZero() || len(bounds.Bounds()) != 1 {
		t.Fatalf("built-in bounds = %#v/%t", bounds, ok)
	}
	copyBounds := bounds.Bounds()
	copyBounds[0] = IntegerBoundFacet{}
	copyMaximum := maximum.Value()
	copyMaximum.value.SetInt64(99)
	repeated, _ := element.TypeReference()
	repeatedBounds, _ := repeated.IntegerBounds()
	repeatedMaximum, _ := repeatedBounds.MaxInclusive()
	if repeatedMaximum.Canonical() != "0" {
		t.Fatalf("mutating copied bound changed schema to %s", repeatedMaximum.Canonical())
	}
}

//nolint:gocognit // Check both consumer diagnostics at the direct local boundary.
func TestSchemaNonPositiveIntegerDirectLocalConsumers(t *testing.T) {
	for _, profile := range nonPositiveIntegerPolicyProfiles() {
		for _, model := range []string{"choice", "sequence"} {
			t.Run(profile.name+"/"+model, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:element name="root" type="r:Record"/><xs:complexType name="Record"><xs:` + model + `><xs:element name="value" type="xs:nonPositiveInteger"/></xs:` + model + `></xs:complexType></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				wantElementLoc := elementReferenceTestAttributeLoc(t, root, `<xs:element name="value"`)
				output, err := GenerateGo(schema, "generated")
				if len(output) != 0 || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("GenerateGo = %q/%v", output, err)
				}
				generated := requireDiagnostic(t, err)
				generationSpec := codegenDirectChoiceSpecReference(profile.version, codegenDirectChoiceElementChoiceReference)
				if model == "sequence" {
					generationSpec = codegenDirectSequenceSpecReference(profile.version, codegenDirectSequenceElementReference)
				}
				if generated.Class() != FailureUnsupported || generated.Code() != diagnosticCodegenUnsupported || generated.Loc() != wantElementLoc || generated.SpecRef() != generationSpec || len(generated.Related()) != 0 || !errors.Is(err, errCodegenUnsupported) {
					t.Fatalf("generation diagnostic = %s, want unsupported at %s", generated, wantElementLoc)
				}
				validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"><value xmlns="">1</value></root>`)))
				if !errors.Is(validationErr, ErrUnsupported) {
					t.Fatalf("ValidateInstance = %v", validationErr)
				}
				validated := requireDiagnostic(t, validationErr)
				wantValidationLoc := wantElementLoc
				if model == "sequence" {
					wantValidationLoc, err = NewLoc("instance.xml", 1, 1)
					if err != nil {
						t.Fatal(err)
					}
				}
				wantRelated := positiveIntegerLocs(t, root, []string{`<xs:element name="root"`, `<xs:complexType name="Record"`, `<xs:` + model + `>`, `<xs:element name="value"`})
				if validated.Class() != FailureUnsupported || validated.Code() != UnsupportedInstanceValidationCode || validated.Loc() != wantValidationLoc || validated.SpecRef() != instanceValidationSpecRef(profile.version) || !reflect.DeepEqual(validated.Related(), wantRelated) || !errors.Is(validationErr, errInstanceUnsupportedType) {
					t.Fatalf("validation diagnostic = %s related=%v spec=%s cause=%v, want unsupported at %s related=%v spec=%s", validated, validated.Related(), validated.SpecRef(), errors.Unwrap(validated), wantValidationLoc, wantRelated, instanceValidationSpecRef(profile.version))
				}
			})
		}
	}
}

//nolint:gocognit // Keep the admitted built-in in each graph-visible named owner.
func TestSchemaNonPositiveIntegerDirectLocalGraphIsDeterministic(t *testing.T) {
	for _, profile := range nonPositiveIntegerPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:include schemaLocation="included.xsd"/><xs:include schemaLocation="included.xsd"/><xs:include schemaLocation="chameleon.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/><xs:element name="root" type="r:Forward"/><xs:complexType name="Root"><xs:choice><xs:element name="rootValue" type="xs:nonPositiveInteger"/></xs:choice></xs:complexType><xs:complexType name="Forward"><xs:sequence><xs:element name="forwardValue" type="xs:nonPositiveInteger"/></xs:sequence></xs:complexType></xs:schema>`
			fixtures := map[string]discoveryFixture{
				"root.xsd":      {id: "root.xsd", contents: root},
				"included.xsd":  {id: "included.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:include schemaLocation="root.xsd"/><xs:complexType name="Included"><xs:choice><xs:element name="includedValue" type="xs:nonPositiveInteger"/></xs:choice></xs:complexType></xs:schema>`},
				"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="` + string(profile.version) + `"><xs:complexType name="Chameleon"><xs:sequence><xs:element name="adoptedValue" type="xs:nonPositiveInteger"/></xs:sequence></xs:complexType></xs:schema>`},
				"other.xsd":     {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other" version="` + string(profile.version) + `"><xs:complexType name="Imported"><xs:choice><xs:element name="importedValue" type="xs:nonPositiveInteger"/></xs:choice></xs:complexType></xs:schema>`},
			}
			first, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil || !reflect.DeepEqual(first.Components(), second.Components()) {
				t.Fatalf("repeated graph components differ: %v", err)
			}
			if len(first.Documents()) != 4 {
				t.Fatalf("documents = %d, want four interned discoveries", len(first.Documents()))
			}
			for _, want := range []struct {
				owner, member, namespace, source, model string
			}{
				{"Root", "rootValue", "urn:root", "root.xsd", "choice"},
				{"Forward", "forwardValue", "urn:root", "root.xsd", "sequence"},
				{"Included", "includedValue", "urn:root", "included.xsd", "choice"},
				{"Chameleon", "adoptedValue", "urn:root", "chameleon.xsd", "sequence"},
				{"Imported", "importedValue", "urn:other", "other.xsd", "choice"},
			} {
				matches := first.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, want.namespace, want.owner))
				if len(matches) != 1 || matches[0].ID().Source() != SourceID(want.source) {
					t.Fatalf("%s owner = %v, want source %s", want.owner, matches, want.source)
				}
				definition, ok := matches[0].ComplexTypeDefinition()
				if !ok {
					t.Fatalf("%s has no complex type view", want.owner)
				}
				var element ElementParticle
				switch particle := definition.Particle().(type) {
				case ChoiceParticle:
					if want.model != "choice" || len(particle.Alternatives()) != 1 {
						t.Fatalf("%s choice = %#v", want.owner, particle)
					}
					element = requireUnsignedLongElementParticle(t, particle.Alternatives()[0])
				case SequenceParticle:
					if want.model != "sequence" || len(particle.Elements()) != 1 {
						t.Fatalf("%s sequence = %#v", want.owner, particle)
					}
					element = particle.Elements()[0]
				default:
					t.Fatalf("%s particle = %T", want.owner, definition.Particle())
				}
				contents := fixtures[want.source].contents
				if want.source == "root.xsd" {
					contents = root
				}
				wantTypeLoc := nonPositiveIntegerLocalGraphLoc(t, SourceID(want.source), contents, want.member)
				if element.Name().Local() != want.member || element.Occurrences().String() != "1/1" {
					t.Fatalf("%s member = %s/%s", want.owner, element.Name(), element.Occurrences())
				}
				assertNonPositiveIntegerDirectLocalReference(t, element, wantTypeLoc, profile.version)
			}
		})
	}
}

func nonPositiveIntegerLocalGraphLoc(t *testing.T, source SourceID, contents, member string) Loc {
	t.Helper()
	memberOffset := strings.Index(contents, `<xs:element name="`+member+`"`)
	if memberOffset < 0 {
		t.Fatalf("missing graph member %q", member)
	}
	typeOffset := strings.Index(contents[memberOffset:], `type="xs:nonPositiveInteger"`)
	if typeOffset < 0 {
		t.Fatalf("missing graph member %q type", member)
	}
	loc, err := NewLoc(source, 1, memberOffset+typeOffset+1)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestSchemaNonPositiveIntegerDirectLocalExclusions(t *testing.T) {
	for _, profile := range nonPositiveIntegerPolicyProfiles() {
		assertDirectLocalIntegerExclusions(t, profile.name, profile.policy, profile.version, "nonPositiveInteger")
	}
}

func TestSchemaNonPositiveIntegerLocalAttributeExclusions(t *testing.T) {
	for _, profile := range nonPositiveIntegerPolicyProfiles() {
		for _, test := range []struct {
			name, definitions, attribute, marker, related string
			ref                                           bool
		}{
			{"direct", "", `<xs:attribute name="value" type="xs:nonPositiveInteger"/>`, `type="xs:nonPositiveInteger"`, "", false},
			{"named derivative", `<xs:simpleType name="Alias"><xs:restriction base="xs:nonPositiveInteger"/></xs:simpleType>`, `<xs:attribute name="value" type="r:Alias"/>`, `type="r:Alias"`, "", false},
			{"inline derivative", "", `<xs:attribute name="value"><xs:simpleType><xs:restriction base="xs:nonPositiveInteger"/></xs:simpleType></xs:attribute>`, `<xs:simpleType>`, "", false},
			{"global ref", `<xs:attribute name="global" type="xs:nonPositiveInteger"/>`, `<xs:attribute ref="r:global"/>`, `ref="r:global"`, `<xs:attribute name="global"`, true},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `">` + test.definitions + `<xs:complexType name="Record">` + test.attribute + `</xs:complexType></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantLoc := elementReferenceTestAttributeLoc(t, root, test.marker)
				wantSpec := schemaAttributeTypeSpecRef(profile.version)
				wantRelated := positiveIntegerLocs(t, root, nil)
				wantCause := errSchemaAttributeUseUnsupported
				if test.ref {
					wantSpec = schemaAttributeUseSpecRef(profile.version)
					wantRelated = positiveIntegerLocs(t, root, []string{test.related})
					wantCause = errSchemaAttributeReferenceUnsupported
				}
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != wantSpec || !reflect.DeepEqual(diagnostic.Related(), wantRelated) || !errors.Is(err, ErrUnsupported) || !errors.Is(err, wantCause) {
					t.Fatalf("%s diagnostic = %s related=%v spec=%s cause=%v, want unsupported at %s related=%v spec=%s", test.name, diagnostic, diagnostic.Related(), diagnostic.SpecRef(), errors.Unwrap(diagnostic), wantLoc, wantRelated, wantSpec)
				}
			})
		}
	}
}

func assertDirectLocalIntegerExclusions(t *testing.T, profileName string, policy LanguagePolicy, version XSDVersion, atomic string) {
	t.Helper()
	for _, model := range []string{"choice", "sequence"} {
		tests := []struct {
			name, body, marker string
		}{
			{"anonymous derivative", `<xs:complexType name="Record"><xs:` + model + `><xs:element name="value"><xs:simpleType><xs:restriction base="xs:` + atomic + `"/></xs:simpleType></xs:element></xs:` + model + `></xs:complexType>`, `<xs:simpleType>`},
			{"complex content extension", `<xs:complexType name="Base"/><xs:complexType name="Record"><xs:complexContent><xs:extension base="r:Base"><xs:` + model + `><xs:element name="value" type="xs:` + atomic + `"/></xs:` + model + `></xs:extension></xs:complexContent></xs:complexType>`, `type="xs:` + atomic + `"`},
			{"inline complex owner", `<xs:element name="root"><xs:complexType><xs:` + model + `><xs:element name="value" type="xs:` + atomic + `"/></xs:` + model + `></xs:complexType></xs:element>`, `type="xs:` + atomic + `"`},
		}
		for _, test := range tests {
			t.Run(profileName+"/"+model+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(version) + `">` + test.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantLoc := elementReferenceTestAttributeLoc(t, root, test.marker)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != wantLoc || diagnostic.Feature() != FeatureSchemaSyntax || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("%s diagnostic = %s, want unsupported at %s", test.name, diagnostic, wantLoc)
				}
				if diagnostic.SpecRef() != positiveIntegerFeatureSpecRef(t, FeatureSchemaSyntax, version) || len(diagnostic.Related()) != 0 {
					t.Fatalf("%s spec/related = %q/%v", test.name, diagnostic.SpecRef(), diagnostic.Related())
				}
			})
		}
	}
}

//nolint:gocognit // Verify both particle views keep only ordered mapped children.
func TestSchemaNonPositiveIntegerDirectLocalZeroOccurrenceOmits(t *testing.T) {
	for _, profile := range nonPositiveIntegerPolicyProfiles() {
		for _, model := range []string{"choice", "sequence"} {
			t.Run(profile.name+"/"+model, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:simpleType name="Alias"><xs:restriction base="xs:nonPositiveInteger"/></xs:simpleType><xs:complexType name="Record"><xs:` + model + `><xs:element name="before" type="xs:nonPositiveInteger"/><xs:element name="omitBuiltin" type="xs:nonPositiveInteger" minOccurs="0" maxOccurs="0"/><xs:element name="omitNamed" type="r:Alias" minOccurs="0" maxOccurs="0"/><xs:element name="after" type="xs:nonPositiveInteger"/></xs:` + model + `></xs:complexType></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				definition := requireUnsignedLongParticleComplexType(t, schema, "Record")
				var elements []ElementParticle
				switch particle := definition.Particle().(type) {
				case ChoiceParticle:
					for _, alternative := range particle.Alternatives() {
						elements = append(elements, requireUnsignedLongElementParticle(t, alternative))
					}
				case SequenceParticle:
					elements = particle.Elements()
				default:
					t.Fatalf("particle = %T", definition.Particle())
				}
				if len(elements) != 2 || elements[0].Name().Local() != "before" || elements[1].Name().Local() != "after" {
					t.Fatalf("mapped ordered elements = %#v, want before/after", elements)
				}
			})
		}
	}
}

//nolint:gocognit // Assert each exit before 0/0 omission at the local particle boundary.
func TestSchemaNonPositiveIntegerDirectLocalDiagnosticExits(t *testing.T) {
	for _, profile := range nonPositiveIntegerPolicyProfiles() {
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
				{"malformed occurrence", `<xs:element name="value" type="xs:nonPositiveInteger" minOccurs="0" maxOccurs="many"/>`, "", `maxOccurs="many"`, nil, FailureInvalid, invalidSchemaCompositionCode, nil, schemaParticleOccurrenceDatatypeSpecRef, true},
				{"reversed occurrence", `<xs:element name="value" type="xs:nonPositiveInteger" minOccurs="2" maxOccurs="1"/>`, "", `<xs:element name="value"`, []string{`minOccurs="2"`, `maxOccurs="1"`}, FailureInvalid, invalidSchemaCompositionCode, errParticleOccurrenceMinimumExceedsMaximum, schemaParticleCorrectSpecRef, false},
				{"unresolved before omission", `<xs:element name="value" type="r:Missing" minOccurs="0" maxOccurs="0"/>`, "", `type="r:Missing"`, nil, FailureInvalid, diagnosticSchemaElementTypeUnresolvedCode, errSchemaElementTypeUnresolved, schemaElementTypeSpecRef, false},
				{"wrong kind before omission", `<xs:element name="value" type="r:Wrong" minOccurs="0" maxOccurs="0"/>`, `<xs:element name="Wrong" type="xs:nonPositiveInteger"/>`, `type="r:Wrong"`, []string{`<xs:element name="Wrong"`}, FailureInvalid, diagnosticSchemaElementTypeWrongKindCode, errSchemaElementTypeWrongKind, schemaElementTypeSpecRef, false},
				{"invalid derivative before omission", `<xs:element name="value" type="r:Bad" minOccurs="0" maxOccurs="0"/>`, `<xs:simpleType name="Bad"><xs:restriction base="xs:nonPositiveInteger"><xs:maxInclusive value="1"/></xs:restriction></xs:simpleType>`, `value="1"`, nil, FailureInvalid, InvalidBoundRestrictionCode, errInvalidBoundRestriction, nonPositiveIntegerBoundRestrictionSpecRef, false},
				{"cycle before omission", `<xs:element name="value" type="r:One" minOccurs="0" maxOccurs="0"/>`, `<xs:simpleType name="One"><xs:restriction base="r:Two"/></xs:simpleType><xs:simpleType name="Two"><xs:restriction base="r:One"/></xs:simpleType>`, `base="r:Two"`, []string{`base="r:One"`}, FailureInvalid, diagnosticSchemaSimpleTypeCycleCode, errSchemaSimpleTypeBaseCycle, schemaSimpleTypeSpecRef, false},
			} {
				t.Run(profile.name+"/"+model+"/"+test.name, func(t *testing.T) {
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:complexType name="Record"><xs:` + model + `>` + test.element + `</xs:` + model + `></xs:complexType>` + test.definitions + `</xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertZeroSchema(t, schema)
					diagnostic := requireDiagnostic(t, err)
					wantLoc := elementReferenceTestAttributeLoc(t, root, test.marker)
					wantRelated := positiveIntegerLocs(t, root, test.related)
					if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != test.spec(profile.version) || !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
						t.Fatalf("diagnostic = %s related=%v, want %s at %s related=%v spec=%s", diagnostic, diagnostic.Related(), test.code, wantLoc, wantRelated, test.spec(profile.version))
					}
					if test.cause != nil && !errors.Is(err, test.cause) {
						t.Fatalf("diagnostic lost %v: %v", test.cause, err)
					}
					if test.lexicalCause {
						var inner Diagnostic
						if !errors.As(errors.Unwrap(diagnostic), &inner) || inner.Code() != InvalidIntegerLexicalCode || inner.Loc() != wantLoc {
							t.Fatalf("malformed occurrence lost located lexical cause: %v", errors.Unwrap(diagnostic))
						}
					}
				})
			}
		}
	}
}

func nonPositiveIntegerBoundRestrictionSpecRef(version XSDVersion) string {
	return boundSpecRef(version, BoundMaxInclusive, boundRestrictionRule)
}

//nolint:gocognit // A global ref remains opaque in both neighboring models and consumers.
func TestSchemaNonPositiveIntegerDirectLocalReferencesKeepConsumerBoundary(t *testing.T) {
	for _, profile := range nonPositiveIntegerPolicyProfiles() {
		for _, model := range []string{"choice", "sequence"} {
			t.Run(profile.name+"/"+model, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:element name="target" type="xs:nonPositiveInteger"/><xs:element name="root" type="r:Record"/><xs:complexType name="Record"><xs:` + model + `><xs:element ref="r:target"/></xs:` + model + `></xs:complexType></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				definition := requireUnsignedLongParticleComplexType(t, schema, "Record")
				var particle Particle
				if model == "choice" {
					choice, ok := definition.Particle().(ChoiceParticle)
					if !ok || len(choice.Alternatives()) != 1 {
						t.Fatalf("choice = %T", definition.Particle())
					}
					particle = choice.Alternatives()[0]
				}
				if model == "sequence" {
					sequence, ok := definition.Particle().(SequenceParticle)
					if !ok || len(sequence.Particles()) != 1 {
						t.Fatalf("sequence = %T", definition.Particle())
					}
					particle = sequence.Particles()[0]
				}
				reference, ok := particle.(ElementReferenceParticle)
				target := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:root", "target"))
				wantRefLoc := elementReferenceTestAttributeLoc(t, root, `ref="r:target"`)
				if !ok || len(target) != 1 || reference.Ref() != mustTestQName(t, "urn:root", "target") || reference.TargetID() != target[0].ID() || reference.RefLoc() != wantRefLoc || reference.Occurrences().String() != "1/1" {
					t.Fatalf("reference = %#v/%t, target=%v", reference, ok, target)
				}
				output, generationErr := GenerateGo(schema, "generated")
				if len(output) != 0 || !errors.Is(generationErr, ErrUnsupported) {
					t.Fatalf("GenerateGo = %q/%v", output, generationErr)
				}
				generation := requireDiagnostic(t, generationErr)
				generationSpec := codegenDirectChoiceSpecReference(profile.version, codegenDirectChoiceElementChoiceReference)
				if model == "sequence" {
					generationSpec = codegenDirectSequenceSpecReference(profile.version, codegenDirectSequenceElementReference)
				}
				wantGenerationRelated := positiveIntegerLocs(t, root, []string{`<xs:element ref="r:target"`, `<xs:element name="target"`})
				if generation.Class() != FailureUnsupported || generation.Code() != diagnosticCodegenUnsupported || generation.Loc() != wantRefLoc || generation.SpecRef() != generationSpec || !reflect.DeepEqual(generation.Related(), wantGenerationRelated) || !errors.Is(generationErr, errCodegenUnsupported) {
					t.Fatalf("generation diagnostic = %s related=%v", generation, generation.Related())
				}
				validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"><target>1</target></root>`)))
				if !errors.Is(validationErr, ErrUnsupported) {
					t.Fatalf("ValidateInstance = %v", validationErr)
				}
				validation := requireDiagnostic(t, validationErr)
				wantValidationLoc, err := NewLoc("instance.xml", 1, 1)
				if err != nil {
					t.Fatal(err)
				}
				wantValidationRelated := positiveIntegerLocs(t, root, []string{`<xs:element name="root"`, `<xs:complexType name="Record"`, `<xs:` + model + `>`, `<xs:element ref="r:target"`, `<xs:element name="target"`})
				validationCause := errInstanceUnsupportedType
				if model == "sequence" {
					validationCause = errInstanceSequenceTarget
					wantValidationRelated[4] = wantRefLoc
				}
				if validation.Class() != FailureUnsupported || validation.Code() != UnsupportedInstanceValidationCode || validation.Loc() != wantValidationLoc || validation.SpecRef() != instanceValidationSpecRef(profile.version) || !reflect.DeepEqual(validation.Related(), wantValidationRelated) || !errors.Is(validationErr, validationCause) {
					t.Fatalf("validation diagnostic = %s related=%v spec=%s cause=%v, want loc=%s related=%v spec=%s cause=%v", validation, validation.Related(), validation.SpecRef(), errors.Unwrap(validation), wantValidationLoc, wantValidationRelated, instanceValidationSpecRef(profile.version), validationCause)
				}
			})
		}
	}
}

func TestSchemaNonPositiveIntegerDirectLocalPolicyGateBeforeOmission(t *testing.T) {
	for _, model := range []string{"choice", "sequence"} {
		t.Run(model, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root" version="1.0"><xs:complexType name="Record"><xs:` + model + `><xs:element name="value" type="xs:nonPositiveInteger" targetNamespace="urn:root" minOccurs="0" maxOccurs="0"/></xs:` + model + `></xs:complexType></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			wantLoc := elementReferenceTestAttributeLoc(t, root, `targetNamespace="urn:root" minOccurs`)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != "xsd11-structures#cSchemaDocument" || len(diagnostic.Related()) != 0 || !errors.Is(err, errLanguagePolicyMismatch) {
				t.Fatalf("policy diagnostic = %s related=%v spec=%s, want mismatch at %s", diagnostic, diagnostic.Related(), diagnostic.SpecRef(), wantLoc)
			}
		})
	}
}

//nolint:dupl // Preserve the graph visibility regression for this built-in category.
func TestSchemaNonPositiveIntegerDirectLocalHiddenGraphTypeBeforeOmission(t *testing.T) {
	for _, profile := range nonPositiveIntegerPolicyProfiles() {
		for _, model := range []string{"choice", "sequence"} {
			t.Run(profile.name+"/"+model, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:f="urn:foreign" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:include schemaLocation="bridge.xsd"/><xs:complexType name="Record"><xs:` + model + `><xs:element name="value" type="f:Hidden" minOccurs="0" maxOccurs="0"/></xs:` + model + `></xs:complexType></xs:schema>`
				fixtures := map[string]discoveryFixture{
					"bridge.xsd":  {id: "bridge.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:import namespace="urn:foreign" schemaLocation="foreign.xsd"/></xs:schema>`},
					"foreign.xsd": {id: "foreign.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:foreign" version="` + string(profile.version) + `"><xs:simpleType name="Hidden"><xs:restriction base="xs:nonPositiveInteger"/></xs:simpleType></xs:schema>`},
				}
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantLoc := elementReferenceTestAttributeLoc(t, root, `type="f:Hidden"`)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaElementTypeUnresolvedCode || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != schemaElementTypeSpecRef(profile.version) || len(diagnostic.Related()) != 0 || !errors.Is(err, errSchemaElementTypeUnresolved) {
					t.Fatalf("hidden type diagnostic = %s related=%v spec=%s, want unresolved at %s", diagnostic, diagnostic.Related(), diagnostic.SpecRef(), wantLoc)
				}
			})
		}
	}
}
