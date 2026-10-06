package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

type intParticleProfile struct {
	name    string
	policy  LanguagePolicy
	version XSDVersion
}

func intParticleProfiles() []intParticleProfile {
	return []intParticleProfile{
		{name: "compatible 1.0", policy: Compatibility, version: XSDVersion10},
		{name: "strict 1.0", policy: Strict10, version: XSDVersion10},
		{name: "compatible 1.1", policy: Compatibility, version: XSDVersion11},
		{name: "strict 1.1", policy: Strict11, version: XSDVersion11},
	}
}

//nolint:gocognit,funlen // Exercise the four owner shapes against one graph and exact fact contract.
func TestSchemaIntLocalParticlesAcrossGraphsAndOwners(t *testing.T) {
	for _, profile := range intParticleProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			factVersion := profile.version
			if profile.policy == Compatibility {
				factVersion = XSDVersion11
			}
			root, fixtures := intParticleGraph(profile.version)
			first, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("repeated discoverTestSchemaWithPolicy: %v", err)
			}
			before := first.Components()
			if !reflect.DeepEqual(before, second.Components()) {
				t.Fatal("repeated graph builds changed int component facts or order")
			}
			if len(first.Documents()) != 4 {
				t.Fatalf("document count = %d, want 4", len(first.Documents()))
			}

			for _, owner := range []struct {
				name      string
				model     string
				extension bool
			}{
				{name: "Choice", model: "choice"},
				{name: "Sequence", model: "sequence"},
				{name: "ExtendedChoice", model: "choice", extension: true},
				{name: "ExtendedSequence", model: "sequence", extension: true},
			} {
				definition := requireUnsignedLongParticleComplexType(t, first, owner.name)
				if owner.extension && (definition.Derivation() != ComplexTypeDerivationExtension || definition.Base() != mustTestQName(t, "urn:root", "Base")) {
					t.Fatalf("%s derivation/base = %q/%q, want extension/r:Base", owner.name, definition.Derivation(), definition.Base())
				}
				var elements []ElementParticle
				switch particle := definition.Particle().(type) {
				case ChoiceParticle:
					if owner.model != "choice" {
						t.Fatalf("%s has choice, want sequence", owner.name)
					}
					for _, alternative := range particle.Alternatives() {
						elements = append(elements, requireUnsignedLongElementParticle(t, alternative))
					}
				case SequenceParticle:
					if owner.model != "sequence" {
						t.Fatalf("%s has sequence, want choice", owner.name)
					}
					elements = particle.Elements()
				default:
					t.Fatalf("%s particle = %T, want choice or sequence", owner.name, definition.Particle())
				}
				if len(elements) != 5 {
					t.Fatalf("%s children = %d, want 5 after zero omission", owner.name, len(elements))
				}
				wantOwnerOccurrence := "0/unbounded"
				if owner.model == "sequence" {
					wantOwnerOccurrence = "0/18446744073709551617"
				}
				if definition.Particle().Occurrences().String() != wantOwnerOccurrence {
					t.Fatalf("%s occurrences = %s, want %s", owner.name, definition.Particle().Occurrences(), wantOwnerOccurrence)
				}
				for index, expected := range []struct {
					local, typeName, namespace, source, minimum, maximum, occurrence string
				}{
					{local: "builtin", typeName: "int", namespace: testXSDNamespace, occurrence: "2/18446744073709551616"},
					{local: "forward", typeName: "Forward", namespace: "urn:root", source: "root.xsd", minimum: "-100", maximum: "100", occurrence: "1/unbounded"},
					{local: "included", typeName: "Included", namespace: "urn:root", source: "ordinary.xsd", minimum: "-10", maximum: "10", occurrence: "1/1"},
					{local: "chameleon", typeName: "Chameleon", namespace: "urn:root", source: "chameleon.xsd", minimum: "-20", maximum: "20", occurrence: "1/1"},
					{local: "imported", typeName: "Imported", namespace: "urn:other", source: "other.xsd", minimum: "-30", maximum: "30", occurrence: "1/1"},
				} {
					element := elements[index]
					nameNamespace := ""
					if index == 0 {
						nameNamespace = "urn:root"
					}
					ownerMarker := `<xs:complexType name="` + owner.name + `">`
					wantElementLoc := unsignedLongParticleTokenLocAfter(t, root, ownerMarker, `<xs:element name="`+expected.local+`"`)
					if element.Name() != mustTestQName(t, nameNamespace, expected.local) || element.Occurrences().String() != expected.occurrence || element.Loc() != wantElementLoc {
						t.Fatalf("%s element %d = %q/%s/%s, want %s/%s/located", owner.name, index, element.Name(), element.Occurrences(), element.Loc(), expected.local, expected.occurrence)
					}
					reference, ok := element.TypeReference()
					typeLexical := "r:" + expected.typeName
					if index == 0 {
						typeLexical = "xs:int"
					}
					if index == 4 {
						typeLexical = "o:Imported"
					}
					wantTypeLoc := unsignedLongParticleTokenLocAfter(t, root, ownerMarker, `type="`+typeLexical+`"`)
					if !ok || reference.Loc() != wantTypeLoc || reference.Name() != mustTestQName(t, expected.namespace, expected.typeName) {
						t.Fatalf("%s element %d reference = %#v/%t, want located %s", owner.name, index, reference, ok, expected.typeName)
					}
					if index == 0 {
						assertIntBuiltinReference(t, reference, reference.Loc(), factVersion)
						bounds, hasBounds := reference.IntegerBounds()
						minFacet, hasMin := bounds.MinInclusiveFacet()
						maxFacet, hasMax := bounds.MaxInclusiveFacet()
						if !hasBounds || !hasMin || !hasMax || !minFacet.Loc().IsZero() || !maxFacet.Loc().IsZero() {
							t.Fatalf("built-in int bounds = %#v, want intrinsic bounds without fabricated locations", bounds)
						}
						for _, lexical := range []string{"-2147483649", "2147483648"} {
							value, parseErr := ParseStrictInteger(lexical, reference.Loc())
							if parseErr != nil {
								t.Fatalf("ParseStrictInteger(%q): %v", lexical, parseErr)
							}
							if err := bounds.ValidateInteger(value, reference.Loc()); err == nil {
								t.Fatalf("int intrinsic bounds accepted %s", lexical)
							}
						}
						id, hasID := element.TypeID()
						if hasID || !id.IsZero() {
							t.Fatalf("built-in int ID = %v/%t, want zero/false", id, hasID)
						}
						if element.DeclaredType() != mustTestQName(t, testXSDNamespace, "int") || !element.IsNillable() || !reflect.DeepEqual(element.DisallowedSubstitutions(), []string{"substitution"}) {
							t.Fatalf("built-in int policy facts = %q/%t/%v", element.DeclaredType(), element.IsNillable(), element.DisallowedSubstitutions())
						}
						if element.DisallowedSubstitutionsLoc() != unsignedLongParticleTokenLocAfter(t, root, ownerMarker, `block="substitution"`) {
							t.Fatalf("built-in int block location = %s, want explicit block attribute", element.DisallowedSubstitutionsLoc())
						}
						continue
					}
					if !reference.IsNamed() || element.DeclaredType() != reference.Name() {
						t.Fatalf("%s element %d lost named type QName", owner.name, index)
					}
					id, hasID := element.TypeID()
					refID, refHasID := reference.ComponentID()
					if !hasID || !refHasID || id != refID || id.Source() != SourceID(expected.source) {
						t.Fatalf("%s element %d IDs = %v/%t and %v/%t, want source %s", owner.name, index, id, hasID, refID, refHasID, expected.source)
					}
					assertIntegerReferenceFacts(t, reference.facts, factVersion, schemaSimpleTypeAtomicInt, "int", expected.minimum, expected.maximum)
					definition := first.FindKind(ComponentKindSimpleTypeDefinition, reference.Name())
					if len(definition) != 1 || definition[0].ID() != id {
						t.Fatalf("%s element %d resolved named type does not own ID %v", owner.name, index, id)
					}
					bounds, hasBounds := reference.IntegerBounds()
					if !hasBounds {
						t.Fatalf("%s element %d has no effective int bounds", owner.name, index)
					}
					if index > 1 {
						minFacet, _ := bounds.MinInclusiveFacet()
						maxFacet, _ := bounds.MaxInclusiveFacet()
						if minFacet.Loc().Source() != SourceID(expected.source) || maxFacet.Loc().Source() != SourceID(expected.source) {
							t.Fatalf("%s element %d facets lost %s provenance", owner.name, index, expected.source)
						}
					}
				}
				elements[0] = ElementParticle{}
			}
			if !reflect.DeepEqual(before, first.Components()) {
				t.Fatal("mutating copied int particle views changed the schema")
			}
		})
	}
}

