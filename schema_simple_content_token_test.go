package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func tokenSimpleContentPolicies() []struct {
	name    string
	policy  LanguagePolicy
	version XSDVersion
} {
	return []struct {
		name    string
		policy  LanguagePolicy
		version XSDVersion
	}{
		{"compatibility", Compatibility, XSDVersion11},
		{"strict10", Strict10, XSDVersion10},
		{"strict11", Strict11, XSDVersion11},
	}
}

func tokenSimpleContentRoot(body string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:token" targetNamespace="urn:token">` + body + `</xs:schema>`
}

func tokenSimpleContentOwner(base, content string) string {
	return tokenSimpleContentNamedOwner("Value", base, content)
}

func tokenSimpleContentNamedOwner(name, base, content string) string {
	return `<xs:complexType name="` + name + `"><xs:simpleContent><xs:extension base="` + base + `">` + content + `</xs:extension></xs:simpleContent></xs:complexType>`
}

func tokenSimpleContentLoc(t *testing.T, source SourceID, document, marker string) Loc {
	t.Helper()
	return allParticleTestTokenLoc(t, source, document, marker, 1)
}

//nolint:gocognit,funlen // One matrix checks the same completed model across policies and graph shapes.
func TestTokenSimpleContentQueriesAcrossPoliciesAndGraphs(t *testing.T) {
	for _, profile := range tokenSimpleContentPolicies() {
		for _, test := range []struct {
			name, base, extra, source, declaration, variety, value, valueMarker, whiteSpaceMarker string
		}{
			{"builtin", "xs:token", "", "", "", "", "", "", ""},
			{"forward", "t:Forward", `<xs:simpleType name="Forward"><xs:restriction base="t:Direct"/></xs:simpleType><xs:simpleType name="Direct"><xs:restriction base="xs:token"><xs:enumeration value=" first "/><xs:enumeration value="second"/></xs:restriction></xs:simpleType>`, "root.xsd", "", `<xs:restriction base="t:Direct"`, " first ", `value=" first "`, ""},
			{"explicit whitespace", "t:Named", `<xs:simpleType name="Named"><xs:restriction base="xs:token"><xs:whiteSpace value="collapse" fixed="true"/><xs:enumeration value="named"/></xs:restriction></xs:simpleType>`, "root.xsd", "", `<xs:restriction base="xs:token"`, "named", `value="named"`, `value="collapse"`},
			{"include", "t:Included", `<xs:include schemaLocation="included.xsd"/>`, "included.xsd", `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:token"><xs:simpleType name="Included"><xs:restriction base="xs:token"><xs:enumeration value="included"/></xs:restriction></xs:simpleType></xs:schema>`, `<xs:restriction base="xs:token"`, "included", `value="included"`, ""},
			{"chameleon", "t:Chameleon", `<xs:include schemaLocation="chameleon.xsd"/>`, "chameleon.xsd", `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Chameleon"><xs:restriction base="xs:token"><xs:enumeration value="chameleon"/></xs:restriction></xs:simpleType></xs:schema>`, `<xs:restriction base="xs:token"`, "chameleon", `value="chameleon"`, ""},
			{"import", "o:Imported", `<xs:import namespace="urn:other" schemaLocation="other.xsd"/>`, "other.xsd", `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Imported"><xs:restriction base="xs:token"><xs:enumeration value="imported"/></xs:restriction></xs:simpleType></xs:schema>`, `<xs:restriction base="xs:token"`, "imported", `value="imported"`, ""},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := tokenSimpleContentRoot(test.extra + tokenSimpleContentOwner(test.base, "") + `<xs:element name="root" type="t:Value"/>`)
				root = strings.Replace(root, `xmlns:t="urn:token"`, `xmlns:t="urn:token" xmlns:o="urn:other"`, 1)
				if test.name == "forward" {
					root = tokenSimpleContentRoot(tokenSimpleContentOwner(test.base, "") + test.extra + `<xs:element name="root" type="t:Value"/>`)
				}
				fixtures := map[string]discoveryFixture{}
				if test.source != "" && test.source != "root.xsd" {
					fixtures[test.source] = discoveryFixture{id: SourceID(test.source), contents: test.declaration}
				}
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				if err != nil {
					t.Fatalf("parse: %v", err)
				}
				nameSpace := "urn:token"
				if test.name == "import" {
					nameSpace = "urn:other"
				}
				owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:token", "Value")
				_, hasComplexBase := owner.BaseReference()
				if owner.Particle() != nil || len(owner.AttributeUses()) != 0 || owner.Base() != (QName{}) || hasComplexBase {
					// The complex-type derivation API represents complex bases; scalar facts use SimpleContentExtension.
					t.Fatalf("owner particle/uses/complex base = %T/%d/%s", owner.Particle(), len(owner.AttributeUses()), owner.Base())
				}
				extension, ok := owner.SimpleContentExtension()
				if !ok {
					t.Fatal("missing simpleContent extension")
				}
				wantBase := mustTestQName(t, nameSpace, strings.Split(test.base, ":")[1])
				if test.name == "builtin" {
					wantBase = mustTestQName(t, testXSDNamespace, "token")
				}
				baseLoc := tokenSimpleContentLoc(t, "root.xsd", root, `base="`+test.base+`"`)
				if extension.Base() != wantBase || extension.BaseLoc() != baseLoc {
					t.Fatalf("written base = %s/%s, want %s/%s", extension.Base(), extension.BaseLoc(), wantBase, baseLoc)
				}
				reference, ok := extension.TypeReference()
				if !ok || reference.Name() != wantBase || reference.Loc() != baseLoc || reference.Variety() != SimpleTypeVarietyAtomicRestriction {
					t.Fatalf("resolved reference = %#v/%t", reference, ok)
				}
				space, hasSpace := reference.StringWhiteSpaceFacet()
				if !hasSpace || space.Value() != "collapse" || !space.Fixed() {
					t.Fatalf("whiteSpace = %#v/%t", space, hasSpace)
				}
				wantWhiteSpaceLoc := Loc{}
				if test.whiteSpaceMarker != "" {
					wantWhiteSpaceLoc = tokenSimpleContentLoc(t, SourceID(test.source), root, test.whiteSpaceMarker)
				}
				if space.Loc() != wantWhiteSpaceLoc {
					t.Fatalf("whiteSpace location = %s, want %s", space.Loc(), wantWhiteSpaceLoc)
				}
				id, hasID := extension.TypeID()
				refID, hasRefID := reference.ComponentID()
				if test.name == "builtin" {
					if !reference.IsBuiltin() || hasID || hasRefID || !id.IsZero() || !refID.IsZero() || reference.VarietyLoc() != baseLoc {
						t.Fatalf("built-in identity/provenance = %v/%t/%v/%t/%s", id, hasID, refID, hasRefID, reference.VarietyLoc())
					}
				}
				if test.name != "builtin" {
					definitionSource := root
					if test.source != "root.xsd" {
						definitionSource = test.declaration
					}
					if !reference.IsNamed() || !hasID || !hasRefID || id != refID || id != componentIDForName(t, schema, wantBase) || reference.VarietyLoc() != tokenSimpleContentLoc(t, SourceID(test.source), definitionSource, test.variety) {
						t.Fatalf("named identity/provenance = %v/%t/%v/%t/%s", id, hasID, refID, hasRefID, reference.VarietyLoc())
					}
					component, found := schema.Lookup(id)
					definition, simple := component.SimpleTypeDefinition()
					if !found || !simple || definition.Base().IsZero() || definition.BaseLoc().IsZero() {
						t.Fatal("named base derivation facts missing")
					}
					values := reference.StringEnumerationFacets().Values()
					if len(values) == 0 || values[0] != test.value {
						t.Fatalf("effective enumeration = %v, want %q", values, test.value)
					}
					locations := reference.StringEnumerationFacets().Locations()
					if locations[0] != tokenSimpleContentLoc(t, SourceID(test.source), definitionSource, test.valueMarker) {
						t.Fatalf("enumeration location = %v", locations)
					}
					values[0] = "changed"
					locations[0] = Loc{}
					if reference.StringEnumerationFacets().Values()[0] != test.value || reference.StringEnumerationFacets().Locations()[0].IsZero() {
						t.Fatal("mutating copied facets changed completed schema")
					}
				}
				components := schema.Components()
				components[0] = Component{}
				walked := make([]QName, 0)
				if err := schema.Walk(func(component Component) error { walked = append(walked, component.Name()); return nil }); err != nil {
					t.Fatalf("walk: %v", err)
				}
				wantWalk := []string{"Value", "root"}
				if test.name == "forward" {
					wantWalk = []string{"Value", "Forward", "Direct", "root"}
				}
				if test.name == "explicit whitespace" {
					wantWalk = []string{"Named", "Value", "root"}
				}
				if test.source != "" && test.source != "root.xsd" {
					wantWalk = append(wantWalk, strings.Split(test.base, ":")[1])
				}
				gotWalk := make([]string, 0, len(walked))
				for _, name := range walked {
					gotWalk = append(gotWalk, name.Local())
				}
				if !reflect.DeepEqual(gotWalk, wantWalk) {
					t.Fatalf("walk order = %v, want %v", gotWalk, wantWalk)
				}
			})
		}
	}
}

