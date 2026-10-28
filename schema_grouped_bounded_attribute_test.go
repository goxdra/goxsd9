package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func groupedBoundedUse(f boundedLocalFamily) string {
	return `<xs:attribute name="bounded" type="xs:` + f.name + `" use="required" form="unqualified"/>`
}

//nolint:gocognit,funlen // One public query checks the complete newly admitted group/use/base view.
func TestGroupedBoundedBuiltinPublicFacts(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			for _, zero := range []bool{false, true} {
				name, occurrences := "present", ` minOccurs="2" maxOccurs="3"`
				if zero {
					name, occurrences = "zero", ` minOccurs="0" maxOccurs="0"`
				}
				t.Run(profile.name+"/"+family.name+"/"+name, func(t *testing.T) {
					uses := `<xs:attribute name="before" type="xs:boolean"/>` + groupedBoundedUse(family) + `<xs:attribute name="gone" type="xs:` + family.name + `" use="prohibited"/><xs:attribute name="after" type="xs:integer"/>`
					root := groupedExtensionSchema(string(profile.version), occurrences, uses)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					again, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
						t.Fatalf("nondeterministic components: %v", err)
					}
					if schema.LanguagePolicy() != profile.policy || len(schema.Documents()) != 1 || schema.Documents()[0].Source() != "root.xsd" {
						t.Fatalf("policy/documents = %v/%v", schema.LanguagePolicy(), schema.Documents())
					}
					owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Derived")
					base, hasBase := owner.BaseReference()
					baseID, hasBaseID := base.ComponentID()
					bases := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Base"))
					if owner.Loc() != complexContentTestLoc(t, root, `<xs:complexType name="Derived"`) || owner.DerivationLoc() != complexContentTestLoc(t, root, `<xs:extension`) || owner.BaseLoc() != complexContentTestLoc(t, root, `base="t:Base"`) || !hasBase || !hasBaseID || len(bases) != 1 || baseID != bases[0].ID() || base.Loc() != owner.BaseLoc() {
						t.Fatalf("owner/base = %#v/%#v", owner, base)
					}
					group, hasGroup := owner.Particle().(ModelGroupReferenceParticle)
					groups := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, "urn:root", "Fields"))
					wantRange := "2/3"
					if zero {
						wantRange = "0/0"
					}
					if zero && owner.Particle() != nil {
						t.Fatalf("zero group was published: %#v", owner.Particle())
					}
					if !zero && (!hasGroup || len(groups) != 1 || group.Ref() != mustTestQName(t, "urn:root", "Fields") || group.TargetID() != groups[0].ID() || group.Loc() != complexContentTestLoc(t, root, `<xs:group ref="t:Fields"`) || group.RefLoc() != complexContentTestLoc(t, root, `ref="t:Fields"`) || group.Occurrences().String() != wantRange) {
						t.Fatalf("group = %#v", owner.Particle())
					}
					usesFound := owner.AttributeUses()
					if len(usesFound) != 3 || usesFound[0].Name().Local() != "before" || usesFound[1].Name().Local() != "bounded" || usesFound[2].Name().Local() != "after" {
						t.Fatalf("ordered uses = %#v", usesFound)
					}
					use, ok := usesFound[1].(LocalAttributeUse)
					if !ok || use.Name() != mustTestQName(t, "", "bounded") || use.Loc() != complexContentTestLoc(t, root, `<xs:attribute name="bounded"`) || use.NameLoc() != complexContentTestLoc(t, root, `name="bounded"`) || use.TypeLoc() != complexContentTestLoc(t, root, `type="xs:`+family.name+`"`) || use.Use() != AttributeUseRequired || use.UseLoc() != complexContentTestLoc(t, root, `use="required"`) || use.FormLoc() != complexContentTestLoc(t, root, `form="unqualified"`) {
						t.Fatalf("bounded use = %#v", usesFound[1])
					}
					ref, hasRef := use.TypeReference()
					id, hasID := use.TypeID()
					refID, hasRefID := ref.ComponentID()
					if !hasRef || !ref.IsBuiltin() || ref.Variety() != SimpleTypeVarietyAtomicRestriction || use.DeclaredType() != mustTestQName(t, testXSDNamespace, family.name) || ref.Name() != use.DeclaredType() || ref.QName() != use.DeclaredType() || ref.Loc() != use.TypeLoc() || ref.VarietyLoc() != use.TypeLoc() || hasID || hasRefID || !id.IsZero() || !refID.IsZero() {
						t.Fatalf("builtin type = %#v/%v/%v", ref, id, refID)
					}
					assertAllIntegerBounds(t, ref, profile.version, family.minimum, family.maximum, Loc{}, Loc{})
					bounds, _ := ref.IntegerBounds()
					for _, bound := range bounds.Bounds() {
						bound.Value().value.SetInt64(0)
					}
					assertAllIntegerBounds(t, ref, profile.version, family.minimum, family.maximum, Loc{}, Loc{})
					usesFound[1] = nil
					components := schema.Components()
					components[0] = Component{}
					if owner.AttributeUses()[1] == nil || schema.Components()[0].Name().Local() != "root" {
						t.Fatal("public copies mutated schema")
					}
					var walks [2][]ComponentID
					for i := range walks {
						if walkErr := schema.Walk(func(component Component) error { walks[i] = append(walks[i], component.ID()); return nil }); walkErr != nil {
							t.Fatal(walkErr)
						}
					}
					if !reflect.DeepEqual(walks[0], walks[1]) || !reflect.DeepEqual(walks[0], componentIDs(schema.Components())) {
						t.Fatalf("walks = %v/%v", walks[0], walks[1])
					}
					controlRoot := strings.Replace(root, groupedBoundedUse(family), "", 1)
					control, err := discoverTestSchemaWithPolicy(t, controlRoot, nil, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					controlOwner := modelGroupReferenceTestComplexTypeNamed(t, control, "urn:root", "Derived")
					controlBase, _ := controlOwner.BaseReference()
					controlBaseID, _ := controlBase.ComponentID()
					if controlBaseID != baseID || !reflect.DeepEqual(componentIDs(control.Components()), componentIDs(schema.Components())) || len(controlOwner.AttributeUses()) != 2 {
						t.Fatalf("absent-use control changed shared facts = %#v", controlOwner)
					}
					if zero && controlOwner.Particle() != nil {
						t.Fatalf("absent-use zero group = %#v", controlOwner.Particle())
					}
					if !zero {
						controlGroup, ok := controlOwner.Particle().(ModelGroupReferenceParticle)
						if !ok || controlGroup.TargetID() != group.TargetID() || controlGroup.Occurrences().String() != wantRange {
							t.Fatalf("absent-use group = %#v", controlOwner.Particle())
						}
					}
				})
			}
		}
	}
}

