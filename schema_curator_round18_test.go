package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

type curatorLongFamilyCase struct {
	name       string
	atomicKind schemaSimpleTypeAtomicKind
	minimum    string
	maximum    string
}

func curatorLongFamilyCases() []curatorLongFamilyCase {
	return []curatorLongFamilyCase{
		{name: "long", atomicKind: schemaSimpleTypeAtomicLong, minimum: "-9223372036854775808", maximum: "9223372036854775807"},
		{name: "unsignedLong", atomicKind: schemaSimpleTypeAtomicUnsignedLong, minimum: "0", maximum: "18446744073709551615"},
		{name: "negativeInteger", atomicKind: schemaSimpleTypeAtomicNegativeInteger, maximum: "-1"},
		{name: "nonNegativeInteger", atomicKind: schemaSimpleTypeAtomicNonNegativeInteger, minimum: "0"},
		{name: "nonPositiveInteger", atomicKind: schemaSimpleTypeAtomicNonPositiveInteger, maximum: "0"},
	}
}

type curatorPolicyProfile struct {
	name    string
	policy  LanguagePolicy
	version XSDVersion
}

func curatorPolicyProfiles() []curatorPolicyProfile {
	return []curatorPolicyProfile{
		{name: "Compatibility", policy: Compatibility, version: XSDVersion11},
		{name: "Strict10", policy: Strict10, version: XSDVersion10},
		{name: "Strict11", policy: Strict11, version: XSDVersion11},
	}
}

