package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func groupedNamedLanguageRoot(kind, uses, extra string) string {
	root := groupedLanguageAnyURIRoot(` minOccurs="2" maxOccurs="3"`, uses)
	root = strings.Replace(root, `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>`, 1)
	return strings.Replace(root, `</xs:schema>`, `<xs:simpleType name="Direct"><xs:restriction base="xs:`+kind+`"/></xs:simpleType><xs:simpleType name="Child"><xs:restriction base="t:Direct"/></xs:simpleType>`+extra+`</xs:schema>`, 1)
}

//nolint:gocognit,funlen // Public facts from the completed named chain must survive the grouped owner unchanged.
func TestGroupedNamedLanguageAnyURIPublicFacts(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, kind := range []string{"language", "anyURI"} {
			for _, label := range []string{"1.0", "1.1"} {
				t.Run(string(policy)+"/"+kind+"/"+label, func(t *testing.T) {
					uses := `<xs:attribute name="before" type="xs:boolean"/>` +
						`<xs:attribute name="direct" type="t:Direct" use="required"/>` +
						`<xs:attribute name="child" type="t:Child" form="unqualified"/>` +
						`<xs:attribute name="gone" type="t:Child" use="prohibited"/>` +
						`<xs:attribute name="after" type="xs:integer"/>`
					root := groupedNamedLanguageRoot(kind, uses, "")
					root = strings.Replace(root, `version="1.1"`, `version="`+label+`"`, 1)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
					if err != nil {
						t.Fatal(err)
					}
					if schema.LanguagePolicy() != policy || len(schema.Documents()) != 1 || schema.Documents()[0].Source() != "root.xsd" {
						t.Fatalf("policy/documents = %s/%v", schema.LanguagePolicy(), schema.Documents())
					}
					owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Derived")
					if owner.Loc() != complexContentTestLoc(t, root, `<xs:complexType name="Derived"`) || owner.Derivation() != ComplexTypeDerivationExtension || owner.DerivationLoc() != complexContentTestLoc(t, root, `<xs:extension`) || owner.Base() != mustTestQName(t, "urn:root", "Base") || owner.BaseLoc() != complexContentTestLoc(t, root, `base="t:Base"`) {
						t.Fatalf("owner = %v", owner)
					}
					base, hasBase := owner.BaseReference()
					baseID, hasBaseID := base.ComponentID()
					bases := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Base"))
					if !hasBase || !hasBaseID || len(bases) != 1 || baseID != bases[0].ID() || base.Loc() != owner.BaseLoc() {
						t.Fatalf("base = %v", base)
					}
					group, ok := owner.Particle().(ModelGroupReferenceParticle)
					groups := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, "urn:root", "Fields"))
					if !ok || len(groups) != 1 || group.TargetID() != groups[0].ID() || group.Ref() != mustTestQName(t, "urn:root", "Fields") || group.Loc() != complexContentTestLoc(t, root, `<xs:group ref="t:Fields"`) || group.RefLoc() != complexContentTestLoc(t, root, `ref="t:Fields"`) || group.Occurrences().String() != "2/3" {
						t.Fatalf("group = %v", owner.Particle())
					}
					wildcard, hasWildcard := owner.AnyAttribute()
					if !hasWildcard || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" || wildcard.Loc() != complexContentTestLoc(t, root, `<xs:anyAttribute`) {
						t.Fatalf("inherited wildcard = %v", wildcard)
					}
					attributes := owner.AttributeUses()
					if len(attributes) != 4 || attributes[0].Name().Local() != "before" || attributes[3].Name().Local() != "after" {
						t.Fatalf("ordered uses = %v", attributes)
					}
					for index, want := range []string{"Direct", "Child"} {
						use, useOK := attributes[index+1].(LocalAttributeUse)
						name := strings.ToLower(want)
						typeLoc := complexContentTestLoc(t, root, `type="t:`+want+`"`)
						qname := mustTestQName(t, "urn:root", want)
						if !useOK || use.Loc() != complexContentTestLoc(t, root, `<xs:attribute name="`+name+`"`) || use.NameLoc() != complexContentTestLoc(t, root, `name="`+name+`"`) || use.TypeLoc() != typeLoc || use.DeclaredType() != qname {
							t.Fatalf("use %s = %v", want, attributes[index+1])
						}
						if index == 0 && (use.Name() != mustTestQName(t, "urn:root", "direct") || use.Use() != AttributeUseRequired || use.UseLoc() != complexContentTestLoc(t, root, `use="required"`)) {
							t.Fatalf("required use = %v", use)
						}
						if index == 1 && (use.Name() != mustTestQName(t, "", "child") || use.FormLoc() != complexContentTestLoc(t, root, `form="unqualified"`)) {
							t.Fatalf("unqualified use = %v", use)
						}
						ref, hasRef := use.TypeReference()
						id, hasID := use.TypeID()
						refID, hasRefID := ref.ComponentID()
						if !hasRef || !ref.IsNamed() || ref.IsBuiltin() || ref.Name() != qname || ref.QName() != qname || ref.Loc() != typeLoc || ref.Variety() != SimpleTypeVarietyAtomicRestriction || !hasID || !hasRefID || id != refID || id != componentIDForName(t, schema, qname) || ref.StringEnumerationFacets().HasEnumeration() {
							t.Fatalf("type %s = %v/%v", want, use, ref)
						}
						if _, hasSpace := ref.StringWhiteSpaceFacet(); hasSpace {
							t.Fatalf("authored whitespace on %s", want)
						}
						definition := schemaEnumerationTestDefinitionInNamespace(t, schema, "urn:root", want)
						baseName := mustTestQName(t, testXSDNamespace, kind)
						baseLexical := "xs:" + kind
						if want == "Child" {
							baseName = mustTestQName(t, "urn:root", "Direct")
							baseLexical = "t:Direct"
						}
						if definition.ID() != id || definition.Loc() != complexContentTestLoc(t, root, `<xs:simpleType name="`+want+`"`) || definition.Base() != baseName || definition.BaseLoc() != complexContentTestLoc(t, root, `base="`+baseLexical+`"`) || ref.VarietyLoc() != complexContentTestLoc(t, root, `<xs:restriction base="`+baseLexical+`"`) {
							t.Fatalf("definition %s = %v", want, definition)
						}
						baseRef, hasBaseRef := definition.BaseReference()
						if !hasBaseRef || baseRef.Name() != baseName || baseRef.IsBuiltin() != (want == "Direct") || baseRef.Variety() != SimpleTypeVarietyAtomicRestriction {
							t.Fatalf("base of %s = %v", want, baseRef)
						}
					}
					attributes[0] = nil
					components := schema.Components()
					components[0] = Component{}
					if owner.AttributeUses()[0] == nil || schema.Components()[0].Name().IsZero() {
						t.Fatal("caller changed published facts")
					}
					var first, second []ComponentID
					if err := schema.Walk(func(component Component) error { first = append(first, component.ID()); return nil }); err != nil {
						t.Fatal(err)
					}
					if err := schema.Walk(func(component Component) error { second = append(second, component.ID()); return nil }); err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(first, second) || !reflect.DeepEqual(first, componentIDs(schema.Components())) {
						t.Fatalf("walks = %v/%v", first, second)
					}
					withoutUses := strings.Replace(root, `<xs:attribute name="direct" type="t:Direct" use="required"/>`, "", 1)
					withoutUses = strings.Replace(withoutUses, `<xs:attribute name="child" type="t:Child" form="unqualified"/>`, "", 1)
					withoutUses = strings.Replace(withoutUses, `<xs:attribute name="gone" type="t:Child" use="prohibited"/>`, "", 1)
					control, controlErr := discoverTestSchemaWithPolicy(t, withoutUses, nil, policy)
					if controlErr != nil {
						t.Fatal(controlErr)
					}
					controlOwner := modelGroupReferenceTestComplexTypeNamed(t, control, "urn:root", "Derived")
					controlGroup, ok := controlOwner.Particle().(ModelGroupReferenceParticle)
					if !ok || !reflect.DeepEqual(componentIDs(control.Components()), componentIDs(schema.Components())) || controlGroup.TargetID() != group.TargetID() || controlGroup.Occurrences().String() != group.Occurrences().String() || len(controlOwner.AttributeUses()) != 2 {
						t.Fatalf("control changed identity/bounds = %v", controlOwner)
					}
					again, againErr := discoverTestSchemaWithPolicy(t, root, nil, policy)
					if againErr != nil {
						t.Fatal(againErr)
					}
					againOwner := modelGroupReferenceTestComplexTypeNamed(t, again, "urn:root", "Derived")
					if !reflect.DeepEqual(componentIDs(again.Components()), componentIDs(schema.Components())) || !reflect.DeepEqual(againOwner.AttributeUses(), owner.AttributeUses()) || !reflect.DeepEqual(againOwner.Particle(), owner.Particle()) {
						t.Fatalf("repeated parse changed public facts: %v", againOwner)
					}
				})
			}
		}
	}
}

