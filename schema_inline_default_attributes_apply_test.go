package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func inlineDefaultAttributesApplySchema(body string, value schemaDefaultAttributesApplyValue, version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" elementFormDefault="qualified" version="` + string(version) + `">` + "\n" +
		`<xs:element name="root">` + "\n" + `<xs:complexType` + schemaDefaultAttributesApplyAttribute(value) + `>` + "\n" + body + "\n" + `</xs:complexType>` + "\n" + `</xs:element>` + "\n" +
		`<xs:element name="target" type="xs:integer"/>` + "\n" + `<xs:attribute name="global" type="xs:boolean"/>` + "\n" +
		`<xs:complexType name="Base"/></xs:schema>`
}

//nolint:gocognit // The complete Boolean and supported-body matrix shares one public snapshot.
func TestInlineDefaultAttributesApplyMatchesOmission(t *testing.T) {
	bodies := []struct{ name, xml string }{
		{"empty", ""},
		{"attribute only", `<xs:attribute name="flag" type="xs:boolean" use="required"/><xs:attribute ref="t:global"/>`},
		{"sequence and attributes", `<xs:sequence minOccurs="0" maxOccurs="2"><xs:element ref="t:target" minOccurs="0" maxOccurs="3"/><xs:element name="local" type="xs:integer"/></xs:sequence><xs:attribute name="flag" type="xs:boolean" use="required"/><xs:attribute ref="t:global"/>`},
		{"choice", `<xs:choice><xs:element name="first" type="xs:integer"/><xs:element name="second" type="xs:decimal"/></xs:choice>`},
		{"simple content", `<xs:simpleContent><xs:extension base="xs:string"><xs:attribute name="flag" type="xs:boolean"/></xs:extension></xs:simpleContent>`},
		{"complex extension", `<xs:complexContent><xs:extension base="t:Base"><xs:sequence><xs:element ref="t:target"/></xs:sequence></xs:extension></xs:complexContent>`},
		{"complex restriction", `<xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent>`},
	}
	for _, policy := range []struct {
		name  string
		value LanguagePolicy
	}{{"Compatibility", Compatibility}, {"Strict11", Strict11}} {
		for _, version := range []XSDVersion{XSDVersion10, XSDVersion11} {
			for _, body := range bodies {
				t.Run(policy.name+"/"+string(version)+"/"+body.name, func(t *testing.T) {
					values := schemaDefaultAttributesApplyValues()
					omitted := inlineMixedSnapshot(t, inlineDefaultAttributesApplySchema(body.xml, values[0], version), policy.value)
					for _, value := range values[1:] {
						got := inlineMixedSnapshot(t, inlineDefaultAttributesApplySchema(body.xml, value, version), policy.value)
						if !reflect.DeepEqual(got, omitted) {
							t.Fatalf("%s changed public queries, walk, or consumers: got %#v; omitted %#v", value.name, got, omitted)
						}
					}
				})
			}
		}
	}
}

