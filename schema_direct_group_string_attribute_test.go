package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func directGroupStringTypes(kind string) string {
	whiteSpace := ""
	if kind == "normalizedString" {
		whiteSpace = `<xs:whiteSpace value="collapse"/>`
	}
	return `<xs:simpleType name="Child"><xs:restriction base="r:Parent"><xs:enumeration value="one"/></xs:restriction></xs:simpleType>` +
		`<xs:simpleType name="Parent"><xs:restriction base="xs:` + kind + `">` + whiteSpace +
		`<xs:enumeration value="one"/><xs:enumeration value="two"/></xs:restriction></xs:simpleType>`
}

//nolint:gocognit // Each graph arrangement checks both opaque group and named datatype provenance.
func TestDirectGroupStringAttributeGraphProvenance(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"NCName", "normalizedString"} {
			for _, target := range []string{"choice", "sequence"} {
				for _, graph := range []string{"forward", "include", "import", "chameleon"} {
					t.Run(profile.name+"/"+kind+"/"+target+"/"+graph, func(t *testing.T) {
						lexical, namespace, source := "r", "urn:root", SourceID("root.xsd")
						directive := ""
						if graph == "include" || graph == "chameleon" {
							directive, source = `<xs:include schemaLocation="part.xsd"/>`, "part.xsd"
						}
						if graph == "import" {
							directive, lexical, namespace, source = `<xs:import namespace="urn:other" schemaLocation="part.xsd"/>`, "o", "urn:other", "part.xsd"
						}
						owner := `<xs:complexType name="Record"><xs:group ref="` + lexical + `:Fields" minOccurs="1" maxOccurs="3"/><xs:attribute name="named" type="` + lexical + `:Child" form="unqualified"/></xs:complexType>`
						termRef := lexical + ":value"
						if graph == "chameleon" {
							termRef = "value"
						}
						defs := `<xs:group name="Fields"><xs:` + target + `><xs:element ref="` + termRef + `"/></xs:` + target + `></xs:group><xs:element name="value" type="xs:integer"/><xs:simpleType name="Child"><xs:restriction base="xs:` + kind + `"><xs:enumeration value="one"/></xs:restriction></xs:simpleType>`
						root := languageUseRoot(directive + owner)
						root = strings.Replace(root, `xmlns:r="urn:root"`, `xmlns:r="urn:root" xmlns:o="urn:other"`, 1)
						fixtures := map[string]discoveryFixture{}
						contents := root
						if graph == "forward" {
							root = strings.Replace(root, `</xs:schema>`, defs+`</xs:schema>`, 1)
							contents = root
						}
						if graph != "forward" {
							namespaceAttribute := ` targetNamespace="` + namespace + `"`
							if graph == "chameleon" {
								namespaceAttribute = ""
							}
							contents = `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other"` + namespaceAttribute + `>` + defs + `</xs:schema>`
							fixtures["part.xsd"] = discoveryFixture{id: source, contents: contents}
						}
						schema, err := parseIDUseSchema(t, root, fixtures, profile.policy)
						if err != nil {
							t.Fatal(err)
						}
						documents := schema.Documents()
						wantDocuments := 2
						if graph == "forward" {
							wantDocuments = 1
						}
						if len(documents) != wantDocuments || documents[0].Source() != "root.xsd" || documents[0].RootLoc() != elementReferenceTestAttributeLoc(t, root, `<xs:schema`) {
							t.Fatalf("root document provenance = %v", documents)
						}
						if graph != "forward" && (documents[1].Source() != source || documents[1].RootLoc() != complexContentTestSourceLoc(t, source, contents, `<xs:schema`)) {
							t.Fatalf("referenced document provenance = %v", documents)
						}
						ownerDefinition := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Record")
						group, ok := ownerDefinition.Particle().(ModelGroupReferenceParticle)
						groups := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, namespace, "Fields"))
						if !ok || len(groups) != 1 || group.TargetID() != groups[0].ID() || group.TargetID().Source() != source || group.Ref() != mustTestQName(t, namespace, "Fields") || group.Occurrences().String() != "1/3" || group.RefLoc() != elementReferenceTestAttributeLoc(t, root, `ref="`+lexical+`:Fields"`) || groups[0].Loc() != complexContentTestSourceLoc(t, source, contents, `<xs:group name="Fields"`) {
							t.Fatalf("group provenance = %v/%v", ownerDefinition.Particle(), groups)
						}
						uses := ownerDefinition.AttributeUses()
						use, ok := uses[0].(LocalAttributeUse)
						if len(uses) != 1 || !ok || use.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="named"`) || use.FormLoc() != elementReferenceTestAttributeLoc(t, root, `form="unqualified"`) || use.TypeLoc() != elementReferenceTestAttributeLoc(t, root, `type="`+lexical+`:Child"`) {
							t.Fatalf("use provenance = %v", uses)
						}
						ref, hasRef := use.TypeReference()
						id, hasID := use.TypeID()
						types := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, namespace, "Child"))
						if !hasRef || !hasID || len(types) != 1 || id != types[0].ID() || id.Source() != source || ref.Name() != mustTestQName(t, namespace, "Child") || ref.QName() != use.DeclaredType() || ref.Loc() != use.TypeLoc() || types[0].Loc() != complexContentTestSourceLoc(t, source, contents, `<xs:simpleType name="Child"`) {
							t.Fatalf("type provenance = %v/%v", ref, types)
						}
						assertStringEnumerationFacts(t, ref.StringEnumerationFacets(), profile.version, []string{"one"}, []Loc{complexContentTestSourceLoc(t, source, contents, `value="one"`)})
						var walked []ComponentID
						var walkedNames []string
						if walkErr := schema.Walk(func(component Component) error {
							walked = append(walked, component.ID())
							walkedNames = append(walkedNames, string(component.ID().Source())+":"+component.Name().Local())
							return nil
						}); walkErr != nil {
							t.Fatal(walkErr)
						}
						wantWalk := []string{"root.xsd:Record", string(source) + ":Fields", string(source) + ":value", string(source) + ":Child"}
						if !reflect.DeepEqual(walked, componentIDs(schema.Components())) || !reflect.DeepEqual(walkedNames, wantWalk) {
							t.Fatalf("walk order = %v", walkedNames)
						}
					})
				}
			}
		}
	}
}

func TestDirectGroupStringAttributeResolverCause(t *testing.T) {
	root := languageUseRoot(`<xs:include schemaLocation="missing.xsd"/><xs:complexType name="Record"><xs:group ref="r:Fields"/><xs:attribute name="a" type="xs:NCName"/></xs:complexType>`)
	source, err := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("direct-group datatype source unavailable")
	failed, failure := ParseSchemaWithPolicy(source, &discoveryResolver{failures: map[string]error{"missing.xsd": cause}}, Strict11)
	assertNCNameDiagnostic(t, failed, failure, FailureResolution, SourceResolveCode, elementReferenceTestAttributeLoc(t, root, `<xs:include`), nil, "", cause)
}

func directGroupStringRoot(kind, target, group, uses string) string {
	return directGroupTokenRoot(target, group, uses, directGroupStringTypes(kind))
}

//nolint:gocognit // The matrix checks each newly admitted shape through the immutable public model.
func TestDirectGroupStringAttributePublicFacts(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, target := range []string{"choice", "sequence"} {
			for _, kind := range []string{"NCName", "normalizedString"} {
				for _, omitted := range []bool{false, true} {
					t.Run(profile.name+"/"+target+"/"+kind+"/"+map[bool]string{false: "present", true: "omitted"}[omitted], func(t *testing.T) {
						group := `<xs:group ref="r:Fields" minOccurs="2" maxOccurs="5"/>`
						if omitted {
							group = `<xs:group ref="r:Fields" minOccurs="0" maxOccurs="0"/>`
						}
						uses := `<xs:attribute name="before" type="xs:boolean"/><xs:attribute name="named" type="r:Child" use="required" form="unqualified"/><xs:attribute name="built" type="xs:` + kind + `"/><xs:attribute name="gone" type="r:Child" use="prohibited"/><xs:attribute name="after" type="xs:integer"/>`
						root := directGroupStringRoot(kind, target, group, uses)
						label := "1.0"
						if profile.policy == Strict10 {
							label = "1.1"
						}
						root = strings.Replace(root, `attributeFormDefault="qualified"`, `attributeFormDefault="qualified" version="`+label+`"`, 1)
						schema, err := parseIDUseSchema(t, root, nil, profile.policy)
						if err != nil {
							t.Fatal(err)
						}
						if schema.LanguagePolicy() != profile.policy || len(schema.Documents()) != 1 || schema.Documents()[0].Source() != "root.xsd" || schema.Documents()[0].RootLoc() != elementReferenceTestAttributeLoc(t, root, `<xs:schema`) || !reflect.DeepEqual(componentIDs(schema.Documents()[0].Components()), componentIDs(schema.Components())) {
							t.Fatalf("policy/documents = %s/%v", schema.LanguagePolicy(), schema.Documents())
						}
						owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Record")
						if owner.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Record"`) {
							t.Fatalf("owner Loc = %s", owner.Loc())
						}
						groups := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, "urn:root", "Fields"))
						if len(groups) != 1 || groups[0].Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:group name="Fields"`) {
							t.Fatalf("group definition = %v", groups)
						}
						if omitted {
							if owner.Particle() != nil {
								t.Fatalf("0/0 particle = %T", owner.Particle())
							}
						}
						if !omitted {
							particle, ok := owner.Particle().(ModelGroupReferenceParticle)
							if !ok || particle.Ref() != mustTestQName(t, "urn:root", "Fields") || particle.TargetID() != groups[0].ID() || particle.Occurrences().String() != "2/5" || particle.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:group ref="r:Fields"`) || particle.RefLoc() != elementReferenceTestAttributeLoc(t, root, `ref="r:Fields"`) {
								t.Fatalf("particle = %#v", owner.Particle())
							}
						}
						ordered := owner.AttributeUses()
						if len(ordered) != 4 || ordered[0].Name().Local() != "before" || ordered[1].Name().Local() != "named" || ordered[2].Name().Local() != "built" || ordered[3].Name().Local() != "after" {
							t.Fatalf("lexical use order = %v", ordered)
						}
						named, ok := ordered[1].(LocalAttributeUse)
						if !ok || named.Name() != mustTestQName(t, "", "named") || named.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="named"`) || named.NameLoc() != elementReferenceTestAttributeLoc(t, root, `name="named"`) || named.TypeLoc() != elementReferenceTestAttributeLoc(t, root, `type="r:Child"`) || named.UseLoc() != elementReferenceTestAttributeLoc(t, root, `use="required"`) || named.FormLoc() != elementReferenceTestAttributeLoc(t, root, `form="unqualified"`) || named.Use() != AttributeUseRequired {
							t.Fatalf("named use = %#v", ordered[1])
						}
						ref, hasRef := named.TypeReference()
						id, hasID := named.TypeID()
						refID, hasRefID := ref.ComponentID()
						child := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:root", "Child"))
						if !hasRef || !hasID || !hasRefID || len(child) != 1 || ref.Kind() != SimpleTypeReferenceNamed || ref.Variety() != SimpleTypeVarietyAtomicRestriction || ref.Name() != mustTestQName(t, "urn:root", "Child") || ref.QName() != named.DeclaredType() || ref.Loc() != named.TypeLoc() || ref.VarietyLoc() != elementReferenceTestAttributeLoc(t, root, `<xs:restriction base="r:Parent"`) || id != refID || id != child[0].ID() || child[0].Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="Child"`) {
							t.Fatalf("named type = %v/%v", ref, id)
						}
						assertStringEnumerationFacts(t, ref.StringEnumerationFacets(), profile.version, []string{"one"}, []Loc{elementReferenceTestAttributeLoc(t, root, `value="one"`)})
						white, hasWhite := ref.StringWhiteSpaceFacet()
						wantWhite, wantFixed, wantWhiteLoc := "collapse", true, Loc{}
						if kind == "normalizedString" {
							wantFixed, wantWhiteLoc = false, elementReferenceTestAttributeLoc(t, root, `value="collapse"`)
						}
						if !hasWhite || white.Value() != wantWhite || white.Fixed() != wantFixed || white.Loc() != wantWhiteLoc {
							t.Fatalf("named whiteSpace = %v", white)
						}
						built, ok := ordered[2].(LocalAttributeUse)
						builtRef, hasBuiltRef := built.TypeReference()
						builtID, hasBuiltID := built.TypeID()
						wantBuilt := mustTestQName(t, testXSDNamespace, kind)
						if !ok || !hasBuiltRef || !builtRef.IsBuiltin() || builtRef.Kind() != SimpleTypeReferenceBuiltin || builtRef.Name() != wantBuilt || built.DeclaredType() != wantBuilt || built.TypeLoc() != elementReferenceTestAttributeLoc(t, root, `type="xs:`+kind+`"`) || hasBuiltID || !builtID.IsZero() {
							t.Fatalf("built-in type = %v/%v", built, builtRef)
						}
						builtWhite, hasBuiltWhite := builtRef.StringWhiteSpaceFacet()
						if !hasBuiltWhite || !builtWhite.Loc().IsZero() || builtWhite.Value() != map[string]string{"NCName": "collapse", "normalizedString": "replace"}[kind] || builtWhite.Fixed() != (kind == "NCName") {
							t.Fatalf("built-in whiteSpace = %v", builtWhite)
						}
						assertDirectGroupStringPublicCopiesAndOrder(t, root, schema, owner, ref, ordered, profile.policy)
					})
				}
			}
		}
	}
}