//nolint:gocognit // Assert both consumer diagnostics at the same admitted query boundary.
func TestTokenSimpleContentConsumersRejectAtUse(t *testing.T) {
	for _, profile := range tokenSimpleContentPolicies() {
		for _, base := range []string{"xs:token", "t:Named"} {
			t.Run(profile.name+"/"+base, func(t *testing.T) {
				root := tokenSimpleContentRoot(tokenSimpleContentOwner(base, "") + `<xs:simpleType name="Named"><xs:restriction base="xs:token"/></xs:simpleType><xs:element name="root" type="t:Value"/>`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("parse: %v", err)
				}
				instanceErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:token">value</root>`)))
				if instanceErr == nil {
					t.Fatal("validator accepted token simpleContent")
				}
				diagnostic := requireDiagnostic(t, instanceErr)
				ownerLoc := tokenSimpleContentLoc(t, "root.xsd", root, `<xs:complexType name="Value"`)
				elementLoc := tokenSimpleContentLoc(t, "root.xsd", root, `<xs:element name="root"`)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedInstanceValidationCode || diagnostic.Loc() != mustTestLoc(t, "instance.xml", 1, 1) || !reflect.DeepEqual(diagnostic.Related(), []Loc{elementLoc, ownerLoc}) || diagnostic.SpecRef() != instanceValidationSpecRef(profile.version) || !errors.Is(instanceErr, ErrUnsupported) || !errors.Is(instanceErr, errInstanceChoiceParticle) {
					t.Fatalf("validation diagnostic = %s", diagnostic)
				}
				generated, generationErr := GenerateGo(schema, "generated")
				if generationErr == nil || generated != nil {
					t.Fatalf("generator result = %s/%v", generated, generationErr)
				}
				generation := requireDiagnostic(t, generationErr)
				if generation.Class() != FailureUnsupported || generation.Code() != diagnosticCodegenUnsupported || generation.Loc() != ownerLoc || len(generation.Related()) != 0 || generation.SpecRef() != codegenDirectChoiceSpecReference(profile.version, codegenDirectChoiceParticlesReference) || !errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenUnsupported) {
					t.Fatalf("generation diagnostic = %s", generation)
				}
			})
		}
	}
}