//nolint:gocognit // Verify copied ordered facts and exact bounds for the admitted annotation.
func TestInlineDefaultAttributesApplyPreservesCopiedFacts(t *testing.T) {
	value := schemaDefaultAttributesApplyValues()[1]
	root := inlineDefaultAttributesApplySchema(`<xs:sequence minOccurs="0" maxOccurs="2"><xs:element ref="t:target" minOccurs="0" maxOccurs="3"/><xs:element name="local" type="xs:integer"/></xs:sequence><xs:attribute name="flag" type="xs:boolean" use="required"/><xs:attribute ref="t:global"/>`, value, XSDVersion11)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatal(err)
	}
	components := schema.Components()
	if len(components) != 4 || components[0].Name().Local() != "root" || components[1].Name().Local() != "target" || components[2].Name().Local() != "global" || components[3].Name().Local() != "Base" {
		t.Fatalf("component declaration order = %#v", components)
	}
	definition, ok := auxiliaryElement(t, schema, "root").InlineComplexType()
	if !ok || definition.Loc() != schemaMixedComplexLoc(t, root, `<xs:complexType`) {
		t.Fatalf("inline type view/location = %v/%s", ok, definition.Loc())
	}
	sequence, ok := definition.Particle().(SequenceParticle)
	if !ok || sequence.Occurrences().String() != "0/2" || sequence.Loc() != schemaMixedComplexLoc(t, root, `<xs:sequence`) {
		t.Fatalf("sequence facts = %T", definition.Particle())
	}
	children := sequence.Particles()
	if len(children) != 2 {
		t.Fatalf("sequence children = %d", len(children))
	}
	reference, ok := children[0].(ElementReferenceParticle)
	if !ok || reference.TargetID() != components[1].ID() || reference.RefLoc() != schemaMixedComplexLoc(t, root, `ref="t:target"`) || reference.Occurrences().String() != "0/3" {
		t.Fatalf("reference facts = %#v", children[0])
	}
	local, ok := children[1].(ElementParticle)
	if !ok || local.Name().Local() != "local" || local.Occurrences().String() != "1/1" || local.Loc() != schemaMixedComplexLoc(t, root, `<xs:element name="local"`) {
		t.Fatalf("local facts = %#v", children[1])
	}
	uses := definition.AttributeUses()
	if len(uses) != 2 || uses[0].Name().Local() != "flag" || uses[0].Use() != AttributeUseRequired || uses[0].Loc() != schemaMixedComplexLoc(t, root, `<xs:attribute name="flag"`) {
		t.Fatalf("ordered uses = %#v", uses)
	}
	globalUse, ok := uses[1].(AttributeReferenceUse)
	if !ok || globalUse.TargetID() != components[2].ID() || globalUse.RefLoc() != schemaMixedComplexLoc(t, root, `ref="t:global"`) {
		t.Fatalf("global use = %#v", uses[1])
	}
	components[0] = Component{}
	children[0] = nil
	uses[0] = nil
	fresh, ok := definition.Particle().(SequenceParticle)
	if !ok || schema.Components()[0].Kind() != ComponentKindElementDeclaration || len(fresh.Particles()) != 2 || definition.AttributeUses()[0] == nil {
		t.Fatal("mutating query slices changed schema facts")
	}
}

//nolint:gocognit // Exercise all alternate exits at the inline attribute boundary.
func TestInlineDefaultAttributesApplyAttributeExits(t *testing.T) {
	for _, policy := range []struct {
		name    string
		value   LanguagePolicy
		version XSDVersion
	}{{"Compatibility", Compatibility, XSDVersion11}, {"Strict10", Strict10, XSDVersion10}, {"Strict11", Strict11, XSDVersion11}} {
		for _, lexical := range []string{"true", "false", "1", "0", "", "maybe", "TRUE", "true false"} {
			if policy.value != Strict10 && (lexical == "true" || lexical == "false" || lexical == "1" || lexical == "0") {
				continue
			}
			t.Run(policy.name+"/"+lexical, func(t *testing.T) {
				root := inlineDefaultAttributesApplySchema(`<xs:sequence/>`, schemaDefaultAttributesApplyValue{present: true, lexical: lexical}, policy.version)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.value)
				if err == nil {
					t.Fatal("invalid or Strict10 value returned a schema")
				}
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				loc := schemaMixedComplexLoc(t, root, `defaultAttributesApply="`+lexical+`"`)
				if d.Loc() != loc || len(d.Related()) != 0 {
					t.Fatalf("location/related = %s/%v, want %s/none", d.Loc(), d.Related(), loc)
				}
				valid := lexical == "true" || lexical == "false" || lexical == "1" || lexical == "0"
				if policy.value == Strict10 && valid {
					if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Feature() != FeatureSchemaSyntax || d.SpecRef() != "xsd11-structures#cSchemaDocument" || !errors.Is(err, ErrUnsupported) || !errors.Is(err, errLanguagePolicyMismatch) {
						t.Fatalf("Strict10 mismatch = %s (%v)", d, err)
					}
					return
				}
				if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.SpecRef() != "" || errors.Is(err, ErrUnsupported) || errors.Is(err, errLanguagePolicyMismatch) {
					t.Fatalf("invalid Boolean diagnostic = %s (%v)", d, err)
				}
				if d.Message() != `attribute "defaultAttributesApply" has an invalid boolean value` || d.Unwrap() != nil {
					t.Fatalf("direct invalid Boolean diagnostic = %s", d)
				}
			})
		}
	}
}

