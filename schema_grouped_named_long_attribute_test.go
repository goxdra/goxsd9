package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

const groupedNamedLongTypes = `<xs:simpleType name="Narrow"><xs:restriction base="t:Wide"><xs:minInclusive value="-3"/><xs:maxInclusive value="4"/><xs:enumeration value="2"/></xs:restriction></xs:simpleType><xs:simpleType name="Wide"><xs:restriction base="xs:long"><xs:minInclusive value="-100"/><xs:maxInclusive value="100"/><xs:totalDigits value="3"/></xs:restriction></xs:simpleType>`

func TestGroupedFacetFreeNamedLong(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		root := groupedExtensionSchema(string(profile.version), "", `<xs:attribute name="named" type="t:Alias" use="required"/>`)
		root = strings.Replace(root, `</xs:schema>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:long"/></xs:simpleType></xs:schema>`, 1)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
		if err != nil {
			t.Fatal(err)
		}
		owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Derived")
		uses := owner.AttributeUses()
		if len(uses) != 1 {
			t.Fatalf("uses = %v", uses)
		}
		use, ok := uses[0].(LocalAttributeUse)
		if !ok || use.Use() != AttributeUseRequired {
			t.Fatalf("required named use = %v", uses[0])
		}
		ref, ok := use.TypeReference()
		if !ok || !ref.IsNamed() || ref.QName() != mustTestQName(t, "urn:root", "Alias") {
			t.Fatalf("named type = %#v", ref)
		}
		assertAllIntegerBounds(t, ref, profile.version, "-9223372036854775808", "9223372036854775807", Loc{}, Loc{})
	}
}

