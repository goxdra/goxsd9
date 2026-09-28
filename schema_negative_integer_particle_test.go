package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func negativeIntegerParticleProfiles() []nonNegativeIntegerPolicyProfile {
	return []nonNegativeIntegerPolicyProfile{
		{name: "Compatibility 1.0", policy: Compatibility, version: XSDVersion10},
		{name: "Compatibility 1.1", policy: Compatibility, version: XSDVersion11},
		{name: "Strict 1.0", policy: Strict10, version: XSDVersion10},
		{name: "Strict 1.1", policy: Strict11, version: XSDVersion11},
	}
}

func negativeIntegerParticleFactVersion(profile nonNegativeIntegerPolicyProfile) XSDVersion {
	if profile.policy == Compatibility {
		return XSDVersion11
	}
	return profile.version
}

//nolint:gocognit // Exercise the four admitted owners and exact copied facts together.
func TestSchemaNegativeIntegerDirectParticlesAcrossPolicies(t *testing.T) {
	for _, profile := range negativeIntegerParticleProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := negativeIntegerParticleRoot(profile.version)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discover schema: %v", err)
			}
			again, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
				t.Fatalf("repeated component order/facts differ: %v", err)
			}
			for _, owner := range []struct {
				name, model, rangeText string
				extension              bool
			}{
				{name: "Choice", model: "choice", rangeText: "0/unbounded"},
				{name: "Sequence", model: "sequence", rangeText: "0/18446744073709551617"},
				{name: "ExtendedChoice", model: "choice", rangeText: "0/unbounded", extension: true},
				{name: "ExtendedSequence", model: "sequence", rangeText: "0/18446744073709551617", extension: true},
			} {
				definition := requireUnsignedLongParticleComplexType(t, schema, owner.name)
				if owner.extension && (definition.Derivation() != ComplexTypeDerivationExtension || definition.Base() != mustTestQName(t, "urn:root", "Base")) {
					t.Fatalf("%s extension facts = %q/%q", owner.name, definition.Derivation(), definition.Base())
				}
				var elements []ElementParticle
				if owner.model == "choice" {
					choice, ok := definition.Particle().(ChoiceParticle)
					if !ok || choice.Occurrences().String() != owner.rangeText {
						t.Fatalf("%s choice = %T/%s", owner.name, definition.Particle(), choice.Occurrences())
					}
					for _, alternative := range choice.Alternatives() {
						elements = append(elements, requireUnsignedLongElementParticle(t, alternative))
					}
				}
				if owner.model == "sequence" {
					sequence, ok := definition.Particle().(SequenceParticle)
					if !ok || sequence.Occurrences().String() != owner.rangeText {
						t.Fatalf("%s sequence = %T/%s", owner.name, definition.Particle(), sequence.Occurrences())
					}
					elements = sequence.Elements()
				}
				if len(elements) != 2 {
					t.Fatalf("%s children = %d, want two after 0/0 omission", owner.name, len(elements))
				}
				for index, expected := range []struct {
					name, occurrence string
				}{
					{name: "first", occurrence: "2/18446744073709551616"},
					{name: "second", occurrence: "1/unbounded"},
				} {
					element := elements[index]
					marker := `<xs:complexType name="` + owner.name + `">`
					wantLoc := unsignedLongParticleTokenLocAfter(t, root, marker, `<xs:element name="`+expected.name+`"`)
					wantTypeLoc := negativeIntegerParticleTypeLoc(t, root, owner.name, expected.name)
					reference, ok := element.TypeReference()
					if element.Name() != mustTestQName(t, "urn:root", expected.name) || element.Loc() != wantLoc || element.Occurrences().String() != expected.occurrence || element.DeclaredType() != mustTestQName(t, testXSDNamespace, "negativeInteger") {
						t.Fatalf("%s child %d facts = %q/%s/%s/%q", owner.name, index, element.Name(), element.Loc(), element.Occurrences(), element.DeclaredType())
					}
					if !ok || !reference.IsBuiltin() || reference.Name() != element.DeclaredType() || reference.Loc() != wantTypeLoc || reference.VarietyLoc() != wantTypeLoc {
						t.Fatalf("%s child %d reference = %#v/%t", owner.name, index, reference, ok)
					}
					if typeID, hasTypeID := element.TypeID(); hasTypeID || !typeID.IsZero() {
						t.Fatalf("%s child %d type ID = %v/%t", owner.name, index, typeID, hasTypeID)
					}
					if typeID, hasTypeID := reference.ComponentID(); hasTypeID || !typeID.IsZero() {
						t.Fatalf("%s child %d reference ID = %v/%t", owner.name, index, typeID, hasTypeID)
					}
					bounds, hasBounds := reference.IntegerBounds()
					maximum, hasMaximum := bounds.MaxInclusiveFacet()
					if !hasBounds || !hasMaximum || bounds.Version() != negativeIntegerParticleFactVersion(profile) || maximum.Kind() != BoundMaxInclusive || maximum.Value().Canonical() != "-1" || maximum.Loc() != wantTypeLoc {
						t.Fatalf("%s child %d bound = %v/%t", owner.name, index, maximum, hasMaximum)
					}
				}
				if !elements[0].IsNillable() || !reflect.DeepEqual(elements[0].DisallowedSubstitutions(), []string{"substitution"}) {
					t.Fatalf("%s first element lost local modifiers", owner.name)
				}
				elements[0] = ElementParticle{}
				if !reflect.DeepEqual(schema.Components(), again.Components()) {
					t.Fatalf("%s mutation changed schema facts", owner.name)
				}
			}
		})
	}
}

