package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

type inlineMixedFacts struct {
	components []ComponentID
	locations  []Loc
	walk       []ComponentID
	node       ComplexTypeID
	typeLoc    Loc
	base       QName
	baseLoc    Loc
	derivation ComplexTypeDerivation
	particle   []inlineMixedParticleFact
	uses       []inlineMixedUseFact
	scalarBase QName
	scalarLoc  Loc
	valid      inlineMixedDiagnostic
	invalid    inlineMixedDiagnostic
	generated  string
	generation inlineMixedDiagnostic
}

type inlineMixedParticleFact struct {
	kind, name, rangeValue string
	loc, refLoc            Loc
	target                 ComponentID
}

type inlineMixedUseFact struct {
	name, kind  string
	loc, refLoc Loc
	target      ComponentID
}

type inlineMixedDiagnostic struct {
	code, class, spec string
	loc               Loc
	related           []Loc
	unsupported       bool
}

func inlineMixedError(t *testing.T, err error) inlineMixedDiagnostic {
	t.Helper()
	if err == nil {
		return inlineMixedDiagnostic{}
	}
	diagnostic := requireDiagnostic(t, err)
	return inlineMixedDiagnostic{code: diagnostic.Code(), class: string(diagnostic.Class()), spec: diagnostic.SpecRef(), loc: diagnostic.Loc(), related: diagnostic.Related(), unsupported: errors.Is(err, ErrUnsupported)}
}

func inlineMixedSchema(body, value, version string) string {
	attribute := schemaMixedComplexAttribute(schemaMixedComplexValue{present: value != "", value: value})
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" elementFormDefault="qualified" version="` + version + `">` +
		`<xs:element name="root"><xs:complexType` + attribute + `>` + body + `</xs:complexType></xs:element>` +
		`<xs:element name="target" type="xs:integer"/><xs:attribute name="global" type="xs:boolean"/>` +
		`<xs:complexType name="Base"/></xs:schema>`
}

