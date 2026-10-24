package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func groupedTokenUse(kind string) string {
	return `<xs:attribute name="` + kind + `" type="xs:` + kind + `" use="required" form="unqualified"/>`
}

//nolint:gocognit // The one public view owns ordered, copied group, base, and use facts.
func TestGroupedTokenAttributePublicFacts(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kinds := range [][]string{{"token"}, {"NMTOKEN"}, {"token", "NMTOKEN"}} {
			t.Run(profile.name+"/"+strings.Join(kinds, "+"), func(t *testing.T) {
				var uses strings.Builder
				uses.WriteString(`<xs:attribute name="before" type="xs:boolean"/>`)
				for _, kind := range kinds {
					uses.WriteString(groupedTokenUse(kind))
				}
				uses.WriteString(`<xs:attribute name="gone" type="xs:token" use="prohibited"/><xs:attribute name="after" type="xs:integer"/>`)
				root := groupedExtensionSchema("1.0", ` minOccurs="2" maxOccurs="3"`, uses.String())
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				if schema.LanguagePolicy() != profile.policy || len(schema.Documents()) != 1 || schema.Documents()[0].Source() != "root.xsd" {
					t.Fatalf("policy/documents = %s/%v", schema.LanguagePolicy(), schema.Documents())
				}
				owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Derived")
				if owner.Loc() != complexContentTestLoc(t, root, `<xs:complexType name="Derived"`) || owner.Derivation() != ComplexTypeDerivationExtension || owner.DerivationLoc() != complexContentTestLoc(t, root, `<xs:extension`) || owner.Base() != mustTestQName(t, "urn:root", "Base") || owner.BaseLoc() != complexContentTestLoc(t, root, `base="t:Base"`) {
					t.Fatalf("owner/base = %#v", owner)
				}
				base, hasBase := owner.BaseReference()
				bases := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Base"))
				baseID, hasBaseID := base.ComponentID()
				if !hasBase || !hasBaseID || len(bases) != 1 || baseID != bases[0].ID() || base.Loc() != owner.BaseLoc() {
					t.Fatalf("base reference = %#v", base)
				}
				group, hasGroup := owner.Particle().(ModelGroupReferenceParticle)
				groups := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, "urn:root", "Fields"))
				if !hasGroup || len(groups) != 1 || group.Ref() != mustTestQName(t, "urn:root", "Fields") || group.TargetID() != groups[0].ID() || group.Loc() != complexContentTestLoc(t, root, `<xs:group ref="t:Fields"`) || group.RefLoc() != complexContentTestLoc(t, root, `ref="t:Fields"`) || group.Occurrences().String() != "2/3" {
					t.Fatalf("group = %#v", owner.Particle())
				}
				attributes := owner.AttributeUses()
				if len(attributes) != len(kinds)+2 || attributes[0].Name().Local() != "before" || attributes[len(attributes)-1].Name().Local() != "after" {
					t.Fatalf("ordered uses = %#v", attributes)
				}
				for index, kind := range kinds {
					use, ok := attributes[index+1].(LocalAttributeUse)
					wantType := mustTestQName(t, testXSDNamespace, kind)
					wantTypeLoc := complexContentTestLoc(t, root, `type="xs:`+kind+`"`)
					if !ok || use.Name() != mustTestQName(t, "", kind) || use.DeclaredType() != wantType || use.Loc() != complexContentTestLoc(t, root, `<xs:attribute name="`+kind+`"`) || use.NameLoc() != complexContentTestLoc(t, root, `name="`+kind+`"`) || use.TypeLoc() != wantTypeLoc || use.Use() != AttributeUseRequired || use.UseLoc() != allParticleTestTokenLoc(t, "root.xsd", root, `use="required"`, index+1) || use.FormLoc() != allParticleTestTokenLoc(t, "root.xsd", root, `form="unqualified"`, index+1) {
						t.Fatalf("%s use = %#v", kind, attributes[index+1])
					}
					ref, hasRef := use.TypeReference()
					id, hasID := use.TypeID()
					refID, hasRefID := ref.ComponentID()
					white, hasWhite := ref.StringWhiteSpaceFacet()
					if !hasRef || !ref.IsBuiltin() || ref.Kind() != SimpleTypeReferenceBuiltin || ref.Name() != wantType || ref.QName() != wantType || ref.Loc() != wantTypeLoc || ref.Variety() != SimpleTypeVarietyAtomicRestriction || !hasWhite || white.Value() != "collapse" || !white.Fixed() || !white.Loc().IsZero() || ref.StringEnumerationFacets().HasEnumeration() || hasID || hasRefID || !id.IsZero() || !refID.IsZero() {
						t.Fatalf("%s type = %#v/%v/%v", kind, ref, id, refID)
					}
				}
				attributes[0] = nil
				components := schema.Components()
				components[0] = Component{}
				retainedGroup, hasRetainedGroup := owner.Particle().(ModelGroupReferenceParticle)
				if owner.AttributeUses()[0] == nil || schema.Components()[0].Name().Local() != "root" || !hasRetainedGroup || retainedGroup.Occurrences().String() != "2/3" {
					t.Fatal("caller changed published facts")
				}
				var walks [2][]ComponentID
				for i := range walks {
					if err := schema.Walk(func(component Component) error { walks[i] = append(walks[i], component.ID()); return nil }); err != nil {
						t.Fatal(err)
					}
				}
				if !reflect.DeepEqual(walks[0], walks[1]) || !reflect.DeepEqual(walks[0], componentIDs(schema.Components())) {
					t.Fatalf("walks = %v/%v", walks[0], walks[1])
				}
				controlRoot := root
				for _, kind := range kinds {
					controlRoot = strings.Replace(controlRoot, groupedTokenUse(kind), "", 1)
				}
				controlRoot = strings.Replace(controlRoot, `<xs:attribute name="gone" type="xs:token" use="prohibited"/>`, "", 1)
				control, controlErr := discoverTestSchemaWithPolicy(t, controlRoot, nil, profile.policy)
				if controlErr != nil {
					t.Fatal(controlErr)
				}
				controlOwner := modelGroupReferenceTestComplexTypeNamed(t, control, "urn:root", "Derived")
				controlGroup, hasControlGroup := controlOwner.Particle().(ModelGroupReferenceParticle)
				controlBase, _ := controlOwner.BaseReference()
				controlBaseID, _ := controlBase.ComponentID()
				if !hasControlGroup || !reflect.DeepEqual(componentIDs(control.Components()), componentIDs(schema.Components())) || controlGroup.TargetID() != group.TargetID() || controlGroup.Occurrences().String() != group.Occurrences().String() || controlBaseID != baseID || len(controlOwner.AttributeUses()) != 2 {
					t.Fatalf("control changed identities/bounds = %#v", controlOwner)
				}
			})
		}
	}
}