//nolint:gocognit // One public view ties the grouped owner to completed named type facts.
func TestGroupedNamedLongPublicFacts(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			uses := `<xs:attribute name="builtin" type="xs:long" use="required"/><xs:attribute name="named" type="t:Narrow" use="optional" form="unqualified"/><xs:attribute name="middle" type="xs:integer"/><xs:attribute name="gone" type="t:Narrow" use="prohibited"/>`
			root := groupedExtensionSchema(string(profile.version), ` minOccurs="2" maxOccurs="3"`, uses)
			root = strings.Replace(root, `</xs:schema>`, groupedNamedLongTypes+`</xs:schema>`, 1)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			again, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
				t.Fatalf("components vary: %v", err)
			}
			owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Derived")
			group, groupOK := owner.Particle().(ModelGroupReferenceParticle)
			base, baseOK := owner.BaseReference()
			if !groupOK || group.RefLoc() != complexContentTestLoc(t, root, `ref="t:Fields"`) || group.Occurrences().String() != "2/3" || !baseOK || base.Loc() != complexContentTestLoc(t, root, `base="t:Base"`) {
				t.Fatalf("group/base = %#v/%#v", group, base)
			}
			usesOut := owner.AttributeUses()
			if len(usesOut) != 3 || usesOut[0].Name().Local() != "builtin" || usesOut[1].Name().Local() != "named" || usesOut[2].Name().Local() != "middle" {
				t.Fatalf("ordered uses = %#v", usesOut)
			}
			use, ok := usesOut[1].(LocalAttributeUse)
			if !ok || use.Name() != mustTestQName(t, "", "named") || use.Loc() != complexContentTestLoc(t, root, `<xs:attribute name="named"`) || use.NameLoc() != complexContentTestLoc(t, root, `name="named"`) || use.TypeLoc() != complexContentTestLoc(t, root, `type="t:Narrow"`) || use.Use() != AttributeUseOptional || use.UseLoc() != complexContentTestLoc(t, root, `use="optional"`) || use.FormLoc() != complexContentTestLoc(t, root, `form="unqualified"`) {
				t.Fatalf("named use = %#v", usesOut[1])
			}
			ref, refOK := use.TypeReference()
			id, hasID := use.TypeID()
			refID, hasRefID := ref.ComponentID()
			if !refOK || !ref.IsNamed() || !hasID || !hasRefID || id != refID || id != componentIDForName(t, schema, mustTestQName(t, "urn:root", "Narrow")) || use.DeclaredType() != mustTestQName(t, "urn:root", "Narrow") || ref.QName() != use.DeclaredType() || ref.Loc() != use.TypeLoc() || ref.VarietyLoc() != complexContentTestLoc(t, root, `<xs:restriction base="t:Wide"`) {
				t.Fatalf("named type = %#v/%v/%v", ref, id, refID)
			}
			assertAllIntegerBounds(t, ref, profile.version, "-3", "4", complexContentTestLoc(t, root, `value="-3"`), complexContentTestLoc(t, root, `value="4"`))
			definitionComponent := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:root", "Narrow"))
			if len(definitionComponent) != 1 {
				t.Fatalf("Narrow components = %v", definitionComponent)
			}
			definition, ok := definitionComponent[0].SimpleTypeDefinition()
			if !ok {
				t.Fatal("Narrow has no simple type view")
			}
			digits, hasDigits := definition.DigitFacets().TotalDigits()
			digitLoc, hasDigitLoc := definition.DigitFacets().TotalDigitsLoc()
			enumeration := definition.IntegerEnumerationFacets()
			values, locs := enumeration.Values(), enumeration.Locations()
			if !hasDigits || digits.Canonical() != "3" || !hasDigitLoc || digitLoc != complexContentTestLoc(t, root, `value="3"`) || len(values) != 1 || values[0].Canonical() != "2" || !reflect.DeepEqual(locs, []Loc{complexContentTestLoc(t, root, `<xs:enumeration value="2"`)}) {
				t.Fatalf("named facets = %v/%v at %v/%v", digits, values, digitLoc, locs)
			}
			values[0].value.SetInt64(99)
			locs[0] = Loc{}
			bounds, _ := ref.IntegerBounds()
			for _, bound := range bounds.Bounds() {
				bound.Value().value.SetInt64(0)
			}
			usesOut[1] = nil
			if owner.AttributeUses()[1] == nil || definition.IntegerEnumerationFacets().Values()[0].Canonical() != "2" {
				t.Fatal("public copies changed the schema")
			}
			fresh, freshOK := owner.AttributeUses()[1].(LocalAttributeUse)
			if !freshOK {
				t.Fatalf("fresh use = %T", owner.AttributeUses()[1])
			}
			freshRef, freshHasRef := fresh.TypeReference()
			if !freshHasRef {
				t.Fatal("fresh use lost type reference")
			}
			assertAllIntegerBounds(t, freshRef, profile.version, "-3", "4", complexContentTestLoc(t, root, `value="-3"`), complexContentTestLoc(t, root, `value="4"`))
			var walked []ComponentID
			if err := schema.Walk(func(component Component) error { walked = append(walked, component.ID()); return nil }); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(walked, componentIDs(schema.Components())) {
				t.Fatalf("walk order = %v", walked)
			}
		})
	}
}

func TestGroupedNamedLongConsumers(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		root := groupedExtensionSchema(string(profile.version), "", `<xs:attribute name="named" type="t:Narrow"/>`)
		root = strings.Replace(root, `</xs:schema>`, groupedNamedLongTypes+`</xs:schema>`, 1)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
		if err != nil {
			t.Fatal(err)
		}
		useLoc := complexContentTestLoc(t, root, `<xs:attribute name="named"`)
		ownerLoc := complexContentTestLoc(t, root, `<xs:complexType name="Derived"`)
		rootLoc := complexContentTestLoc(t, root, `<xs:element name="root"`)
		instanceErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
		vd := requireDiagnostic(t, instanceErr)
		if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != useLoc || vd.SpecRef() != instanceValidationSpecRef(profile.version) || !reflect.DeepEqual(vd.Related(), []Loc{rootLoc, ownerLoc, useLoc}) || !errors.Is(instanceErr, errInstanceAttributes) {
			t.Fatalf("validation = %v", instanceErr)
		}
		output, generationErr := GenerateGo(schema, "generated")
		gd := requireDiagnostic(t, generationErr)
		if output != nil || gd.Class() != FailureUnsupported || gd.Code() != diagnosticCodegenUnsupported || gd.Loc() != useLoc || gd.SpecRef() != codegenDirectSequenceSpecReference(profile.version, codegenDirectSequenceParticlesReference) || !reflect.DeepEqual(gd.Related(), []Loc{ownerLoc, useLoc}) || !errors.Is(generationErr, errCodegenUnsupported) {
			t.Fatalf("generation = %v/%v", output, generationErr)
		}
	}
}

