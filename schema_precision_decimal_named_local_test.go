package goxsd9

import (
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"
)

type namedPrecisionDecimalGraph struct {
	name          string
	typeLexical   string
	typeName      QName
	baseName      QName
	namespaceDecl string
	directive     string
	declarations  string
	fixtures      map[string]discoveryFixture
}

func namedPrecisionDecimalGraphs(t *testing.T, version string) []namedPrecisionDecimalGraph {
	t.Helper()
	return []namedPrecisionDecimalGraph{
		{
			name:          "local",
			typeLexical:   "t:NamedPrecision",
			typeName:      mustTestQName(t, "urn:root", "NamedPrecision"),
			baseName:      mustTestQName(t, "urn:root", "PrecisionBase"),
			namespaceDecl: `xmlns:t="urn:root"`,
			declarations:  `<xs:simpleType name="PrecisionBase"><xs:restriction base="xs:precisionDecimal"/></xs:simpleType><xs:simpleType name="NamedPrecision"><xs:restriction base="t:PrecisionBase"/></xs:simpleType>`,
		},
		{
			name:          "imported",
			typeLexical:   "b:NamedPrecision",
			typeName:      mustTestQName(t, "urn:base", "NamedPrecision"),
			baseName:      mustTestQName(t, "urn:base", "PrecisionBase"),
			namespaceDecl: `xmlns:t="urn:root" xmlns:b="urn:base"`,
			directive:     `<xs:import namespace="urn:base" schemaLocation="precision.xsd"/>`,
			fixtures: map[string]discoveryFixture{
				"precision.xsd": {
					id:       "precision.xsd",
					contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:b="urn:base" targetNamespace="urn:base" version="` + version + `"><xs:simpleType name="PrecisionBase"><xs:restriction base="xs:precisionDecimal"/></xs:simpleType><xs:simpleType name="NamedPrecision"><xs:restriction base="b:PrecisionBase"/></xs:simpleType></xs:schema>`,
				},
			},
		},
	}
}

type namedPrecisionDecimalPlacement struct {
	name              string
	model             string
	parentOccurrences string
	childOccurrences  string
	extension         bool
	wantSchema        bool
	wantElement       bool
}

func namedPrecisionDecimalPlacements() []namedPrecisionDecimalPlacement {
	return []namedPrecisionDecimalPlacement{
		{name: "choice default", model: "choice", wantSchema: true, wantElement: true},
		{name: "choice zero child", model: "choice", childOccurrences: ` minOccurs="0" maxOccurs="0"`, wantSchema: true},
		{name: "choice zero owner", model: "choice", parentOccurrences: ` minOccurs="0" maxOccurs="0"`, wantSchema: true},
		{name: "choice non-default child", model: "choice", childOccurrences: ` minOccurs="0" maxOccurs="1"`},
		{name: "choice non-default owner", model: "choice", parentOccurrences: ` minOccurs="0" maxOccurs="1"`},
		{name: "sequence default", model: "sequence"},
		{name: "sequence zero child", model: "sequence", childOccurrences: ` minOccurs="0" maxOccurs="0"`, wantSchema: true},
		{name: "sequence zero owner", model: "sequence", parentOccurrences: ` minOccurs="0" maxOccurs="0"`, wantSchema: true},
		{name: "extension choice default", model: "choice", extension: true, wantSchema: true, wantElement: true},
		{name: "extension choice zero child", model: "choice", extension: true, childOccurrences: ` minOccurs="0" maxOccurs="0"`, wantSchema: true},
		{name: "extension choice zero owner", model: "choice", extension: true, parentOccurrences: ` minOccurs="0" maxOccurs="0"`, wantSchema: true},
		{name: "extension choice non-default child", model: "choice", extension: true, childOccurrences: ` minOccurs="0" maxOccurs="1"`},
		{name: "extension choice non-default owner", model: "choice", extension: true, parentOccurrences: ` minOccurs="0" maxOccurs="1"`},
		{name: "extension sequence default", model: "sequence", extension: true},
	}
}

//nolint:gocognit // Keep the effective-type placement and exact occurrence matrix together.
func TestSchemaBridgeAdmitsEffectiveNamedPrecisionDecimalLocals(t *testing.T) {
	profiles := []struct {
		name    string
		policy  LanguagePolicy
		version string
	}{
		{name: "compatibility", policy: Compatibility, version: "1.1"},
		{name: "strict11", policy: Strict11, version: "1.1"},
	}
	for _, profile := range profiles {
		for _, graph := range namedPrecisionDecimalGraphs(t, profile.version) {
			for _, placement := range namedPrecisionDecimalPlacements() {
				t.Run(profile.name+"/"+graph.name+"/"+placement.name, func(t *testing.T) {
					root := namedPrecisionDecimalLocalRoot(profile.version, graph, placement, false)
					schema, err := discoverTestSchemaWithPolicy(t, root, graph.fixtures, profile.policy)
					if !placement.wantSchema {
						if err == nil {
							t.Fatal("discoverSchema accepted an unsupported effective named precisionDecimal placement")
						}
						assertZeroSchema(t, schema)
						diagnostic := requireDiagnostic(t, err)
						if diagnostic.Class() != FailureUnsupported || !errors.Is(err, ErrUnsupported) {
							t.Fatalf("diagnostic = %s, want located unsupported diagnostic", diagnostic)
						}
						return
					}
					if err != nil {
						t.Fatalf("discoverSchema: %v", err)
					}
					if !placement.wantElement {
						assertNamedPrecisionDecimalZeroOmission(t, schema, placement)
						return
					}
					element := namedPrecisionDecimalLocalElement(t, schema, placement.model)
					if got, want := element.Occurrences().String(), "1/1"; got != want {
						t.Fatalf("effective named precisionDecimal child occurrences = %q, want %q", got, want)
					}
					if got, want := schemaPrecisionDecimalOwnerOccurrences(t, schema, placement), "1/1"; got != want {
						t.Fatalf("effective named precisionDecimal owner occurrences = %q, want %q", got, want)
					}
					assertEffectiveNamedPrecisionDecimalElement(t, schema, element, graph)
				})
			}
		}
	}
}

func namedPrecisionDecimalLocalRoot(version string, graph namedPrecisionDecimalGraph, placement namedPrecisionDecimalPlacement, includeRoot bool) string {
	particle := `<xs:` + placement.model + placement.parentOccurrences + `><xs:element name="value" type="` + graph.typeLexical + `"` + placement.childOccurrences + `/></xs:` + placement.model + `>`
	if placement.extension {
		particle = `<xs:complexContent><xs:extension base="t:Base">` + particle + `</xs:extension></xs:complexContent>`
	}
	base := ""
	if placement.extension {
		base = `<xs:complexType name="Base"/>`
	}
	root := ""
	if includeRoot {
		root = `<xs:element name="root" type="t:Record"/>`
	}
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" ` + graph.namespaceDecl + ` targetNamespace="urn:root" version="` + version + `">` + graph.directive + root + graph.declarations + base + `<xs:complexType name="Record">` + particle + `</xs:complexType></xs:schema>`
}

func namedPrecisionDecimalLocalElement(t *testing.T, schema Schema, model string) ElementParticle {
	t.Helper()
	definition := localInlineComplexType(t, schema, "Record")
	particle := definition.Particle()
	if particle == nil {
		t.Fatal("Record particle is absent")
	}
	var particles []Particle
	switch model {
	case "choice":
		choice, ok := particle.(ChoiceParticle)
		if !ok {
			t.Fatalf("Record particle = %T, want ChoiceParticle", particle)
		}
		particles = choice.Alternatives()
	case "sequence":
		sequence, ok := particle.(SequenceParticle)
		if !ok {
			t.Fatalf("Record particle = %T, want SequenceParticle", particle)
		}
		particles = sequence.Particles()
	default:
		t.Fatalf("unknown particle model %q", model)
	}
	if len(particles) != 1 {
		t.Fatalf("Record %s child count = %d, want one", model, len(particles))
	}
	return requireInlineElementParticle(t, particles[0])
}

func schemaPrecisionDecimalOwnerOccurrences(t *testing.T, schema Schema, placement namedPrecisionDecimalPlacement) string {
	t.Helper()
	definition := localInlineComplexType(t, schema, "Record")
	particle := definition.Particle()
	if particle == nil {
		t.Fatal("effective named precisionDecimal owner particle is absent")
	}
	if placement.extension && definition.extensionBody() == nil {
		t.Fatal("effective named precisionDecimal extension body is absent")
	}
	return particle.Occurrences().String()
}

//nolint:gocognit // Keep direct and extension zero-occurrence ownership checks together.
func assertNamedPrecisionDecimalZeroOmission(t *testing.T, schema Schema, placement namedPrecisionDecimalPlacement) {
	t.Helper()
	definition := localInlineComplexType(t, schema, "Record")
	if placement.parentOccurrences != "" {
		if placement.extension {
			body := definition.extensionBody()
			if definition.Particle() != nil {
				t.Fatalf("zero-owner extension particle = %T, want absent", definition.Particle())
			}
			if body == nil {
				t.Fatal("zero-owner extension body is absent")
			}
			if body.particle != nil {
				t.Fatalf("zero-owner extension body particle = %T, want absent", body.particle)
			}
			return
		}
		if definition.Particle() != nil {
			t.Fatalf("zero-owner particle = %T, want absent", definition.Particle())
		}
		return
	}
	particle := definition.Particle()
	if particle == nil {
		t.Fatal("zero-child particle is absent")
	}
	var particles []Particle
	switch placement.model {
	case "choice":
		choice, ok := particle.(ChoiceParticle)
		if !ok {
			t.Fatalf("zero-child particle = %T, want ChoiceParticle", particle)
		}
		particles = choice.Alternatives()
	case "sequence":
		sequence, ok := particle.(SequenceParticle)
		if !ok {
			t.Fatalf("zero-child particle = %T, want SequenceParticle", particle)
		}
		particles = sequence.Particles()
	default:
		t.Fatalf("unknown zero-child model %q", placement.model)
	}
	if len(particles) != 0 {
		t.Fatalf("zero-child %s count = %d, want zero", placement.model, len(particles))
	}
}

func assertEffectiveNamedPrecisionDecimalElement(t *testing.T, schema Schema, element ElementParticle, graph namedPrecisionDecimalGraph) {
	t.Helper()
	if element.DeclaredType() != graph.typeName {
		t.Fatalf("declared type = %q, want %q", element.DeclaredType(), graph.typeName)
	}
	reference, ok := element.TypeReference()
	if !ok || reference.Kind() != SimpleTypeReferenceNamed || reference.Name() != graph.typeName {
		t.Fatalf("type reference = %q/%q/%t, want named %q", reference.Kind(), reference.Name(), ok, graph.typeName)
	}
	if _, anonymous := reference.AnonymousID(); anonymous {
		t.Fatal("named precisionDecimal reference exposed an anonymous identity")
	}
	typeID, hasTypeID := element.TypeID()
	referenceID, hasReferenceID := reference.ComponentID()
	if !hasTypeID || !hasReferenceID || typeID.IsZero() || typeID != referenceID {
		t.Fatalf("named type identity = %v/%t and %v/%t, want one nonzero identity", typeID, hasTypeID, referenceID, hasReferenceID)
	}
	component, ok := schema.Lookup(typeID)
	if !ok || component.Kind() != ComponentKindSimpleTypeDefinition {
		t.Fatalf("named type component = %#v/%t, want simple type", component, ok)
	}
	definition, ok := component.SimpleTypeDefinition()
	if !ok || !definition.HasPrecisionDecimalFacets() {
		t.Fatalf("named type definition = %#v/%t, want effective precisionDecimal facets", definition, ok)
	}
	base, ok := definition.BaseReference()
	if !ok || base.Kind() != SimpleTypeReferenceNamed || base.Name() != graph.baseName {
		t.Fatalf("effective named base = %q/%q/%t, want named %q", base.Kind(), base.Name(), ok, graph.baseName)
	}
}

//nolint:gocognit // Keep named/imported validation, generation, and extension gates paired.
func TestSchemaBridgeNamedEffectivePrecisionDecimalConsumers(t *testing.T) {
	for _, graph := range namedPrecisionDecimalGraphs(t, "1.1") {
		t.Run(graph.name+"/direct-choice", func(t *testing.T) {
			placement := namedPrecisionDecimalPlacement{name: "choice default", model: "choice", wantSchema: true, wantElement: true}
			root := namedPrecisionDecimalLocalRoot("1.1", graph, placement, true)
			schema, err := discoverTestSchemaWithPolicy(t, root, graph.fixtures, Strict11)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}
			namedPrecisionDecimalLocalElement(t, schema, placement.model)
			if err := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"><value xmlns="">1.25</value></root>`))); err != nil {
				t.Fatalf("ValidateInstance: %v", err)
			}
			output, generationErr := GenerateGo(schema, "generated")
			if output != nil || generationErr == nil {
				t.Fatalf("GenerateGo result = (%q, %v), want unsupported with no output", output, generationErr)
			}
			generationDiagnostic := requireDiagnostic(t, generationErr)
			if generationDiagnostic.Class() != FailureUnsupported || generationDiagnostic.Feature() != FeatureCodegen || !errors.Is(generationErr, ErrUnsupported) {
				t.Fatalf("GenerateGo diagnostic = %s, want unsupported precisionDecimal target", generationDiagnostic)
			}
		})

		t.Run(graph.name+"/extension-choice", func(t *testing.T) {
			placement := namedPrecisionDecimalPlacement{name: "extension choice default", model: "choice", extension: true, wantSchema: true, wantElement: true}
			root := namedPrecisionDecimalLocalRoot("1.1", graph, placement, true)
			schema, err := discoverTestSchemaWithPolicy(t, root, graph.fixtures, Strict11)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}
			namedPrecisionDecimalLocalElement(t, schema, placement.model)
			validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"><value xmlns="">1.25</value></root>`)))
			if validationErr == nil {
				t.Fatal("ValidateInstance accepted an effective named precisionDecimal extension")
			}
			validationDiagnostic := requireDiagnostic(t, validationErr)
			if validationDiagnostic.Class() != FailureUnsupported || validationDiagnostic.Feature() != FeatureInstanceValidation || !errors.Is(validationErr, ErrUnsupported) {
				t.Fatalf("validation diagnostic = %s, want unsupported extension consumer", validationDiagnostic)
			}
			output, generationErr := GenerateGo(schema, "generated")
			if output != nil || generationErr == nil {
				t.Fatalf("GenerateGo result = (%q, %v), want unsupported with no output", output, generationErr)
			}
			generationDiagnostic := requireDiagnostic(t, generationErr)
			if generationDiagnostic.Class() != FailureUnsupported || generationDiagnostic.Feature() != FeatureCodegen || !errors.Is(generationErr, ErrUnsupported) {
				t.Fatalf("GenerateGo diagnostic = %s, want unsupported extension consumer", generationDiagnostic)
			}
		})
	}
}

//nolint:gocognit // Keep policy-first rejection ahead of both named zero-occurrence placements.
func TestSchemaBridgeNamedEffectivePrecisionDecimalStrict10RejectsBeforeZeroOmission(t *testing.T) {
	for _, graph := range namedPrecisionDecimalGraphs(t, "1.0") {
		for _, extension := range []bool{false, true} {
			for _, occurrences := range []struct {
				name   string
				parent string
				child  string
			}{
				{name: "zero child", child: ` minOccurs="0" maxOccurs="0"`},
				{name: "zero owner", parent: ` minOccurs="0" maxOccurs="0"`},
			} {
				t.Run(graph.name+"/extension="+strconv.FormatBool(extension)+"/"+occurrences.name, func(t *testing.T) {
					placement := namedPrecisionDecimalPlacement{
						name:              "choice zero",
						model:             "choice",
						extension:         extension,
						parentOccurrences: occurrences.parent,
						childOccurrences:  occurrences.child,
						wantSchema:        true,
					}
					root := namedPrecisionDecimalLocalRoot("1.0", graph, placement, false)
					schema, err := discoverTestSchemaWithPolicy(t, root, graph.fixtures, Strict10)
					if err == nil {
						t.Fatal("Strict10 accepted an effective named precisionDecimal before zero omission")
					}
					assertZeroSchema(t, schema)
					diagnostic := requireDiagnostic(t, err)
					if diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != FeatureDatatypeFacets || diagnostic.Code() != diagnosticSchemaPrecisionDecimalVersionCode {
						t.Fatalf("diagnostic = %s/%q/%q, want precisionDecimal policy mismatch", diagnostic, diagnostic.Feature(), diagnostic.Code())
					}
					if !errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaPrecisionDecimalVersion) || !errors.Is(err, errLanguagePolicyMismatch) {
						t.Fatalf("policy diagnostic lost causes: %v", err)
					}
				})
			}
		}
	}
}