//nolint:gocognit // The two queryable built-ins keep separate consumer diagnostics.
func TestGroupedTokenAttributeConsumersUnsupported(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			t.Run(profile.name+"/"+kind, func(t *testing.T) {
				root := groupedExtensionSchema("1.1", "", groupedTokenUse(kind))
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				useLoc := complexContentTestLoc(t, root, `<xs:attribute name="`+kind+`"`)
				ownerLoc := complexContentTestLoc(t, root, `<xs:complexType name="Derived"`)
				rootLoc := complexContentTestLoc(t, root, `<xs:element name="root"`)
				instanceErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
				vd := requireDiagnostic(t, instanceErr)
				if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != useLoc || vd.SpecRef() != instanceValidationSpecRef(profile.version) || !reflect.DeepEqual(vd.Related(), []Loc{rootLoc, ownerLoc, useLoc}) || !errors.Is(instanceErr, errInstanceAttributes) {
					t.Fatalf("validation = %v", instanceErr)
				}
				output, generationErr := GenerateGo(schema, "generated")
				gd := requireDiagnostic(t, generationErr)
				if output != nil || gd.Class() != FailureUnsupported || gd.Code() != diagnosticCodegenUnsupported || gd.Loc() != useLoc || gd.SpecRef() != codegenDirectSequenceSpecReference(profile.version, codegenDirectSequenceParticlesReference) || !reflect.DeepEqual(gd.Related(), []Loc{ownerLoc, useLoc}) || !errors.Is(generationErr, errCodegenUnsupported) {
					t.Fatalf("generation = %v/%v", output, generationErr)
				}
			})
		}
	}
}