func TestTokenSimpleContentAttributeExclusions(t *testing.T) {
	for _, profile := range tokenSimpleContentPolicies() {
		for _, test := range []struct {
			name, attribute, extra, marker string
		}{
			{"direct", `<xs:attribute name="flag" type="xs:boolean"/>`, "", `<xs:attribute name="flag"`},
			{"named", `<xs:attribute name="flag" type="t:Flag"/>`, `<xs:simpleType name="Flag"><xs:restriction base="xs:boolean"/></xs:simpleType>`, `<xs:attribute name="flag"`},
			{"inline", `<xs:attribute name="flag"><xs:simpleType><xs:restriction base="xs:boolean"/></xs:simpleType></xs:attribute>`, "", `<xs:attribute name="flag"`},
			{"ref", `<xs:attribute ref="t:flag"/>`, `<xs:attribute name="flag" type="xs:boolean"/>`, `<xs:attribute ref="t:flag"`},
			{"prohibited", `<xs:attribute name="flag" type="xs:boolean" use="prohibited"/>`, "", `<xs:attribute name="flag"`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := tokenSimpleContentRoot(tokenSimpleContentOwner("xs:token", test.attribute) + test.extra)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				if err == nil {
					t.Fatal("token simpleContent admitted an attribute")
				}
				diagnostic := requireDiagnostic(t, err)
				want := tokenSimpleContentLoc(t, "root.xsd", root, test.marker)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != want || diagnostic.SpecRef() != schemaSimpleContentSpecRef(profile.version) || len(diagnostic.Related()) != 0 || !errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaSimpleContentBaseUnsupported) {
					t.Fatalf("attribute exclusion = %s, want located unsupported at %s", diagnostic, want)
				}
			})
		}
	}
}

//nolint:gocognit // Owner, derivation, wildcard, and assertion exits must remain distinct.
func TestTokenSimpleContentOtherExclusions(t *testing.T) {
	for _, profile := range tokenSimpleContentPolicies() {
		for _, test := range []struct {
			name, body, marker, code, spec string
			class                          FailureClass
			cause                          error
		}{
			{"anonymous builtin", `<xs:element name="root"><xs:complexType><xs:simpleContent><xs:extension base="xs:token"/></xs:simpleContent></xs:complexType></xs:element>`, `<xs:simpleContent>`, UnsupportedSchemaSyntaxCode, schemaSimpleContentSpecRef(profile.version), FailureUnsupported, errSchemaSimpleContentBaseUnsupported},
			{"anonymous named", `<xs:simpleType name="Named"><xs:restriction base="xs:token"/></xs:simpleType><xs:element name="root"><xs:complexType><xs:simpleContent><xs:extension base="t:Named"/></xs:simpleContent></xs:complexType></xs:element>`, `<xs:simpleContent>`, UnsupportedSchemaSyntaxCode, schemaSimpleContentSpecRef(profile.version), FailureUnsupported, errSchemaSimpleContentBaseUnsupported},
			{"restriction", `<xs:complexType name="Value"><xs:simpleContent><xs:restriction base="xs:token"/></xs:simpleContent></xs:complexType>`, `<xs:restriction base="xs:token"`, UnsupportedSchemaSyntaxCode, "", FailureUnsupported, ErrUnsupported},
			{"anyAttribute", tokenSimpleContentOwner("xs:token", `<xs:anyAttribute/>`), `<xs:anyAttribute`, UnsupportedSchemaSyntaxCode, "", FailureUnsupported, ErrUnsupported},
			{"assertion", tokenSimpleContentOwner("xs:token", `<xs:assert test="true()"/>`), `<xs:assert`, UnsupportedSchemaSyntaxCode, "", FailureUnsupported, ErrUnsupported},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := tokenSimpleContentRoot(test.body)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				if err == nil {
					t.Fatal("excluded token simpleContent returned a schema")
				}
				diagnostic := requireDiagnostic(t, err)
				want := tokenSimpleContentLoc(t, "root.xsd", root, test.marker)
				if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != want || !errors.Is(err, test.cause) {
					t.Fatalf("%s diagnostic = %s, want %s/%s at %s", test.name, diagnostic, test.class, test.code, want)
				}
				if test.spec != "" && diagnostic.SpecRef() != test.spec {
					t.Fatalf("spec ref = %s, want %s", diagnostic.SpecRef(), test.spec)
				}
			})
		}
	}
}

