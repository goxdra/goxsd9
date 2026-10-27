package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func idAllOwner(uses string) string {
	return `<xs:complexType name="Record"><xs:all><xs:element name="value" type="xs:integer"/></xs:all>` + uses + `</xs:complexType>`
}

func idAllVersion(policy LanguagePolicy) XSDVersion {
	if policy == Strict10 {
		return XSDVersion10
	}
	return XSDVersion11
}

func idAllUse(t *testing.T, schema Schema) LocalAttributeUse {
	t.Helper()
	found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Record"))
	if len(found) != 1 {
		t.Fatalf("Record components = %v", found)
	}
	uses := requireTestComplexTypeDefinition(t, found[0], "Record").AttributeUses()
	if len(uses) != 1 {
		t.Fatalf("Record attribute uses = %#v", uses)
	}
	use, ok := uses[0].(LocalAttributeUse)
	if !ok {
		t.Fatalf("Record use = %T", uses[0])
	}
	return use
}

//nolint:gocognit,funlen // One graph tests source order, type identity, provenance, and public copies together.
func TestDirectAllIDNamedGraphFactsAndCopies(t *testing.T) {
	root := idUseRoot(`<xs:include schemaLocation="included.xsd"/><xs:include schemaLocation="included.xsd"/><xs:include schemaLocation="chameleon.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/>
<xs:complexType name="Record"><xs:all minOccurs="0"><xs:element name="value" type="xs:integer" minOccurs="2" maxOccurs="unbounded"/><xs:element ref="r:neighbor"/></xs:all><xs:attribute name="builtin" type="xs:ID" use="required"/><xs:attribute name="forward" type="r:Derived"/><xs:attribute name="included" type="r:Included" form="unqualified"/><xs:attribute name="adopted" type="r:Adopted"/><xs:attribute name="foreign" type="o:Foreign"/></xs:complexType>
<xs:element name="neighbor" type="xs:integer"/><xs:simpleType name="Derived"><xs:restriction base="r:Forward"/></xs:simpleType><xs:simpleType name="Forward"><xs:restriction base="xs:ID"/></xs:simpleType>`, ``)
	root = strings.Replace(root, `xmlns:r="urn:root"`, `xmlns:r="urn:root" xmlns:o="urn:other"`, 1)
	fixtures := map[string]discoveryFixture{
		"root.xsd":      {id: "root.xsd", contents: root},
		"included.xsd":  {id: "included.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:include schemaLocation="root.xsd"/><xs:simpleType name="Included"><xs:restriction base="xs:ID"/></xs:simpleType></xs:schema>`},
		"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Adopted"><xs:restriction base="xs:ID"/></xs:simpleType></xs:schema>`},
		"other.xsd":     {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Foreign"><xs:restriction base="xs:ID"/></xs:simpleType></xs:schema>`},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema, err := parseIDUseSchema(t, root, fixtures, policy)
			if err != nil {
				t.Fatal(err)
			}
			again, err := parseIDUseSchema(t, root, fixtures, policy)
			if err != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
				t.Fatalf("unstable graph facts: %v", err)
			}
			if schema.LanguagePolicy() != policy {
				t.Fatalf("policy = %s", schema.LanguagePolicy())
			}
			for index, source := range []SourceID{"root.xsd", "included.xsd", "chameleon.xsd", "other.xsd"} {
				if documents := schema.Documents(); len(documents) != 4 || documents[index].Source() != source {
					t.Fatalf("document order = %v", documents)
				}
			}
			found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Record"))
			if len(found) != 1 {
				t.Fatalf("Record components = %v", found)
			}
			definition := requireTestComplexTypeDefinition(t, found[0], "Record")
			all, ok := definition.Particle().(AllParticle)
			if !ok || all.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:all minOccurs="0"`) || all.Occurrences().String() != "0/1" {
				t.Fatalf("all = %#v", definition.Particle())
			}
			members := all.Members()
			if len(members) != 2 {
				t.Fatalf("all members = %#v", members)
			}
			first, firstOK := members[0].(ElementParticle)
			second, secondOK := members[1].(ElementReferenceParticle)
			if !firstOK || !secondOK || first.Occurrences().String() != "2/unbounded" || second.TargetID() != schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:root", "neighbor"))[0].ID() {
				t.Fatalf("all members = %#v", members)
			}
			uses := definition.AttributeUses()
			if len(uses) != 5 {
				t.Fatalf("uses = %#v", uses)
			}
			for index, want := range []struct{ local, lexical, namespace, typeName, source, variety string }{
				{"builtin", "xs:ID", testXSDNamespace, "ID", "", ""},
				{"forward", "r:Derived", "urn:root", "Derived", "root.xsd", `<xs:restriction base="r:Forward"`},
				{"included", "r:Included", "urn:root", "Included", "included.xsd", `<xs:restriction base="xs:ID"`},
				{"adopted", "r:Adopted", "urn:root", "Adopted", "chameleon.xsd", `<xs:restriction base="xs:ID"`},
				{"foreign", "o:Foreign", "urn:other", "Foreign", "other.xsd", `<xs:restriction base="xs:ID"`},
			} {
				use, ok := uses[index].(LocalAttributeUse)
				wantType := mustTestQName(t, want.namespace, want.typeName)
				wantUseLoc := elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="`+want.local+`"`)
				wantTypeLoc := elementReferenceTestAttributeLoc(t, root, `type="`+want.lexical+`"`)
				if !ok || use.Loc() != wantUseLoc || use.NameLoc() != elementReferenceTestAttributeLoc(t, root, `name="`+want.local+`"`) || use.TypeLoc() != wantTypeLoc || use.DeclaredType() != wantType || use.Name().Local() != want.local {
					t.Fatalf("use %d = %#v", index, uses[index])
				}
				wantNameSpace := "urn:root"
				if want.local == "included" {
					wantNameSpace = ""
				}
				if use.Name().Namespace() != wantNameSpace {
					t.Fatalf("use %d mapped name = %s", index, use.Name())
				}
				wantFormLoc := Loc{}
				if want.local == "included" {
					wantFormLoc = elementReferenceTestAttributeLoc(t, root, `form="unqualified"`)
				}
				if use.FormLoc() != wantFormLoc {
					t.Fatalf("use %d form location = %s, want %s", index, use.FormLoc(), wantFormLoc)
				}
				ref, hasRef := use.TypeReference()
				if !hasRef || ref.Name() != wantType || ref.QName() != wantType || ref.Loc() != wantTypeLoc || ref.Variety() != SimpleTypeVarietyAtomicRestriction {
					t.Fatalf("use %d reference = %#v", index, ref)
				}
				id, hasID := use.TypeID()
				refID, hasRefID := ref.ComponentID()
				if index == 0 {
					if !ref.IsBuiltin() || hasID || hasRefID || !id.IsZero() || !refID.IsZero() || ref.VarietyLoc() != wantTypeLoc || use.Use() != AttributeUseRequired || use.UseLoc() != elementReferenceTestAttributeLoc(t, root, `use="required"`) {
						t.Fatalf("built-in identity/use = %#v/%v/%v", ref, id, use)
					}
					continue
				}
				declaration := fixtures[want.source].contents
				if want.source == "root.xsd" {
					declaration = root
				}
				if !ref.IsNamed() || !hasID || !hasRefID || id != refID || id != componentIDForName(t, schema, wantType) || id.Source() != SourceID(want.source) || ref.VarietyLoc() != allParticleTestTokenLoc(t, SourceID(want.source), declaration, want.variety, 1) || use.Use() != AttributeUseOptional || !use.UseLoc().IsZero() {
					t.Fatalf("named identity/use %d = %#v/%v/%v", index, ref, id, use)
				}
			}
			uses[0] = nil
			members[0] = nil
			minimum := first.Occurrences().Minimum()
			minimum.value.SetInt64(9)
			if fresh := definition.AttributeUses(); len(fresh) != 5 || fresh[0] == nil || all.Members()[0] == nil || all.Members()[0].Occurrences().String() != "2/unbounded" || !reflect.DeepEqual(schema.Components(), again.Components()) {
				t.Fatal("caller changed completed facts")
			}
			var walked []ComponentID
			if err := schema.Walk(func(component Component) error { walked = append(walked, component.ID()); return nil }); err != nil {
				t.Fatal(err)
			}
			if len(walked) != len(schema.Components()) {
				t.Fatalf("walked %d components, want %d", len(walked), len(schema.Components()))
			}
			for index, component := range schema.Components() {
				if walked[index] != component.ID() {
					t.Fatalf("walk order %d = %v, want %v", index, walked[index], component.ID())
				}
			}
		})
	}
}

