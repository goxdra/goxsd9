package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func qnameSchema(body string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test">` + body + `</xs:schema>`
}

func qnameDefinition(t *testing.T, schema Schema, local string) SimpleTypeDefinition {
	t.Helper()
	return schemaEnumerationTestDefinitionInNamespace(t, schema, "urn:test", local)
}

func assertQNameBuiltin(t *testing.T, reference SimpleTypeReference, loc Loc) {
	t.Helper()
	want := mustTestQName(t, testXSDNamespace, "QName")
	if !reference.IsBuiltin() || reference.Name() != want || reference.QName() != want || reference.Variety() != SimpleTypeVarietyAtomicRestriction || reference.Loc() != loc || reference.VarietyLoc() != loc {
		t.Fatalf("QName reference = %#v, want built-in atomic at %s", reference, loc)
	}
	if reference.facts == nil || reference.facts.atomicKind != schemaSimpleTypeAtomicQName {
		t.Fatalf("QName atomic kind = %#v", reference.facts)
	}
	if _, ok := reference.facts.facets.(schemaAtomicFacetVariant); !ok {
		t.Fatalf("QName facet representation = %T", reference.facts.facets)
	}
	if id, ok := reference.ComponentID(); ok || !id.IsZero() {
		t.Fatalf("built-in has synthetic ID %v/%t", id, ok)
	}
	if id, ok := reference.AnonymousID(); ok || !id.IsZero() {
		t.Fatalf("built-in has anonymous ID %v/%t", id, ok)
	}
}

//nolint:gocognit,funlen // The public reference positions and copied views form one admission contract.
func TestQNameReferencesAcrossPolicies(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := qnameSchema(`
  <xs:element name="direct" type="xs:QName"/>
  <xs:element name="named" type="t:Alias"/>
  <xs:element name="inline"><xs:simpleType><xs:restriction base="xs:QName"/></xs:simpleType></xs:element>
  <xs:simpleType name="Alias"><xs:restriction base="xs:QName"/></xs:simpleType>
  <xs:simpleType name="Child"><xs:restriction base="t:Alias"/></xs:simpleType>
  <xs:simpleType name="List"><xs:list itemType="xs:QName"/></xs:simpleType>
  <xs:simpleType name="NamedList"><xs:list itemType="t:Alias"/></xs:simpleType>
  <xs:simpleType name="Union"><xs:union memberTypes="xs:QName t:Alias"><xs:simpleType><xs:restriction base="xs:QName"/></xs:simpleType></xs:union></xs:simpleType>
`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("ParseSchema: %v", err)
			}
			if schema.LanguagePolicy() != profile.policy || len(schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, testXSDNamespace, "QName"))) != 0 {
				t.Fatal("policy or built-in component identity changed")
			}
			components := schema.Components()
			wantOrder := []string{"direct", "named", "inline", "Alias", "Child", "List", "NamedList", "Union"}
			if len(components) != len(wantOrder) {
				t.Fatalf("components = %d, want %d", len(components), len(wantOrder))
			}
			for index, local := range wantOrder {
				if components[index].Name() != mustTestQName(t, "urn:test", local) {
					t.Fatalf("component %d = %q, want %s", index, components[index].Name(), local)
				}
			}
			direct := tokenElementDefinition(t, schema, "direct")
			directRef, ok := direct.TypeReference()
			if !ok || direct.DeclaredType() != mustTestQName(t, testXSDNamespace, "QName") {
				t.Fatal("direct QName type missing")
			}
			assertQNameBuiltin(t, directRef, mustSchemaTokenLoc(t, "root.xsd", root, 2, `type="xs:QName"`))
			if id, hasTypeID := direct.TypeID(); hasTypeID || !id.IsZero() {
				t.Fatalf("direct built-in type ID = %v/%t", id, hasTypeID)
			}
			alias := qnameDefinition(t, schema, "Alias")
			if alias.IsString() || alias.Variety() != SimpleTypeVarietyAtomicRestriction {
				t.Fatal("QName alias was treated as string or non-atomic")
			}
			namedElement := tokenElementDefinition(t, schema, "named")
			namedElementRef, hasNamedElementRef := namedElement.TypeReference()
			aliasName := mustTestQName(t, "urn:test", "Alias")
			if !hasNamedElementRef || !namedElementRef.IsNamed() || namedElementRef.Name() != aliasName || namedElementRef.Variety() != SimpleTypeVarietyAtomicRestriction || namedElementRef.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 3, `type="t:Alias"`) {
				t.Fatalf("named global reference = %#v/%t", namedElementRef, hasNamedElementRef)
			}
			if id, hasID := namedElementRef.ComponentID(); !hasID || id != componentIDForName(t, schema, aliasName) {
				t.Fatalf("named global reference ID = %v/%t", id, hasID)
			}
			if id, hasID := namedElement.TypeID(); !hasID || id != componentIDForName(t, schema, aliasName) {
				t.Fatalf("named global TypeID = %v/%t", id, hasID)
			}
			base, ok := alias.BaseReference()
			if !ok {
				t.Fatal("QName base missing")
			}
			assertQNameBuiltin(t, base, mustSchemaTokenLoc(t, "root.xsd", root, 5, `base="xs:QName"`))
			childBase, ok := qnameDefinition(t, schema, "Child").BaseReference()
			if !ok || !childBase.IsNamed() || childBase.Name() != aliasName || childBase.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 6, `base="t:Alias"`) {
				t.Fatalf("Child base = %#v/%t", childBase, ok)
			}
			if id, hasID := childBase.ComponentID(); !hasID || id != componentIDForName(t, schema, aliasName) {
				t.Fatalf("Child target ID = %v/%t", id, hasID)
			}
			item, ok := qnameDefinition(t, schema, "List").ItemType()
			if !ok {
				t.Fatal("list item missing")
			}
			assertQNameBuiltin(t, item, mustSchemaTokenLoc(t, "root.xsd", root, 7, `itemType="xs:QName"`))
			namedItem, ok := qnameDefinition(t, schema, "NamedList").ItemType()
			if !ok || !namedItem.IsNamed() || namedItem.Name() != aliasName || namedItem.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 8, `itemType="t:Alias"`) {
				t.Fatalf("named list item = %#v/%t", namedItem, ok)
			}
			if id, hasID := namedItem.ComponentID(); !hasID || id != componentIDForName(t, schema, aliasName) {
				t.Fatalf("named list ID = %v/%t", id, hasID)
			}
			members := qnameDefinition(t, schema, "Union").MemberTypes()
			if len(members) != 3 {
				t.Fatalf("union members = %d", len(members))
			}
			assertQNameBuiltin(t, members[0], mustSchemaTokenLoc(t, "root.xsd", root, 9, `memberTypes="`))
			if !members[1].IsNamed() || members[1].Name() != aliasName || !members[2].IsAnonymous() {
				t.Fatalf("union lexical order = %#v", members)
			}
			inlineUnion, ok := members[2].AnonymousType()
			if !ok {
				t.Fatal("union inline member missing")
			}
			inlineUnionBase, ok := inlineUnion.BaseReference()
			if !ok {
				t.Fatal("union inline base missing")
			}
			assertQNameBuiltin(t, inlineUnionBase, mustSchemaTokenLoc(t, "root.xsd", root, 9, `base="xs:QName"`))
			inlineRef, ok := tokenElementDefinition(t, schema, "inline").TypeReference()
			if !ok || !inlineRef.IsAnonymous() {
				t.Fatal("inline global QName type missing")
			}
			inlineDef, ok := inlineRef.AnonymousType()
			if !ok {
				t.Fatal("inline type view missing")
			}
			inlineBase, ok := inlineDef.BaseReference()
			if !ok {
				t.Fatal("inline type base missing")
			}
			assertQNameBuiltin(t, inlineBase, mustSchemaTokenLoc(t, "root.xsd", root, 4, `base="xs:QName"`))
			members[0] = SimpleTypeReference{}
			components[0] = Component{}
			if !qnameDefinition(t, schema, "Union").MemberTypes()[0].IsBuiltin() || schema.Components()[0].Name().Local() != "direct" {
				t.Fatal("returned views modified Schema")
			}
			var walked []ComponentID
			if err := schema.Walk(func(component Component) error { walked = append(walked, component.ID()); return nil }); err != nil {
				t.Fatalf("Walk: %v", err)
			}
			for index, component := range schema.Components() {
				if walked[index] != component.ID() {
					t.Fatalf("walk order %d changed", index)
				}
			}
		})
	}
}

