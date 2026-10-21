package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func anonymousGroupedInheritableSchema(marker, groupBounds string) string {
	uses := `<xs:attribute name="named" type="t:Named" form="qualified" use="required"` + marker + `/>` +
		`<xs:attribute name="inline"` + marker + `><xs:simpleType><xs:restriction base="xs:decimal"/></xs:simpleType></xs:attribute>` +
		`<xs:attribute name="gone" type="xs:boolean" use="prohibited"` + marker + `/>`
	root := anonymousGroupedExtensionSchema("1.0", groupBounds, uses)
	root = strings.Replace(root, `<xs:group name="Fields"><xs:sequence/></xs:group>`, `<xs:group name="Fields"><xs:choice/></xs:group>`, 1)
	root = strings.Replace(root, `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType><xs:simpleType name="Named"><xs:restriction base="xs:integer"><xs:minInclusive value="1"/></xs:restriction></xs:simpleType>`, 1)
	return root
}

//nolint:gocognit // Compare every published grouped fact with the omission case.
func TestAnonymousGroupedFalseInheritablePublicFacts(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, spelling := range []struct{ name, marker string }{
			{"omitted", ""}, {"false", ` inheritable="false"`}, {"zero", ` inheritable="0"`},
			{"padded false", ` inheritable=" &#x9;false&#xA; "`}, {"padded zero", ` inheritable=" &#xA;0&#x9; "`},
		} {
			t.Run(string(policy)+"/"+spelling.name, func(t *testing.T) {
				root := anonymousGroupedInheritableSchema(spelling.marker, ` minOccurs="2" maxOccurs="18446744073709551616"`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err != nil {
					t.Fatal(err)
				}
				components := schema.Components()
				if len(components) != 5 || components[0].Name().Local() != "root" || components[1].Name().Local() != "Fields" || components[2].Name().Local() != "global" || components[3].Name().Local() != "Base" || components[4].Name().Local() != "Named" {
					t.Fatalf("walk = %v", components)
				}
				inline := inlineGroupedDefinition(t, schema)
				if inline.Loc() != complexContentTestLoc(t, root, `<xs:complexType>`) || inline.Derivation() != ComplexTypeDerivationExtension || inline.DerivationLoc() != complexContentTestLoc(t, root, `<xs:extension`) || inline.Base() != mustTestQName(t, "urn:root", "Base") || inline.BaseLoc() != complexContentTestLoc(t, root, `base="t:Base"`) {
					t.Fatalf("owner/base = %v", inline)
				}
				base, ok := inline.BaseReference()
				baseID, hasID := base.ComponentID()
				if !ok || !hasID || baseID != components[3].ID() || base.Loc() != inline.BaseLoc() {
					t.Fatalf("base reference = %v", base)
				}
				wildcard, ok := inline.AnyAttribute()
				if !ok || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" || wildcard.Loc() != complexContentTestLoc(t, root, `<xs:anyAttribute`) || wildcard.NamespaceLoc() != complexContentTestLoc(t, root, `namespace="##other"`) || wildcard.ProcessContentsLoc() != complexContentTestLoc(t, root, `processContents="lax"`) {
					t.Fatalf("inherited wildcard = %v", wildcard)
				}
				group, ok := inline.Particle().(ModelGroupReferenceParticle)
				if !ok || group.Ref() != mustTestQName(t, "urn:root", "Fields") || group.TargetID() != components[1].ID() || group.Occurrences().String() != "2/18446744073709551616" || group.Loc() != complexContentTestLoc(t, root, `<xs:group ref="t:Fields"`) || group.RefLoc() != complexContentTestLoc(t, root, `ref="t:Fields"`) {
					t.Fatalf("group = %v", inline.Particle())
				}
				uses := inline.AttributeUses()
				if len(uses) != 2 || uses[0].Name().Local() != "named" || uses[1].Name().Local() != "inline" {
					t.Fatalf("ordered/prohibited uses = %v", uses)
				}
				named, ok := uses[0].(LocalAttributeUse)
				namedID, namedHasID := named.TypeID()
				if !ok || !namedHasID || namedID != components[4].ID() || named.Name() != mustTestQName(t, "urn:root", "named") || named.DeclaredType() != mustTestQName(t, "urn:root", "Named") || named.Loc() != complexContentTestLoc(t, root, `<xs:attribute name="named"`) || named.NameLoc() != complexContentTestLoc(t, root, `name="named"`) || named.TypeLoc() != complexContentTestLoc(t, root, `type="t:Named"`) || named.Use() != AttributeUseRequired || named.UseLoc() != complexContentTestLoc(t, root, `use="required"`) || named.FormLoc() != complexContentTestLoc(t, root, `form="qualified"`) {
					t.Fatalf("named local = %v", uses[0])
				}
				namedType, ok := named.TypeReference()
				bounds, hasBounds := namedType.IntegerBounds()
				minimum, hasMinimum := bounds.MinInclusive()
				minimumLoc, hasMinimumLoc := bounds.MinInclusiveLoc()
				if !ok || !hasBounds || !hasMinimum || minimum.String() != "1" || !hasMinimumLoc || minimumLoc != complexContentTestLoc(t, root, `value="1"`) {
					t.Fatalf("copied named type facets = %v", namedType)
				}
				declarations := bounds.Declarations()
				if len(declarations) != 1 {
					t.Fatalf("bound declarations = %v", declarations)
				}
				declarations[0] = IntegerBoundFacet{}
				copiedBounds, ok := namedType.IntegerBounds()
				copiedMinimum, hasCopiedMinimum := copiedBounds.MinInclusive()
				if !ok || !hasCopiedMinimum || copiedMinimum.String() != "1" {
					t.Fatal("returned bounds changed copied type facts")
				}
				anonymous, ok := uses[1].(LocalAttributeUse)
				if !ok || anonymous.Name() != mustTestQName(t, "", "inline") || anonymous.Loc() != complexContentTestLoc(t, root, `<xs:attribute name="inline"`) || anonymous.NameLoc() != complexContentTestLoc(t, root, `name="inline"`) || anonymous.Use() != AttributeUseOptional || !anonymous.UseLoc().IsZero() {
					t.Fatalf("inline local = %v", uses[1])
				}
				inlineType, hasInlineType := anonymous.TypeReference()
				anonymousID, hasAnonymousID := inlineType.AnonymousID()
				if !hasInlineType || !hasAnonymousID || anonymousID.IsZero() || inlineType.Loc() != complexContentTestLoc(t, root, `<xs:simpleType>`) {
					t.Fatalf("inline type = %v", inlineType)
				}
				before := schema.Components()
				uses[0] = nil
				if inline.AttributeUses()[0] == nil || !reflect.DeepEqual(before, schema.Components()) {
					t.Fatal("returned use or walk slice mutated schema")
				}
				repeat, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err != nil || !reflect.DeepEqual(schema.Components(), repeat.Components()) || !reflect.DeepEqual(inline.AttributeUses(), inlineGroupedDefinition(t, repeat).AttributeUses()) {
					t.Fatalf("query/walk changed on repeat: %v", err)
				}
			})
		}
	}
}