//nolint:gocognit // Keep the three policies and omitted forms at one cardinality boundary.
func TestDirectAllIDPolicyCardinalityAndOmission(t *testing.T) {
	uses := `<xs:attribute name="first" type="xs:ID"/><xs:attribute name="gone" type="xs:ID" use="prohibited"/><xs:attribute name="second" type="r:Alias"/>`
	tail := `<xs:simpleType name="Alias"><xs:restriction base="xs:ID"/></xs:simpleType>`
	root := idUseRoot(idAllOwner(uses), tail)
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		schema, err := parseIDUseSchema(t, root, nil, policy)
		if err != nil {
			t.Fatalf("%s: %v", policy, err)
		}
		definition := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Record"))[0], "Record")
		retained := definition.AttributeUses()
		if len(retained) != 2 || retained[0].Name().Local() != "first" || retained[1].Name().Local() != "second" {
			t.Fatalf("%s effective ID order = %#v", policy, retained)
		}
	}
	schema, err := parseIDUseSchema(t, root, nil, Strict10)
	assertZeroSchema(t, schema)
	d := requireDiagnostic(t, err)
	if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaAttributeUseIDDuplicateCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="second"`) || !reflect.DeepEqual(d.Related(), []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="first"`)}) || d.SpecRef() != "xsd10-structures#ct-props-correct" || !errors.Is(err, errSchemaAttributeUseIDDuplicate) {
		t.Fatalf("Strict10 two-ID diagnostic = %v related %v", err, d.Related())
	}
	one := idUseRoot(idAllOwner(`<xs:attribute name="first" type="xs:ID"/><xs:attribute name="gone" type="xs:ID" use="prohibited"/>`), ``)
	schema, err = parseIDUseSchema(t, one, nil, Strict10)
	if err != nil || idAllUse(t, schema).Name().Local() != "first" {
		t.Fatalf("Strict10 one effective ID = %v/%v", schema, err)
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		omitted := idUseRoot(`<xs:complexType name="Record"><xs:all minOccurs="0" maxOccurs="0"><xs:element name="value" type="xs:integer"/></xs:all><xs:attribute name="id" type="xs:ID"/></xs:complexType>`, ``)
		schema, err := parseIDUseSchema(t, omitted, nil, policy)
		if err != nil {
			t.Fatalf("%s omitted all: %v", policy, err)
		}
		definition := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Record"))[0], "Record")
		if definition.Particle() != nil || idAllUse(t, schema).Name().Local() != "id" {
			t.Fatalf("%s omitted all lost ID use", policy)
		}
	}
}