//nolint:gocognit // Final policy differs by edition and must retain its use-site and declaration locations.
func TestTokenSimpleContentFinalPolicy(t *testing.T) {
	for _, profile := range tokenSimpleContentPolicies() {
		t.Run(profile.name+"/schema default extension", func(t *testing.T) {
			root := strings.Replace(tokenSimpleContentRoot(tokenSimpleContentOwner("t:Named", "")+`<xs:simpleType name="Named"><xs:restriction base="xs:token"/></xs:simpleType>`), `targetNamespace="urn:token"`, `targetNamespace="urn:token" finalDefault="extension"`, 1)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if profile.policy == Strict10 {
				if err != nil {
					t.Fatalf("Strict10 schema finalDefault extension should not block simple type extension: %v", err)
				}
				if len(schema.Components()) != 2 {
					t.Fatalf("component count = %d", len(schema.Components()))
				}
				return
			}
			assertZeroSchema(t, schema)
			if err == nil {
				t.Fatal("finalDefault extension admitted token simpleContent")
			}
			diagnostic := requireDiagnostic(t, err)
			want := tokenSimpleContentLoc(t, "root.xsd", root, `base="t:Named"`)
			related := tokenSimpleContentLoc(t, "root.xsd", root, `finalDefault="extension"`)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaSimpleTypeBaseCode || diagnostic.Loc() != want || !reflect.DeepEqual(diagnostic.Related(), []Loc{related}) || diagnostic.SpecRef() != schemaSimpleContentSpecRef(profile.version) || !errors.Is(err, errSchemaSimpleTypeInvalidDerivation) {
				t.Fatalf("default final diagnostic = %s", diagnostic)
			}
		})
		if profile.policy == Strict10 {
			continue
		}
		t.Run(profile.name+"/named final extension", func(t *testing.T) {
			root := tokenSimpleContentRoot(tokenSimpleContentOwner("t:Named", "") + `<xs:simpleType name="Named" final="extension"><xs:restriction base="xs:token"/></xs:simpleType>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			assertZeroSchema(t, schema)
			if err == nil {
				t.Fatal("final extension admitted token simpleContent")
			}
			diagnostic := requireDiagnostic(t, err)
			want := tokenSimpleContentLoc(t, "root.xsd", root, `base="t:Named"`)
			related := tokenSimpleContentLoc(t, "root.xsd", root, `final="extension"`)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaSimpleTypeBaseCode || diagnostic.Loc() != want || !reflect.DeepEqual(diagnostic.Related(), []Loc{related}) || diagnostic.SpecRef() != schemaSimpleContentSpecRef(profile.version) || !errors.Is(err, errSchemaSimpleTypeInvalidDerivation) {
				t.Fatalf("final diagnostic = %s", diagnostic)
			}
		})
	}
}

//nolint:gocognit // Resolve failures share the use-site but have distinct stable codes and causes.
func TestTokenSimpleContentBaseResolutionExits(t *testing.T) {
	for _, profile := range tokenSimpleContentPolicies() {
		for _, test := range []struct {
			name, base, extra, code, relatedMarker string
			cause                                  error
		}{
			{"missing", "t:Missing", "", diagnosticSchemaSimpleContentBaseUnresolvedCode, "", errSchemaSimpleContentBaseUnresolved},
			{"wrong kind", "t:Wrong", `<xs:complexType name="Wrong"/>`, diagnosticSchemaSimpleContentBaseWrongKindCode, `<xs:complexType name="Wrong"`, errSchemaSimpleContentBaseWrongKind},
			{"unsupported atomic", "xs:NMTOKEN", "", UnsupportedSchemaSyntaxCode, "", errSchemaSimpleContentBaseUnsupported},
			{"unsupported list", "t:List", `<xs:simpleType name="List"><xs:list itemType="xs:token"/></xs:simpleType>`, UnsupportedSchemaSyntaxCode, "", errSchemaSimpleContentBaseUnsupported},
			{"unsupported union", "t:Union", `<xs:simpleType name="Union"><xs:union memberTypes="xs:token xs:string"/></xs:simpleType>`, UnsupportedSchemaSyntaxCode, "", errSchemaSimpleContentBaseUnsupported},
			{"complex token content", "t:Inner", strings.Replace(tokenSimpleContentOwner("xs:token", ""), `name="Value"`, `name="Inner"`, 1), UnsupportedSchemaSyntaxCode, `<xs:complexType name="Inner"`, errSchemaSimpleContentBaseUnsupported},
			{"complex named token content", "t:Inner", `<xs:simpleType name="Named"><xs:restriction base="xs:token"/></xs:simpleType>` + strings.Replace(tokenSimpleContentOwner("t:Named", ""), `name="Value"`, `name="Inner"`, 1), UnsupportedSchemaSyntaxCode, `<xs:complexType name="Inner"`, errSchemaSimpleContentBaseUnsupported},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := tokenSimpleContentRoot(tokenSimpleContentOwner(test.base, "") + test.extra)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				if err == nil {
					t.Fatal("bad token base returned a schema")
				}
				diagnostic := requireDiagnostic(t, err)
				class := FailureInvalid
				if test.code == UnsupportedSchemaSyntaxCode {
					class = FailureUnsupported
				}
				want := tokenSimpleContentLoc(t, "root.xsd", root, `base="`+test.base+`"`)
				var related []Loc
				if test.relatedMarker != "" {
					related = []Loc{tokenSimpleContentLoc(t, "root.xsd", root, test.relatedMarker)}
				}
				if diagnostic.Class() != class || diagnostic.Code() != test.code || diagnostic.Loc() != want || diagnostic.SpecRef() != schemaSimpleContentSpecRef(profile.version) || !reflect.DeepEqual(diagnostic.Related(), related) || !errors.Is(err, test.cause) {
					t.Fatalf("%s diagnostic = %s", test.name, diagnostic)
				}
			})
		}
	}
}

func TestTokenSimpleContentComplexBaseChainOrder(t *testing.T) {
	inner := tokenSimpleContentNamedOwner("Inner", "xs:token", "")
	middle := tokenSimpleContentNamedOwner("Middle", "t:Inner", "")
	outer := tokenSimpleContentNamedOwner("Outer", "t:Middle", "")
	for _, profile := range tokenSimpleContentPolicies() {
		for _, test := range []struct {
			name, declarations string
		}{
			{"forward", outer + middle + inner},
			{"reordered", inner + middle + outer},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := tokenSimpleContentRoot(test.declarations)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				if err == nil {
					t.Fatal("complex simpleContent base chain returned a schema")
				}
				diagnostic := requireDiagnostic(t, err)
				want := tokenSimpleContentLoc(t, "root.xsd", root, `base="t:Inner"`)
				related := []Loc{tokenSimpleContentLoc(t, "root.xsd", root, `<xs:complexType name="Inner"`)}
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != want || !reflect.DeepEqual(diagnostic.Related(), related) || diagnostic.SpecRef() != schemaSimpleContentSpecRef(profile.version) || !errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaSimpleContentBaseUnsupported) || !errors.Is(err, errSchemaSimpleContentBaseWrongKind) {
					t.Fatalf("complex base chain diagnostic = %s", diagnostic)
				}
			})
		}
	}
}

//nolint:gocognit // The same final provenance matrix covers policy, source, and declaration order.
func TestTokenSimpleContentComplexBasePropagatesFinal(t *testing.T) {
	inner := tokenSimpleContentNamedOwner("Inner", "t:Named", "")
	outer := tokenSimpleContentNamedOwner("Outer", "t:Inner", "")
	for _, profile := range tokenSimpleContentPolicies() {
		for _, test := range []struct {
			name, rootFinal, named, finalMarker string
		}{
			{"explicit", "", `<xs:simpleType name="Named" final="extension"><xs:restriction base="xs:token"/></xs:simpleType>`, `final="extension"`},
			{"default", ` finalDefault="extension"`, `<xs:simpleType name="Named"><xs:restriction base="xs:token"/></xs:simpleType>`, `finalDefault="extension"`},
		} {
			if profile.policy == Strict10 && test.name == "explicit" {
				continue
			}
			for _, order := range []struct{ name, declarations string }{
				{"forward", outer + inner + test.named},
				{"reordered", test.named + inner + outer},
			} {
				t.Run(profile.name+"/"+test.name+"/"+order.name, func(t *testing.T) {
					root := strings.Replace(tokenSimpleContentRoot(order.declarations), `targetNamespace="urn:token"`, `targetNamespace="urn:token"`+test.rootFinal, 1)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertZeroSchema(t, schema)
					if err == nil {
						t.Fatal("complex simpleContent base bypassed final policy")
					}
					diagnostic := requireDiagnostic(t, err)
					if profile.policy == Strict10 {
						want := tokenSimpleContentLoc(t, "root.xsd", root, `base="t:Inner"`)
						related := []Loc{tokenSimpleContentLoc(t, "root.xsd", root, test.finalMarker)}
						if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.Loc() != want || !reflect.DeepEqual(diagnostic.Related(), related) || diagnostic.SpecRef() != schemaComplexTypeExtensionSpecRef(profile.version) || !errors.Is(err, errSchemaComplexTypeBaseUnsupported) {
							t.Fatalf("Strict10 complex base diagnostic = %s", diagnostic)
						}
						return
					}
					want := tokenSimpleContentLoc(t, "root.xsd", root, `base="t:Named"`)
					related := []Loc{tokenSimpleContentLoc(t, "root.xsd", root, test.finalMarker)}
					if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaSimpleTypeBaseCode || diagnostic.Loc() != want || !reflect.DeepEqual(diagnostic.Related(), related) || diagnostic.SpecRef() != schemaSimpleContentSpecRef(profile.version) || !errors.Is(err, errSchemaSimpleTypeInvalidDerivation) {
						t.Fatalf("complex base final diagnostic = %s", diagnostic)
					}
				})
			}
		}
	}
}

//nolint:gocognit // The final-policy matrix checks both prohibited and permitted extension exits.
func TestTokenSimpleContentComplexBaseFinalPolicy(t *testing.T) {
	outer := tokenSimpleContentNamedOwner("Outer", "t:Inner", "")
	for _, profile := range tokenSimpleContentPolicies() {
		for _, test := range []struct {
			name, rootFinal, innerFinal, finalMarker string
			prohibited                               bool
		}{
			{"explicit extension", "", ` final="extension"`, `final="extension"`, true},
			{"explicit all", "", ` final="#all"`, `final="#all"`, true},
			{"default extension", ` finalDefault="extension"`, "", `finalDefault="extension"`, true},
			{"default all", ` finalDefault="#all"`, "", `finalDefault="#all"`, true},
			{"explicit empty override", ` finalDefault="extension"`, ` final=""`, "", false},
			{"restriction override", ` finalDefault="extension"`, ` final="restriction"`, "", false},
		} {
			inner := strings.Replace(tokenSimpleContentNamedOwner("Inner", "xs:token", ""), `name="Inner"`, `name="Inner"`+test.innerFinal, 1)
			for _, order := range []struct{ name, declarations string }{
				{"forward", outer + inner},
				{"reordered", inner + outer},
			} {
				t.Run(profile.name+"/"+test.name+"/"+order.name, func(t *testing.T) {
					root := strings.Replace(tokenSimpleContentRoot(order.declarations), `targetNamespace="urn:token"`, `targetNamespace="urn:token"`+test.rootFinal, 1)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertZeroSchema(t, schema)
					if err == nil {
						t.Fatal("complex simpleContent base returned a schema")
					}
					diagnostic := requireDiagnostic(t, err)
					want := tokenSimpleContentLoc(t, "root.xsd", root, `base="t:Inner"`)
					if test.prohibited {
						related := []Loc{tokenSimpleContentLoc(t, "root.xsd", root, test.finalMarker)}
						if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.Loc() != want || !reflect.DeepEqual(diagnostic.Related(), related) || diagnostic.SpecRef() != schemaComplexTypeExtensionSpecRef(profile.version) || !errors.Is(err, errSchemaComplexTypeBaseUnsupported) {
							t.Fatalf("prohibited complex final diagnostic = %s", diagnostic)
						}
						return
					}
					related := []Loc{tokenSimpleContentLoc(t, "root.xsd", root, `<xs:complexType name="Inner"`)}
					if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != want || !reflect.DeepEqual(diagnostic.Related(), related) || diagnostic.SpecRef() != schemaSimpleContentSpecRef(profile.version) || !errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaSimpleContentBaseUnsupported) || !errors.Is(err, errSchemaSimpleContentBaseWrongKind) {
						t.Fatalf("permitted complex final diagnostic = %s", diagnostic)
					}
				})
			}
		}
	}
}

func TestTokenSimpleContentComplexBaseFinalDefaultDocument(t *testing.T) {
	for _, profile := range tokenSimpleContentPolicies() {
		for _, test := range []struct{ name, link, target, prefix string }{
			{"include", `<xs:include schemaLocation="inner.xsd"/>`, "urn:token", "t"},
			{"import", `<xs:import namespace="urn:other" schemaLocation="inner.xsd"/>`, "urn:other", "o"},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := tokenSimpleContentRoot(test.link + tokenSimpleContentNamedOwner("Outer", test.prefix+":Inner", ""))
				root = strings.Replace(root, `xmlns:t="urn:token"`, `xmlns:t="urn:token" xmlns:o="urn:other"`, 1)
				inner := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="` + test.target + `" finalDefault="extension">` + tokenSimpleContentNamedOwner("Inner", "xs:token", "") + `</xs:schema>`
				fixtures := map[string]discoveryFixture{"inner.xsd": {id: "inner.xsd", contents: inner}}
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				assertZeroSchema(t, schema)
				if err == nil {
					t.Fatal("target-document complex final default returned a schema")
				}
				diagnostic := requireDiagnostic(t, err)
				want := tokenSimpleContentLoc(t, "root.xsd", root, `base="`+test.prefix+`:Inner"`)
				related := []Loc{tokenSimpleContentLoc(t, "inner.xsd", inner, `finalDefault="extension"`)}
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.Loc() != want || !reflect.DeepEqual(diagnostic.Related(), related) || diagnostic.SpecRef() != schemaComplexTypeExtensionSpecRef(profile.version) || !errors.Is(err, errSchemaComplexTypeBaseUnsupported) {
					t.Fatalf("target-document final diagnostic = %s", diagnostic)
				}
			})
		}
	}
}

