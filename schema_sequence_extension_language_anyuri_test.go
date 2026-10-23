package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func sequenceLanguageAnyURIUse(name string) string {
	return `<xs:attribute name="` + name + `" type="xs:` + name + `" use="required" form="unqualified"/>`
}

//nolint:gocognit // The owner and base retain source identity across supported graph edges.
func TestSequenceExtensionLanguageAnyURIGraphProvenance(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, graph := range []string{"forward", "included", "imported", "chameleon"} {
			for _, name := range []string{"language", "anyURI"} {
				t.Run(string(policy)+"/"+graph+"/"+name, func(t *testing.T) {
					fixture := emptyComplexContentExtensionGraph(t, "1.0", graph)
					use := sequenceLanguageAnyURIUse(name)
					fixture.root = strings.Replace(fixture.root, `<xs:extension base="`+fixture.baseLexical+`"/>`, `<xs:extension base="`+fixture.baseLexical+`"><xs:sequence minOccurs="1" maxOccurs="2"><xs:element name="value" type="xs:integer" minOccurs="0" maxOccurs="3"/></xs:sequence>`+use+`</xs:extension>`, 1)
					schema, err := discoverTestSchemaWithPolicy(t, fixture.root, fixture.fixtures, policy)
					if err != nil {
						t.Fatal(err)
					}
					owner := requireTestComplexTypeDefinition(t, schema.Components()[0], "Derived")
					baseRef, hasBase := owner.BaseReference()
					baseID, hasBaseID := baseRef.ComponentID()
					if !hasBase || !hasBaseID || baseID.Source() != fixture.baseSource || owner.BaseLoc() != complexContentTestLoc(t, fixture.root, `base="`+fixture.baseLexical+`"`) {
						t.Fatalf("base = %v", baseRef)
					}
					wildcard, hasWildcard := owner.AnyAttribute()
					if !hasWildcard || wildcard.Loc() != complexContentTestSourceLoc(t, fixture.baseSource, fixture.baseContents, `<xs:anyAttribute`) {
						t.Fatalf("wildcard = %v", wildcard)
					}
					particle, ok := owner.Particle().(SequenceParticle)
					uses := owner.AttributeUses()
					if !ok || particle.Occurrences().String() != "1/2" || particle.Elements()[0].Occurrences().String() != "0/3" || len(uses) != 1 || uses[0].Loc() != complexContentTestLoc(t, fixture.root, `<xs:attribute name="`+name+`"`) {
						t.Fatalf("particle/uses = %v/%v", owner.Particle(), uses)
					}
					local, localOK := uses[0].(LocalAttributeUse)
					if !localOK {
						t.Fatalf("use = %T", uses[0])
					}
					if local.DeclaredType() != mustTestQName(t, testXSDNamespace, name) || local.TypeLoc() != complexContentTestLoc(t, fixture.root, `type="xs:`+name+`"`) {
						t.Fatalf("use type = %v", local)
					}
				})
			}
		}
	}
}

func TestSequenceExtensionLanguageAnyURIAcquisitionCause(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		root := sequenceLanguageAnyURIRoot("", sequenceLanguageAnyURIMembers(), sequenceLanguageAnyURIUse("language"), "")
		root = strings.Replace(root, `<xs:element name="root"`, `<xs:include schemaLocation="missing.xsd"/><xs:element name="root"`, 1)
		source, sourceErr := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
		if sourceErr != nil {
			t.Fatal(sourceErr)
		}
		cause := errors.New("sequence-extension source unavailable")
		schema, err := ParseSchemaWithPolicy(source, &discoveryResolver{failures: map[string]error{"missing.xsd": cause}}, policy)
		assertZeroSchema(t, schema)
		d := requireDiagnostic(t, err)
		if d.Class() != FailureResolution || d.Code() != SourceResolveCode || d.Loc() != complexContentTestLoc(t, root, `<xs:include`) || d.SpecRef() != "" || len(d.Related()) != 0 || !errors.Is(err, cause) {
			t.Fatalf("acquisition = %v related=%v", err, d.Related())
		}
	}
}