func assertDirectGroupStringPublicCopiesAndOrder(t *testing.T, root string, schema Schema, owner ComplexTypeDefinition, ref SimpleTypeReference, ordered []AttributeUse, policy LanguagePolicy) {
	t.Helper()
	values, locations := ref.StringEnumerationFacets().Values(), ref.StringEnumerationFacets().Locations()
	values[0], locations[0], ordered[1] = "mutated", Loc{}, nil
	components := schema.Components()
	components[0] = Component{}
	fresh, ok := owner.AttributeUses()[1].(LocalAttributeUse)
	if !ok {
		t.Fatal("fresh use is not local")
	}
	freshRef, _ := fresh.TypeReference()
	if freshRef.StringEnumerationFacets().Values()[0] != "one" || freshRef.StringEnumerationFacets().Locations()[0].IsZero() || schema.Components()[0].Name().IsZero() {
		t.Fatal("caller mutated completed facts")
	}
	var walks [2][]ComponentID
	for index := range walks {
		if walkErr := schema.Walk(func(component Component) error { walks[index] = append(walks[index], component.ID()); return nil }); walkErr != nil {
			t.Fatal(walkErr)
		}
	}
	if !reflect.DeepEqual(walks[0], walks[1]) || !reflect.DeepEqual(walks[0], componentIDs(schema.Components())) {
		t.Fatalf("walks = %v/%v", walks[0], walks[1])
	}
	again, repeatErr := parseIDUseSchema(t, root, nil, policy)
	if repeatErr != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
		t.Fatalf("repeat = %v", repeatErr)
	}
}

