package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit // Keep graph order, inline ownership, and copied facet facts together.
func TestNCNameEnumerationGraphAndInlineAttributeFacts(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:n" xmlns:o="urn:o" targetNamespace="urn:n" version="` + string(profile.version) + `">
  <xs:include schemaLocation="part.xsd"/>
  <xs:import namespace="urn:o" schemaLocation="other.xsd"/>
  <xs:attribute name="space"><xs:simpleType><xs:restriction base="xs:NCName"><xs:enumeration value=" default "/><xs:enumeration value="preserve"/></xs:restriction></xs:simpleType></xs:attribute>
  <xs:simpleType name="Forward"><xs:restriction base="t:Later"><xs:enumeration value=" _alpha "/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Later"><xs:restriction base="xs:NCName"><xs:enumeration value="_alpha"/><xs:enumeration value="é·x"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Inherited"><xs:restriction base="t:Later"/></xs:simpleType>
  <xs:simpleType name="IncludedChild"><xs:restriction base="t:Included"><xs:enumeration value=" from_include "/></xs:restriction></xs:simpleType>
  <xs:simpleType name="ImportedChild"><xs:restriction base="o:Imported"><xs:enumeration value="remote"/></xs:restriction></xs:simpleType>
</xs:schema>`
			part := `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="` + string(profile.version) + `"><xs:simpleType name="Included"><xs:restriction base="xs:NCName"><xs:enumeration value="from_include"/></xs:restriction></xs:simpleType></xs:schema>`
			other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:o" version="` + string(profile.version) + `"><xs:simpleType name="Imported"><xs:restriction base="xs:NCName"><xs:enumeration value="remote"/></xs:restriction></xs:simpleType></xs:schema>`
			fixtures := map[string]discoveryFixture{"part.xsd": {id: "part.xsd", contents: part}, "other.xsd": {id: "other.xsd", contents: other}}
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("NCName graph: %v", err)
			}
			again, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
				t.Fatalf("repeated graph = %v, equal %t", err, reflect.DeepEqual(schema.Components(), again.Components()))
			}
			components := schema.Components()
			want := []QName{mustTestQName(t, "urn:n", "space"), mustTestQName(t, "urn:n", "Forward"), mustTestQName(t, "urn:n", "Later"), mustTestQName(t, "urn:n", "Inherited"), mustTestQName(t, "urn:n", "IncludedChild"), mustTestQName(t, "urn:n", "ImportedChild"), mustTestQName(t, "urn:n", "Included"), mustTestQName(t, "urn:o", "Imported")}
			if len(components) != len(want) {
				t.Fatalf("components = %d, want %d", len(components), len(want))
			}
			for i, name := range want {
				if components[i].Name() != name {
					t.Fatalf("component %d = %q, want %q", i, components[i].Name(), name)
				}
			}
			attribute, ok := components[0].AttributeDeclaration()
			if !ok {
				t.Fatal("global inline attribute missing")
			}
			inlineRef, ok := attribute.TypeReference()
			inline, hasInline := attribute.InlineSimpleType()
			if !ok || !hasInline || !inlineRef.IsAnonymous() || inlineRef.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 4, `<xs:simpleType`) || inlineRef.VarietyLoc() != mustSchemaTokenLoc(t, "root.xsd", root, 4, `<xs:restriction`) {
				t.Fatalf("inline identity/location = %#v/%t", inlineRef, hasInline)
			}
			if id, hasID := inlineRef.AnonymousID(); !hasID || id.IsZero() || id.Source() != "root.xsd" {
				t.Fatalf("inline ID = %v/%t", id, hasID)
			}
			base, ok := inline.BaseReference()
			if !ok || !base.IsBuiltin() || base.Name() != mustTestQName(t, testXSDNamespace, "NCName") || base.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 4, `base="xs:NCName"`) {
				t.Fatalf("inline base = %#v/%t", base, ok)
			}
			assertStringEnumerationFacts(t, inline.StringEnumerationFacets(), profile.version, []string{" default ", "preserve"}, []Loc{mustSchemaTokenLoc(t, "root.xsd", root, 4, `value=" default "`), mustSchemaTokenLoc(t, "root.xsd", root, 4, `value="preserve"`)})
			white, ok := inline.StringWhiteSpaceFacet()
			if !ok || white.Value() != "collapse" || !white.Fixed() || !white.Loc().IsZero() {
				t.Fatalf("intrinsic NCName whiteSpace = %#v/%t", white, ok)
			}
			if inline.IsString() {
				t.Fatal("NCName widened public string datatype identity")
			}
			later := schemaEnumerationTestDefinitionInNamespace(t, schema, "urn:n", "Later")
			assertStringEnumerationFacts(t, later.StringEnumerationFacets(), profile.version, []string{"_alpha", "é·x"}, []Loc{mustSchemaTokenLoc(t, "root.xsd", root, 6, `value="_alpha"`), mustSchemaTokenLoc(t, "root.xsd", root, 6, `value="é·x"`)})
			forward := schemaEnumerationTestDefinitionInNamespace(t, schema, "urn:n", "Forward")
			assertStringEnumerationFacts(t, forward.StringEnumerationFacets(), profile.version, []string{" _alpha "}, []Loc{mustSchemaTokenLoc(t, "root.xsd", root, 5, `value=" _alpha "`)})
			inherited := schemaEnumerationTestDefinitionInNamespace(t, schema, "urn:n", "Inherited")
			assertStringEnumerationFacts(t, inherited.StringEnumerationFacets(), profile.version, later.StringEnumerationFacets().Values(), later.StringEnumerationFacets().Locations())
			included := schemaEnumerationTestDefinitionInNamespace(t, schema, "urn:n", "Included")
			assertStringEnumerationFacts(t, included.StringEnumerationFacets(), profile.version, []string{"from_include"}, []Loc{mustSchemaTokenLoc(t, "part.xsd", part, 1, `value="from_include"`)})
			assertStringEnumerationFacts(t, schemaEnumerationTestDefinitionInNamespace(t, schema, "urn:n", "IncludedChild").StringEnumerationFacets(), profile.version, []string{" from_include "}, []Loc{mustSchemaTokenLoc(t, "root.xsd", root, 8, `value=" from_include "`)})
			assertStringEnumerationFacts(t, schemaEnumerationTestDefinitionInNamespace(t, schema, "urn:n", "ImportedChild").StringEnumerationFacets(), profile.version, []string{"remote"}, []Loc{mustSchemaTokenLoc(t, "root.xsd", root, 9, `value="remote"`)})
			values := forward.StringEnumerationFacets().Values()
			values[0] = "changed"
			locs := forward.StringEnumerationFacets().Locations()
			locs[0] = Loc{}
			decls := forward.StringEnumerationFacets().Declarations()
			decls[0] = StringEnumerationFacet{}
			if got := forward.StringEnumerationFacets().Values(); !reflect.DeepEqual(got, []string{" _alpha "}) || forward.StringEnumerationFacets().Locations()[0].IsZero() {
				t.Fatalf("mutated copied NCName facts = %v", got)
			}
		})
	}
}