//nolint:gocognit // Each alternate exit keeps its first semantic error before omission or publication.
func TestSequenceExtensionLanguageAnyURIAlternateExits(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		version := languageUseVersion(policy)
		for _, name := range []string{"language", "anyURI"} {
			for _, test := range []struct {
				name, outer, members, uses, base, baseName, marker, related, code, spec string
				class                                                                   FailureClass
				cause                                                                   error
			}{
				{"invalid occurrence before type/base", ` minOccurs="2" maxOccurs="1"`, `<xs:element name="value" type="xs:integer"/>`, sequenceLanguageAnyURIUse(name), "", "MissingBase", `<xs:sequence minOccurs="2"`, `minOccurs="2"`, invalidSchemaCompositionCode, schemaParticleCorrectSpecRef(version), FailureInvalid, errParticleOccurrenceMinimumExceedsMaximum},
				{"unresolved child before admitted use", ` minOccurs="0" maxOccurs="0"`, `<xs:element ref="t:Missing"/>`, sequenceLanguageAnyURIUse(name), "", "Base", `ref="t:Missing"`, "", diagnosticSchemaElementReferenceUnresolvedCode, schemaElementReferenceSpecRef(version), FailureInvalid, errSchemaElementReferenceUnresolved},
				{"prohibited unresolved type", ` minOccurs="0" maxOccurs="0"`, `<xs:element ref="t:target"/>`, sequenceLanguageAnyURIUse(name) + `<xs:attribute name="gone" type="t:Missing" use="prohibited"/>`, "", "Base", `type="t:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(version), FailureInvalid, errSchemaAttributeTypeUnresolved},
				{"wrong-kind type", "", sequenceLanguageAnyURIMembers(), sequenceLanguageAnyURIUse(name) + `<xs:attribute name="bad" type="t:target"/>`, "", "Base", `type="t:target"`, `<xs:element name="target"`, diagnosticSchemaAttributeTypeWrongKindCode, schemaAttributeTypeSpecRef(version), FailureInvalid, errSchemaAttributeTypeWrongKind},
				{"duplicate use", "", sequenceLanguageAnyURIMembers(), sequenceLanguageAnyURIUse(name) + `<xs:attribute name="` + name + `" type="xs:boolean" form="unqualified"/>`, "", "Base", `<xs:attribute name="` + name + `" type="xs:boolean"`, `<xs:attribute name="` + name + `" type="xs:` + name + `"`, diagnosticSchemaAttributeUseDuplicateCode, schemaAttributeUseSpecRef(version), FailureInvalid, errSchemaAttributeUseDuplicate},
				{"unresolved base", "", sequenceLanguageAnyURIMembers(), sequenceLanguageAnyURIUse(name), "", "MissingBase", `base="t:MissingBase"`, "", invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(version), FailureInvalid, errSchemaComplexTypeBaseUnresolved},
				{"wrong-kind base", "", sequenceLanguageAnyURIMembers(), sequenceLanguageAnyURIUse(name), "", "NamedID", `base="t:NamedID"`, `<xs:simpleType name="NamedID"`, invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(version), FailureInvalid, errSchemaComplexTypeBaseWrongKind},
				{"base final", "", sequenceLanguageAnyURIMembers(), sequenceLanguageAnyURIUse(name), "", "Base", `base="t:Base"`, `final="extension"`, invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(version), FailureInvalid, errSchemaComplexTypeBaseUnsupported},
				{"nonempty base", "", sequenceLanguageAnyURIMembers(), sequenceLanguageAnyURIUse(name), `<xs:sequence><xs:element name="baseValue" type="xs:integer"/></xs:sequence>`, "Base", `base="t:Base"`, `<xs:sequence><xs:element name="baseValue"`, UnsupportedSchemaSyntaxCode, schemaComplexTypeExtensionSpecRef(version), FailureUnsupported, errSchemaComplexTypeBaseNonEmpty},
			} {
				t.Run(string(policy)+"/"+name+"/"+test.name, func(t *testing.T) {
					root := sequenceLanguageAnyURIRoot(test.outer, test.members, test.uses, test.base)
					if test.baseName != "Base" {
						root = strings.Replace(root, `base="t:Base"`, `base="t:`+test.baseName+`"`, 1)
					}
					if test.name == "base final" {
						root = strings.Replace(root, `<xs:complexType name="Base">`, `<xs:complexType name="Base" final="extension">`, 1)
					}
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					var related []Loc
					if test.related != "" {
						related = []Loc{complexContentTestLoc(t, root, test.related)}
					}
					if test.name == "nonempty base" {
						related = []Loc{complexContentTestLoc(t, root, `<xs:complexType name="Base"`), complexContentTestLoc(t, root, test.related)}
					}
					if test.name == "invalid occurrence before type/base" {
						related = []Loc{complexContentTestLoc(t, root, `minOccurs="2"`), complexContentTestLoc(t, root, `maxOccurs="1"`)}
					}
					if d.Class() != test.class || d.Code() != test.code || d.Loc() != complexContentTestLoc(t, root, test.marker) || d.SpecRef() != test.spec || !reflect.DeepEqual(d.Related(), related) || !errors.Is(err, test.cause) {
						t.Fatalf("%s = %v related=%v", test.name, err, d.Related())
					}
					if test.class == FailureUnsupported && !errors.Is(err, ErrUnsupported) {
						t.Fatalf("unsupported cause = %v", err)
					}
				})
			}
		}
	}
}

func TestSequenceExtensionLanguageAnyURIOmissionAndBaseCycle(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, name := range []string{"language", "anyURI"} {
			root := sequenceLanguageAnyURIRoot(` minOccurs="0" maxOccurs="0"`, `<xs:element ref="t:target"/>`, sequenceLanguageAnyURIUse(name)+`<xs:attribute name="gone" type="xs:`+name+`" use="prohibited"/>`, "")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatal(err)
			}
			owner := requireTestComplexTypeDefinition(t, schema.Components()[1], "Derived")
			if owner.Particle() != nil || len(owner.AttributeUses()) != 1 || owner.AttributeUses()[0].Name().Local() != name || owner.BaseLoc() != complexContentTestLoc(t, root, `base="t:Base"`) {
				t.Fatalf("omission = %v/%v", owner.Particle(), owner.AttributeUses())
			}
			cycle := strings.Replace(root, `base="t:Base"`, `base="t:Derived"`, 1)
			badSchema, cycleErr := discoverTestSchemaWithPolicy(t, cycle, nil, policy)
			assertZeroSchema(t, badSchema)
			d := requireDiagnostic(t, cycleErr)
			if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.Loc() != complexContentTestLoc(t, cycle, `base="t:Derived"`) || d.SpecRef() != schemaComplexTypeExtensionSpecRef(languageUseVersion(policy)) || len(d.Related()) != 0 || !errors.Is(cycleErr, errSchemaComplexTypeBaseCycle) {
				t.Fatalf("cycle = %v related=%v", cycleErr, d.Related())
			}
		}
	}
}

//nolint:gocognit // Policy and edition labels use one public QName fixture.
func TestSequenceExtensionLanguageAnyURIEditionLabelAndPrefix(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, label := range []string{"1.0", "1.1"} {
			for _, name := range []string{"language", "anyURI"} {
				root := sequenceLanguageAnyURIRoot("", `<xs:element name="value" type="xs:integer"/>`, `<xs:attribute name="value" type="builtin:`+name+`"/>`, "")
				root = strings.Replace(root, `version="1.1"`, `version="`+label+`"`, 1)
				root = strings.Replace(root, `xmlns:t="urn:root"`, `xmlns:t="urn:root" xmlns:builtin="`+testXSDNamespace+`"`, 1)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err != nil {
					t.Fatalf("%s/%s/%s: %v", policy, label, name, err)
				}
				owner := requireTestComplexTypeDefinition(t, schema.Components()[1], "Derived")
				uses := owner.AttributeUses()
				if len(uses) != 1 {
					t.Fatalf("uses = %v", uses)
				}
				use, useOK := uses[0].(LocalAttributeUse)
				if !useOK {
					t.Fatalf("use = %T", uses[0])
				}
				reference, ok := use.TypeReference()
				typeLoc := complexContentTestLoc(t, root, `type="builtin:`+name+`"`)
				if !ok || !reference.IsBuiltin() || use.DeclaredType() != mustTestQName(t, testXSDNamespace, name) || reference.QName() != use.DeclaredType() || reference.Loc() != typeLoc || use.TypeLoc() != typeLoc {
					t.Fatalf("bound type = %v/%v", use, reference)
				}
			}
		}
	}
}

func TestSequenceExtensionLanguageAnyURIInvisibleBase(t *testing.T) {
	foreign := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:foreign"><xs:complexType name="Base"/></xs:schema>`
	bridge := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge"><xs:import namespace="urn:foreign" schemaLocation="foreign.xsd"/></xs:schema>`
	fixtures := map[string]discoveryFixture{
		"bridge.xsd":  {id: "bridge.xsd", contents: bridge},
		"foreign.xsd": {id: "foreign.xsd", contents: foreign},
	}
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, name := range []string{"language", "anyURI"} {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:f="urn:foreign" targetNamespace="urn:root"><xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/><xs:complexType name="Derived"><xs:complexContent><xs:extension base="f:Base"><xs:sequence><xs:element name="value" type="xs:integer"/></xs:sequence>` + sequenceLanguageAnyURIUse(name) + `</xs:extension></xs:complexContent></xs:complexType></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy)
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.Loc() != complexContentTestLoc(t, root, `base="f:Base"`) || d.SpecRef() != schemaComplexTypeExtensionSpecRef(languageUseVersion(policy)) || !reflect.DeepEqual(d.Related(), []Loc{complexContentTestSourceLoc(t, "foreign.xsd", foreign, `<xs:complexType name="Base"`)}) || !errors.Is(err, errSchemaComplexTypeBaseUnresolved) {
				t.Fatalf("invisible base = %v related=%v", err, d.Related())
			}
		}
	}
}

func TestSequenceExtensionLanguageAnyURIDuplicateBasePreemptsUse(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, name := range []string{"language", "anyURI"} {
			root := sequenceLanguageAnyURIRoot("", `<xs:element name="value" type="xs:integer"/>`, sequenceLanguageAnyURIUse(name), "")
			root = strings.Replace(root, `</xs:schema>`, `<xs:complexType name="Base"/></xs:schema>`, 1)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			first := allParticleTestTokenLoc(t, "root.xsd", root, `<xs:complexType name="Base"`, 1)
			second := allParticleTestTokenLoc(t, "root.xsd", root, `<xs:complexType name="Base"`, 2)
			if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaGlobalDuplicateCode || d.Loc() != second || d.SpecRef() != schemaGlobalDuplicateSpecRef(languageUseVersion(policy)) || !reflect.DeepEqual(d.Related(), []Loc{first}) || !errors.Is(err, errSchemaGlobalDeclarationDuplicate) {
				t.Fatalf("duplicate base = %v related=%v", err, d.Related())
			}
		}
	}
}

