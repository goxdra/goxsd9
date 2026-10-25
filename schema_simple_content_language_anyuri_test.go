package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func languageURISimpleContentNamed(name, kind string) string {
	return `<xs:simpleType name="` + name + `"><xs:restriction base="xs:` + kind + `"/></xs:simpleType>`
}

func languageURISimpleContentRoot(body string) string {
	return strings.Replace(tokenSimpleContentRoot(body), `xmlns:t="urn:token"`, `xmlns:t="urn:token" xmlns:o="urn:other"`, 1)
}

func parseLanguageURISimpleContent(t *testing.T, root string, fixtures map[string]discoveryFixture, policy LanguagePolicy) (Schema, error) {
	t.Helper()
	source, err := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
	if err != nil {
		t.Fatalf("source: %v", err)
	}
	return ParseSchemaWithPolicy(source, &discoveryResolver{fixtures: fixtures}, policy)
}

func assertLanguageURISimpleContentDiagnostic(t *testing.T, schema Schema, err error, root, marker, code, spec string, class FailureClass, cause error, related ...string) {
	t.Helper()
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	locations := make([]Loc, 0, len(related))
	for _, value := range related {
		locations = append(locations, tokenSimpleContentLoc(t, "root.xsd", root, value))
	}
	want := tokenSimpleContentLoc(t, "root.xsd", root, marker)
	if diagnostic.Class() != class || diagnostic.Code() != code || diagnostic.Loc() != want || !slices.Equal(diagnostic.Related(), locations) || diagnostic.SpecRef() != spec || cause != nil && !errors.Is(err, cause) {
		t.Fatalf("diagnostic = %s related %v spec %q; want %s/%s at %s related %v spec %q cause %v", diagnostic, diagnostic.Related(), diagnostic.SpecRef(), class, code, want, locations, spec, cause)
	}
	if class == FailureUnsupported && !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unsupported cause lost: %v", err)
	}
}

