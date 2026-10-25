package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func groupedLanguageAnyURIUse(name string) string {
	return `<xs:attribute name="` + name + `" type="xs:` + name + `" use="required" form="unqualified"/>`
}

func groupedLanguageAnyURIRoot(group, uses string) string {
	root := groupedExtensionSchema("1.1", group, uses)
	root = strings.Replace(root, `<xs:group name="Fields"><xs:sequence/></xs:group>`, `<xs:group name="Fields"><xs:sequence><xs:element ref="t:last"/><xs:element ref="t:first"/></xs:sequence></xs:group><xs:element name="first" type="xs:integer"/><xs:element name="last" type="xs:integer"/>`, 1)
	return root
}

//nolint:gocognit,funlen // One public fixture checks the copied facts and ordered grouped-owner view.
func TestGroupedLanguageAnyURIPublicFacts(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, names := range [][]string{{"language"}, {"anyURI"}, {"language", "anyURI"}} {
			t.Run(string(policy)+"/"+strings.Join(names, "+"), func(t *testing.T) {
				uses := []string{`<xs:attribute name="before" type="xs:boolean"/>`}
				for _, name := range names {
					uses = append(uses, groupedLanguageAnyURIUse(name))
				}
				uses = append(uses, `<xs:attribute name="gone" type="xs:anyURI" use="prohibited"/>`, `<xs:attribute name="after" type="xs:integer"/>`)
				root := groupedLanguageAnyURIRoot(` minOccurs="2" maxOccurs="3"`, strings.Join(uses, ""))
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err != nil {
					t.Fatal(err)
				}
				if schema.LanguagePolicy() != policy || len(schema.Documents()) != 1 || schema.Documents()[0].Source() != "root.xsd" {
					t.Fatalf("policy/documents = %s/%v", schema.LanguagePolicy(), schema.Documents())
				}
				found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Derived"))
				if len(found) != 1 || found[0].ID().Source() != "root.xsd" {
					t.Fatalf("owner = %v", found)
				}
				owner := requireTestComplexTypeDefinition(t, found[0], "Derived")
				base, hasBase := owner.BaseReference()
				baseFound := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Base"))
				baseID, hasBaseID := base.ComponentID()
				if owner.Loc() != complexContentTestLoc(t, root, `<xs:complexType name="Derived"`) || owner.Derivation() != ComplexTypeDerivationExtension || owner.Base() != mustTestQName(t, "urn:root", "Base") || owner.BaseLoc() != complexContentTestLoc(t, root, `base="t:Base"`) || !hasBase || !hasBaseID || len(baseFound) != 1 || baseID != baseFound[0].ID() || base.Loc() != owner.BaseLoc() {
					t.Fatalf("owner/base = %#v/%#v", owner, base)
				}
				group, hasGroup := owner.Particle().(ModelGroupReferenceParticle)
				groupFound := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, "urn:root", "Fields"))
				if !hasGroup || len(groupFound) != 1 || group.Ref() != mustTestQName(t, "urn:root", "Fields") || group.TargetID() != groupFound[0].ID() || group.Loc() != complexContentTestLoc(t, root, `<xs:group ref="t:Fields"`) || group.RefLoc() != complexContentTestLoc(t, root, `ref="t:Fields"`) || group.Occurrences().String() != "2/3" {
					t.Fatalf("group = %#v, target = %v", owner.Particle(), groupFound)
				}
				attributes := owner.AttributeUses()
				if len(attributes) != len(names)+2 || attributes[0].Name().Local() != "before" || attributes[len(attributes)-1].Name().Local() != "after" {
					t.Fatalf("ordered uses = %#v", attributes)
				}
				for index, name := range names {
					use, ok := attributes[index+1].(LocalAttributeUse)
					typeLoc := complexContentTestLoc(t, root, `type="xs:`+name+`"`)
					wantType := mustTestQName(t, testXSDNamespace, name)
					if !ok || use.Name() != mustTestQName(t, "", name) || use.DeclaredType() != wantType || use.Use() != AttributeUseRequired || use.Loc() != complexContentTestLoc(t, root, `<xs:attribute name="`+name+`"`) || use.NameLoc() != complexContentTestLoc(t, root, `name="`+name+`"`) || use.TypeLoc() != typeLoc || use.UseLoc() != allParticleTestTokenLoc(t, "root.xsd", root, `use="required"`, index+1) || use.FormLoc() != allParticleTestTokenLoc(t, "root.xsd", root, `form="unqualified"`, index+1) {
						t.Fatalf("%s use = %#v", name, attributes[index+1])
					}
					ref, hasRef := use.TypeReference()
					id, hasID := use.TypeID()
					refID, hasRefID := ref.ComponentID()
					if !hasRef || !ref.IsBuiltin() || ref.Kind() != SimpleTypeReferenceBuiltin || ref.Name() != wantType || ref.QName() != wantType || ref.Loc() != typeLoc || ref.VarietyLoc() != typeLoc || ref.Variety() != SimpleTypeVarietyAtomicRestriction || hasID || hasRefID || !id.IsZero() || !refID.IsZero() {
						t.Fatalf("%s type = %#v/%v/%v", name, ref, id, refID)
					}
					if _, hasWhiteSpace := ref.StringWhiteSpaceFacet(); hasWhiteSpace || ref.StringEnumerationFacets().HasEnumeration() {
						t.Fatalf("%s acquired user facets", name)
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
				markerFree := root
				for _, name := range names {
					markerFree = strings.Replace(markerFree, groupedLanguageAnyURIUse(name), "", 1)
				}
				markerFree = strings.Replace(markerFree, `<xs:attribute name="gone" type="xs:anyURI" use="prohibited"/>`, "", 1)
				control, controlErr := discoverTestSchemaWithPolicy(t, markerFree, nil, policy)
				if controlErr != nil {
					t.Fatal(controlErr)
				}
				controlOwner := modelGroupReferenceTestComplexTypeNamed(t, control, "urn:root", "Derived")
				controlGroup, hasControlGroup := controlOwner.Particle().(ModelGroupReferenceParticle)
				if !hasControlGroup {
					t.Fatalf("marker-free particle = %T", controlOwner.Particle())
				}
				controlBase, _ := controlOwner.BaseReference()
				controlBaseID, _ := controlBase.ComponentID()
				if !reflect.DeepEqual(componentIDs(control.Components()), componentIDs(schema.Components())) || controlGroup.TargetID() != group.TargetID() || controlGroup.Occurrences().String() != group.Occurrences().String() || controlBaseID != baseID || len(controlOwner.AttributeUses()) != 2 {
					t.Fatalf("marker-free owner changed identities/bounds: %#v", controlOwner)
				}
			})
		}
	}
}

