package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func normalizedStringLocalRoot(version, body string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:n" targetNamespace="urn:n" attributeFormDefault="qualified" version="` + version + `">` + body + `</xs:schema>`
}

func normalizedStringLocalOwner(body string) string {
	return `<xs:complexType name="Box">` + body + `</xs:complexType>`
}

//nolint:gocognit // Each supported owner and policy must expose the same copied public facts.
func TestNormalizedStringLocalAttributeNamedOwnersExposeEffectiveFacts(t *testing.T) {
	shapes := []struct{ name, body string }{
		{"empty", `<xs:attribute name="built" type="xs:normalizedString"/><xs:attribute name="named" type="t:Child" use="required"/>`},
		{"choice", `<xs:choice><xs:element name="item" type="xs:integer"/></xs:choice><xs:attribute name="built" type="xs:normalizedString"/><xs:attribute name="named" type="t:Child" use="required"/>`},
		{"sequence", `<xs:sequence><xs:element name="item" type="xs:integer" minOccurs="2" maxOccurs="3"/></xs:sequence><xs:attribute name="built" type="xs:normalizedString"/><xs:attribute name="named" type="t:Child" use="required"/>`},
		{"grouped extension", `<xs:complexContent><xs:extension base="t:Base"><xs:group ref="t:Fields" minOccurs="0" maxOccurs="0"/><xs:attribute name="built" type="xs:normalizedString"/><xs:attribute name="named" type="t:Child" use="required"/></xs:extension></xs:complexContent>`},
		{"sequence extension", `<xs:complexContent><xs:extension base="t:Base"><xs:sequence><xs:element name="item" type="xs:integer"/></xs:sequence><xs:attribute name="built" type="xs:normalizedString"/><xs:attribute name="named" type="t:Child" use="required"/></xs:extension></xs:complexContent>`},
	}
	for _, profile := range tokenPolicyProfiles() {
		for _, shape := range shapes {
			t.Run(profile.name+"/"+shape.name, func(t *testing.T) {
				root := normalizedStringLocalRoot(string(profile.version), normalizedStringLocalOwner(shape.body)+`<xs:complexType name="Base"/><xs:group name="Fields"><xs:sequence><xs:element ref="t:field"/></xs:sequence></xs:group><xs:element name="field" type="xs:integer"/><xs:simpleType name="Child"><xs:restriction base="t:Parent"><xs:whiteSpace value="collapse"/><xs:enumeration value="one"/></xs:restriction></xs:simpleType><xs:simpleType name="Parent"><xs:restriction base="xs:normalizedString"><xs:enumeration value="one"/><xs:enumeration value="two"/></xs:restriction></xs:simpleType>`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("ParseSchema: %v", err)
				}
				found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:n", "Box"))
				if len(found) != 1 {
					t.Fatalf("Box components = %v", found)
				}
				definition := requireTestComplexTypeDefinition(t, found[0], "Box")
				uses := definition.AttributeUses()
				if len(uses) != 2 {
					t.Fatalf("ordered uses = %v", uses)
				}
				for index, lexical := range []string{"xs:normalizedString", "t:Child"} {
					use, ok := uses[index].(LocalAttributeUse)
					if !ok {
						t.Fatalf("use %d = %T", index, uses[index])
					}
					name := []string{"built", "named"}[index]
					if use.Name() != mustTestQName(t, "urn:n", name) || use.NameLoc() != elementReferenceTestAttributeLoc(t, root, `name="`+name+`"`) || use.TypeLoc() != elementReferenceTestAttributeLoc(t, root, `type="`+lexical+`"`) || use.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="`+name+`"`) {
						t.Fatalf("use %d name/type provenance = %#v", index, use)
					}
					reference, ok := use.TypeReference()
					if !ok || reference.Loc() != use.TypeLoc() || reference.Name() != use.DeclaredType() || reference.Variety() != SimpleTypeVarietyAtomicRestriction {
						t.Fatalf("use %d reference = %#v/%t", index, reference, ok)
					}
					white, hasWhite := reference.StringWhiteSpaceFacet()
					if !hasWhite {
						t.Fatalf("use %d has no effective whiteSpace", index)
					}
					if index == 0 {
						if white.Value() != "replace" || white.Fixed() || !white.Loc().IsZero() {
							t.Fatalf("built-in whiteSpace = %#v", white)
						}
						if !reference.IsBuiltin() || use.DeclaredType() != mustTestQName(t, testXSDNamespace, "normalizedString") || reference.StringEnumerationFacets().HasEnumeration() {
							t.Fatalf("builtin reference = %#v", reference)
						}
						if id, hasID := use.TypeID(); hasID || !id.IsZero() {
							t.Fatalf("builtin ID = %v/%t", id, hasID)
						}
						continue
					}
					if !reference.IsNamed() || use.DeclaredType() != mustTestQName(t, "urn:n", "Child") || use.Use() != AttributeUseRequired || use.UseLoc() != elementReferenceTestAttributeLoc(t, root, `use="required"`) {
						t.Fatalf("named use = %#v", use)
					}
					if white.Value() != "collapse" || white.Fixed() || white.Loc() != elementReferenceTestAttributeLoc(t, root, `value="collapse"`) {
						t.Fatalf("named whiteSpace = %#v", white)
					}
					id, hasID := use.TypeID()
					refID, refHasID := reference.ComponentID()
					if !hasID || !refHasID || id != refID || id != componentIDForName(t, schema, mustTestQName(t, "urn:n", "Child")) {
						t.Fatalf("named target ID = %v/%t, ref %v/%t", id, hasID, refID, refHasID)
					}
					assertStringEnumerationFacts(t, reference.StringEnumerationFacets(), profile.version, []string{"one"}, []Loc{elementReferenceTestAttributeLoc(t, root, `value="one"`)})
					values := reference.StringEnumerationFacets().Values()
					values[0] = "changed"
					locations := reference.StringEnumerationFacets().Locations()
					locations[0] = Loc{}
					if got := reference.StringEnumerationFacets().Values(); !reflect.DeepEqual(got, []string{"one"}) || reference.StringEnumerationFacets().Locations()[0].IsZero() {
						t.Fatalf("mutated enumeration view = %v", got)
					}
				}
				uses[0] = nil
				if definition.AttributeUses()[0].Name().Local() != "built" {
					t.Fatal("mutated ordered use view")
				}
				if shape.name == "grouped extension" && definition.Particle() != nil {
					t.Fatalf("0/0 group particle = %T", definition.Particle())
				}
				if shape.name == "sequence" {
					sequence, ok := definition.Particle().(SequenceParticle)
					if !ok || len(sequence.Particles()) != 1 || sequence.Particles()[0].Occurrences().String() != "2/3" {
						t.Fatalf("sequence bounds = %T/%v", definition.Particle(), sequence.Particles())
					}
				}
			})
		}
	}
}

func TestNormalizedStringLocalAttributeFacetFailuresStayLocated(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct {
			name, restriction, primary, related, code, spec string
			class                                           FailureClass
			cause                                           error
		}{
			{"enumeration narrowing", `<xs:simpleType name="Parent"><xs:restriction base="xs:normalizedString"><xs:enumeration value="a  b"/></xs:restriction></xs:simpleType><xs:simpleType name="Child"><xs:restriction base="t:Parent"><xs:enumeration value="a b"/></xs:restriction></xs:simpleType>`, `value="a b"`, `value="a  b"`, InvalidEnumerationRestrictionCode, enumerationSpecRef(profile.version, enumerationRestrictionRule), FailureInvalid, errInvalidEnumerationRestriction},
			{"whiteSpace widening", `<xs:simpleType name="Child"><xs:restriction base="xs:normalizedString"><xs:whiteSpace value="preserve"/></xs:restriction></xs:simpleType>`, `value="preserve"`, "", InvalidStringWhiteSpaceRestrictionCode, stringWhiteSpaceSpecRef(profile.version), FailureInvalid, errInvalidStringWhiteSpaceRestriction},
			{"length", `<xs:simpleType name="Child"><xs:restriction base="xs:normalizedString"><xs:length value="2"/></xs:restriction></xs:simpleType>`, `<xs:length`, "", UnsupportedDatatypeFacetCode, tokenDiagnosticSpecRef(profile.version, "decimal"), FailureUnsupported, ErrUnsupported},
			{"pattern", `<xs:simpleType name="Child"><xs:restriction base="xs:normalizedString"><xs:pattern value=".*"/></xs:restriction></xs:simpleType>`, `<xs:pattern`, "", UnsupportedDatatypeFacetCode, tokenDiagnosticSpecRef(profile.version, "decimal"), FailureUnsupported, ErrUnsupported},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := normalizedStringLocalRoot(string(profile.version), normalizedStringLocalOwner(`<xs:attribute name="a" type="t:Child"/>`)+test.restriction)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				var related []Loc
				if test.related != "" {
					related = []Loc{elementReferenceTestAttributeLoc(t, root, test.related)}
				}
				assertNCNameDiagnostic(t, schema, err, test.class, test.code, elementReferenceTestAttributeLoc(t, root, test.primary), related, test.spec, test.cause)
			})
		}
	}
}

func TestNormalizedStringLocalAttributeExcludedFormsStayLocated(t *testing.T) {
	tests := []struct {
		name, body, extra, primary, related string
		spec                                func(XSDVersion) string
		cause                               error
	}{
		{"inline restriction", normalizedStringLocalOwner(`<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:normalizedString"><xs:enumeration value="one"/></xs:restriction></xs:simpleType></xs:attribute>`), "", `<xs:simpleType`, "", schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnsupported},
		{"ref built-in", normalizedStringLocalOwner(`<xs:attribute ref="t:a"/>`), `<xs:attribute name="a" type="xs:normalizedString"/>`, `ref="t:a"`, `<xs:attribute name="a"`, schemaAttributeUseSpecRef, errSchemaAttributeReferenceUnsupported},
		{"ref named", normalizedStringLocalOwner(`<xs:attribute ref="t:a"/>`), `<xs:attribute name="a" type="t:N"/><xs:simpleType name="N"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`, `ref="t:a"`, `<xs:attribute name="a"`, schemaAttributeUseSpecRef, errSchemaAttributeReferenceUnsupported},
		{"ref inline", normalizedStringLocalOwner(`<xs:attribute ref="t:a"/>`), `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:attribute>`, `ref="t:a"`, `<xs:attribute name="a"`, schemaAttributeUseSpecRef, errSchemaAttributeReferenceUnsupported},
		{"default built-in", normalizedStringLocalOwner(`<xs:attribute name="a" type="xs:normalizedString" default="one"/>`), "", `default="one"`, "", schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported},
		{"fixed named", normalizedStringLocalOwner(`<xs:attribute name="a" type="t:N" fixed="one"/>`), `<xs:simpleType name="N"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`, `fixed="one"`, "", schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported},
		{"default named", normalizedStringLocalOwner(`<xs:attribute name="a" type="t:N" default="one"/>`), `<xs:simpleType name="N"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`, `default="one"`, "", schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported},
		{"fixed built-in", normalizedStringLocalOwner(`<xs:attribute name="a" type="xs:normalizedString" fixed="one"/>`), "", `fixed="one"`, "", schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported},
		{"default inline", normalizedStringLocalOwner(`<xs:attribute name="a" default="one"><xs:simpleType><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:attribute>`), "", `default="one"`, "", schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported},
		{"fixed inline", normalizedStringLocalOwner(`<xs:attribute name="a" fixed="one"><xs:simpleType><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:attribute>`), "", `fixed="one"`, "", schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported},
		{"named list", normalizedStringLocalOwner(`<xs:attribute name="a" type="t:L"/>`), `<xs:simpleType name="L"><xs:list itemType="xs:normalizedString"/></xs:simpleType>`, `type="t:L"`, "", schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnsupported},
		{"named union", normalizedStringLocalOwner(`<xs:attribute name="a" type="t:U"/>`), `<xs:simpleType name="U"><xs:union memberTypes="xs:normalizedString"/></xs:simpleType>`, `type="t:U"`, "", schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnsupported},
		{"inline list", normalizedStringLocalOwner(`<xs:attribute name="a"><xs:simpleType><xs:list itemType="xs:normalizedString"/></xs:simpleType></xs:attribute>`), "", `<xs:simpleType`, "", schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnsupported},
		{"inline union", normalizedStringLocalOwner(`<xs:attribute name="a"><xs:simpleType><xs:union memberTypes="xs:normalizedString"/></xs:simpleType></xs:attribute>`), "", `<xs:simpleType`, "", schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnsupported},
		{"direct all", normalizedStringLocalOwner(`<xs:all><xs:element name="item" type="xs:integer"/></xs:all><xs:attribute name="a" type="xs:normalizedString"/>`), "", `type="xs:normalizedString"`, "", schemaAttributeTypeSpecRef, errSchemaAttributeUseUnsupported},
		{"direct all named", normalizedStringLocalOwner(`<xs:all><xs:element name="item" type="xs:integer"/></xs:all><xs:attribute name="a" type="t:N"/>`), `<xs:simpleType name="N"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`, `type="t:N"`, "", schemaAttributeTypeSpecRef, errSchemaAttributeUseUnsupported},
		{"direct all inline", normalizedStringLocalOwner(`<xs:all><xs:element name="item" type="xs:integer"/></xs:all><xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:attribute>`), "", `<xs:simpleType`, "", schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnsupported},
		{"direct all ref", normalizedStringLocalOwner(`<xs:all><xs:element name="item" type="xs:integer"/></xs:all><xs:attribute ref="t:a"/>`), `<xs:attribute name="a" type="xs:normalizedString"/>`, `ref="t:a"`, `<xs:attribute name="a"`, schemaAttributeUseSpecRef, errSchemaAttributeReferenceUnsupported},
		{"direct group", normalizedStringLocalOwner(`<xs:group ref="t:Fields"/><xs:attribute name="a" type="xs:normalizedString"/>`), `<xs:group name="Fields"><xs:choice><xs:element ref="t:item"/></xs:choice></xs:group><xs:element name="item" type="xs:integer"/>`, `type="xs:normalizedString"`, "", schemaAttributeTypeSpecRef, errSchemaAttributeUseUnsupported},
		{"direct group named", normalizedStringLocalOwner(`<xs:group ref="t:Fields"/><xs:attribute name="a" type="t:N"/>`), `<xs:group name="Fields"><xs:choice><xs:element ref="t:item"/></xs:choice></xs:group><xs:element name="item" type="xs:integer"/><xs:simpleType name="N"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`, `type="t:N"`, "", schemaAttributeTypeSpecRef, errSchemaAttributeUseUnsupported},
		{"direct group inline", normalizedStringLocalOwner(`<xs:group ref="t:Fields"/><xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:attribute>`), `<xs:group name="Fields"><xs:choice><xs:element ref="t:item"/></xs:choice></xs:group><xs:element name="item" type="xs:integer"/>`, `<xs:simpleType`, "", schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnsupported},
		{"direct group ref", normalizedStringLocalOwner(`<xs:group ref="t:Fields"/><xs:attribute ref="t:a"/>`), `<xs:group name="Fields"><xs:choice><xs:element ref="t:item"/></xs:choice></xs:group><xs:element name="item" type="xs:integer"/><xs:attribute name="a" type="xs:normalizedString"/>`, `ref="t:a"`, `<xs:attribute name="a"`, schemaAttributeUseSpecRef, errSchemaAttributeReferenceUnsupported},
		{"simple content", normalizedStringLocalOwner(`<xs:simpleContent><xs:extension base="xs:string"><xs:attribute name="a" type="xs:normalizedString"/></xs:extension></xs:simpleContent>`), "", `type="xs:normalizedString"`, "", schemaAttributeTypeSpecRef, errSchemaAttributeUseUnsupported},
		{"simple content named", normalizedStringLocalOwner(`<xs:simpleContent><xs:extension base="xs:string"><xs:attribute name="a" type="t:N"/></xs:extension></xs:simpleContent>`), `<xs:simpleType name="N"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`, `type="t:N"`, "", schemaAttributeTypeSpecRef, errSchemaAttributeUseUnsupported},
		{"simple content inline", normalizedStringLocalOwner(`<xs:simpleContent><xs:extension base="xs:string"><xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:attribute></xs:extension></xs:simpleContent>`), "", `<xs:simpleType`, "", schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnsupported},
		{"simple content ref", normalizedStringLocalOwner(`<xs:simpleContent><xs:extension base="xs:string"><xs:attribute ref="t:a"/></xs:extension></xs:simpleContent>`), `<xs:attribute name="a" type="xs:normalizedString"/>`, `ref="t:a"`, `<xs:attribute name="a"`, schemaAttributeUseSpecRef, errSchemaAttributeReferenceUnsupported},
		{"inline complex owner", `<xs:element name="root"><xs:complexType><xs:attribute name="a" type="xs:normalizedString"/></xs:complexType></xs:element>`, "", `type="xs:normalizedString"`, "", schemaAttributeTypeSpecRef, errSchemaAttributeUseUnsupported},
		{"inline complex owner named", `<xs:element name="root"><xs:complexType><xs:attribute name="a" type="t:N"/></xs:complexType></xs:element>`, `<xs:simpleType name="N"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`, `type="t:N"`, "", schemaAttributeTypeSpecRef, errSchemaAttributeUseUnsupported},
		{"inline complex owner inline", `<xs:element name="root"><xs:complexType><xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:attribute></xs:complexType></xs:element>`, "", `<xs:simpleType`, "", schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnsupported},
		{"inline complex owner ref", `<xs:element name="root"><xs:complexType><xs:attribute ref="t:a"/></xs:complexType></xs:element>`, `<xs:attribute name="a" type="xs:normalizedString"/>`, `ref="t:a"`, `<xs:attribute name="a"`, schemaAttributeUseSpecRef, errSchemaAttributeReferenceUnsupported},
	}
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range tests {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := normalizedStringLocalRoot(string(profile.version), test.body+test.extra)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				var related []Loc
				if test.related != "" {
					related = []Loc{elementReferenceTestAttributeLoc(t, root, test.related)}
				}
				assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, test.primary), related, test.spec(profile.version), test.cause)
			})
		}
	}
}

