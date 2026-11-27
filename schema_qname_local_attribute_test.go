package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func qnameLocalOwner(shape, uses string) string {
	return namedLanguageAnyURIOwner(shape, uses)
}

//nolint:gocognit,funlen // Each public owner shape must retain the same graph and use provenance.
func TestQNameLocalAttributeGraphFacts(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, shape := range []string{"empty", "choice", "sequence"} {
			t.Run(profile.name+"/"+shape, func(t *testing.T) {
				root, fixtures := namedLanguageAnyURIGraph("QName", shape)
				first, err := parseIDUseSchema(t, root, fixtures, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				second, err := parseIDUseSchema(t, root, fixtures, profile.policy)
				if err != nil || !reflect.DeepEqual(first.Components(), second.Components()) {
					t.Fatalf("nondeterministic graph: %v", err)
				}
				if first.LanguagePolicy() != profile.policy {
					t.Fatalf("policy = %s", first.LanguagePolicy())
				}
				wantSources := []SourceID{"root.xsd", "included.xsd", "chameleon.xsd", "other.xsd"}
				for i, document := range first.Documents() {
					if i >= len(wantSources) || document.Source() != wantSources[i] {
						t.Fatalf("document order %d = %s", i, document.Source())
					}
				}
				if len(first.Documents()) != len(wantSources) {
					t.Fatalf("documents = %d", len(first.Documents()))
				}
				var walked []ComponentID
				if err := first.Walk(func(c Component) error { walked = append(walked, c.ID()); return nil }); err != nil {
					t.Fatal(err)
				}
				for i, component := range first.Components() {
					if i >= len(walked) || walked[i] != component.ID() {
						t.Fatalf("walk order %d = %v", i, walked)
					}
				}
				owner := requireTestComplexTypeDefinition(t, first.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Record"))[0], "Record")
				if owner.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Record"`) {
					t.Fatalf("owner Loc = %s", owner.Loc())
				}
				if shape == "empty" && owner.Particle() != nil {
					t.Fatalf("empty owner particle = %T", owner.Particle())
				}
				if shape != "empty" {
					if owner.Particle().Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:`+shape+` minOccurs=`) || owner.Particle().Occurrences().String() != "0/1" {
						t.Fatalf("particle bounds/Loc = %v", owner.Particle())
					}
					child := elementReferenceTestAttributeLoc(t, root, `<xs:element name="item"`)
					switch p := owner.Particle().(type) {
					case ChoiceParticle:
						if shape != "choice" || len(p.Alternatives()) != 1 || p.Alternatives()[0].Loc() != child || p.Alternatives()[0].Occurrences().String() != "0/2" {
							t.Fatalf("choice = %v", p.Alternatives())
						}
					case SequenceParticle:
						if shape != "sequence" || len(p.Elements()) != 1 || p.Elements()[0].Loc() != child || p.Elements()[0].Occurrences().String() != "0/2" {
							t.Fatalf("sequence = %v", p.Elements())
						}
					default:
						t.Fatalf("particle = %T", p)
					}
				}
				uses := owner.AttributeUses()
				if len(uses) != 6 {
					t.Fatalf("uses = %d", len(uses))
				}
				for i, want := range []struct{ local, lexical, namespace, typeName, source string }{
					{"direct", "r:Direct", "urn:root", "Direct", "root.xsd"},
					{"inherited", "r:Child", "urn:root", "Child", "root.xsd"},
					{"included", "r:Included", "urn:root", "Included", "included.xsd"},
					{"adopted", "r:Adopted", "urn:root", "Adopted", "chameleon.xsd"},
					{"imported", "o:Imported", "urn:other", "Imported", "other.xsd"},
				} {
					use, ok := uses[i].(LocalAttributeUse)
					if !ok {
						t.Fatalf("use %d = %T", i, uses[i])
					}
					useNS := "urn:root"
					if want.local == "included" {
						useNS = ""
					}
					typeName := mustTestQName(t, want.namespace, want.typeName)
					typeLoc := elementReferenceTestAttributeLoc(t, root, `type="`+want.lexical+`"`)
					if use.Name() != mustTestQName(t, useNS, want.local) || use.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="`+want.local+`"`) || use.NameLoc() != elementReferenceTestAttributeLoc(t, root, `name="`+want.local+`"`) || use.DeclaredType() != typeName || use.TypeLoc() != typeLoc {
						t.Fatalf("use %s facts = %#v", want.local, use)
					}
					ref, ok := use.TypeReference()
					id, hasID := use.TypeID()
					refID, hasRefID := ref.ComponentID()
					if !ok || !ref.IsNamed() || ref.Name() != typeName || ref.QName() != typeName || ref.Loc() != typeLoc || ref.Variety() != SimpleTypeVarietyAtomicRestriction || !hasID || !hasRefID || id != refID || id != componentIDForName(t, first, typeName) || id.Source() != SourceID(want.source) {
						t.Fatalf("use %s reference = %#v/%v", want.local, ref, id)
					}
					declaration := root
					if want.source != "root.xsd" {
						declaration = fixtures[want.source].contents
					}
					base := `<xs:restriction base="xs:QName"`
					if want.local == "inherited" {
						base = `<xs:restriction base="r:Direct"`
					}
					if ref.VarietyLoc() != allParticleTestTokenLoc(t, SourceID(want.source), declaration, base, 1) {
						t.Fatalf("use %s variety provenance = %s", want.local, ref.VarietyLoc())
					}
					_, hasWhiteSpace := ref.StringWhiteSpaceFacet()
					if len(ref.StringEnumerationFacets().Values()) != 0 || hasWhiteSpace {
						t.Fatalf("QName exposed string facets: %v", ref)
					}
				}
				builtin, ok := uses[5].(LocalAttributeUse)
				if !ok || builtin.Name().Local() != "after" {
					t.Fatalf("source use order = %v", uses)
				}
				if firstUse, ok := uses[0].(LocalAttributeUse); !ok || firstUse.Use() != AttributeUseRequired || firstUse.UseLoc() != elementReferenceTestAttributeLoc(t, root, `use="required"`) {
					t.Fatalf("use modifier = %v", uses[0])
				}
				if third, ok := uses[2].(LocalAttributeUse); !ok || third.FormLoc() != elementReferenceTestAttributeLoc(t, root, `form="unqualified"`) {
					t.Fatalf("form = %v", uses[2])
				}
				uses[0] = nil
				if owner.AttributeUses()[0] == nil {
					t.Fatal("mutating uses changed owner")
				}
				components := first.Components()
				components[0] = Component{}
				if !reflect.DeepEqual(first.Components(), second.Components()) {
					t.Fatal("mutating components changed schema")
				}
			})
		}
	}
}