func TestSequenceExtensionLanguageAnyURIPolicySyntax(t *testing.T) {
	for _, name := range []string{"language", "anyURI"} {
		use := strings.Replace(sequenceLanguageAnyURIUse(name), `/>`, ` inheritable="false"/>`, 1)
		root := sequenceLanguageAnyURIRoot("", `<xs:element name="value" type="xs:integer"/>`, use, "")
		for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil || len(requireTestComplexTypeDefinition(t, schema.Components()[1], "Derived").AttributeUses()) != 1 {
				t.Fatalf("%s/%s false inheritable = %v/%v", policy, name, schema, err)
			}
		}
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
		assertZeroSchema(t, schema)
		d := requireDiagnostic(t, err)
		if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != complexContentTestLoc(t, root, `inheritable="false"`) || d.SpecRef() != "xsd11-structures#cSchemaDocument" || len(d.Related()) != 0 || !errors.Is(err, ErrUnsupported) || !errors.Is(err, errLanguagePolicyMismatch) {
			t.Fatalf("Strict10 = %v related=%v", err, d.Related())
		}
	}
}

func sequenceLanguageAnyURIRoot(outer, members, uses, base string) string {
	return sequenceAttributeExtensionSchema("1.1", outer, members, uses, base)
}

func sequenceLanguageAnyURIMembers() string {
	return `<xs:element name="local" type="xs:integer" minOccurs="2" maxOccurs="3"/><xs:element ref="t:target" minOccurs="0" maxOccurs="4"/>`
}

