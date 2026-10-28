package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func boundedSequenceExtensionUse(lexical string) string {
	return `<xs:attribute name="a" type="` + lexical + `"/>`
}

//nolint:gocognit,funlen // The family matrix checks the public extension, type, and copied facet facts.
func TestBoundedSequenceExtensionPublicFacts(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			for _, graph := range []string{"forward", "included", "imported", "chameleon"} {
				t.Run(profile.name+"/"+family.name+"/"+graph, func(t *testing.T) {
					fixture := emptyComplexContentExtensionGraph(t, string(profile.version), graph)
					if graph == "imported" {
						fixture.root = strings.Replace(fixture.root, `xmlns:b="urn:base"`, `xmlns:b="urn:base" xmlns:t="urn:root"`, 1)
					}
					uses := `<xs:attribute name="before" type="xs:boolean"/>` + boundedSequenceExtensionUse("xs:"+family.name) + `<xs:attribute name="named" type="t:Narrow" form="unqualified" use="required"/><xs:attribute name="gone" type="xs:` + family.name + `" use="prohibited"/><xs:attribute name="after" type="xs:decimal"/>`
					members := `<xs:element name="last" type="xs:integer" minOccurs="0" maxOccurs="4"/><xs:element name="first" type="xs:integer"/>`
					body := `<xs:extension base="` + fixture.baseLexical + `"><xs:sequence minOccurs="2" maxOccurs="3">` + members + `</xs:sequence>` + uses + `</xs:extension>`
					fixture.root = strings.Replace(fixture.root, `<xs:extension base="`+fixture.baseLexical+`"/>`, body, 1)
					fixture.root = strings.Replace(fixture.root, `</xs:schema>`, `<xs:simpleType name="Narrow"><xs:restriction base="t:Wide"><xs:minInclusive value="`+family.childMinimum+`"/><xs:maxInclusive value="`+family.childMaximum+`"/><xs:enumeration value="2"/></xs:restriction></xs:simpleType><xs:simpleType name="Wide"><xs:restriction base="xs:`+family.name+`"><xs:minInclusive value="`+family.parentMinimum+`"/><xs:maxInclusive value="`+family.parentMaximum+`"/><xs:totalDigits value="3"/></xs:restriction></xs:simpleType></xs:schema>`, 1)
					schema, err := discoverTestSchemaWithPolicy(t, fixture.root, fixture.fixtures, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					again, err := discoverTestSchemaWithPolicy(t, fixture.root, fixture.fixtures, profile.policy)
					if err != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
						t.Fatalf("repeat parse = %v", err)
					}
					if schema.LanguagePolicy() != profile.policy || len(schema.Documents()) == 0 || schema.Documents()[0].Source() != "root.xsd" {
						t.Fatalf("policy/documents = %v/%v", schema.LanguagePolicy(), schema.Documents())
					}
					owner := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Derived"))[0], "Derived")
					base, hasBase := owner.BaseReference()
					baseID, hasBaseID := base.ComponentID()
					if owner.Loc() != complexContentTestLoc(t, fixture.root, `<xs:complexType name="Derived"`) || owner.Derivation() != ComplexTypeDerivationExtension || owner.DerivationLoc() != complexContentTestLoc(t, fixture.root, `<xs:extension`) || owner.BaseLoc() != complexContentTestLoc(t, fixture.root, `base="`+fixture.baseLexical+`"`) || !hasBase || !hasBaseID || baseID.Source() != fixture.baseSource || base.Name() != mustTestQName(t, fixture.baseNamespace, "Base") || base.Loc() != owner.BaseLoc() {
						t.Fatalf("owner/base = %#v/%#v", owner, base)
					}
					wildcard, hasWildcard := owner.AnyAttribute()
					if !hasWildcard || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" || wildcard.Loc() != complexContentTestSourceLoc(t, fixture.baseSource, fixture.baseContents, `<xs:anyAttribute`) {
						t.Fatalf("inherited wildcard = %#v", wildcard)
					}
					sequence, ok := owner.Particle().(SequenceParticle)
					if !ok || sequence.Occurrences().String() != "2/3" || sequence.Loc() != complexContentTestLoc(t, fixture.root, `<xs:sequence`) {
						t.Fatalf("sequence = %#v", owner.Particle())
					}
					terms := sequence.Particles()
					if len(terms) != 2 || terms[0].Occurrences().String() != "0/4" || terms[1].Occurrences().String() != "1/1" || terms[0].Loc() != complexContentTestLoc(t, fixture.root, `<xs:element name="last"`) || terms[1].Loc() != complexContentTestLoc(t, fixture.root, `<xs:element name="first"`) {
						t.Fatalf("terms = %v", terms)
					}
					usesOut := owner.AttributeUses()
					if len(usesOut) != 4 || usesOut[0].Name().Local() != "before" || usesOut[1].Name().Local() != "a" || usesOut[2].Name().Local() != "named" || usesOut[3].Name().Local() != "after" {
						t.Fatalf("lexical uses = %v", usesOut)
					}
					for i, expected := range []struct{ name, lexical, minimum, maximum string }{{"a", "xs:" + family.name, family.minimum, family.maximum}, {"named", "t:Narrow", family.childMinimum, family.childMaximum}} {
						use, hasUse := usesOut[i+1].(LocalAttributeUse)
						if !hasUse || use.Loc() != complexContentTestLoc(t, fixture.root, `<xs:attribute name="`+expected.name+`"`) || use.NameLoc() != complexContentTestLoc(t, fixture.root, `name="`+expected.name+`"`) || use.TypeLoc() != unsignedLongParticleTokenLocAfter(t, fixture.root, `<xs:attribute name="`+expected.name+`"`, `type="`+expected.lexical+`"`) {
							t.Fatalf("use %d = %#v", i, usesOut[i+1])
						}
						if use.Name() != mustTestQName(t, "", expected.name) {
							t.Fatalf("effective name = %s", use.Name())
						}
						ref, hasRef := use.TypeReference()
						if !hasRef || ref.Loc() != use.TypeLoc() || ref.QName() != use.DeclaredType() || ref.Name() != use.DeclaredType() || ref.Variety() != SimpleTypeVarietyAtomicRestriction {
							t.Fatalf("type ref %d = %#v", i, ref)
						}
						if i == 0 {
							id, hasID := use.TypeID()
							refID, hasRefID := ref.ComponentID()
							if !ref.IsBuiltin() || hasID || hasRefID || !id.IsZero() || !refID.IsZero() || ref.VarietyLoc() != use.TypeLoc() || use.DeclaredType() != mustTestQName(t, testXSDNamespace, family.name) {
								t.Fatalf("builtin = %#v", use)
							}
						}
						if i == 1 {
							id, hasID := use.TypeID()
							refID, hasRefID := ref.ComponentID()
							if !ref.IsNamed() || !hasID || !hasRefID || id != refID || id != componentIDForName(t, schema, use.DeclaredType()) || ref.VarietyLoc() != complexContentTestLoc(t, fixture.root, `<xs:restriction base="t:Wide"`) || use.FormLoc() != complexContentTestLoc(t, fixture.root, `form="unqualified"`) || use.Use() != AttributeUseRequired || use.UseLoc() != complexContentTestLoc(t, fixture.root, `use="required"`) {
								t.Fatalf("named = %#v", use)
							}
						}
						minLoc, maxLoc := Loc{}, Loc{}
						if i == 1 {
							minLoc = complexContentTestLoc(t, fixture.root, `value="`+family.childMinimum+`"`)
							maxLoc = complexContentTestLoc(t, fixture.root, `value="`+family.childMaximum+`"`)
						}
						assertAllIntegerBounds(t, ref, profile.version, expected.minimum, expected.maximum, minLoc, maxLoc)
					}
					definition := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:root", "Narrow"))[0]
					narrow, ok := definition.SimpleTypeDefinition()
					if !ok {
						t.Fatal("Narrow definition missing")
					}
					values, locations := narrow.IntegerEnumerationFacets().Values(), narrow.IntegerEnumerationFacets().Locations()
					digits, hasDigits := narrow.DigitFacets().TotalDigits()
					digitsLoc, hasDigitsLoc := narrow.DigitFacets().TotalDigitsLoc()
					if len(values) != 1 || values[0].Canonical() != "2" || !reflect.DeepEqual(locations, []Loc{complexContentTestLoc(t, fixture.root, `<xs:enumeration value="2"`)}) || !hasDigits || digits.Canonical() != "3" || !hasDigitsLoc || digitsLoc != complexContentTestLoc(t, fixture.root, `value="3"`) {
						t.Fatalf("facet provenance = %v/%v/%v", values, locations, digits)
					}
					values[0].value.SetInt64(99)
					locations[0] = Loc{}
					digits.value.SetInt64(0)
					freshDigits, hasFreshDigits := narrow.DigitFacets().TotalDigits()
					if narrow.IntegerEnumerationFacets().Values()[0].Canonical() != "2" || !hasFreshDigits || freshDigits.Canonical() != "3" {
						t.Fatal("facet copy mutated schema")
					}
					namedUse, hasNamedUse := usesOut[2].(LocalAttributeUse)
					if !hasNamedUse {
						t.Fatal("named use missing")
					}
					ref, hasRef := namedUse.TypeReference()
					if !hasRef {
						t.Fatal("named type missing")
					}
					bounds, hasBounds := ref.IntegerBounds()
					if !hasBounds {
						t.Fatal("named bounds missing")
					}
					for _, bound := range bounds.Bounds() {
						bound.Value().value.SetInt64(0)
					}
					assertAllIntegerBounds(t, ref, profile.version, family.childMinimum, family.childMaximum, complexContentTestLoc(t, fixture.root, `value="`+family.childMinimum+`"`), complexContentTestLoc(t, fixture.root, `value="`+family.childMaximum+`"`))
					usesOut[1] = nil
					terms[0] = nil
					freshSequence, hasSequence := owner.Particle().(SequenceParticle)
					if !hasSequence || owner.AttributeUses()[1] == nil || len(freshSequence.Particles()) != 2 {
						t.Fatal("slice copy mutated schema")
					}
					var walks [2][]ComponentID
					for i := range walks {
						if err := schema.Walk(func(c Component) error { walks[i] = append(walks[i], c.ID()); return nil }); err != nil {
							t.Fatal(err)
						}
					}
					if !reflect.DeepEqual(walks[0], walks[1]) || !reflect.DeepEqual(walks[0], componentIDs(schema.Components())) {
						t.Fatalf("walks = %v/%v", walks[0], walks[1])
					}
				})
			}
		}
	}
}