//nolint:gocognit // Both public consumers are checked for every policy and bounded type.
func TestGroupedBoundedBuiltinConsumersRemainUnsupported(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			t.Run(profile.name+"/"+family.name, func(t *testing.T) {
				root := groupedExtensionSchema(string(profile.version), "", groupedBoundedUse(family))
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				useLoc := complexContentTestLoc(t, root, `<xs:attribute name="bounded"`)
				ownerLoc := complexContentTestLoc(t, root, `<xs:complexType name="Derived"`)
				rootLoc := complexContentTestLoc(t, root, `<xs:element name="root"`)
				instanceErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
				vd := requireDiagnostic(t, instanceErr)
				if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != useLoc || vd.SpecRef() != instanceValidationSpecRef(profile.version) || !reflect.DeepEqual(vd.Related(), []Loc{rootLoc, ownerLoc, useLoc}) || !errors.Is(instanceErr, errInstanceAttributes) {
					t.Fatalf("validation = %v related=%v", instanceErr, vd.Related())
				}
				output, generationErr := GenerateGo(schema, "generated")
				gd := requireDiagnostic(t, generationErr)
				if output != nil || gd.Class() != FailureUnsupported || gd.Code() != diagnosticCodegenUnsupported || gd.Loc() != useLoc || gd.SpecRef() != codegenDirectSequenceSpecReference(profile.version, codegenDirectSequenceParticlesReference) || !reflect.DeepEqual(gd.Related(), []Loc{ownerLoc, useLoc}) || !errors.Is(generationErr, errCodegenUnsupported) {
					t.Fatalf("generation = %v/%v related=%v", output, generationErr, gd.Related())
				}
			})
		}
	}
}

