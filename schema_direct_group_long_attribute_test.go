package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func directGroupLongRoot(target, group, uses, extra string) string {
	return directGroupTokenRoot(target, group, uses, extra)
}

func directGroupLongTypes() string {
	return `<xs:simpleType name="Child"><xs:restriction base="r:Parent"><xs:minExclusive value="-3"/><xs:maxExclusive value="4"/><xs:enumeration value="2"/></xs:restriction></xs:simpleType>` +
		`<xs:simpleType name="Parent"><xs:restriction base="xs:long"><xs:minInclusive value="-100"/><xs:maxInclusive value="100"/><xs:totalDigits value="3"/></xs:restriction></xs:simpleType>`
}

func assertDirectGroupLongExclusiveBounds(t *testing.T, ref SimpleTypeReference, version XSDVersion, minLoc, maxLoc Loc) {
	t.Helper()
	bounds, ok := ref.IntegerBounds()
	minimum, hasMin := bounds.MinExclusiveFacet()
	maximum, hasMax := bounds.MaxExclusiveFacet()
	if !ok || bounds.Version() != version || !hasMin || !hasMax || minimum.Value().Canonical() != "-3" || maximum.Value().Canonical() != "4" || minimum.Loc() != minLoc || maximum.Loc() != maxLoc || minimum.Version() != version || maximum.Version() != version {
		t.Fatalf("exclusive long bounds = %v/%v", minimum, maximum)
	}
}