//nolint:gocognit,funlen // The same public fixture checks both added uses, copied facts, and immutable order.
func TestSequenceExtensionLanguageAnyURIPublicFacts(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, names := range [][]string{{"language"}, {"anyURI"}, {"language", "anyURI"}} {
			t.Run(string(policy)+"/"+strings.Join(names, "+"), func(t *testing.T) {
				uses := []string{`<xs:attribute name="before" type="xs:boolean"/>`}
				for _, name := range names {
					uses = append(uses, sequenceLanguageAnyURIUse(name))
				}
				uses = append(uses, `<xs:attribute name="gone" type="xs:anyURI" use="prohibited"/>`, `<xs:attribute name="after" type="xs:integer"/>`)
				base := `<xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent>`
				root := sequenceLanguageAnyURIRoot(` minOccurs="2" maxOccurs="5"`, sequenceLanguageAnyURIMembers(), strings.Join(uses, ""), base)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err != nil {
					t.Fatal(err)
				}
				if schema.LanguagePolicy() != policy || len(schema.Documents()) != 1 || schema.Documents()[0].Source() != "root.xsd" {
					t.Fatalf("policy/documents = %s/%v", schema.LanguagePolicy(), schema.Documents())
				}
				components := schema.Components()
				if len(components) != 5 || components[0].Name().Local() != "root" || components[1].Name().Local() != "Derived" || components[2].Name().Local() != "target" || components[3].Name().Local() != "Base" {
					t.Fatalf("component order = %v", components)
				}
				owner := requireTestComplexTypeDefinition(t, components[1], "Derived")
				if owner.Loc() != complexContentTestLoc(t, root, `<xs:complexType name="Derived"`) || owner.Derivation() != ComplexTypeDerivationExtension || owner.DerivationLoc() != complexContentTestLoc(t, root, `<xs:extension`) || owner.Base() != mustTestQName(t, "urn:root", "Base") || owner.BaseLoc() != complexContentTestLoc(t, root, `base="t:Base"`) {
					t.Fatalf("owner/base = %v", owner)
				}
				baseRef, hasBase := owner.BaseReference()
				baseID, hasBaseID := baseRef.ComponentID()
				if !hasBase || !hasBaseID || baseID != components[3].ID() {
					t.Fatalf("base reference = %v", baseRef)
				}
				wildcard, hasWildcard := owner.AnyAttribute()
				if !hasWildcard || wildcard.Loc() != complexContentTestLoc(t, root, `<xs:anyAttribute`) || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" {
					t.Fatalf("inherited wildcard = %v", wildcard)
				}
				sequence, ok := owner.Particle().(SequenceParticle)
				if !ok || sequence.Loc() != complexContentTestLoc(t, root, `<xs:sequence minOccurs=`) || sequence.Occurrences().String() != "2/5" {
					t.Fatalf("sequence = %v", owner.Particle())
				}
				terms := sequence.Particles()
				if len(terms) != 2 || terms[0].Occurrences().String() != "2/3" || terms[1].Occurrences().String() != "0/4" {
					t.Fatalf("terms = %v", terms)
				}
				ref, ok := terms[1].(ElementReferenceParticle)
				if !ok || ref.TargetID() != components[2].ID() || ref.RefLoc() != complexContentTestLoc(t, root, `ref="t:target"`) || ref.Ref() != mustTestQName(t, "urn:root", "target") {
					t.Fatalf("term reference = %v", terms[1])
				}
				gotUses := owner.AttributeUses()
				if len(gotUses) != len(names)+2 || gotUses[0].Name().Local() != "before" || gotUses[len(gotUses)-1].Name().Local() != "after" {
					t.Fatalf("ordered uses = %v", gotUses)
				}
				for i, name := range names {
					use, ok := gotUses[i+1].(LocalAttributeUse)
					typeLoc := complexContentTestLoc(t, root, `type="xs:`+name+`"`)
					if !ok || use.Name() != mustTestQName(t, "", name) || use.Loc() != complexContentTestLoc(t, root, `<xs:attribute name="`+name+`"`) || use.NameLoc() != complexContentTestLoc(t, root, `name="`+name+`"`) || use.Use() != AttributeUseRequired || use.TypeLoc() != typeLoc || use.DeclaredType() != mustTestQName(t, testXSDNamespace, name) {
						t.Fatalf("%s use = %v", name, gotUses[i+1])
					}
					if use.UseLoc() != allParticleTestTokenLoc(t, "root.xsd", root, `use="required"`, i+1) || use.FormLoc() != allParticleTestTokenLoc(t, "root.xsd", root, `form="unqualified"`, i+1) {
						t.Fatalf("%s use/form Loc = %v/%v", name, use.UseLoc(), use.FormLoc())
					}
					id, hasID := use.TypeID()
					reference, hasReference := use.TypeReference()
					refID, hasRefID := reference.ComponentID()
					if hasID || !id.IsZero() || !hasReference || !reference.IsBuiltin() || reference.QName() != use.DeclaredType() || reference.Loc() != typeLoc || reference.Variety() != SimpleTypeVarietyAtomicRestriction || hasRefID || !refID.IsZero() {
						t.Fatalf("%s type = %v/%v", name, use, reference)
					}
					if _, hasWhiteSpace := reference.StringWhiteSpaceFacet(); hasWhiteSpace || reference.StringEnumerationFacets().HasEnumeration() {
						t.Fatalf("%s acquired user facets", name)
					}
				}
				gotUses[0] = nil
				terms[0] = nil
				components[0] = Component{}
				if owner.AttributeUses()[0] == nil || sequence.Particles()[0] == nil || schema.Components()[0].Name().Local() != "root" {
					t.Fatal("public copy changed schema")
				}
				var first, second []ComponentID
				for _, walk := range []*[]ComponentID{&first, &second} {
					if walkErr := schema.Walk(func(component Component) error { *walk = append(*walk, component.ID()); return nil }); walkErr != nil {
						t.Fatal(walkErr)
					}
				}
				if !reflect.DeepEqual(first, second) || len(first) != 5 {
					t.Fatalf("walks = %v/%v", first, second)
				}
				control := root
				for _, name := range names {
					control = strings.Replace(control, sequenceLanguageAnyURIUse(name), "", 1)
				}
				without, err := discoverTestSchemaWithPolicy(t, control, nil, policy)
				if err != nil || len(requireTestComplexTypeDefinition(t, without.Components()[1], "Derived").AttributeUses()) != 2 || !reflect.DeepEqual(componentIDs(without.Components()), componentIDs(schema.Components())) {
					t.Fatalf("control = %v/%v", without, err)
				}
				again, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
					t.Fatalf("repeated parse = %v", err)
				}
			})
		}
	}
}

