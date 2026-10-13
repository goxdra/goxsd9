package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func ncNameLocalRoot(version, body string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:n" targetNamespace="urn:n" attributeFormDefault="qualified" version="` + version + `">` + body + `</xs:schema>`
}

func ncNameLocalOwner(body string) string {
	return `<xs:complexType name="Box">` + body + `</xs:complexType>`
}

//nolint:gocognit // Each supported owner and policy must expose the same copied public facts.
func TestNCNameLocalAttributeNamedOwnersExposeEffectiveFacts(t *testing.T) {
	shapes := []struct{ name, body string }{
		{"empty", `<xs:attribute name="built" type="xs:NCName"/><xs:attribute name="named" type="t:Child" use="required"/>`},
		{"choice", `<xs:choice><xs:element name="item" type="xs:integer"/></xs:choice><xs:attribute name="built" type="xs:NCName"/><xs:attribute name="named" type="t:Child" use="required"/>`},
		{"sequence", `<xs:sequence><xs:element name="item" type="xs:integer" minOccurs="2" maxOccurs="3"/></xs:sequence><xs:attribute name="built" type="xs:NCName"/><xs:attribute name="named" type="t:Child" use="required"/>`},
		{"grouped extension", `<xs:complexContent><xs:extension base="t:Base"><xs:group ref="t:Fields" minOccurs="0" maxOccurs="0"/><xs:attribute name="built" type="xs:NCName"/><xs:attribute name="named" type="t:Child" use="required"/></xs:extension></xs:complexContent>`},
		{"sequence extension", `<xs:complexContent><xs:extension base="t:Base"><xs:sequence><xs:element name="item" type="xs:integer"/></xs:sequence><xs:attribute name="built" type="xs:NCName"/><xs:attribute name="named" type="t:Child" use="required"/></xs:extension></xs:complexContent>`},
	}
	for _, profile := range tokenPolicyProfiles() {
		for _, shape := range shapes {
			t.Run(profile.name+"/"+shape.name, func(t *testing.T) {
				root := ncNameLocalRoot(string(profile.version), ncNameLocalOwner(shape.body)+`<xs:complexType name="Base"/><xs:group name="Fields"><xs:sequence><xs:element ref="t:field"/></xs:sequence></xs:group><xs:element name="field" type="xs:integer"/><xs:simpleType name="Child"><xs:restriction base="t:Parent"><xs:enumeration value="one"/></xs:restriction></xs:simpleType><xs:simpleType name="Parent"><xs:restriction base="xs:NCName"><xs:enumeration value="one"/><xs:enumeration value="two"/></xs:restriction></xs:simpleType>`)
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
				for index, lexical := range []string{"xs:NCName", "t:Child"} {
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
					if index == 0 {
						if !reference.IsBuiltin() || use.DeclaredType() != mustTestQName(t, testXSDNamespace, "NCName") || reference.StringEnumerationFacets().HasEnumeration() {
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

//nolint:gocognit // Type resolution precedes omission for each classified exit.
func TestNCNameLocalAttributeProhibitedChecksTypeBeforeOmission(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct {
			name, typeName, extra, code, primary, related, spec string
			cause                                               error
		}{
			{"valid", "xs:NCName", "", "", "", "", "", nil},
			{"unresolved", "t:Missing", "", diagnosticSchemaAttributeTypeUnresolvedCode, `type="t:Missing"`, "", schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved},
			{"wrong kind", "t:Wrong", `<xs:complexType name="Wrong"/>`, diagnosticSchemaAttributeTypeWrongKindCode, `type="t:Wrong"`, `<xs:complexType name="Wrong"`, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeWrongKind},
			{"invalid narrowing", "t:Bad", `<xs:simpleType name="Bad"><xs:restriction base="t:Parent"><xs:enumeration value="other"/></xs:restriction></xs:simpleType><xs:simpleType name="Parent"><xs:restriction base="xs:NCName"><xs:enumeration value="one"/></xs:restriction></xs:simpleType>`, InvalidEnumerationRestrictionCode, `value="other"`, `value="one"`, enumerationSpecRef(profile.version, enumerationRestrictionRule), errInvalidEnumerationRestriction},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := ncNameLocalRoot(string(profile.version), ncNameLocalOwner(`<xs:attribute name="gone" type="`+test.typeName+`" use="prohibited"/>`)+test.extra)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if test.code == "" {
					if err != nil || len(requireTestComplexTypeDefinition(t, schema.Components()[0], "Box").AttributeUses()) != 0 {
						t.Fatalf("prohibited use = %v / %v", schema.Components(), err)
					}
					return
				}
				var related []Loc
				if test.related != "" {
					related = []Loc{elementReferenceTestAttributeLoc(t, root, test.related)}
				}
				assertNCNameDiagnostic(t, schema, err, FailureInvalid, test.code, elementReferenceTestAttributeLoc(t, root, test.primary), related, test.spec, test.cause)
			})
		}
	}
}

//nolint:gocognit // One graph exercises ordered visibility and effective facts from every source kind.
func TestNCNameLocalAttributeNamedGraphPreservesIdentityAndOrder(t *testing.T) {
	root := ncNameLocalRoot("1.1", `<xs:include schemaLocation="part.xsd"/><xs:include schemaLocation="part.xsd"/><xs:include schemaLocation="chameleon.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/><xs:complexType name="Box"><xs:attribute name="forward" type="t:Child"/><xs:attribute name="included" type="t:Included" form="unqualified"/><xs:attribute name="adopted" type="t:Adopted"/><xs:attribute name="foreign" type="o:Foreign"/><xs:attribute name="gone" type="xs:NCName" use="prohibited"/></xs:complexType><xs:simpleType name="Child"><xs:restriction base="t:Parent"><xs:enumeration value=" one "/></xs:restriction></xs:simpleType><xs:simpleType name="Parent"><xs:restriction base="xs:NCName"><xs:enumeration value="one"/><xs:enumeration value="two"/></xs:restriction></xs:simpleType>`)
	root = strings.Replace(root, `xmlns:t="urn:n"`, `xmlns:t="urn:n" xmlns:o="urn:other"`, 1)
	part := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:n"><xs:simpleType name="Included"><xs:restriction base="xs:NCName"><xs:enumeration value="inc"/></xs:restriction></xs:simpleType></xs:schema>`
	chameleon := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Adopted"><xs:restriction base="xs:NCName"><xs:enumeration value="adopt"/></xs:restriction></xs:simpleType></xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Foreign"><xs:restriction base="xs:NCName"><xs:enumeration value="remote"/></xs:restriction></xs:simpleType></xs:schema>`
	fixtures := map[string]discoveryFixture{"part.xsd": {id: "part.xsd", contents: part}, "chameleon.xsd": {id: "chameleon.xsd", contents: chameleon}, "other.xsd": {id: "other.xsd", contents: other}}
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy)
			if err != nil {
				t.Fatalf("graph: %v", err)
			}
			found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:n", "Box"))
			uses := requireTestComplexTypeDefinition(t, found[0], "Box").AttributeUses()
			if len(uses) != 4 {
				t.Fatalf("ordered uses = %v", uses)
			}
			want := []struct {
				local, lexical, namespace, source string
				values                            []string
				location                          Loc
			}{
				{"forward", "t:Child", "urn:n", "root.xsd", []string{" one "}, elementReferenceTestAttributeLoc(t, root, `value=" one "`)},
				{"included", "t:Included", "", "part.xsd", []string{"inc"}, mustSchemaTokenLoc(t, "part.xsd", part, 1, `value="inc"`)},
				{"adopted", "t:Adopted", "urn:n", "chameleon.xsd", []string{"adopt"}, mustSchemaTokenLoc(t, "chameleon.xsd", chameleon, 1, `value="adopt"`)},
				{"foreign", "o:Foreign", "urn:n", "other.xsd", []string{"remote"}, mustSchemaTokenLoc(t, "other.xsd", other, 1, `value="remote"`)},
			}
			for index, expected := range want {
				use, ok := uses[index].(LocalAttributeUse)
				if !ok || use.Name() != mustTestQName(t, expected.namespace, expected.local) || use.TypeLoc() != elementReferenceTestAttributeLoc(t, root, `type="`+expected.lexical+`"`) || use.NameLoc() != elementReferenceTestAttributeLoc(t, root, `name="`+expected.local+`"`) {
					t.Fatalf("use %d = %v", index, uses[index])
				}
				if index == 1 && use.FormLoc() != elementReferenceTestAttributeLoc(t, root, `form="unqualified"`) {
					t.Fatalf("included form Loc = %v", use.FormLoc())
				}
				id, hasID := use.TypeID()
				reference, hasReference := use.TypeReference()
				if !hasID || !hasReference || id.Source() != SourceID(expected.source) || reference.Name() != use.DeclaredType() || reference.Loc() != use.TypeLoc() {
					t.Fatalf("use %d identity = %v/%t, ref %v/%t", index, id, hasID, reference, hasReference)
				}
				version := XSDVersion11
				if policy == Strict10 {
					version = XSDVersion10
				}
				assertStringEnumerationFacts(t, reference.StringEnumerationFacets(), version, expected.values, []Loc{expected.location})
			}
			again, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy)
			if err != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
				t.Fatalf("repeated graph = %v, same %t", err, reflect.DeepEqual(schema.Components(), again.Components()))
			}
		})
	}
}

//nolint:gocognit // The same inherited facts must follow each policy across both version labels.
func TestNCNameLocalAttributeInheritedEnumerationUsesSelectedPolicy(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, label := range []string{"1.0", "1.1"} {
			t.Run(profile.name+"/label-"+label, func(t *testing.T) {
				root := ncNameLocalRoot(label, ncNameLocalOwner(`<xs:attribute name="a" type="t:Inherited"/>`)+`<xs:simpleType name="Inherited"><xs:restriction base="t:Parent"/></xs:simpleType><xs:simpleType name="Parent"><xs:restriction base="xs:NCName"><xs:enumeration value="one"/><xs:enumeration value="two"/></xs:restriction></xs:simpleType>`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("inherited type: %v", err)
				}
				use, ok := requireTestComplexTypeDefinition(t, schema.Components()[0], "Box").AttributeUses()[0].(LocalAttributeUse)
				if !ok {
					t.Fatal("inherited NCName use is not local")
				}
				reference, ok := use.TypeReference()
				if !ok || reference.Name() != mustTestQName(t, "urn:n", "Inherited") {
					t.Fatalf("inherited reference = %v/%t", reference, ok)
				}
				assertStringEnumerationFacts(t, reference.StringEnumerationFacets(), profile.version, []string{"one", "two"}, []Loc{elementReferenceTestAttributeLoc(t, root, `value="one"`), elementReferenceTestAttributeLoc(t, root, `value="two"`)})
			})
		}
	}
}

func TestNCNameLocalAttributeGraphAndReferenceFailuresStayLocated(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct {
			name, extra, typeName, code, related string
			cause                                error
		}{
			{"unresolved", "", "t:Missing", diagnosticSchemaAttributeTypeUnresolvedCode, "", errSchemaAttributeTypeUnresolved},
			{"wrong kind", `<xs:complexType name="Wrong"/>`, "t:Wrong", diagnosticSchemaAttributeTypeWrongKindCode, `<xs:complexType name="Wrong"`, errSchemaAttributeTypeWrongKind},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := ncNameLocalRoot(string(profile.version), ncNameLocalOwner(`<xs:attribute name="a" type="`+test.typeName+`"/>`)+test.extra)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				var related []Loc
				if test.related != "" {
					related = []Loc{elementReferenceTestAttributeLoc(t, root, test.related)}
				}
				assertNCNameDiagnostic(t, schema, err, FailureInvalid, test.code, elementReferenceTestAttributeLoc(t, root, `type="`+test.typeName+`"`), related, schemaAttributeTypeSpecRef(profile.version), test.cause)
			})
		}
	}
	root := ncNameLocalRoot("1.1", `<xs:include schemaLocation="missing.xsd"/>`+ncNameLocalOwner(`<xs:attribute name="a" type="t:Missing"/>`))
	source, err := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("NCName type source unavailable")
	schema, err := ParseSchemaWithPolicy(source, &discoveryResolver{failures: map[string]error{"missing.xsd": cause}}, Strict11)
	assertNCNameDiagnostic(t, schema, err, FailureResolution, SourceResolveCode, elementReferenceTestAttributeLoc(t, root, `<xs:include`), nil, "", cause)
}

func TestNCNameLocalAttributeExcludedFormsStayLocated(t *testing.T) {
	tests := []struct {
		name, body, extra, primary, related string
		spec                                func(XSDVersion) string
		cause                               error
	}{
		{"inline restriction", ncNameLocalOwner(`<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:NCName"><xs:enumeration value="one"/></xs:restriction></xs:simpleType></xs:attribute>`), "", `<xs:simpleType`, "", schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnsupported},
		{"ref built-in", ncNameLocalOwner(`<xs:attribute ref="t:a"/>`), `<xs:attribute name="a" type="xs:NCName"/>`, `ref="t:a"`, `<xs:attribute name="a"`, schemaAttributeUseSpecRef, errSchemaAttributeReferenceUnsupported},
		{"ref named", ncNameLocalOwner(`<xs:attribute ref="t:a"/>`), `<xs:attribute name="a" type="t:N"/><xs:simpleType name="N"><xs:restriction base="xs:NCName"/></xs:simpleType>`, `ref="t:a"`, `<xs:attribute name="a"`, schemaAttributeUseSpecRef, errSchemaAttributeReferenceUnsupported},
		{"ref inline", ncNameLocalOwner(`<xs:attribute ref="t:a"/>`), `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:NCName"/></xs:simpleType></xs:attribute>`, `ref="t:a"`, `<xs:attribute name="a"`, schemaAttributeUseSpecRef, errSchemaAttributeReferenceUnsupported},
		{"default built-in", ncNameLocalOwner(`<xs:attribute name="a" type="xs:NCName" default="one"/>`), "", `default="one"`, "", schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported},
		{"fixed named", ncNameLocalOwner(`<xs:attribute name="a" type="t:N" fixed="one"/>`), `<xs:simpleType name="N"><xs:restriction base="xs:NCName"/></xs:simpleType>`, `fixed="one"`, "", schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported},
		{"default named", ncNameLocalOwner(`<xs:attribute name="a" type="t:N" default="one"/>`), `<xs:simpleType name="N"><xs:restriction base="xs:NCName"/></xs:simpleType>`, `default="one"`, "", schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported},
		{"fixed built-in", ncNameLocalOwner(`<xs:attribute name="a" type="xs:NCName" fixed="one"/>`), "", `fixed="one"`, "", schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported},
		{"default inline", ncNameLocalOwner(`<xs:attribute name="a" default="one"><xs:simpleType><xs:restriction base="xs:NCName"/></xs:simpleType></xs:attribute>`), "", `default="one"`, "", schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported},
		{"fixed inline", ncNameLocalOwner(`<xs:attribute name="a" fixed="one"><xs:simpleType><xs:restriction base="xs:NCName"/></xs:simpleType></xs:attribute>`), "", `fixed="one"`, "", schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported},
		{"named list", ncNameLocalOwner(`<xs:attribute name="a" type="t:L"/>`), `<xs:simpleType name="L"><xs:list itemType="xs:NCName"/></xs:simpleType>`, `type="t:L"`, "", schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnsupported},
		{"named union", ncNameLocalOwner(`<xs:attribute name="a" type="t:U"/>`), `<xs:simpleType name="U"><xs:union memberTypes="xs:NCName"/></xs:simpleType>`, `type="t:U"`, "", schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnsupported},
		{"inline list", ncNameLocalOwner(`<xs:attribute name="a"><xs:simpleType><xs:list itemType="xs:NCName"/></xs:simpleType></xs:attribute>`), "", `<xs:simpleType`, "", schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnsupported},
		{"inline union", ncNameLocalOwner(`<xs:attribute name="a"><xs:simpleType><xs:union memberTypes="xs:NCName"/></xs:simpleType></xs:attribute>`), "", `<xs:simpleType`, "", schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnsupported},
		{"direct all", ncNameLocalOwner(`<xs:all><xs:element name="item" type="xs:integer"/></xs:all><xs:attribute name="a" type="xs:NCName"/>`), "", `type="xs:NCName"`, "", schemaAttributeTypeSpecRef, errSchemaAttributeUseUnsupported},
		{"direct group", ncNameLocalOwner(`<xs:group ref="t:Fields"/><xs:attribute name="a" type="xs:NCName"/>`), `<xs:group name="Fields"><xs:choice><xs:element ref="t:item"/></xs:choice></xs:group><xs:element name="item" type="xs:integer"/>`, `type="xs:NCName"`, "", schemaAttributeTypeSpecRef, errSchemaAttributeUseUnsupported},
		{"simple content", ncNameLocalOwner(`<xs:simpleContent><xs:extension base="xs:string"><xs:attribute name="a" type="xs:NCName"/></xs:extension></xs:simpleContent>`), "", `type="xs:NCName"`, "", schemaAttributeTypeSpecRef, errSchemaAttributeUseUnsupported},
		{"inline complex owner", `<xs:element name="root"><xs:complexType><xs:attribute name="a" type="xs:NCName"/></xs:complexType></xs:element>`, "", `type="xs:NCName"`, "", schemaAttributeTypeSpecRef, errSchemaAttributeUseUnsupported},
	}
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range tests {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := ncNameLocalRoot(string(profile.version), test.body+test.extra)
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

func TestNCNameLocalAttributeInvisibleImportAndCyclesFailBeforePublication(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name+"/invisible", func(t *testing.T) {
			root := ncNameLocalRoot(string(profile.version), `<xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/>`+ncNameLocalOwner(`<xs:attribute name="a" type="o:Hidden"/>`))
			root = strings.Replace(root, `xmlns:t="urn:n"`, `xmlns:t="urn:n" xmlns:o="urn:other"`, 1)
			fixtures := map[string]discoveryFixture{
				"bridge.xsd": {id: "bridge.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge"><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`},
				"other.xsd":  {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Hidden"><xs:restriction base="xs:NCName"/></xs:simpleType></xs:schema>`},
			}
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			assertNCNameDiagnostic(t, schema, err, FailureInvalid, diagnosticSchemaAttributeTypeUnresolvedCode, elementReferenceTestAttributeLoc(t, root, `type="o:Hidden"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved)
		})
		t.Run(profile.name+"/cycle", func(t *testing.T) {
			root := ncNameLocalRoot(string(profile.version), ncNameLocalOwner(`<xs:attribute name="a" type="t:First"/>`)+`<xs:simpleType name="First"><xs:restriction base="t:Second"/></xs:simpleType><xs:simpleType name="Second"><xs:restriction base="t:First"/></xs:simpleType>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			diagnostic := requireDiagnostic(t, err)
			if schema.storage != nil || diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaAttributeTypeCycleCode || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, `type="t:First"`) || diagnostic.SpecRef() != schemaAttributeTypeSpecRef(profile.version) || !errors.Is(err, errSchemaSimpleTypeBaseCycle) {
				t.Fatalf("cycle = %s / %v", diagnostic, err)
			}
			want := []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="a"`), elementReferenceTestAttributeLoc(t, root, `base="t:Second"`), elementReferenceTestAttributeLoc(t, root, `base="t:First"`), elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="First"`), elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="Second"`)}
			if !reflect.DeepEqual(diagnostic.Related(), want) {
				t.Fatalf("cycle related = %v, want %v", diagnostic.Related(), want)
			}
		})
	}
}

func TestNCNameLocalAttributeDoesNotAcquireIDCardinality(t *testing.T) {
	root := ncNameLocalRoot("1.0", ncNameLocalOwner(`<xs:attribute name="first" type="xs:NCName"/><xs:attribute name="id" type="xs:ID"/><xs:attribute name="second" type="t:N"/>`)+`<xs:simpleType name="N"><xs:restriction base="xs:NCName"/></xs:simpleType>`)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	if err != nil {
		t.Fatalf("NCName and one ID: %v", err)
	}
	definition := requireTestComplexTypeDefinition(t, schema.Components()[0], "Box")
	if len(definition.AttributeUses()) != 3 {
		t.Fatalf("uses = %v", definition.AttributeUses())
	}
	root = ncNameLocalRoot("1.0", ncNameLocalOwner(`<xs:attribute name="first" type="xs:NCName"/><xs:attribute name="id" type="xs:ID"/><xs:attribute name="second" type="xs:ID"/>`))
	schema, err = discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	assertNCNameDiagnostic(t, schema, err, FailureInvalid, diagnosticSchemaAttributeUseIDDuplicateCode, elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="second"`), []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="id"`)}, "xsd10-structures#cos-ct-props-correct", errSchemaAttributeUseIDDuplicate)
}

//nolint:gocognit // Malformed forms and duplicate names retain distinct public diagnostics.
func TestNCNameLocalAttributeMalformedUsesAndDuplicatesStayLocated(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct {
			name, uses, extra, primary, code string
			cause                            error
			spec                             string
		}{
			{"bad name", `<xs:attribute name="a:b" type="xs:NCName"/>`, "", `name="a:b"`, invalidSchemaCompositionCode, nil, ""},
			{"bad use", `<xs:attribute name="a" type="xs:NCName" use="sometimes"/>`, "", `use="sometimes"`, invalidSchemaCompositionCode, nil, ""},
			{"name and ref", `<xs:attribute name="a" ref="t:global" type="xs:NCName"/>`, `<xs:attribute name="global" type="xs:NCName"/>`, `ref="t:global"`, invalidSchemaCompositionCode, nil, ""},
			{"type and inline", `<xs:attribute name="a" type="xs:NCName"><xs:simpleType><xs:restriction base="xs:NCName"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, invalidSchemaCompositionCode, nil, ""},
			{"duplicate name", `<xs:attribute name="a" type="xs:NCName"/><xs:attribute name="a" type="t:N"/>`, `<xs:simpleType name="N"><xs:restriction base="xs:NCName"/></xs:simpleType>`, `<xs:attribute name="a" type="t:N"`, diagnosticSchemaAttributeUseDuplicateCode, errSchemaAttributeUseDuplicate, schemaAttributeUseSpecRef(profile.version)},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := ncNameLocalRoot(string(profile.version), ncNameLocalOwner(test.uses)+test.extra)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil {
					t.Fatalf("malformed use published schema: %v / %v", schema.storage, err)
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, test.primary) || diagnostic.SpecRef() != test.spec || test.cause != nil && !errors.Is(err, test.cause) {
					t.Fatalf("malformed use diagnostic = %s / %v", diagnostic, err)
				}
				if test.name == "duplicate name" {
					wantRelated := []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="a"`)}
					if !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
						t.Fatalf("duplicate related = %v", diagnostic.Related())
					}
				}
			})
		}
	}
}

//nolint:gocognit // Each admitted owner has two independent unsupported consumers.
func TestNCNameLocalAttributeConsumersRemainUnsupported(t *testing.T) {
	for _, shape := range []struct{ name, body string }{
		{"empty", `<xs:attribute name="a" type="xs:NCName"/>`},
		{"choice", `<xs:choice><xs:element name="item" type="xs:integer"/></xs:choice><xs:attribute name="a" type="xs:NCName"/>`},
		{"sequence", `<xs:sequence><xs:element name="item" type="xs:integer"/></xs:sequence><xs:attribute name="a" type="xs:NCName"/>`},
		{"grouped extension", `<xs:complexContent><xs:extension base="t:Base"><xs:group ref="t:Fields" minOccurs="0" maxOccurs="0"/><xs:attribute name="a" type="xs:NCName"/></xs:extension></xs:complexContent>`},
		{"sequence extension", `<xs:complexContent><xs:extension base="t:Base"><xs:sequence><xs:element name="item" type="xs:integer"/></xs:sequence><xs:attribute name="a" type="xs:NCName"/></xs:extension></xs:complexContent>`},
	} {
		t.Run(shape.name, func(t *testing.T) {
			root := ncNameLocalRoot("1.1", ncNameLocalOwner(shape.body)+`<xs:element name="root" type="t:Box"/><xs:complexType name="Base"/><xs:group name="Fields"><xs:sequence><xs:element ref="t:field"/></xs:sequence></xs:group><xs:element name="field" type="xs:integer"/>`)
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
