package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

// The targets stand in for the bootstrap annotation and group declarations;
// their names are intentionally unrelated to the admission rule.
func sequenceBaseGroupExtensionSchema(baseShape, wildcard, groupRange, uses string) string {
	base := `<xs:complexType name="Base"><xs:annotation><xs:documentation>base</xs:documentation></xs:annotation><xs:sequence><xs:element ref="t:note" minOccurs="0"/></xs:sequence>` + wildcard + `</xs:complexType>`
	if baseShape == "extension" {
		empty := `<xs:complexType name="Empty"/>`
		if wildcard != "" {
			empty = `<xs:complexType name="Empty"><xs:complexContent><xs:restriction base="xs:anyType">` + wildcard + `</xs:restriction></xs:complexContent></xs:complexType>`
		}
		base = empty +
			`<xs:complexType name="Base"><xs:complexContent><xs:extension base="t:Empty"><xs:sequence><xs:element ref="t:note" minOccurs="0"/></xs:sequence></xs:extension></xs:complexContent></xs:complexType>`
	}
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root">
  <xs:element name="root" type="t:Derived"/>
  <xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Base"><xs:group ref="t:Fields"` + groupRange + `/>` + uses + `</xs:extension></xs:complexContent></xs:complexType>
  <xs:element name="note" type="xs:string"/>
  <xs:group name="Fields"><xs:sequence><xs:element ref="t:field"/></xs:sequence></xs:group>
  <xs:element name="field" type="xs:integer"/>
  ` + base + `
</xs:schema>`
}

func sequenceBaseGroupComponents(t *testing.T, schema Schema) (ComplexTypeDefinition, ComplexTypeDefinition, Component, Component) {
	t.Helper()
	var base, derived ComplexTypeDefinition
	var note, group Component
	for _, component := range schema.Components() {
		switch component.Name().Local() {
		case "Base":
			base = requireTestComplexTypeDefinition(t, component, "Base")
		case "Derived":
			derived = requireTestComplexTypeDefinition(t, component, "Derived")
		case "note":
			note = component
		case "Fields":
			group = component
		}
	}
	if base.ID().IsZero() || derived.ID().IsZero() || note.ID().IsZero() || group.ID().IsZero() {
		t.Fatalf("missing synthetic target/base/derived: %v", schema.Components())
	}
	return base, derived, note, group
}

//nolint:gocognit // Compare the two authored particles with the phase composition in one control.
func TestSequenceBaseGroupExtensionPublicAndEffectiveFacts(t *testing.T) {
	for _, shape := range []string{"direct", "extension"} {
		for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
			for _, withWildcard := range []bool{false, true} {
				name := shape + "/" + string(policy)
				if withWildcard {
					name += "/wildcard"
				}
				t.Run(name, func(t *testing.T) {
					wildcard := ""
					if withWildcard {
						wildcard = `<xs:anyAttribute namespace="##other" processContents="lax"/>`
					}
					root := sequenceBaseGroupExtensionSchema(shape, wildcard, ` minOccurs="2" maxOccurs="3"`, "")
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
					if err != nil {
						t.Fatal(err)
					}
					base, derived, note, group := sequenceBaseGroupComponents(t, schema)
					if base.Loc() != complexContentTestLoc(t, root, `<xs:complexType name="Base"`) || derived.Loc() != complexContentTestLoc(t, root, `<xs:complexType name="Derived"`) {
						t.Fatalf("type locations: %s %s", base.Loc(), derived.Loc())
					}
					baseRef, ok := derived.BaseReference()
					baseID, hasID := baseRef.ComponentID()
					if !ok || !hasID || baseID != base.ID() || baseRef.Name() != mustTestQName(t, "urn:root", "Base") || baseRef.Loc() != complexContentTestLoc(t, root, `base="t:Base"`) {
						t.Fatalf("base reference: %v", baseRef)
					}
					sequence, ok := base.Particle().(SequenceParticle)
					if !ok || !sequence.Occurrences().IsDefault() || sequence.Loc() != complexContentTestLoc(t, root, `<xs:sequence><xs:element ref="t:note"`) {
						t.Fatalf("authored base sequence: %v", base.Particle())
					}
					members := sequence.Particles()
					if len(members) != 1 {
						t.Fatalf("base members: %v", members)
					}
					noteRef, ok := members[0].(ElementReferenceParticle)
					if !ok || noteRef.Occurrences().String() != "0/1" || noteRef.Ref() != mustTestQName(t, "urn:root", "note") || noteRef.TargetID() != note.ID() || noteRef.Loc() != complexContentTestLoc(t, root, `<xs:element ref="t:note"`) || noteRef.RefLoc() != complexContentTestLoc(t, root, `ref="t:note"`) {
						t.Fatalf("base reference member: %v", members[0])
					}
					groupRef, ok := derived.Particle().(ModelGroupReferenceParticle)
					if !ok || groupRef.Ref() != mustTestQName(t, "urn:root", "Fields") || groupRef.TargetID() != group.ID() || groupRef.Occurrences().String() != "2/3" || groupRef.Loc() != complexContentTestLoc(t, root, `<xs:group ref="t:Fields"`) || groupRef.RefLoc() != complexContentTestLoc(t, root, `ref="t:Fields"`) {
						t.Fatalf("authored derived group: %v", derived.Particle())
					}
					if len(base.AttributeUses()) != 0 || len(derived.AttributeUses()) != 0 {
						t.Fatal("unexpected effective attribute use")
					}
					phaseBody := schemaComplexTypeBodyResult(&schemaComplexTypeDirectBodyResult{particle: sequence})
					if shape == "extension" {
						phaseBody = &schemaComplexTypeExtensionBodyResult{particle: sequence}
					}
					content, admitted := schemaComposeOptionalReferenceSequenceBase(phaseBody, groupRef)
					if !admitted || content.base.Loc() != sequence.Loc() || content.derived.Loc() != groupRef.Loc() || len(content.base.Particles()) != 1 {
						t.Fatalf("phase content: %#v/%v", content, admitted)
					}
					members[0] = nil
					freshSequence, fresh := base.Particle().(SequenceParticle)
					if !fresh || len(freshSequence.Particles()) != 1 || freshSequence.Particles()[0] == nil {
						t.Fatal("base particle slice mutated schema")
					}
					_, baseWildcard := base.AnyAttribute()
					derivedWildcard, inherited := derived.AnyAttribute()
					if baseWildcard != withWildcard || inherited != withWildcard {
						t.Fatalf("wildcard presence: base=%v derived=%v", baseWildcard, inherited)
					}
					if withWildcard && (derivedWildcard.Loc() != complexContentTestLoc(t, root, `<xs:anyAttribute`) || derivedWildcard.NamespaceLoc() != complexContentTestLoc(t, root, `namespace="##other"`) || derivedWildcard.ProcessContentsLoc() != complexContentTestLoc(t, root, `processContents="lax"`)) {
						t.Fatalf("wildcard provenance: %v", derivedWildcard)
					}
					again, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
					if err != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
						t.Fatalf("repeat: %v", err)
					}
				})
			}
		}
	}
}

//nolint:gocognit // Pair present and omitted consumer exits across both source shapes.
func TestSequenceBaseGroupExtensionOmissionAndConsumers(t *testing.T) {
	for _, shape := range []string{"direct", "extension"} {
		for _, groupRange := range []string{"", ` minOccurs="0" maxOccurs="0"`} {
			root := sequenceBaseGroupExtensionSchema(shape, "", groupRange, `<xs:attribute name="gone" type="xs:boolean" use="prohibited"/>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			if err != nil {
				t.Fatal(err)
			}
			_, derived, _, group := sequenceBaseGroupComponents(t, schema)
			if len(derived.AttributeUses()) != 0 {
				t.Fatalf("prohibited use persisted: %v", derived.AttributeUses())
			}
			if groupRange == "" && derived.Particle() == nil || groupRange != "" && derived.Particle() != nil {
				t.Fatalf("omitted group particle: %v", derived.Particle())
			}
			validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
			if validationErr == nil || !errors.Is(validationErr, ErrUnsupported) {
				t.Fatalf("validation: %v", validationErr)
			}
			validation := requireDiagnostic(t, validationErr)
			rootLoc := complexContentTestLoc(t, root, `<xs:element name="root"`)
			derivedLoc := complexContentTestLoc(t, root, `<xs:complexType name="Derived"`)
			contentLoc := complexContentTestLoc(t, root, `<xs:complexContent`)
			extensionLoc := complexContentTestLoc(t, root, `<xs:extension base="t:Base"`)
			baseLoc := complexContentTestLoc(t, root, `base="t:Base"`)
			groupLoc := complexContentTestLoc(t, root, `<xs:group ref="t:Fields"`)
			groupRefLoc := complexContentTestLoc(t, root, `ref="t:Fields"`)
			wantValidationLoc := groupRefLoc
			wantValidationRelated := []Loc{rootLoc, derivedLoc, contentLoc, extensionLoc, baseLoc, groupLoc}
			wantValidationCause := errInstanceModelGroupReference
			if groupRange != "" {
				wantValidationLoc = extensionLoc
				wantValidationRelated = []Loc{rootLoc, derivedLoc, contentLoc, extensionLoc, baseLoc}
				wantValidationCause = errInstanceComplexContentExtension
			}
			if validation.Class() != FailureUnsupported || validation.Code() != UnsupportedInstanceValidationCode || validation.Feature() != FeatureInstanceValidation || validation.SpecRef() != instanceValidationXSD11SpecRef || validation.Loc() != wantValidationLoc || !reflect.DeepEqual(validation.Related(), wantValidationRelated) || !errors.Is(validationErr, wantValidationCause) {
				t.Fatalf("validation diagnostic: %v", validationErr)
			}
			generated, generationErr := GenerateGo(schema, "generated")
			if generated != nil || generationErr == nil || !errors.Is(generationErr, ErrUnsupported) {
				t.Fatalf("generation: %q, %v", generated, generationErr)
			}
			generation := requireDiagnostic(t, generationErr)
			wantGenerationLoc := groupRefLoc
			wantGenerationRelated := []Loc{derivedLoc, groupLoc, groupRefLoc, group.Loc()}
			wantGenerationSpec := codegenDirectSequenceSpecReference(XSDVersion11, codegenDirectSequenceParticlesReference)
			if groupRange != "" {
				wantGenerationLoc = extensionLoc
				wantGenerationRelated = []Loc{contentLoc, extensionLoc, baseLoc}
				wantGenerationSpec = codegenDirectChoiceSpecReference(XSDVersion11, codegenDirectChoiceParticlesReference)
			}
			if generation.Class() != FailureUnsupported || generation.Code() != diagnosticCodegenUnsupported || generation.Feature() != FeatureCodegen || generation.SpecRef() != wantGenerationSpec || generation.Loc() != wantGenerationLoc || !reflect.DeepEqual(generation.Related(), wantGenerationRelated) || !errors.Is(generationErr, errCodegenUnsupported) {
				t.Fatalf("generation diagnostic: %v", generationErr)
			}
		}
	}
}

