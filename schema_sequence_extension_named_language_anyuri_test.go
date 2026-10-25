package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func namedSequenceLanguageAnyURIRoot(kind, use, extra string) string {
	root := sequenceLanguageAnyURIRoot(` minOccurs="2" maxOccurs="5"`, sequenceLanguageAnyURIMembers(), use,
		`<xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent>`)
	return strings.Replace(root, `</xs:schema>`, `<xs:simpleType name="Direct"><xs:restriction base="xs:`+kind+`"/></xs:simpleType><xs:simpleType name="Child"><xs:restriction base="t:Direct"/></xs:simpleType>`+extra+`</xs:schema>`, 1)
}

//nolint:gocognit,funlen // One public query checks the identity, copies, locations, and inherited facts together.
func TestSequenceExtensionNamedLanguageAnyURIPublicFacts(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, kind := range []string{"language", "anyURI"} {
			for _, label := range []string{"1.0", "1.1"} {
				t.Run(string(policy)+"/"+kind+"/"+label, func(t *testing.T) {
					uses := `<xs:attribute name="before" type="xs:boolean"/>` +
						`<xs:attribute name="direct" type="t:Direct" use="required"/>` +
						`<xs:attribute name="child" type="t:Child" form="unqualified"/>` +
						`<xs:attribute name="gone" type="t:Child" use="prohibited"/>` +
						`<xs:attribute name="after" type="xs:integer"/>`
					root := namedSequenceLanguageAnyURIRoot(kind, uses, "")
					root = strings.Replace(root, `version="1.1"`, `version="`+label+`"`, 1)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
					if err != nil {
						t.Fatal(err)
					}
					if schema.LanguagePolicy() != policy || len(schema.Documents()) != 1 || schema.Documents()[0].Source() != "root.xsd" {
						t.Fatalf("policy/documents = %s/%v", schema.LanguagePolicy(), schema.Documents())
					}
					owner := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Derived"))[0], "Derived")
					if owner.Loc() != complexContentTestLoc(t, root, `<xs:complexType name="Derived"`) || owner.Derivation() != ComplexTypeDerivationExtension || owner.DerivationLoc() != complexContentTestLoc(t, root, `<xs:extension`) {
						t.Fatalf("owner = %v", owner)
					}
					base, hasBase := owner.BaseReference()
					baseID, hasBaseID := base.ComponentID()
					baseComponents := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Base"))
					if !hasBase || !hasBaseID || len(baseComponents) != 1 || baseID != baseComponents[0].ID() || owner.BaseLoc() != complexContentTestLoc(t, root, `base="t:Base"`) {
						t.Fatalf("base = %v", base)
					}
					wildcard, hasWildcard := owner.AnyAttribute()
					if !hasWildcard || wildcard.Loc() != complexContentTestLoc(t, root, `<xs:anyAttribute`) || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" {
						t.Fatalf("wildcard = %v", wildcard)
					}
					sequence, ok := owner.Particle().(SequenceParticle)
					if !ok || sequence.Loc() != complexContentTestLoc(t, root, `<xs:sequence minOccurs=`) || sequence.Occurrences().String() != "2/5" || len(sequence.Particles()) != 2 || sequence.Particles()[0].Occurrences().String() != "2/3" || sequence.Particles()[1].Occurrences().String() != "0/4" {
						t.Fatalf("sequence = %v", owner.Particle())
					}
					if sequence.Particles()[0].Loc() != complexContentTestLoc(t, root, `<xs:element name="local"`) || sequence.Particles()[1].Loc() != complexContentTestLoc(t, root, `<xs:element ref="t:target"`) {
						t.Fatalf("member locations = %v", sequence.Particles())
					}
					memberRef, ok := sequence.Particles()[1].(ElementReferenceParticle)
					if !ok || memberRef.Ref() != mustTestQName(t, "urn:root", "target") || memberRef.RefLoc() != complexContentTestLoc(t, root, `ref="t:target"`) {
						t.Fatalf("member reference = %v", sequence.Particles()[1])
					}
					usesView := owner.AttributeUses()
					if len(usesView) != 4 || usesView[0].Name().Local() != "before" || usesView[3].Name().Local() != "after" {
						t.Fatalf("uses = %v", usesView)
					}
					for i, want := range []string{"Direct", "Child"} {
						use, ok := usesView[i+1].(LocalAttributeUse)
						name := strings.ToLower(want)
						typeLoc := complexContentTestLoc(t, root, `type="t:`+want+`"`)
						qname := mustTestQName(t, "urn:root", want)
						if !ok || use.Name().Local() != name || use.Loc() != complexContentTestLoc(t, root, `<xs:attribute name="`+name+`"`) || use.NameLoc() != complexContentTestLoc(t, root, `name="`+name+`"`) || use.TypeLoc() != typeLoc || use.DeclaredType() != qname {
							t.Fatalf("use %d = %v", i, usesView[i+1])
						}
						if i == 0 && (use.Use() != AttributeUseRequired || use.UseLoc() != complexContentTestLoc(t, root, `use="required"`)) {
							t.Fatalf("required use = %v", use)
						}
						if i == 1 && use.FormLoc() != complexContentTestLoc(t, root, `form="unqualified"`) {
							t.Fatalf("unqualified use = %v", use)
						}
						ref, hasRef := use.TypeReference()
						id, hasID := use.TypeID()
						refID, hasRefID := ref.ComponentID()
						if !hasRef || !ref.IsNamed() || ref.IsBuiltin() || ref.Name() != qname || ref.QName() != qname || ref.Loc() != typeLoc || ref.Variety() != SimpleTypeVarietyAtomicRestriction || !hasID || !hasRefID || id != refID || id != componentIDForName(t, schema, qname) || ref.StringEnumerationFacets().HasEnumeration() {
							t.Fatalf("type %d = %v/%v", i, use, ref)
						}
						if _, hasSpace := ref.StringWhiteSpaceFacet(); hasSpace {
							t.Fatalf("authored whitespace for %s", want)
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
						if !hasBaseRef || baseRef.Name() != baseName || baseRef.Variety() != SimpleTypeVarietyAtomicRestriction || baseRef.IsBuiltin() != (want == "Direct") {
							t.Fatalf("effective base %s = %v", want, baseRef)
						}
						if _, hasSpace := definition.StringWhiteSpaceFacet(); hasSpace {
							t.Fatalf("authored definition whitespace for %s", want)
						}
					}
					usesView[0] = nil
					parts := sequence.Particles()
					parts[0] = nil
					components := schema.Components()
					components[0] = Component{}
					if owner.AttributeUses()[0] == nil || sequence.Particles()[0] == nil || schema.Components()[0].Name().IsZero() {
						t.Fatal("public copy changed schema")
					}
					var first, second []ComponentID
					for _, walk := range []*[]ComponentID{&first, &second} {
						if err := schema.Walk(func(c Component) error { *walk = append(*walk, c.ID()); return nil }); err != nil {
							t.Fatal(err)
						}
					}
					if !reflect.DeepEqual(first, second) || len(first) != 7 {
						t.Fatalf("walk = %v/%v", first, second)
					}
					wantOrder := []string{"root", "Derived", "target", "Base", "NamedID", "Direct", "Child"}
					for i, component := range schema.Components() {
						if component.Name().Local() != wantOrder[i] || component.ID() != first[i] {
							t.Fatalf("component %d = %v", i, component)
						}
					}
				})
			}
		}
	}
}