//nolint:gocognit // Every newly admitted family and group target has separate consumer exits.
func TestDirectGroupStringAttributeConsumers(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, target := range []string{"choice", "sequence"} {
			for _, kind := range []string{"NCName", "normalizedString"} {
				for _, useType := range []string{"builtin", "named"} {
					for _, omitted := range []bool{false, true} {
						t.Run(profile.name+"/"+target+"/"+kind+"/"+useType+"/"+map[bool]string{false: "present", true: "omitted"}[omitted], func(t *testing.T) {
							group := `<xs:group ref="r:Fields"/>`
							if omitted {
								group = `<xs:group ref="r:Fields" minOccurs="0" maxOccurs="0"/>`
							}
							typeName := "xs:" + kind
							if useType == "named" {
								typeName = "r:Child"
							}
							root := directGroupStringRoot(kind, target, group, `<xs:attribute name="selected" type="`+typeName+`"/>`)
							root = strings.Replace(root, `</xs:schema>`, `<xs:element name="root" type="r:Record"/></xs:schema>`, 1)
							schema, err := parseIDUseSchema(t, root, nil, profile.policy)
							if err != nil {
								t.Fatal(err)
							}
							useLoc := elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="selected"`)
							ownerLoc := elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Record"`)
							rootLoc := elementReferenceTestAttributeLoc(t, root, `<xs:element name="root"`)
							instanceErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<r:root xmlns:r="urn:root"/>`)))
							vd := requireDiagnostic(t, instanceErr)
							if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != useLoc || vd.SpecRef() != instanceValidationSpecRef(profile.version) || !reflect.DeepEqual(vd.Related(), []Loc{rootLoc, ownerLoc, useLoc}) || !errors.Is(instanceErr, ErrUnsupported) || !errors.Is(instanceErr, errInstanceAttributes) {
								t.Fatalf("validation = %v", instanceErr)
							}
							output, generationErr := GenerateGo(schema, "generated")
							gd := requireDiagnostic(t, generationErr)
							if output != nil || gd.Class() != FailureUnsupported || gd.Code() != diagnosticCodegenUnsupported || gd.Loc() != useLoc || gd.SpecRef() != codegenDirectSequenceSpecReference(profile.version, codegenDirectSequenceParticlesReference) || !reflect.DeepEqual(gd.Related(), []Loc{ownerLoc, useLoc}) || !errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenUnsupported) {
								t.Fatalf("generation = %v/%v", output, generationErr)
							}
						})
					}
				}
			}
		}
	}
}

func TestDirectGroupStringAttributeExcludedForms(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"NCName", "normalizedString"} {
			for _, test := range []struct {
				name, use, extra, marker, related string
				spec                              string
				cause                             error
			}{
				{"named list", `<xs:attribute name="a" type="r:Alias"/>`, `<xs:simpleType name="Alias"><xs:list itemType="xs:` + kind + `"/></xs:simpleType>`, `type="r:Alias"`, "", schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeUseUnsupported},
				{"named union", `<xs:attribute name="a" type="r:Alias"/>`, `<xs:simpleType name="Alias"><xs:union memberTypes="xs:` + kind + ` xs:` + kind + `"/></xs:simpleType>`, `type="r:Alias"`, "", schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeUseUnsupported},
				{"inline atomic", `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:` + kind + `"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, "", schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnsupported},
				{"inline list", `<xs:attribute name="a"><xs:simpleType><xs:list itemType="xs:` + kind + `"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, "", schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnsupported},
				{"inline union", `<xs:attribute name="a"><xs:simpleType><xs:union memberTypes="xs:` + kind + ` xs:` + kind + `"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, "", schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnsupported},
				{"ref built-in", `<xs:attribute ref="r:a"/>`, `<xs:attribute name="a" type="xs:` + kind + `"/>`, `ref="r:a"`, `<xs:attribute name="a"`, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeReferenceUnsupported},
				{"ref named", `<xs:attribute ref="r:a"/>`, `<xs:attribute name="a" type="r:Alias"/><xs:simpleType name="Alias"><xs:restriction base="xs:` + kind + `"/></xs:simpleType>`, `ref="r:a"`, `<xs:attribute name="a"`, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeReferenceUnsupported},
				{"ref inline", `<xs:attribute ref="r:a"/>`, `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:` + kind + `"/></xs:simpleType></xs:attribute>`, `ref="r:a"`, `<xs:attribute name="a"`, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeReferenceUnsupported},
				{"default built-in", `<xs:attribute name="a" type="xs:` + kind + `" default="one"/>`, "", `default="one"`, "", schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseUnsupported},
				{"fixed built-in", `<xs:attribute name="a" type="xs:` + kind + `" fixed="one"/>`, "", `fixed="one"`, "", schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseUnsupported},
				{"default named", `<xs:attribute name="a" type="r:Alias" default="one"/>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:` + kind + `"/></xs:simpleType>`, `default="one"`, "", schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseUnsupported},
				{"fixed named", `<xs:attribute name="a" type="r:Alias" fixed="one"/>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:` + kind + `"/></xs:simpleType>`, `fixed="one"`, "", schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseUnsupported},
				{"default inline", `<xs:attribute name="a" default="one"><xs:simpleType><xs:restriction base="xs:` + kind + `"/></xs:simpleType></xs:attribute>`, "", `default="one"`, "", schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseUnsupported},
				{"fixed inline", `<xs:attribute name="a" fixed="one"><xs:simpleType><xs:restriction base="xs:` + kind + `"/></xs:simpleType></xs:attribute>`, "", `fixed="one"`, "", schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseUnsupported},
				{"default ref", `<xs:attribute ref="r:a" default="one"/>`, `<xs:attribute name="a" type="xs:` + kind + `"/>`, `default="one"`, "", schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseUnsupported},
				{"fixed ref", `<xs:attribute ref="r:a" fixed="one"/>`, `<xs:attribute name="a" type="xs:` + kind + `"/>`, `fixed="one"`, "", schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseUnsupported},
				{"default ref named", `<xs:attribute ref="r:a" default="one"/>`, `<xs:attribute name="a" type="r:Alias"/><xs:simpleType name="Alias"><xs:restriction base="xs:` + kind + `"/></xs:simpleType>`, `default="one"`, "", schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseUnsupported},
				{"fixed ref named", `<xs:attribute ref="r:a" fixed="one"/>`, `<xs:attribute name="a" type="r:Alias"/><xs:simpleType name="Alias"><xs:restriction base="xs:` + kind + `"/></xs:simpleType>`, `fixed="one"`, "", schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseUnsupported},
				{"default ref inline", `<xs:attribute ref="r:a" default="one"/>`, `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:` + kind + `"/></xs:simpleType></xs:attribute>`, `default="one"`, "", schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseUnsupported},
				{"fixed ref inline", `<xs:attribute ref="r:a" fixed="one"/>`, `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:` + kind + `"/></xs:simpleType></xs:attribute>`, `fixed="one"`, "", schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseUnsupported},
			} {
				t.Run(profile.name+"/"+kind+"/"+test.name, func(t *testing.T) {
					root := directGroupTokenRoot("choice", `<xs:group ref="r:Fields"/>`, test.use, test.extra)
					failed, failure := parseIDUseSchema(t, root, nil, profile.policy)
					var related []Loc
					if test.related != "" {
						related = []Loc{elementReferenceTestAttributeLoc(t, root, test.related)}
					}
					assertNCNameDiagnostic(t, failed, failure, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, test.marker), related, test.spec, test.cause)
				})
			}
		}
	}
}

func TestDirectGroupStringAttributeOwnerAndFamilyLimits(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"NCName", "normalizedString"} {
			for _, target := range []string{"choice", "sequence"} {
				t.Run(profile.name+"/"+kind+"/"+target+"/anonymous owner", func(t *testing.T) {
					root := languageUseRoot(`<xs:element name="root"><xs:complexType><xs:group ref="r:Fields"/><xs:attribute name="a" type="xs:` + kind + `"/></xs:complexType></xs:element><xs:group name="Fields"><xs:` + target + `/></xs:group>`)
					failed, failure := parseIDUseSchema(t, root, nil, profile.policy)
					assertNCNameDiagnostic(t, failed, failure, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, `ref="r:Fields"`), nil, "xsd10-structures#schema-document", ErrUnsupported)
				})
			}
			t.Run(profile.name+"/"+kind+"/simple content owner", func(t *testing.T) {
				root := languageUseRoot(`<xs:complexType name="Record"><xs:simpleContent><xs:extension base="xs:string"><xs:attribute name="a" type="xs:` + kind + `"/></xs:extension></xs:simpleContent></xs:complexType>`)
				failed, failure := parseIDUseSchema(t, root, nil, profile.policy)
				assertNCNameDiagnostic(t, failed, failure, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, `type="xs:`+kind+`"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeUseUnsupported)
			})
			t.Run(profile.name+"/"+kind+"/nested direct group owner", func(t *testing.T) {
				root := languageUseRoot(`<xs:complexType name="Record"><xs:sequence><xs:group ref="r:Fields"/></xs:sequence><xs:attribute name="a" type="xs:` + kind + `"/></xs:complexType>` + directGroupChoice)
				failed, failure := parseIDUseSchema(t, root, nil, profile.policy)
				assertNCNameDiagnostic(t, failed, failure, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, `ref="r:Fields"`), nil, "xsd10-structures#schema-document", ErrUnsupported)
			})
		}
		t.Run(profile.name+"/ID sibling", func(t *testing.T) {
			root := directGroupTokenRoot("choice", `<xs:group ref="r:Fields"/>`, `<xs:attribute name="a" type="xs:ID"/>`, "")
			failed, failure := parseIDUseSchema(t, root, nil, profile.policy)
			assertNCNameDiagnostic(t, failed, failure, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, `type="xs:ID"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeUseUnsupported)
		})
	}
}