//nolint:gocognit // Observe one completed schema through all public fact and consumer views.
func inlineMixedSnapshot(t *testing.T, root string, policy LanguagePolicy) inlineMixedFacts {
	t.Helper()
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
	if err != nil {
		t.Fatalf("parse inline mixed fixture: %v", err)
	}
	components := schema.Components()
	if len(components) != 4 {
		t.Fatalf("components = %d, want 4", len(components))
	}
	result := inlineMixedFacts{}
	for _, component := range components {
		result.components = append(result.components, component.ID())
		result.locations = append(result.locations, component.Loc())
	}
	if err := schema.Walk(func(component Component) error {
		result.walk = append(result.walk, component.ID())
		return nil
	}); err != nil {
		t.Fatalf("walk: %v", err)
	}
	if !reflect.DeepEqual(result.walk, result.components) {
		t.Fatalf("walk order = %v, components = %v", result.walk, result.components)
	}
	rootElement := auxiliaryElement(t, schema, "root")
	definition, ok := rootElement.InlineComplexType()
	if !ok || definition.Component() != (Component{}) || definition.ID() != (ComponentID{}) {
		t.Fatal("root lost its anonymous complex view")
	}
	result.node, ok = definition.NodeID()
	if !ok || result.node.IsZero() {
		t.Fatal("root lost anonymous type identity")
	}
	result.typeLoc = definition.Loc()
	if result.typeLoc != schemaMixedComplexLoc(t, root, `<xs:complexType`) {
		t.Fatalf("anonymous type location = %s", result.typeLoc)
	}
	result.base, result.baseLoc, result.derivation = definition.Base(), definition.BaseLoc(), definition.Derivation()
	inlineMixedCollectParticle(&result.particle, definition.Particle())
	for _, use := range definition.AttributeUses() {
		fact := inlineMixedUseFact{name: use.Name().Local(), kind: string(use.Use()), loc: use.Loc()}
		if reference, ok := use.(AttributeReferenceUse); ok {
			fact.refLoc, fact.target = reference.RefLoc(), reference.TargetID()
		}
		result.uses = append(result.uses, fact)
	}
	uses := definition.AttributeUses()
	if len(uses) > 0 {
		uses[0] = nil
		if definition.AttributeUses()[0] == nil {
			t.Fatal("mutating attribute uses changed schema")
		}
	}
	if scalar, ok := definition.SimpleContentExtension(); ok {
		result.scalarBase, result.scalarLoc = scalar.Base(), scalar.BaseLoc()
	}
	result.valid = inlineMixedError(t, ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"/>`))))
	result.invalid = inlineMixedError(t, ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"><unexpected/></root>`))))
	generated, generationErr := GenerateGo(schema, "generated")
	result.generated, result.generation = string(generated), inlineMixedError(t, generationErr)
	if generationErr != nil && generated != nil {
		t.Fatal("generation error returned partial output")
	}
	return result
}

//nolint:gocognit // Assert identity, order, bounds, locations, and copy isolation together.
func TestInlineMixedFalseRetainsExactPublicFactsAndCopies(t *testing.T) {
	body := `<xs:sequence minOccurs="0" maxOccurs="2"><xs:element ref="t:target" minOccurs="0" maxOccurs="3"/><xs:element name="local" type="xs:integer"/></xs:sequence><xs:attribute name="flag" type="xs:boolean" use="required"/><xs:attribute ref="t:global"/>`
	root := inlineMixedSchema(body, "false", "1.1")
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	components := schema.Components()
	if len(components) != 4 || components[0].Name().Local() != "root" || components[1].Name().Local() != "target" || components[2].Name().Local() != "global" || components[3].Name().Local() != "Base" {
		t.Fatalf("component declaration order = %#v", components)
	}
	definition, ok := auxiliaryElement(t, schema, "root").InlineComplexType()
	if !ok {
		t.Fatal("missing anonymous complex type")
	}
	sequence, ok := definition.Particle().(SequenceParticle)
	if !ok || sequence.Occurrences().String() != "0/2" || sequence.Loc() != schemaMixedComplexLoc(t, root, `<xs:sequence`) {
		t.Fatalf("sequence facts = %T, %s", definition.Particle(), definition.Particle().Occurrences())
	}
	children := sequence.Particles()
	if len(children) != 2 {
		t.Fatalf("sequence children = %d", len(children))
	}
	reference, ok := children[0].(ElementReferenceParticle)
	if !ok || reference.Name().Local() != "target" || reference.TargetID() != components[1].ID() || reference.RefLoc() != schemaMixedComplexLoc(t, root, `ref="t:target"`) || reference.Occurrences().String() != "0/3" {
		t.Fatalf("first child reference = %#v", children[0])
	}
	local, ok := children[1].(ElementParticle)
	if !ok || local.Name().Local() != "local" || local.Loc() != schemaMixedComplexLoc(t, root, `<xs:element name="local"`) || local.Occurrences().String() != "1/1" {
		t.Fatalf("second child local = %#v", children[1])
	}
	uses := definition.AttributeUses()
	if len(uses) != 2 || uses[0].Name().Local() != "flag" || uses[0].Use() != AttributeUseRequired || uses[0].Loc() != schemaMixedComplexLoc(t, root, `<xs:attribute name="flag"`) {
		t.Fatalf("ordered attribute uses = %#v", uses)
	}
	globalUse, ok := uses[1].(AttributeReferenceUse)
	if !ok || globalUse.TargetID() != components[2].ID() || globalUse.RefLoc() != schemaMixedComplexLoc(t, root, `ref="t:global"`) {
		t.Fatalf("global attribute use = %#v", uses[1])
	}
	components[0] = Component{}
	children[0] = nil
	uses[0] = nil
	fresh, ok := definition.Particle().(SequenceParticle)
	if !ok || schema.Components()[0].Kind() != ComponentKindElementDeclaration || len(fresh.Particles()) != 2 || definition.AttributeUses()[0] == nil {
		t.Fatal("mutating public result changed schema facts")
	}
}

func inlineMixedCollectParticle(facts *[]inlineMixedParticleFact, particle Particle) {
	if particle == nil {
		return
	}
	fact := inlineMixedParticleFact{rangeValue: particle.Occurrences().String(), loc: particle.Loc()}
	switch typed := particle.(type) {
	case SequenceParticle:
		fact.kind = "sequence"
		*facts = append(*facts, fact)
		for _, child := range typed.Particles() {
			inlineMixedCollectParticle(facts, child)
		}
	case ChoiceParticle:
		fact.kind = "choice"
		*facts = append(*facts, fact)
		for _, child := range typed.Alternatives() {
			inlineMixedCollectParticle(facts, child)
		}
	case ElementParticle:
		fact.kind, fact.name = "element", typed.Name().Local()
		*facts = append(*facts, fact)
	case ElementReferenceParticle:
		fact.kind, fact.name, fact.refLoc, fact.target = "ref", typed.Name().Local(), typed.RefLoc(), typed.TargetID()
		*facts = append(*facts, fact)
	}
}

//nolint:gocognit // Each supported anonymous body is compared through the same public observations.
func TestInlineMixedFalseMatchesOmissionForSupportedBodies(t *testing.T) {
	bodies := []struct{ name, xml string }{
		{"empty", ""},
		{"attribute only", `<xs:attribute name="flag" type="xs:boolean" use="required"/><xs:attribute ref="t:global"/>`},
		{"sequence", `<xs:sequence minOccurs="0" maxOccurs="2"><xs:element ref="t:target" minOccurs="0" maxOccurs="3"/><xs:element name="local" type="xs:integer"/></xs:sequence>`},
		{"sequence attributes", `<xs:sequence minOccurs="0" maxOccurs="2"><xs:element ref="t:target" minOccurs="0" maxOccurs="3"/><xs:element name="local" type="xs:integer"/></xs:sequence><xs:attribute name="flag" type="xs:boolean" use="required"/><xs:attribute ref="t:global"/>`},
		{"choice", `<xs:choice><xs:element name="first" type="xs:integer"/><xs:element name="second" type="xs:decimal"/></xs:choice>`},
		{"simple content", `<xs:simpleContent><xs:extension base="xs:string"><xs:attribute name="flag" type="xs:boolean"/></xs:extension></xs:simpleContent>`},
		{"complex extension", `<xs:complexContent><xs:extension base="t:Base"><xs:sequence><xs:element ref="t:target"/></xs:sequence></xs:extension></xs:complexContent>`},
		{"complex restriction", `<xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent>`},
	}
	for _, policy := range schemaMixedComplexPolicies() {
		for _, version := range schemaMixedComplexVersions() {
			for _, body := range bodies {
				t.Run(policy.name+"/"+version.name+"/"+body.name, func(t *testing.T) {
					baseline := inlineMixedSnapshot(t, inlineMixedSchema(body.xml, "", string(version.version)), policy.policy)
					for _, value := range []string{"false", "0"} {
						actual := inlineMixedSnapshot(t, inlineMixedSchema(body.xml, value, string(version.version)), policy.policy)
						if !reflect.DeepEqual(actual, baseline) {
							t.Fatalf("mixed=%q changed public facts: got %#v, omitted %#v", value, actual, baseline)
						}
					}
				})
			}
		}
	}
}

//nolint:gocognit // Boolean lexical and unsupported exits share the same inline attribute boundary.
func TestInlineMixedAttributeDiagnosticExits(t *testing.T) {
	for _, policy := range schemaMixedComplexPolicies() {
		wantVersion := XSDVersion11
		if policy.policy == Strict10 {
			wantVersion = XSDVersion10
		}
		for _, version := range schemaMixedComplexVersions() {
			for _, value := range []string{"true", "1", "", "00", "False", "maybe"} {
				t.Run(policy.name+"/"+version.name+"/"+value, func(t *testing.T) {
					attribute := schemaMixedComplexAttribute(schemaMixedComplexValue{present: true, value: value})
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="` + string(version.version) + `"><xs:element name="root"><xs:complexType` + attribute + `><xs:sequence/></xs:complexType></xs:element></xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
					if err == nil {
						t.Fatal("unsupported or malformed mixed unexpectedly succeeded")
					}
					assertZeroSchema(t, schema)
					diagnostic := requireDiagnostic(t, err)
					wantLoc := schemaMixedComplexLoc(t, root, `mixed="`+value+`"`)
					if diagnostic.Loc() != wantLoc || len(diagnostic.Related()) != 0 {
						t.Fatalf("mixed diagnostic location/related = %s/%v, want %s/none", diagnostic.Loc(), diagnostic.Related(), wantLoc)
					}
					if value == "true" || value == "1" {
						wantSpec := newSchemaSyntaxUnsupportedForVersion(Loc{}, "", wantVersion).SpecRef()
						if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Feature() != FeatureSchemaSyntax || diagnostic.SpecRef() != wantSpec || diagnostic.Unwrap() != nil || !errors.Is(err, ErrUnsupported) {
							t.Fatalf("true mixed diagnostic = %s; want unsupported with %s", diagnostic, wantSpec)
						}
						return
					}
					if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.Feature() != "" || diagnostic.SpecRef() != "" || errors.Is(err, ErrUnsupported) {
						t.Fatalf("malformed mixed diagnostic = %s, want invalid Boolean", diagnostic)
					}
					cause := diagnostic.Unwrap()
					if cause == nil {
						t.Fatal("malformed mixed lost lexical validation cause")
					}
					lexical := requireDiagnostic(t, cause)
					if lexical.Class() != FailureInvalid || lexical.Code() != invalidSchemaCompositionCode || lexical.Loc() != wantLoc || lexical.Message() != `attribute "mixed" has an invalid boolean value` || lexical.Unwrap() != nil {
						t.Fatalf("malformed mixed lexical cause = %s, want original located Boolean diagnostic", lexical)
					}
				})
			}
		}
	}
}

//nolint:gocognit // Exercise both collapsed false lexemes under every policy and edition label.
func TestInlineMixedWhitespaceCollapsedFalse(t *testing.T) {
	for _, policy := range schemaMixedComplexPolicies() {
		for _, version := range schemaMixedComplexVersions() {
			for _, value := range []string{"&#x9;false&#xA;", "&#xA;0&#x9;"} {
				t.Run(policy.name+"/"+version.name+"/"+value, func(t *testing.T) {
					root := inlineMixedSchema(`<xs:sequence/>`, value, string(version.version))
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
					if err != nil {
						t.Fatalf("padded false mixed: %v", err)
					}
					definition, ok := auxiliaryElement(t, schema, "root").InlineComplexType()
					if !ok || definition.Particle() == nil {
						t.Fatal("padded false lost inline sequence")
					}
				})
			}
		}
	}
}

func TestInlineMixedLaterInvalidChildPrecedesUnsupportedCandidate(t *testing.T) {
	for _, policy := range schemaMixedComplexPolicies() {
		for _, value := range []string{"true", "1"} {
			root := inlineMixedSchema(`<xs:sequence><xs:element name="bad" type="xs:integer" abstract="true"/></xs:sequence>`, value, "1.1")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
			if err == nil {
				t.Fatal("later invalid child unexpectedly succeeded")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.Loc() != schemaMixedComplexLoc(t, root, `abstract="true"`) || len(diagnostic.Related()) != 0 || errors.Is(err, ErrUnsupported) {
				t.Fatalf("later invalid child diagnostic = %s, want invalid at abstract", diagnostic)
			}
		}
	}
}

//nolint:gocognit // The owner, local, reference, and zero-occurrence exclusions share a diagnostic contract.
func TestInlineMixedFalseKeepsExcludedShapes(t *testing.T) {
	const prefix = `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" version="1.1">`
	tests := []struct {
		name, body, marker, code string
		class                    FailureClass
		spec                     func(XSDVersion) string
		cause                    error
		related                  string
	}{
		{
			name: "direct group inline owner", body: `<xs:element name="root"><xs:complexType mixed="false"><xs:group ref="t:Fields"/></xs:complexType></xs:element><xs:group name="Fields"><xs:sequence/></xs:group>`,
			marker: `ref="t:Fields"`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, spec: func(XSDVersion) string { return newSchemaSyntaxUnsupported(Loc{}, "").SpecRef() },
		},
		{
			name: "direct zero group inline owner", body: `<xs:element name="root"><xs:complexType mixed="false"><xs:group ref="t:Fields" minOccurs="0" maxOccurs="0"/></xs:complexType></xs:element><xs:group name="Fields"><xs:sequence/></xs:group>`,
			marker: `ref="t:Fields"`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, spec: func(XSDVersion) string { return newSchemaSyntaxUnsupported(Loc{}, "").SpecRef() },
		},
		{
			name: "direct all inline owner", body: `<xs:element name="root"><xs:complexType mixed="false"><xs:all><xs:element name="v" type="xs:integer"/></xs:all></xs:complexType></xs:element>`,
			marker: `<xs:all>`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, spec: func(XSDVersion) string { return newSchemaSyntaxUnsupported(Loc{}, "").SpecRef() },
		},
		{
			name: "grouped extension anonymous owner", body: `<xs:element name="root"><xs:complexType mixed="false"><xs:complexContent><xs:extension base="t:Base"><xs:group ref="t:Fields"/><xs:attribute name="flag" type="xs:boolean"/></xs:extension></xs:complexContent></xs:complexType></xs:element><xs:group name="Fields"><xs:sequence/></xs:group><xs:complexType name="Base"/>`,
			marker: `<xs:extension`, related: `<xs:complexType mixed=`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, spec: schemaComplexContentExtensionSpecRef, cause: errSchemaGroupedExtensionAnonymousOwner,
		},
		{
			name: "local inline child sequence", body: `<xs:complexType name="Owner"><xs:sequence><xs:element name="child"><xs:complexType mixed="false"/></xs:element></xs:sequence></xs:complexType>`,
			marker: `<xs:complexType mixed=`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, spec: func(XSDVersion) string { return newSchemaSyntaxUnsupported(Loc{}, "").SpecRef() },
		},
		{
			name: "all member inline child", body: `<xs:complexType name="Owner"><xs:all><xs:element name="child"><xs:complexType mixed="false"/></xs:element></xs:all></xs:complexType>`,
			marker: `<xs:complexType mixed=`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, spec: func(v XSDVersion) string { return newSchemaSyntaxUnsupportedForVersion(Loc{}, "", v).SpecRef() },
		},
		{
			name: "all member zero", body: `<xs:complexType name="Owner"><xs:all><xs:element name="child" minOccurs="0" maxOccurs="0"><xs:complexType mixed="false"/></xs:element></xs:all></xs:complexType>`,
			marker: `<xs:complexType mixed=`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, spec: schemaAllLimitedSpecRef, cause: errSchemaAllMemberInlineComplex,
		},
		{
			name: "local ref plus inline type", body: `<xs:element name="target" type="xs:integer"/><xs:complexType name="Owner"><xs:sequence><xs:element ref="t:target"><xs:complexType mixed="false"/></xs:element></xs:sequence></xs:complexType>`,
			marker: `<xs:complexType mixed=`, class: FailureInvalid, code: invalidSchemaCompositionCode,
		},
		{
			name: "global named type plus inline type", body: `<xs:complexType name="Named"/><xs:element name="root" type="t:Named"><xs:complexType mixed="false"/></xs:element>`,
			marker: `<xs:complexType mixed=`, class: FailureInvalid, code: invalidSchemaCompositionCode,
		},
	}
	for _, policy := range schemaMixedComplexPolicies() {
		version := XSDVersion11
		if policy.policy == Strict10 {
			version = XSDVersion10
		}
		for _, test := range tests {
			t.Run(policy.name+"/"+test.name, func(t *testing.T) {
				root := prefix + test.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
				if err == nil {
					t.Fatal("excluded form unexpectedly succeeded")
				}
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantLoc := schemaMixedComplexLoc(t, root, test.marker)
				if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != wantLoc {
					t.Fatalf("diagnostic = %s, want %s/%s at %s", diagnostic, test.class, test.code, wantLoc)
				}
				wantRelated := []Loc(nil)
				if test.related != "" {
					wantRelated = []Loc{schemaMixedComplexLoc(t, root, test.related)}
				}
				if !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
					t.Fatalf("related = %v, want %v", diagnostic.Related(), wantRelated)
				}
				if test.class == FailureUnsupported {
					if diagnostic.SpecRef() != test.spec(version) || !errors.Is(err, ErrUnsupported) {
						t.Fatalf("unsupported provenance = %s/%v, want %s", diagnostic.SpecRef(), err, test.spec(version))
					}
				}
				if test.class != FailureUnsupported && (diagnostic.SpecRef() != "" || errors.Is(err, ErrUnsupported)) {
					t.Fatalf("invalid form acquired unsupported provenance: %s", diagnostic)
				}
				if test.cause != nil && !errors.Is(err, test.cause) {
					t.Fatalf("lost exclusion cause %v: %v", test.cause, err)
				}
			})
		}
	}
}

//nolint:gocognit // Compare omission with malformed, unresolved, and policy-gated zero terms.
func TestInlineMixedFalseKeepsZeroOccurrenceSemanticGates(t *testing.T) {
	for _, policy := range schemaMixedComplexPolicies() {
		version := XSDVersion11
		if policy.policy == Strict10 {
			version = XSDVersion10
		}
		t.Run(policy.name+"/valid omitted local inline", func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="Owner"><xs:sequence><xs:element name="child" minOccurs="0" maxOccurs="0"><xs:complexType mixed="false"/></xs:element></xs:sequence></xs:complexType></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
			if err != nil {
				t.Fatalf("valid omitted local inline: %v", err)
			}
			definition, ok := schema.Components()[0].ComplexTypeDefinition()
			if !ok {
				t.Fatal("named owner missing")
			}
			sequence, ok := definition.Particle().(SequenceParticle)
			if !ok || len(sequence.Particles()) != 0 {
				t.Fatalf("zero local inline particle = %T/%d", definition.Particle(), len(sequence.Particles()))
			}
		})
		t.Run(policy.name+"/invalid zero local inline", func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="Owner"><xs:sequence><xs:element name="child" minOccurs="0" maxOccurs="0"><xs:complexType mixed="false" name="Bad"/></xs:element></xs:sequence></xs:complexType></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
			if err == nil {
				t.Fatal("invalid omitted local inline succeeded")
			}
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.Loc() != schemaMixedComplexLoc(t, root, `name="Bad"`) || len(d.Related()) != 0 || errors.Is(err, ErrUnsupported) {
				t.Fatalf("invalid zero inline = %s", d)
			}
		})
		t.Run(policy.name+"/unresolved zero reference", func(t *testing.T) {
			root := inlineMixedSchema(`<xs:sequence><xs:element ref="t:Missing" minOccurs="0" maxOccurs="0"/></xs:sequence>`, "false", "1.1")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
			if err == nil {
				t.Fatal("unresolved zero ref succeeded")
			}
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaElementReferenceUnresolvedCode || d.Loc() != schemaMixedComplexLoc(t, root, `ref="t:Missing"`) || len(d.Related()) != 0 || d.SpecRef() != schemaElementReferenceSpecRef(version) || !errors.Is(err, errSchemaElementReferenceUnresolved) {
				t.Fatalf("unresolved zero ref = %s (%v)", d, err)
			}
		})
	}
	t.Run("Strict10/policy zero type", func(t *testing.T) {
		root := inlineMixedSchema(`<xs:sequence><xs:element name="v" type="xs:precisionDecimal" minOccurs="0" maxOccurs="0"/></xs:sequence>`, "false", "1.1")
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
		if err == nil {
			t.Fatal("Strict10 admitted zero precisionDecimal")
		}
		assertZeroSchema(t, schema)
		d := requireDiagnostic(t, err)
		if d.Class() != FailureUnsupported || d.Code() != diagnosticSchemaPrecisionDecimalVersionCode || d.Loc() != schemaMixedComplexLoc(t, root, `type="xs:precisionDecimal"`) || len(d.Related()) != 0 || !errors.Is(err, ErrUnsupported) || !errors.Is(err, errLanguagePolicyMismatch) {
			t.Fatalf("Strict10 zero policy diagnostic = %s (%v)", d, err)
		}
	})
}

