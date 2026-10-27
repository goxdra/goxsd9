package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func directGroupTokenRoot(target, group, uses, extra string) string {
	return languageUseRoot(`<xs:complexType name="Record">` + group + uses + `</xs:complexType><xs:group name="Fields"><xs:` + target + `><xs:element ref="r:value"/></xs:` + target + `></xs:group><xs:element name="value" type="xs:integer"/>` + extra)
}

func directGroupTokenUse(kind string) string {
	return `<xs:attribute name="` + kind + `" type="xs:` + kind + `" use="required" form="unqualified"/>`
}

//nolint:gocognit // One public view ties group identity, bounds, copied types, locations, and lexical order together.
func TestDirectGroupTokenFamilyPublicFacts(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, target := range []string{"choice", "sequence"} {
			for _, kind := range []string{"token", "NMTOKEN"} {
				t.Run(profile.name+"/"+target+"/"+kind, func(t *testing.T) {
					uses := `<xs:attribute name="before" type="xs:boolean"/>` + directGroupTokenUse(kind) + `<xs:attribute name="gone" type="xs:` + kind + `" use="prohibited"/><xs:attribute name="after" type="xs:integer"/>`
					root := directGroupTokenRoot(target, `<xs:group ref="r:Fields" minOccurs="2" maxOccurs="5"/>`, uses, "")
					schema, err := parseIDUseSchema(t, root, nil, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					if schema.LanguagePolicy() != profile.policy || len(schema.Documents()) != 1 || schema.Documents()[0].Source() != "root.xsd" {
						t.Fatalf("policy/documents = %s/%v", schema.LanguagePolicy(), schema.Documents())
					}
					owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Record")
					if owner.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Record"`) || owner.Derivation() != "" || !owner.Base().IsZero() || !owner.DerivationLoc().IsZero() || !owner.BaseLoc().IsZero() {
						t.Fatalf("owner = %#v", owner)
					}
					particle, ok := owner.Particle().(ModelGroupReferenceParticle)
					groups := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, "urn:root", "Fields"))
					if !ok || len(groups) != 1 || particle.Name() != mustTestQName(t, "urn:root", "Fields") || particle.Ref() != mustTestQName(t, "urn:root", "Fields") || particle.TargetID() != groups[0].ID() || particle.Occurrences().String() != "2/5" || particle.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:group ref="r:Fields"`) || particle.RefLoc() != elementReferenceTestAttributeLoc(t, root, `ref="r:Fields"`) {
						t.Fatalf("particle = %#v", owner.Particle())
					}
					usesView := owner.AttributeUses()
					if len(usesView) != 3 || usesView[0].Name().Local() != "before" || usesView[1].Name().Local() != kind || usesView[2].Name().Local() != "after" {
						t.Fatalf("uses = %v", usesView)
					}
					use, ok := usesView[1].(LocalAttributeUse)
					wantType := mustTestQName(t, testXSDNamespace, kind)
					if !ok || use.Name() != mustTestQName(t, "", kind) || use.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="`+kind+`"`) || use.NameLoc() != elementReferenceTestAttributeLoc(t, root, `name="`+kind+`"`) || use.TypeLoc() != elementReferenceTestAttributeLoc(t, root, `type="xs:`+kind+`"`) || use.UseLoc() != elementReferenceTestAttributeLoc(t, root, `use="required"`) || use.FormLoc() != elementReferenceTestAttributeLoc(t, root, `form="unqualified"`) || use.Use() != AttributeUseRequired || use.DeclaredType() != wantType {
						t.Fatalf("use = %#v", usesView[1])
					}
					ref, hasRef := use.TypeReference()
					id, hasID := use.TypeID()
					refID, hasRefID := ref.ComponentID()
					white, hasWhite := ref.StringWhiteSpaceFacet()
					if !hasRef || !ref.IsBuiltin() || ref.Kind() != SimpleTypeReferenceBuiltin || ref.Name() != wantType || ref.QName() != wantType || ref.Loc() != use.TypeLoc() || ref.Variety() != SimpleTypeVarietyAtomicRestriction || !hasWhite || white.Value() != "collapse" || !white.Fixed() || !white.Loc().IsZero() || ref.StringEnumerationFacets().HasEnumeration() || hasID || hasRefID || !id.IsZero() || !refID.IsZero() {
						t.Fatalf("type = %#v/%v/%v", ref, id, refID)
					}
					usesView[1] = nil
					components := schema.Components()
					components[0] = Component{}
					retained, hasRetained := owner.Particle().(ModelGroupReferenceParticle)
					if owner.AttributeUses()[1] == nil || schema.Components()[0].Name().Local() != "Record" || !hasRetained || retained.Occurrences().String() != "2/5" {
						t.Fatal("caller mutated completed facts")
					}
					for repeat := 0; repeat < 2; repeat++ {
						var walked []ComponentID
						if walkErr := schema.Walk(func(component Component) error { walked = append(walked, component.ID()); return nil }); walkErr != nil {
							t.Fatal(walkErr)
						}
						if !reflect.DeepEqual(walked, componentIDs(schema.Components())) {
							t.Fatalf("walk %d = %v", repeat, walked)
						}
					}
					again, err := parseIDUseSchema(t, root, nil, profile.policy)
					if err != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
						t.Fatalf("repeat = %v", err)
					}
				})
			}
		}
	}
}