func TestNCNameEnumerationInvalidExits(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, value := range []string{"", "a:b", "9name", "bad name", "-start", "&#x9;&#xA;"} {
			t.Run(profile.name+"/"+value, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:n" version="` + string(profile.version) + `"><xs:simpleType name="Bad"><xs:restriction base="xs:NCName"><xs:enumeration value="` + value + `"/></xs:restriction></xs:simpleType></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertNCNameDiagnostic(t, schema, err, FailureInvalid, InvalidEnumerationRestrictionCode, elementReferenceTestAttributeLoc(t, root, `value="`+value+`"`), []Loc{elementReferenceTestAttributeLoc(t, root, `base="xs:NCName"`)}, enumerationSpecRef(profile.version, enumerationRestrictionRule), errSchemaNCNameValueViolation)
			})
		}
		t.Run(profile.name+"/invalid inherited lexical", func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:n" targetNamespace="urn:n" version="` + string(profile.version) + `"><xs:simpleType name="Base"><xs:restriction base="xs:NCName"><xs:enumeration value="one"/></xs:restriction></xs:simpleType><xs:simpleType name="Child"><xs:restriction base="t:Base"><xs:enumeration value="a:b"/></xs:restriction></xs:simpleType></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			assertNCNameDiagnostic(t, schema, err, FailureInvalid, InvalidEnumerationRestrictionCode, elementReferenceTestAttributeLoc(t, root, `value="a:b"`), []Loc{elementReferenceTestAttributeLoc(t, root, `base="t:Base"`)}, enumerationSpecRef(profile.version, enumerationRestrictionRule), errSchemaNCNameValueViolation)
		})
		t.Run(profile.name+"/narrowing", func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:n" targetNamespace="urn:n" version="` + string(profile.version) + `"><xs:simpleType name="Base"><xs:restriction base="xs:NCName"><xs:enumeration value="one"/></xs:restriction></xs:simpleType><xs:simpleType name="Child"><xs:restriction base="t:Base"><xs:enumeration value="two"/></xs:restriction></xs:simpleType></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			diagnostic := requireDiagnostic(t, err)
			if schema.storage != nil || len(schema.Components()) != 0 || diagnostic.Class() != FailureInvalid || diagnostic.Code() != InvalidEnumerationRestrictionCode || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, `value="two"`) || diagnostic.SpecRef() != enumerationSpecRef(profile.version, enumerationRestrictionRule) || !errors.Is(err, errInvalidEnumerationRestriction) {
				t.Fatalf("narrowing = %s / %v", diagnostic, err)
			}
			if got, want := diagnostic.Related(), []Loc{elementReferenceTestAttributeLoc(t, root, `value="one"`)}; !reflect.DeepEqual(got, want) {
				t.Fatalf("narrowing related = %v", got)
			}
		})
	}
}