//nolint:gocognit,funlen // Every excluded sibling has its own observable parse exit.
func TestSequenceBaseGroupExtensionExcludedShapes(t *testing.T) {
	baseSequence := `<xs:sequence><xs:element ref="t:note" minOccurs="0"/></xs:sequence>`
	for _, shape := range []string{"direct", "extension"} {
		for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
			version := XSDVersion11
			if policy == Strict10 {
				version = XSDVersion10
			}
			for _, test := range []struct {
				name   string
				mutate func(string) string
			}{
				{"required reference", func(root string) string {
					return strings.Replace(root, `ref="t:note" minOccurs="0"`, `ref="t:note"`, 1)
				}},
				{"multiple references", func(root string) string {
					return strings.Replace(root, baseSequence, `<xs:sequence><xs:element ref="t:note" minOccurs="0"/><xs:element ref="t:field"/></xs:sequence>`, 1)
				}},
				{"local base member", func(root string) string {
					return strings.Replace(root, baseSequence, `<xs:sequence><xs:element name="local" type="xs:integer" minOccurs="0"/></xs:sequence>`, 1)
				}},
				{"choice base", func(root string) string {
					return strings.Replace(root, baseSequence, `<xs:choice><xs:element ref="t:note" minOccurs="0"/></xs:choice>`, 1)
				}},
				{"all base", func(root string) string {
					return strings.Replace(root, baseSequence, `<xs:all><xs:element ref="t:note" minOccurs="0"/></xs:all>`, 1)
				}},
				{"group base", func(root string) string {
					return strings.Replace(root, baseSequence, `<xs:group ref="t:Fields"/>`, 1)
				}},
				{"base use", func(root string) string {
					if shape == "direct" {
						return strings.Replace(root, baseSequence, baseSequence+`<xs:attribute name="used" type="xs:boolean"/>`, 1)
					}
					return strings.Replace(root, baseSequence+`</xs:extension>`, baseSequence+`<xs:attribute name="used" type="xs:boolean"/></xs:extension>`, 1)
				}},
				{"derived sequence", func(root string) string {
					return strings.Replace(root, `<xs:group ref="t:Fields"/>`, `<xs:sequence><xs:element ref="t:field"/></xs:sequence>`, 1)
				}},
				{"derived choice", func(root string) string {
					return strings.Replace(root, `<xs:group ref="t:Fields"/>`, `<xs:choice><xs:element ref="t:field"/></xs:choice>`, 1)
				}},
				{"derived use", func(root string) string {
					return strings.Replace(root, `</xs:extension></xs:complexContent></xs:complexType>`, `<xs:attribute name="used" type="xs:boolean"/></xs:extension></xs:complexContent></xs:complexType>`, 1)
				}},
				{"inline derived owner", func(root string) string {
					root = strings.Replace(root, `<xs:element name="root" type="t:Derived"/>`, `<xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:Base"><xs:group ref="t:Fields"/></xs:extension></xs:complexContent></xs:complexType></xs:element>`, 1)
					return strings.Replace(root, `  <xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Base"><xs:group ref="t:Fields"/></xs:extension></xs:complexContent></xs:complexType>`+"\n", "", 1)
				}},
			} {
				t.Run(shape+"/"+string(policy)+"/"+test.name, func(t *testing.T) {
					root := test.mutate(sequenceBaseGroupExtensionSchema(shape, "", "", ""))
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
					if err == nil {
						t.Fatal("excluded shape unexpectedly passed")
					}
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					if shape == "extension" && test.name == "all base" {
						if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Feature() != FeatureSchemaSyntax || d.Loc() != complexContentTestLoc(t, root, `minOccurs="0"`) || d.SpecRef() != schemaComplexParticleExtensionSpecRef(version) || len(d.Related()) != 0 || !errors.Is(err, ErrUnsupported) {
							t.Fatalf("earlier all-member gate: %v, spec=%s", err, d.SpecRef())
						}
						return
					}
					wantLoc := complexContentTestLoc(t, root, `base="t:Base"`)
					if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Feature() != FeatureSchemaSyntax || d.Loc() != wantLoc || d.SpecRef() != schemaComplexTypeExtensionSpecRef(version) || !errors.Is(err, ErrUnsupported) {
						t.Fatalf("excluded %s: %v; related=%v", test.name, err, d.Related())
					}
					if test.name != "derived sequence" && test.name != "derived choice" && test.name != "derived use" && test.name != "inline derived owner" && !errors.Is(err, errSchemaComplexTypeBaseNonEmpty) && shape == "direct" {
						t.Fatalf("base content cause lost: %v", err)
					}
					wantRelated := []Loc{complexContentTestLoc(t, root, `<xs:complexType name="Base"`)}
					if shape == "extension" {
						wantRelated = append(wantRelated, complexContentTestLoc(t, root, `<xs:extension base="t:Empty"`))
					}
					baseParticle := `<xs:sequence>`
					if test.name == "choice base" {
						baseParticle = `<xs:choice>`
					}
					if test.name == "all base" {
						baseParticle = `<xs:all>`
					}
					if test.name == "group base" {
						baseParticle = `<xs:group ref="t:Fields"`
					}
					wantRelated = append(wantRelated, sequenceBaseGroupLastLoc(t, root, baseParticle))
					if !reflect.DeepEqual(d.Related(), wantRelated) {
						t.Fatalf("related = %v, want %v", d.Related(), wantRelated)
					}
				})
			}
		}
	}
}