//nolint:gocognit // Each excluded owner/consumer shape retains its own diagnostic boundary.
func TestInlineDefaultAttributesApplyKeepsExcludedShapes(t *testing.T) {
	const prefix = `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root">`
	tests := []struct {
		name, body, marker, related, code string
		class                             FailureClass
		spec                              func(XSDVersion) string
		cause                             error
	}{
		{name: "direct group inline owner", body: `<xs:element name="root"><xs:complexType defaultAttributesApply="true"><xs:group ref="t:Fields"/></xs:complexType></xs:element><xs:group name="Fields"><xs:sequence/></xs:group>`, marker: `ref="t:Fields"`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, spec: func(XSDVersion) string { return newSchemaSyntaxUnsupported(Loc{}, "").SpecRef() }},
		{name: "direct zero group inline owner", body: `<xs:element name="root"><xs:complexType defaultAttributesApply="true"><xs:group ref="t:Fields" minOccurs="0" maxOccurs="0"/></xs:complexType></xs:element><xs:group name="Fields"><xs:sequence/></xs:group>`, marker: `ref="t:Fields"`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, spec: func(XSDVersion) string { return newSchemaSyntaxUnsupported(Loc{}, "").SpecRef() }},
		{name: "direct all inline owner", body: `<xs:element name="root"><xs:complexType defaultAttributesApply="true"><xs:all><xs:element name="v" type="xs:integer"/></xs:all></xs:complexType></xs:element>`, marker: `<xs:all>`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, spec: func(XSDVersion) string { return newSchemaSyntaxUnsupported(Loc{}, "").SpecRef() }},
		{name: "grouped extension anonymous owner", body: `<xs:element name="root"><xs:complexType defaultAttributesApply="true"><xs:complexContent><xs:extension base="t:Base"><xs:group ref="t:Fields"/><xs:attribute name="flag" type="xs:boolean"/></xs:extension></xs:complexContent></xs:complexType></xs:element><xs:group name="Fields"><xs:sequence/></xs:group><xs:complexType name="Base"/>`, marker: `<xs:extension`, related: `<xs:complexType defaultAttributesApply=`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, spec: schemaComplexContentExtensionSpecRef, cause: errSchemaGroupedExtensionAnonymousOwner},
		{name: "local inline child sequence", body: `<xs:complexType name="Owner"><xs:sequence><xs:element name="child"><xs:complexType defaultAttributesApply="true"/></xs:element></xs:sequence></xs:complexType>`, marker: `<xs:complexType defaultAttributesApply=`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, spec: func(XSDVersion) string { return newSchemaSyntaxUnsupported(Loc{}, "").SpecRef() }},
		{name: "all member inline child", body: `<xs:complexType name="Owner"><xs:all><xs:element name="child"><xs:complexType defaultAttributesApply="true"/></xs:element></xs:all></xs:complexType>`, marker: `<xs:complexType defaultAttributesApply=`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, spec: func(v XSDVersion) string { return newSchemaSyntaxUnsupportedForVersion(Loc{}, "", v).SpecRef() }},
		{name: "all member zero", body: `<xs:complexType name="Owner"><xs:all><xs:element name="child" minOccurs="0" maxOccurs="0"><xs:complexType defaultAttributesApply="true"/></xs:element></xs:all></xs:complexType>`, marker: `<xs:complexType defaultAttributesApply=`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, spec: schemaAllLimitedSpecRef, cause: errSchemaAllMemberInlineComplex},
		{name: "local ref plus inline type", body: `<xs:element name="target" type="xs:integer"/><xs:complexType name="Owner"><xs:sequence><xs:element ref="t:target"><xs:complexType defaultAttributesApply="true"/></xs:element></xs:sequence></xs:complexType>`, marker: `<xs:complexType defaultAttributesApply=`, class: FailureInvalid, code: invalidSchemaCompositionCode},
		{name: "global named type plus inline type", body: `<xs:complexType name="Named"/><xs:element name="root" type="t:Named"><xs:complexType defaultAttributesApply="true"/></xs:element>`, marker: `<xs:complexType defaultAttributesApply=`, class: FailureInvalid, code: invalidSchemaCompositionCode},
		{name: "alternative inline complex type", body: `<xs:element name="root"><xs:alternative><xs:complexType defaultAttributesApply="true"/></xs:alternative></xs:element>`, marker: `<xs:alternative>`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, spec: func(v XSDVersion) string { return newSchemaSyntaxUnsupportedForVersion(Loc{}, "", v).SpecRef() }},
	}
	for _, policy := range []struct {
		name  string
		value LanguagePolicy
	}{{"Compatibility", Compatibility}, {"Strict11", Strict11}} {
		for _, test := range tests {
			t.Run(policy.name+"/"+test.name, func(t *testing.T) {
				root := prefix + test.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.value)
				if err == nil {
					t.Fatal("excluded shape returned a schema")
				}
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				loc := schemaMixedComplexLoc(t, root, test.marker)
				if d.Class() != test.class || d.Code() != test.code || d.Loc() != loc {
					t.Fatalf("diagnostic = %s, want %s/%s at %s", d, test.class, test.code, loc)
				}
				related := []Loc(nil)
				if test.related != "" {
					related = []Loc{schemaMixedComplexLoc(t, root, test.related)}
				}
				if !reflect.DeepEqual(d.Related(), related) {
					t.Fatalf("related = %v, want %v", d.Related(), related)
				}
				if test.class == FailureUnsupported && (d.SpecRef() != test.spec(XSDVersion11) || !errors.Is(err, ErrUnsupported)) {
					t.Fatalf("unsupported provenance = %s (%v)", d, err)
				}
				if test.class == FailureInvalid && (d.SpecRef() != "" || errors.Is(err, ErrUnsupported)) {
					t.Fatalf("invalid shape acquired unsupported provenance: %s", d)
				}
				if test.cause != nil && !errors.Is(err, test.cause) {
					t.Fatalf("lost exclusion cause %v: %v", test.cause, err)
				}
			})
		}
	}
}

