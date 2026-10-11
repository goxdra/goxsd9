package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func localInheritableSchema(owner, declarations string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" attributeFormDefault="qualified">` +
		owner + declarations + `</xs:schema>`
}

func localInheritableDirect(body string) string {
	return `<xs:complexType name="Record">` + body + `</xs:complexType>`
}

func localInheritableUse(spelling string) string {
	return `<xs:attribute name="flag" type="xs:boolean" use="required"` + spelling + `/>`
}

//nolint:gocognit // The matrix compares public facts across owners and spellings.
func TestLocalInheritableFalseMatchesOmissionOnSupportedOwners(t *testing.T) {
	owners := []struct {
		name  string
		build func(string) string
	}{
		{"empty", localInheritableDirect},
		{"choice", func(attribute string) string {
			return localInheritableDirect(`<xs:choice><xs:element name="item" type="xs:integer"/></xs:choice>` + attribute)
		}},
		{"sequence", func(attribute string) string {
			return localInheritableDirect(`<xs:sequence><xs:element name="item" type="xs:integer"/></xs:sequence>` + attribute)
		}},
		{"group", func(attribute string) string {
			return localInheritableDirect(`<xs:group ref="t:Fields"/>` + attribute)
		}},
		{"grouped extension", func(attribute string) string {
			return `<xs:complexType name="Record"><xs:complexContent><xs:extension base="t:Base"><xs:group ref="t:Fields" minOccurs="2" maxOccurs="18446744073709551616"/>` + attribute + `</xs:extension></xs:complexContent></xs:complexType>`
		}},
	}
	declarations := `<xs:group name="Fields"><xs:sequence/></xs:group><xs:complexType name="Base"/>`
	spellings := []struct{ name, text string }{
		{"omitted", ""},
		{"false", ` inheritable="false"`},
		{"zero", ` inheritable="0"`},
		{"XML whitespace", " inheritable=\" &#x9;false&#xA;\""},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, owner := range owners {
			for _, spelling := range spellings {
				t.Run(string(policy)+"/"+owner.name+"/"+spelling.name, func(t *testing.T) {
					attribute := localInheritableUse(spelling.text)
					root := localInheritableSchema(owner.build(attribute), declarations)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
					if err != nil {
						t.Fatalf("parse: %v", err)
					}
					components := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Record"))
					if len(components) != 1 {
						t.Fatalf("Record components = %d, want 1", len(components))
					}
					definition := requireTestComplexTypeDefinition(t, components[0], "Record")
					uses := definition.AttributeUses()
					if len(uses) != 1 {
						t.Fatalf("attribute uses = %d, want 1", len(uses))
					}
					use, ok := uses[0].(LocalAttributeUse)
					if !ok || use.Name() != mustTestQName(t, "urn:root", "flag") || use.Use() != AttributeUseRequired ||
						use.DeclaredType() != mustTestQName(t, testXSDNamespace, "boolean") {
						t.Fatalf("local use = %#v", uses[0])
					}
					if use.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="flag"`) ||
						use.NameLoc() != elementReferenceTestAttributeLoc(t, root, `name="flag"`) ||
						use.TypeLoc() != elementReferenceTestAttributeLoc(t, root, `type="xs:boolean"`) ||
						use.UseLoc() != elementReferenceTestAttributeLoc(t, root, `use="required"`) {
						t.Fatalf("local use provenance = %s/%s/%s/%s", use.Loc(), use.NameLoc(), use.TypeLoc(), use.UseLoc())
					}
					typeRef, ok := use.TypeReference()
					if !ok || !typeRef.IsBuiltin() || typeRef.Name() != mustTestQName(t, testXSDNamespace, "boolean") {
						t.Fatalf("type reference = %#v/%t", typeRef, ok)
					}
					if id, ok := use.TypeID(); ok || !id.IsZero() {
						t.Fatalf("built-in type ID = %v/%t", id, ok)
					}
					if owner.name == "grouped extension" {
						group, ok := definition.Particle().(ModelGroupReferenceParticle)
						if !ok || group.Occurrences().Minimum().String() != "2" || group.Occurrences().Maximum().String() != "18446744073709551616" {
							t.Fatalf("grouped extension bounds = %#v", definition.Particle())
						}
					}
					before := definition.AttributeUses()
					uses[0] = nil
					if !reflect.DeepEqual(before, definition.AttributeUses()) {
						t.Fatal("mutating returned use slice changed completed facts")
					}
				})
			}
		}
	}
}

//nolint:gocognit // Keep scalar identity, location, and ordered facts together.
func TestLocalInheritableFalseScalarAndOrderFacts(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			attributes := `<xs:attribute name="first" type="xs:integer" inheritable="0"/>` +
				`<xs:attribute name="second" type="t:Named" inheritable=" false "/>` +
				`<xs:attribute name="third" inheritable="false"><xs:simpleType><xs:restriction base="xs:decimal"/></xs:simpleType></xs:attribute>` +
				`<xs:attribute name="fourth" type="xs:ID" inheritable="0"/>` +
				`<xs:attribute name="excluded" type="xs:boolean" use="prohibited" inheritable="false"/>`
			root := localInheritableSchema(localInheritableDirect(attributes), `<xs:simpleType name="Named"><xs:restriction base="xs:integer"/></xs:simpleType>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			definition := requireTestComplexTypeDefinition(t, schema.Components()[0], "Record")
			uses := definition.AttributeUses()
			if len(uses) != 4 {
				t.Fatalf("uses = %d, want 4", len(uses))
			}
			for index, name := range []string{"first", "second", "third", "fourth"} {
				use, ok := uses[index].(LocalAttributeUse)
				if !ok || use.Name() != mustTestQName(t, "urn:root", name) {
					t.Fatalf("use %d = %#v, want %s", index, uses[index], name)
				}
				if use.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="`+name+`"`) {
					t.Fatalf("use %d Loc = %s", index, use.Loc())
				}
			}
			second, ok := uses[1].(LocalAttributeUse)
			if !ok {
				t.Fatalf("second use = %T", uses[1])
			}
			if id, hasID := second.TypeID(); !hasID || id != schema.Components()[1].ID() {
				t.Fatalf("named type identity = %v/%t", id, hasID)
			}
			third, ok := uses[2].(LocalAttributeUse)
			if !ok {
				t.Fatalf("third use = %T", uses[2])
			}
			ref, ok := third.TypeReference()
			if !ok || !ref.IsAnonymous() || third.TypeLoc() != elementReferenceTestAttributeLoc(t, root, `<xs:simpleType>`) {
				t.Fatalf("inline type = %#v/%t, Loc %s", ref, ok, third.TypeLoc())
			}
			first, ok := uses[0].(LocalAttributeUse)
			if !ok {
				t.Fatalf("first use = %T", uses[0])
			}
			if first.UseLoc() != (Loc{}) || first.Use() != AttributeUseOptional {
				t.Fatalf("default use = %q/%s", first.Use(), first.UseLoc())
			}
		})
	}
}

//nolint:gocognit // Keep every excluded owner and lexical exit in one matrix.
func TestLocalInheritableMalformedAndExcludedShapes(t *testing.T) {
	ownerCases := []struct {
		name, owner, extra string
	}{
		{"direct named", localInheritableDirect(`<xs:attribute name="flag" type="xs:boolean" inheritable="%s"/>`), ""},
		{"direct anonymous inline", `<xs:element name="root"><xs:complexType><xs:attribute name="flag" type="xs:boolean" inheritable="%s"/></xs:complexType></xs:element>`, ""},
		{"direct all", localInheritableDirect(`<xs:all/><xs:attribute name="flag" type="xs:boolean" inheritable="%s"/>`), ""},
		{"simpleContent extension", `<xs:complexType name="Record"><xs:simpleContent><xs:extension base="xs:string"><xs:attribute name="flag" type="xs:boolean" inheritable="%s"/></xs:extension></xs:simpleContent></xs:complexType>`, ""},
		{"sequence extension", `<xs:complexType name="Record"><xs:complexContent><xs:extension base="t:Base"><xs:sequence/><xs:attribute name="flag" type="xs:boolean" inheritable="%s"/></xs:extension></xs:complexContent></xs:complexType>`, `<xs:complexType name="Base"/>`},
		{"grouped extension", `<xs:complexType name="Record"><xs:complexContent><xs:extension base="t:Base"><xs:group ref="t:Fields"/><xs:attribute name="flag" type="xs:boolean" inheritable="%s"/></xs:extension></xs:complexContent></xs:complexType>`, `<xs:group name="Fields"><xs:sequence/></xs:group><xs:complexType name="Base"/>`},
		{"grouped anonymous inline", `<xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:Base"><xs:group ref="t:Fields"/><xs:attribute name="flag" type="xs:boolean" inheritable="%s"/></xs:extension></xs:complexContent></xs:complexType></xs:element>`, `<xs:group name="Fields"><xs:sequence/></xs:group><xs:complexType name="Base"/>`},
		{"attribute group", `<xs:attributeGroup name="Attrs"><xs:attribute name="flag" type="xs:boolean" inheritable="%s"/></xs:attributeGroup>`, ""},
		{"local ref", localInheritableDirect(`<xs:attribute ref="t:global" inheritable="%s"/>`), `<xs:attribute name="global" type="xs:boolean" inheritable="true"/>`},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		version := XSDVersion11
		if policy == Strict10 {
			version = XSDVersion10
		}
		for _, owner := range ownerCases {
			for _, spelling := range []string{"maybe", "true", "1", "false", "0"} {
				if (owner.name == "direct named" || owner.name == "grouped extension") && (spelling == "false" || spelling == "0") && policy != Strict10 {
					continue
				}
				t.Run(string(policy)+"/"+owner.name+"/"+spelling, func(t *testing.T) {
					root := localInheritableSchema(strings.Replace(owner.owner, "%s", spelling, 1), owner.extra)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
					if err == nil {
						t.Fatal("excluded or malformed inheritable accepted")
					}
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					wantLoc := elementReferenceTestAttributeLoc(t, root, `inheritable="`+spelling+`"`)
					if d.Loc() != wantLoc {
						t.Fatalf("primary Loc = %s, want %s: %v", d.Loc(), wantLoc, err)
					}
					if len(d.Related()) != 0 {
						t.Fatalf("related Locs = %v, want none", d.Related())
					}
					if spelling == "maybe" {
						cause := errors.Unwrap(d)
						var lexical Diagnostic
						if d.Code() != invalidSchemaCompositionCode || d.Class() != FailureInvalid || d.SpecRef() != strictBooleanSpecRef(version) ||
							!errors.As(cause, &lexical) || lexical.Code() != InvalidBooleanLexicalCode || lexical.Loc() != wantLoc || lexical.SpecRef() != strictBooleanSpecRef(version) {
							t.Fatalf("malformed diagnostic = %#v, cause %v", d, cause)
						}
						return
					}
					if policy == Strict10 {
						if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.SpecRef() != "xsd11-structures#cSchemaDocument" || !errors.Is(err, errLanguagePolicyMismatch) {
							t.Fatalf("Strict10 = %v", err)
						}
						return
					}
					wantSpec := "xsd11-structures#cSchemaDocument"
					if owner.name == "sequence extension" || owner.name == "grouped extension" || owner.name == "grouped anonymous inline" {
						wantSpec = schemaComplexTypeExtensionSpecRef(version)
					}
					if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.SpecRef() != wantSpec || !errors.Is(err, ErrUnsupported) {
						t.Fatalf("excluded shape = %v, spec=%q", err, d.SpecRef())
					}
				})
			}
		}
	}
}

func TestLocalInheritableFalseConsumersMatchOmission(t *testing.T) {
	for _, spelling := range []string{"", ` inheritable="false"`, ` inheritable="0"`} {
		t.Run(spelling, func(t *testing.T) {
			root := localInheritableSchema(localInheritableDirect(`<xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence>`+localInheritableUse(spelling)), `<xs:element name="root" type="t:Record"/>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			err = ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<t:root xmlns:t="urn:root"><t:value>1</t:value></t:root>`)))
			d := requireDiagnostic(t, err)
			want := elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="flag"`)
			if d.Class() != FailureUnsupported || d.Code() != UnsupportedInstanceValidationCode || d.Loc() != want || !errors.Is(err, errInstanceAttributes) {
				t.Fatalf("validation = %v", err)
			}
			generated, err := GenerateGo(schema, "generated")
			d = requireDiagnostic(t, err)
			if generated != nil || d.Class() != FailureUnsupported || d.Code() != diagnosticCodegenUnsupported || d.Loc() != want || !errors.Is(err, errCodegenUnsupported) {
				t.Fatalf("generation = %q/%v", generated, err)
			}
		})
	}
}

func TestLocalInheritableFalseGraphDiscoveryAndVisibility(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" attributeFormDefault="qualified">
  <xs:include schemaLocation="child.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:complexType name="Root"><xs:attribute name="first" type="o:Amount" inheritable="false"/></xs:complexType>
</xs:schema>`
	child := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" attributeFormDefault="qualified">
  <xs:include schemaLocation="root.xsd"/>
  <xs:complexType name="Child"><xs:attribute name="second" type="xs:integer" inheritable="0"/></xs:complexType>
</xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Amount"><xs:restriction base="xs:decimal"/></xs:simpleType></xs:schema>`
	fixtures := map[string]discoveryFixture{
		"root.xsd":  {id: "root.xsd", contents: root},
		"child.xsd": {id: "child.xsd", contents: child},
		"other.xsd": {id: "other.xsd", contents: other},
	}
	schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, Strict11)
	if err != nil {
		t.Fatalf("parse graph: %v", err)
	}
	documents := schema.Documents()
	if len(documents) != 3 {
		t.Fatalf("documents = %d, want root/include/import", len(documents))
	}
	for index, source := range []SourceID{"root.xsd", "child.xsd", "other.xsd"} {
		if documents[index].Source() != source {
			t.Fatalf("document %d source = %q, want %q", index, documents[index].Source(), source)
		}
	}
	for _, test := range []struct {
		name, source, local string
	}{
		{"Root", "root.xsd", "first"},
		{"Child", "child.xsd", "second"},
	} {
		components := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", test.name))
		if len(components) != 1 {
			t.Fatalf("%s components = %d", test.name, len(components))
		}
		definition := requireTestComplexTypeDefinition(t, components[0], test.name)
		uses := definition.AttributeUses()
		if len(uses) != 1 || uses[0].Name() != mustTestQName(t, "urn:root", test.local) || uses[0].Loc().Source() != SourceID(test.source) {
			t.Fatalf("%s uses = %#v", test.name, uses)
		}
	}
	rootUse, ok := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Root"))[0], "Root").AttributeUses()[0].(LocalAttributeUse)
	if !ok {
		t.Fatal("Root use is not local")
	}
	if rootUse.DeclaredType() != mustTestQName(t, "urn:other", "Amount") {
		t.Fatalf("imported type = %q", rootUse.DeclaredType())
	}
	if id, ok := rootUse.TypeID(); !ok || id.Source() != SourceID("other.xsd") {
		t.Fatalf("imported type identity = %v/%t", id, ok)
	}
}

//nolint:gocognit // Preserve related locations and causes for alternate exits.
func TestLocalInheritableFalseBoundaryDiagnostics(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		version := XSDVersion11
		for _, tc := range []struct {
			name, attributes, declarations, marker, related string
			code                                            string
			cause                                           error
			spec                                            string
		}{
			{"duplicate effective name", `<xs:attribute name="flag" type="xs:boolean" inheritable="false"/><xs:attribute name="flag" type="xs:integer"/>`, "", `<xs:attribute name="flag" type="xs:integer"`, `<xs:attribute name="flag" type="xs:boolean"`, diagnosticSchemaAttributeUseDuplicateCode, errSchemaAttributeUseDuplicate, schemaAttributeUseSpecRef(version)},
			{"duplicate global type", `<xs:attribute name="flag" type="t:Named" inheritable="false"/>`, `<xs:simpleType name="Named"><xs:restriction base="xs:integer"/></xs:simpleType><xs:simpleType name="Named" id="second"><xs:restriction base="xs:integer"/></xs:simpleType>`, `<xs:simpleType name="Named" id="second"`, `<xs:simpleType name="Named"><xs:restriction`, diagnosticSchemaGlobalDuplicateCode, errSchemaGlobalDeclarationDuplicate, schemaGlobalDuplicateSpecRef(version)},
			{"unresolved type", `<xs:attribute name="flag" type="t:Missing" inheritable="0"/>`, "", `type="t:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, errSchemaAttributeTypeUnresolved, schemaAttributeTypeSpecRef(version)},
			{"wrong kind type", `<xs:attribute name="flag" type="t:Wrong" inheritable="false"/>`, `<xs:element name="Wrong" type="xs:integer"/>`, `type="t:Wrong"`, `<xs:element name="Wrong"`, diagnosticSchemaAttributeTypeWrongKindCode, errSchemaAttributeTypeWrongKind, schemaAttributeTypeSpecRef(version)},
			{"prohibited unresolved", `<xs:attribute name="flag" type="t:Missing" use="prohibited" inheritable="false"/>`, "", `type="t:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, errSchemaAttributeTypeUnresolved, schemaAttributeTypeSpecRef(version)},
		} {
			t.Run(string(policy)+"/"+tc.name, func(t *testing.T) {
				root := localInheritableSchema(localInheritableDirect(tc.attributes), tc.declarations)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err == nil {
					t.Fatal("invalid boundary accepted")
				}
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				wantRelated := []Loc(nil)
				if tc.related != "" {
					wantRelated = []Loc{elementReferenceTestAttributeLoc(t, root, tc.related)}
				}
				if d.Class() != FailureInvalid || d.Code() != tc.code || d.Loc() != elementReferenceTestAttributeLoc(t, root, tc.marker) ||
					!reflect.DeepEqual(d.Related(), wantRelated) || d.SpecRef() != tc.spec || !errors.Is(err, tc.cause) {
					t.Fatalf("boundary diagnostic = %v related=%v wantRelated=%v spec=%q wantSpec=%q loc=%s wantLoc=%s cause=%t", err, d.Related(), wantRelated, d.SpecRef(), tc.spec, d.Loc(), elementReferenceTestAttributeLoc(t, root, tc.marker), errors.Is(err, tc.cause))
				}
			})
		}
	}
}

func TestLocalInheritableFalseInvisibleTypeRetainsResolutionDiagnostic(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:root">
  <xs:complexType name="Record"><xs:attribute name="flag" type="o:Foreign" inheritable="false"/></xs:complexType>
</xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Foreign"><xs:restriction base="xs:integer"/></xs:simpleType></xs:schema>`
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			documents := []*syntaxDocument{
				elementReferenceTestSyntaxDocument(t, "root.xsd", root),
				elementReferenceTestSyntaxDocument(t, "other.xsd", other),
			}
			schema, err := newSchemaFromDiscoveryWithPolicy(syntaxDiscoveryResult{documents: documents}, policy)
			if err == nil {
				t.Fatal("invisible type accepted")
			}
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaAttributeTypeUnresolvedCode ||
				d.Loc() != elementReferenceTestAttributeLoc(t, root, `type="o:Foreign"`) ||
				d.SpecRef() != schemaAttributeTypeSpecRef(XSDVersion11) || !errors.Is(err, errSchemaAttributeTypeUnresolved) {
				t.Fatalf("invisible type = %v related=%v", err, d.Related())
			}
			if len(d.Related()) != 0 {
				t.Fatalf("invisible type related = %v, want no visible target", d.Related())
			}
		})
	}
}

//nolint:gocognit // Keep the grouped graph diagnostic matrix together.
func TestLocalInheritableFalseGroupedGraphExits(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, tc := range []struct {
			name, group, marker string
			code                string
			cause               error
			related             string
			spec                string
		}{
			{"missing group", "", `ref="t:Fields"`, diagnosticSchemaModelGroupReferenceUnresolvedCode, errSchemaModelGroupReferenceUnresolved, "", schemaModelGroupReferenceSpecRef(XSDVersion11)},
			{"cycle", `<xs:group name="Fields"><xs:choice><xs:group ref="t:Fields"/></xs:choice></xs:group>`, `<xs:group ref="t:Fields"/></xs:choice>`, diagnosticSchemaModelGroupReferenceCycleCode, errSchemaModelGroupReferenceCycle, `<xs:group name="Fields"`, "xsd11-structures#mg-props-correct"},
		} {
			t.Run(string(policy)+"/"+tc.name, func(t *testing.T) {
				root := localInheritableSchema(
					`<xs:complexType name="Record"><xs:complexContent><xs:extension base="t:Base"><xs:group ref="t:Fields"/><xs:attribute name="flag" type="xs:boolean" inheritable="false"/></xs:extension></xs:complexContent></xs:complexType>`,
					tc.group+`<xs:complexType name="Base"/>`,
				)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err == nil {
					t.Fatal("group graph exit accepted")
				}
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				wantLoc := elementReferenceTestAttributeLoc(t, root, tc.marker)
				if tc.name == "cycle" {
					wantLoc = elementReferenceTestAttributeLoc(t, root, `ref="t:Fields"/></xs:choice>`)
				}
				if d.Code() != tc.code || d.Class() != FailureInvalid || d.Loc() != wantLoc || d.SpecRef() != tc.spec || !errors.Is(err, tc.cause) {
					t.Fatalf("group graph exit = %v related=%v spec=%q", err, d.Related(), d.SpecRef())
				}
				if tc.related != "" && (len(d.Related()) == 0 || d.Related()[0] != elementReferenceTestAttributeLoc(t, root, tc.related)) {
					t.Fatalf("group related = %v, want target group", d.Related())
				}
			})
		}
	}
}

func TestLocalInheritableFalseGroupedConsumersMatchOmission(t *testing.T) {
	for _, spelling := range []string{"", ` inheritable="false"`, ` inheritable="0"`} {
		t.Run(spelling, func(t *testing.T) {
			root := groupedExtensionSchema("1.1", "", localInheritableUse(spelling))
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			err = ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
			d := requireDiagnostic(t, err)
			want := elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="flag"`)
			if d.Class() != FailureUnsupported || d.Code() != UnsupportedInstanceValidationCode || d.Loc() != want ||
				!errors.Is(err, errInstanceAttributes) || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("grouped validation = %v", err)
			}
			generated, err := GenerateGo(schema, "generated")
			d = requireDiagnostic(t, err)
			if generated != nil || d.Class() != FailureUnsupported || d.Code() != diagnosticCodegenUnsupported || d.Loc() != want ||
				!errors.Is(err, errCodegenUnsupported) || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("grouped generation = %q/%v", generated, err)
			}
		})
	}
}
