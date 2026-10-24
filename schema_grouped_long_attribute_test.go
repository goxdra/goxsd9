package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

const groupedLongUse = `<xs:attribute name="long" type="xs:long" use="required" form="unqualified"/>`

//nolint:gocognit,funlen // This checks the complete public group, use, and base view.
func TestGroupedLongPublicFacts(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, mixed := range []bool{false, true} {
			name := profile.name + "/alone"
			uses := groupedLongUse
			if mixed {
				name = profile.name + "/mixed"
				uses = `<xs:attribute name="before" type="xs:boolean"/>` + uses + `<xs:attribute name="gone" type="xs:long" use="prohibited"/><xs:attribute name="after" type="xs:integer"/>`
			}
			t.Run(name, func(t *testing.T) {
				root := groupedExtensionSchema(string(profile.version), ` minOccurs="2" maxOccurs="3"`, uses)
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
				if !hasGroup || len(groups) != 1 || group.Ref() != mustTestQName(t, "urn:root", "Fields") || group.TargetID() != groups[0].ID() || group.Loc() != complexContentTestLoc(t, root, `<xs:group ref="t:Fields"`) || group.RefLoc() != complexContentTestLoc(t, root, `ref="t:Fields"`) || group.Occurrences().String() != "2/3" {
					t.Fatalf("group = %#v", owner.Particle())
				}
				attributes := owner.AttributeUses()
				longIndex, wantCount := 0, 1
				if mixed {
					longIndex, wantCount = 1, 3
				}
				if len(attributes) != wantCount || attributes[longIndex].Name().Local() != "long" {
					t.Fatalf("uses = %#v", attributes)
				}
				if mixed && (attributes[0].Name().Local() != "before" || attributes[2].Name().Local() != "after") {
					t.Fatalf("use order = %#v", attributes)
				}
				use, ok := attributes[longIndex].(LocalAttributeUse)
				if !ok || use.Name() != mustTestQName(t, "", "long") || use.Loc() != complexContentTestLoc(t, root, `<xs:attribute name="long"`) || use.NameLoc() != complexContentTestLoc(t, root, `name="long"`) || use.TypeLoc() != complexContentTestLoc(t, root, `type="xs:long"`) || use.Use() != AttributeUseRequired || use.UseLoc() != complexContentTestLoc(t, root, `use="required"`) || use.FormLoc() != complexContentTestLoc(t, root, `form="unqualified"`) {
					t.Fatalf("long use = %#v", attributes[longIndex])
				}
				ref, hasRef := use.TypeReference()
				id, hasID := use.TypeID()
				refID, hasRefID := ref.ComponentID()
				if !hasRef || !ref.IsBuiltin() || ref.Variety() != SimpleTypeVarietyAtomicRestriction || use.DeclaredType() != mustTestQName(t, testXSDNamespace, "long") || ref.Name() != use.DeclaredType() || ref.QName() != use.DeclaredType() || ref.Loc() != use.TypeLoc() || hasID || hasRefID || !id.IsZero() || !refID.IsZero() {
					t.Fatalf("long type = %#v/%v/%v", ref, id, refID)
				}
				assertAllIntegerBounds(t, ref, profile.version, "-9223372036854775808", "9223372036854775807", Loc{}, Loc{})
				bounds, _ := ref.IntegerBounds()
				for _, bound := range bounds.Bounds() {
					bound.Value().value.SetInt64(0)
				}
				assertAllIntegerBounds(t, ref, profile.version, "-9223372036854775808", "9223372036854775807", Loc{}, Loc{})
				attributes[longIndex] = nil
				components := schema.Components()
				components[0] = Component{}
				if owner.AttributeUses()[longIndex] == nil || schema.Components()[0].Name().Local() != "root" {
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
				controlRoot := strings.Replace(root, groupedLongUse, "", 1)
				if !mixed {
					controlRoot = strings.Replace(controlRoot, `</xs:extension>`, `<xs:attribute name="before" type="xs:boolean"/></xs:extension>`, 1)
				}
				control, err := discoverTestSchemaWithPolicy(t, controlRoot, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				controlOwner := modelGroupReferenceTestComplexTypeNamed(t, control, "urn:root", "Derived")
				controlGroup, hasControlGroup := controlOwner.Particle().(ModelGroupReferenceParticle)
				controlBase, _ := controlOwner.BaseReference()
				controlBaseID, _ := controlBase.ComponentID()
				if !hasControlGroup || controlGroup.TargetID() != group.TargetID() || controlGroup.Occurrences().String() != "2/3" || controlBaseID != baseID || !reflect.DeepEqual(componentIDs(control.Components()), componentIDs(schema.Components())) {
					t.Fatalf("omitted control changed shared facts = %#v", controlOwner)
				}
			})
		}
	}
}

func TestGroupedLongConsumerBoundaries(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := groupedExtensionSchema(string(profile.version), "", groupedLongUse)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			useLoc := complexContentTestLoc(t, root, `<xs:attribute name="long"`)
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

func TestGroupedLongExcludedForms(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		alias := `<xs:simpleType name="Alias"><xs:restriction base="xs:long"/></xs:simpleType>`
		inline := `<xs:simpleType><xs:restriction base="xs:long"/></xs:simpleType>`
		for _, test := range []struct {
			name, use, extra, marker, related string
			spec                              func(XSDVersion) string
			cause                             error
		}{
			{"inline atomic", `<xs:attribute name="a">` + inline + `</xs:attribute>`, "", `<xs:simpleType`, "", schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnsupported},
			{"inline list", `<xs:attribute name="a"><xs:simpleType><xs:list itemType="xs:long"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, "", schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnsupported},
			{"inline union", `<xs:attribute name="a"><xs:simpleType><xs:union memberTypes="xs:long xs:int"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, "", schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnsupported},
			{"ref builtin", `<xs:attribute ref="t:longGlobal"/>`, `<xs:attribute name="longGlobal" type="xs:long"/>`, `ref="t:longGlobal"`, `<xs:attribute name="longGlobal"`, schemaAttributeUseSpecRef, errSchemaAttributeReferenceUnsupported},
			{"ref named", `<xs:attribute ref="t:aliasGlobal"/>`, `<xs:attribute name="aliasGlobal" type="t:Alias"/>` + alias, `ref="t:aliasGlobal"`, `<xs:attribute name="aliasGlobal"`, schemaAttributeUseSpecRef, errSchemaAttributeReferenceUnsupported},
			{"ref inline", `<xs:attribute ref="t:inlineGlobal"/>`, `<xs:attribute name="inlineGlobal">` + inline + `</xs:attribute>`, `ref="t:inlineGlobal"`, `<xs:attribute name="inlineGlobal"`, schemaAttributeUseSpecRef, errSchemaAttributeReferenceUnsupported},
			{"ref default", `<xs:attribute ref="t:longGlobal" default="7"/>`, `<xs:attribute name="longGlobal" type="xs:long"/>`, `default="7"`, "", schemaComplexTypeExtensionSpecRef, errSchemaAttributeUseUnsupported},
			{"ref fixed", `<xs:attribute ref="t:longGlobal" fixed="7"/>`, `<xs:attribute name="longGlobal" type="xs:long"/>`, `fixed="7"`, "", schemaComplexTypeExtensionSpecRef, errSchemaAttributeUseUnsupported},
			{"direct default", `<xs:attribute name="a" type="xs:long" default="7"/>`, "", `default="7"`, "", schemaComplexTypeExtensionSpecRef, errSchemaAttributeUseUnsupported},
			{"direct fixed", `<xs:attribute name="a" type="xs:long" fixed="7"/>`, "", `fixed="7"`, "", schemaComplexTypeExtensionSpecRef, errSchemaAttributeUseUnsupported},
			{"named default", `<xs:attribute name="a" type="t:Alias" default="7"/>`, alias, `default="7"`, "", schemaComplexTypeExtensionSpecRef, errSchemaAttributeUseUnsupported},
			{"named fixed", `<xs:attribute name="a" type="t:Alias" fixed="7"/>`, alias, `fixed="7"`, "", schemaComplexTypeExtensionSpecRef, errSchemaAttributeUseUnsupported},
			{"inline default", `<xs:attribute name="a" default="7">` + inline + `</xs:attribute>`, "", `default="7"`, "", schemaComplexTypeExtensionSpecRef, errSchemaAttributeUseUnsupported},
			{"inline fixed", `<xs:attribute name="a" fixed="7">` + inline + `</xs:attribute>`, "", `fixed="7"`, "", schemaComplexTypeExtensionSpecRef, errSchemaAttributeUseUnsupported},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := groupedExtensionSchema(string(profile.version), "", groupedLongUse+test.use)
				root = strings.Replace(root, `</xs:schema>`, test.extra+`</xs:schema>`, 1)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				var related []Loc
				if test.related != "" {
					related = []Loc{complexContentTestLoc(t, root, test.related)}
				}
				assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, complexContentTestLoc(t, root, test.marker), related, test.spec(profile.version), test.cause)
			})
		}
		anonymous := anonymousGroupedExtensionSchema(string(profile.version), "", groupedLongUse)
		schema, err := discoverTestSchemaWithPolicy(t, anonymous, nil, profile.policy)
		assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, complexContentTestLoc(t, anonymous, `type="xs:long"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeUseUnsupported)
		anonymous = anonymousGroupedExtensionSchema(string(profile.version), "", `<xs:attribute name="a" type="t:Alias"/>`)
		anonymous = strings.Replace(anonymous, `</xs:schema>`, alias+`</xs:schema>`, 1)
		schema, err = discoverTestSchemaWithPolicy(t, anonymous, nil, profile.policy)
		assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, complexContentTestLoc(t, anonymous, `type="t:Alias"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeUseUnsupported)
	}
}