//nolint:gocognit // Check both consumer diagnostics after grouped omission variants.
func TestAnonymousGroupedFalseInheritableZeroAndConsumers(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, bounds := range []string{"", ` minOccurs="0" maxOccurs="0"`} {
			root := anonymousGroupedInheritableSchema(` inheritable="0"`, bounds)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatal(err)
			}
			inline := inlineGroupedDefinition(t, schema)
			if (inline.Particle() == nil) != (bounds != "") || len(inline.AttributeUses()) != 2 {
				t.Fatalf("zero/group facts = %v/%v", inline.Particle(), inline.AttributeUses())
			}
			validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
			vd := requireDiagnostic(t, validationErr)
			instanceLoc, err := NewLoc("instance.xml", 1, 1)
			if err != nil {
				t.Fatal(err)
			}
			if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != instanceLoc || vd.SpecRef() != instanceValidationSpecRef(XSDVersion11) || !errors.Is(validationErr, ErrUnsupported) || !errors.Is(validationErr, errInstanceComplexContentExtension) {
				t.Fatalf("validation = %v related=%v", validationErr, vd.Related())
			}
			wantRelated := []Loc{
				complexContentTestLoc(t, root, `<xs:element name="root"`),
				complexContentTestLoc(t, root, `<xs:complexType>`),
				complexContentTestLoc(t, root, `<xs:complexContent>`),
				complexContentTestLoc(t, root, `<xs:extension`),
				complexContentTestLoc(t, root, `base="t:Base"`),
			}
			if bounds == "" {
				wantRelated = append(wantRelated, complexContentTestLoc(t, root, `<xs:group ref="t:Fields"`))
			}
			wantRelated = append(wantRelated, complexContentTestLoc(t, root, `<xs:anyAttribute`))
			if !reflect.DeepEqual(vd.Related(), wantRelated) {
				t.Fatalf("validation related = %v, want %v", vd.Related(), wantRelated)
			}
			output, generationErr := GenerateGo(schema, "generated")
			gd := requireDiagnostic(t, generationErr)
			if output != nil || gd.Class() != FailureUnsupported || gd.Code() != diagnosticCodegenUnsupported || gd.Loc() != complexContentTestLoc(t, root, `<xs:element name="root"`) || gd.SpecRef() != schemaElementTypeSpecRef(XSDVersion11) || !errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenUnsupported) {
				t.Fatalf("generation = %q/%v", output, generationErr)
			}
		}
	}
}