//nolint:gocognit // Both public consumers have separate located exits for every admitted shape.
func TestQNameLocalAttributeConsumersReject(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, shape := range []string{"empty", "choice", "sequence"} {
			t.Run(profile.name+"/"+shape, func(t *testing.T) {
				root := languageUseRoot(qnameLocalOwner(shape, `<xs:attribute name="value" type="r:Alias"/>`) + `<xs:simpleType name="Alias"><xs:restriction base="xs:QName"/></xs:simpleType><xs:element name="root" type="r:Record"/>`)
				schema, err := parseIDUseSchema(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				useLoc := elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="value"`)
				ownerLoc := elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Record"`)
				rootLoc := elementReferenceTestAttributeLoc(t, root, `<xs:element name="root"`)
				err = ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<r:root xmlns:r="urn:root" value="r:item"/>`)))
				d := requireDiagnostic(t, err)
				related := []Loc{rootLoc, ownerLoc}
				if shape == "sequence" {
					related = append(related, elementReferenceTestAttributeLoc(t, root, `<xs:sequence`))
				}
				related = append(related, useLoc)
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedInstanceValidationCode || d.Loc() != useLoc || d.SpecRef() != instanceValidationSpecRef(profile.version) || !reflect.DeepEqual(d.Related(), related) || !errors.Is(err, errInstanceAttributes) {
					t.Fatalf("validation = %v, related %v", err, d.Related())
				}
				output, err := GenerateGo(schema, "generated")
				d = requireDiagnostic(t, err)
				wantSpec := codegenDirectSequenceSpecReference(profile.version, codegenDirectSequenceParticlesReference)
				if shape == "choice" {
					wantSpec = codegenDirectChoiceSpecReference(profile.version, codegenDirectChoiceParticlesReference)
				}
				if output != nil || d.Class() != FailureUnsupported || d.Code() != diagnosticCodegenUnsupported || d.Loc() != useLoc || d.SpecRef() != wantSpec || !reflect.DeepEqual(d.Related(), []Loc{ownerLoc, useLoc}) || !errors.Is(err, errCodegenUnsupported) {
					t.Fatalf("generation = %v/%v, related %v", output, err, d.Related())
				}
			})
		}
	}
}

//nolint:gocognit // Built-in and named facts have different identity contracts in each owner shape.
func TestQNameLocalAttributeBuiltinAndProhibited(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, shape := range []string{"empty", "choice", "sequence"} {
			t.Run(profile.name+"/"+shape, func(t *testing.T) {
				root := languageUseRoot(qnameLocalOwner(shape, `<xs:attribute name="first" type="xs:QName" use="required" form="unqualified"/><xs:attribute name="gone" type="r:Alias" use="prohibited"/><xs:attribute name="last" type="r:Alias"/>`) + `<xs:simpleType name="Alias"><xs:restriction base="xs:QName"/></xs:simpleType>`)
				schema, err := parseIDUseSchema(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				owner := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Record"))[0], "Record")
				uses := owner.AttributeUses()
				if len(uses) != 2 || uses[0].Name().Local() != "first" || uses[1].Name().Local() != "last" {
					t.Fatalf("prohibited/order = %v", uses)
				}
				first, ok := uses[0].(LocalAttributeUse)
				if !ok {
					t.Fatalf("first = %T", uses[0])
				}
				firstLoc := elementReferenceTestAttributeLoc(t, root, `type="xs:QName"`)
				ref, hasRef := first.TypeReference()
				if !hasRef || first.Name() != mustTestQName(t, "", "first") || first.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="first"`) || first.NameLoc() != elementReferenceTestAttributeLoc(t, root, `name="first"`) || first.TypeLoc() != firstLoc || first.FormLoc() != elementReferenceTestAttributeLoc(t, root, `form="unqualified"`) || first.Use() != AttributeUseRequired || first.UseLoc() != elementReferenceTestAttributeLoc(t, root, `use="required"`) || first.DeclaredType() != mustTestQName(t, testXSDNamespace, "QName") {
					t.Fatalf("builtin use = %#v", first)
				}
				assertQNameBuiltin(t, ref, firstLoc)
				if id, hasID := first.TypeID(); hasID || !id.IsZero() {
					t.Fatalf("builtin TypeID = %v/%t", id, hasID)
				}
				last, ok := uses[1].(LocalAttributeUse)
				if !ok {
					t.Fatalf("last = %T", uses[1])
				}
				lastRef, hasRef := last.TypeReference()
				if !hasRef || !lastRef.IsNamed() || last.DeclaredType() != mustTestQName(t, "urn:root", "Alias") || last.TypeLoc() != allParticleTestTokenLoc(t, "root.xsd", root, `type="r:Alias"`, 2) || lastRef.VarietyLoc() != elementReferenceTestAttributeLoc(t, root, `<xs:restriction base="xs:QName"`) {
					t.Fatalf("named use = %#v/%#v", last, lastRef)
				}
			})
		}
	}
}

