package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit,funlen // Each row proves the newly admitted type and owner through public facts.
func TestLongSequenceExtensionPublicFacts(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, graph := range []string{"forward", "included", "imported", "chameleon"} {
			t.Run(profile.name+"/"+graph, func(t *testing.T) {
				fixture := emptyComplexContentExtensionGraph(t, string(profile.version), graph)
				if graph == "imported" {
					fixture.root = strings.Replace(fixture.root, `xmlns:b="urn:base"`, `xmlns:b="urn:base" xmlns:t="urn:root"`, 1)
				}
				uses := `<xs:attribute name="builtin" type="xs:long" use="required"/><xs:attribute name="named" type="t:Narrow"/><xs:attribute name="middle" type="xs:integer"/><xs:attribute name="gone" type="xs:long" use="prohibited"/>`
				body := `<xs:extension base="` + fixture.baseLexical + `"><xs:sequence minOccurs="2" maxOccurs="3"><xs:element name="value" type="xs:integer" minOccurs="0" maxOccurs="4"/><xs:element name="last" type="xs:integer"/></xs:sequence>` + uses + `</xs:extension>`
				fixture.root = strings.Replace(fixture.root, `<xs:extension base="`+fixture.baseLexical+`"/>`, body, 1)
				fixture.root = strings.Replace(fixture.root, `</xs:schema>`, `<xs:simpleType name="Narrow"><xs:restriction base="t:Wide"><xs:minInclusive value="-3"/><xs:maxInclusive value="4"/><xs:enumeration value="2"/></xs:restriction></xs:simpleType><xs:simpleType name="Wide"><xs:restriction base="xs:long"><xs:minInclusive value="-100"/><xs:maxInclusive value="100"/><xs:totalDigits value="3"/></xs:restriction></xs:simpleType></xs:schema>`, 1)
				schema, err := discoverTestSchemaWithPolicy(t, fixture.root, fixture.fixtures, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				again, err := discoverTestSchemaWithPolicy(t, fixture.root, fixture.fixtures, profile.policy)
				if err != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
					t.Fatalf("nondeterministic public components: %v", err)
				}
				found := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Derived"))
				if len(found) != 1 {
					t.Fatalf("derived = %v", found)
				}
				owner := requireTestComplexTypeDefinition(t, found[0], "Derived")
				if owner.Loc() != complexContentTestLoc(t, fixture.root, `<xs:complexType name="Derived"`) || owner.Derivation() != ComplexTypeDerivationExtension || owner.DerivationLoc() != complexContentTestLoc(t, fixture.root, `<xs:extension`) || owner.BaseLoc() != complexContentTestLoc(t, fixture.root, `base="`+fixture.baseLexical+`"`) {
					t.Fatalf("owner derivation = %#v", owner)
				}
				base, ok := owner.BaseReference()
				baseID, hasID := base.ComponentID()
				if !ok || !hasID || base.Name() != mustTestQName(t, fixture.baseNamespace, "Base") || baseID.Source() != fixture.baseSource || base.Loc() != owner.BaseLoc() {
					t.Fatalf("base = %#v", base)
				}
				wildcard, ok := owner.AnyAttribute()
				if !ok || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" || wildcard.Loc() != complexContentTestSourceLoc(t, fixture.baseSource, fixture.baseContents, `<xs:anyAttribute`) {
					t.Fatalf("inherited wildcard = %#v", wildcard)
				}
				sequence, ok := owner.Particle().(SequenceParticle)
				if !ok || sequence.Occurrences().String() != "2/3" || sequence.Loc() != complexContentTestLoc(t, fixture.root, `<xs:sequence`) {
					t.Fatalf("sequence = %#v", owner.Particle())
				}
				terms := sequence.Particles()
				if len(terms) != 2 || terms[0].Occurrences().String() != "0/4" || terms[0].Loc() != complexContentTestLoc(t, fixture.root, `<xs:element name="value"`) || terms[1].Occurrences().String() != "1/1" || terms[1].Loc() != complexContentTestLoc(t, fixture.root, `<xs:element name="last"`) {
					t.Fatalf("particle terms = %v", terms)
				}
				firstTerm, firstOK := terms[0].(ElementParticle)
				lastTerm, lastOK := terms[1].(ElementParticle)
				if !firstOK || !lastOK || firstTerm.Name().Local() != "value" || lastTerm.Name().Local() != "last" {
					t.Fatalf("lexical particle order = %v", terms)
				}
				usesOut := owner.AttributeUses()
				if len(usesOut) != 3 {
					t.Fatalf("uses = %v", usesOut)
				}
				for i, want := range []struct{ local, lexical, min, max string }{{"builtin", "xs:long", "-9223372036854775808", "9223372036854775807"}, {"named", "t:Narrow", "-3", "4"}, {"middle", "xs:integer", "", ""}} {
					use, useOK := usesOut[i].(LocalAttributeUse)
					if !useOK {
						t.Fatalf("use %d = %T", i, usesOut[i])
					}
					if use.Name() != mustTestQName(t, "", want.local) || use.Loc() != complexContentTestLoc(t, fixture.root, `<xs:attribute name="`+want.local+`"`) || use.NameLoc() != complexContentTestLoc(t, fixture.root, `name="`+want.local+`"`) || use.TypeLoc() != unsignedLongParticleTokenLocAfter(t, fixture.root, `<xs:attribute name="`+want.local+`"`, `type="`+want.lexical+`"`) {
						t.Fatalf("use %d = %#v", i, usesOut[i])
					}
					ref, hasRef := use.TypeReference()
					if !hasRef || ref.Loc() != use.TypeLoc() || ref.Name() != use.DeclaredType() || ref.QName() != use.DeclaredType() {
						t.Fatalf("type ref %d = %#v", i, ref)
					}
					if i == 0 && (use.Use() != AttributeUseRequired || use.UseLoc() != complexContentTestLoc(t, fixture.root, `use="required"`) || !ref.IsBuiltin() || use.DeclaredType() != mustTestQName(t, testXSDNamespace, "long")) {
						t.Fatalf("builtin facts = %#v", use)
					}
					if i == 1 {
						id, hasID := use.TypeID()
						refID, refHasID := ref.ComponentID()
						if !ref.IsNamed() || !hasID || !refHasID || id != refID || id != componentIDForName(t, schema, use.DeclaredType()) || id.Source() != "root.xsd" || ref.VarietyLoc() != complexContentTestLoc(t, fixture.root, `<xs:restriction base="t:Wide"`) {
							t.Fatalf("named identity = %#v", use)
						}
					}
					if i < 2 {
						minLoc, maxLoc := Loc{}, Loc{}
						if i == 1 {
							minLoc = complexContentTestLoc(t, fixture.root, `value="-3"`)
							maxLoc = complexContentTestLoc(t, fixture.root, `value="4"`)
						}
						assertAllIntegerBounds(t, ref, profile.version, want.min, want.max, minLoc, maxLoc)
					}
				}
				narrow := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:root", "Narrow"))
				if len(narrow) != 1 {
					t.Fatalf("Narrow = %v", narrow)
				}
				definition, ok := narrow[0].SimpleTypeDefinition()
				if !ok {
					t.Fatal("Narrow has no simple type view")
				}
				digits, hasDigits := definition.DigitFacets().TotalDigits()
				digitsLoc, hasDigitsLoc := definition.DigitFacets().TotalDigitsLoc()
				enumeration := definition.IntegerEnumerationFacets()
				values, locations := enumeration.Values(), enumeration.Locations()
				if !hasDigits || digits.Canonical() != "3" || !hasDigitsLoc || digitsLoc != complexContentTestLoc(t, fixture.root, `value="3"`) || len(values) != 1 || values[0].Canonical() != "2" || !reflect.DeepEqual(locations, []Loc{complexContentTestLoc(t, fixture.root, `<xs:enumeration value="2"`)}) {
					t.Fatalf("copied digit/enumeration facts = %v/%v at %v/%v", digits, values, digitsLoc, locations)
				}
				values[0].value.SetInt64(99)
				locations[0] = Loc{}
				if definition.IntegerEnumerationFacets().Values()[0].Canonical() != "2" {
					t.Fatal("enumeration copy mutated definition")
				}
				namedUse, namedUseOK := usesOut[1].(LocalAttributeUse)
				if !namedUseOK {
					t.Fatalf("named use = %T", usesOut[1])
				}
				ref, refOK := namedUse.TypeReference()
				if !refOK {
					t.Fatal("named use type reference missing")
				}
				bounds, ok := ref.IntegerBounds()
				if !ok {
					t.Fatal("Narrow use has no bounds")
				}
				for _, bound := range bounds.Bounds() {
					bound.Value().value.SetInt64(0)
				}
				assertAllIntegerBounds(t, ref, profile.version, "-3", "4", complexContentTestLoc(t, fixture.root, `value="-3"`), complexContentTestLoc(t, fixture.root, `value="4"`))
				usesOut[0] = nil
				terms[0] = nil
				freshSequence, sequenceOK := owner.Particle().(SequenceParticle)
				if !sequenceOK || owner.AttributeUses()[0].Name().Local() != "builtin" || len(freshSequence.Particles()) != 2 {
					t.Fatal("public slices mutated schema")
				}
			})
		}
	}
}