//nolint:gocognit // Resolution, syntax, and facet exits must win over both omission paths.
func TestDirectGroupStringAttributeOmissionAndBoundaryDiagnostics(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"NCName", "normalizedString"} {
			for _, target := range []string{"choice", "sequence"} {
				t.Run(profile.name+"/"+kind+"/"+target, func(t *testing.T) {
					root := directGroupStringRoot(kind, target, `<xs:group ref="r:Fields" minOccurs="0" maxOccurs="0"/>`, `<xs:attribute name="gone" type="r:Child" use="prohibited"/>`)
					schema, err := parseIDUseSchema(t, root, nil, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Record")
					if owner.Particle() != nil || len(owner.AttributeUses()) != 0 {
						t.Fatalf("omitted shape = %v/%v", owner.Particle(), owner.AttributeUses())
					}
					for _, test := range []struct {
						name, from, to, marker, related, code, spec string
						cause                                       error
					}{
						{"missing group", `ref="r:Fields"`, `ref="r:Missing"`, `ref="r:Missing"`, "", diagnosticSchemaModelGroupReferenceUnresolvedCode, schemaModelGroupReferenceSpecRef(profile.version), errSchemaModelGroupReferenceUnresolved},
						{"wrong group kind", `ref="r:Fields"`, `ref="r:value"`, `ref="r:value"`, `<xs:element name="value"`, diagnosticSchemaModelGroupReferenceWrongKindCode, schemaModelGroupReferenceSpecRef(profile.version), errSchemaModelGroupReferenceWrongKind},
						{"missing prohibited type", `type="r:Child"`, `type="r:Missing"`, `type="r:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved},
						{"wrong prohibited type kind", `type="r:Child"`, `type="r:Fields"`, `type="r:Fields"`, `<xs:group name="Fields"`, diagnosticSchemaAttributeTypeWrongKindCode, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeWrongKind},
						{"reversed bounds", `minOccurs="0" maxOccurs="0"`, `minOccurs="2" maxOccurs="1"`, `<xs:group ref="r:Fields"`, `minOccurs="2"`, invalidSchemaCompositionCode, schemaParticleCorrectSpecRef(profile.version), errParticleOccurrenceMinimumExceedsMaximum},
						{"invalid narrowing", `<xs:enumeration value="one"/></xs:restriction></xs:simpleType>`, `<xs:enumeration value="other"/></xs:restriction></xs:simpleType>`, `value="other"`, `value="one"`, InvalidEnumerationRestrictionCode, enumerationSpecRef(profile.version, enumerationRestrictionRule), errInvalidEnumerationRestriction},
					} {
						t.Run(test.name, func(t *testing.T) {
							changed := strings.Replace(root, test.from, test.to, 1)
							failed, failure := parseIDUseSchema(t, changed, nil, profile.policy)
							var related []Loc
							if test.related != "" {
								related = []Loc{elementReferenceTestAttributeLoc(t, changed, test.related)}
							}
							if test.name == "invalid narrowing" {
								related = append(related, elementReferenceTestAttributeLoc(t, changed, `value="two"`))
							}
							if test.name == "reversed bounds" {
								related = append(related, elementReferenceTestAttributeLoc(t, changed, `maxOccurs="1"`))
							}
							assertNCNameDiagnostic(t, failed, failure, FailureInvalid, test.code, elementReferenceTestAttributeLoc(t, changed, test.marker), related, test.spec, test.cause)
						})
					}
				})
			}
		}
	}
}

//nolint:gocognit // Alternate target and declaration failures retain their own code, location, and cause.
func TestDirectGroupStringAttributeGraphFailures(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"NCName", "normalizedString"} {
			for _, test := range []struct {
				name, group, extra, marker, related, code, spec string
				cause                                           error
			}{
				{"missing group", `<xs:group ref="r:Missing"/>`, "", `ref="r:Missing"`, "", diagnosticSchemaModelGroupReferenceUnresolvedCode, schemaModelGroupReferenceSpecRef(profile.version), errSchemaModelGroupReferenceUnresolved},
				{"wrong group kind", `<xs:group ref="r:Wrong"/>`, `<xs:element name="Wrong" type="xs:integer"/>`, `ref="r:Wrong"`, `<xs:element name="Wrong"`, diagnosticSchemaModelGroupReferenceWrongKindCode, schemaModelGroupReferenceSpecRef(profile.version), errSchemaModelGroupReferenceWrongKind},
				{"group cycle", `<xs:group ref="r:Fields"/>`, `<xs:group name="Fields"><xs:choice><xs:group ref="r:Fields"/></xs:choice></xs:group>`, `ref="r:Fields"/></xs:choice>`, `<xs:group name="Fields"`, diagnosticSchemaModelGroupReferenceCycleCode, directGroupCycleSpecRef(profile.version), errSchemaModelGroupReferenceCycle},
				{"missing type", `<xs:group ref="r:Fields"/>`, directGroupChoice, `type="r:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved},
				{"wrong type kind", `<xs:group ref="r:Fields"/>`, directGroupChoice, `type="r:Fields"`, `<xs:group name="Fields"`, diagnosticSchemaAttributeTypeWrongKindCode, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeWrongKind},
			} {
				t.Run(profile.name+"/"+kind+"/"+test.name, func(t *testing.T) {
					typeName := "xs:" + kind
					if test.name == "missing type" {
						typeName = "r:Missing"
					}
					if test.name == "wrong type kind" {
						typeName = "r:Fields"
					}
					root := languageUseRoot(`<xs:complexType name="Record">` + test.group + `<xs:attribute name="a" type="` + typeName + `"/></xs:complexType>` + test.extra)
					failed, failure := parseIDUseSchema(t, root, nil, profile.policy)
					var related []Loc
					if test.related != "" {
						related = []Loc{elementReferenceTestAttributeLoc(t, root, test.related)}
					}
					assertNCNameDiagnostic(t, failed, failure, FailureInvalid, test.code, elementReferenceTestAttributeLoc(t, root, test.marker), related, test.spec, test.cause)
				})
			}
			t.Run(profile.name+"/"+kind+"/duplicate group", func(t *testing.T) {
				root := directGroupTokenRoot("choice", `<xs:group ref="r:Fields"/>`, `<xs:attribute name="a" type="xs:`+kind+`"/>`, `<xs:group name="Fields"><xs:choice/></xs:group>`)
				failed, failure := parseIDUseSchema(t, root, nil, profile.policy)
				assertNCNameDiagnostic(t, failed, failure, FailureInvalid, diagnosticSchemaGlobalDuplicateCode, elementReferenceTestAttributeLoc(t, root, `<xs:group name="Fields"><xs:choice/>`), []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:group name="Fields"><xs:choice><xs:element`)}, schemaGlobalDuplicateSpecRef(profile.version), errSchemaGlobalDeclarationDuplicate)
			})
			t.Run(profile.name+"/"+kind+"/type cycle", func(t *testing.T) {
				root := directGroupTokenRoot("choice", `<xs:group ref="r:Fields"/>`, `<xs:attribute name="a" type="r:First"/>`, `<xs:simpleType name="First"><xs:restriction base="r:Second"/></xs:simpleType><xs:simpleType name="Second"><xs:restriction base="r:First"/></xs:simpleType>`)
				failed, failure := parseIDUseSchema(t, root, nil, profile.policy)
				related := []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="a"`), elementReferenceTestAttributeLoc(t, root, `base="r:Second"`), elementReferenceTestAttributeLoc(t, root, `base="r:First"`), elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="First"`), elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="Second"`)}
				assertNCNameDiagnostic(t, failed, failure, FailureInvalid, diagnosticSchemaAttributeTypeCycleCode, elementReferenceTestAttributeLoc(t, root, `type="r:First"`), related, schemaAttributeTypeSpecRef(profile.version), errSchemaSimpleTypeBaseCycle)
			})
		}
	}
}

func TestDirectGroupStringAttributeInvisibleTargets(t *testing.T) {
	foreign := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:other"><xs:group name="Fields"><xs:choice><xs:element ref="o:value"/></xs:choice></xs:group><xs:element name="value" type="xs:integer"/><xs:simpleType name="Hidden"><xs:restriction base="xs:NCName"/></xs:simpleType></xs:schema>`
	bridge := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`
	fixtures := map[string]discoveryFixture{"bridge.xsd": {id: "bridge.xsd", contents: bridge}, "other.xsd": {id: "other.xsd", contents: foreign}}
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"NCName", "normalizedString"} {
			t.Run(profile.name+"/"+kind+"/group", func(t *testing.T) {
				root := languageUseRoot(`<xs:include schemaLocation="bridge.xsd"/><xs:complexType name="Record"><xs:group ref="o:Fields" minOccurs="0" maxOccurs="0"/><xs:attribute name="a" type="xs:` + kind + `"/></xs:complexType>`)
				root = strings.Replace(root, `xmlns:r="urn:root"`, `xmlns:r="urn:root" xmlns:o="urn:other"`, 1)
				failed, failure := parseIDUseSchema(t, root, fixtures, profile.policy)
				assertNCNameDiagnostic(t, failed, failure, FailureInvalid, diagnosticSchemaModelGroupReferenceNamespaceCode, elementReferenceTestAttributeLoc(t, root, `ref="o:Fields"`), []Loc{complexContentTestSourceLoc(t, "other.xsd", foreign, `<xs:group name="Fields"`)}, schemaModelGroupReferenceImportSpecRef(profile.version), errSchemaModelGroupReferenceNamespace)
			})
			t.Run(profile.name+"/"+kind+"/type", func(t *testing.T) {
				root := languageUseRoot(`<xs:include schemaLocation="bridge.xsd"/><xs:complexType name="Record"><xs:group ref="r:Fields" minOccurs="0" maxOccurs="0"/><xs:attribute name="a" type="o:Hidden" use="prohibited"/></xs:complexType>` + directGroupChoice)
				root = strings.Replace(root, `xmlns:r="urn:root"`, `xmlns:r="urn:root" xmlns:o="urn:other"`, 1)
				failed, failure := parseIDUseSchema(t, root, fixtures, profile.policy)
				assertNCNameDiagnostic(t, failed, failure, FailureInvalid, diagnosticSchemaAttributeTypeUnresolvedCode, elementReferenceTestAttributeLoc(t, root, `type="o:Hidden"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved)
			})
		}
	}
}