//nolint:gocognit // Every excluded attribute form is exercised at each affected owner boundary.
func TestQNameLocalAttributeExcludedForms(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, shape := range []string{"empty", "choice", "sequence"} {
			for _, test := range []struct {
				name, use, declaration, marker, spec string
				cause                                error
			}{
				{"inline", `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:QName"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnsupported},
				{"inline list", `<xs:attribute name="a"><xs:simpleType><xs:list itemType="xs:QName"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnsupported},
				{"inline union", `<xs:attribute name="a"><xs:simpleType><xs:union memberTypes="xs:QName"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnsupported},
				{"named list", `<xs:attribute name="a" type="r:List"/>`, `<xs:simpleType name="List"><xs:list itemType="xs:QName"/></xs:simpleType>`, `type="r:List"`, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnsupported},
				{"named union", `<xs:attribute name="a" type="r:Union"/>`, `<xs:simpleType name="Union"><xs:union memberTypes="xs:QName"/></xs:simpleType>`, `type="r:Union"`, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnsupported},
				{"builtin default", `<xs:attribute name="a" type="xs:QName" default="r:v"/>`, "", `default="r:v"`, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseUnsupported},
				{"builtin fixed", `<xs:attribute name="a" type="xs:QName" fixed="r:v"/>`, "", `fixed="r:v"`, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseUnsupported},
				{"named default", `<xs:attribute name="a" type="r:Alias" default="r:v"/>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:QName"/></xs:simpleType>`, `default="r:v"`, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseUnsupported},
				{"named fixed", `<xs:attribute name="a" type="r:Alias" fixed="r:v"/>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:QName"/></xs:simpleType>`, `fixed="r:v"`, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseUnsupported},
				{"ref builtin", `<xs:attribute ref="r:a"/>`, `<xs:attribute name="a" type="xs:QName"/>`, `ref="r:a"`, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeReferenceUnsupported},
				{"ref named", `<xs:attribute ref="r:a"/>`, `<xs:attribute name="a" type="r:Alias"/><xs:simpleType name="Alias"><xs:restriction base="xs:QName"/></xs:simpleType>`, `ref="r:a"`, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeReferenceUnsupported},
			} {
				t.Run(profile.name+"/"+shape+"/"+test.name, func(t *testing.T) {
					root := languageUseRoot(qnameLocalOwner(shape, test.use) + test.declaration)
					schema, err := parseIDUseSchema(t, root, nil, profile.policy)
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					var related []Loc
					if strings.HasPrefix(test.name, "ref") {
						related = []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="a"`)}
					}
					if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, test.marker) || d.SpecRef() != test.spec || !reflect.DeepEqual(d.Related(), related) || !errors.Is(err, test.cause) {
						t.Fatalf("excluded %s = %v, related %v", test.name, err, d.Related())
					}
				})
			}
		}
	}
}

//nolint:gocognit // Each earlier resolution/syntax exit is observed before the supported type gate.
func TestQNameLocalAttributeAlternateExits(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, shape := range []string{"empty", "choice", "sequence"} {
			for _, test := range []struct {
				name, use, declaration, primary, related, code, spec string
				class                                                FailureClass
				cause                                                error
			}{
				{"missing", `<xs:attribute name="a" type="r:Missing"/>`, "", `type="r:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(profile.version), FailureInvalid, errSchemaAttributeTypeUnresolved},
				{"wrong kind", `<xs:attribute name="a" type="r:Target"/>`, `<xs:element name="Target" type="xs:QName"/>`, `type="r:Target"`, `<xs:element name="Target"`, diagnosticSchemaAttributeTypeWrongKindCode, schemaAttributeTypeSpecRef(profile.version), FailureInvalid, errSchemaAttributeTypeWrongKind},
				{"cycle", `<xs:attribute name="a" type="r:A"/>`, `<xs:simpleType name="A"><xs:restriction base="r:B"/></xs:simpleType><xs:simpleType name="B"><xs:restriction base="r:A"/></xs:simpleType>`, `type="r:A"`, `<xs:attribute name="a"`, diagnosticSchemaAttributeTypeCycleCode, schemaAttributeTypeSpecRef(profile.version), FailureInvalid, errSchemaSimpleTypeBaseCycle},
				{"malformed", `<xs:attribute name="a" type="r:bad:QName"/>`, "", `type="r:bad:QName"`, "", invalidSchemaConditionalCode, schemaAttributeUseSpecRef(profile.version), FailureInvalid, nil},
				{"unbound", `<xs:attribute name="a" type="missing:QName"/>`, "", `type="missing:QName"`, "", invalidSchemaConditionalCode, schemaAttributeUseSpecRef(profile.version), FailureInvalid, nil},
				{"prohibited missing", `<xs:attribute name="a" type="r:Missing" use="prohibited"/>`, "", `type="r:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(profile.version), FailureInvalid, errSchemaAttributeTypeUnresolved},
				{"prohibited wrong kind", `<xs:attribute name="a" type="r:Target" use="prohibited"/>`, `<xs:element name="Target" type="xs:QName"/>`, `type="r:Target"`, `<xs:element name="Target"`, diagnosticSchemaAttributeTypeWrongKindCode, schemaAttributeTypeSpecRef(profile.version), FailureInvalid, errSchemaAttributeTypeWrongKind},
				{"prohibited cycle", `<xs:attribute name="a" type="r:A" use="prohibited"/>`, `<xs:simpleType name="A"><xs:restriction base="r:B"/></xs:simpleType><xs:simpleType name="B"><xs:restriction base="r:A"/></xs:simpleType>`, `type="r:A"`, `<xs:attribute name="a"`, diagnosticSchemaAttributeTypeCycleCode, schemaAttributeTypeSpecRef(profile.version), FailureInvalid, errSchemaSimpleTypeBaseCycle},
			} {
				t.Run(profile.name+"/"+shape+"/"+test.name, func(t *testing.T) {
					root := languageUseRoot(qnameLocalOwner(shape, test.use) + test.declaration)
					schema, err := parseIDUseSchema(t, root, nil, profile.policy)
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					if d.Class() != test.class || d.Code() != test.code || d.Loc() != elementReferenceTestAttributeLoc(t, root, test.primary) || d.SpecRef() != test.spec {
						t.Fatalf("alternate exit = %v class %s code %s loc %s spec %s", d, d.Class(), d.Code(), d.Loc(), d.SpecRef())
					}
					var related []Loc
					if test.related != "" {
						related = []Loc{elementReferenceTestAttributeLoc(t, root, test.related)}
					}
					if strings.Contains(test.name, "cycle") {
						related = []Loc{
							elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="a"`),
							elementReferenceTestAttributeLoc(t, root, `base="r:B"`),
							elementReferenceTestAttributeLoc(t, root, `base="r:A"`),
							elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="A"`),
							elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="B"`),
						}
					}
					if !reflect.DeepEqual(d.Related(), related) {
						t.Fatalf("related = %v, want %v", d.Related(), related)
					}
					if test.cause != nil && !errors.Is(err, test.cause) {
						t.Fatalf("lost cause %v: %v", test.cause, err)
					}
				})
			}
		}
	}
}

