package goxsd9

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func inlineTokenURIExtensionSchema(version, member, uses string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:token-uri" targetNamespace="urn:token-uri" version="` + version + `" attributeFormDefault="qualified">` +
		`<xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:Base"><xs:sequence minOccurs="2" maxOccurs="4">` + member + `</xs:sequence>` + uses +
		`</xs:extension></xs:complexContent></xs:complexType></xs:element>` +
		`<xs:complexType name="Base"/><xs:element name="child" type="xs:integer"/><xs:attribute name="global" type="xs:boolean"/>` +
		`<xs:simpleType name="NamedToken"><xs:restriction base="xs:token"/></xs:simpleType></xs:schema>`
}

//nolint:gocognit // Visibility, duplicate candidates, cycles, and acquisition keep separate exits.
func TestInlineExtensionTokenAnyURIGraphFailures(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "anyURI"} {
			root := inlineTokenURIExtensionSchema("1.0", `<xs:element ref="t:child"/>`, `<xs:attribute name="value" type="xs:`+kind+`"/>`)
			t.Run(profile.name+"/"+kind+"/invisible", func(t *testing.T) {
				invisible := strings.Replace(root, `<xs:complexType name="Base"/>`, "", 1)
				invisible = strings.Replace(invisible, `xmlns:t="urn:token-uri"`, `xmlns:t="urn:token-uri" xmlns:f="urn:foreign"`, 1)
				invisible = strings.Replace(invisible, `base="t:Base"`, `base="f:Base"`, 1)
				invisible = strings.Replace(invisible, `attributeFormDefault="qualified">`, `attributeFormDefault="qualified"><xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/>`, 1)
				bridge := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge"><xs:import namespace="urn:foreign" schemaLocation="base.xsd"/></xs:schema>`
				base := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:foreign"><xs:complexType name="Base"/></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, invisible, map[string]discoveryFixture{"bridge.xsd": {id: "bridge.xsd", contents: bridge}, "base.xsd": {id: "base.xsd", contents: base}}, profile.policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.Loc() != complexContentTestLoc(t, invisible, `base="f:Base"`) || d.SpecRef() != schemaComplexTypeExtensionSpecRef(profile.version) || !errors.Is(err, errSchemaComplexTypeBaseUnresolved) || !reflect.DeepEqual(d.Related(), []Loc{complexContentTestSourceLoc(t, "base.xsd", base, `<xs:complexType name="Base"`)}) {
					t.Fatalf("invisible = %v related=%v", err, d.Related())
				}
			})
			t.Run(profile.name+"/"+kind+"/duplicate", func(t *testing.T) {
				duplicate := strings.Replace(root, `<xs:complexType name="Base"/>`, "", 1)
				duplicate = strings.Replace(duplicate, `attributeFormDefault="qualified">`, `attributeFormDefault="qualified"><xs:include schemaLocation="a.xsd"/><xs:include schemaLocation="b.xsd"/>`, 1)
				base := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:token-uri"><xs:complexType name="Base"/></xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, duplicate, map[string]discoveryFixture{"a.xsd": {id: "a.xsd", contents: base}, "b.xsd": {id: "b.xsd", contents: base}}, profile.policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaGlobalDuplicateCode || d.Loc() != complexContentTestSourceLoc(t, "b.xsd", base, `<xs:complexType name="Base"`) || d.SpecRef() != schemaGlobalDuplicateSpecRef(profile.version) || !errors.Is(err, errSchemaGlobalDeclarationDuplicate) || !reflect.DeepEqual(d.Related(), []Loc{complexContentTestSourceLoc(t, "a.xsd", base, `<xs:complexType name="Base"`)}) {
					t.Fatalf("duplicate = %v related=%v", err, d.Related())
				}
			})
			t.Run(profile.name+"/"+kind+"/cycle", func(t *testing.T) {
				cyclic := strings.Replace(root, `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:complexContent><xs:extension base="t:Base"/></xs:complexContent></xs:complexType>`, 1)
				schema, err := discoverTestSchemaWithPolicy(t, cyclic, nil, profile.policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				want, locErr := NewLoc("root.xsd", 1, strings.LastIndex(cyclic, `base="t:Base"`)+1)
				if locErr != nil {
					t.Fatal(locErr)
				}
				if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.Loc() != want || d.SpecRef() != schemaComplexTypeExtensionSpecRef(profile.version) || !errors.Is(err, errSchemaComplexTypeBaseCycle) || len(d.Related()) != 0 {
					t.Fatalf("cycle = %v related=%v", err, d.Related())
				}
			})
			t.Run(profile.name+"/"+kind+"/acquisition", func(t *testing.T) {
				missing := strings.Replace(root, `attributeFormDefault="qualified">`, `attributeFormDefault="qualified"><xs:include schemaLocation="missing.xsd"/>`, 1)
				source, err := NewResolvedSource(context.Background(), "root.xsd", &discoveryReader{data: []byte(missing)})
				if err != nil {
					t.Fatal(err)
				}
				cause := errors.New("source unavailable")
				resolver := &discoveryResolver{failures: map[string]error{"missing.xsd": cause}}
				schema, err := discoverSchemaWithPolicy(source, resolver, profile.policy)
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != FailureResolution || d.Code() != SourceResolveCode || d.Loc() != complexContentTestLoc(t, missing, `<xs:include`) || !errors.Is(err, cause) {
					t.Fatalf("acquisition = %v", err)
				}
			})
		}
	}
}

