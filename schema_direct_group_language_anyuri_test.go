package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

const directGroupChoice = `<xs:group name="Fields"><xs:choice><xs:element ref="r:value"/></xs:choice></xs:group><xs:element name="value" type="xs:integer"/>`

func directGroupLanguageOwner(group, uses string) string {
	return `<xs:complexType name="Record">` + group + uses + `</xs:complexType>`
}

func directGroupLanguageRoot(group, uses, extra string) string {
	return languageUseRoot(directGroupLanguageOwner(group, uses) + directGroupChoice + extra)
}

func directGroupLanguageUse(name string) string {
	return `<xs:attribute name="` + name + `" type="xs:` + name + `"/>`
}

//nolint:gocognit // The named direct-group admission is one public copied-fact contract.
func TestDirectGroupLanguageAnyURIFactsAcrossPolicies(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, names := range [][]string{{"language"}, {"anyURI"}, {"language", "anyURI"}} {
			caseName := strings.Join(names, "+")
			t.Run(string(policy)+"/"+caseName, func(t *testing.T) {
				useElements := []string{`<xs:attribute name="flag" type="xs:boolean"/>`}
				for _, name := range names {
					useElements = append(useElements, `<xs:attribute name="`+name+`" type="builtin:`+name+`" use="required" form="unqualified"/>`)
				}
				useElements = append(useElements, `<xs:attribute name="gone" type="xs:anyURI" use="prohibited"/>`)
				root := directGroupLanguageRoot(`<xs:group ref="r:Fields" minOccurs="2" maxOccurs="5"/>`, strings.Join(useElements, ""), "")
				root = strings.Replace(root, `xmlns:r="urn:root"`, `xmlns:r="urn:root" xmlns:builtin="`+testXSDNamespace+`"`, 1)
				schema, err := parseIDUseSchema(t, root, nil, policy)
				if err != nil {
					t.Fatal(err)
				}
				if schema.LanguagePolicy() != policy || len(schema.Documents()) != 1 || schema.Documents()[0].Source() != "root.xsd" {
					t.Fatalf("policy/documents = %s/%v", schema.LanguagePolicy(), schema.Documents())
				}
				owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Record")
				if owner.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Record"`) {
					t.Fatalf("owner Loc = %s", owner.Loc())
				}
				group, ok := owner.Particle().(ModelGroupReferenceParticle)
				groupName := mustTestQName(t, "urn:root", "Fields")
				groups := schema.FindKind(ComponentKindModelGroupDefinition, groupName)
				if !ok || len(groups) != 1 || group.Name() != groupName || group.Ref() != groupName || group.TargetID() != groups[0].ID() || group.Occurrences().String() != "2/5" || group.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:group ref="r:Fields"`) || group.RefLoc() != elementReferenceTestAttributeLoc(t, root, `ref="r:Fields"`) {
					t.Fatalf("group = %#v; targets = %v", group, groups)
				}
				uses := owner.AttributeUses()
				if len(uses) != len(names)+1 || uses[0].Name() != mustTestQName(t, "urn:root", "flag") {
					t.Fatalf("ordered uses = %v", uses)
				}
				for index, name := range names {
					use, ok := uses[index+1].(LocalAttributeUse)
					attributeLoc := elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="`+name+`"`)
					typeLoc := elementReferenceTestAttributeLoc(t, root, `type="builtin:`+name+`"`)
					useLoc := mustTestLoc(t, "root.xsd", attributeLoc.Line(), attributeLoc.Column()+len(`<xs:attribute name="`+name+`" type="builtin:`+name+`" `))
					formLoc := mustTestLoc(t, "root.xsd", useLoc.Line(), useLoc.Column()+len(`use="required" `))
					wantType := mustTestQName(t, testXSDNamespace, name)
					if !ok || use.Name() != mustTestQName(t, "", name) || use.Loc() != attributeLoc || use.NameLoc() != elementReferenceTestAttributeLoc(t, root, `name="`+name+`"`) || use.TypeLoc() != typeLoc || use.UseLoc() != useLoc || use.FormLoc() != formLoc || use.Use() != AttributeUseRequired || use.DeclaredType() != wantType {
						t.Fatalf("%s use facts = %#v", name, use)
					}
					if id, hasID := use.TypeID(); hasID || !id.IsZero() {
						t.Fatalf("%s synthetic type ID = %v/%t", name, id, hasID)
					}
					ref, hasRef := use.TypeReference()
					if !hasRef || !ref.IsBuiltin() || ref.QName() != wantType || ref.Name() != wantType || ref.Loc() != typeLoc || ref.VarietyLoc() != typeLoc || ref.Variety() != SimpleTypeVarietyAtomicRestriction || len(ref.StringEnumerationFacets().Values()) != 0 {
						t.Fatalf("%s built-in facts = %#v/%t", name, ref, hasRef)
					}
					if id, hasID := ref.ComponentID(); hasID || !id.IsZero() {
						t.Fatalf("%s synthetic reference ID = %v/%t", name, id, hasID)
					}
				}
				uses[0] = nil
				if owner.AttributeUses()[0] == nil {
					t.Fatal("AttributeUses mutation reached schema")
				}
				components := schema.Components()
				components[0] = Component{}
				if schema.Components()[0].Name() != mustTestQName(t, "urn:root", "Record") {
					t.Fatal("Components mutation reached schema")
				}
				wantWalk := []string{"Record", "Fields", "value"}
				for replay := 0; replay < 2; replay++ {
					var walk []string
					if walkErr := schema.Walk(func(component Component) error { walk = append(walk, component.Name().Local()); return nil }); walkErr != nil {
						t.Fatal(walkErr)
					}
					if !reflect.DeepEqual(walk, wantWalk) {
						t.Fatalf("walk %d = %v", replay, walk)
					}
				}
			})
		}
	}
}

