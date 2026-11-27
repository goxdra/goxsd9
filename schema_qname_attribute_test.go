package goxsd9

import (
	"errors"
	"reflect"
	"testing"
)

func qnameAttributeGraph(version XSDVersion) (string, map[string]discoveryFixture) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="` + string(version) + `">
  <xs:include schemaLocation="included.xsd"/>
  <xs:include schemaLocation="included.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:attribute name="direct" type="xs:QName"/>
  <xs:attribute name="forward" type="r:Child"/>
  <xs:attribute name="included" type="r:Included"/>
  <xs:attribute name="imported" type="o:Imported"/>
  <xs:attribute name="chameleon" type="r:Chameleon"/>
  <xs:simpleType name="Child"><xs:restriction base="r:Alias"/></xs:simpleType>
  <xs:simpleType name="Alias"><xs:restriction base="xs:QName"/></xs:simpleType>
</xs:schema>`
	fixtures := map[string]discoveryFixture{
		"root.xsd":      {id: "root.xsd", contents: root},
		"included.xsd":  {id: "included.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root"><xs:include schemaLocation="root.xsd"/><xs:simpleType name="Included"><xs:restriction base="xs:QName"/></xs:simpleType><xs:attribute name="includedDirect" type="xs:QName"/></xs:schema>`},
		"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Chameleon"><xs:restriction base="xs:QName"/></xs:simpleType><xs:attribute name="chameleonDirect" type="xs:QName"/></xs:schema>`},
		"other.xsd":     {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Imported"><xs:restriction base="xs:QName"/></xs:simpleType><xs:attribute name="importedDirect" type="xs:QName"/></xs:schema>`},
	}
	return root, fixtures
}

//nolint:gocognit,funlen // One graph checks identity, provenance, order, and copied public facts.
func TestQNameGlobalAttributeFactsAcrossPolicies(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root, fixtures := qnameAttributeGraph(profile.version)
			first, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("ParseSchema: %v", err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil || !reflect.DeepEqual(first.Components(), second.Components()) {
				t.Fatalf("repeated graph facts changed: %v", err)
			}
			if first.LanguagePolicy() != profile.policy || len(first.Documents()) != 4 {
				t.Fatalf("policy/documents = %v/%d", first.LanguagePolicy(), len(first.Documents()))
			}
			want := []struct {
				local, namespace, source, written, typeNamespace, typeLocal string
			}{
				{"direct", "urn:root", "root.xsd", "xs:QName", testXSDNamespace, "QName"},
				{"forward", "urn:root", "root.xsd", "r:Child", "urn:root", "Child"},
				{"included", "urn:root", "root.xsd", "r:Included", "urn:root", "Included"},
				{"imported", "urn:root", "root.xsd", "o:Imported", "urn:other", "Imported"},
				{"chameleon", "urn:root", "root.xsd", "r:Chameleon", "urn:root", "Chameleon"},
				{"includedDirect", "urn:root", "included.xsd", "xs:QName", testXSDNamespace, "QName"},
				{"chameleonDirect", "urn:root", "chameleon.xsd", "xs:QName", testXSDNamespace, "QName"},
				{"importedDirect", "urn:other", "other.xsd", "xs:QName", testXSDNamespace, "QName"},
			}
			var attributes []Component
			for _, component := range first.Components() {
				if component.Kind() == ComponentKindAttributeDeclaration {
					attributes = append(attributes, component)
				}
			}
			if len(attributes) != len(want) {
				t.Fatalf("attributes = %d, want %d", len(attributes), len(want))
			}
			for index, expected := range want {
				component := attributes[index]
				name := mustTestQName(t, expected.namespace, expected.local)
				typeName := mustTestQName(t, expected.typeNamespace, expected.typeLocal)
				if component.Name() != name || component.ID().Source() != SourceID(expected.source) {
					t.Fatalf("attribute %d identity = %q/%v", index, component.Name(), component.ID())
				}
				declaration, ok := component.AttributeDeclaration()
				if !ok || declaration.ID() != component.ID() || declaration.Name() != name {
					t.Fatalf("attribute %s declaration = %#v/%t", expected.local, declaration, ok)
				}
				declarationLoc := schemaBuiltinReferenceAttributeLoc(t, SourceID(expected.source), `<xs:attribute name="`+expected.local+`"`, root, fixtures)
				typeLoc := schemaBuiltinReferenceAttributeLoc(t, SourceID(expected.source), `type="`+expected.written+`"`, root, fixtures)
				if component.Loc() != declarationLoc || declaration.Loc() != declarationLoc || declaration.DeclaredType() != typeName {
					t.Fatalf("attribute %s location/type = %s/%s/%q", expected.local, component.Loc(), declaration.Loc(), declaration.DeclaredType())
				}
				reference, ok := declaration.TypeReference()
				if !ok || reference.Name() != typeName || reference.QName() != typeName || reference.Loc() != typeLoc || reference.Variety() != SimpleTypeVarietyAtomicRestriction {
					t.Fatalf("attribute %s reference = %#v/%t", expected.local, reference, ok)
				}
				if _, hasBounds := reference.IntegerBounds(); hasBounds {
					t.Fatalf("QName attribute %s exposed integer bounds", expected.local)
				}
				if expected.typeNamespace == testXSDNamespace {
					assertQNameBuiltin(t, reference, typeLoc)
					if id, hasID := declaration.TypeID(); hasID || !id.IsZero() {
						t.Fatalf("built-in attribute %s TypeID = %v/%t", expected.local, id, hasID)
					}
					continue
				}
				wantID := componentIDForName(t, first, typeName)
				id, hasID := declaration.TypeID()
				refID, hasRefID := reference.ComponentID()
				if !reference.IsNamed() || !hasID || !hasRefID || id != wantID || refID != wantID {
					t.Fatalf("named attribute %s target = %v/%t, reference %v/%t", expected.local, id, hasID, refID, hasRefID)
				}
				found := first.FindKind(ComponentKindSimpleTypeDefinition, typeName)
				definition, ok := found[0].SimpleTypeDefinition()
				if len(found) != 1 || !ok || reference.VarietyLoc() != definition.VarietyLoc() {
					t.Fatalf("named attribute %s variety provenance = %s", expected.local, reference.VarietyLoc())
				}
			}
			components := first.Components()
			components[0] = Component{}
			found := first.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:root", "forward"))
			found[0] = Component{}
			documentComponents := first.Documents()[0].Components()
			documentComponents[0] = Component{}
			if !reflect.DeepEqual(first.Components(), second.Components()) {
				t.Fatal("mutating returned slices changed schema facts")
			}
			var walked []ComponentID
			if err := first.Walk(func(component Component) error { walked = append(walked, component.ID()); return nil }); err != nil {
				t.Fatalf("Walk: %v", err)
			}
			for index, component := range first.Components() {
				if walked[index] != component.ID() {
					t.Fatalf("Walk order %d = %v, want %v", index, walked[index], component.ID())
				}
			}
		})
	}
}

