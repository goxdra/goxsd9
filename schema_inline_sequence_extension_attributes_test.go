package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func inlineSequenceAttributeSchema(version, outer, members, uses, base string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" version="` + version + `" elementFormDefault="qualified" attributeFormDefault="unqualified">
  <xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:Base"><xs:sequence` + outer + `>` + members + `</xs:sequence>` + uses + `</xs:extension></xs:complexContent></xs:complexType></xs:element>
  <xs:element name="target" type="xs:integer"/>
  <xs:attribute name="global" type="xs:boolean"/>
  <xs:complexType name="Base">` + base + `</xs:complexType>
  <xs:simpleType name="Named"><xs:restriction base="xs:boolean"/></xs:simpleType>
</xs:schema>`
}

func inlineSequenceExtension(t *testing.T, schema Schema) ComplexTypeDefinition {
	t.Helper()
	components := schema.Components()
	if len(components) != 5 || components[0].Name().Local() != "root" || components[1].Name().Local() != "target" || components[2].Name().Local() != "global" || components[3].Name().Local() != "Base" || components[4].Name().Local() != "Named" {
		t.Fatalf("component order = %v", components)
	}
	root, ok := components[0].Element()
	if !ok {
		t.Fatal("root element absent")
	}
	inline, ok := root.InlineComplexType()
	if !ok || !inline.ID().IsZero() || !inline.Name().IsZero() {
		t.Fatalf("inline identity = %v/%t", inline, ok)
	}
	id, ok := inline.NodeID()
	if !ok || id.IsZero() {
		t.Fatal("inline node identity absent")
	}
	for _, component := range components {
		if component.ID() == inline.ID() {
			t.Fatal("anonymous owner entered global component walk")
		}
	}
	return inline
}

//nolint:gocognit // Compare each retained public fact with its lexical source.
func TestInlineSequenceExtensionAttributePublicFacts(t *testing.T) {
	members := `<xs:element name="local" type="xs:integer" minOccurs="2" maxOccurs="3"/><xs:element ref="t:target" minOccurs="0" maxOccurs="4"/>`
	uses := `<xs:attribute name="first" type="t:Named" use="required" form="qualified"/><xs:attribute ref="t:global" use="optional"/><xs:attribute name="gone" type="xs:boolean" use="prohibited"/>`
	base := `<xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent>`
	for _, profile := range []struct {
		name, version string
		policy        LanguagePolicy
	}{
		{"compat10", "1.0", Compatibility}, {"compat11", "1.1", Compatibility},
		{"strict10", "1.1", Strict10}, {"strict11", "1.0", Strict11},
	} {
		t.Run(profile.name, func(t *testing.T) {
			root := inlineSequenceAttributeSchema(profile.version, ` minOccurs="2" maxOccurs="5"`, members, uses, base)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			inline := inlineSequenceExtension(t, schema)
			if inline.Loc() != complexContentTestLoc(t, root, `<xs:complexType>`) || inline.Derivation() != ComplexTypeDerivationExtension || inline.DerivationLoc() != complexContentTestLoc(t, root, `<xs:extension`) || inline.Base() != mustTestQName(t, "urn:root", "Base") || inline.BaseLoc() != complexContentTestLoc(t, root, `base="t:Base"`) {
				t.Fatalf("inline derivation = %v", inline)
			}
			baseRef, ok := inline.BaseReference()
			baseID, hasID := baseRef.ComponentID()
			if !ok || !hasID || baseID != schema.Components()[3].ID() || baseRef.Loc() != inline.BaseLoc() {
				t.Fatalf("base ref = %v", baseRef)
			}
			wildcard, ok := inline.AnyAttribute()
			if !ok || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" || wildcard.Loc() != complexContentTestLoc(t, root, `<xs:anyAttribute`) || wildcard.NamespaceLoc() != complexContentTestLoc(t, root, `namespace="##other"`) || wildcard.ProcessContentsLoc() != complexContentTestLoc(t, root, `processContents="lax"`) {
				t.Fatalf("wildcard = %v", wildcard)
			}
			sequence, ok := inline.Particle().(SequenceParticle)
			if !ok || sequence.Occurrences().String() != "2/5" || sequence.Loc() != complexContentTestLoc(t, root, `<xs:sequence`) {
				t.Fatalf("sequence = %v", inline.Particle())
			}
			terms := sequence.Particles()
			if len(terms) != 2 || terms[0].Occurrences().String() != "2/3" || terms[1].Occurrences().String() != "0/4" {
				t.Fatalf("terms = %v", terms)
			}
			local, ok := terms[0].(ElementParticle)
			if !ok || local.Name() != mustTestQName(t, "urn:root", "local") || local.DeclaredType() != mustTestQName(t, testXSDNamespace, "integer") || local.Loc() != complexContentTestLoc(t, root, `<xs:element name="local"`) {
				t.Fatalf("local = %v", terms[0])
			}
			ref, ok := terms[1].(ElementReferenceParticle)
			if !ok || ref.Ref() != mustTestQName(t, "urn:root", "target") || ref.TargetID() != schema.Components()[1].ID() || ref.RefLoc() != complexContentTestLoc(t, root, `ref="t:target"`) {
				t.Fatalf("ref = %v", terms[1])
			}
			attributeUses := inline.AttributeUses()
			if len(attributeUses) != 2 {
				t.Fatalf("uses = %v", attributeUses)
			}
			first, ok := attributeUses[0].(LocalAttributeUse)
			if !ok || first.Name() != mustTestQName(t, "urn:root", "first") || first.DeclaredType() != mustTestQName(t, "urn:root", "Named") || first.Use() != AttributeUseRequired || first.Loc() != complexContentTestLoc(t, root, `<xs:attribute name="first"`) || first.NameLoc() != complexContentTestLoc(t, root, `name="first"`) || first.TypeLoc() != complexContentTestLoc(t, root, `type="t:Named"`) || first.UseLoc() != complexContentTestLoc(t, root, `use="required"`) {
				t.Fatalf("first = %v", attributeUses[0])
			}
			typeID, ok := first.TypeID()
			if !ok || typeID != schema.Components()[4].ID() {
				t.Fatalf("first type = %v/%t", typeID, ok)
			}
			second, ok := attributeUses[1].(AttributeReferenceUse)
			if !ok || second.Ref() != mustTestQName(t, "urn:root", "global") || second.TargetID() != schema.Components()[2].ID() || second.RefLoc() != complexContentTestLoc(t, root, `ref="t:global"`) || second.UseLoc() != complexContentTestLoc(t, root, `use="optional"`) {
				t.Fatalf("second = %v", attributeUses[1])
			}
			attributeUses[0] = nil
			if len(inline.AttributeUses()) != 2 || inline.AttributeUses()[0] == nil {
				t.Fatal("returned use view mutated schema")
			}
			repeated, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil || !reflect.DeepEqual(schema.Components(), repeated.Components()) {
				t.Fatalf("repeat = %v: %v", repeated.Components(), err)
			}
		})
	}
}

//nolint:gocognit // Each failure is a separate exit from the newly admitted boundary.
func TestInlineSequenceExtensionErrorOrderAndExits(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		version := XSDVersion11
		if policy == Strict10 {
			version = XSDVersion10
		}
		for _, tc := range []struct {
			name, members, uses, base, marker, code, spec string
			class                                         FailureClass
			cause                                         error
			related                                       []string
		}{
			{"child before use", `<xs:element ref="t:missing"/>`, `<xs:attribute name="a" type="t:missing"/>`, "Base", `ref="t:missing"`, diagnosticSchemaElementReferenceUnresolvedCode, schemaElementReferenceSpecRef(version), FailureInvalid, errSchemaElementReferenceUnresolved, nil},
			{"use before base", ``, `<xs:attribute name="a" type="t:missing" use="prohibited"/>`, "missing", `type="t:missing"`, diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(version), FailureInvalid, errSchemaAttributeTypeUnresolved, nil},
			{"unresolved base", ``, `<xs:attribute name="a" type="xs:boolean"/>`, "missing", `base="t:missing"`, invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(version), FailureInvalid, errSchemaComplexTypeBaseUnresolved, nil},
			{"wrong kind base", ``, `<xs:attribute name="a" type="xs:boolean"/>`, "Named", `base="t:Named"`, invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(version), FailureInvalid, errSchemaComplexTypeBaseWrongKind, []string{`<xs:simpleType name="Named"`}},
			{"duplicate use", ``, `<xs:attribute name="a" type="xs:boolean"/><xs:attribute name="a" type="xs:boolean" use="required"/>`, "Base", `<xs:attribute name="a" type="xs:boolean" use="required"`, diagnosticSchemaAttributeUseDuplicateCode, schemaAttributeUseSpecRef(version), FailureInvalid, errSchemaAttributeUseDuplicate, []string{`<xs:attribute name="a" type="xs:boolean"/>`}},
			{"nonempty base", ``, `<xs:attribute name="a" type="xs:boolean"/>`, "Base", `base="t:Base"`, UnsupportedSchemaSyntaxCode, schemaComplexTypeExtensionSpecRef(version), FailureUnsupported, errSchemaComplexTypeBaseNonEmpty, []string{`<xs:sequence><xs:element name="inBase"`}},
		} {
			t.Run(string(policy)+"/"+tc.name, func(t *testing.T) {
				base := ``
				if tc.name == "nonempty base" {
					base = `<xs:sequence><xs:element name="inBase" type="xs:integer"/></xs:sequence>`
				}
				root := inlineSequenceAttributeSchema("1.0", "", tc.members, tc.uses, base)
				root = strings.Replace(root, `base="t:Base"`, `base="t:`+tc.base+`"`, 1)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err == nil {
					t.Fatal("bad schema passed")
				}
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != tc.class || d.Code() != tc.code || d.SpecRef() != tc.spec || !errors.Is(err, tc.cause) {
					t.Fatalf("diagnostic = %v related %v", d, d.Related())
				}
				wantLoc := complexContentTestLoc(t, root, tc.marker)
				if d.Loc() != wantLoc {
					t.Fatalf("loc = %v want %v", d.Loc(), wantLoc)
				}
				for _, marker := range tc.related {
					if !slices.Contains(d.Related(), complexContentTestLoc(t, root, marker)) {
						t.Fatalf("related = %v missing %s", d.Related(), marker)
					}
				}
			})
		}
	}
}

//nolint:gocognit // Assert both consumer exits for each surviving/omitted shape.
func TestInlineSequenceExtensionConsumers(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		version := XSDVersion11
		if policy == Strict10 {
			version = XSDVersion10
		}
		for _, tc := range []struct{ name, outer, uses string }{
			{"surviving sequence and use", "", `<xs:attribute name="flag" type="xs:boolean"/>`},
			{"zero sequence and use", ` minOccurs="0" maxOccurs="0"`, `<xs:attribute name="flag" type="xs:boolean"/>`},
			{"zero sequence and prohibited use", ` minOccurs="0" maxOccurs="0"`, `<xs:attribute name="gone" type="xs:boolean" use="prohibited"/>`},
		} {
			t.Run(string(policy)+"/"+tc.name, func(t *testing.T) {
				root := inlineSequenceAttributeSchema("1.1", tc.outer, `<xs:element ref="t:target"/>`, tc.uses, ``)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err != nil {
					t.Fatal(err)
				}
				inline := inlineSequenceExtension(t, schema)
				if tc.outer != "" && inline.Particle() != nil {
					t.Fatalf("zero sequence survived: %v", inline.Particle())
				}
				if strings.Contains(tc.uses, "prohibited") && len(inline.AttributeUses()) != 0 {
					t.Fatalf("prohibited use survived: %v", inline.AttributeUses())
				}
				validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
				if validationErr == nil {
					t.Fatal("validation accepted extension")
				}
				vd := requireDiagnostic(t, validationErr)
				instanceLoc, locErr := NewLoc("instance.xml", 1, 1)
				if locErr != nil {
					t.Fatal(locErr)
				}
				if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != instanceLoc || vd.SpecRef() != instanceValidationSpecRef(version) || !slices.Contains(vd.Related(), complexContentTestLoc(t, root, `<xs:extension`)) || !errors.Is(validationErr, ErrUnsupported) || !errors.Is(validationErr, errInstanceComplexContentExtension) {
					t.Fatalf("validation = %v", validationErr)
				}
				wantValidationRelated := []Loc{
					complexContentTestLoc(t, root, `<xs:element name="root"`),
					complexContentTestLoc(t, root, `<xs:complexType>`),
					complexContentTestLoc(t, root, `<xs:complexContent>`),
					complexContentTestLoc(t, root, `<xs:extension`),
					complexContentTestLoc(t, root, `base="t:Base"`),
				}
				if tc.outer == "" {
					wantValidationRelated = append(wantValidationRelated, complexContentTestLoc(t, root, `<xs:sequence`))
				}
				if !reflect.DeepEqual(vd.Related(), wantValidationRelated) {
					t.Fatalf("validation related = %v, want %v", vd.Related(), wantValidationRelated)
				}
				output, generationErr := GenerateGo(schema, "generated")
				if generationErr == nil || output != nil {
					t.Fatalf("generation = %q/%v", output, generationErr)
				}
				gd := requireDiagnostic(t, generationErr)
				if gd.Class() != FailureUnsupported || gd.Code() != diagnosticCodegenUnsupported || gd.Loc() != complexContentTestLoc(t, root, `<xs:element name="root"`) || gd.SpecRef() != schemaElementTypeSpecRef(version) || !errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenUnsupported) {
					t.Fatalf("generation = %v", generationErr)
				}
				wantGenerationRelated := []Loc{complexContentTestLoc(t, root, `<xs:complexType>`), complexContentTestLoc(t, root, `<xs:extension`), complexContentTestLoc(t, root, `base="t:Base"`)}
				if !reflect.DeepEqual(gd.Related(), wantGenerationRelated) {
					t.Fatalf("generation related = %v, want %v", gd.Related(), wantGenerationRelated)
				}
			})
		}
	}
}

//nolint:gocognit // Every graph form must retain the base identity and source locations.
func TestInlineSequenceExtensionAttributeGraphOwnership(t *testing.T) {
	for _, graph := range []string{"forward", "included", "imported", "chameleon"} {
		for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
			t.Run(graph+"/"+string(policy), func(t *testing.T) {
				fixture := emptyComplexContentExtensionGraph(t, "1.0", graph)
				fixture.root = strings.Replace(fixture.root,
					`<xs:complexType name="Derived"><xs:complexContent><xs:extension base="`+fixture.baseLexical+`"/></xs:complexContent></xs:complexType>`,
					`<xs:element name="Derived"><xs:complexType><xs:complexContent><xs:extension base="`+fixture.baseLexical+`"><xs:sequence><xs:element name="child" type="xs:integer"/></xs:sequence><xs:attribute name="flag" type="xs:boolean"/></xs:extension></xs:complexContent></xs:complexType></xs:element>`, 1)
				schema, err := discoverTestSchemaWithPolicy(t, fixture.root, fixture.fixtures, policy)
				if err != nil {
					t.Fatal(err)
				}
				components := schema.Components()
				if len(components) != 2 || components[0].Document() != "root.xsd" || components[1].Document() != fixture.baseSource {
					t.Fatalf("components = %v", components)
				}
				owner, ok := components[0].Element()
				if !ok {
					t.Fatal("root element absent")
				}
				inline, ok := owner.InlineComplexType()
				if !ok || !inline.ID().IsZero() || inline.Base() != mustTestQName(t, fixture.baseNamespace, "Base") || inline.BaseLoc() != complexContentTestLoc(t, fixture.root, `base="`+fixture.baseLexical+`"`) {
					t.Fatalf("inline base = %v/%t", inline, ok)
				}
				baseRef, ok := inline.BaseReference()
				id, hasID := baseRef.ComponentID()
				if !ok || !hasID || id != components[1].ID() {
					t.Fatalf("base reference = %v", baseRef)
				}
				wildcard, ok := inline.AnyAttribute()
				if !ok || wildcard.Loc() != complexContentTestSourceLoc(t, fixture.baseSource, fixture.baseContents, `<xs:anyAttribute`) {
					t.Fatalf("wildcard = %v/%t", wildcard, ok)
				}
				if len(inline.AttributeUses()) != 1 || inline.AttributeUses()[0].Name().Local() != "flag" {
					t.Fatalf("uses = %v", inline.AttributeUses())
				}
				repeated, err := discoverTestSchemaWithPolicy(t, fixture.root, fixture.fixtures, policy)
				if err != nil || !reflect.DeepEqual(schema.Components(), repeated.Components()) {
					t.Fatalf("repeat = %v: %v", repeated.Components(), err)
				}
			})
		}
	}
}

//nolint:gocognit // Pair each omitted-use semantic exit with policy and related locations.
func TestInlineSequenceExtensionProhibitedSemanticChecks(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		version := XSDVersion11
		if policy == Strict10 {
			version = XSDVersion10
		}
		for _, tc := range []struct {
			name, members, uses, marker, code, spec string
			cause                                   error
		}{
			{"zero child", `<xs:element ref="t:missing"/>`, `<xs:attribute name="gone" type="xs:boolean" use="prohibited"/>`, `ref="t:missing"`, diagnosticSchemaElementReferenceUnresolvedCode, schemaElementReferenceSpecRef(version), errSchemaElementReferenceUnresolved},
			{"wrong kind child", `<xs:element ref="t:global"/>`, `<xs:attribute name="gone" type="xs:boolean" use="prohibited"/>`, `ref="t:global"`, diagnosticSchemaElementReferenceWrongKindCode, schemaElementReferenceSpecRef(version), errSchemaElementReferenceWrongKind},
			{"prohibited local", `<xs:element ref="t:target"/>`, `<xs:attribute name="gone" type="t:missing" use="prohibited"/>`, `type="t:missing"`, diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(version), errSchemaAttributeTypeUnresolved},
			{"prohibited ref", `<xs:element ref="t:target"/>`, `<xs:attribute ref="t:missing" use="prohibited"/>`, `ref="t:missing"`, diagnosticSchemaAttributeReferenceUnresolvedCode, schemaAttributeUseResolveSpecRef(version), errSchemaAttributeReferenceUnresolved},
			{"wrong kind ref", `<xs:element name="v" type="xs:integer"/>`, `<xs:attribute ref="t:target" use="prohibited"/>`, `ref="t:target"`, diagnosticSchemaAttributeReferenceWrongKindCode, schemaAttributeUseResolveSpecRef(version), errSchemaAttributeReferenceWrongKind},
		} {
			t.Run(string(policy)+"/"+tc.name, func(t *testing.T) {
				root := inlineSequenceAttributeSchema("1.0", ` minOccurs="0" maxOccurs="0"`, tc.members, tc.uses, ``)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err == nil {
					t.Fatal("semantic failure omitted")
				}
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != FailureInvalid || d.Code() != tc.code || d.Loc() != complexContentTestLoc(t, root, tc.marker) || d.SpecRef() != tc.spec || !errors.Is(err, tc.cause) {
					t.Fatalf("diagnostic = %v got %s/%s/%s/%s cause=%v want %s/%s/%s/%s cause=%v", err, d.Class(), d.Code(), d.Loc(), d.SpecRef(), errors.Is(err, tc.cause), FailureInvalid, tc.code, complexContentTestLoc(t, root, tc.marker), tc.spec, tc.cause)
				}
				if tc.name == "wrong kind child" && !slices.Contains(d.Related(), complexContentTestLoc(t, root, `<xs:attribute name="global"`)) {
					t.Fatalf("wrong-kind child related = %v", d.Related())
				}
				if tc.name == "wrong kind ref" && !slices.Contains(d.Related(), complexContentTestLoc(t, root, `<xs:element name="target"`)) {
					t.Fatalf("wrong-kind reference related = %v", d.Related())
				}
			})
		}
	}
}

//nolint:gocognit // The inline-owner exclusions must survive even when the use is prohibited.
func TestInlineSequenceExtensionExcludedShapes(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		version := XSDVersion11
		if policy == Strict10 {
			version = XSDVersion10
		}
		for _, tc := range []struct {
			name, members, uses, marker, spec string
			cause                             error
		}{
			{"built-in ID", ``, `<xs:attribute name="id" type="xs:ID" use="prohibited"/>`, `type="xs:ID"`, schemaAttributeTypeSpecRef(version), errSchemaAttributeUseUnsupported},
			{"named ID", ``, `<xs:attribute name="id" type="t:NamedID"/>`, `type="t:NamedID"`, schemaAttributeTypeSpecRef(version), errSchemaAttributeUseUnsupported},
			{"inline ID", ``, `<xs:attribute name="id"><xs:simpleType><xs:restriction base="xs:ID"/></xs:simpleType></xs:attribute>`, `<xs:simpleType>`, schemaAttributeTypeSpecRef(version), errSchemaAttributeUseUnsupported},
			{"ID reference", ``, `<xs:attribute ref="t:globalID" use="prohibited"/>`, `ref="t:globalID"`, schemaAttributeUseSpecRef(version), errSchemaAttributeReferenceUnsupported},
			{"nested model", `<xs:sequence><xs:element name="v" type="xs:integer"/></xs:sequence>`, `<xs:attribute name="flag" type="xs:boolean"/>`, `<xs:sequence><xs:element`, schemaComplexParticleExtensionSpecRef(version), ErrUnsupported},
			{"grouped extension", ``, `<xs:attribute name="flag" type="xs:boolean"/>`, `<xs:extension`, schemaComplexContentExtensionSpecRef(version), errSchemaGroupedExtensionAnonymousOwner},
			{"direct anyAttribute", ``, `<xs:attribute name="flag" type="xs:boolean"/><xs:anyAttribute namespace="##other" processContents="lax"/>`, `<xs:anyAttribute`, schemaComplexTypeExtensionSpecRef(version), ErrUnsupported},
		} {
			t.Run(string(policy)+"/"+tc.name, func(t *testing.T) {
				root := inlineSequenceAttributeSchema("1.0", "", tc.members, tc.uses, ``)
				root = strings.Replace(root, `<xs:simpleType name="Named">`, `<xs:attribute name="globalID" type="xs:ID"/><xs:simpleType name="NamedID"><xs:restriction base="xs:ID"/></xs:simpleType><xs:simpleType name="Named">`, 1)
				if tc.name == "grouped extension" {
					root = strings.Replace(root, `<xs:sequence></xs:sequence>`, `<xs:group ref="t:Fields"/>`, 1)
					root = strings.Replace(root, `</xs:schema>`, `<xs:group name="Fields"><xs:sequence/></xs:group></xs:schema>`, 1)
				}
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err == nil {
					t.Fatal("excluded inline extension passed")
				}
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != complexContentTestLoc(t, root, tc.marker) || d.SpecRef() != tc.spec || !errors.Is(err, ErrUnsupported) || !errors.Is(err, tc.cause) {
					t.Fatalf("diagnostic = %v related=%v spec=%s want %s/%s", err, d.Related(), d.SpecRef(), tc.marker, tc.spec)
				}
			})
		}
	}
}

func TestInlineSequenceExtensionLocalVarietyExclusions(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, tc := range []struct{ name, use, extra, marker string }{
			{"inline list", `<xs:attribute name="a" use="prohibited"><xs:simpleType><xs:list itemType="xs:precisionDecimal"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType><xs:list`},
			{"inline union", `<xs:attribute name="a"><xs:simpleType><xs:union memberTypes="xs:precisionDecimal xs:negativeInteger"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType><xs:union`},
			{"named list", `<xs:attribute name="a" type="t:List"/>`, `<xs:simpleType name="List"><xs:list itemType="xs:precisionDecimal"/></xs:simpleType>`, `type="t:List"`},
			{"named union", `<xs:attribute name="a" type="t:Union" use="prohibited"/>`, `<xs:simpleType name="Union"><xs:union memberTypes="xs:precisionDecimal xs:negativeInteger"/></xs:simpleType>`, `type="t:Union"`},
		} {
			t.Run(string(policy)+"/"+tc.name, func(t *testing.T) {
				root := inlineSequenceAttributeSchema("1.1", ` minOccurs="0" maxOccurs="0"`, ``, tc.use, ``)
				root = strings.Replace(root, `</xs:schema>`, tc.extra+`</xs:schema>`, 1)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err == nil {
					t.Fatal("local variety passed")
				}
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != complexContentTestLoc(t, root, tc.marker) || d.SpecRef() != schemaAttributeUseSpecRef(XSDVersion11) || !errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaAttributeUseUnsupported) {
					t.Fatalf("diagnostic = %v related=%v", err, d.Related())
				}
			})
		}
	}
}

func TestInlineSequenceExtensionBaseGraphFailureExits(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:b="urn:base" targetNamespace="urn:root" version="1.1"><xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/><xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="b:Base"><xs:sequence/><xs:attribute name="flag" type="xs:boolean"/></xs:extension></xs:complexContent></xs:complexType></xs:element></xs:schema>`
	bridge := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge" version="1.1"><xs:import namespace="urn:base" schemaLocation="base.xsd"/></xs:schema>`
	base := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:base" version="1.1"><xs:complexType name="Base"/></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
		"bridge.xsd": {id: "bridge.xsd", contents: bridge}, "base.xsd": {id: "base.xsd", contents: base},
	}, Strict11)
	if err == nil {
		t.Fatal("invisible base passed")
	}
	assertZeroSchema(t, schema)
	d := requireDiagnostic(t, err)
	if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.Loc() != complexContentTestLoc(t, root, `base="b:Base"`) || d.SpecRef() != schemaComplexTypeExtensionSpecRef(XSDVersion11) || !errors.Is(err, errSchemaComplexTypeBaseUnresolved) || len(d.Related()) == 0 || d.Related()[0].Source() != "base.xsd" {
		t.Fatalf("invisible base = %v related=%v", err, d.Related())
	}

	ambiguous := strings.Replace(root, `<xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/>`, `<xs:import namespace="urn:base" schemaLocation="a.xsd"/><xs:import namespace="urn:base" schemaLocation="b.xsd"/>`, 1)
	schema, err = discoverTestSchemaWithPolicy(t, ambiguous, map[string]discoveryFixture{
		"a.xsd": {id: "a.xsd", contents: base}, "b.xsd": {id: "b.xsd", contents: base},
	}, Strict11)
	if err == nil {
		t.Fatal("ambiguous base passed")
	}
	assertZeroSchema(t, schema)
	d = requireDiagnostic(t, err)
	if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaGlobalDuplicateCode || d.Loc() != complexContentTestSourceLoc(t, "b.xsd", base, `<xs:complexType name="Base"`) || d.SpecRef() != schemaGlobalDuplicateSpecRef(XSDVersion11) || !errors.Is(err, errSchemaGlobalDeclarationDuplicate) || !reflect.DeepEqual(d.Related(), []Loc{complexContentTestSourceLoc(t, "a.xsd", base, `<xs:complexType name="Base"`)}) {
		t.Fatalf("duplicate base declaration = %v related=%v", err, d.Related())
	}

	cycle := inlineSequenceAttributeSchema("1.1", "", ``, `<xs:attribute name="flag" type="xs:boolean"/>`, ``)
	cycle = strings.Replace(cycle, `<xs:complexType name="Base"></xs:complexType>`, `<xs:complexType name="Base"><xs:complexContent><xs:extension base="t:Other"/></xs:complexContent></xs:complexType><xs:complexType name="Other"><xs:complexContent><xs:extension base="t:Base"/></xs:complexContent></xs:complexType>`, 1)
	schema, err = discoverTestSchemaWithPolicy(t, cycle, nil, Strict11)
	if err == nil {
		t.Fatal("base cycle passed")
	}
	assertZeroSchema(t, schema)
	d = requireDiagnostic(t, err)
	if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.SpecRef() != schemaComplexTypeExtensionSpecRef(XSDVersion11) || !errors.Is(err, errSchemaComplexTypeBaseCycle) || len(d.Related()) == 0 {
		t.Fatalf("base cycle = %v related=%v", err, d.Related())
	}
}