func TestGroupedNamedLanguageAnyURIConsumersUnsupported(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, kind := range []string{"language", "anyURI"} {
			root := groupedNamedLanguageRoot(kind, `<xs:attribute name="value" type="t:Child"/>`, "")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatal(err)
			}
			useLoc := complexContentTestLoc(t, root, `<xs:attribute name="value"`)
			ownerLoc := complexContentTestLoc(t, root, `<xs:complexType name="Derived"`)
			rootLoc := complexContentTestLoc(t, root, `<xs:element name="root"`)
			instanceErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
			vd := requireDiagnostic(t, instanceErr)
			if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != useLoc || vd.SpecRef() != instanceValidationSpecRef(languageUseVersion(policy)) || !reflect.DeepEqual(vd.Related(), []Loc{rootLoc, ownerLoc, useLoc}) || !errors.Is(instanceErr, ErrUnsupported) || !errors.Is(instanceErr, errInstanceAttributes) {
				t.Fatalf("validation = %v related=%v", instanceErr, vd.Related())
			}
			output, generateErr := GenerateGo(schema, "generated")
			gd := requireDiagnostic(t, generateErr)
			if output != nil || gd.Class() != FailureUnsupported || gd.Code() != diagnosticCodegenUnsupported || gd.Loc() != useLoc || gd.SpecRef() != codegenDirectSequenceSpecReference(languageUseVersion(policy), codegenDirectSequenceParticlesReference) || !reflect.DeepEqual(gd.Related(), []Loc{ownerLoc, useLoc}) || !errors.Is(generateErr, ErrUnsupported) || !errors.Is(generateErr, errCodegenUnsupported) {
				t.Fatalf("generation = %v/%v related=%v", output, generateErr, gd.Related())
			}
		}
	}
}