func negativeIntegerParticleRoot(version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(version) + `">
<xs:complexType name="Base"/>` +
		negativeIntegerParticleOwner("Choice", "choice", false) +
		negativeIntegerParticleOwner("Sequence", "sequence", false) +
		negativeIntegerParticleOwner("ExtendedChoice", "choice", true) +
		negativeIntegerParticleOwner("ExtendedSequence", "sequence", true) +
		`</xs:schema>`
}

func negativeIntegerParticleOwner(name, model string, extension bool) string {
	rangeText := ` minOccurs="0" maxOccurs="unbounded"`
	if model == "sequence" {
		rangeText = ` minOccurs="0" maxOccurs="18446744073709551617"`
	}
	content := `<xs:` + model + rangeText + `><xs:element name="first" type="xs:negativeInteger" form="qualified" nillable="true" block="substitution" minOccurs="2" maxOccurs="18446744073709551616"/><xs:element name="omitted" type="xs:negativeInteger" minOccurs="0" maxOccurs="0"/><xs:element name="second" type="xs:negativeInteger" form="qualified" maxOccurs="unbounded"/></xs:` + model + `>`
	if extension {
		content = `<xs:complexContent><xs:extension base="r:Base">` + content + `</xs:extension></xs:complexContent>`
	}
	return `<xs:complexType name="` + name + `">` + content + `</xs:complexType>`
}

func negativeIntegerParticleTypeLoc(t *testing.T, root, owner, element string) Loc {
	t.Helper()
	ownerAt := strings.Index(root, `<xs:complexType name="`+owner+`">`)
	if ownerAt < 0 {
		t.Fatalf("fixture has no owner %q", owner)
	}
	elementAt := strings.Index(root[ownerAt:], `<xs:element name="`+element+`"`)
	if elementAt < 0 {
		t.Fatalf("fixture has no element %q in owner %q", element, owner)
	}
	typeAt := strings.Index(root[ownerAt+elementAt:], `type="xs:negativeInteger"`)
	if typeAt < 0 {
		t.Fatalf("fixture has no built-in type on %q", element)
	}
	return namedGroupLocAt(t, root, ownerAt+elementAt+typeAt)
}

