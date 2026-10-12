package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func inlineGroupedDefinition(t *testing.T, schema Schema) ComplexTypeDefinition {
	t.Helper()
	components := schema.Components()
	if len(components) == 0 || components[0].Name().Local() != "root" {
		t.Fatalf("global walk = %v", components)
	}
	element, ok := components[0].Element()
	if !ok {
		t.Fatal("root element missing")
	}
	inline, ok := element.InlineComplexType()
	if !ok || !inline.ID().IsZero() || !inline.Name().IsZero() {
		t.Fatalf("anonymous type = %v", inline)
	}
	if id, ok := inline.NodeID(); !ok || id.IsZero() {
		t.Fatal("anonymous node ID missing")
	}
	return inline
}

//nolint:gocognit // Exercise omission and both consumer boundaries for each selected edition.
func TestInlineGroupedExtensionZeroAndConsumers(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		version := XSDVersion11
		if policy == Strict10 {
			version = XSDVersion10
		}
		for _, test := range []struct{ name, group, use string }{
			{"present group and use", "", `<xs:attribute name="flag" type="xs:boolean"/>`},
			{"zero group and use", ` minOccurs="0" maxOccurs="0"`, `<xs:attribute name="flag" type="xs:boolean"/>`},
			{"present group prohibited use", "", `<xs:attribute name="gone" type="xs:boolean" use="prohibited"/>`},
			{"zero group prohibited use", ` minOccurs="0" maxOccurs="0"`, `<xs:attribute name="gone" type="xs:boolean" use="prohibited"/>`},
		} {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				root := anonymousGroupedExtensionSchema("1.0", test.group, test.use)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err != nil {
					t.Fatal(err)
				}
				inline := inlineGroupedDefinition(t, schema)
				if inline.Derivation() != ComplexTypeDerivationExtension || inline.BaseLoc() != complexContentTestLoc(t, root, `base="t:Base"`) {
					t.Fatalf("base/derivation = %v", inline)
				}
				if (inline.Particle() == nil) != (test.group != "") || (len(inline.AttributeUses()) == 0) != strings.Contains(test.use, "prohibited") {
					t.Fatalf("omitted facts = %v/%v", inline.Particle(), inline.AttributeUses())
				}
				validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`)))
				vd := requireDiagnostic(t, validationErr)
				instanceLoc, locErr := NewLoc("instance.xml", 1, 1)
				if locErr != nil {
					t.Fatal(locErr)
				}
				wantRelated := []Loc{
					complexContentTestLoc(t, root, `<xs:element name="root"`),
					complexContentTestLoc(t, root, `<xs:complexType>`),
					complexContentTestLoc(t, root, `<xs:complexContent>`),
					complexContentTestLoc(t, root, `<xs:extension`),
					complexContentTestLoc(t, root, `base="t:Base"`),
				}
				if test.group == "" {
					wantRelated = append(wantRelated, complexContentTestLoc(t, root, `<xs:group ref="t:Fields"`))
				}
				if vd.Class() != FailureUnsupported || vd.Code() != UnsupportedInstanceValidationCode || vd.Loc() != instanceLoc || vd.SpecRef() != instanceValidationSpecRef(version) || !reflect.DeepEqual(vd.Related(), wantRelated) || !errors.Is(validationErr, ErrUnsupported) || !errors.Is(validationErr, errInstanceComplexContentExtension) {
					t.Fatalf("validation = %v related=%v", validationErr, vd.Related())
				}
				output, generationErr := GenerateGo(schema, "generated")
				gd := requireDiagnostic(t, generationErr)
				if output != nil || gd.Class() != FailureUnsupported || gd.Code() != diagnosticCodegenUnsupported || gd.Loc() != complexContentTestLoc(t, root, `<xs:element name="root"`) || gd.SpecRef() != schemaElementTypeSpecRef(version) || !errors.Is(generationErr, ErrUnsupported) || !errors.Is(generationErr, errCodegenUnsupported) {
					t.Fatalf("generation = %q/%v", output, generationErr)
				}
			})
		}
	}
}

//nolint:gocognit // Every exclusion must fail before zero/prohibited omission.
func TestInlineGroupedExtensionExcludedUses(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		version := XSDVersion11
		if policy == Strict10 {
			version = XSDVersion10
		}
		for _, test := range []struct {
			name, use, extra, marker, code, spec string
			cause                                error
		}{
			{"local ID", `<xs:attribute name="id" type="xs:ID" use="prohibited"/>`, "", `type="xs:ID"`, UnsupportedSchemaSyntaxCode, schemaAttributeTypeSpecRef(version), errSchemaAttributeUseUnsupported},
			{"named ID", `<xs:attribute name="id" type="t:NamedID"/>`, `<xs:simpleType name="NamedID"><xs:restriction base="xs:ID"/></xs:simpleType>`, `type="t:NamedID"`, UnsupportedSchemaSyntaxCode, schemaAttributeTypeSpecRef(version), errSchemaAttributeUseUnsupported},
			{"inline ID", `<xs:attribute name="id"><xs:simpleType><xs:restriction base="xs:ID"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType>`, UnsupportedSchemaSyntaxCode, schemaAttributeTypeSpecRef(version), errSchemaAttributeUseUnsupported},
			{"ID ref", `<xs:attribute ref="t:globalID" use="prohibited"/>`, `<xs:attribute name="globalID" type="xs:ID"/>`, `ref="t:globalID"`, UnsupportedSchemaSyntaxCode, schemaAttributeUseSpecRef(version), errSchemaAttributeReferenceUnsupported},
			{"inline list", `<xs:attribute name="a" use="prohibited"><xs:simpleType><xs:list itemType="xs:precisionDecimal"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType><xs:list`, UnsupportedSchemaSyntaxCode, schemaAttributeUseSpecRef(version), errSchemaAttributeUseUnsupported},
			{"named list", `<xs:attribute name="a" type="t:List" use="prohibited"/>`, `<xs:simpleType name="List"><xs:list itemType="xs:precisionDecimal"/></xs:simpleType>`, `type="t:List"`, UnsupportedSchemaSyntaxCode, schemaAttributeUseSpecRef(version), errSchemaAttributeUseUnsupported},
			{"inline union", `<xs:attribute name="a"><xs:simpleType><xs:union memberTypes="xs:precisionDecimal xs:negativeInteger"/></xs:simpleType></xs:attribute>`, "", `<xs:simpleType><xs:union`, UnsupportedSchemaSyntaxCode, schemaAttributeUseSpecRef(version), errSchemaAttributeUseUnsupported},
			{"named union", `<xs:attribute name="a" type="t:Union"/>`, `<xs:simpleType name="Union"><xs:union memberTypes="xs:precisionDecimal xs:negativeInteger"/></xs:simpleType>`, `type="t:Union"`, UnsupportedSchemaSyntaxCode, schemaAttributeUseSpecRef(version), errSchemaAttributeUseUnsupported},
		} {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				root := anonymousGroupedExtensionSchema("1.1", ` minOccurs="0" maxOccurs="0"`, test.use)
				root = strings.Replace(root, `</xs:schema>`, test.extra+`</xs:schema>`, 1)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err == nil {
					t.Fatal("excluded use accepted")
				}
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if policy == Strict10 && (strings.Contains(test.name, "list") || strings.Contains(test.name, "union")) {
					marker := test.marker
					if test.name == "inline list" {
						marker = `<xs:simpleType>`
					}
					if d.Class() != FailureUnsupported || d.Code() != diagnosticSchemaPrecisionDecimalVersionCode || d.Loc() != complexContentTestLoc(t, root, marker) || d.SpecRef() != "xsd11-datatypes#dt-primitive" || len(d.Related()) == 0 || !errors.Is(err, errSchemaPrecisionDecimalVersion) || !errors.Is(err, errLanguagePolicyMismatch) {
						t.Fatalf("Strict10 variety = %v spec=%s loc=%s want %s related=%v causes=%t/%t", err, d.SpecRef(), d.Loc(), complexContentTestLoc(t, root, marker), d.Related(), errors.Is(err, errSchemaPrecisionDecimalVersion), errors.Is(err, errLanguagePolicyMismatch))
					}
					return
				}
				if d.Class() != FailureUnsupported || d.Code() != test.code || d.Loc() != complexContentTestLoc(t, root, test.marker) || d.SpecRef() != test.spec || !errors.Is(err, ErrUnsupported) || !errors.Is(err, test.cause) {
					t.Fatalf("excluded use = %v related=%v", err, d.Related())
				}
			})
		}
	}
}

//nolint:gocognit // The same boundary offers distinct group, use, and base exits.
func TestInlineGroupedExtensionDiagnosticOrderAndExits(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		version := XSDVersion11
		if policy == Strict10 {
			version = XSDVersion10
		}
		for _, test := range []struct {
			name, group, use, base, marker, code, spec string
			class                                      FailureClass
			cause                                      error
			related                                    string
		}{
			{"group before use", "Missing", `<xs:attribute ref="t:missing"/>`, "Missing", `ref="t:Missing"`, diagnosticSchemaModelGroupReferenceUnresolvedCode, schemaModelGroupReferenceSpecRef(version), FailureInvalid, errSchemaModelGroupReferenceUnresolved, ""},
			{"use before base", "Fields", `<xs:attribute ref="t:missing" use="prohibited"/>`, "Missing", `ref="t:missing"`, diagnosticSchemaAttributeReferenceUnresolvedCode, schemaAttributeUseResolveSpecRef(version), FailureInvalid, errSchemaAttributeReferenceUnresolved, ""},
			{"wrong kind group", "global", `<xs:attribute name="flag" type="xs:boolean"/>`, "Base", `ref="t:global"`, diagnosticSchemaModelGroupReferenceWrongKindCode, schemaModelGroupReferenceSpecRef(version), FailureInvalid, errSchemaModelGroupReferenceWrongKind, `<xs:attribute name="global"`},
			{"wrong kind use", "Fields", `<xs:attribute ref="t:Base"/>`, "Base", `ref="t:Base"`, diagnosticSchemaAttributeReferenceWrongKindCode, schemaAttributeUseResolveSpecRef(version), FailureInvalid, errSchemaAttributeReferenceWrongKind, `<xs:complexType name="Base"`},
			{"unresolved base", "Fields", `<xs:attribute name="flag" type="xs:boolean"/>`, "Missing", `base="t:Missing"`, invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(version), FailureInvalid, errSchemaComplexTypeBaseUnresolved, ""},
			{"wrong kind base", "Fields", `<xs:attribute name="flag" type="xs:boolean"/>`, "Fields", `base="t:Fields"`, invalidSchemaCompositionCode, schemaComplexTypeExtensionSpecRef(version), FailureInvalid, errSchemaComplexTypeBaseWrongKind, `<xs:group name="Fields"`},
			{"duplicate use", "Fields", `<xs:attribute name="same" type="xs:boolean"/><xs:attribute name="same" type="xs:integer"/>`, "Base", `<xs:attribute name="same" type="xs:integer"`, diagnosticSchemaAttributeUseDuplicateCode, schemaAttributeUseSpecRef(version), FailureInvalid, errSchemaAttributeUseDuplicate, `<xs:attribute name="same" type="xs:boolean"`},
			{"nonempty base", "Fields", `<xs:attribute name="flag" type="xs:boolean"/>`, "Base", `base="t:Base"`, UnsupportedSchemaSyntaxCode, schemaComplexTypeExtensionSpecRef(version), FailureUnsupported, errSchemaComplexTypeBaseNonEmpty, `<xs:sequence><xs:element name="inBase"`},
		} {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				root := anonymousGroupedExtensionSchema("1.1", "", test.use)
				root = strings.Replace(root, `ref="t:Fields"`, `ref="t:`+test.group+`"`, 1)
				if test.name == "group before use" {
					root = strings.Replace(root, `ref="t:Missing"/>`, `ref="t:Missing" minOccurs="0" maxOccurs="0"/>`, 1)
				}
				root = strings.Replace(root, `base="t:Base"`, `base="t:`+test.base+`"`, 1)
				if test.name == "nonempty base" {
					root = strings.Replace(root, `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:sequence><xs:element name="inBase" type="xs:integer"/></xs:sequence></xs:complexType>`, 1)
				}
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err == nil {
					t.Fatal("bad grouped extension accepted")
				}
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != test.class || d.Code() != test.code || d.Loc() != complexContentTestLoc(t, root, test.marker) || d.SpecRef() != test.spec || !errors.Is(err, test.cause) {
					t.Fatalf("diagnostic = %v related=%v", err, d.Related())
				}
				if test.related != "" && !slices.Contains(d.Related(), complexContentTestLoc(t, root, test.related)) {
					t.Fatalf("related = %v, missing %s", d.Related(), test.related)
				}
			})
		}
	}
}

//nolint:gocognit // Each graph shape checks the three independently resolved identities.
func TestInlineGroupedExtensionGraphFacts(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		for _, graph := range []string{"forward", "include", "import", "chameleon"} {
			t.Run(string(policy)+"/"+graph, func(t *testing.T) {
				namespace := "urn:root"
				prefix := "t"
				source := SourceID("root.xsd")
				declarations := `<xs:group name="Fields"><xs:choice/></xs:group><xs:attribute name="global" type="xs:boolean"/><xs:complexType name="Base"><xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent></xs:complexType>`
				rootDecls := declarations
				directive := ""
				fixtures := map[string]discoveryFixture{}
				if graph != "forward" {
					rootDecls = ""
					source = SourceID(graph + ".xsd")
					target := ` targetNamespace="urn:root"`
					if graph == "import" {
						namespace = "urn:foreign"
						prefix = "f"
						target = ` targetNamespace="urn:foreign"`
						directive = `<xs:import namespace="urn:foreign" schemaLocation="import.xsd"/>`
					}
					if graph == "include" {
						directive = `<xs:include schemaLocation="include.xsd"/>`
					}
					if graph == "chameleon" {
						target = ""
						directive = `<xs:include schemaLocation="chameleon.xsd"/>`
					}
					content := `<xs:schema xmlns:xs="` + testXSDNamespace + `"` + target + `>` + declarations + `</xs:schema>`
					fixtures[string(source)] = discoveryFixture{id: source, contents: content}
				}
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" xmlns:f="urn:foreign" targetNamespace="urn:root" version="1.0">` + directive +
					`<xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="` + prefix + `:Base"><xs:group ref="` + prefix + `:Fields" minOccurs="2" maxOccurs="3"/><xs:attribute ref="` + prefix + `:global" use="required"/></xs:extension></xs:complexContent></xs:complexType></xs:element>` + rootDecls + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy)
				if err != nil {
					t.Fatal(err)
				}
				inline := inlineGroupedDefinition(t, schema)
				groups := schema.FindKind(ComponentKindModelGroupDefinition, mustTestQName(t, namespace, "Fields"))
				bases := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, namespace, "Base"))
				globals := schema.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, namespace, "global"))
				if len(groups) != 1 || len(bases) != 1 || len(globals) != 1 || groups[0].ID().Source() != source || bases[0].ID().Source() != source || globals[0].ID().Source() != source {
					t.Fatalf("targets = %v/%v/%v", groups, bases, globals)
				}
				group, ok := inline.Particle().(ModelGroupReferenceParticle)
				if !ok || group.TargetID() != groups[0].ID() || group.Ref() != mustTestQName(t, namespace, "Fields") || group.RefLoc() != complexContentTestLoc(t, root, `ref="`+prefix+`:Fields"`) || group.Occurrences().String() != "2/3" {
					t.Fatalf("group = %v", group)
				}
				base, ok := inline.BaseReference()
				baseID, hasID := base.ComponentID()
				if !ok || !hasID || baseID != bases[0].ID() || base.Loc() != complexContentTestLoc(t, root, `base="`+prefix+`:Base"`) {
					t.Fatalf("base = %v", base)
				}
				uses := inline.AttributeUses()
				ref, ok := uses[0].(AttributeReferenceUse)
				if len(uses) != 1 || !ok || ref.TargetID() != globals[0].ID() || ref.RefLoc() != complexContentTestLoc(t, root, `ref="`+prefix+`:global"`) {
					t.Fatalf("use = %v", uses)
				}
				wildcard, ok := inline.AnyAttribute()
				if !ok || wildcard.Namespace() != "##other" || wildcard.ProcessContents() != "lax" || wildcard.Loc().Source() != source {
					t.Fatalf("wildcard = %v", wildcard)
				}
				for _, component := range schema.Components() {
					if component.ID() == inline.ID() {
						t.Fatal("anonymous owner leaked into walk")
					}
				}
			})
		}
	}
}