//nolint:gocognit // All graph provenance claims use the same fixture.
func TestQNameGraphReferencesAcrossPolicies(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" xmlns:o="urn:other" targetNamespace="urn:test">
  <xs:include schemaLocation="included.xsd"/>
  <xs:include schemaLocation="included.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:simpleType name="Forward"><xs:restriction base="t:Later"/></xs:simpleType>
  <xs:simpleType name="Later"><xs:restriction base="xs:QName"/></xs:simpleType>
  <xs:simpleType name="FromIncluded"><xs:restriction base="t:Included"/></xs:simpleType>
  <xs:simpleType name="FromImported"><xs:restriction base="o:Imported"/></xs:simpleType>
</xs:schema>`
			included := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Included"><xs:restriction base="xs:QName"/></xs:simpleType></xs:schema>`
			imported := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Imported"><xs:restriction base="xs:QName"/></xs:simpleType></xs:schema>`
			fixtures := map[string]discoveryFixture{
				"included.xsd": {id: "included.xsd", contents: included},
				"other.xsd":    {id: "other.xsd", contents: imported},
			}
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("ParseSchema: %v", err)
			}
			want := []QName{
				mustTestQName(t, "urn:test", "Forward"), mustTestQName(t, "urn:test", "Later"),
				mustTestQName(t, "urn:test", "FromIncluded"), mustTestQName(t, "urn:test", "FromImported"),
				mustTestQName(t, "urn:test", "Included"), mustTestQName(t, "urn:other", "Imported"),
			}
			if len(schema.Components()) != len(want) {
				t.Fatalf("components = %d, want %d", len(schema.Components()), len(want))
			}
			for index, component := range schema.Components() {
				if component.Name() != want[index] {
					t.Fatalf("component %d = %q, want %q", index, component.Name(), want[index])
				}
			}
			for _, test := range []struct{ name, target, source, lexical string }{
				{"Forward", "Later", "root.xsd", `base="t:Later"`},
				{"FromIncluded", "Included", "included.xsd", `base="t:Included"`},
				{"FromImported", "Imported", "other.xsd", `base="o:Imported"`},
			} {
				base, ok := qnameDefinition(t, schema, test.name).BaseReference()
				if !ok || !base.IsNamed() || base.Name().Local() != test.target || base.Loc() != elementReferenceTestAttributeLoc(t, root, test.lexical) {
					t.Fatalf("%s base = %#v/%t", test.name, base, ok)
				}
				id, ok := base.ComponentID()
				if !ok || id.Source() != SourceID(test.source) || id != componentIDForName(t, schema, base.Name()) {
					t.Fatalf("%s target ID = %v/%t", test.name, id, ok)
				}
			}
			for _, test := range []struct{ name, namespace, source, document, lexical string }{
				{"Later", "urn:test", "root.xsd", root, `base="xs:QName"`},
				{"Included", "urn:test", "included.xsd", included, `base="xs:QName"`},
				{"Imported", "urn:other", "other.xsd", imported, `base="xs:QName"`},
			} {
				found := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, test.namespace, test.name))
				if len(found) != 1 {
					t.Fatalf("%s count = %d", test.name, len(found))
				}
				definition, ok := found[0].SimpleTypeDefinition()
				if !ok {
					t.Fatalf("%s definition missing", test.name)
				}
				base, ok := definition.BaseReference()
				if !ok {
					t.Fatalf("%s base missing", test.name)
				}
				loc := elementReferenceTestAttributeLoc(t, test.document, test.lexical)
				if test.source != "root.xsd" {
					loc = mustSchemaTokenLoc(t, SourceID(test.source), test.document, 1, test.lexical)
				}
				assertQNameBuiltin(t, base, loc)
			}
			repeated, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil || !reflect.DeepEqual(schema.Components(), repeated.Components()) {
				t.Fatalf("repeated graph changed order/facts: %v", err)
			}
		})
	}
}

