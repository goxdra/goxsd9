package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func namedLanguageAnyURIOwner(shape, uses string) string {
	particle := ""
	if shape != "empty" {
		childMaximum := "2"
		if shape == "all" {
			childMaximum = "1"
		}
		particle = `<xs:` + shape + ` minOccurs="0" maxOccurs="1"><xs:element name="item" type="xs:integer" minOccurs="0" maxOccurs="` + childMaximum + `"/></xs:` + shape + `>`
	}
	return `<xs:complexType name="Record">` + particle + uses + `</xs:complexType>`
}

func namedLanguageAnyURIGraph(kind, shape string) (string, map[string]discoveryFixture) {
	uses := `<xs:attribute name="direct" type="r:Direct" use="required"/>` +
		`<xs:attribute name="inherited" type="r:Child"/>` +
		`<xs:attribute name="included" type="r:Included" form="unqualified"/>` +
		`<xs:attribute name="adopted" type="r:Adopted"/>` +
		`<xs:attribute name="imported" type="o:Imported"/>` +
		`<xs:attribute name="gone" type="r:Child" use="prohibited"/>` +
		`<xs:attribute name="after" type="xs:boolean"/>`
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" attributeFormDefault="qualified">` +
		`<xs:include schemaLocation="included.xsd"/><xs:include schemaLocation="chameleon.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/>` +
		namedLanguageAnyURIOwner(shape, uses) +
		`<xs:simpleType name="Direct"><xs:restriction base="xs:` + kind + `"/></xs:simpleType>` +
		`<xs:simpleType name="Child"><xs:restriction base="r:Direct"/></xs:simpleType>` +
		`<xs:element name="root" type="r:Record"/></xs:schema>`
	included := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:simpleType name="Included"><xs:restriction base="xs:` + kind + `"/></xs:simpleType></xs:schema>`
	chameleon := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Adopted"><xs:restriction base="xs:` + kind + `"/></xs:simpleType></xs:schema>`
	imported := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Imported"><xs:restriction base="xs:` + kind + `"/></xs:simpleType></xs:schema>`
	return root, map[string]discoveryFixture{
		"included.xsd":  {id: "included.xsd", contents: included},
		"chameleon.xsd": {id: "chameleon.xsd", contents: chameleon},
		"other.xsd":     {id: "other.xsd", contents: imported},
	}
}