func TestSequenceBaseGroupExtensionDoesNotAdmitDeeperChain(t *testing.T) {
	for _, shape := range []string{"direct", "extension"} {
		root := sequenceBaseGroupExtensionSchema(shape, "", "", "")
		root = strings.Replace(root, `</xs:schema>`, `<xs:complexType name="Deep"><xs:complexContent><xs:extension base="t:Derived"><xs:group ref="t:Fields"/></xs:extension></xs:complexContent></xs:complexType></xs:schema>`, 1)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
		if err == nil {
			t.Fatal("deeper extension chain passed")
		}
		assertZeroSchema(t, schema)
		d := requireDiagnostic(t, err)
		wantRelated := []Loc{
			complexContentTestLoc(t, root, `<xs:complexType name="Derived"`),
			complexContentTestLoc(t, root, `<xs:extension base="t:Base"`),
			complexContentTestLoc(t, root, `<xs:group ref="t:Fields"`),
		}
		if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Feature() != FeatureSchemaSyntax || d.Loc() != complexContentTestLoc(t, root, `base="t:Derived"`) || d.SpecRef() != schemaComplexTypeExtensionSpecRef(XSDVersion11) || !reflect.DeepEqual(d.Related(), wantRelated) || !errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaComplexTypeBaseUnsupported) {
			t.Fatalf("deeper chain diagnostic: %v; related=%v", err, d.Related())
		}
	}
}