//nolint:gocognit // Each alternate datatype and syntax exit remains explicit at this owner boundary.
func TestDirectGroupStringAttributeFacetSyntaxAndFinalExits(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"NCName", "normalizedString"} {
			for _, facet := range []struct{ name, value string }{{"pattern", ".*"}, {"length", "2"}} {
				for _, shape := range []string{"named", "inline", "ref"} {
					t.Run(profile.name+"/"+kind+"/"+facet.name+"/"+shape, func(t *testing.T) {
						faceted := `<xs:simpleType name="Alias"><xs:restriction base="xs:` + kind + `"><xs:` + facet.name + ` value="` + facet.value + `"/></xs:restriction></xs:simpleType>`
						use, extra := `<xs:attribute name="a" type="r:Alias"/>`, faceted
						if shape == "inline" {
							use, extra = `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:`+kind+`"><xs:`+facet.name+` value="`+facet.value+`"/></xs:restriction></xs:simpleType></xs:attribute>`, ""
						}
						if shape == "ref" {
							use, extra = `<xs:attribute ref="r:a"/>`, `<xs:attribute name="a" type="r:Alias"/>`+faceted
						}
						root := directGroupTokenRoot("choice", `<xs:group ref="r:Fields"/>`, use, extra)
						failed, failure := parseIDUseSchema(t, root, nil, profile.policy)
						assertNCNameDiagnostic(t, failed, failure, FailureUnsupported, UnsupportedDatatypeFacetCode, elementReferenceTestAttributeLoc(t, root, `<xs:`+facet.name), nil, tokenDiagnosticSpecRef(profile.version, "decimal"), ErrUnsupported)
					})
				}
			}
			t.Run(profile.name+"/"+kind+"/final", func(t *testing.T) {
				root := directGroupStringRoot(kind, "choice", `<xs:group ref="r:Fields"/>`, `<xs:attribute name="a" type="r:Child"/>`)
				root = strings.Replace(root, `<xs:simpleType name="Parent">`, `<xs:simpleType name="Parent" final="restriction">`, 1)
				failed, failure := parseIDUseSchema(t, root, nil, profile.policy)
				assertNCNameDiagnostic(t, failed, failure, FailureInvalid, diagnosticSchemaSimpleTypeBaseCode, elementReferenceTestAttributeLoc(t, root, `base="r:Parent"`), []Loc{elementReferenceTestAttributeLoc(t, root, `final="restriction"`)}, schemaSimpleTypeRestrictionSpecRef(profile.version), errSchemaSimpleTypeInvalidDerivation)
			})
			if kind == "normalizedString" {
				t.Run(profile.name+"/"+kind+"/whiteSpace widening", func(t *testing.T) {
					root := directGroupStringRoot(kind, "choice", `<xs:group ref="r:Fields"/>`, `<xs:attribute name="a" type="r:Child"/>`)
					root = strings.Replace(root, `value="collapse"`, `value="preserve"`, 1)
					failed, failure := parseIDUseSchema(t, root, nil, profile.policy)
					assertNCNameDiagnostic(t, failed, failure, FailureInvalid, InvalidStringWhiteSpaceRestrictionCode, elementReferenceTestAttributeLoc(t, root, `value="preserve"`), nil, stringWhiteSpaceSpecRef(profile.version), errInvalidStringWhiteSpaceRestriction)
				})
			}
			for _, malformed := range []struct {
				name, group, use, marker, spec string
			}{
				{"bad name", `<xs:group ref="r:Fields"/>`, `<xs:attribute name="a:b" type="xs:` + kind + `"/>`, `name="a:b"`, schemaAttributeUseSpecRef(profile.version)},
				{"bad use", `<xs:group ref="r:Fields"/>`, `<xs:attribute name="a" type="xs:` + kind + `" use="sometimes"/>`, `use="sometimes"`, schemaAttributeUseSpecRef(profile.version)},
				{"bad form", `<xs:group ref="r:Fields"/>`, `<xs:attribute name="a" type="xs:` + kind + `" form="local"/>`, `form="local"`, schemaAttributeUseSpecRef(profile.version)},
				{"bad occurrence", `<xs:group ref="r:Fields" minOccurs="many"/>`, `<xs:attribute name="a" type="xs:` + kind + `"/>`, `minOccurs="many"`, schemaParticleOccurrenceDatatypeSpecRef(profile.version)},
			} {
				t.Run(profile.name+"/"+kind+"/"+malformed.name, func(t *testing.T) {
					root := directGroupTokenRoot("choice", malformed.group, malformed.use, "")
					failed, failure := parseIDUseSchema(t, root, nil, profile.policy)
					assertZeroSchema(t, failed)
					d := requireDiagnostic(t, failure)
					if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, malformed.marker) || d.SpecRef() != malformed.spec || len(d.Related()) != 0 || errors.Is(failure, ErrUnsupported) {
						t.Fatalf("malformed %s diagnostic = %v", malformed.name, failure)
					}
					if malformed.name == "bad occurrence" && errors.Unwrap(failure) == nil {
						t.Fatalf("malformed occurrence lost lexical cause: %v", failure)
					}
					if malformed.name == "bad occurrence" {
						var lexical Diagnostic
						if !errors.As(errors.Unwrap(failure), &lexical) || lexical.Code() != InvalidIntegerLexicalCode || lexical.Loc() != d.Loc() {
							t.Fatalf("malformed occurrence lexical cause = %v", errors.Unwrap(failure))
						}
					}
				})
			}
			t.Run(profile.name+"/"+kind+"/duplicate use", func(t *testing.T) {
				root := directGroupTokenRoot("choice", `<xs:group ref="r:Fields"/>`, `<xs:attribute name="a" type="xs:`+kind+`" use="optional"/><xs:attribute name="a" type="xs:`+kind+`"/>`, "")
				failed, failure := parseIDUseSchema(t, root, nil, profile.policy)
				first := elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="a" type="xs:`+kind+`" use="optional"/>`)
				second := elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="a" type="xs:`+kind+`"/>`)
				assertNCNameDiagnostic(t, failed, failure, FailureInvalid, diagnosticSchemaAttributeUseDuplicateCode, second, []Loc{first}, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseDuplicate)
			})
		}
	}
}