//nolint:gocognit,funlen // Exercise each consumer and the 0/0 owner/child boundaries.
func TestSchemaNegativeIntegerParticleOmissionAndConsumers(t *testing.T) {
	for _, profile := range negativeIntegerParticleProfiles() {
		for _, model := range []string{"choice", "sequence"} {
			for _, extension := range []bool{false, true} {
				for _, zeroOwner := range []bool{false, true} {
					ownerOccurs := ""
					childOccurs := ` minOccurs="0" maxOccurs="0"`
					if zeroOwner {
						ownerOccurs = childOccurs
						childOccurs = ""
					}
					content := `<xs:` + model + ownerOccurs + `><xs:element name="value" type="xs:negativeInteger"` + childOccurs + `/></xs:` + model + `>`
					if extension {
						content = `<xs:complexContent><xs:extension base="r:Base">` + content + `</xs:extension></xs:complexContent>`
					}
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:complexType name="Base"/><xs:complexType name="Record">` + content + `</xs:complexType></xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err != nil {
						t.Fatalf("%s/%s/%t/%t: %v", profile.name, model, extension, zeroOwner, err)
					}
					particle := requireUnsignedLongParticleComplexType(t, schema, "Record").Particle()
					if zeroOwner {
						if particle != nil {
							t.Fatalf("%s/%s/%t zero owner published %T", profile.name, model, extension, particle)
						}
						continue
					}
					switch typed := particle.(type) {
					case ChoiceParticle:
						if len(typed.Alternatives()) != 0 {
							t.Fatalf("zero child published %d choices", len(typed.Alternatives()))
						}
					case SequenceParticle:
						if len(typed.Particles()) != 0 {
							t.Fatalf("zero child published %d sequence particles", len(typed.Particles()))
						}
					default:
						t.Fatalf("zero child owner = %T", particle)
					}
				}
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:complexType name="Record">`
				content := `<xs:` + model + `><xs:element name="value" type="xs:negativeInteger"/></xs:` + model + `>`
				if extension {
					content = `<xs:complexContent><xs:extension base="r:Base">` + content + `</xs:extension></xs:complexContent>`
				}
				root += content + `</xs:complexType><xs:complexType name="Base"/><xs:element name="root" type="r:Record"/></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("consumer schema: %v", err)
				}
				wantParticleLoc := elementReferenceTestAttributeLoc(t, root, `<xs:element name="value"`)
				if extension {
					wantParticleLoc = elementReferenceTestAttributeLoc(t, root, `<xs:extension`)
				}
				output, generationErr := GenerateGo(schema, "generated")
				if output != nil || generationErr == nil || !errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenUnsupported) {
					t.Fatalf("generation = %q/%v", output, generationErr)
				}
				generationDiagnostic := requireDiagnostic(t, generationErr)
				factVersion := negativeIntegerParticleFactVersion(profile)
				wantGenerationSpec := codegenDirectChoiceSpecReference(factVersion, codegenDirectChoiceElementChoiceReference)
				if model == "sequence" {
					wantGenerationSpec = codegenDirectSequenceSpecReference(factVersion, codegenDirectSequenceElementReference)
				}
				if extension {
					wantGenerationSpec = codegenDirectSequenceSpecReference(factVersion, codegenDirectSequenceParticlesReference)
				}
				if generationDiagnostic.Class() != FailureUnsupported || generationDiagnostic.Code() != diagnosticCodegenUnsupported || generationDiagnostic.Loc() != wantParticleLoc || generationDiagnostic.SpecRef() != wantGenerationSpec {
					t.Fatalf("generation diagnostic = %s, want unsupported at %s", generationDiagnostic, wantParticleLoc)
				}
				validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"><value xmlns="">-1</value></root>`)))
				if validationErr == nil || !errors.Is(validationErr, ErrUnsupported) {
					t.Fatalf("validation = %v", validationErr)
				}
				validationDiagnostic := requireDiagnostic(t, validationErr)
				wantValidationLoc := mustTestLoc(t, "instance.xml", 1, 1)
				wantValidationCause := errInstanceUnsupportedType
				if model == "choice" {
					wantValidationLoc = elementReferenceTestAttributeLoc(t, root, `<xs:element name="value"`)
				}
				if extension {
					wantValidationCause = errInstanceComplexContentExtension
					if model == "choice" {
						wantValidationLoc = elementReferenceTestAttributeLoc(t, root, `<xs:extension`)
					}
				}
				if validationDiagnostic.Class() != FailureUnsupported || validationDiagnostic.Code() != UnsupportedInstanceValidationCode || validationDiagnostic.Loc() != wantValidationLoc || !errors.Is(validationErr, wantValidationCause) {
					t.Fatalf("validation diagnostic = %s", validationDiagnostic)
				}
				if extension && validationDiagnostic.SpecRef() != instanceValidationSpecRef(factVersion) {
					t.Fatalf("extension validation spec = %q, want %q", validationDiagnostic.SpecRef(), instanceValidationSpecRef(factVersion))
				}
			}
		}
	}
}