func componentIDs(components []Component) []ComponentID {
	ids := make([]ComponentID, 0, len(components))
	for _, component := range components {
		ids = append(ids, component.ID())
	}
	return ids
}

//nolint:gocognit // Each consumer has its own stable located unsupported outcome.
func TestGroupedLanguageAnyURIConsumersUnsupported(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, name := range []string{"language", "anyURI"} {
			t.Run(string(policy)+"/"+name, func(t *testing.T) {
				root := groupedExtensionSchema("1.1", "", groupedLanguageAnyURIUse(name))
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err != nil {
					t.Fatal(err)
				}
				useLoc := complexContentTestLoc(t, root, `<xs:attribute name="`+name+`"`)
				ownerLoc := complexContentTestLoc(t, root, `<xs:complexType name="Derived"`)
				rootLoc := complexContentTestLoc(t, root, `<xs:element name="root"`)
				instanceErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
				vd := requireDiagnostic(t, instanceErr)
				if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != useLoc || vd.SpecRef() != instanceValidationSpecRef(languageUseVersion(policy)) || !reflect.DeepEqual(vd.Related(), []Loc{rootLoc, ownerLoc, useLoc}) || !errors.Is(instanceErr, ErrUnsupported) || !errors.Is(instanceErr, errInstanceAttributes) {
					t.Fatalf("validation = %v, related = %v", instanceErr, vd.Related())
				}
				output, generateErr := GenerateGo(schema, "generated")
				gd := requireDiagnostic(t, generateErr)
				if output != nil || gd.Class() != FailureUnsupported || gd.Code() != diagnosticCodegenUnsupported || gd.Loc() != useLoc || gd.SpecRef() != codegenDirectSequenceSpecReference(languageUseVersion(policy), codegenDirectSequenceParticlesReference) || !reflect.DeepEqual(gd.Related(), []Loc{ownerLoc, useLoc}) || !errors.Is(generateErr, ErrUnsupported) || !errors.Is(generateErr, errCodegenUnsupported) {
					t.Fatalf("generation = %v/%v, related = %v", output, generateErr, gd.Related())
				}
			})
		}
	}
}

