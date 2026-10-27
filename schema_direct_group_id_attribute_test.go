package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func directGroupIDRoot(target, group, uses, extra string) string {
	return directGroupTokenRoot(target, group, uses, extra)
}

func directGroupIDNamedTypes() string {
	return `<xs:simpleType name="Child"><xs:restriction base="r:Parent"/></xs:simpleType><xs:simpleType name="Parent"><xs:restriction base="xs:ID"/></xs:simpleType>`
}

//nolint:gocognit // The public matrix ties opaque group and completed ID-use facts to each policy and target.
func TestDirectGroupIDPublicFactsAndCopies(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, target := range []string{"choice", "sequence"} {
			for _, kind := range []string{"builtin", "named"} {
				t.Run(profile.name+"/"+target+"/"+kind, func(t *testing.T) {
					lexical, extra := "xs:ID", ""
					if kind == "named" {
						lexical, extra = "r:Child", directGroupIDNamedTypes()
					}
					uses := `<xs:attribute name="before" type="xs:boolean"/><xs:attribute name="id" type="` + lexical + `" use="required" form="unqualified"/><xs:attribute name="gone" type="xs:ID" use="prohibited"/><xs:attribute name="after" type="xs:integer"/>`
					root := directGroupIDRoot(target, `<xs:group ref="r:Fields" minOccurs="2" maxOccurs="5"/>`, uses, extra)
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
					if len(usesView) != 3 || usesView[0].Name().Local() != "before" || usesView[1].Name().Local() != "id" || usesView[2].Name().Local() != "after" {
						t.Fatalf("lexical uses = %v", usesView)
					}
					use, ok := usesView[1].(LocalAttributeUse)
					wantQName := mustTestQName(t, testXSDNamespace, "ID")
					if kind == "named" {
						wantQName = mustTestQName(t, "urn:root", "Child")
					}
					if !ok || use.Name() != mustTestQName(t, "", "id") || use.DeclaredType() != wantQName || use.Use() != AttributeUseRequired || use.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="id"`) || use.NameLoc() != elementReferenceTestAttributeLoc(t, root, `name="id"`) || use.TypeLoc() != elementReferenceTestAttributeLoc(t, root, `type="`+lexical+`"`) || use.UseLoc() != elementReferenceTestAttributeLoc(t, root, `use="required"`) || use.FormLoc() != elementReferenceTestAttributeLoc(t, root, `form="unqualified"`) {
						t.Fatalf("ID use = %v", use)
					}
					ref, hasRef := use.TypeReference()
					useID, hasID := use.TypeID()
					refID, hasRefID := ref.ComponentID()
					if !hasRef || ref.Name() != wantQName || ref.QName() != wantQName || ref.Variety() != SimpleTypeVarietyAtomicRestriction || ref.Loc() != use.TypeLoc() || len(ref.StringEnumerationFacets().Values()) != 0 {
						t.Fatalf("ID reference = %v", ref)
					}
					if kind == "builtin" {
						if !ref.IsBuiltin() || hasID || hasRefID || !useID.IsZero() || !refID.IsZero() || ref.VarietyLoc() != use.TypeLoc() {
							t.Fatalf("built-in identity = %v/%v/%v", ref, useID, refID)
						}
					}
					if kind == "named" {
						child := schema.FindKind(ComponentKindSimpleTypeDefinition, wantQName)
						if !ref.IsNamed() || len(child) != 1 || !hasID || !hasRefID || useID != refID || refID != child[0].ID() || refID.Source() != "root.xsd" || child[0].Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="Child"`) || ref.VarietyLoc() != elementReferenceTestAttributeLoc(t, root, `<xs:restriction base="r:Parent"`) {
							t.Fatalf("named identity/provenance = %v/%v/%v", ref, child, useID)
						}
						definition, ok := child[0].SimpleTypeDefinition()
						parentName := mustTestQName(t, "urn:root", "Parent")
						parents := schema.FindKind(ComponentKindSimpleTypeDefinition, parentName)
						baseID, hasBaseID := definition.BaseID()
						baseRef, hasBaseRef := definition.BaseReference()
						if !ok || len(parents) != 1 || definition.Base() != parentName || definition.BaseLoc() != elementReferenceTestAttributeLoc(t, root, `base="r:Parent"`) || !hasBaseID || baseID != parents[0].ID() || !hasBaseRef || !baseRef.IsNamed() || baseRef.Name() != parentName || baseRef.Variety() != SimpleTypeVarietyAtomicRestriction {
							t.Fatalf("completed ID chain = %v", child[0])
						}
					}
					usesView[1] = nil
					components := schema.Components()
					components[0] = Component{}
					if owner.AttributeUses()[1].Name().Local() != "id" || schema.Components()[0].Name().IsZero() {
						t.Fatal("caller changed immutable facts")
					}
					var walks [2][]ComponentID
					for index := range walks {
						if walkErr := schema.Walk(func(component Component) error { walks[index] = append(walks[index], component.ID()); return nil }); walkErr != nil {
							t.Fatal(walkErr)
						}
					}
					if !reflect.DeepEqual(walks[0], walks[1]) || !reflect.DeepEqual(walks[0], componentIDs(schema.Components())) {
						t.Fatalf("walk order = %v/%v", walks[0], walks[1])
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

//nolint:gocognit // Validation and generation have separate located gates for each newly queryable shape.
func TestDirectGroupIDConsumers(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, target := range []string{"choice", "sequence"} {
			for _, kind := range []string{"builtin", "named"} {
				t.Run(profile.name+"/"+target+"/"+kind, func(t *testing.T) {
					lexical, extra := "xs:ID", ""
					if kind == "named" {
						lexical, extra = "r:Child", directGroupIDNamedTypes()
					}
					root := directGroupIDRoot(target, `<xs:group ref="r:Fields"/>`, `<xs:attribute name="id" type="`+lexical+`"/>`, extra+`<xs:element name="root" type="r:Record"/>`)
					schema, err := parseIDUseSchema(t, root, nil, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					useLoc := elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="id"`)
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

//nolint:gocognit // Both omissions still require reference resolution and Strict10 effective-use cardinality.
func TestDirectGroupIDOmissionAndStrict10Cardinality(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, target := range []string{"choice", "sequence"} {
			root := directGroupIDRoot(target, `<xs:group ref="r:Fields" minOccurs="0" maxOccurs="0"/>`, `<xs:attribute name="id" type="xs:ID"/><xs:attribute name="gone" type="r:Child" use="prohibited"/>`, directGroupIDNamedTypes())
			schema, err := parseIDUseSchema(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Record")
			if owner.Particle() != nil || len(owner.AttributeUses()) != 1 || owner.AttributeUses()[0].Name().Local() != "id" {
				t.Fatalf("omission = %v/%v", owner.Particle(), owner.AttributeUses())
			}
			for _, test := range []struct {
				name, from, to, marker, code, spec string
				cause                              error
			}{
				{"group", `ref="r:Fields"`, `ref="r:Missing"`, `ref="r:Missing"`, diagnosticSchemaModelGroupReferenceUnresolvedCode, schemaModelGroupReferenceSpecRef(profile.version), errSchemaModelGroupReferenceUnresolved},
				{"prohibited type", `name="gone" type="r:Child"`, `name="gone" type="r:Missing"`, `type="r:Missing"`, diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved},
			} {
				t.Run(profile.name+"/"+target+"/"+test.name, func(t *testing.T) {
					broken := strings.Replace(root, test.from, test.to, 1)
					failed, failure := parseIDUseSchema(t, broken, nil, profile.policy)
					assertNCNameDiagnostic(t, failed, failure, FailureInvalid, test.code, elementReferenceTestAttributeLoc(t, broken, test.marker), nil, test.spec, test.cause)
				})
			}
			second := strings.Replace(root, `</xs:complexType>`, `<xs:attribute name="second" type="r:Child"/></xs:complexType>`, 1)
			other, otherErr := parseIDUseSchema(t, second, nil, profile.policy)
			if profile.policy == Strict10 {
				assertNCNameDiagnostic(t, other, otherErr, FailureInvalid, diagnosticSchemaAttributeUseIDDuplicateCode, elementReferenceTestAttributeLoc(t, second, `<xs:attribute name="second"`), []Loc{elementReferenceTestAttributeLoc(t, second, `<xs:attribute name="id"`)}, "xsd10-structures#ct-props-correct", errSchemaAttributeUseIDDuplicate)
				continue
			}
			if otherErr != nil || len(modelGroupReferenceTestComplexTypeNamed(t, other, "urn:root", "Record").AttributeUses()) != 2 {
				t.Fatalf("%s multiple effective IDs = %v", profile.name, otherErr)
			}
		}
	}
}

//nolint:gocognit // One graph matrix checks group and named ID visibility, identity, and source provenance together.
func TestDirectGroupIDForwardAndDiscoveredGraphs(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, target := range []string{"choice", "sequence"} {
			for _, graph := range []string{"forward", "include", "import", "chameleon"} {
				t.Run(profile.name+"/"+target+"/"+graph, func(t *testing.T) {
					prefix, namespace, source := "r:", "urn:root", SourceID("root.xsd")
					var directive string
					if graph == "include" || graph == "chameleon" {
						directive, source = `<xs:include schemaLocation="types.xsd"/>`, "types.xsd"
					}
					if graph == "import" {
						directive, prefix, namespace, source = `<xs:import namespace="urn:other" schemaLocation="types.xsd"/>`, "o:", "urn:other", "types.xsd"
					}
					declarations := `<xs:group name="Fields"><xs:` + target + `/></xs:group><xs:simpleType name="Child"><xs:restriction base="xs:ID"/></xs:simpleType>`
					owner := directGroupLanguageOwner(`<xs:group ref="`+prefix+`Fields" minOccurs="1" maxOccurs="3"/>`, `<xs:attribute name="id" type="`+prefix+`Child"/>`)
					root := languageUseRoot(directive + owner)
					root = strings.Replace(root, `xmlns:r="urn:root"`, `xmlns:r="urn:root" xmlns:o="urn:other"`, 1)
					fixtures := map[string]discoveryFixture{}
					contents := root
					if graph == "forward" {
						root = strings.Replace(root, `</xs:schema>`, declarations+`</xs:schema>`, 1)
						contents = root
					}
					if graph != "forward" {
						ns := ` targetNamespace="` + namespace + `"`
						if graph == "chameleon" {
							ns = ""
						}
						contents = `<xs:schema xmlns:xs="` + testXSDNamespace + `"` + ns + `>` + declarations + `</xs:schema>`
						fixtures["types.xsd"] = discoveryFixture{id: source, contents: contents}
					}
					schema, err := parseIDUseSchema(t, root, fixtures, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					definition := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Record")
					group, ok := definition.Particle().(ModelGroupReferenceParticle)
					groupName := mustTestQName(t, namespace, "Fields")
					groups := schema.FindKind(ComponentKindModelGroupDefinition, groupName)
					if !ok || len(groups) != 1 || group.Ref() != groupName || group.TargetID() != groups[0].ID() || group.TargetID().Source() != source || group.RefLoc() != elementReferenceTestAttributeLoc(t, root, `ref="`+prefix+`Fields"`) || groups[0].Loc() != complexContentTestSourceLoc(t, source, contents, `<xs:group name="Fields"`) || group.Occurrences().String() != "1/3" {
						t.Fatalf("group graph facts = %v/%v", group, groups)
					}
					uses := definition.AttributeUses()
					if len(uses) != 1 {
						t.Fatalf("uses = %v", uses)
					}
					use, ok := uses[0].(LocalAttributeUse)
					if !ok {
						t.Fatalf("use = %T", uses[0])
					}
					ref, hasRef := use.TypeReference()
					id, hasID := use.TypeID()
					refID, hasRefID := ref.ComponentID()
					typeName := mustTestQName(t, namespace, "Child")
					types := schema.FindKind(ComponentKindSimpleTypeDefinition, typeName)
					if !hasRef || !hasID || !hasRefID || !ref.IsNamed() || ref.Name() != typeName || use.DeclaredType() != typeName || len(types) != 1 || id != refID || id != types[0].ID() || id.Source() != source || ref.Loc() != elementReferenceTestAttributeLoc(t, root, `type="`+prefix+`Child"`) || ref.VarietyLoc() != complexContentTestSourceLoc(t, source, contents, `<xs:restriction base="xs:ID"`) || types[0].Loc() != complexContentTestSourceLoc(t, source, contents, `<xs:simpleType name="Child"`) {
						t.Fatalf("named graph facts = %v/%v/%v", use, ref, types)
					}
					wantDocuments := 1
					if graph != "forward" {
						wantDocuments = 2
					}
					if len(schema.Documents()) != wantDocuments || schema.Documents()[0].Source() != "root.xsd" {
						t.Fatalf("discovery = %v", schema.Documents())
					}
					if wantDocuments == 2 && schema.Documents()[1].Source() != source {
						t.Fatalf("discovery = %v", schema.Documents())
					}
				})
			}
		}
	}
}

func TestDirectGroupIDExclusions(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, target := range []string{"choice", "sequence"} {
			for _, test := range []struct {
				name, use, extra, marker, code, spec string
				cause                                error
				class                                FailureClass
				related                              string
			}{
				{"inline atomic", `<xs:attribute name="id"><xs:simpleType><xs:restriction base="xs:ID"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, UnsupportedSchemaSyntaxCode, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeUseUnsupported, FailureUnsupported, ""},
				{"inline list", `<xs:attribute name="id"><xs:simpleType><xs:list itemType="xs:ID"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, UnsupportedSchemaSyntaxCode, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeUseUnsupported, FailureUnsupported, ""},
				{"inline union", `<xs:attribute name="id"><xs:simpleType><xs:union memberTypes="xs:ID xs:ID"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, UnsupportedSchemaSyntaxCode, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeUseUnsupported, FailureUnsupported, ""},
				{"named list", `<xs:attribute name="id" type="r:IDs"/>`, `<xs:simpleType name="IDs"><xs:list itemType="xs:ID"/></xs:simpleType>`, `type="r:IDs"`, UnsupportedSchemaSyntaxCode, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeUseUnsupported, FailureUnsupported, ""},
				{"named union", `<xs:attribute name="id" type="r:IDs"/>`, `<xs:simpleType name="IDs"><xs:union memberTypes="xs:ID xs:ID"/></xs:simpleType>`, `type="r:IDs"`, UnsupportedSchemaSyntaxCode, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeUseUnsupported, FailureUnsupported, ""},
				{"attribute ref", `<xs:attribute ref="r:id"/>`, `<xs:attribute name="id" type="xs:ID"/>`, `ref="r:id"`, UnsupportedSchemaSyntaxCode, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeReferenceUnsupported, FailureUnsupported, `<xs:attribute name="id"`},
				{"builtin default", `<xs:attribute name="id" type="xs:ID" default="a"/>`, "", `default="a"`, UnsupportedSchemaSyntaxCode, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseUnsupported, FailureUnsupported, ""},
				{"builtin fixed", `<xs:attribute name="id" type="xs:ID" fixed="a"/>`, "", `fixed="a"`, UnsupportedSchemaSyntaxCode, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseUnsupported, FailureUnsupported, ""},
				{"named default", `<xs:attribute name="id" type="r:Child" default="a"/>`, directGroupIDNamedTypes(), `default="a"`, UnsupportedSchemaSyntaxCode, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseUnsupported, FailureUnsupported, ""},
				{"named fixed", `<xs:attribute name="id" type="r:Child" fixed="a"/>`, directGroupIDNamedTypes(), `fixed="a"`, UnsupportedSchemaSyntaxCode, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseUnsupported, FailureUnsupported, ""},
				{"named facet", `<xs:attribute name="id" type="r:Patterned"/>`, `<xs:simpleType name="Patterned"><xs:restriction base="xs:ID"><xs:pattern value="[a-z]+"/></xs:restriction></xs:simpleType>`, `<xs:pattern`, UnsupportedDatatypeFacetCode, tokenDiagnosticSpecRef(profile.version, "decimal"), ErrUnsupported, FailureUnsupported, ""},
				{"inline facet", `<xs:attribute name="id"><xs:simpleType><xs:restriction base="xs:ID"><xs:pattern value="[a-z]+"/></xs:restriction></xs:simpleType></xs:attribute>`, "", `<xs:pattern`, UnsupportedDatatypeFacetCode, tokenDiagnosticSpecRef(profile.version, "decimal"), ErrUnsupported, FailureUnsupported, ""},
			} {
				t.Run(profile.name+"/"+target+"/"+test.name, func(t *testing.T) {
					root := directGroupIDRoot(target, `<xs:group ref="r:Fields"/>`, test.use, test.extra)
					failed, failure := parseIDUseSchema(t, root, nil, profile.policy)
					var related []Loc
					if test.related != "" {
						related = []Loc{elementReferenceTestAttributeLoc(t, root, test.related)}
					}
					assertNCNameDiagnostic(t, failed, failure, test.class, test.code, elementReferenceTestAttributeLoc(t, root, test.marker), related, test.spec, test.cause)
				})
			}
		}
	}
}

func TestDirectGroupIDExcludedOwners(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, target := range []string{"choice", "sequence"} {
			for _, test := range []struct {
				name, body, marker, code, spec string
				cause                          error
			}{
				{"anonymous", `<xs:element name="root"><xs:complexType><xs:group ref="r:Fields"/><xs:attribute name="id" type="xs:ID"/></xs:complexType></xs:element>`, `ref="r:Fields"`, UnsupportedSchemaSyntaxCode, "xsd10-structures#schema-document", ErrUnsupported},
				{"nested group", `<xs:complexType name="Record"><xs:sequence><xs:group ref="r:Fields"/></xs:sequence><xs:attribute name="id" type="xs:ID"/></xs:complexType>`, `ref="r:Fields"`, UnsupportedSchemaSyntaxCode, "xsd10-structures#schema-document", ErrUnsupported},
				{"simple content", `<xs:complexType name="Record"><xs:simpleContent><xs:extension base="xs:string"><xs:attribute name="id" type="xs:ID"/></xs:extension></xs:simpleContent></xs:complexType>`, `type="xs:ID"`, UnsupportedSchemaSyntaxCode, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeUseUnsupported},
			} {
				t.Run(profile.name+"/"+target+"/"+test.name, func(t *testing.T) {
					root := idUseRoot(test.body, `<xs:group name="Fields"><xs:`+target+`/></xs:group>`)
					failed, failure := parseIDUseSchema(t, root, nil, profile.policy)
					assertNCNameDiagnostic(t, failed, failure, FailureUnsupported, test.code, elementReferenceTestAttributeLoc(t, root, test.marker), nil, test.spec, test.cause)
				})
			}
		}
	}
}

//nolint:gocognit // Each alternate reference exit remains located and preserves its underlying cause.
func TestDirectGroupIDReferenceAndCycleExits(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, target := range []string{"choice", "sequence"} {
			baseGroup := `<xs:group name="Fields"><xs:` + target + `/></xs:group>`
			for _, test := range []struct {
				name, group, use, declaration, marker, related, code, spec string
				cause, resolverCause                                       error
			}{
				{"missing group", `<xs:group ref="r:Missing"/>`, `<xs:attribute name="id" type="r:Child"/>`, directGroupIDNamedTypes(), `ref="r:Missing"`, "", diagnosticSchemaModelGroupReferenceUnresolvedCode, schemaModelGroupReferenceSpecRef(profile.version), errSchemaModelGroupReferenceUnresolved, nil},
				{"wrong group kind", `<xs:group ref="r:Wrong"/>`, `<xs:attribute name="id" type="r:Child"/>`, `<xs:element name="Wrong" type="xs:integer"/>` + directGroupIDNamedTypes(), `ref="r:Wrong"`, `<xs:element name="Wrong"`, diagnosticSchemaModelGroupReferenceWrongKindCode, schemaModelGroupReferenceSpecRef(profile.version), errSchemaModelGroupReferenceWrongKind, nil},
				{"group cycle", `<xs:group ref="r:Fields"/>`, `<xs:attribute name="id" type="xs:ID"/>`, `<xs:group name="Fields"><xs:` + target + `><xs:group ref="r:Fields"/></xs:` + target + `></xs:group>`, `ref="r:Fields"/></xs:` + target + `>`, `<xs:group name="Fields"`, diagnosticSchemaModelGroupReferenceCycleCode, directGroupCycleSpecRef(profile.version), errSchemaModelGroupReferenceCycle, nil},
				{"missing type", `<xs:group ref="r:Fields"/>`, `<xs:attribute name="id" type="r:Missing"/>`, baseGroup, `type="r:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved, errSchemaSimpleTypeBaseUnresolved},
				{"wrong type kind", `<xs:group ref="r:Fields"/>`, `<xs:attribute name="id" type="r:Fields"/>`, baseGroup, `type="r:Fields"`, `<xs:group name="Fields"`, diagnosticSchemaAttributeTypeWrongKindCode, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeWrongKind, errSchemaSimpleTypeBaseWrongKind},
			} {
				t.Run(profile.name+"/"+target+"/"+test.name, func(t *testing.T) {
					root := languageUseRoot(directGroupLanguageOwner(test.group, test.use) + test.declaration)
					failed, failure := parseIDUseSchema(t, root, nil, profile.policy)
					var related []Loc
					if test.related != "" {
						related = []Loc{elementReferenceTestAttributeLoc(t, root, test.related)}
					}
					assertNCNameDiagnostic(t, failed, failure, FailureInvalid, test.code, elementReferenceTestAttributeLoc(t, root, test.marker), related, test.spec, test.cause)
					if test.resolverCause != nil && !errors.Is(failure, test.resolverCause) {
						t.Fatalf("underlying resolver cause lost: %v", failure)
					}
				})
			}
			for _, test := range []struct {
				name, use, extra, marker, code, spec string
				cause                                error
				related                              []string
			}{
				{"type cycle", `<xs:attribute name="id" type="r:First"/>`, `<xs:simpleType name="First"><xs:restriction base="r:Second"/></xs:simpleType><xs:simpleType name="Second"><xs:restriction base="r:First"/></xs:simpleType>`, `type="r:First"`, diagnosticSchemaAttributeTypeCycleCode, schemaAttributeTypeSpecRef(profile.version), errSchemaSimpleTypeBaseCycle, []string{`<xs:attribute name="id"`, `base="r:Second"`, `base="r:First"`, `<xs:simpleType name="First"`, `<xs:simpleType name="Second"`}},
				{"duplicate type", `<xs:attribute name="id" type="r:Child"/>`, `<xs:simpleType name="Child"><xs:restriction base="xs:ID"/></xs:simpleType><xs:simpleType name="Child" id="second"><xs:restriction base="xs:ID"/></xs:simpleType>`, `<xs:simpleType name="Child" id="second"`, diagnosticSchemaGlobalDuplicateCode, schemaGlobalDuplicateSpecRef(profile.version), errSchemaGlobalDeclarationDuplicate, []string{`<xs:simpleType name="Child"><xs:restriction`}},
				{"duplicate group", `<xs:attribute name="id" type="xs:ID"/>`, `<xs:group name="Fields"><xs:` + target + `/></xs:group>`, `<xs:group name="Fields"><xs:` + target + `/>`, diagnosticSchemaGlobalDuplicateCode, schemaGlobalDuplicateSpecRef(profile.version), errSchemaGlobalDeclarationDuplicate, []string{`<xs:group name="Fields"><xs:` + target + `><xs:element`}},
			} {
				t.Run(profile.name+"/"+target+"/"+test.name, func(t *testing.T) {
					root := directGroupIDRoot(target, `<xs:group ref="r:Fields"/>`, test.use, test.extra)
					failed, failure := parseIDUseSchema(t, root, nil, profile.policy)
					var related []Loc
					for _, marker := range test.related {
						related = append(related, elementReferenceTestAttributeLoc(t, root, marker))
					}
					primary := elementReferenceTestAttributeLoc(t, root, test.marker)
					if test.name == "duplicate group" {
						primary = allParticleTestTokenLoc(t, "root.xsd", root, test.marker, 1)
					}
					assertNCNameDiagnostic(t, failed, failure, FailureInvalid, test.code, primary, related, test.spec, test.cause)
				})
			}
		}
	}
}

func TestDirectGroupIDVisibilityAndAcquisitionExits(t *testing.T) {
	foreign := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:group name="Fields"><xs:choice/></xs:group><xs:simpleType name="Child"><xs:restriction base="xs:ID"/></xs:simpleType></xs:schema>`
	bridge := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`
	fixtures := map[string]discoveryFixture{"bridge.xsd": {id: "bridge.xsd", contents: bridge}, "other.xsd": {id: "other.xsd", contents: foreign}}
	for _, profile := range tokenPolicyProfiles() {
		for _, target := range []string{"choice", "sequence"} {
			t.Run(profile.name+"/"+target+"/invisible group", func(t *testing.T) {
				root := languageUseRoot(`<xs:include schemaLocation="bridge.xsd"/>` + directGroupLanguageOwner(`<xs:group ref="o:Fields" minOccurs="0" maxOccurs="0"/>`, `<xs:attribute name="id" type="xs:ID"/>`))
				root = strings.Replace(root, `xmlns:r="urn:root"`, `xmlns:r="urn:root" xmlns:o="urn:other"`, 1)
				failed, failure := parseIDUseSchema(t, root, fixtures, profile.policy)
				assertNCNameDiagnostic(t, failed, failure, FailureInvalid, diagnosticSchemaModelGroupReferenceNamespaceCode, elementReferenceTestAttributeLoc(t, root, `ref="o:Fields"`), []Loc{complexContentTestSourceLoc(t, "other.xsd", foreign, `<xs:group name="Fields"`)}, schemaModelGroupReferenceImportSpecRef(profile.version), errSchemaModelGroupReferenceNamespace)
			})
			t.Run(profile.name+"/"+target+"/invisible type", func(t *testing.T) {
				root := languageUseRoot(`<xs:include schemaLocation="bridge.xsd"/>` + directGroupLanguageOwner(`<xs:group ref="r:Fields"/>`, `<xs:attribute name="id" type="o:Child"/>`) + `<xs:group name="Fields"><xs:` + target + `/></xs:group>`)
				root = strings.Replace(root, `xmlns:r="urn:root"`, `xmlns:r="urn:root" xmlns:o="urn:other"`, 1)
				failed, failure := parseIDUseSchema(t, root, fixtures, profile.policy)
				assertNCNameDiagnostic(t, failed, failure, FailureInvalid, diagnosticSchemaAttributeTypeUnresolvedCode, elementReferenceTestAttributeLoc(t, root, `type="o:Child"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved)
			})
			t.Run(profile.name+"/"+target+"/source failure", func(t *testing.T) {
				root := languageUseRoot(`<xs:include schemaLocation="missing.xsd"/>` + directGroupLanguageOwner(`<xs:group ref="r:Fields" minOccurs="0" maxOccurs="0"/>`, `<xs:attribute name="id" type="xs:ID"/>`) + `<xs:group name="Fields"><xs:` + target + `/></xs:group>`)
				source, err := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
				if err != nil {
					t.Fatal(err)
				}
				cause := errors.New("ID source unavailable")
				failed, failure := ParseSchemaWithPolicy(source, &discoveryResolver{failures: map[string]error{"missing.xsd": cause}}, profile.policy)
				assertNCNameDiagnostic(t, failed, failure, FailureResolution, SourceResolveCode, elementReferenceTestAttributeLoc(t, root, `<xs:include`), nil, "", cause)
			})
		}
	}
}

//nolint:gocognit // Syntax and duplicate-name checks precede ID cardinality at the affected owner.
func TestDirectGroupIDOccurrencePolicyAndCollisionExits(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, target := range []string{"choice", "sequence"} {
			for _, test := range []struct {
				name, group, use, marker, code, spec string
				cause                                error
				related                              []string
			}{
				{"reversed bounds", `<xs:group ref="r:Fields" minOccurs="2" maxOccurs="1"/>`, `<xs:attribute name="id" type="xs:ID"/>`, `<xs:group ref="r:Fields"`, invalidSchemaCompositionCode, schemaParticleCorrectSpecRef(profile.version), errParticleOccurrenceMinimumExceedsMaximum, []string{`minOccurs="2"`, `maxOccurs="1"`}},
				{"duplicate name", `<xs:group ref="r:Fields"/>`, `<xs:attribute name="id" type="xs:ID"/><xs:attribute name="id" type="xs:ID" use="required"/>`, `<xs:attribute name="id"`, diagnosticSchemaAttributeUseDuplicateCode, schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseDuplicate, []string{`<xs:attribute name="id"`}},
			} {
				t.Run(profile.name+"/"+target+"/"+test.name, func(t *testing.T) {
					root := directGroupIDRoot(target, test.group, test.use, "")
					primary := elementReferenceTestAttributeLoc(t, root, test.marker)
					if test.name == "duplicate name" {
						primary = allParticleTestTokenLoc(t, "root.xsd", root, test.marker, 2)
					}
					var related []Loc
					for _, marker := range test.related {
						related = append(related, elementReferenceTestAttributeLoc(t, root, marker))
					}
					failed, failure := parseIDUseSchema(t, root, nil, profile.policy)
					assertNCNameDiagnostic(t, failed, failure, FailureInvalid, test.code, primary, related, test.spec, test.cause)
				})
			}
			for _, spelling := range []string{"false", "0", "true", "1", "maybe"} {
				t.Run(profile.name+"/"+target+"/inheritable="+spelling, func(t *testing.T) {
					root := directGroupIDRoot(target, `<xs:group ref="r:Fields"/>`, `<xs:attribute name="id" type="xs:ID" inheritable="`+spelling+`"/>`, "")
					failed, failure := parseIDUseSchema(t, root, nil, profile.policy)
					if (spelling == "false" || spelling == "0") && profile.policy != Strict10 {
						if failure != nil || len(modelGroupReferenceTestComplexTypeNamed(t, failed, "urn:root", "Record").AttributeUses()) != 1 {
							t.Fatalf("false inheritable = %v", failure)
						}
						return
					}
					loc := elementReferenceTestAttributeLoc(t, root, `inheritable="`+spelling+`"`)
					if spelling == "maybe" {
						assertZeroSchema(t, failed)
						d := requireDiagnostic(t, failure)
						var lexical Diagnostic
						if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.Loc() != loc || len(d.Related()) != 0 || d.SpecRef() != strictBooleanSpecRef(profile.version) || !errors.As(errors.Unwrap(d), &lexical) || lexical.Code() != InvalidBooleanLexicalCode || lexical.Loc() != loc {
							t.Fatalf("malformed inheritable = %v", failure)
						}
						return
					}
					cause := ErrUnsupported
					if profile.policy == Strict10 {
						cause = errLanguagePolicyMismatch
					}
					assertNCNameDiagnostic(t, failed, failure, FailureUnsupported, UnsupportedSchemaSyntaxCode, loc, nil, "xsd11-structures#cSchemaDocument", cause)
				})
			}
		}
	}
}

func TestDirectGroupIDOtherBoundaryExits(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, target := range []string{"choice", "sequence"} {
			t.Run(profile.name+"/"+target+"/malformed occurrence", func(t *testing.T) {
				root := languageUseRoot(directGroupLanguageOwner(`<xs:group ref="r:Fields" minOccurs="many"/>`, `<xs:attribute name="id" type="xs:ID"/>`) + `<xs:group name="Fields"><xs:` + target + `/></xs:group>`)
				failed, failure := parseIDUseSchema(t, root, nil, profile.policy)
				assertZeroSchema(t, failed)
				d := requireDiagnostic(t, failure)
				if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `minOccurs="many"`) || len(d.Related()) != 0 || d.SpecRef() != schemaParticleOccurrenceDatatypeSpecRef(profile.version) {
					t.Fatalf("occurrence = %v", failure)
				}
			})
			t.Run(profile.name+"/"+target+"/named base final", func(t *testing.T) {
				extra := `<xs:simpleType name="Child"><xs:restriction base="r:Parent"/></xs:simpleType><xs:simpleType name="Parent" final="restriction"><xs:restriction base="xs:ID"/></xs:simpleType>`
				root := directGroupIDRoot(target, `<xs:group ref="r:Fields"/>`, `<xs:attribute name="id" type="r:Child"/>`, extra)
				failed, failure := parseIDUseSchema(t, root, nil, profile.policy)
				assertNCNameDiagnostic(t, failed, failure, FailureInvalid, diagnosticSchemaSimpleTypeBaseCode, elementReferenceTestAttributeLoc(t, root, `base="r:Parent"`), []Loc{elementReferenceTestAttributeLoc(t, root, `final="restriction"`)}, schemaSimpleTypeRestrictionSpecRef(profile.version), errSchemaSimpleTypeInvalidDerivation)
			})
			t.Run(profile.name+"/"+target+"/other datatype", func(t *testing.T) {
				root := directGroupIDRoot(target, `<xs:group ref="r:Fields"/>`, `<xs:attribute name="other" type="xs:QName"/>`, "")
				failed, failure := parseIDUseSchema(t, root, nil, profile.policy)
				assertNCNameDiagnostic(t, failed, failure, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, `type="xs:QName"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeUseUnsupported)
			})
		}
		t.Run(profile.name+"/unsupported group target", func(t *testing.T) {
			root := languageUseRoot(directGroupLanguageOwner(`<xs:group ref="r:Fields"/>`, `<xs:attribute name="id" type="xs:ID"/>`) + `<xs:group name="Fields"><xs:all/></xs:group>`)
			failed, failure := parseIDUseSchema(t, root, nil, profile.policy)
			assertNCNameDiagnostic(t, failed, failure, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, `<xs:all`), nil, directGroupUnsupportedSyntaxSpecRef(profile.version), ErrUnsupported)
		})
	}
}

//nolint:gocognit // Every target, ID kind, and authored omission has the same excluded owner gate.
func TestDirectGroupIDAnonymousOwnerAndOmittedGroupExclusions(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, target := range []string{"choice", "sequence"} {
			for _, kind := range []string{"builtin", "named"} {
				for _, bounds := range []string{"", ` minOccurs="0" maxOccurs="0"`} {
					t.Run(profile.name+"/"+target+"/"+kind+"/"+bounds, func(t *testing.T) {
						lexical, extra := "xs:ID", ""
						if kind == "named" {
							lexical, extra = "r:Child", directGroupIDNamedTypes()
						}
						root := idUseRoot(`<xs:element name="root"><xs:complexType><xs:group ref="r:Fields"`+bounds+`/><xs:attribute name="id" type="`+lexical+`"/></xs:complexType></xs:element>`, `<xs:group name="Fields"><xs:`+target+`/></xs:group>`+extra)
						failed, failure := parseIDUseSchema(t, root, nil, profile.policy)
						assertNCNameDiagnostic(t, failed, failure, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, `ref="r:Fields"`), nil, "xsd10-structures#schema-document", ErrUnsupported)
					})
				}
			}
		}
	}
}