//nolint:gocognit // Verify discovery order and built-in identity in each visible graph document.
func TestSchemaNegativeIntegerDirectParticlesComposeGraphs(t *testing.T) {
	for _, profile := range negativeIntegerParticleProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:include schemaLocation="included.xsd"/><xs:include schemaLocation="chameleon.xsd"/><xs:import namespace="urn:other" schemaLocation="imported.xsd"/>` +
				negativeIntegerGraphOwner("Root", "choice") + `</xs:schema>`
			fixtures := map[string]discoveryFixture{
				"included.xsd":  {id: "included.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:include schemaLocation="root.xsd"/>` + negativeIntegerGraphOwner("Included", "sequence") + `</xs:schema>`},
				"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `">` + negativeIntegerGraphOwner("Chameleon", "choice") + `</xs:schema>`},
				"imported.xsd":  {id: "imported.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other">` + negativeIntegerGraphOwner("Imported", "sequence") + `</xs:schema>`},
				"root.xsd":      {id: "root.xsd", contents: root},
			}
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("graph schema: %v", err)
			}
			again, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
				t.Fatalf("graph facts changed between builds: %v", err)
			}
			documents := schema.Documents()
			if len(documents) != 4 {
				t.Fatalf("documents = %d, want four", len(documents))
			}
			for index, expected := range []struct {
				name, namespace, source, model string
			}{
				{name: "Root", namespace: "urn:root", source: "root.xsd", model: "choice"},
				{name: "Included", namespace: "urn:root", source: "included.xsd", model: "sequence"},
				{name: "Chameleon", namespace: "urn:root", source: "chameleon.xsd", model: "choice"},
				{name: "Imported", namespace: "urn:other", source: "imported.xsd", model: "sequence"},
			} {
				if documents[index].Source() != SourceID(expected.source) {
					t.Fatalf("document %d = %s, want %s", index, documents[index].Source(), expected.source)
				}
				matches := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, expected.namespace, expected.name))
				if len(matches) != 1 {
					t.Fatalf("%s matches = %d, want one", expected.name, len(matches))
				}
				definition, ok := matches[0].ComplexTypeDefinition()
				if !ok || definition.ID().Source() != SourceID(expected.source) {
					t.Fatalf("%s provenance = %v/%t", expected.name, definition.ID(), ok)
				}
				var element ElementParticle
				if expected.model == "choice" {
					choice, choiceOK := definition.Particle().(ChoiceParticle)
					if !choiceOK || len(choice.Alternatives()) != 1 {
						t.Fatalf("%s choice = %T", expected.name, definition.Particle())
					}
					element = requireUnsignedLongElementParticle(t, choice.Alternatives()[0])
				}
				if expected.model == "sequence" {
					sequence, sequenceOK := definition.Particle().(SequenceParticle)
					if !sequenceOK || len(sequence.Elements()) != 1 {
						t.Fatalf("%s sequence = %T", expected.name, definition.Particle())
					}
					element = sequence.Elements()[0]
				}
				reference, ok := element.TypeReference()
				if !ok || !reference.IsBuiltin() || reference.Name() != mustTestQName(t, testXSDNamespace, "negativeInteger") || reference.Loc().Source() != SourceID(expected.source) || element.Loc().Source() != SourceID(expected.source) {
					t.Fatalf("%s local reference = %#v/%t", expected.name, reference, ok)
				}
				bounds, ok := reference.IntegerBounds()
				maximum, present := bounds.MaxInclusive()
				if !ok || !present || maximum.Canonical() != "-1" {
					t.Fatalf("%s intrinsic maximum = %q/%t", expected.name, maximum.Canonical(), present)
				}
			}
		})
	}
}