//nolint:gocognit // Each excluded declaration form is a distinct public parse exit.
func TestGroupedLanguageAnyURIExcludedUseForms(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, name := range []string{"language", "anyURI"} {
			alias := `<xs:simpleType name="Alias"><xs:restriction base="xs:` + name + `"/></xs:simpleType>`
			inline := `<xs:simpleType><xs:restriction base="xs:` + name + `"/></xs:simpleType>`
			value := "en"
			if name == "anyURI" {
				value = "relative/path"
			}
			for _, test := range []struct {
				name, use, extra, marker, related string
				useSpec                           bool
				cause                             error
			}{
				{"inline atomic", `<xs:attribute name="a">` + inline + `</xs:attribute>`, "", `<xs:simpleType`, "", false, errSchemaAttributeUseUnsupported},
				{"inline list", `<xs:attribute name="a"><xs:simpleType><xs:list itemType="xs:` + name + `"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, "", false, errSchemaAttributeUseUnsupported},
				{"inline union", `<xs:attribute name="a"><xs:simpleType><xs:union memberTypes="xs:` + name + ` xs:` + name + `"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, "", false, errSchemaAttributeUseUnsupported},
				{"ref builtin", `<xs:attribute ref="t:a"/>`, `<xs:attribute name="a" type="xs:` + name + `"/>`, `ref="t:a"`, `<xs:attribute name="a"`, true, errSchemaAttributeReferenceUnsupported},
				{"ref named", `<xs:attribute ref="t:a"/>`, `<xs:attribute name="a" type="t:Alias"/>` + alias, `ref="t:a"`, `<xs:attribute name="a"`, true, errSchemaAttributeReferenceUnsupported},
				{"ref inline", `<xs:attribute ref="t:a"/>`, `<xs:attribute name="a">` + inline + `</xs:attribute>`, `ref="t:a"`, `<xs:attribute name="a"`, true, errSchemaAttributeReferenceUnsupported},
				{"direct default", `<xs:attribute name="a" type="xs:` + name + `" default="` + value + `"/>`, "", `default="` + value + `"`, "", true, errSchemaAttributeUseUnsupported},
				{"direct fixed", `<xs:attribute name="a" type="xs:` + name + `" fixed="` + value + `"/>`, "", `fixed="` + value + `"`, "", true, errSchemaAttributeUseUnsupported},
				{"named default", `<xs:attribute name="a" type="t:Alias" default="` + value + `"/>`, alias, `default="` + value + `"`, "", true, errSchemaAttributeUseUnsupported},
				{"named fixed", `<xs:attribute name="a" type="t:Alias" fixed="` + value + `"/>`, alias, `fixed="` + value + `"`, "", true, errSchemaAttributeUseUnsupported},
				{"inline default", `<xs:attribute name="a" default="` + value + `">` + inline + `</xs:attribute>`, "", `default="` + value + `"`, "", true, errSchemaAttributeUseUnsupported},
				{"inline fixed", `<xs:attribute name="a" fixed="` + value + `">` + inline + `</xs:attribute>`, "", `fixed="` + value + `"`, "", true, errSchemaAttributeUseUnsupported},
			} {
				t.Run(string(policy)+"/"+name+"/"+test.name, func(t *testing.T) {
					root := groupedExtensionSchema("1.1", "", groupedLanguageAnyURIUse(name)+test.use)
					root = strings.Replace(root, `</xs:schema>`, test.extra+`</xs:schema>`, 1)
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
						t.Fatalf("%s = %v; got %s/%s at %s spec %q related %v; want loc %s spec %q", test.name, err, d.Class(), d.Code(), d.Loc(), d.SpecRef(), d.Related(), complexContentTestLoc(t, root, test.marker), wantSpec)
					}
				})
			}
		}
	}
}