func TestQNameLocalAttributeExcludedOwners(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, typeName := range []string{"xs:QName", "r:Alias"} {
			for _, test := range []struct{ name, body string }{
				{"all", `<xs:complexType name="Record"><xs:all><xs:element name="item" type="xs:integer"/></xs:all><xs:attribute name="a" type="%s"/></xs:complexType>`},
				{"direct group", `<xs:group name="G"><xs:sequence><xs:element name="item" type="xs:integer"/></xs:sequence></xs:group><xs:complexType name="Record"><xs:group ref="r:G"/><xs:attribute name="a" type="%s"/></xs:complexType>`},
				{"sequence extension", `<xs:complexType name="Base"/><xs:complexType name="Record"><xs:complexContent><xs:extension base="r:Base"><xs:sequence><xs:element name="item" type="xs:integer"/></xs:sequence><xs:attribute name="a" type="%s"/></xs:extension></xs:complexContent></xs:complexType>`},
				{"grouped extension", `<xs:group name="G"><xs:sequence><xs:element name="item" type="xs:integer"/></xs:sequence></xs:group><xs:complexType name="Base"/><xs:complexType name="Record"><xs:complexContent><xs:extension base="r:Base"><xs:group ref="r:G"/><xs:attribute name="a" type="%s"/></xs:extension></xs:complexContent></xs:complexType>`},
				{"anonymous empty", `<xs:element name="root"><xs:complexType><xs:attribute name="a" type="%s"/></xs:complexType></xs:element>`},
				{"simple content", `<xs:complexType name="Record"><xs:simpleContent><xs:extension base="xs:string"><xs:attribute name="a" type="%s"/></xs:extension></xs:simpleContent></xs:complexType>`},
			} {
				t.Run(profile.name+"/"+typeName+"/"+test.name, func(t *testing.T) {
					root := languageUseRoot(strings.Replace(test.body, "%s", typeName, 1) + `<xs:simpleType name="Alias"><xs:restriction base="xs:QName"/></xs:simpleType>`)
					schema, err := parseIDUseSchema(t, root, nil, profile.policy)
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `type="`+typeName+`"`) || d.SpecRef() != schemaAttributeTypeSpecRef(profile.version) || len(d.Related()) != 0 || !errors.Is(err, errSchemaAttributeUseUnsupported) {
						t.Fatalf("excluded owner = %v", err)
					}
				})
			}
		}
	}
}

