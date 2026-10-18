package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func normalizedSimpleContentRoot(body string) string {
	return strings.Replace(tokenSimpleContentRoot(body), `xmlns:t="urn:token"`, `xmlns:t="urn:token" xmlns:o="urn:other"`, 1)
}

//nolint:gocognit,funlen // Public facts are checked across policies and graph boundaries.
func TestNormalizedStringSimpleContentQuery(t *testing.T) {
	for _, profile := range tokenSimpleContentPolicies() {
		for _, tc := range []struct {
			name, base, before, after, fixture, source, variety, enumMarker, enumValue, whiteMarker string
			walk                                                                                    []string
		}{
			{name: "builtin", base: "xs:normalizedString", walk: []string{"Value", "root"}},
			{name: "forward", base: "t:Forward", after: `<xs:simpleType name="Forward"><xs:restriction base="t:Direct"/></xs:simpleType><xs:simpleType name="Direct"><xs:restriction base="xs:normalizedString"><xs:enumeration value=" a  b "/></xs:restriction></xs:simpleType>`, source: "root.xsd", variety: `<xs:restriction base="t:Direct"`, enumMarker: `value=" a  b "`, enumValue: " a  b ", walk: []string{"Value", "root", "Forward", "Direct"}},
			{name: "explicit replace", base: "t:Named", before: `<xs:simpleType name="Named"><xs:restriction base="xs:normalizedString"><xs:whiteSpace value="replace" fixed="true"/><xs:enumeration value="named"/></xs:restriction></xs:simpleType>`, source: "root.xsd", variety: `<xs:restriction base="xs:normalizedString"`, enumMarker: `value="named"`, enumValue: "named", whiteMarker: `value="replace"`, walk: []string{"Named", "Value", "root"}},
			{name: "include", base: "t:Included", before: `<xs:include schemaLocation="included.xsd"/>`, source: "included.xsd", fixture: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:token"><xs:simpleType name="Included"><xs:restriction base="xs:normalizedString"><xs:enumeration value="included"/></xs:restriction></xs:simpleType></xs:schema>`, variety: `<xs:restriction base="xs:normalizedString"`, enumMarker: `value="included"`, enumValue: "included", walk: []string{"Value", "root", "Included"}},
			{name: "chameleon", base: "t:Chameleon", before: `<xs:include schemaLocation="chameleon.xsd"/>`, source: "chameleon.xsd", fixture: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Chameleon"><xs:restriction base="xs:normalizedString"><xs:enumeration value="chameleon"/></xs:restriction></xs:simpleType></xs:schema>`, variety: `<xs:restriction base="xs:normalizedString"`, enumMarker: `value="chameleon"`, enumValue: "chameleon", walk: []string{"Value", "root", "Chameleon"}},
			{name: "import", base: "o:Imported", before: `<xs:import namespace="urn:other" schemaLocation="other.xsd"/>`, source: "other.xsd", fixture: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Imported"><xs:restriction base="xs:normalizedString"><xs:enumeration value="imported"/></xs:restriction></xs:simpleType></xs:schema>`, variety: `<xs:restriction base="xs:normalizedString"`, enumMarker: `value="imported"`, enumValue: "imported", walk: []string{"Value", "root", "Imported"}},
		} {
			t.Run(profile.name+"/"+tc.name, func(t *testing.T) {
				root := normalizedSimpleContentRoot(tc.before + tokenSimpleContentOwner(tc.base, "") + `<xs:element name="root" type="t:Value"/>` + tc.after)
				fixtures := map[string]discoveryFixture{}
				if tc.fixture != "" {
					fixtures[tc.source] = discoveryFixture{id: SourceID(tc.source), contents: tc.fixture}
				}
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				if err != nil {
					t.Fatalf("parse: %v", err)
				}
				owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:token", "Value")
				if owner.Particle() != nil || len(owner.AttributeUses()) != 0 {
					t.Fatalf("particle/uses = %T/%d", owner.Particle(), len(owner.AttributeUses()))
				}
				ext, ok := owner.SimpleContentExtension()
				if !ok {
					t.Fatal("missing extension")
				}
				ns := "urn:token"
				if tc.name == "builtin" {
					ns = testXSDNamespace
				}
				if tc.name == "import" {
					ns = "urn:other"
				}
				wantName := mustTestQName(t, ns, strings.Split(tc.base, ":")[1])
				baseLoc := tokenSimpleContentLoc(t, "root.xsd", root, `base="`+tc.base+`"`)
				ref, ok := ext.TypeReference()
				if !ok || ext.Base() != wantName || ext.BaseLoc() != baseLoc || ref.Name() != wantName || ref.Loc() != baseLoc || ref.Variety() != SimpleTypeVarietyAtomicRestriction {
					t.Fatalf("extension/reference = %#v/%#v", ext, ref)
				}
				ws, ok := ref.StringWhiteSpaceFacet()
				if !ok || ws.Value() != "replace" || ws.Fixed() != (tc.whiteMarker != "") {
					t.Fatalf("whitespace = %#v/%t", ws, ok)
				}
				wantWS := Loc{}
				if tc.whiteMarker != "" {
					wantWS = tokenSimpleContentLoc(t, SourceID(tc.source), root, tc.whiteMarker)
				}
				if ws.Loc() != wantWS {
					t.Fatalf("whitespace loc = %s, want %s", ws.Loc(), wantWS)
				}
				id, hasID := ext.TypeID()
				refID, hasRefID := ref.ComponentID()
				if tc.name == "builtin" {
					if !ref.IsBuiltin() || hasID || hasRefID || !id.IsZero() || !refID.IsZero() || ref.VarietyLoc() != baseLoc {
						t.Fatalf("builtin identity = %#v/%#v", ext, ref)
					}
				}
				if tc.name != "builtin" {
					definition := root
					if tc.fixture != "" {
						definition = tc.fixture
					}
					wantVariety := tokenSimpleContentLoc(t, SourceID(tc.source), definition, tc.variety)
					if !ref.IsNamed() || !hasID || !hasRefID || id != refID || id != componentIDForName(t, schema, wantName) || ref.VarietyLoc() != wantVariety {
						t.Fatalf("named identity/provenance = %v/%v/%s want %s", id, refID, ref.VarietyLoc(), wantVariety)
					}
					facts := ref.StringEnumerationFacets()
					wantEnum := tokenSimpleContentLoc(t, SourceID(tc.source), definition, tc.enumMarker)
					if !reflect.DeepEqual(facts.Values(), []string{tc.enumValue}) || !reflect.DeepEqual(facts.Locations(), []Loc{wantEnum}) {
						t.Fatalf("facets = %v/%v", facts.Values(), facts.Locations())
					}
					values := facts.Values()
					locs := facts.Locations()
					values[0] = "changed"
					locs[0] = Loc{}
					if ref.StringEnumerationFacets().Values()[0] != tc.enumValue || ref.StringEnumerationFacets().Locations()[0] != wantEnum {
						t.Fatal("facet copies mutated")
					}
				}
				for n := 0; n < 2; n++ {
					queried := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:token", "Value")
					again, _ := queried.SimpleContentExtension()
					againRef, _ := again.TypeReference()
					againSpace, hasAgainSpace := againRef.StringWhiteSpaceFacet()
					if again.Base() != wantName || !hasAgainSpace || againSpace.Value() != "replace" {
						t.Fatal("query changed")
					}
					var walked []string
					if err := schema.Walk(func(c Component) error { walked = append(walked, c.Name().Local()); return nil }); err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(walked, tc.walk) {
						t.Fatalf("walk = %v, want %v", walked, tc.walk)
					}
				}
			})
		}
	}
}

//nolint:gocognit // Check distinct public diagnostics for both consumers and base forms.
func TestNormalizedStringSimpleContentConsumers(t *testing.T) {
	for _, profile := range tokenSimpleContentPolicies() {
		for _, base := range []string{"xs:normalizedString", "t:Named"} {
			t.Run(profile.name+"/"+base, func(t *testing.T) {
				root := normalizedSimpleContentRoot(tokenSimpleContentOwner(base, "") + `<xs:simpleType name="Named"><xs:restriction base="xs:normalizedString"/></xs:simpleType><xs:element name="root" type="t:Value"/>`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				instanceErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:token">value</root>`)))
				diag := requireDiagnostic(t, instanceErr)
				ownerLoc := tokenSimpleContentLoc(t, "root.xsd", root, `<xs:complexType name="Value"`)
				elementLoc := tokenSimpleContentLoc(t, "root.xsd", root, `<xs:element name="root"`)
				if diag.Class() != FailureUnsupported || diag.Code() != UnsupportedInstanceValidationCode || diag.Loc() != mustTestLoc(t, "instance.xml", 1, 1) || !reflect.DeepEqual(diag.Related(), []Loc{elementLoc, ownerLoc}) || diag.SpecRef() != instanceValidationSpecRef(profile.version) || !errors.Is(instanceErr, ErrUnsupported) {
					t.Fatalf("validation = %s", diag)
				}
				output, generateErr := GenerateGo(schema, "generated")
				if output != nil {
					t.Fatalf("generated output = %s", output)
				}
				generation := requireDiagnostic(t, generateErr)
				if generation.Class() != FailureUnsupported || generation.Code() != diagnosticCodegenUnsupported || generation.Loc() != ownerLoc || len(generation.Related()) != 0 || generation.SpecRef() != codegenDirectChoiceSpecReference(profile.version, codegenDirectChoiceParticlesReference) || !errors.Is(generateErr, ErrUnsupported) {
					t.Fatalf("generation = %s", generation)
				}
			})
		}
	}
}

//nolint:gocognit // Each syntax shape and raw attribute form has its own located exit.
func TestNormalizedStringSimpleContentExclusions(t *testing.T) {
	for _, profile := range tokenSimpleContentPolicies() {
		named := `<xs:simpleType name="Named"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`
		for _, tc := range []struct {
			name, body, marker, code, spec string
			class                          FailureClass
			cause                          error
		}{
			{"anonymous builtin", `<xs:element name="root"><xs:complexType><xs:simpleContent><xs:extension base="xs:normalizedString"/></xs:simpleContent></xs:complexType></xs:element>`, `<xs:simpleContent>`, UnsupportedSchemaSyntaxCode, schemaSimpleContentSpecRef(profile.version), FailureUnsupported, errSchemaSimpleContentBaseUnsupported},
			{"anonymous named", named + `<xs:element name="root"><xs:complexType><xs:simpleContent><xs:extension base="t:Named"/></xs:simpleContent></xs:complexType></xs:element>`, `<xs:simpleContent>`, UnsupportedSchemaSyntaxCode, schemaSimpleContentSpecRef(profile.version), FailureUnsupported, errSchemaSimpleContentBaseUnsupported},
			{"restriction", `<xs:complexType name="Value"><xs:simpleContent><xs:restriction base="xs:normalizedString"/></xs:simpleContent></xs:complexType>`, `<xs:restriction base="xs:normalizedString"`, UnsupportedSchemaSyntaxCode, "xsd10-structures#schema-document", FailureUnsupported, ErrUnsupported},
			{"named restriction", named + `<xs:complexType name="Value"><xs:simpleContent><xs:restriction base="t:Named"/></xs:simpleContent></xs:complexType>`, `<xs:restriction base="t:Named"`, UnsupportedSchemaSyntaxCode, "xsd10-structures#schema-document", FailureUnsupported, ErrUnsupported},
			{"inline base", tokenSimpleContentOwner("xs:normalizedString", `<xs:simpleType><xs:restriction base="xs:normalizedString"/></xs:simpleType>`), `<xs:simpleType>`, invalidSchemaCompositionCode, "", FailureInvalid, nil},
			{"wildcard", tokenSimpleContentOwner("xs:normalizedString", `<xs:anyAttribute/>`), `<xs:anyAttribute`, UnsupportedSchemaSyntaxCode, schemaAnyAttributeSpecRef(profile.version), FailureUnsupported, ErrUnsupported},
			{"list", tokenSimpleContentOwner("t:List", "") + `<xs:simpleType name="List"><xs:list itemType="xs:normalizedString"/></xs:simpleType>`, `base="t:List"`, UnsupportedSchemaSyntaxCode, schemaSimpleContentSpecRef(profile.version), FailureUnsupported, errSchemaSimpleContentBaseUnsupported},
			{"union", tokenSimpleContentOwner("t:Union", "") + `<xs:simpleType name="Union"><xs:union memberTypes="xs:normalizedString xs:token"/></xs:simpleType>`, `base="t:Union"`, UnsupportedSchemaSyntaxCode, schemaSimpleContentSpecRef(profile.version), FailureUnsupported, errSchemaSimpleContentBaseUnsupported},
			{"other scalar", tokenSimpleContentOwner("xs:language", ""), `base="xs:language"`, UnsupportedSchemaSyntaxCode, schemaSimpleContentSpecRef(profile.version), FailureUnsupported, errSchemaSimpleContentBaseUnsupported},
		} {
			t.Run(profile.name+"/"+tc.name, func(t *testing.T) {
				root := normalizedSimpleContentRoot(tc.body)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				want := tokenSimpleContentLoc(t, "root.xsd", root, tc.marker)
				if d.Class() != tc.class || d.Code() != tc.code || d.Loc() != want || len(d.Related()) != 0 || tc.spec != "" && d.SpecRef() != tc.spec || tc.cause != nil && !errors.Is(err, tc.cause) {
					t.Fatalf("%s: %s; want %s/%s at %s, spec %s", tc.name, d, tc.class, tc.code, want, tc.spec)
				}
			})
		}
		for _, base := range []string{"xs:normalizedString", "t:Named"} {
			for _, tc := range []struct{ name, attribute, extra, marker string }{
				{"direct", `<xs:attribute name="flag" type="xs:boolean"/>`, "", `<xs:attribute name="flag"`},
				{"named", `<xs:attribute name="flag" type="t:Flag"/>`, `<xs:simpleType name="Flag"><xs:restriction base="xs:boolean"/></xs:simpleType>`, `<xs:attribute name="flag"`},
				{"inline", `<xs:attribute name="flag"><xs:simpleType><xs:restriction base="xs:boolean"/></xs:simpleType></xs:attribute>`, "", `<xs:attribute name="flag"`},
				{"ref", `<xs:attribute ref="t:flag"/>`, `<xs:attribute name="flag" type="xs:boolean"/>`, `<xs:attribute ref="t:flag"`},
				{"prohibited", `<xs:attribute name="flag" type="xs:boolean" use="prohibited"/>`, "", `<xs:attribute name="flag"`},
			} {
				t.Run(profile.name+"/"+base+"/"+tc.name, func(t *testing.T) {
					root := normalizedSimpleContentRoot(tokenSimpleContentOwner(base, tc.attribute) + named + tc.extra)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					want := tokenSimpleContentLoc(t, "root.xsd", root, tc.marker)
					if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != want || len(d.Related()) != 0 || d.SpecRef() != schemaSimpleContentSpecRef(profile.version) || !errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaSimpleContentBaseUnsupported) {
						t.Fatalf("attribute exclusion: %s", d)
					}
				})
			}
		}
	}
}

//nolint:gocognit // Competing resolution, final, and chain exits establish phase order.
func TestNormalizedStringSimpleContentBoundaryExits(t *testing.T) {
	for _, profile := range tokenSimpleContentPolicies() {
		for _, tc := range []struct {
			name, body, primary, related, code, spec string
			class                                    FailureClass
			cause                                    error
		}{
			{"missing before owner", `<xs:element name="root"><xs:complexType><xs:simpleContent><xs:extension base="t:Missing"><xs:attribute name="flag" type="xs:boolean"/></xs:extension></xs:simpleContent></xs:complexType></xs:element>`, `base="t:Missing"`, "", diagnosticSchemaSimpleContentBaseUnresolvedCode, schemaSimpleContentSpecRef(profile.version), FailureInvalid, errSchemaSimpleContentBaseUnresolved},
			{"wrong kind before owner", `<xs:complexType name="Wrong"/><xs:element name="root"><xs:complexType><xs:simpleContent><xs:extension base="t:Wrong"/></xs:simpleContent></xs:complexType></xs:element>`, `base="t:Wrong"`, `<xs:complexType name="Wrong"`, diagnosticSchemaSimpleContentBaseWrongKindCode, schemaSimpleContentSpecRef(profile.version), FailureInvalid, errSchemaSimpleContentBaseWrongKind},
			{"complex chain", tokenSimpleContentNamedOwner("Outer", "t:Inner", "") + tokenSimpleContentNamedOwner("Inner", "xs:normalizedString", ""), `base="t:Inner"`, `<xs:complexType name="Inner"`, UnsupportedSchemaSyntaxCode, schemaSimpleContentSpecRef(profile.version), FailureUnsupported, errSchemaSimpleContentBaseUnsupported},
			{"complex named chain", tokenSimpleContentNamedOwner("Outer", "t:Inner", "") + tokenSimpleContentNamedOwner("Inner", "t:Named", "") + `<xs:simpleType name="Named"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`, `base="t:Inner"`, `<xs:complexType name="Inner"`, UnsupportedSchemaSyntaxCode, schemaSimpleContentSpecRef(profile.version), FailureUnsupported, errSchemaSimpleContentBaseUnsupported},
			{"complex inner missing", tokenSimpleContentNamedOwner("Outer", "t:Inner", "") + tokenSimpleContentNamedOwner("Inner", "t:Missing", ""), `base="t:Missing"`, "", diagnosticSchemaSimpleContentBaseUnresolvedCode, schemaSimpleContentSpecRef(profile.version), FailureInvalid, errSchemaSimpleContentBaseUnresolved},
			{"complex inner final", tokenSimpleContentNamedOwner("Outer", "t:Inner", "") + strings.Replace(tokenSimpleContentNamedOwner("Inner", "xs:normalizedString", ""), `name="Inner"`, `name="Inner" final="extension"`, 1), `base="t:Inner"`, `final="extension"`, invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(profile.version), FailureInvalid, errSchemaComplexTypeBaseUnsupported},
			{"simple type cycle", tokenSimpleContentOwner("t:A", "") + `<xs:simpleType name="A"><xs:restriction base="t:B"/></xs:simpleType><xs:simpleType name="B"><xs:restriction base="t:A"/></xs:simpleType>`, `base="t:A"`, `<xs:complexType name="Value"`, diagnosticSchemaSimpleContentBaseUnresolvedCode, schemaSimpleContentSpecRef(profile.version), FailureInvalid, errSchemaSimpleTypeBaseCycle},
		} {
			t.Run(profile.name+"/"+tc.name, func(t *testing.T) {
				root := normalizedSimpleContentRoot(tc.body)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				want := tokenSimpleContentLoc(t, "root.xsd", root, tc.primary)
				if d.Class() != tc.class || d.Code() != tc.code || d.Loc() != want || d.SpecRef() != tc.spec || !errors.Is(err, tc.cause) {
					t.Fatalf("boundary exit: %s", d)
				}
				var related []Loc
				if tc.related != "" {
					related = []Loc{tokenSimpleContentLoc(t, "root.xsd", root, tc.related)}
				}
				if tc.name == "simple type cycle" {
					related = []Loc{
						tokenSimpleContentLoc(t, "root.xsd", root, `<xs:complexType name="Value"`),
						tokenSimpleContentLoc(t, "root.xsd", root, `base="t:B"`),
						allParticleTestTokenLoc(t, "root.xsd", root, `base="t:A"`, 2),
						tokenSimpleContentLoc(t, "root.xsd", root, `<xs:simpleType name="A"`),
						tokenSimpleContentLoc(t, "root.xsd", root, `<xs:simpleType name="B"`),
					}
				}
				if !reflect.DeepEqual(d.Related(), related) {
					t.Fatalf("related = %v, want %v", d.Related(), related)
				}
			})
		}
		for _, final := range []struct{ name, schemaAttr, namedAttr, marker string }{
			{"finalDefault", ` finalDefault="extension"`, "", `finalDefault="extension"`},
			{"named final", "", ` final="extension"`, `final="extension"`},
			{"empty override", ` finalDefault="extension"`, ` final=""`, ""},
		} {
			if profile.policy == Strict10 && final.name == "named final" {
				continue
			}
			t.Run(profile.name+"/"+final.name, func(t *testing.T) {
				owner := tokenSimpleContentOwner("t:Named", `<xs:attribute name="flag" type="xs:boolean" use="prohibited"/>`)
				named := strings.Replace(`<xs:simpleType name="Named"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`, `name="Named"`, `name="Named"`+final.namedAttr, 1)
				root := strings.Replace(normalizedSimpleContentRoot(owner+named), `targetNamespace="urn:token"`, `targetNamespace="urn:token"`+final.schemaAttr, 1)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if final.name == "empty override" || profile.policy == Strict10 {
					want := tokenSimpleContentLoc(t, "root.xsd", root, `<xs:attribute name="flag"`)
					if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != want || len(d.Related()) != 0 || d.SpecRef() != schemaSimpleContentSpecRef(profile.version) || !errors.Is(err, errSchemaSimpleContentBaseUnsupported) {
						t.Fatalf("override/Strict10 = %s", d)
					}
					return
				}
				want := tokenSimpleContentLoc(t, "root.xsd", root, `base="t:Named"`)
				related := []Loc{tokenSimpleContentLoc(t, "root.xsd", root, final.marker)}
				if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaSimpleTypeBaseCode || d.Loc() != want || !reflect.DeepEqual(d.Related(), related) || d.SpecRef() != schemaSimpleContentSpecRef(profile.version) || !errors.Is(err, errSchemaSimpleTypeInvalidDerivation) {
					t.Fatalf("final = %s", d)
				}
			})
		}
	}
}

//nolint:gocognit // Visibility, cycles, facet validation, and final overrides precede the owner gate.
func TestNormalizedStringSimpleContentGraphPrecedence(t *testing.T) {
	for _, profile := range tokenSimpleContentPolicies() {
		t.Run(profile.name+"/invisible", func(t *testing.T) {
			root := normalizedSimpleContentRoot(tokenSimpleContentOwner("o:Hidden", `<xs:attribute name="flag" type="xs:boolean"/>`))
			other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Hidden"><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:schema>`
			rootDoc := elementReferenceTestSyntaxDocument(t, "root.xsd", root)
			otherDoc := elementReferenceTestSyntaxDocument(t, "other.xsd", other)
			schema, err := newSchemaFromDiscoveryWithPolicy(syntaxDiscoveryResult{documents: []*syntaxDocument{rootDoc, otherDoc}}, profile.policy)
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			want := tokenSimpleContentLoc(t, "root.xsd", root, `base="o:Hidden"`)
			if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaSimpleContentBaseUnresolvedCode || d.Loc() != want || len(d.Related()) != 0 || d.SpecRef() != schemaSimpleContentSpecRef(profile.version) || !errors.Is(err, errSchemaSimpleContentBaseUnresolved) {
				t.Fatalf("invisible = %s", d)
			}
		})
		t.Run(profile.name+"/invalid facet before owner", func(t *testing.T) {
			root := normalizedSimpleContentRoot(tokenSimpleContentOwner("t:Bad", `<xs:attribute name="flag" type="xs:boolean"/>`) + `<xs:simpleType name="Bad"><xs:restriction base="xs:normalizedString"><xs:whiteSpace value="preserve"/></xs:restriction></xs:simpleType>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			want := tokenSimpleContentLoc(t, "root.xsd", root, `value="preserve"`)
			if d.Class() != FailureInvalid || d.Code() != InvalidStringWhiteSpaceRestrictionCode || d.Loc() != want || d.SpecRef() != stringWhiteSpaceSpecRef(profile.version) || !errors.Is(err, errInvalidStringWhiteSpaceRestriction) {
				t.Fatalf("invalid facet = %s", d)
			}
		})
		t.Run(profile.name+"/complex cycle before owner", func(t *testing.T) {
			root := normalizedSimpleContentRoot(tokenSimpleContentNamedOwner("Outer", "t:Inner", `<xs:attribute name="flag" type="xs:boolean"/>`) + tokenSimpleContentNamedOwner("Inner", "t:Outer", ""))
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			want := tokenSimpleContentLoc(t, "root.xsd", root, `base="t:Outer"`)
			related := []Loc{tokenSimpleContentLoc(t, "root.xsd", root, `base="t:Inner"`)}
			if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaSimpleContentBaseUnresolvedCode || d.Loc() != want || !reflect.DeepEqual(d.Related(), related) || d.SpecRef() != schemaSimpleContentSpecRef(profile.version) || !errors.Is(err, errSchemaComplexTypeBaseCycle) || !errors.Is(err, errSchemaSimpleContentBaseUnresolved) {
				t.Fatalf("cycle = %s", d)
			}
		})
		t.Run(profile.name+"/complex finalDefault before owner", func(t *testing.T) {
			root := strings.Replace(normalizedSimpleContentRoot(tokenSimpleContentNamedOwner("Outer", "t:Inner", `<xs:attribute name="flag" type="xs:boolean"/>`)+tokenSimpleContentNamedOwner("Inner", "xs:normalizedString", "")), `targetNamespace="urn:token"`, `targetNamespace="urn:token" finalDefault="extension"`, 1)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			want := tokenSimpleContentLoc(t, "root.xsd", root, `base="t:Inner"`)
			related := []Loc{tokenSimpleContentLoc(t, "root.xsd", root, `finalDefault="extension"`)}
			if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.Loc() != want || !reflect.DeepEqual(d.Related(), related) || d.SpecRef() != schemaComplexTypeExtensionSpecRef(profile.version) || !errors.Is(err, errSchemaComplexTypeBaseUnsupported) {
				t.Fatalf("complex finalDefault = %s", d)
			}
		})
		t.Run(profile.name+"/complex empty final override", func(t *testing.T) {
			inner := strings.Replace(tokenSimpleContentNamedOwner("Inner", "xs:normalizedString", ""), `name="Inner"`, `name="Inner" final=""`, 1)
			root := strings.Replace(normalizedSimpleContentRoot(tokenSimpleContentNamedOwner("Outer", "t:Inner", "")+inner), `targetNamespace="urn:token"`, `targetNamespace="urn:token" finalDefault="extension"`, 1)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			want := tokenSimpleContentLoc(t, "root.xsd", root, `base="t:Inner"`)
			related := []Loc{tokenSimpleContentLoc(t, "root.xsd", root, `<xs:complexType name="Inner"`)}
			if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != want || !reflect.DeepEqual(d.Related(), related) || d.SpecRef() != schemaSimpleContentSpecRef(profile.version) || !errors.Is(err, errSchemaSimpleContentBaseWrongKind) || !errors.Is(err, errSchemaSimpleContentBaseUnsupported) {
				t.Fatalf("complex override = %s", d)
			}
		})
	}
}

func TestNormalizedStringSimpleContentEmptyFinalOverride(t *testing.T) {
	for _, profile := range tokenSimpleContentPolicies() {
		t.Run(profile.name, func(t *testing.T) {
			named := `<xs:simpleType name="Named" final=""><xs:restriction base="xs:normalizedString"><xs:enumeration value=" a  b "/></xs:restriction></xs:simpleType>`
			root := strings.Replace(normalizedSimpleContentRoot(tokenSimpleContentOwner("t:Named", "")+named), `targetNamespace="urn:token"`, `targetNamespace="urn:token" finalDefault="extension"`, 1)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("empty final override: %v", err)
			}
			owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:token", "Value")
			ext, ok := owner.SimpleContentExtension()
			if !ok || owner.Particle() != nil {
				t.Fatalf("owner extension/particle = %t/%T", ok, owner.Particle())
			}
			ref, ok := ext.TypeReference()
			if !ok || !ref.IsNamed() || !reflect.DeepEqual(ref.StringEnumerationFacets().Values(), []string{" a  b "}) {
				t.Fatalf("effective named facts = %#v", ref)
			}
		})
	}
}
