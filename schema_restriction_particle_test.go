package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func boundedParticleRestrictionRoot(baseShape, annotationRange, groupRange string) string {
	root := sequenceBaseGroupExtensionSchema(baseShape, "", "", "")
	root = strings.Replace(root, `<xs:group name="Fields"><xs:sequence><xs:element ref="t:field"/></xs:sequence></xs:group>`, `<xs:group name="Fields"><xs:choice><xs:element ref="t:field"/><xs:element ref="t:second"/><xs:element ref="t:third"/></xs:choice></xs:group><xs:element name="second" type="xs:integer"/><xs:element name="third" type="xs:integer"/>`, 1)
	root = strings.Replace(root, `name="root" type="t:Derived"`, `name="root" type="t:Restricted"`, 1)
	restriction := `<xs:complexType name="Restricted"><xs:complexContent><xs:restriction base="t:Derived"><xs:sequence><xs:element ref="t:note"` + annotationRange + `/><xs:group ref="t:Fields"` + groupRange + `/></xs:sequence></xs:restriction></xs:complexContent></xs:complexType>`
	return strings.Replace(root, `<xs:complexType name="Derived">`, restriction+`<xs:complexType name="Derived">`, 1)
}

//nolint:gocognit // Assert the complete immutable query and consumer contract across policies.
func TestBoundedParticleRestrictionPublicFacts(t *testing.T) {
	for _, shape := range []string{"direct", "extension"} {
		for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
			t.Run(shape+"/"+string(policy), func(t *testing.T) {
				root := boundedParticleRestrictionRoot(shape, ` minOccurs="0"`, "")
				var first []Component
				for attempt := range 2 {
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
					if err != nil {
						t.Fatalf("attempt %d: %v", attempt, err)
					}
					var restricted, base ComplexTypeDefinition
					var note, group Component
					for _, component := range schema.Components() {
						switch component.Name().Local() {
						case "Restricted":
							restricted, _ = component.ComplexType()
						case "Derived":
							base, _ = component.ComplexType()
						case "note":
							note = component
						case "Fields":
							group = component
						}
					}
					if restricted.ID().IsZero() || base.ID().IsZero() || restricted.Derivation() != ComplexTypeDerivationRestriction || restricted.DerivationLoc() != complexContentTestLoc(t, root, `<xs:restriction base="t:Derived"`) || restricted.BaseLoc() != complexContentTestLoc(t, root, `base="t:Derived"`) || len(restricted.AttributeUses()) != 0 {
						t.Fatalf("published restriction: %v", restricted)
					}
					baseRef, hasBase := restricted.BaseReference()
					baseID, hasID := baseRef.ComponentID()
					if !hasBase || !hasID || baseID != base.ID() || baseRef.Name() != base.Name() || baseRef.Loc() != restricted.BaseLoc() {
						t.Fatalf("base reference = %v", baseRef)
					}
					if _, present := restricted.AnyAttribute(); present {
						t.Fatal("unexpected wildcard")
					}
					sequence, ok := restricted.Particle().(SequenceParticle)
					if !ok || !sequence.Occurrences().IsDefault() || sequence.Loc() != complexContentTestLoc(t, root, `<xs:sequence><xs:element ref="t:note" minOccurs="0"`) {
						t.Fatalf("authored sequence = %T", restricted.Particle())
					}
					members := sequence.Particles()
					if len(members) != 2 {
						t.Fatalf("members = %v", members)
					}
					element, elementOK := members[0].(ElementReferenceParticle)
					groupRef, groupOK := members[1].(ModelGroupReferenceParticle)
					if !elementOK || !groupOK || element.TargetID() != note.ID() || groupRef.TargetID() != group.ID() || element.Occurrences().String() != "0/1" || !groupRef.Occurrences().IsDefault() || element.RefLoc() != complexContentTestLoc(t, root, `ref="t:note" minOccurs="0"`) || groupRef.RefLoc() != complexContentTestLoc(t, root, `ref="t:Fields"/></xs:sequence>`) {
						t.Fatalf("copied references = %v", members)
					}
					members[0] = nil
					copied, copiedOK := restricted.Particle().(SequenceParticle)
					if !copiedOK || copied.Particles()[0] == nil {
						t.Fatal("particle returned mutable slice")
					}
					if attempt == 0 {
						first = schema.Components()
						continue
					}
					if !reflect.DeepEqual(first, schema.Components()) {
						t.Fatal("repeated public components changed")
					}
					validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
					if validationErr == nil || !errors.Is(validationErr, ErrUnsupported) {
						t.Fatalf("validation = %v", validationErr)
					}
					validation := requireDiagnostic(t, validationErr)
					wantValidationRelated := []Loc{
						complexContentTestLoc(t, root, `<xs:element name="root"`), restricted.Loc(), sequence.Loc(),
						complexContentTestLoc(t, root, `<xs:complexContent><xs:restriction base="t:Derived"`),
						restricted.DerivationLoc(), restricted.BaseLoc(),
					}
					if validation.Class() != FailureUnsupported || validation.Code() != UnsupportedInstanceValidationCode || validation.Loc() != mustTestLoc(t, "instance.xml", 1, 1) || !reflect.DeepEqual(validation.Related(), wantValidationRelated) || validation.SpecRef() != instanceValidationSpecRef(instanceSchemaValidationVersion(schema)) || !errors.Is(validationErr, errInstanceOpenAttrsType) {
						t.Fatalf("validation diagnostic = %v, related=%v", validationErr, validation.Related())
					}
					generated, generationErr := GenerateGo(schema, "generated")
					if generationErr == nil || generated != nil || !errors.Is(generationErr, ErrUnsupported) {
						t.Fatalf("generation = %q/%v", generated, generationErr)
					}
					generation := requireDiagnostic(t, generationErr)
					wantGenerationRelated := []Loc{restricted.Loc(), complexContentTestLoc(t, root, `<xs:complexContent><xs:restriction base="t:Derived"`), restricted.DerivationLoc(), restricted.BaseLoc(), sequence.Loc()}
					if generation.Class() != FailureUnsupported || generation.Code() != diagnosticCodegenUnsupported || generation.Loc() != restricted.Loc() || !reflect.DeepEqual(generation.Related(), wantGenerationRelated) || generation.SpecRef() != codegenDirectSequenceSpecReference(instanceSchemaValidationVersion(schema), codegenDirectSequenceParticlesReference) || !errors.Is(generationErr, errCodegenUnsupported) {
						t.Fatalf("generation diagnostic = %v, related=%v", generationErr, generation.Related())
					}
				}
			})
		}
	}
}