//nolint:gocognit // A facet on the completed type must fail before local use publication.
func TestQNameLocalAttributeFacetsAndVisibility(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, shape := range []string{"empty", "choice", "sequence"} {
			for _, test := range []struct{ name, declaration, marker string }{
				{"direct pattern", `<xs:simpleType name="Alias"><xs:restriction base="xs:QName"><xs:pattern value=".*"/></xs:restriction></xs:simpleType>`, `<xs:pattern`},
				{"inherited enumeration", `<xs:simpleType name="Parent"><xs:restriction base="xs:QName"><xs:enumeration value="r:v"/></xs:restriction></xs:simpleType><xs:simpleType name="Alias"><xs:restriction base="r:Parent"/></xs:simpleType>`, `<xs:enumeration`},
			} {
				t.Run(profile.name+"/"+shape+"/"+test.name, func(t *testing.T) {
					root := languageUseRoot(qnameLocalOwner(shape, `<xs:attribute name="a" type="r:Alias"/>`) + test.declaration)
					schema, err := parseIDUseSchema(t, root, nil, profile.policy)
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					if d.Class() != FailureUnsupported || d.Code() != UnsupportedDatatypeFacetCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, test.marker) || d.SpecRef() != qnameFacetSpecRef(profile.version) || len(d.Related()) != 0 || !errors.Is(err, ErrUnsupported) {
						t.Fatalf("faceted type = %v, related %v", err, d.Related())
					}
				})
			}
			t.Run(profile.name+"/"+shape+"/invisible", func(t *testing.T) {
				root := languageUseRoot(`<xs:include schemaLocation="bridge.xsd"/>` + qnameLocalOwner(shape, `<xs:attribute name="a" type="o:Hidden"/>`))
				root = strings.Replace(root, `xmlns:r="urn:root"`, `xmlns:r="urn:root" xmlns:o="urn:other"`, 1)
				fixtures := map[string]discoveryFixture{
					"bridge.xsd": {id: "bridge.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`},
					"other.xsd":  {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Hidden"><xs:restriction base="xs:QName"/></xs:simpleType></xs:schema>`},
				}
				schema, err := parseIDUseSchema(t, root, fixtures, profile.policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaAttributeTypeUnresolvedCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `type="o:Hidden"`) || d.SpecRef() != schemaAttributeTypeSpecRef(profile.version) || len(d.Related()) != 0 || !errors.Is(err, errSchemaAttributeTypeUnresolved) {
					t.Fatalf("invisible target = %v, related %v", err, d.Related())
				}
			})
		}
	}
}