//nolint:gocognit // Every direct group syntax exit is checked before either omission path under each policy.
func TestDirectGroupStringAttributeGroupSyntaxDiagnostics(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"NCName", "normalizedString"} {
			for _, omitted := range []bool{false, true} {
				for _, test := range []struct {
					name, group, marker, code string
				}{
					{"malformed QName", `<xs:group@ ref="r::Fields"/>`, `ref="r::Fields"`, invalidSchemaConditionalCode},
					{"empty QName", `<xs:group@ ref=""/>`, `ref=""`, invalidSchemaCompositionCode},
					{"unbound QName", `<xs:group@ ref="u:Fields"/>`, `ref="u:Fields"`, invalidSchemaConditionalCode},
					{"missing ref", `<xs:group@/>`, `<xs:group`, invalidSchemaCompositionCode},
					{"forbidden attribute", `<xs:group@ ref="r:Fields" name="bad"/>`, `name="bad"`, invalidSchemaCompositionCode},
					{"forbidden qualified attribute", `<xs:group@ ref="r:Fields" xs:bogus="bad"/>`, `xs:bogus="bad"`, invalidSchemaCompositionCode},
					{"invalid id", `<xs:group@ ref="r:Fields" id="bad:name"/>`, `id="bad:name"`, invalidSchemaCompositionCode},
					{"forbidden child", `<xs:group@ ref="r:Fields"><xs:element ref="r:value"/></xs:group>`, `<xs:element ref="r:value"`, invalidSchemaCompositionCode},
					{"nonwhitespace text", `<xs:group@ ref="r:Fields">bad</xs:group>`, `bad</xs:group>`, invalidSchemaCompositionCode},
					{"duplicate annotation", `<xs:group@ ref="r:Fields"><xs:annotation><xs:documentation>first</xs:documentation></xs:annotation><xs:annotation/></xs:group>`, `<xs:annotation/></xs:group>`, invalidSchemaCompositionCode},
				} {
					occurrence := ""
					if omitted {
						occurrence = ` minOccurs="0" maxOccurs="0"`
					}
					for _, target := range []string{"choice", "sequence"} {
						t.Run(profile.name+"/"+kind+"/"+target+"/"+map[bool]string{false: "present", true: "omitted"}[omitted]+"/"+test.name, func(t *testing.T) {
							group := strings.Replace(test.group, "@", occurrence, 1)
							root := directGroupTokenRoot(target, group, `<xs:attribute name="a" type="xs:`+kind+`"/>`, "")
							failed, failure := parseIDUseSchema(t, root, nil, profile.policy)
							assertZeroSchema(t, failed)
							diagnostic := requireDiagnostic(t, failure)
							wantLoc := elementReferenceTestAttributeLoc(t, root, test.marker)
							if diagnostic.Class() != FailureInvalid || diagnostic.Code() != test.code || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != schemaGroupParticleSpecRef(profile.version) || len(diagnostic.Related()) != 0 || errors.Unwrap(failure) != nil || errors.Is(failure, ErrUnsupported) {
								t.Fatalf("group syntax diagnostic = %v; Loc=%s related=%v SpecRef=%q cause=%v", failure, diagnostic.Loc(), diagnostic.Related(), diagnostic.SpecRef(), errors.Unwrap(failure))
							}
						})
					}
				}
			}
		}
	}
}