func TestQNameExcludedSchemaShapes(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct {
			name, body, mark, code, spec string
		}{
			{"local direct", `<xs:complexType name="Box"><xs:sequence><xs:element name="a" type="xs:QName"/></xs:sequence></xs:complexType>`, `type="xs:QName"`, UnsupportedSchemaSyntaxCode, schemaSyntaxSpecRefForVersion(profile.version)},
			{"local named", `<xs:complexType name="Box"><xs:sequence><xs:element name="a" type="t:Alias"/></xs:sequence></xs:complexType><xs:simpleType name="Alias"><xs:restriction base="xs:QName"/></xs:simpleType>`, `type="t:Alias"`, UnsupportedSchemaSyntaxCode, schemaSyntaxSpecRefForVersion(profile.version)},
			{"local inline", `<xs:complexType name="Box"><xs:sequence><xs:element name="a"><xs:simpleType><xs:restriction base="xs:QName"/></xs:simpleType></xs:element></xs:sequence></xs:complexType>`, `<xs:simpleType`, UnsupportedSchemaSyntaxCode, schemaSyntaxSpecRefForVersion(profile.version)},
			{"global attribute direct", `<xs:attribute name="a" type="xs:QName"/>`, `type="xs:QName"`, UnsupportedSchemaSyntaxCode, schemaAttributeTypeSpecRef(profile.version)},
			{"global attribute named", `<xs:attribute name="a" type="t:Alias"/><xs:simpleType name="Alias"><xs:restriction base="xs:QName"/></xs:simpleType>`, `type="t:Alias"`, UnsupportedSchemaSyntaxCode, schemaAttributeTypeSpecRef(profile.version)},
			{"global attribute inline", `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:QName"/></xs:simpleType></xs:attribute>`, `<xs:simpleType`, UnsupportedSchemaSyntaxCode, schemaAttributeTypeSpecRef(profile.version)},
			{"global attribute ref", `<xs:complexType name="Box"><xs:attribute ref="t:a"/></xs:complexType><xs:attribute name="a" type="xs:QName"/>`, `type="xs:QName"`, UnsupportedSchemaSyntaxCode, schemaAttributeTypeSpecRef(profile.version)},
			{"enumeration facet", `<xs:simpleType name="Alias"><xs:restriction base="xs:QName"><xs:enumeration value="t:a"/></xs:restriction></xs:simpleType>`, `<xs:enumeration`, UnsupportedDatatypeFacetCode, qnameFacetSpecRef(profile.version)},
			{"pattern facet", `<xs:simpleType name="Alias"><xs:restriction base="xs:QName"><xs:pattern value=".*"/></xs:restriction></xs:simpleType>`, `<xs:pattern`, UnsupportedDatatypeFacetCode, qnameFacetSpecRef(profile.version)},
			{"whitespace facet", `<xs:simpleType name="Alias"><xs:restriction base="xs:QName"><xs:whiteSpace value="collapse"/></xs:restriction></xs:simpleType>`, `<xs:whiteSpace`, UnsupportedDatatypeFacetCode, qnameFacetSpecRef(profile.version)},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := qnameSchema("\n  " + test.body + "\n")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil {
					t.Fatalf("excluded shape returned schema or no error: %v", err)
				}
				d := requireDiagnostic(t, err)
				want := mustSchemaTokenLoc(t, "root.xsd", root, 2, test.mark)
				if d.Class() != FailureUnsupported || d.Code() != test.code || d.Loc() != want || d.SpecRef() != test.spec || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("diagnostic = %v; want %s at %s spec %s", d, test.code, want, test.spec)
				}
			})
		}
	}
}