func boundedRestrictionVersionedRoot(root string, version XSDVersion) string {
	return strings.Replace(root, `targetNamespace="urn:root"`, `targetNamespace="urn:root" version="`+string(version)+`"`, 1)
}

func boundedRestrictionLoc(t *testing.T, root, marker string, last bool) Loc {
	t.Helper()
	if last {
		return namedRestrictionLastLoc(t, root, marker)
	}
	return complexContentTestLoc(t, root, marker)
}

func assertBoundedRestrictionFailure(t *testing.T, root string, policy LanguagePolicy, class FailureClass, code, primary string, primaryLast bool, related []string, specRef string, cause error) {
	t.Helper()
	var first Diagnostic
	for attempt := range 2 {
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
		if err == nil {
			t.Fatalf("attempt %d unexpectedly parsed", attempt)
		}
		assertZeroSchema(t, schema)
		diagnostic := requireDiagnostic(t, err)
		wantRelated := make([]Loc, 0, len(related))
		for _, marker := range related {
			last := strings.HasPrefix(marker, "last:")
			marker = strings.TrimPrefix(marker, "last:")
			wantRelated = append(wantRelated, boundedRestrictionLoc(t, root, marker, last))
		}
		wantPrimary := boundedRestrictionLoc(t, root, primary, primaryLast)
		if diagnostic.Class() != class || diagnostic.Code() != code || diagnostic.Loc() != wantPrimary || !slices.Equal(diagnostic.Related(), wantRelated) || diagnostic.SpecRef() != specRef || !errors.Is(err, cause) {
			t.Fatalf("attempt %d diagnostic = %v, related=%v, spec=%q; want %s/%s at %s (%q), related=%v, spec=%q, cause=%v (matches=%t)", attempt, diagnostic, diagnostic.Related(), diagnostic.SpecRef(), class, code, wantPrimary, primary, wantRelated, specRef, cause, errors.Is(err, cause))
		}
		if class == FailureUnsupported && !errors.Is(err, ErrUnsupported) {
			t.Fatalf("attempt %d lost unsupported marker: %v", attempt, err)
		}
		if attempt == 0 {
			first = diagnostic
			continue
		}
		if first.Error() != diagnostic.Error() {
			t.Fatalf("repeat diagnostic changed: %v / %v", first, diagnostic)
		}
	}
}