func TestGroupedLongOmissionAndAlternateExits(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		zero := groupedExtensionSchema(string(profile.version), ` minOccurs="0" maxOccurs="0"`, groupedLongUse+`<xs:attribute name="gone" type="xs:long" use="prohibited"/>`)
		schema, err := discoverTestSchemaWithPolicy(t, zero, nil, profile.policy)
		if err != nil {
			t.Fatal(err)
		}
		owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Derived")
		if owner.Particle() != nil || len(owner.AttributeUses()) != 1 || owner.AttributeUses()[0].Name().Local() != "long" {
			t.Fatalf("omissions = %#v/%#v", owner.Particle(), owner.AttributeUses())
		}
		testGroupedLongExits(t, profile, zero, []groupedLongExit{
			{"missing group", `ref="t:Fields"`, `ref="t:Missing"`, `ref="t:Missing"`, "", diagnosticSchemaModelGroupReferenceUnresolvedCode, FailureInvalid, errSchemaModelGroupReferenceUnresolved, schemaModelGroupReferenceSpecRef},
			{"wrong group kind", `ref="t:Fields"`, `ref="t:root"`, `ref="t:root"`, `<xs:element name="root"`, diagnosticSchemaModelGroupReferenceWrongKindCode, FailureInvalid, errSchemaModelGroupReferenceWrongKind, schemaModelGroupReferenceSpecRef},
			{"wrong base kind", `base="t:Base"`, `base="t:Fields"`, `base="t:Fields"`, `<xs:group name="Fields"`, invalidSchemaCompositionCode, FailureInvalid, errSchemaComplexTypeBaseWrongKind, schemaComplexTypeExtensionSpecRef},
			{"used base", `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:attribute name="inherited" type="xs:boolean"/></xs:complexType>`, `base="t:Base"`, `<xs:complexType name="Base"`, UnsupportedSchemaSyntaxCode, FailureUnsupported, errSchemaComplexTypeBaseUnsupported, schemaComplexTypeExtensionSpecRef},
			{"nonempty base", `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:sequence><xs:element ref="t:root"/></xs:sequence></xs:complexType>`, `base="t:Base"`, `<xs:complexType name="Base"`, UnsupportedSchemaSyntaxCode, FailureUnsupported, errSchemaComplexTypeBaseNonEmpty, schemaComplexTypeExtensionSpecRef},
			{"final base", `<xs:complexType name="Base"/>`, `<xs:complexType name="Base" final="extension"/>`, `base="t:Base"`, `final="extension"`, invalidSchemaCompositionCode, FailureInvalid, errSchemaComplexTypeBaseUnsupported, schemaComplexTypeExtensionSpecRef},
			{"prohibited unresolved type", `type="xs:long" use="prohibited"`, `type="t:MissingType" use="prohibited"`, `type="t:MissingType"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, FailureInvalid, errSchemaAttributeTypeUnresolved, schemaAttributeTypeSpecRef},
		})
		cycle := strings.Replace(zero, `<xs:group name="Fields"><xs:sequence/></xs:group>`, `<xs:group name="Fields"><xs:sequence><xs:group ref="t:Fields"/></xs:sequence></xs:group>`, 1)
		failed, err := discoverTestSchemaWithPolicy(t, cycle, nil, profile.policy)
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

//nolint:gocognit // Graph identities and wildcard provenance are one public view.
func TestGroupedLongGraphAndWildcard(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root"><xs:include schemaLocation="shared.xsd"/><xs:include schemaLocation="again.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/><xs:complexType name="Derived"><xs:complexContent><xs:extension base="o:Base"><xs:group ref="r:Fields" minOccurs="1" maxOccurs="2"/>` + groupedLongUse + `</xs:extension></xs:complexContent></xs:complexType></xs:schema>`
	shared := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other"><xs:include schemaLocation="root.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/><xs:group name="Fields"><xs:sequence><xs:element ref="o:item"/></xs:sequence></xs:group></xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:element name="item" type="xs:integer"/><xs:complexType name="Base"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType></xs:schema>`
	fixtures := map[string]discoveryFixture{"root.xsd": {id: "root.xsd", contents: root}, "shared.xsd": {id: "shared.xsd", contents: shared}, "again.xsd": {id: "shared.xsd", contents: shared}, "other.xsd": {id: "other.xsd", contents: other}}
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
			group, hasGroup := owner.Particle().(ModelGroupReferenceParticle)
			groups := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, "urn:root", "Fields"))
			base, hasBase := owner.BaseReference()
			baseID, hasBaseID := base.ComponentID()
			bases := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:other", "Base"))
			wildcard, hasWildcard := owner.AnyAttribute()
			if !hasGroup || len(groups) != 1 || group.TargetID() != groups[0].ID() || group.Occurrences().String() != "1/2" || group.RefLoc() != complexContentTestLoc(t, root, `ref="r:Fields"`) || !hasBase || !hasBaseID || len(bases) != 1 || baseID != bases[0].ID() || base.Loc() != complexContentTestLoc(t, root, `base="o:Base"`) || !hasWildcard || wildcard.Loc() != complexContentTestSourceLoc(t, "other.xsd", other, `<xs:anyAttribute`) || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" {
				t.Fatalf("graph facts = %#v/%#v/%#v", group, base, wildcard)
			}
			use, useOK := owner.AttributeUses()[0].(LocalAttributeUse)
			if !useOK {
				t.Fatalf("long use = %T", owner.AttributeUses()[0])
			}
			ref, _ := use.TypeReference()
			assertAllIntegerBounds(t, ref, profile.version, "-9223372036854775808", "9223372036854775807", Loc{}, Loc{})
		})
	}
}

func TestGroupedLongResolverCause(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		root := groupedExtensionSchema(string(profile.version), "", groupedLongUse)
		root = strings.Replace(root, `<xs:element name="root"`, `<xs:include schemaLocation="missing.xsd"/><xs:element name="root"`, 1)
		source, sourceErr := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
		if sourceErr != nil {
			t.Fatal(sourceErr)
		}
		cause := errors.New("grouped long dependency unavailable")
		schema, err := ParseSchemaWithPolicy(source, &discoveryResolver{failures: map[string]error{"missing.xsd": cause}}, profile.policy)
		assertNCNameDiagnostic(t, schema, err, FailureResolution, SourceResolveCode, complexContentTestLoc(t, root, `<xs:include`), nil, "", cause)
	}
}

//nolint:gocognit // Group syntax, ordered use resolution, and base gates precede publication.
func TestGroupedLongSyntaxAndResolutionOrder(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct {
			name, group, uses, marker, code, spec string
			class                                 FailureClass
			cause                                 error
		}{
			{"invalid bounds", ` minOccurs="2" maxOccurs="1"`, groupedLongUse, `<xs:group ref="t:Fields"`, invalidSchemaCompositionCode, schemaParticleCorrectSpecRef(profile.version), FailureInvalid, errParticleOccurrenceMinimumExceedsMaximum},
			{"duplicate model", "", `<xs:group ref="t:Fields"/>` + groupedLongUse, `<xs:group ref="t:Fields"`, invalidSchemaCompositionCode, schemaComplexContentExtensionSpecRef(profile.version), FailureInvalid, nil},
			{"invalid use", "", `<xs:attribute name="a" type="xs:long" use="sometimes"/>`, `use="sometimes"`, invalidSchemaCompositionCode, schemaAttributeUseSpecRef(profile.version), FailureInvalid, nil},
			{"unresolved named type", "", `<xs:attribute name="a" type="t:Missing"/>`, `type="t:Missing"`, diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(profile.version), FailureInvalid, errSchemaAttributeTypeUnresolved},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := groupedExtensionSchema(string(profile.version), test.group, test.uses)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				loc := complexContentTestLoc(t, root, test.marker)
				var related []Loc
				if test.name == "duplicate model" {
					loc = allParticleTestTokenLoc(t, "root.xsd", root, test.marker, 2)
				}
				if test.name == "invalid bounds" {
					related = []Loc{complexContentTestLoc(t, root, `minOccurs="2"`), complexContentTestLoc(t, root, `maxOccurs="1"`)}
				}
				if d.Class() != test.class || d.Code() != test.code || d.Loc() != loc || d.SpecRef() != test.spec || !reflect.DeepEqual(d.Related(), related) {
					t.Fatalf("diagnostic = %v; related=%v", err, d.Related())
				}
				if test.cause != nil && !errors.Is(err, test.cause) {
					t.Fatalf("cause = %v", err)
				}
			})
		}
	}
}

func TestGroupedLongGraphVisibilityAndDuplicate(t *testing.T) {
	bridge := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge"><xs:import namespace="urn:foreign" schemaLocation="foreign.xsd"/></xs:schema>`
	foreign := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:foreign"><xs:group name="Fields"><xs:sequence/></xs:group><xs:complexType name="Base"/></xs:schema>`
	fixtures := map[string]discoveryFixture{"bridge.xsd": {id: "bridge.xsd", contents: bridge}, "foreign.xsd": {id: "foreign.xsd", contents: foreign}}
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct {
			name, base, group, marker, related, code, spec string
			cause                                          error
		}{
			{"hidden group", `xs:anyType`, `f:Fields`, `ref="f:Fields"`, `<xs:group name="Fields"`, diagnosticSchemaModelGroupReferenceNamespaceCode, schemaModelGroupReferenceImportSpecRef(profile.version), errSchemaModelGroupReferenceNamespace},
			{"hidden base", `f:Base`, `r:Fields`, `base="f:Base"`, `<xs:complexType name="Base"`, invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(profile.version), errSchemaComplexTypeBaseUnresolved},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:f="urn:foreign" targetNamespace="urn:root"><xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/><xs:group name="Fields"><xs:sequence/></xs:group><xs:complexType name="Derived"><xs:complexContent><xs:extension base="` + test.base + `"><xs:group ref="` + test.group + `"/>` + groupedLongUse + `</xs:extension></xs:complexContent></xs:complexType></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				assertNCNameDiagnostic(t, schema, err, FailureInvalid, test.code, complexContentTestLoc(t, root, test.marker), []Loc{complexContentTestSourceLoc(t, "foreign.xsd", foreign, test.related)}, test.spec, test.cause)
			})
		}
		duplicate := groupedExtensionSchema(string(profile.version), "", groupedLongUse)
		duplicate = strings.Replace(duplicate, `</xs:schema>`, `<xs:group name="Fields"><xs:sequence/></xs:group></xs:schema>`, 1)
		schema, err := discoverTestSchemaWithPolicy(t, duplicate, nil, profile.policy)
		assertNCNameDiagnostic(t, schema, err, FailureInvalid, diagnosticSchemaGlobalDuplicateCode, allParticleTestTokenLoc(t, "root.xsd", duplicate, `<xs:group name="Fields"`, 2), []Loc{complexContentTestLoc(t, duplicate, `<xs:group name="Fields"`)}, schemaGlobalDuplicateSpecRef(profile.version), errSchemaGlobalDeclarationDuplicate)
	}
}