//nolint:gocognit // Each public consumer must identify its own located unsupported exit.
func TestDirectGroupLanguageAnyURIConsumersRemainUnsupported(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, name := range []string{"language", "anyURI"} {
			t.Run(string(policy)+"/"+name, func(t *testing.T) {
				root := directGroupLanguageRoot(`<xs:group ref="r:Fields"/>`, directGroupLanguageUse(name), `<xs:element name="root" type="r:Record"/>`)
				schema, err := parseIDUseSchema(t, root, nil, policy)
				if err != nil {
					t.Fatal(err)
				}
				useLoc := elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="`+name+`"`)
				ownerLoc := elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Record"`)
				elementLoc := elementReferenceTestAttributeLoc(t, root, `<xs:element name="root"`)
				instanceErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<r:root xmlns:r="urn:root"/>`)))
				vd := requireDiagnostic(t, instanceErr)
				if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != useLoc || vd.SpecRef() != instanceValidationSpecRef(languageUseVersion(policy)) || !reflect.DeepEqual(vd.Related(), []Loc{elementLoc, ownerLoc, useLoc}) || !errors.Is(instanceErr, ErrUnsupported) || !errors.Is(instanceErr, errInstanceAttributes) {
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

//nolint:gocognit // Prohibited built-ins omit before each consumer reaches the group reference.
func TestDirectGroupLanguageAnyURIProhibitedUseConsumerFallback(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, name := range []string{"language", "anyURI"} {
			t.Run(string(policy)+"/"+name, func(t *testing.T) {
				root := directGroupLanguageRoot(`<xs:group ref="r:Fields"/>`, `<xs:attribute name="gone" type="xs:`+name+`" use="prohibited"/>`, `<xs:element name="root" type="r:Record"/>`)
				schema, err := parseIDUseSchema(t, root, nil, policy)
				if err != nil {
					t.Fatal(err)
				}
				owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Record")
				if len(owner.AttributeUses()) != 0 {
					t.Fatalf("prohibited use survived: %v", owner.AttributeUses())
				}
				refLoc := elementReferenceTestAttributeLoc(t, root, `ref="r:Fields"`)
				groupLoc := elementReferenceTestAttributeLoc(t, root, `<xs:group ref="r:Fields"`)
				ownerLoc := elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Record"`)
				rootLoc := elementReferenceTestAttributeLoc(t, root, `<xs:element name="root"`)
				targetLoc := elementReferenceTestAttributeLoc(t, root, `<xs:group name="Fields"`)
				instanceErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<r:root xmlns:r="urn:root"/>`)))
				vd := requireDiagnostic(t, instanceErr)
				if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != refLoc || vd.SpecRef() != instanceValidationSpecRef(languageUseVersion(policy)) || !reflect.DeepEqual(vd.Related(), []Loc{rootLoc, ownerLoc, groupLoc}) || !errors.Is(instanceErr, ErrUnsupported) || !errors.Is(instanceErr, errInstanceModelGroupReference) {
					t.Fatalf("validation group fallback = %v; related = %v", instanceErr, vd.Related())
				}
				output, generationErr := GenerateGo(schema, "generated")
				gd := requireDiagnostic(t, generationErr)
				if output != nil || gd.Class() != FailureUnsupported || gd.Code() != diagnosticCodegenUnsupported || gd.Loc() != refLoc || gd.SpecRef() != codegenDirectSequenceSpecReference(languageUseVersion(policy), codegenDirectSequenceParticlesReference) || !reflect.DeepEqual(gd.Related(), []Loc{ownerLoc, groupLoc, refLoc, targetLoc}) || !errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenDirectModelGroupReference) {
					t.Fatalf("generation group fallback = %v/%v; related = %v", output, generationErr, gd.Related())
				}
			})
		}
	}
}

func TestDirectGroupLanguageAnyURIResolverFailureKeepsCause(t *testing.T) {
	root := languageUseRoot(`<xs:include schemaLocation="missing.xsd"/>` + directGroupLanguageOwner(`<xs:group ref="r:Fields"/>`, directGroupLanguageUse("language")) + directGroupChoice)
	source, err := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("direct-group source unavailable")
	schema, err := ParseSchemaWithPolicy(source, &discoveryResolver{failures: map[string]error{"missing.xsd": cause}}, Strict11)
	assertZeroSchema(t, schema)
	d := requireDiagnostic(t, err)
	if d.Class() != FailureResolution || d.Code() != SourceResolveCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:include`) || d.SpecRef() != "" || len(d.Related()) != 0 || !errors.Is(err, cause) {
		t.Fatalf("resolver diagnostic = %v", err)
	}
}

