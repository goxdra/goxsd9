package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func sequenceExtensionTokenRoot(version, uses, base string) string {
	return sequenceAttributeExtensionSchema(version, ` minOccurs="2" maxOccurs="5"`, sequenceLanguageAnyURIMembers(), uses, base)
}

func sequenceExtensionTokenUse(kind string) string {
	return `<xs:attribute name="` + kind + `" type="xs:` + kind + `" use="required" form="unqualified"/>`
}

//nolint:gocognit // One public fixture checks admission, copied facts, bounds, provenance, and immutable order.
func TestSequenceExtensionBuiltinTokenFamilyPublicFacts(t *testing.T) {
	base := `<xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent>`
	uses := `<xs:attribute name="before" type="xs:boolean"/>` + sequenceExtensionTokenUse("token") +
		`<xs:attribute name="gone" type="xs:decimal" use="prohibited"/>` + sequenceExtensionTokenUse("NMTOKEN") +
		`<xs:attribute name="after" type="xs:integer"/>`
	for _, profile := range tokenPolicyProfiles() {
		for _, edition := range []string{"1.0", "1.1"} {
			t.Run(profile.name+"/"+edition, func(t *testing.T) {
				root := sequenceExtensionTokenRoot(edition, uses, base)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				if schema.LanguagePolicy() != profile.policy || len(schema.Documents()) != 1 || schema.Documents()[0].Source() != "root.xsd" {
					t.Fatalf("policy/documents = %s/%v", schema.LanguagePolicy(), schema.Documents())
				}
				owner := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Derived"))[0], "Derived")
				if owner.Loc() != complexContentTestLoc(t, root, `<xs:complexType name="Derived"`) || owner.Derivation() != ComplexTypeDerivationExtension || owner.DerivationLoc() != complexContentTestLoc(t, root, `<xs:extension`) || owner.Base() != mustTestQName(t, "urn:root", "Base") || owner.BaseLoc() != complexContentTestLoc(t, root, `base="t:Base"`) {
					t.Fatalf("owner = %#v", owner)
				}
				baseRef, hasBase := owner.BaseReference()
				baseID, hasBaseID := baseRef.ComponentID()
				baseComponents := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Base"))
				if !hasBase || !hasBaseID || len(baseComponents) != 1 || baseID != baseComponents[0].ID() || baseRef.Loc() != owner.BaseLoc() {
					t.Fatalf("base reference = %#v", baseRef)
				}
				wildcard, hasWildcard := owner.AnyAttribute()
				if !hasWildcard || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" || wildcard.Loc() != complexContentTestLoc(t, root, `<xs:anyAttribute`) || wildcard.NamespaceLoc() != complexContentTestLoc(t, root, `namespace="##other"`) || wildcard.ProcessContentsLoc() != complexContentTestLoc(t, root, `processContents="lax"`) {
					t.Fatalf("inherited wildcard = %#v", wildcard)
				}
				sequence, ok := owner.Particle().(SequenceParticle)
				if !ok || sequence.Loc() != complexContentTestLoc(t, root, `<xs:sequence`) || sequence.Occurrences().String() != "2/5" {
					t.Fatalf("sequence = %#v", owner.Particle())
				}
				terms := sequence.Particles()
				if len(terms) != 2 || terms[0].Occurrences().String() != "2/3" || terms[1].Occurrences().String() != "0/4" || terms[0].Loc() != complexContentTestLoc(t, root, `<xs:element name="local"`) || terms[1].Loc() != complexContentTestLoc(t, root, `<xs:element ref="t:target"`) {
					t.Fatalf("sequence members = %#v", terms)
				}
				ref, ok := terms[1].(ElementReferenceParticle)
				if !ok || ref.Ref() != mustTestQName(t, "urn:root", "target") || ref.RefLoc() != complexContentTestLoc(t, root, `ref="t:target"`) || ref.TargetID() != schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:root", "target"))[0].ID() {
					t.Fatalf("sequence reference = %#v", terms[1])
				}
				view := owner.AttributeUses()
				if len(view) != 4 || view[0].Name().Local() != "before" || view[3].Name().Local() != "after" {
					t.Fatalf("ordered uses = %#v", view)
				}
				for i, kind := range []string{"token", "NMTOKEN"} {
					use, ok := view[i+1].(LocalAttributeUse)
					qname := mustTestQName(t, testXSDNamespace, kind)
					typeLoc := complexContentTestLoc(t, root, `type="xs:`+kind+`"`)
					if !ok || use.Name() != mustTestQName(t, "", kind) || use.Loc() != complexContentTestLoc(t, root, `<xs:attribute name="`+kind+`"`) || use.NameLoc() != complexContentTestLoc(t, root, `name="`+kind+`"`) || use.FormLoc() != allParticleTestTokenLoc(t, "root.xsd", root, `form="unqualified"`, i+1) || use.TypeLoc() != typeLoc || use.DeclaredType() != qname || use.Use() != AttributeUseRequired || use.UseLoc() != allParticleTestTokenLoc(t, "root.xsd", root, `use="required"`, i+1) {
						t.Fatalf("%s use = %#v", kind, view[i+1])
					}
					atomic, hasType := use.TypeReference()
					id, hasID := use.TypeID()
					refID, hasRefID := atomic.ComponentID()
					space, hasSpace := atomic.StringWhiteSpaceFacet()
					if !hasType || !atomic.IsBuiltin() || atomic.Kind() != SimpleTypeReferenceBuiltin || atomic.QName() != qname || atomic.Name() != qname || atomic.Loc() != typeLoc || atomic.Variety() != SimpleTypeVarietyAtomicRestriction || !hasSpace || space.Value() != "collapse" || !space.Fixed() || !space.Loc().IsZero() || atomic.StringEnumerationFacets().HasEnumeration() || hasID || hasRefID || !id.IsZero() || !refID.IsZero() {
						t.Fatalf("%s copied type = %#v", kind, atomic)
					}
				}
				view[0] = nil
				terms[0] = nil
				components := schema.Components()
				components[0] = Component{}
				if owner.AttributeUses()[0] == nil || sequence.Particles()[0] == nil || schema.Components()[0].Name().IsZero() {
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
				second, repeatErr := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if repeatErr != nil || !reflect.DeepEqual(schema.Components(), second.Components()) {
					t.Fatalf("repeat parse = %v: %v", second.Components(), repeatErr)
				}
			})
		}
	}
}