//nolint:gocognit // Semantic failures precede valid zero-occurrence omission.
func TestInlineDefaultAttributesApplyZeroOccurrenceGates(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		t.Run(string(policy)+"/valid local zero", func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="Owner"><xs:sequence><xs:element name="child" minOccurs="0" maxOccurs="0"><xs:complexType defaultAttributesApply="true"/></xs:element></xs:sequence></xs:complexType></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatal(err)
			}
			definition, ok := schema.Components()[0].ComplexTypeDefinition()
			if !ok {
				t.Fatal("named owner missing")
			}
			sequence, ok := definition.Particle().(SequenceParticle)
			if !ok || len(sequence.Particles()) != 0 {
				t.Fatalf("zero local inline particle = %T", definition.Particle())
			}
		})
		for _, test := range []struct {
			name, body, marker, code string
			class                    FailureClass
			cause                    error
			spec                     string
		}{
			{"invalid occurrence", `<xs:sequence><xs:element name="child" minOccurs="bad" maxOccurs="0"><xs:complexType defaultAttributesApply="true"/></xs:element></xs:sequence>`, `minOccurs="bad"`, invalidSchemaCompositionCode, FailureInvalid, nil, schemaParticleOccurrenceDatatypeSpecRef(XSDVersion11)},
			{"invalid inline child", `<xs:sequence><xs:element name="child" minOccurs="0" maxOccurs="0"><xs:complexType defaultAttributesApply="true" name="Bad"/></xs:element></xs:sequence>`, `name="Bad"`, invalidSchemaCompositionCode, FailureInvalid, nil, ""},
			{"unresolved zero ref", `<xs:sequence><xs:element ref="t:Missing" minOccurs="0" maxOccurs="0"/></xs:sequence>`, `ref="t:Missing"`, diagnosticSchemaElementReferenceUnresolvedCode, FailureInvalid, errSchemaElementReferenceUnresolved, schemaElementReferenceSpecRef(XSDVersion11)},
		} {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				root := inlineDefaultAttributesApplySchema(test.body, schemaDefaultAttributesApplyValues()[1], XSDVersion11)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err == nil {
					t.Fatal("semantic failure was omitted")
				}
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != test.class || d.Code() != test.code || d.Loc() != schemaMixedComplexLoc(t, root, test.marker) || d.SpecRef() != test.spec || len(d.Related()) != 0 || errors.Is(err, ErrUnsupported) {
					t.Fatalf("zero occurrence diagnostic = %s (%v)", d, err)
				}
				if test.cause != nil && !errors.Is(err, test.cause) {
					t.Fatalf("lost reference cause: %v", err)
				}
				if test.name == "invalid occurrence" {
					cause := d.Unwrap()
					if cause == nil {
						t.Fatal("invalid occurrence lost located lexical cause")
					}
					lexical := requireDiagnostic(t, cause)
					if lexical.Code() != InvalidIntegerLexicalCode || lexical.Loc() != d.Loc() {
						t.Fatalf("invalid occurrence cause = %s", lexical)
					}
				}
			})
		}
	}
}