//nolint:gocognit // Validate and generate have separate observable failure contracts.
func TestLongSequenceExtensionConsumersRejectEachUse(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"builtin", "named"} {
			t.Run(profile.name+"/"+kind, func(t *testing.T) {
				lexical, extra := "xs:long", ""
				if kind == "named" {
					lexical = "t:Long"
					extra = `<xs:simpleType name="Long"><xs:restriction base="xs:long"/></xs:simpleType>`
				}
				root := sequenceAttributeExtensionSchema(string(profile.version), "", `<xs:element name="value" type="xs:integer"/>`, `<xs:attribute name="a" type="`+lexical+`"/>`, "")
				root = strings.Replace(root, `</xs:schema>`, extra+`</xs:schema>`, 1)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root" a="1"/>`)))
				vd := requireDiagnostic(t, validationErr)
				if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != mustTestLoc(t, "instance.xml", 1, 1) || vd.SpecRef() != instanceValidationSpecRef(profile.version) || !errors.Is(validationErr, errInstanceComplexContentExtension) {
					t.Fatalf("validation = %v", validationErr)
				}
				output, generationErr := GenerateGo(schema, "generated")
				gd := requireDiagnostic(t, generationErr)
				if output != nil || gd.Class() != FailureUnsupported || gd.Code() != diagnosticCodegenUnsupported || gd.Loc() != complexContentTestLoc(t, root, `<xs:attribute name="a"`) || gd.SpecRef() != codegenDirectSequenceSpecReference(profile.version, codegenDirectSequenceParticlesReference) || !errors.Is(generationErr, errCodegenUnsupported) {
					t.Fatalf("generation = %v/%v", output, generationErr)
				}
			})
		}
	}
}