//nolint:gocognit // Alternate exits at the grouped named-use boundary retain their own diagnostic evidence.
func TestGroupedNamedLanguageAnyURIAlternateExits(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		version := languageUseVersion(policy)
		for _, kind := range []string{"language", "anyURI"} {
			for _, test := range []struct {
				name, old, replacement, marker, related, code, spec string
				class                                               FailureClass
				cause                                               error
			}{
				{"invalid occurrence", `minOccurs="2" maxOccurs="3"`, `minOccurs="4" maxOccurs="3"`, `<xs:group ref="t:Fields"`, "", invalidSchemaCompositionCode, schemaParticleCorrectSpecRef(version), FailureInvalid, errParticleOccurrenceMinimumExceedsMaximum},
				{"unresolved group", `ref="t:Fields"`, `ref="t:Missing"`, `ref="t:Missing"`, "", diagnosticSchemaModelGroupReferenceUnresolvedCode, schemaModelGroupReferenceSpecRef(version), FailureInvalid, errSchemaModelGroupReferenceUnresolved},
				{"wrong-kind group", `ref="t:Fields"`, `ref="t:root"`, `ref="t:root"`, `<xs:element name="root"`, diagnosticSchemaModelGroupReferenceWrongKindCode, schemaModelGroupReferenceSpecRef(version), FailureInvalid, errSchemaModelGroupReferenceWrongKind},
				{"missing type", `type="t:Child"`, `type="t:Missing"`, `type="t:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(version), FailureInvalid, errSchemaAttributeTypeUnresolved},
				{"wrong-kind type", `type="t:Child"`, `type="t:Fields"`, `type="t:Fields"`, `<xs:group name="Fields"`, diagnosticSchemaAttributeTypeWrongKindCode, schemaAttributeTypeSpecRef(version), FailureInvalid, errSchemaAttributeTypeWrongKind},
				{"type cycle", `base="xs:` + kind + `"`, `base="t:Child"`, `type="t:Child"`, "", diagnosticSchemaAttributeTypeCycleCode, schemaAttributeTypeSpecRef(version), FailureInvalid, errSchemaSimpleTypeBaseCycle},
				{"authored facet", `<xs:restriction base="xs:` + kind + `"/>`, `<xs:restriction base="xs:` + kind + `"><xs:pattern value=".*"/></xs:restriction>`, `<xs:pattern`, "", UnsupportedDatatypeFacetCode, tokenDiagnosticSpecRef(version, "decimal"), FailureUnsupported, ErrUnsupported},
				{"inherited authored facet", `<xs:restriction base="xs:` + kind + `"/>`, `<xs:restriction base="xs:` + kind + `"><xs:enumeration value="en"/></xs:restriction>`, `<xs:enumeration`, "", UnsupportedDatatypeFacetCode, tokenDiagnosticSpecRef(version, "decimal"), FailureUnsupported, ErrUnsupported},
				{"missing base", `base="t:Base"`, `base="t:MissingBase"`, `base="t:MissingBase"`, "", invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(version), FailureInvalid, errSchemaComplexTypeBaseUnresolved},
				{"wrong-kind base", `base="t:Base"`, `base="t:Direct"`, `base="t:Direct"`, `<xs:simpleType name="Direct"`, invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(version), FailureInvalid, errSchemaComplexTypeBaseWrongKind},
				{"base cycle", `base="t:Base"`, `base="t:Derived"`, `base="t:Derived"`, "", invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(version), FailureInvalid, errSchemaComplexTypeBaseCycle},
				{"final base", `<xs:complexType name="Base">`, `<xs:complexType name="Base" final="extension">`, `base="t:Base"`, `final="extension"`, invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(version), FailureInvalid, errSchemaComplexTypeBaseUnsupported},
				{"nonempty base", `<xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent>`, `<xs:sequence><xs:element name="baseValue" type="xs:integer"/></xs:sequence>`, `base="t:Base"`, `<xs:sequence><xs:element name="baseValue"`, UnsupportedSchemaSyntaxCode, schemaComplexTypeExtensionSpecRef(version), FailureUnsupported, errSchemaComplexTypeBaseNonEmpty},
				{"used base", `<xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent>`, `<xs:attribute name="inherited" type="xs:boolean"/>`, `base="t:Base"`, `<xs:complexType name="Base"`, UnsupportedSchemaSyntaxCode, schemaComplexTypeExtensionSpecRef(version), FailureUnsupported, errSchemaComplexTypeBaseUnsupported},
				{"grouped base", `<xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent>`, `<xs:group ref="t:Fields"/>`, `base="t:Base"`, `<xs:complexType name="Base"`, UnsupportedSchemaSyntaxCode, schemaComplexTypeExtensionSpecRef(version), FailureUnsupported, errSchemaComplexTypeBaseNonEmpty},
				{"simple content base", `<xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent>`, `<xs:simpleContent><xs:extension base="xs:token"/></xs:simpleContent>`, `base="t:Base"`, `<xs:complexType name="Base"`, UnsupportedSchemaSyntaxCode, schemaComplexTypeExtensionSpecRef(version), FailureUnsupported, errSchemaComplexTypeBaseUnsupported},
			} {
				t.Run(string(policy)+"/"+kind+"/"+test.name, func(t *testing.T) {
					root := groupedNamedLanguageRoot(kind, `<xs:attribute name="value" type="t:Child"/>`, "")
					root = strings.Replace(root, test.old, test.replacement, 1)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					var related []Loc
					if test.related != "" {
						related = []Loc{complexContentTestLoc(t, root, test.related)}
					}
					if test.name == "invalid occurrence" {
						related = []Loc{complexContentTestLoc(t, root, `minOccurs="4"`), complexContentTestLoc(t, root, `maxOccurs="3"`)}
					}
					if test.name == "nonempty base" {
						related = []Loc{complexContentTestLoc(t, root, `<xs:complexType name="Base"`), complexContentTestLoc(t, root, test.related)}
					}
					if test.name == "grouped base" {
						related = []Loc{complexContentTestLoc(t, root, `<xs:complexType name="Base"`), allParticleTestTokenLoc(t, "root.xsd", root, `<xs:group ref="t:Fields"`, 2)}
					}
					if test.name == "type cycle" {
						related = []Loc{complexContentTestLoc(t, root, `<xs:attribute name="value"`), complexContentTestLoc(t, root, `base="t:Child"`), complexContentTestLoc(t, root, `base="t:Direct"`), complexContentTestLoc(t, root, `<xs:simpleType name="Child"`), complexContentTestLoc(t, root, `<xs:simpleType name="Direct"`)}
					}
					if d.Class() != test.class || d.Code() != test.code || d.Loc() != complexContentTestLoc(t, root, test.marker) || d.SpecRef() != test.spec || !reflect.DeepEqual(d.Related(), related) || !errors.Is(err, test.cause) {
						t.Fatalf("%s = %v class=%s code=%s loc=%s spec=%s related=%v; want %s %s %s %v", test.name, err, d.Class(), d.Code(), d.Loc(), d.SpecRef(), d.Related(), test.code, complexContentTestLoc(t, root, test.marker), test.spec, related)
					}
				})
			}
		}
	}
}

//nolint:gocognit // Each excluded use form has an independent located exit.
func TestGroupedNamedLanguageAnyURIExcludedUseForms(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, kind := range []string{"language", "anyURI"} {
			value := "en"
			if kind == "anyURI" {
				value = "relative/path"
			}
			for _, test := range []struct {
				name, use, extra, marker, related string
				cause                             error
				useSpec                           bool
			}{
				{"inline restriction", `<xs:attribute name="a"><xs:simpleType><xs:restriction base="t:Child"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType><xs:restriction`, "", errSchemaAttributeUseUnsupported, false},
				{"inline list", `<xs:attribute name="a"><xs:simpleType><xs:list itemType="t:Child"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType><xs:list`, "", errSchemaAttributeUseUnsupported, false},
				{"inline union", `<xs:attribute name="a"><xs:simpleType><xs:union memberTypes="t:Child"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType><xs:union`, "", errSchemaAttributeUseUnsupported, false},
				{"named list", `<xs:attribute name="a" type="t:Broader"/>`, `<xs:simpleType name="Broader"><xs:list itemType="t:Child"/></xs:simpleType>`, `type="t:Broader"`, "", errSchemaAttributeUseUnsupported, false},
				{"named union", `<xs:attribute name="a" type="t:Broader"/>`, `<xs:simpleType name="Broader"><xs:union memberTypes="t:Child"/></xs:simpleType>`, `type="t:Broader"`, "", errSchemaAttributeUseUnsupported, false},
				{"ref named", `<xs:attribute ref="t:a"/>`, `<xs:attribute name="a" type="t:Child"/>`, `ref="t:a"`, `<xs:attribute name="a" type="t:Child"`, errSchemaAttributeReferenceUnsupported, true},
				{"ref inline", `<xs:attribute ref="t:a"/>`, `<xs:attribute name="a"><xs:simpleType><xs:restriction base="t:Child"/></xs:simpleType></xs:attribute>`, `ref="t:a"`, `<xs:attribute name="a"><xs:simpleType`, errSchemaAttributeReferenceUnsupported, true},
				{"named default", `<xs:attribute name="a" type="t:Child" default="` + value + `"/>`, "", `default="` + value + `"`, "", errSchemaAttributeUseUnsupported, true},
				{"named fixed", `<xs:attribute name="a" type="t:Child" fixed="` + value + `"/>`, "", `fixed="` + value + `"`, "", errSchemaAttributeUseUnsupported, true},
			} {
				t.Run(string(policy)+"/"+kind+"/"+test.name, func(t *testing.T) {
					root := groupedNamedLanguageRoot(kind, test.use, test.extra)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					wantSpec := schemaAttributeTypeSpecRef(languageUseVersion(policy))
					if test.useSpec {
						wantSpec = schemaAttributeUseSpecRef(languageUseVersion(policy))
					}
					if strings.Contains(test.name, "default") || strings.Contains(test.name, "fixed") {
						wantSpec = schemaComplexTypeExtensionSpecRef(languageUseVersion(policy))
					}
					var related []Loc
					if test.related != "" {
						related = []Loc{complexContentTestLoc(t, root, test.related)}
					}
					if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != complexContentTestLoc(t, root, test.marker) || d.SpecRef() != wantSpec || !reflect.DeepEqual(d.Related(), related) || !errors.Is(err, ErrUnsupported) || !errors.Is(err, test.cause) {
						t.Fatalf("excluded %s = %v loc=%s want=%s spec=%s want=%s related=%v want=%v cause=%t", test.name, err, d.Loc(), complexContentTestLoc(t, root, test.marker), d.SpecRef(), wantSpec, d.Related(), related, errors.Is(err, test.cause))
					}
				})
			}
		}
	}
}

func TestGroupedNamedLanguageAnyURIExcludedAnonymousOwner(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, kind := range []string{"language", "anyURI"} {
			root := anonymousGroupedExtensionSchema("1.1", "", `<xs:attribute name="value" type="t:Child"/>`)
			root = strings.Replace(root, `</xs:schema>`, `<xs:simpleType name="Direct"><xs:restriction base="xs:`+kind+`"/></xs:simpleType><xs:simpleType name="Child"><xs:restriction base="t:Direct"/></xs:simpleType></xs:schema>`, 1)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != complexContentTestLoc(t, root, `type="t:Child"`) || d.SpecRef() != schemaAttributeTypeSpecRef(languageUseVersion(policy)) || len(d.Related()) != 0 || !errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaAttributeUseUnsupported) {
				t.Fatalf("anonymous grouped = %v", err)
			}
		}
	}
}

//nolint:gocognit // Both named types cross include/import/chameleon edges with ordered opaque group and base facts.
func TestGroupedNamedLanguageAnyURITogetherGraph(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="1.1" attributeFormDefault="qualified"><xs:include schemaLocation="shared.xsd"/><xs:include schemaLocation="again.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/><xs:complexType name="Derived"><xs:complexContent><xs:extension base="o:Base"><xs:group ref="r:Fields" minOccurs="1" maxOccurs="2"/><xs:attribute name="language" type="r:Language" use="required"/><xs:attribute name="uri" type="o:URI" form="unqualified"/></xs:extension></xs:complexContent></xs:complexType><xs:element name="root" type="r:Derived"/></xs:schema>`
	shared := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other"><xs:include schemaLocation="root.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/><xs:group name="Fields"><xs:sequence><xs:element ref="o:item"/></xs:sequence></xs:group><xs:simpleType name="Language"><xs:restriction base="xs:language"/></xs:simpleType></xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:element name="item" type="xs:integer"/><xs:complexType name="Base"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType><xs:simpleType name="URI"><xs:restriction base="xs:anyURI"/></xs:simpleType></xs:schema>`
	fixtures := map[string]discoveryFixture{
		"root.xsd":   {id: "root.xsd", contents: root},
		"shared.xsd": {id: "shared.xsd", contents: shared},
		"again.xsd":  {id: "shared.xsd", contents: shared},
		"other.xsd":  {id: "other.xsd", contents: other},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy)
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
			group, ok := owner.Particle().(ModelGroupReferenceParticle)
			base, hasBase := owner.BaseReference()
			baseID, hasBaseID := base.ComponentID()
			if len(groups) != 1 || len(bases) != 1 || !ok || group.TargetID() != groups[0].ID() || group.Ref() != mustTestQName(t, "urn:root", "Fields") || group.RefLoc() != complexContentTestLoc(t, root, `ref="r:Fields"`) || group.Occurrences().String() != "1/2" || !hasBase || !hasBaseID || baseID != bases[0].ID() || owner.BaseLoc() != complexContentTestLoc(t, root, `base="o:Base"`) {
				t.Fatalf("group/base = %v/%v", group, base)
			}
			wildcard, hasWildcard := owner.AnyAttribute()
			if !hasWildcard || wildcard.Loc() != complexContentTestSourceLoc(t, "other.xsd", other, `<xs:anyAttribute`) || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" {
				t.Fatalf("wildcard = %v", wildcard)
			}
			uses := owner.AttributeUses()
			if len(uses) != 2 || uses[0].Name() != mustTestQName(t, "urn:root", "language") || uses[1].Name() != mustTestQName(t, "", "uri") {
				t.Fatalf("uses = %v", uses)
			}
			for index, want := range []struct{ name, ns, source, contents string }{
				{"Language", "urn:root", "shared.xsd", shared},
				{"URI", "urn:other", "other.xsd", other},
			} {
				use, ok := uses[index].(LocalAttributeUse)
				qname := mustTestQName(t, want.ns, want.name)
				ref, hasRef := use.TypeReference()
				id, hasID := use.TypeID()
				refID, hasRefID := ref.ComponentID()
				if !ok || !hasRef || !hasID || !hasRefID || !ref.IsNamed() || use.DeclaredType() != qname || ref.QName() != qname || ref.Name() != qname || id != refID || id.Source() != SourceID(want.source) || use.TypeLoc() != complexContentTestLoc(t, root, `type="`+[]string{"r:Language", "o:URI"}[index]+`"`) {
					t.Fatalf("use %d = %v/%v", index, use, ref)
				}
				definition := schemaEnumerationTestDefinitionInNamespace(t, schema, want.ns, want.name)
				if definition.ID() != id || definition.Loc() != complexContentTestSourceLoc(t, SourceID(want.source), want.contents, `<xs:simpleType name="`+want.name+`"`) {
					t.Fatalf("definition %s = %v", want.name, definition)
				}
			}
			var first, second []ComponentID
			if err := schema.Walk(func(component Component) error { first = append(first, component.ID()); return nil }); err != nil {
				t.Fatal(err)
			}
			if err := schema.Walk(func(component Component) error { second = append(second, component.ID()); return nil }); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(first, second) || !reflect.DeepEqual(first, componentIDs(schema.Components())) {
				t.Fatalf("walks = %v/%v", first, second)
			}
		})
	}
}

//nolint:gocognit // Group and type dependencies are resolved before 0/0 or prohibited omission and before the base gate.
func TestGroupedNamedLanguageAnyURIOmissionAndPrecedence(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		version := languageUseVersion(policy)
		for _, kind := range []string{"language", "anyURI"} {
			root := groupedNamedLanguageRoot(kind, `<xs:attribute name="gone" type="t:Child" use="prohibited"/><xs:attribute name="keep" type="t:Direct"/>`, "")
			root = strings.Replace(root, `minOccurs="2" maxOccurs="3"`, `minOccurs="0" maxOccurs="0"`, 1)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatal(err)
			}
			owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Derived")
			if owner.Particle() != nil || len(owner.AttributeUses()) != 1 || owner.AttributeUses()[0].Name().Local() != "keep" {
				t.Fatalf("omitted model/uses = %v/%v", owner.Particle(), owner.AttributeUses())
			}
			for _, test := range []struct {
				name, old, replacement, marker, code, spec string
				cause                                      error
			}{
				{"missing omitted group", `ref="t:Fields"`, `ref="t:Missing"`, `ref="t:Missing"`, diagnosticSchemaModelGroupReferenceUnresolvedCode, schemaModelGroupReferenceSpecRef(version), errSchemaModelGroupReferenceUnresolved},
				{"missing prohibited type", `type="t:Child"`, `type="t:Missing"`, `type="t:Missing"`, diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(version), errSchemaAttributeTypeUnresolved},
			} {
				t.Run(string(policy)+"/"+kind+"/"+test.name, func(t *testing.T) {
					broken := strings.Replace(root, test.old, test.replacement, 1)
					failed, brokenErr := discoverTestSchemaWithPolicy(t, broken, nil, policy)
					assertZeroSchema(t, failed)
					d := requireDiagnostic(t, brokenErr)
					if d.Class() != FailureInvalid || d.Code() != test.code || d.Loc() != complexContentTestLoc(t, broken, test.marker) || d.SpecRef() != test.spec || len(d.Related()) != 0 || !errors.Is(brokenErr, test.cause) {
						t.Fatalf("%s = %v related=%v", test.name, brokenErr, d.Related())
					}
				})
			}
			usedBase := strings.Replace(root, `<xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent>`, `<xs:attribute name="inherited" type="xs:boolean"/>`, 1)
			usedBase = strings.Replace(usedBase, `type="t:Direct"`, `type="t:Missing"`, 1)
			failed, err := discoverTestSchemaWithPolicy(t, usedBase, nil, policy)
			assertZeroSchema(t, failed)
			d := requireDiagnostic(t, err)
			if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaAttributeTypeUnresolvedCode || d.Loc() != complexContentTestLoc(t, usedBase, `type="t:Missing"`) || d.SpecRef() != schemaAttributeTypeSpecRef(version) || len(d.Related()) != 0 || !errors.Is(err, errSchemaAttributeTypeUnresolved) {
				t.Fatalf("type before base = %v", err)
			}
			missingGroup := strings.Replace(usedBase, `ref="t:Fields"`, `ref="t:MissingGroup"`, 1)
			failed, err = discoverTestSchemaWithPolicy(t, missingGroup, nil, policy)
			assertZeroSchema(t, failed)
			d = requireDiagnostic(t, err)
			if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaModelGroupReferenceUnresolvedCode || d.Loc() != complexContentTestLoc(t, missingGroup, `ref="t:MissingGroup"`) || d.SpecRef() != schemaModelGroupReferenceSpecRef(version) || len(d.Related()) != 0 || !errors.Is(err, errSchemaModelGroupReferenceUnresolved) {
				t.Fatalf("group before type/base = %v", err)
			}
		}
	}
}

func TestGroupedNamedLanguageAnyURIInvisibleType(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, kind := range []string{"language", "anyURI"} {
			root := groupedNamedLanguageRoot(kind, `<xs:attribute name="value" type="o:Hidden"/>`, "")
			testNamedLanguageAnyURIInvisibleType(t, root, kind, policy)
		}
	}
}

func testNamedLanguageAnyURIInvisibleType(t *testing.T, root, kind string, policy LanguagePolicy) {
	t.Helper()
	root = strings.Replace(root, `xmlns:t="urn:root"`, `xmlns:t="urn:root" xmlns:o="urn:other"`, 1)
	root = strings.Replace(root, `<xs:element name="root"`, `<xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/><xs:element name="root"`, 1)
	bridge := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge"><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Hidden"><xs:restriction base="xs:` + kind + `"/></xs:simpleType></xs:schema>`
	fixtures := map[string]discoveryFixture{"bridge.xsd": {id: "bridge.xsd", contents: bridge}, "other.xsd": {id: "other.xsd", contents: other}}
	schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy)
	assertZeroSchema(t, schema)
	d := requireDiagnostic(t, err)
	if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaAttributeTypeUnresolvedCode || d.Loc() != complexContentTestLoc(t, root, `type="o:Hidden"`) || d.SpecRef() != schemaAttributeTypeSpecRef(languageUseVersion(policy)) || len(d.Related()) != 0 || !errors.Is(err, errSchemaAttributeTypeUnresolved) {
		t.Fatalf("invisible type = %v related=%v", err, d.Related())
	}
}

func TestGroupedNamedLanguageAnyURIFinalType(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, kind := range []string{"language", "anyURI"} {
			root := groupedNamedLanguageRoot(kind, `<xs:attribute name="value" type="t:Direct"/>`, "")
			root = strings.Replace(root, `<xs:simpleType name="Direct">`, `<xs:simpleType name="Direct" final="restriction">`, 1)
			root = strings.Replace(root, `<xs:simpleType name="Child"><xs:restriction base="t:Direct"/></xs:simpleType>`, "", 1)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatal(err)
			}
			owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Derived")
			if len(owner.AttributeUses()) != 1 || owner.AttributeUses()[0].Name().Local() != "value" {
				t.Fatalf("direct final type = %v", owner.AttributeUses())
			}
			broken := strings.Replace(root, `</xs:schema>`, `<xs:simpleType name="Child"><xs:restriction base="t:Direct"/></xs:simpleType></xs:schema>`, 1)
			broken = strings.Replace(broken, `type="t:Direct"`, `type="t:Child"`, 1)
			failed, err := discoverTestSchemaWithPolicy(t, broken, nil, policy)
			assertZeroSchema(t, failed)
			d := requireDiagnostic(t, err)
			if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaSimpleTypeBaseCode || d.Loc() != complexContentTestLoc(t, broken, `base="t:Direct"`) || d.SpecRef() != schemaSimpleTypeRestrictionSpecRef(languageUseVersion(policy)) || !reflect.DeepEqual(d.Related(), []Loc{complexContentTestLoc(t, broken, `final="restriction"`)}) || !errors.Is(err, errSchemaSimpleTypeInvalidDerivation) {
				t.Fatalf("final restriction = %v related=%v", err, d.Related())
			}
		}
	}
}