//nolint:gocognit // Each excluded type/use form is tested at the affected owner with a located result.
func TestGroupedBoundedBuiltinExcludedForms(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			alias := `<xs:simpleType name="Alias"><xs:restriction base="xs:` + family.name + `"/></xs:simpleType>`
			inline := `<xs:simpleType><xs:restriction base="xs:` + family.name + `"/></xs:simpleType>`
			for _, tc := range []struct {
				name, use, extra, marker, related string
				spec                              func(XSDVersion) string
				cause                             error
			}{
				{"inline atomic", `<xs:attribute name="a">` + inline + `</xs:attribute>`, "", `<xs:simpleType`, "", schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnsupported},
				{"inline list", `<xs:attribute name="a"><xs:simpleType><xs:list itemType="xs:` + family.name + `"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, "", schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnsupported},
				{"inline union", `<xs:attribute name="a"><xs:simpleType><xs:union memberTypes="xs:` + family.name + ` xs:integer"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, "", schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnsupported},
				{"named list", `<xs:attribute name="a" type="t:List"/>`, `<xs:simpleType name="List"><xs:list itemType="xs:` + family.name + `"/></xs:simpleType>`, `type="t:List"`, "", schemaAttributeTypeSpecRef, errSchemaAttributeUseUnsupported},
				{"named union", `<xs:attribute name="a" type="t:Union"/>`, `<xs:simpleType name="Union"><xs:union memberTypes="xs:` + family.name + ` xs:integer"/></xs:simpleType>`, `type="t:Union"`, "", schemaAttributeTypeSpecRef, errSchemaAttributeUseUnsupported},
				{"ref builtin", `<xs:attribute ref="t:boundedGlobal"/>`, `<xs:attribute name="boundedGlobal" type="xs:` + family.name + `"/>`, `ref="t:boundedGlobal"`, `<xs:attribute name="boundedGlobal"`, schemaAttributeUseSpecRef, errSchemaAttributeReferenceUnsupported},
				{"ref named", `<xs:attribute ref="t:boundedGlobal"/>`, `<xs:attribute name="boundedGlobal" type="t:Alias"/>` + alias, `ref="t:boundedGlobal"`, `<xs:attribute name="boundedGlobal"`, schemaAttributeUseSpecRef, errSchemaAttributeReferenceUnsupported},
				{"ref inline", `<xs:attribute ref="t:boundedGlobal"/>`, `<xs:attribute name="boundedGlobal">` + inline + `</xs:attribute>`, `ref="t:boundedGlobal"`, `<xs:attribute name="boundedGlobal"`, schemaAttributeUseSpecRef, errSchemaAttributeReferenceUnsupported},
			} {
				t.Run(profile.name+"/"+family.name+"/"+tc.name, func(t *testing.T) {
					root := groupedExtensionSchema(string(profile.version), "", tc.use)
					root = strings.Replace(root, `</xs:schema>`, tc.extra+`</xs:schema>`, 1)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					var related []Loc
					if tc.related != "" {
						related = []Loc{complexContentTestLoc(t, root, tc.related)}
					}
					assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, complexContentTestLoc(t, root, tc.marker), related, tc.spec(profile.version), tc.cause)
				})
			}
			for _, kind := range []string{"default", "fixed"} {
				for _, shape := range []string{"direct", "named", "inline", "ref"} {
					t.Run(profile.name+"/"+family.name+"/"+shape+"/"+kind, func(t *testing.T) {
						use, extra := boundedLocalValueUse(family, shape, kind)
						use = strings.ReplaceAll(use, "r:", "t:")
						extra = strings.ReplaceAll(extra, "r:", "t:")
						root := groupedExtensionSchema(string(profile.version), "", use)
						root = strings.Replace(root, `</xs:schema>`, extra+`</xs:schema>`, 1)
						schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
						assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, complexContentTestLoc(t, root, kind+`="2"`), nil, schemaComplexTypeExtensionSpecRef(profile.version), errSchemaAttributeUseUnsupported)
					})
				}
			}
			anonymous := anonymousGroupedExtensionSchema(string(profile.version), "", groupedBoundedUse(family))
			schema, err := discoverTestSchemaWithPolicy(t, anonymous, nil, profile.policy)
			assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, complexContentTestLoc(t, anonymous, `type="xs:`+family.name+`"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeUseUnsupported)
			anonymous = anonymousGroupedExtensionSchema(string(profile.version), "", `<xs:attribute name="bounded" type="t:Alias"/>`)
			anonymous = strings.Replace(anonymous, `</xs:schema>`, alias+`</xs:schema>`, 1)
			schema, err = discoverTestSchemaWithPolicy(t, anonymous, nil, profile.policy)
			assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, complexContentTestLoc(t, anonymous, `type="t:Alias"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeUseUnsupported)
		}
	}
}