//nolint:gocognit // One public view ties exact group and completed datatype provenance together.
func TestDirectGroupLongPublicFacts(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, target := range []string{"choice", "sequence"} {
			t.Run(profile.name+"/"+target, func(t *testing.T) {
				uses := `<xs:attribute name="before" type="xs:boolean"/><xs:attribute name="named" type="r:Child" use="required" form="unqualified"/><xs:attribute name="built" type="xs:long"/><xs:attribute name="gone" type="r:Child" use="prohibited"/><xs:attribute name="after" type="xs:integer"/>`
				root := directGroupLongRoot(target, `<xs:group ref="r:Fields" minOccurs="2" maxOccurs="18446744073709551616"/>`, uses, directGroupLongTypes())
				schema, err := parseIDUseSchema(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				if schema.LanguagePolicy() != profile.policy || len(schema.Documents()) != 1 || schema.Documents()[0].Source() != "root.xsd" {
					t.Fatalf("policy/documents = %v/%v", schema.LanguagePolicy(), schema.Documents())
				}
				owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Record")
				group, hasGroup := owner.Particle().(ModelGroupReferenceParticle)
				groups := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, "urn:root", "Fields"))
				if owner.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Record"`) || !hasGroup || len(groups) != 1 || group.Ref() != mustTestQName(t, "urn:root", "Fields") || group.TargetID() != groups[0].ID() || group.Occurrences().String() != "2/18446744073709551616" || group.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:group ref="r:Fields"`) || group.RefLoc() != elementReferenceTestAttributeLoc(t, root, `ref="r:Fields"`) || groups[0].Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:group name="Fields"`) {
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
				children := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:root", "Child"))
				if !hasRef || !hasID || !hasRefID || len(children) != 1 || ref.Kind() != SimpleTypeReferenceNamed || ref.Variety() != SimpleTypeVarietyAtomicRestriction || ref.Name() != use.DeclaredType() || ref.QName() != use.DeclaredType() || ref.Loc() != use.TypeLoc() || ref.VarietyLoc() != elementReferenceTestAttributeLoc(t, root, `<xs:restriction base="r:Parent"`) || id != refID || id != children[0].ID() || children[0].Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="Child"`) {
					t.Fatalf("named type = %v/%v", ref, id)
				}
				assertDirectGroupLongExclusiveBounds(t, ref, profile.version, elementReferenceTestAttributeLoc(t, root, `value="-3"`), elementReferenceTestAttributeLoc(t, root, `value="4"`))
				definition, ok := children[0].SimpleTypeDefinition()
				if !ok {
					t.Fatal("Child definition missing")
				}
				digits, hasDigits := definition.DigitFacets().TotalDigits()
				digitLoc, hasDigitLoc := definition.DigitFacets().TotalDigitsLoc()
				enumeration := definition.IntegerEnumerationFacets()
				if !hasDigits || digits.Canonical() != "3" || !hasDigitLoc || digitLoc != elementReferenceTestAttributeLoc(t, root, `value="3"`) || len(enumeration.Values()) != 1 || enumeration.Values()[0].Canonical() != "2" || !reflect.DeepEqual(enumeration.Locations(), []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:enumeration value="2"`)}) {
					t.Fatalf("facets = %v/%v", definition.DigitFacets(), enumeration)
				}
				built, ok := usesView[2].(LocalAttributeUse)
				builtRef, hasBuiltRef := built.TypeReference()
				builtID, hasBuiltID := built.TypeID()
				if !ok || !hasBuiltRef || !builtRef.IsBuiltin() || builtRef.Name() != mustTestQName(t, testXSDNamespace, "long") || built.DeclaredType() != builtRef.Name() || builtRef.Loc() != built.TypeLoc() || hasBuiltID || !builtID.IsZero() {
					t.Fatalf("built-in type = %v/%v", built, builtRef)
				}
				assertAllIntegerBounds(t, builtRef, profile.version, "-9223372036854775808", "9223372036854775807", Loc{}, Loc{})
				bounds, _ := ref.IntegerBounds()
				for _, bound := range bounds.Bounds() {
					bound.Value().value.SetInt64(0)
				}
				values, locations := enumeration.Values(), enumeration.Locations()
				values[0].value.SetInt64(0)
				locations[0] = Loc{}
				usesView[1] = nil
				components := schema.Components()
				components[0] = Component{}
				fresh, ok := owner.AttributeUses()[1].(LocalAttributeUse)
				if !ok {
					t.Fatal("fresh use is not local")
				}
				freshRef, _ := fresh.TypeReference()
				assertDirectGroupLongExclusiveBounds(t, freshRef, profile.version, elementReferenceTestAttributeLoc(t, root, `value="-3"`), elementReferenceTestAttributeLoc(t, root, `value="4"`))
				if schema.Components()[0].Name().IsZero() || definition.IntegerEnumerationFacets().Values()[0].Canonical() != "2" {
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

//nolint:gocognit // Keep both policy and group-target inheritance checks together.
func TestDirectGroupLongInheritedBounds(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, target := range []string{"choice", "sequence"} {
			root := directGroupLongRoot(target, `<xs:group ref="r:Fields"/>`, `<xs:attribute name="value" type="r:Child"/>`, `<xs:simpleType name="Child"><xs:restriction base="r:Parent"/></xs:simpleType><xs:simpleType name="Parent"><xs:restriction base="xs:long"><xs:minExclusive value="-9"/><xs:maxInclusive value="12"/></xs:restriction></xs:simpleType>`)
			schema, err := parseIDUseSchema(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Record")
			use, ok := owner.AttributeUses()[0].(LocalAttributeUse)
			if !ok {
				t.Fatal("inherited long use is not local")
			}
			ref, ok := use.TypeReference()
			if !ok {
				t.Fatal("inherited long type reference missing")
			}
			bounds, ok := ref.IntegerBounds()
			minimum, hasMin := bounds.MinExclusiveFacet()
			maximum, hasMax := bounds.MaxInclusiveFacet()
			if !ok || bounds.Version() != profile.version || !hasMin || !hasMax || minimum.Value().Canonical() != "-9" || minimum.Loc() != elementReferenceTestAttributeLoc(t, root, `value="-9"`) || maximum.Value().Canonical() != "12" || maximum.Loc() != elementReferenceTestAttributeLoc(t, root, `value="12"`) {
				t.Fatalf("inherited long bounds = %v/%v", minimum, maximum)
			}
		}
	}
}

//nolint:gocognit // Validation and generation are independent consumers of both admitted use kinds.
func TestDirectGroupLongConsumers(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, target := range []string{"choice", "sequence"} {
			for _, kind := range []string{"builtin", "named"} {
				for _, group := range []struct{ name, syntax string }{{"present", `<xs:group ref="r:Fields"/>`}, {"omitted", `<xs:group ref="r:Fields" minOccurs="0" maxOccurs="0"/>`}} {
					t.Run(profile.name+"/"+target+"/"+kind+"/"+group.name, func(t *testing.T) {
						lexical, extra := "xs:long", ""
						if kind == "named" {
							lexical, extra = "r:Child", directGroupLongTypes()
						}
						root := directGroupLongRoot(target, group.syntax, `<xs:attribute name="value" type="`+lexical+`"/>`, extra+`<xs:element name="root" type="r:Record"/>`)
						schema, err := parseIDUseSchema(t, root, nil, profile.policy)
						if err != nil {
							t.Fatal(err)
						}
						useLoc := elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="value"`)
						ownerLoc := elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Record"`)
						rootLoc := elementReferenceTestAttributeLoc(t, root, `<xs:element name="root"`)
						instanceErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<r:root xmlns:r="urn:root" value="2"/>`)))
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

func TestDirectGroupLongOmissionDependencies(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, target := range []string{"choice", "sequence"} {
			root := directGroupLongRoot(target, `<xs:group ref="r:Fields" minOccurs="0" maxOccurs="0"/>`, `<xs:attribute name="keep" type="xs:long"/><xs:attribute name="gone" type="xs:long" use="prohibited"/>`, "")
			schema, err := parseIDUseSchema(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Record")
			if owner.Particle() != nil || len(owner.AttributeUses()) != 1 || owner.AttributeUses()[0].Name().Local() != "keep" {
				t.Fatalf("omitted = %v/%v", owner.Particle(), owner.AttributeUses())
			}
			for _, tc := range []struct {
				name, from, to, marker, code, spec string
				cause                              error
			}{
				{"group", `ref="r:Fields"`, `ref="r:Missing"`, `ref="r:Missing"`, diagnosticSchemaModelGroupReferenceUnresolvedCode, schemaModelGroupReferenceSpecRef(profile.version), errSchemaModelGroupReferenceUnresolved},
				{"prohibited type", `name="gone" type="xs:long"`, `name="gone" type="r:Missing"`, `type="r:Missing"`, diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved},
			} {
				t.Run(profile.name+"/"+target+"/"+tc.name, func(t *testing.T) {
					changed := strings.Replace(root, tc.from, tc.to, 1)
					failed, failure := parseIDUseSchema(t, changed, nil, profile.policy)
					assertNCNameDiagnostic(t, failed, failure, FailureInvalid, tc.code, elementReferenceTestAttributeLoc(t, changed, tc.marker), nil, tc.spec, tc.cause)
				})
			}
		}
	}
}

func TestDirectGroupLongResolverCause(t *testing.T) {
	root := languageUseRoot(`<xs:include schemaLocation="missing.xsd"/>` + directGroupLanguageOwner(`<xs:group ref="r:Fields"/>`, `<xs:attribute name="value" type="xs:long"/>`) + directGroupChoice)
	source, err := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("direct group long source unavailable")
	schema, err := ParseSchemaWithPolicy(source, &discoveryResolver{failures: map[string]error{"missing.xsd": cause}}, Strict11)
	assertNCNameDiagnostic(t, schema, err, FailureResolution, SourceResolveCode, elementReferenceTestAttributeLoc(t, root, `<xs:include`), nil, "", cause)
}

func TestDirectGroupLongExcludedUseForms(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, tc := range []struct {
			name, use, extra, marker, related string
			spec                              func(XSDVersion) string
			cause                             error
		}{
			{"named list", `<xs:attribute name="a" type="r:Alias"/>`, `<xs:simpleType name="Alias"><xs:list itemType="xs:long"/></xs:simpleType>`, `type="r:Alias"`, "", schemaAttributeTypeSpecRef, errSchemaAttributeUseUnsupported},
			{"named union", `<xs:attribute name="a" type="r:Alias"/>`, `<xs:simpleType name="Alias"><xs:union memberTypes="xs:long xs:int"/></xs:simpleType>`, `type="r:Alias"`, "", schemaAttributeTypeSpecRef, errSchemaAttributeUseUnsupported},
			{"inline atomic", `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:long"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, "", schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnsupported},
			{"inline list", `<xs:attribute name="a"><xs:simpleType><xs:list itemType="xs:long"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, "", schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnsupported},
			{"inline union", `<xs:attribute name="a"><xs:simpleType><xs:union memberTypes="xs:long xs:int"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, "", schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnsupported},
			{"ref built", `<xs:attribute ref="r:a"/>`, `<xs:attribute name="a" type="xs:long"/>`, `ref="r:a"`, `<xs:attribute name="a"`, schemaAttributeUseSpecRef, errSchemaAttributeReferenceUnsupported},
			{"ref named", `<xs:attribute ref="r:a"/>`, `<xs:attribute name="a" type="r:Alias"/><xs:simpleType name="Alias"><xs:restriction base="xs:long"/></xs:simpleType>`, `ref="r:a"`, `<xs:attribute name="a"`, schemaAttributeUseSpecRef, errSchemaAttributeReferenceUnsupported},
			{"ref inline", `<xs:attribute ref="r:a"/>`, `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:long"/></xs:simpleType></xs:attribute>`, `ref="r:a"`, `<xs:attribute name="a"`, schemaAttributeUseSpecRef, errSchemaAttributeReferenceUnsupported},
			{"built default", `<xs:attribute name="a" type="xs:long" default="7"/>`, "", `default="7"`, "", schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported},
			{"built fixed", `<xs:attribute name="a" type="xs:long" fixed="7"/>`, "", `fixed="7"`, "", schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported},
			{"named default", `<xs:attribute name="a" type="r:Alias" default="7"/>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:long"/></xs:simpleType>`, `default="7"`, "", schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported},
			{"named fixed", `<xs:attribute name="a" type="r:Alias" fixed="7"/>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:long"/></xs:simpleType>`, `fixed="7"`, "", schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported},
			{"inline default", `<xs:attribute name="a" default="7"><xs:simpleType><xs:restriction base="xs:long"/></xs:simpleType></xs:attribute>`, "", `default="7"`, "", schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported},
			{"inline fixed", `<xs:attribute name="a" fixed="7"><xs:simpleType><xs:restriction base="xs:long"/></xs:simpleType></xs:attribute>`, "", `fixed="7"`, "", schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported},
			{"ref default", `<xs:attribute ref="r:a" default="7"/>`, `<xs:attribute name="a" type="xs:long"/>`, `default="7"`, "", schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported},
			{"ref fixed", `<xs:attribute ref="r:a" fixed="7"/>`, `<xs:attribute name="a" type="xs:long"/>`, `fixed="7"`, "", schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported},
		} {
			t.Run(profile.name+"/"+tc.name, func(t *testing.T) {
				root := directGroupLongRoot("choice", `<xs:group ref="r:Fields"/>`, tc.use, tc.extra)
				schema, err := parseIDUseSchema(t, root, nil, profile.policy)
				var related []Loc
				if tc.related != "" {
					related = []Loc{elementReferenceTestAttributeLoc(t, root, tc.related)}
				}
				assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, tc.marker), related, tc.spec(profile.version), tc.cause)
			})
		}
	}
}

func TestDirectGroupLongOwnerAndSiblingLimits(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"xs:long", "r:Alias"} {
			for _, target := range []string{"choice", "sequence"} {
				t.Run(profile.name+"/anonymous/"+kind+"/"+target, func(t *testing.T) {
					root := languageUseRoot(`<xs:element name="root"><xs:complexType><xs:group ref="r:Fields"/><xs:attribute name="a" type="` + kind + `"/></xs:complexType></xs:element><xs:group name="Fields"><xs:` + target + `/></xs:group><xs:simpleType name="Alias"><xs:restriction base="xs:long"/></xs:simpleType>`)
					schema, err := parseIDUseSchema(t, root, nil, profile.policy)
					assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, `ref="r:Fields"`), nil, "xsd10-structures#schema-document", ErrUnsupported)
				})
			}
		}
		for _, family := range []string{"short", "int", "byte", "unsignedLong", "QName"} {
			t.Run(profile.name+"/sibling/"+family, func(t *testing.T) {
				root := directGroupLongRoot("choice", `<xs:group ref="r:Fields"/>`, `<xs:attribute name="a" type="xs:`+family+`"/>`, "")
				schema, err := parseIDUseSchema(t, root, nil, profile.policy)
				assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, `type="xs:`+family+`"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeUseUnsupported)
			})
		}
	}
}

//nolint:gocognit // Group and type sources are independently resolved and retain their source IDs.
func TestDirectGroupLongGraphProvenance(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, target := range []string{"choice", "sequence"} {
			for _, graph := range []struct {
				name, directive, groupRef, groupNS, groupSource, typeRef, typeNS, typeSource string
			}{
				{"forward", "", "r:Fields", "urn:root", "root.xsd", "r:Alias", "urn:root", "root.xsd"},
				{"include", `<xs:include schemaLocation="parts.xsd"/><xs:include schemaLocation="parts.xsd"/>`, "r:Fields", "urn:root", "parts.xsd", "r:Alias", "urn:root", "parts.xsd"},
				{"import", `<xs:import namespace="urn:other" schemaLocation="parts.xsd"/>`, "o:Fields", "urn:other", "parts.xsd", "o:Alias", "urn:other", "parts.xsd"},
				{"chameleon", `<xs:include schemaLocation="parts.xsd"/>`, "r:Fields", "urn:root", "parts.xsd", "r:Alias", "urn:root", "parts.xsd"},
			} {
				t.Run(profile.name+"/"+target+"/"+graph.name, func(t *testing.T) {
					groupDefinition := `<xs:group name="Fields"><xs:` + target + `><xs:element ref="` + graph.groupRef[:2] + `value"/></xs:` + target + `></xs:group><xs:element name="value" type="xs:integer"/>`
					typeDefinition := `<xs:simpleType name="Alias"><xs:restriction base="xs:long"><xs:minInclusive value="-7"/><xs:maxInclusive value="9"/></xs:restriction></xs:simpleType>`
					root := languageUseRoot(graph.directive + directGroupLanguageOwner(`<xs:group ref="`+graph.groupRef+`" minOccurs="1" maxOccurs="3"/>`, `<xs:attribute name="value" type="`+graph.typeRef+`"/>`))
					root = strings.Replace(root, `xmlns:r="urn:root"`, `xmlns:r="urn:root" xmlns:o="urn:other"`, 1)
					fixtures := map[string]discoveryFixture{}
					contents := root
					if graph.name == "forward" {
						root = strings.Replace(root, `</xs:schema>`, groupDefinition+typeDefinition+`</xs:schema>`, 1)
						contents = root
					}
					if graph.name != "forward" {
						namespace := ` targetNamespace="` + graph.groupNS + `"`
						if graph.name == "chameleon" {
							namespace = ""
							groupDefinition = strings.ReplaceAll(groupDefinition, `r:value`, `value`)
						}
						contents = `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other"` + namespace + `>` + groupDefinition + typeDefinition + `</xs:schema>`
						fixtures["parts.xsd"] = discoveryFixture{id: "parts.xsd", contents: contents}
					}
					schema, err := parseIDUseSchema(t, root, fixtures, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Record")
					group, ok := owner.Particle().(ModelGroupReferenceParticle)
					groups := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, graph.groupNS, "Fields"))
					if !ok || len(groups) != 1 || group.TargetID() != groups[0].ID() || group.TargetID().Source() != SourceID(graph.groupSource) || group.Ref() != mustTestQName(t, graph.groupNS, "Fields") || group.Occurrences().String() != "1/3" || group.RefLoc() != elementReferenceTestAttributeLoc(t, root, `ref="`+graph.groupRef+`"`) || groups[0].Loc() != complexContentTestSourceLoc(t, SourceID(graph.groupSource), contents, `<xs:group name="Fields"`) {
						t.Fatalf("group = %v/%v", group, groups)
					}
					use, isLocal := owner.AttributeUses()[0].(LocalAttributeUse)
					if !isLocal {
						t.Fatal("graph use is not local")
					}
					ref, ok := use.TypeReference()
					id, hasID := use.TypeID()
					types := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, graph.typeNS, "Alias"))
					if !ok || !hasID || len(types) != 1 || id != types[0].ID() || id.Source() != SourceID(graph.typeSource) || use.DeclaredType() != mustTestQName(t, graph.typeNS, "Alias") || use.TypeLoc() != elementReferenceTestAttributeLoc(t, root, `type="`+graph.typeRef+`"`) || ref.VarietyLoc() != complexContentTestSourceLoc(t, SourceID(graph.typeSource), contents, `<xs:restriction base="xs:long"`) || types[0].Loc() != complexContentTestSourceLoc(t, SourceID(graph.typeSource), contents, `<xs:simpleType name="Alias"`) {
						t.Fatalf("type = %v/%v", use, types)
					}
					assertAllIntegerBounds(t, ref, profile.version, "-7", "9", complexContentTestSourceLoc(t, SourceID(graph.typeSource), contents, `value="-7"`), complexContentTestSourceLoc(t, SourceID(graph.typeSource), contents, `value="9"`))
					var walk []ComponentID
					if walkErr := schema.Walk(func(component Component) error { walk = append(walk, component.ID()); return nil }); walkErr != nil {
						t.Fatal(walkErr)
					}
					if !reflect.DeepEqual(walk, componentIDs(schema.Components())) {
						t.Fatalf("walk = %v", walk)
					}
				})
			}
		}
	}
}

