package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit,funlen // Check one graph across every supported owner and policy.
func TestSchemaNonNegativeIntegerParticleGraph(t *testing.T) {
	for _, profile := range nonNegativeIntegerPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root, fixtures := nonNegativeIntegerParticleGraph(profile.version)
			first, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("discover schema: %v", err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil || !reflect.DeepEqual(first.Components(), second.Components()) {
				t.Fatalf("repeated graph facts differ: %v", err)
			}
			if len(first.Documents()) != 4 {
				t.Fatalf("documents = %d, want four", len(first.Documents()))
			}
			factVersion := profile.version
			if profile.policy == Compatibility {
				factVersion = XSDVersion11
			}
			for _, owner := range []struct {
				name, model string
				extension   bool
			}{
				{name: "Choice", model: "choice"},
				{name: "Sequence", model: "sequence"},
				{name: "ExtendedChoice", model: "choice", extension: true},
				{name: "ExtendedSequence", model: "sequence", extension: true},
			} {
				definition := requireUnsignedLongParticleComplexType(t, first, owner.name)
				if owner.extension && (definition.Derivation() != ComplexTypeDerivationExtension || definition.Base() != mustTestQName(t, "urn:root", "Base")) {
					t.Fatalf("%s extension facts = %q/%q", owner.name, definition.Derivation(), definition.Base())
				}
				var elements []ElementParticle
				if owner.model == "choice" {
					choice, ok := definition.Particle().(ChoiceParticle)
					if !ok || choice.Occurrences().String() != "0/unbounded" {
						t.Fatalf("%s particle = %T, want exact choice", owner.name, definition.Particle())
					}
					for _, alternative := range choice.Alternatives() {
						elements = append(elements, requireUnsignedLongElementParticle(t, alternative))
					}
				}
				if owner.model == "sequence" {
					sequence, ok := definition.Particle().(SequenceParticle)
					if !ok || sequence.Occurrences().String() != "0/18446744073709551617" {
						t.Fatalf("%s particle = %T, want exact sequence", owner.name, definition.Particle())
					}
					elements = sequence.Elements()
				}
				if len(elements) != 5 {
					t.Fatalf("%s children = %d, want five in lexical order", owner.name, len(elements))
				}
				for index, expected := range []struct {
					name, typeName, namespace, source, occurrence, minimum string
				}{
					{name: "builtin", typeName: "nonNegativeInteger", namespace: testXSDNamespace, occurrence: "2/18446744073709551616", minimum: "0"},
					{name: "forward", typeName: "Forward", namespace: "urn:root", source: "root.xsd", occurrence: "1/unbounded", minimum: "2"},
					{name: "included", typeName: "Included", namespace: "urn:root", source: "ordinary.xsd", occurrence: "1/1", minimum: "3"},
					{name: "chameleon", typeName: "Chameleon", namespace: "urn:root", source: "chameleon.xsd", occurrence: "1/1", minimum: "4"},
					{name: "imported", typeName: "Imported", namespace: "urn:other", source: "other.xsd", occurrence: "1/1", minimum: "5"},
				} {
					element := elements[index]
					marker := `<xs:complexType name="` + owner.name + `">`
					wantLoc := unsignedLongParticleTokenLocAfter(t, root, marker, `<xs:element name="`+expected.name+`"`)
					if element.Name().Local() != expected.name || element.Loc() != wantLoc || element.Occurrences().String() != expected.occurrence {
						t.Fatalf("%s child %d = %q/%s/%s", owner.name, index, element.Name(), element.Loc(), element.Occurrences())
					}
					reference, ok := element.TypeReference()
					lexical := "r:" + expected.typeName
					if index == 0 {
						lexical = "xs:nonNegativeInteger"
					}
					if index == 4 {
						lexical = "o:Imported"
					}
					wantTypeLoc := unsignedLongParticleTokenLocAfter(t, root, marker, `type="`+lexical+`"`)
					if !ok || reference.Name() != mustTestQName(t, expected.namespace, expected.typeName) || reference.Loc() != wantTypeLoc || element.DeclaredType() != reference.Name() {
						t.Fatalf("%s child %d reference = %#v/%t", owner.name, index, reference, ok)
					}
					if reference.facts == nil || reference.facts.atomicKind != schemaSimpleTypeAtomicNonNegativeInteger {
						t.Fatalf("%s child %d lost nonNegativeInteger facts", owner.name, index)
					}
					bounds, hasBounds := reference.IntegerBounds()
					minimum, hasMinimum := bounds.MinInclusiveFacet()
					if !hasBounds || !hasMinimum || bounds.Version() != factVersion || minimum.Value().Canonical() != expected.minimum {
						t.Fatalf("%s child %d minimum = %s/%t", owner.name, index, minimum.Value().Canonical(), hasMinimum)
					}
					if index == 0 {
						if !reference.IsBuiltin() || !minimum.Loc().IsZero() || !element.IsNillable() || !reflect.DeepEqual(element.DisallowedSubstitutions(), []string{"substitution"}) {
							t.Fatalf("%s built-in reference lost intrinsic or policy facts", owner.name)
						}
						id, hasID := element.TypeID()
						if hasID || !id.IsZero() {
							t.Fatalf("%s built-in type ID = %v/%t", owner.name, id, hasID)
						}
						continue
					}
					id, hasID := element.TypeID()
					refID, refHasID := reference.ComponentID()
					if !reference.IsNamed() || !hasID || !refHasID || id != refID || id.Source() != SourceID(expected.source) || minimum.Loc().Source() != SourceID(expected.source) {
						t.Fatalf("%s child %d lost named provenance: %v/%v/%s", owner.name, index, id, refID, minimum.Loc())
					}
				}
			}
			before := first.Components()
			before[0] = Component{}
			if !reflect.DeepEqual(first.Components(), second.Components()) {
				t.Fatal("mutating copied views changed immutable schema")
			}
		})
	}
}