func intParticleGraph(version XSDVersion) (string, map[string]discoveryFixture) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="` + string(version) + `">
  <xs:include schemaLocation="ordinary.xsd"/><xs:include schemaLocation="chameleon.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:complexType name="Base"/>` + intParticleOwner("Choice", "choice", false) + intParticleOwner("Sequence", "sequence", false) + intParticleOwner("ExtendedChoice", "choice", true) + intParticleOwner("ExtendedSequence", "sequence", true) + `
  <xs:simpleType name="Forward"><xs:restriction base="r:Later"/></xs:simpleType>
  <xs:simpleType name="Later"><xs:restriction base="xs:int"><xs:minInclusive value="-100"/><xs:maxInclusive value="100"/></xs:restriction></xs:simpleType>
</xs:schema>`
	return root, map[string]discoveryFixture{
		"ordinary.xsd":  {id: "ordinary.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:simpleType name="Included"><xs:restriction base="xs:int"><xs:minInclusive value="-10"/><xs:maxInclusive value="10"/></xs:restriction></xs:simpleType></xs:schema>`},
		"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Chameleon"><xs:restriction base="xs:int"><xs:minInclusive value="-20"/><xs:maxInclusive value="20"/></xs:restriction></xs:simpleType></xs:schema>`},
		"other.xsd":     {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Imported"><xs:restriction base="xs:int"><xs:minInclusive value="-30"/><xs:maxInclusive value="30"/></xs:restriction></xs:simpleType></xs:schema>`},
	}
}