//nolint:gocognit,funlen // Keep the public query and both consumer boundaries in one matrix.
func TestCuratorGlobalInlineLongFamilyMatrixAcrossPolicies(t *testing.T) {
	for _, family := range curatorLongFamilyCases() {
		for _, profile := range curatorPolicyProfiles() {
			t.Run(family.name+"/"+profile.name, func(t *testing.T) {
				root := curatorGlobalInlineLongFamilyRoot(profile.version, family.name)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("discover schema: %v", err)
				}

				components := schema.Components()
				if len(components) != 1 || components[0].Kind() != ComponentKindElementDeclaration {
					t.Fatalf("components = %#v, want one global element declaration", components)
				}
				component := components[0]
				if component.ID().Source() != "root.xsd" || component.ID().Ordinal() != 1 {
					t.Fatalf("component identity = %v, want root.xsd ordinal 1", component.ID())
				}
				declaration, ok := component.ElementDeclaration()
				if !ok {
					t.Fatal("global inline declaration view is missing")
				}
				wantDeclarationLoc := mustSchemaTokenLoc(t, "root.xsd", root, 2, `<xs:element name="value"`)
				wantTypeLoc := mustSchemaTokenLoc(t, "root.xsd", root, 2, "<xs:simpleType>")
				wantVarietyLoc := mustSchemaTokenLoc(t, "root.xsd", root, 2, "<xs:restriction")
				wantBaseLoc := mustSchemaTokenLoc(t, "root.xsd", root, 2, `base="xs:`+family.name+`"`)
				if declaration.ID() != component.ID() || declaration.Name() != mustTestQName(t, "urn:test", "value") || declaration.Loc() != wantDeclarationLoc {
					t.Fatalf("declaration facts = %v/%q/%s, want component identity/{urn:test}value/%s", declaration.ID(), declaration.Name(), declaration.Loc(), wantDeclarationLoc)
				}
				if !declaration.DeclaredType().IsZero() {
					t.Fatalf("declared type = %q, want zero for an inline type", declaration.DeclaredType())
				}
				if typeID, hasTypeID := declaration.TypeID(); hasTypeID || !typeID.IsZero() {
					t.Fatalf("declaration type ownership = %v/%t, want zero/false", typeID, hasTypeID)
				}

				reference, ok := declaration.TypeReference()
				if !ok || !reference.IsAnonymous() || reference.Name() != (QName{}) || reference.Loc() != wantTypeLoc || reference.VarietyLoc() != wantVarietyLoc {
					t.Fatalf("type reference = %#v/%t, want anonymous reference with exact locations", reference, ok)
				}
				if typeID, hasTypeID := reference.ComponentID(); hasTypeID || !typeID.IsZero() {
					t.Fatalf("anonymous reference component ownership = %v/%t, want zero/false", typeID, hasTypeID)
				}
				anonymousID, hasAnonymousID := reference.AnonymousID()
				anonymous, hasAnonymous := reference.AnonymousType()
				modelID, hasModelID := anonymous.NodeID()
				if !hasAnonymousID || anonymousID.IsZero() || !hasAnonymous || !hasModelID || modelID != anonymousID {
					t.Fatalf("anonymous ownership = %v/%t/%#v/%t, want one nonzero model identity", anonymousID, hasAnonymousID, anonymous, hasAnonymous)
				}
				if anonymous.ID().IsZero() == false || anonymous.Name() != (QName{}) || !anonymous.IsAnonymous() || anonymous.Loc() != wantTypeLoc || anonymous.VarietyLoc() != wantVarietyLoc {
					t.Fatalf("anonymous definition ownership/locations = %v/%q/%t/%s/%s, want zero/{}/true/%s/%s", anonymous.ID(), anonymous.Name(), anonymous.IsAnonymous(), anonymous.Loc(), anonymous.VarietyLoc(), wantTypeLoc, wantVarietyLoc)
				}
				if anonymous.facts == nil || anonymous.facts.atomicKind != family.atomicKind {
					t.Fatalf("anonymous atomic kind = %#v, want %s", anonymous.facts, family.name)
				}
				if anonymous.Base() != mustTestQName(t, testXSDNamespace, family.name) || anonymous.BaseLoc() != wantBaseLoc {
					t.Fatalf("anonymous base facts = %q/%s, want xs:%s/%s", anonymous.Base(), anonymous.BaseLoc(), family.name, wantBaseLoc)
				}
				base, ok := anonymous.BaseReference()
				if !ok || !base.IsBuiltin() || base.Name() != anonymous.Base() || base.Loc() != wantBaseLoc || base.VarietyLoc() != wantBaseLoc {
					t.Fatalf("anonymous base reference = %#v/%t, want built-in %s at %s", base, ok, family.name, wantBaseLoc)
				}
				assertCuratorIntegerBounds(t, anonymous, family, profile.version)
				if matches := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, testXSDNamespace, family.name)); len(matches) != 0 {
					t.Fatalf("built-in %s is visible as %d named component(s)", family.name, len(matches))
				}

				generated, generationErr := GenerateGo(schema, "generated")
				if generated != nil || generationErr == nil {
					t.Fatalf("GenerateGo result = (%q, %v), want unsupported with no output", generated, generationErr)
				}
				generationDiagnostic := requireDiagnostic(t, generationErr)
				if generationDiagnostic.Class() != FailureUnsupported || generationDiagnostic.Code() != diagnosticCodegenUnsupported || generationDiagnostic.Feature() != FeatureCodegen || generationDiagnostic.Loc() != wantDeclarationLoc || !errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenUnsupported) {
					t.Fatalf("GenerateGo diagnostic = %s, want unsupported at %s with preserved cause", generationDiagnostic, wantDeclarationLoc)
				}
				if !reflect.DeepEqual(generationDiagnostic.Related(), []Loc{wantTypeLoc}) {
					t.Fatalf("GenerateGo related locations = %v, want [%s]", generationDiagnostic.Related(), wantTypeLoc)
				}

				validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<value xmlns="urn:test">0</value>`)))
				if validationErr == nil {
					t.Fatal("ValidateInstance accepted a global inline long-family target")
				}
				validationDiagnostic := requireDiagnostic(t, validationErr)
				if validationDiagnostic.Class() != FailureUnsupported || validationDiagnostic.Code() != UnsupportedInstanceValidationCode || validationDiagnostic.Feature() != FeatureInstanceValidation || validationDiagnostic.Loc() != mustTestLoc(t, "instance.xml", 1, 1) || !errors.Is(validationErr, ErrUnsupported) || !errors.Is(validationErr, errInstanceNoDeclaredType) {
					t.Fatalf("ValidateInstance diagnostic = %s, want unsupported at instance root with preserved cause", validationDiagnostic)
				}
				if !reflect.DeepEqual(validationDiagnostic.Related(), []Loc{wantDeclarationLoc}) {
					t.Fatalf("ValidateInstance related locations = %v, want [%s]", validationDiagnostic.Related(), wantDeclarationLoc)
				}
			})
		}
	}
}

func curatorGlobalInlineLongFamilyRoot(version XSDVersion, family string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test" version="` + string(version) + `">
  <xs:element name="value"><xs:simpleType><xs:restriction base="xs:` + family + `"/></xs:simpleType></xs:element>
</xs:schema>`
}