//nolint:gocognit // Graph discovery, group visibility, and wildcard copying are observable together.
func TestGroupedBoundedBuiltinGraphAndWildcard(t *testing.T) {
	shared := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other"><xs:include schemaLocation="root.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/><xs:group name="Fields"><xs:sequence><xs:element ref="o:item"/></xs:sequence></xs:group></xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:element name="item" type="xs:integer"/><xs:complexType name="Base"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType></xs:schema>`
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			t.Run(profile.name+"/"+family.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:include schemaLocation="shared.xsd"/><xs:include schemaLocation="again.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/><xs:complexType name="Derived"><xs:complexContent><xs:extension base="o:Base"><xs:group ref="r:Fields" minOccurs="1" maxOccurs="2"/>` + groupedBoundedUse(family) + `</xs:extension></xs:complexContent></xs:complexType></xs:schema>`
				fixtures := map[string]discoveryFixture{"root.xsd": {id: "root.xsd", contents: root}, "shared.xsd": {id: "shared.xsd", contents: shared}, "again.xsd": {id: "shared.xsd", contents: shared}, "other.xsd": {id: "other.xsd", contents: other}}
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				var sources []SourceID
				for _, document := range schema.Documents() {
					sources = append(sources, document.Source())
				}
				if !reflect.DeepEqual(sources, []SourceID{"root.xsd", "shared.xsd", "other.xsd"}) {
					t.Fatalf("discovery order = %v", sources)
				}
				owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Derived")
				group, hasGroup := owner.Particle().(ModelGroupReferenceParticle)
				groups := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, "urn:root", "Fields"))
				base, hasBase := owner.BaseReference()
				baseID, hasBaseID := base.ComponentID()
				bases := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:other", "Base"))
				wildcard, hasWildcard := owner.AnyAttribute()
				if !hasGroup || len(groups) != 1 || group.TargetID() != groups[0].ID() || group.Occurrences().String() != "1/2" || group.RefLoc() != complexContentTestLoc(t, root, `ref="r:Fields"`) || !hasBase || !hasBaseID || len(bases) != 1 || baseID != bases[0].ID() || base.Loc() != complexContentTestLoc(t, root, `base="o:Base"`) || !hasWildcard || wildcard.Loc() != complexContentTestSourceLoc(t, "other.xsd", other, `<xs:anyAttribute`) || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" {
					t.Fatalf("graph facts = %#v/%#v/%#v", group, base, wildcard)
				}
				uses := owner.AttributeUses()
				use, ok := uses[0].(LocalAttributeUse)
				ref, hasRef := use.TypeReference()
				if len(uses) != 1 || !ok || !hasRef || !ref.IsBuiltin() || use.TypeLoc() != complexContentTestLoc(t, root, `type="xs:`+family.name+`"`) {
					t.Fatalf("bounded use = %#v", uses)
				}
				assertAllIntegerBounds(t, ref, profile.version, family.minimum, family.maximum, Loc{}, Loc{})
			})
		}
	}
}