//nolint:gocognit // Each alternate group/type exit is checked at its original located boundary.
func TestDirectGroupLongBoundaryDiagnostics(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, tc := range []struct {
			name, group, use, extra, marker, related, code, spec string
			class                                                FailureClass
			cause                                                error
		}{
			{"missing group", `<xs:group ref="r:Missing"/>`, `<xs:attribute name="a" type="xs:long"/>`, "", `ref="r:Missing"`, "", diagnosticSchemaModelGroupReferenceUnresolvedCode, schemaModelGroupReferenceSpecRef(profile.version), FailureInvalid, errSchemaModelGroupReferenceUnresolved},
			{"wrong group kind", `<xs:group ref="r:Wrong"/>`, `<xs:attribute name="a" type="xs:long"/>`, `<xs:element name="Wrong" type="xs:integer"/>`, `ref="r:Wrong"`, `<xs:element name="Wrong"`, diagnosticSchemaModelGroupReferenceWrongKindCode, schemaModelGroupReferenceSpecRef(profile.version), FailureInvalid, errSchemaModelGroupReferenceWrongKind},
			{"cycle", `<xs:group ref="r:Fields"/>`, `<xs:attribute name="a" type="xs:long"/>`, ``, `ref="r:Fields"/></xs:choice>`, `<xs:group name="Fields"`, diagnosticSchemaModelGroupReferenceCycleCode, directGroupCycleSpecRef(profile.version), FailureInvalid, errSchemaModelGroupReferenceCycle},
			{"invalid occurrence", `<xs:group ref="r:Missing" minOccurs="many"/>`, `<xs:attribute name="a" type="xs:long"/>`, "", `minOccurs="many"`, "", invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(profile.version), FailureInvalid, nil},
			{"reversed occurrence", `<xs:group ref="r:Fields" minOccurs="2" maxOccurs="1"/>`, `<xs:attribute name="a" type="xs:long"/>`, "", `<xs:group ref="r:Fields"`, `minOccurs="2"`, invalidSchemaCompositionCode, schemaParticleCorrectSpecRef(profile.version), FailureInvalid, errParticleOccurrenceMinimumExceedsMaximum},
			{"missing type", `<xs:group ref="r:Fields"/>`, `<xs:attribute name="a" type="r:Missing"/>`, "", `type="r:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(profile.version), FailureInvalid, errSchemaAttributeTypeUnresolved},
			{"wrong type kind", `<xs:group ref="r:Fields"/>`, `<xs:attribute name="a" type="r:Fields"/>`, "", `type="r:Fields"`, `<xs:group name="Fields"`, diagnosticSchemaAttributeTypeWrongKindCode, schemaAttributeTypeSpecRef(profile.version), FailureInvalid, errSchemaAttributeTypeWrongKind},
			{"type cycle", `<xs:group ref="r:Fields"/>`, `<xs:attribute name="a" type="r:First"/>`, `<xs:simpleType name="First"><xs:restriction base="r:Second"/></xs:simpleType><xs:simpleType name="Second"><xs:restriction base="r:First"/></xs:simpleType>`, `type="r:First"`, "", diagnosticSchemaAttributeTypeCycleCode, schemaAttributeTypeSpecRef(profile.version), FailureInvalid, errSchemaSimpleTypeBaseCycle},
			{"invalid use", `<xs:group ref="r:Fields"/>`, `<xs:attribute name="a" type="xs:long" use="sometimes"/>`, "", `use="sometimes"`, "", invalidSchemaCompositionCode, schemaAttributeUseSpecRef(profile.version), FailureInvalid, nil},
			{"duplicate use", `<xs:group ref="r:Fields"/>`, `<xs:attribute name="a" type="xs:long"/><xs:attribute name="a" type="xs:long" use="required"/>`, "", `<xs:attribute name="a" type="xs:long" use="required"`, `<xs:attribute name="a" type="xs:long"/>`, diagnosticSchemaAttributeUseDuplicateCode, schemaAttributeUseSpecRef(profile.version), FailureInvalid, errSchemaAttributeUseDuplicate},
		} {
			t.Run(profile.name+"/"+tc.name, func(t *testing.T) {
				root := directGroupLongRoot("choice", tc.group, tc.use, tc.extra)
				if tc.name == "cycle" {
					root = strings.Replace(root, `<xs:group name="Fields"><xs:choice><xs:element ref="r:value"/></xs:choice></xs:group>`, `<xs:group name="Fields"><xs:choice><xs:group ref="r:Fields"/></xs:choice></xs:group>`, 1)
				}
				schema, err := parseIDUseSchema(t, root, nil, profile.policy)
				var related []Loc
				if tc.related != "" {
					related = []Loc{elementReferenceTestAttributeLoc(t, root, tc.related)}
				}
				if tc.name == "reversed occurrence" {
					related = append(related, elementReferenceTestAttributeLoc(t, root, `maxOccurs="1"`))
				}
				if tc.name == "type cycle" {
					related = []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="a"`), elementReferenceTestAttributeLoc(t, root, `base="r:Second"`), elementReferenceTestAttributeLoc(t, root, `base="r:First"`), elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="First"`), elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="Second"`)}
				}
				if tc.cause == nil {
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					if d.Class() != tc.class || d.Code() != tc.code || d.Loc() != elementReferenceTestAttributeLoc(t, root, tc.marker) || d.SpecRef() != tc.spec || !reflect.DeepEqual(d.Related(), related) {
						t.Fatalf("diagnostic = %v", err)
					}
					return
				}
				assertNCNameDiagnostic(t, schema, err, tc.class, tc.code, elementReferenceTestAttributeLoc(t, root, tc.marker), related, tc.spec, tc.cause)
			})
		}
	}
}