//nolint:gocognit // Keep the exact one-sided and two-sided bound contract together.
func assertCuratorIntegerBounds(t *testing.T, definition SimpleTypeDefinition, family curatorLongFamilyCase, version XSDVersion) {
	t.Helper()
	if definition.DigitFacets().Kind() != DigitDatatypeInteger || definition.DigitFacets().Version() != version {
		t.Fatalf("digit facts = %q/%q, want integer/%q", definition.DigitFacets().Kind(), definition.DigitFacets().Version(), version)
	}
	bounds, ok := definition.IntegerBounds()
	if !ok {
		t.Fatal("integer bounds are absent")
	}
	minimum, hasMinimum := bounds.MinInclusive()
	if family.minimum == "" {
		if hasMinimum {
			t.Fatalf("minInclusive = %q, want absent", minimum.Canonical())
		}
	}
	if family.minimum != "" {
		if !hasMinimum || minimum.Canonical() != family.minimum {
			t.Fatalf("minInclusive = %q/%t, want %s/true", minimum.Canonical(), hasMinimum, family.minimum)
		}
		minimumFacet, present := bounds.MinInclusiveFacet()
		if !present || minimumFacet.Kind() != BoundMinInclusive || minimumFacet.Value().Canonical() != family.minimum || minimumFacet.Version() != version || !minimumFacet.Loc().IsZero() {
			t.Fatalf("minInclusive facts = %q/%s/%q/%t, want %s/zero/%q/true", minimumFacet.Value().Canonical(), minimumFacet.Loc(), minimumFacet.Kind(), present, family.minimum, version)
		}
	}
	maximum, hasMaximum := bounds.MaxInclusive()
	if family.maximum == "" {
		if hasMaximum {
			t.Fatalf("maxInclusive = %q, want absent", maximum.Canonical())
		}
	}
	if family.maximum != "" {
		if !hasMaximum || maximum.Canonical() != family.maximum {
			t.Fatalf("maxInclusive = %q/%t, want %s/true", maximum.Canonical(), hasMaximum, family.maximum)
		}
		maximumFacet, present := bounds.MaxInclusiveFacet()
		wantMaximumLoc := Loc{}
		if family.name == "negativeInteger" {
			wantMaximumLoc = definition.BaseLoc()
		}
		if !present || maximumFacet.Kind() != BoundMaxInclusive || maximumFacet.Value().Canonical() != family.maximum || maximumFacet.Version() != version || maximumFacet.Loc() != wantMaximumLoc {
			t.Fatalf("maxInclusive facts = %q/%s/%q/%t, want %s/%s/%q/true", maximumFacet.Value().Canonical(), maximumFacet.Loc(), maximumFacet.Kind(), present, family.maximum, wantMaximumLoc, version)
		}
	}
	ordered := bounds.Bounds()
	wantBounds := 0
	if family.minimum != "" {
		wantBounds++
	}
	if family.maximum != "" {
		wantBounds++
	}
	if len(ordered) != wantBounds {
		t.Fatalf("ordered bounds = %d, want %d", len(ordered), wantBounds)
	}
}

