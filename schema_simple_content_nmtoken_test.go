package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func nmtokenSimpleContentNamed(name string) string {
	return `<xs:simpleType name="` + name + `"><xs:restriction base="xs:NMTOKEN"><xs:enumeration value="A:B"/><xs:enumeration value="C-D"/></xs:restriction></xs:simpleType>`
}

func nmtokenSyntaxSpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return "xsd10-structures#schema-document"
	}
	return "xsd11-structures#cSchemaDocument"
}

//nolint:gocognit,funlen // The same public facts are checked across policies and graph edges.
func TestNMTOKENSimpleContentQueries(t *testing.T) {
	for _, profile := range tokenSimpleContentPolicies() {
		for _, test := range []struct {
			name, base, before, after, source, document, variety string
		}{
			{"built-in", "xs:NMTOKEN", "", "", "", "", ""},
			{"forward named", "t:Forward", "", nmtokenSimpleContentNamed("Forward"), "root.xsd", "", `<xs:restriction base="xs:NMTOKEN"`},
			{"explicit whitespace", "t:Spaced", `<xs:simpleType name="Spaced"><xs:restriction base="xs:NMTOKEN"><xs:whiteSpace value="collapse" fixed="true"/><xs:enumeration value="A:B"/><xs:enumeration value="C-D"/></xs:restriction></xs:simpleType>`, "", "root.xsd", "", `<xs:restriction base="xs:NMTOKEN"`},
			{"effective named", "t:Derived", "", `<xs:simpleType name="Derived"><xs:restriction base="t:Forward"/></xs:simpleType>` + nmtokenSimpleContentNamed("Forward"), "root.xsd", "", `<xs:restriction base="t:Forward"`},
			{"include", "t:External", `<xs:include schemaLocation="part.xsd"/>`, "", "part.xsd", tokenSimpleContentRoot(nmtokenSimpleContentNamed("External")), `<xs:restriction base="xs:NMTOKEN"`},
			{"chameleon", "t:External", `<xs:include schemaLocation="part.xsd"/>`, "", "part.xsd", `<xs:schema xmlns:xs="` + testXSDNamespace + `">` + nmtokenSimpleContentNamed("External") + `</xs:schema>`, `<xs:restriction base="xs:NMTOKEN"`},
			{"import", "o:External", `<xs:import namespace="urn:other" schemaLocation="part.xsd"/>`, "", "part.xsd", `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other">` + nmtokenSimpleContentNamed("External") + `</xs:schema>`, `<xs:restriction base="xs:NMTOKEN"`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := tokenSimpleContentRoot(test.before + tokenSimpleContentOwner(test.base, "") + test.after + `<xs:element name="root" type="t:Value"/>`)
				root = strings.Replace(root, `xmlns:t="urn:token"`, `xmlns:t="urn:token" xmlns:o="urn:other"`, 1)
				fixtures := map[string]discoveryFixture{}
				if test.document != "" {
					fixtures[test.source] = discoveryFixture{id: SourceID(test.source), contents: test.document}
				}
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				if err != nil {
					t.Fatalf("parse: %v", err)
				}
				owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:token", "Value")
				if owner.Particle() != nil || len(owner.AttributeUses()) != 0 {
					t.Fatalf("particle/uses = %v/%v", owner.Particle(), owner.AttributeUses())
				}
				extension, ok := owner.SimpleContentExtension()
				if !ok {
					t.Fatal("missing simple-content extension")
				}
				baseNamespace := "urn:token"
				if test.name == "import" {
					baseNamespace = "urn:other"
				}
				if test.name == "built-in" {
					baseNamespace = testXSDNamespace
				}
				wantBase := mustTestQName(t, baseNamespace, strings.Split(test.base, ":")[1])
				baseLoc := tokenSimpleContentLoc(t, "root.xsd", root, `base="`+test.base+`"`)
				if extension.Base() != wantBase || extension.BaseLoc() != baseLoc {
					t.Fatalf("authored base = %s/%s, want %s/%s", extension.Base(), extension.BaseLoc(), wantBase, baseLoc)
				}
				reference, ok := extension.TypeReference()
				if !ok || reference.Name() != wantBase || reference.Loc() != baseLoc || reference.Variety() != SimpleTypeVarietyAtomicRestriction {
					t.Fatalf("type reference = %#v/%t", reference, ok)
				}
				space, ok := reference.StringWhiteSpaceFacet()
				if !ok || space.Value() != "collapse" || !space.Fixed() {
					t.Fatalf("whitespace = %#v/%t", space, ok)
				}
				if test.name == "explicit whitespace" && space.Loc() != tokenSimpleContentLoc(t, "root.xsd", root, `value="collapse"`) {
					t.Fatalf("whitespace location = %s", space.Loc())
				}
				id, hasID := extension.TypeID()
				refID, hasRefID := reference.ComponentID()
				if test.name == "built-in" {
					if !reference.IsBuiltin() || hasID || hasRefID || !id.IsZero() || !refID.IsZero() || reference.VarietyLoc() != baseLoc || !space.Loc().IsZero() {
						t.Fatalf("built-in provenance = %#v/%t/%#v/%t", id, hasID, refID, hasRefID)
					}
				}
				if test.name != "built-in" {
					definition := root
					if test.document != "" {
						definition = test.document
					}
					wantVariety := tokenSimpleContentLoc(t, SourceID(test.source), definition, test.variety)
					if !reference.IsNamed() || !hasID || !hasRefID || id != refID || id != componentIDForName(t, schema, wantBase) || reference.VarietyLoc() != wantVariety {
						t.Fatalf("named provenance = %#v/%t/%#v/%t/%s want %s", id, hasID, refID, hasRefID, reference.VarietyLoc(), wantVariety)
					}
					component, found := schema.Lookup(id)
					simpleDefinition, simple := component.SimpleTypeDefinition()
					writtenBase := mustTestQName(t, testXSDNamespace, "NMTOKEN")
					baseMarker := `base="xs:NMTOKEN"`
					if test.name == "effective named" {
						writtenBase = mustTestQName(t, "urn:token", "Forward")
						baseMarker = `base="t:Forward"`
					}
					wantDerivedBaseLoc := tokenSimpleContentLoc(t, SourceID(test.source), definition, baseMarker)
					if !found || !simple || simpleDefinition.Base() != writtenBase || simpleDefinition.BaseLoc() != wantDerivedBaseLoc {
						t.Fatalf("named derivation = %v/%t/%s/%s", found, simple, simpleDefinition.Base(), simpleDefinition.BaseLoc())
					}
					baseReference, hasBaseReference := simpleDefinition.BaseReference()
					if !hasBaseReference || baseReference.Name() != writtenBase || baseReference.Loc() != wantDerivedBaseLoc {
						t.Fatalf("named base reference = %#v/%t", baseReference, hasBaseReference)
					}
					values := reference.StringEnumerationFacets().Values()
					locations := reference.StringEnumerationFacets().Locations()
					if !reflect.DeepEqual(values, []string{"A:B", "C-D"}) || len(locations) != 2 {
						t.Fatalf("effective enumeration = %v/%v", values, locations)
					}
					if test.name != "effective named" {
						want := tokenSimpleContentLoc(t, SourceID(test.source), definition, `value="A:B"`)
						if locations[0] != want {
							t.Fatalf("enumeration location = %s, want %s", locations[0], want)
						}
					}
					values[0] = "changed"
					locations[0] = Loc{}
					if reference.StringEnumerationFacets().Values()[0] != "A:B" || reference.StringEnumerationFacets().Locations()[0].IsZero() {
						t.Fatal("mutated completed facets")
					}
				}
				components := schema.Components()
				components[0] = Component{}
				var walked []string
				if err := schema.Walk(func(component Component) error { walked = append(walked, component.Name().Local()); return nil }); err != nil {
					t.Fatalf("walk: %v", err)
				}
				wantWalk := []string{"Value", "root"}
				if test.name == "forward named" {
					wantWalk = []string{"Value", "Forward", "root"}
				}
				if test.name == "explicit whitespace" {
					wantWalk = []string{"Spaced", "Value", "root"}
				}
				if test.name == "effective named" {
					wantWalk = []string{"Value", "Derived", "Forward", "root"}
				}
				if test.document != "" {
					wantWalk = []string{"Value", "root", "External"}
				}
				if !reflect.DeepEqual(walked, wantWalk) {
					t.Fatalf("walk = %v, want %v", walked, wantWalk)
				}
			})
		}
	}
}

