package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

type byteParticleProfile struct {
	name    string
	policy  LanguagePolicy
	version XSDVersion
}

func byteParticleProfiles() []byteParticleProfile {
	return []byteParticleProfile{
		{name: "compatible 1.0", policy: Compatibility, version: XSDVersion10},
		{name: "strict 1.0", policy: Strict10, version: XSDVersion10},
		{name: "compatible 1.1", policy: Compatibility, version: XSDVersion11},
		{name: "strict 1.1", policy: Strict11, version: XSDVersion11},
	}
}

//nolint:gocognit,funlen // Exercise the four owner shapes against one graph and exact fact contract.
func TestSchemaByteLocalParticlesAcrossGraphsAndOwners(t *testing.T) {
	for _, profile := range byteParticleProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root, fixtures := byteParticleGraph(profile.version)
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			again, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			before := schema.Components()
			if !reflect.DeepEqual(before, again.Components()) || len(schema.Documents()) != 4 {
				t.Fatal("byte graph discovery changed component order or document count")
			}
			factVersion := profile.version
			if profile.policy == Compatibility {
				factVersion = XSDVersion11
			}
			for _, owner := range []struct {
				name, model, occurrence string
				extension               bool
			}{
				{name: "Choice", model: "choice", occurrence: "0/unbounded"},
				{name: "Sequence", model: "sequence", occurrence: "0/18446744073709551617"},
				{name: "ExtendedChoice", model: "choice", occurrence: "0/unbounded", extension: true},
				{name: "ExtendedSequence", model: "sequence", occurrence: "0/18446744073709551617", extension: true},
			} {
				definition := requireUnsignedLongParticleComplexType(t, schema, owner.name)
				if owner.extension && (definition.Derivation() != ComplexTypeDerivationExtension || definition.Base() != mustTestQName(t, "urn:root", "Base")) {
					t.Fatalf("%s lost extension base", owner.name)
				}
				if definition.Particle().Occurrences().String() != owner.occurrence {
					t.Fatalf("%s occurrence = %s", owner.name, definition.Particle().Occurrences())
				}
				children := byteParticleChildren(t, definition.Particle(), owner.model)
				if len(children) != 5 {
					t.Fatalf("%s children = %d, want 5", owner.name, len(children))
				}
				marker := `<xs:complexType name="` + owner.name + `">`
				for index, expected := range []struct{ local, lexical, namespace, source, min, max, occurrence string }{
					{local: "builtin", lexical: "xs:byte", namespace: testXSDNamespace, occurrence: "2/18446744073709551616"},
					{local: "forward", lexical: "r:Forward", namespace: "urn:root", source: "root.xsd", min: "-100", max: "100", occurrence: "1/unbounded"},
					{local: "included", lexical: "r:Included", namespace: "urn:root", source: "ordinary.xsd", min: "-10", max: "10", occurrence: "1/1"},
					{local: "chameleon", lexical: "r:Chameleon", namespace: "urn:root", source: "chameleon.xsd", min: "-20", max: "20", occurrence: "1/1"},
					{local: "imported", lexical: "o:Imported", namespace: "urn:other", source: "other.xsd", min: "-30", max: "30", occurrence: "1/1"},
				} {
					child := children[index]
					wantNameNS := ""
					if index == 0 {
						wantNameNS = "urn:root"
					}
					if child.Name() != mustTestQName(t, wantNameNS, expected.local) || child.Occurrences().String() != expected.occurrence || child.Loc() != unsignedLongParticleTokenLocAfter(t, root, marker, `<xs:element name="`+expected.local+`"`) {
						t.Fatalf("%s child %d lost lexical name, location, or occurrence", owner.name, index)
					}
					ref, ok := child.TypeReference()
					wantType := mustTestQName(t, expected.namespace, strings.SplitN(expected.lexical, ":", 2)[1])
					if !ok || ref.Name() != wantType || ref.Loc() != unsignedLongParticleTokenLocAfter(t, root, marker, `type="`+expected.lexical+`"`) {
						t.Fatalf("%s child %d lost type reference location or QName", owner.name, index)
					}
					if index == 0 {
						assertByteParticleBuiltinBounds(t, ref, factVersion)
						assertByteParticleBuiltinMetadata(t, child, root, marker)
						continue
					}
					id, hasID := child.TypeID()
					refID, refHasID := ref.ComponentID()
					if !ref.IsNamed() || !hasID || !refHasID || id != refID || id.Source() != SourceID(expected.source) {
						t.Fatalf("%s child %d lost named identity", owner.name, index)
					}
					found := schema.FindKind(ComponentKindSimpleTypeDefinition, ref.Name())
					if len(found) != 1 || found[0].ID() != id {
						t.Fatalf("%s child %d lost graph owner", owner.name, index)
					}
					bounds, ok := ref.IntegerBounds()
					if !ok {
						t.Fatalf("%s child %d lost bounds", owner.name, index)
					}
					low, lowOK := bounds.MinInclusiveFacet()
					high, highOK := bounds.MaxInclusiveFacet()
					if !lowOK || !highOK || low.Value().Canonical() != expected.min || high.Value().Canonical() != expected.max {
						t.Fatalf("%s child %d bounds = %v", owner.name, index, bounds)
					}
					if index > 1 && (low.Loc().Source() != SourceID(expected.source) || high.Loc().Source() != SourceID(expected.source)) {
						t.Fatalf("%s child %d lost facet provenance", owner.name, index)
					}
					bounds.lower.loc = Loc{}
					copied, _ := ref.IntegerBounds()
					copiedLow, _ := copied.MinInclusiveFacet()
					if index > 1 && copiedLow.Loc().Source() != SourceID(expected.source) {
						t.Fatal("bounds view aliased schema")
					}
				}
				children[0] = ElementParticle{}
			}
			if !reflect.DeepEqual(before, schema.Components()) {
				t.Fatal("particle views aliased schema")
			}
		})
	}
}