func TestDirectAllIDExcludedUseShapes(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		version := idAllVersion(policy)
		for _, test := range []struct {
			name, use, extra, marker, related string
			spec, code                        string
			cause                             error
		}{
			{"inline ID", `<xs:attribute name="id"><xs:simpleType><xs:restriction base="xs:ID"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, "", schemaAttributeTypeSpecRef(version), UnsupportedSchemaSyntaxCode, errSchemaAttributeUseUnsupported},
			{"inline ID list", `<xs:attribute name="id"><xs:simpleType><xs:list itemType="xs:ID"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, "", schemaAttributeTypeSpecRef(version), UnsupportedSchemaSyntaxCode, errSchemaAttributeUseUnsupported},
			{"inline ID union", `<xs:attribute name="id"><xs:simpleType><xs:union memberTypes="xs:ID xs:ID"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, "", schemaAttributeTypeSpecRef(version), UnsupportedSchemaSyntaxCode, errSchemaAttributeUseUnsupported},
			{"named ID list", `<xs:attribute name="id" type="r:IDs"/>`, `<xs:simpleType name="IDs"><xs:list itemType="xs:ID"/></xs:simpleType>`, `type="r:IDs"`, "", schemaAttributeTypeSpecRef(version), UnsupportedSchemaSyntaxCode, errSchemaAttributeUseUnsupported},
			{"named ID union", `<xs:attribute name="id" type="r:IDs"/>`, `<xs:simpleType name="IDs"><xs:union memberTypes="xs:ID xs:ID"/></xs:simpleType>`, `type="r:IDs"`, "", schemaAttributeTypeSpecRef(version), UnsupportedSchemaSyntaxCode, errSchemaAttributeUseUnsupported},
			{"built-in default", `<xs:attribute name="id" type="xs:ID" default="a"/>`, "", `default="a"`, "", schemaAttributeUseSpecRef(version), UnsupportedSchemaSyntaxCode, errSchemaAttributeUseUnsupported},
			{"built-in fixed", `<xs:attribute name="id" type="xs:ID" fixed="a"/>`, "", `fixed="a"`, "", schemaAttributeUseSpecRef(version), UnsupportedSchemaSyntaxCode, errSchemaAttributeUseUnsupported},
			{"named default", `<xs:attribute name="id" type="r:Alias" default="a"/>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:ID"/></xs:simpleType>`, `default="a"`, "", schemaAttributeUseSpecRef(version), UnsupportedSchemaSyntaxCode, errSchemaAttributeUseUnsupported},
			{"named fixed", `<xs:attribute name="id" type="r:Alias" fixed="a"/>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:ID"/></xs:simpleType>`, `fixed="a"`, "", schemaAttributeUseSpecRef(version), UnsupportedSchemaSyntaxCode, errSchemaAttributeUseUnsupported},
			{"inline default", `<xs:attribute name="id" default="a"><xs:simpleType><xs:restriction base="xs:ID"/></xs:simpleType></xs:attribute>`, "", `default="a"`, "", schemaAttributeUseSpecRef(version), UnsupportedSchemaSyntaxCode, errSchemaAttributeUseUnsupported},
			{"inline fixed", `<xs:attribute name="id" fixed="a"><xs:simpleType><xs:restriction base="xs:ID"/></xs:simpleType></xs:attribute>`, "", `fixed="a"`, "", schemaAttributeUseSpecRef(version), UnsupportedSchemaSyntaxCode, errSchemaAttributeUseUnsupported},
			{"attribute ref", `<xs:attribute ref="r:id"/>`, `<xs:attribute name="id" type="xs:ID"/>`, `ref="r:id"`, `<xs:attribute name="id"`, schemaAttributeUseSpecRef(version), UnsupportedSchemaSyntaxCode, errSchemaAttributeReferenceUnsupported},
		} {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				root := idUseRoot(idAllOwner(test.use), test.extra)
				schema, err := parseIDUseSchema(t, root, nil, policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				var related []Loc
				if test.related != "" {
					related = []Loc{elementReferenceTestAttributeLoc(t, root, test.related)}
				}
				if d.Class() != FailureUnsupported || d.Code() != test.code || d.Loc() != elementReferenceTestAttributeLoc(t, root, test.marker) || d.SpecRef() != test.spec || !reflect.DeepEqual(d.Related(), related) || !errors.Is(err, ErrUnsupported) || !errors.Is(err, test.cause) {
					t.Fatalf("excluded %s = %v related %v", test.name, err, d.Related())
				}
			})
		}
	}
}