func TestNMTOKENSimpleContentAttributeExclusions(t *testing.T) {
	for _, profile := range tokenSimpleContentPolicies() {
		for _, base := range []string{"xs:NMTOKEN", "t:Named"} {
			for _, test := range []struct{ name, use, extra, marker string }{
				{"direct", `<xs:attribute name="flag" type="xs:boolean"/>`, "", `<xs:attribute name="flag"`},
				{"named", `<xs:attribute name="flag" type="t:Flag"/>`, `<xs:simpleType name="Flag"><xs:restriction base="xs:boolean"/></xs:simpleType>`, `<xs:attribute name="flag"`},
				{"inline", `<xs:attribute name="flag"><xs:simpleType><xs:restriction base="xs:boolean"/></xs:simpleType></xs:attribute>`, "", `<xs:attribute name="flag"`},
				{"ref", `<xs:attribute ref="t:flag"/>`, `<xs:attribute name="flag" type="xs:boolean"/>`, `<xs:attribute ref="t:flag"`},
				{"prohibited", `<xs:attribute name="flag" type="xs:boolean" use="prohibited"/>`, "", `<xs:attribute name="flag"`},
			} {
				t.Run(profile.name+"/"+base+"/"+test.name, func(t *testing.T) {
					root := tokenSimpleContentRoot(tokenSimpleContentOwner(base, test.use) + nmtokenSimpleContentNamed("Named") + test.extra)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertZeroSchema(t, schema)
					diagnostic := requireDiagnostic(t, err)
					want := tokenSimpleContentLoc(t, "root.xsd", root, test.marker)
					if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != want || len(diagnostic.Related()) != 0 || diagnostic.SpecRef() != schemaSimpleContentSpecRef(profile.version) || !errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaSimpleContentBaseUnsupported) {
						t.Fatalf("attribute exclusion = %s", diagnostic)
					}
				})
			}
		}
	}
}