//nolint:gocognit // Duplicate global identities precede local QName use mapping in every owner shape.
func TestQNameLocalAttributeDuplicateAndEditionLabel(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, shape := range []string{"empty", "choice", "sequence"} {
			for _, label := range []string{"1.0", "1.1"} {
				t.Run(profile.name+"/"+shape+"/"+label, func(t *testing.T) {
					root := languageUseRoot(qnameLocalOwner(shape, `<xs:attribute name="a" type="r:Alias"/>`) + `<xs:simpleType name="Alias"><xs:restriction base="xs:QName"/></xs:simpleType>`)
					root = strings.Replace(root, `attributeFormDefault="qualified"`, `attributeFormDefault="qualified" version="`+label+`"`, 1)
					schema, err := parseIDUseSchema(t, root, nil, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					owner := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Record"))[0], "Record")
					uses := owner.AttributeUses()
					if len(uses) != 1 {
						t.Fatalf("uses = %v", uses)
					}
					use, ok := uses[0].(LocalAttributeUse)
					if !ok {
						t.Fatalf("use = %T", uses[0])
					}
					ref, ok := use.TypeReference()
					if !ok || !ref.IsNamed() || use.TypeLoc() != elementReferenceTestAttributeLoc(t, root, `type="r:Alias"`) || ref.VarietyLoc() != elementReferenceTestAttributeLoc(t, root, `<xs:restriction base="xs:QName"`) {
						t.Fatalf("label changed type facts: %#v/%#v", use, ref)
					}
				})
			}
			t.Run(profile.name+"/"+shape+"/duplicate", func(t *testing.T) {
				root := languageUseRoot(qnameLocalOwner(shape, `<xs:attribute name="a" type="r:Alias"/>`) + `<xs:simpleType name="Alias"><xs:restriction base="xs:QName"/></xs:simpleType><xs:simpleType name="Alias" id="second"><xs:restriction base="xs:QName"/></xs:simpleType>`)
				schema, err := parseIDUseSchema(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				first := elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="Alias"><xs:restriction`)
				second := elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="Alias" id="second"`)
				if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaGlobalDuplicateCode || d.Loc() != second || !reflect.DeepEqual(d.Related(), []Loc{first}) || d.SpecRef() != schemaGlobalDuplicateSpecRef(profile.version) || !errors.Is(err, errSchemaGlobalDeclarationDuplicate) {
					t.Fatalf("duplicate = %v, related %v", err, d.Related())
				}
			})
		}
	}
}