func sequenceBaseGroupLastLoc(t *testing.T, source, marker string) Loc {
	t.Helper()
	index := strings.LastIndex(source, marker)
	if index < 0 {
		t.Fatalf("missing marker %q", marker)
	}
	line, column := 1, 1
	for _, character := range source[:index] {
		if character == '\n' {
			line, column = line+1, 1
			continue
		}
		column++
	}
	return mustTestLoc(t, "root.xsd", line, column)
}

//nolint:gocognit // These are alternate exits at the new composition boundary.
func TestSequenceBaseGroupExtensionDependencyAndInvalidExits(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		version := XSDVersion11
		if policy == Strict10 {
			version = XSDVersion10
		}
		for _, test := range []struct {
			name, marker, related, code, spec string
			cause                             error
			mutate                            func(string) string
		}{
			{"missing group", `ref="t:Absent"`, "", diagnosticSchemaModelGroupReferenceUnresolvedCode, schemaModelGroupReferenceSpecRef(version), errSchemaModelGroupReferenceUnresolved, func(root string) string { return strings.Replace(root, `ref="t:Fields"`, `ref="t:Absent"`, 1) }},
			{"missing omitted group", `ref="t:Absent"`, "", diagnosticSchemaModelGroupReferenceUnresolvedCode, schemaModelGroupReferenceSpecRef(version), errSchemaModelGroupReferenceUnresolved, func(_ string) string {
				return strings.Replace(sequenceBaseGroupExtensionSchema("direct", "", ` minOccurs="0" maxOccurs="0"`, ""), `ref="t:Fields"`, `ref="t:Absent"`, 1)
			}},
			{"wrong-kind group", `ref="t:note"`, `<xs:element name="note"`, diagnosticSchemaModelGroupReferenceWrongKindCode, schemaModelGroupReferenceSpecRef(version), errSchemaModelGroupReferenceWrongKind, func(root string) string { return strings.Replace(root, `ref="t:Fields"`, `ref="t:note"`, 1) }},
			{"missing base member", `ref="t:Absent"`, "", diagnosticSchemaElementReferenceUnresolvedCode, schemaElementReferenceSpecRef(version), errSchemaElementReferenceUnresolved, func(root string) string { return strings.Replace(root, `ref="t:note"`, `ref="t:Absent"`, 1) }},
			{"missing base", `base="t:Absent"`, "", invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(version), errSchemaComplexTypeBaseUnresolved, func(root string) string { return strings.Replace(root, `base="t:Base"`, `base="t:Absent"`, 1) }},
			{"wrong-kind base", `base="t:note"`, `<xs:element name="note"`, invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(version), errSchemaComplexTypeBaseWrongKind, func(root string) string { return strings.Replace(root, `base="t:Base"`, `base="t:note"`, 1) }},
			{"final base", `base="t:Base"`, `final="extension"`, invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(version), errSchemaComplexTypeBaseUnsupported, func(root string) string {
				return strings.Replace(root, `<xs:complexType name="Base"`, `<xs:complexType name="Base" final="extension"`, 1)
			}},
			{"base cycle", `base="t:Derived"`, `base="t:Base"`, invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(version), errSchemaComplexTypeBaseCycle, func(root string) string {
				return strings.Replace(root, `<xs:complexType name="Base"><xs:annotation><xs:documentation>base</xs:documentation></xs:annotation><xs:sequence><xs:element ref="t:note" minOccurs="0"/></xs:sequence></xs:complexType>`, `<xs:complexType name="Base"><xs:complexContent><xs:extension base="t:Derived"><xs:sequence><xs:element ref="t:note" minOccurs="0"/></xs:sequence></xs:extension></xs:complexContent></xs:complexType>`, 1)
			}},
		} {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				root := test.mutate(sequenceBaseGroupExtensionSchema("direct", "", "", ""))
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err == nil {
					t.Fatal("invalid dependency unexpectedly passed")
				}
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != FailureInvalid || d.Code() != test.code || d.Loc() != complexContentTestLoc(t, root, test.marker) || d.SpecRef() != test.spec || !errors.Is(err, test.cause) {
					t.Fatalf("dependency %s: %v, related=%v", test.name, err, d.Related())
				}
				var wantRelated []Loc
				if test.related != "" {
					wantRelated = []Loc{complexContentTestLoc(t, root, test.related)}
				}
				if !reflect.DeepEqual(d.Related(), wantRelated) {
					t.Fatalf("related = %v, want %v", d.Related(), wantRelated)
				}
			})
		}
	}
}