//nolint:gocognit // Each composite route to the local precisionDecimal exception is observable.
func TestQNameBearingPrecisionUnionsRejectNonzeroLocals(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		if profile.policy == Strict10 {
			continue
		}
		for _, test := range []struct{ name, definitions, local, mark string }{
			{"direct member", `<xs:simpleType name="U"><xs:union memberTypes="xs:precisionDecimal xs:QName"/></xs:simpleType>`, `<xs:element name="a" type="t:U"/>`, `type="t:U"`},
			{"named member", `<xs:simpleType name="Q"><xs:restriction base="xs:QName"/></xs:simpleType><xs:simpleType name="U"><xs:union memberTypes="xs:precisionDecimal t:Q"/></xs:simpleType>`, `<xs:element name="a" type="t:U"/>`, `type="t:U"`},
			{"named list member", `<xs:simpleType name="QList"><xs:list itemType="xs:QName"/></xs:simpleType><xs:simpleType name="U"><xs:union memberTypes="xs:precisionDecimal t:QList"/></xs:simpleType>`, `<xs:element name="a" type="t:U"/>`, `type="t:U"`},
			{"inline list member", `<xs:simpleType name="U"><xs:union memberTypes="xs:precisionDecimal"><xs:simpleType><xs:list itemType="xs:QName"/></xs:simpleType></xs:union></xs:simpleType>`, `<xs:element name="a" type="t:U"/>`, `type="t:U"`},
			{"direct list", `<xs:simpleType name="QList"><xs:list itemType="xs:QName"/></xs:simpleType>`, `<xs:element name="a" type="t:QList"/>`, `type="t:QList"`},
			{"inline union", `<xs:simpleType name="U"><xs:union memberTypes="xs:precisionDecimal xs:QName"/></xs:simpleType>`, `<xs:element name="a"><xs:simpleType><xs:union memberTypes="xs:precisionDecimal xs:QName"/></xs:simpleType></xs:element>`, `<xs:simpleType`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := qnameSchema("\n  " + test.definitions + `<xs:complexType name="Box"><xs:sequence>` + test.local + `</xs:sequence></xs:complexType>` + "\n")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil {
					t.Fatalf("QName-bearing local returned schema/no error: %v", err)
				}
				d := requireDiagnostic(t, err)
				want := mustSchemaTokenLoc(t, "root.xsd", root, 2, test.mark)
				wantSpec := schemaSyntaxSpecRefForVersion(profile.version)
				if test.name == "inline union" {
					want = mustTestLoc(t, "root.xsd", 2, strings.LastIndex(strings.Split(root, "\n")[1], `<xs:simpleType`)+1)
					wantSpec = "xsd10-structures#schema-document"
				}
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != want || d.SpecRef() != wantSpec || len(d.Related()) != 0 || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("diagnostic = %v related=%v spec=%s; want unsupported at %s spec=%s", d, d.Related(), d.SpecRef(), want, wantSpec)
				}
			})
		}
	}
}

func qnameSharedUnionSchema(baseMember, topMember string) string {
	definitions := []string{`<xs:simpleType name="U0"><xs:union memberTypes="xs:string ` + baseMember + `"/></xs:simpleType>`}
	for index := 1; index <= 28; index++ {
		previous := "t:U" + strconv.Itoa(index-1)
		definitions = append(definitions, `<xs:simpleType name="U`+strconv.Itoa(index)+`"><xs:union memberTypes="`+previous+` `+previous+`"/></xs:simpleType>`)
	}
	definitions = append(definitions, `<xs:simpleType name="Top"><xs:union memberTypes="xs:precisionDecimal t:U28`+topMember+`"/></xs:simpleType>`)
	return qnameSchema("\n  " + strings.Join(definitions, "") + `<xs:complexType name="Box"><xs:sequence><xs:element name="a" type="t:Top"/></xs:sequence></xs:complexType>` + "\n")
}