func inlineTokenURIUse(t *testing.T, schema Schema) (ComplexTypeDefinition, LocalAttributeUse) {
	t.Helper()
	root := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:token-uri", "root"))
	if len(root) != 1 {
		t.Fatalf("root = %v", root)
	}
	element, ok := root[0].Element()
	if !ok {
		t.Fatal("root has no element view")
	}
	owner, ok := element.InlineComplexType()
	if !ok || !owner.ID().IsZero() || !owner.Name().IsZero() {
		t.Fatalf("inline owner = %v/%t", owner, ok)
	}
	if id, hasID := owner.NodeID(); !hasID || id.IsZero() {
		t.Fatalf("inline owner node = %v/%t", id, hasID)
	}
	uses := owner.AttributeUses()
	if len(uses) != 1 {
		t.Fatalf("uses = %v", uses)
	}
	use, ok := uses[0].(LocalAttributeUse)
	if !ok {
		t.Fatalf("use = %T", uses[0])
	}
	return owner, use
}

//nolint:gocognit // Every newly admitted datatype is checked against its public owner and copied facts.
func TestInlineExtensionTokenAnyURIPublicFacts(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "anyURI"} {
			t.Run(profile.name+"/"+kind, func(t *testing.T) {
				root := inlineTokenURIExtensionSchema("1.0", `<xs:element ref="t:child" minOccurs="0" maxOccurs="3"/>`, `<xs:attribute name="value" type="xs:`+kind+`" use="required"/>`)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				if len(schema.Documents()) != 1 || schema.Documents()[0].Source() != "root.xsd" || len(schema.Components()) != 5 {
					t.Fatalf("document/components = %v/%v", schema.Documents(), schema.Components())
				}
				owner, use := inlineTokenURIUse(t, schema)
				if owner.Loc() != complexContentTestLoc(t, root, `<xs:complexType>`) || owner.DerivationLoc() != complexContentTestLoc(t, root, `<xs:extension`) || owner.Base() != mustTestQName(t, "urn:token-uri", "Base") || owner.BaseLoc() != complexContentTestLoc(t, root, `base="t:Base"`) {
					t.Fatalf("owner = %v", owner)
				}
				base, ok := owner.BaseReference()
				baseID, hasID := base.ComponentID()
				if !ok || !hasID || baseID != schema.Components()[1].ID() || base.Loc() != owner.BaseLoc() {
					t.Fatalf("base = %v", base)
				}
				sequence, ok := owner.Particle().(SequenceParticle)
				if !ok || sequence.Loc() != complexContentTestLoc(t, root, `<xs:sequence`) || sequence.Occurrences().String() != "2/4" {
					t.Fatalf("sequence = %T", owner.Particle())
				}
				terms := sequence.Particles()
				if len(terms) != 1 {
					t.Fatalf("terms = %v", terms)
				}
				child, ok := terms[0].(ElementReferenceParticle)
				if !ok || child.Ref() != mustTestQName(t, "urn:token-uri", "child") || child.TargetID() != schema.Components()[2].ID() || child.RefLoc() != complexContentTestLoc(t, root, `ref="t:child"`) || child.Occurrences().String() != "0/3" {
					t.Fatalf("child = %v", terms[0])
				}
				if use.Loc() != complexContentTestLoc(t, root, `<xs:attribute name="value"`) || use.NameLoc() != complexContentTestLoc(t, root, `name="value"`) || use.TypeLoc() != complexContentTestLoc(t, root, `type="xs:`+kind+`"`) || use.UseLoc() != complexContentTestLoc(t, root, `use="required"`) || use.Name() != mustTestQName(t, "urn:token-uri", "value") || use.DeclaredType() != mustTestQName(t, testXSDNamespace, kind) || use.Use() != AttributeUseRequired {
					t.Fatalf("use = %#v", use)
				}
				if id, hasID := use.TypeID(); hasID || !id.IsZero() {
					t.Fatalf("built-in ID = %v/%t", id, hasID)
				}
				reference, ok := use.TypeReference()
				if !ok || !reference.IsBuiltin() || reference.Name() != use.DeclaredType() || reference.QName() != use.DeclaredType() || reference.Loc() != use.TypeLoc() || reference.Variety() != SimpleTypeVarietyAtomicRestriction {
					t.Fatalf("type reference = %v/%t", reference, ok)
				}
				if id, hasID := reference.ComponentID(); hasID || !id.IsZero() {
					t.Fatalf("reference ID = %v/%t", id, hasID)
				}
				if kind == "token" {
					white, ok := reference.StringWhiteSpaceFacet()
					if !ok || white.Value() != "collapse" || !white.Fixed() || !white.Loc().IsZero() {
						t.Fatalf("token whitespace = %v/%t", white, ok)
					}
				}
				if kind == "anyURI" {
					if white, ok := reference.StringWhiteSpaceFacet(); ok {
						t.Fatalf("anyURI exposed a new string facet view: %v", white)
					}
				}
				uses := owner.AttributeUses()
				uses[0] = nil
				if len(owner.AttributeUses()) != 1 || owner.AttributeUses()[0] == nil {
					t.Fatal("use slice mutated owner")
				}
				terms[0] = nil
				if len(sequence.Particles()) != 1 || sequence.Particles()[0] == nil {
					t.Fatal("particle slice mutated owner")
				}
				var walked []string
				if walkErr := schema.Walk(func(component Component) error {
					walked = append(walked, component.Name().Local())
					return nil
				}); walkErr != nil {
					t.Fatal(walkErr)
				}
				if !reflect.DeepEqual(walked, []string{"root", "Base", "child", "global", "NamedToken"}) {
					t.Fatalf("walk = %v", walked)
				}
				second, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil || !reflect.DeepEqual(schema.Components(), second.Components()) {
					t.Fatalf("repeat = %v: %v", second.Components(), err)
				}
			})
		}
	}
}