//nolint:gocognit // Every admitted owner keeps each excluded local form at its authored location.
func TestNormalizedStringLocalAttributeExcludedFormsAcrossNamedOwners(t *testing.T) {
	shapes := []struct{ name, before, after string }{
		{"empty", "", ""},
		{"choice", `<xs:choice><xs:element name="item" type="xs:integer"/></xs:choice>`, ""},
		{"sequence", `<xs:sequence><xs:element name="item" type="xs:integer"/></xs:sequence>`, ""},
		{"grouped extension", `<xs:complexContent><xs:extension base="t:Base"><xs:group ref="t:Fields" minOccurs="0" maxOccurs="0"/>`, `</xs:extension></xs:complexContent>`},
		{"sequence extension", `<xs:complexContent><xs:extension base="t:Base"><xs:sequence><xs:element name="item" type="xs:integer"/></xs:sequence>`, `</xs:extension></xs:complexContent>`},
	}
	base := `<xs:complexType name="Base"/><xs:group name="Fields"><xs:sequence><xs:element ref="t:field"/></xs:sequence></xs:group><xs:element name="field" type="xs:integer"/>`
	for _, profile := range tokenPolicyProfiles() {
		for _, shape := range shapes {
			for _, form := range []struct {
				name, use, extra, primary, related string
				spec                               func(XSDVersion) string
				cause                              error
			}{
				{"inline", `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, "", schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnsupported},
				{"ref built-in", `<xs:attribute ref="t:a"/>`, `<xs:attribute name="a" type="xs:normalizedString"/>`, `ref="t:a"`, `<xs:attribute name="a"`, schemaAttributeUseSpecRef, errSchemaAttributeReferenceUnsupported},
				{"ref named", `<xs:attribute ref="t:a"/>`, `<xs:attribute name="a" type="t:N"/><xs:simpleType name="N"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`, `ref="t:a"`, `<xs:attribute name="a"`, schemaAttributeUseSpecRef, errSchemaAttributeReferenceUnsupported},
				{"ref inline", `<xs:attribute ref="t:a"/>`, `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:attribute>`, `ref="t:a"`, `<xs:attribute name="a"`, schemaAttributeUseSpecRef, errSchemaAttributeReferenceUnsupported},
				{"default built-in", `<xs:attribute name="a" type="xs:normalizedString" default="one"/>`, "", `default="one"`, "", schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported},
				{"fixed built-in", `<xs:attribute name="a" type="xs:normalizedString" fixed="one"/>`, "", `fixed="one"`, "", schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported},
				{"default named", `<xs:attribute name="a" type="t:N" default="one"/>`, `<xs:simpleType name="N"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`, `default="one"`, "", schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported},
				{"fixed named", `<xs:attribute name="a" type="t:N" fixed="one"/>`, `<xs:simpleType name="N"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`, `fixed="one"`, "", schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported},
				{"default inline", `<xs:attribute name="a" default="one"><xs:simpleType><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:attribute>`, "", `default="one"`, "", schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported},
				{"fixed inline", `<xs:attribute name="a" fixed="one"><xs:simpleType><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:attribute>`, "", `fixed="one"`, "", schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported},
			} {
				t.Run(profile.name+"/"+shape.name+"/"+form.name, func(t *testing.T) {
					root := normalizedStringLocalRoot(string(profile.version), normalizedStringLocalOwner(shape.before+form.use+shape.after)+base+form.extra)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					var related []Loc
					if form.related != "" {
						related = []Loc{elementReferenceTestAttributeLoc(t, root, form.related)}
					}
					spec := form.spec(profile.version)
					if (shape.name == "grouped extension" || shape.name == "sequence extension") && (strings.HasPrefix(form.name, "default") || strings.HasPrefix(form.name, "fixed")) {
						spec = schemaComplexTypeExtensionSpecRef(profile.version)
					}
					assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, form.primary), related, spec, form.cause)
				})
			}
		}
	}
}

func TestNormalizedStringLocalAttributeExcludesUnsupportedOtherStringKinds(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"string", "QName"} {
			for _, named := range []bool{false, true} {
				name := "built-in"
				typeName := "xs:" + kind
				extra := ""
				if named {
					name = "named"
					typeName = "t:N"
					extra = `<xs:simpleType name="N"><xs:restriction base="xs:` + kind + `"/></xs:simpleType>`
				}
				t.Run(profile.name+"/"+kind+"/"+name, func(t *testing.T) {
					root := normalizedStringLocalRoot(string(profile.version), normalizedStringLocalOwner(`<xs:attribute name="a" type="`+typeName+`"/>`)+extra)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, `type="`+typeName+`"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnsupported)
				})
			}
		}
	}
}