func assertDirectGroupStringOwnerInvalid(t *testing.T, failed Schema, failure error, wantLoc Loc, wantSpec string) {
	t.Helper()
	assertZeroSchema(t, failed)
	diagnostic := requireDiagnostic(t, failure)
	if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.Loc() != wantLoc || len(diagnostic.Related()) != 0 || diagnostic.SpecRef() != wantSpec || errors.Unwrap(failure) != nil || errors.Is(failure, ErrUnsupported) {
		t.Fatalf("owner diagnostic = %v; Loc=%s related=%v SpecRef=%q cause=%v", failure, diagnostic.Loc(), diagnostic.Related(), diagnostic.SpecRef(), errors.Unwrap(failure))
	}
}

//nolint:gocognit // Cover owner grammar exits before and after direct group validation under each policy.
func TestDirectGroupStringAttributeOwnerGrammarDiagnostics(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		wantSpec := "xsd11-structures#element-complexType"
		if profile.version == XSDVersion10 {
			wantSpec = "xsd10-structures#element-complexType"
		}
		for _, kind := range []string{"NCName", "normalizedString"} {
			for _, target := range []string{"choice", "sequence"} {
				for _, omitted := range []bool{false, true} {
					occurrence := ""
					bounds := "present"
					if omitted {
						occurrence = ` minOccurs="0" maxOccurs="0"`
						bounds = "omitted"
					}
					group := `<xs:group ref="r:Fields"` + occurrence + `/>`
					second := `<xs:group ref="r:Fields" id="second"` + occurrence + `/>`
					use := `<xs:attribute name="a" type="xs:` + kind + `"/>`
					choice := `<xs:choice><xs:element ref="r:value"/></xs:choice>`
					sequence := `<xs:sequence><xs:element ref="r:value"/></xs:sequence>`
					all := `<xs:all><xs:element ref="r:value"/></xs:all>`
					for _, test := range []struct {
						name, body, marker string
					}{
						{"late group", use + group, `<xs:group ref="r:Fields"`},
						{"duplicate group", group + second + use, `<xs:group ref="r:Fields" id="second"`},
						{"group then choice", group + choice + use, `<xs:choice>`},
						{"group then sequence", group + sequence + use, `<xs:sequence>`},
						{"group then all", group + all + use, `<xs:all>`},
						{"choice then group", choice + group + use, `<xs:group ref="r:Fields"`},
						{"sequence then group", sequence + group + use, `<xs:group ref="r:Fields"`},
						{"all then group", all + group + use, `<xs:group ref="r:Fields"`},
						{"simpleContent then group", `<xs:simpleContent><xs:extension base="xs:string"/></xs:simpleContent>` + group + use, `<xs:group ref="r:Fields"`},
						{"complexContent then group", `<xs:complexContent><xs:extension base="xs:anyType"/></xs:complexContent>` + group + use, `<xs:group ref="r:Fields"`},
						{"anyAttribute then group", `<xs:anyAttribute namespace="##other"/>` + group + use, `<xs:group ref="r:Fields"`},
						{"assert then group", `<xs:assert test="true()"/>` + group + use, `<xs:group ref="r:Fields"`},
						{"group then simpleContent", group + `<xs:simpleContent/>` + use, `<xs:simpleContent`},
						{"group then complexContent", group + `<xs:complexContent/>` + use, `<xs:complexContent`},
						{"group then openContent", group + `<xs:openContent mode="none"/>` + use, `<xs:openContent`},
						{"group then annotation", group + `<xs:annotation/>` + use, `<xs:annotation`},
						{"group then forbidden child", group + `<xs:element name="extra" type="xs:string"/>` + use, `<xs:element name="extra"`},
						{"group then text", group + `bad` + use, `bad<xs:attribute`},
						{"group then foreign child", group + `<f:extra xmlns:f="urn:foreign"/>` + use, `<f:extra`},
						{"group then wildcard then attribute", group + `<xs:anyAttribute namespace="##other"/>` + use, `<xs:attribute name="a"`},
						{"group then duplicate wildcard", group + `<xs:anyAttribute namespace="##other"/><xs:anyAttribute namespace="##local"/>` + use, `<xs:anyAttribute namespace="##local"`},
						{"group then assert then attribute", group + `<xs:assert test="true()"/>` + use, `<xs:attribute name="a"`},
					} {
						t.Run(profile.name+"/"+kind+"/"+target+"/"+bounds+"/"+test.name, func(t *testing.T) {
							root := directGroupTokenRoot(target, test.body, "", "")
							failed, failure := parseIDUseSchema(t, root, nil, profile.policy)
							assertDirectGroupStringOwnerInvalid(t, failed, failure, elementReferenceTestAttributeLoc(t, root, test.marker), wantSpec)
						})
					}
				}
			}
		}
	}
}

//nolint:gocognit // Each owner attribute exit is checked under both group bounds and all policies.
func TestDirectGroupStringAttributeOwnerAttributeDiagnostics(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		wantSpec := "xsd11-structures#element-complexType"
		if profile.version == XSDVersion10 {
			wantSpec = "xsd10-structures#element-complexType"
		}
		for _, kind := range []string{"NCName", "normalizedString"} {
			for _, omitted := range []bool{false, true} {
				group := `<xs:group ref="r:Fields"/>`
				bounds := "present"
				if omitted {
					group = `<xs:group ref="r:Fields" minOccurs="0" maxOccurs="0"/>`
					bounds = "omitted"
				}
				for _, test := range []struct{ name, attribute string }{
					{"invalid mixed", `mixed="maybe"`},
					{"invalid final", `final="bad"`},
					{"invalid id", `id="bad:name"`},
				} {
					t.Run(profile.name+"/"+kind+"/"+bounds+"/"+test.name, func(t *testing.T) {
						root := directGroupTokenRoot("choice", group, `<xs:attribute name="a" type="xs:`+kind+`"/>`, "")
						root = strings.Replace(root, `<xs:complexType name="Record"`, `<xs:complexType name="Record" `+test.attribute, 1)
						failed, failure := parseIDUseSchema(t, root, nil, profile.policy)
						assertDirectGroupStringOwnerInvalid(t, failed, failure, elementReferenceTestAttributeLoc(t, root, test.attribute), wantSpec)
					})
				}
			}
		}
	}
}

//nolint:gocognit // Generic owner-level unsupported exits need the selected owner reference without changing their class or feature.
func TestDirectGroupStringAttributeOwnerUnsupportedReferences(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		wantSpec := "xsd11-structures#element-complexType"
		if profile.version == XSDVersion10 {
			wantSpec = "xsd10-structures#element-complexType"
		}
		for _, kind := range []string{"NCName", "normalizedString"} {
			for _, target := range []string{"choice", "sequence"} {
				for _, omitted := range []bool{false, true} {
					group := `<xs:group ref="r:Fields"/>`
					bounds := "present"
					if omitted {
						group = `<xs:group ref="r:Fields" minOccurs="0" maxOccurs="0"/>`
						bounds = "omitted"
					}
					use := `<xs:attribute name="a" type="xs:` + kind + `"/>`
					for _, test := range []struct {
						name, child, ownerAttribute, extra, marker string
					}{
						{"unknown child", `<xs:future/>`, "", "", `<xs:future`},
						{"attributeGroup reference", `<xs:attributeGroup ref="r:Attrs"/>`, "", `<xs:attributeGroup name="Attrs"/>`, `<xs:attributeGroup ref="r:Attrs"`},
						{"mixed true", "", `mixed="true"`, "", `mixed="true"`},
					} {
						t.Run(profile.name+"/"+kind+"/"+target+"/"+bounds+"/"+test.name, func(t *testing.T) {
							root := directGroupTokenRoot(target, group+test.child, use, test.extra)
							if test.ownerAttribute != "" {
								root = strings.Replace(root, `<xs:complexType name="Record"`, `<xs:complexType name="Record" `+test.ownerAttribute, 1)
							}
							failed, failure := parseIDUseSchema(t, root, nil, profile.policy)
							assertZeroSchema(t, failed)
							diagnostic := requireDiagnostic(t, failure)
							if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, test.marker) || len(diagnostic.Related()) != 0 || diagnostic.SpecRef() != wantSpec || errors.Unwrap(failure) != nil || !errors.Is(failure, ErrUnsupported) {
								t.Fatalf("owner unsupported = %v; Loc=%s related=%v SpecRef=%q cause=%v", failure, diagnostic.Loc(), diagnostic.Related(), diagnostic.SpecRef(), errors.Unwrap(failure))
							}
						})
					}
				}
			}
		}
	}
}