func TestSequenceExtensionLanguageAnyURIConsumers(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, name := range []string{"language", "anyURI"} {
			root := sequenceLanguageAnyURIRoot("", sequenceLanguageAnyURIMembers(), sequenceLanguageAnyURIUse(name), "")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatal(err)
			}
			useLoc := complexContentTestLoc(t, root, `<xs:attribute name="`+name+`"`)
			validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
			vd := requireDiagnostic(t, validationErr)
			wantRelated := []Loc{
				complexContentTestLoc(t, root, `<xs:element name="root"`),
				complexContentTestLoc(t, root, `<xs:complexType name="Derived"`),
				complexContentTestLoc(t, root, `<xs:sequence`),
				complexContentTestLoc(t, root, `<xs:complexContent`),
				complexContentTestLoc(t, root, `<xs:extension`),
				complexContentTestLoc(t, root, `base="t:Base"`),
			}
			if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != mustTestLoc(t, "instance.xml", 1, 1) || vd.SpecRef() != instanceValidationSpecRef(languageUseVersion(policy)) || !reflect.DeepEqual(vd.Related(), wantRelated) || !errors.Is(validationErr, ErrUnsupported) || !errors.Is(validationErr, errInstanceComplexContentExtension) {
				t.Fatalf("validation = %v related=%v", validationErr, vd.Related())
			}
			output, generationErr := GenerateGo(schema, "generated")
			gd := requireDiagnostic(t, generationErr)
			if output != nil || gd.Class() != FailureUnsupported || gd.Code() != diagnosticCodegenUnsupported || gd.Loc() != useLoc || gd.SpecRef() != codegenDirectSequenceSpecReference(languageUseVersion(policy), codegenDirectSequenceParticlesReference) || !reflect.DeepEqual(gd.Related(), []Loc{complexContentTestLoc(t, root, `<xs:complexType name="Derived"`), useLoc}) || !errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenUnsupported) {
				t.Fatalf("generation = %v/%v related=%v", output, generationErr, gd.Related())
			}
		}
	}
}