func assertNCNameDiagnostic(t *testing.T, schema Schema, err error, class FailureClass, code string, loc Loc, related []Loc, spec string, cause error) {
	t.Helper()
	if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
		t.Fatalf("diagnostic returned schema=%v error=%v", schema.storage, err)
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != class || diagnostic.Code() != code || diagnostic.Loc() != loc || diagnostic.SpecRef() != spec || !reflect.DeepEqual(diagnostic.Related(), related) || !errors.Is(err, cause) {
		t.Fatalf("diagnostic = %s, related=%v, cause=%v; want %q %s %v %q %v", diagnostic, diagnostic.Related(), err, code, loc, related, spec, cause)
	}
	if code == InvalidEnumerationRestrictionCode && !errors.Is(err, errInvalidEnumerationRestriction) {
		t.Fatalf("invalid enumeration lost restriction cause: %v", err)
	}
}

//nolint:gocognit // Each excluded facet and value shape needs its own located result.
func TestNCNameEnumerationFacetAndValueConstraintExclusions(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, facet := range []struct{ name, value string }{{"pattern", ".*"}, {"length", "2"}, {"minLength", "1"}, {"maxLength", "3"}, {"whiteSpace", "collapse"}} {
			for _, shape := range []struct{ name, body string }{
				{"direct named", `<xs:simpleType name="Bad"><xs:restriction base="xs:NCName"><xs:` + facet.name + ` value="` + facet.value + `"/></xs:restriction></xs:simpleType>`},
				{"inherited named", `<xs:simpleType name="Base"><xs:restriction base="xs:NCName"><xs:enumeration value="one"/></xs:restriction></xs:simpleType><xs:simpleType name="Bad"><xs:restriction base="t:Base"><xs:` + facet.name + ` value="` + facet.value + `"/></xs:restriction></xs:simpleType>`},
				{"inline attribute", `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:NCName"><xs:` + facet.name + ` value="` + facet.value + `"/></xs:restriction></xs:simpleType></xs:attribute>`},
				{"ref source", `<xs:attribute name="a" type="t:Bad"/><xs:complexType name="Box"><xs:attribute ref="t:a"/></xs:complexType><xs:simpleType name="Bad"><xs:restriction base="xs:NCName"><xs:` + facet.name + ` value="` + facet.value + `"/></xs:restriction></xs:simpleType>`},
			} {
				t.Run(profile.name+"/"+facet.name+"/"+shape.name, func(t *testing.T) {
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:n" targetNamespace="urn:n" version="` + string(profile.version) + `">` + shape.body + `</xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					diagnostic := requireDiagnostic(t, err)
					if schema.storage != nil || diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedDatatypeFacetCode || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:`+facet.name) || diagnostic.SpecRef() != tokenDiagnosticSpecRef(profile.version, "decimal") || !errors.Is(err, ErrUnsupported) {
						t.Fatalf("facet exclusion = %s / %v", diagnostic, err)
					}
				})
			}
		}
		for _, constraint := range []string{"default", "fixed"} {
			for _, shape := range []struct{ name, body string }{
				{"direct", `<xs:attribute name="a" type="xs:NCName" ` + constraint + `="one"/>`},
				{"named", `<xs:attribute name="a" type="t:T" ` + constraint + `="one"/><xs:simpleType name="T"><xs:restriction base="xs:NCName"><xs:enumeration value="one"/></xs:restriction></xs:simpleType>`},
				{"inline", `<xs:attribute name="a" ` + constraint + `="one"><xs:simpleType><xs:restriction base="xs:NCName"><xs:enumeration value="one"/></xs:restriction></xs:simpleType></xs:attribute>`},
				{"ref", `<xs:attribute name="a" type="xs:NCName" ` + constraint + `="one"/><xs:complexType name="Box"><xs:attribute ref="t:a"/></xs:complexType>`},
			} {
				t.Run(profile.name+"/"+constraint+"/"+shape.name, func(t *testing.T) {
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:n" targetNamespace="urn:n" version="` + string(profile.version) + `">` + shape.body + `</xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, constraint+`="one"`), nil, schemaAttributeValueConstraintSpecRef(profile.version), errSchemaAttributeValueConstraintUnsupported)
				})
			}
		}
	}
}

func TestNCNameEnumerationLocalAndVarietyExclusions(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct {
			name, body, primary, related, spec string
			cause                              error
		}{
			{"local direct", `<xs:complexType name="Box"><xs:attribute name="a" type="xs:NCName"/></xs:complexType>`, `type="xs:NCName"`, ``, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnsupported},
			{"local named", `<xs:complexType name="Box"><xs:attribute name="a" type="t:T"/></xs:complexType><xs:simpleType name="T"><xs:restriction base="xs:NCName"><xs:enumeration value="one"/></xs:restriction></xs:simpleType>`, `type="t:T"`, ``, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnsupported},
			{"local inline", `<xs:complexType name="Box"><xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:NCName"><xs:enumeration value="one"/></xs:restriction></xs:simpleType></xs:attribute></xs:complexType>`, `<xs:simpleType`, ``, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnsupported},
			{"local ref direct", `<xs:attribute name="a" type="xs:NCName"/><xs:complexType name="Box"><xs:attribute ref="t:a"/></xs:complexType>`, `ref="t:a"`, `<xs:attribute name="a"`, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeReferenceUnsupported},
			{"local ref named", `<xs:attribute name="a" type="t:T"/><xs:complexType name="Box"><xs:attribute ref="t:a"/></xs:complexType><xs:simpleType name="T"><xs:restriction base="xs:NCName"><xs:enumeration value="one"/></xs:restriction></xs:simpleType>`, `ref="t:a"`, `<xs:attribute name="a"`, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeReferenceUnsupported},
			{"local ref inline", `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:NCName"><xs:enumeration value="one"/></xs:restriction></xs:simpleType></xs:attribute><xs:complexType name="Box"><xs:attribute ref="t:a"/></xs:complexType>`, `ref="t:a"`, `<xs:attribute name="a"`, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeReferenceUnsupported},
			{"named list", `<xs:attribute name="a" type="t:T"/><xs:simpleType name="T"><xs:list itemType="xs:NCName"/></xs:simpleType>`, `type="t:T"`, ``, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnsupported},
			{"named union", `<xs:attribute name="a" type="t:T"/><xs:simpleType name="T"><xs:union memberTypes="xs:NCName"/></xs:simpleType>`, `type="t:T"`, ``, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnsupported},
			{"inline list ref", `<xs:attribute name="a"><xs:simpleType><xs:list itemType="xs:NCName"/></xs:simpleType></xs:attribute><xs:complexType name="Box"><xs:attribute ref="t:a"/></xs:complexType>`, `ref="t:a"`, `<xs:attribute name="a"`, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeReferenceUnsupported},
			{"inline union ref", `<xs:attribute name="a"><xs:simpleType><xs:union memberTypes="xs:NCName"/></xs:simpleType></xs:attribute><xs:complexType name="Box"><xs:attribute ref="t:a"/></xs:complexType>`, `ref="t:a"`, `<xs:attribute name="a"`, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeReferenceUnsupported},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:n" targetNamespace="urn:n" version="` + string(profile.version) + `">` + test.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				related := []Loc(nil)
				if test.related != "" {
					related = []Loc{elementReferenceTestAttributeLoc(t, root, test.related)}
				}
				assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, test.primary), related, test.spec, test.cause)
			})
		}
	}
}