//nolint:gocognit // Both built-ins retain wildcard provenance after particle and use omission.
func TestInlineExtensionTokenAnyURIInheritedWildcardAndOmission(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "anyURI"} {
			t.Run(profile.name+"/"+kind, func(t *testing.T) {
				root := inlineTokenURIExtensionSchema("1.0", `<xs:element ref="t:child"/>`, `<xs:attribute name="omitted" type="xs:`+kind+`" use="prohibited"/>`)
				root = strings.Replace(root, `minOccurs="2" maxOccurs="4"`, `minOccurs="0" maxOccurs="0"`, 1)
				root = strings.Replace(root, `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>`, 1)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				owner, uses := inlineTokenURIUsePair(t, schema)
				if owner.Particle() != nil || len(uses) != 0 {
					t.Fatalf("omitted particle/uses = %v/%v", owner.Particle(), uses)
				}
				wildcard, ok := owner.AnyAttribute()
				if !ok || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" || wildcard.Loc() != complexContentTestLoc(t, root, `<xs:anyAttribute`) || wildcard.NamespaceLoc() != complexContentTestLoc(t, root, `namespace="##other"`) || wildcard.ProcessContentsLoc() != complexContentTestLoc(t, root, `processContents="lax"`) {
					t.Fatalf("inherited wildcard = %v/%t", wildcard, ok)
				}
			})
		}
	}
}