func negativeIntegerGraphOwner(name, model string) string {
	return `<xs:complexType name="` + name + `"><xs:` + model + `><xs:element name="value" type="xs:negativeInteger"/></xs:` + model + `></xs:complexType>`
}

//nolint:gocognit // Confirm every supported local type shape remains query-only in both consumers.
func TestSchemaNegativeIntegerLocalShapesRejectConsumers(t *testing.T) {
	for _, profile := range negativeIntegerParticleProfiles() {
		for _, model := range []string{"choice", "sequence"} {
			for _, shape := range []struct {
				name, element, definition string
			}{
				{name: "named", element: `<xs:element name="value" type="r:Named"/>`, definition: `<xs:simpleType name="Named"><xs:restriction base="xs:negativeInteger"/></xs:simpleType>`},
				{name: "inline", element: `<xs:element name="value"><xs:simpleType><xs:restriction base="xs:negativeInteger"/></xs:simpleType></xs:element>`},
				{name: "ref", element: `<xs:element ref="r:value"/>`, definition: `<xs:element name="value" type="xs:negativeInteger"/>`},
			} {
				t.Run(profile.name+"/"+model+"/"+shape.name, func(t *testing.T) {
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:complexType name="Record"><xs:` + model + `>` + shape.element + `</xs:` + model + `></xs:complexType>` + shape.definition + `<xs:element name="root" type="r:Record"/></xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err != nil {
						t.Fatalf("schema: %v", err)
					}
					output, generationErr := GenerateGo(schema, "generated")
					if output != nil || generationErr == nil || !errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenUnsupported) {
						t.Fatalf("generation = %q/%v", output, generationErr)
					}
					generationDiagnostic := requireDiagnostic(t, generationErr)
					factVersion := negativeIntegerParticleFactVersion(profile)
					wantParticleLoc := elementReferenceTestAttributeLoc(t, root, `<xs:element name="value"`)
					wantGenerationLoc := wantParticleLoc
					wantGenerationSpec := codegenDirectChoiceSpecReference(factVersion, codegenDirectChoiceElementChoiceReference)
					if model == "sequence" {
						wantGenerationSpec = codegenDirectSequenceSpecReference(factVersion, codegenDirectSequenceElementReference)
					}
					if shape.name == "named" {
						wantGenerationSpec = schemaSimpleTypeSpecRef(factVersion)
					}
					if shape.name == "ref" {
						wantGenerationLoc = elementReferenceTestAttributeLoc(t, root, `ref="r:value"`)
					}
					if generationDiagnostic.Class() != FailureUnsupported || generationDiagnostic.Code() != diagnosticCodegenUnsupported || generationDiagnostic.Loc() != wantGenerationLoc || generationDiagnostic.SpecRef() != wantGenerationSpec {
						t.Fatalf("generation diagnostic = %s", generationDiagnostic)
					}
					validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"><value>-1</value></root>`)))
					if validationErr == nil || !errors.Is(validationErr, ErrUnsupported) {
						t.Fatalf("validation = %v", validationErr)
					}
					validationDiagnostic := requireDiagnostic(t, validationErr)
					wantValidationLoc := wantParticleLoc
					if (model == "sequence" && shape.name != "inline") || shape.name == "ref" {
						wantValidationLoc = mustTestLoc(t, "instance.xml", 1, 1)
					}
					wantValidationSpec := instanceValidationSpecRef(factVersion)
					if shape.name == "ref" && model == "choice" {
						wantValidationSpec = instanceValidationSpecRef(XSDVersion11)
					}
					if validationDiagnostic.Class() != FailureUnsupported || validationDiagnostic.Code() != UnsupportedInstanceValidationCode || validationDiagnostic.Loc() != wantValidationLoc || validationDiagnostic.SpecRef() != wantValidationSpec {
						t.Fatalf("validation diagnostic = %s", validationDiagnostic)
					}
				})
			}
		}
	}
}