//nolint:gocognit // Each graph tests the same named use through public identities and locations.
func TestGroupedNamedLongGraphVisibility(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, graph := range []string{"forward", "included", "imported", "chameleon"} {
			t.Run(profile.name+"/"+graph, func(t *testing.T) {
				lexical, typeNS, sourceID := "r:Long", "urn:root", SourceID("root.xsd")
				baseLexical := "r:Base"
				directive, target := "", ` targetNamespace="urn:root"`
				if graph == "included" || graph == "chameleon" {
					directive = `<xs:include schemaLocation="dependency.xsd"/>`
					sourceID = "dependency.xsd"
				}
				if graph == "imported" {
					directive = `<xs:import namespace="urn:other" schemaLocation="dependency.xsd"/>`
					lexical, typeNS, sourceID, target = "o:Long", "urn:other", "dependency.xsd", ` targetNamespace="urn:other"`
					baseLexical = "o:Base"
				}
				if graph == "chameleon" {
					target = ""
				}
				declarations := `<xs:complexType name="Base"/><xs:simpleType name="Long"><xs:restriction base="xs:long"><xs:minInclusive value="-7"/></xs:restriction></xs:simpleType>`
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="` + string(profile.version) + `">` + directive + `<xs:element name="root" type="r:Derived"/><xs:complexType name="Derived"><xs:complexContent><xs:extension base="` + baseLexical + `"><xs:group ref="r:Fields" minOccurs="1" maxOccurs="2"/><xs:attribute name="named" type="` + lexical + `"/></xs:extension></xs:complexContent></xs:complexType><xs:group name="Fields"><xs:sequence/></xs:group>`
				fixtures := map[string]discoveryFixture{}
				contents := root + declarations + `</xs:schema>`
				if graph == "forward" {
					root = contents
				}
				if graph != "forward" {
					root += `</xs:schema>`
					contents = `<xs:schema xmlns:xs="` + testXSDNamespace + `"` + target + ` version="` + string(profile.version) + `">` + declarations + `</xs:schema>`
					fixtures["dependency.xsd"] = discoveryFixture{id: "dependency.xsd", contents: contents}
				}
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Derived")
				group, groupOK := owner.Particle().(ModelGroupReferenceParticle)
				base, baseOK := owner.BaseReference()
				baseID, baseHasID := base.ComponentID()
				use, useOK := owner.AttributeUses()[0].(LocalAttributeUse)
				ref, refOK := use.TypeReference()
				id, hasID := use.TypeID()
				if !groupOK || group.Occurrences().String() != "1/2" || group.RefLoc() != complexContentTestLoc(t, root, `ref="r:Fields"`) || !baseOK || !baseHasID || baseID.Source() != sourceID || !useOK || !refOK || !hasID || id.Source() != sourceID || id != componentIDForName(t, schema, mustTestQName(t, typeNS, "Long")) || use.DeclaredType() != mustTestQName(t, typeNS, "Long") || ref.Loc() != complexContentTestLoc(t, root, `type="`+lexical+`"`) || ref.VarietyLoc() != complexContentTestSourceLoc(t, sourceID, contents, `<xs:restriction base="xs:long"`) {
					t.Fatalf("graph facts = %#v/%#v/%#v", group, base, use)
				}
				assertAllIntegerBounds(t, ref, profile.version, "-7", "9223372036854775807", complexContentTestSourceLoc(t, sourceID, contents, `value="-7"`), Loc{})
			})
		}
	}
}

func TestGroupedNamedLongAlternateExits(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct {
			name, typeName, declaration, marker, related, code string
			class                                              FailureClass
			spec                                               func(XSDVersion) string
			cause                                              error
		}{
			{"missing type", "t:Missing", "", `type="t:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, FailureInvalid, schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnresolved},
			{"wrong type kind", "t:Base", "", `type="t:Base"`, `<xs:complexType name="Base"`, diagnosticSchemaAttributeTypeWrongKindCode, FailureInvalid, schemaAttributeTypeSpecRef, errSchemaAttributeTypeWrongKind},
			{"cycle", "t:First", `<xs:simpleType name="First"><xs:restriction base="t:Second"/></xs:simpleType><xs:simpleType name="Second"><xs:restriction base="t:First"/></xs:simpleType>`, `type="t:First"`, `<xs:attribute name="named"|base="t:Second"|base="t:First"|<xs:simpleType name="First"|<xs:simpleType name="Second"`, diagnosticSchemaAttributeTypeCycleCode, FailureInvalid, schemaAttributeTypeSpecRef, errSchemaSimpleTypeBaseCycle},
			{"malformed bound", "t:Bad", `<xs:simpleType name="Bad"><xs:restriction base="xs:long"><xs:minInclusive value="bad"/></xs:restriction></xs:simpleType>`, `value="bad"`, "", InvalidBoundCode, FailureInvalid, func(v XSDVersion) string { return boundSpecRef(v, BoundMinInclusive, boundDefinitionRule) }, errInvalidBoundValue},
			{"unsupported facet", "t:Bad", `<xs:simpleType name="Bad"><xs:restriction base="xs:long"><xs:pattern value="[0-9]+"/></xs:restriction></xs:simpleType>`, `<xs:pattern`, "", UnsupportedDatatypeFacetCode, FailureUnsupported, func(v XSDVersion) string { return tokenDiagnosticSpecRef(v, "decimal") }, ErrUnsupported},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := groupedExtensionSchema(string(profile.version), "", `<xs:attribute name="named" type="`+test.typeName+`"/>`)
				root = strings.Replace(root, `</xs:schema>`, test.declaration+`</xs:schema>`, 1)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				var related []Loc
				if test.related != "" {
					for _, marker := range strings.Split(test.related, "|") {
						related = append(related, complexContentTestLoc(t, root, marker))
					}
				}
				assertNCNameDiagnostic(t, schema, err, test.class, test.code, complexContentTestLoc(t, root, test.marker), related, test.spec(profile.version), test.cause)
			})
		}
	}
}