func TestInlineMixedFalseKeepsZeroAllOwnerBoundary(t *testing.T) {
	for _, policy := range schemaMixedComplexPolicies() {
		root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="root"><xs:complexType mixed="false"><xs:all minOccurs="0" maxOccurs="0"><xs:element name="v" type="xs:integer"/></xs:all></xs:complexType></xs:element></xs:schema>`
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
		if err == nil {
			t.Fatalf("%s admitted zero all on inline owner", policy.name)
		}
		assertZeroSchema(t, schema)
		d := requireDiagnostic(t, err)
		if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || !errors.Is(err, ErrUnsupported) || len(d.Related()) != 0 {
			t.Fatalf("%s zero all owner diagnostic = %s", policy.name, d)
		}
		if policy.policy == Strict10 {
			if d.Loc() != schemaMixedComplexLoc(t, root, `maxOccurs="0"`) || d.SpecRef() != newSchemaSyntaxUnsupportedForVersion(Loc{}, "", XSDVersion11).SpecRef() || !errors.Is(err, errLanguagePolicyMismatch) {
				t.Fatalf("Strict10 zero all policy diagnostic = %s", d)
			}
			continue
		}
		if d.Loc() != schemaMixedComplexLoc(t, root, `minOccurs="0"`) || d.SpecRef() != newSchemaSyntaxUnsupported(Loc{}, "").SpecRef() {
			t.Fatalf("%s zero all owner diagnostic = %s", policy.name, d)
		}
	}
}

//nolint:gocognit // Test validator and generator independently for every accepted false spelling.
func TestInlineMixedFalseKeepsValidatorAndGeneratorBoundaries(t *testing.T) {
	for _, policy := range schemaMixedComplexPolicies() {
		var wantValidation, wantGeneration inlineMixedDiagnostic
		for index, value := range []string{"", "false", "0"} {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" version="1.1">` +
				`<xs:element name="root"><xs:complexType` + schemaMixedComplexAttribute(schemaMixedComplexValue{present: value != "", value: value}) + `><xs:sequence><xs:element ref="t:target"/></xs:sequence></xs:complexType></xs:element>` +
				`<xs:element name="target" type="xs:integer"/></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
			if err != nil {
				t.Fatalf("%s mixed=%q parse: %v", policy.name, value, err)
			}
			validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"><target>7</target></root>`)))
			validation := inlineMixedError(t, validationErr)
			if validation.class != string(FailureUnsupported) || validation.code != UnsupportedInstanceValidationCode || validation.loc != mustTestLoc(t, "instance.xml", 1, 1) || !validation.unsupported {
				t.Fatalf("%s mixed=%q validator = %#v", policy.name, value, validation)
			}
			output, generationErr := GenerateGo(schema, "generated")
			generation := inlineMixedError(t, generationErr)
			if output != nil || generation.class != string(FailureUnsupported) || generation.code != diagnosticCodegenUnsupported || generation.loc != schemaMixedComplexLoc(t, root, `<xs:element name="root"`) || !generation.unsupported {
				t.Fatalf("%s mixed=%q generator = %d bytes/%#v", policy.name, value, len(output), generation)
			}
			if index == 0 {
				wantValidation, wantGeneration = validation, generation
				continue
			}
			if !reflect.DeepEqual(validation, wantValidation) || !reflect.DeepEqual(generation, wantGeneration) {
				t.Fatalf("%s mixed=%q changed consumer diagnostics: validation %#v/%#v, generation %#v/%#v", policy.name, value, validation, wantValidation, generation, wantGeneration)
			}
		}
	}
}