//nolint:gocognit // The matrix pins each local form at the newly admitted owner boundary.
func TestLongSequenceExtensionExcludedLocalForms(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct {
			name, use, extra, marker, related, code string
			spec                                    func(XSDVersion) string
			cause                                   error
		}{
			{"inline atomic", `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:long"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, "", "", schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnsupported},
			{"named list", `<xs:attribute name="a" type="t:List"/>`, `<xs:simpleType name="List"><xs:list itemType="xs:long"/></xs:simpleType>`, `type="t:List"`, `<xs:simpleType name="List"|<xs:list`, "", schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported},
			{"named union", `<xs:attribute name="a" type="t:Union"/>`, `<xs:simpleType name="Union"><xs:union memberTypes="xs:long"/></xs:simpleType>`, `type="t:Union"`, `<xs:simpleType name="Union"|<xs:union`, "", schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported},
			{"inline list", `<xs:attribute name="a"><xs:simpleType><xs:list itemType="xs:long"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, `<xs:list`, "", schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported},
			{"inline union", `<xs:attribute name="a"><xs:simpleType><xs:union memberTypes="xs:long"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType`, `<xs:union`, "", schemaAttributeUseSpecRef, errSchemaAttributeUseUnsupported},
			{"ref direct", `<xs:attribute ref="t:a"/>`, `<xs:attribute name="a" type="xs:long"/>`, `ref="t:a"`, `<xs:attribute name="a" type="xs:long"`, "", schemaAttributeUseSpecRef, errSchemaAttributeReferenceUnsupported},
			{"ref named", `<xs:attribute ref="t:a"/>`, `<xs:attribute name="a" type="t:Long"/><xs:simpleType name="Long"><xs:restriction base="xs:long"/></xs:simpleType>`, `ref="t:a"`, `<xs:attribute name="a" type="t:Long"`, "", schemaAttributeUseSpecRef, errSchemaAttributeReferenceUnsupported},
			{"ref inline", `<xs:attribute ref="t:a"/>`, `<xs:attribute name="a"><xs:simpleType><xs:restriction base="xs:long"/></xs:simpleType></xs:attribute>`, `ref="t:a"`, `<xs:attribute name="a"`, "", schemaAttributeUseSpecRef, errSchemaAttributeReferenceUnsupported},
			{"direct default", `<xs:attribute name="a" type="xs:long" default="1"/>`, "", `default="1"`, "", "", schemaComplexTypeExtensionSpecRef, errSchemaAttributeUseUnsupported},
			{"direct fixed", `<xs:attribute name="a" type="xs:long" fixed="1"/>`, "", `fixed="1"`, "", "", schemaComplexTypeExtensionSpecRef, errSchemaAttributeUseUnsupported},
			{"named default", `<xs:attribute name="a" type="t:Long" default="1"/>`, `<xs:simpleType name="Long"><xs:restriction base="xs:long"/></xs:simpleType>`, `default="1"`, "", "", schemaComplexTypeExtensionSpecRef, errSchemaAttributeUseUnsupported},
			{"named fixed", `<xs:attribute name="a" type="t:Long" fixed="1"/>`, `<xs:simpleType name="Long"><xs:restriction base="xs:long"/></xs:simpleType>`, `fixed="1"`, "", "", schemaComplexTypeExtensionSpecRef, errSchemaAttributeUseUnsupported},
			{"inline default", `<xs:attribute name="a" default="1"><xs:simpleType><xs:restriction base="xs:long"/></xs:simpleType></xs:attribute>`, "", `default="1"`, "", "", schemaComplexTypeExtensionSpecRef, errSchemaAttributeUseUnsupported},
			{"inline fixed", `<xs:attribute name="a" fixed="1"><xs:simpleType><xs:restriction base="xs:long"/></xs:simpleType></xs:attribute>`, "", `fixed="1"`, "", "", schemaComplexTypeExtensionSpecRef, errSchemaAttributeUseUnsupported},
			{"ref default", `<xs:attribute ref="t:a" default="1"/>`, `<xs:attribute name="a" type="xs:long"/>`, `default="1"`, "", "", schemaComplexTypeExtensionSpecRef, errSchemaAttributeUseUnsupported},
			{"ref fixed", `<xs:attribute ref="t:a" fixed="1"/>`, `<xs:attribute name="a" type="xs:long"/>`, `fixed="1"`, "", "", schemaComplexTypeExtensionSpecRef, errSchemaAttributeUseUnsupported},
			{"new facet", `<xs:attribute name="a" type="t:Facet"/>`, `<xs:simpleType name="Facet"><xs:restriction base="xs:long"><xs:pattern value="[0-9]+"/></xs:restriction></xs:simpleType>`, `<xs:pattern`, "", UnsupportedDatatypeFacetCode, func(v XSDVersion) string { return tokenDiagnosticSpecRef(v, "decimal") }, ErrUnsupported},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := sequenceAttributeExtensionSchema(string(profile.version), "", `<xs:element name="value" type="xs:integer"/>`, test.use, "")
				root = strings.Replace(root, `</xs:schema>`, test.extra+`</xs:schema>`, 1)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				var related []Loc
				if test.related != "" {
					for _, marker := range strings.Split(test.related, "|") {
						related = append(related, complexContentTestLoc(t, root, marker))
					}
				}
				code := test.code
				if code == "" {
					code = UnsupportedSchemaSyntaxCode
				}
				assertNCNameDiagnostic(t, schema, err, FailureUnsupported, code, complexContentTestLoc(t, root, test.marker), related, test.spec(profile.version), test.cause)
			})
		}
	}
}