func TestGroupedBoundedBuiltinResolverCause(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		root := groupedExtensionSchema(string(profile.version), "", groupedBoundedUse(boundedLocalFamilies()[0]))
		root = strings.Replace(root, `<xs:element name="root"`, `<xs:include schemaLocation="missing.xsd"/><xs:element name="root"`, 1)
		source, sourceErr := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
		if sourceErr != nil {
			t.Fatal(sourceErr)
		}
		cause := errors.New("grouped bounded dependency unavailable")
		schema, err := ParseSchemaWithPolicy(source, &discoveryResolver{failures: map[string]error{"missing.xsd": cause}}, profile.policy)
		assertNCNameDiagnostic(t, schema, err, FailureResolution, SourceResolveCode, complexContentTestLoc(t, root, `<xs:include`), nil, "", cause)
	}
}

//nolint:gocognit // Each alternate exit precedes grouped publication and keeps its own cause.
func TestGroupedBoundedBuiltinAlternateExits(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			zero := groupedExtensionSchema(string(profile.version), ` minOccurs="0" maxOccurs="0"`, groupedBoundedUse(family)+`<xs:attribute name="gone" type="xs:`+family.name+`" use="prohibited"/>`)
			testGroupedLongExits(t, profile, zero, []groupedLongExit{
				{"missing group", `ref="t:Fields"`, `ref="t:Missing"`, `ref="t:Missing"`, "", diagnosticSchemaModelGroupReferenceUnresolvedCode, FailureInvalid, errSchemaModelGroupReferenceUnresolved, schemaModelGroupReferenceSpecRef},
				{"wrong group kind", `ref="t:Fields"`, `ref="t:root"`, `ref="t:root"`, `<xs:element name="root"`, diagnosticSchemaModelGroupReferenceWrongKindCode, FailureInvalid, errSchemaModelGroupReferenceWrongKind, schemaModelGroupReferenceSpecRef},
				{"wrong base kind", `base="t:Base"`, `base="t:Fields"`, `base="t:Fields"`, `<xs:group name="Fields"`, invalidSchemaCompositionCode, FailureInvalid, errSchemaComplexTypeBaseWrongKind, schemaComplexTypeExtensionSpecRef},
				{"used base", `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:attribute name="inherited" type="xs:boolean"/></xs:complexType>`, `base="t:Base"`, `<xs:complexType name="Base"`, UnsupportedSchemaSyntaxCode, FailureUnsupported, errSchemaComplexTypeBaseUnsupported, schemaComplexTypeExtensionSpecRef},
				{"nonempty base", `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:sequence><xs:element ref="t:root"/></xs:sequence></xs:complexType>`, `base="t:Base"`, `<xs:complexType name="Base"`, UnsupportedSchemaSyntaxCode, FailureUnsupported, errSchemaComplexTypeBaseNonEmpty, schemaComplexTypeExtensionSpecRef},
				{"final base", `<xs:complexType name="Base"/>`, `<xs:complexType name="Base" final="extension"/>`, `base="t:Base"`, `final="extension"`, invalidSchemaCompositionCode, FailureInvalid, errSchemaComplexTypeBaseUnsupported, schemaComplexTypeExtensionSpecRef},
				{"prohibited unresolved type", `type="xs:` + family.name + `" use="prohibited"`, `type="t:MissingType" use="prohibited"`, `type="t:MissingType"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, FailureInvalid, errSchemaAttributeTypeUnresolved, schemaAttributeTypeSpecRef},
				{"wrong type kind", `type="xs:` + family.name + `" use="prohibited"`, `type="t:Base" use="prohibited"`, `type="t:Base"`, `<xs:complexType name="Base"`, diagnosticSchemaAttributeTypeWrongKindCode, FailureInvalid, errSchemaAttributeTypeWrongKind, schemaAttributeTypeSpecRef},
			})
			for _, tc := range []struct {
				name, group, uses, marker, code, spec string
				cause                                 error
			}{
				{"invalid bounds", ` minOccurs="2" maxOccurs="1"`, groupedBoundedUse(family), `<xs:group ref="t:Fields"`, invalidSchemaCompositionCode, schemaParticleCorrectSpecRef(profile.version), errParticleOccurrenceMinimumExceedsMaximum},
				{"invalid use", "", `<xs:attribute name="a" type="xs:` + family.name + `" use="sometimes"/>`, `use="sometimes"`, invalidSchemaCompositionCode, schemaAttributeUseSpecRef(profile.version), nil},
				{"unresolved named type", "", `<xs:attribute name="a" type="t:Missing"/>`, `type="t:Missing"`, diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved},
			} {
				t.Run(profile.name+"/"+family.name+"/"+tc.name, func(t *testing.T) {
					root := groupedExtensionSchema(string(profile.version), tc.group, tc.uses)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					var related []Loc
					if tc.name == "invalid bounds" {
						related = []Loc{complexContentTestLoc(t, root, `minOccurs="2"`), complexContentTestLoc(t, root, `maxOccurs="1"`)}
					}
					if tc.cause == nil {
						assertZeroSchema(t, schema)
						d := requireDiagnostic(t, err)
						if d.Class() != FailureInvalid || d.Code() != tc.code || d.Loc() != complexContentTestLoc(t, root, tc.marker) || d.SpecRef() != tc.spec || !reflect.DeepEqual(d.Related(), related) {
							t.Fatalf("syntax diagnostic = %v related=%v", err, d.Related())
						}
						return
					}
					assertNCNameDiagnostic(t, schema, err, FailureInvalid, tc.code, complexContentTestLoc(t, root, tc.marker), related, tc.spec, tc.cause)
				})
			}
			missingGroupAndType := strings.Replace(zero, `ref="t:Fields"`, `ref="t:Missing"`, 1)
			missingGroupAndType = strings.Replace(missingGroupAndType, `type="xs:`+family.name+`" use="required"`, `type="t:MissingType" use="required"`, 1)
			failed, err := discoverTestSchemaWithPolicy(t, missingGroupAndType, nil, profile.policy)
			assertNCNameDiagnostic(t, failed, err, FailureInvalid, diagnosticSchemaModelGroupReferenceUnresolvedCode, complexContentTestLoc(t, missingGroupAndType, `ref="t:Missing"`), nil, schemaModelGroupReferenceSpecRef(profile.version), errSchemaModelGroupReferenceUnresolved)
			missingTypeAndBase := strings.Replace(zero, `base="t:Base"`, `base="t:MissingBase"`, 1)
			missingTypeAndBase = strings.Replace(missingTypeAndBase, `type="xs:`+family.name+`" use="required"`, `type="t:MissingType" use="required"`, 1)
			failed, err = discoverTestSchemaWithPolicy(t, missingTypeAndBase, nil, profile.policy)
			assertNCNameDiagnostic(t, failed, err, FailureInvalid, diagnosticSchemaAttributeTypeUnresolvedCode, complexContentTestLoc(t, missingTypeAndBase, `type="t:MissingType"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved)
			cycle := strings.Replace(zero, `<xs:group name="Fields"><xs:sequence/></xs:group>`, `<xs:group name="Fields"><xs:sequence><xs:group ref="t:Fields"/></xs:sequence></xs:group>`, 1)
			failed, err = discoverTestSchemaWithPolicy(t, cycle, nil, profile.policy)
			cycleSpec := "xsd11-structures#mg-props-correct"
			if profile.version == XSDVersion10 {
				cycleSpec = "xsd10-structures#mg-props-correct"
			}
			assertNCNameDiagnostic(t, failed, err, FailureInvalid, diagnosticSchemaModelGroupReferenceCycleCode, allParticleTestTokenLoc(t, "root.xsd", cycle, `ref="t:Fields"`, 2), []Loc{complexContentTestLoc(t, cycle, `<xs:group name="Fields"`)}, cycleSpec, errSchemaModelGroupReferenceCycle)
			baseCycle := strings.Replace(zero, `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:complexContent><xs:extension base="t:Derived"><xs:group ref="t:Fields"/><xs:attribute name="other" type="xs:integer"/></xs:extension></xs:complexContent></xs:complexType>`, 1)
			failed, err = discoverTestSchemaWithPolicy(t, baseCycle, nil, profile.policy)
			assertNCNameDiagnostic(t, failed, err, FailureInvalid, invalidSchemaCompositionCode, complexContentTestLoc(t, baseCycle, `base="t:Derived"`), []Loc{complexContentTestLoc(t, baseCycle, `base="t:Base"`)}, schemaComplexTypeExtensionSpecRef(profile.version), errSchemaComplexTypeBaseCycle)
		}
	}
}