func TestDirectAllIDInlineOwnerStaysUnsupported(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, test := range []struct{ name, lexical, extra string }{
			{"built-in", "xs:ID", ""},
			{"named", "r:Alias", `<xs:simpleType name="Alias"><xs:restriction base="xs:ID"/></xs:simpleType>`},
		} {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				root := idUseRoot(`<xs:element name="root"><xs:complexType><xs:all><xs:element name="value" type="xs:integer"/></xs:all><xs:attribute name="id" type="`+test.lexical+`"/></xs:complexType></xs:element>`, test.extra)
				schema, err := parseIDUseSchema(t, root, nil, policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:all>`) || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("inline all owner = %v", err)
				}
			})
		}
	}
}

func TestDirectAllNamedIDConsumersRejectAtUse(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		root := idUseRoot(idAllOwner(`<xs:attribute name="id" type="r:Alias"/>`)+`<xs:element name="root" type="r:Record"/>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:ID"/></xs:simpleType>`)
		schema, err := parseIDUseSchema(t, root, nil, policy)
		if err != nil {
			t.Fatalf("%s: %v", policy, err)
		}
		want := elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="id"`)
		ownerLoc := elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Record"`)
		elementLoc := elementReferenceTestAttributeLoc(t, root, `<xs:element name="root"`)
		instanceErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root" id="a"><value>1</value></root>`)))
		vd := requireDiagnostic(t, instanceErr)
		if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != want || vd.SpecRef() != instanceValidationSpecRef(idAllVersion(policy)) || !reflect.DeepEqual(vd.Related(), []Loc{elementLoc, ownerLoc, want}) || !errors.Is(instanceErr, ErrUnsupported) || !errors.Is(instanceErr, errInstanceAttributes) {
			t.Fatalf("%s ValidateInstance = %v", policy, instanceErr)
		}
		output, generationErr := GenerateGo(schema, "generated")
		gd := requireDiagnostic(t, generationErr)
		if output != nil || gd.Class() != FailureUnsupported || gd.Code() != diagnosticCodegenUnsupported || gd.Loc() != want || gd.SpecRef() != codegenDirectSequenceSpecReference(idAllVersion(policy), codegenDirectSequenceParticlesReference) || !reflect.DeepEqual(gd.Related(), []Loc{ownerLoc, want}) || !errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenUnsupported) {
			t.Fatalf("%s GenerateGo = %v/%v", policy, output, generationErr)
		}
	}
}