//nolint:gocognit // Both consumers have independently located unsupported results.
func TestSequenceExtensionBuiltinTokenFamilyConsumers(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			t.Run(profile.name+"/"+kind, func(t *testing.T) {
				root := sequenceExtensionTokenRoot("1.0", sequenceExtensionTokenUse(kind), "")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
				vd := requireDiagnostic(t, validationErr)
				ownerLoc := complexContentTestLoc(t, root, `<xs:complexType name="Derived"`)
				useLoc := complexContentTestLoc(t, root, `<xs:attribute name="`+kind+`"`)
				wantRelated := []Loc{complexContentTestLoc(t, root, `<xs:element name="root"`), ownerLoc, complexContentTestLoc(t, root, `<xs:sequence`), complexContentTestLoc(t, root, `<xs:complexContent`), complexContentTestLoc(t, root, `<xs:extension`), complexContentTestLoc(t, root, `base="t:Base"`)}
				if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != mustTestLoc(t, "instance.xml", 1, 1) || vd.SpecRef() != instanceValidationSpecRef(profile.version) || !reflect.DeepEqual(vd.Related(), wantRelated) || !errors.Is(validationErr, ErrUnsupported) || !errors.Is(validationErr, errInstanceComplexContentExtension) {
					t.Fatalf("validation = %v related=%v", validationErr, vd.Related())
				}
				output, generationErr := GenerateGo(schema, "generated")
				gd := requireDiagnostic(t, generationErr)
				if output != nil || gd.Class() != FailureUnsupported || gd.Code() != diagnosticCodegenUnsupported || gd.Loc() != useLoc || gd.SpecRef() != codegenDirectSequenceSpecReference(profile.version, codegenDirectSequenceParticlesReference) || !reflect.DeepEqual(gd.Related(), []Loc{ownerLoc, useLoc}) || !errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenUnsupported) {
					t.Fatalf("generation = %v/%v related=%v", output, generationErr, gd.Related())
				}
			})
		}
	}
}