//nolint:gocognit // Both consumers are checked independently for each new type query.
func TestBoundedSequenceExtensionConsumers(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			for _, shape := range []string{"builtin", "named"} {
				t.Run(profile.name+"/"+family.name+"/"+shape, func(t *testing.T) {
					lexical, extra := "xs:"+family.name, ""
					if shape == "named" {
						lexical = "t:Alias"
						extra = `<xs:simpleType name="Alias"><xs:restriction base="xs:` + family.name + `"/></xs:simpleType>`
					}
					root := sequenceAttributeExtensionSchema(string(profile.version), "", `<xs:element name="v" type="xs:integer"/>`, boundedSequenceExtensionUse(lexical), "")
					root = strings.Replace(root, `</xs:schema>`, extra+`</xs:schema>`, 1)
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root" a="2"/>`)))
					vd := requireDiagnostic(t, validationErr)
					wantRelated := []Loc{complexContentTestLoc(t, root, `<xs:element name="root"`), complexContentTestLoc(t, root, `<xs:complexType name="Derived"`), complexContentTestLoc(t, root, `<xs:sequence`), complexContentTestLoc(t, root, `<xs:complexContent`), complexContentTestLoc(t, root, `<xs:extension`), complexContentTestLoc(t, root, `base="t:Base"`)}
					if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != mustTestLoc(t, "instance.xml", 1, 1) || vd.SpecRef() != instanceValidationSpecRef(profile.version) || !reflect.DeepEqual(vd.Related(), wantRelated) || !errors.Is(validationErr, ErrUnsupported) || !errors.Is(validationErr, errInstanceComplexContentExtension) {
						t.Fatalf("validation = %v related=%v", validationErr, vd.Related())
					}
					output, generationErr := GenerateGo(schema, "generated")
					gd := requireDiagnostic(t, generationErr)
					useLoc := complexContentTestLoc(t, root, `<xs:attribute name="a"`)
					ownerLoc := complexContentTestLoc(t, root, `<xs:complexType name="Derived"`)
					if output != nil || gd.Class() != FailureUnsupported || gd.Code() != diagnosticCodegenUnsupported || gd.Loc() != useLoc || gd.SpecRef() != codegenDirectSequenceSpecReference(profile.version, codegenDirectSequenceParticlesReference) || !reflect.DeepEqual(gd.Related(), []Loc{ownerLoc, useLoc}) || !errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenUnsupported) {
						t.Fatalf("generation = %v/%v related=%v", output, generationErr, gd.Related())
					}
				})
			}
		}
	}
}