//nolint:gocognit // Validator success/failure and generator failure are independent consumers.
func TestInlineDefaultAttributesApplyConsumersMatchOmission(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		var baselineInvalid inlineMixedDiagnostic
		var baselineGeneration inlineMixedDiagnostic
		for index, value := range schemaDefaultAttributesApplyValues() {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="1.1">` + "\n" +
				`<xs:element name="root">` + "\n" +
				`<xs:complexType` + schemaDefaultAttributesApplyAttribute(value) + `>` + "\n" +
				`<xs:sequence><xs:element name="value" type="xs:precisionDecimal"/></xs:sequence>` + "\n" +
				`</xs:complexType></xs:element></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatalf("%s/%s parse: %v", policy, value.name, err)
			}
			if err := ValidateInstance(schema, "valid.xml", io.NopCloser(strings.NewReader(`<root><value>7</value></root>`))); err != nil {
				t.Fatalf("%s/%s valid instance: %v", policy, value.name, err)
			}
			invalid := inlineMixedError(t, ValidateInstance(schema, "invalid.xml", io.NopCloser(strings.NewReader(`<root><value>bad</value></root>`))))
			if invalid.class != string(FailureInvalid) || invalid.code != diagnosticPrecisionDecimalLexicalCode || invalid.loc != mustTestLoc(t, "invalid.xml", 1, 14) || invalid.unsupported {
				t.Fatalf("%s/%s invalid instance = %#v", policy, value.name, invalid)
			}
			output, generationErr := GenerateGo(schema, "generated")
			generation := inlineMixedError(t, generationErr)
			if output != nil || generation.class != string(FailureUnsupported) || generation.code != diagnosticCodegenUnsupported || generation.loc != schemaMixedComplexLoc(t, root, `<xs:element name="root"`) || !generation.unsupported {
				t.Fatalf("%s/%s generation = %d bytes/%#v", policy, value.name, len(output), generation)
			}
			if index == 0 {
				baselineInvalid, baselineGeneration = invalid, generation
				continue
			}
			if !reflect.DeepEqual(invalid, baselineInvalid) || !reflect.DeepEqual(generation, baselineGeneration) {
				t.Fatalf("%s/%s changed consumer diagnostics: invalid %#v/%#v, generation %#v/%#v", policy, value.name, invalid, baselineInvalid, generation, baselineGeneration)
			}
		}
	}
}