//nolint:gocognit // Both final and atomic-kind exits require exact diagnostic provenance.
func TestTokenSimpleContentComplexBaseFinalBeforeAtomicFallback(t *testing.T) {
	for _, profile := range tokenSimpleContentPolicies() {
		for _, test := range []struct{ name, final, code, marker string }{
			{"prohibited", ` final="extension"`, invalidSchemaCompositionCode, `final="extension"`},
			{"permitted", ` final=""`, diagnosticSchemaSimpleContentBaseWrongKindCode, `<xs:complexType name="Inner"`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				inner := strings.Replace(tokenSimpleContentNamedOwner("Inner", "xs:string", ""), `name="Inner"`, `name="Inner"`+test.final, 1)
				root := tokenSimpleContentRoot(tokenSimpleContentNamedOwner("Outer", "t:Inner", "") + inner)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				if err == nil {
					t.Fatal("non-token complex base returned a schema")
				}
				diagnostic := requireDiagnostic(t, err)
				want := tokenSimpleContentLoc(t, "root.xsd", root, `base="t:Inner"`)
				related := []Loc{tokenSimpleContentLoc(t, "root.xsd", root, test.marker)}
				cause, spec := errSchemaComplexTypeBaseUnsupported, schemaComplexTypeExtensionSpecRef(profile.version)
				if test.name == "permitted" {
					cause, spec = errSchemaSimpleContentBaseWrongKind, schemaSimpleContentSpecRef(profile.version)
				}
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code || diagnostic.Loc() != want || !reflect.DeepEqual(diagnostic.Related(), related) || diagnostic.SpecRef() != spec || !errors.Is(err, cause) {
					t.Fatalf("non-token final diagnostic = %s", diagnostic)
				}
			})
		}
	}
}

