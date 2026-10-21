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

//nolint:gocognit,funlen // The matrix compares public facts across owners and spellings.
func TestLocalInheritableFalseMatchesOmissionOnSupportedOwners(t *testing.T) {
	owners := []struct {
		name  string
		build func(string) string
	}{
		{"empty", localInheritableDirect},
		{"choice", func(attribute string) string {
			return localInheritableDirect(`<xs:choice><xs:element name="item" type="xs:integer"/></xs:choice>` + attribute)
		}},
		{"all", func(attribute string) string {
			return localInheritableDirect(`<xs:all><xs:element name="item" type="xs:integer"/></xs:all>` + attribute)
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
		{"sequence extension", func(attribute string) string {
			return `<xs:complexType name="Record"><xs:complexContent><xs:extension base="t:Base"><xs:sequence minOccurs="2" maxOccurs="18446744073709551616"><xs:element name="item" type="xs:integer" minOccurs="0" maxOccurs="3"/></xs:sequence>` + attribute + `</xs:extension></xs:complexContent></xs:complexType>`
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
					if owner.name == "sequence extension" {
						sequence, ok := definition.Particle().(SequenceParticle)
						if !ok || sequence.Occurrences().String() != "2/18446744073709551616" || sequence.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:sequence minOccurs=`) {
							t.Fatalf("sequence extension bounds/provenance = %#v", definition.Particle())
						}
						terms := sequence.Particles()
						if len(terms) != 1 || terms[0].Occurrences().String() != "0/3" || terms[0].Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:element name="item"`) {
							t.Fatalf("sequence terms = %#v", terms)
						}
						terms[0] = nil
						if len(sequence.Particles()) != 1 || sequence.Particles()[0] == nil {
							t.Fatal("mutating returned sequence terms changed completed facts")
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

//nolint:gocognit // Check the retained use order and all-particle facts together.
func TestLocalInheritableFalseDirectAllOrderedFacts(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, spelling := range []string{"", ` inheritable="false"`, ` inheritable="0"`} {
			t.Run(string(policy)+"/"+spelling, func(t *testing.T) {
				attributes := `<xs:attribute name="first" type="xs:boolean" use="required"` + spelling + `/>` +
					`<xs:attribute name="second" type="t:Named" inheritable=" &#x9;0&#xA; "/>` +
					`<xs:attribute name="excluded" type="xs:integer" use="prohibited" inheritable="false"/>`
				root := localInheritableSchema(localInheritableDirect(
					`<xs:all minOccurs="0"><xs:element name="item" type="xs:integer" minOccurs="2" maxOccurs="18446744073709551616"/></xs:all>`+attributes),
					`<xs:simpleType name="Named"><xs:restriction base="xs:decimal"/></xs:simpleType>`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err != nil {
					t.Fatalf("parse: %v", err)
				}
				definition := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Record"))[0], "Record")
				all, ok := definition.Particle().(AllParticle)
				if !ok || all.Occurrences().String() != "0/1" || all.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:all minOccurs=`) {
					t.Fatalf("all facts = %#v", definition.Particle())
				}
				members := all.Members()
				if len(members) != 1 || members[0].Occurrences().String() != "2/18446744073709551616" || members[0].Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:element name="item"`) {
					t.Fatalf("all members = %#v", members)
				}
				uses := definition.AttributeUses()
				if len(uses) != 2 {
					t.Fatalf("ordered uses = %#v", uses)
				}
				for index, name := range []string{"first", "second"} {
					use, useOK := uses[index].(LocalAttributeUse)
					if !useOK || use.Name() != mustTestQName(t, "urn:root", name) ||
						use.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="`+name+`"`) ||
						use.NameLoc() != elementReferenceTestAttributeLoc(t, root, `name="`+name+`"`) {
						t.Fatalf("use %d = %#v", index, uses[index])
					}
				}
				first, ok := uses[0].(LocalAttributeUse)
				if !ok {
					t.Fatalf("first use = %T", uses[0])
				}
				if first.Use() != AttributeUseRequired || first.UseLoc() != elementReferenceTestAttributeLoc(t, root, `use="required"`) ||
					first.DeclaredType() != mustTestQName(t, testXSDNamespace, "boolean") || first.TypeLoc() != elementReferenceTestAttributeLoc(t, root, `type="xs:boolean"`) {
					t.Fatalf("first facts = %#v", first)
				}
				second, ok := uses[1].(LocalAttributeUse)
				if !ok {
					t.Fatalf("second use = %T", uses[1])
				}
				if second.Use() != AttributeUseOptional || second.UseLoc() != (Loc{}) ||
					second.DeclaredType() != mustTestQName(t, "urn:root", "Named") || second.TypeLoc() != elementReferenceTestAttributeLoc(t, root, `type="t:Named"`) {
					t.Fatalf("second facts = %#v", second)
				}
				named := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:root", "Named"))
				if len(named) != 1 {
					t.Fatalf("Named components = %d", len(named))
				}
				if id, hasID := second.TypeID(); !hasID || id != named[0].ID() {
					t.Fatalf("named type identity = %v/%t", id, hasID)
				}
				uses[0] = nil
				members[0] = nil
				if len(definition.AttributeUses()) != 2 || definition.AttributeUses()[0] == nil || len(all.Members()) != 1 || all.Members()[0] == nil {
					t.Fatal("mutation changed completed use or all facts")
				}
			})
		}
	}
}

//nolint:gocognit // Keep every excluded owner and lexical exit in one matrix.
func TestLocalInheritableMalformedAndExcludedShapes(t *testing.T) {
	ownerCases := []struct {
		name, owner, extra string
	}{
		{"direct named", localInheritableDirect(`<xs:attribute name="flag" type="xs:boolean" inheritable="%s"/>`), ""},
		{"direct anonymous inline", `<xs:element name="root"><xs:complexType><xs:attribute name="flag" type="xs:boolean" inheritable="%s"/></xs:complexType></xs:element>`, ""},
		{"direct all anonymous inline", `<xs:element name="root"><xs:complexType><xs:all/><xs:attribute name="flag" type="xs:boolean" inheritable="%s"/></xs:complexType></xs:element>`, ""},
		{"direct all", localInheritableDirect(`<xs:all/><xs:attribute name="flag" type="xs:boolean" inheritable="%s"/>`), ""},
		{"direct all ref", localInheritableDirect(`<xs:all/><xs:attribute ref="t:global" inheritable="%s"/>`), `<xs:attribute name="global" type="xs:boolean"/>`},
		{"simpleContent extension", `<xs:complexType name="Record"><xs:simpleContent><xs:extension base="xs:string"><xs:attribute name="flag" type="xs:boolean" inheritable="%s"/></xs:extension></xs:simpleContent></xs:complexType>`, ""},
		{"sequence extension", `<xs:complexType name="Record"><xs:complexContent><xs:extension base="t:Base"><xs:sequence/><xs:attribute name="flag" type="xs:boolean" inheritable="%s"/></xs:extension></xs:complexContent></xs:complexType>`, `<xs:complexType name="Base"/>`},
		{"sequence extension anonymous inline", `<xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:Base"><xs:sequence/><xs:attribute name="flag" type="xs:boolean" inheritable="%s"/></xs:extension></xs:complexContent></xs:complexType></xs:element>`, `<xs:complexType name="Base"/>`},
		{"sequence extension ref", `<xs:complexType name="Record"><xs:complexContent><xs:extension base="t:Base"><xs:sequence/><xs:attribute ref="t:global" inheritable="%s"/></xs:extension></xs:complexContent></xs:complexType>`, `<xs:complexType name="Base"/><xs:attribute name="global" type="xs:boolean"/>`},
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
				if (owner.name == "direct named" || owner.name == "direct all" || owner.name == "grouped extension" || owner.name == "grouped anonymous inline" || owner.name == "sequence extension" || owner.name == "sequence extension anonymous inline") && (spelling == "false" || spelling == "0") && policy != Strict10 {
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
					if owner.name == "direct all anonymous inline" && policy != Strict10 && spelling != "maybe" {
						wantLoc = elementReferenceTestAttributeLoc(t, root, `<xs:all/>`)
					}
					if d.Loc() != wantLoc {
						t.Fatalf("primary Loc = %s, want %s: %v", d.Loc(), wantLoc, err)
					}
					if len(d.Related()) != 0 {
						t.Fatalf("related Locs = %v, want none", d.Related())
					}
					if owner.name == "direct all anonymous inline" && policy != Strict10 && spelling != "maybe" {
						if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode ||
							d.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:all/>`) || !errors.Is(err, ErrUnsupported) {
							t.Fatalf("anonymous all owner diagnostic = %v", err)
						}
						return
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
					if owner.name == "sequence extension" || owner.name == "sequence extension anonymous inline" || owner.name == "sequence extension ref" || owner.name == "grouped extension" || owner.name == "grouped anonymous inline" {
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

//nolint:gocognit // Compare both consumers across editions and spellings.
func TestLocalInheritableFalseDirectAllConsumersMatchOmission(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, spelling := range []string{"", ` inheritable="false"`, ` inheritable="0"`} {
			t.Run(string(policy)+"/"+spelling, func(t *testing.T) {
				root := localInheritableSchema(localInheritableDirect(
					`<xs:all><xs:element name="item" type="xs:integer"/></xs:all>`+localInheritableUse(spelling)),
					`<xs:element name="root" type="t:Record"/>`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err != nil {
					t.Fatalf("parse: %v", err)
				}
				instanceErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<t:root xmlns:t="urn:root"><t:item>1</t:item></t:root>`)))
				instanceDiagnostic := requireDiagnostic(t, instanceErr)
				if instanceDiagnostic.Class() != FailureUnsupported || instanceDiagnostic.Code() != UnsupportedInstanceValidationCode ||
					instanceDiagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="flag"`) ||
					instanceDiagnostic.SpecRef() != instanceValidationSpecRef(XSDVersion11) || !errors.Is(instanceErr, errInstanceAttributes) {
					t.Fatalf("direct all validation = %v", instanceErr)
				}
				output, generationErr := GenerateGo(schema, "generated")
				generationDiagnostic := requireDiagnostic(t, generationErr)
				if output != nil || generationDiagnostic.Class() != FailureUnsupported || generationDiagnostic.Code() != diagnosticCodegenUnsupported ||
					generationDiagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="flag"`) ||
					generationDiagnostic.SpecRef() != codegenDirectSequenceSpecReference(XSDVersion11, codegenDirectSequenceParticlesReference) ||
					!errors.Is(generationErr, errCodegenUnsupported) {
					t.Fatalf("direct all generation = %q/%v", output, generationErr)
				}
			})
		}
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
		for _, owner := range []struct {
			name, model string
		}{{"empty", ""}, {"all", `<xs:all><xs:element name="item" type="xs:integer"/></xs:all>`}} {
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
				{"duplicate inheritable", `<xs:attribute name="flag" type="xs:boolean" inheritable="false" inheritable="0"/>`, "", `inheritable="0"`, "", InvalidXMLSyntaxCode, nil, ""},
				{"invalid use", `<xs:attribute name="flag" type="xs:boolean" inheritable="false" use="sometimes"/>`, "", `use="sometimes"`, "", invalidSchemaCompositionCode, nil, schemaAttributeUseSpecRef(version)},
				{"ref type conflict", `<xs:attribute ref="t:global" type="xs:boolean" inheritable="false"/>`, "", `ref="t:global"`, "", invalidSchemaCompositionCode, nil, schemaAttributeUseSpecRef(version)},
			} {
				t.Run(string(policy)+"/"+owner.name+"/"+tc.name, func(t *testing.T) {
					root := localInheritableSchema(localInheritableDirect(owner.model+tc.attributes), tc.declarations)
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
					causeOK := tc.cause != nil && errors.Is(err, tc.cause)
					if tc.cause == nil {
						causeOK = errors.Unwrap(d) == nil
					}
					relatedOK := len(wantRelated) == 0 && len(d.Related()) == 0
					if len(wantRelated) > 0 {
						relatedOK = reflect.DeepEqual(d.Related(), wantRelated)
					}
					if d.Class() != FailureInvalid || d.Code() != tc.code || d.Loc() != elementReferenceTestAttributeLoc(t, root, tc.marker) ||
						!relatedOK || d.SpecRef() != tc.spec || !causeOK {
						t.Fatalf("boundary diagnostic = %v related=%v wantRelated=%v spec=%q wantSpec=%q loc=%s wantLoc=%s causeOK=%t", err, d.Related(), wantRelated, d.SpecRef(), tc.spec, d.Loc(), elementReferenceTestAttributeLoc(t, root, tc.marker), causeOK)
					}
				})
			}
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