//nolint:gocognit // Check validation and generation independently for each admitted shape.
func TestNCNameEnumerationElementConsumersRemainUnsupported(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, shape := range []struct{ name, body, instance, genMark string }{
			{"direct", `<xs:element name="item" type="xs:NCName"/>`, `<item xmlns="urn:n">one</item>`, `<xs:element name="item"`},
			{"named", `<xs:element name="item" type="t:T"/><xs:simpleType name="T"><xs:restriction base="xs:NCName"><xs:enumeration value="one"/></xs:restriction></xs:simpleType>`, `<item xmlns="urn:n">one</item>`, `<xs:element name="item"`},
			{"inline", `<xs:element name="item"><xs:simpleType><xs:restriction base="xs:NCName"><xs:enumeration value="one"/></xs:restriction></xs:simpleType></xs:element>`, `<item xmlns="urn:n">one</item>`, `<xs:element name="item"`},
			{"ref", `<xs:complexType name="Box"><xs:choice><xs:element ref="t:item"/></xs:choice></xs:complexType><xs:element name="root" type="t:Box"/><xs:element name="item" type="xs:NCName"/>`, `<root xmlns="urn:n"><item>one</item></root>`, `ref="t:item"`},
		} {
			t.Run(profile.name+"/"+shape.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:n" targetNamespace="urn:n" version="` + string(profile.version) + `">` + shape.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("query schema: %v", err)
				}
				output, genErr := GenerateGo(schema, "generated")
				genDiagnostic := requireDiagnostic(t, genErr)
				if output != nil || genDiagnostic.Class() != FailureUnsupported || genDiagnostic.Code() != diagnosticCodegenUnsupported || genDiagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, shape.genMark) || !errors.Is(genErr, errCodegenUnsupported) {
					t.Fatalf("generation = %q / %s / %v", output, genDiagnostic, genErr)
				}
				validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(shape.instance)))
				validationDiagnostic := requireDiagnostic(t, validationErr)
				cause := errInstanceUnsupportedType
				if shape.name == "inline" {
					cause = errInstanceNoDeclaredType
				}
				if validationDiagnostic.Class() != FailureUnsupported || validationDiagnostic.Code() != UnsupportedInstanceValidationCode || validationDiagnostic.Loc() != mustTestLoc(t, "instance.xml", 1, 1) || !errors.Is(validationErr, cause) {
					t.Fatalf("validation = %s / %v", validationDiagnostic, validationErr)
				}
			})
		}
	}
}

