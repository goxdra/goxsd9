package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func directGroupNamedTokenTypes(kind string) string {
	return `<xs:simpleType name="Child"><xs:restriction base="r:Parent"><xs:enumeration value="two"/><xs:enumeration value="three"/></xs:restriction></xs:simpleType>` +
		`<xs:simpleType name="Parent"><xs:restriction base="xs:` + kind + `"><xs:whiteSpace value="collapse"/><xs:enumeration value="one"/><xs:enumeration value="two"/><xs:enumeration value="three"/></xs:restriction></xs:simpleType>`
}

func directGroupNamedTokenRoot(target, group, uses, kind string) string {
	return directGroupTokenRoot(target, group, uses, directGroupNamedTokenTypes(kind))
}

//nolint:gocognit // The matrix checks one immutable public fact across policy and group shapes.
func TestDirectGroupNamedTokenPublicFacts(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, target := range []string{"choice", "sequence"} {
			for _, kind := range []string{"token", "NMTOKEN"} {
				t.Run(profile.name+"/"+target+"/"+kind, func(t *testing.T) {
					uses := `<xs:attribute name="before" type="xs:boolean"/><xs:attribute name="named" type="r:Child" use="required" form="unqualified"/><xs:attribute name="built" type="xs:` + kind + `"/><xs:attribute name="gone" type="r:Child" use="prohibited"/><xs:attribute name="after" type="xs:integer"/>`
					root := directGroupNamedTokenRoot(target, `<xs:group ref="r:Fields" minOccurs="2" maxOccurs="5"/>`, uses, kind)
					schema, err := parseIDUseSchema(t, root, nil, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					if schema.LanguagePolicy() != profile.policy || len(schema.Documents()) != 1 || schema.Documents()[0].Source() != "root.xsd" {
						t.Fatalf("policy/documents = %v/%v", schema.LanguagePolicy(), schema.Documents())
					}
					owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Record")
					group, ok := owner.Particle().(ModelGroupReferenceParticle)
					groups := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, "urn:root", "Fields"))
					if owner.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Record"`) || !ok || len(groups) != 1 || group.Ref() != mustTestQName(t, "urn:root", "Fields") || group.TargetID() != groups[0].ID() || group.Occurrences().String() != "2/5" || group.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:group ref="r:Fields"`) || group.RefLoc() != elementReferenceTestAttributeLoc(t, root, `ref="r:Fields"`) || groups[0].Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:group name="Fields"`) {
						t.Fatalf("owner/group = %v/%v", owner, group)
					}
					usesView := owner.AttributeUses()
					if len(usesView) != 4 || usesView[0].Name().Local() != "before" || usesView[1].Name().Local() != "named" || usesView[2].Name().Local() != "built" || usesView[3].Name().Local() != "after" {
						t.Fatalf("ordered uses = %v", usesView)
					}
					use, ok := usesView[1].(LocalAttributeUse)
					if !ok || use.Name() != mustTestQName(t, "", "named") || use.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="named"`) || use.NameLoc() != elementReferenceTestAttributeLoc(t, root, `name="named"`) || use.TypeLoc() != elementReferenceTestAttributeLoc(t, root, `type="r:Child"`) || use.UseLoc() != elementReferenceTestAttributeLoc(t, root, `use="required"`) || use.FormLoc() != elementReferenceTestAttributeLoc(t, root, `form="unqualified"`) || use.Use() != AttributeUseRequired || use.DeclaredType() != mustTestQName(t, "urn:root", "Child") {
						t.Fatalf("named use = %v", usesView[1])
					}
					ref, hasRef := use.TypeReference()
					id, hasID := use.TypeID()
					refID, hasRefID := ref.ComponentID()
					child := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:root", "Child"))
					if !hasRef || !hasID || !hasRefID || len(child) != 1 || ref.Kind() != SimpleTypeReferenceNamed || ref.Variety() != SimpleTypeVarietyAtomicRestriction || ref.Name() != mustTestQName(t, "urn:root", "Child") || ref.QName() != use.DeclaredType() || ref.Loc() != use.TypeLoc() || ref.VarietyLoc() != elementReferenceTestAttributeLoc(t, root, `<xs:restriction base="r:Parent"`) || id != refID || id != child[0].ID() || child[0].Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="Child"`) {
						t.Fatalf("named type = %v/%v", ref, id)
					}
					white, hasWhite := ref.StringWhiteSpaceFacet()
					if !hasWhite || white.Value() != "collapse" || !white.Fixed() || white.Loc() != elementReferenceTestAttributeLoc(t, root, `value="collapse"`) {
						t.Fatalf("effective whitespace = %v", white)
					}
					assertStringEnumerationFacts(t, ref.StringEnumerationFacets(), profile.version, []string{"two", "three"}, []Loc{elementReferenceTestAttributeLoc(t, root, `value="two"`), elementReferenceTestAttributeLoc(t, root, `value="three"`)})
					built, ok := usesView[2].(LocalAttributeUse)
					builtRef, hasBuiltRef := built.TypeReference()
					if !ok || !hasBuiltRef || !builtRef.IsBuiltin() || builtRef.Name() != mustTestQName(t, testXSDNamespace, kind) || built.DeclaredType() != builtRef.Name() {
						t.Fatalf("built-in control = %v/%v", built, builtRef)
					}
					values, locations := ref.StringEnumerationFacets().Values(), ref.StringEnumerationFacets().Locations()
					values[0], locations[0], usesView[1] = "changed", Loc{}, nil
					components := schema.Components()
					components[0] = Component{}
					fresh, ok := owner.AttributeUses()[1].(LocalAttributeUse)
					if !ok {
						t.Fatal("fresh use is not local")
					}
					freshRef, _ := fresh.TypeReference()
					if freshRef.StringEnumerationFacets().Values()[0] != "two" || freshRef.StringEnumerationFacets().Locations()[0].IsZero() || schema.Components()[0].Name().IsZero() {
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
					again, repeatErr := parseIDUseSchema(t, root, nil, profile.policy)
					if repeatErr != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
						t.Fatalf("repeat = %v", repeatErr)
					}
				})
			}
		}
	}
}