func byteParticleChildren(t *testing.T, particle Particle, model string) []ElementParticle {
	t.Helper()
	if model == "sequence" {
		sequence, ok := particle.(SequenceParticle)
		if !ok {
			t.Fatalf("particle = %T, want sequence", particle)
		}
		return sequence.Elements()
	}
	choice, ok := particle.(ChoiceParticle)
	if !ok {
		t.Fatalf("particle = %T, want choice", particle)
	}
	children := make([]ElementParticle, 0, len(choice.Alternatives()))
	for _, alternative := range choice.Alternatives() {
		children = append(children, requireUnsignedLongElementParticle(t, alternative))
	}
	return children
}

func assertByteParticleBuiltinBounds(t *testing.T, reference SimpleTypeReference, version XSDVersion) {
	t.Helper()
	assertByteBuiltinReference(t, reference, reference.Loc(), version)
	bounds, hasBounds := reference.IntegerBounds()
	minimum, hasMin := bounds.MinInclusiveFacet()
	maximum, hasMax := bounds.MaxInclusiveFacet()
	if !hasBounds || !hasMin || !hasMax || !minimum.Loc().IsZero() || !maximum.Loc().IsZero() {
		t.Fatalf("built-in byte bounds = %#v, want intrinsic bounds without fabricated locations", bounds)
	}
	for _, lexical := range []string{"-129", "128"} {
		value, err := ParseStrictInteger(lexical, reference.Loc())
		if err != nil {
			t.Fatalf("ParseStrictInteger(%q): %v", lexical, err)
		}
		if err := bounds.ValidateInteger(value, reference.Loc()); err == nil {
			t.Fatalf("byte intrinsic bounds accepted %s", lexical)
		}
	}
}

func assertByteParticleBuiltinMetadata(t *testing.T, element ElementParticle, root, ownerMarker string) {
	t.Helper()
	id, hasID := element.TypeID()
	if hasID || !id.IsZero() {
		t.Fatalf("built-in byte ID = %v/%t, want zero/false", id, hasID)
	}
	if element.DeclaredType() != mustTestQName(t, testXSDNamespace, "byte") || !element.IsNillable() || !reflect.DeepEqual(element.DisallowedSubstitutions(), []string{"substitution"}) {
		t.Fatalf("built-in byte policy facts = %q/%t/%v", element.DeclaredType(), element.IsNillable(), element.DisallowedSubstitutions())
	}
	if element.DisallowedSubstitutionsLoc() != unsignedLongParticleTokenLocAfter(t, root, ownerMarker, `block="substitution"`) {
		t.Fatalf("built-in byte block location = %s, want explicit block attribute", element.DisallowedSubstitutionsLoc())
	}
}