//nolint:gocognit // Each excluded type/use form has an independent located parse result.
func TestGroupedTokenAttributeExcludedForms(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			alias := `<xs:simpleType name="Alias"><xs:restriction base="xs:` + kind + `"/></xs:simpleType>`
			inline := `<xs:simpleType><xs:restriction base="xs:` + kind + `"/></xs:simpleType>`
			for _, test := range []struct {
				name, use, extra, marker, related string
				useSpec                           bool
				cause                             error
			}{
				{"named alias", `<xs:attribute name="a" type="t:Alias"/>`, alias, `type="t:Alias"`, "", false, errSchemaAttributeUseUnsupported},
				{"inline atomic", `<xs:attribute name="a">` + inline + `</xs:attribute>`, "", `<xs:simpleType`, "", false, errSchemaAttributeTypeUnsupported},
				{"inline list", `<xs:attribute name="a"><xs:simpleType><xs:list itemType="xs:` + kind + `"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, "", false, errSchemaAttributeTypeUnsupported},
				{"inline union", `<xs:attribute name="a"><xs:simpleType><xs:union memberTypes="xs:` + kind + ` xs:` + kind + `"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, "", false, errSchemaAttributeTypeUnsupported},
				{"ref builtin", `<xs:attribute ref="t:a"/>`, `<xs:attribute name="a" type="xs:` + kind + `"/>`, `ref="t:a"`, `<xs:attribute name="a"`, true, errSchemaAttributeReferenceUnsupported},
				{"ref named", `<xs:attribute ref="t:a"/>`, `<xs:attribute name="a" type="t:Alias"/>` + alias, `ref="t:a"`, `<xs:attribute name="a"`, true, errSchemaAttributeReferenceUnsupported},
				{"ref inline", `<xs:attribute ref="t:a"/>`, `<xs:attribute name="a">` + inline + `</xs:attribute>`, `ref="t:a"`, `<xs:attribute name="a"`, true, errSchemaAttributeReferenceUnsupported},
				{"direct default", `<xs:attribute name="a" type="xs:` + kind + `" default="one"/>`, "", `default="one"`, "", true, errSchemaAttributeUseUnsupported},
				{"direct fixed", `<xs:attribute name="a" type="xs:` + kind + `" fixed="one"/>`, "", `fixed="one"`, "", true, errSchemaAttributeUseUnsupported},
				{"named default", `<xs:attribute name="a" type="t:Alias" default="one"/>`, alias, `default="one"`, "", true, errSchemaAttributeUseUnsupported},
				{"named fixed", `<xs:attribute name="a" type="t:Alias" fixed="one"/>`, alias, `fixed="one"`, "", true, errSchemaAttributeUseUnsupported},
				{"inline default", `<xs:attribute name="a" default="one">` + inline + `</xs:attribute>`, "", `default="one"`, "", true, errSchemaAttributeUseUnsupported},
				{"inline fixed", `<xs:attribute name="a" fixed="one">` + inline + `</xs:attribute>`, "", `fixed="one"`, "", true, errSchemaAttributeUseUnsupported},
			} {
				if test.name == "named alias" {
					continue
				}
				t.Run(profile.name+"/"+kind+"/"+test.name, func(t *testing.T) {
					root := groupedExtensionSchema("1.1", "", groupedTokenUse(kind)+test.use)
					root = strings.Replace(root, `</xs:schema>`, test.extra+`</xs:schema>`, 1)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					spec := schemaAttributeTypeSpecRef(profile.version)
					if test.useSpec {
						spec = schemaAttributeUseSpecRef(profile.version)
					}
					if strings.Contains(test.name, "default") || strings.Contains(test.name, "fixed") {
						spec = schemaComplexTypeExtensionSpecRef(profile.version)
					}
					marker, cause := test.marker, test.cause
					if kind == "NMTOKEN" && strings.HasPrefix(test.name, "ref ") {
						marker, spec, cause = `type="xs:NMTOKEN"`, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnsupported
						if test.name == "ref named" {
							marker = `type="t:Alias"`
						}
						if test.name == "ref inline" {
							marker = `<xs:simpleType`
						}
					}
					var related []Loc
					if test.related != "" && kind != "NMTOKEN" {
						related = []Loc{complexContentTestLoc(t, root, test.related)}
					}
					loc := complexContentTestLoc(t, root, marker)
					if kind == "NMTOKEN" && test.name == "ref builtin" {
						loc = allParticleTestTokenLoc(t, "root.xsd", root, marker, 2)
					}
					assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, loc, related, spec, cause)
				})
			}
		}
	}
}