//nolint:gocognit // Datatype construction and owner policy precede long-use publication.
func TestDirectGroupLongFacetAndPolicyExits(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, tc := range []struct {
			name, declaration, marker, code string
			class                           FailureClass
			spec                            func(XSDVersion) string
			cause                           error
		}{
			{"outside intrinsic", `<xs:simpleType name="Alias"><xs:restriction base="xs:long"><xs:maxInclusive value="9223372036854775808"/></xs:restriction></xs:simpleType>`, `value="9223372036854775808"`, InvalidBoundRestrictionCode, FailureInvalid, func(v XSDVersion) string { return boundSpecRef(v, BoundMaxInclusive, boundRestrictionRule) }, errInvalidBoundRestriction},
			{"malformed bound", `<xs:simpleType name="Alias"><xs:restriction base="xs:long"><xs:minInclusive value="bad"/></xs:restriction></xs:simpleType>`, `value="bad"`, InvalidBoundCode, FailureInvalid, func(v XSDVersion) string { return boundSpecRef(v, BoundMinInclusive, boundDefinitionRule) }, errInvalidBoundValue},
			{"unsupported facet", `<xs:simpleType name="Alias"><xs:restriction base="xs:long"><xs:pattern value="[0-9]+"/></xs:restriction></xs:simpleType>`, `<xs:pattern`, UnsupportedDatatypeFacetCode, FailureUnsupported, func(v XSDVersion) string { return tokenDiagnosticSpecRef(v, "decimal") }, ErrUnsupported},
		} {
			t.Run(profile.name+"/"+tc.name, func(t *testing.T) {
				root := directGroupLongRoot("choice", `<xs:group ref="r:Fields"/>`, `<xs:attribute name="a" type="r:Alias"/>`, tc.declaration)
				schema, err := parseIDUseSchema(t, root, nil, profile.policy)
				assertNCNameDiagnostic(t, schema, err, tc.class, tc.code, elementReferenceTestAttributeLoc(t, root, tc.marker), nil, tc.spec(profile.version), tc.cause)
			})
		}
		t.Run(profile.name+"/named base final", func(t *testing.T) {
			declarations := `<xs:simpleType name="Child"><xs:restriction base="r:Parent"/></xs:simpleType><xs:simpleType name="Parent" final="restriction"><xs:restriction base="xs:long"/></xs:simpleType>`
			root := directGroupLongRoot("choice", `<xs:group ref="r:Fields"/>`, `<xs:attribute name="a" type="r:Child"/>`, declarations)
			schema, err := parseIDUseSchema(t, root, nil, profile.policy)
			assertNCNameDiagnostic(t, schema, err, FailureInvalid, diagnosticSchemaSimpleTypeBaseCode, elementReferenceTestAttributeLoc(t, root, `base="r:Parent"`), []Loc{elementReferenceTestAttributeLoc(t, root, `final="restriction"`)}, schemaSimpleTypeRestrictionSpecRef(profile.version), errSchemaSimpleTypeInvalidDerivation)
		})
		for _, spelling := range []string{"false", "0"} {
			t.Run(profile.name+"/inheritable="+spelling, func(t *testing.T) {
				root := directGroupLongRoot("choice", `<xs:group ref="r:Fields"/>`, `<xs:attribute name="a" type="xs:long" inheritable="`+spelling+`"/>`, "")
				schema, err := parseIDUseSchema(t, root, nil, profile.policy)
				if profile.policy == Strict10 {
					assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, `inheritable="`+spelling+`"`), nil, "xsd11-structures#cSchemaDocument", errLanguagePolicyMismatch)
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Record")
				if len(owner.AttributeUses()) != 1 || owner.AttributeUses()[0].Name().Local() != "a" {
					t.Fatalf("inheritable omission changed use = %v", owner.AttributeUses())
				}
			})
		}
		for _, spelling := range []string{"true", "1", "maybe"} {
			t.Run(profile.name+"/excluded inheritable="+spelling, func(t *testing.T) {
				root := directGroupLongRoot("choice", `<xs:group ref="r:Fields"/>`, `<xs:attribute name="a" type="xs:long" inheritable="`+spelling+`"/>`, "")
				schema, err := parseIDUseSchema(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				loc := elementReferenceTestAttributeLoc(t, root, `inheritable="`+spelling+`"`)
				if d.Loc() != loc || len(d.Related()) != 0 {
					t.Fatalf("inheritable location = %v", err)
				}
				if spelling == "maybe" {
					var lexical Diagnostic
					if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.SpecRef() != strictBooleanSpecRef(profile.version) || !errors.As(errors.Unwrap(d), &lexical) || lexical.Code() != InvalidBooleanLexicalCode || lexical.Loc() != loc || lexical.SpecRef() != d.SpecRef() {
						t.Fatalf("malformed inheritable = %v", err)
					}
					return
				}
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.SpecRef() != "xsd11-structures#cSchemaDocument" || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("excluded inheritable = %v", err)
				}
			})
		}
	}
}