//nolint:gocognit // Each datatype and mixed use checks both consumer diagnostics and related locations.
func TestInlineExtensionTokenAnyURIMixedAndConsumers(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, tc := range []struct{ name, uses string }{
			{"token", `<xs:attribute name="token" type="xs:token"/>`},
			{"anyURI", `<xs:attribute name="uri" type="xs:anyURI"/>`},
			{"mixed", `<xs:attribute name="token" type="xs:token"/><xs:attribute name="uri" type="xs:anyURI"/><xs:attribute name="old" type="xs:token" use="prohibited"/>`},
		} {
			t.Run(profile.name+"/"+tc.name, func(t *testing.T) {
				root := inlineTokenURIExtensionSchema("1.1", `<xs:element ref="t:child"/>`, tc.uses)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatal(err)
				}
				owner, uses := inlineTokenURIUsePair(t, schema)
				if tc.name == "mixed" && (len(uses) != 2 || uses[0].Name().Local() != "token" || uses[1].Name().Local() != "uri") || tc.name != "mixed" && len(uses) != 1 {
					t.Fatalf("ordered uses = %v", owner.AttributeUses())
				}
				validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:token-uri"/>`)))
				vd := requireDiagnostic(t, validationErr)
				wantInstance, locErr := NewLoc("instance.xml", 1, 1)
				if locErr != nil {
					t.Fatal(locErr)
				}
				wantValidationRelated := []Loc{
					complexContentTestLoc(t, root, `<xs:element name="root"`), complexContentTestLoc(t, root, `<xs:complexType>`),
					complexContentTestLoc(t, root, `<xs:complexContent>`), complexContentTestLoc(t, root, `<xs:extension`),
					complexContentTestLoc(t, root, `base="t:Base"`), complexContentTestLoc(t, root, `<xs:sequence`),
				}
				if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != wantInstance || vd.SpecRef() != instanceValidationSpecRef(profile.version) || !reflect.DeepEqual(vd.Related(), wantValidationRelated) || !errors.Is(validationErr, errInstanceComplexContentExtension) {
					t.Fatalf("validation = %v related=%v", validationErr, vd.Related())
				}
				generated, generationErr := GenerateGo(schema, "generated")
				gd := requireDiagnostic(t, generationErr)
				wantGenerationRelated := []Loc{complexContentTestLoc(t, root, `<xs:complexType>`), complexContentTestLoc(t, root, `<xs:extension`), complexContentTestLoc(t, root, `base="t:Base"`)}
				if generated != nil || gd.Class() != FailureUnsupported || gd.Code() != diagnosticCodegenUnsupported || gd.Loc() != complexContentTestLoc(t, root, `<xs:element name="root"`) || gd.SpecRef() != schemaElementTypeSpecRef(profile.version) || !reflect.DeepEqual(gd.Related(), wantGenerationRelated) || !errors.Is(generationErr, errCodegenUnsupported) {
					t.Fatalf("generation = %q/%v related=%v", generated, generationErr, gd.Related())
				}
			})
		}
	}
}

func inlineTokenURIUsePair(t *testing.T, schema Schema) (ComplexTypeDefinition, []AttributeUse) {
	t.Helper()
	root := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:token-uri", "root"))
	if len(root) != 1 {
		t.Fatalf("root = %v", root)
	}
	element, ok := root[0].Element()
	if !ok {
		t.Fatal("root has no element view")
	}
	owner, ok := element.InlineComplexType()
	if !ok {
		t.Fatal("root has no inline type")
	}
	return owner, owner.AttributeUses()
}