// Each exit changes one obligation after resolving the same completed base.
func TestBoundedParticleRestrictionProofExits(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{
		{Strict10, XSDVersion10}, {Strict11, XSDVersion11}, {Compatibility, XSDVersion11},
	} {
		t.Run(string(profile.policy)+"/"+string(profile.version), func(t *testing.T) {
			base := boundedRestrictionVersionedRoot(boundedParticleRestrictionRoot("direct", ` minOccurs="0"`, ""), profile.version)
			particleSpec := "xsd11-structures#cos-content-act-restrict"
			if profile.version == XSDVersion10 {
				particleSpec = "xsd10-structures#cos-particle-restrict"
			}
			duplicateGroup := strings.Replace(base, `</xs:schema>`, `<xs:group name="Fields"><xs:choice><xs:element ref="t:field"/><xs:element ref="t:second"/><xs:element ref="t:third"/></xs:choice></xs:group></xs:schema>`, 1)
			for _, test := range []struct {
				name, root, code, primary, spec string
				primaryLast                     bool
				related                         []string
				class                           FailureClass
				cause                           error
			}{
				{"element widened", boundedRestrictionVersionedRoot(boundedParticleRestrictionRoot("direct", ` minOccurs="0" maxOccurs="2"`, ""), profile.version), invalidSchemaCompositionCode, `<xs:element ref="t:note" minOccurs="0" maxOccurs="2"`, particleSpec, false, []string{`last:<xs:element ref="t:note" minOccurs="0"`}, FailureInvalid, errSchemaComplexTypeParticleRestriction},
				{"element narrowed", boundedRestrictionVersionedRoot(boundedParticleRestrictionRoot("direct", "", ""), profile.version), UnsupportedSchemaSyntaxCode, `<xs:element ref="t:note"/>`, particleSpec, false, []string{`last:<xs:element ref="t:note" minOccurs="0"`}, FailureUnsupported, errSchemaComplexTypeParticleUnsupported},
				{"malformed element bounds", boundedRestrictionVersionedRoot(boundedParticleRestrictionRoot("direct", ` minOccurs="2" maxOccurs="1"`, ""), profile.version), invalidSchemaCompositionCode, `<xs:element ref="t:note" minOccurs="2" maxOccurs="1"`, schemaParticleCorrectSpecRef(profile.version), false, []string{`minOccurs="2"`, `maxOccurs="1"`}, FailureInvalid, errParticleOccurrenceMinimumExceedsMaximum},
				{"group widened", boundedRestrictionVersionedRoot(boundedParticleRestrictionRoot("direct", ` minOccurs="0"`, ` maxOccurs="2"`), profile.version), invalidSchemaCompositionCode, `<xs:group ref="t:Fields" maxOccurs="2"`, particleSpec, false, []string{`last:<xs:group ref="t:Fields"/>`}, FailureInvalid, errSchemaComplexTypeParticleRestriction},
				{"group narrowed", boundedRestrictionVersionedRoot(boundedParticleRestrictionRoot("direct", ` minOccurs="0"`, ` minOccurs="0"`), profile.version), UnsupportedSchemaSyntaxCode, `<xs:group ref="t:Fields" minOccurs="0"`, particleSpec, false, []string{`last:<xs:group ref="t:Fields"/>`}, FailureUnsupported, errSchemaComplexTypeParticleUnsupported},
				{"element omitted", boundedRestrictionVersionedRoot(boundedParticleRestrictionRoot("direct", ` minOccurs="0" maxOccurs="0"`, ""), profile.version), UnsupportedSchemaSyntaxCode, `<xs:sequence><xs:element ref="t:note"`, particleSpec, false, []string{`<xs:complexType name="Derived"`}, FailureUnsupported, errSchemaComplexTypeParticleUnsupported},
				{"omitted element still resolves", strings.Replace(boundedRestrictionVersionedRoot(boundedParticleRestrictionRoot("direct", ` minOccurs="0" maxOccurs="0"`, ""), profile.version), `ref="t:note" minOccurs="0" maxOccurs="0"`, `ref="t:Missing" minOccurs="0" maxOccurs="0"`, 1), diagnosticSchemaElementReferenceUnresolvedCode, `ref="t:Missing"`, schemaElementReferenceSpecRef(profile.version), false, nil, FailureInvalid, errSchemaElementReferenceUnresolved},
				{"missing group", strings.Replace(base, `ref="t:Fields"/></xs:sequence>`, `ref="t:Missing"/></xs:sequence>`, 1), diagnosticSchemaModelGroupReferenceUnresolvedCode, `ref="t:Missing"`, schemaModelGroupReferenceSpecRef(profile.version), false, nil, FailureInvalid, errSchemaModelGroupReferenceUnresolved},
				{"missing element", strings.Replace(base, `ref="t:note" minOccurs="0"`, `ref="t:Missing" minOccurs="0"`, 1), diagnosticSchemaElementReferenceUnresolvedCode, `ref="t:Missing"`, schemaElementReferenceSpecRef(profile.version), false, nil, FailureInvalid, errSchemaElementReferenceUnresolved},
				{"missing base", strings.Replace(base, `base="t:Derived"`, `base="t:Missing"`, 1), invalidSchemaCompositionCode, `base="t:Missing"`, schemaComplexTypeDerivationSpecRef(profile.version), false, nil, FailureInvalid, errSchemaComplexTypeBaseUnresolved},
				{"wrong kind group", strings.Replace(strings.Replace(base, `ref="t:Fields"/></xs:sequence>`, `ref="t:Wrong"/></xs:sequence>`, 1), `</xs:schema>`, `<xs:element name="Wrong" type="xs:integer"/></xs:schema>`, 1), diagnosticSchemaModelGroupReferenceWrongKindCode, `ref="t:Wrong"`, schemaModelGroupReferenceSpecRef(profile.version), false, []string{`<xs:element name="Wrong"`}, FailureInvalid, errSchemaModelGroupReferenceWrongKind},
				{"wrong kind element", strings.Replace(strings.Replace(base, `ref="t:note" minOccurs="0"`, `ref="t:Wrong" minOccurs="0"`, 1), `</xs:schema>`, `<xs:group name="Wrong"><xs:sequence/></xs:group></xs:schema>`, 1), diagnosticSchemaElementReferenceWrongKindCode, `ref="t:Wrong"`, schemaElementReferenceSpecRef(profile.version), false, []string{`<xs:group name="Wrong"`}, FailureInvalid, errSchemaElementReferenceWrongKind},
				{"final restriction", strings.Replace(base, `<xs:complexType name="Derived">`, `<xs:complexType name="Derived" final="restriction">`, 1), invalidSchemaCompositionCode, `base="t:Derived"`, schemaComplexTypeDerivationSpecRef(profile.version), false, []string{`final="restriction"`}, FailureInvalid, errSchemaComplexTypeBaseUnsupported},
				{"extension cycle", strings.Replace(base, `base="t:Base"`, `base="t:Restricted"`, 1), invalidSchemaCompositionCode, `base="t:Restricted"`, schemaComplexTypeExtensionSpecRef(profile.version), false, []string{`base="t:Derived"`}, FailureInvalid, errSchemaComplexTypeBaseCycle},
				{"ambiguous group masked by duplicate declaration", duplicateGroup, diagnosticSchemaGlobalDuplicateCode, `<xs:group name="Fields"`, schemaGlobalDuplicateSpecRef(profile.version), true, []string{`<xs:group name="Fields"`}, FailureInvalid, errSchemaGlobalDeclarationDuplicate},
			} {
				t.Run(test.name, func(t *testing.T) {
					assertBoundedRestrictionFailure(t, test.root, profile.policy, test.class, test.code, test.primary, test.primaryLast, test.related, test.spec, test.cause)
				})
			}
		})
	}
}