func TestNormalizedStringLocalAttributeInvisibleImportAndCyclesFailBeforePublication(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		root := normalizedStringLocalRoot(string(profile.version), `<xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/>`+normalizedStringLocalOwner(`<xs:attribute name="a" type="o:Hidden"/>`))
		root = strings.Replace(root, `xmlns:t="urn:n"`, `xmlns:t="urn:n" xmlns:o="urn:other"`, 1)
		fixtures := map[string]discoveryFixture{
			"bridge.xsd": {id: "bridge.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge"><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`},
			"other.xsd":  {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Hidden"><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:schema>`},
		}
		schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
		assertNCNameDiagnostic(t, schema, err, FailureInvalid, diagnosticSchemaAttributeTypeUnresolvedCode, elementReferenceTestAttributeLoc(t, root, `type="o:Hidden"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved)
		root = normalizedStringLocalRoot(string(profile.version), normalizedStringLocalOwner(`<xs:attribute name="a" type="t:First"/>`)+`<xs:simpleType name="First"><xs:restriction base="t:Second"/></xs:simpleType><xs:simpleType name="Second"><xs:restriction base="t:First"/></xs:simpleType>`)
		schema, err = discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
		related := []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="a"`), elementReferenceTestAttributeLoc(t, root, `base="t:Second"`), elementReferenceTestAttributeLoc(t, root, `base="t:First"`), elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="First"`), elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="Second"`)}
		assertNCNameDiagnostic(t, schema, err, FailureInvalid, diagnosticSchemaAttributeTypeCycleCode, elementReferenceTestAttributeLoc(t, root, `type="t:First"`), related, schemaAttributeTypeSpecRef(profile.version), errSchemaSimpleTypeBaseCycle)
	}
}