//nolint:gocognit // Shape and use-form exclusions are separately located for both built-ins.
func TestInlineExtensionTokenAnyURIExcludedShapes(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "anyURI"} {
			for _, tc := range []struct {
				name, member, use, edit, marker, spec string
			}{
				{"inline direct", `<xs:element ref="t:child"/>`, `<xs:attribute name="value" type="xs:` + kind + `"/>`, "direct", `type="xs:` + kind + `"`, schemaAttributeTypeSpecRef(profile.version)},
				{"anonymous grouped", `<xs:element ref="t:child"/>`, `<xs:attribute name="value" type="xs:` + kind + `"/>`, "grouped", `type="xs:` + kind + `"`, schemaAttributeTypeSpecRef(profile.version)},
				{"nested choice", `<xs:choice><xs:element ref="t:child"/></xs:choice>`, `<xs:attribute name="value" type="xs:` + kind + `"/>`, "", `<xs:attribute name="value"`, schemaComplexTypeExtensionSpecRef(profile.version)},
				{"nested sequence", `<xs:sequence><xs:group ref="t:Fields"/><xs:element ref="t:child"/></xs:sequence>`, `<xs:attribute name="value" type="xs:` + kind + `"/>`, "nested", `<xs:attribute name="value"`, schemaComplexTypeExtensionSpecRef(profile.version)},
				{"local child", `<xs:element name="local" type="xs:integer"/>`, `<xs:attribute name="value" type="xs:` + kind + `"/>`, "", `type="xs:` + kind + `"`, schemaAttributeTypeSpecRef(profile.version)},
				{"inline type", `<xs:element ref="t:child"/>`, `<xs:attribute name="value"><xs:simpleType><xs:restriction base="xs:` + kind + `"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType>`, schemaAttributeTypeSpecRef(profile.version)},
				{"named restriction", `<xs:element ref="t:child"/>`, `<xs:attribute name="value" type="t:Named` + kind + `"/>`, "named restriction", `type="t:Named` + kind + `"`, schemaAttributeTypeSpecRef(profile.version)},
				{"attribute ref", `<xs:element ref="t:child"/>`, `<xs:attribute ref="t:typed"/>`, "ref", `ref="t:typed"`, schemaAttributeUseSpecRef(profile.version)},
				{"value constraint", `<xs:element ref="t:child"/>`, `<xs:attribute name="value" type="xs:` + kind + `" default="value"/>`, "", `default="value"`, schemaComplexTypeExtensionSpecRef(profile.version)},
				{"fixed constraint", `<xs:element ref="t:child"/>`, `<xs:attribute name="value" type="xs:` + kind + `" fixed="value"/>`, "", `fixed="value"`, schemaComplexTypeExtensionSpecRef(profile.version)},
			} {
				t.Run(profile.name+"/"+kind+"/"+tc.name, func(t *testing.T) {
					root := inlineTokenURIExtensionSchema("1.0", tc.member, tc.use)
					switch tc.edit {
					case "direct":
						root = strings.Replace(root, `<xs:complexContent><xs:extension base="t:Base">`, ``, 1)
						root = strings.Replace(root, `</xs:extension></xs:complexContent>`, ``, 1)
					case "grouped":
						root = strings.Replace(root, `<xs:sequence minOccurs="2" maxOccurs="4">`+tc.member+`</xs:sequence>`, `<xs:group ref="t:Fields"/>`, 1)
						root = strings.Replace(root, `</xs:schema>`, `<xs:group name="Fields"><xs:sequence><xs:element ref="t:child"/></xs:sequence></xs:group></xs:schema>`, 1)
					case "nested":
						root = strings.Replace(root, `</xs:schema>`, `<xs:group name="Fields"><xs:sequence><xs:element ref="t:child"/></xs:sequence></xs:group></xs:schema>`, 1)
					case "named restriction":
						root = strings.Replace(root, `</xs:schema>`, `<xs:simpleType name="Named`+kind+`"><xs:restriction base="xs:`+kind+`"/></xs:simpleType></xs:schema>`, 1)
					case "ref":
						root = strings.Replace(root, `</xs:schema>`, `<xs:attribute name="typed" type="xs:`+kind+`"/></xs:schema>`, 1)
					}
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != complexContentTestLoc(t, root, tc.marker) || d.SpecRef() != tc.spec || !errors.Is(err, ErrUnsupported) {
						t.Fatalf("excluded %s = %v, related=%v, spec=%q want=%q, loc=%s want=%s", tc.name, err, d.Related(), d.SpecRef(), tc.spec, d.Loc(), complexContentTestLoc(t, root, tc.marker))
					}
				})
			}
		}
	}
}