//nolint:gocognit // Each excluded type, use, and value form is checked at this owner boundary.
func TestSequenceExtensionLanguageAnyURIExcludedUses(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, name := range []string{"language", "anyURI"} {
			excluded := append([]directBuiltinExcludedUse{
				{"inline list", `<xs:attribute name="a"><xs:simpleType><xs:list itemType="xs:` + name + `"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, `<xs:list`, errSchemaAttributeUseUnsupported, true},
				{"inline union", `<xs:attribute name="a"><xs:simpleType><xs:union memberTypes="xs:` + name + `"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, `<xs:union`, errSchemaAttributeUseUnsupported, true},
			}, directBuiltinExcludedUses(name)...)
			for _, test := range excluded {
				t.Run(string(policy)+"/"+name+"/"+test.name, func(t *testing.T) {
					root := sequenceLanguageAnyURIRoot("", sequenceLanguageAnyURIMembers(), strings.ReplaceAll(test.attribute, "r:", "t:"), "")
					root = strings.Replace(root, `</xs:schema>`, strings.ReplaceAll(test.extra, "r:", "t:")+`</xs:schema>`, 1)
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
						related = []Loc{complexContentTestLoc(t, root, strings.ReplaceAll(test.related, "r:", "t:"))}
					}
					if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != complexContentTestLoc(t, root, strings.ReplaceAll(test.marker, "r:", "t:")) || d.SpecRef() != wantSpec || !reflect.DeepEqual(d.Related(), related) || !errors.Is(err, ErrUnsupported) || !errors.Is(err, test.cause) {
						t.Fatalf("%s = %v related=%v", test.name, err, d.Related())
					}
				})
			}
		}
	}
}