//nolint:gocognit // Syntax, group resolution, and base checks must survive omission.
func TestGroupedLanguageAnyURIOmissionAndAlternateExits(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, name := range []string{"language", "anyURI"} {
			t.Run(string(policy)+"/"+name, func(t *testing.T) {
				use := groupedLanguageAnyURIUse(name)
				zero := groupedExtensionSchema("1.1", ` minOccurs="0" maxOccurs="0"`, use+`<xs:attribute name="gone" type="xs:`+name+`" use="prohibited"/>`)
				schema, err := discoverTestSchemaWithPolicy(t, zero, nil, policy)
				if err != nil {
					t.Fatal(err)
				}
				owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Derived")
				if owner.Particle() != nil || len(owner.AttributeUses()) != 1 || owner.AttributeUses()[0].Name().Local() != name {
					t.Fatalf("zero group/prohibited use = %#v/%#v", owner.Particle(), owner.AttributeUses())
				}
				for _, test := range []struct {
					name, from, to, marker, related string
					class                           FailureClass
					code                            string
					cause                           error
				}{
					{"unresolved group", `ref="t:Fields"`, `ref="t:Missing"`, `ref="t:Missing"`, "", FailureInvalid, diagnosticSchemaModelGroupReferenceUnresolvedCode, errSchemaModelGroupReferenceUnresolved},
					{"wrong kind group", `ref="t:Fields"`, `ref="t:root"`, `ref="t:root"`, `<xs:element name="root"`, FailureInvalid, diagnosticSchemaModelGroupReferenceWrongKindCode, errSchemaModelGroupReferenceWrongKind},
					{"wrong kind base", `base="t:Base"`, `base="t:Fields"`, `base="t:Fields"`, `<xs:group name="Fields"`, FailureInvalid, invalidSchemaCompositionCode, errSchemaComplexTypeBaseWrongKind},
					{"nonempty base", `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:sequence><xs:element ref="t:root"/></xs:sequence></xs:complexType>`, `base="t:Base"`, `<xs:complexType name="Base"`, FailureUnsupported, UnsupportedSchemaSyntaxCode, errSchemaComplexTypeBaseNonEmpty},
					{"used base", `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:attribute name="inherited" type="xs:boolean"/></xs:complexType>`, `base="t:Base"`, `<xs:complexType name="Base"`, FailureUnsupported, UnsupportedSchemaSyntaxCode, errSchemaComplexTypeBaseUnsupported},
					{"final base", `<xs:complexType name="Base"/>`, `<xs:complexType name="Base" final="extension"/>`, `base="t:Base"`, `final="extension"`, FailureInvalid, invalidSchemaCompositionCode, errSchemaComplexTypeBaseUnsupported},
				} {
					t.Run(test.name, func(t *testing.T) {
						root := strings.Replace(zero, test.from, test.to, 1)
						failed, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
						assertZeroSchema(t, failed)
						d := requireDiagnostic(t, err)
						wantSpec := schemaComplexTypeExtensionSpecRef(languageUseVersion(policy))
						var related []Loc
						switch test.name {
						case "unresolved group", "wrong kind group":
							wantSpec = schemaModelGroupReferenceSpecRef(languageUseVersion(policy))
						}
						switch test.name {
						case "wrong kind group", "wrong kind base":
							related = []Loc{complexContentTestLoc(t, root, test.related)}
						case "nonempty base":
							related = []Loc{complexContentTestLoc(t, root, test.related), allParticleTestTokenLoc(t, "root.xsd", root, `<xs:sequence`, 2)}
						case "used base":
							related = []Loc{complexContentTestLoc(t, root, test.related)}
						case "final base":
							related = []Loc{complexContentTestLoc(t, root, test.related)}
						}
						if d.Class() != test.class || d.Code() != test.code || d.Loc() != complexContentTestLoc(t, root, test.marker) || d.SpecRef() != wantSpec || !reflect.DeepEqual(d.Related(), related) || !errors.Is(err, test.cause) {
							t.Fatalf("%s = %v; got spec %q related %v; want spec %q related %v", test.name, err, d.SpecRef(), d.Related(), wantSpec, related)
						}
					})
				}
			})
		}
	}
}