//nolint:gocognit // Each alternate exit at the particle/use/base boundary checks location and cause.
func TestInlineExtensionTokenAnyURIDiagnosticExits(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "anyURI"} {
			for _, tc := range []struct {
				name, member, use, replace, marker, code, spec, related string
				class                                                   FailureClass
				cause                                                   error
			}{
				{"unresolved child before use", `<xs:element ref="t:missing"/>`, `<xs:attribute name="value" type="xs:` + kind + `"/>`, "", `ref="t:missing"`, diagnosticSchemaElementReferenceUnresolvedCode, schemaElementReferenceSpecRef(profile.version), "", FailureInvalid, errSchemaElementReferenceUnresolved},
				{"wrong kind child", `<xs:element ref="t:global"/>`, `<xs:attribute name="value" type="xs:` + kind + `"/>`, "", `ref="t:global"`, diagnosticSchemaElementReferenceWrongKindCode, schemaElementReferenceSpecRef(profile.version), `<xs:attribute name="global"`, FailureInvalid, errSchemaElementReferenceWrongKind},
				{"unresolved type before prohibited", `<xs:element ref="t:child"/>`, `<xs:attribute name="value" type="t:Missing" use="prohibited"/>`, "", `type="t:Missing"`, diagnosticSchemaAttributeTypeUnresolvedCode, schemaAttributeTypeSpecRef(profile.version), "", FailureInvalid, errSchemaAttributeTypeUnresolved},
				{"wrong kind type", `<xs:element ref="t:child"/>`, `<xs:attribute name="value" type="t:child"/>`, "", `type="t:child"`, diagnosticSchemaAttributeTypeWrongKindCode, schemaAttributeTypeSpecRef(profile.version), `<xs:element name="child"`, FailureInvalid, errSchemaAttributeTypeWrongKind},
				{"unsupported type before prohibited", `<xs:element ref="t:child"/>`, `<xs:attribute name="value" type="xs:ID" use="prohibited"/>`, "", `type="xs:ID"`, UnsupportedSchemaSyntaxCode, schemaAttributeTypeSpecRef(profile.version), "", FailureUnsupported, errSchemaAttributeUseUnsupported},
				{"duplicate effective use", `<xs:element ref="t:child"/>`, `<xs:attribute name="value" type="xs:` + kind + `"/><xs:attribute name="value" type="xs:` + kind + `" use="required"/>`, "", `<xs:attribute name="value" type="xs:` + kind + `" use="required"`, diagnosticSchemaAttributeUseDuplicateCode, schemaAttributeUseSpecRef(profile.version), `<xs:attribute name="value" type="xs:` + kind + `"/>`, FailureInvalid, errSchemaAttributeUseDuplicate},
				{"unresolved base after use", `<xs:element ref="t:child"/>`, `<xs:attribute name="value" type="xs:` + kind + `"/>`, `base="t:Base"|base="t:Missing"`, `base="t:Missing"`, invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(profile.version), "", FailureInvalid, errSchemaComplexTypeBaseUnresolved},
				{"wrong kind base", `<xs:element ref="t:child"/>`, `<xs:attribute name="value" type="xs:` + kind + `"/>`, `base="t:Base"|base="t:NamedToken"`, `base="t:NamedToken"`, invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(profile.version), `<xs:simpleType name="NamedToken"`, FailureInvalid, errSchemaComplexTypeBaseWrongKind},
			} {
				t.Run(profile.name+"/"+kind+"/"+tc.name, func(t *testing.T) {
					root := inlineTokenURIExtensionSchema("1.0", tc.member, tc.use)
					if tc.replace != "" {
						parts := strings.Split(tc.replace, "|")
						root = strings.Replace(root, parts[0], parts[1], 1)
					}
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					if d.Class() != tc.class || d.Code() != tc.code || d.Loc() != complexContentTestLoc(t, root, tc.marker) || d.SpecRef() != tc.spec || !errors.Is(err, tc.cause) {
						t.Fatalf("diagnostic = %v, related=%v, want %s/%s at %s spec=%s cause=%v", err, d.Related(), tc.class, tc.code, tc.marker, tc.spec, tc.cause)
					}
					if tc.related != "" && !reflect.DeepEqual(d.Related(), []Loc{complexContentTestLoc(t, root, tc.related)}) {
						t.Fatalf("related = %v, want %s", d.Related(), tc.related)
					}
				})
			}
		}
	}
}