//nolint:gocognit // Direct and named use/generator exits share target provenance.
func TestQNameGlobalAttributeReferenceAndConsumerStayUnsupported(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, named := range []bool{false, true} {
			name := "direct"
			typeName := "xs:QName"
			alias := ""
			if named {
				name = "named"
				typeName = "t:Alias"
				alias = `<xs:simpleType name="Alias"><xs:restriction base="xs:QName"/></xs:simpleType>`
			}
			t.Run(profile.name+"/"+name, func(t *testing.T) {
				root := qnameSchema(`<xs:attribute name="a" type="` + typeName + `"/>` + alias + `<xs:complexType name="Box"><xs:attribute ref="t:a"/></xs:complexType>`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil {
					t.Fatalf("attribute use returned schema: %v", err)
				}
				d := requireDiagnostic(t, err)
				wantRelated := []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="a"`)}
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `ref="t:a"`) || !reflect.DeepEqual(d.Related(), wantRelated) || d.SpecRef() != schemaAttributeUseSpecRef(profile.version) || !errors.Is(err, errSchemaAttributeReferenceUnsupported) {
					t.Fatalf("attribute use diagnostic = %v, related %v", d, d.Related())
				}
				root = qnameSchema(`<xs:attribute name="a" type="` + typeName + `"/>` + alias)
				schema, err = discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("standalone attribute: %v", err)
				}
				output, err := GenerateGo(schema, "generated")
				if err == nil || output != nil {
					t.Fatalf("GenerateGo = %q/%v", output, err)
				}
				d = requireDiagnostic(t, err)
				if d.Class() != FailureUnsupported || d.Code() != diagnosticCodegenUnsupported || d.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="a"`) || !errors.Is(err, errCodegenUnsupported) {
					t.Fatalf("generation diagnostic = %v", d)
				}
			})
		}
	}
}