//nolint:gocognit // Keep all local family, owner, and effective-occurrence cases together.
func TestCuratorLocalInlineLongFamilyOccurrenceMatrixAcrossPolicies(t *testing.T) {
	occurrences := []struct {
		name  string
		child string
		zero  bool
	}{
		{name: "mapped"},
		{name: "zero child", child: ` minOccurs="0" maxOccurs="0"`, zero: true},
	}
	for _, family := range curatorLongFamilyCases() {
		for _, profile := range curatorPolicyProfiles() {
			for _, model := range []string{"choice", "sequence"} {
				for _, occurrence := range occurrences {
					t.Run(family.name+"/"+profile.name+"/"+model+"/"+occurrence.name, func(t *testing.T) {
						mappedNegative := family.name == "negativeInteger" && !occurrence.zero
						root := curatorLocalInlineLongFamilyRoot(profile.version, model, family.name, occurrence.child, mappedNegative)
						schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
						if !occurrence.zero && !mappedNegative {
							if err == nil {
								t.Fatal("discover schema accepted mapped local long-family type")
							}
							assertZeroSchema(t, schema)
							diagnostic := requireDiagnostic(t, err)
							wantLoc := mustSchemaTokenLoc(t, "root.xsd", root, 2, "<xs:simpleType>")
							if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax || diagnostic.Loc() != wantLoc || !errors.Is(err, ErrUnsupported) {
								t.Fatalf("diagnostic = %s, want unsupported schema syntax at %s", diagnostic, wantLoc)
							}
							return
						}
						if err != nil {
							t.Fatalf("discover zero-occurrence schema: %v", err)
						}
						definition := curatorLocalInlineLongFamilyDefinition(t, schema)
						if mappedNegative {
							element, anonymous := curatorLocalInlineLongFamilyParticle(t, definition, model)
							curatorAssertLocalInlineLongFamilyFacts(t, root, element, anonymous, family, profile.version)
							assertCuratorLocalInlineLongFamilyConsumers(t, schema, root)
							return
						}
						if model == "choice" {
							choice, ok := definition.Particle().(ChoiceParticle)
							if !ok || len(choice.Alternatives()) != 0 {
								t.Fatalf("zero-child particle = %T/%d, want empty choice", definition.Particle(), len(choice.Alternatives()))
							}
							return
						}
						sequence, ok := definition.Particle().(SequenceParticle)
						if !ok || len(sequence.Particles()) != 0 {
							t.Fatalf("zero-child particle = %T/%d, want empty sequence", definition.Particle(), len(sequence.Particles()))
						}
					})
				}
			}
		}
	}
}

