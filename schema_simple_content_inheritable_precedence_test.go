package goxsd9

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func excludedSimpleContentSchema(named bool, base, use, declarations string) string {
	content := `<xs:simpleContent><xs:extension base="` + base + `">` + use + `</xs:extension></xs:simpleContent>`
	owner := `<xs:element name="root"><xs:complexType>` + content + `</xs:complexType></xs:element>`
	if named {
		owner = `<xs:element name="root" type="t:Record"/><xs:complexType name="Record">` + content + `</xs:complexType>`
	}
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" version="1.0">` + owner + declarations + `</xs:schema>`
}

//nolint:gocognit // Pair every applicable base/use exit with omission across both excluded owners and policies.
func TestSimpleContentFalseInheritableDependencyPrecedence(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, named := range []bool{false, true} {
			for _, spelling := range []string{"false", "0", " &#x9;false&#xA; ", " &#xA;0&#x9; "} {
				for _, tc := range []struct {
					name, base, use, declarations, marker, related, code, spec string
					class                                                      FailureClass
					cause                                                      error
				}{
					{"base before type", "t:Missing", `<xs:attribute name="a" type="t:MissingType"/>`, "", `base="t:Missing"`, "", diagnosticSchemaSimpleContentBaseUnresolvedCode, schemaSimpleContentSpecRef(XSDVersion11), FailureInvalid, errSchemaSimpleContentBaseUnresolved},
					{"wrong kind base", "t:Wrong", `<xs:attribute name="a" type="xs:boolean"/>`, `<xs:complexType name="Wrong"/>`, `base="t:Wrong"`, `<xs:complexType name="Wrong"`, diagnosticSchemaSimpleContentBaseWrongKindCode, schemaSimpleContentSpecRef(XSDVersion11), FailureInvalid, errSchemaSimpleContentBaseWrongKind},
					{"missing type", "xs:string", `<xs:attribute name="a" type="t:Missing"/>`, "", `type="t:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(XSDVersion11), FailureInvalid, errSchemaAttributeTypeUnresolved},
					{"prohibited missing type", "xs:string", `<xs:attribute name="a" type="t:Missing" use="prohibited"/>`, "", `type="t:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(XSDVersion11), FailureInvalid, errSchemaAttributeTypeUnresolved},
					{"wrong kind type", "xs:string", `<xs:attribute name="a" type="t:Wrong"/>`, `<xs:complexType name="Wrong"/>`, `type="t:Wrong"`, `<xs:complexType name="Wrong"`, diagnosticSchemaAttributeTypeWrongKindCode, schemaAttributeTypeSpecRef(XSDVersion11), FailureInvalid, errSchemaAttributeTypeWrongKind},
					{"unsupported ID", "xs:string", `<xs:attribute name="a" type="xs:ID"/>`, "", `type="xs:ID"`, "", UnsupportedSchemaSyntaxCode, schemaAttributeTypeSpecRef(XSDVersion11), FailureUnsupported, errSchemaAttributeUseUnsupported},
					{"duplicate use", "xs:string", `<xs:attribute name="a" type="xs:boolean"/><xs:attribute name="a" type="xs:integer"/>`, "", `<xs:attribute name="a" type="xs:integer"`, `<xs:attribute name="a" type="xs:boolean"`, diagnosticSchemaAttributeUseDuplicateCode, schemaAttributeUseSpecRef(XSDVersion11), FailureInvalid, errSchemaAttributeUseDuplicate},
				} {
					t.Run(string(policy)+"/"+map[bool]string{false: "anonymous", true: "named"}[named]+"/"+spelling+"/"+tc.name, func(t *testing.T) {
						use := strings.Replace(tc.use, `/>`, ` inheritable="`+spelling+`"/>`, 1)
						root := excludedSimpleContentSchema(named, tc.base, use, tc.declarations)
						for _, candidate := range []string{root, strings.Replace(root, ` inheritable="`+spelling+`"`, "", 1)} {
							if tc.related == "" {
								assertInlineInheritableDiagnostic(t, candidate, policy, tc.marker, tc.class, tc.code, tc.spec, tc.cause)
								continue
							}
							assertInlineInheritableDiagnostic(t, candidate, policy, tc.marker, tc.class, tc.code, tc.spec, tc.cause, tc.related)
						}
					})
				}
			}
		}
	}
}