//nolint:gocognit,funlen // Each owner and datatype exposes the same ordered public graph facts.
func TestNamedLanguageAnyURIAttributeGraphFacts(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, kind := range []string{"language", "anyURI"} {
			for _, shape := range []string{"empty", "choice", "sequence", "all"} {
				t.Run(string(policy)+"/"+kind+"/"+shape, func(t *testing.T) {
					root, fixtures := namedLanguageAnyURIGraph(kind, shape)
					schema, err := parseIDUseSchema(t, root, fixtures, policy)
					if err != nil {
						t.Fatal(err)
					}
					again, err := parseIDUseSchema(t, root, fixtures, policy)
					if err != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
						t.Fatalf("unstable graph: %v", err)
					}
					if schema.LanguagePolicy() != policy {
						t.Fatalf("policy = %s", schema.LanguagePolicy())
					}
					wantSources := []SourceID{"root.xsd", "included.xsd", "chameleon.xsd", "other.xsd"}
					documents := schema.Documents()
					if len(documents) != len(wantSources) {
						t.Fatalf("documents = %v", documents)
					}
					for i, document := range documents {
						if document.Source() != wantSources[i] {
							t.Fatalf("document %d = %s", i, document.Source())
						}
					}
					var walked []string
					if err := schema.Walk(func(c Component) error {
						walked = append(walked, string(c.ID().Source())+":"+c.Name().Local())
						return nil
					}); err != nil {
						t.Fatal(err)
					}
					if want := []string{"root.xsd:Record", "root.xsd:Direct", "root.xsd:Child", "root.xsd:root", "included.xsd:Included", "chameleon.xsd:Adopted", "other.xsd:Imported"}; !reflect.DeepEqual(walked, want) {
						t.Fatalf("walk = %v, want %v", walked, want)
					}
					found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Record"))
					if len(found) != 1 {
						t.Fatalf("owner = %v", found)
					}
					owner := requireTestComplexTypeDefinition(t, found[0], "Record")
					ownerLoc := elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Record"`)
					if owner.Loc() != ownerLoc {
						t.Fatalf("owner Loc = %s", owner.Loc())
					}
					if shape == "empty" && owner.Particle() != nil {
						t.Fatalf("empty particle = %T", owner.Particle())
					}
					if shape != "empty" {
						wantLoc := elementReferenceTestAttributeLoc(t, root, `<xs:`+shape+` minOccurs=`)
						if owner.Particle().Loc() != wantLoc || owner.Particle().Occurrences().String() != "0/1" {
							t.Fatalf("particle = %T/%s", owner.Particle(), owner.Particle().Occurrences())
						}
						childLoc := elementReferenceTestAttributeLoc(t, root, `<xs:element name="item"`)
						switch particle := owner.Particle().(type) {
						case ChoiceParticle:
							if shape != "choice" || len(particle.Alternatives()) != 1 || particle.Alternatives()[0].Loc() != childLoc || particle.Alternatives()[0].Occurrences().String() != "0/2" {
								t.Fatalf("choice child = %#v", particle.Alternatives())
							}
						case SequenceParticle:
							if shape != "sequence" || len(particle.Elements()) != 1 || particle.Elements()[0].Loc() != childLoc || particle.Elements()[0].Occurrences().String() != "0/2" {
								t.Fatalf("sequence child = %#v", particle.Elements())
							}
						case AllParticle:
							if shape != "all" || len(particle.Members()) != 1 || particle.Members()[0].Loc() != childLoc || particle.Members()[0].Occurrences().String() != "0/1" {
								t.Fatalf("all child = %#v", particle.Members())
							}
						default:
							t.Fatalf("unexpected particle %T", owner.Particle())
						}
					}
					uses := owner.AttributeUses()
					if len(uses) != 6 {
						t.Fatalf("uses = %v", uses)
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
						namespace := "urn:root"
						if want.local == "included" {
							namespace = ""
						}
						typeLoc := elementReferenceTestAttributeLoc(t, root, `type="`+want.lexical+`"`)
						name := mustTestQName(t, want.namespace, want.typeName)
						if use.Name() != mustTestQName(t, namespace, want.local) || use.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="`+want.local+`"`) || use.NameLoc() != elementReferenceTestAttributeLoc(t, root, `name="`+want.local+`"`) || use.TypeLoc() != typeLoc || use.DeclaredType() != name {
							t.Fatalf("%s use provenance = %#v", want.local, use)
						}
						ref, hasRef := use.TypeReference()
						id, hasID := use.TypeID()
						refID, hasRefID := ref.ComponentID()
						if !hasRef || !ref.IsNamed() || ref.IsBuiltin() || ref.Name() != name || ref.QName() != name || ref.Loc() != typeLoc || ref.Variety() != SimpleTypeVarietyAtomicRestriction || !hasID || !hasRefID || id != refID || id != componentIDForName(t, schema, name) || id.Source() != SourceID(want.source) {
							t.Fatalf("%s type facts = %#v/%v/%t", want.local, ref, id, hasID)
						}
						declaration := root
						if want.source != "root.xsd" {
							declaration = fixtures[want.source].contents
						}
						base := `<xs:restriction base="xs:` + kind + `"`
						if want.local == "inherited" {
							base = `<xs:restriction base="r:Direct"`
						}
						if ref.VarietyLoc() != allParticleTestTokenLoc(t, SourceID(want.source), declaration, base, 1) {
							t.Fatalf("%s variety Loc = %s", want.local, ref.VarietyLoc())
						}
						if i == 0 && (use.Use() != AttributeUseRequired || use.UseLoc() != elementReferenceTestAttributeLoc(t, root, `use="required"`)) {
							t.Fatalf("required use = %#v", use)
						}
						if i == 2 && use.FormLoc() != elementReferenceTestAttributeLoc(t, root, `form="unqualified"`) {
							t.Fatalf("form Loc = %s", use.FormLoc())
						}
						if len(ref.StringEnumerationFacets().Values()) != 0 {
							t.Fatalf("unexpected effective enumeration facets = %v", ref.StringEnumerationFacets().Values())
						}
						if _, hasSpace := ref.StringWhiteSpaceFacet(); hasSpace {
							t.Fatalf("unexpected string whiteSpace view for %s", want.local)
						}
						definition := schemaEnumerationTestDefinitionInNamespace(t, schema, want.namespace, want.typeName)
						baseName := mustTestQName(t, testXSDNamespace, kind)
						if want.local == "inherited" {
							baseName = mustTestQName(t, "urn:root", "Direct")
						}
						baseRef, hasBase := definition.BaseReference()
						if definition.ID() != id || definition.Loc() != allParticleTestTokenLoc(t, SourceID(want.source), declaration, `<xs:simpleType name="`+want.typeName+`"`, 1) || definition.Variety() != SimpleTypeVarietyAtomicRestriction || definition.VarietyLoc() != ref.VarietyLoc() || definition.Base() != baseName || definition.BaseLoc() != allParticleTestTokenLoc(t, SourceID(want.source), declaration, `base="`+map[bool]string{true: "r:Direct", false: "xs:" + kind}[want.local == "inherited"]+`"`, 1) || !hasBase || baseRef.Name() != baseName {
							t.Fatalf("%s completed definition = %#v/%#v", want.local, definition, baseRef)
						}
					}
					if uses[5].Name().Local() != "after" {
						t.Fatalf("use order = %v", uses)
					}
					uses[0] = nil
					if owner.AttributeUses()[0] == nil {
						t.Fatal("mutated AttributeUses view")
					}
					components := schema.Components()
					components[0] = Component{}
					if schema.Components()[0].Name().Local() != "Record" {
						t.Fatal("mutated Components view")
					}
				})
			}
		}
	}
}

//nolint:gocognit // Each newly admitted owner and datatype reaches both public consumer guards.
func TestNamedLanguageAnyURIAttributeConsumersReject(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, kind := range []string{"language", "anyURI"} {
			for _, shape := range []string{"empty", "choice", "sequence", "all"} {
				t.Run(string(policy)+"/"+kind+"/"+shape, func(t *testing.T) {
					root := languageUseRoot(namedLanguageAnyURIOwner(shape, `<xs:attribute name="value" type="r:Alias"/>`) + `<xs:simpleType name="Alias"><xs:restriction base="xs:` + kind + `"/></xs:simpleType><xs:element name="root" type="r:Record"/>`)
					schema, err := parseIDUseSchema(t, root, nil, policy)
					if err != nil {
						t.Fatal(err)
					}
					useLoc := elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="value"`)
					ownerLoc := elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Record"`)
					elementLoc := elementReferenceTestAttributeLoc(t, root, `<xs:element name="root"`)
					instance := `<r:root xmlns:r="urn:root" value="en"/>`
					err = ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(instance)))
					d := requireDiagnostic(t, err)
					related := []Loc{elementLoc, ownerLoc}
					if shape == "sequence" {
						related = append(related, elementReferenceTestAttributeLoc(t, root, `<xs:sequence`))
					}
					related = append(related, useLoc)
					if d.Class() != FailureUnsupported || d.Code() != UnsupportedInstanceValidationCode || d.Loc() != useLoc || d.SpecRef() != instanceValidationSpecRef(languageUseVersion(policy)) || !reflect.DeepEqual(d.Related(), related) || !errors.Is(err, ErrUnsupported) || !errors.Is(err, errInstanceAttributes) {
						t.Fatalf("validation = %v, related %v", err, d.Related())
					}
					output, err := GenerateGo(schema, "generated")
					d = requireDiagnostic(t, err)
					wantSpec := codegenDirectSequenceSpecReference(languageUseVersion(policy), codegenDirectSequenceParticlesReference)
					if shape == "choice" {
						wantSpec = codegenDirectChoiceSpecReference(languageUseVersion(policy), codegenDirectChoiceParticlesReference)
					}
					if shape == "empty" {
						wantSpec = codegenDirectSequenceSpecReference(languageUseVersion(policy), codegenDirectSequenceParticlesReference)
					}
					if output != nil || d.Class() != FailureUnsupported || d.Code() != diagnosticCodegenUnsupported || d.Loc() != useLoc || d.SpecRef() != wantSpec || !reflect.DeepEqual(d.Related(), []Loc{ownerLoc, useLoc}) || !errors.Is(err, ErrUnsupported) || !errors.Is(err, errCodegenUnsupported) {
						t.Fatalf("generation = %v/%v, related %v", output, err, d.Related())
					}
				})
			}
		}
	}
}