func intParticleOwner(name, model string, extension bool) string {
	children := `<xs:element name="builtin" type="xs:int" form="qualified" nillable="true" block="substitution" minOccurs="2" maxOccurs="18446744073709551616"/>` +
		`<xs:element name="forward" type="r:Forward" maxOccurs="unbounded"/>` +
		`<xs:element name="included" type="r:Included"/>` +
		`<xs:element name="chameleon" type="r:Chameleon"/>` +
		`<xs:element name="imported" type="o:Imported"/>` +
		`<xs:element name="omitted" type="xs:int" minOccurs="0" maxOccurs="0"/>`
	occurrences := ` minOccurs="0" maxOccurs="unbounded"`
	if model == "sequence" {
		occurrences = ` minOccurs="0" maxOccurs="18446744073709551617"`
	}
	content := `<xs:` + model + occurrences + `>` + children + `</xs:` + model + `>`
	if extension {
		content = `<xs:complexContent><xs:extension base="r:Base">` + content + `</xs:extension></xs:complexContent>`
	}
	return `<xs:complexType name="` + name + `">` + content + `</xs:complexType>`
}

//nolint:gocognit // Keep the edition, owner-shape, and extension omission matrix together.
func TestSchemaIntZeroOwnersOmitParticles(t *testing.T) {
	for _, profile := range intParticleProfiles() {
		for _, model := range []string{"choice", "sequence"} {
			for _, extension := range []bool{false, true} {
				content := `<xs:` + model + ` minOccurs="0" maxOccurs="0"><xs:element name="value" type="xs:int"/></xs:` + model + `>`
				if extension {
					content = `<xs:complexContent><xs:extension base="r:Base">` + content + `</xs:extension></xs:complexContent>`
				}
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:complexType name="Base"/><xs:complexType name="Record">` + content + `</xs:complexType></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("%s/%s/extension=%t: %v", profile.name, model, extension, err)
				}
				if particle := requireUnsignedLongParticleComplexType(t, schema, "Record").Particle(); particle != nil {
					t.Fatalf("%s/%s/extension=%t: zero owner published %T", profile.name, model, extension, particle)
				}
			}
		}
	}
}