//nolint:gocognit // Valid and prohibited uses must resolve before the excluded owner refusal.
func TestSimpleContentFalseInheritableValidOwnerRefusal(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, named := range []bool{false, true} {
			for _, value := range []string{"false", "0", " &#x9;false&#xA; ", " &#xA;0&#x9; "} {
				for _, prohibited := range []bool{false, true} {
					use := `<xs:attribute name="a" type="xs:boolean" inheritable="` + value + `"/>`
					if prohibited {
						use = `<xs:attribute name="a" type="xs:boolean" use="prohibited" inheritable="` + value + `"/>`
					}
					root := excludedSimpleContentSchema(named, "xs:string", use, "")
					assertInlineInheritableDiagnostic(t, root, policy, `inheritable="`+value+`"`, FailureUnsupported, UnsupportedSchemaSyntaxCode, "xsd11-structures#cSchemaDocument", ErrUnsupported)
					without := strings.Replace(root, ` inheritable="`+value+`"`, "", 1)
					if _, err := discoverTestSchemaWithPolicy(t, without, nil, policy); err != nil {
						t.Fatalf("simpleContent omission: %v", err)
					}
				}
			}
		}
	}
}

func TestSimpleContentInheritableSyntaxPrecedence(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, named := range []bool{false, true} {
			for _, value := range []string{"true", "1"} {
				root := excludedSimpleContentSchema(named, "t:Missing", `<xs:attribute name="a" type="t:MissingType" inheritable="`+value+`"/>`, "")
				assertInlineInheritableDiagnostic(t, root, policy, `inheritable="`+value+`"`, FailureUnsupported, UnsupportedSchemaSyntaxCode, "xsd11-structures#cSchemaDocument", ErrUnsupported)
			}
			root := excludedSimpleContentSchema(named, "t:Missing", `<xs:attribute ref="t:missing" inheritable="false"/>`, "")
			assertInlineInheritableDiagnostic(t, root, policy, `inheritable="false"`, FailureUnsupported, UnsupportedSchemaSyntaxCode, "xsd11-structures#cSchemaDocument", ErrUnsupported)
			root = excludedSimpleContentSchema(named, "t:Missing", `<xs:attribute name="a" type="xs:boolean" inheritable="maybe"/>`, "")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			var lexical Diagnostic
			if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.Loc() != complexContentTestLoc(t, root, `inheritable="maybe"`) || d.SpecRef() != strictBooleanSpecRef(XSDVersion11) || len(d.Related()) != 0 || !errors.As(errors.Unwrap(d), &lexical) || lexical.Code() != InvalidBooleanLexicalCode || lexical.Loc() != d.Loc() || lexical.SpecRef() != d.SpecRef() {
				t.Fatalf("malformed inheritable = %v", err)
			}
		}
	}
	for _, named := range []bool{false, true} {
		root := excludedSimpleContentSchema(named, "t:Missing", `<xs:attribute name="a" type="t:MissingType" inheritable="0"/>`, "")
		assertInlineInheritableDiagnostic(t, root, Strict10, `inheritable="0"`, FailureUnsupported, UnsupportedSchemaSyntaxCode, "xsd11-structures#cSchemaDocument", errLanguagePolicyMismatch)
	}
}