func TestNamedLanguageAnyURIAttributeExcludedOwners(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, kind := range []string{"language", "anyURI"} {
			for _, test := range []struct{ name, body string }{
				{"grouped", `<xs:complexType name="Record"><xs:group ref="r:Fields"/><xs:attribute name="value" type="r:Alias"/></xs:complexType><xs:group name="Fields"><xs:sequence><xs:element ref="r:item"/></xs:sequence></xs:group><xs:element name="item" type="xs:integer"/>`},
				{"anonymous", `<xs:element name="root"><xs:complexType><xs:attribute name="value" type="r:Alias"/></xs:complexType></xs:element>`},
				{"simpleContent", `<xs:complexType name="Record"><xs:simpleContent><xs:extension base="xs:string"><xs:attribute name="value" type="r:Alias"/></xs:extension></xs:simpleContent></xs:complexType>`},
			} {
				t.Run(string(policy)+"/"+kind+"/"+test.name, func(t *testing.T) {
					root := languageUseRoot(test.body + `<xs:simpleType name="Alias"><xs:restriction base="xs:` + kind + `"/></xs:simpleType>`)
					schema, err := parseIDUseSchema(t, root, nil, policy)
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `type="r:Alias"`) || d.SpecRef() != schemaAttributeTypeSpecRef(languageUseVersion(policy)) || len(d.Related()) != 0 || !errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaAttributeUseUnsupported) {
						t.Fatalf("excluded owner = %v, related %v", err, d.Related())
					}
				})
			}
		}
	}
}