//nolint:gocognit // Both consumer paths retain their separate public diagnostics.
func TestNMTOKENSimpleContentConsumers(t *testing.T) {
	for _, profile := range tokenSimpleContentPolicies() {
		for _, base := range []string{"xs:NMTOKEN", "t:Named"} {
			t.Run(profile.name+"/"+base, func(t *testing.T) {
				root := tokenSimpleContentRoot(tokenSimpleContentOwner(base, "") + nmtokenSimpleContentNamed("Named") + `<xs:element name="root" type="t:Value"/>`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				instanceErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:token">A:B</root>`)))
				instance := requireDiagnostic(t, instanceErr)
				ownerLoc := tokenSimpleContentLoc(t, "root.xsd", root, `<xs:complexType name="Value"`)
				elementLoc := tokenSimpleContentLoc(t, "root.xsd", root, `<xs:element name="root"`)
				if instance.Class() != FailureUnsupported || instance.Code() != UnsupportedInstanceValidationCode || instance.Loc() != mustTestLoc(t, "instance.xml", 1, 1) || !reflect.DeepEqual(instance.Related(), []Loc{elementLoc, ownerLoc}) || instance.SpecRef() != instanceValidationSpecRef(profile.version) || !errors.Is(instanceErr, ErrUnsupported) {
					t.Fatalf("validator diagnostic = %s", instance)
				}
				output, generationErr := GenerateGo(schema, "generated")
				generation := requireDiagnostic(t, generationErr)
				if output != nil || generation.Class() != FailureUnsupported || generation.Code() != diagnosticCodegenUnsupported || generation.Loc() != ownerLoc || len(generation.Related()) != 0 || generation.SpecRef() != codegenDirectChoiceSpecReference(profile.version, codegenDirectChoiceParticlesReference) || !errors.Is(generationErr, ErrUnsupported) {
					t.Fatalf("generator result = %v/%s", output, generation)
				}
			})
		}
	}
}

//nolint:gocognit // Distinct exits at the base boundary preserve their own location, code, and cause.
func TestNMTOKENSimpleContentBaseDiagnostics(t *testing.T) {
	for _, profile := range tokenSimpleContentPolicies() {
		for _, test := range []struct {
			name, base, extra, primary, related, code, spec string
			class                                           FailureClass
			cause                                           error
		}{
			{"missing", "t:Missing", "", `base="t:Missing"`, "", diagnosticSchemaSimpleContentBaseUnresolvedCode, schemaSimpleContentSpecRef(profile.version), FailureInvalid, errSchemaSimpleContentBaseUnresolved},
			{"wrong kind", "t:Wrong", `<xs:complexType name="Wrong"/>`, `base="t:Wrong"`, `<xs:complexType name="Wrong"`, diagnosticSchemaSimpleContentBaseWrongKindCode, schemaSimpleContentSpecRef(profile.version), FailureInvalid, errSchemaSimpleContentBaseWrongKind},
			{"list", "t:List", `<xs:simpleType name="List"><xs:list itemType="xs:NMTOKEN"/></xs:simpleType>`, `base="t:List"`, "", UnsupportedSchemaSyntaxCode, schemaSimpleContentSpecRef(profile.version), FailureUnsupported, errSchemaSimpleContentBaseUnsupported},
			{"union", "t:Union", `<xs:simpleType name="Union"><xs:union memberTypes="xs:NMTOKEN xs:token"/></xs:simpleType>`, `base="t:Union"`, "", UnsupportedSchemaSyntaxCode, schemaSimpleContentSpecRef(profile.version), FailureUnsupported, errSchemaSimpleContentBaseUnsupported},
			{"invalid enumeration", "t:Bad", `<xs:simpleType name="Bad"><xs:restriction base="xs:NMTOKEN"><xs:enumeration value="bad/value"/></xs:restriction></xs:simpleType>`, `value="bad/value"`, "", InvalidEnumerationRestrictionCode, enumerationSpecRef(profile.version, enumerationRestrictionRule), FailureInvalid, errSchemaNMTOKENValueViolation},
			{"invalid whitespace", "t:Bad", `<xs:simpleType name="Bad"><xs:restriction base="xs:NMTOKEN"><xs:whiteSpace value="preserve"/></xs:restriction></xs:simpleType>`, `value="preserve"`, "", InvalidStringWhiteSpaceRestrictionCode, stringWhiteSpaceSpecRef(profile.version), FailureInvalid, errInvalidStringWhiteSpaceRestriction},
			{"adjacent normalizedString", "xs:normalizedString", "", `base="xs:normalizedString"`, "", UnsupportedSchemaSyntaxCode, schemaSimpleContentSpecRef(profile.version), FailureUnsupported, errSchemaSimpleContentBaseUnsupported},
			{"adjacent precisionDecimal list", "t:PDList", `<xs:simpleType name="PDList"><xs:list itemType="xs:precisionDecimal"/></xs:simpleType>`, `base="t:PDList"`, "", UnsupportedSchemaSyntaxCode, schemaSimpleContentSpecRef(profile.version), FailureUnsupported, errSchemaSimpleContentBaseUnsupported},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := tokenSimpleContentRoot(tokenSimpleContentOwner(test.base, "") + test.extra)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				want := tokenSimpleContentLoc(t, "root.xsd", root, test.primary)
				var related []Loc
				if test.related != "" {
					related = []Loc{tokenSimpleContentLoc(t, "root.xsd", root, test.related)}
				}
				if test.name == "adjacent precisionDecimal list" && profile.policy == Strict10 {
					test.code, test.spec, test.cause = diagnosticSchemaPrecisionDecimalVersionCode, "xsd11-datatypes#dt-primitive", errSchemaPrecisionDecimalVersion
					related = []Loc{
						tokenSimpleContentLoc(t, "root.xsd", root, `itemType="xs:precisionDecimal"`),
						tokenSimpleContentLoc(t, "root.xsd", root, `<xs:simpleType name="PDList"`),
					}
				}
				if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != want || !reflect.DeepEqual(diagnostic.Related(), related) || diagnostic.SpecRef() != test.spec || !errors.Is(err, test.cause) {
					t.Fatalf("base exit = %s, got %s/%s at %s related %v spec %s, want %s/%s at %s related %v spec %s cause %v", diagnostic, diagnostic.Class(), diagnostic.Code(), diagnostic.Loc(), diagnostic.Related(), diagnostic.SpecRef(), test.class, test.code, want, related, test.spec, test.cause)
				}
			})
		}
		for _, test := range []struct{ name, rootFinal, namedFinal, finalMarker string }{
			{"named final", "", ` final="extension"`, `final="extension"`},
			{"schema finalDefault", ` finalDefault="extension"`, "", `finalDefault="extension"`},
		} {
			if profile.policy == Strict10 && test.name == "named final" {
				continue
			}
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				named := strings.Replace(nmtokenSimpleContentNamed("Named"), `name="Named"`, `name="Named"`+test.namedFinal, 1)
				root := strings.Replace(tokenSimpleContentRoot(tokenSimpleContentOwner("t:Named", "")+named), `targetNamespace="urn:token"`, `targetNamespace="urn:token"`+test.rootFinal, 1)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if profile.policy == Strict10 {
					if err != nil {
						t.Fatalf("Strict10 finalDefault extension: %v", err)
					}
					return
				}
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				want := tokenSimpleContentLoc(t, "root.xsd", root, `base="t:Named"`)
				related := []Loc{tokenSimpleContentLoc(t, "root.xsd", root, test.finalMarker)}
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaSimpleTypeBaseCode || diagnostic.Loc() != want || !reflect.DeepEqual(diagnostic.Related(), related) || diagnostic.SpecRef() != schemaSimpleContentSpecRef(profile.version) || !errors.Is(err, errSchemaSimpleTypeInvalidDerivation) {
					t.Fatalf("final exit = %s", diagnostic)
				}
			})
		}
	}
}