//nolint:gocognit // Direct, named, inline, reference, and value forms retain separate rejection sites.
func TestSequenceExtensionTokenFamilyExcludedUseForms(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			alias := `<xs:simpleType name="Alias"><xs:restriction base="xs:` + kind + `"/></xs:simpleType>`
			inline := `<xs:simpleType><xs:restriction base="xs:` + kind + `"/></xs:simpleType>`
			for _, test := range []struct {
				name, use, extra, marker, related string
				spec, cause                       string
			}{
				{"named alias", `<xs:attribute name="a" type="t:Alias"/>`, alias, `type="t:Alias"`, "", "type", "unsupported-use"},
				{"inline atomic", `<xs:attribute name="a">` + inline + `</xs:attribute>`, "", `<xs:simpleType`, "", "type", "unsupported-type"},
				{"inline list", `<xs:attribute name="a"><xs:simpleType><xs:list itemType="xs:` + kind + `"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, "", "type", "unsupported-type"},
				{"inline union", `<xs:attribute name="a"><xs:simpleType><xs:union memberTypes="xs:` + kind + ` xs:` + kind + `"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, "", "type", "unsupported-type"},
				{"ref builtin", `<xs:attribute ref="t:a"/>`, `<xs:attribute name="a" type="xs:` + kind + `"/>`, `ref="t:a"`, `<xs:attribute name="a"`, "use", "unsupported-ref"},
				{"ref named", `<xs:attribute ref="t:a"/>`, `<xs:attribute name="a" type="t:Alias"/>` + alias, `ref="t:a"`, `<xs:attribute name="a"`, "use", "unsupported-ref"},
				{"ref inline", `<xs:attribute ref="t:a"/>`, `<xs:attribute name="a">` + inline + `</xs:attribute>`, `ref="t:a"`, `<xs:attribute name="a"`, "use", "unsupported-ref"},
				{"direct default", `<xs:attribute name="a" type="xs:` + kind + `" default="one"/>`, "", `default="one"`, "", "extension", "unsupported-use"},
				{"direct fixed", `<xs:attribute name="a" type="xs:` + kind + `" fixed="one"/>`, "", `fixed="one"`, "", "extension", "unsupported-use"},
				{"named default", `<xs:attribute name="a" type="t:Alias" default="one"/>`, alias, `default="one"`, "", "extension", "unsupported-use"},
				{"named fixed", `<xs:attribute name="a" type="t:Alias" fixed="one"/>`, alias, `fixed="one"`, "", "extension", "unsupported-use"},
				{"inline default", `<xs:attribute name="a" default="one">` + inline + `</xs:attribute>`, "", `default="one"`, "", "extension", "unsupported-use"},
				{"inline fixed", `<xs:attribute name="a" fixed="one">` + inline + `</xs:attribute>`, "", `fixed="one"`, "", "extension", "unsupported-use"},
				{"ref default", `<xs:attribute ref="t:a" default="one"/>`, `<xs:attribute name="a" type="xs:` + kind + `"/>`, `default="one"`, "", "extension", "unsupported-use"},
				{"ref fixed", `<xs:attribute ref="t:a" fixed="one"/>`, `<xs:attribute name="a" type="xs:` + kind + `"/>`, `fixed="one"`, "", "extension", "unsupported-use"},
			} {
				t.Run(profile.name+"/"+kind+"/"+test.name, func(t *testing.T) {
					root := sequenceExtensionTokenRoot("1.0", test.use, "")
					root = strings.Replace(root, `</xs:schema>`, test.extra+`</xs:schema>`, 1)
					marker, related := test.marker, test.related
					spec := schemaAttributeTypeSpecRef(profile.version)
					cause := errSchemaAttributeUseUnsupported
					switch test.spec {
					case "use":
						spec = schemaAttributeUseSpecRef(profile.version)
					case "extension":
						spec = schemaComplexTypeExtensionSpecRef(profile.version)
					}
					switch test.cause {
					case "unsupported-type":
						cause = errSchemaAttributeTypeUnsupported
					case "unsupported-ref":
						cause = errSchemaAttributeReferenceUnsupported
					}
					if kind == "NMTOKEN" && (test.name == "ref builtin" || test.name == "ref named" || test.name == "ref inline") {
						marker, related, spec, cause = `type="xs:NMTOKEN"`, "", schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnsupported
						if test.name == "ref named" {
							marker = `type="t:Alias"`
						}
						if test.name == "ref inline" {
							marker = `<xs:simpleType`
						}
					}
					if test.name == "inline list" || test.name == "inline union" {
						spec, cause = schemaAttributeUseSpecRef(profile.version), errSchemaAttributeUseUnsupported
						if test.name == "inline list" {
							related = `<xs:list`
						}
						if test.name == "inline union" {
							related = `<xs:union`
						}
					}
					var relatedLocs []Loc
					if related != "" {
						relatedLocs = []Loc{complexContentTestLoc(t, root, related)}
					}
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					loc := complexContentTestLoc(t, root, marker)
					if kind == "NMTOKEN" && test.name == "ref inline" {
						loc = allParticleTestTokenLoc(t, "root.xsd", root, marker, 2)
					}
					assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, loc, relatedLocs, spec, cause)
				})
			}
		}
	}
}