func TestSequenceExtensionNamedLanguageAnyURIConsumers(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, kind := range []string{"language", "anyURI"} {
			root := namedSequenceLanguageAnyURIRoot(kind, `<xs:attribute name="value" type="t:Child"/>`, "")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatal(err)
			}
			validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
			vd := requireDiagnostic(t, validationErr)
			wantRelated := []Loc{complexContentTestLoc(t, root, `<xs:element name="root"`), complexContentTestLoc(t, root, `<xs:complexType name="Derived"`), complexContentTestLoc(t, root, `<xs:sequence`), complexContentTestLoc(t, root, `<xs:complexContent`), complexContentTestLoc(t, root, `<xs:extension`), complexContentTestLoc(t, root, `base="t:Base"`), complexContentTestLoc(t, root, `<xs:anyAttribute`)}
			if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != mustTestLoc(t, "instance.xml", 1, 1) || vd.SpecRef() != instanceValidationSpecRef(languageUseVersion(policy)) || !reflect.DeepEqual(vd.Related(), wantRelated) || !errors.Is(validationErr, ErrUnsupported) || !errors.Is(validationErr, errInstanceComplexContentExtension) {
				t.Fatalf("validation = %v related=%v", validationErr, vd.Related())
			}
			output, generationErr := GenerateGo(schema, "generated")
			gd := requireDiagnostic(t, generationErr)
			useLoc := complexContentTestLoc(t, root, `<xs:attribute name="value"`)
			if output != nil || gd.Class() != FailureUnsupported || gd.Code() != diagnosticCodegenUnsupported || gd.Loc() != useLoc || gd.SpecRef() != codegenDirectSequenceSpecReference(languageUseVersion(policy), codegenDirectSequenceParticlesReference) || !reflect.DeepEqual(gd.Related(), []Loc{complexContentTestLoc(t, root, `<xs:complexType name="Derived"`), useLoc}) || !errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenUnsupported) {
				t.Fatalf("generation = %v/%v related=%v", output, generationErr, gd.Related())
			}
		}
	}
}

