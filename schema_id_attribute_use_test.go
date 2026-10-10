package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func idUseRoot(body, extras string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" attributeFormDefault="qualified">` + body + extras + `</xs:schema>`
}

func parseIDUseSchema(t *testing.T, root string, fixtures map[string]discoveryFixture, policy LanguagePolicy) (Schema, error) {
	t.Helper()
	source, err := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
	if err != nil {
		t.Fatalf("NewResolvedSource: %v", err)
	}
	return ParseSchemaWithPolicy(source, &discoveryResolver{fixtures: fixtures}, policy)
}

//nolint:gocognit // Every admitted owner checks the public copy and provenance contract.
func TestIDLocalAttributeUsesPreserveFactsAcrossPoliciesAndOwners(t *testing.T) {
	shapes := []struct {
		name string
		body string
	}{
		{"empty", `<xs:attribute name="id" type="xs:ID" use="required"/>`},
		{"choice", `<xs:choice><xs:element name="value" type="xs:integer"/></xs:choice><xs:attribute name="id" type="xs:ID" use="required"/>`},
		{"sequence", `<xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence><xs:attribute name="id" type="xs:ID" use="required"/>`},
		{"grouped extension", `<xs:complexContent><xs:extension base="r:Base"><xs:group ref="r:Fields" minOccurs="0" maxOccurs="0"/><xs:attribute name="id" type="xs:ID" use="required"/></xs:extension></xs:complexContent>`},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, shape := range shapes {
			t.Run(string(policy)+"/"+shape.name, func(t *testing.T) {
				extra := `<xs:complexType name="Base"/><xs:group name="Fields"><xs:sequence><xs:element ref="r:value"/></xs:sequence></xs:group><xs:element name="value" type="xs:integer"/>`
				root := idUseRoot(`<xs:complexType name="Record">`+shape.body+`</xs:complexType>`, extra)
				schema, err := parseIDUseSchema(t, root, nil, policy)
				if err != nil {
					t.Fatalf("ParseSchema: %v", err)
				}
				if schema.LanguagePolicy() != policy {
					t.Fatalf("schema policy = %s, want %s", schema.LanguagePolicy(), policy)
				}
				found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Record"))
				if len(found) != 1 {
					t.Fatalf("Record = %v", found)
				}
				definition := requireTestComplexTypeDefinition(t, found[0], "Record")
				uses := definition.AttributeUses()
				if len(uses) != 1 {
					t.Fatalf("attribute uses = %#v", uses)
				}
				use, ok := uses[0].(LocalAttributeUse)
				if !ok {
					t.Fatalf("attribute use = %T", uses[0])
				}
				wantUse := elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="id"`)
				wantName := elementReferenceTestAttributeLoc(t, root, `name="id"`)
				wantType := elementReferenceTestAttributeLoc(t, root, `type="xs:ID"`)
				wantRequired := elementReferenceTestAttributeLoc(t, root, `use="required"`)
				if use.Loc() != wantUse || use.NameLoc() != wantName || use.TypeLoc() != wantType || use.UseLoc() != wantRequired || use.Name() != mustTestQName(t, "urn:root", "id") || use.DeclaredType() != mustTestQName(t, testXSDNamespace, "ID") || use.Use() != AttributeUseRequired {
					t.Fatalf("ID use facts = %#v", use)
				}
				ref, ok := use.TypeReference()
				if !ok || !ref.IsBuiltin() || ref.Name() != mustTestQName(t, testXSDNamespace, "ID") || ref.Loc() != wantType || ref.VarietyLoc() != wantType || ref.Variety() != SimpleTypeVarietyAtomicRestriction {
					t.Fatalf("built-in ID reference = %#v/%t", ref, ok)
				}
				if typeID, hasID := use.TypeID(); hasID || !typeID.IsZero() {
					t.Fatalf("built-in use has synthetic ID %v/%t", typeID, hasID)
				}
				if typeID, hasID := ref.ComponentID(); hasID || !typeID.IsZero() {
					t.Fatalf("built-in reference has synthetic ID %v/%t", typeID, hasID)
				}
				uses[0] = nil
				if len(definition.AttributeUses()) != 1 || definition.AttributeUses()[0].Name() != use.Name() {
					t.Fatal("attribute use query mutated Schema")
				}
				if shape.name == "grouped extension" {
					if definition.Particle() != nil || definition.Base() != mustTestQName(t, "urn:root", "Base") {
						t.Fatalf("grouped 0/0 facts = %T/%q", definition.Particle(), definition.Base())
					}
				}
			})
		}
	}
}

//nolint:gocognit // Graph order and named target identity are one public contract.
func TestIDLocalAttributeUsesResolveNamedGraphAndLexicalOrder(t *testing.T) {
	root := idUseRoot(`<xs:include schemaLocation="ordinary.xsd"/><xs:include schemaLocation="chameleon.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/>
<xs:complexType name="Record"><xs:attribute name="first" type="r:Derived"/><xs:attribute name="gone" type="xs:ID" use="prohibited"/><xs:attribute name="included" type="r:Included" form="unqualified"/><xs:attribute name="adopted" type="r:Adopted"/><xs:attribute name="foreign" type="o:Foreign"/></xs:complexType>
<xs:simpleType name="Forward"><xs:restriction base="xs:ID"/></xs:simpleType><xs:simpleType name="Derived"><xs:restriction base="r:Forward"/></xs:simpleType>`, ``)
	root = strings.Replace(root, `xmlns:r="urn:root"`, `xmlns:r="urn:root" xmlns:o="urn:other"`, 1)
	fixtures := map[string]discoveryFixture{
		"ordinary.xsd":  {id: "ordinary.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:simpleType name="Included"><xs:restriction base="xs:ID"/></xs:simpleType></xs:schema>`},
		"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Adopted"><xs:restriction base="xs:ID"/></xs:simpleType></xs:schema>`},
		"other.xsd":     {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Foreign"><xs:restriction base="xs:ID"/></xs:simpleType></xs:schema>`},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		schema, err := parseIDUseSchema(t, root, fixtures, policy)
		if err != nil {
			t.Fatalf("%s ParseSchema: %v", policy, err)
		}
		found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Record"))
		if len(found) != 1 {
			t.Fatalf("Record = %v", found)
		}
		uses := requireTestComplexTypeDefinition(t, found[0], "Record").AttributeUses()
		wantNames := []string{"first", "included", "adopted", "foreign"}
		wantTypes := []QName{mustTestQName(t, "urn:root", "Derived"), mustTestQName(t, "urn:root", "Included"), mustTestQName(t, "urn:root", "Adopted"), mustTestQName(t, "urn:other", "Foreign")}
		wantSources := []SourceID{"root.xsd", "ordinary.xsd", "chameleon.xsd", "other.xsd"}
		if len(uses) != len(wantNames) {
			t.Fatalf("ordered use count = %d", len(uses))
		}
		for index, item := range uses {
			local, ok := item.(LocalAttributeUse)
			if !ok || local.Name().Local() != wantNames[index] || local.DeclaredType() != wantTypes[index] || local.TypeLoc() != elementReferenceTestAttributeLoc(t, root, `type="`+[]string{"r:Derived", "r:Included", "r:Adopted", "o:Foreign"}[index]+`"`) || local.Use() != AttributeUseOptional || !local.UseLoc().IsZero() {
				t.Fatalf("use %d = %#v", index, item)
			}
			if index == 1 && local.Name().Namespace() != "" {
				t.Fatalf("form=unqualified name = %q", local.Name())
			}
			ref, ok := local.TypeReference()
			id, hasID := local.TypeID()
			refID, hasRefID := ref.ComponentID()
			if !ok || !ref.IsNamed() || ref.Name() != wantTypes[index] || !hasID || !hasRefID || id != refID || id.Source() != wantSources[index] || id != componentIDForName(t, schema, wantTypes[index]) {
				t.Fatalf("named type %d = %#v/%v/%t", index, ref, id, hasID)
			}
		}
		uses[0] = nil
		if got := requireTestComplexTypeDefinition(t, found[0], "Record").AttributeUses()[0].Name().Local(); got != "first" {
			t.Fatalf("mutating copy changed schema: %q", got)
		}
	}
}

func TestStrict10IDNamedTypesAcrossRepeatedDiscoveryAndCycle(t *testing.T) {
	root := idUseRoot(`<xs:include schemaLocation="included.xsd"/><xs:include schemaLocation="included.xsd"/><xs:include schemaLocation="chameleon.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/>
<xs:complexType name="ForwardUse"><xs:attribute name="id" type="r:Forward"/></xs:complexType>
<xs:complexType name="IncludedUse"><xs:attribute name="id" type="r:Included"/></xs:complexType>
<xs:complexType name="AdoptedUse"><xs:attribute name="id" type="r:Adopted"/></xs:complexType>
<xs:complexType name="ForeignUse"><xs:attribute name="id" type="o:Foreign"/></xs:complexType>
<xs:simpleType name="Forward"><xs:restriction base="xs:ID"/></xs:simpleType>`, ``)
	root = strings.Replace(root, `xmlns:r="urn:root"`, `xmlns:r="urn:root" xmlns:o="urn:other"`, 1)
	fixtures := map[string]discoveryFixture{
		"root.xsd":      {id: "root.xsd", contents: root},
		"included.xsd":  {id: "included.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:include schemaLocation="root.xsd"/><xs:simpleType name="Included"><xs:restriction base="xs:ID"/></xs:simpleType></xs:schema>`},
		"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Adopted"><xs:restriction base="xs:ID"/></xs:simpleType></xs:schema>`},
		"other.xsd":     {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Foreign"><xs:restriction base="xs:ID"/></xs:simpleType></xs:schema>`},
	}
	schema, err := parseIDUseSchema(t, root, fixtures, Strict10)
	if err != nil {
		t.Fatalf("ParseSchema: %v", err)
	}
	for index, test := range []struct {
		owner, namespace, name string
		source                 SourceID
	}{
		{"ForwardUse", "urn:root", "Forward", "root.xsd"},
		{"IncludedUse", "urn:root", "Included", "included.xsd"},
		{"AdoptedUse", "urn:root", "Adopted", "chameleon.xsd"},
		{"ForeignUse", "urn:other", "Foreign", "other.xsd"},
	} {
		found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", test.owner))
		if len(found) != 1 {
			t.Fatalf("owner %d = %v", index, found)
		}
		uses := requireTestComplexTypeDefinition(t, found[0], test.owner).AttributeUses()
		if len(uses) != 1 {
			t.Fatalf("owner %d uses = %v", index, uses)
		}
		use, ok := uses[0].(LocalAttributeUse)
		if !ok {
			t.Fatalf("owner %d use = %T", index, uses[0])
		}
		wantName := mustTestQName(t, test.namespace, test.name)
		id, hasID := use.TypeID()
		ref, hasRef := use.TypeReference()
		if !hasID || !hasRef || !ref.IsNamed() || ref.Name() != wantName || id != componentIDForName(t, schema, wantName) || id.Source() != test.source {
			t.Fatalf("owner %d named ID = %v/%v/%v", index, use, id, ref)
		}
	}
}

//nolint:gocognit // The table checks the classified exits at the same reference boundary.
func TestIDLocalAttributeNamedReferenceFailureExits(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, test := range []struct {
			name, typeName, extra string
			code                  string
			cause                 error
			related               string
		}{
			{"unresolved", "r:Missing", ``, diagnosticSchemaAttributeTypeUnresolvedCode, errSchemaAttributeTypeUnresolved, ``},
			{"wrong kind", "r:Wrong", `<xs:complexType name="Wrong"/>`, diagnosticSchemaAttributeTypeWrongKindCode, errSchemaAttributeTypeWrongKind, `<xs:complexType name="Wrong"`},
			{"cycle", "r:First", `<xs:simpleType name="First"><xs:restriction base="r:Second"/></xs:simpleType><xs:simpleType name="Second"><xs:restriction base="r:First"/></xs:simpleType>`, diagnosticSchemaAttributeTypeCycleCode, errSchemaSimpleTypeBaseCycle, ``},
		} {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				root := idUseRoot(`<xs:complexType name="Record"><xs:attribute name="id" type="`+test.typeName+`"/></xs:complexType>`, test.extra)
				schema, err := parseIDUseSchema(t, root, nil, policy)
				if err == nil {
					t.Fatal("bad named ID reference was admitted")
				}
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				want := elementReferenceTestAttributeLoc(t, root, `type="`+test.typeName+`"`)
				version := XSDVersion11
				if policy == Strict10 {
					version = XSDVersion10
				}
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code || diagnostic.Loc() != want || diagnostic.SpecRef() != schemaAttributeTypeSpecRef(version) || !errors.Is(err, test.cause) {
					t.Fatalf("named reference diagnostic = %v", err)
				}
				if test.related != "" && !reflect.DeepEqual(diagnostic.Related(), []Loc{elementReferenceTestAttributeLoc(t, root, test.related)}) {
					t.Fatalf("named reference related = %v", diagnostic.Related())
				}
				if test.name == "unresolved" && len(diagnostic.Related()) != 0 {
					t.Fatalf("unresolved related = %v", diagnostic.Related())
				}
				if test.name == "unresolved" && !errors.Is(err, errSchemaSimpleTypeBaseUnresolved) {
					t.Fatalf("unresolved simple-type cause lost: %v", err)
				}
				if test.name == "wrong kind" && !errors.Is(err, errSchemaSimpleTypeBaseWrongKind) {
					t.Fatalf("wrong-kind simple-type cause lost: %v", err)
				}
				if test.name == "cycle" {
					wantRelated := []Loc{
						elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="id"`),
						elementReferenceTestAttributeLoc(t, root, `base="r:Second"`),
						elementReferenceTestAttributeLoc(t, root, `base="r:First"`),
						elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="First"`),
						elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="Second"`),
					}
					if !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
						t.Fatalf("cycle related = %v, want %v", diagnostic.Related(), wantRelated)
					}
				}
			})
		}
	}
}

func TestIDLocalAttributeTransitiveImportIsInvisible(t *testing.T) {
	root := idUseRoot(`<xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/><xs:complexType name="Record"><xs:attribute name="id" type="o:Foreign"/></xs:complexType>`, ``)
	root = strings.Replace(root, `xmlns:r="urn:root"`, `xmlns:r="urn:root" xmlns:o="urn:other"`, 1)
	fixtures := map[string]discoveryFixture{
		"bridge.xsd": {id: "bridge.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge"><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`},
		"other.xsd":  {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Foreign"><xs:restriction base="xs:ID"/></xs:simpleType></xs:schema>`},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		schema, err := parseIDUseSchema(t, root, fixtures, policy)
		if err == nil {
			t.Fatalf("%s admitted transitive import", policy)
		}
		assertZeroSchema(t, schema)
		d := requireDiagnostic(t, err)
		version := XSDVersion11
		if policy == Strict10 {
			version = XSDVersion10
		}
		if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaAttributeTypeUnresolvedCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `type="o:Foreign"`) || d.SpecRef() != schemaAttributeTypeSpecRef(version) || !errors.Is(err, errSchemaAttributeTypeUnresolved) {
			t.Fatalf("%s visibility diagnostic = %v", policy, err)
		}
	}
}

func TestIDLocalAttributeNamedSourceAcquisitionRetainsCause(t *testing.T) {
	root := idUseRoot(`<xs:include schemaLocation="missing.xsd"/><xs:complexType name="Record"><xs:attribute name="id" type="r:Included"/></xs:complexType>`, ``)
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		source, err := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
		if err != nil {
			t.Fatalf("NewResolvedSource: %v", err)
		}
		cause := errors.New("ID type source unavailable")
		schema, err := ParseSchemaWithPolicy(source, &discoveryResolver{failures: map[string]error{"missing.xsd": cause}}, policy)
		if err == nil {
			t.Fatalf("%s accepted missing included source", policy)
		}
		assertZeroSchema(t, schema)
		d := requireDiagnostic(t, err)
		if d.Class() != FailureResolution || d.Code() != SourceResolveCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:include`) || !errors.Is(err, cause) {
			t.Fatalf("%s acquisition diagnostic = %v", policy, err)
		}
	}
}

func TestIDLocalAttributeNamedFacetStaysUnsupported(t *testing.T) {
	root := idUseRoot(`<xs:complexType name="Record"><xs:attribute name="id" type="r:Patterned"/></xs:complexType>`, `<xs:simpleType name="Patterned"><xs:restriction base="xs:ID"><xs:pattern value="[a-z]+"/></xs:restriction></xs:simpleType>`)
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		schema, err := parseIDUseSchema(t, root, nil, policy)
		if err == nil {
			t.Fatalf("%s admitted ID pattern", policy)
		}
		assertZeroSchema(t, schema)
		d := requireDiagnostic(t, err)
		wantSpec := "xsd11-datatypes#decimal"
		if policy == Strict10 {
			wantSpec = "xsd10-datatypes#decimal"
		}
		if d.Class() != FailureUnsupported || d.Code() != UnsupportedDatatypeFacetCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:pattern`) || d.SpecRef() != wantSpec || len(d.Related()) != 0 || !errors.Is(err, ErrUnsupported) {
			t.Fatalf("%s ID facet diagnostic = %v", policy, err)
		}
	}
}