func TestNMTOKENSimpleContentShapeExclusions(t *testing.T) {
	for _, profile := range tokenSimpleContentPolicies() {
		for _, test := range []struct {
			name, body, marker, code, spec string
			class                          FailureClass
			cause                          error
		}{
			{"anonymous built-in", `<xs:element name="root"><xs:complexType><xs:simpleContent><xs:extension base="xs:NMTOKEN"/></xs:simpleContent></xs:complexType></xs:element>`, `<xs:simpleContent>`, UnsupportedSchemaSyntaxCode, schemaSimpleContentSpecRef(profile.version), FailureUnsupported, errSchemaSimpleContentBaseUnsupported},
			{"anonymous named", nmtokenSimpleContentNamed("Named") + `<xs:element name="root"><xs:complexType><xs:simpleContent><xs:extension base="t:Named"/></xs:simpleContent></xs:complexType></xs:element>`, `<xs:simpleContent>`, UnsupportedSchemaSyntaxCode, schemaSimpleContentSpecRef(profile.version), FailureUnsupported, errSchemaSimpleContentBaseUnsupported},
			{"restriction", `<xs:complexType name="Value"><xs:simpleContent><xs:restriction base="xs:NMTOKEN"/></xs:simpleContent></xs:complexType>`, `<xs:restriction base="xs:NMTOKEN"`, UnsupportedSchemaSyntaxCode, "xsd10-structures#schema-document", FailureUnsupported, ErrUnsupported},
			{"inline base", tokenSimpleContentOwner("xs:NMTOKEN", `<xs:simpleType><xs:restriction base="xs:NMTOKEN"/></xs:simpleType>`), `<xs:simpleType>`, invalidSchemaCompositionCode, "", FailureInvalid, nil},
			{"wildcard", tokenSimpleContentOwner("xs:NMTOKEN", `<xs:anyAttribute/>`), `<xs:anyAttribute`, UnsupportedSchemaSyntaxCode, schemaAnyAttributeSpecRef(profile.version), FailureUnsupported, ErrUnsupported},
			{"assertion", tokenSimpleContentOwner("xs:NMTOKEN", `<xs:assert test="true()"/>`), `<xs:assert`, UnsupportedSchemaSyntaxCode, nmtokenSyntaxSpecRef(profile.version), FailureUnsupported, ErrUnsupported},
			{"particle", tokenSimpleContentOwner("xs:NMTOKEN", `<xs:sequence/>`), `<xs:sequence`, invalidSchemaCompositionCode, "", FailureInvalid, nil},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := tokenSimpleContentRoot(test.body)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				want := tokenSimpleContentLoc(t, "root.xsd", root, test.marker)
				if test.name == "assertion" && profile.policy == Strict10 {
					test.spec, test.cause = "xsd11-structures#cAssertions", errLanguagePolicyMismatch
				}
				if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != want || len(diagnostic.Related()) != 0 || diagnostic.SpecRef() != test.spec || test.cause != nil && !errors.Is(err, test.cause) {
					t.Fatalf("shape exit = %s, want %s/%s at %s", diagnostic, test.class, test.code, want)
				}
			})
		}
	}
}