//nolint:gocognit // Pin both owner forms against every authored simple-type cycle location.
func TestSimpleContentFalseInheritableGraphExits(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, named := range []bool{false, true} {
			root := excludedSimpleContentSchema(named, "t:A", `<xs:attribute name="a" type="xs:boolean" inheritable="false"/>`, `<xs:simpleType name="A"><xs:restriction base="t:B"/></xs:simpleType><xs:simpleType name="B"><xs:restriction base="t:A"/></xs:simpleType>`)
			for _, candidate := range []string{root, strings.Replace(root, ` inheritable="false"`, "", 1)} {
				schema, err := discoverTestSchemaWithPolicy(t, candidate, nil, policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaSimpleContentBaseUnresolvedCode || d.Loc() != complexContentTestLoc(t, candidate, `base="t:A"`) || d.SpecRef() != schemaSimpleContentSpecRef(XSDVersion11) || !errors.Is(err, errSchemaSimpleTypeBaseCycle) || !errors.Is(err, errSchemaSimpleContentBaseUnresolved) || len(d.Related()) == 0 {
					t.Fatalf("simple base cycle = %v related=%v", err, d.Related())
				}
				ownerMarker := `<xs:element name="root"`
				if named {
					ownerMarker = `<xs:complexType name="Record"`
				}
				wantRelated := []Loc{
					complexContentTestLoc(t, candidate, ownerMarker),
					complexContentTestLoc(t, candidate, `base="t:B"`),
					allParticleTestTokenLoc(t, "root.xsd", candidate, `base="t:A"`, 2),
					complexContentTestLoc(t, candidate, `<xs:simpleType name="A"`),
					complexContentTestLoc(t, candidate, `<xs:simpleType name="B"`),
				}
				if !reflect.DeepEqual(d.Related(), wantRelated) {
					t.Fatalf("cycle related = %v, want %v", d.Related(), wantRelated)
				}
			}
		}
	}
}

func TestNamedSimpleContentFalseInheritableComplexBaseCycle(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		root := excludedSimpleContentSchema(true, "t:Other", `<xs:attribute name="a" type="xs:boolean" inheritable="false"/>`, `<xs:complexType name="Other"><xs:simpleContent><xs:extension base="t:Record"/></xs:simpleContent></xs:complexType>`)
		for _, candidate := range []string{root, strings.Replace(root, ` inheritable="false"`, "", 1)} {
			assertInlineInheritableDiagnostic(t, candidate, policy, `base="t:Record"`, FailureInvalid, diagnosticSchemaSimpleContentBaseUnresolvedCode, schemaSimpleContentSpecRef(XSDVersion11), errSchemaComplexTypeBaseCycle, `base="t:Other"`)
		}
	}
}

//nolint:gocognit // Test hidden base and type targets with authored cross-source related locations.
func TestSimpleContentFalseInheritableGraphVisibility(t *testing.T) {
	bridge := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge"><xs:import namespace="urn:hidden" schemaLocation="hidden.xsd"/></xs:schema>`
	hidden := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:hidden"><xs:simpleType name="Hidden"><xs:restriction base="xs:boolean"/></xs:simpleType></xs:schema>`
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, named := range []bool{false, true} {
			for _, tc := range []struct {
				name, base, use, marker, code, spec string
				cause                               error
			}{
				{"hidden base", "h:Hidden", `<xs:attribute name="a" type="xs:boolean"/>`, `base="h:Hidden"`, diagnosticSchemaSimpleContentBaseUnresolvedCode, schemaSimpleContentSpecRef(XSDVersion11), errSchemaSimpleContentBaseUnresolved},
				{"hidden type", "xs:string", `<xs:attribute name="a" type="h:Hidden"/>`, `type="h:Hidden"`, diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(XSDVersion11), errSchemaAttributeTypeUnresolved},
			} {
				root := excludedSimpleContentSchema(named, tc.base, strings.Replace(tc.use, `/>`, ` inheritable="false"/>`, 1), "")
				root = strings.Replace(root, `xmlns:t="urn:root"`, `xmlns:t="urn:root" xmlns:h="urn:hidden"`, 1)
				root = strings.Replace(root, `<xs:element name="root"`, `<xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/><xs:element name="root"`, 1)
				for _, candidate := range []string{root, strings.Replace(root, ` inheritable="false"`, "", 1)} {
					schema, err := discoverTestSchemaWithPolicy(t, candidate, map[string]discoveryFixture{
						"bridge.xsd": {id: "bridge.xsd", contents: bridge}, "hidden.xsd": {id: "hidden.xsd", contents: hidden},
					}, policy)
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					var wantRelated []Loc
					if d.Class() != FailureInvalid || d.Code() != tc.code || d.Loc() != complexContentTestLoc(t, candidate, tc.marker) || d.SpecRef() != tc.spec || !errors.Is(err, tc.cause) || !reflect.DeepEqual(d.Related(), wantRelated) {
						t.Fatalf("%s = %v related=%v, want %v", tc.name, err, d.Related(), wantRelated)
					}
				}
				visible := strings.Replace(root, `<xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/>`, `<xs:import namespace="urn:hidden" schemaLocation="hidden.xsd"/>`, 1)
				schema, err := discoverTestSchemaWithPolicy(t, visible, map[string]discoveryFixture{"hidden.xsd": {id: "hidden.xsd", contents: hidden}}, policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != complexContentTestLoc(t, visible, `inheritable="false"`) || d.SpecRef() != "xsd11-structures#cSchemaDocument" || len(d.Related()) != 0 || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("visible %s refusal = %v", tc.name, err)
				}
				without := strings.Replace(visible, ` inheritable="false"`, "", 1)
				if _, err := discoverTestSchemaWithPolicy(t, without, map[string]discoveryFixture{"hidden.xsd": {id: "hidden.xsd", contents: hidden}}, policy); err != nil {
					t.Fatalf("visible %s: %v", tc.name, err)
				}
			}
		}
	}
}