//nolint:gocognit // The shared graph must preserve both local admission and rejection observables.
func TestQNameSharedUnionGraphLocalAdmission(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		if profile.policy == Strict10 {
			continue
		}
		for _, test := range []struct {
			name, baseMember, topMember string
			containsQName               bool
		}{
			{"QName-free shared graph", "xs:boolean", "", false},
			{"QName after shared graph", "xs:boolean", " xs:QName", true},
			{"QName in shared leaf", "xs:QName", "", true},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := qnameSharedUnionSchema(test.baseMember, test.topMember)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				localLoc := mustSchemaTokenLoc(t, "root.xsd", root, 2, `type="t:Top"`)
				if test.containsQName {
					if err == nil || schema.storage != nil {
						t.Fatalf("QName-bearing shared graph returned schema/no error: %v", err)
					}
					d := requireDiagnostic(t, err)
					if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != localLoc || d.SpecRef() != schemaSyntaxSpecRefForVersion(profile.version) || len(d.Related()) != 0 || !errors.Is(err, ErrUnsupported) {
						t.Fatalf("shared graph diagnostic = %v related=%v spec=%s", d, d.Related(), d.SpecRef())
					}
					return
				}
				if err != nil {
					t.Fatalf("QName-free shared graph: %v", err)
				}
				components := schema.Components()
				if len(components) != 31 || components[30].Name() != mustTestQName(t, "urn:test", "Box") {
					t.Fatalf("shared graph component order/count = %#v", components)
				}
				box, ok := components[30].ComplexTypeDefinition()
				if !ok {
					t.Fatal("shared graph Box has no complex type view")
				}
				sequence, ok := box.Particle().(SequenceParticle)
				if !ok || len(sequence.Particles()) != 1 {
					t.Fatalf("shared graph Box particle = %T", box.Particle())
				}
				local, ok := sequence.Particles()[0].(ElementParticle)
				if !ok || local.Loc() == (Loc{}) || local.DeclaredType() != mustTestQName(t, "urn:test", "Top") {
					t.Fatalf("shared graph local particle = %#v/%t", sequence.Particles()[0], ok)
				}
				reference, ok := local.TypeReference()
				if !ok || !reference.IsNamed() || reference.Variety() != SimpleTypeVarietyUnion || reference.Loc() != localLoc {
					t.Fatalf("shared graph local type reference = %#v/%t", reference, ok)
				}
				if id, hasID := reference.ComponentID(); !hasID || id != componentIDForName(t, schema, reference.Name()) {
					t.Fatalf("shared graph local type ID = %v/%t", id, hasID)
				}
			})
		}
	}
}

//nolint:gocognit // Standalone identity, omission, and reference targets are separate admission shapes.
func TestQNameBearingPrecisionUnionIdentityAndOmission(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		if profile.policy == Strict10 {
			continue
		}
		root := qnameSchema(`
  <xs:simpleType name="Q"><xs:restriction base="xs:QName"/></xs:simpleType>
  <xs:simpleType name="U"><xs:union memberTypes="xs:precisionDecimal t:Q"/></xs:simpleType>
  <xs:simpleType name="QList"><xs:list itemType="t:Q"/></xs:simpleType>
  <xs:element name="item" type="xs:QName"/>
  <xs:complexType name="Box"><xs:sequence><xs:element name="omitted" type="t:U" minOccurs="0" maxOccurs="0"/></xs:sequence></xs:complexType>
  <xs:complexType name="Refs"><xs:choice><xs:element ref="t:item"/></xs:choice></xs:complexType>
`)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
		if err != nil {
			t.Fatalf("standalone QName-bearing identities: %v", err)
		}
		members := qnameDefinition(t, schema, "U").MemberTypes()
		if len(members) != 2 || !members[0].IsBuiltin() || members[0].Name().Local() != "precisionDecimal" || !members[1].IsNamed() || members[1].Name() != mustTestQName(t, "urn:test", "Q") || members[1].Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 3, `memberTypes="`) {
			t.Fatalf("union members = %#v", members)
		}
		if id, hasID := members[1].ComponentID(); !hasID || id != componentIDForName(t, schema, members[1].Name()) {
			t.Fatalf("QName union member ID = %v/%t", id, hasID)
		}
		item, ok := qnameDefinition(t, schema, "QList").ItemType()
		if !ok || !item.IsNamed() || item.Name() != mustTestQName(t, "urn:test", "Q") {
			t.Fatalf("QName list item = %#v/%t", item, ok)
		}
		if id, hasID := item.ComponentID(); !hasID || id != componentIDForName(t, schema, item.Name()) {
			t.Fatalf("QName list item ID = %v/%t", id, hasID)
		}
		if len(schema.Components()) != 6 || schema.Components()[5].Name() != mustTestQName(t, "urn:test", "Refs") {
			t.Fatalf("component order/count = %#v", schema.Components())
		}
		box, ok := schema.Components()[4].ComplexTypeDefinition()
		if !ok {
			t.Fatal("zero-omission owner has no complex type view")
		}
		sequence, ok := box.Particle().(SequenceParticle)
		if !ok || len(sequence.Particles()) != 0 {
			t.Fatalf("validated 0/0 QName-bearing local was retained: %T", box.Particle())
		}
		refs, ok := schema.Components()[5].ComplexTypeDefinition()
		if !ok {
			t.Fatal("element-ref owner has no complex type view")
		}
		choice, ok := refs.Particle().(ChoiceParticle)
		if !ok || len(choice.Alternatives()) != 1 {
			t.Fatalf("ref choice = %T", refs.Particle())
		}
		ref, ok := choice.Alternatives()[0].(ElementReferenceParticle)
		if !ok || ref.Ref() != mustTestQName(t, "urn:test", "item") || ref.RefLoc() != mustSchemaTokenLoc(t, "root.xsd", root, 7, `ref="t:item"`) || ref.TargetID() != schema.Components()[3].ID() {
			t.Fatalf("QName global element ref = %#v/%t", ref, ok)
		}
	}
}