//nolint:gocognit // Alternate exits at the newly admitted all-use boundary keep their exact diagnostics.
func TestDirectAllIDBoundaryDiagnostics(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		version := idAllVersion(policy)
		for _, test := range []struct {
			name, uses, extra, primary, related, code, spec string
			primaryOccurrence                               int
			class                                           FailureClass
			cause, resolverCause                            error
		}{
			{"duplicate use", `<xs:attribute name="id" type="xs:ID"/><xs:attribute name="id" type="xs:ID" use="required"/>`, "", `<xs:attribute name="id"`, `<xs:attribute name="id"`, diagnosticSchemaAttributeUseDuplicateCode, schemaAttributeUseSpecRef(version), 2, FailureInvalid, errSchemaAttributeUseDuplicate, nil},
			{"invalid use", `<xs:attribute name="id" type="xs:ID" use="sometimes"/>`, "", `use="sometimes"`, "", invalidSchemaCompositionCode, schemaAttributeUseSpecRef(version), 1, FailureInvalid, nil, nil},
			{"invalid form", `<xs:attribute name="id" type="xs:ID" form="neither"/>`, "", `form="neither"`, "", invalidSchemaCompositionCode, schemaAttributeUseSpecRef(version), 1, FailureInvalid, nil, nil},
			{"unresolved", `<xs:attribute name="id" type="r:Missing"/>`, "", `type="r:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(version), 1, FailureInvalid, errSchemaAttributeTypeUnresolved, errSchemaSimpleTypeBaseUnresolved},
			{"prohibited unresolved", `<xs:attribute name="id" type="r:Missing" use="prohibited"/>`, "", `type="r:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(version), 1, FailureInvalid, errSchemaAttributeTypeUnresolved, errSchemaSimpleTypeBaseUnresolved},
			{"wrong kind", `<xs:attribute name="id" type="r:Wrong"/>`, `<xs:element name="Wrong" type="xs:integer"/>`, `type="r:Wrong"`, `<xs:element name="Wrong"`, diagnosticSchemaAttributeTypeWrongKindCode, schemaAttributeTypeSpecRef(version), 1, FailureInvalid, errSchemaAttributeTypeWrongKind, errSchemaSimpleTypeBaseWrongKind},
			{"cycle", `<xs:attribute name="id" type="r:First"/>`, `<xs:simpleType name="First"><xs:restriction base="r:Second"/></xs:simpleType><xs:simpleType name="Second"><xs:restriction base="r:First"/></xs:simpleType>`, `type="r:First"`, "", diagnosticSchemaAttributeTypeCycleCode, schemaAttributeTypeSpecRef(version), 1, FailureInvalid, errSchemaSimpleTypeBaseCycle, nil},
			{"unsupported facet", `<xs:attribute name="id" type="r:Patterned"/>`, `<xs:simpleType name="Patterned"><xs:restriction base="xs:ID"><xs:pattern value="[a-z]+"/></xs:restriction></xs:simpleType>`, `<xs:pattern`, "", UnsupportedDatatypeFacetCode, tokenDiagnosticSpecRef(version, "decimal"), 1, FailureUnsupported, ErrUnsupported, nil},
			{"duplicate target", `<xs:attribute name="id" type="r:Alias"/>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:ID"/></xs:simpleType><xs:simpleType name="Alias" id="second"><xs:restriction base="xs:ID"/></xs:simpleType>`, `<xs:simpleType name="Alias" id="second"`, `<xs:simpleType name="Alias"><xs:restriction`, diagnosticSchemaGlobalDuplicateCode, schemaGlobalDuplicateSpecRef(version), 1, FailureInvalid, errSchemaGlobalDeclarationDuplicate, nil},
		} {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				root := idUseRoot(idAllOwner(test.uses), test.extra)
				schema, err := parseIDUseSchema(t, root, nil, policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				want := allParticleTestTokenLoc(t, "root.xsd", root, test.primary, test.primaryOccurrence)
				var related []Loc
				if test.related != "" {
					related = []Loc{elementReferenceTestAttributeLoc(t, root, test.related)}
				}
				if test.name == "cycle" {
					related = []Loc{
						elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="id"`),
						elementReferenceTestAttributeLoc(t, root, `base="r:Second"`),
						elementReferenceTestAttributeLoc(t, root, `base="r:First"`),
						elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="First"`),
						elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="Second"`),
					}
				}
				if d.Class() != test.class || d.Code() != test.code || d.Loc() != want || d.SpecRef() != test.spec || !reflect.DeepEqual(d.Related(), related) || test.cause != nil && !errors.Is(err, test.cause) || test.resolverCause != nil && !errors.Is(err, test.resolverCause) {
					t.Fatalf("%s = %v, related %v; want %s/%s/%s/%v", test.name, err, d.Related(), test.code, want, test.spec, related)
				}
			})
		}
	}
}

func TestDirectAllIDGraphVisibilityAndAcquisition(t *testing.T) {
	root := idUseRoot(`<xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/>`+idAllOwner(`<xs:attribute name="id" type="o:Foreign"/>`), ``)
	root = strings.Replace(root, `xmlns:r="urn:root"`, `xmlns:r="urn:root" xmlns:o="urn:other"`, 1)
	fixtures := map[string]discoveryFixture{
		"bridge.xsd": {id: "bridge.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge"><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`},
		"other.xsd":  {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Foreign"><xs:restriction base="xs:ID"/></xs:simpleType></xs:schema>`},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		schema, err := parseIDUseSchema(t, root, fixtures, policy)
		assertZeroSchema(t, schema)
		d := requireDiagnostic(t, err)
		if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaAttributeTypeUnresolvedCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `type="o:Foreign"`) || len(d.Related()) != 0 || d.SpecRef() != schemaAttributeTypeSpecRef(idAllVersion(policy)) || !errors.Is(err, errSchemaAttributeTypeUnresolved) {
			t.Fatalf("%s invisible type = %v", policy, err)
		}
		missing := idUseRoot(`<xs:include schemaLocation="missing.xsd"/>`+idAllOwner(`<xs:attribute name="id" type="r:Included"/>`), ``)
		cause := errors.New("ID type source unavailable")
		source, sourceErr := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(missing)))
		if sourceErr != nil {
			t.Fatal(sourceErr)
		}
		schema, err = ParseSchemaWithPolicy(source, &discoveryResolver{failures: map[string]error{"missing.xsd": cause}}, policy)
		assertZeroSchema(t, schema)
		d = requireDiagnostic(t, err)
		if d.Class() != FailureResolution || d.Code() != SourceResolveCode || d.Loc() != elementReferenceTestAttributeLoc(t, missing, `<xs:include`) || len(d.Related()) != 0 || !errors.Is(err, cause) {
			t.Fatalf("%s source acquisition = %v", policy, err)
		}
	}
}