//nolint:gocognit // Compare both inert labels for every public policy and admitted type.
func TestGroupedBoundedBuiltinEditionLabelIsInert(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, label := range []string{"1.0", "1.1"} {
			for _, family := range boundedLocalFamilies() {
				t.Run(profile.name+"/"+label+"/"+family.name, func(t *testing.T) {
					root := groupedExtensionSchema(label, "", groupedBoundedUse(family))
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Derived")
					use, ok := owner.AttributeUses()[0].(LocalAttributeUse)
					if !ok {
						t.Fatalf("admitted use = %T", owner.AttributeUses()[0])
					}
					ref, ok := use.TypeReference()
					if !ok || !ref.IsBuiltin() {
						t.Fatalf("admitted type = %#v", ref)
					}
					assertAllIntegerBounds(t, ref, profile.version, family.minimum, family.maximum, Loc{}, Loc{})
				})
			}
		}
	}
}

func TestGroupedBoundedBuiltinVisibilityAndDuplicate(t *testing.T) {
	bridge := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge"><xs:import namespace="urn:foreign" schemaLocation="foreign.xsd"/></xs:schema>`
	foreign := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:foreign"><xs:group name="Fields"><xs:sequence/></xs:group><xs:complexType name="Base"/><xs:simpleType name="Bounded"><xs:restriction base="xs:short"/></xs:simpleType></xs:schema>`
	fixtures := map[string]discoveryFixture{"bridge.xsd": {id: "bridge.xsd", contents: bridge}, "foreign.xsd": {id: "foreign.xsd", contents: foreign}}
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			for _, tc := range []struct {
				name, base, group, use, marker, related, code, spec string
				cause                                               error
			}{
				{"hidden group", "xs:anyType", "f:Fields", groupedBoundedUse(family), `ref="f:Fields"`, `<xs:group name="Fields"`, diagnosticSchemaModelGroupReferenceNamespaceCode, schemaModelGroupReferenceImportSpecRef(profile.version), errSchemaModelGroupReferenceNamespace},
				{"hidden base", "f:Base", "r:Fields", groupedBoundedUse(family), `base="f:Base"`, `<xs:complexType name="Base"`, invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(profile.version), errSchemaComplexTypeBaseUnresolved},
				{"hidden type", "xs:anyType", "r:Fields", `<xs:attribute name="a" type="f:Bounded"/>`, `type="f:Bounded"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved},
			} {
				t.Run(profile.name+"/"+family.name+"/"+tc.name, func(t *testing.T) {
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:f="urn:foreign" targetNamespace="urn:root" version="` + string(profile.version) + `"><xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/><xs:group name="Fields"><xs:sequence/></xs:group><xs:complexType name="Derived"><xs:complexContent><xs:extension base="` + tc.base + `"><xs:group ref="` + tc.group + `"/>` + tc.use + `</xs:extension></xs:complexContent></xs:complexType></xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
					var related []Loc
					if tc.related != "" {
						related = []Loc{complexContentTestSourceLoc(t, "foreign.xsd", foreign, tc.related)}
					}
					assertNCNameDiagnostic(t, schema, err, FailureInvalid, tc.code, complexContentTestLoc(t, root, tc.marker), related, tc.spec, tc.cause)
				})
			}
			duplicate := groupedExtensionSchema(string(profile.version), "", groupedBoundedUse(family))
			duplicate = strings.Replace(duplicate, `</xs:schema>`, `<xs:group name="Fields"><xs:sequence/></xs:group></xs:schema>`, 1)
			schema, err := discoverTestSchemaWithPolicy(t, duplicate, nil, profile.policy)
			assertNCNameDiagnostic(t, schema, err, FailureInvalid, diagnosticSchemaGlobalDuplicateCode, allParticleTestTokenLoc(t, "root.xsd", duplicate, `<xs:group name="Fields"`, 2), []Loc{complexContentTestLoc(t, duplicate, `<xs:group name="Fields"`)}, schemaGlobalDuplicateSpecRef(profile.version), errSchemaGlobalDeclarationDuplicate)
		}
	}
}