func byteParticleGraph(version XSDVersion) (string, map[string]discoveryFixture) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="` + string(version) + `">
  <xs:include schemaLocation="ordinary.xsd"/><xs:include schemaLocation="chameleon.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:complexType name="Base"/>` + byteParticleOwner("Choice", "choice", false) + byteParticleOwner("Sequence", "sequence", false) + byteParticleOwner("ExtendedChoice", "choice", true) + byteParticleOwner("ExtendedSequence", "sequence", true) + `
  <xs:simpleType name="Forward"><xs:restriction base="r:Later"/></xs:simpleType>
  <xs:simpleType name="Later"><xs:restriction base="xs:byte"><xs:minInclusive value="-100"/><xs:maxInclusive value="100"/></xs:restriction></xs:simpleType>
</xs:schema>`
	return root, map[string]discoveryFixture{
		"ordinary.xsd":  {id: "ordinary.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:simpleType name="Included"><xs:restriction base="xs:byte"><xs:minInclusive value="-10"/><xs:maxInclusive value="10"/><xs:enumeration value="-10"/><xs:enumeration value="10"/></xs:restriction></xs:simpleType></xs:schema>`},
		"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Chameleon"><xs:restriction base="xs:byte"><xs:minInclusive value="-20"/><xs:maxInclusive value="20"/></xs:restriction></xs:simpleType></xs:schema>`},
		"other.xsd":     {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Imported"><xs:restriction base="xs:byte"><xs:minInclusive value="-30"/><xs:maxInclusive value="30"/></xs:restriction></xs:simpleType></xs:schema>`},
	}
}