//nolint:gocognit // Graph failures and body exclusion compete at distinct phase boundaries.
func TestNMTOKENSimpleContentGraphAndPrecedence(t *testing.T) {
	for _, profile := range tokenSimpleContentPolicies() {
		for _, test := range []struct {
			name, body, primary, code, spec string
			related                         []string
			class                           FailureClass
			cause                           error
		}{
			{"simple type cycle", tokenSimpleContentOwner("t:A", "") + `<xs:simpleType name="A"><xs:restriction base="t:B"/></xs:simpleType><xs:simpleType name="B"><xs:restriction base="t:A"/></xs:simpleType>`, `base="t:A"`, diagnosticSchemaSimpleContentBaseUnresolvedCode, schemaSimpleContentSpecRef(profile.version), []string{`<xs:complexType name="Value"`, `base="t:B"`, `base="t:A"`, `<xs:simpleType name="A"`, `<xs:simpleType name="B"`}, FailureInvalid, errSchemaSimpleTypeBaseCycle},
			{"duplicate named base", tokenSimpleContentOwner("t:Named", "") + nmtokenSimpleContentNamed("Named") + nmtokenSimpleContentNamed("Named"), `<xs:simpleType name="Named"`, diagnosticSchemaGlobalDuplicateCode, schemaGlobalDuplicateSpecRef(profile.version), []string{`<xs:simpleType name="Named"`}, FailureInvalid, errSchemaGlobalDeclarationDuplicate},
			{"missing before attribute exclusion", tokenSimpleContentOwner("t:Missing", `<xs:attribute name="flag" type="xs:boolean" use="prohibited"/>`), `base="t:Missing"`, diagnosticSchemaSimpleContentBaseUnresolvedCode, schemaSimpleContentSpecRef(profile.version), nil, FailureInvalid, errSchemaSimpleContentBaseUnresolved},
			{"invalid facet before attribute exclusion", tokenSimpleContentOwner("t:Bad", `<xs:attribute name="flag" type="xs:boolean" use="prohibited"/>`) + `<xs:simpleType name="Bad"><xs:restriction base="xs:NMTOKEN"><xs:enumeration value="bad/value"/></xs:restriction></xs:simpleType>`, `value="bad/value"`, InvalidEnumerationRestrictionCode, enumerationSpecRef(profile.version, enumerationRestrictionRule), nil, FailureInvalid, errSchemaNMTOKENValueViolation},
			{"complex NMTOKEN chain", tokenSimpleContentNamedOwner("Outer", "t:Inner", "") + tokenSimpleContentNamedOwner("Inner", "xs:NMTOKEN", ""), `base="t:Inner"`, diagnosticSchemaSimpleContentBaseWrongKindCode, schemaSimpleContentSpecRef(profile.version), []string{`<xs:complexType name="Inner"`}, FailureInvalid, errSchemaSimpleContentBaseWrongKind},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := tokenSimpleContentRoot(test.body)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				occurrence := 1
				if test.name == "duplicate named base" {
					occurrence = 2
				}
				want := allParticleTestTokenLoc(t, "root.xsd", root, test.primary, occurrence)
				var related []Loc
				for _, marker := range test.related {
					occurrence = 1
					if test.name == "simple type cycle" && marker == `base="t:A"` {
						occurrence = 2
					}
					related = append(related, allParticleTestTokenLoc(t, "root.xsd", root, marker, occurrence))
				}
				if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != want || !reflect.DeepEqual(diagnostic.Related(), related) || diagnostic.SpecRef() != test.spec || !errors.Is(err, test.cause) {
					t.Fatalf("graph exit = %s; got related %v spec %s, want %v spec %s", diagnostic, diagnostic.Related(), diagnostic.SpecRef(), related, test.spec)
				}
				if test.name == "complex NMTOKEN chain" && !errors.Is(err, errSchemaSimpleContentBaseWrongKind) {
					t.Fatalf("complex chain lost wrong-kind cause: %v", err)
				}
			})
		}
		t.Run(profile.name+"/invisible named base", func(t *testing.T) {
			root := strings.Replace(tokenSimpleContentRoot(tokenSimpleContentOwner("o:Hidden", "")), `xmlns:t="urn:token"`, `xmlns:t="urn:token" xmlns:o="urn:other"`, 1)
			other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other">` + nmtokenSimpleContentNamed("Hidden") + `</xs:schema>`
			rootDocument := elementReferenceTestSyntaxDocument(t, "root.xsd", root)
			otherDocument := elementReferenceTestSyntaxDocument(t, "other.xsd", other)
			schema, err := newSchemaFromDiscoveryWithPolicy(syntaxDiscoveryResult{documents: []*syntaxDocument{rootDocument, otherDocument}}, profile.policy)
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			want := tokenSimpleContentLoc(t, "root.xsd", root, `base="o:Hidden"`)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaSimpleContentBaseUnresolvedCode || diagnostic.Loc() != want || len(diagnostic.Related()) != 0 || diagnostic.SpecRef() != schemaSimpleContentSpecRef(profile.version) || !errors.Is(err, errSchemaSimpleContentBaseUnresolved) {
				t.Fatalf("invisible base = %s", diagnostic)
			}
		})
	}
}