func curatorLocalInlineLongFamilyRoot(version XSDVersion, model, family, childOccurrences string, includeGlobalElement bool) string {
	globalElement := ""
	if includeGlobalElement {
		globalElement = `<xs:element name="root" type="t:Record"/>`
	}
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" version="` + string(version) + `">
  ` + globalElement + `<xs:complexType name="Record"><xs:` + model + `><xs:element name="value"` + childOccurrences + `><xs:simpleType><xs:restriction base="xs:` + family + `"/></xs:simpleType></xs:element></xs:` + model + `></xs:complexType>
</xs:schema>`
}

func curatorLocalInlineLongFamilyDefinition(t *testing.T, schema Schema) ComplexTypeDefinition {
	t.Helper()
	matches := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Record"))
	if len(matches) != 1 {
		t.Fatalf("Record matches = %d, want one", len(matches))
	}
	definition, ok := matches[0].ComplexTypeDefinition()
	if !ok {
		t.Fatal("Record complex type view is missing")
	}
	return definition
}

func curatorLocalInlineLongFamilyParticle(t *testing.T, definition ComplexTypeDefinition, model string) (ElementParticle, SimpleTypeDefinition) {
	t.Helper()
	var particles []Particle
	switch model {
	case "choice":
		choice, ok := definition.Particle().(ChoiceParticle)
		if !ok {
			t.Fatalf("particle = %T, want choice", definition.Particle())
		}
		particles = choice.Alternatives()
	case "sequence":
		sequence, ok := definition.Particle().(SequenceParticle)
		if !ok {
			t.Fatalf("particle = %T, want sequence", definition.Particle())
		}
		particles = sequence.Particles()
	default:
		t.Fatalf("unknown model %q", model)
	}
	if len(particles) != 1 {
		t.Fatalf("particle child count = %d, want one", len(particles))
	}
	element, ok := particles[0].(ElementParticle)
	if !ok {
		t.Fatalf("particle child = %T, want ElementParticle", particles[0])
	}
	reference, ok := element.TypeReference()
	if !ok || !reference.IsAnonymous() {
		t.Fatalf("local inline reference = %#v/%t, want anonymous", reference, ok)
	}
	anonymous, ok := reference.AnonymousType()
	if !ok {
		t.Fatal("local inline anonymous definition is missing")
	}
	return element, anonymous
}

func curatorAssertLocalInlineLongFamilyFacts(t *testing.T, root string, element ElementParticle, anonymous SimpleTypeDefinition, family curatorLongFamilyCase, version XSDVersion) {
	t.Helper()
	wantElementLoc := mustSchemaTokenLoc(t, "root.xsd", root, 2, `<xs:element name="value"`)
	wantTypeLoc := mustSchemaTokenLoc(t, "root.xsd", root, 2, "<xs:simpleType>")
	wantVarietyLoc := mustSchemaTokenLoc(t, "root.xsd", root, 2, "<xs:restriction")
	wantBaseLoc := mustSchemaTokenLoc(t, "root.xsd", root, 2, `base="xs:`+family.name+`"`)
	if element.Name().Local() != "value" || element.Name().Namespace() != "" || element.Loc() != wantElementLoc || element.Occurrences().String() != "1/1" || !element.DeclaredType().IsZero() {
		t.Fatalf("local element facts = %q/%s/%s/%q, want value/%s/1/1/zero", element.Name(), element.Loc(), element.Occurrences(), element.DeclaredType(), wantElementLoc)
	}
	reference, ok := element.TypeReference()
	if !ok || !reference.IsAnonymous() || reference.Loc() != wantTypeLoc || reference.VarietyLoc() != wantVarietyLoc {
		t.Fatalf("local type reference = %#v/%t, want anonymous reference at exact locations", reference, ok)
	}
	anonymousID, hasAnonymousID := reference.AnonymousID()
	nodeID, hasNodeID := anonymous.NodeID()
	if !hasAnonymousID || !hasNodeID || anonymousID.IsZero() || anonymousID != nodeID || !anonymous.IsAnonymous() || !anonymous.ID().IsZero() || anonymous.Loc() != wantTypeLoc || anonymous.VarietyLoc() != wantVarietyLoc {
		t.Fatalf("local anonymous ownership/locations = %v/%t/%v/%t/%t/%v/%s/%s, want one model identity and exact locations", anonymousID, hasAnonymousID, nodeID, hasNodeID, anonymous.IsAnonymous(), anonymous.ID(), anonymous.Loc(), anonymous.VarietyLoc())
	}
	if anonymous.Base() != mustTestQName(t, testXSDNamespace, family.name) || anonymous.BaseLoc() != wantBaseLoc {
		t.Fatalf("local anonymous base = %q/%s, want xs:%s/%s", anonymous.Base(), anonymous.BaseLoc(), family.name, wantBaseLoc)
	}
	assertCuratorIntegerBounds(t, anonymous, family, version)
}

func assertCuratorLocalInlineLongFamilyConsumers(t *testing.T, schema Schema, root string) {
	t.Helper()
	declarationLoc := complexContentTestLoc(t, root, `<xs:element name="root"`)
	definitionLoc := complexContentTestLoc(t, root, `<xs:complexType name="Record"`)
	elementLoc := complexContentTestLoc(t, root, `<xs:element name="value"`)

	validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"><value>-1</value></root>`)))
	if validationErr == nil {
		t.Fatal("ValidateInstance accepted a local anonymous long-family target")
	}
	validationDiagnostic := requireDiagnostic(t, validationErr)
	if validationDiagnostic.Class() != FailureUnsupported || validationDiagnostic.Code() != UnsupportedInstanceValidationCode || validationDiagnostic.Feature() != FeatureInstanceValidation || !errors.Is(validationErr, ErrUnsupported) {
		t.Fatalf("ValidateInstance diagnostic = %s, want unsupported local anonymous target", validationDiagnostic)
	}
	if validationDiagnostic.Loc() != elementLoc {
		t.Fatalf("ValidateInstance location = %s, want local element %s", validationDiagnostic.Loc(), elementLoc)
	}
	if !containsLoc(validationDiagnostic.Related(), declarationLoc) || !containsLoc(validationDiagnostic.Related(), definitionLoc) {
		t.Fatalf("ValidateInstance related locations = %v, want declaration and definition", validationDiagnostic.Related())
	}

	generated, generationErr := GenerateGo(schema, "generated")
	if generated != nil || generationErr == nil {
		t.Fatalf("GenerateGo result = (%q, %v), want unsupported with no output", generated, generationErr)
	}
	generationDiagnostic := requireDiagnostic(t, generationErr)
	if generationDiagnostic.Class() != FailureUnsupported || generationDiagnostic.Code() != diagnosticCodegenUnsupported || generationDiagnostic.Feature() != FeatureCodegen || !errors.Is(generationErr, ErrUnsupported) {
		t.Fatalf("GenerateGo diagnostic = %s, want unsupported local anonymous target", generationDiagnostic)
	}
	if generationDiagnostic.Loc() != elementLoc {
		t.Fatalf("GenerateGo location = %s, want local element %s", generationDiagnostic.Loc(), elementLoc)
	}
}