func TestAnonymousGroupedFalseInheritableExcludedUses(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, tc := range []struct {
			name, use, extra, marker, spec string
			cause                          error
		}{
			{"built-in ID", `<xs:attribute name="id" type="xs:ID" inheritable="false"/>`, "", `type="xs:ID"`, schemaAttributeTypeSpecRef(XSDVersion11), errSchemaAttributeUseUnsupported},
			{"named ID", `<xs:attribute name="id" type="t:NamedID" inheritable="0"/>`, `<xs:simpleType name="NamedID"><xs:restriction base="xs:ID"/></xs:simpleType>`, `type="t:NamedID"`, schemaAttributeTypeSpecRef(XSDVersion11), errSchemaAttributeUseUnsupported},
			{"inline ID", `<xs:attribute name="id" inheritable="false"><xs:simpleType><xs:restriction base="xs:ID"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType>`, schemaAttributeTypeSpecRef(XSDVersion11), errSchemaAttributeUseUnsupported},
			{"attribute ref", `<xs:attribute ref="t:global" inheritable="0"/>`, "", `inheritable="0"`, schemaComplexTypeExtensionSpecRef(XSDVersion11), ErrUnsupported},
			{"inline list", `<xs:attribute name="list" inheritable="false"><xs:simpleType><xs:list itemType="xs:boolean"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType>`, schemaAttributeUseSpecRef(XSDVersion11), errSchemaAttributeUseUnsupported},
		} {
			t.Run(string(policy)+"/"+tc.name, func(t *testing.T) {
				root := anonymousGroupedExtensionSchema("1.0", ` minOccurs="0" maxOccurs="0"`, tc.use)
				root = strings.Replace(root, `</xs:schema>`, tc.extra+`</xs:schema>`, 1)
				if tc.name == "inline list" {
					assertInlineInheritableDiagnostic(t, root, policy, tc.marker, FailureUnsupported, UnsupportedSchemaSyntaxCode, tc.spec, tc.cause, `<xs:list`)
					return
				}
				assertInlineInheritableDiagnostic(t, root, policy, tc.marker, FailureUnsupported, UnsupportedSchemaSyntaxCode, tc.spec, tc.cause)
			})
		}
	}
}

func TestAnonymousGroupedFalseInheritableMalformed(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		version := XSDVersion11
		if policy == Strict10 {
			version = XSDVersion10
		}
		root := anonymousGroupedExtensionSchema("1.0", "", `<xs:attribute name="a" type="xs:boolean" inheritable="maybe"/>`)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
		assertZeroSchema(t, schema)
		d := requireDiagnostic(t, err)
		var lexical Diagnostic
		if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.Loc() != complexContentTestLoc(t, root, `inheritable="maybe"`) || d.SpecRef() != strictBooleanSpecRef(version) || len(d.Related()) != 0 || !errors.As(errors.Unwrap(d), &lexical) || lexical.Code() != InvalidBooleanLexicalCode || lexical.Loc() != d.Loc() || lexical.SpecRef() != d.SpecRef() {
			t.Fatalf("malformed = %v related=%v", err, d.Related())
		}
	}
}

func TestAnonymousGroupedFalseInheritableBoundsBeforeOmission(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		root := anonymousGroupedExtensionSchema("1.0", ` minOccurs="2" maxOccurs="1"`, `<xs:attribute name="a" type="xs:boolean" inheritable="false"/>`)
		assertInlineInheritableDiagnostic(t, root, policy, `<xs:group ref="t:Fields"`, FailureInvalid, invalidSchemaCompositionCode, schemaParticleCorrectSpecRef(XSDVersion11), errParticleOccurrenceMinimumExceedsMaximum, `minOccurs="2"`, `maxOccurs="1"`)
		zero := anonymousGroupedExtensionSchema("1.0", ` minOccurs="0" maxOccurs="0"`, `<xs:attribute name="a" type="xs:boolean" inheritable="0"/>`)
		zero = strings.Replace(zero, `ref="t:Fields"`, `ref="t:Missing"`, 1)
		assertInlineInheritableDiagnostic(t, zero, policy, `ref="t:Missing"`, FailureInvalid, diagnosticSchemaModelGroupReferenceUnresolvedCode, schemaModelGroupReferenceSpecRef(XSDVersion11), errSchemaModelGroupReferenceUnresolved)
	}
}