//nolint:gocognit // The same public facts must survive every supported graph shape and policy.
func TestLanguageURISimpleContentQueries(t *testing.T) {
	for _, profile := range tokenSimpleContentPolicies() {
		for _, kind := range []string{"language", "anyURI"} {
			for _, test := range []struct {
				name, base, before, after, fixture, source, variety string
				walk                                                []string
			}{
				{"builtin", "xs:" + kind, "", "", "", "", "", []string{"Value", "root"}},
				{"forward named", "t:Named", "", languageURISimpleContentNamed("Named", kind), "", "root.xsd", `<xs:restriction base="xs:` + kind + `"`, []string{"Value", "root", "Named"}},
				{"effective chain", "t:Chain", "", `<xs:simpleType name="Chain"><xs:restriction base="t:Named"/></xs:simpleType>` + languageURISimpleContentNamed("Named", kind), "", "root.xsd", `<xs:restriction base="t:Named"`, []string{"Value", "root", "Chain", "Named"}},
				{"include", "t:Named", `<xs:include schemaLocation="part.xsd"/>`, "", tokenSimpleContentRoot(languageURISimpleContentNamed("Named", kind)), "part.xsd", `<xs:restriction base="xs:` + kind + `"`, []string{"Value", "root", "Named"}},
				{"chameleon", "t:Named", `<xs:include schemaLocation="part.xsd"/>`, "", `<xs:schema xmlns:xs="` + testXSDNamespace + `">` + languageURISimpleContentNamed("Named", kind) + `</xs:schema>`, "part.xsd", `<xs:restriction base="xs:` + kind + `"`, []string{"Value", "root", "Named"}},
				{"import", "o:Named", `<xs:import namespace="urn:other" schemaLocation="part.xsd"/>`, "", `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other">` + languageURISimpleContentNamed("Named", kind) + `</xs:schema>`, "part.xsd", `<xs:restriction base="xs:` + kind + `"`, []string{"Value", "root", "Named"}},
			} {
				t.Run(profile.name+"/"+kind+"/"+test.name, func(t *testing.T) {
					root := languageURISimpleContentRoot(test.before + tokenSimpleContentOwner(test.base, "") + `<xs:element name="root" type="t:Value"/>` + test.after)
					fixtures := map[string]discoveryFixture{}
					if test.fixture != "" {
						fixtures[test.source] = discoveryFixture{id: SourceID(test.source), contents: test.fixture}
					}
					schema, err := parseLanguageURISimpleContent(t, root, fixtures, profile.policy)
					if err != nil {
						t.Fatalf("parse: %v", err)
					}
					owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:token", "Value")
					ownerLoc := tokenSimpleContentLoc(t, "root.xsd", root, `<xs:complexType name="Value"`)
					if owner.Loc() != ownerLoc || owner.ID().Source() != "root.xsd" || owner.ID().IsZero() || owner.Component().Document() != "root.xsd" {
						t.Fatalf("owner provenance = %s/%v/%s", owner.Loc(), owner.ID(), owner.Component().Document())
					}
					if owner.Particle() != nil || len(owner.AttributeUses()) != 0 || !owner.Base().IsZero() {
						t.Fatalf("body/uses/complex base = %T/%v/%s", owner.Particle(), owner.AttributeUses(), owner.Base())
					}
					extension, ok := owner.SimpleContentExtension()
					if !ok {
						t.Fatal("simple-content body absent")
					}
					namespace := "urn:token"
					if test.name == "builtin" {
						namespace = testXSDNamespace
					}
					if test.name == "import" {
						namespace = "urn:other"
					}
					wantName := mustTestQName(t, namespace, strings.Split(test.base, ":")[1])
					baseLoc := tokenSimpleContentLoc(t, "root.xsd", root, `base="`+test.base+`"`)
					reference, ok := extension.TypeReference()
					if !ok || extension.Base() != wantName || extension.BaseLoc() != baseLoc || reference.Name() != wantName || reference.Loc() != baseLoc || reference.Variety() != SimpleTypeVarietyAtomicRestriction {
						t.Fatalf("base/reference = %s/%s/%#v", extension.Base(), extension.BaseLoc(), reference)
					}
					if values := reference.StringEnumerationFacets().Values(); len(values) != 0 {
						t.Fatalf("unexpected authored facets: %v", values)
					}
					if _, present := reference.StringWhiteSpaceFacet(); present {
						t.Fatal("atomic built-in acquired a string-facet whiteSpace fact")
					}
					id, hasID := extension.TypeID()
					refID, hasRefID := reference.ComponentID()
					if test.name == "builtin" {
						if !reference.IsBuiltin() || hasID || hasRefID || !id.IsZero() || !refID.IsZero() || reference.VarietyLoc() != baseLoc {
							t.Fatalf("builtin identity/provenance = %#v/%#v", extension, reference)
						}
					}
					if test.name != "builtin" {
						definition := root
						if test.fixture != "" {
							definition = test.fixture
						}
						wantVariety := tokenSimpleContentLoc(t, SourceID(test.source), definition, test.variety)
						if !reference.IsNamed() || !hasID || !hasRefID || id != refID || id != componentIDForName(t, schema, wantName) || reference.VarietyLoc() != wantVariety {
							t.Fatalf("named identity/provenance = %v/%v/%s; want variety %s", id, refID, reference.VarietyLoc(), wantVariety)
						}
						component, found := schema.Lookup(id)
						baseDefinition, simple := component.SimpleTypeDefinition()
						if !found || !simple || baseDefinition.Variety() != SimpleTypeVarietyAtomicRestriction || baseDefinition.BaseLoc().IsZero() || component.Document() != SourceID(test.source) {
							t.Fatal("named base definition missing")
						}
					}
					components := schema.Components()
					components[0] = Component{}
					var walk []string
					if err := schema.Walk(func(component Component) error { walk = append(walk, component.Name().Local()); return nil }); err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(walk, test.walk) {
						t.Fatalf("walk = %v, want %v", walk, test.walk)
					}
				})
			}
		}
	}
}

//nolint:gocognit // The neighboring base controls use the same policy/shape matrix.
func TestLanguageURISimpleContentNeighborControls(t *testing.T) {
	for _, profile := range tokenSimpleContentPolicies() {
		for _, kind := range []string{"token", "normalizedString"} {
			for _, base := range []string{"xs:" + kind, "t:Named"} {
				t.Run(profile.name+"/"+kind+"/"+base, func(t *testing.T) {
					root := languageURISimpleContentRoot(tokenSimpleContentOwner(base, "") + `<xs:simpleType name="Named"><xs:restriction base="xs:` + kind + `"/></xs:simpleType>`)
					schema, err := parseLanguageURISimpleContent(t, root, nil, profile.policy)
					if err != nil {
						t.Fatalf("control parse: %v", err)
					}
					owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:token", "Value")
					if _, ok := owner.SimpleContentExtension(); !ok {
						t.Fatal("control simpleContent missing")
					}
				})
			}
		}
	}
}