func TestIDAttributeUseDoesNotAdmitParticleBearingExtensionComposition(t *testing.T) {
	root := idUseRoot(`<xs:complexType name="Base"/><xs:complexType name="Record"><xs:complexContent><xs:extension base="r:Base"><xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence><xs:attribute name="id" type="xs:ID"/></xs:extension></xs:complexContent></xs:complexType>`, ``)
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		schema, err := parseIDUseSchema(t, root, nil, policy)
		if err == nil {
			t.Fatalf("%s admitted particle-bearing extension ID use", policy)
		}
		assertZeroSchema(t, schema)
		d := requireDiagnostic(t, err)
		version := XSDVersion11
		if policy == Strict10 {
			version = XSDVersion10
		}
		if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="id"`) || d.SpecRef() != schemaComplexTypeExtensionSpecRef(version) || len(d.Related()) != 0 || !errors.Is(err, ErrUnsupported) {
			t.Fatalf("%s extension diagnostic = %v", policy, err)
		}
	}
}

func TestStrict10RejectsTwoEffectiveIDUses(t *testing.T) {
	body := `<xs:complexType name="Record"><xs:attribute name="first" type="xs:ID"/><xs:attribute name="excluded" type="xs:ID" use="prohibited"/><xs:attribute name="second" type="xs:ID"/></xs:complexType>`
	root := idUseRoot(body, ``)
	schema, err := parseIDUseSchema(t, root, nil, Strict10)
	if err == nil {
		t.Fatal("Strict10 admitted two effective ID uses")
	}
	assertZeroSchema(t, schema)
	diagnostic := requireDiagnostic(t, err)
	wantFirst := elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="first"`)
	wantSecond := elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="second"`)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaAttributeUseIDDuplicateCode || diagnostic.Loc() != wantSecond || !reflect.DeepEqual(diagnostic.Related(), []Loc{wantFirst}) || diagnostic.SpecRef() != "xsd10-structures#cos-ct-props-correct" || !errors.Is(err, errSchemaAttributeUseIDDuplicate) {
		t.Fatalf("Strict10 cardinality diagnostic = %v", err)
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		admitted, parseErr := parseIDUseSchema(t, root, nil, policy)
		if parseErr != nil || len(requireTestComplexTypeDefinition(t, admitted.Components()[0], "Record").AttributeUses()) != 2 {
			t.Fatalf("%s two-ID uses = %v/%v", policy, admitted, parseErr)
		}
	}
	before := idUseRoot(`<xs:complexType name="Record"><xs:attribute name="first" type="xs:ID"/><xs:attribute name="excluded" type="xs:ID" use="prohibited"/></xs:complexType>`, ``)
	admitted, parseErr := parseIDUseSchema(t, before, nil, Strict10)
	if parseErr != nil || len(requireTestComplexTypeDefinition(t, admitted.Components()[0], "Record").AttributeUses()) != 1 {
		t.Fatalf("Strict10 prohibited omission = %v/%v", admitted, parseErr)
	}
}

//nolint:gocognit // The table pins each excluded shape at its public diagnostic boundary.
func TestIDLocalAttributeUseExclusionsStayLocated(t *testing.T) {
	tests := []struct {
		name, body, extra, marker string
		cause                     error
		related                   string
		useSpec                   bool
	}{
		{"inline ID", `<xs:complexType name="Record"><xs:attribute name="id"><xs:simpleType><xs:restriction base="xs:ID"/></xs:simpleType></xs:attribute></xs:complexType>`, ``, `<xs:simpleType`, errSchemaAttributeUseUnsupported, ``, false},
		{"named ID list", `<xs:complexType name="Record"><xs:attribute name="id" type="r:IDs"/></xs:complexType>`, `<xs:simpleType name="IDs"><xs:list itemType="xs:ID"/></xs:simpleType>`, `type="r:IDs"`, errSchemaAttributeUseUnsupported, ``, false},
		{"default ID", `<xs:complexType name="Record"><xs:attribute name="id" type="xs:ID" default="a"/></xs:complexType>`, ``, `default="a"`, errSchemaAttributeUseUnsupported, ``, true},
		{"fixed ID", `<xs:complexType name="Record"><xs:attribute name="id" type="xs:ID" fixed="a"/></xs:complexType>`, ``, `fixed="a"`, errSchemaAttributeUseUnsupported, ``, true},
		{"named default ID", `<xs:complexType name="Record"><xs:attribute name="id" type="r:Alias" default="a"/></xs:complexType>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:ID"/></xs:simpleType>`, `default="a"`, errSchemaAttributeUseUnsupported, ``, true},
		{"named fixed ID", `<xs:complexType name="Record"><xs:attribute name="id" type="r:Alias" fixed="a"/></xs:complexType>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:ID"/></xs:simpleType>`, `fixed="a"`, errSchemaAttributeUseUnsupported, ``, true},
		{"inline default ID", `<xs:complexType name="Record"><xs:attribute name="id" default="a"><xs:simpleType><xs:restriction base="xs:ID"/></xs:simpleType></xs:attribute></xs:complexType>`, ``, `default="a"`, errSchemaAttributeUseUnsupported, ``, true},
		{"inline fixed ID", `<xs:complexType name="Record"><xs:attribute name="id" fixed="a"><xs:simpleType><xs:restriction base="xs:ID"/></xs:simpleType></xs:attribute></xs:complexType>`, ``, `fixed="a"`, errSchemaAttributeUseUnsupported, ``, true},
		{"global ref ID", `<xs:complexType name="Record"><xs:attribute ref="r:id"/></xs:complexType>`, `<xs:attribute name="id" type="xs:ID"/>`, `ref="r:id"`, errSchemaAttributeReferenceUnsupported, `<xs:attribute name="id"`, true},
		{"direct all ID", `<xs:complexType name="Record"><xs:all><xs:element name="value" type="xs:integer"/></xs:all><xs:attribute name="id" type="xs:ID"/></xs:complexType>`, ``, `type="xs:ID"`, errSchemaAttributeUseUnsupported, ``, false},
		{"direct group ID", `<xs:complexType name="Record"><xs:group ref="r:Fields"/><xs:attribute name="id" type="xs:ID"/></xs:complexType>`, `<xs:group name="Fields"><xs:choice><xs:element ref="r:value"/></xs:choice></xs:group><xs:element name="value" type="xs:integer"/>`, `type="xs:ID"`, errSchemaAttributeUseUnsupported, ``, false},
		{"simpleContent ID", `<xs:complexType name="Record"><xs:simpleContent><xs:extension base="xs:string"><xs:attribute name="id" type="xs:ID"/></xs:extension></xs:simpleContent></xs:complexType>`, ``, `type="xs:ID"`, errSchemaAttributeUseUnsupported, ``, false},
		{"inline complex ID", `<xs:element name="root"><xs:complexType><xs:attribute name="id" type="xs:ID"/></xs:complexType></xs:element>`, ``, `type="xs:ID"`, errSchemaAttributeUseUnsupported, ``, false},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, test := range tests {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				root := idUseRoot(test.body, test.extra)
				schema, err := parseIDUseSchema(t, root, nil, policy)
				if err == nil {
					t.Fatal("excluded ID use was admitted")
				}
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				want := elementReferenceTestAttributeLoc(t, root, test.marker)
				version := XSDVersion11
				if policy == Strict10 {
					version = XSDVersion10
				}
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != want || !errors.Is(err, ErrUnsupported) || !errors.Is(err, test.cause) {
					t.Fatalf("excluded %s diagnostic = %v", test.name, err)
				}
				wantSpec := schemaAttributeTypeSpecRef(version)
				if test.useSpec {
					wantSpec = schemaAttributeUseSpecRef(version)
				}
				var wantRelated []Loc
				if test.related != "" {
					wantRelated = []Loc{elementReferenceTestAttributeLoc(t, root, test.related)}
				}
				if diagnostic.SpecRef() != wantSpec || !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
					t.Fatalf("diagnostic metadata = %s/%v, want %s/%v", diagnostic.SpecRef(), diagnostic.Related(), wantSpec, wantRelated)
				}
			})
		}
	}
}