func TestSimpleContentFalseInheritableStructuralBaseControl(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, named := range []bool{false, true} {
			root := excludedSimpleContentSchema(named, "xs:token", `<xs:attribute name="a" type="t:Missing" inheritable="false"/>`, "")
			for _, candidate := range []string{root, strings.Replace(root, ` inheritable="false"`, "", 1)} {
				schema, err := discoverTestSchemaWithPolicy(t, candidate, nil, policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				marker := `<xs:simpleContent>`
				if named {
					marker = `<xs:attribute name="a"`
				}
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != complexContentTestLoc(t, candidate, marker) || d.SpecRef() != schemaSimpleContentSpecRef(XSDVersion11) || !errors.Is(err, errSchemaSimpleContentBaseUnsupported) || len(d.Related()) != 0 {
					t.Fatalf("structural base = %v", err)
				}
			}
		}
	}
}

func TestFalseInheritableIndependentStructuralRoutes(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, tc := range []struct {
			name, body, declarations, withMarker, withoutMarker, withSpec, withoutSpec string
		}{
			{"direct group", `<xs:group ref="t:Fields"/><xs:attribute name="a" type="xs:boolean"@/>`, `<xs:group name="Fields"><xs:sequence/></xs:group>`, `ref="t:Fields"`, `ref="t:Fields"`, schemaSyntaxSpecRefForVersion(XSDVersion11), schemaSyntaxSpecRefForVersion(XSDVersion11)},
			{"direct all", `<xs:all/><xs:attribute name="a" type="xs:boolean"@/>`, "", `<xs:all/>`, `<xs:all/>`, schemaSyntaxSpecRefForVersion(XSDVersion11), schemaSyntaxSpecRefForVersion(XSDVersion11)},
			{"complex extension choice", `<xs:complexContent><xs:extension base="xs:anyType"><xs:choice/><xs:attribute name="a" type="xs:boolean"@/></xs:extension></xs:complexContent>`, "", `inheritable="false"`, `<xs:attribute name="a"`, schemaComplexTypeExtensionSpecRef(XSDVersion11), schemaComplexTypeExtensionSpecRef(XSDVersion11)},
			{"complex extension no model", `<xs:complexContent><xs:extension base="xs:anyType"><xs:attribute name="a" type="xs:boolean"@/></xs:extension></xs:complexContent>`, "", `inheritable="false"`, `<xs:attribute name="a"`, schemaComplexContentExtensionSpecRef(XSDVersion11), schemaComplexContentExtensionSpecRef(XSDVersion11)},
			{"complex extension nested sequence", `<xs:complexContent><xs:extension base="xs:anyType"><xs:sequence><xs:sequence><xs:element name="v" type="xs:integer"/></xs:sequence></xs:sequence><xs:attribute name="a" type="xs:boolean"@/></xs:extension></xs:complexContent>`, "", `<xs:element name="v"`, `<xs:element name="v"`, schemaComplexParticleExtensionSpecRef(XSDVersion11), schemaComplexParticleExtensionSpecRef(XSDVersion11)},
			{"complex extension nested choice", `<xs:complexContent><xs:extension base="xs:anyType"><xs:sequence><xs:choice><xs:element name="v" type="xs:integer"/></xs:choice></xs:sequence><xs:attribute name="a" type="xs:boolean"@/></xs:extension></xs:complexContent>`, "", `<xs:element name="v"`, `<xs:element name="v"`, schemaComplexParticleExtensionSpecRef(XSDVersion11), schemaComplexParticleExtensionSpecRef(XSDVersion11)},
			{"complex restriction use", `<xs:complexContent><xs:restriction base="xs:anyType"><xs:attribute name="a" type="xs:boolean"@/></xs:restriction></xs:complexContent>`, "", `inheritable="false"`, `<xs:attribute name="a"`, "xsd11-structures#cSchemaDocument", "xsd10-structures#schema-document"},
			{"simple restriction use", `<xs:simpleContent><xs:restriction base="xs:string"><xs:attribute name="a" type="xs:boolean"@/></xs:restriction></xs:simpleContent>`, "", `inheritable="false"`, `<xs:restriction`, "xsd11-structures#cSchemaDocument", "xsd10-structures#schema-document"},
			{"local child inline simple content", `<xs:sequence><xs:element name="child"><xs:complexType><xs:simpleContent><xs:extension base="xs:string"><xs:attribute name="a" type="xs:boolean"@/></xs:extension></xs:simpleContent></xs:complexType></xs:element></xs:sequence>`, "", `<xs:complexType><xs:simpleContent>`, `<xs:complexType><xs:simpleContent>`, "xsd10-structures#schema-document", "xsd10-structures#schema-document"},
		} {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" version="1.0"><xs:element name="root"><xs:complexType>` + tc.body + `</xs:complexType></xs:element>` + tc.declarations + `</xs:schema>`
			for _, state := range []struct{ value, marker, spec string }{
				{` inheritable="false"`, tc.withMarker, tc.withSpec}, {"", tc.withoutMarker, tc.withoutSpec},
			} {
				t.Run(string(policy)+"/"+tc.name+"/"+state.value, func(t *testing.T) {
					candidate := strings.Replace(root, "@", state.value, 1)
					assertInlineInheritableDiagnostic(t, candidate, policy, state.marker, FailureUnsupported, UnsupportedSchemaSyntaxCode, state.spec, ErrUnsupported)
				})
			}
		}
	}
}

func TestFalseInheritableAttributeGroupHasNoBuildRoute(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root" version="1.0"><xs:attributeGroup name="Attrs"><xs:attribute name="a" type="t:Missing" inheritable="false"/></xs:attributeGroup></xs:schema>`
		root = strings.Replace(root, `targetNamespace="urn:root"`, `xmlns:t="urn:root" targetNamespace="urn:root"`, 1)
		assertInlineInheritableDiagnostic(t, root, policy, `inheritable="false"`, FailureUnsupported, UnsupportedSchemaSyntaxCode, "xsd11-structures#cSchemaDocument", ErrUnsupported)
		without := strings.Replace(root, ` inheritable="false"`, "", 1)
		schema, err := discoverTestSchemaWithPolicy(t, without, nil, policy)
		if err != nil {
			t.Fatalf("attributeGroup placeholder: %v", err)
		}
		components := schema.Components()
		if len(components) != 1 || components[0].Kind() != ComponentKindAttributeGroupDefinition {
			t.Fatalf("attributeGroup walk = %v", components)
		}
	}
}