//nolint:gocognit // Type resolution must precede prohibited-use omission at each named boundary.
func TestNamedLanguageAnyURIAttributeProhibitedResolution(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, kind := range []string{"language", "anyURI"} {
			for _, shape := range []string{"empty", "choice", "sequence", "all"} {
				t.Run(string(policy)+"/"+kind+"/"+shape, func(t *testing.T) {
					root := languageUseRoot(namedLanguageAnyURIOwner(shape, `<xs:attribute name="gone" type="r:Alias" use="prohibited"/><xs:attribute name="keep" type="xs:boolean"/>`) +
						`<xs:simpleType name="Alias"><xs:restriction base="xs:` + kind + `"/></xs:simpleType>`)
					schema, err := parseIDUseSchema(t, root, nil, policy)
					if err != nil {
						t.Fatal(err)
					}
					owner := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Record"))[0], "Record")
					uses := owner.AttributeUses()
					if len(uses) != 1 || uses[0].Name().Local() != "keep" {
						t.Fatalf("prohibited use not omitted: %v", uses)
					}
					for _, test := range []struct {
						name, declaration, marker, related, code string
						cause                                    error
					}{
						{"unresolved", "", `type="r:Alias"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, errSchemaAttributeTypeUnresolved},
						{"wrong kind", `<xs:element name="Alias" type="xs:integer"/>`, `type="r:Alias"`, `<xs:element name="Alias"`, diagnosticSchemaAttributeTypeWrongKindCode, errSchemaAttributeTypeWrongKind},
						{"cycle", `<xs:simpleType name="Alias"><xs:restriction base="r:Other"/></xs:simpleType><xs:simpleType name="Other"><xs:restriction base="r:Alias"/></xs:simpleType>`, `type="r:Alias"`, "", diagnosticSchemaAttributeTypeCycleCode, errSchemaSimpleTypeBaseCycle},
					} {
						t.Run(test.name, func(t *testing.T) {
							broken := languageUseRoot(namedLanguageAnyURIOwner(shape, `<xs:attribute name="gone" type="r:Alias" use="prohibited"/>`) + test.declaration)
							badSchema, badErr := parseIDUseSchema(t, broken, nil, policy)
							assertZeroSchema(t, badSchema)
							d := requireDiagnostic(t, badErr)
							var related []Loc
							if test.related != "" {
								related = []Loc{elementReferenceTestAttributeLoc(t, broken, test.related)}
							}
							if test.name == "cycle" {
								related = []Loc{
									elementReferenceTestAttributeLoc(t, broken, `<xs:attribute name="gone"`),
									elementReferenceTestAttributeLoc(t, broken, `base="r:Other"`),
									elementReferenceTestAttributeLoc(t, broken, `base="r:Alias"`),
									elementReferenceTestAttributeLoc(t, broken, `<xs:simpleType name="Alias"`),
									elementReferenceTestAttributeLoc(t, broken, `<xs:simpleType name="Other"`),
								}
							}
							if d.Class() != FailureInvalid || d.Code() != test.code || d.Loc() != elementReferenceTestAttributeLoc(t, broken, test.marker) || d.SpecRef() != schemaAttributeTypeSpecRef(languageUseVersion(policy)) || !reflect.DeepEqual(d.Related(), related) || !errors.Is(badErr, test.cause) {
								t.Fatalf("prohibited resolution = %v, related %v", badErr, d.Related())
							}
						})
					}
				})
			}
		}
	}
}

//nolint:gocognit // Facets and broader varieties fail before owner publication.
func TestNamedLanguageAnyURIAttributeExcludedTypeFacts(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, kind := range []string{"language", "anyURI"} {
			for _, shape := range []string{"empty", "choice", "sequence", "all"} {
				for _, test := range []struct {
					name, declaration, marker, code, spec string
					cause                                 error
				}{
					{"direct pattern", `<xs:simpleType name="Alias"><xs:restriction base="xs:` + kind + `"><xs:pattern value=".*"/></xs:restriction></xs:simpleType>`, `<xs:pattern`, UnsupportedDatatypeFacetCode, tokenDiagnosticSpecRef(languageUseVersion(policy), "decimal"), ErrUnsupported},
					{"inherited enumeration", `<xs:simpleType name="Parent"><xs:restriction base="xs:` + kind + `"><xs:enumeration value="en"/></xs:restriction></xs:simpleType><xs:simpleType name="Alias"><xs:restriction base="r:Parent"/></xs:simpleType>`, `<xs:enumeration`, UnsupportedDatatypeFacetCode, tokenDiagnosticSpecRef(languageUseVersion(policy), "decimal"), ErrUnsupported},
					{"list", `<xs:simpleType name="Alias"><xs:list itemType="xs:` + kind + `"/></xs:simpleType>`, `type="r:Alias"`, UnsupportedSchemaSyntaxCode, schemaAttributeTypeSpecRef(languageUseVersion(policy)), errSchemaAttributeTypeUnsupported},
					{"union", `<xs:simpleType name="Alias"><xs:union memberTypes="xs:` + kind + `"/></xs:simpleType>`, `type="r:Alias"`, UnsupportedSchemaSyntaxCode, schemaAttributeTypeSpecRef(languageUseVersion(policy)), errSchemaAttributeTypeUnsupported},
				} {
					t.Run(string(policy)+"/"+kind+"/"+shape+"/"+test.name, func(t *testing.T) {
						root := languageUseRoot(namedLanguageAnyURIOwner(shape, `<xs:attribute name="a" type="r:Alias"/>`) + test.declaration)
						schema, err := parseIDUseSchema(t, root, nil, policy)
						assertZeroSchema(t, schema)
						d := requireDiagnostic(t, err)
						if d.Class() != FailureUnsupported || d.Code() != test.code || d.Loc() != elementReferenceTestAttributeLoc(t, root, test.marker) || d.SpecRef() != test.spec || len(d.Related()) != 0 || !errors.Is(err, test.cause) || !errors.Is(err, ErrUnsupported) {
							t.Fatalf("excluded type = %v, related %v", err, d.Related())
						}
					})
				}
			}
		}
	}
}

//nolint:gocognit // The policy chooses the edition for every owner and datatype.
func TestNamedLanguageAnyURIAttributeEditionLabels(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, label := range []string{"1.0", "1.1"} {
			for _, kind := range []string{"language", "anyURI"} {
				for _, shape := range []string{"empty", "choice", "sequence", "all"} {
					t.Run(string(policy)+"/"+label+"/"+kind+"/"+shape, func(t *testing.T) {
						root := languageUseRoot(namedLanguageAnyURIOwner(shape, `<xs:attribute name="a" type="r:Alias"/>`) + `<xs:simpleType name="Alias"><xs:restriction base="xs:` + kind + `"/></xs:simpleType>`)
						root = strings.Replace(root, `attributeFormDefault="qualified"`, `attributeFormDefault="qualified" version="`+label+`"`, 1)
						schema, err := parseIDUseSchema(t, root, nil, policy)
						if err != nil {
							t.Fatal(err)
						}
						owner := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Record"))[0], "Record")
						if len(owner.AttributeUses()) != 1 {
							t.Fatalf("uses = %v", owner.AttributeUses())
						}
						use, ok := owner.AttributeUses()[0].(LocalAttributeUse)
						if !ok {
							t.Fatalf("use = %T", owner.AttributeUses()[0])
						}
						ref, ok := use.TypeReference()
						if !ok || !ref.IsNamed() || use.TypeLoc() != elementReferenceTestAttributeLoc(t, root, `type="r:Alias"`) || ref.VarietyLoc() != elementReferenceTestAttributeLoc(t, root, `<xs:restriction base="xs:`+kind+`"`) {
							t.Fatalf("edition-label facts = %#v/%#v", use, ref)
						}
					})
				}
			}
		}
	}
}

//nolint:gocognit // Non-visible types and duplicate declarations retain their original exits.
func TestNamedLanguageAnyURIAttributeGraphVisibilityAndDuplicates(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, kind := range []string{"language", "anyURI"} {
			for _, shape := range []string{"empty", "choice", "sequence", "all"} {
				t.Run(string(policy)+"/"+kind+"/"+shape, func(t *testing.T) {
					root := languageUseRoot(`<xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/>` +
						namedLanguageAnyURIOwner(shape, `<xs:attribute name="a" type="o:Hidden"/>`))
					root = strings.Replace(root, `xmlns:r="urn:root"`, `xmlns:r="urn:root" xmlns:o="urn:other"`, 1)
					fixtures := map[string]discoveryFixture{
						"bridge.xsd": {id: "bridge.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge"><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`},
						"other.xsd":  {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Hidden"><xs:restriction base="xs:` + kind + `"/></xs:simpleType></xs:schema>`},
					}
					schema, err := parseIDUseSchema(t, root, fixtures, policy)
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaAttributeTypeUnresolvedCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `type="o:Hidden"`) || d.SpecRef() != schemaAttributeTypeSpecRef(languageUseVersion(policy)) || len(d.Related()) != 0 || !errors.Is(err, errSchemaAttributeTypeUnresolved) {
						t.Fatalf("invisible type = %v, related %v", err, d.Related())
					}
					duplicate := languageUseRoot(namedLanguageAnyURIOwner(shape, `<xs:attribute name="a" type="r:Alias"/>`) +
						`<xs:simpleType name="Alias"><xs:restriction base="xs:` + kind + `"/></xs:simpleType>` +
						`<xs:simpleType name="Alias" id="second"><xs:restriction base="xs:` + kind + `"/></xs:simpleType>`)
					schema, err = parseIDUseSchema(t, duplicate, nil, policy)
					assertZeroSchema(t, schema)
					d = requireDiagnostic(t, err)
					first := elementReferenceTestAttributeLoc(t, duplicate, `<xs:simpleType name="Alias"><xs:restriction`)
					second := elementReferenceTestAttributeLoc(t, duplicate, `<xs:simpleType name="Alias" id="second"`)
					if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaGlobalDuplicateCode || d.Loc() != second || !reflect.DeepEqual(d.Related(), []Loc{first}) || d.SpecRef() != schemaGlobalDuplicateSpecRef(languageUseVersion(policy)) || !errors.Is(err, errSchemaGlobalDeclarationDuplicate) {
						t.Fatalf("duplicate target = %v, related %v", err, d.Related())
					}
				})
			}
		}
	}
}