func TestInlineGroupedExtensionGraphFailures(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		version := XSDVersion11
		if policy == Strict10 {
			version = XSDVersion10
		}
		root := anonymousGroupedExtensionSchema("1.1", ` minOccurs="0" maxOccurs="0"`, `<xs:attribute name="flag" type="xs:boolean"/>`)
		root = strings.Replace(root, `<xs:group name="Fields"><xs:sequence/></xs:group>`, `<xs:group name="Fields"><xs:sequence><xs:group ref="t:Fields"/></xs:sequence></xs:group>`, 1)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
		if err == nil {
			t.Fatal("cyclic group accepted")
		}
		assertZeroSchema(t, schema)
		d := requireDiagnostic(t, err)
		cycleSpec := "xsd11-structures#mg-props-correct"
		if policy == Strict10 {
			cycleSpec = "xsd10-structures#mg-props-correct"
		}
		if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaModelGroupReferenceCycleCode || d.Loc() != complexContentTestLoc(t, root, `ref="t:Fields"/></xs:sequence>`) || d.SpecRef() != cycleSpec || len(d.Related()) == 0 || !errors.Is(err, errSchemaModelGroupReferenceCycle) {
			t.Fatalf("cycle = %v related=%v", err, d.Related())
		}

		root = `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:f="urn:foreign" targetNamespace="urn:root"><xs:import namespace="urn:bridge" schemaLocation="bridge.xsd"/><xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="f:Base"><xs:group ref="f:Fields"/><xs:attribute name="flag" type="xs:boolean"/></xs:extension></xs:complexContent></xs:complexType></xs:element></xs:schema>`
		bridge := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:bridge"><xs:import namespace="urn:foreign" schemaLocation="foreign.xsd"/></xs:schema>`
		foreign := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:foreign"><xs:group name="Fields"><xs:sequence/></xs:group><xs:complexType name="Base"/></xs:schema>`
		schema, err = discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{"bridge.xsd": {id: "bridge.xsd", contents: bridge}, "foreign.xsd": {id: "foreign.xsd", contents: foreign}}, policy)
		if err == nil {
			t.Fatal("invisible group accepted")
		}
		assertZeroSchema(t, schema)
		d = requireDiagnostic(t, err)
		if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaModelGroupReferenceNamespaceCode || d.Loc() != complexContentTestLoc(t, root, `ref="f:Fields"`) || d.SpecRef() != schemaModelGroupReferenceImportSpecRef(version) || !errors.Is(err, errSchemaModelGroupReferenceNamespace) || len(d.Related()) == 0 || d.Related()[0].Source() != "foreign.xsd" {
			t.Fatalf("visibility = %v related=%v", err, d.Related())
		}
	}
}