type curatorPrecisionExtensionCase struct {
	name              string
	model             string
	parentOccurrences string
	childOccurrences  string
	wantSchema        bool
	wantLocation      string
	consumer          bool
}

func curatorPrecisionExtensionCases() []curatorPrecisionExtensionCase {
	return []curatorPrecisionExtensionCase{
		{name: "choice default", model: "choice", wantSchema: true, consumer: true},
		{name: "choice zero child", model: "choice", childOccurrences: ` minOccurs="0" maxOccurs="0"`, wantSchema: true},
		{name: "choice zero owner", model: "choice", parentOccurrences: ` minOccurs="0" maxOccurs="0"`, wantSchema: true},
		{name: "choice non-default child", model: "choice", childOccurrences: ` minOccurs="0" maxOccurs="1"`, wantLocation: `type="xs:precisionDecimal"`},
		{name: "choice non-default owner", model: "choice", parentOccurrences: ` minOccurs="0" maxOccurs="1"`, wantLocation: "<xs:choice"},
		{name: "sequence default", model: "sequence", wantLocation: `type="xs:precisionDecimal"`},
		{name: "sequence zero child", model: "sequence", childOccurrences: ` minOccurs="0" maxOccurs="0"`, wantSchema: true, consumer: true},
		{name: "sequence zero owner", model: "sequence", parentOccurrences: ` minOccurs="0" maxOccurs="0"`, wantSchema: true},
		{name: "sequence non-default child", model: "sequence", childOccurrences: ` minOccurs="0" maxOccurs="1"`, wantLocation: `type="xs:precisionDecimal"`},
		{name: "sequence non-default owner", model: "sequence", parentOccurrences: ` minOccurs="0" maxOccurs="1"`, wantLocation: `type="xs:precisionDecimal"`},
	}
}