//nolint:gocognit // Cycle matrix asserts ordered edge locations for each policy and graph shape.
func TestTokenSimpleContentComplexBaseCycle(t *testing.T) {
	for _, profile := range tokenSimpleContentPolicies() {
		for _, test := range []struct {
			name, declarations, closing string
			prior                       []string
		}{
			{"self", tokenSimpleContentNamedOwner("Self", "t:Self", ""), `base="t:Self"`, nil},
			{"forward pair", tokenSimpleContentNamedOwner("Outer", "t:Inner", "") + tokenSimpleContentNamedOwner("Inner", "t:Outer", ""), `base="t:Outer"`, []string{`base="t:Inner"`}},
			{"reordered pair", tokenSimpleContentNamedOwner("Inner", "t:Outer", "") + tokenSimpleContentNamedOwner("Outer", "t:Inner", ""), `base="t:Inner"`, []string{`base="t:Outer"`}},
			{"three nodes", tokenSimpleContentNamedOwner("Outer", "t:Middle", "") + tokenSimpleContentNamedOwner("Middle", "t:Inner", "") + tokenSimpleContentNamedOwner("Inner", "t:Outer", ""), `base="t:Outer"`, []string{`base="t:Middle"`, `base="t:Inner"`}},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := tokenSimpleContentRoot(test.declarations)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				if err == nil {
					t.Fatal("complex simpleContent base cycle returned a schema")
				}
				diagnostic := requireDiagnostic(t, err)
				want := tokenSimpleContentLoc(t, "root.xsd", root, test.closing)
				var related []Loc
				for _, marker := range test.prior {
					related = append(related, tokenSimpleContentLoc(t, "root.xsd", root, marker))
				}
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaSimpleContentBaseUnresolvedCode || diagnostic.Loc() != want || !reflect.DeepEqual(diagnostic.Related(), related) || diagnostic.SpecRef() != schemaSimpleContentSpecRef(profile.version) || !errors.Is(err, errSchemaComplexTypeBaseCycle) || !errors.Is(err, errSchemaSimpleContentBaseUnresolved) {
					t.Fatalf("complex base cycle diagnostic = %s", diagnostic)
				}
			})
		}
	}
}