func TestDirectGroupLongVersionLabelsDoNotSelectPolicy(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, label := range []string{"1.0", "1.1"} {
			root := directGroupLongRoot("choice", `<xs:group ref="r:Fields"/>`, `<xs:attribute name="a" type="xs:long"/>`, "")
			root = strings.Replace(root, `<xs:schema `, `<xs:schema version="`+label+`" `, 1)
			schema, err := parseIDUseSchema(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			if schema.LanguagePolicy() != profile.policy {
				t.Fatalf("label %s selected policy %s", label, schema.LanguagePolicy())
			}
			owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Record")
			use, ok := owner.AttributeUses()[0].(LocalAttributeUse)
			if !ok {
				t.Fatal("long use is not local")
			}
			ref, ok := use.TypeReference()
			if !ok {
				t.Fatal("long reference missing")
			}
			assertAllIntegerBounds(t, ref, profile.version, "-9223372036854775808", "9223372036854775807", Loc{}, Loc{})
		}
	}
}

func TestDirectGroupLongInvisibleAndDuplicateTargets(t *testing.T) {
	foreign := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:other"><xs:group name="Fields"><xs:choice><xs:element ref="o:value"/></xs:choice></xs:group><xs:element name="value" type="xs:integer"/><xs:simpleType name="Alias"><xs:restriction base="xs:long"/></xs:simpleType></xs:schema>`
	bridge := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`
	fixtures := map[string]discoveryFixture{"bridge.xsd": {id: "bridge.xsd", contents: bridge}, "other.xsd": {id: "other.xsd", contents: foreign}}
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name+"/invisible group", func(t *testing.T) {
			root := languageUseRoot(`<xs:include schemaLocation="bridge.xsd"/>` + directGroupLanguageOwner(`<xs:group ref="o:Fields" minOccurs="0" maxOccurs="0"/>`, `<xs:attribute name="a" type="xs:long"/>`))
			root = strings.Replace(root, `xmlns:r="urn:root"`, `xmlns:r="urn:root" xmlns:o="urn:other"`, 1)
			schema, err := parseIDUseSchema(t, root, fixtures, profile.policy)
			assertNCNameDiagnostic(t, schema, err, FailureInvalid, diagnosticSchemaModelGroupReferenceNamespaceCode, elementReferenceTestAttributeLoc(t, root, `ref="o:Fields"`), []Loc{complexContentTestSourceLoc(t, "other.xsd", foreign, `<xs:group name="Fields"`)}, schemaModelGroupReferenceImportSpecRef(profile.version), errSchemaModelGroupReferenceNamespace)
		})
		t.Run(profile.name+"/invisible type", func(t *testing.T) {
			root := languageUseRoot(`<xs:include schemaLocation="bridge.xsd"/>` + directGroupLanguageOwner(`<xs:group ref="r:Fields"/>`, `<xs:attribute name="a" type="o:Alias"/>`) + directGroupChoice)
			root = strings.Replace(root, `xmlns:r="urn:root"`, `xmlns:r="urn:root" xmlns:o="urn:other"`, 1)
			schema, err := parseIDUseSchema(t, root, fixtures, profile.policy)
			assertNCNameDiagnostic(t, schema, err, FailureInvalid, diagnosticSchemaAttributeTypeUnresolvedCode, elementReferenceTestAttributeLoc(t, root, `type="o:Alias"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved)
		})
		t.Run(profile.name+"/duplicate group", func(t *testing.T) {
			root := directGroupLongRoot("choice", `<xs:group ref="r:Fields"/>`, `<xs:attribute name="a" type="xs:long"/>`, `<xs:group name="Fields"><xs:choice/></xs:group>`)
			schema, err := parseIDUseSchema(t, root, nil, profile.policy)
			assertNCNameDiagnostic(t, schema, err, FailureInvalid, diagnosticSchemaGlobalDuplicateCode, elementReferenceTestAttributeLoc(t, root, `<xs:group name="Fields"><xs:choice/>`), []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:group name="Fields"><xs:choice><xs:element`)}, schemaGlobalDuplicateSpecRef(profile.version), errSchemaGlobalDeclarationDuplicate)
		})
		t.Run(profile.name+"/duplicate type", func(t *testing.T) {
			root := directGroupLongRoot("choice", `<xs:group ref="r:Fields"/>`, `<xs:attribute name="a" type="r:Alias"/>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:long"/></xs:simpleType><xs:simpleType name="Alias" id="second"><xs:restriction base="xs:long"/></xs:simpleType>`)
			schema, err := parseIDUseSchema(t, root, nil, profile.policy)
			assertNCNameDiagnostic(t, schema, err, FailureInvalid, diagnosticSchemaGlobalDuplicateCode, elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="Alias" id="second"`), []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:simpleType name="Alias"><xs:restriction`)}, schemaGlobalDuplicateSpecRef(profile.version), errSchemaGlobalDeclarationDuplicate)
		})
	}
}