//nolint:gocognit // Semantic exits precede zero-occurrence and prohibited-use publication.
func TestSequenceExtensionBuiltinTokenFamilyOmissionAndAlternateExits(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			root := sequenceAttributeExtensionSchema("1.0", ` minOccurs="0" maxOccurs="0"`, `<xs:element ref="t:target"/>`, sequenceExtensionTokenUse(kind)+`<xs:attribute name="gone" type="xs:decimal" use="prohibited"/>`, "")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			owner := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Derived"))[0], "Derived")
			if owner.Particle() != nil || len(owner.AttributeUses()) != 1 || owner.AttributeUses()[0].Name().Local() != kind {
				t.Fatalf("omitted facts = %v/%v", owner.Particle(), owner.AttributeUses())
			}
			for _, test := range []struct {
				name, from, to, marker, related, code, spec string
				class                                       FailureClass
				cause                                       error
			}{
				{"malformed bound", `maxOccurs="0"`, `maxOccurs="maybe"`, `maxOccurs="maybe"`, "", invalidSchemaCompositionCode, "occurrence", FailureInvalid, nil},
				{"reversed bounds", `minOccurs="0" maxOccurs="0"`, `minOccurs="2" maxOccurs="1"`, `<xs:sequence minOccurs="2"`, "bounds", invalidSchemaCompositionCode, "particle", FailureInvalid, errParticleOccurrenceMinimumExceedsMaximum},
				{"invalid use", `use="required"`, `use="sometimes"`, `use="sometimes"`, "", invalidSchemaCompositionCode, "use", FailureInvalid, nil},
				{"unresolved child before omitted particle", `ref="t:target"`, `ref="t:Missing"`, `ref="t:Missing"`, "", diagnosticSchemaElementReferenceUnresolvedCode, "element", FailureInvalid, errSchemaElementReferenceUnresolved},
				{"wrong-kind child", `ref="t:target"`, `ref="t:Base"`, `ref="t:Base"`, `<xs:complexType name="Base"`, diagnosticSchemaElementReferenceWrongKindCode, "element", FailureInvalid, errSchemaElementReferenceWrongKind},
				{"unresolved prohibited type", `type="xs:decimal" use="prohibited"`, `type="t:Missing" use="prohibited"`, `type="t:Missing"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, "attribute", FailureInvalid, errSchemaAttributeTypeUnresolved},
				{"duplicate use", `<xs:attribute name="gone" type="xs:decimal" use="prohibited"/>`, `<xs:attribute name="` + kind + `" type="xs:decimal" form="unqualified"/>`, `<xs:attribute name="` + kind + `" type="xs:decimal"`, `<xs:attribute name="` + kind + `" type="xs:` + kind + `"`, diagnosticSchemaAttributeUseDuplicateCode, "use", FailureInvalid, errSchemaAttributeUseDuplicate},
				{"missing base", `base="t:Base"`, `base="t:Missing"`, `base="t:Missing"`, "", invalidSchemaCompositionCode, "base", FailureInvalid, errSchemaComplexTypeBaseUnresolved},
				{"wrong-kind base", `base="t:Base"`, `base="t:NamedID"`, `base="t:NamedID"`, `<xs:simpleType name="NamedID"`, invalidSchemaCompositionCode, "base", FailureInvalid, errSchemaComplexTypeBaseWrongKind},
				{"base cycle", `base="t:Base"`, `base="t:Derived"`, `base="t:Derived"`, "", invalidSchemaCompositionCode, "base", FailureInvalid, errSchemaComplexTypeBaseCycle},
				{"base final", `<xs:complexType name="Base">`, `<xs:complexType name="Base" final="extension">`, `base="t:Base"`, `final="extension"`, invalidSchemaCompositionCode, "base", FailureInvalid, errSchemaComplexTypeBaseUnsupported},
				{"nonempty base", `<xs:complexType name="Base"></xs:complexType>`, `<xs:complexType name="Base"><xs:sequence><xs:element name="baseValue" type="xs:integer"/></xs:sequence></xs:complexType>`, `base="t:Base"`, "nonempty", UnsupportedSchemaSyntaxCode, "base", FailureUnsupported, errSchemaComplexTypeBaseNonEmpty},
				{"used base", `<xs:complexType name="Base"></xs:complexType>`, `<xs:complexType name="Base"><xs:attribute name="inherited" type="xs:boolean"/></xs:complexType>`, `base="t:Base"`, `<xs:complexType name="Base"`, UnsupportedSchemaSyntaxCode, "base", FailureUnsupported, errSchemaComplexTypeBaseUnsupported},
			} {
				t.Run(profile.name+"/"+kind+"/"+test.name, func(t *testing.T) {
					changed := strings.Replace(root, test.from, test.to, 1)
					if changed == root {
						t.Fatalf("fixture replacement missed %q", test.from)
					}
					var related []Loc
					switch test.related {
					case "bounds":
						related = []Loc{complexContentTestLoc(t, changed, `minOccurs="2"`), complexContentTestLoc(t, changed, `maxOccurs="1"`)}
					case "nonempty":
						related = []Loc{complexContentTestLoc(t, changed, `<xs:complexType name="Base"`), allParticleTestTokenLoc(t, "root.xsd", changed, `<xs:sequence`, 2)}
					case "":
					default:
						related = []Loc{complexContentTestLoc(t, changed, test.related)}
					}
					spec := schemaComplexTypeExtensionSpecRef(profile.version)
					switch test.spec {
					case "occurrence":
						spec = schemaParticleOccurrenceDatatypeSpecRef(profile.version)
					case "particle":
						spec = schemaParticleCorrectSpecRef(profile.version)
					case "element":
						spec = schemaElementReferenceSpecRef(profile.version)
					case "attribute":
						spec = schemaAttributeTypeSpecRef(profile.version)
					case "use":
						spec = schemaAttributeUseSpecRef(profile.version)
					}
					failed, err := discoverTestSchemaWithPolicy(t, changed, nil, profile.policy)
					assertZeroSchema(t, failed)
					d := requireDiagnostic(t, err)
					if d.Class() != test.class || d.Code() != test.code || d.Loc() != complexContentTestLoc(t, changed, test.marker) || d.SpecRef() != spec || !reflect.DeepEqual(d.Related(), related) || (test.cause != nil && !errors.Is(err, test.cause)) {
						t.Fatalf("%s = %v related=%v", test.name, err, d.Related())
					}
				})
			}
		}
	}
}

//nolint:gocognit // Graph edges retain base identity, lexical source order, and use-site provenance.
func TestSequenceExtensionBuiltinTokenFamilyBaseGraphs(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, graph := range []string{"forward", "included", "imported", "chameleon"} {
			for _, kind := range []string{"token", "NMTOKEN"} {
				t.Run(profile.name+"/"+graph+"/"+kind, func(t *testing.T) {
					fixture := emptyComplexContentExtensionGraph(t, "1.0", graph)
					fixture.root = strings.Replace(fixture.root, `<xs:extension base="`+fixture.baseLexical+`"/>`, `<xs:extension base="`+fixture.baseLexical+`"><xs:sequence minOccurs="1" maxOccurs="2"><xs:element name="item" type="xs:integer" minOccurs="0" maxOccurs="3"/></xs:sequence>`+sequenceExtensionTokenUse(kind)+`</xs:extension>`, 1)
					schema, err := discoverTestSchemaWithPolicy(t, fixture.root, fixture.fixtures, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					owner := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Derived"))[0], "Derived")
					base, hasBase := owner.BaseReference()
					baseID, hasID := base.ComponentID()
					baseParts := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, fixture.baseNamespace, "Base"))
					if !hasBase || !hasID || len(baseParts) != 1 || baseID != baseParts[0].ID() || baseID.Source() != fixture.baseSource || owner.BaseLoc() != complexContentTestLoc(t, fixture.root, `base="`+fixture.baseLexical+`"`) {
						t.Fatalf("base graph = %v/%v", owner, base)
					}
					wildcard, hasWildcard := owner.AnyAttribute()
					if !hasWildcard || wildcard.Loc() != complexContentTestSourceLoc(t, fixture.baseSource, fixture.baseContents, `<xs:anyAttribute`) || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" {
						t.Fatalf("wildcard = %v/%t", wildcard, hasWildcard)
					}
					sequence, ok := owner.Particle().(SequenceParticle)
					uses := owner.AttributeUses()
					if !ok || sequence.Occurrences().String() != "1/2" || len(sequence.Particles()) != 1 || sequence.Particles()[0].Occurrences().String() != "0/3" || len(uses) != 1 || uses[0].Loc() != complexContentTestLoc(t, fixture.root, `<xs:attribute name="`+kind+`"`) {
						t.Fatalf("particle/use = %v/%v", owner.Particle(), uses)
					}
					use, ok := uses[0].(LocalAttributeUse)
					if !ok || use.DeclaredType() != mustTestQName(t, testXSDNamespace, kind) || use.TypeLoc() != complexContentTestLoc(t, fixture.root, `type="xs:`+kind+`"`) {
						t.Fatalf("located use = %v", uses[0])
					}
					wantDocuments := []SourceID{"root.xsd"}
					if graph != "forward" {
						wantDocuments = append(wantDocuments, fixture.baseSource)
					}
					var gotDocuments []SourceID
					for _, document := range schema.Documents() {
						gotDocuments = append(gotDocuments, document.Source())
					}
					if !reflect.DeepEqual(gotDocuments, wantDocuments) {
						t.Fatalf("document order = %v", gotDocuments)
					}
				})
			}
		}
	}
}

//nolint:gocognit // The new named gate must not widen other extension and anonymous owners.
func TestSequenceExtensionTokenFamilyExcludedOwnersAndControl(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			for _, shape := range []string{"attribute-only", "choice", "nested choice", "nested sequence"} {
				t.Run(profile.name+"/"+kind+"/"+shape, func(t *testing.T) {
					root := sequenceExtensionTokenRoot("1.0", sequenceExtensionTokenUse(kind), "")
					marker := `<xs:attribute name="` + kind + `"`
					spec := schemaComplexTypeExtensionSpecRef(profile.version)
					if shape == "attribute-only" {
						root = strings.Replace(root, `<xs:sequence minOccurs="2" maxOccurs="5">`+sequenceLanguageAnyURIMembers()+`</xs:sequence>`, "", 1)
						spec = schemaComplexContentExtensionSpecRef(profile.version)
					}
					if shape == "choice" {
						root = strings.Replace(root, `<xs:sequence minOccurs="2" maxOccurs="5">`, `<xs:choice minOccurs="2" maxOccurs="5">`, 1)
						root = strings.Replace(root, `</xs:sequence>`, `</xs:choice>`, 1)
					}
					if shape == "nested choice" {
						root = strings.Replace(root, sequenceLanguageAnyURIMembers(), `<xs:choice><xs:element ref="t:target"/></xs:choice>`, 1)
					}
					if shape == "nested sequence" {
						root = strings.Replace(root, sequenceLanguageAnyURIMembers(), `<xs:sequence><xs:element ref="t:target"/></xs:sequence>`, 1)
					}
					if shape == "nested choice" || shape == "nested sequence" {
						spec = schemaComplexParticleExtensionSpecRef(profile.version)
						marker = `<xs:choice`
						if shape == "nested sequence" {
							marker = `<xs:sequence><xs:element`
						}
					}
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, complexContentTestLoc(t, root, marker), nil, spec, ErrUnsupported)
				})
			}
		}
		refOnly := inlineTokenURIExtensionSchema("1.0", `<xs:element ref="t:child"/>`, `<xs:attribute name="value" type="xs:token"/>`)
		schema, err := discoverTestSchemaWithPolicy(t, refOnly, nil, profile.policy)
		if err != nil {
			t.Fatalf("existing anonymous ref-only token control: %v", err)
		}
		owner, use := inlineTokenURIUse(t, schema)
		if !owner.ID().IsZero() || owner.Particle() == nil || use.DeclaredType() != mustTestQName(t, testXSDNamespace, "token") || use.TypeLoc() != complexContentTestLoc(t, refOnly, `type="xs:token"`) {
			t.Fatalf("anonymous ref-only control = %v/%v", owner, use)
		}
		for _, test := range []struct{ name, member, use, marker, related string }{
			{"local child", `<xs:element name="local" type="xs:integer"/>`, `<xs:attribute name="value" type="xs:token"/>`, `type="xs:token"`, ""},
			{"NMTOKEN ref-only", `<xs:element ref="t:child"/>`, `<xs:attribute name="value" type="xs:NMTOKEN"/>`, `type="xs:NMTOKEN"`, ""},
			{"named type", `<xs:element ref="t:child"/>`, `<xs:attribute name="value" type="t:NamedToken"/>`, `type="t:NamedToken"`, ""},
			{"inline type", `<xs:element ref="t:child"/>`, `<xs:attribute name="value"><xs:simpleType><xs:restriction base="xs:token"/></xs:simpleType></xs:attribute>`, `<xs:simpleType>`, ""},
			{"attribute ref", `<xs:element ref="t:child"/>`, `<xs:attribute ref="t:global"/>`, `ref="t:global"`, `<xs:attribute name="global"`},
		} {
			t.Run(profile.name+"/anonymous/"+test.name, func(t *testing.T) {
				root := inlineTokenURIExtensionSchema("1.0", test.member, test.use)
				if test.name == "attribute ref" {
					root = strings.Replace(root, `<xs:attribute name="global" type="xs:boolean"/>`, `<xs:attribute name="global" type="xs:token"/>`, 1)
				}
				failed, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				spec := schemaAttributeTypeSpecRef(profile.version)
				cause := errSchemaAttributeTypeUnsupported
				var related []Loc
				if test.name == "attribute ref" {
					spec = schemaAttributeUseSpecRef(profile.version)
					cause = errSchemaAttributeReferenceUnsupported
					related = []Loc{complexContentTestLoc(t, root, test.related)}
				}
				assertNCNameDiagnostic(t, failed, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, complexContentTestLoc(t, root, test.marker), related, spec, cause)
			})
		}
	}
}

func TestSequenceExtensionBuiltinTokenFamilyVisibilityAndPrecedence(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			root := sequenceExtensionTokenRoot("1.0", sequenceExtensionTokenUse(kind), "")
			invisible := strings.Replace(root, `<xs:complexType name="Base"></xs:complexType>`, "", 1)
			invisible = strings.Replace(invisible, `xmlns:t="urn:root"`, `xmlns:t="urn:root" xmlns:f="urn:foreign"`, 1)
			invisible = strings.Replace(invisible, `base="t:Base"`, `base="f:Base"`, 1)
			invisible = strings.Replace(invisible, `attributeFormDefault="qualified">`, `attributeFormDefault="qualified"><xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/>`, 1)
			bridge := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge"><xs:import namespace="urn:foreign" schemaLocation="base.xsd"/></xs:schema>`
			base := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:foreign"><xs:complexType name="Base"/></xs:schema>`
			fixtures := map[string]discoveryFixture{"bridge.xsd": {id: "bridge.xsd", contents: bridge}, "base.xsd": {id: "base.xsd", contents: base}}
			failed, err := discoverTestSchemaWithPolicy(t, invisible, fixtures, profile.policy)
			assertNCNameDiagnostic(t, failed, err, FailureInvalid, invalidSchemaCompositionCode, complexContentTestLoc(t, invisible, `base="f:Base"`), []Loc{complexContentTestSourceLoc(t, "base.xsd", base, `<xs:complexType name="Base"`)}, schemaComplexTypeExtensionSpecRef(profile.version), errSchemaComplexTypeBaseUnresolved)

			combined := strings.Replace(root, `ref="t:target"`, `ref="t:MissingChild"`, 1)
			combined = strings.Replace(combined, `type="xs:`+kind+`"`, `type="t:MissingType"`, 1)
			combined = strings.Replace(combined, `base="t:Base"`, `base="t:MissingBase"`, 1)
			failed, err = discoverTestSchemaWithPolicy(t, combined, nil, profile.policy)
			assertNCNameDiagnostic(t, failed, err, FailureInvalid, diagnosticSchemaElementReferenceUnresolvedCode, complexContentTestLoc(t, combined, `ref="t:MissingChild"`), nil, schemaElementReferenceSpecRef(profile.version), errSchemaElementReferenceUnresolved)
			withoutParticleError := strings.Replace(combined, `ref="t:MissingChild"`, `ref="t:target"`, 1)
			failed, err = discoverTestSchemaWithPolicy(t, withoutParticleError, nil, profile.policy)
			assertNCNameDiagnostic(t, failed, err, FailureInvalid, diagnosticSchemaAttributeTypeUnresolvedCode, complexContentTestLoc(t, withoutParticleError, `type="t:MissingType"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved)
			withoutUseError := strings.Replace(withoutParticleError, `type="t:MissingType"`, `type="xs:`+kind+`"`, 1)
			failed, err = discoverTestSchemaWithPolicy(t, withoutUseError, nil, profile.policy)
			assertNCNameDiagnostic(t, failed, err, FailureInvalid, invalidSchemaCompositionCode, complexContentTestLoc(t, withoutUseError, `base="t:MissingBase"`), nil, schemaComplexTypeExtensionSpecRef(profile.version), errSchemaComplexTypeBaseUnresolved)
		}
	}
}

func TestSequenceExtensionBuiltinTokenFamilyInheritablePolicy(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "NMTOKEN"} {
			root := sequenceExtensionTokenRoot("1.1", `<xs:attribute name="value" type="xs:`+kind+`" inheritable="false"/>`, "")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if profile.policy == Strict10 {
				assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, complexContentTestLoc(t, root, `inheritable="false"`), nil, "xsd11-structures#cSchemaDocument", errLanguagePolicyMismatch)
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			owner := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Derived"))[0], "Derived")
			if len(owner.AttributeUses()) != 1 || owner.AttributeUses()[0].Name().Local() != "value" {
				t.Fatalf("false inheritable changed use facts: %v", owner.AttributeUses())
			}
			trueValue := strings.Replace(root, `inheritable="false"`, `inheritable="true"`, 1)
			failed, trueErr := discoverTestSchemaWithPolicy(t, trueValue, nil, profile.policy)
			assertNCNameDiagnostic(t, failed, trueErr, FailureUnsupported, UnsupportedSchemaSyntaxCode, complexContentTestLoc(t, trueValue, `inheritable="true"`), nil, schemaComplexTypeExtensionSpecRef(profile.version), ErrUnsupported)
		}
	}
}