//nolint:gocognit // Group identities, document discovery, and use provenance are queried together.
func TestDirectGroupLanguageAnyURIGraphVisibilityAndProvenance(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, test := range []struct {
			name, composition, ref, targetNamespace, targetSource string
			fixtures                                              map[string]discoveryFixture
		}{
			{"forward", "", "r:Fields", "urn:root", "root.xsd", nil},
			{"include", `<xs:include schemaLocation="included.xsd"/>`, "r:Fields", "urn:root", "included.xsd", map[string]discoveryFixture{"included.xsd": {id: "included.xsd", contents: languageUseRoot(directGroupChoice)}}},
			{"import", `<xs:import namespace="urn:other" schemaLocation="other.xsd"/>`, "o:Fields", "urn:other", "other.xsd", map[string]discoveryFixture{"other.xsd": {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:other"><xs:group name="Fields"><xs:choice><xs:element ref="o:value"/></xs:choice></xs:group><xs:element name="value" type="xs:integer"/></xs:schema>`}}},
			{"chameleon", `<xs:include schemaLocation="chameleon.xsd"/>`, "r:Fields", "urn:root", "chameleon.xsd", map[string]discoveryFixture{"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:group name="Fields"><xs:choice><xs:element ref="value"/></xs:choice></xs:group><xs:element name="value" type="xs:integer"/></xs:schema>`}}},
		} {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				root := languageUseRoot(test.composition + directGroupLanguageOwner(`<xs:group ref="`+test.ref+`" minOccurs="1" maxOccurs="3"/>`, directGroupLanguageUse("language")+directGroupLanguageUse("anyURI")))
				root = strings.Replace(root, `xmlns:r="urn:root"`, `xmlns:r="urn:root" xmlns:o="urn:other"`, 1)
				if test.name == "forward" {
					root = strings.Replace(root, `</xs:schema>`, directGroupChoice+`</xs:schema>`, 1)
				}
				schema, err := parseIDUseSchema(t, root, test.fixtures, policy)
				if err != nil {
					t.Fatal(err)
				}
				owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Record")
				particle, ok := owner.Particle().(ModelGroupReferenceParticle)
				groups := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, test.targetNamespace, "Fields"))
				if !ok || len(groups) != 1 || groups[0].ID().Source() != SourceID(test.targetSource) || particle.TargetID() != groups[0].ID() || particle.Ref() != mustTestQName(t, test.targetNamespace, "Fields") || particle.Occurrences().String() != "1/3" || particle.RefLoc() != elementReferenceTestAttributeLoc(t, root, `ref="`+test.ref+`"`) {
					t.Fatalf("group facts = %#v / %v", particle, groups)
				}
				uses := owner.AttributeUses()
				if len(uses) != 2 || uses[0].Name() != mustTestQName(t, "urn:root", "language") || uses[1].Name() != mustTestQName(t, "urn:root", "anyURI") {
					t.Fatalf("ordered uses = %v", uses)
				}
				for i, name := range []string{"language", "anyURI"} {
					use, ok := uses[i].(LocalAttributeUse)
					if !ok || use.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="`+name+`"`) || use.TypeLoc() != elementReferenceTestAttributeLoc(t, root, `type="xs:`+name+`"`) || use.DeclaredType() != mustTestQName(t, testXSDNamespace, name) {
						t.Fatalf("%s use provenance = %#v", name, uses[i])
					}
				}
				var first []string
				for replay := 0; replay < 2; replay++ {
					var walked []string
					if walkErr := schema.Walk(func(component Component) error {
						walked = append(walked, string(component.ID().Source())+":"+component.Name().Local())
						return nil
					}); walkErr != nil {
						t.Fatal(walkErr)
					}
					if replay == 0 {
						first = walked
					}
					if !reflect.DeepEqual(first, walked) || len(walked) != 3 || walked[0] != "root.xsd:Record" || walked[1] != test.targetSource+":Fields" || walked[2] != test.targetSource+":value" {
						t.Fatalf("walk %d = %v", replay, walked)
					}
				}
			})
		}
	}
}

//nolint:gocognit // Omitted terms and uses still require references and types to resolve.
func TestDirectGroupLanguageAnyURIOmissionPrecedence(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, name := range []string{"language", "anyURI"} {
			t.Run(string(policy)+"/"+name, func(t *testing.T) {
				group := `<xs:group ref="r:Fields" minOccurs="0" maxOccurs="0"/>`
				uses := directGroupLanguageUse(name) + `<xs:attribute name="gone" type="xs:` + name + `" use="prohibited"/>`
				root := directGroupLanguageRoot(group, uses, "")
				schema, err := parseIDUseSchema(t, root, nil, policy)
				if err != nil {
					t.Fatal(err)
				}
				owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Record")
				if owner.Particle() != nil || len(owner.AttributeUses()) != 1 || owner.AttributeUses()[0].Name().Local() != name {
					t.Fatalf("omitted facts = %T/%v", owner.Particle(), owner.AttributeUses())
				}
				for _, bad := range []struct {
					name, text, old, replacement, marker, code, spec string
					cause                                            error
				}{
					{"missing group", root, `ref="r:Fields"`, `ref="r:Missing"`, `ref="r:Missing"`, diagnosticSchemaModelGroupReferenceUnresolvedCode, schemaModelGroupReferenceSpecRef(languageUseVersion(policy)), errSchemaModelGroupReferenceUnresolved},
					{"prohibited type", root, `type="xs:` + name + `" use="prohibited"`, `type="r:Missing" use="prohibited"`, `type="r:Missing"`, diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(languageUseVersion(policy)), errSchemaAttributeTypeUnresolved},
				} {
					t.Run(bad.name, func(t *testing.T) {
						text := strings.Replace(bad.text, bad.old, bad.replacement, 1)
						failed, failure := parseIDUseSchema(t, text, nil, policy)
						assertZeroSchema(t, failed)
						d := requireDiagnostic(t, failure)
						if d.Class() != FailureInvalid || d.Code() != bad.code || d.Loc() != elementReferenceTestAttributeLoc(t, text, bad.marker) || d.SpecRef() != bad.spec || len(d.Related()) != 0 || !errors.Is(failure, bad.cause) {
							t.Fatalf("%s diagnostic = %v; related = %v", bad.name, failure, d.Related())
						}
					})
				}
			})
		}
	}
}

//nolint:gocognit // The admission boundary shares syntax and use exits with direct owners.
func TestDirectGroupLanguageAnyURIUseBoundaryDiagnostics(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, name := range []string{"language", "anyURI"} {
			for _, boundary := range directBuiltinBoundaryCases(name) {
				t.Run(string(policy)+"/"+name+"/"+boundary.name, func(t *testing.T) {
					root := directGroupLanguageRoot(`<xs:group ref="r:Fields"/>`, boundary.attributes, boundary.extra)
					failed, err := parseIDUseSchema(t, root, nil, policy)
					assertZeroSchema(t, failed)
					d := requireDiagnostic(t, err)
					wantSpec := schemaAttributeTypeSpecRef(languageUseVersion(policy))
					switch boundary.spec {
					case "use":
						wantSpec = schemaAttributeUseSpecRef(languageUseVersion(policy))
					case "composition":
						wantSpec = ""
					case "global":
						wantSpec = schemaGlobalDuplicateSpecRef(languageUseVersion(policy))
					}
					var related []Loc
					if boundary.related != "" {
						related = []Loc{elementReferenceTestAttributeLoc(t, root, boundary.related)}
					}
					if d.Class() != boundary.class || d.Code() != boundary.code || d.Loc() != elementReferenceTestAttributeLoc(t, root, boundary.marker) || d.SpecRef() != wantSpec || !reflect.DeepEqual(d.Related(), related) {
						t.Fatalf("%s diagnostic = %v; related = %v", boundary.name, err, d.Related())
					}
					if boundary.cause != nil && !errors.Is(err, boundary.cause) {
						t.Fatalf("%s lost cause: %v", boundary.name, err)
					}
				})
			}
		}
	}
}

//nolint:gocognit // Each excluded direct, named, inline, ref, and value form has a separate exit.
func TestDirectGroupLanguageAnyURIAttributeFormsStayExcluded(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, name := range []string{"language", "anyURI"} {
			alias := `<xs:simpleType name="Alias"><xs:restriction base="xs:` + name + `"/></xs:simpleType>`
			inline := `<xs:simpleType><xs:restriction base="xs:` + name + `"/></xs:simpleType>`
			cases := append(directBuiltinExcludedUses(name),
				directBuiltinExcludedUse{"named alias", `<xs:attribute name="a" type="r:Alias"/>`, alias, `type="r:Alias"`, "", errSchemaAttributeUseUnsupported, false},
				directBuiltinExcludedUse{"ref default", `<xs:attribute ref="r:a" default="en"/>`, `<xs:attribute name="a" type="xs:` + name + `"/>`, `default="en"`, "", errSchemaAttributeUseUnsupported, true},
				directBuiltinExcludedUse{"ref fixed", `<xs:attribute ref="r:a" fixed="en"/>`, `<xs:attribute name="a" type="xs:` + name + `"/>`, `fixed="en"`, "", errSchemaAttributeUseUnsupported, true},
				directBuiltinExcludedUse{"named list", `<xs:attribute name="a" type="r:List"/>`, `<xs:simpleType name="List"><xs:list itemType="xs:` + name + `"/></xs:simpleType>`, `type="r:List"`, "", errSchemaAttributeUseUnsupported, false},
				directBuiltinExcludedUse{"named union", `<xs:attribute name="a" type="r:Union"/>`, `<xs:simpleType name="Union"><xs:union memberTypes="xs:` + name + ` xs:` + name + `"/></xs:simpleType>`, `type="r:Union"`, "", errSchemaAttributeUseUnsupported, false},
				directBuiltinExcludedUse{"inline list", `<xs:attribute name="a"><xs:simpleType><xs:list itemType="xs:` + name + `"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, "", errSchemaAttributeUseUnsupported, false},
				directBuiltinExcludedUse{"inline union", `<xs:attribute name="a"><xs:simpleType><xs:union memberTypes="xs:` + name + `"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, "", errSchemaAttributeUseUnsupported, false},
				directBuiltinExcludedUse{"inline atomic", `<xs:attribute name="a">` + inline + `</xs:attribute>`, "", `<xs:simpleType`, "", errSchemaAttributeUseUnsupported, false},
			)
			for _, excluded := range cases {
				t.Run(string(policy)+"/"+name+"/"+excluded.name, func(t *testing.T) {
					root := directGroupLanguageRoot(`<xs:group ref="r:Fields"/>`, excluded.attribute, excluded.extra)
					failed, err := parseIDUseSchema(t, root, nil, policy)
					assertZeroSchema(t, failed)
					d := requireDiagnostic(t, err)
					wantSpec := schemaAttributeTypeSpecRef(languageUseVersion(policy))
					if excluded.useSpec {
						wantSpec = schemaAttributeUseSpecRef(languageUseVersion(policy))
					}
					var related []Loc
					if excluded.related != "" {
						related = []Loc{elementReferenceTestAttributeLoc(t, root, excluded.related)}
					}
					if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, excluded.marker) || d.SpecRef() != wantSpec || !reflect.DeepEqual(d.Related(), related) || !errors.Is(err, ErrUnsupported) || !errors.Is(err, excluded.cause) {
						t.Fatalf("%s diagnostic = %v; related = %v", excluded.name, err, d.Related())
					}
				})
			}
		}
	}
}

func TestDirectGroupLanguageAnyURIFacetsStayExcluded(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, name := range []string{"language", "anyURI"} {
			for _, test := range []struct{ name, attribute, extra string }{
				{"named", `<xs:attribute name="a" type="r:Alias"/>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:` + name + `"><xs:pattern value=".*"/></xs:restriction></xs:simpleType>`},
				{"inline", `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:` + name + `"><xs:pattern value=".*"/></xs:restriction></xs:simpleType></xs:attribute>`, ""},
			} {
				t.Run(string(policy)+"/"+name+"/"+test.name, func(t *testing.T) {
					root := directGroupLanguageRoot(`<xs:group ref="r:Fields"/>`, test.attribute, test.extra)
					failed, err := parseIDUseSchema(t, root, nil, policy)
					assertZeroSchema(t, failed)
					d := requireDiagnostic(t, err)
					if d.Class() != FailureUnsupported || d.Code() != UnsupportedDatatypeFacetCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:pattern`) || d.SpecRef() != tokenDiagnosticSpecRef(languageUseVersion(policy), "decimal") || len(d.Related()) != 0 || !errors.Is(err, ErrUnsupported) {
						t.Fatalf("%s facet = %v; related = %v", test.name, err, d.Related())
					}
				})
			}
		}
	}
}

//nolint:gocognit // Target failures retain source-selected group diagnostics before use admission.
func TestDirectGroupLanguageAnyURIGroupTargetDiagnostics(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, name := range []string{"language", "anyURI"} {
			for _, test := range []struct {
				name, group, declaration, marker, related, code, spec string
				cause                                                 error
			}{
				{"missing", `<xs:group ref="r:Missing"/>`, "", `ref="r:Missing"`, "", diagnosticSchemaModelGroupReferenceUnresolvedCode, schemaModelGroupReferenceSpecRef(languageUseVersion(policy)), errSchemaModelGroupReferenceUnresolved},
				{"wrong kind", `<xs:group ref="r:Wrong"/>`, `<xs:element name="Wrong" type="xs:integer"/>`, `ref="r:Wrong"`, `<xs:element name="Wrong"`, diagnosticSchemaModelGroupReferenceWrongKindCode, schemaModelGroupReferenceSpecRef(languageUseVersion(policy)), errSchemaModelGroupReferenceWrongKind},
				{"cycle", `<xs:group ref="r:Fields"/>`, `<xs:group name="Fields"><xs:choice><xs:group ref="r:Fields"/></xs:choice></xs:group>`, `ref="r:Fields"/></xs:choice>`, `<xs:group name="Fields"`, diagnosticSchemaModelGroupReferenceCycleCode, directGroupCycleSpecRef(languageUseVersion(policy)), errSchemaModelGroupReferenceCycle},
			} {
				t.Run(string(policy)+"/"+name+"/"+test.name, func(t *testing.T) {
					root := languageUseRoot(directGroupLanguageOwner(test.group, directGroupLanguageUse(name)) + test.declaration)
					failed, err := parseIDUseSchema(t, root, nil, policy)
					assertZeroSchema(t, failed)
					d := requireDiagnostic(t, err)
					var related []Loc
					if test.related != "" {
						related = []Loc{elementReferenceTestAttributeLoc(t, root, test.related)}
					}
					if d.Class() != FailureInvalid || d.Code() != test.code || d.Loc() != elementReferenceTestAttributeLoc(t, root, test.marker) || d.SpecRef() != test.spec || !reflect.DeepEqual(d.Related(), related) || !errors.Is(err, test.cause) {
						t.Fatalf("%s group diagnostic = %v; related = %v", test.name, err, d.Related())
					}
				})
			}
		}
	}
}

func directGroupCycleSpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return "xsd10-structures#mg-props-correct"
	}
	return "xsd11-structures#mg-props-correct"
}

//nolint:gocognit // Syntax and duplicate declarations precede the newly admitted use.
func TestDirectGroupLanguageAnyURIOccurrenceAndDuplicateExits(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, name := range []string{"language", "anyURI"} {
			for _, test := range []struct {
				name, group, declaration, marker, code, spec string
				related                                      []string
				cause                                        error
			}{
				{"malformed minimum", `<xs:group ref="r:Fields" minOccurs="many"/>`, directGroupChoice, `minOccurs="many"`, invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(languageUseVersion(policy)), nil, nil},
				{"reversed bounds", `<xs:group ref="r:Fields" minOccurs="2" maxOccurs="1"/>`, directGroupChoice, `<xs:group ref="r:Fields"`, invalidSchemaCompositionCode, schemaParticleCorrectSpecRef(languageUseVersion(policy)), []string{`minOccurs="2"`, `maxOccurs="1"`}, errParticleOccurrenceMinimumExceedsMaximum},
				{"duplicate group", `<xs:group ref="r:Fields"/>`, directGroupChoice + `<xs:group name="Fields"><xs:choice/></xs:group>`, `<xs:group name="Fields"><xs:choice/>`, diagnosticSchemaGlobalDuplicateCode, schemaGlobalDuplicateSpecRef(languageUseVersion(policy)), []string{`<xs:group name="Fields"><xs:choice><xs:element`}, errSchemaGlobalDeclarationDuplicate},
			} {
				t.Run(string(policy)+"/"+name+"/"+test.name, func(t *testing.T) {
					root := languageUseRoot(directGroupLanguageOwner(test.group, directGroupLanguageUse(name)) + test.declaration)
					failed, err := parseIDUseSchema(t, root, nil, policy)
					assertZeroSchema(t, failed)
					d := requireDiagnostic(t, err)
					var related []Loc
					for _, marker := range test.related {
						related = append(related, elementReferenceTestAttributeLoc(t, root, marker))
					}
					if d.Class() != FailureInvalid || d.Code() != test.code || d.Loc() != elementReferenceTestAttributeLoc(t, root, test.marker) || d.SpecRef() != test.spec || !reflect.DeepEqual(d.Related(), related) {
						t.Fatalf("%s = %v; loc=%s want=%s spec=%q want=%q related=%v", test.name, err, d.Loc(), elementReferenceTestAttributeLoc(t, root, test.marker), d.SpecRef(), test.spec, d.Related())
					}
					if test.cause != nil && !errors.Is(err, test.cause) {
						t.Fatalf("%s lost cause: %v", test.name, err)
					}
				})
			}
		}
	}
}

func TestDirectGroupLanguageAnyURIInvisibleImportedTarget(t *testing.T) {
	foreign := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:other"><xs:group name="Fields"><xs:choice><xs:element ref="o:value"/></xs:choice></xs:group><xs:element name="value" type="xs:integer"/></xs:schema>`
	bridge := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`
	fixtures := map[string]discoveryFixture{"bridge.xsd": {id: "bridge.xsd", contents: bridge}, "other.xsd": {id: "other.xsd", contents: foreign}}
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, name := range []string{"language", "anyURI"} {
			t.Run(string(policy)+"/"+name, func(t *testing.T) {
				root := languageUseRoot(`<xs:include schemaLocation="bridge.xsd"/>` + directGroupLanguageOwner(`<xs:group ref="o:Fields" minOccurs="0" maxOccurs="0"/>`, directGroupLanguageUse(name)))
				root = strings.Replace(root, `xmlns:r="urn:root"`, `xmlns:r="urn:root" xmlns:o="urn:other"`, 1)
				failed, err := parseIDUseSchema(t, root, fixtures, policy)
				assertZeroSchema(t, failed)
				d := requireDiagnostic(t, err)
				wantRelated := []Loc{complexContentTestSourceLoc(t, "other.xsd", foreign, `<xs:group name="Fields"`)}
				if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaModelGroupReferenceNamespaceCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `ref="o:Fields"`) || d.SpecRef() != schemaModelGroupReferenceImportSpecRef(languageUseVersion(policy)) || !reflect.DeepEqual(d.Related(), wantRelated) || !errors.Is(err, errSchemaModelGroupReferenceNamespace) {
					t.Fatalf("invisible group = %v; related = %v", err, d.Related())
				}
			})
		}
	}
}

//nolint:gocognit // Type cycles and hidden types have distinct, located resolution exits.
func TestDirectGroupLanguageAnyURITypeGraphExits(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, name := range []string{"language", "anyURI"} {
			t.Run(string(policy)+"/"+name+"/cycle", func(t *testing.T) {
				root := directGroupLanguageRoot(`<xs:group ref="r:Fields"/>`, directGroupLanguageUse(name)+`<xs:attribute name="cycle" type="r:First"/>`, `<xs:simpleType name="First"><xs:restriction base="r:Second"/></xs:simpleType><xs:simpleType name="Second"><xs:restriction base="r:First"/></xs:simpleType>`)
				failed, err := parseIDUseSchema(t, root, nil, policy)
				assertZeroSchema(t, failed)
				d := requireDiagnostic(t, err)
				related := []Loc{
					elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="cycle"`),
					elementReferenceTestAttributeLoc(t, root, `base="r:Second"`),
					elementReferenceTestAttributeLoc(t, root, `base="r:First"`),
					elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="First"`),
					elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="Second"`),
				}
				if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaAttributeTypeCycleCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `type="r:First"`) || d.SpecRef() != schemaAttributeTypeSpecRef(languageUseVersion(policy)) || !reflect.DeepEqual(d.Related(), related) || !errors.Is(err, errSchemaSimpleTypeBaseCycle) {
					t.Fatalf("type cycle = %v; related = %v", err, d.Related())
				}
			})
			for _, relation := range []string{"unimported", "indirect-import"} {
				t.Run(string(policy)+"/"+name+"/hidden-"+relation, func(t *testing.T) {
					root, fixtures := typeVisibilityTestHiddenGraph(t, string(languageUseVersion(policy)), relation, false, true, false, "")
					owner := directGroupLanguageOwner(`<xs:group ref="r:Fields"/>`, directGroupLanguageUse(name)+`<xs:attribute name="hidden" type="f:Hidden" use="prohibited"/>`)
					root = strings.Replace(root, `<xs:element name="item" type="f:Hidden"/>`, owner+directGroupChoice, 1)
					root = strings.Replace(root, `xmlns:f="urn:foreign"`, `xmlns:r="urn:root" xmlns:f="urn:foreign"`, 1)
					failed, err := parseIDUseSchema(t, root, fixtures, policy)
					assertZeroSchema(t, failed)
					d := requireDiagnostic(t, err)
					if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaAttributeTypeUnresolvedCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `type="f:Hidden"`) || d.SpecRef() != schemaAttributeTypeSpecRef(languageUseVersion(policy)) || len(d.Related()) != 0 || !errors.Is(err, errSchemaAttributeTypeUnresolved) {
						t.Fatalf("hidden type = %v; related = %v", err, d.Related())
					}
				})
			}
		}
	}
}