//nolint:gocognit // The same public references cross each supported discovery edge.
func TestSequenceBaseGroupExtensionGraphVisibilityAndProvenance(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, graph := range []string{"forward", "include", "import", "chameleon"} {
			t.Run(string(policy)+"/"+graph, func(t *testing.T) {
				basePrefix, dependencyNS := "r", ` targetNamespace="urn:root"`
				directive := ""
				if graph == "include" || graph == "chameleon" {
					directive = `<xs:include schemaLocation="dependency.xsd"/>`
				}
				if graph == "import" {
					basePrefix, dependencyNS = "o", ` targetNamespace="urn:other"`
					directive = `<xs:import namespace="urn:other" schemaLocation="dependency.xsd"/>`
				}
				if graph == "chameleon" {
					dependencyNS = ""
				}
				declarations := `<xs:element name="note" type="xs:string"/><xs:complexType name="Base"><xs:sequence><xs:element ref="` + basePrefix + `:note" minOccurs="0"/></xs:sequence></xs:complexType>`
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root">` + directive +
					`<xs:element name="root" type="r:Derived"/><xs:complexType name="Derived"><xs:complexContent><xs:extension base="` + basePrefix + `:Base"><xs:group ref="r:Fields" minOccurs="2" maxOccurs="3"/></xs:extension></xs:complexContent></xs:complexType><xs:group name="Fields"><xs:sequence/></xs:group>`
				fixtures := map[string]discoveryFixture{}
				dependency := root + declarations + `</xs:schema>`
				if graph == "forward" {
					root = dependency
				}
				if graph != "forward" {
					root += `</xs:schema>`
					dependency = `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other"` + dependencyNS + `>` + declarations + `</xs:schema>`
					fixtures["dependency.xsd"] = discoveryFixture{id: "dependency.xsd", contents: dependency}
				}
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy)
				if err != nil {
					t.Fatal(err)
				}
				baseNS := "urn:root"
				if graph == "import" {
					baseNS = "urn:other"
				}
				baseComponents := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, baseNS, "Base"))
				noteComponents := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, baseNS, "note"))
				groups := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, "urn:root", "Fields"))
				if len(baseComponents) != 1 || len(noteComponents) != 1 || len(groups) != 1 {
					t.Fatalf("discovered targets: %v", schema.Components())
				}
				derived := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Derived")
				baseRef, hasBase := derived.BaseReference()
				baseID, hasBaseID := baseRef.ComponentID()
				groupRef, hasGroup := derived.Particle().(ModelGroupReferenceParticle)
				base := requireTestComplexTypeDefinition(t, baseComponents[0], "Base")
				sequence, hasSequence := base.Particle().(SequenceParticle)
				if !hasSequence || len(sequence.Particles()) != 1 {
					t.Fatalf("base sequence: %v", base.Particle())
				}
				noteRef, hasNote := sequence.Particles()[0].(ElementReferenceParticle)
				if !hasBase || !hasBaseID || baseID != baseComponents[0].ID() || baseRef.Name() != mustTestQName(t, baseNS, "Base") || baseRef.Loc() != complexContentTestLoc(t, root, `base="`+basePrefix+`:Base"`) || !hasGroup || groupRef.TargetID() != groups[0].ID() || groupRef.Occurrences().String() != "2/3" || !hasNote || noteRef.TargetID() != noteComponents[0].ID() || noteRef.Ref() != mustTestQName(t, baseNS, "note") || noteRef.Occurrences().String() != "0/1" {
					t.Fatalf("graph references: base=%v group=%v note=%v", baseRef, groupRef, noteRef)
				}
				sourceID := SourceID("root.xsd")
				if graph != "forward" {
					sourceID = "dependency.xsd"
				}
				if noteRef.RefLoc() != complexContentTestSourceLoc(t, sourceID, dependency, `ref="`+basePrefix+`:note"`) || noteRef.Loc() != complexContentTestSourceLoc(t, sourceID, dependency, `<xs:element ref="`+basePrefix+`:note"`) {
					t.Fatalf("dependency provenance: %v", noteRef)
				}
			})
		}
	}
}