func byteParticleOwner(name, model string, extension bool) string {
	children := `<xs:element name="builtin" type="xs:byte" form="qualified" nillable="true" block="substitution" minOccurs="2" maxOccurs="18446744073709551616"/>` +
		`<xs:element name="forward" type="r:Forward" maxOccurs="unbounded"/>` +
		`<xs:element name="included" type="r:Included"/>` +
		`<xs:element name="chameleon" type="r:Chameleon"/>` +
		`<xs:element name="imported" type="o:Imported"/>` +
		`<xs:element name="omitted" type="r:Forward" minOccurs="0" maxOccurs="0"/>`
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
func TestSchemaByteZeroOwnersOmitParticles(t *testing.T) {
	for _, profile := range byteParticleProfiles() {
		for _, model := range []string{"choice", "sequence"} {
			for _, extension := range []bool{false, true} {
				content := `<xs:` + model + ` minOccurs="0" maxOccurs="0"><xs:element name="value" type="xs:byte"/></xs:` + model + `>`
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
func TestSchemaByteLocalParticleGatesAndConsumers(t *testing.T) {
	for _, profile := range byteParticleProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			for _, test := range byteParticleFailureCases() {
				t.Run(test.name, func(t *testing.T) {
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:complexType name="Record"><xs:choice>` + test.body + `</xs:choice></xs:complexType>` + test.defs + `</xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertZeroSchema(t, schema)
					if err == nil || test.cause != nil && !errors.Is(err, test.cause) {
						t.Fatalf("error = %v, want cause %v", err, test.cause)
					}
					diagnostic := requireDiagnostic(t, err)
					if diagnostic.Class() != test.class || diagnostic.Loc().IsZero() || !strings.HasPrefix(test.name, "malformed QName") && diagnostic.SpecRef() == "" {
						t.Fatalf("diagnostic = %s, want located %s with spec", diagnostic, test.class)
					}
				})
			}
			for _, model := range []string{"choice", "sequence"} {
				for _, typeName := range []string{"xs:byte", "r:Named"} {
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:element name="root" type="r:Record"/><xs:complexType name="Record"><xs:` + model + `><xs:element name="value" type="` + typeName + `"/></xs:` + model + `></xs:complexType><xs:simpleType name="Named"><xs:restriction base="xs:byte"/></xs:simpleType></xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err != nil {
						t.Fatalf("%s schema: %v", model, err)
					}
					wantLoc := elementReferenceTestAttributeLoc(t, root, `<xs:element name="value"`)
					assertByteParticleGenerationUnsupported(t, schema, model, wantLoc)
					assertByteParticleValidationUnsupported(t, schema, model, wantLoc)
				}
			}
		})
	}
}

func assertByteParticleGenerationUnsupported(t *testing.T, schema Schema, model string, wantLoc Loc) {
	t.Helper()
	generated, err := GenerateGo(schema, "generated")
	if generated != nil || err == nil || !errors.Is(err, ErrUnsupported) {
		t.Fatalf("%s generation = %q/%v, want explicit unsupported", model, generated, err)
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != diagnosticCodegenUnsupported || diagnostic.Loc() != wantLoc {
		t.Fatalf("%s generation diagnostic = %s, want located unsupported", model, diagnostic)
	}
}

func assertByteParticleValidationUnsupported(t *testing.T, schema Schema, model string, wantLoc Loc) {
	t.Helper()
	if model == "sequence" {
		wantLoc = mustTestLoc(t, "instance.xml", 1, 1)
	}
	err := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"><value xmlns="">1</value></root>`)))
	if err == nil || !errors.Is(err, ErrUnsupported) {
		t.Fatalf("%s validation = %v, want explicit unsupported", model, err)
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedInstanceValidationCode || diagnostic.Loc() != wantLoc {
		t.Fatalf("%s validation diagnostic = %s, want located unsupported", model, diagnostic)
	}
}

//nolint:gocognit // Keep mapped inline exclusions in the same owner-shape matrix.
func TestSchemaByteInlineLocalParticleExclusions(t *testing.T) {
	for _, profile := range byteParticleProfiles() {
		for _, shape := range []struct {
			name, model string
			extension   bool
		}{
			{name: "direct choice", model: "choice"},
			{name: "direct sequence", model: "sequence"},
			{name: "extension choice", model: "choice", extension: true},
			{name: "extension sequence", model: "sequence", extension: true},
		} {
			t.Run(profile.name+"/"+shape.name, func(t *testing.T) {
				content := `<xs:` + shape.model + `><xs:element name="value"><xs:simpleType><xs:restriction base="xs:byte"/></xs:simpleType></xs:element></xs:` + shape.model + `>`
				if shape.extension {
					content = `<xs:complexContent><xs:extension base="r:Base">` + content + `</xs:extension></xs:complexContent>`
				}
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:complexType name="Base"/><xs:complexType name="Record">` + content + `</xs:complexType></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				if err == nil || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("inline byte = %v, want unsupported", err)
				}
				diagnostic := requireDiagnostic(t, err)
				wantLoc := elementReferenceTestAttributeLoc(t, root, `<xs:simpleType>`)
				wantVersion := profile.version
				if profile.policy == Compatibility {
					wantVersion = XSDVersion11
				}
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != schemaSyntaxSpecRefForVersion(wantVersion) || len(diagnostic.Related()) != 0 {
					t.Fatalf("inline byte diagnostic = %s, want located schema unsupported at %s", diagnostic, wantLoc)
				}
			})
		}
	}
}

func byteParticleFailureCases() []struct {
	name, body, defs string
	cause            error
	class            FailureClass
} {
	return []struct {
		name, body, defs string
		cause            error
		class            FailureClass
	}{
		{name: "unresolved", body: `<xs:element name="value" type="r:Missing"/>`, cause: errSchemaElementTypeUnresolved, class: FailureInvalid},
		{name: "element reference zero", body: `<xs:element ref="r:Missing" minOccurs="0" maxOccurs="0"/>`, cause: errSchemaElementReferenceUnresolved, class: FailureInvalid},
		{name: "cycle zero", body: `<xs:element name="value" type="r:A" minOccurs="0" maxOccurs="0"/>`, defs: `<xs:simpleType name="A"><xs:restriction base="r:B"/></xs:simpleType><xs:simpleType name="B"><xs:restriction base="r:A"/></xs:simpleType>`, cause: errSchemaSimpleTypeBaseCycle, class: FailureInvalid},
		{name: "invalid occurrence zero", body: `<xs:element name="value" type="xs:byte" minOccurs="not-a-number" maxOccurs="0"/>`, class: FailureInvalid},
		{name: "invalid intrinsic bound", body: `<xs:element name="value" type="r:Bad" minOccurs="0" maxOccurs="0"/>`, defs: `<xs:simpleType name="Bad"><xs:restriction base="xs:byte"><xs:maxInclusive value="128"/></xs:restriction></xs:simpleType>`, cause: errInvalidBoundRestriction, class: FailureInvalid},
		{name: "nested", body: `<xs:sequence><xs:element name="value" type="xs:byte"/></xs:sequence>`, cause: ErrUnsupported, class: FailureUnsupported},
	}
}