//nolint:gocognit // Public graph provenance crosses both namespace-preserving graph edges.
func TestSequenceExtensionNamedLanguageAnyURIGraph(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, kind := range []string{"language", "anyURI"} {
			root := namedSequenceLanguageAnyURIRoot(kind, `<xs:attribute name="included" type="t:Included"/><xs:attribute name="imported" type="o:Imported"/>`, "")
			root = strings.Replace(root, `xmlns:t="urn:root"`, `xmlns:t="urn:root" xmlns:o="urn:other"`, 1)
			root = strings.Replace(root, `<xs:element name="root"`, `<xs:include schemaLocation="included.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/><xs:element name="root"`, 1)
			included := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:simpleType name="Included"><xs:restriction base="xs:` + kind + `"/></xs:simpleType></xs:schema>`
			imported := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Imported"><xs:restriction base="xs:` + kind + `"/></xs:simpleType></xs:schema>`
			fixtures := map[string]discoveryFixture{"included.xsd": {id: "included.xsd", contents: included}, "other.xsd": {id: "other.xsd", contents: imported}}
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy)
			if err != nil {
				t.Fatal(err)
			}
			owner := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Derived"))[0], "Derived")
			uses := owner.AttributeUses()
			if len(uses) != 2 {
				t.Fatalf("graph uses = %v", uses)
			}
			for i, want := range []struct{ local, namespace, source, contents string }{{"Included", "urn:root", "included.xsd", included}, {"Imported", "urn:other", "other.xsd", imported}} {
				use, ok := uses[i].(LocalAttributeUse)
				if !ok {
					t.Fatalf("use = %T", uses[i])
				}
				qname := mustTestQName(t, want.namespace, want.local)
				id, hasID := use.TypeID()
				ref, hasRef := use.TypeReference()
				if !hasID || !hasRef || !ref.IsNamed() || use.DeclaredType() != qname || ref.QName() != qname || id.Source() != SourceID(want.source) || ref.Loc() != complexContentTestLoc(t, root, `type="`+[]string{"t:Included", "o:Imported"}[i]+`"`) {
					t.Fatalf("resolved use %d = %v/%v", i, use, ref)
				}
				definition := schemaEnumerationTestDefinitionInNamespace(t, schema, want.namespace, want.local)
				if definition.ID() != id || definition.Loc() != complexContentTestSourceLoc(t, SourceID(want.source), want.contents, `<xs:simpleType name="`+want.local+`"`) {
					t.Fatalf("definition %s = %v", want.local, definition)
				}
			}
			wantSources := []SourceID{"root.xsd", "included.xsd", "other.xsd"}
			for i, document := range schema.Documents() {
				if document.Source() != wantSources[i] {
					t.Fatalf("document order = %v", schema.Documents())
				}
			}
		}
	}
}