//nolint:gocognit // Source identities and visibility are public graph facts.
func TestGroupedLanguageAnyURIGraphProvenance(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" attributeFormDefault="qualified"><xs:include schemaLocation="shared.xsd"/><xs:include schemaLocation="again.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/><xs:complexType name="Derived"><xs:complexContent><xs:extension base="o:Base"><xs:group ref="r:Fields" minOccurs="1" maxOccurs="2"/>` + groupedLanguageAnyURIUse("language") + groupedLanguageAnyURIUse("anyURI") + `</xs:extension></xs:complexContent></xs:complexType></xs:schema>`
	shared := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other"><xs:include schemaLocation="root.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/><xs:group name="Fields"><xs:sequence><xs:element ref="o:item"/></xs:sequence></xs:group></xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:element name="item" type="xs:integer"/><xs:complexType name="Base"/></xs:schema>`
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
			if len(groups) != 1 || len(bases) != 1 || !ok || group.TargetID() != groups[0].ID() || group.RefLoc() != complexContentTestLoc(t, root, `ref="r:Fields"`) || group.Occurrences().String() != "1/2" || !hasBase || !hasBaseID || baseID != bases[0].ID() || owner.BaseLoc() != complexContentTestLoc(t, root, `base="o:Base"`) {
				t.Fatalf("graph group/base = %#v/%#v", group, base)
			}
			uses := owner.AttributeUses()
			if len(uses) != 2 || uses[0].Name() != mustTestQName(t, "", "language") || uses[1].Name() != mustTestQName(t, "", "anyURI") || uses[0].Loc() != complexContentTestLoc(t, root, `<xs:attribute name="language"`) || uses[1].Loc() != complexContentTestLoc(t, root, `<xs:attribute name="anyURI"`) {
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

func TestGroupedLanguageAnyURIInheritedWildcard(t *testing.T) {
	for _, name := range []string{"language", "anyURI"} {
		root := groupedExtensionSchema("1.1", "", groupedLanguageAnyURIUse(name))
		root = strings.Replace(root, `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>`, 1)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
		if err != nil {
			t.Fatal(err)
		}
		owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Derived")
		wildcard, ok := owner.AnyAttribute()
		if !ok || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" || wildcard.Loc() != complexContentTestLoc(t, root, `<xs:anyAttribute`) || len(owner.AttributeUses()) != 1 {
			t.Fatalf("%s wildcard = %#v", name, wildcard)
		}
	}
}

//nolint:gocognit // Facets and malformed syntax exit before the grouped type gate.
func TestGroupedLanguageAnyURIFacetAndSyntaxDiagnostics(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, name := range []string{"language", "anyURI"} {
			version := languageUseVersion(policy)
			t.Run(string(policy)+"/"+name+"/facet", func(t *testing.T) {
				root := groupedExtensionSchema("1.1", "", `<xs:attribute name="a" type="t:Alias"/>`)
				root = strings.Replace(root, `</xs:schema>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:`+name+`"><xs:pattern value=".*"/></xs:restriction></xs:simpleType></xs:schema>`, 1)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				wantSpec := "xsd11-datatypes#decimal"
				if version == XSDVersion10 {
					wantSpec = "xsd10-datatypes#decimal"
				}
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedDatatypeFacetCode || d.Loc() != complexContentTestLoc(t, root, `<xs:pattern`) || d.SpecRef() != wantSpec || len(d.Related()) != 0 || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("facet = %v", err)
				}
			})
			t.Run(string(policy)+"/"+name+"/invalid use", func(t *testing.T) {
				root := groupedExtensionSchema("1.1", "", `<xs:attribute name="a" type="xs:`+name+`" use="sometimes"/>`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.Loc() != complexContentTestLoc(t, root, `use="sometimes"`) || d.SpecRef() != schemaAttributeUseSpecRef(version) || len(d.Related()) != 0 {
					t.Fatalf("invalid use = %v", err)
				}
			})
			t.Run(string(policy)+"/"+name+"/anonymous owner", func(t *testing.T) {
				root := anonymousGroupedExtensionSchema("1.1", "", groupedLanguageAnyURIUse(name))
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != complexContentTestLoc(t, root, `type="xs:`+name+`"`) || d.SpecRef() != schemaAttributeTypeSpecRef(version) || len(d.Related()) != 0 || !errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaAttributeUseUnsupported) {
					t.Fatalf("anonymous grouped = %v", err)
				}
			})
		}
	}
}