func TestSequenceBaseGroupExtensionGraphAcquisitionRetainsCause(t *testing.T) {
	for _, shape := range []string{"direct", "extension"} {
		for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
			t.Run(shape+"/"+string(policy), func(t *testing.T) {
				root := sequenceBaseGroupExtensionSchema(shape, "", "", "")
				root = strings.Replace(root, `<xs:element name="root"`, `<xs:include schemaLocation="missing.xsd"/><xs:element name="root"`, 1)
				source, sourceErr := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
				if sourceErr != nil {
					t.Fatal(sourceErr)
				}
				cause := errors.New("sequence base source unavailable")
				schema, err := ParseSchemaWithPolicy(source, &discoveryResolver{failures: map[string]error{"missing.xsd": cause}}, policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != FailureResolution || d.Code() != SourceResolveCode || d.Loc() != complexContentTestLoc(t, root, `<xs:include`) || d.SpecRef() != "" || len(d.Related()) != 0 || !errors.Is(err, cause) {
					t.Fatalf("acquisition diagnostic: %v, related=%v", err, d.Related())
				}
			})
		}
	}
}

func TestSequenceBaseGroupExtensionGroupCycleBeforeBaseComposition(t *testing.T) {
	root := sequenceBaseGroupExtensionSchema("direct", "", "", "")
	root = strings.Replace(root, `<xs:group name="Fields"><xs:sequence><xs:element ref="t:field"/></xs:sequence></xs:group>`, `<xs:group name="Fields"><xs:sequence><xs:group ref="t:Fields"/></xs:sequence></xs:group>`, 1)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err == nil {
		t.Fatal("cyclic group passed")
	}
	assertZeroSchema(t, schema)
	d := requireDiagnostic(t, err)
	if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaModelGroupReferenceCycleCode || d.Loc() != sequenceBaseGroupLastLoc(t, root, `ref="t:Fields"`) || d.SpecRef() != "xsd11-structures#mg-props-correct" || !reflect.DeepEqual(d.Related(), []Loc{complexContentTestLoc(t, root, `<xs:group name="Fields"`)}) || !errors.Is(err, errSchemaModelGroupReferenceCycle) {
		t.Fatalf("group cycle: %v; related=%v", err, d.Related())
	}
}