//nolint:gocognit // Each local declaration/reference/value shape keeps its own located exclusion.
func TestBoundedSequenceExtensionExcludedUseForms(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			builtin := "xs:" + family.name
			alias := `<xs:simpleType name="Alias"><xs:restriction base="` + builtin + `"/></xs:simpleType>`
			inline := `<xs:simpleType><xs:restriction base="` + builtin + `"/></xs:simpleType>`
			for _, test := range []struct {
				name, use, extra, marker, related string
				spec                              func(XSDVersion) string
				cause                             error
			}{
				{"inline atomic", `<xs:attribute name="a">` + inline + `</xs:attribute>`, "", `<xs:simpleType`, "", schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnsupported},
				{"inline list", `<xs:attribute name="a"><xs:simpleType><xs:list itemType="` + builtin + `"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, `<xs:list`, schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported},
				{"inline union", `<xs:attribute name="a"><xs:simpleType><xs:union memberTypes="` + builtin + ` ` + builtin + `"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, `<xs:union`, schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported},
				{"named list", boundedSequenceExtensionUse("t:List"), `<xs:simpleType name="List"><xs:list itemType="` + builtin + `"/></xs:simpleType>`, `type="t:List"`, `<xs:simpleType name="List"|<xs:list`, schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported},
				{"named union", boundedSequenceExtensionUse("t:Union"), `<xs:simpleType name="Union"><xs:union memberTypes="` + builtin + `"/></xs:simpleType>`, `type="t:Union"`, `<xs:simpleType name="Union"|<xs:union`, schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported},
				{"ref direct", `<xs:attribute ref="t:a"/>`, `<xs:attribute name="a" type="` + builtin + `"/>`, `ref="t:a"`, `<xs:attribute name="a"`, schemaAttributeUseSpecRef, errSchemaAttributeReferenceUnsupported},
				{"ref named", `<xs:attribute ref="t:a"/>`, `<xs:attribute name="a" type="t:Alias"/>` + alias, `ref="t:a"`, `<xs:attribute name="a"`, schemaAttributeUseSpecRef, errSchemaAttributeReferenceUnsupported},
				{"ref inline", `<xs:attribute ref="t:a"/>`, `<xs:attribute name="a">` + inline + `</xs:attribute>`, `ref="t:a"`, `<xs:attribute name="a"`, schemaAttributeUseSpecRef, errSchemaAttributeReferenceUnsupported},
				{"direct default", `<xs:attribute name="a" type="` + builtin + `" default="2"/>`, "", `default="2"`, "", schemaComplexTypeExtensionSpecRef, errSchemaAttributeUseUnsupported},
				{"direct fixed", `<xs:attribute name="a" type="` + builtin + `" fixed="2"/>`, "", `fixed="2"`, "", schemaComplexTypeExtensionSpecRef, errSchemaAttributeUseUnsupported},
				{"named default", `<xs:attribute name="a" type="t:Alias" default="2"/>`, alias, `default="2"`, "", schemaComplexTypeExtensionSpecRef, errSchemaAttributeUseUnsupported},
				{"named fixed", `<xs:attribute name="a" type="t:Alias" fixed="2"/>`, alias, `fixed="2"`, "", schemaComplexTypeExtensionSpecRef, errSchemaAttributeUseUnsupported},
				{"inline default", `<xs:attribute name="a" default="2">` + inline + `</xs:attribute>`, "", `default="2"`, "", schemaComplexTypeExtensionSpecRef, errSchemaAttributeUseUnsupported},
				{"inline fixed", `<xs:attribute name="a" fixed="2">` + inline + `</xs:attribute>`, "", `fixed="2"`, "", schemaComplexTypeExtensionSpecRef, errSchemaAttributeUseUnsupported},
				{"ref default", `<xs:attribute ref="t:a" default="2"/>`, `<xs:attribute name="a" type="` + builtin + `"/>`, `default="2"`, "", schemaComplexTypeExtensionSpecRef, errSchemaAttributeUseUnsupported},
				{"ref fixed", `<xs:attribute ref="t:a" fixed="2"/>`, `<xs:attribute name="a" type="` + builtin + `"/>`, `fixed="2"`, "", schemaComplexTypeExtensionSpecRef, errSchemaAttributeUseUnsupported},
			} {
				t.Run(profile.name+"/"+family.name+"/"+test.name, func(t *testing.T) {
					root := sequenceAttributeExtensionSchema(string(profile.version), "", `<xs:element name="v" type="xs:integer"/>`, test.use, "")
					root = strings.Replace(root, `</xs:schema>`, test.extra+`</xs:schema>`, 1)
					var related []Loc
					if test.related != "" {
						for _, marker := range strings.Split(test.related, "|") {
							related = append(related, complexContentTestLoc(t, root, marker))
						}
					}
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, complexContentTestLoc(t, root, test.marker), related, test.spec(profile.version), test.cause)
				})
			}
		}
	}
}