func TestNCNameEnumerationGraphFailuresKeepLocatedCauses(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct {
			name, body, mark, related, code string
			cause                           error
		}{
			{"unresolved", `<xs:simpleType name="T"><xs:restriction base="t:Missing"><xs:enumeration value="one"/></xs:restriction></xs:simpleType>`, `base="t:Missing"`, ``, diagnosticSchemaSimpleTypeUnresolvedCode, errSchemaSimpleTypeBaseUnresolved},
			{"cycle", `<xs:simpleType name="A"><xs:restriction base="t:B"><xs:enumeration value="one"/></xs:restriction></xs:simpleType><xs:simpleType name="B"><xs:restriction base="t:A"><xs:enumeration value="one"/></xs:restriction></xs:simpleType>`, `base="t:B"`, `base="t:A"`, diagnosticSchemaSimpleTypeCycleCode, errSchemaSimpleTypeBaseCycle},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:n" targetNamespace="urn:n" version="` + string(profile.version) + `">` + test.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				related := []Loc(nil)
				if test.related != "" {
					related = []Loc{elementReferenceTestAttributeLoc(t, root, test.related)}
				}
				assertNCNameDiagnostic(t, schema, err, FailureInvalid, test.code, elementReferenceTestAttributeLoc(t, root, test.mark), related, schemaSimpleTypeSpecRef(profile.version), test.cause)
			})
		}
		for _, relation := range []string{"unimported", "indirect-import"} {
			t.Run(profile.name+"/"+relation, func(t *testing.T) {
				root, fixtures := typeVisibilityTestHiddenGraph(t, string(profile.version), relation, false, false, false, "restriction")
				foreign := fixtures["foreign.xsd"]
				foreign.contents = `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:foreign" version="` + string(profile.version) + `"><xs:simpleType name="Hidden"><xs:restriction base="xs:NCName"><xs:enumeration value="one"/></xs:restriction></xs:simpleType></xs:schema>`
				fixtures["foreign.xsd"] = foreign
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				assertNCNameDiagnostic(t, schema, err, FailureInvalid, diagnosticSchemaSimpleTypeUnresolvedCode, elementReferenceTestAttributeLoc(t, root, `base="f:Hidden"`), nil, schemaSimpleTypeSpecRef(profile.version), errSchemaSimpleTypeBaseUnresolved)
			})
		}
	}
}