//nolint:gocognit // Compare the XSD 1.1 agreement rule with the Strict10 mixed-content boundary.
func TestInlineMixedFalseKeepsComplexContentAgreement(t *testing.T) {
	for _, policy := range schemaMixedComplexPolicies() {
		for _, pair := range []struct{ outer, inner string }{{"false", "0"}, {"0", "false"}} {
			body := `<xs:complexContent mixed="` + pair.inner + `"><xs:extension base="t:Base"/></xs:complexContent>`
			root := inlineMixedSchema(body, pair.outer, "1.1")
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
			if err != nil {
				t.Fatalf("%s outer=%s inner=%s: %v", policy.name, pair.outer, pair.inner, err)
			}
			definition, ok := auxiliaryElement(t, schema, "root").InlineComplexType()
			if !ok || definition.Derivation() != ComplexTypeDerivationExtension {
				t.Fatal("equal mixed values lost inline extension")
			}
		}
		root := inlineMixedSchema(`<xs:complexContent mixed="true"><xs:extension base="t:Base"/></xs:complexContent>`, "false", "1.1")
		schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
		if err == nil {
			t.Fatalf("%s admitted contradictory mixed values", policy.name)
		}
		assertZeroSchema(t, schema)
		d := requireDiagnostic(t, err)
		wantLoc := schemaMixedComplexLoc(t, root, `mixed="true"`)
		if policy.policy == Strict10 {
			if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != schemaMixedComplexLoc(t, root, `<xs:complexContent`) || d.SpecRef() != schemaComplexContentExtensionSpecRef(XSDVersion10) || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("Strict10 inner mixed diagnostic = %s", d)
			}
			continue
		}
		if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.Loc() != wantLoc || len(d.Related()) != 0 || errors.Is(err, ErrUnsupported) {
			t.Fatalf("XSD 1.1 mixed disagreement = %s", d)
		}
	}
}
