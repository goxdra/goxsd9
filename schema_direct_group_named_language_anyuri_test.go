package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func directGroupNamedLanguageGraph(kind, target string) (string, map[string]discoveryFixture) {
	root, fixtures := namedLanguageAnyURIGraph(kind, "empty")
	root = strings.Replace(root, `<xs:complexType name="Record">`, `<xs:complexType name="Record"><xs:group ref="r:Fields" minOccurs="2" maxOccurs="18446744073709551616"/>`, 1)
	group := `<xs:group name="Fields"><xs:` + target + `><xs:element ref="r:z"/><xs:element ref="r:a"/></xs:` + target + `></xs:group>` +
		`<xs:element name="z" type="xs:integer"/><xs:element name="a" type="xs:integer"/>`
	root = strings.Replace(root, `</xs:schema>`, group+`</xs:schema>`, 1)
	return root, fixtures
}

//nolint:gocognit,funlen // The new owner gate must copy completed named facts across graph boundaries.
func TestDirectGroupNamedLanguageAnyURIPublicFacts(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, kind := range []string{"language", "anyURI"} {
			for _, target := range []string{"choice", "sequence"} {
				t.Run(string(policy)+"/"+kind+"/"+target, func(t *testing.T) {
					root, fixtures := directGroupNamedLanguageGraph(kind, target)
					schema, err := parseIDUseSchema(t, root, fixtures, policy)
					if err != nil {
						t.Fatal(err)
					}
					again, err := parseIDUseSchema(t, root, fixtures, policy)
					if err != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
						t.Fatalf("repeated parse = %v", err)
					}
					if schema.LanguagePolicy() != policy {
						t.Fatalf("policy = %s", schema.LanguagePolicy())
					}
					wantSources := []SourceID{"root.xsd", "included.xsd", "chameleon.xsd", "other.xsd"}
					for i, document := range schema.Documents() {
						if i >= len(wantSources) || document.Source() != wantSources[i] {
							t.Fatalf("document %d = %s", i, document.Source())
						}
					}
					if len(schema.Documents()) != len(wantSources) {
						t.Fatalf("documents = %v", schema.Documents())
					}
					owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Record")
					if owner.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Record"`) {
						t.Fatalf("owner Loc = %s", owner.Loc())
					}
					particle, ok := owner.Particle().(ModelGroupReferenceParticle)
					group := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, "urn:root", "Fields"))
					if !ok || len(group) != 1 || particle.Name() != mustTestQName(t, "urn:root", "Fields") || particle.Ref() != particle.Name() || particle.TargetID() != group[0].ID() || particle.Occurrences().String() != "2/18446744073709551616" || particle.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:group ref="r:Fields"`) || particle.RefLoc() != elementReferenceTestAttributeLoc(t, root, `ref="r:Fields"`) {
						t.Fatalf("group = %#v; definitions = %v", owner.Particle(), group)
					}
					minimum := particle.Occurrences().Minimum()
					maximum, finite := particle.Occurrences().Maximum().Finite()
					if !finite {
						t.Fatal("finite group bound became unbounded")
					}
					minimum.value.SetInt64(9)
					maximum.value.SetInt64(9)
					if particle.Occurrences().String() != "2/18446744073709551616" {
						t.Fatal("occurrence mutation reached schema")
					}
					definition, hasDefinition := group[0].ModelGroupDefinition()
					if !hasDefinition || definition.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:group name="Fields"`) {
						t.Fatalf("group definition = %v", group[0])
					}
					groupParticle := definition.Particle()
					var terms []Particle
					if target == "choice" {
						choice, ok := groupParticle.(ChoiceParticle)
						if !ok {
							t.Fatalf("target = %T", groupParticle)
						}
						terms = choice.Alternatives()
					}
					if target == "sequence" {
						sequence, ok := groupParticle.(SequenceParticle)
						if !ok {
							t.Fatalf("target = %T", groupParticle)
						}
						terms = sequence.Particles()
					}
					if len(terms) != 2 {
						t.Fatalf("target terms = %v", terms)
					}
					for index, local := range []string{"z", "a"} {
						term, ok := terms[index].(ElementReferenceParticle)
						if !ok || term.Ref() != mustTestQName(t, "urn:root", local) || term.RefLoc() != elementReferenceTestAttributeLoc(t, root, `ref="r:`+local+`"`) {
							t.Fatalf("term %d = %v", index, terms[index])
						}
					}
					uses := owner.AttributeUses()
					if len(uses) != 6 {
						t.Fatalf("uses = %v", uses)
					}
					for i, want := range []struct{ local, written, namespace, typeName, source string }{
						{"direct", "r:Direct", "urn:root", "Direct", "root.xsd"},
						{"inherited", "r:Child", "urn:root", "Child", "root.xsd"},
						{"included", "r:Included", "urn:root", "Included", "included.xsd"},
						{"adopted", "r:Adopted", "urn:root", "Adopted", "chameleon.xsd"},
						{"imported", "o:Imported", "urn:other", "Imported", "other.xsd"},
					} {
						use, ok := uses[i].(LocalAttributeUse)
						wantType := mustTestQName(t, want.namespace, want.typeName)
						typeLoc := elementReferenceTestAttributeLoc(t, root, `type="`+want.written+`"`)
						useNamespace := "urn:root"
						if want.local == "included" {
							useNamespace = ""
						}
						if !ok || use.Name() != mustTestQName(t, useNamespace, want.local) || use.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="`+want.local+`"`) || use.NameLoc() != elementReferenceTestAttributeLoc(t, root, `name="`+want.local+`"`) || use.TypeLoc() != typeLoc || use.DeclaredType() != wantType {
							t.Fatalf("%s use = %#v", want.local, uses[i])
						}
						ref, hasRef := use.TypeReference()
						id, hasID := use.TypeID()
						refID, hasRefID := ref.ComponentID()
						if !hasRef || !hasID || !hasRefID || !ref.IsNamed() || ref.IsBuiltin() || ref.Name() != wantType || ref.QName() != wantType || ref.Loc() != typeLoc || ref.Variety() != SimpleTypeVarietyAtomicRestriction || id != refID || id.Source() != SourceID(want.source) || id != componentIDForName(t, schema, wantType) || len(ref.StringEnumerationFacets().Values()) != 0 {
							t.Fatalf("%s copied type = %#v/%v", want.local, ref, id)
						}
						declaration := root
						if want.source != "root.xsd" {
							declaration = fixtures[want.source].contents
						}
						base := `base="xs:` + kind + `"`
						baseName := mustTestQName(t, testXSDNamespace, kind)
						if want.local == "inherited" {
							base = `base="r:Direct"`
							baseName = mustTestQName(t, "urn:root", "Direct")
						}
						definition := schemaEnumerationTestDefinitionInNamespace(t, schema, want.namespace, want.typeName)
						baseRef, hasBase := definition.BaseReference()
						variety := `<xs:restriction ` + base
						if definition.ID() != id || definition.Loc() != allParticleTestTokenLoc(t, SourceID(want.source), declaration, `<xs:simpleType name="`+want.typeName+`"`, 1) || definition.Variety() != ref.Variety() || definition.VarietyLoc() != ref.VarietyLoc() || definition.VarietyLoc() != allParticleTestTokenLoc(t, SourceID(want.source), declaration, variety, 1) || definition.Base() != baseName || definition.BaseLoc() != allParticleTestTokenLoc(t, SourceID(want.source), declaration, base, 1) || !hasBase || baseRef.Name() != baseName || len(definition.StringEnumerationFacets().Values()) != 0 {
							t.Fatalf("%s definition = %#v", want.local, definition)
						}
					}
					if uses[0].Name().Local() != "direct" || uses[5].Name().Local() != "after" {
						t.Fatalf("lexical use order = %v", uses)
					}
					uses[0] = nil
					if owner.AttributeUses()[0] == nil {
						t.Fatal("use slice mutation reached schema")
					}
					components := schema.Components()
					components[0] = Component{}
					if schema.Components()[0].Name().Local() != "Record" {
						t.Fatal("component slice mutation reached schema")
					}
					var walk []string
					if err := schema.Walk(func(component Component) error {
						walk = append(walk, string(component.ID().Source())+":"+component.Name().Local())
						return nil
					}); err != nil {
						t.Fatal(err)
					}
					wantWalk := []string{"root.xsd:Record", "root.xsd:Direct", "root.xsd:Child", "root.xsd:root", "root.xsd:Fields", "root.xsd:z", "root.xsd:a", "included.xsd:Included", "chameleon.xsd:Adopted", "other.xsd:Imported"}
					if !reflect.DeepEqual(walk, wantWalk) {
						t.Fatalf("walk = %v", walk)
					}
				})
			}
		}
	}
}

//nolint:gocognit // Both public consumers must reject each newly queryable target and type.
func TestDirectGroupNamedLanguageAnyURIConsumersUnsupported(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, kind := range []string{"language", "anyURI"} {
			for _, target := range []string{"choice", "sequence"} {
				t.Run(string(policy)+"/"+kind+"/"+target, func(t *testing.T) {
					root := languageUseRoot(directGroupLanguageOwner(`<xs:group ref="r:Fields"/>`, `<xs:attribute name="direct" type="r:Alias"/>`) +
						`<xs:group name="Fields"><xs:` + target + `><xs:element ref="r:value"/></xs:` + target + `></xs:group>` +
						`<xs:element name="value" type="xs:integer"/>` +
						`<xs:simpleType name="Alias"><xs:restriction base="xs:` + kind + `"/></xs:simpleType>` +
						`<xs:element name="root" type="r:Record"/>`)
					schema, err := parseIDUseSchema(t, root, nil, policy)
					if err != nil {
						t.Fatal(err)
					}
					useLoc := elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="direct"`)
					ownerLoc := elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Record"`)
					rootLoc := elementReferenceTestAttributeLoc(t, root, `<xs:element name="root"`)
					instanceErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<r:root xmlns:r="urn:root"/>`)))
					vd := requireDiagnostic(t, instanceErr)
					if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != useLoc || vd.SpecRef() != instanceValidationSpecRef(languageUseVersion(policy)) || !reflect.DeepEqual(vd.Related(), []Loc{rootLoc, ownerLoc, useLoc}) || !errors.Is(instanceErr, ErrUnsupported) || !errors.Is(instanceErr, errInstanceAttributes) {
						t.Fatalf("validation = %v; related = %v", instanceErr, vd.Related())
					}
					output, generationErr := GenerateGo(schema, "generated")
					gd := requireDiagnostic(t, generationErr)
					if output != nil || gd.Class() != FailureUnsupported || gd.Code() != diagnosticCodegenUnsupported || gd.Loc() != useLoc || gd.SpecRef() != codegenDirectSequenceSpecReference(languageUseVersion(policy), codegenDirectSequenceParticlesReference) || !reflect.DeepEqual(gd.Related(), []Loc{ownerLoc, useLoc}) || !errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenUnsupported) {
						t.Fatalf("generation = %v/%v; related = %v", output, generationErr, gd.Related())
					}
				})
			}
		}
	}
}

func directGroupNamedLanguageRoot(kind, group, uses, extra string) string {
	return languageUseRoot(directGroupLanguageOwner(group, uses) + directGroupChoice +
		`<xs:simpleType name="Direct"><xs:restriction base="xs:` + kind + `"/></xs:simpleType>` +
		`<xs:simpleType name="Child"><xs:restriction base="r:Direct"/></xs:simpleType>` + extra)
}

//nolint:gocognit // Each named dependency exits with its own located diagnostic before publication.
func TestDirectGroupNamedLanguageAnyURIAlternateExits(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		version := languageUseVersion(policy)
		for _, kind := range []string{"language", "anyURI"} {
			for _, test := range []struct {
				name, old, replacement, marker, code, spec string
				related                                    []string
				class                                      FailureClass
				cause                                      error
			}{
				{"missing type", `type="r:Child"`, `type="r:Missing"`, `type="r:Missing"`, diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(version), nil, FailureInvalid, errSchemaAttributeTypeUnresolved},
				{"wrong kind type", `type="r:Child"`, `type="r:Fields"`, `type="r:Fields"`, diagnosticSchemaAttributeTypeWrongKindCode, schemaAttributeTypeSpecRef(version), []string{`<xs:group name="Fields"`}, FailureInvalid, errSchemaAttributeTypeWrongKind},
				{"type cycle", `base="xs:` + kind + `"`, `base="r:Child"`, `type="r:Child"`, diagnosticSchemaAttributeTypeCycleCode, schemaAttributeTypeSpecRef(version), []string{`<xs:attribute name="value"`, `base="r:Child"`, `base="r:Direct"`, `<xs:simpleType name="Child"`, `<xs:simpleType name="Direct"`}, FailureInvalid, errSchemaSimpleTypeBaseCycle},
				{"simple final", `<xs:simpleType name="Direct">`, `<xs:simpleType name="Direct" final="restriction">`, `base="r:Direct"`, diagnosticSchemaSimpleTypeBaseCode, schemaSimpleTypeRestrictionSpecRef(version), []string{`final="restriction"`}, FailureInvalid, errSchemaSimpleTypeInvalidDerivation},
				{"authored facet", `<xs:restriction base="r:Direct"/>`, `<xs:restriction base="r:Direct"><xs:pattern value=".*"/></xs:restriction>`, `<xs:pattern`, UnsupportedDatatypeFacetCode, tokenDiagnosticSpecRef(version, "decimal"), nil, FailureUnsupported, ErrUnsupported},
				{"parent facet", `<xs:restriction base="xs:` + kind + `"/>`, `<xs:restriction base="xs:` + kind + `"><xs:enumeration value="en"/></xs:restriction>`, `<xs:enumeration`, UnsupportedDatatypeFacetCode, tokenDiagnosticSpecRef(version, "decimal"), nil, FailureUnsupported, ErrUnsupported},
				{"group before type", `ref="r:Fields"`, `ref="r:Missing"`, `ref="r:Missing"`, diagnosticSchemaModelGroupReferenceUnresolvedCode, schemaModelGroupReferenceSpecRef(version), nil, FailureInvalid, errSchemaModelGroupReferenceUnresolved},
			} {
				t.Run(string(policy)+"/"+kind+"/"+test.name, func(t *testing.T) {
					root := directGroupNamedLanguageRoot(kind, `<xs:group ref="r:Fields"/>`, `<xs:attribute name="value" type="r:Child"/>`, "")
					root = strings.Replace(root, test.old, test.replacement, 1)
					if test.name == "group before type" {
						root = strings.Replace(root, `type="r:Child"`, `type="r:Absent"`, 1)
					}
					schema, err := parseIDUseSchema(t, root, nil, policy)
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					var related []Loc
					for _, marker := range test.related {
						related = append(related, elementReferenceTestAttributeLoc(t, root, marker))
					}
					if d.Class() != test.class || d.Code() != test.code || d.Loc() != elementReferenceTestAttributeLoc(t, root, test.marker) || d.SpecRef() != test.spec || !reflect.DeepEqual(d.Related(), related) || !errors.Is(err, test.cause) {
						t.Fatalf("%s = %v, related %v, want %v", test.name, err, d.Related(), related)
					}
				})
			}
		}
	}
}

//nolint:gocognit // Omission follows named dependency and group target resolution.
func TestDirectGroupNamedLanguageAnyURIOmissionAndPrecedence(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, kind := range []string{"language", "anyURI"} {
			t.Run(string(policy)+"/"+kind, func(t *testing.T) {
				group := `<xs:group ref="r:Fields" minOccurs="0" maxOccurs="0"/>`
				uses := `<xs:attribute name="keep" type="r:Child"/>` + `<xs:attribute name="gone" type="r:Direct" use="prohibited"/>`
				root := directGroupNamedLanguageRoot(kind, group, uses, "")
				schema, err := parseIDUseSchema(t, root, nil, policy)
				if err != nil {
					t.Fatal(err)
				}
				owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Record")
				if owner.Particle() != nil || len(owner.AttributeUses()) != 1 || owner.AttributeUses()[0].Name().Local() != "keep" {
					t.Fatalf("omitted facts = %T/%v", owner.Particle(), owner.AttributeUses())
				}
				for _, test := range []struct {
					name, old, replacement, marker, code, spec string
					cause                                      error
				}{
					{"missing zero group", `ref="r:Fields"`, `ref="r:Missing"`, `ref="r:Missing"`, diagnosticSchemaModelGroupReferenceUnresolvedCode, schemaModelGroupReferenceSpecRef(languageUseVersion(policy)), errSchemaModelGroupReferenceUnresolved},
					{"missing prohibited type", `type="r:Direct" use="prohibited"`, `type="r:Missing" use="prohibited"`, `type="r:Missing"`, diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(languageUseVersion(policy)), errSchemaAttributeTypeUnresolved},
				} {
					t.Run(test.name, func(t *testing.T) {
						broken := strings.Replace(root, test.old, test.replacement, 1)
						failed, failure := parseIDUseSchema(t, broken, nil, policy)
						assertZeroSchema(t, failed)
						d := requireDiagnostic(t, failure)
						if d.Class() != FailureInvalid || d.Code() != test.code || d.Loc() != elementReferenceTestAttributeLoc(t, broken, test.marker) || d.SpecRef() != test.spec || len(d.Related()) != 0 || !errors.Is(failure, test.cause) {
							t.Fatalf("%s = %v; related = %v", test.name, failure, d.Related())
						}
					})
				}
			})
		}
	}
}

func TestDirectGroupNamedLanguageAnyURIOtherShapesStayExcluded(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, kind := range []string{"language", "anyURI"} {
			attribute := `<xs:attribute name="value" type="r:Alias"/>`
			alias := `<xs:simpleType name="Alias"><xs:restriction base="xs:` + kind + `"/></xs:simpleType>`
			for _, test := range []struct{ name, body, extra, marker, spec string }{
				{"anonymous direct group", `<xs:element name="root"><xs:complexType><xs:group ref="r:Fields"/>` + attribute + `</xs:complexType></xs:element>`, directGroupChoice, `ref="r:Fields"`, "xsd10-structures#schema-document"},
				{"nested direct group", `<xs:complexType name="Record"><xs:choice><xs:group ref="r:Fields"/></xs:choice>` + attribute + `</xs:complexType>`, directGroupChoice, `ref="r:Fields"`, "xsd10-structures#schema-document"},
				{"all group target", directGroupLanguageOwner(`<xs:group ref="r:Fields"/>`, attribute), `<xs:group name="Fields"><xs:all/></xs:group>`, `<xs:all`, directGroupUnsupportedSyntaxSpecRef(languageUseVersion(policy))},
				{"recursive group target", directGroupLanguageOwner(`<xs:group ref="r:Fields"/>`, attribute), `<xs:group name="Fields"><xs:choice><xs:choice><xs:group ref="r:Fields"/></xs:choice></xs:choice></xs:group>`, `ref="r:Fields"/></xs:choice></xs:choice>`, directGroupUnsupportedSyntaxSpecRef(languageUseVersion(policy))},
			} {
				t.Run(string(policy)+"/"+kind+"/"+test.name, func(t *testing.T) {
					root := languageUseRoot(test.body + test.extra + alias)
					root = strings.Replace(root, `attributeFormDefault="qualified"`, `attributeFormDefault="qualified" version="`+string(languageUseVersion(policy))+`"`, 1)
					failed, err := parseIDUseSchema(t, root, nil, policy)
					assertZeroSchema(t, failed)
					d := requireDiagnostic(t, err)
					if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, test.marker) || d.SpecRef() != test.spec || len(d.Related()) != 0 || !errors.Is(err, ErrUnsupported) {
						t.Fatalf("%s = %v; related = %v", test.name, err, d.Related())
					}
				})
			}
		}
	}
}