func TestNormalizedStringLocalAttributeDoesNotAcquireIDCardinality(t *testing.T) {
	root := normalizedStringLocalRoot("1.0", normalizedStringLocalOwner(`<xs:attribute name="first" type="xs:normalizedString"/><xs:attribute name="id" type="xs:ID"/><xs:attribute name="second" type="t:N"/>`)+`<xs:simpleType name="N"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	if err != nil {
		t.Fatalf("NormalizedString and one ID: %v", err)
	}
	definition := requireTestComplexTypeDefinition(t, schema.Components()[0], "Box")
	if len(definition.AttributeUses()) != 3 {
		t.Fatalf("uses = %v", definition.AttributeUses())
	}
	root = normalizedStringLocalRoot("1.0", normalizedStringLocalOwner(`<xs:attribute name="first" type="xs:normalizedString"/><xs:attribute name="id" type="xs:ID"/><xs:attribute name="second" type="xs:ID"/>`))
	schema, err = discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	assertNCNameDiagnostic(t, schema, err, FailureInvalid, diagnosticSchemaAttributeUseIDDuplicateCode, elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="second"`), []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="id"`)}, "xsd10-structures#cos-ct-props-correct", errSchemaAttributeUseIDDuplicate)
}

//nolint:gocognit // Each admitted owner has two independent unsupported consumers.
func TestNormalizedStringLocalAttributeConsumersRemainUnsupported(t *testing.T) {
	for _, shape := range []struct{ name, body string }{
		{"empty", `<xs:attribute name="a" type="xs:normalizedString"/>`},
		{"choice", `<xs:choice><xs:element name="item" type="xs:integer"/></xs:choice><xs:attribute name="a" type="xs:normalizedString"/>`},
		{"sequence", `<xs:sequence><xs:element name="item" type="xs:integer"/></xs:sequence><xs:attribute name="a" type="xs:normalizedString"/>`},
		{"grouped extension", `<xs:complexContent><xs:extension base="t:Base"><xs:group ref="t:Fields" minOccurs="0" maxOccurs="0"/><xs:attribute name="a" type="xs:normalizedString"/></xs:extension></xs:complexContent>`},
		{"sequence extension", `<xs:complexContent><xs:extension base="t:Base"><xs:sequence><xs:element name="item" type="xs:integer"/></xs:sequence><xs:attribute name="a" type="xs:normalizedString"/></xs:extension></xs:complexContent>`},
	} {
		for _, typeName := range []string{"xs:normalizedString", "t:N"} {
			t.Run(shape.name+"/"+typeName, func(t *testing.T) {
				body := strings.Replace(shape.body, "xs:normalizedString", typeName, 1)
				root := normalizedStringLocalRoot("1.1", normalizedStringLocalOwner(body)+`<xs:element name="root" type="t:Box"/><xs:complexType name="Base"/><xs:group name="Fields"><xs:sequence><xs:element ref="t:field"/></xs:sequence></xs:group><xs:element name="field" type="xs:integer"/><xs:simpleType name="N"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
				if err != nil {
					t.Fatalf("query schema: %v", err)
				}
				useLoc := elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="a"`)
				instance := `<root xmlns="urn:n" a="one"/>`
				validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(instance)))
				validation := requireDiagnostic(t, validationErr)
				validationCause := errInstanceAttributes
				if shape.name == "sequence extension" {
					validationCause = errInstanceComplexContentExtension
				}
				if validation.Class() != FailureUnsupported || validation.Code() != UnsupportedInstanceValidationCode || validation.SpecRef() != "xsd11-structures#cvc-elt" || !errors.Is(validationErr, ErrUnsupported) || !errors.Is(validationErr, validationCause) {
					t.Fatalf("validation = %s / %v", validation, validationErr)
				}
				wantValidationLoc := useLoc
				if shape.name == "sequence extension" {
					wantValidationLoc = mustTestLoc(t, "instance.xml", 1, 1)
				}
				if validation.Loc() != wantValidationLoc {
					t.Fatalf("validation Loc = %v, want %v", validation.Loc(), wantValidationLoc)
				}
				output, generationErr := GenerateGo(schema, "generated")
				generation := requireDiagnostic(t, generationErr)
				if output != nil || generation.Class() != FailureUnsupported || generation.Code() != diagnosticCodegenUnsupported || generation.Loc() != useLoc || !errors.Is(generationErr, ErrUnsupported) {
					t.Fatalf("generation = %q / %s / %v", output, generation, generationErr)
				}
			})
		}
	}
}

func TestNormalizedStringLocalAttributeProhibitedStillResolvesType(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		root := normalizedStringLocalRoot(string(profile.version), normalizedStringLocalOwner(`<xs:attribute name="gone" type="xs:normalizedString" use="prohibited"/>`))
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
		if err != nil || len(requireTestComplexTypeDefinition(t, schema.Components()[0], "Box").AttributeUses()) != 0 {
			t.Fatalf("%s: prohibited use = %v / %v", profile.name, schema.Components(), err)
		}
		for _, exit := range []struct {
			name, typeName, extra, code, primary, related, spec string
			cause                                               error
		}{
			{"unresolved", "t:Missing", "", diagnosticSchemaAttributeTypeUnresolvedCode, `type="t:Missing"`, "", schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved},
			{"wrong kind", "t:Wrong", `<xs:complexType name="Wrong"/>`, diagnosticSchemaAttributeTypeWrongKindCode, `type="t:Wrong"`, `<xs:complexType name="Wrong"`, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeWrongKind},
			{"invalid enumeration", "t:Bad", `<xs:simpleType name="Base"><xs:restriction base="xs:normalizedString"><xs:enumeration value="one"/></xs:restriction></xs:simpleType><xs:simpleType name="Bad"><xs:restriction base="t:Base"><xs:enumeration value="other"/></xs:restriction></xs:simpleType>`, InvalidEnumerationRestrictionCode, `value="other"`, `value="one"`, enumerationSpecRef(profile.version, enumerationRestrictionRule), errInvalidEnumerationRestriction},
			{"whiteSpace widening", "t:Bad", `<xs:simpleType name="Bad"><xs:restriction base="xs:normalizedString"><xs:whiteSpace value="preserve"/></xs:restriction></xs:simpleType>`, InvalidStringWhiteSpaceRestrictionCode, `value="preserve"`, "", stringWhiteSpaceSpecRef(profile.version), errInvalidStringWhiteSpaceRestriction},
		} {
			t.Run(profile.name+"/"+exit.name, func(t *testing.T) {
				root := normalizedStringLocalRoot(string(profile.version), normalizedStringLocalOwner(`<xs:attribute name="gone" type="`+exit.typeName+`" use="prohibited"/>`)+exit.extra)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				var related []Loc
				if exit.related != "" {
					related = []Loc{elementReferenceTestAttributeLoc(t, root, exit.related)}
				}
				assertNCNameDiagnostic(t, schema, err, FailureInvalid, exit.code, elementReferenceTestAttributeLoc(t, root, exit.primary), related, exit.spec, exit.cause)
			})
		}
	}
}

//nolint:gocognit // Distinct source graphs expose whitespace, enumeration, and identity together.
func TestNormalizedStringLocalAttributeGraphFacts(t *testing.T) {
	root := normalizedStringLocalRoot("1.1", `<xs:include schemaLocation="part.xsd"/><xs:include schemaLocation="part.xsd"/><xs:include schemaLocation="chameleon.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/>`+normalizedStringLocalOwner(`<xs:attribute name="built" type="xs:normalizedString"/><xs:attribute name="forward" type="t:Forward"/><xs:attribute name="included" type="t:Included" form="unqualified"/><xs:attribute name="adopted" type="t:Adopted"/><xs:attribute name="foreign" type="o:Foreign"/>`)+`<xs:simpleType name="Forward"><xs:restriction base="xs:normalizedString"><xs:whiteSpace value="collapse"/><xs:enumeration value="a b"/></xs:restriction></xs:simpleType>`)
	root = strings.Replace(root, `xmlns:t="urn:n"`, `xmlns:t="urn:n" xmlns:o="urn:other"`, 1)
	part := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:n"><xs:simpleType name="Included"><xs:restriction base="xs:normalizedString"><xs:enumeration value=" a  b "/></xs:restriction></xs:simpleType></xs:schema>`
	chameleon := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Adopted"><xs:restriction base="xs:normalizedString"><xs:enumeration value="adopt"/></xs:restriction></xs:simpleType></xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Foreign"><xs:restriction base="xs:normalizedString"><xs:enumeration value="remote"/></xs:restriction></xs:simpleType></xs:schema>`
	fixtures := map[string]discoveryFixture{"part.xsd": {id: "part.xsd", contents: part}, "chameleon.xsd": {id: "chameleon.xsd", contents: chameleon}, "other.xsd": {id: "other.xsd", contents: other}}
	want := []struct {
		local, lexical, namespace, source, white string
		values                                   []string
		facetLoc                                 Loc
	}{
		{"built", "xs:normalizedString", "urn:n", "", "replace", nil, Loc{}},
		{"forward", "t:Forward", "urn:n", "root.xsd", "collapse", []string{"a b"}, elementReferenceTestAttributeLoc(t, root, `value="a b"`)},
		{"included", "t:Included", "", "part.xsd", "replace", []string{" a  b "}, mustSchemaTokenLoc(t, "part.xsd", part, 1, `value=" a  b "`)},
		{"adopted", "t:Adopted", "urn:n", "chameleon.xsd", "replace", []string{"adopt"}, mustSchemaTokenLoc(t, "chameleon.xsd", chameleon, 1, `value="adopt"`)},
		{"foreign", "o:Foreign", "urn:n", "other.xsd", "replace", []string{"remote"}, mustSchemaTokenLoc(t, "other.xsd", other, 1, `value="remote"`)},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy)
			if err != nil {
				t.Fatal(err)
			}
			found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:n", "Box"))
			uses := requireTestComplexTypeDefinition(t, found[0], "Box").AttributeUses()
			if len(uses) != len(want) || len(schema.Documents()) != 4 {
				t.Fatalf("uses/documents = %d/%d", len(uses), len(schema.Documents()))
			}
			for index, expected := range want {
				use, ok := uses[index].(LocalAttributeUse)
				if !ok {
					t.Fatalf("use %d = %T", index, uses[index])
				}
				ref, ok := use.TypeReference()
				if !ok || use.Name() != mustTestQName(t, expected.namespace, expected.local) || use.TypeLoc() != elementReferenceTestAttributeLoc(t, root, `type="`+expected.lexical+`"`) || ref.Name() != use.DeclaredType() || ref.Loc() != use.TypeLoc() {
					t.Fatalf("use %d = %#v, reference %#v", index, use, ref)
				}
				white, ok := ref.StringWhiteSpaceFacet()
				if !ok || white.Value() != expected.white {
					t.Fatalf("use %d whitespace = %#v", index, white)
				}
				if index == 0 {
					id, hasID := use.TypeID()
					if !ref.IsBuiltin() || hasID || !id.IsZero() || !white.Loc().IsZero() {
						t.Fatalf("built-in facts = %#v/%v", ref, id)
					}
					continue
				}
				id, hasID := use.TypeID()
				if !ref.IsNamed() || !hasID || id.Source() != SourceID(expected.source) || id != componentIDForName(t, schema, ref.Name()) {
					t.Fatalf("use %d named ID = %v", index, id)
				}
				facets := ref.StringEnumerationFacets()
				version := XSDVersion11
				if policy == Strict10 {
					version = XSDVersion10
				}
				assertStringEnumerationFacts(t, facets, version, expected.values, []Loc{expected.facetLoc})
				values := facets.Values()
				values[0] = "changed"
				if ref.StringEnumerationFacets().Values()[0] != expected.values[0] {
					t.Fatal("facet copy changed schema")
				}
			}
			again, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy)
			if err != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
				t.Fatalf("graph order/facts changed: %v", err)
			}
		})
	}
}

//nolint:gocognit // Both document labels must expose inherited facts under each selected policy.
func TestNormalizedStringLocalAttributeInheritedFacetsAcrossEditionLabels(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, label := range []string{"1.0", "1.1"} {
			root := normalizedStringLocalRoot(label, normalizedStringLocalOwner(`<xs:attribute name="a" type="t:Child"/>`)+`<xs:simpleType name="Child"><xs:restriction base="t:Parent"/></xs:simpleType><xs:simpleType name="Parent"><xs:restriction base="xs:normalizedString"><xs:enumeration value=" a  b "/></xs:restriction></xs:simpleType>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("%s label %s: %v", profile.name, label, err)
			}
			use, ok := requireTestComplexTypeDefinition(t, schema.Components()[0], "Box").AttributeUses()[0].(LocalAttributeUse)
			if !ok {
				t.Fatal("inherited use is not local")
			}
			ref, hasReference := use.TypeReference()
			if !hasReference {
				t.Fatal("inherited use has no type reference")
			}
			white, ok := ref.StringWhiteSpaceFacet()
			if !ok || white.Value() != "replace" || !white.Loc().IsZero() {
				t.Fatalf("%s label %s inherited whitespace = %#v", profile.name, label, white)
			}
			assertStringEnumerationFacts(t, ref.StringEnumerationFacets(), profile.version, []string{" a  b "}, []Loc{elementReferenceTestAttributeLoc(t, root, `value=" a  b "`)})
		}
	}
}