//nolint:gocognit // The direct-sequence owner is deliberately narrower than sibling owners.
func TestSequenceExtensionLanguageAnyURIExcludedOwnersAndBases(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, name := range []string{"language", "anyURI"} {
			for _, test := range []struct{ name, outer, members, base, marker string }{
				{"attribute-only extension", "", "", "", `<xs:attribute name="`},
				{"choice extension", "", `<xs:choice><xs:element name="v" type="xs:integer"/></xs:choice>`, "", `<xs:attribute name="`},
				{"nonempty base", "", sequenceLanguageAnyURIMembers(), `<xs:sequence><xs:element name="baseValue" type="xs:integer"/></xs:sequence>`, `base="t:Base"`},
				{"used base", "", sequenceLanguageAnyURIMembers(), `<xs:attribute name="inherited" type="xs:boolean"/>`, `base="t:Base"`},
				{"grouped base", "", sequenceLanguageAnyURIMembers(), `<xs:group ref="t:Fields"/>`, `base="t:Base"`},
				{"simple-content base", "", sequenceLanguageAnyURIMembers(), `<xs:simpleContent><xs:extension base="xs:string"/></xs:simpleContent>`, `base="t:Base"`},
			} {
				t.Run(string(policy)+"/"+name+"/"+test.name, func(t *testing.T) {
					root := sequenceLanguageAnyURIRoot(test.outer, test.members, sequenceLanguageAnyURIUse(name), test.base)
					if test.name == "attribute-only extension" {
						root = strings.Replace(root, `<xs:sequence></xs:sequence>`, "", 1)
					}
					if test.name == "choice extension" {
						root = strings.Replace(root, `<xs:sequence>`+test.members+`</xs:sequence>`, test.members, 1)
					}
					if test.name == "grouped base" {
						root = strings.Replace(root, `</xs:schema>`, `<xs:group name="Fields"><xs:sequence/></xs:group></xs:schema>`, 1)
					}
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || !errors.Is(err, ErrUnsupported) || d.Loc() != complexContentTestLoc(t, root, test.marker) {
						t.Fatalf("%s = %v related=%v", test.name, err, d.Related())
					}
				})
			}
		}
	}
}

func TestSequenceExtensionLanguageAnyURIAnonymousLocalOwnerExcluded(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, name := range []string{"language", "anyURI"} {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root"><xs:complexType name="Base"/><xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:Base"><xs:sequence><xs:element name="local" type="xs:integer"/></xs:sequence><xs:attribute name="value" type="xs:` + name + `"/></xs:extension></xs:complexContent></xs:complexType></xs:element></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != complexContentTestLoc(t, root, `type="xs:`+name+`"`) || d.SpecRef() != schemaAttributeTypeSpecRef(languageUseVersion(policy)) || len(d.Related()) != 0 || !errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaAttributeUseUnsupported) {
				t.Fatalf("anonymous owner = %v related=%v", err, d.Related())
			}
		}
	}
}