//nolint:gocognit // The bounded group, use, and base exits retain their own causes and locations.
func TestGroupedTokenAttributeOmissionAndAlternateExits(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			t.Run(profile.name+"/"+kind, func(t *testing.T) {
				zero := groupedExtensionSchema("1.1", ` minOccurs="0" maxOccurs="0"`, groupedTokenUse(kind)+`<xs:attribute name="gone" type="xs:`+kind+`" use="prohibited"/>`)
				schema, err := discoverTestSchemaWithPolicy(t, zero, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Derived")
				if owner.Particle() != nil || len(owner.AttributeUses()) != 1 || owner.AttributeUses()[0].Name().Local() != kind {
					t.Fatalf("zero group/prohibited use = %#v/%#v", owner.Particle(), owner.AttributeUses())
				}
				for _, test := range []struct {
					name, from, to, marker, related string
					class                           FailureClass
					code                            string
					cause                           error
				}{
					{"unresolved group", `ref="t:Fields"`, `ref="t:Missing"`, `ref="t:Missing"`, "", FailureInvalid, diagnosticSchemaModelGroupReferenceUnresolvedCode, errSchemaModelGroupReferenceUnresolved},
					{"wrong group kind", `ref="t:Fields"`, `ref="t:root"`, `ref="t:root"`, `<xs:element name="root"`, FailureInvalid, diagnosticSchemaModelGroupReferenceWrongKindCode, errSchemaModelGroupReferenceWrongKind},
					{"wrong base kind", `base="t:Base"`, `base="t:Fields"`, `base="t:Fields"`, `<xs:group name="Fields"`, FailureInvalid, invalidSchemaCompositionCode, errSchemaComplexTypeBaseWrongKind},
					{"nonempty base", `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:sequence><xs:element ref="t:root"/></xs:sequence></xs:complexType>`, `base="t:Base"`, `<xs:complexType name="Base"`, FailureUnsupported, UnsupportedSchemaSyntaxCode, errSchemaComplexTypeBaseNonEmpty},
					{"used base", `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:attribute name="inherited" type="xs:boolean"/></xs:complexType>`, `base="t:Base"`, `<xs:complexType name="Base"`, FailureUnsupported, UnsupportedSchemaSyntaxCode, errSchemaComplexTypeBaseUnsupported},
					{"grouped base", `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:group ref="t:Fields"/></xs:complexType>`, `base="t:Base"`, `<xs:complexType name="Base"`, FailureUnsupported, UnsupportedSchemaSyntaxCode, errSchemaComplexTypeBaseNonEmpty},
					{"simple content base", `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:simpleContent><xs:extension base="xs:token"/></xs:simpleContent></xs:complexType>`, `base="t:Base"`, `<xs:complexType name="Base"`, FailureUnsupported, UnsupportedSchemaSyntaxCode, errSchemaComplexTypeBaseUnsupported},
					{"final base", `<xs:complexType name="Base"/>`, `<xs:complexType name="Base" final="extension"/>`, `base="t:Base"`, `final="extension"`, FailureInvalid, invalidSchemaCompositionCode, errSchemaComplexTypeBaseUnsupported},
					{"prohibited unresolved type", `type="xs:` + kind + `" use="prohibited"`, `type="t:MissingType" use="prohibited"`, `type="t:MissingType"`, "", FailureInvalid, diagnosticSchemaAttributeTypeUnresolvedCode, errSchemaAttributeTypeUnresolved},
					{"invalid use", `use="required"`, `use="sometimes"`, `use="sometimes"`, "", FailureInvalid, invalidSchemaCompositionCode, nil},
				} {
					t.Run(test.name, func(t *testing.T) {
						root := strings.Replace(zero, test.from, test.to, 1)
						failed, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
						spec := schemaComplexTypeExtensionSpecRef(profile.version)
						var related []Loc
						switch test.name {
						case "unresolved group", "wrong group kind":
							spec = schemaModelGroupReferenceSpecRef(profile.version)
						case "prohibited unresolved type":
							spec = schemaAttributeTypeSpecRef(profile.version)
						case "invalid use":
							spec = schemaAttributeUseSpecRef(profile.version)
						}
						if test.related != "" {
							related = []Loc{complexContentTestLoc(t, root, test.related)}
						}
						if test.name == "nonempty base" {
							related = append(related, allParticleTestTokenLoc(t, "root.xsd", root, `<xs:sequence`, 2))
						}
						if test.name == "grouped base" {
							related = append(related, allParticleTestTokenLoc(t, "root.xsd", root, `<xs:group ref="t:Fields"`, 2))
						}
						if test.cause == nil {
							assertZeroSchema(t, failed)
							d := requireDiagnostic(t, err)
							if d.Class() != test.class || d.Code() != test.code || d.Loc() != complexContentTestLoc(t, root, test.marker) || d.SpecRef() != spec || !reflect.DeepEqual(d.Related(), related) {
								t.Fatalf("%s = %v", test.name, err)
							}
							return
						}
						assertNCNameDiagnostic(t, failed, err, test.class, test.code, complexContentTestLoc(t, root, test.marker), related, spec, test.cause)
					})
				}
			})
		}
	}
}