func TestNormalizedStringLocalAttributeResolverCause(t *testing.T) {
	root := normalizedStringLocalRoot("1.1", `<xs:include schemaLocation="missing.xsd"/>`+normalizedStringLocalOwner(`<xs:attribute name="a" type="t:Missing"/>`))
	source, err := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("normalizedString type source unavailable")
	schema, err := ParseSchemaWithPolicy(source, &discoveryResolver{failures: map[string]error{"missing.xsd": cause}}, Strict11)
	assertNCNameDiagnostic(t, schema, err, FailureResolution, SourceResolveCode, elementReferenceTestAttributeLoc(t, root, `<xs:include`), nil, "", cause)
}

//nolint:gocognit // Malformed lexical forms and duplicate resolution share one diagnostic matrix.
func TestNormalizedStringLocalAttributeMalformedUses(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, malformed := range []struct {
			name, use, extra, primary, code, related, spec string
			cause                                          error
		}{
			{"bad name", `<xs:attribute name="a:b" type="xs:normalizedString"/>`, "", `name="a:b"`, invalidSchemaCompositionCode, "", schemaAttributeUseSpecRef(profile.version), nil},
			{"bad use", `<xs:attribute name="a" type="xs:normalizedString" use="sometimes"/>`, "", `use="sometimes"`, invalidSchemaCompositionCode, "", schemaAttributeUseSpecRef(profile.version), nil},
			{"bad form", `<xs:attribute name="a" type="xs:normalizedString" form="local"/>`, "", `form="local"`, invalidSchemaCompositionCode, "", schemaAttributeUseSpecRef(profile.version), nil},
			{"name and ref", `<xs:attribute name="a" ref="t:global" type="xs:normalizedString"/>`, `<xs:attribute name="global" type="xs:normalizedString"/>`, `ref="t:global"`, invalidSchemaCompositionCode, "", schemaAttributeUseSpecRef(profile.version), nil},
			{"type and inline", `<xs:attribute name="a" type="xs:normalizedString"><xs:simpleType><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, invalidSchemaCompositionCode, "", schemaAttributeUseSpecRef(profile.version), nil},
			{"duplicate", `<xs:attribute name="a" type="xs:normalizedString"/><xs:attribute name="a" type="t:N"/>`, `<xs:simpleType name="N"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`, `<xs:attribute name="a" type="t:N"`, diagnosticSchemaAttributeUseDuplicateCode, `<xs:attribute name="a"`, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseDuplicate},
		} {
			t.Run(profile.name+"/"+malformed.name, func(t *testing.T) {
				root := normalizedStringLocalRoot(string(profile.version), normalizedStringLocalOwner(malformed.use)+malformed.extra)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil {
					t.Fatalf("malformed use returned schema: %v", err)
				}
				diagnostic := requireDiagnostic(t, err)
				var related []Loc
				if malformed.related != "" {
					related = []Loc{elementReferenceTestAttributeLoc(t, root, malformed.related)}
				}
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != malformed.code || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, malformed.primary) || diagnostic.SpecRef() != malformed.spec || !reflect.DeepEqual(diagnostic.Related(), related) || malformed.cause != nil && !errors.Is(err, malformed.cause) {
					t.Fatalf("malformed %s = %v", malformed.name, err)
				}
			})
		}
	}
}