//nolint:gocognit // Alternate exits at particle, use, and base phases retain distinct diagnostics.
func TestBoundedSequenceExtensionAlternateExits(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			for _, test := range []struct {
				name, member, use, extra, base, marker, related, code string
				class                                                 FailureClass
				spec                                                  func(XSDVersion) string
				cause                                                 error
			}{
				{"particle before use", `<xs:element ref="t:MissingElement"/>`, `<xs:attribute name="a" type="t:MissingType"/>`, "", "t:MissingBase", `ref="t:MissingElement"`, "", diagnosticSchemaElementReferenceUnresolvedCode, FailureInvalid, schemaElementReferenceSpecRef, errSchemaElementReferenceUnresolved},
				{"particle before prohibited", `<xs:element ref="t:MissingElement"/>`, `<xs:attribute name="a" type="t:MissingType" use="prohibited"/>`, "", "t:MissingBase", `ref="t:MissingElement"`, "", diagnosticSchemaElementReferenceUnresolvedCode, FailureInvalid, schemaElementReferenceSpecRef, errSchemaElementReferenceUnresolved},
				{"use before base", `<xs:element name="v" type="xs:integer"/>`, `<xs:attribute name="a" type="t:MissingType"/>`, "", "t:MissingBase", `type="t:MissingType"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, FailureInvalid, schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnresolved},
				{"prohibited before base", `<xs:element name="v" type="xs:integer"/>`, `<xs:attribute name="a" type="t:MissingType" use="prohibited"/>`, "", "t:MissingBase", `type="t:MissingType"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, FailureInvalid, schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnresolved},
				{"wrong kind type", `<xs:element name="v" type="xs:integer"/>`, `<xs:attribute name="a" type="t:Wrong"/>`, `<xs:complexType name="Wrong"/>`, "t:Base", `type="t:Wrong"`, `<xs:complexType name="Wrong"`, diagnosticSchemaAttributeTypeWrongKindCode, FailureInvalid, schemaAttributeTypeSpecRef, errSchemaAttributeTypeWrongKind},
				{"cycle type", `<xs:element name="v" type="xs:integer"/>`, `<xs:attribute name="a" type="t:First"/>`, `<xs:simpleType name="First"><xs:restriction base="t:Second"/></xs:simpleType><xs:simpleType name="Second"><xs:restriction base="t:First"/></xs:simpleType>`, "t:Base", `type="t:First"`, `<xs:attribute name="a"|base="t:Second"|base="t:First"|<xs:simpleType name="First"|<xs:simpleType name="Second"`, diagnosticSchemaAttributeTypeCycleCode, FailureInvalid, schemaAttributeTypeSpecRef, errSchemaSimpleTypeBaseCycle},
				{"duplicate use", `<xs:element name="v" type="xs:integer"/>`, boundedSequenceExtensionUse("xs:"+family.name) + `<xs:attribute name="a" type="xs:` + family.name + `" use="required"/>`, "", "t:MissingBase", `<xs:attribute name="a" type="xs:` + family.name + `" use="required"`, boundedSequenceExtensionUse("xs:" + family.name), diagnosticSchemaAttributeUseDuplicateCode, FailureInvalid, schemaAttributeUseSpecRef, errSchemaAttributeUseDuplicate},
				{"missing base", `<xs:element name="v" type="xs:integer"/>`, boundedSequenceExtensionUse("xs:" + family.name), "", "t:MissingBase", `base="t:MissingBase"`, "", invalidSchemaCompositionCode, FailureInvalid, schemaComplexTypeExtensionSpecRef, errSchemaComplexTypeBaseUnresolved},
				{"wrong base", `<xs:element name="v" type="xs:integer"/>`, boundedSequenceExtensionUse("xs:" + family.name), "", "t:NamedID", `base="t:NamedID"`, `<xs:simpleType name="NamedID"`, invalidSchemaCompositionCode, FailureInvalid, schemaComplexTypeExtensionSpecRef, errSchemaComplexTypeBaseWrongKind},
				{"final base", `<xs:element name="v" type="xs:integer"/>`, boundedSequenceExtensionUse("xs:" + family.name), "", "t:Base", `base="t:Base"`, `final="extension"`, invalidSchemaCompositionCode, FailureInvalid, schemaComplexTypeExtensionSpecRef, errSchemaComplexTypeBaseUnsupported},
				{"cycle base", `<xs:element name="v" type="xs:integer"/>`, boundedSequenceExtensionUse("xs:" + family.name), "", "t:Derived", `base="t:Derived"`, "", invalidSchemaCompositionCode, FailureInvalid, schemaComplexTypeExtensionSpecRef, errSchemaComplexTypeBaseCycle},
				{"used base", `<xs:element name="v" type="xs:integer"/>`, boundedSequenceExtensionUse("xs:" + family.name), "", "t:Base", `base="t:Base"`, `<xs:complexType name="Base"`, UnsupportedSchemaSyntaxCode, FailureUnsupported, schemaComplexTypeExtensionSpecRef, errSchemaComplexTypeBaseUnsupported},
				{"nonempty base", `<xs:element name="v" type="xs:integer"/>`, boundedSequenceExtensionUse("xs:" + family.name), "", "t:Base", `base="t:Base"`, `<xs:complexType name="Base"|<xs:sequence><xs:element name="prior"`, UnsupportedSchemaSyntaxCode, FailureUnsupported, schemaComplexTypeExtensionSpecRef, errSchemaComplexTypeBaseNonEmpty},
			} {
				t.Run(profile.name+"/"+family.name+"/"+test.name, func(t *testing.T) {
					base := ""
					if test.name == "used base" {
						base = `<xs:attribute name="inherited" type="xs:integer"/>`
					}
					if test.name == "nonempty base" {
						base = `<xs:sequence><xs:element name="prior" type="xs:integer"/></xs:sequence>`
					}
					outer := ""
					if test.name == "particle before prohibited" {
						outer = ` minOccurs="0" maxOccurs="0"`
					}
					root := sequenceAttributeExtensionSchema(string(profile.version), outer, test.member, test.use, base)
					if test.name == "final base" {
						root = strings.Replace(root, `<xs:complexType name="Base">`, `<xs:complexType name="Base" final="extension">`, 1)
					}
					root = strings.Replace(root, `base="t:Base"`, `base="`+test.base+`"`, 1)
					root = strings.Replace(root, `</xs:schema>`, test.extra+`</xs:schema>`, 1)
					var related []Loc
					if test.related != "" {
						for _, marker := range strings.Split(test.related, "|") {
							related = append(related, complexContentTestLoc(t, root, marker))
						}
					}
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertNCNameDiagnostic(t, schema, err, test.class, test.code, complexContentTestLoc(t, root, test.marker), related, test.spec(profile.version), test.cause)
				})
			}
		}
	}
}