//nolint:gocognit // Discovery order, chameleon adoption, and imported base/group identities are public.
func TestGroupedTokenAttributeGraphProvenance(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root"><xs:include schemaLocation="shared.xsd"/><xs:include schemaLocation="again.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/><xs:complexType name="Derived"><xs:complexContent><xs:extension base="o:Base"><xs:group ref="r:Fields" minOccurs="1" maxOccurs="2"/>` + groupedTokenUse("token") + groupedTokenUse("NMTOKEN") + `</xs:extension></xs:complexContent></xs:complexType></xs:schema>`
	shared := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other"><xs:include schemaLocation="root.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/><xs:group name="Fields"><xs:sequence><xs:element ref="o:item"/></xs:sequence></xs:group></xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:element name="item" type="xs:integer"/><xs:complexType name="Base"/></xs:schema>`
	fixtures := map[string]discoveryFixture{
		"root.xsd": {id: "root.xsd", contents: root}, "shared.xsd": {id: "shared.xsd", contents: shared},
		"again.xsd": {id: "shared.xsd", contents: shared}, "other.xsd": {id: "other.xsd", contents: other},
	}
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			var sources []SourceID
			for _, document := range schema.Documents() {
				sources = append(sources, document.Source())
			}
			if !reflect.DeepEqual(sources, []SourceID{"root.xsd", "shared.xsd", "other.xsd"}) {
				t.Fatalf("documents = %v", sources)
			}
			owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Derived")
			groups := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, "urn:root", "Fields"))
			bases := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:other", "Base"))
			group, hasGroup := owner.Particle().(ModelGroupReferenceParticle)
			base, hasBase := owner.BaseReference()
			baseID, hasBaseID := base.ComponentID()
			if !hasGroup || len(groups) != 1 || group.TargetID() != groups[0].ID() || group.RefLoc() != complexContentTestLoc(t, root, `ref="r:Fields"`) || group.Occurrences().String() != "1/2" || !hasBase || !hasBaseID || len(bases) != 1 || baseID != bases[0].ID() || owner.BaseLoc() != complexContentTestLoc(t, root, `base="o:Base"`) {
				t.Fatalf("graph group/base = %#v/%#v", group, base)
			}
			uses := owner.AttributeUses()
			if len(uses) != 2 || uses[0].Name() != mustTestQName(t, "", "token") || uses[1].Name() != mustTestQName(t, "", "NMTOKEN") || uses[0].Loc() != complexContentTestLoc(t, root, `<xs:attribute name="token"`) || uses[1].Loc() != complexContentTestLoc(t, root, `<xs:attribute name="NMTOKEN"`) {
				t.Fatalf("graph uses = %#v", uses)
			}
			var walked []ComponentID
			if err := schema.Walk(func(component Component) error { walked = append(walked, component.ID()); return nil }); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(walked, componentIDs(schema.Components())) {
				t.Fatalf("walk = %v", walked)
			}
		})
	}
}