//nolint:gocognit // The table proves invalid bounds and unsupported shapes keep their own exits.
func TestInlineGroupedExtensionBoundsAndShapeExits(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		version := XSDVersion11
		if policy == Strict10 {
			version = XSDVersion10
		}
		for _, test := range []struct {
			name, group, use, marker, code, spec string
			class                                FailureClass
			cause                                error
			related                              []string
		}{
			{"bad minimum", `<xs:group ref="t:Fields" minOccurs="bad"/>`, `<xs:attribute name="flag" type="xs:boolean"/>`, `minOccurs="bad"`, invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(version), FailureInvalid, nil, nil},
			{"reversed bounds", `<xs:group ref="t:Fields" minOccurs="2" maxOccurs="1"/>`, `<xs:attribute name="flag" type="xs:boolean"/>`, `<xs:group ref="t:Fields"`, invalidSchemaCompositionCode, schemaParticleCorrectSpecRef(version), FailureInvalid, errParticleOccurrenceMinimumExceedsMaximum, []string{`minOccurs="2"`, `maxOccurs="1"`}},
			{"nested group in sequence", `<xs:sequence><xs:group ref="t:Fields"/></xs:sequence>`, `<xs:attribute name="flag" type="xs:boolean"/>`, `ref="t:Fields"`, UnsupportedSchemaSyntaxCode, schemaComplexParticleExtensionSpecRef(version), FailureUnsupported, ErrUnsupported, nil},
			{"all model", `<xs:all/>`, `<xs:attribute name="flag" type="xs:boolean"/>`, `<xs:attribute name="flag"`, UnsupportedSchemaSyntaxCode, schemaComplexTypeExtensionSpecRef(version), FailureUnsupported, ErrUnsupported, nil},
			{"new wildcard", `<xs:group ref="t:Fields"/>`, `<xs:attribute name="flag" type="xs:boolean"/><xs:anyAttribute namespace="##other" processContents="lax"/>`, `<xs:anyAttribute`, UnsupportedSchemaSyntaxCode, schemaComplexTypeExtensionSpecRef(version), FailureUnsupported, ErrUnsupported, nil},
		} {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				root := anonymousGroupedExtensionSchema("1.0", "", test.use)
				root = strings.Replace(root, `<xs:group ref="t:Fields"/>`, test.group, 1)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err == nil {
					t.Fatal("excluded model accepted")
				}
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != test.class || d.Code() != test.code || d.Loc() != complexContentTestLoc(t, root, test.marker) || d.SpecRef() != test.spec || (test.cause != nil && !errors.Is(err, test.cause)) {
					t.Fatalf("exit = %v spec=%s want %s loc=%s want %s related=%v", err, d.SpecRef(), test.spec, d.Loc(), complexContentTestLoc(t, root, test.marker), d.Related())
				}
				for _, marker := range test.related {
					if !slices.Contains(d.Related(), complexContentTestLoc(t, root, marker)) {
						t.Fatalf("related = %v missing %s", d.Related(), marker)
					}
				}
			})
		}
	}
}