//nolint:gocognit // Named-base graph paths preserve anonymous owner and source provenance.
func TestInlineExtensionTokenAnyURIBaseGraphs(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, kind := range []string{"token", "anyURI"} {
			for _, graph := range []string{"forward", "include", "import", "chameleon", "repeated include"} {
				t.Run(profile.name+"/"+kind+"/"+graph, func(t *testing.T) {
					root := inlineTokenURIExtensionSchema("1.0", `<xs:element ref="t:child"/>`, `<xs:attribute name="value" type="xs:`+kind+`"/>`)
					fixtures := map[string]discoveryFixture{}
					baseSource := SourceID("root.xsd")
					baseNamespace := "urn:token-uri"
					baseLexical := "t:Base"
					if graph != "forward" {
						root = strings.Replace(root, `<xs:complexType name="Base"/>`, "", 1)
						baseSource = "base.xsd"
						base := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:token-uri"><xs:complexType name="Base"/></xs:schema>`
						link := `<xs:include schemaLocation="base.xsd"/>`
						switch graph {
						case "import":
							baseNamespace = "urn:foreign"
							baseLexical = "f:Base"
							base = strings.Replace(base, `urn:token-uri`, `urn:foreign`, 1)
							link = `<xs:import namespace="urn:foreign" schemaLocation="base.xsd"/>`
							root = strings.Replace(root, `xmlns:t="urn:token-uri"`, `xmlns:t="urn:token-uri" xmlns:f="urn:foreign"`, 1)
							root = strings.Replace(root, `base="t:Base"`, `base="f:Base"`, 1)
						case "chameleon":
							base = strings.Replace(base, ` targetNamespace="urn:token-uri"`, "", 1)
						case "repeated include":
							link += link
						}
						fixtures["base.xsd"] = discoveryFixture{id: "base.xsd", contents: base}
						root = strings.Replace(root, `attributeFormDefault="qualified">`, `attributeFormDefault="qualified">`+link, 1)
					}
					schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
					if err != nil {
						t.Fatal(err)
					}
					owner, use := inlineTokenURIUse(t, schema)
					base, ok := owner.BaseReference()
					id, hasID := base.ComponentID()
					if !ok || !hasID || id.Source() != baseSource || owner.Base() != mustTestQName(t, baseNamespace, "Base") || owner.BaseLoc() != complexContentTestLoc(t, root, `base="`+baseLexical+`"`) || use.DeclaredType() != mustTestQName(t, testXSDNamespace, kind) || use.Loc().Source() != "root.xsd" {
						t.Fatalf("graph facts base=%v/%t owner=%v use=%v", id, hasID, owner, use)
					}
					if len(schema.Documents()) != 1 && graph == "forward" || len(schema.Documents()) != 2 && graph != "forward" {
						t.Fatalf("documents = %v", schema.Documents())
					}
				})
			}
		}
	}
}