func boundedRestrictionGraph(version XSDVersion, relation string) (string, map[string]discoveryFixture) {
	partNamespace := "urn:graph"
	rootNamespace := partNamespace
	include := `<xs:include schemaLocation="part.xsd"/>`
	if relation == "import" {
		rootNamespace = "urn:owner"
		include = `<xs:import namespace="urn:graph" schemaLocation="part.xsd"/>`
	}
	partTarget := ` targetNamespace="urn:graph"`
	if relation == "chameleon" {
		partTarget = ""
	}
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:p="urn:graph" targetNamespace="` + rootNamespace + `" version="` + string(version) + `">` + include +
		`<xs:element name="root" type="` + func() string {
		if relation == "import" {
			return "o:Restricted"
		}
		return "p:Restricted"
	}() + `"/><xs:complexType name="Restricted"><xs:complexContent><xs:restriction base="p:Derived"><xs:sequence><xs:element ref="p:note" minOccurs="0"/><xs:group ref="p:Fields"/></xs:sequence></xs:restriction></xs:complexContent></xs:complexType></xs:schema>`
	if relation == "import" {
		root = strings.Replace(root, `xmlns:p="urn:graph"`, `xmlns:p="urn:graph" xmlns:o="urn:owner"`, 1)
	}
	part := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:p="urn:graph"` + partTarget + ` version="` + string(version) + `">` +
		`<xs:complexType name="Derived"><xs:complexContent><xs:extension base="p:Base"><xs:group ref="p:Fields"/></xs:extension></xs:complexContent></xs:complexType>` +
		`<xs:complexType name="Base"><xs:sequence><xs:element ref="p:note" minOccurs="0"/></xs:sequence></xs:complexType>` +
		`<xs:group name="Fields"><xs:choice><xs:element ref="p:one"/><xs:element ref="p:two"/><xs:element ref="p:three"/></xs:choice></xs:group>` +
		`<xs:element name="note" type="xs:string"/><xs:element name="one" type="xs:string"/><xs:element name="two" type="xs:string"/><xs:element name="three" type="xs:string"/></xs:schema>`
	return root, map[string]discoveryFixture{"part.xsd": {id: "part.xsd", contents: part}}
}

//nolint:gocognit // Query cross-document identities, order, and source locations per policy.
func TestBoundedParticleRestrictionGraphVisibility(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{{Strict10, XSDVersion10}, {Strict11, XSDVersion11}, {Compatibility, XSDVersion11}} {
		for _, relation := range []string{"include", "chameleon", "import"} {
			t.Run(string(profile.policy)+"/"+relation, func(t *testing.T) {
				root, fixtures := boundedRestrictionGraph(profile.version, relation)
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				if len(schema.Documents()) != 2 || len(schema.Components()) != 9 {
					t.Fatalf("documents/components = %d/%d", len(schema.Documents()), len(schema.Components()))
				}
				var restricted, base ComplexTypeDefinition
				var note, group Component
				for _, component := range schema.Components() {
					switch component.Name().Local() {
					case "Restricted":
						restricted, _ = component.ComplexType()
					case "Derived":
						base, _ = component.ComplexType()
					case "note":
						note = component
					case "Fields":
						group = component
					}
				}
				baseRef, hasBase := restricted.BaseReference()
				baseID, hasID := baseRef.ComponentID()
				sequence, ok := restricted.Particle().(SequenceParticle)
				if !ok || !hasBase || !hasID || baseID != base.ID() || len(sequence.Particles()) != 2 || restricted.ID().Source() != "root.xsd" || base.ID().Source() != "part.xsd" {
					t.Fatalf("resolved graph facts = %v/%v", restricted, baseRef)
				}
				annotation, elementOK := sequence.Particles()[0].(ElementReferenceParticle)
				groupRef, groupOK := sequence.Particles()[1].(ModelGroupReferenceParticle)
				if !elementOK || !groupOK || annotation.TargetID() != note.ID() || groupRef.TargetID() != group.ID() || annotation.RefLoc() != complexContentTestLoc(t, root, `ref="p:note"`) || groupRef.RefLoc() != complexContentTestLoc(t, root, `ref="p:Fields"`) {
					t.Fatalf("cross-document targets = %v/%v", annotation, groupRef)
				}
			})
		}
	}
}

func TestBoundedParticleRestrictionHiddenGroup(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{{Strict10, XSDVersion10}, {Strict11, XSDVersion11}, {Compatibility, XSDVersion11}} {
		t.Run(string(profile.policy), func(t *testing.T) {
			root, fixtures := boundedRestrictionGraph(profile.version, "import")
			root = strings.Replace(root, `xmlns:o="urn:owner"`, `xmlns:o="urn:owner" xmlns:h="urn:hidden"`, 1)
			root = strings.Replace(root, `ref="p:Fields"`, `ref="h:Fields"`, 1)
			part := fixtures["part.xsd"]
			part.contents = strings.Replace(part.contents, `targetNamespace="urn:graph"`, `targetNamespace="urn:graph" xmlns:h="urn:hidden"`, 1)
			part.contents = strings.Replace(part.contents, `version="`+string(profile.version)+`">`, `version="`+string(profile.version)+`"><xs:import namespace="urn:hidden" schemaLocation="hidden.xsd"/>`, 1)
			part.contents = strings.Replace(part.contents, `ref="p:Fields"`, `ref="h:Fields"`, 1)
			fixtures["part.xsd"] = part
			hidden := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:h="urn:hidden" targetNamespace="urn:hidden" version="` + string(profile.version) + `"><xs:group name="Fields"><xs:choice><xs:element ref="h:one"/><xs:element ref="h:two"/><xs:element ref="h:three"/></xs:choice></xs:group><xs:element name="one" type="xs:string"/><xs:element name="two" type="xs:string"/><xs:element name="three" type="xs:string"/></xs:schema>`
			fixtures["hidden.xsd"] = discoveryFixture{id: "hidden.xsd", contents: hidden}
			for attempt := range 2 {
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				if err == nil {
					t.Fatalf("attempt %d made unimported group visible", attempt)
				}
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantRelated := []Loc{allParticleTestTokenLoc(t, "hidden.xsd", hidden, `<xs:group name="Fields"`, 1)}
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaModelGroupReferenceNamespaceCode || diagnostic.Loc() != complexContentTestLoc(t, root, `ref="h:Fields"`) || !reflect.DeepEqual(diagnostic.Related(), wantRelated) || diagnostic.SpecRef() != schemaModelGroupReferenceImportSpecRef(profile.version) || !errors.Is(err, errSchemaModelGroupReferenceNamespace) {
					t.Fatalf("attempt %d hidden group diagnostic = %v, related=%v", attempt, diagnostic, diagnostic.Related())
				}
			}
		})
	}
}