func TestInlineDefaultAttributesApplyDoesNotAdmitSchemaDefaultAttributes(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" defaultAttributes="Defaults">` + "\n" +
			`<xs:element name="root"><xs:complexType defaultAttributesApply="true"/></xs:element></xs:schema>`
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
		if err == nil {
			t.Fatal("schema defaultAttributes returned a schema")
		}
		assertZeroSchema(t, schema)
		d := requireDiagnostic(t, err)
		if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != schemaMixedComplexLoc(t, root, `defaultAttributes="Defaults"`) || len(d.Related()) != 0 || d.SpecRef() != newSchemaSyntaxUnsupported(Loc{}, "").SpecRef() || !errors.Is(err, ErrUnsupported) {
			t.Fatalf("schema defaultAttributes diagnostic = %s (%v)", d, err)
		}
	}
}

//nolint:gocognit // Source admission is checked at root, chameleon, and imported declarations.
func TestInlineDefaultAttributesApplyGraphSourcesMatchOmission(t *testing.T) {
	for _, policy := range []LanguagePolicy{Compatibility, Strict11} {
		for _, source := range []struct {
			name, element, namespace string
		}{{"root", "Root", "urn:root"}, {"chameleon", "Included", "urn:root"}, {"import", "Imported", "urn:direct"}} {
			var baseline schemaFinalDefaultSnapshot
			for index, value := range schemaDefaultAttributesApplyValues() {
				rootAttribute, includedAttribute, importedAttribute := "", "", ""
				switch source.name {
				case "root":
					rootAttribute = schemaDefaultAttributesApplyAttribute(value)
				case "chameleon":
					includedAttribute = schemaDefaultAttributesApplyAttribute(value)
				case "import":
					importedAttribute = schemaDefaultAttributesApplyAttribute(value)
				}
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root">` + "\n" +
					`<xs:include schemaLocation="included.xsd"/>` + "\n" +
					`<xs:import namespace="urn:direct" schemaLocation="imported.xsd"/>` + "\n" +
					`<xs:element name="Root"><xs:complexType` + rootAttribute + `/></xs:element></xs:schema>`
				fixtures := map[string]discoveryFixture{
					"included.xsd": {id: "included.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="Included"><xs:complexType` + includedAttribute + `/></xs:element></xs:schema>`},
					"imported.xsd": {id: "imported.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:direct"><xs:element name="Imported"><xs:complexType` + importedAttribute + `/></xs:element></xs:schema>`},
				}
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy)
				if err != nil {
					t.Fatalf("%s/%s/%s: %v", policy, source.name, value.name, err)
				}
				if len(schema.Documents()) != 3 || schema.Documents()[0].Source() != "root.xsd" || schema.Documents()[1].Source() != "included.xsd" || schema.Documents()[2].Source() != "imported.xsd" {
					t.Fatalf("document discovery order = %#v", schema.Documents())
				}
				name := mustTestQName(t, source.namespace, source.element)
				found := schema.FindKind(ComponentKindElementDeclaration, name)
				if len(found) != 1 {
					t.Fatalf("%s/%s found = %d, want one", source.name, value.name, len(found))
				}
				declaration, ok := found[0].Element()
				if !ok {
					t.Fatal("queried element has no declaration")
				}
				definition, ok := declaration.InlineComplexType()
				if !ok || definition.Loc().Source() != found[0].Document() {
					t.Fatalf("inline type source = %s; declaration source = %s", definition.Loc().Source(), found[0].Document())
				}
				current := snapshotSchemaForComponentKind(t, schema, name, ComponentKindElementDeclaration)
				if index == 0 {
					baseline = current
					continue
				}
				if !reflect.DeepEqual(current, baseline) {
					t.Fatalf("%s/%s changed graph public facts", source.name, value.name)
				}
			}
		}
	}
}