//nolint:gocognit // Distinguish invalid, omitted, and excluded forms under each edition policy.
func TestSchemaIntLocalParticleGatesAndConsumers(t *testing.T) {
	for _, profile := range intParticleProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			for _, test := range []struct {
				name, body, defs string
				cause            error
				class            FailureClass
			}{
				{name: "unresolved", body: `<xs:element name="value" type="r:Missing"/>`, cause: errSchemaElementTypeUnresolved, class: FailureInvalid},
				{name: "wrong kind", body: `<xs:element name="value" type="r:NotType"/>`, defs: `<xs:element name="NotType" type="xs:int"/>`, cause: errSchemaElementTypeWrongKind, class: FailureInvalid},
				{name: "cycle", body: `<xs:element name="value" type="r:A"/>`, defs: `<xs:simpleType name="A"><xs:restriction base="r:B"/></xs:simpleType><xs:simpleType name="B"><xs:restriction base="r:A"/></xs:simpleType>`, cause: errSchemaSimpleTypeBaseCycle, class: FailureInvalid},
				{name: "invalid occurrence", body: `<xs:element name="value" type="xs:int" minOccurs="not-a-number"/>`, class: FailureInvalid},
				{name: "malformed QName", body: `<xs:element name="value" type="r:bad:q"/>`, class: FailureInvalid},
				{name: "invalid intrinsic bound", body: `<xs:element name="value" type="r:Bad" minOccurs="0" maxOccurs="0"/>`, defs: `<xs:simpleType name="Bad"><xs:restriction base="xs:int"><xs:maxInclusive value="2147483648"/></xs:restriction></xs:simpleType>`, cause: errInvalidBoundRestriction, class: FailureInvalid},
				{name: "inline", body: `<xs:element name="value"><xs:simpleType><xs:restriction base="xs:int"/></xs:simpleType></xs:element>`, cause: ErrUnsupported, class: FailureUnsupported},
				{name: "value constraint", body: `<xs:element name="value" type="xs:int" default="1"/>`, cause: ErrUnsupported, class: FailureUnsupported},
				{name: "nested", body: `<xs:sequence><xs:element name="value" type="xs:int"/></xs:sequence>`, cause: ErrUnsupported, class: FailureUnsupported},
			} {
				t.Run(test.name, func(t *testing.T) {
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:complexType name="Record"><xs:choice>` + test.body + `</xs:choice></xs:complexType>` + test.defs + `</xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertZeroSchema(t, schema)
					if err == nil || test.cause != nil && !errors.Is(err, test.cause) {
						t.Fatalf("error = %v, want cause %v", err, test.cause)
					}
					diagnostic := requireDiagnostic(t, err)
					if diagnostic.Class() != test.class || diagnostic.Loc().IsZero() || test.name != "malformed QName" && diagnostic.SpecRef() == "" {
						t.Fatalf("diagnostic = %s, want located %s with spec", diagnostic, test.class)
					}
				})
			}
			for _, model := range []string{"choice", "sequence"} {
				for _, typeName := range []string{"xs:int", "r:Named"} {
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:element name="root" type="r:Record"/><xs:complexType name="Record"><xs:` + model + `><xs:element name="value" type="` + typeName + `"/></xs:` + model + `></xs:complexType><xs:simpleType name="Named"><xs:restriction base="xs:int"/></xs:simpleType></xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err != nil {
						t.Fatalf("%s schema: %v", model, err)
					}
					generated, err := GenerateGo(schema, "generated")
					if generated != nil || err == nil || !errors.Is(err, ErrUnsupported) {
						t.Fatalf("%s generation = %q/%v, want explicit unsupported", model, generated, err)
					}
					generationDiagnostic := requireDiagnostic(t, err)
					if generationDiagnostic.Class() != FailureUnsupported || generationDiagnostic.Code() != diagnosticCodegenUnsupported || generationDiagnostic.Loc().IsZero() {
						t.Fatalf("%s generation diagnostic = %s, want located unsupported", model, generationDiagnostic)
					}
					err = ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"><value xmlns="">1</value></root>`)))
					if err == nil || !errors.Is(err, ErrUnsupported) {
						t.Fatalf("%s validation = %v, want explicit unsupported", model, err)
					}
					validationDiagnostic := requireDiagnostic(t, err)
					if validationDiagnostic.Class() != FailureUnsupported || validationDiagnostic.Code() != UnsupportedInstanceValidationCode || validationDiagnostic.Loc().IsZero() {
						t.Fatalf("%s validation diagnostic = %s, want located unsupported", model, validationDiagnostic)
					}
				}
			}
		})
	}
}

func TestSchemaIntLocalInlineConsumerGates(t *testing.T) {
	for _, profile := range intParticleProfiles() {
		for _, model := range []string{"choice", "sequence"} {
			t.Run(profile.name+"/"+model, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="` + string(profile.version) + `"><xs:complexType name="Record"><xs:` + model + `><xs:element name="value"><xs:simpleType><xs:restriction base="xs:int"/></xs:simpleType></xs:element></xs:` + model + `></xs:complexType></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				wantLoc := complexContentTestLoc(t, root, `<xs:simpleType>`)
				diagnosticVersion := profile.version
				if profile.policy == Compatibility {
					diagnosticVersion = XSDVersion11
				}
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != wantLoc || d.SpecRef() != schemaSyntaxSpecRefForVersion(diagnosticVersion) || len(d.Related()) != 0 || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("%s inline int exclusion = %s, related %v; want %s", model, d, d.Related(), wantLoc)
				}
			})
		}
	}
}