//nolint:gocognit // Alternate exits share the owner boundary and must preserve their causes.
func TestSequenceExtensionNamedLanguageAnyURIAlternateExits(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		version := languageUseVersion(policy)
		for _, kind := range []string{"language", "anyURI"} {
			for _, test := range []struct {
				name, use, extra, replaceOld, replaceNew, marker, related, code, spec string
				class                                                                 FailureClass
				cause                                                                 error
			}{
				{"invalid occurrence", `<xs:attribute name="value" type="t:Child"/>`, "", `<xs:sequence minOccurs="2" maxOccurs="5">`, `<xs:sequence minOccurs="5" maxOccurs="2">`, `<xs:sequence minOccurs="5"`, "", invalidSchemaCompositionCode, schemaParticleCorrectSpecRef(version), FailureInvalid, errParticleOccurrenceMinimumExceedsMaximum},
				{"missing type", `<xs:attribute name="value" type="t:Missing"/>`, "", "", "", `type="t:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(version), FailureInvalid, errSchemaAttributeTypeUnresolved},
				{"wrong kind", `<xs:attribute name="value" type="t:target"/>`, "", "", "", `type="t:target"`, `<xs:element name="target"`, diagnosticSchemaAttributeTypeWrongKindCode, schemaAttributeTypeSpecRef(version), FailureInvalid, errSchemaAttributeTypeWrongKind},
				{"cycle", `<xs:attribute name="value" type="t:Direct"/>`, "", `<xs:restriction base="xs:` + kind + `"/>`, `<xs:restriction base="t:Child"/>`, `type="t:Direct"`, "", diagnosticSchemaAttributeTypeCycleCode, schemaAttributeTypeSpecRef(version), FailureInvalid, errSchemaSimpleTypeBaseCycle},
				{"authored pattern", `<xs:attribute name="value" type="t:Direct"/>`, "", `<xs:restriction base="xs:` + kind + `"/>`, `<xs:restriction base="xs:` + kind + `"><xs:pattern value=".*"/></xs:restriction>`, `<xs:pattern`, "", UnsupportedDatatypeFacetCode, tokenDiagnosticSpecRef(version, "decimal"), FailureUnsupported, ErrUnsupported},
				{"authored inherited enumeration", `<xs:attribute name="value" type="t:Child"/>`, "", `<xs:restriction base="xs:` + kind + `"/>`, `<xs:restriction base="xs:` + kind + `"><xs:enumeration value="en"/></xs:restriction>`, `<xs:enumeration`, "", UnsupportedDatatypeFacetCode, tokenDiagnosticSpecRef(version, "decimal"), FailureUnsupported, ErrUnsupported},
				{"missing base", `<xs:attribute name="value" type="t:Child"/>`, "", `base="t:Base"`, `base="t:MissingBase"`, `base="t:MissingBase"`, "", invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(version), FailureInvalid, errSchemaComplexTypeBaseUnresolved},
				{"wrong-kind base", `<xs:attribute name="value" type="t:Child"/>`, "", `base="t:Base"`, `base="t:Direct"`, `base="t:Direct"`, `<xs:simpleType name="Direct"`, invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(version), FailureInvalid, errSchemaComplexTypeBaseWrongKind},
				{"base cycle", `<xs:attribute name="value" type="t:Child"/>`, "", `base="t:Base"`, `base="t:Derived"`, `base="t:Derived"`, "", invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(version), FailureInvalid, errSchemaComplexTypeBaseCycle},
				{"base final", `<xs:attribute name="value" type="t:Child"/>`, "", `<xs:complexType name="Base">`, `<xs:complexType name="Base" final="extension">`, `base="t:Base"`, `final="extension"`, invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(version), FailureInvalid, errSchemaComplexTypeBaseUnsupported},
				{"nonempty base", `<xs:attribute name="value" type="t:Child"/>`, "", `<xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent>`, `<xs:sequence><xs:element name="baseValue" type="xs:integer"/></xs:sequence>`, `base="t:Base"`, `<xs:sequence><xs:element name="baseValue"`, UnsupportedSchemaSyntaxCode, schemaComplexTypeExtensionSpecRef(version), FailureUnsupported, errSchemaComplexTypeBaseNonEmpty},
				{"used base", `<xs:attribute name="value" type="t:Child"/>`, "", `<xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent>`, `<xs:attribute name="inherited" type="xs:boolean"/>`, `base="t:Base"`, `<xs:complexType name="Base"`, UnsupportedSchemaSyntaxCode, schemaComplexTypeExtensionSpecRef(version), FailureUnsupported, errSchemaComplexTypeBaseUnsupported},
				{"attribute-only owner", `<xs:attribute name="value" type="t:Child"/>`, "", `<xs:sequence minOccurs="2" maxOccurs="5">` + sequenceLanguageAnyURIMembers() + `</xs:sequence>`, "", `<xs:attribute name="value"`, "", UnsupportedSchemaSyntaxCode, schemaComplexContentExtensionSpecRef(version), FailureUnsupported, ErrUnsupported},
				{"choice owner", `<xs:attribute name="value" type="t:Child"/>`, "", `<xs:sequence minOccurs="2" maxOccurs="5">`, `<xs:choice minOccurs="2" maxOccurs="5">`, `<xs:attribute name="value"`, "", UnsupportedSchemaSyntaxCode, schemaComplexTypeExtensionSpecRef(version), FailureUnsupported, ErrUnsupported},
			} {
				t.Run(string(policy)+"/"+kind+"/"+test.name, func(t *testing.T) {
					root := namedSequenceLanguageAnyURIRoot(kind, test.use, test.extra)
					if test.name == "choice owner" {
						root = strings.Replace(root, `</xs:sequence>`, `</xs:choice>`, 1)
					}
					if test.replaceOld != "" {
						root = strings.Replace(root, test.replaceOld, test.replaceNew, 1)
					}
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					var related []Loc
					if test.related != "" {
						related = []Loc{complexContentTestLoc(t, root, test.related)}
					}
					if test.name == "cycle" {
						related = []Loc{complexContentTestLoc(t, root, `<xs:attribute name="value"`), complexContentTestLoc(t, root, `base="t:Child"`), complexContentTestLoc(t, root, `base="t:Direct"`), complexContentTestLoc(t, root, `<xs:simpleType name="Direct"`), complexContentTestLoc(t, root, `<xs:simpleType name="Child"`)}
					}
					if test.name == "invalid occurrence" {
						related = []Loc{complexContentTestLoc(t, root, `minOccurs="5"`), complexContentTestLoc(t, root, `maxOccurs="2"`)}
					}
					if test.name == "nonempty base" {
						related = []Loc{complexContentTestLoc(t, root, `<xs:complexType name="Base"`), complexContentTestLoc(t, root, test.related)}
					}
					if d.Class() != test.class || d.Code() != test.code || d.Loc() != complexContentTestLoc(t, root, test.marker) || d.SpecRef() != test.spec || !reflect.DeepEqual(d.Related(), related) || !errors.Is(err, test.cause) {
						t.Fatalf("exit = %v class=%v code=%v loc=%v wantloc=%v spec=%v wantspec=%v related=%v wantrelated=%v cause=%v is=%t", err, d.Class(), d.Code(), d.Loc(), complexContentTestLoc(t, root, test.marker), d.SpecRef(), test.spec, d.Related(), related, test.cause, errors.Is(err, test.cause))
					}
				})
			}
		}
	}
}

//nolint:gocognit // Named direct, inline, ref, and value forms have distinct exits.
func TestSequenceExtensionNamedLanguageAnyURIExcludedForms(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, kind := range []string{"language", "anyURI"} {
			value := "en"
			if kind == "anyURI" {
				value = "../private.xsd"
			}
			for _, test := range []struct {
				name, use, extra, marker, related string
				cause                             error
				useSpec                           bool
			}{
				{"inline restriction", `<xs:attribute name="a"><xs:simpleType><xs:restriction base="t:Child"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType><xs:restriction`, "", errSchemaAttributeUseUnsupported, false},
				{"inline list", `<xs:attribute name="a"><xs:simpleType><xs:list itemType="t:Child"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType><xs:list`, `<xs:list`, errSchemaAttributeUseUnsupported, true},
				{"inline union", `<xs:attribute name="a"><xs:simpleType><xs:union memberTypes="t:Child"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType><xs:union`, `<xs:union`, errSchemaAttributeUseUnsupported, true},
				{"named list", `<xs:attribute name="a" type="t:Broader"/>`, `<xs:simpleType name="Broader"><xs:list itemType="t:Child"/></xs:simpleType>`, `type="t:Broader"`, "", errSchemaAttributeUseUnsupported, true},
				{"named union", `<xs:attribute name="a" type="t:Broader"/>`, `<xs:simpleType name="Broader"><xs:union memberTypes="t:Child"/></xs:simpleType>`, `type="t:Broader"`, "", errSchemaAttributeUseUnsupported, true},
				{"ref named", `<xs:attribute ref="t:a"/>`, `<xs:attribute name="a" type="t:Child"/>`, `ref="t:a"`, `<xs:attribute name="a" type="t:Child"`, errSchemaAttributeReferenceUnsupported, true},
				{"ref inline", `<xs:attribute ref="t:a"/>`, `<xs:attribute name="a"><xs:simpleType><xs:restriction base="t:Child"/></xs:simpleType></xs:attribute>`, `ref="t:a"`, `<xs:attribute name="a"><xs:simpleType`, errSchemaAttributeReferenceUnsupported, true},
				{"named default", `<xs:attribute name="a" type="t:Child" default="` + value + `"/>`, "", `default="` + value + `"`, "", errSchemaAttributeUseUnsupported, true},
				{"named fixed", `<xs:attribute name="a" type="t:Child" fixed="` + value + `"/>`, "", `fixed="` + value + `"`, "", errSchemaAttributeUseUnsupported, true},
			} {
				t.Run(string(policy)+"/"+kind+"/"+test.name, func(t *testing.T) {
					root := namedSequenceLanguageAnyURIRoot(kind, test.use, test.extra)
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
					if test.name == "named list" {
						related = []Loc{complexContentTestLoc(t, root, `<xs:simpleType name="Broader"`), complexContentTestLoc(t, root, `<xs:list`)}
					}
					if test.name == "named union" {
						related = []Loc{complexContentTestLoc(t, root, `<xs:simpleType name="Broader"`), complexContentTestLoc(t, root, `<xs:union`)}
					}
					if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != complexContentTestLoc(t, root, test.marker) || d.SpecRef() != wantSpec || !reflect.DeepEqual(d.Related(), related) || !errors.Is(err, ErrUnsupported) || !errors.Is(err, test.cause) {
						t.Fatalf("excluded form = %v spec=%v related=%v", err, d.SpecRef(), d.Related())
					}
				})
			}
		}
	}
}

//nolint:gocognit // Final on a simple type restricts derivation, while extension final belongs to the complex base.
func TestSequenceExtensionNamedLanguageAnyURIFinalAndOmission(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, kind := range []string{"language", "anyURI"} {
			root := namedSequenceLanguageAnyURIRoot(kind, `<xs:attribute name="value" type="t:Direct"/>`, "")
			root = strings.Replace(root, `<xs:simpleType name="Direct">`, `<xs:simpleType name="Direct" final="restriction">`, 1)
			root = strings.Replace(root, `<xs:simpleType name="Child"><xs:restriction base="t:Direct"/></xs:simpleType>`, "", 1)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatalf("final type use: %v", err)
			}
			owner := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Derived"))[0], "Derived")
			if len(owner.AttributeUses()) != 1 || owner.AttributeUses()[0].Name().Local() != "value" {
				t.Fatalf("final type use = %v", owner.AttributeUses())
			}
			broken := strings.Replace(root, `</xs:schema>`, `<xs:simpleType name="Child"><xs:restriction base="t:Direct"/></xs:simpleType></xs:schema>`, 1)
			broken = strings.Replace(broken, `type="t:Direct"`, `type="t:Child"`, 1)
			badSchema, badErr := discoverTestSchemaWithPolicy(t, broken, nil, policy)
			assertZeroSchema(t, badSchema)
			d := requireDiagnostic(t, badErr)
			if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaSimpleTypeBaseCode || d.Loc() != complexContentTestLoc(t, broken, `base="t:Direct"`) || d.SpecRef() != schemaSimpleTypeRestrictionSpecRef(languageUseVersion(policy)) || !reflect.DeepEqual(d.Related(), []Loc{complexContentTestLoc(t, broken, `final="restriction"`)}) || !errors.Is(badErr, errSchemaSimpleTypeInvalidDerivation) {
				t.Fatalf("final restriction = %v related=%v", badErr, d.Related())
			}
			omitted := namedSequenceLanguageAnyURIRoot(kind, `<xs:attribute name="gone" type="t:Child" use="prohibited"/><xs:attribute name="keep" type="t:Direct"/>`, "")
			omitted = strings.Replace(omitted, `<xs:sequence minOccurs="2" maxOccurs="5">`, `<xs:sequence minOccurs="0" maxOccurs="0">`, 1)
			schema, err = discoverTestSchemaWithPolicy(t, omitted, nil, policy)
			if err != nil {
				t.Fatalf("omission: %v", err)
			}
			owner = requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Derived"))[0], "Derived")
			if owner.Particle() != nil || len(owner.AttributeUses()) != 1 || owner.AttributeUses()[0].Name().Local() != "keep" {
				t.Fatalf("omitted model/uses = %v/%v", owner.Particle(), owner.AttributeUses())
			}
			for _, test := range []struct {
				name, old, replacement, marker, code string
				cause                                error
			}{
				{"unresolved omitted child", `ref="t:target"`, `ref="t:Missing"`, `ref="t:Missing"`, diagnosticSchemaElementReferenceUnresolvedCode, errSchemaElementReferenceUnresolved},
				{"unresolved prohibited use", `type="t:Child"`, `type="t:Missing"`, `type="t:Missing"`, diagnosticSchemaAttributeTypeUnresolvedCode, errSchemaAttributeTypeUnresolved},
			} {
				bad := strings.Replace(omitted, test.old, test.replacement, 1)
				badSchema, badErr := discoverTestSchemaWithPolicy(t, bad, nil, policy)
				assertZeroSchema(t, badSchema)
				d := requireDiagnostic(t, badErr)
				wantSpec := schemaElementReferenceSpecRef(languageUseVersion(policy))
				if test.name == "unresolved prohibited use" {
					wantSpec = schemaAttributeTypeSpecRef(languageUseVersion(policy))
				}
				if d.Class() != FailureInvalid || d.Code() != test.code || d.Loc() != complexContentTestLoc(t, bad, test.marker) || d.SpecRef() != wantSpec || len(d.Related()) != 0 || !errors.Is(badErr, test.cause) {
					t.Fatalf("%s = %v related=%v", test.name, badErr, d.Related())
				}
			}
		}
	}
}

func TestSequenceExtensionNamedLanguageAnyURIInvisibleType(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, kind := range []string{"language", "anyURI"} {
			root := namedSequenceLanguageAnyURIRoot(kind, `<xs:attribute name="value" type="o:Hidden"/>`, "")
			testNamedLanguageAnyURIInvisibleType(t, root, kind, policy)
		}
	}
}

// Named aliases remain unavailable to anonymous extension owners.
func TestSequenceExtensionNamedLanguageAnyURIExcludedOwners(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, kind := range []string{"language", "anyURI"} {
			for _, test := range []struct{ name, body, extra, marker string }{
				{"anonymous extension", `<xs:complexType name="Base"/><xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:Base"><xs:sequence><xs:element name="local" type="xs:integer"/></xs:sequence><xs:attribute name="value" type="t:Child"/></xs:extension></xs:complexContent></xs:complexType></xs:element>`, "", `type="t:Child"`},
			} {
				t.Run(string(policy)+"/"+kind+"/"+test.name, func(t *testing.T) {
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root">` + test.body + `<xs:simpleType name="Direct"><xs:restriction base="xs:` + kind + `"/></xs:simpleType><xs:simpleType name="Child"><xs:restriction base="t:Direct"/></xs:simpleType>` + test.extra + `</xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != complexContentTestLoc(t, root, test.marker) || d.SpecRef() != schemaAttributeTypeSpecRef(languageUseVersion(policy)) || len(d.Related()) != 0 || !errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaAttributeUseUnsupported) {
						t.Fatalf("excluded owner = %v related=%v", err, d.Related())
					}
				})
			}
		}
	}
}