//nolint:gocognit // Version, policy, and lexical exits share the QName facet boundary.
func TestQNameFacetAlternateDiagnostics(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct {
			name, facet, mark, code, spec string
			class                         FailureClass
			cause                         error
		}{
			{"invalid whiteSpace", `<xs:whiteSpace value="unknown"/>`, `value="unknown"`, InvalidStringWhiteSpaceCode, stringWhiteSpaceSpecRef(profile.version), FailureInvalid, errInvalidStringWhiteSpaceValue},
			{"QName minScale", `<xs:minScale value="1"/>`, `<xs:minScale`, UnsupportedDatatypeFacetCode, qnameFacetSpecRef(profile.version), FailureUnsupported, ErrUnsupported},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := qnameSchema("\n  " + `<xs:simpleType name="Q"><xs:restriction base="xs:QName">` + test.facet + `</xs:restriction></xs:simpleType>` + "\n")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil {
					t.Fatalf("QName facet returned schema/no error: %v", err)
				}
				d := requireDiagnostic(t, err)
				wantSpec := test.spec
				if profile.version == XSDVersion10 && test.name == "QName minScale" {
					wantSpec = tokenDiagnosticSpecRef(XSDVersion11, "decimal")
				}
				if d.Class() != test.class || d.Code() != test.code || d.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 2, test.mark) || d.SpecRef() != wantSpec || len(d.Related()) != 0 || !errors.Is(err, test.cause) {
					t.Fatalf("diagnostic = %v related=%v spec=%s", d, d.Related(), d.SpecRef())
				}
				if profile.version == XSDVersion10 && test.name == "QName minScale" && !errors.Is(err, errLanguagePolicyMismatch) {
					t.Fatalf("XSD 1.1-only facet lost policy cause: %v", err)
				}
			})
		}
	}
}