//nolint:gocognit // Keep policy-first, omission, mapped-range, and consumer contracts paired.
func TestCuratorPrecisionDecimalExtensionOccurrenceMatrixAcrossPolicies(t *testing.T) {
	for _, profile := range curatorPolicyProfiles() {
		for _, test := range curatorPrecisionExtensionCases() {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := curatorPrecisionExtensionRoot(profile.version, test.model, test.parentOccurrences, test.childOccurrences)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				precisionLoc := mustSchemaTokenLoc(t, "root.xsd", root, 2, `type="xs:precisionDecimal"`)
				if profile.policy == Strict10 {
					if err == nil {
						t.Fatal("Strict10 accepted an explicitly typed extension precisionDecimal")
					}
					assertZeroSchema(t, schema)
					diagnostic := requireDiagnostic(t, err)
					if diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != FeatureDatatypeFacets || diagnostic.Code() != diagnosticSchemaPrecisionDecimalVersionCode || diagnostic.SpecRef() != "xsd11-datatypes#dt-primitive" || diagnostic.Loc() != precisionLoc || !errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaPrecisionDecimalVersion) || !errors.Is(err, errLanguagePolicyMismatch) {
						t.Fatalf("Strict10 diagnostic = %s, want policy-first precisionDecimal mismatch at %s", diagnostic, precisionLoc)
					}
					return
				}
				if !test.wantSchema {
					if err == nil {
						t.Fatal("Compatibility/Strict11 accepted non-default mapped extension precisionDecimal")
					}
					assertZeroSchema(t, schema)
					diagnostic := requireDiagnostic(t, err)
					wantLoc := mustSchemaTokenLoc(t, "root.xsd", root, 2, test.wantLocation)
					if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax || diagnostic.Loc() != wantLoc || !errors.Is(err, ErrUnsupported) {
						t.Fatalf("diagnostic = %s, want schema-syntax unsupported at %s", diagnostic, wantLoc)
					}
					return
				}
				if err != nil {
					t.Fatalf("discover admitted extension schema: %v", err)
				}
				derived := curatorPrecisionExtensionDefinition(t, schema)
				body := derived.extensionBody()
				if body == nil || derived.Derivation() != ComplexTypeDerivationExtension {
					t.Fatal("derived extension body is missing")
				}
				switch {
				case test.parentOccurrences != "":
					if derived.Particle() != nil || body.particle != nil {
						t.Fatalf("zero-owner particles = %T/%T, want absent", derived.Particle(), body.particle)
					}
				case test.childOccurrences != "":
					curatorAssertEmptyExtensionParticle(t, derived.Particle(), test.model)
				default:
					particles := curatorExtensionParticles(t, derived.Particle(), test.model)
					if len(particles) != 1 {
						t.Fatalf("default extension child count = %d, want 1", len(particles))
					}
					element, ok := particles[0].(ElementParticle)
					if !ok || element.Name().Local() != "value" || element.Occurrences().String() != "1/1" || element.DeclaredType() != mustTestQName(t, testXSDNamespace, "precisionDecimal") {
						t.Fatalf("default extension child = %#v, want value/xs:precisionDecimal/1/1", particles[0])
					}
				}
				if test.consumer {
					assertCuratorPrecisionExtensionConsumers(t, schema, root, test.model)
				}
			})
		}
	}
}

func curatorAssertEmptyExtensionParticle(t *testing.T, particle Particle, model string) {
	t.Helper()
	particles := curatorExtensionParticles(t, particle, model)
	if len(particles) != 0 {
		t.Fatalf("zero-child %s count = %d, want zero", model, len(particles))
	}
}

func curatorExtensionParticles(t *testing.T, particle Particle, model string) []Particle {
	t.Helper()
	switch model {
	case "choice":
		choice, ok := particle.(ChoiceParticle)
		if !ok {
			t.Fatalf("extension particle = %T, want choice", particle)
		}
		return choice.Alternatives()
	case "sequence":
		sequence, ok := particle.(SequenceParticle)
		if !ok {
			t.Fatalf("extension particle = %T, want sequence", particle)
		}
		return sequence.Particles()
	default:
		t.Fatalf("unknown extension model %q", model)
		return nil
	}
}