func TestQNameGlobalAttributeExcludedFormsAcrossPolicies(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct {
			name, body, needle, spec string
			cause                    error
		}{
			{"local direct", `<xs:complexType name="Box"><xs:attribute name="a" type="xs:QName"/></xs:complexType>`, `type="xs:QName"`, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnsupported},
			{"local named", `<xs:simpleType name="Alias"><xs:restriction base="xs:QName"/></xs:simpleType><xs:complexType name="Box"><xs:attribute name="a" type="t:Alias"/></xs:complexType>`, `type="t:Alias"`, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnsupported},
			{"local inline", `<xs:complexType name="Box"><xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:QName"/></xs:simpleType></xs:attribute></xs:complexType>`, `<xs:simpleType`, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnsupported},
			{"global inline", `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:QName"/></xs:simpleType></xs:attribute>`, `<xs:simpleType`, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnsupported},
			{"named list", `<xs:attribute name="a" type="t:List"/><xs:simpleType name="List"><xs:list itemType="xs:QName"/></xs:simpleType>`, `type="t:List"`, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnsupported},
			{"named union", `<xs:attribute name="a" type="t:Union"/><xs:simpleType name="Union"><xs:union memberTypes="xs:QName"/></xs:simpleType>`, `type="t:Union"`, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnsupported},
			{"direct default", `<xs:attribute name="a" type="xs:QName" default="t:v"/>`, `default="t:v"`, schemaAttributeValueConstraintSpecRef(profile.version), errSchemaAttributeValueConstraintUnsupported},
			{"direct fixed", `<xs:attribute name="a" type="xs:QName" fixed="t:v"/>`, `fixed="t:v"`, schemaAttributeValueConstraintSpecRef(profile.version), errSchemaAttributeValueConstraintUnsupported},
			{"named default", `<xs:attribute name="a" type="t:Alias" default="t:v"/><xs:simpleType name="Alias"><xs:restriction base="xs:QName"/></xs:simpleType>`, `default="t:v"`, schemaAttributeValueConstraintSpecRef(profile.version), errSchemaAttributeValueConstraintUnsupported},
			{"named fixed", `<xs:attribute name="a" type="t:Alias" fixed="t:v"/><xs:simpleType name="Alias"><xs:restriction base="xs:QName"/></xs:simpleType>`, `fixed="t:v"`, schemaAttributeValueConstraintSpecRef(profile.version), errSchemaAttributeValueConstraintUnsupported},
			{"inline default", `<xs:attribute name="a" default="t:v"><xs:simpleType><xs:restriction base="xs:QName"/></xs:simpleType></xs:attribute>`, `default="t:v"`, schemaAttributeValueConstraintSpecRef(profile.version), errSchemaAttributeValueConstraintUnsupported},
			{"inline fixed", `<xs:attribute name="a" fixed="t:v"><xs:simpleType><xs:restriction base="xs:QName"/></xs:simpleType></xs:attribute>`, `fixed="t:v"`, schemaAttributeValueConstraintSpecRef(profile.version), errSchemaAttributeValueConstraintUnsupported},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := qnameSchema(test.body)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil {
					t.Fatalf("excluded QName attribute returned schema: %v", err)
				}
				d := requireDiagnostic(t, err)
				want := elementReferenceTestAttributeLoc(t, root, test.needle)
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != want || d.SpecRef() != test.spec || !errors.Is(err, ErrUnsupported) || !errors.Is(err, test.cause) {
					t.Fatalf("diagnostic = %v, want unsupported at %s with %v and %s", d, want, test.cause, test.spec)
				}
			})
		}
	}
}