func TestGroupedTokenAttributeGraphDiagnostics(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			t.Run(profile.name+"/"+kind, func(t *testing.T) {
				root := groupedExtensionSchema("1.1", "", groupedTokenUse(kind))
				groupCycle := strings.Replace(root, `<xs:group name="Fields"><xs:sequence/></xs:group>`, `<xs:group name="Fields"><xs:sequence><xs:group ref="t:Fields"/></xs:sequence></xs:group>`, 1)
				failed, err := discoverTestSchemaWithPolicy(t, groupCycle, nil, profile.policy)
				cycleSpec := "xsd11-structures#mg-props-correct"
				if profile.version == XSDVersion10 {
					cycleSpec = "xsd10-structures#mg-props-correct"
				}
				assertNCNameDiagnostic(t, failed, err, FailureInvalid, diagnosticSchemaModelGroupReferenceCycleCode, allParticleTestTokenLoc(t, "root.xsd", groupCycle, `ref="t:Fields"`, 2), []Loc{complexContentTestLoc(t, groupCycle, `<xs:group name="Fields"`)}, cycleSpec, errSchemaModelGroupReferenceCycle)
				baseCycle := strings.Replace(root, `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:complexContent><xs:extension base="t:Derived"><xs:group ref="t:Fields"/><xs:attribute name="other" type="xs:integer"/></xs:extension></xs:complexContent></xs:complexType>`, 1)
				failed, err = discoverTestSchemaWithPolicy(t, baseCycle, nil, profile.policy)
				assertNCNameDiagnostic(t, failed, err, FailureInvalid, invalidSchemaCompositionCode, complexContentTestLoc(t, baseCycle, `base="t:Derived"`), []Loc{complexContentTestLoc(t, baseCycle, `base="t:Base"`)}, schemaComplexTypeExtensionSpecRef(profile.version), errSchemaComplexTypeBaseCycle)
			})
		}
	}
}

func TestGroupedTokenAttributeVisibilityAndWildcard(t *testing.T) {
	bridge := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge"><xs:import namespace="urn:foreign" schemaLocation="foreign.xsd"/></xs:schema>`
	foreign := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:foreign"><xs:group name="Fields"><xs:sequence/></xs:group><xs:complexType name="Base"/></xs:schema>`
	fixtures := map[string]discoveryFixture{"bridge.xsd": {id: "bridge.xsd", contents: bridge}, "foreign.xsd": {id: "foreign.xsd", contents: foreign}}
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			for _, test := range []struct {
				name, base, group, marker, related, code, spec string
				cause                                          error
			}{
				{"hidden group", `xs:anyType`, `f:Fields`, `ref="f:Fields"`, `<xs:group name="Fields"`, diagnosticSchemaModelGroupReferenceNamespaceCode, schemaModelGroupReferenceImportSpecRef(profile.version), errSchemaModelGroupReferenceNamespace},
				{"hidden base", `f:Base`, `r:Fields`, `base="f:Base"`, `<xs:complexType name="Base"`, invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(profile.version), errSchemaComplexTypeBaseUnresolved},
			} {
				t.Run(profile.name+"/"+kind+"/"+test.name, func(t *testing.T) {
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:f="urn:foreign" targetNamespace="urn:root"><xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/><xs:group name="Fields"><xs:sequence/></xs:group><xs:complexType name="Derived"><xs:complexContent><xs:extension base="` + test.base + `"><xs:group ref="` + test.group + `"/>` + groupedTokenUse(kind) + `</xs:extension></xs:complexContent></xs:complexType></xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
					assertNCNameDiagnostic(t, schema, err, FailureInvalid, test.code, complexContentTestLoc(t, root, test.marker), []Loc{complexContentTestSourceLoc(t, "foreign.xsd", foreign, test.related)}, test.spec, test.cause)
				})
			}
			wildcard := groupedExtensionSchema("1.1", "", groupedTokenUse(kind))
			wildcard = strings.Replace(wildcard, `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>`, 1)
			schema, err := discoverTestSchemaWithPolicy(t, wildcard, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Derived")
			inherited, hasInherited := owner.AnyAttribute()
			if !hasInherited || inherited.Namespace() != "##other" || inherited.ProcessContents() != "lax" || inherited.Loc() != complexContentTestLoc(t, wildcard, `<xs:anyAttribute`) || len(owner.AttributeUses()) != 1 {
				t.Fatalf("%s wildcard = %#v", kind, inherited)
			}
		}
	}
}