func boundedRestrictionInlineOwner(root string) string {
	const open = `<xs:complexType name="Restricted">`
	start := strings.Index(root, open)
	if start < 0 {
		return root
	}
	end := strings.Index(root[start:], `</xs:complexType>`)
	if end < 0 {
		return root
	}
	end += start + len(`</xs:complexType>`)
	body := root[start+len(open) : end-len(`</xs:complexType>`)]
	root = root[:start] + root[end:]
	return strings.Replace(root, `<xs:element name="root" type="t:Restricted"/>`, `<xs:element name="root"><xs:complexType>`+body+`</xs:complexType></xs:element>`, 1)
}

func TestBoundedParticleRestrictionExcludedShapes(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{{Strict10, XSDVersion10}, {Strict11, XSDVersion11}, {Compatibility, XSDVersion11}} {
		t.Run(string(profile.policy), func(t *testing.T) {
			root := boundedRestrictionVersionedRoot(boundedParticleRestrictionRoot("direct", ` minOccurs="0"`, ""), profile.version)
			particleSpec := "xsd11-structures#cos-content-act-restrict"
			if profile.version == XSDVersion10 {
				particleSpec = "xsd10-structures#cos-particle-restrict"
			}
			wildcard := strings.Replace(root, `</xs:sequence></xs:complexType>`, `</xs:sequence><xs:anyAttribute namespace="##other" processContents="lax"/></xs:complexType>`, 1)
			uses := strings.Replace(root, `<xs:group ref="t:Fields"/></xs:extension>`, `<xs:group ref="t:Fields"/><xs:attribute name="used" type="xs:integer"/></xs:extension>`, 1)
			otherGroup := strings.Replace(root, `<xs:group name="Fields">`, `<xs:group name="Other"><xs:choice><xs:element ref="t:field"/><xs:element ref="t:second"/><xs:element ref="t:third"/></xs:choice></xs:group><xs:group name="Fields">`, 1)
			otherGroup = strings.Replace(otherGroup, `ref="t:Fields"/></xs:sequence>`, `ref="t:Other"/></xs:sequence>`, 1)
			groupSequence := strings.Replace(root, `<xs:choice>`, `<xs:sequence>`, 1)
			groupSequence = strings.Replace(groupSequence, `</xs:choice>`, `</xs:sequence>`, 1)
			for _, test := range []struct {
				name, source, primary, related, spec string
			}{
				{"inline owner", boundedRestrictionInlineOwner(root), `<xs:restriction base="t:Derived"`, `<xs:complexType name="Derived"`, particleSpec},
				{"inherited wildcard", wildcard, `base="t:Derived"`, `<xs:complexType name="Derived"`, particleSpec},
				{"different group identity", otherGroup, `ref="t:Other"`, `last:ref="t:Fields"`, particleSpec},
				{"other group language", groupSequence, `ref="t:Fields"/></xs:sequence>`, `<xs:group name="Fields"`, particleSpec},
			} {
				t.Run(test.name, func(t *testing.T) {
					assertBoundedRestrictionFailure(t, test.source, profile.policy, FailureUnsupported, UnsupportedSchemaSyntaxCode, test.primary, false, []string{test.related}, test.spec, errSchemaComplexTypeParticleUnsupported)
				})
			}
			t.Run("effective use earlier base gate", func(t *testing.T) {
				assertBoundedRestrictionFailure(t, uses, profile.policy, FailureUnsupported, UnsupportedSchemaSyntaxCode,
					`base="t:Base"`, false,
					[]string{`<xs:complexType name="Base"`, `last:<xs:sequence><xs:element ref="t:note" minOccurs="0"`},
					schemaComplexTypeExtensionSpecRef(profile.version), errSchemaComplexTypeBaseNonEmpty)
			})
		})
	}
}