//nolint:gocognit // Every authored use shape is an independent excluded boundary.
func TestLanguageURISimpleContentUseExclusions(t *testing.T) {
	for _, profile := range tokenSimpleContentPolicies() {
		for _, kind := range []string{"language", "anyURI"} {
			for _, base := range []string{"xs:" + kind, "t:Named"} {
				for _, test := range []struct{ name, use, extra, marker string }{
					{"direct", `<xs:attribute name="flag" type="xs:boolean"/>`, "", `<xs:attribute name="flag"`},
					{"named", `<xs:attribute name="flag" type="t:Flag"/>`, `<xs:simpleType name="Flag"><xs:restriction base="xs:boolean"/></xs:simpleType>`, `<xs:attribute name="flag"`},
					{"inline", `<xs:attribute name="flag"><xs:simpleType><xs:restriction base="xs:boolean"/></xs:simpleType></xs:attribute>`, "", `<xs:attribute name="flag"`},
					{"ref", `<xs:attribute ref="t:flag"/>`, `<xs:attribute name="flag" type="xs:boolean"/>`, `<xs:attribute ref="t:flag"`},
					{"prohibited", `<xs:attribute name="flag" type="xs:boolean" use="prohibited"/>`, "", `<xs:attribute name="flag"`},
					{"direct default", `<xs:attribute name="flag" type="xs:boolean" default="true"/>`, "", `default="true"`},
					{"named fixed", `<xs:attribute name="flag" type="t:Flag" fixed="true"/>`, `<xs:simpleType name="Flag"><xs:restriction base="xs:boolean"/></xs:simpleType>`, `fixed="true"`},
					{"inline default", `<xs:attribute name="flag" default="true"><xs:simpleType><xs:restriction base="xs:boolean"/></xs:simpleType></xs:attribute>`, "", `default="true"`},
					{"ref fixed", `<xs:attribute ref="t:flag" fixed="true"/>`, `<xs:attribute name="flag" type="xs:boolean"/>`, `fixed="true"`},
				} {
					t.Run(profile.name+"/"+kind+"/"+base+"/"+test.name, func(t *testing.T) {
						root := languageURISimpleContentRoot(tokenSimpleContentOwner(base, test.use) + languageURISimpleContentNamed("Named", kind) + test.extra)
						schema, err := parseLanguageURISimpleContent(t, root, nil, profile.policy)
						spec := schemaSimpleContentSpecRef(profile.version)
						cause := errSchemaSimpleContentBaseUnsupported
						if strings.Contains(test.name, "default") || strings.Contains(test.name, "fixed") {
							spec = schemaAttributeUseSpecRef(profile.version)
							cause = errSchemaAttributeUseUnsupported
						}
						assertLanguageURISimpleContentDiagnostic(t, schema, err, root, test.marker, UnsupportedSchemaSyntaxCode, spec, FailureUnsupported, cause)
					})
				}
			}
		}
	}
}