//nolint:gocognit // Syntax, policy, and bounds are checked before grouped type admission.
func TestGroupedTokenAttributeSyntaxAndResolution(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			for _, test := range []struct {
				name, group, use, marker, code, spec string
				class                                FailureClass
			}{
				{"invalid use", "", `<xs:attribute name="a" type="xs:` + kind + `" use="sometimes"/>`, `use="sometimes"`, invalidSchemaCompositionCode, schemaAttributeUseSpecRef(profile.version), FailureInvalid},
				{"invalid bounds", ` minOccurs="2" maxOccurs="1"`, groupedTokenUse(kind), `<xs:group ref="t:Fields"`, invalidSchemaCompositionCode, schemaParticleCorrectSpecRef(profile.version), FailureInvalid},
				{"duplicate model", "", `<xs:group ref="t:Fields"/>` + groupedTokenUse(kind), `<xs:group ref="t:Fields"`, invalidSchemaCompositionCode, schemaComplexContentExtensionSpecRef(profile.version), FailureInvalid},
			} {
				t.Run(profile.name+"/"+kind+"/"+test.name, func(t *testing.T) {
					root := groupedExtensionSchema("1.1", test.group, test.use)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					loc := complexContentTestLoc(t, root, test.marker)
					if test.name == "duplicate model" {
						loc = allParticleTestTokenLoc(t, "root.xsd", root, test.marker, 2)
					}
					var related []Loc
					if test.name == "invalid bounds" {
						related = []Loc{complexContentTestLoc(t, root, `minOccurs="2"`), complexContentTestLoc(t, root, `maxOccurs="1"`)}
						if !errors.Is(err, errParticleOccurrenceMinimumExceedsMaximum) {
							t.Fatalf("occurrence cause = %v", err)
						}
					}
					if d.Class() != test.class || d.Code() != test.code || d.Loc() != loc || d.SpecRef() != test.spec || !reflect.DeepEqual(d.Related(), related) {
						t.Fatalf("%s = %v; got code=%s loc=%s spec=%s related=%v; want code=%s loc=%s spec=%s", test.name, err, d.Code(), d.Loc(), d.SpecRef(), d.Related(), test.code, loc, test.spec)
					}
				})
			}
			if profile.policy == Strict10 {
				root := groupedExtensionSchema("1.1", "", `<xs:attribute name="a" type="xs:`+kind+`" inheritable="false"/>`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != complexContentTestLoc(t, root, `inheritable="false"`) || d.SpecRef() != "xsd11-structures#cSchemaDocument" || len(d.Related()) != 0 || !errors.Is(err, errLanguagePolicyMismatch) {
					t.Fatalf("Strict10 inheritable = %v", err)
				}
			}
			root := groupedExtensionSchema("1.1", "", groupedTokenUse(kind))
			root = strings.Replace(root, `<xs:element name="root"`, `<xs:include schemaLocation="missing.xsd"/><xs:element name="root"`, 1)
			source, sourceErr := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
			if sourceErr != nil {
				t.Fatal(sourceErr)
			}
			cause := errors.New("grouped token dependency unavailable")
			schema, err := ParseSchemaWithPolicy(source, &discoveryResolver{failures: map[string]error{"missing.xsd": cause}}, profile.policy)
			assertNCNameDiagnostic(t, schema, err, FailureResolution, SourceResolveCode, complexContentTestLoc(t, root, `<xs:include`), nil, "", cause)
		}
	}
}