//nolint:gocognit // Pair every competing exit with its phase-selected diagnostic.
func TestLongSequenceExtensionBoundaryDiagnostics(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct {
			name, member, use, extra, base, marker, related, code string
			class                                                 FailureClass
			spec                                                  func(XSDVersion) string
			cause                                                 error
		}{
			{"child before use and base", `<xs:element ref="t:MissingElement"/>`, `<xs:attribute name="a" type="t:MissingType"/>`, "", "t:MissingBase", `ref="t:MissingElement"`, "", diagnosticSchemaElementReferenceUnresolvedCode, FailureInvalid, schemaElementReferenceSpecRef, errSchemaElementReferenceUnresolved},
			{"invalid child range before use", `<xs:element name="v" type="xs:integer" minOccurs="2" maxOccurs="1"/>`, `<xs:attribute name="a" type="t:MissingType"/>`, "", "t:MissingBase", `<xs:element name="v"`, `minOccurs="2"|maxOccurs="1"`, invalidSchemaCompositionCode, FailureInvalid, schemaParticleCorrectSpecRef, errParticleOccurrenceMinimumExceedsMaximum},
			{"zero child before prohibited use", `<xs:element ref="t:MissingElement"/>`, `<xs:attribute name="a" type="t:MissingType" use="prohibited"/>`, "", "t:MissingBase", `ref="t:MissingElement"`, "", diagnosticSchemaElementReferenceUnresolvedCode, FailureInvalid, schemaElementReferenceSpecRef, errSchemaElementReferenceUnresolved},
			{"use before base", `<xs:element name="v" type="xs:integer"/>`, `<xs:attribute name="a" type="t:MissingType"/>`, "", "t:MissingBase", `type="t:MissingType"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, FailureInvalid, schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnresolved},
			{"prohibited use before base", `<xs:element name="v" type="xs:integer"/>`, `<xs:attribute name="a" type="t:MissingType" use="prohibited"/>`, "", "t:MissingBase", `type="t:MissingType"`, "", diagnosticSchemaAttributeTypeUnresolvedCode, FailureInvalid, schemaAttributeTypeSpecRef, errSchemaAttributeTypeUnresolved},
			{"wrong kind use", `<xs:element name="v" type="xs:integer"/>`, `<xs:attribute name="a" type="t:Wrong"/>`, `<xs:complexType name="Wrong"/>`, "t:Base", `type="t:Wrong"`, `<xs:complexType name="Wrong"`, diagnosticSchemaAttributeTypeWrongKindCode, FailureInvalid, schemaAttributeTypeSpecRef, errSchemaAttributeTypeWrongKind},
			{"type cycle", `<xs:element name="v" type="xs:integer"/>`, `<xs:attribute name="a" type="t:First"/>`, `<xs:simpleType name="First"><xs:restriction base="t:Second"/></xs:simpleType><xs:simpleType name="Second"><xs:restriction base="t:First"/></xs:simpleType>`, "t:Base", `type="t:First"`, `<xs:attribute name="a"|base="t:Second"|base="t:First"|<xs:simpleType name="First"|<xs:simpleType name="Second"`, diagnosticSchemaAttributeTypeCycleCode, FailureInvalid, schemaAttributeTypeSpecRef, errSchemaSimpleTypeBaseCycle},
			{"malformed long bound", `<xs:element name="v" type="xs:integer"/>`, `<xs:attribute name="a" type="t:Bad"/>`, `<xs:simpleType name="Bad"><xs:restriction base="xs:long"><xs:minInclusive value="bad"/></xs:restriction></xs:simpleType>`, "t:Base", `value="bad"`, "", InvalidBoundCode, FailureInvalid, func(v XSDVersion) string { return boundSpecRef(v, BoundMinInclusive, boundDefinitionRule) }, errInvalidBoundValue},
			{"invalid long bound", `<xs:element name="v" type="xs:integer"/>`, `<xs:attribute name="a" type="t:Bad"/>`, `<xs:simpleType name="Bad"><xs:restriction base="xs:long"><xs:maxInclusive value="9223372036854775808"/></xs:restriction></xs:simpleType>`, "t:Base", `value="9223372036854775808"`, "", InvalidBoundRestrictionCode, FailureInvalid, func(v XSDVersion) string { return boundSpecRef(v, BoundMaxInclusive, boundRestrictionRule) }, errInvalidBoundRestriction},
			{"duplicate use before base", `<xs:element name="v" type="xs:integer"/>`, `<xs:attribute name="a" type="xs:long"/><xs:attribute name="a" type="xs:long" use="required"/>`, "", "t:MissingBase", `<xs:attribute name="a" type="xs:long" use="required"`, `<xs:attribute name="a" type="xs:long"/>`, diagnosticSchemaAttributeUseDuplicateCode, FailureInvalid, schemaAttributeUseSpecRef, errSchemaAttributeUseDuplicate},
			{"unresolved base", `<xs:element name="v" type="xs:integer"/>`, `<xs:attribute name="a" type="xs:long"/>`, "", "t:MissingBase", `base="t:MissingBase"`, "", invalidSchemaCompositionCode, FailureInvalid, schemaComplexTypeExtensionSpecRef, errSchemaComplexTypeBaseUnresolved},
			{"wrong kind base", `<xs:element name="v" type="xs:integer"/>`, `<xs:attribute name="a" type="xs:long"/>`, "", "t:NamedID", `base="t:NamedID"`, `<xs:simpleType name="NamedID"`, invalidSchemaCompositionCode, FailureInvalid, schemaComplexTypeExtensionSpecRef, errSchemaComplexTypeBaseWrongKind},
			{"base final", `<xs:element name="v" type="xs:integer"/>`, `<xs:attribute name="a" type="xs:long"/>`, "", "t:Base", `base="t:Base"`, `final="extension"`, invalidSchemaCompositionCode, FailureInvalid, schemaComplexTypeExtensionSpecRef, errSchemaComplexTypeBaseUnsupported},
			{"base cycle", `<xs:element name="v" type="xs:integer"/>`, `<xs:attribute name="a" type="xs:long"/>`, "", "t:Derived", `base="t:Derived"`, "", invalidSchemaCompositionCode, FailureInvalid, schemaComplexTypeExtensionSpecRef, errSchemaComplexTypeBaseCycle},
			{"nonempty base", `<xs:element name="v" type="xs:integer"/>`, `<xs:attribute name="a" type="xs:long"/>`, "", "t:Base", `base="t:Base"`, `<xs:complexType name="Base"|<xs:sequence><xs:element name="prior"`, UnsupportedSchemaSyntaxCode, FailureUnsupported, schemaComplexTypeExtensionSpecRef, errSchemaComplexTypeBaseNonEmpty},
			{"base uses", `<xs:element name="v" type="xs:integer"/>`, `<xs:attribute name="a" type="xs:long"/>`, "", "t:Base", `base="t:Base"`, `<xs:complexType name="Base"`, UnsupportedSchemaSyntaxCode, FailureUnsupported, schemaComplexTypeExtensionSpecRef, errSchemaComplexTypeBaseUnsupported},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				base := ""
				if test.name == "nonempty base" {
					base = `<xs:sequence><xs:element name="prior" type="xs:integer"/></xs:sequence>`
				}
				if test.name == "base uses" {
					base = `<xs:attribute name="inherited" type="xs:long"/>`
				}
				outer := ""
				if test.name == "zero child before prohibited use" {
					outer = ` minOccurs="0" maxOccurs="0"`
				}
				root := sequenceAttributeExtensionSchema(string(profile.version), outer, test.member, test.use, base)
				if test.name == "base final" {
					root = strings.Replace(root, `<xs:complexType name="Base">`, `<xs:complexType name="Base" final="extension">`, 1)
				}
				root = strings.Replace(root, `base="t:Base"`, `base="`+test.base+`"`, 1)
				root = strings.Replace(root, `</xs:schema>`, test.extra+`</xs:schema>`, 1)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				var related []Loc
				if test.related != "" {
					for _, marker := range strings.Split(test.related, "|") {
						related = append(related, complexContentTestLoc(t, root, marker))
					}
				}
				assertNCNameDiagnostic(t, schema, err, test.class, test.code, complexContentTestLoc(t, root, test.marker), related, test.spec(profile.version), test.cause)
			})
		}
	}
}

func TestLongSequenceExtensionExcludedOwners(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"builtin", "named"} {
			lexical, typeDefinition := "xs:long", ""
			if kind == "named" {
				lexical = "t:Long"
				typeDefinition = `<xs:simpleType name="Long"><xs:restriction base="xs:long"/></xs:simpleType>`
			}
			use := `<xs:attribute name="a" type="` + lexical + `"/>`
			for _, test := range []struct {
				name, owner, extra, marker string
				spec                       func(XSDVersion) string
				cause                      error
			}{
				{"grouped", `<xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Base"><xs:group ref="t:Fields"/>` + use + `</xs:extension></xs:complexContent></xs:complexType>`, `<xs:group name="Fields"><xs:sequence/></xs:group>`, `type="` + lexical + `"`, schemaAttributeTypeSpecRef, errSchemaAttributeUseUnsupported},
				{"anonymous sequence", `<xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:Base"><xs:sequence><xs:element name="v" type="xs:integer"/></xs:sequence>` + use + `</xs:extension></xs:complexContent></xs:complexType></xs:element>`, "", `type="` + lexical + `"`, schemaAttributeTypeSpecRef, errSchemaAttributeUseUnsupported},
				{"choice extension", `<xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Base"><xs:choice><xs:element name="v" type="xs:integer"/></xs:choice>` + use + `</xs:extension></xs:complexContent></xs:complexType>`, "", `<xs:attribute name="a"`, schemaComplexTypeExtensionSpecRef, ErrUnsupported},
				{"all extension", `<xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Base"><xs:all><xs:element name="v" type="xs:integer"/></xs:all>` + use + `</xs:extension></xs:complexContent></xs:complexType>`, "", `<xs:attribute name="a"`, schemaComplexTypeExtensionSpecRef, ErrUnsupported},
				{"nested sequence", `<xs:complexType name="Derived"><xs:complexContent><xs:extension base="t:Base"><xs:sequence><xs:sequence><xs:element name="v" type="xs:integer"/></xs:sequence></xs:sequence>` + use + `</xs:extension></xs:complexContent></xs:complexType>`, "", `<xs:sequence><xs:element`, schemaComplexParticleExtensionSpecRef, ErrUnsupported},
				{"restriction owner", `<xs:complexType name="Derived"><xs:complexContent><xs:restriction base="t:Base"><xs:sequence><xs:element name="v" type="xs:integer"/></xs:sequence>` + use + `</xs:restriction></xs:complexContent></xs:complexType>`, "", `<xs:sequence`, schemaSyntaxSpecRefForVersion, ErrUnsupported},
			} {
				t.Run(profile.name+"/"+kind+"/"+test.name, func(t *testing.T) {
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" version="` + string(profile.version) + `">` + test.owner + `<xs:complexType name="Base"/>` + test.extra + typeDefinition + `</xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertNCNameDiagnostic(t, schema, err, FailureUnsupported, UnsupportedSchemaSyntaxCode, complexContentTestLoc(t, root, test.marker), nil, test.spec(profile.version), test.cause)
				})
			}
		}
	}
}