func TestBoundedSequenceExtensionExcludedOwners(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			for _, shape := range []string{"builtin", "named"} {
				lexical, extra := "xs:"+family.name, ""
				if shape == "named" {
					lexical = "t:Alias"
					extra = `<xs:simpleType name="Alias"><xs:restriction base="xs:` + family.name + `"/></xs:simpleType>`
				}
				use := boundedSequenceExtensionUse(lexical)
				for _, test := range []struct {
					name, owner, marker string
					spec                func(XSDVersion) string
					cause               error
				}{
					{"anonymous", `<xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:Base"><xs:sequence><xs:element name="v" type="xs:integer"/></xs:sequence>` + use + `</xs:extension></xs:complexContent></xs:complexType></xs:element>`, `type="` + lexical + `"`, schemaAttributeTypeSpecRef, errSchemaAttributeUseUnsupported},
					{"choice", `<xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Base"><xs:choice><xs:element name="v" type="xs:integer"/></xs:choice>` + use + `</xs:extension></xs:complexContent></xs:complexType>`, `<xs:attribute name="a"`, schemaComplexTypeExtensionSpecRef, ErrUnsupported},
					{"all", `<xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Base"><xs:all><xs:element name="v" type="xs:integer"/></xs:all>` + use + `</xs:extension></xs:complexContent></xs:complexType>`, `<xs:attribute name="a"`, schemaComplexTypeExtensionSpecRef, ErrUnsupported},
					{"nested sequence", `<xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Base"><xs:sequence><xs:sequence><xs:element name="v" type="xs:integer"/></xs:sequence></xs:sequence>` + use + `</xs:extension></xs:complexContent></xs:complexType>`, `<xs:sequence><xs:element`, schemaComplexParticleExtensionSpecRef, ErrUnsupported},
					{"restriction", `<xs:complexType name="Derived"><xs:complexContent><xs:restriction base="t:Base"><xs:sequence><xs:element name="v" type="xs:integer"/></xs:sequence>` + use + `</xs:restriction></xs:complexContent></xs:complexType>`, `<xs:sequence`, schemaSyntaxSpecRefForVersion, ErrUnsupported},
				} {
					t.Run(profile.name+"/"+family.name+"/"+shape+"/"+test.name, func(t *testing.T) {
						root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `">` + test.owner + `<xs:complexType name="Base"/>` + extra + `</xs:schema>`
						schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
						assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, complexContentTestLoc(t, root, test.marker), nil, test.spec(profile.version), test.cause)
					})
				}
			}
		}
	}
}