//nolint:gocognit // Consumers have independent located exits for each newly admitted shape.
func TestDirectGroupTokenFamilyConsumers(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, target := range []string{"choice", "sequence"} {
			for _, kind := range []string{"token", "NMTOKEN"} {
				for _, group := range []struct{ name, syntax string }{{"normal", `<xs:group ref="r:Fields"/>`}, {"omitted", `<xs:group ref="r:Fields" minOccurs="0" maxOccurs="0"/>`}} {
					t.Run(profile.name+"/"+target+"/"+kind+"/"+group.name, func(t *testing.T) {
						root := directGroupTokenRoot(target, group.syntax, directGroupTokenUse(kind), `<xs:element name="root" type="r:Record"/>`)
						schema, err := parseIDUseSchema(t, root, nil, profile.policy)
						if err != nil {
							t.Fatal(err)
						}
						useLoc := elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="`+kind+`"`)
						ownerLoc := elementReferenceTestAttributeLoc(t, root, `<xs:complexType name="Record"`)
						rootLoc := elementReferenceTestAttributeLoc(t, root, `<xs:element name="root"`)
						validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<r:root xmlns:r="urn:root"/>`)))
						vd := requireDiagnostic(t, validationErr)
						if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != useLoc || vd.SpecRef() != instanceValidationSpecRef(profile.version) || !reflect.DeepEqual(vd.Related(), []Loc{rootLoc, ownerLoc, useLoc}) || !errors.Is(validationErr, ErrUnsupported) || !errors.Is(validationErr, errInstanceAttributes) {
							t.Fatalf("validation = %v", validationErr)
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

//nolint:gocognit // Each excluded reference kind and variety is checked at this direct-group owner boundary.
func TestDirectGroupTokenFamilyExcludedForms(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			for _, test := range []struct {
				name, use, extra, marker, related string
				cause                             error
				useSpec                           bool
			}{
				{"named alias", `<xs:attribute name="a" type="r:Alias"/>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:` + kind + `"/></xs:simpleType>`, `type="r:Alias"`, "", errSchemaAttributeUseUnsupported, false},
				{"named list", `<xs:attribute name="a" type="r:Alias"/>`, `<xs:simpleType name="Alias"><xs:list itemType="xs:` + kind + `"/></xs:simpleType>`, `type="r:Alias"`, "", errSchemaAttributeUseUnsupported, false},
				{"named union", `<xs:attribute name="a" type="r:Alias"/>`, `<xs:simpleType name="Alias"><xs:union memberTypes="xs:` + kind + ` xs:` + kind + `"/></xs:simpleType>`, `type="r:Alias"`, "", errSchemaAttributeUseUnsupported, false},
				{"inline atomic", `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:` + kind + `"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, "", errSchemaAttributeTypeUnsupported, false},
				{"inline list", `<xs:attribute name="a"><xs:simpleType><xs:list itemType="xs:` + kind + `"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, "", errSchemaAttributeTypeUnsupported, false},
				{"inline union", `<xs:attribute name="a"><xs:simpleType><xs:union memberTypes="xs:` + kind + ` xs:` + kind + `"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, "", errSchemaAttributeTypeUnsupported, false},
				{"ref", `<xs:attribute ref="r:a"/>`, `<xs:attribute name="a" type="xs:token"/>`, `ref="r:a"`, `<xs:attribute name="a"`, errSchemaAttributeReferenceUnsupported, true},
				{"ref named", `<xs:attribute ref="r:a"/>`, `<xs:attribute name="a" type="r:Alias"/><xs:simpleType name="Alias"><xs:restriction base="xs:token"/></xs:simpleType>`, `ref="r:a"`, `<xs:attribute name="a"`, errSchemaAttributeReferenceUnsupported, true},
				{"ref inline", `<xs:attribute ref="r:a"/>`, `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:token"/></xs:simpleType></xs:attribute>`, `ref="r:a"`, `<xs:attribute name="a"`, errSchemaAttributeReferenceUnsupported, true},
				{"default", `<xs:attribute name="a" type="xs:` + kind + `" default="one"/>`, "", `default="one"`, "", errSchemaAttributeUseUnsupported, true},
				{"fixed", `<xs:attribute name="a" type="xs:` + kind + `" fixed="one"/>`, "", `fixed="one"`, "", errSchemaAttributeUseUnsupported, true},
			} {
				t.Run(profile.name+"/"+kind+"/"+test.name, func(t *testing.T) {
					root := directGroupTokenRoot("choice", `<xs:group ref="r:Fields"/>`, test.use, test.extra)
					schema, err := parseIDUseSchema(t, root, nil, profile.policy)
					var related []Loc
					if test.related != "" {
						related = []Loc{elementReferenceTestAttributeLoc(t, root, test.related)}
					}
					spec := schemaAttributeTypeSpecRef(profile.version)
					if test.useSpec {
						spec = schemaAttributeUseSpecRef(profile.version)
					}
					assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, test.marker), related, spec, test.cause)
				})
			}
		}
	}
}

//nolint:gocognit // Resolution precedes both omission paths, and the alternate group/use exits stay located.
func TestDirectGroupTokenFamilyOmissionAndExits(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, target := range []string{"choice", "sequence"} {
			for _, kind := range []string{"token", "NMTOKEN"} {
				t.Run(profile.name+"/"+target+"/"+kind, func(t *testing.T) {
					root := directGroupTokenRoot(target, `<xs:group ref="r:Fields" minOccurs="0" maxOccurs="0"/>`, directGroupTokenUse(kind)+`<xs:attribute name="gone" type="xs:`+kind+`" use="prohibited"/>`, "")
					schema, err := parseIDUseSchema(t, root, nil, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Record")
					if owner.Particle() != nil || len(owner.AttributeUses()) != 1 || owner.AttributeUses()[0].Name().Local() != kind {
						t.Fatalf("omitted = %T/%v", owner.Particle(), owner.AttributeUses())
					}
					for _, test := range []struct {
						name, from, to, marker, related, code, spec string
						cause                                       error
					}{
						{"missing group", `ref="r:Fields"`, `ref="r:Missing"`, `ref="r:Missing"`, "", diagnosticSchemaModelGroupReferenceUnresolvedCode, schemaModelGroupReferenceSpecRef(profile.version), errSchemaModelGroupReferenceUnresolved},
						{"wrong group kind", `ref="r:Fields"`, `ref="r:value"`, `ref="r:value"`, `<xs:element name="value"`, diagnosticSchemaModelGroupReferenceWrongKindCode, schemaModelGroupReferenceSpecRef(profile.version), errSchemaModelGroupReferenceWrongKind},
						{"prohibited unresolved type", `type="xs:` + kind + `" use="prohibited"`, `type="r:Missing" use="prohibited"`, `type="r:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved},
					} {
						t.Run(test.name, func(t *testing.T) {
							changed := strings.Replace(root, test.from, test.to, 1)
							failed, failure := parseIDUseSchema(t, changed, nil, profile.policy)
							var related []Loc
							if test.related != "" {
								related = []Loc{elementReferenceTestAttributeLoc(t, changed, test.related)}
							}
							assertNCNameDiagnostic(t, failed, failure, FailureInvalid, test.code, elementReferenceTestAttributeLoc(t, changed, test.marker), related, test.spec, test.cause)
						})
					}
				})
			}
		}
	}
}

func TestDirectGroupTokenFamilyResolverCause(t *testing.T) {
	root := languageUseRoot(`<xs:include schemaLocation="missing.xsd"/>` + directGroupLanguageOwner(`<xs:group ref="r:Fields"/>`, directGroupTokenUse("token")) + directGroupChoice)
	source, err := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("token group source unavailable")
	schema, err := ParseSchemaWithPolicy(source, &discoveryResolver{failures: map[string]error{"missing.xsd": cause}}, Strict11)
	assertZeroSchema(t, schema)
	d := requireDiagnostic(t, err)
	if d.Class() != FailureResolution || d.Code() != SourceResolveCode || d.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:include`) || d.SpecRef() != "" || len(d.Related()) != 0 || !errors.Is(err, cause) {
		t.Fatalf("resolver diagnostic = %v", err)
	}
}

//nolint:gocognit // Group graph provenance and ordered built-in uses are queried together.
func TestDirectGroupTokenFamilyGraphProvenance(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, target := range []string{"choice", "sequence"} {
			for _, graph := range []struct {
				name, directive, ref, namespace, source string
				fixtures                                map[string]discoveryFixture
			}{
				{"forward", "", "r:Fields", "urn:root", "root.xsd", nil},
				{"include", `<xs:include schemaLocation="part.xsd"/>`, "r:Fields", "urn:root", "part.xsd", map[string]discoveryFixture{"part.xsd": {id: "part.xsd", contents: languageUseRoot(`<xs:group name="Fields"><xs:` + target + `><xs:element ref="r:value"/></xs:` + target + `></xs:group><xs:element name="value" type="xs:integer"/>`)}}},
				{"import", `<xs:import namespace="urn:other" schemaLocation="part.xsd"/>`, "o:Fields", "urn:other", "part.xsd", map[string]discoveryFixture{"part.xsd": {id: "part.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:other"><xs:group name="Fields"><xs:` + target + `><xs:element ref="o:value"/></xs:` + target + `></xs:group><xs:element name="value" type="xs:integer"/></xs:schema>`}}},
				{"chameleon", `<xs:include schemaLocation="part.xsd"/>`, "r:Fields", "urn:root", "part.xsd", map[string]discoveryFixture{"part.xsd": {id: "part.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:group name="Fields"><xs:` + target + `><xs:element ref="value"/></xs:` + target + `></xs:group><xs:element name="value" type="xs:integer"/></xs:schema>`}}},
			} {
				t.Run(profile.name+"/"+target+"/"+graph.name, func(t *testing.T) {
					root := languageUseRoot(graph.directive + directGroupLanguageOwner(`<xs:group ref="`+graph.ref+`" minOccurs="1" maxOccurs="3"/>`, directGroupTokenUse("token")+directGroupTokenUse("NMTOKEN")))
					root = strings.Replace(root, `xmlns:r="urn:root"`, `xmlns:r="urn:root" xmlns:o="urn:other"`, 1)
					if graph.name == "forward" {
						root = strings.Replace(root, `</xs:schema>`, `<xs:group name="Fields"><xs:`+target+`><xs:element ref="r:value"/></xs:`+target+`></xs:group><xs:element name="value" type="xs:integer"/></xs:schema>`, 1)
					}
					schema, err := parseIDUseSchema(t, root, graph.fixtures, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Record")
					group, ok := owner.Particle().(ModelGroupReferenceParticle)
					groups := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, graph.namespace, "Fields"))
					if !ok || len(groups) != 1 || group.TargetID() != groups[0].ID() || group.TargetID().Source() != SourceID(graph.source) || group.Ref() != mustTestQName(t, graph.namespace, "Fields") || group.Occurrences().String() != "1/3" || group.RefLoc() != elementReferenceTestAttributeLoc(t, root, `ref="`+graph.ref+`"`) {
						t.Fatalf("group = %v/%v", group, groups)
					}
					uses := owner.AttributeUses()
					if len(uses) != 2 || uses[0].Name().Local() != "token" || uses[1].Name().Local() != "NMTOKEN" {
						t.Fatalf("uses = %v", uses)
					}
					for index, kind := range []string{"token", "NMTOKEN"} {
						use, ok := uses[index].(LocalAttributeUse)
						if !ok || use.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="`+kind+`"`) || use.TypeLoc() != elementReferenceTestAttributeLoc(t, root, `type="xs:`+kind+`"`) || use.DeclaredType() != mustTestQName(t, testXSDNamespace, kind) {
							t.Fatalf("use %d = %v", index, uses[index])
						}
					}
					var walked []string
					if err := schema.Walk(func(component Component) error {
						walked = append(walked, string(component.ID().Source())+":"+component.Name().Local())
						return nil
					}); err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(walked, []string{"root.xsd:Record", graph.source + ":Fields", graph.source + ":value"}) {
						t.Fatalf("walk = %v", walked)
					}
				})
			}
		}
	}
}

//nolint:gocognit // Invalid and unsupported alternate exits precede completed publication.
func TestDirectGroupTokenFamilyBoundaryDiagnostics(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			for _, test := range []struct {
				name, group, use, declaration, marker, related, code, spec string
				class                                                      FailureClass
				cause                                                      error
			}{
				{"missing group", `<xs:group ref="r:Missing"/>`, directGroupTokenUse(kind), "", `ref="r:Missing"`, "", diagnosticSchemaModelGroupReferenceUnresolvedCode, schemaModelGroupReferenceSpecRef(profile.version), FailureInvalid, errSchemaModelGroupReferenceUnresolved},
				{"wrong group kind", `<xs:group ref="r:Wrong"/>`, directGroupTokenUse(kind), `<xs:element name="Wrong" type="xs:integer"/>`, `ref="r:Wrong"`, `<xs:element name="Wrong"`, diagnosticSchemaModelGroupReferenceWrongKindCode, schemaModelGroupReferenceSpecRef(profile.version), FailureInvalid, errSchemaModelGroupReferenceWrongKind},
				{"cycle", `<xs:group ref="r:Fields"/>`, directGroupTokenUse(kind), `<xs:group name="Fields"><xs:choice><xs:group ref="r:Fields"/></xs:choice></xs:group>`, `ref="r:Fields"/></xs:choice>`, `<xs:group name="Fields"`, diagnosticSchemaModelGroupReferenceCycleCode, directGroupCycleSpecRef(profile.version), FailureInvalid, errSchemaModelGroupReferenceCycle},
				{"invalid occurrence", `<xs:group ref="r:Missing" minOccurs="many"/>`, directGroupTokenUse(kind), "", `minOccurs="many"`, "", invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(profile.version), FailureInvalid, nil},
				{"reversed bounds", `<xs:group ref="r:Fields" minOccurs="2" maxOccurs="1"/>`, directGroupTokenUse(kind), directGroupChoice, `<xs:group ref="r:Fields"`, `minOccurs="2"`, invalidSchemaCompositionCode, schemaParticleCorrectSpecRef(profile.version), FailureInvalid, errParticleOccurrenceMinimumExceedsMaximum},
				{"unknown type", `<xs:group ref="r:Fields"/>`, `<xs:attribute name="a" type="r:Missing"/>`, directGroupChoice, `type="r:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(profile.version), FailureInvalid, errSchemaAttributeTypeUnresolved},
				{"wrong type kind", `<xs:group ref="r:Fields"/>`, `<xs:attribute name="a" type="r:Fields"/>`, directGroupChoice, `type="r:Fields"`, `<xs:group name="Fields"`, diagnosticSchemaAttributeTypeWrongKindCode, schemaAttributeTypeSpecRef(profile.version), FailureInvalid, errSchemaAttributeTypeWrongKind},
			} {
				t.Run(profile.name+"/"+kind+"/"+test.name, func(t *testing.T) {
					root := languageUseRoot(directGroupLanguageOwner(test.group, test.use) + test.declaration)
					failed, err := parseIDUseSchema(t, root, nil, profile.policy)
					var related []Loc
					if test.related != "" {
						related = []Loc{elementReferenceTestAttributeLoc(t, root, test.related)}
					}
					if test.name == "reversed bounds" {
						related = append(related, elementReferenceTestAttributeLoc(t, root, `maxOccurs="1"`))
					}
					if test.cause == nil {
						assertZeroSchema(t, failed)
						d := requireDiagnostic(t, err)
						if d.Class() != test.class || d.Code() != test.code || d.Loc() != elementReferenceTestAttributeLoc(t, root, test.marker) || d.SpecRef() != test.spec || !reflect.DeepEqual(d.Related(), related) {
							t.Fatalf("diagnostic = %v", err)
						}
						return
					}
					assertNCNameDiagnostic(t, failed, err, test.class, test.code, elementReferenceTestAttributeLoc(t, root, test.marker), related, test.spec, test.cause)
				})
			}
		}
	}
}

// The built-in gate does not admit other scalar families or anonymous direct-group owners.
func TestDirectGroupTokenFamilyOwnerAndFamilyLimits(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			for _, target := range []string{"choice", "sequence"} {
				t.Run(profile.name+"/"+kind+"/"+target+"/anonymous owner", func(t *testing.T) {
					root := languageUseRoot(`<xs:element name="root"><xs:complexType><xs:group ref="r:Fields"/>` + directGroupTokenUse(kind) + `</xs:complexType></xs:element><xs:group name="Fields"><xs:` + target + `/></xs:group>`)
					failed, err := parseIDUseSchema(t, root, nil, profile.policy)
					assertNCNameDiagnostic(t, failed, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, `ref="r:Fields"`), nil, "xsd10-structures#schema-document", ErrUnsupported)
				})
			}
		}
		for _, family := range []string{"long", "short", "int", "byte", "unsignedLong", "normalizedString", "QName"} {
			t.Run(profile.name+"/"+family+"/excluded sibling", func(t *testing.T) {
				root := directGroupTokenRoot("choice", `<xs:group ref="r:Fields"/>`, `<xs:attribute name="other" type="xs:`+family+`"/>`, "")
				failed, err := parseIDUseSchema(t, root, nil, profile.policy)
				assertNCNameDiagnostic(t, failed, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, `type="xs:`+family+`"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeUseUnsupported)
			})
		}
	}
}

//nolint:gocognit // The two independent facet and policy boundaries share one small matrix.
func TestDirectGroupTokenFamilyFacetAndPolicyBoundaries(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			for _, shape := range []string{"named", "inline"} {
				t.Run(profile.name+"/"+kind+"/"+shape+"/pattern", func(t *testing.T) {
					use, extra := `<xs:attribute name="a" type="r:Alias"/>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:`+kind+`"><xs:pattern value=".*"/></xs:restriction></xs:simpleType>`
					if shape == "inline" {
						use, extra = `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:`+kind+`"><xs:pattern value=".*"/></xs:restriction></xs:simpleType></xs:attribute>`, ""
					}
					root := directGroupTokenRoot("choice", `<xs:group ref="r:Fields"/>`, use, extra)
					failed, err := parseIDUseSchema(t, root, nil, profile.policy)
					assertNCNameDiagnostic(t, failed, err, FailureUnsupported, UnsupportedDatatypeFacetCode, elementReferenceTestAttributeLoc(t, root, `<xs:pattern`), nil, tokenDiagnosticSpecRef(profile.version, "decimal"), ErrUnsupported)
				})
			}
			for _, spelling := range []string{"false", "0"} {
				t.Run(profile.name+"/"+kind+"/inheritable="+spelling, func(t *testing.T) {
					use := strings.Replace(directGroupTokenUse(kind), `/>`, ` inheritable="`+spelling+`"/>`, 1)
					root := directGroupTokenRoot("choice", `<xs:group ref="r:Fields"/>`, use, "")
					schema, err := parseIDUseSchema(t, root, nil, profile.policy)
					if profile.policy == Strict10 {
						assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, elementReferenceTestAttributeLoc(t, root, `inheritable="`+spelling+`"`), nil, "xsd11-structures#cSchemaDocument", errLanguagePolicyMismatch)
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Record")
					if len(owner.AttributeUses()) != 1 || owner.AttributeUses()[0].Name().Local() != kind {
						t.Fatalf("omitted inheritable changed use = %v", owner.AttributeUses())
					}
				})
			}
		}
	}
}

// Visibility and duplicate declarations have separate exits before local-use admission.
func TestDirectGroupTokenFamilyInvisibleAndDuplicateGroups(t *testing.T) {
	foreign := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:other"><xs:group name="Fields"><xs:choice><xs:element ref="o:value"/></xs:choice></xs:group><xs:element name="value" type="xs:integer"/></xs:schema>`
	bridge := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`
	fixtures := map[string]discoveryFixture{"bridge.xsd": {id: "bridge.xsd", contents: bridge}, "other.xsd": {id: "other.xsd", contents: foreign}}
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			t.Run(profile.name+"/"+kind+"/invisible", func(t *testing.T) {
				root := languageUseRoot(`<xs:include schemaLocation="bridge.xsd"/>` + directGroupLanguageOwner(`<xs:group ref="o:Fields" minOccurs="0" maxOccurs="0"/>`, directGroupTokenUse(kind)))
				root = strings.Replace(root, `xmlns:r="urn:root"`, `xmlns:r="urn:root" xmlns:o="urn:other"`, 1)
				failed, err := parseIDUseSchema(t, root, fixtures, profile.policy)
				assertNCNameDiagnostic(t, failed, err, FailureInvalid, diagnosticSchemaModelGroupReferenceNamespaceCode, elementReferenceTestAttributeLoc(t, root, `ref="o:Fields"`), []Loc{complexContentTestSourceLoc(t, "other.xsd", foreign, `<xs:group name="Fields"`)}, schemaModelGroupReferenceImportSpecRef(profile.version), errSchemaModelGroupReferenceNamespace)
			})
			t.Run(profile.name+"/"+kind+"/duplicate", func(t *testing.T) {
				root := directGroupTokenRoot("choice", `<xs:group ref="r:Fields"/>`, directGroupTokenUse(kind), `<xs:group name="Fields"><xs:choice/></xs:group>`)
				failed, err := parseIDUseSchema(t, root, nil, profile.policy)
				assertNCNameDiagnostic(t, failed, err, FailureInvalid, diagnosticSchemaGlobalDuplicateCode, elementReferenceTestAttributeLoc(t, root, `<xs:group name="Fields"><xs:choice/>`), []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:group name="Fields"><xs:choice><xs:element`)}, schemaGlobalDuplicateSpecRef(profile.version), errSchemaGlobalDeclarationDuplicate)
			})
		}
	}
}