//nolint:gocognit // Each admitted owner must be rejected by both public consumers.
func TestIDAttributeUseConsumersRejectWithNoOutput(t *testing.T) {
	for _, shape := range []struct{ name, body, instance string }{
		{"empty", `<xs:attribute name="id" type="xs:ID"/>`, `<root xmlns="urn:root" id="a"/>`},
		{"choice", `<xs:choice><xs:element name="value" type="xs:integer"/></xs:choice><xs:attribute name="id" type="xs:ID"/>`, `<root xmlns="urn:root" id="a"><value>1</value></root>`},
		{"sequence", `<xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence><xs:attribute name="id" type="xs:ID"/>`, `<root xmlns="urn:root" id="a"><value>1</value></root>`},
	} {
		t.Run(shape.name, func(t *testing.T) {
			root := idUseRoot(`<xs:complexType name="Record">`+shape.body+`</xs:complexType><xs:element name="root" type="r:Record"/>`, ``)
			schema, err := parseIDUseSchema(t, root, nil, Strict11)
			if err != nil {
				t.Fatalf("ParseSchema: %v", err)
			}
			useLoc := elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="id"`)
			validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(shape.instance)))
			vd := requireDiagnostic(t, validationErr)
			if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != useLoc || !errors.Is(validationErr, errInstanceAttributes) {
				t.Fatalf("ValidateInstance = %v", validationErr)
			}
			output, generationErr := GenerateGo(schema, "generated")
			gd := requireDiagnostic(t, generationErr)
			if output != nil || gd.Class() != FailureUnsupported || gd.Code() != diagnosticCodegenUnsupported || gd.Loc() != useLoc || !errors.Is(generationErr, errCodegenUnsupported) {
				t.Fatalf("GenerateGo = %v/%v", output, generationErr)
			}
		})
	}
	root := groupedExtensionSchema("1.1", ` minOccurs="0" maxOccurs="0"`, `<xs:attribute name="id" type="xs:ID"/>`)
	schema, err := parseIDUseSchema(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("ParseSchema grouped extension: %v", err)
	}
	useLoc := elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="id"`)
	validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root" id="a"/>`)))
	vd := requireDiagnostic(t, validationErr)
	if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != useLoc || vd.SpecRef() != "xsd11-structures#cvc-elt" || !errors.Is(validationErr, errInstanceAttributes) {
		t.Fatalf("ValidateInstance grouped extension = %v", validationErr)
	}
	output, generationErr := GenerateGo(schema, "generated")
	gd := requireDiagnostic(t, generationErr)
	if output != nil || gd.Class() != FailureUnsupported || gd.Code() != diagnosticCodegenUnsupported || gd.Loc() != useLoc || !errors.Is(generationErr, errCodegenUnsupported) {
		t.Fatalf("GenerateGo grouped extension = %v/%v", output, generationErr)
	}
}