//nolint:gocognit // Compare named and inline exclusion exits in every policy.
func TestBoundedParticleRestrictionExcludedDerivedUses(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{{Strict10, XSDVersion10}, {Strict11, XSDVersion11}, {Compatibility, XSDVersion11}} {
		t.Run(string(profile.policy), func(t *testing.T) {
			root := boundedRestrictionVersionedRoot(boundedParticleRestrictionRoot("direct", ` minOccurs="0"`, ""), profile.version)
			particleSpec := "xsd11-structures#cos-content-act-restrict"
			if profile.version == XSDVersion10 {
				particleSpec = "xsd10-structures#cos-particle-restrict"
			}
			for _, owner := range []string{"named", "inline"} {
				for _, excluded := range []struct {
					name, child, primary string
				}{
					{"local use", `<xs:attribute name="used" type="xs:integer"/>`, `<xs:attribute name="used"`},
					{"reference use", `<xs:attribute ref="t:used"/>`, `<xs:attribute ref="t:used"`},
					{"prohibited reference", `<xs:attribute ref="t:used" use="prohibited"/>`, `<xs:attribute ref="t:used" use="prohibited"`},
					{"wildcard", `<xs:anyAttribute namespace="##other" processContents="lax"/>`, `<xs:anyAttribute namespace="##other"`},
					{"use then wildcard", `<xs:attribute ref="t:used"/><xs:anyAttribute namespace="##other" processContents="lax"/>`, `<xs:attribute ref="t:used"`},
				} {
					t.Run(owner+"/"+excluded.name, func(t *testing.T) {
						withChild := strings.Replace(root, `</xs:sequence></xs:restriction>`, `</xs:sequence>`+excluded.child+`</xs:restriction>`, 1)
						withChild = strings.Replace(withChild, `</xs:schema>`, `<xs:attribute name="used" type="xs:integer"/></xs:schema>`, 1)
						if owner == "inline" {
							withChild = boundedRestrictionInlineOwner(withChild)
						}
						assertBoundedRestrictionFailure(t, withChild, profile.policy, FailureUnsupported, UnsupportedSchemaSyntaxCode,
							excluded.primary, false, []string{`<xs:complexType name="Derived"`}, particleSpec, errSchemaComplexTypeParticleUnsupported)
					})
				}
			}
			t.Run("missing attribute target precedes unsupported", func(t *testing.T) {
				withUse := strings.Replace(root, `</xs:sequence></xs:restriction>`, `</xs:sequence><xs:attribute ref="t:Missing"/></xs:restriction>`, 1)
				assertBoundedRestrictionFailure(t, withUse, profile.policy, FailureInvalid, diagnosticSchemaAttributeReferenceUnresolvedCode,
					`ref="t:Missing"`, false, nil, schemaAttributeUseResolveSpecRef(profile.version), errSchemaAttributeReferenceUnresolved)
			})
			t.Run("missing element target precedes excluded wildcard", func(t *testing.T) {
				withWildcard := strings.Replace(root, `</xs:sequence></xs:restriction>`, `</xs:sequence><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction>`, 1)
				withWildcard = strings.Replace(withWildcard, `ref="t:note" minOccurs="0"`, `ref="t:Missing" minOccurs="0"`, 1)
				assertBoundedRestrictionFailure(t, withWildcard, profile.policy, FailureInvalid, diagnosticSchemaElementReferenceUnresolvedCode,
					`ref="t:Missing"`, false, nil, schemaElementReferenceSpecRef(profile.version), errSchemaElementReferenceUnresolved)
			})
		})
	}
}