//nolint:gocognit // Each selected owner and datatype gets both consumer diagnostics.
func TestDirectGroupNamedTokenConsumers(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, target := range []string{"choice", "sequence"} {
			for _, kind := range []string{"token", "NMTOKEN"} {
				t.Run(profile.name+"/"+target+"/"+kind, func(t *testing.T) {
					root := directGroupNamedTokenRoot(target, `<xs:group ref="r:Fields"/>`, `<xs:attribute name="named" type="r:Child"/>`, kind)
					root = strings.Replace(root, `</xs:schema>`, `<xs:element name="root" type="r:Record"/></xs:schema>`, 1)
					schema, err := parseIDUseSchema(t, root, nil, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					useLoc := elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="named"`)
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

//nolint:gocognit // Each source arrangement proves named type identity and source provenance.
func TestDirectGroupNamedTokenTypeGraphs(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			for _, graph := range []string{"forward", "include", "import", "chameleon"} {
				t.Run(profile.name+"/"+kind+"/"+graph, func(t *testing.T) {
					lexical, namespace, source := "r:Child", "urn:root", SourceID("root.xsd")
					declaration := `<xs:simpleType name="Child"><xs:restriction base="xs:` + kind + `"><xs:enumeration value="two"/></xs:restriction></xs:simpleType>`
					var directive string
					fixtures := map[string]discoveryFixture{}
					if graph == "include" || graph == "chameleon" {
						directive = `<xs:include schemaLocation="types.xsd"/>`
						source = "types.xsd"
					}
					if graph == "import" {
						directive = `<xs:import namespace="urn:other" schemaLocation="types.xsd"/>`
						lexical, namespace, source = "o:Child", "urn:other", "types.xsd"
					}
					root := languageUseRoot(directive + directGroupLanguageOwner(`<xs:group ref="r:Fields"/>`, `<xs:attribute name="named" type="`+lexical+`"/>`) + directGroupChoice)
					root = strings.Replace(root, `xmlns:r="urn:root"`, `xmlns:r="urn:root" xmlns:o="urn:other"`, 1)
					contents := root
					if graph == "forward" {
						root = strings.Replace(root, `</xs:schema>`, declaration+`</xs:schema>`, 1)
						contents = root
					}
					if graph != "forward" {
						namespaceAttribute := ` targetNamespace="` + namespace + `"`
						if graph == "chameleon" {
							namespaceAttribute = ""
						}
						contents = `<xs:schema xmlns:xs="` + testXSDNamespace + `"` + namespaceAttribute + `>` + declaration + `</xs:schema>`
						fixtures["types.xsd"] = discoveryFixture{id: source, contents: contents}
					}
					schema, err := parseIDUseSchema(t, root, fixtures, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Record")
					use, ok := owner.AttributeUses()[0].(LocalAttributeUse)
					if !ok {
						t.Fatal("use is not local")
					}
					ref, ok := use.TypeReference()
					id, hasID := ref.ComponentID()
					qname := mustTestQName(t, namespace, "Child")
					components := schema.FindKind(ComponentKindSimpleTypeDefinition, qname)
					if !ok || !hasID || len(components) != 1 || ref.Name() != qname || ref.QName() != qname || ref.Loc() != elementReferenceTestAttributeLoc(t, root, `type="`+lexical+`"`) || id != components[0].ID() || id.Source() != source || components[0].Loc() != complexContentTestSourceLoc(t, source, contents, `<xs:simpleType name="Child"`) {
						t.Fatalf("named type provenance = %v/%v", ref, components)
					}
					assertStringEnumerationFacts(t, ref.StringEnumerationFacets(), profile.version, []string{"two"}, []Loc{complexContentTestSourceLoc(t, source, contents, `value="two"`)})
				})
			}
		}
	}
}

//nolint:gocognit // Resolution precedes both 0/0 and prohibited-use normalization.
func TestDirectGroupNamedTokenOmissionAndDependencyExits(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, target := range []string{"choice", "sequence"} {
			for _, kind := range []string{"token", "NMTOKEN"} {
				t.Run(profile.name+"/"+target+"/"+kind, func(t *testing.T) {
					root := directGroupNamedTokenRoot(target, `<xs:group ref="r:Fields" minOccurs="0" maxOccurs="0"/>`, `<xs:attribute name="named" type="r:Child"/><xs:attribute name="gone" type="r:Child" use="prohibited"/>`, kind)
					schema, err := parseIDUseSchema(t, root, nil, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Record")
					if owner.Particle() != nil || len(owner.AttributeUses()) != 1 || owner.AttributeUses()[0].Name().Local() != "named" {
						t.Fatalf("omitted = %v/%v", owner.Particle(), owner.AttributeUses())
					}
					for _, test := range []struct {
						name, from, to, marker, code, spec string
						cause                              error
					}{
						{"group before omission", `ref="r:Fields"`, `ref="r:Missing"`, `ref="r:Missing"`, diagnosticSchemaModelGroupReferenceUnresolvedCode, schemaModelGroupReferenceSpecRef(profile.version), errSchemaModelGroupReferenceUnresolved},
						{"prohibited type before omission", `name="gone" type="r:Child"`, `name="gone" type="r:Missing"`, `type="r:Missing"`, diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved},
					} {
						t.Run(test.name, func(t *testing.T) {
							changed := strings.Replace(root, test.from, test.to, 1)
							failed, failure := parseIDUseSchema(t, changed, nil, profile.policy)
							assertNCNameDiagnostic(t, failed, failure, FailureInvalid, test.code, elementReferenceTestAttributeLoc(t, changed, test.marker), nil, test.spec, test.cause)
						})
					}
				})
			}
		}
	}
}

func TestDirectGroupNamedTokenReferenceExits(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			root := directGroupNamedTokenRoot("choice", `<xs:group ref="r:Fields"/>`, `<xs:attribute name="named" type="r:Child"/>`, kind)
			for _, test := range []struct {
				name, from, to, marker, related, code, spec string
				cause                                       error
			}{
				{"missing group", `ref="r:Fields"`, `ref="r:Missing"`, `ref="r:Missing"`, "", diagnosticSchemaModelGroupReferenceUnresolvedCode, schemaModelGroupReferenceSpecRef(profile.version), errSchemaModelGroupReferenceUnresolved},
				{"wrong group kind", `ref="r:Fields"`, `ref="r:value"`, `ref="r:value"`, `<xs:element name="value"`, diagnosticSchemaModelGroupReferenceWrongKindCode, schemaModelGroupReferenceSpecRef(profile.version), errSchemaModelGroupReferenceWrongKind},
				{"missing type", `type="r:Child"`, `type="r:Missing"`, `type="r:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved},
				{"wrong type kind", `type="r:Child"`, `type="r:Fields"`, `type="r:Fields"`, `<xs:group name="Fields"`, diagnosticSchemaAttributeTypeWrongKindCode, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeWrongKind},
			} {
				t.Run(profile.name+"/"+kind+"/"+test.name, func(t *testing.T) {
					changed := strings.Replace(root, test.from, test.to, 1)
					failed, failure := parseIDUseSchema(t, changed, nil, profile.policy)
					var related []Loc
					if test.related != "" {
						related = []Loc{elementReferenceTestAttributeLoc(t, changed, test.related)}
					}
					assertNCNameDiagnostic(t, failed, failure, FailureInvalid, test.code, elementReferenceTestAttributeLoc(t, changed, test.marker), related, test.spec, test.cause)
				})
			}
		}
	}
}