//nolint:gocognit // Narrow fallback must leave existing group, use, policy, and fixed feature references intact.
func TestDirectGroupStringAttributeSpecificReferencesPreserved(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"NCName", "normalizedString"} {
			for _, omitted := range []bool{false, true} {
				group := `<xs:group ref="r:Fields"/>`
				bounds := "present"
				if omitted {
					group = `<xs:group ref="r:Fields" minOccurs="0" maxOccurs="0"/>`
					bounds = "omitted"
				}
				badOccurrenceGroup := `<xs:group ref="r:Fields" minOccurs="bad"/>`
				if omitted {
					badOccurrenceGroup = `<xs:group ref="r:Fields" minOccurs="bad" maxOccurs="0"/>`
				}
				use := `<xs:attribute name="a" type="xs:` + kind + `"/>`
				for _, test := range []struct {
					name, group, use, ownerAttribute, marker, code, spec string
					class                                                FailureClass
					feature                                              FeatureID
					cause                                                error
					lexical                                              bool
				}{
					{"group QName", strings.Replace(group, `ref="r:Fields"`, `ref="u:Fields"`, 1), use, "", `ref="u:Fields"`, invalidSchemaConditionalCode, schemaGroupParticleSpecRef(profile.version), FailureInvalid, "", nil, false},
					{"group occurrence", badOccurrenceGroup, use, "", `minOccurs="bad"`, invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(profile.version), FailureInvalid, "", nil, true},
					{"local use", group, strings.Replace(use, `/>`, ` use="bad"/>`, 1), "", `use="bad"`, invalidSchemaCompositionCode, schemaAttributeUseSpecRef(profile.version), FailureInvalid, "", nil, false},
					{"nested local child", group, strings.Replace(use, `/>`, `><xs:future/></xs:attribute>`, 1), "", `<xs:future`, UnsupportedSchemaSyntaxCode, "xsd10-structures#schema-document", FailureUnsupported, FeatureSchemaSyntax, ErrUnsupported, false},
					{"block", group, use, `block="bad"`, `block="bad"`, diagnosticSchemaBlockCode, schemaBlockSpecRef(profile.version, schemaBlockComplex), FailureInvalid, "", errSchemaBlock, false},
					{"xml base", group, use, `xml:base="urn:base"`, `xml:base="urn:base"`, UnsupportedSchemaSyntaxCode, "xmlbase#matching", FailureUnsupported, featureSchemaXMLBase, ErrUnsupported, false},
				} {
					t.Run(profile.name+"/"+kind+"/"+bounds+"/"+test.name, func(t *testing.T) {
						root := directGroupTokenRoot("choice", test.group, test.use, "")
						if test.ownerAttribute != "" {
							root = strings.Replace(root, `<xs:complexType name="Record"`, `<xs:complexType name="Record" `+test.ownerAttribute, 1)
						}
						failed, failure := parseIDUseSchema(t, root, nil, profile.policy)
						assertZeroSchema(t, failed)
						diagnostic := requireDiagnostic(t, failure)
						if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Feature() != test.feature || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, test.marker) || len(diagnostic.Related()) != 0 || diagnostic.SpecRef() != test.spec || test.cause != nil && !errors.Is(failure, test.cause) {
							t.Fatalf("specific diagnostic = %v; Loc=%s related=%v SpecRef=%q cause=%v", failure, diagnostic.Loc(), diagnostic.Related(), diagnostic.SpecRef(), errors.Unwrap(failure))
						}
						if test.lexical {
							var lexical Diagnostic
							if !errors.As(errors.Unwrap(failure), &lexical) || lexical.Code() != InvalidIntegerLexicalCode || lexical.Loc() != diagnostic.Loc() {
								t.Fatalf("occurrence lexical cause = %v", errors.Unwrap(failure))
							}
						}
						if !test.lexical && (test.cause == nil || test.class == FailureUnsupported) && errors.Unwrap(failure) != nil {
							t.Fatalf("specific diagnostic gained cause = %v", errors.Unwrap(failure))
						}
					})
				}
			}
		}
	}
}

//nolint:gocognit // The selected policy controls local-attribute syntax independently of schema/@version.
func TestDirectGroupStringAttributePolicyAndExactBounds(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"NCName", "normalizedString"} {
			t.Run(profile.name+"/"+kind+"/large unbounded", func(t *testing.T) {
				root := directGroupTokenRoot("choice", `<xs:group ref="r:Fields" minOccurs="18446744073709551616" maxOccurs="unbounded"/>`, `<xs:attribute name="a" type="xs:`+kind+`"/>`, "")
				schema, err := parseIDUseSchema(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Record")
				group, ok := owner.Particle().(ModelGroupReferenceParticle)
				if !ok || group.Occurrences().String() != "18446744073709551616/unbounded" || group.RefLoc() != elementReferenceTestAttributeLoc(t, root, `ref="r:Fields"`) || len(owner.AttributeUses()) != 1 {
					t.Fatalf("exact group range = %v/%v", owner.Particle(), owner.AttributeUses())
				}
			})
			for _, spelling := range []string{"false", "0", "true", "1"} {
				t.Run(profile.name+"/"+kind+"/inheritable="+spelling, func(t *testing.T) {
					use := `<xs:attribute name="a" type="xs:` + kind + `" inheritable="` + spelling + `"/>`
					root := directGroupTokenRoot("choice", `<xs:group ref="r:Fields"/>`, use, "")
					schema, err := parseIDUseSchema(t, root, nil, profile.policy)
					if profile.policy == Strict10 {
						assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, `inheritable="`+spelling+`"`), nil, "xsd11-structures#cSchemaDocument", errLanguagePolicyMismatch)
						return
					}
					if spelling == "true" || spelling == "1" {
						assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, `inheritable="`+spelling+`"`), nil, "xsd11-structures#cSchemaDocument", ErrUnsupported)
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Record")
					if len(owner.AttributeUses()) != 1 || owner.AttributeUses()[0].Name().Local() != "a" {
						t.Fatalf("omitted false/0 inheritable = %v", owner.AttributeUses())
					}
				})
			}
		}
	}
}

func TestDirectGroupStringAttributeInvalidPolicyPrecedesAdmission(t *testing.T) {
	root := directGroupStringRoot("NCName", "choice", `<xs:group ref="r:Fields" minOccurs="0" maxOccurs="0"/>`, `<xs:attribute name="gone" type="r:Child" use="prohibited"/>`)
	source, err := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
	if err != nil {
		t.Fatal(err)
	}
	failed, failure := ParseSchemaWithPolicy(source, nil, LanguagePolicy("invalid"))
	assertNCNameDiagnostic(t, failed, failure, FailureInvalid, InvalidLanguagePolicyCode, Loc{}, nil, "", errInvalidLanguagePolicy)
}