//nolint:gocognit // Discovery and type visibility are asserted against the public use references.
func TestLongSequenceExtensionTypeGraphAndVisibility(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := longLocalRoot(string(profile.version), `<xs:include schemaLocation="included.xsd"/><xs:include schemaLocation="chameleon.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/><xs:complexType name="Derived"><xs:complexContent><xs:extension base="r:Base"><xs:sequence><xs:element name="v" type="xs:integer"/></xs:sequence><xs:attribute name="forward" type="r:Forward"/><xs:attribute name="included" type="r:Included"/><xs:attribute name="adopted" type="r:Adopted"/><xs:attribute name="foreign" type="o:Foreign"/></xs:extension></xs:complexContent></xs:complexType><xs:complexType name="Base"/><xs:simpleType name="Forward"><xs:restriction base="xs:long"/></xs:simpleType>`)
			included := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:long-local"><xs:include schemaLocation="root.xsd"/><xs:simpleType name="Included"><xs:restriction base="xs:long"/></xs:simpleType></xs:schema>`
			chameleon := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Adopted"><xs:restriction base="xs:long"/></xs:simpleType></xs:schema>`
			other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Foreign"><xs:restriction base="xs:long"/></xs:simpleType></xs:schema>`
			fixtures := map[string]discoveryFixture{"root.xsd": {id: "root.xsd", contents: root}, "included.xsd": {id: "included.xsd", contents: included}, "chameleon.xsd": {id: "chameleon.xsd", contents: chameleon}, "other.xsd": {id: "other.xsd", contents: other}}
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			var walk []string
			if err := schema.Walk(func(c Component) error {
				walk = append(walk, string(c.ID().Source())+":"+c.Name().Local())
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			wantWalk := []string{"root.xsd:Derived", "root.xsd:Base", "root.xsd:Forward", "included.xsd:Included", "chameleon.xsd:Adopted", "other.xsd:Foreign"}
			if !reflect.DeepEqual(walk, wantWalk) {
				t.Fatalf("walk = %v", walk)
			}
			derived := requireTestComplexTypeDefinition(t, schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:long-local", "Derived"))[0], "Derived")
			for i, want := range []struct{ local, ns, source string }{{"Forward", "urn:long-local", "root.xsd"}, {"Included", "urn:long-local", "included.xsd"}, {"Adopted", "urn:long-local", "chameleon.xsd"}, {"Foreign", "urn:other", "other.xsd"}} {
				use, ok := derived.AttributeUses()[i].(LocalAttributeUse)
				ref, hasRef := use.TypeReference()
				id, hasID := use.TypeID()
				if !ok || !hasRef || !hasID || !ref.IsNamed() || use.DeclaredType() != mustTestQName(t, want.ns, want.local) || id.Source() != SourceID(want.source) || ref.Variety() != SimpleTypeVarietyAtomicRestriction {
					t.Fatalf("graph use %d = %#v", i, use)
				}
				assertAllIntegerBounds(t, ref, profile.version, "-9223372036854775808", "9223372036854775807", Loc{}, Loc{})
			}
		})
		t.Run(profile.name+"/invisible", func(t *testing.T) {
			root := longLocalRoot(string(profile.version), `<xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/><xs:complexType name="Derived"><xs:complexContent><xs:extension base="r:Base"><xs:sequence><xs:element name="v" type="xs:integer"/></xs:sequence><xs:attribute name="a" type="o:Hidden"/></xs:extension></xs:complexContent></xs:complexType><xs:complexType name="Base"/>`)
			fixtures := map[string]discoveryFixture{"bridge.xsd": {id: "bridge.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge"><xs:import namespace="urn:other" schemaLocation="other.xsd"/></xs:schema>`}, "other.xsd": {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Hidden"><xs:restriction base="xs:long"/></xs:simpleType></xs:schema>`}}
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			assertNCNameDiagnostic(t, schema, err, FailureInvalid, diagnosticSchemaAttributeTypeUnresolvedCode, complexContentTestLoc(t, root, `type="o:Hidden"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved)
		})
	}
}

//nolint:gocognit // Keep both policy labels and omitted shapes in one matrix.
func TestLongSequenceExtensionZeroRangeAndEditionPolicy(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, label := range []string{"1.0", "1.1"} {
			t.Run(profile.name+"/"+label, func(t *testing.T) {
				root := sequenceAttributeExtensionSchema(label, ` minOccurs="0" maxOccurs="0"`, `<xs:element name="v" type="xs:integer"/>`, `<xs:attribute name="a" type="xs:long"/><xs:attribute name="gone" type="xs:long" use="prohibited"/>`, "")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				derived := requireTestComplexTypeDefinition(t, schema.Components()[1], "Derived")
				uses := derived.AttributeUses()
				if derived.Particle() != nil || len(uses) != 1 || uses[0].Name().Local() != "a" {
					t.Fatalf("zero sequence and omitted use = %v/%v", derived.Particle(), uses)
				}
				use, useOK := uses[0].(LocalAttributeUse)
				if !useOK {
					t.Fatalf("long use = %T", uses[0])
				}
				ref, refOK := use.TypeReference()
				if !refOK {
					t.Fatal("long type reference missing")
				}
				assertAllIntegerBounds(t, ref, profile.version, "-9223372036854775808", "9223372036854775807", Loc{}, Loc{})
				bad := strings.Replace(root, `type="xs:long" use="prohibited"`, `type="t:Missing" use="prohibited"`, 1)
				failed, failure := discoverTestSchemaWithPolicy(t, bad, nil, profile.policy)
				assertNCNameDiagnostic(t, failed, failure, FailureInvalid, diagnosticSchemaAttributeTypeUnresolvedCode, complexContentTestLoc(t, bad, `type="t:Missing"`), nil, schemaAttributeTypeSpecRef(profile.version), errSchemaAttributeTypeUnresolved)
			})
		}
	}
}

func TestLongSequenceExtensionDuplicateTypeCandidatePrecedesAmbiguity(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := sequenceAttributeExtensionSchema(string(profile.version), "", `<xs:element name="v" type="xs:integer"/>`, `<xs:attribute name="a" type="t:Alias"/>`, "")
			root = strings.Replace(root, `</xs:schema>`, `<xs:simpleType name="Alias"><xs:restriction base="xs:long"/></xs:simpleType><xs:simpleType name="Alias" id="second"><xs:restriction base="xs:long"/></xs:simpleType></xs:schema>`, 1)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			first := complexContentTestLoc(t, root, `<xs:simpleType name="Alias"><xs:restriction`)
			second := complexContentTestLoc(t, root, `<xs:simpleType name="Alias" id="second"`)
			assertNCNameDiagnostic(t, schema, err, FailureInvalid, diagnosticSchemaGlobalDuplicateCode, second, []Loc{first}, schemaGlobalDuplicateSpecRef(profile.version), errSchemaGlobalDeclarationDuplicate)
			primary := complexContentTestLoc(t, root, `type="t:Alias"`)
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