func TestTokenSimpleContentComplexBaseInvalidTarget(t *testing.T) {
	for _, profile := range tokenSimpleContentPolicies() {
		for _, test := range []struct{ name, declarations string }{
			{"forward", tokenSimpleContentNamedOwner("Outer", "t:Inner", "") + tokenSimpleContentNamedOwner("Inner", "t:Wrong", "") + `<xs:complexType name="Wrong"/>`},
			{"reordered", `<xs:complexType name="Wrong"/>` + tokenSimpleContentNamedOwner("Inner", "t:Wrong", "") + tokenSimpleContentNamedOwner("Outer", "t:Inner", "")},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := tokenSimpleContentRoot(test.declarations)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				if err == nil {
					t.Fatal("invalid simpleContent dependency returned a schema")
				}
				diagnostic := requireDiagnostic(t, err)
				want := tokenSimpleContentLoc(t, "root.xsd", root, `base="t:Wrong"`)
				related := []Loc{tokenSimpleContentLoc(t, "root.xsd", root, `<xs:complexType name="Wrong"`)}
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaSimpleContentBaseWrongKindCode || diagnostic.Loc() != want || !reflect.DeepEqual(diagnostic.Related(), related) || diagnostic.SpecRef() != schemaSimpleContentSpecRef(profile.version) || !errors.Is(err, errSchemaSimpleContentBaseWrongKind) || !errors.Is(err, errSchemaSimpleTypeBaseWrongKind) {
					t.Fatalf("invalid dependency diagnostic = %s", diagnostic)
				}
			})
		}
	}
}

func TestTokenSimpleContentComplexBaseInvisible(t *testing.T) {
	for _, profile := range tokenSimpleContentPolicies() {
		t.Run(profile.name, func(t *testing.T) {
			root := strings.Replace(tokenSimpleContentRoot(tokenSimpleContentNamedOwner("Outer", "o:Inner", "")), `xmlns:t="urn:token"`, `xmlns:t="urn:token" xmlns:o="urn:other"`, 1)
			other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other">` + tokenSimpleContentNamedOwner("Inner", "xs:token", "") + `</xs:schema>`
			rootDocument := elementReferenceTestSyntaxDocument(t, "root.xsd", root)
			otherDocument := elementReferenceTestSyntaxDocument(t, "other.xsd", other)
			schema, err := newSchemaFromDiscoveryWithPolicy(syntaxDiscoveryResult{documents: []*syntaxDocument{rootDocument, otherDocument}}, profile.policy)
			assertZeroSchema(t, schema)
			if err == nil {
				t.Fatal("invisible complex base returned a schema")
			}
			diagnostic := requireDiagnostic(t, err)
			want := tokenSimpleContentLoc(t, "root.xsd", root, `base="o:Inner"`)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaSimpleContentBaseUnresolvedCode || diagnostic.Loc() != want || len(diagnostic.Related()) != 0 || diagnostic.SpecRef() != schemaSimpleContentSpecRef(profile.version) || !errors.Is(err, errSchemaSimpleContentBaseUnresolved) {
				t.Fatalf("invisible complex base diagnostic = %s", diagnostic)
			}
		})
	}
}

//nolint:gocognit // Graph failures have different phase origins; each must prevent publication.
func TestTokenSimpleContentGraphFailureBoundaries(t *testing.T) {
	for _, profile := range tokenSimpleContentPolicies() {
		for _, test := range []struct {
			name, extra, primary, code, spec string
			class                            FailureClass
			cause                            error
			fixtures                         map[string]discoveryFixture
		}{
			{
				name: "cyclic named base", extra: `<xs:simpleType name="A"><xs:restriction base="t:B"/></xs:simpleType><xs:simpleType name="B"><xs:restriction base="t:A"/></xs:simpleType>`,
				primary: `base="t:A"`, code: diagnosticSchemaSimpleContentBaseUnresolvedCode, spec: schemaSimpleContentSpecRef(profile.version), class: FailureInvalid, cause: errSchemaSimpleTypeBaseCycle,
			},
			{
				name: "invalid named facet", extra: `<xs:simpleType name="A"><xs:restriction base="xs:token"><xs:whiteSpace value="preserve"/></xs:restriction></xs:simpleType>`,
				primary: `value="preserve"`, code: InvalidStringWhiteSpaceRestrictionCode, spec: stringWhiteSpaceSpecRef(profile.version), class: FailureInvalid, cause: errInvalidStringWhiteSpaceRestriction,
			},
			{
				name: "duplicate target", extra: `<xs:simpleType name="A"><xs:restriction base="xs:token"/></xs:simpleType><xs:simpleType name="A"><xs:restriction base="xs:token"/></xs:simpleType>`,
				primary: `<xs:simpleType name="A"`, code: diagnosticSchemaGlobalDuplicateCode, spec: schemaGlobalDuplicateSpecRef(profile.version), class: FailureInvalid, cause: errSchemaGlobalDeclarationDuplicate,
			},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := tokenSimpleContentRoot(tokenSimpleContentOwner("t:A", "") + test.extra)
				schema, err := discoverTestSchemaWithPolicy(t, root, test.fixtures, profile.policy)
				assertZeroSchema(t, schema)
				if err == nil {
					t.Fatal("graph failure returned a schema")
				}
				diagnostic := requireDiagnostic(t, err)
				want := tokenSimpleContentLoc(t, "root.xsd", root, test.primary)
				if test.name == "duplicate target" {
					want = allParticleTestTokenLoc(t, "root.xsd", root, test.primary, 2)
				}
				if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != want || diagnostic.SpecRef() != test.spec || !errors.Is(err, test.cause) {
					t.Fatalf("graph diagnostic = %s, want %s/%s at %s", diagnostic, test.class, test.code, want)
				}
				if test.name == "duplicate target" {
					first := tokenSimpleContentLoc(t, "root.xsd", root, test.primary)
					if !reflect.DeepEqual(diagnostic.Related(), []Loc{first}) {
						t.Fatalf("duplicate related = %v, want [%s]", diagnostic.Related(), first)
					}
				}
				if test.name == "cyclic named base" {
					wantRelated := []Loc{
						tokenSimpleContentLoc(t, "root.xsd", root, `<xs:complexType name="Value"`),
						tokenSimpleContentLoc(t, "root.xsd", root, `base="t:B"`),
						allParticleTestTokenLoc(t, "root.xsd", root, `base="t:A"`, 2),
						tokenSimpleContentLoc(t, "root.xsd", root, `<xs:simpleType name="A"`),
						tokenSimpleContentLoc(t, "root.xsd", root, `<xs:simpleType name="B"`),
					}
					if !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
						t.Fatalf("cycle related = %v, want %v", diagnostic.Related(), wantRelated)
					}
				}
				if test.name == "invalid named facet" && len(diagnostic.Related()) != 0 {
					t.Fatalf("invalid facet related = %v, want none", diagnostic.Related())
				}
			})
		}
	}
}