func TestDirectAllIDStrict10NamedTargetsAcrossDiscoveryGraph(t *testing.T) {
	root := idUseRoot(`<xs:include schemaLocation="included.xsd"/><xs:include schemaLocation="included.xsd"/><xs:include schemaLocation="chameleon.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/>
<xs:complexType name="ForwardOwner"><xs:all><xs:element name="value" type="xs:integer"/></xs:all><xs:attribute name="id" type="r:Forward"/></xs:complexType>
<xs:complexType name="IncludedOwner"><xs:all><xs:element name="value" type="xs:integer"/></xs:all><xs:attribute name="id" type="r:Included"/></xs:complexType>
<xs:complexType name="AdoptedOwner"><xs:all><xs:element name="value" type="xs:integer"/></xs:all><xs:attribute name="id" type="r:Adopted"/></xs:complexType>
<xs:complexType name="ForeignOwner"><xs:all><xs:element name="value" type="xs:integer"/></xs:all><xs:attribute name="id" type="o:Foreign"/></xs:complexType>
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
		t.Fatal(err)
	}
	for _, want := range []struct{ owner, namespace, name, lexical, source string }{
		{"ForwardOwner", "urn:root", "Forward", "r:Forward", "root.xsd"},
		{"IncludedOwner", "urn:root", "Included", "r:Included", "included.xsd"},
		{"AdoptedOwner", "urn:root", "Adopted", "r:Adopted", "chameleon.xsd"},
		{"ForeignOwner", "urn:other", "Foreign", "o:Foreign", "other.xsd"},
	} {
		found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", want.owner))
		if len(found) != 1 {
			t.Fatalf("%s owners = %v", want.owner, found)
		}
		definition := requireTestComplexTypeDefinition(t, found[0], want.owner)
		if _, ok := definition.Particle().(AllParticle); !ok {
			t.Fatalf("%s particle = %T", want.owner, definition.Particle())
		}
		uses := definition.AttributeUses()
		if len(uses) != 1 {
			t.Fatalf("%s uses = %v", want.owner, uses)
		}
		use, ok := uses[0].(LocalAttributeUse)
		name := mustTestQName(t, want.namespace, want.name)
		id, hasID := use.TypeID()
		ref, hasRef := use.TypeReference()
		if !ok || !hasID || !hasRef || !ref.IsNamed() || use.DeclaredType() != name || ref.Name() != name || id != componentIDForName(t, schema, name) || id.Source() != SourceID(want.source) || use.TypeLoc() != elementReferenceTestAttributeLoc(t, root, `type="`+want.lexical+`"`) {
			t.Fatalf("%s named ID facts = %v/%v/%v", want.owner, use, id, ref)
		}
	}
}