func curatorPrecisionExtensionRoot(version XSDVersion, model, parentOccurrences, childOccurrences string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" version="` + string(version) + `">
  <xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Base"><xs:` + model + parentOccurrences + `><xs:element name="value" type="xs:precisionDecimal"` + childOccurrences + `/></xs:` + model + `></xs:extension></xs:complexContent></xs:complexType>
  <xs:element name="root" type="t:Derived"/>
  <xs:complexType name="Base"/>
</xs:schema>`
}

func curatorPrecisionExtensionDefinition(t *testing.T, schema Schema) ComplexTypeDefinition {
	t.Helper()
	matches := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Derived"))
	if len(matches) != 1 {
		t.Fatalf("Derived matches = %d, want one", len(matches))
	}
	definition, ok := matches[0].ComplexTypeDefinition()
	if !ok {
		t.Fatal("Derived complex type view is missing")
	}
	return definition
}

func assertCuratorPrecisionExtensionConsumers(t *testing.T, schema Schema, root, model string) {
	t.Helper()
	declarationLoc := complexContentTestLoc(t, root, `<xs:element name="root"`)
	definitionLoc := complexContentTestLoc(t, root, `<xs:complexType name="Derived"`)
	complexContentLoc := complexContentTestLoc(t, root, "<xs:complexContent")
	extensionLoc := complexContentTestLoc(t, root, "<xs:extension")
	baseLoc := complexContentTestLoc(t, root, `base="t:Base"`)
	particleLoc := complexContentTestLoc(t, root, "<xs:"+model)

	validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"><value>1</value></root>`)))
	if validationErr == nil {
		t.Fatal("ValidateInstance accepted precisionDecimal extension content")
	}
	validationDiagnostic := requireDiagnostic(t, validationErr)
	wantValidationLoc := extensionLoc
	if model == "sequence" {
		wantValidationLoc = mustTestLoc(t, "instance.xml", 1, 1)
	}
	if validationDiagnostic.Class() != FailureUnsupported || validationDiagnostic.Code() != UnsupportedInstanceValidationCode || validationDiagnostic.Feature() != FeatureInstanceValidation || validationDiagnostic.Loc() != wantValidationLoc || !errors.Is(validationErr, ErrUnsupported) || !errors.Is(validationErr, errInstanceComplexContentExtension) {
		t.Fatalf("ValidateInstance diagnostic = %s, want extension unsupported at %s", validationDiagnostic, wantValidationLoc)
	}
	wantValidationRelated := []Loc{declarationLoc, definitionLoc}
	if model == "sequence" {
		wantValidationRelated = append(wantValidationRelated, particleLoc)
	}
	wantValidationRelated = append(wantValidationRelated, complexContentLoc, extensionLoc, baseLoc)
	if model == "choice" {
		wantValidationRelated = append(wantValidationRelated, particleLoc)
	}
	if !reflect.DeepEqual(validationDiagnostic.Related(), wantValidationRelated) {
		t.Fatalf("ValidateInstance related locations = %v, want %v", validationDiagnostic.Related(), wantValidationRelated)
	}

	generated, generationErr := GenerateGo(schema, "generated")
	if generated != nil || generationErr == nil {
		t.Fatalf("GenerateGo result = (%q, %v), want unsupported with no output", generated, generationErr)
	}
	generationDiagnostic := requireDiagnostic(t, generationErr)
	if generationDiagnostic.Class() != FailureUnsupported || generationDiagnostic.Code() != diagnosticCodegenUnsupported || generationDiagnostic.Feature() != FeatureCodegen || generationDiagnostic.Loc() != extensionLoc || !errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenUnsupported) {
		t.Fatalf("GenerateGo diagnostic = %s, want extension unsupported at %s", generationDiagnostic, extensionLoc)
	}
	wantGenerationRelated := []Loc{complexContentLoc, extensionLoc, baseLoc, particleLoc}
	if !reflect.DeepEqual(generationDiagnostic.Related(), wantGenerationRelated) {
		t.Fatalf("GenerateGo related locations = %v, want %v", generationDiagnostic.Related(), wantGenerationRelated)
	}
}