//nolint:gocognit // Both consumers must reject every admitted global type shape.
func TestQNameGlobalElementConsumers(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct{ name, body string }{
			{"direct", `<xs:element name="item" type="xs:QName"/>`},
			{"named", `<xs:element name="item" type="t:Alias"/><xs:simpleType name="Alias"><xs:restriction base="xs:QName"/></xs:simpleType>`},
			{"inline", `<xs:element name="item"><xs:simpleType><xs:restriction base="xs:QName"/></xs:simpleType></xs:element>`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := qnameSchema("\n  " + test.body + "\n")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("ParseSchema: %v", err)
				}
				output, err := GenerateGo(schema, "generated")
				if err == nil || output != nil {
					t.Fatalf("GenerateGo = %q/%v, want no output", output, err)
				}
				gen := requireDiagnostic(t, err)
				if gen.Class() != FailureUnsupported || gen.Code() != diagnosticCodegenUnsupported || gen.Loc().IsZero() || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("generation diagnostic = %v", gen)
				}
				err = ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<item xmlns="urn:test" xmlns:t="urn:test">t:a</item>`)))
				if err == nil {
					t.Fatal("QName instance validation succeeded")
				}
				validation := requireDiagnostic(t, err)
				if validation.Class() != FailureUnsupported || validation.Code() != UnsupportedInstanceValidationCode || validation.Loc() != (Loc{source: "instance.xml", line: 1, column: 1}) || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("validation diagnostic = %v", validation)
				}
			})
		}
	}
}

//nolint:gocognit // Resolution exits keep their original code, cause, and locations.
func TestQNameReferenceResolutionDiagnostics(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct {
			name, body, mark, related, code string
			cause                           error
		}{
			{"unresolved", `<xs:simpleType name="Alias"><xs:restriction base="t:Missing"/></xs:simpleType><xs:simpleType name="Seed"><xs:restriction base="xs:QName"/></xs:simpleType>`, `base="t:Missing"`, "", diagnosticSchemaSimpleTypeUnresolvedCode, errSchemaSimpleTypeBaseUnresolved},
			{"wrong kind", `<xs:element name="Target" type="xs:QName"/><xs:simpleType name="Alias"><xs:restriction base="t:Target"/></xs:simpleType>`, `base="t:Target"`, `<xs:element name="Target"`, diagnosticSchemaSimpleTypeWrongKindCode, errSchemaSimpleTypeBaseWrongKind},
			{"cycle", `<xs:simpleType name="A"><xs:restriction base="t:B"/></xs:simpleType><xs:simpleType name="B"><xs:restriction base="t:A"/></xs:simpleType><xs:simpleType name="Seed"><xs:restriction base="xs:QName"/></xs:simpleType>`, `base="t:B"`, `base="t:A"`, diagnosticSchemaSimpleTypeCycleCode, errSchemaSimpleTypeBaseCycle},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := qnameSchema("\n  " + test.body + "\n")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil {
					t.Fatalf("bad reference returned schema/no error: %v", err)
				}
				d := requireDiagnostic(t, err)
				loc := mustSchemaTokenLoc(t, "root.xsd", root, 2, test.mark)
				if d.Class() != FailureInvalid || d.Code() != test.code || d.Loc() != loc || d.SpecRef() != schemaSimpleTypeSpecRef(profile.version) || !errors.Is(err, test.cause) {
					t.Fatalf("diagnostic = %v, want %s at %s with %s", d, test.code, loc, test.cause)
				}
				var related []Loc
				if test.related != "" {
					related = []Loc{mustSchemaTokenLoc(t, "root.xsd", root, 2, test.related)}
				}
				if !reflect.DeepEqual(d.Related(), related) {
					t.Fatalf("related = %v, want %v", d.Related(), related)
				}
			})
		}
	}
}

//nolint:gocognit // Both invalid exits have distinct location and cause contracts.
func TestQNameMalformedAndAmbiguousDiagnostics(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct {
			name, body, mark, related, code, spec string
			cause                                 error
		}{
			{"malformed", `<xs:simpleType name="Alias"><xs:restriction base="t:bad:QName"/></xs:simpleType><xs:simpleType name="Seed"><xs:restriction base="xs:QName"/></xs:simpleType>`, `base="t:bad:QName"`, "", invalidSchemaConditionalCode, "", nil},
			{"ambiguous", `<xs:simpleType name="Alias"><xs:restriction base="xs:QName"/></xs:simpleType><xs:simpleType name="Alias"><xs:restriction base="xs:QName"/></xs:simpleType><xs:element name="item" type="t:Alias"/>`, `<xs:simpleType name="Alias"`, `<xs:simpleType name="Alias"`, diagnosticSchemaGlobalDuplicateCode, schemaGlobalDuplicateSpecRef(profile.version), errSchemaGlobalDeclarationDuplicate},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := qnameSchema("\n  " + test.body + "\n")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil {
					t.Fatalf("invalid QName reference returned schema/no error: %v", err)
				}
				d := requireDiagnostic(t, err)
				if d.Class() != FailureInvalid || d.Code() != test.code {
					t.Fatalf("diagnostic = %v", d)
				}
				if test.cause != nil && !errors.Is(err, test.cause) {
					t.Fatalf("diagnostic lost cause %v: %v", test.cause, err)
				}
				if test.spec != "" && d.SpecRef() != test.spec {
					t.Fatalf("spec = %s, want %s", d.SpecRef(), test.spec)
				}
				if test.name == "malformed" && d.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 2, test.mark) {
					t.Fatalf("malformed Loc = %s", d.Loc())
				}
				if test.name == "ambiguous" {
					first := mustSchemaTokenLoc(t, "root.xsd", root, 2, test.mark)
					second := mustTestLoc(t, "root.xsd", 2, strings.LastIndex(strings.Split(root, "\n")[1], test.mark)+1)
					if d.Loc() != second || len(d.Related()) != 1 || d.Related()[0] != first {
						t.Fatalf("ambiguous location/related = %s/%v", d.Loc(), d.Related())
					}
				}
			})
		}
	}
}

func TestQNameInvisibleGraphReference(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:f="urn:foreign" targetNamespace="urn:root"><xs:include schemaLocation="child.xsd"/><xs:simpleType name="Ref"><xs:restriction base="f:Hidden"/></xs:simpleType><xs:simpleType name="Seed"><xs:restriction base="xs:QName"/></xs:simpleType></xs:schema>`
			fixtures := map[string]discoveryFixture{
				"child.xsd":   {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:import namespace="urn:foreign" schemaLocation="foreign.xsd"/></xs:schema>`},
				"foreign.xsd": {id: "foreign.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:foreign"><xs:simpleType name="Hidden"><xs:restriction base="xs:QName"/></xs:simpleType></xs:schema>`},
			}
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err == nil || schema.storage != nil {
				t.Fatalf("invisible type returned schema/no error: %v", err)
			}
			d := requireDiagnostic(t, err)
			if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaSimpleTypeUnresolvedCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `base="f:Hidden"`) || len(d.Related()) != 0 || d.SpecRef() != schemaSimpleTypeSpecRef(profile.version) || !errors.Is(err, errSchemaSimpleTypeBaseUnresolved) {
				t.Fatalf("invisible reference diagnostic = %v", d)
			}
		})
	}
}