func TestGroupedNamedLongInvisibleType(t *testing.T) {
	bridge := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge"><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`
	foreign := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Long"><xs:restriction base="xs:long"/></xs:simpleType></xs:schema>`
	fixtures := map[string]discoveryFixture{"bridge.xsd": {id: "bridge.xsd", contents: bridge}, "other.xsd": {id: "other.xsd", contents: foreign}}
	for _, profile := range tokenPolicyProfiles() {
		root := groupedExtensionSchema(string(profile.version), "", `<xs:attribute name="named" type="o:Long"/>`)
		root = strings.Replace(root, `xmlns:t="urn:root"`, `xmlns:t="urn:root" xmlns:o="urn:other"`, 1)
		root = strings.Replace(root, `<xs:element name="root"`, `<xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/><xs:element name="root"`, 1)
		schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
		assertNCNameDiagnostic(t, schema, err, FailureInvalid, diagnosticSchemaAttributeTypeUnresolvedCode, complexContentTestLoc(t, root, `type="o:Long"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved)
	}
}

func TestGroupedNamedLongOmissionAndBaseExits(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		zero := groupedExtensionSchema(string(profile.version), ` minOccurs="0" maxOccurs="0"`, `<xs:attribute name="named" type="t:Narrow"/><xs:attribute name="gone" type="t:Narrow" use="prohibited"/>`)
		zero = strings.Replace(zero, `</xs:schema>`, groupedNamedLongTypes+`</xs:schema>`, 1)
		schema, err := discoverTestSchemaWithPolicy(t, zero, nil, profile.policy)
		if err != nil {
			t.Fatal(err)
		}
		owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Derived")
		if owner.Particle() != nil || len(owner.AttributeUses()) != 1 || owner.AttributeUses()[0].Name().Local() != "named" {
			t.Fatalf("omitted facts = %#v/%#v", owner.Particle(), owner.AttributeUses())
		}
		testGroupedLongExits(t, profile, zero, []groupedLongExit{
			{"group before named use", `ref="t:Fields"`, `ref="t:Missing"`, `ref="t:Missing"`, "", diagnosticSchemaModelGroupReferenceUnresolvedCode, FailureInvalid, errSchemaModelGroupReferenceUnresolved, schemaModelGroupReferenceSpecRef},
			{"prohibited named use before base", `type="t:Narrow" use="prohibited"`, `type="t:Missing" use="prohibited"`, `type="t:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, FailureInvalid, errSchemaAttributeTypeUnresolved, schemaAttributeTypeSpecRef},
			{"missing base", `base="t:Base"`, `base="t:Missing"`, `base="t:Missing"`, "", invalidSchemaCompositionCode, FailureInvalid, errSchemaComplexTypeBaseUnresolved, schemaComplexTypeExtensionSpecRef},
			{"wrong base kind", `base="t:Base"`, `base="t:Narrow"`, `base="t:Narrow"`, `<xs:simpleType name="Narrow"`, invalidSchemaCompositionCode, FailureInvalid, errSchemaComplexTypeBaseWrongKind, schemaComplexTypeExtensionSpecRef},
			{"base final", `<xs:complexType name="Base"/>`, `<xs:complexType name="Base" final="extension"/>`, `base="t:Base"`, `final="extension"`, invalidSchemaCompositionCode, FailureInvalid, errSchemaComplexTypeBaseUnsupported, schemaComplexTypeExtensionSpecRef},
			{"base uses", `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:attribute name="inherited" type="xs:boolean"/></xs:complexType>`, `base="t:Base"`, `<xs:complexType name="Base"`, UnsupportedSchemaSyntaxCode, FailureUnsupported, errSchemaComplexTypeBaseUnsupported, schemaComplexTypeExtensionSpecRef},
			{"nonempty base", `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:sequence><xs:element ref="t:root"/></xs:sequence></xs:complexType>`, `base="t:Base"`, `<xs:complexType name="Base"`, UnsupportedSchemaSyntaxCode, FailureUnsupported, errSchemaComplexTypeBaseNonEmpty, schemaComplexTypeExtensionSpecRef},
		})
	}
}

type groupedLongExit struct {
	name, from, to, marker, related, code string
	class                                 FailureClass
	cause                                 error
	spec                                  func(XSDVersion) string
}

func testGroupedLongExits(t *testing.T, profile tokenPolicyProfile, source string, cases []groupedLongExit) {
	t.Helper()
	for _, test := range cases {
		t.Run(profile.name+"/"+test.name, func(t *testing.T) {
			root := strings.Replace(source, test.from, test.to, 1)
			failed, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			var related []Loc
			if test.related != "" {
				related = []Loc{complexContentTestLoc(t, root, test.related)}
			}
			if test.name == "nonempty base" {
				related = append(related, allParticleTestTokenLoc(t, "root.xsd", root, `<xs:sequence`, 2))
			}
			assertNCNameDiagnostic(t, failed, err, test.class, test.code, complexContentTestLoc(t, root, test.marker), related, test.spec(profile.version), test.cause)
		})
	}
}