func TestInlineSequenceExtensionInvalidFormLocation(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		root := inlineSequenceAttributeSchema("1.1", "", ``, `<xs:attribute name="flag" type="xs:boolean" form="neither"/>`, ``)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
		if err == nil {
			t.Fatal("invalid form passed")
		}
		assertZeroSchema(t, schema)
		d := requireDiagnostic(t, err)
		if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.Loc() != complexContentTestLoc(t, root, `form="neither"`) || len(d.Related()) != 0 {
			t.Fatalf("form diagnostic = %v related=%v", err, d.Related())
		}
	}
}

func TestInlineSequenceExtensionAtomicInlineUseIdentity(t *testing.T) {
	root := inlineSequenceAttributeSchema("1.1", "", ``, `<xs:attribute name="inline" use="required"><xs:simpleType><xs:restriction base="xs:boolean"/></xs:simpleType></xs:attribute><xs:attribute name="builtIn" type="xs:decimal"/>`, ``)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatal(err)
	}
	inline := inlineSequenceExtension(t, schema)
	uses := inline.AttributeUses()
	if len(uses) != 2 {
		t.Fatalf("uses = %v", uses)
	}
	first, ok := uses[0].(LocalAttributeUse)
	if !ok || first.Name() != mustTestQName(t, "", "inline") || first.TypeLoc() != complexContentTestLoc(t, root, `<xs:simpleType>`) || first.UseLoc() != complexContentTestLoc(t, root, `use="required"`) {
		t.Fatalf("inline use = %v", uses[0])
	}
	reference, ok := first.TypeReference()
	id, hasID := reference.AnonymousID()
	if !ok || !reference.IsAnonymous() || !hasID || id.IsZero() || reference.Loc() != first.TypeLoc() {
		t.Fatalf("inline reference = %v", reference)
	}
	second, ok := uses[1].(LocalAttributeUse)
	if !ok || second.DeclaredType() != mustTestQName(t, testXSDNamespace, "decimal") || second.TypeLoc() != complexContentTestLoc(t, root, `type="xs:decimal"`) || second.Name() != mustTestQName(t, "", "builtIn") {
		t.Fatalf("built-in use = %v", uses[1])
	}
	builtIn, ok := second.TypeReference()
	if !ok || !builtIn.IsBuiltin() || builtIn.Name() != second.DeclaredType() {
		t.Fatalf("built-in reference = %v", builtIn)
	}
}

func TestInlineSequenceExtensionFacetExit(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		version := XSDVersion11
		if policy == Strict10 {
			version = XSDVersion10
		}
		root := inlineSequenceAttributeSchema("1.0", ` minOccurs="0" maxOccurs="0"`, ``, `<xs:attribute name="flag" use="prohibited"><xs:simpleType><xs:restriction base="xs:boolean"><xs:pattern value="true"/></xs:restriction></xs:simpleType></xs:attribute>`, ``)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
		if err == nil {
			t.Fatal("unsupported facet omitted")
		}
		assertZeroSchema(t, schema)
		d := requireDiagnostic(t, err)
		wantSpec := "xsd11-datatypes#decimal"
		if version == XSDVersion10 {
			wantSpec = "xsd10-datatypes#decimal"
		}
		if d.Class() != FailureUnsupported || d.Feature() != FeatureDatatypeFacets || d.Code() != UnsupportedDatatypeFacetCode || d.Loc() != complexContentTestLoc(t, root, `<xs:pattern`) || d.SpecRef() != wantSpec || !errors.Is(err, ErrUnsupported) {
			t.Fatalf("facet diagnostic = %v related=%v", err, d.Related())
		}
	}
}