func TestSequenceBaseGroupExtensionInaccessibleTargets(t *testing.T) {
	for _, target := range []string{"base", "group"} {
		root := sequenceBaseGroupExtensionSchema("direct", "", "", "")
		root = strings.Replace(root, `xmlns:t="urn:root"`, `xmlns:t="urn:root" xmlns:o="urn:other"`, 1)
		root = strings.Replace(root, `<xs:element name="root"`, `<xs:include schemaLocation="bridge.xsd"/><xs:element name="root"`, 1)
		foreign := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:other">`
		marker, related, code, spec := `base="o:Base"`, `<xs:complexType name="Base"`, invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(XSDVersion11)
		cause := errSchemaComplexTypeBaseUnresolved
		if target == "base" {
			root = strings.Replace(root, `base="t:Base"`, `base="o:Base"`, 1)
			root = strings.Replace(root, `<xs:complexType name="Base"><xs:annotation><xs:documentation>base</xs:documentation></xs:annotation><xs:sequence><xs:element ref="t:note" minOccurs="0"/></xs:sequence></xs:complexType>`, "", 1)
			foreign += `<xs:element name="note" type="xs:string"/><xs:complexType name="Base"><xs:sequence><xs:element ref="o:note" minOccurs="0"/></xs:sequence></xs:complexType>`
		}
		if target == "group" {
			marker, related, code, spec, cause = `ref="o:Fields"`, `<xs:group name="Fields"`, diagnosticSchemaModelGroupReferenceNamespaceCode, schemaModelGroupReferenceImportSpecRef(XSDVersion11), errSchemaModelGroupReferenceNamespace
			root = strings.Replace(root, `ref="t:Fields"`, `ref="o:Fields"`, 1)
			root = strings.Replace(root, `<xs:group name="Fields"><xs:sequence><xs:element ref="t:field"/></xs:sequence></xs:group>`, "", 1)
			foreign += `<xs:group name="Fields"><xs:sequence/></xs:group>`
		}
		foreign += `</xs:schema>`
		bridge := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:import namespace="urn:other" schemaLocation="foreign.xsd"/></xs:schema>`
		fixtures := map[string]discoveryFixture{"bridge.xsd": {id: "bridge.xsd", contents: bridge}, "foreign.xsd": {id: "foreign.xsd", contents: foreign}}
		schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, Strict11)
		if err == nil {
			t.Fatalf("invisible %s passed", target)
		}
		assertZeroSchema(t, schema)
		d := requireDiagnostic(t, err)
		if d.Class() != FailureInvalid || d.Code() != code || d.Loc() != complexContentTestLoc(t, root, marker) || d.SpecRef() != spec || !reflect.DeepEqual(d.Related(), []Loc{complexContentTestSourceLoc(t, "foreign.xsd", foreign, related)}) || !errors.Is(err, cause) {
			t.Fatalf("invisible %s: %v, related=%v", target, err, d.Related())
		}
	}
}

func TestSequenceBaseGroupExtensionSyntaxAndDuplicatePrecedence(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		version := XSDVersion11
		if policy == Strict10 {
			version = XSDVersion10
		}
		root := sequenceBaseGroupExtensionSchema("direct", "", "", "")
		root = strings.Replace(root, `<xs:group ref="t:Fields"`, `<xs:group ref="t:Missing" minOccurs="many"`, 1)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
		assertZeroSchema(t, schema)
		d := requireDiagnostic(t, err)
		var lexical Diagnostic
		if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.Loc() != complexContentTestLoc(t, root, `minOccurs="many"`) || d.SpecRef() != schemaParticleOccurrenceDatatypeSpecRef(version) || len(d.Related()) != 0 || !errors.As(d.Unwrap(), &lexical) || lexical.Code() != InvalidIntegerLexicalCode {
			t.Fatalf("malformed occurrence before target: %v; class=%s code=%s loc=%s spec=%s related=%v cause=%v lexical=%v", err, d.Class(), d.Code(), d.Loc(), d.SpecRef(), d.Related(), d.Unwrap(), lexical)
		}

		duplicate := sequenceBaseGroupExtensionSchema("direct", "", "", "")
		duplicate = strings.Replace(duplicate, `</xs:schema>`, `<xs:group name="Fields"><xs:sequence/></xs:group></xs:schema>`, 1)
		schema, err = discoverTestSchemaWithPolicy(t, duplicate, nil, policy)
		assertZeroSchema(t, schema)
		d = requireDiagnostic(t, err)
		wantRelated := []Loc{complexContentTestLoc(t, duplicate, `<xs:group name="Fields"`)}
		if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaGlobalDuplicateCode || d.Loc() != sequenceBaseGroupLastLoc(t, duplicate, `<xs:group name="Fields"`) || d.SpecRef() != schemaGlobalDuplicateSpecRef(version) || !reflect.DeepEqual(d.Related(), wantRelated) || !errors.Is(err, errSchemaGlobalDeclarationDuplicate) {
			t.Fatalf("duplicate group before composition: %v; related=%v", err, d.Related())
		}
	}
}