//nolint:gocognit // Preserve the nearby resolution and exclusion exits for each admitted owner.
func TestSchemaNegativeIntegerParticleBoundaryDiagnostics(t *testing.T) {
	for _, profile := range negativeIntegerParticleProfiles() {
		for _, model := range []string{"choice", "sequence"} {
			for _, extension := range []bool{false, true} {
				for _, test := range []struct {
					name, body, definitions, marker, code string
					class                                 FailureClass
					cause                                 error
					related                               string
				}{
					{name: "unresolved mapped child", body: `<xs:element name="value" type="r:Missing"/>`, marker: `type="r:Missing"`, code: diagnosticSchemaElementTypeUnresolvedCode, class: FailureInvalid, cause: errSchemaElementTypeUnresolved},
					{name: "wrong kind mapped child", body: `<xs:element name="value" type="r:NotType"/>`, definitions: `<xs:element name="NotType" type="xs:negativeInteger"/>`, marker: `type="r:NotType"`, related: `<xs:element name="NotType"`, code: diagnosticSchemaElementTypeWrongKindCode, class: FailureInvalid, cause: errSchemaElementTypeWrongKind},
					{name: "nested direct", body: `<xs:sequence><xs:element name="value" type="xs:negativeInteger"/></xs:sequence>`, marker: `<xs:sequence>`, code: UnsupportedSchemaSyntaxCode, class: FailureUnsupported, cause: ErrUnsupported},
					{name: "nested named", body: `<xs:sequence><xs:element name="value" type="r:Named"/></xs:sequence>`, definitions: `<xs:simpleType name="Named"><xs:restriction base="xs:negativeInteger"/></xs:simpleType>`, marker: `<xs:sequence>`, code: UnsupportedSchemaSyntaxCode, class: FailureUnsupported, cause: ErrUnsupported},
					{name: "nested inline", body: `<xs:sequence><xs:element name="value"><xs:simpleType><xs:restriction base="xs:negativeInteger"/></xs:simpleType></xs:element></xs:sequence>`, marker: `<xs:sequence>`, code: UnsupportedSchemaSyntaxCode, class: FailureUnsupported, cause: ErrUnsupported},
					{name: "nested ref", body: `<xs:sequence><xs:element ref="r:target"/></xs:sequence>`, definitions: `<xs:element name="target" type="xs:negativeInteger"/>`, marker: `<xs:sequence>`, code: UnsupportedSchemaSyntaxCode, class: FailureUnsupported, cause: ErrUnsupported},
					{name: "excluded long", body: `<xs:element name="value" type="xs:long"/>`, marker: `type="xs:long"`, code: UnsupportedSchemaSyntaxCode, class: FailureUnsupported, cause: ErrUnsupported},
				} {
					subtest := profile.name + "/" + model + "/" + test.name
					if extension {
						subtest += "/extension"
					}
					t.Run(subtest, func(t *testing.T) {
						body := `<xs:` + model + `>` + test.body + `</xs:` + model + `>`
						if extension {
							body = `<xs:complexContent><xs:extension base="r:Base">` + body + `</xs:extension></xs:complexContent>`
						}
						root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:complexType name="Base"/><xs:complexType name="Record">` + body + `</xs:complexType>` + test.definitions + `</xs:schema>`
						schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
						assertZeroSchema(t, schema)
						if err == nil || !errors.Is(err, test.cause) {
							t.Fatalf("schema error = %v, want cause %v", err, test.cause)
						}
						diagnostic := requireDiagnostic(t, err)
						factVersion := negativeIntegerParticleFactVersion(profile)
						wantSpec := schemaSyntaxSpecRefForVersion(factVersion)
						if test.class == FailureInvalid {
							wantSpec = schemaElementTypeSpecRef(factVersion)
						}
						if strings.HasPrefix(test.name, "nested") && !extension {
							wantSpec = schemaSyntaxSpecRefForVersion(XSDVersion10)
						}
						if strings.HasPrefix(test.name, "nested") && extension {
							wantSpec = schemaComplexParticleExtensionSpecRef(factVersion)
						}
						wantLoc := elementReferenceTestAttributeLoc(t, root, test.marker)
						if model == "sequence" && strings.HasPrefix(test.name, "nested") {
							wantLoc = namedGroupLocAt(t, root, strings.LastIndex(root, test.marker))
						}
						if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != wantSpec {
							t.Fatalf("diagnostic = %s (spec %s), want %s/%s at %s with spec %s", diagnostic, diagnostic.SpecRef(), test.class, test.code, wantLoc, wantSpec)
						}
						var wantRelated []Loc
						if test.related != "" {
							wantRelated = []Loc{elementReferenceTestAttributeLoc(t, root, test.related)}
						}
						if !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
							t.Fatalf("related locations = %v, want %v", diagnostic.Related(), wantRelated)
						}
					})
				}
			}
		}
	}
}

//nolint:gocognit // Keep lexical and range failures paired across direct and extension owners.
func TestSchemaNegativeIntegerParticleOccurrenceDiagnostics(t *testing.T) {
	for _, profile := range negativeIntegerParticleProfiles() {
		for _, model := range []string{"choice", "sequence"} {
			for _, extension := range []bool{false, true} {
				for _, test := range []struct {
					name, occurrences, marker string
					related                   []string
					cause                     error
					spec                      func(XSDVersion) string
				}{
					{name: "malformed", occurrences: ` maxOccurs="many"`, marker: `maxOccurs="many"`, spec: schemaParticleOccurrenceDatatypeSpecRef},
					{name: "reversed", occurrences: ` minOccurs="2" maxOccurs="1"`, marker: `<xs:element name="value"`, related: []string{`minOccurs="2"`, `maxOccurs="1"`}, cause: errParticleOccurrenceMinimumExceedsMaximum, spec: schemaParticleCorrectSpecRef},
				} {
					content := `<xs:` + model + `><xs:element name="value" type="xs:negativeInteger"` + test.occurrences + `/></xs:` + model + `>`
					if extension {
						content = `<xs:complexContent><xs:extension base="r:Base">` + content + `</xs:extension></xs:complexContent>`
					}
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:complexType name="Base"/><xs:complexType name="Record">` + content + `</xs:complexType></xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertZeroSchema(t, schema)
					if err == nil || (test.cause != nil && !errors.Is(err, test.cause)) {
						t.Fatalf("%s/%s/%t/%s error = %v, want %v", profile.name, model, extension, test.name, err, test.cause)
					}
					diagnostic := requireDiagnostic(t, err)
					if test.name == "malformed" {
						var inner Diagnostic
						if !errors.As(errors.Unwrap(diagnostic), &inner) || inner.Code() != InvalidIntegerLexicalCode || inner.Loc() != elementReferenceTestAttributeLoc(t, root, test.marker) {
							t.Fatalf("malformed occurrence lost lexical cause: %v", errors.Unwrap(diagnostic))
						}
					}
					wantLoc := elementReferenceTestAttributeLoc(t, root, test.marker)
					wantSpec := test.spec(negativeIntegerParticleFactVersion(profile))
					if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != wantSpec {
						t.Fatalf("%s/%s/%t/%s diagnostic = %s (spec %s), want invalid occurrence at %s with %s", profile.name, model, extension, test.name, diagnostic, diagnostic.SpecRef(), wantLoc, wantSpec)
					}
					var wantRelated []Loc
					for _, marker := range test.related {
						wantRelated = append(wantRelated, elementReferenceTestAttributeLoc(t, root, marker))
					}
					if !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
						t.Fatalf("related = %v, want %v", diagnostic.Related(), wantRelated)
					}
				}
			}
		}
	}
}