//nolint:gocognit // Reference, syntax, and duplicate exits share the admitted declaration boundary.
func TestQNameGlobalAttributeAlternateDiagnostics(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct {
			name, body, primary, related, code, spec string
			cause                                    error
		}{
			{"missing", `<xs:attribute name="a" type="t:Missing"/><xs:simpleType name="Q"><xs:restriction base="xs:QName"/></xs:simpleType>`, `type="t:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved},
			{"wrong kind", `<xs:element name="Target" type="xs:QName"/><xs:attribute name="a" type="t:Target"/>`, `type="t:Target"`, `<xs:element name="Target"`, diagnosticSchemaAttributeTypeWrongKindCode, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeWrongKind},
			{"cycle", `<xs:attribute name="a" type="t:A"/><xs:simpleType name="A"><xs:restriction base="t:B"/></xs:simpleType><xs:simpleType name="B"><xs:restriction base="t:A"/></xs:simpleType><xs:simpleType name="Seed"><xs:restriction base="xs:QName"/></xs:simpleType>`, `type="t:A"`, `<xs:attribute name="a"`, diagnosticSchemaAttributeTypeCycleCode, schemaAttributeTypeSpecRef(profile.version), errSchemaSimpleTypeBaseCycle},
			{"duplicate", `<xs:attribute name="a" type="t:Alias"/><xs:simpleType name="Alias"><xs:restriction base="xs:QName"/></xs:simpleType><xs:simpleType name="Alias"><xs:restriction base="t:Seed"/></xs:simpleType><xs:simpleType name="Seed"><xs:restriction base="xs:QName"/></xs:simpleType>`, `<xs:simpleType name="Alias"><xs:restriction base="t:Seed"`, `<xs:simpleType name="Alias"><xs:restriction base="xs:QName"`, diagnosticSchemaGlobalDuplicateCode, schemaGlobalDuplicateSpecRef(profile.version), errSchemaGlobalDeclarationDuplicate},
			{"malformed", `<xs:attribute name="a" type="t:bad:QName"/>`, `type="t:bad:QName"`, "", invalidSchemaConditionalCode, "", nil},
			{"unbound", `<xs:attribute name="a" type="bad:QName"/>`, `type="bad:QName"`, "", invalidSchemaConditionalCode, "", nil},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := qnameSchema(test.body)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil {
					t.Fatalf("invalid QName attribute returned schema: %v", err)
				}
				d := requireDiagnostic(t, err)
				if d.Class() != FailureInvalid || d.Code() != test.code || d.Loc() != elementReferenceTestAttributeLoc(t, root, test.primary) || d.SpecRef() != test.spec {
					t.Fatalf("diagnostic = %v, want %s at %s spec %s", d, test.code, test.primary, test.spec)
				}
				if test.related == "" && len(d.Related()) != 0 {
					t.Fatalf("unexpected related locations: %v", d.Related())
				}
				if test.related != "" && !schemaLocationListContains(d.Related(), elementReferenceTestAttributeLoc(t, root, test.related)) {
					t.Fatalf("related locations = %v, missing %s", d.Related(), test.related)
				}
				if test.cause != nil && !errors.Is(err, test.cause) {
					t.Fatalf("diagnostic lost %v: %v", test.cause, err)
				}
			})
		}
	}
}

func TestQNameGlobalAttributeInvisibleGraphType(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:f="urn:foreign" targetNamespace="urn:root"><xs:include schemaLocation="child.xsd"/><xs:attribute name="a" type="f:Hidden"/><xs:simpleType name="Seed"><xs:restriction base="xs:QName"/></xs:simpleType></xs:schema>`
			fixtures := map[string]discoveryFixture{
				"child.xsd":   {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:import namespace="urn:foreign" schemaLocation="foreign.xsd"/></xs:schema>`},
				"foreign.xsd": {id: "foreign.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:foreign"><xs:simpleType name="Hidden"><xs:restriction base="xs:QName"/></xs:simpleType></xs:schema>`},
			}
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err == nil || schema.storage != nil {
				t.Fatalf("invisible QName target returned schema: %v", err)
			}
			d := requireDiagnostic(t, err)
			if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaAttributeTypeUnresolvedCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `type="f:Hidden"`) || len(d.Related()) != 0 || d.SpecRef() != schemaAttributeTypeSpecRef(profile.version) || !errors.Is(err, errSchemaAttributeTypeUnresolved) {
				t.Fatalf("invisible QName target diagnostic = %v, related %v", d, d.Related())
			}
		})
	}
}

func TestQNameGlobalAttributeFacetPrecedesPublication(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := qnameSchema(`<xs:attribute name="a" type="t:Alias"/><xs:simpleType name="Alias"><xs:restriction base="xs:QName"><xs:enumeration value="t:v"/></xs:restriction></xs:simpleType>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err == nil || schema.storage != nil {
				t.Fatalf("faceted QName attribute returned schema: %v", err)
			}
			d := requireDiagnostic(t, err)
			if d.Class() != FailureUnsupported || d.Code() != UnsupportedDatatypeFacetCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:enumeration`) || len(d.Related()) != 0 || d.SpecRef() != qnameFacetSpecRef(profile.version) || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("QName facet diagnostic = %v, related %v", d, d.Related())
			}
		})
	}
}