func nonNegativeIntegerParticleGraph(version XSDVersion) (string, map[string]discoveryFixture) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="` + string(version) + `">
<xs:include schemaLocation="ordinary.xsd"/><xs:include schemaLocation="chameleon.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/>
<xs:complexType name="Base"/>` + nonNegativeIntegerParticleOwner("Choice", "choice", false) + nonNegativeIntegerParticleOwner("Sequence", "sequence", false) + nonNegativeIntegerParticleOwner("ExtendedChoice", "choice", true) + nonNegativeIntegerParticleOwner("ExtendedSequence", "sequence", true) + `
<xs:simpleType name="Forward"><xs:restriction base="r:Later"/></xs:simpleType>
<xs:simpleType name="Later"><xs:restriction base="xs:nonNegativeInteger"><xs:minInclusive value="2"/></xs:restriction></xs:simpleType>
</xs:schema>`
	return root, map[string]discoveryFixture{
		"ordinary.xsd":  {id: "ordinary.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:simpleType name="Included"><xs:restriction base="xs:nonNegativeInteger"><xs:minInclusive value="3"/></xs:restriction></xs:simpleType></xs:schema>`},
		"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Chameleon"><xs:restriction base="xs:nonNegativeInteger"><xs:minInclusive value="4"/></xs:restriction></xs:simpleType></xs:schema>`},
		"other.xsd":     {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Imported"><xs:restriction base="xs:nonNegativeInteger"><xs:minInclusive value="5"/></xs:restriction></xs:simpleType></xs:schema>`},
	}
}

func nonNegativeIntegerParticleOwner(name, model string, extension bool) string {
	children := `<xs:element name="builtin" type="xs:nonNegativeInteger" form="qualified" nillable="true" block="substitution" minOccurs="2" maxOccurs="18446744073709551616"/>` +
		`<xs:element name="forward" type="r:Forward" maxOccurs="unbounded"/>` +
		`<xs:element name="included" type="r:Included"/><xs:element name="chameleon" type="r:Chameleon"/><xs:element name="imported" type="o:Imported"/>` +
		`<xs:element name="omitted" type="xs:nonNegativeInteger" minOccurs="0" maxOccurs="0"/>`
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

//nolint:gocognit // Exercise omitted and consumer-only forms with both owner shapes.
func TestSchemaNonNegativeIntegerParticleOmissionAndConsumers(t *testing.T) {
	for _, profile := range nonNegativeIntegerPolicyProfiles() {
		for _, model := range []string{"choice", "sequence"} {
			for _, extension := range []bool{false, true} {
				content := `<xs:` + model + ` minOccurs="0" maxOccurs="0"><xs:element name="value" type="xs:nonNegativeInteger"/></xs:` + model + `>`
				if extension {
					content = `<xs:complexContent><xs:extension base="r:Base">` + content + `</xs:extension></xs:complexContent>`
				}
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:complexType name="Base"/><xs:complexType name="Record">` + content + `</xs:complexType></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("%s/%s/%t: %v", profile.name, model, extension, err)
				}
				if particle := requireUnsignedLongParticleComplexType(t, schema, "Record").Particle(); particle != nil {
					t.Fatalf("%s/%s/%t zero owner published %T", profile.name, model, extension, particle)
				}
			}
			for _, typeName := range []string{"xs:nonNegativeInteger", "r:Named"} {
				for _, extension := range []bool{false, true} {
					content := `<xs:` + model + `><xs:element name="value" type="` + typeName + `"/></xs:` + model + `>`
					if extension {
						content = `<xs:complexContent><xs:extension base="r:Base">` + content + `</xs:extension></xs:complexContent>`
					}
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:element name="root" type="r:Record"/><xs:complexType name="Base"/><xs:complexType name="Record">` + content + `</xs:complexType><xs:simpleType name="Named"><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType></xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err != nil {
						t.Fatalf("%s/%s schema: %v", model, typeName, err)
					}
					output, err := GenerateGo(schema, "generated")
					if output != nil || err == nil || !errors.Is(err, ErrUnsupported) {
						t.Fatalf("%s/%s generation = %q/%v", model, typeName, output, err)
					}
					if diagnostic := requireDiagnostic(t, err); diagnostic.Class() != FailureUnsupported || diagnostic.Code() != diagnosticCodegenUnsupported || diagnostic.Loc().IsZero() {
						t.Fatalf("generation diagnostic = %s", diagnostic)
					}
					err = ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"><value xmlns="">1</value></root>`)))
					if err == nil || !errors.Is(err, ErrUnsupported) {
						t.Fatalf("%s/%s validation = %v", model, typeName, err)
					}
					if diagnostic := requireDiagnostic(t, err); diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedInstanceValidationCode || diagnostic.Loc().IsZero() {
						t.Fatalf("validation diagnostic = %s", diagnostic)
					}
				}
			}
		}
	}
}