//nolint:gocognit // Verify built-in and named attribute facts and immutable copies together.
func TestNCNameEnumerationTypedAttributeFacts(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:n" targetNamespace="urn:n" version="` + string(profile.version) + `"><xs:attribute name="builtin" type="xs:NCName"/><xs:attribute name="named" type="t:Named"/><xs:simpleType name="Named"><xs:restriction base="xs:NCName"><xs:enumeration value=" one "/><xs:enumeration value="two"/></xs:restriction></xs:simpleType></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("typed attribute schema: %v", err)
			}
			for _, test := range []struct {
				name, typeName string
				values         []string
				locations      []Loc
			}{
				{"builtin", "NCName", nil, nil},
				{"named", "Named", []string{" one ", "two"}, []Loc{elementReferenceTestAttributeLoc(t, root, `value=" one "`), elementReferenceTestAttributeLoc(t, root, `value="two"`)}},
			} {
				components := schema.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:n", test.name))
				if len(components) != 1 {
					t.Fatalf("%s count=%d", test.name, len(components))
				}
				declaration, ok := components[0].AttributeDeclaration()
				if !ok {
					t.Fatalf("%s has no attribute view", test.name)
				}
				reference, ok := declaration.TypeReference()
				if !ok || reference.Name().Local() != test.typeName || reference.Loc() != elementReferenceTestAttributeLoc(t, root, `type="`+map[bool]string{true: "xs:NCName", false: "t:Named"}[test.name == "builtin"]+`"`) {
					t.Fatalf("%s ref = %#v/%t", test.name, reference, ok)
				}
				facets := reference.StringEnumerationFacets()
				white, ok := reference.StringWhiteSpaceFacet()
				if !ok || white.Value() != "collapse" || !white.Fixed() {
					t.Fatalf("%s whitespace = %#v/%t", test.name, white, ok)
				}
				if test.values == nil {
					if facets.HasEnumeration() || facets.Values() != nil || facets.Version() != profile.version {
						t.Fatalf("builtin facts = %#v", facets)
					}
					continue
				}
				assertStringEnumerationFacts(t, facets, profile.version, test.values, test.locations)
				values := facets.Values()
				values[0] = "changed"
				if got := reference.StringEnumerationFacets().Values(); !reflect.DeepEqual(got, test.values) {
					t.Fatalf("named attribute facts changed: %v", got)
				}
			}
		})
	}
}

func TestNCNameEnumerationAttributeGenerationRemainsUnsupported(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, shape := range []struct{ name, body string }{
			{"direct", `<xs:attribute name="a" type="xs:NCName"/>`},
			{"named", `<xs:attribute name="a" type="t:T"/><xs:simpleType name="T"><xs:restriction base="xs:NCName"><xs:enumeration value="one"/></xs:restriction></xs:simpleType>`},
			{"inline", `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:NCName"><xs:enumeration value="one"/></xs:restriction></xs:simpleType></xs:attribute>`},
		} {
			t.Run(profile.name+"/"+shape.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:n" targetNamespace="urn:n" version="` + string(profile.version) + `">` + shape.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("query schema: %v", err)
				}
				output, err := GenerateGo(schema, "generated")
				diagnostic := requireDiagnostic(t, err)
				if output != nil || diagnostic.Class() != FailureUnsupported || diagnostic.Code() != diagnosticCodegenUnsupported || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="a"`) || !errors.Is(err, errCodegenUnsupported) {
					t.Fatalf("generation = %q / %s / %v", output, diagnostic, err)
				}
			})
		}
	}
}

func TestNCNameEnumerationPolicyFollowsSelectedEditionAcrossLabels(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, label := range []string{"1.0", "1.1"} {
			t.Run(profile.name+"/label-"+label, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:n" version="` + label + `"><xs:simpleType name="N"><xs:restriction base="xs:NCName"><xs:enumeration value="_one"/></xs:restriction></xs:simpleType></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("NCName enumeration under %s and label %s: %v", profile.name, label, err)
				}
				definition := schemaEnumerationTestDefinitionInNamespace(t, schema, "urn:n", "N")
				assertStringEnumerationFacts(t, definition.StringEnumerationFacets(), profile.version, []string{"_one"}, []Loc{elementReferenceTestAttributeLoc(t, root, `value="_one"`)})
			})
		}
	}
}