//nolint:gocognit // Validation and generation each have a distinct public refusal.
func TestLanguageURISimpleContentConsumers(t *testing.T) {
	for _, profile := range tokenSimpleContentPolicies() {
		for _, kind := range []string{"language", "anyURI"} {
			for _, base := range []string{"xs:" + kind, "t:Named"} {
				t.Run(profile.name+"/"+kind+"/"+base, func(t *testing.T) {
					root := languageURISimpleContentRoot(tokenSimpleContentOwner(base, "") + languageURISimpleContentNamed("Named", kind) + `<xs:element name="root" type="t:Value"/>`)
					schema, err := parseLanguageURISimpleContent(t, root, nil, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					ownerLoc := tokenSimpleContentLoc(t, "root.xsd", root, `<xs:complexType name="Value"`)
					elementLoc := tokenSimpleContentLoc(t, "root.xsd", root, `<xs:element name="root"`)
					instanceErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:token">text</root>`)))
					instance := requireDiagnostic(t, instanceErr)
					if instance.Class() != FailureUnsupported || instance.Code() != UnsupportedInstanceValidationCode || instance.Loc() != mustTestLoc(t, "instance.xml", 1, 1) || !reflect.DeepEqual(instance.Related(), []Loc{elementLoc, ownerLoc}) || instance.SpecRef() != instanceValidationSpecRef(profile.version) || !errors.Is(instanceErr, ErrUnsupported) {
						t.Fatalf("validator = %s", instance)
					}
					output, generationErr := GenerateGo(schema, "generated")
					generation := requireDiagnostic(t, generationErr)
					if output != nil || generation.Class() != FailureUnsupported || generation.Code() != diagnosticCodegenUnsupported || generation.Loc() != ownerLoc || len(generation.Related()) != 0 || generation.SpecRef() != codegenDirectChoiceSpecReference(profile.version, codegenDirectChoiceParticlesReference) || !errors.Is(generationErr, ErrUnsupported) {
						t.Fatalf("generator = %v/%s", output, generation)
					}
				})
			}
		}
	}
}

//nolint:gocognit // The listed exits classify independently invalid and unsupported sibling shapes.
func TestLanguageURISimpleContentShapeExclusions(t *testing.T) {
	for _, profile := range tokenSimpleContentPolicies() {
		for _, kind := range []string{"language", "anyURI"} {
			for _, test := range []struct {
				name, body, marker, code, spec string
				class                          FailureClass
				cause                          error
			}{
				{"anonymous built-in", `<xs:element name="root"><xs:complexType><xs:simpleContent><xs:extension base="xs:` + kind + `"/></xs:simpleContent></xs:complexType></xs:element>`, `<xs:simpleContent>`, UnsupportedSchemaSyntaxCode, schemaSimpleContentSpecRef(profile.version), FailureUnsupported, errSchemaSimpleContentBaseUnsupported},
				{"anonymous named", languageURISimpleContentNamed("Named", kind) + `<xs:element name="root"><xs:complexType><xs:simpleContent><xs:extension base="t:Named"/></xs:simpleContent></xs:complexType></xs:element>`, `<xs:simpleContent>`, UnsupportedSchemaSyntaxCode, schemaSimpleContentSpecRef(profile.version), FailureUnsupported, errSchemaSimpleContentBaseUnsupported},
				{"restriction built-in", `<xs:complexType name="Value"><xs:simpleContent><xs:restriction base="xs:` + kind + `"/></xs:simpleContent></xs:complexType>`, `<xs:restriction`, UnsupportedSchemaSyntaxCode, "xsd10-structures#schema-document", FailureUnsupported, ErrUnsupported},
				{"restriction named", languageURISimpleContentNamed("Named", kind) + `<xs:complexType name="Value"><xs:simpleContent><xs:restriction base="t:Named"/></xs:simpleContent></xs:complexType>`, `<xs:restriction base="t:Named"`, UnsupportedSchemaSyntaxCode, "xsd10-structures#schema-document", FailureUnsupported, ErrUnsupported},
				{"inline base", tokenSimpleContentOwner("xs:"+kind, `<xs:simpleType><xs:restriction base="xs:`+kind+`"/></xs:simpleType>`), `<xs:simpleType>`, invalidSchemaCompositionCode, "", FailureInvalid, nil},
				{"list base", tokenSimpleContentOwner("t:List", "") + `<xs:simpleType name="List"><xs:list itemType="xs:` + kind + `"/></xs:simpleType>`, `base="t:List"`, UnsupportedSchemaSyntaxCode, schemaSimpleContentSpecRef(profile.version), FailureUnsupported, errSchemaSimpleContentBaseUnsupported},
				{"union base", tokenSimpleContentOwner("t:Union", "") + `<xs:simpleType name="Union"><xs:union memberTypes="xs:` + kind + ` xs:token"/></xs:simpleType>`, `base="t:Union"`, UnsupportedSchemaSyntaxCode, schemaSimpleContentSpecRef(profile.version), FailureUnsupported, errSchemaSimpleContentBaseUnsupported},
				{"authored facet", tokenSimpleContentOwner("t:Faceted", "") + `<xs:simpleType name="Faceted"><xs:restriction base="xs:` + kind + `"><xs:enumeration value="value"/></xs:restriction></xs:simpleType>`, `<xs:enumeration`, UnsupportedDatatypeFacetCode, tokenDiagnosticSpecRef(profile.version, "decimal"), FailureUnsupported, ErrUnsupported},
				{"complex base", tokenSimpleContentNamedOwner("Outer", "t:Inner", "") + tokenSimpleContentNamedOwner("Inner", "xs:"+kind, ""), `base="t:Inner"`, diagnosticSchemaSimpleContentBaseWrongKindCode, schemaSimpleContentSpecRef(profile.version), FailureInvalid, errSchemaSimpleContentBaseWrongKind},
				{"wildcard", tokenSimpleContentOwner("xs:"+kind, `<xs:anyAttribute/>`), `<xs:anyAttribute`, UnsupportedSchemaSyntaxCode, schemaAnyAttributeSpecRef(profile.version), FailureUnsupported, ErrUnsupported},
				{"particle", tokenSimpleContentOwner("xs:"+kind, `<xs:sequence/>`), `<xs:sequence`, invalidSchemaCompositionCode, "", FailureInvalid, nil},
			} {
				t.Run(profile.name+"/"+kind+"/"+test.name, func(t *testing.T) {
					root := languageURISimpleContentRoot(test.body)
					schema, err := parseLanguageURISimpleContent(t, root, nil, profile.policy)
					if test.name == "complex base" {
						assertLanguageURISimpleContentDiagnostic(t, schema, err, root, test.marker, test.code, test.spec, test.class, test.cause, `<xs:complexType name="Inner"`)
						return
					}
					if test.spec == "" {
						assertZeroSchema(t, schema)
						d := requireDiagnostic(t, err)
						if d.Class() != test.class || d.Code() != test.code || d.Loc() != tokenSimpleContentLoc(t, "root.xsd", root, test.marker) || len(d.Related()) != 0 || test.cause != nil && !errors.Is(err, test.cause) {
							t.Fatalf("shape = %s", d)
						}
						return
					}
					assertLanguageURISimpleContentDiagnostic(t, schema, err, root, test.marker, test.code, test.spec, test.class, test.cause)
				})
			}
		}
	}
}

//nolint:gocognit // Resolution, cycle, and final errors must retain their own locations and causes.
func TestLanguageURISimpleContentBoundaryExits(t *testing.T) {
	for _, profile := range tokenSimpleContentPolicies() {
		for _, kind := range []string{"language", "anyURI"} {
			for _, test := range []struct {
				name, body, primary, related, code, spec string
				class                                    FailureClass
				cause                                    error
			}{
				{"malformed base QName", tokenSimpleContentOwner("t:Bad:Name", ""), `base="t:Bad:Name"`, "", invalidSchemaConditionalCode, "", FailureInvalid, nil},
				{"missing", tokenSimpleContentOwner("t:Missing", `<xs:attribute name="flag" type="xs:boolean"/>`), `base="t:Missing"`, "", diagnosticSchemaSimpleContentBaseUnresolvedCode, schemaSimpleContentSpecRef(profile.version), FailureInvalid, errSchemaSimpleContentBaseUnresolved},
				{"wrong kind", tokenSimpleContentOwner("t:Wrong", "") + `<xs:complexType name="Wrong"/>`, `base="t:Wrong"`, `<xs:complexType name="Wrong"`, diagnosticSchemaSimpleContentBaseWrongKindCode, schemaSimpleContentSpecRef(profile.version), FailureInvalid, errSchemaSimpleContentBaseWrongKind},
				{"final after base", tokenSimpleContentOwner("t:Named", `<xs:attribute name="flag" type="xs:boolean"/>`) + strings.Replace(languageURISimpleContentNamed("Named", kind), `name="Named"`, `name="Named" final="#all"`, 1), `base="t:Named"`, `final="#all"`, diagnosticSchemaSimpleTypeBaseCode, schemaSimpleContentSpecRef(profile.version), FailureInvalid, errSchemaSimpleTypeInvalidDerivation},
				{"simple type cycle", tokenSimpleContentOwner("t:A", "") + `<xs:simpleType name="A"><xs:restriction base="t:B"/></xs:simpleType><xs:simpleType name="B"><xs:restriction base="t:A"/></xs:simpleType>`, `base="t:A"`, `<xs:complexType name="Value"`, diagnosticSchemaSimpleContentBaseUnresolvedCode, schemaSimpleContentSpecRef(profile.version), FailureInvalid, errSchemaSimpleTypeBaseCycle},
				{"complex cycle", tokenSimpleContentNamedOwner("Outer", "t:Inner", "") + tokenSimpleContentNamedOwner("Inner", "t:Outer", ""), `base="t:Outer"`, `base="t:Inner"`, diagnosticSchemaSimpleContentBaseUnresolvedCode, schemaSimpleContentSpecRef(profile.version), FailureInvalid, errSchemaComplexTypeBaseCycle},
				{"complex base final", tokenSimpleContentNamedOwner("Outer", "t:Inner", "") + strings.Replace(tokenSimpleContentNamedOwner("Inner", "xs:"+kind, ""), `name="Inner"`, `name="Inner" final="extension"`, 1), `base="t:Inner"`, `final="extension"`, invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(profile.version), FailureInvalid, errSchemaComplexTypeBaseUnsupported},
			} {
				if test.name == "final after base" && profile.version == XSDVersion10 {
					continue
				}
				t.Run(profile.name+"/"+kind+"/"+test.name, func(t *testing.T) {
					root := languageURISimpleContentRoot(test.body)
					schema, err := parseLanguageURISimpleContent(t, root, nil, profile.policy)
					if test.name == "simple type cycle" {
						assertZeroSchema(t, schema)
						d := requireDiagnostic(t, err)
						wantRelated := []Loc{
							tokenSimpleContentLoc(t, "root.xsd", root, `<xs:complexType name="Value"`),
							tokenSimpleContentLoc(t, "root.xsd", root, `base="t:B"`),
							allParticleTestTokenLoc(t, "root.xsd", root, `base="t:A"`, 2),
							tokenSimpleContentLoc(t, "root.xsd", root, `<xs:simpleType name="A"`),
							tokenSimpleContentLoc(t, "root.xsd", root, `<xs:simpleType name="B"`),
						}
						if d.Class() != test.class || d.Code() != test.code || d.Loc() != tokenSimpleContentLoc(t, "root.xsd", root, test.primary) || !reflect.DeepEqual(d.Related(), wantRelated) || d.SpecRef() != test.spec || !errors.Is(err, test.cause) || !errors.Is(err, errSchemaSimpleContentBaseUnresolved) {
							t.Fatalf("cycle = %s", d)
						}
						return
					}
					if test.related == "" {
						assertLanguageURISimpleContentDiagnostic(t, schema, err, root, test.primary, test.code, test.spec, test.class, test.cause)
						return
					}
					assertLanguageURISimpleContentDiagnostic(t, schema, err, root, test.primary, test.code, test.spec, test.class, test.cause, test.related)
				})
			}
		}
	}
}

//nolint:gocognit // XSD 1.0 and 1.1 map simple-type extension final differently.
func TestLanguageURISimpleContentNamedFinalPolicies(t *testing.T) {
	for _, profile := range tokenSimpleContentPolicies() {
		for _, kind := range []string{"language", "anyURI"} {
			for _, test := range []struct {
				name, schemaFinal, namedFinal, finalMarker string
				block                                      bool
			}{
				{"local #all", "", ` final="#all"`, `final="#all"`, profile.version == XSDVersion11},
				{"root extension", ` finalDefault="extension"`, "", `finalDefault="extension"`, profile.version == XSDVersion11},
				{"root #all", ` finalDefault="#all"`, "", `finalDefault="#all"`, profile.version == XSDVersion11},
				{"local empty override", ` finalDefault="#all"`, ` final=""`, "", false},
				{"unrelated final", ` finalDefault="restriction"`, "", "", false},
			} {
				t.Run(profile.name+"/"+kind+"/"+test.name, func(t *testing.T) {
					named := strings.Replace(languageURISimpleContentNamed("Named", kind), `name="Named"`, `name="Named"`+test.namedFinal, 1)
					root := strings.Replace(languageURISimpleContentRoot(tokenSimpleContentOwner("t:Named", "")+named), `targetNamespace="urn:token"`, `targetNamespace="urn:token"`+test.schemaFinal, 1)
					schema, err := parseLanguageURISimpleContent(t, root, nil, profile.policy)
					if !test.block {
						if err != nil {
							t.Fatalf("allowed final: %v", err)
						}
						owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:token", "Value")
						if _, present := owner.SimpleContentExtension(); !present {
							t.Fatal("simple-content extension absent")
						}
						return
					}
					assertLanguageURISimpleContentDiagnostic(t, schema, err, root, `base="t:Named"`, diagnosticSchemaSimpleTypeBaseCode, schemaSimpleContentSpecRef(profile.version), FailureInvalid, errSchemaSimpleTypeInvalidDerivation, test.finalMarker)
				})
			}
			if profile.version == XSDVersion10 {
				t.Run(profile.name+"/"+kind+"/local extension lexical", func(t *testing.T) {
					named := strings.Replace(languageURISimpleContentNamed("Named", kind), `name="Named"`, `name="Named" final="extension"`, 1)
					root := languageURISimpleContentRoot(tokenSimpleContentOwner("t:Named", "") + named)
					schema, err := parseLanguageURISimpleContent(t, root, nil, profile.policy)
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.Loc() != tokenSimpleContentLoc(t, "root.xsd", root, `final="extension"`) || len(d.Related()) != 0 || d.SpecRef() != schemaSimpleTypeXSD10SpecRef || !errors.Is(err, errLanguagePolicyMismatch) {
						t.Fatalf("Strict10 final lexical = %s", d)
					}
				})
				continue
			}
			t.Run(profile.name+"/"+kind+"/local extension", func(t *testing.T) {
				named := strings.Replace(languageURISimpleContentNamed("Named", kind), `name="Named"`, `name="Named" final="extension"`, 1)
				root := languageURISimpleContentRoot(tokenSimpleContentOwner("t:Named", "") + named)
				schema, err := parseLanguageURISimpleContent(t, root, nil, profile.policy)
				assertLanguageURISimpleContentDiagnostic(t, schema, err, root, `base="t:Named"`, diagnosticSchemaSimpleTypeBaseCode, schemaSimpleContentSpecRef(profile.version), FailureInvalid, errSchemaSimpleTypeInvalidDerivation, `final="extension"`)
			})
		}
	}
}

func TestLanguageURISimpleContentGraphFailures(t *testing.T) {
	for _, profile := range tokenSimpleContentPolicies() {
		for _, kind := range []string{"language", "anyURI"} {
			t.Run(profile.name+"/"+kind+"/invisible", func(t *testing.T) {
				root := languageURISimpleContentRoot(tokenSimpleContentOwner("o:Hidden", ""))
				other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other">` + languageURISimpleContentNamed("Hidden", kind) + `</xs:schema>`
				rootDocument := elementReferenceTestSyntaxDocument(t, "root.xsd", root)
				otherDocument := elementReferenceTestSyntaxDocument(t, "other.xsd", other)
				schema, err := newSchemaFromDiscoveryWithPolicy(syntaxDiscoveryResult{documents: []*syntaxDocument{rootDocument, otherDocument}}, profile.policy)
				assertLanguageURISimpleContentDiagnostic(t, schema, err, root, `base="o:Hidden"`, diagnosticSchemaSimpleContentBaseUnresolvedCode, schemaSimpleContentSpecRef(profile.version), FailureInvalid, errSchemaSimpleContentBaseUnresolved)
			})
			t.Run(profile.name+"/"+kind+"/duplicate", func(t *testing.T) {
				root := languageURISimpleContentRoot(tokenSimpleContentOwner("t:Named", "") + languageURISimpleContentNamed("Named", kind) + languageURISimpleContentNamed("Named", kind))
				schema, err := parseLanguageURISimpleContent(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				primary := allParticleTestTokenLoc(t, "root.xsd", root, `<xs:simpleType name="Named"`, 2)
				related := []Loc{tokenSimpleContentLoc(t, "root.xsd", root, `<xs:simpleType name="Named"`)}
				if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaGlobalDuplicateCode || d.Loc() != primary || !reflect.DeepEqual(d.Related(), related) || d.SpecRef() != schemaGlobalDuplicateSpecRef(profile.version) || !errors.Is(err, errSchemaGlobalDeclarationDuplicate) {
					t.Fatalf("duplicate = %s", d)
				}
			})
		}
	}
}