func TestTokenSimpleContentInvisibleBase(t *testing.T) {
	for _, profile := range tokenSimpleContentPolicies() {
		t.Run(profile.name, func(t *testing.T) {
			root := tokenSimpleContentRoot(tokenSimpleContentOwner("o:Hidden", ""))
			root = strings.Replace(root, `xmlns:t="urn:token"`, `xmlns:t="urn:token" xmlns:o="urn:other"`, 1)
			other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Hidden"><xs:restriction base="xs:token"/></xs:simpleType></xs:schema>`
			rootDocument := elementReferenceTestSyntaxDocument(t, "root.xsd", root)
			otherDocument := elementReferenceTestSyntaxDocument(t, "other.xsd", other)
			schema, err := newSchemaFromDiscoveryWithPolicy(syntaxDiscoveryResult{documents: []*syntaxDocument{rootDocument, otherDocument}}, profile.policy)
			assertZeroSchema(t, schema)
			if err == nil {
				t.Fatal("base without import became visible")
			}
			diagnostic := requireDiagnostic(t, err)
			want := tokenSimpleContentLoc(t, "root.xsd", root, `base="o:Hidden"`)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaSimpleContentBaseUnresolvedCode || diagnostic.Loc() != want || len(diagnostic.Related()) != 0 || diagnostic.SpecRef() != schemaSimpleContentSpecRef(profile.version) || !errors.Is(err, errSchemaSimpleContentBaseUnresolved) {
				t.Fatalf("invisible base diagnostic = %s", diagnostic)
			}
		})
	}
}

//nolint:gocognit // Preserve edition-specific composition and syntax exits.
func TestTokenSimpleContentMixedAndCompositionExclusions(t *testing.T) {
	for _, profile := range tokenSimpleContentPolicies() {
		for _, test := range []struct {
			name, owner, marker, code string
			occurrence                int
		}{
			{"mixed true", `<xs:complexType name="Value" mixed="true"><xs:simpleContent><xs:extension base="xs:token"/></xs:simpleContent></xs:complexType>`, `mixed="true"`, invalidSchemaCompositionCode, 1},
			{"particle child", `<xs:complexType name="Value"><xs:simpleContent><xs:extension base="xs:token"><xs:sequence/></xs:extension></xs:simpleContent></xs:complexType>`, `<xs:sequence`, invalidSchemaCompositionCode, 1},
			{"duplicate base", `<xs:complexType name="Value"><xs:simpleContent><xs:extension base="xs:token" base="xs:token"/></xs:simpleContent></xs:complexType>`, `base="xs:token"`, InvalidXMLSyntaxCode, 2},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := tokenSimpleContentRoot(test.owner)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				if err == nil {
					t.Fatal("invalid composition returned a schema")
				}
				diagnostic := requireDiagnostic(t, err)
				wantLoc := allParticleTestTokenLoc(t, "root.xsd", root, test.marker, test.occurrence)
				wantClass, wantCode, wantSpec := FailureInvalid, test.code, ""
				if test.name == "mixed true" && profile.policy == Strict10 {
					wantClass, wantCode, wantSpec = FailureUnsupported, UnsupportedSchemaSyntaxCode, "xsd10-structures#schema-document"
				}
				if diagnostic.Class() != wantClass || diagnostic.Code() != wantCode || diagnostic.Loc() != wantLoc || len(diagnostic.Related()) != 0 || diagnostic.SpecRef() != wantSpec {
					t.Fatalf("composition diagnostic = %s, want %s/%s at %s, spec %s", diagnostic, wantClass, wantCode, wantLoc, wantSpec)
				}
				if wantClass == FailureUnsupported && !errors.Is(err, ErrUnsupported) {
					t.Fatalf("mixed policy diagnostic lost unsupported cause: %v", err)
				}
			})
		}
	}
}