//nolint:gocognit // Every value-constraint shape has its own diagnostic.
func TestQNameValueConstraintsStayOutsideSchemaModel(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct {
			name, body, mark string
			class            FailureClass
			code             string
		}{
			{"direct default", `<xs:element name="value" type="xs:QName" default="t:a"/>`, `default="t:a"`, FailureUnsupported, UnsupportedSchemaSyntaxCode},
			{"direct fixed", `<xs:element name="value" type="xs:QName" fixed="t:a"/>`, `fixed="t:a"`, FailureUnsupported, UnsupportedSchemaSyntaxCode},
			{"named default", `<xs:element name="value" type="t:Alias" default="t:a"/><xs:simpleType name="Alias"><xs:restriction base="xs:QName"/></xs:simpleType>`, `default="t:a"`, FailureUnsupported, UnsupportedSchemaSyntaxCode},
			{"named fixed", `<xs:element name="value" type="t:Alias" fixed="t:a"/><xs:simpleType name="Alias"><xs:restriction base="xs:QName"/></xs:simpleType>`, `fixed="t:a"`, FailureUnsupported, UnsupportedSchemaSyntaxCode},
			{"inline default", `<xs:element name="value" default="t:a"><xs:simpleType><xs:restriction base="xs:QName"/></xs:simpleType></xs:element>`, `default="t:a"`, FailureUnsupported, UnsupportedSchemaSyntaxCode},
			{"inline fixed", `<xs:element name="value" fixed="t:a"><xs:simpleType><xs:restriction base="xs:QName"/></xs:simpleType></xs:element>`, `fixed="t:a"`, FailureUnsupported, UnsupportedSchemaSyntaxCode},
			{"ref default", `<xs:element name="target" type="xs:QName"/><xs:complexType name="Root"><xs:choice><xs:element ref="t:target" default="t:a"/></xs:choice></xs:complexType>`, `default="t:a"`, FailureInvalid, invalidSchemaCompositionCode},
			{"ref fixed", `<xs:element name="target" type="xs:QName"/><xs:complexType name="Root"><xs:choice><xs:element ref="t:target" fixed="t:a"/></xs:choice></xs:complexType>`, `fixed="t:a"`, FailureInvalid, invalidSchemaCompositionCode},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := qnameSchema(test.body)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil {
					t.Fatalf("QName value constraint returned schema/no error: %v", err)
				}
				d := requireDiagnostic(t, err)
				loc := elementReferenceTestAttributeLoc(t, root, test.mark)
				if d.Class() != test.class || d.Code() != test.code || d.Loc() != loc || len(d.Related()) != 0 {
					t.Fatalf("diagnostic = %v related=%v, want %s/%s at %s", d, d.Related(), test.class, test.code, loc)
				}
				wantSpec := "xsd10-structures#schema-document"
				if test.class == FailureInvalid {
					wantSpec = ""
				}
				if d.SpecRef() != wantSpec || d.Unwrap() != nil || errors.Is(err, ErrUnsupported) != (test.class == FailureUnsupported) {
					t.Fatalf("diagnostic provenance = %q/%v", d.SpecRef(), d.Unwrap())
				}
			})
		}
	}
}

//nolint:gocognit // Query admission and both ref consumers share one target.
func TestQNameElementReferenceConsumers(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := qnameSchema(`
  <xs:complexType name="Box"><xs:choice><xs:element ref="t:item"/></xs:choice></xs:complexType>
  <xs:element name="box" type="t:Box"/>
  <xs:element name="item" type="xs:QName"/>
`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("ParseSchema: %v", err)
			}
			if len(schema.Components()) != 3 || schema.Components()[2].Name() != mustTestQName(t, "urn:test", "item") {
				t.Fatal("referenced QName target identity/order changed")
			}
			output, err := GenerateGo(schema, "generated")
			if err == nil || output != nil {
				t.Fatal("Go generation admitted QName ref target")
			}
			d := requireDiagnostic(t, err)
			wantRefLoc := mustSchemaTokenLoc(t, "root.xsd", root, 2, `ref="t:item"`)
			wantGenSpec := "xsd11-structures#element-choice"
			if profile.version == XSDVersion10 {
				wantGenSpec = "xsd10-structures#element-choice"
			}
			wantGenRelated := []Loc{
				mustSchemaTokenLoc(t, "root.xsd", root, 2, `<xs:element`),
				mustSchemaTokenLoc(t, "root.xsd", root, 4, `<xs:element`),
			}
			if d.Class() != FailureUnsupported || d.Code() != diagnosticCodegenUnsupported || d.Loc() != wantRefLoc || d.SpecRef() != wantGenSpec || !reflect.DeepEqual(d.Related(), wantGenRelated) || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("generation diagnostic = %v related=%v", d, d.Related())
			}
			err = ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<box xmlns="urn:test"><item xmlns:t="urn:test">t:a</item></box>`)))
			if err == nil {
				t.Fatal("validation admitted QName ref target")
			}
			d = requireDiagnostic(t, err)
			wantValidationRelated := []Loc{
				mustSchemaTokenLoc(t, "root.xsd", root, 3, `<xs:element`),
				mustSchemaTokenLoc(t, "root.xsd", root, 2, `<xs:complexType`),
				mustSchemaTokenLoc(t, "root.xsd", root, 2, `<xs:choice`),
				mustSchemaTokenLoc(t, "root.xsd", root, 2, `<xs:element`),
				mustSchemaTokenLoc(t, "root.xsd", root, 4, `<xs:element`),
			}
			if d.Class() != FailureUnsupported || d.Code() != UnsupportedInstanceValidationCode || d.Loc() != (Loc{source: "instance.xml", line: 1, column: 1}) || d.SpecRef() != instanceValidationSpecRef(profile.version) || !reflect.DeepEqual(d.Related(), wantValidationRelated) || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("validation diagnostic = %v related=%v", d, d.Related())
			}
		})
	}
}