func TestDirectGroupLanguageAnyURIOwnerShapesStayExcluded(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, name := range []string{"language", "anyURI"} {
			attribute := directGroupLanguageUse(name)
			for _, test := range []struct {
				name, body, extra, marker, spec string
			}{
				{"anonymous direct group", `<xs:element name="root"><xs:complexType><xs:group ref="r:Fields"/>` + attribute + `</xs:complexType></xs:element>`, directGroupChoice, `ref="r:Fields"`, "xsd10-structures#schema-document"},
				{"richer group target", directGroupLanguageOwner(`<xs:group ref="r:Fields"/>`, attribute), `<xs:group name="Fields"><xs:all/></xs:group>`, `<xs:all`, directGroupUnsupportedSyntaxSpecRef(languageUseVersion(policy))},
				{"richer recursive target", directGroupLanguageOwner(`<xs:group ref="r:Fields"/>`, attribute), `<xs:group name="Fields"><xs:choice><xs:choice><xs:group ref="r:Fields"/></xs:choice></xs:choice></xs:group>`, `ref="r:Fields"/></xs:choice></xs:choice>`, directGroupUnsupportedSyntaxSpecRef(languageUseVersion(policy))},
			} {
				t.Run(string(policy)+"/"+name+"/"+test.name, func(t *testing.T) {
					root := languageUseRoot(test.body + test.extra)
					root = strings.Replace(root, `attributeFormDefault="qualified"`, `attributeFormDefault="qualified" version="`+string(languageUseVersion(policy))+`"`, 1)
					failed, err := parseIDUseSchema(t, root, nil, policy)
					assertZeroSchema(t, failed)
					d := requireDiagnostic(t, err)
					if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, test.marker) || d.SpecRef() != test.spec || len(d.Related()) != 0 || !errors.Is(err, ErrUnsupported) {
						t.Fatalf("%s = %v; loc=%s want=%s spec=%q want=%q related=%v", test.name, err, d.Loc(), elementReferenceTestAttributeLoc(t, root, test.marker), d.SpecRef(), test.spec, d.Related())
					}
				})
			}
		}
	}
}

func directGroupUnsupportedSyntaxSpecRef(version XSDVersion) string {
	if version == XSDVersion10 {
		return "xsd10-structures#schema-document"
	}
	return "xsd11-structures#cSchemaDocument"
}