func TestBoundedParticleRestrictionAttributionAndConsistency(t *testing.T) {
	for _, profile := range []struct {
		policy  LanguagePolicy
		version XSDVersion
	}{{Strict10, XSDVersion10}, {Strict11, XSDVersion11}, {Compatibility, XSDVersion11}} {
		t.Run(string(profile.policy), func(t *testing.T) {
			root := boundedRestrictionVersionedRoot(boundedParticleRestrictionRoot("direct", ` minOccurs="0"`, ""), profile.version)
			overlap := strings.Replace(root, `<xs:element ref="t:field"/>`, `<xs:element ref="t:note"/>`, 1)
			assertBoundedRestrictionFailure(t, overlap, profile.policy, FailureInvalid, invalidSchemaCompositionCode,
				`ref="t:note"/><xs:element ref="t:second"`, false, []string{`ref="t:note" minOccurs="0"`},
				schemaGroupNonAmbiguousSpecRef(profile.version), errSchemaComplexTypeParticleRestriction)

			inconsistent := strings.Replace(root, `targetNamespace="urn:root"`, `targetNamespace="urn:root" elementFormDefault="qualified"`, 1)
			inconsistent = strings.Replace(inconsistent, `<xs:element ref="t:field"/>`, `<xs:element name="note" type="xs:integer"/>`, 1)
			assertBoundedRestrictionFailure(t, inconsistent, profile.policy, FailureUnsupported, UnsupportedSchemaSyntaxCode,
				`<xs:element name="note" type="xs:integer"`, false, nil,
				schemaSyntaxSpecRefForVersion(profile.version), ErrUnsupported)
		})
	}
}