//nolint:gocognit // Both newly admitted types retain pre-publication graph failures.
func TestGroupedLanguageAnyURIGraphCyclesAndVisibility(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, name := range []string{"language", "anyURI"} {
			version := languageUseVersion(policy)
			t.Run(string(policy)+"/"+name+"/group cycle", func(t *testing.T) {
				root := groupedExtensionSchema("1.1", "", groupedLanguageAnyURIUse(name))
				root = strings.Replace(root, `<xs:group name="Fields"><xs:sequence/></xs:group>`, `<xs:group name="Fields"><xs:sequence><xs:group ref="t:Fields"/></xs:sequence></xs:group>`, 1)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				wantSpec := "xsd11-structures#mg-props-correct"
				if version == XSDVersion10 {
					wantSpec = "xsd10-structures#mg-props-correct"
				}
				if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaModelGroupReferenceCycleCode || d.Loc() != allParticleTestTokenLoc(t, "root.xsd", root, `ref="t:Fields"`, 2) || d.SpecRef() != wantSpec || !reflect.DeepEqual(d.Related(), []Loc{complexContentTestLoc(t, root, `<xs:group name="Fields"`)}) || !errors.Is(err, errSchemaModelGroupReferenceCycle) {
					t.Fatalf("group cycle = %v, related = %v", err, d.Related())
				}
			})
			t.Run(string(policy)+"/"+name+"/base cycle", func(t *testing.T) {
				root := groupedExtensionSchema("1.1", "", groupedLanguageAnyURIUse(name))
				root = strings.Replace(root, `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:complexContent><xs:extension base="t:Derived"><xs:group ref="t:Fields"/><xs:attribute name="other" type="xs:integer"/></xs:extension></xs:complexContent></xs:complexType>`, 1)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.Loc() != complexContentTestLoc(t, root, `base="t:Derived"`) || d.SpecRef() != schemaComplexTypeExtensionSpecRef(version) || !reflect.DeepEqual(d.Related(), []Loc{complexContentTestLoc(t, root, `base="t:Base"`)}) || !errors.Is(err, errSchemaComplexTypeBaseCycle) {
					t.Fatalf("base cycle = %v, related = %v", err, d.Related())
				}
			})
			bridge := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge"><xs:import namespace="urn:foreign" schemaLocation="foreign.xsd"/></xs:schema>`
			foreign := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:foreign"><xs:group name="Fields"><xs:sequence/></xs:group><xs:complexType name="Base"/></xs:schema>`
			fixtures := map[string]discoveryFixture{"bridge.xsd": {id: "bridge.xsd", contents: bridge}, "foreign.xsd": {id: "foreign.xsd", contents: foreign}}
			for _, test := range []struct {
				name, base, group, marker, related, code, spec string
				cause                                          error
			}{
				{"invisible group", `xs:anyType`, `f:Fields`, `ref="f:Fields"`, `<xs:group name="Fields"`, diagnosticSchemaModelGroupReferenceNamespaceCode, schemaModelGroupReferenceImportSpecRef(version), errSchemaModelGroupReferenceNamespace},
				{"invisible base", `f:Base`, `r:Fields`, `base="f:Base"`, `<xs:complexType name="Base"`, invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(version), errSchemaComplexTypeBaseUnresolved},
			} {
				t.Run(string(policy)+"/"+name+"/"+test.name, func(t *testing.T) {
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:f="urn:foreign" targetNamespace="urn:root"><xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/><xs:group name="Fields"><xs:sequence/></xs:group><xs:complexType name="Derived"><xs:complexContent><xs:extension base="` + test.base + `"><xs:group ref="` + test.group + `"/>` + groupedLanguageAnyURIUse(name) + `</xs:extension></xs:complexContent></xs:complexType></xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy)
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					if d.Class() != FailureInvalid || d.Code() != test.code || d.Loc() != complexContentTestLoc(t, root, test.marker) || d.SpecRef() != test.spec || !reflect.DeepEqual(d.Related(), []Loc{complexContentTestSourceLoc(t, "foreign.xsd", foreign, test.related)}) || !errors.Is(err, test.cause) {
						t.Fatalf("%s = %v, related = %v", test.name, err, d.Related())
					}
				})
			}
		}
	}
}