func TestSchemaBridgeKeepsNamedEffectivePrecisionDecimalDistinctFromMappedInline(t *testing.T) {
	graph := namedPrecisionDecimalGraphs(t, "1.1")[0]
	namedPlacement := namedPrecisionDecimalPlacement{name: "choice default", model: "choice", wantSchema: true, wantElement: true}
	namedRoot := namedPrecisionDecimalLocalRoot("1.1", graph, namedPlacement, false)
	namedSchema, err := discoverTestSchemaWithPolicy(t, namedRoot, graph.fixtures, Strict11)
	if err != nil {
		t.Fatalf("discover named schema: %v", err)
	}
	assertEffectiveNamedPrecisionDecimalElement(t, namedSchema, namedPrecisionDecimalLocalElement(t, namedSchema, "choice"), graph)

	inlineRoot := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" version="1.1"><xs:element name="root" type="t:Record"/><xs:complexType name="Record"><xs:choice><xs:element name="value"><xs:simpleType><xs:restriction base="xs:precisionDecimal"/></xs:simpleType></xs:element></xs:choice></xs:complexType></xs:schema>`
	inlineSchema, inlineErr := discoverTestSchemaWithPolicy(t, inlineRoot, nil, Strict11)
	if inlineErr == nil {
		t.Fatal("discoverSchema accepted a mapped inline precisionDecimal where named effective precisionDecimal is admitted")
	}
	assertZeroSchema(t, inlineSchema)
	diagnostic := requireDiagnostic(t, inlineErr)
	if diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != FeatureSchemaSyntax || !errors.Is(inlineErr, ErrUnsupported) {
		t.Fatalf("inline diagnostic = %s, want schema-unsupported with preserved cause", diagnostic)
	}
}