func TestInlineGroupedExtensionBaseCycleAfterGroupAndUses(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict10, Strict11} {
		version := XSDVersion11
		if policy == Strict10 {
			version = XSDVersion10
		}
		root := anonymousGroupedExtensionSchema("1.1", "", `<xs:attribute name="flag" type="xs:boolean"/>`)
		root = strings.Replace(root, `<xs:complexType name="Base"/>`, `<xs:complexType name="Base"><xs:complexContent><xs:extension base="t:Other"/></xs:complexContent></xs:complexType><xs:complexType name="Other"><xs:complexContent><xs:extension base="t:Base"/></xs:complexContent></xs:complexType>`, 1)
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
		if err == nil {
			t.Fatal("base cycle accepted")
		}
		assertZeroSchema(t, schema)
		d := requireDiagnostic(t, err)
		if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.Loc() != complexContentTestLoc(t, root, `base="t:Base"/></xs:complexContent></xs:complexType>`) || d.SpecRef() != schemaComplexTypeExtensionSpecRef(version) || !errors.Is(err, errSchemaComplexTypeBaseCycle) || len(d.Related()) == 0 {
			t.Fatalf("base cycle = %v related=%v", err, d.Related())
		}
	}
}

func TestInlineGroupedExtensionAmbiguousDeclarationsFailAtDuplicate(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:f="urn:foreign" xmlns:t="urn:root" targetNamespace="urn:root"><xs:import namespace="urn:foreign" schemaLocation="a.xsd"/><xs:import namespace="urn:foreign" schemaLocation="b.xsd"/><xs:element name="root"><xs:complexType><xs:complexContent><xs:extension base="t:Base"><xs:group ref="f:Fields"/><xs:attribute name="flag" type="xs:boolean"/></xs:extension></xs:complexContent></xs:complexType></xs:element><xs:complexType name="Base"/></xs:schema>`
	foreign := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:foreign"><xs:group name="Fields"><xs:sequence/></xs:group></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
		"a.xsd": {id: "a.xsd", contents: foreign},
		"b.xsd": {id: "b.xsd", contents: foreign},
	}, Strict11)
	if err == nil {
		t.Fatal("duplicate group declarations accepted")
	}
	assertZeroSchema(t, schema)
	d := requireDiagnostic(t, err)
	wantRelated := []Loc{complexContentTestSourceLoc(t, "a.xsd", foreign, `<xs:group name="Fields"`)}
	if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaGlobalDuplicateCode || d.Loc() != complexContentTestSourceLoc(t, "b.xsd", foreign, `<xs:group name="Fields"`) || d.SpecRef() != schemaGlobalDuplicateSpecRef(XSDVersion11) || !reflect.DeepEqual(d.Related(), wantRelated) || !errors.Is(err, errSchemaGlobalDeclarationDuplicate) {
		t.Fatalf("duplicate = %v related=%v", err, d.Related())
	}
}