func TestGroupedLanguageAnyURIProhibitedResolutionAndDuplicates(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, name := range []string{"language", "anyURI"} {
			version := languageUseVersion(policy)
			t.Run(string(policy)+"/"+name+"/prohibited unresolved", func(t *testing.T) {
				root := groupedExtensionSchema("1.1", ` minOccurs="0" maxOccurs="0"`, groupedLanguageAnyURIUse(name)+`<xs:attribute name="gone" type="t:Missing" use="prohibited"/>`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaAttributeTypeUnresolvedCode || d.Loc() != complexContentTestLoc(t, root, `type="t:Missing"`) || d.SpecRef() != schemaAttributeTypeSpecRef(version) || len(d.Related()) != 0 || !errors.Is(err, errSchemaAttributeTypeUnresolved) {
					t.Fatalf("prohibited type = %v", err)
				}
			})
			t.Run(string(policy)+"/"+name+"/duplicate group", func(t *testing.T) {
				root := groupedExtensionSchema("1.1", "", groupedLanguageAnyURIUse(name))
				root = strings.Replace(root, `<xs:group name="Fields"><xs:sequence/></xs:group>`, `<xs:group name="Fields"><xs:sequence/></xs:group><xs:group name="Fields"><xs:sequence/></xs:group>`, 1)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				first := allParticleTestTokenLoc(t, "root.xsd", root, `<xs:group name="Fields"`, 1)
				second := allParticleTestTokenLoc(t, "root.xsd", root, `<xs:group name="Fields"`, 2)
				if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaGlobalDuplicateCode || d.Loc() != second || d.SpecRef() != schemaGlobalDuplicateSpecRef(version) || !reflect.DeepEqual(d.Related(), []Loc{first}) || !errors.Is(err, errSchemaGlobalDeclarationDuplicate) {
					t.Fatalf("duplicate group = %v, related = %v", err, d.Related())
				}
			})
		}
	}
}