func TestSequenceExtensionNamedLanguageAnyURIPrecedence(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, kind := range []string{"language", "anyURI"} {
			root := namedSequenceLanguageAnyURIRoot(kind, `<xs:attribute name="value" type="t:Missing"/>`, "")
			root = strings.Replace(root, `<xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent>`, `<xs:attribute name="inherited" type="xs:boolean"/>`, 1)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaAttributeTypeUnresolvedCode || d.Loc() != complexContentTestLoc(t, root, `type="t:Missing"`) || d.SpecRef() != schemaAttributeTypeSpecRef(languageUseVersion(policy)) || len(d.Related()) != 0 || !errors.Is(err, errSchemaAttributeTypeUnresolved) {
				t.Fatalf("attribute before unsupported base = %v related=%v", err, d.Related())
			}
			broken := strings.Replace(root, `ref="t:target"`, `ref="t:Absent"`, 1)
			schema, err = discoverTestSchemaWithPolicy(t, broken, nil, policy)
			assertZeroSchema(t, schema)
			d = requireDiagnostic(t, err)
			if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaElementReferenceUnresolvedCode || d.Loc() != complexContentTestLoc(t, broken, `ref="t:Absent"`) || d.SpecRef() != schemaElementReferenceSpecRef(languageUseVersion(policy)) || len(d.Related()) != 0 || !errors.Is(err, errSchemaElementReferenceUnresolved) {
				t.Fatalf("particle before attribute/base = %v related=%v", err, d.Related())
			}
		}
	}
}