//nolint:gocognit // Version labels, omitted terms, and prohibited uses retain distinct phase ordering.
func TestBoundedSequenceExtensionPolicyAndOmission(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			for _, label := range []string{"1.0", "1.1"} {
				t.Run(profile.name+"/"+family.name+"/"+label, func(t *testing.T) {
					root := sequenceAttributeExtensionSchema(label, ` minOccurs="0" maxOccurs="0"`, `<xs:element name="v" type="xs:integer"/>`, boundedSequenceExtensionUse("xs:"+family.name)+`<xs:attribute name="gone" type="xs:`+family.name+`" use="prohibited"/>`, "")
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					owner := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Derived"))[0], "Derived")
					uses := owner.AttributeUses()
					if owner.Particle() != nil || len(uses) != 1 || uses[0].Name().Local() != "a" {
						t.Fatalf("omitted facts = %v/%v", owner.Particle(), uses)
					}
					use, hasUse := uses[0].(LocalAttributeUse)
					if !hasUse {
						t.Fatal("bounded use missing")
					}
					ref, hasRef := use.TypeReference()
					if !hasRef {
						t.Fatal("type reference missing")
					}
					assertAllIntegerBounds(t, ref, profile.version, family.minimum, family.maximum, Loc{}, Loc{})
					bad := strings.Replace(root, `type="xs:`+family.name+`" use="prohibited"`, `type="t:Missing" use="prohibited"`, 1)
					failed, failure := discoverTestSchemaWithPolicy(t, bad, nil, profile.policy)
					assertNCNameDiagnostic(t, failed, failure, FailureInvalid, diagnosticSchemaAttributeTypeUnresolvedCode, complexContentTestLoc(t, bad, `type="t:Missing"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved)
				})
			}
		}
	}
}

func TestBoundedSequenceExtensionTypeVisibility(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			t.Run(profile.name+"/"+family.name, func(t *testing.T) {
				root := boundedLocalRoot(string(profile.version), `<xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/><xs:complexType name="Derived"><xs:complexContent><xs:extension base="r:Base"><xs:sequence><xs:element name="v" type="xs:integer"/></xs:sequence><xs:attribute name="a" type="o:Hidden"/></xs:extension></xs:complexContent></xs:complexType><xs:complexType name="Base"/>`)
				fixtures := map[string]discoveryFixture{"bridge.xsd": {id: "bridge.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge"><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`}, "other.xsd": {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Hidden"><xs:restriction base="xs:` + family.name + `"/></xs:simpleType></xs:schema>`}}
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				assertNCNameDiagnostic(t, schema, err, FailureInvalid, diagnosticSchemaAttributeTypeUnresolvedCode, complexContentTestLoc(t, root, `type="o:Hidden"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved)
				duplicate := sequenceAttributeExtensionSchema(string(profile.version), "", `<xs:element name="v" type="xs:integer"/>`, boundedSequenceExtensionUse("t:Alias"), "")
				duplicate = strings.Replace(duplicate, `</xs:schema>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:`+family.name+`"/></xs:simpleType><xs:simpleType name="Alias" id="second"><xs:restriction base="xs:`+family.name+`"/></xs:simpleType></xs:schema>`, 1)
				failed, failure := discoverTestSchemaWithPolicy(t, duplicate, nil, profile.policy)
				first := complexContentTestLoc(t, duplicate, `<xs:simpleType name="Alias"><xs:restriction`)
				second := complexContentTestLoc(t, duplicate, `<xs:simpleType name="Alias" id="second"`)
				assertNCNameDiagnostic(t, failed, failure, FailureInvalid, diagnosticSchemaGlobalDuplicateCode, second, []Loc{first}, schemaGlobalDuplicateSpecRef(profile.version), errSchemaGlobalDeclarationDuplicate)
				primary := complexContentTestLoc(t, duplicate, `type="t:Alias"`)
				related := []Loc{first, second}
				resolverErr := newSchemaSimpleTypeDiagnostic(diagnosticSchemaSimpleTypeAmbiguousCode, primary, "ambiguous simple type", related, profile.version, errSchemaSimpleTypeBaseAmbiguous)
				reframed := reframeSchemaAttributeReferenceError(&schemaAttributeInput{declaredType: mustTestQName(t, "urn:root", "Alias"), typeLoc: primary}, resolverErr, profile.version)
				diagnostic := requireDiagnostic(t, reframed)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaAttributeTypeAmbiguousCode || diagnostic.Loc() != primary || diagnostic.SpecRef() != schemaAttributeTypeSpecRef(profile.version) || !reflect.DeepEqual(diagnostic.Related(), related) || !errors.Is(reframed, errSchemaAttributeTypeAmbiguous) || !errors.Is(reframed, errSchemaSimpleTypeBaseAmbiguous) {
					t.Fatalf("ambiguous reframe = %v related=%v", reframed, diagnostic.Related())
				}
			})
		}
	}
}

//nolint:gocognit // Invalid facets, syntax, and unsupported facets are separate phase exits.
func TestBoundedSequenceExtensionDatatypeAndOccurrenceExits(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			for _, test := range []struct {
				name, outer, member, use, extra, marker, related, code string
				class                                                  FailureClass
				spec                                                   func(XSDVersion) string
				cause                                                  error
			}{
				{"malformed outer", ` minOccurs="many"`, `<xs:element name="v" type="xs:integer"/>`, boundedSequenceExtensionUse("t:Missing"), "", `minOccurs="many"`, "", invalidSchemaCompositionCode, FailureInvalid, schemaParticleOccurrenceDatatypeSpecRef, nil},
				{"reversed child", "", `<xs:element name="v" type="xs:integer" minOccurs="2" maxOccurs="1"/>`, boundedSequenceExtensionUse("t:Missing"), "", `<xs:element name="v"`, `minOccurs="2"|maxOccurs="1"`, invalidSchemaCompositionCode, FailureInvalid, schemaParticleCorrectSpecRef, errParticleOccurrenceMinimumExceedsMaximum},
				{"invalid bound", "", `<xs:element name="v" type="xs:integer"/>`, boundedSequenceExtensionUse("t:Bad"), `<xs:simpleType name="Bad"><xs:restriction base="xs:` + family.name + `"><xs:maxInclusive value="` + family.maximum + `0"/></xs:restriction></xs:simpleType>`, `value="` + family.maximum + `0"`, "", InvalidBoundRestrictionCode, FailureInvalid, func(v XSDVersion) string { return boundSpecRef(v, BoundMaxInclusive, boundRestrictionRule) }, errInvalidBoundRestriction},
				{"unsupported pattern", "", `<xs:element name="v" type="xs:integer"/>`, boundedSequenceExtensionUse("t:Bad"), `<xs:simpleType name="Bad"><xs:restriction base="xs:` + family.name + `"><xs:pattern value="[0-9]+"/></xs:restriction></xs:simpleType>`, `<xs:pattern`, "", UnsupportedDatatypeFacetCode, FailureUnsupported, func(v XSDVersion) string { return tokenDiagnosticSpecRef(v, "decimal") }, ErrUnsupported},
			} {
				t.Run(profile.name+"/"+family.name+"/"+test.name, func(t *testing.T) {
					root := sequenceAttributeExtensionSchema(string(profile.version), test.outer, test.member, test.use, "")
					root = strings.Replace(root, `</xs:schema>`, test.extra+`</xs:schema>`, 1)
					var related []Loc
					if test.related != "" {
						for _, marker := range strings.Split(test.related, "|") {
							related = append(related, complexContentTestLoc(t, root, marker))
						}
					}
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					if test.cause == nil {
						assertZeroSchema(t, schema)
						d := requireDiagnostic(t, err)
						if d.Class() != test.class || d.Code() != test.code || d.Loc() != complexContentTestLoc(t, root, test.marker) || d.SpecRef() != test.spec(profile.version) || !reflect.DeepEqual(d.Related(), related) {
							t.Fatalf("malformed occurrence = %v related=%v", err, d.Related())
						}
						return
					}
					assertNCNameDiagnostic(t, schema, err, test.class, test.code, complexContentTestLoc(t, root, test.marker), related, test.spec(profile.version), test.cause)
				})
			}
		}
	}
}

func TestBoundedSequenceExtensionGroupedControl(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			builtIn := groupedExtensionSchema(string(profile.version), "", groupedBoundedUse(family))
			schema, err := discoverTestSchemaWithPolicy(t, builtIn, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			owner := modelGroupReferenceTestComplexTypeNamed(t, schema, "urn:root", "Derived")
			use, hasUse := owner.AttributeUses()[0].(LocalAttributeUse)
			if !hasUse {
				t.Fatal("grouped built-in use missing")
			}
			ref, hasRef := use.TypeReference()
			if !hasRef || !ref.IsBuiltin() {
				t.Fatalf("grouped builtin = %#v", use)
			}
			assertAllIntegerBounds(t, ref, profile.version, family.minimum, family.maximum, Loc{}, Loc{})
			named := strings.Replace(builtIn, `type="xs:`+family.name+`"`, `type="t:Alias"`, 1)
			named = strings.Replace(named, `</xs:schema>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:`+family.name+`"/></xs:simpleType></xs:schema>`, 1)
			failed, failure := discoverTestSchemaWithPolicy(t, named, nil, profile.policy)
			assertNCNameDiagnostic(t, failed, failure, FailureUnsupported, UnsupportedSchemaSyntaxCode, complexContentTestLoc(t, named, `type="t:Alias"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeUseUnsupported)
		}
	}
}

//nolint:gocognit // Each discovered named target keeps its source identity and lexical use order.
func TestBoundedSequenceExtensionNamedTypeGraphs(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			t.Run(profile.name+"/"+family.name, func(t *testing.T) {
				root := boundedLocalRoot(string(profile.version), `<xs:include schemaLocation="included.xsd"/><xs:include schemaLocation="chameleon.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/><xs:complexType name="Derived"><xs:complexContent><xs:extension base="r:Base"><xs:sequence><xs:element name="v" type="xs:integer"/></xs:sequence><xs:attribute name="forward" type="r:Forward"/><xs:attribute name="included" type="r:Included"/><xs:attribute name="adopted" type="r:Adopted"/><xs:attribute name="foreign" type="o:Foreign"/></xs:extension></xs:complexContent></xs:complexType><xs:complexType name="Base"/><xs:simpleType name="Forward"><xs:restriction base="xs:`+family.name+`"/></xs:simpleType>`)
				included := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bounded-local"><xs:include schemaLocation="root.xsd"/><xs:simpleType name="Included"><xs:restriction base="xs:` + family.name + `"/></xs:simpleType></xs:schema>`
				chameleon := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Adopted"><xs:restriction base="xs:` + family.name + `"/></xs:simpleType></xs:schema>`
				other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Foreign"><xs:restriction base="xs:` + family.name + `"/></xs:simpleType></xs:schema>`
				fixtures := map[string]discoveryFixture{"root.xsd": {id: "root.xsd", contents: root}, "included.xsd": {id: "included.xsd", contents: included}, "chameleon.xsd": {id: "chameleon.xsd", contents: chameleon}, "other.xsd": {id: "other.xsd", contents: other}}
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				var walk []string
				if err := schema.Walk(func(c Component) error { walk = append(walk, string(c.ID().Source())+":"+c.Name().Local()); return nil }); err != nil {
					t.Fatal(err)
				}
				wantWalk := []string{"root.xsd:Derived", "root.xsd:Base", "root.xsd:Forward", "included.xsd:Included", "chameleon.xsd:Adopted", "other.xsd:Foreign"}
				if !reflect.DeepEqual(walk, wantWalk) {
					t.Fatalf("walk = %v", walk)
				}
				owner := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:bounded-local", "Derived"))[0], "Derived")
				for i, expected := range []struct{ name, ns, source, contents string }{{"Forward", "urn:bounded-local", "root.xsd", root}, {"Included", "urn:bounded-local", "included.xsd", included}, {"Adopted", "urn:bounded-local", "chameleon.xsd", chameleon}, {"Foreign", "urn:other", "other.xsd", other}} {
					use, ok := owner.AttributeUses()[i].(LocalAttributeUse)
					ref, hasRef := use.TypeReference()
					id, hasID := use.TypeID()
					if !ok || !hasRef || !hasID || !ref.IsNamed() || use.DeclaredType() != mustTestQName(t, expected.ns, expected.name) || id.Source() != SourceID(expected.source) || id != componentIDForName(t, schema, use.DeclaredType()) || ref.VarietyLoc() != mustSchemaTokenLoc(t, SourceID(expected.source), expected.contents, 1, `<xs:restriction base="xs:`+family.name+`"`) {
						t.Fatalf("named graph use %d = %#v", i, use)
					}
					assertAllIntegerBounds(t, ref, profile.version, family.minimum, family.maximum, Loc{}, Loc{})
				}
			})
		}
	}
}

func TestBoundedSequenceExtensionAcquisitionFailure(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, family := range boundedLocalFamilies() {
			root := sequenceAttributeExtensionSchema(string(profile.version), "", `<xs:element name="v" type="xs:integer"/>`, boundedSequenceExtensionUse("xs:"+family.name), "")
			root = strings.Replace(root, `<xs:element name="root"`, `<xs:include schemaLocation="missing.xsd"/><xs:element name="root"`, 1)
			source, sourceErr := NewResolvedSource(context.Background(), "root.xsd", io.NopCloser(strings.NewReader(root)))
			if sourceErr != nil {
				t.Fatal(sourceErr)
			}
			cause := errors.New("bounded extension source unavailable")
			schema, err := ParseSchemaWithPolicy(source, &discoveryResolver{failures: map[string]error{"missing.xsd": cause}}, profile.policy)
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			if d.Class() != FailureResolution || d.Code() != SourceResolveCode || d.Loc() != complexContentTestLoc(t, root, `<xs:include`) || d.SpecRef() != "" || len(d.Related()) != 0 || !errors.Is(err, cause) {
				t.Fatalf("acquisition = %v related=%v", err, d.Related())
			}
		}
	}
}