func TestGroupedNamedLanguageAnyURIDuplicateType(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, kind := range []string{"language", "anyURI"} {
			root := groupedNamedLanguageRoot(kind, `<xs:attribute name="value" type="t:Child"/>`, `<xs:simpleType name="Child"><xs:restriction base="xs:`+kind+`"/></xs:simpleType>`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			first := allParticleTestTokenLoc(t, "root.xsd", root, `<xs:simpleType name="Child"`, 1)
			second := allParticleTestTokenLoc(t, "root.xsd", root, `<xs:simpleType name="Child"`, 2)
			if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaGlobalDuplicateCode || d.Loc() != second || d.SpecRef() != schemaGlobalDuplicateSpecRef(languageUseVersion(policy)) || !reflect.DeepEqual(d.Related(), []Loc{first}) || !errors.Is(err, errSchemaGlobalDeclarationDuplicate) {
				t.Fatalf("duplicate named type = %v related=%v", err, d.Related())
			}
		}
	}
}

func TestGroupedNamedLanguageAnyURISourceResolution(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, kind := range []string{"language", "anyURI"} {
			root := groupedNamedLanguageRoot(kind, `<xs:attribute name="value" type="t:Child"/>`, "")
			root = strings.Replace(root, `<xs:element name="root"`, `<xs:include schemaLocation="missing.xsd"/><xs:element name="root"`, 1)
			source, sourceErr := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
			if sourceErr != nil {
				t.Fatal(sourceErr)
			}
			cause := errors.New("named grouped language dependency unavailable")
			failed, err := ParseSchemaWithPolicy(source, &discoveryResolver{failures: map[string]error{"missing.xsd": cause}}, policy)
			assertZeroSchema(t, failed)
			d := requireDiagnostic(t, err)
			if d.Class() != FailureResolution || d.Code() != SourceResolveCode || d.Loc() != complexContentTestLoc(t, root, `<xs:include`) || d.SpecRef() != "" || len(d.Related()) != 0 || !errors.Is(err, cause) {
				t.Fatalf("source resolution = %v related=%v", err, d.Related())
			}
		}
	}
}

func TestGroupedNamedLanguageAnyURIDuplicateUse(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, kind := range []string{"language", "anyURI"} {
			root := groupedNamedLanguageRoot(kind, `<xs:attribute name="value" type="t:Child"/><xs:attribute name="value" type="xs:boolean"/>`, "")
			failed, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			assertZeroSchema(t, failed)
			d := requireDiagnostic(t, err)
			first := complexContentTestLoc(t, root, `<xs:attribute name="value" type="t:Child"`)
			second := complexContentTestLoc(t, root, `<xs:attribute name="value" type="xs:boolean"`)
			if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaAttributeUseDuplicateCode || d.Loc() != second || d.SpecRef() != schemaAttributeUseSpecRef(languageUseVersion(policy)) || !reflect.DeepEqual(d.Related(), []Loc{first}) || !errors.Is(err, errSchemaAttributeUseDuplicate) {
				t.Fatalf("duplicate named use = %v related=%v", err, d.Related())
			}
		}
	}
}