//nolint:gocognit // Preserve classification and source locations across owner shapes.
func TestSchemaNonNegativeIntegerParticleFailureBoundaries(t *testing.T) {
	for _, profile := range nonNegativeIntegerPolicyProfiles() {
		for _, owner := range []struct {
			model     string
			extension bool
		}{
			{model: "choice"}, {model: "sequence"},
			{model: "choice", extension: true}, {model: "sequence", extension: true},
		} {
			for _, test := range []struct {
				name, element, defs, marker string
				class                       FailureClass
				cause                       error
			}{
				{name: "invalid inherited bound", element: `<xs:element name="value" type="r:Bad"/>`, defs: `<xs:simpleType name="Bad"><xs:restriction base="xs:nonNegativeInteger"><xs:maxInclusive value="-1"/></xs:restriction></xs:simpleType>`, marker: `value="-1"`, class: FailureInvalid, cause: errInvalidBoundRestriction},
				{name: "unresolved", element: `<xs:element name="value" type="r:Missing"/>`, marker: `type="r:Missing"`, class: FailureInvalid, cause: errSchemaElementTypeUnresolved},
				{name: "wrong kind", element: `<xs:element name="value" type="r:NotType"/>`, defs: `<xs:element name="NotType" type="xs:nonNegativeInteger"/>`, marker: `type="r:NotType"`, class: FailureInvalid, cause: errSchemaElementTypeWrongKind},
				{name: "inline", element: `<xs:element name="value"><xs:simpleType><xs:restriction base="xs:nonNegativeInteger"/></xs:simpleType></xs:element>`, marker: `<xs:simpleType>`, class: FailureUnsupported, cause: ErrUnsupported},
				{name: "value constraint", element: `<xs:element name="value" type="xs:nonNegativeInteger" default="1"/>`, marker: `default="1"`, class: FailureUnsupported, cause: ErrUnsupported},
				{name: "nested", element: `<xs:sequence><xs:element name="value" type="xs:nonNegativeInteger"/></xs:sequence>`, marker: `<xs:sequence>`, class: FailureUnsupported, cause: ErrUnsupported},
			} {
				content := `<xs:` + owner.model + `>` + test.element + `</xs:` + owner.model + `>`
				if owner.extension {
					content = `<xs:complexContent><xs:extension base="r:Base">` + content + `</xs:extension></xs:complexContent>`
				}
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:complexType name="Base"/><xs:complexType name="Record">` + content + `</xs:complexType>` + test.defs + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				if err == nil || !errors.Is(err, test.cause) {
					t.Fatalf("%s/%s/%t/%s error = %v, want cause %v", profile.name, owner.model, owner.extension, test.name, err, test.cause)
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != test.class || diagnostic.Loc().IsZero() || diagnostic.SpecRef() == "" {
					t.Fatalf("%s/%s/%t/%s diagnostic = %s", profile.name, owner.model, owner.extension, test.name, diagnostic)
				}
				if test.name == "invalid inherited bound" {
					// The invalid facet may be reported at either the facet or its restriction.
					continue
				}
				wantLoc := elementReferenceTestAttributeLoc(t, root, test.marker)
				if test.name == "nested" {
					wantLoc = namedGroupLocAt(t, root, strings.LastIndex(root, test.marker))
				}
				if diagnostic.Loc() != wantLoc {
					t.Fatalf("%s/%s/%t/%s location = %s, want %s", profile.name, owner.model, owner.extension, test.name, diagnostic.Loc(), wantLoc)
				}
			}
		}
	}
}
