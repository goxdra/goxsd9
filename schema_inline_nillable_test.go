package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func inlineNillableAttribute(value string) string {
	if value == "" {
		return ""
	}
	return ` nillable="` + value + `"`
}

func inlineNillableSchema(body, value string, version XSDVersion) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" elementFormDefault="qualified" version="` + string(version) + `">
<xs:element name="root"` + inlineNillableAttribute(value) + `>
<xs:complexType>` + body + `</xs:complexType>
</xs:element>
<xs:element name="target" type="xs:integer"/><xs:attribute name="global" type="xs:boolean"/><xs:complexType name="Base"/>
</xs:schema>`
}

//nolint:gocognit // Compare completed public facts and existing consumer outcomes for each admitted body.
func TestInlineNillableFalseMatchesOmission(t *testing.T) {
	bodies := []struct{ name, xml string }{
		{"empty", ""},
		{"attribute only", `<xs:attribute name="flag" type="xs:boolean" use="required"/><xs:attribute ref="t:global"/>`},
		{"sequence and refs", `<xs:sequence minOccurs="0" maxOccurs="2"><xs:element ref="t:target" minOccurs="0" maxOccurs="3"/><xs:element name="local" type="xs:integer"/></xs:sequence><xs:attribute name="flag" type="xs:boolean" use="required"/><xs:attribute ref="t:global"/>`},
		{"choice", `<xs:choice><xs:element name="first" type="xs:integer"/><xs:element name="second" type="xs:decimal"/></xs:choice>`},
		{"simple content", `<xs:simpleContent><xs:extension base="xs:string"><xs:attribute name="flag" type="xs:boolean"/></xs:extension></xs:simpleContent>`},
		{"complex extension", `<xs:complexContent><xs:extension base="t:Base"><xs:sequence><xs:element ref="t:target"/></xs:sequence></xs:extension></xs:complexContent>`},
		{"complex restriction", `<xs:complexContent><xs:restriction base="xs:anyType"><xs:anyAttribute namespace="##other" processContents="lax"/></xs:restriction></xs:complexContent>`},
	}
	for _, policy := range schemaMixedComplexPolicies() {
		for _, version := range schemaMixedComplexVersions() {
			for _, body := range bodies {
				t.Run(policy.name+"/"+version.name+"/"+body.name, func(t *testing.T) {
					omitted := inlineMixedSnapshot(t, inlineNillableSchema(body.xml, "", version.version), policy.policy)
					for _, value := range []string{"false", "0", "&#x9;false&#xA;", "&#xA;0&#x9;"} {
						root := inlineNillableSchema(body.xml, value, version.version)
						actual := inlineMixedSnapshot(t, root, policy.policy)
						if !reflect.DeepEqual(actual, omitted) {
							t.Fatalf("nillable=%q changed public facts or consumer outcomes: got %#v, omitted %#v", value, actual, omitted)
						}
						schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
						if err != nil {
							t.Fatalf("nillable=%q: %v", value, err)
						}
						declaration := auxiliaryElement(t, schema, "root")
						if declaration.IsNillable() || declaration.Loc() != schemaMixedComplexLoc(t, root, `<xs:element name="root"`) {
							t.Fatalf("nillable=%q changed effective fact or declaration Loc", value)
						}
						components := schema.Components()
						components[0] = Component{}
						if schema.Components()[0].Kind() != ComponentKindElementDeclaration {
							t.Fatal("mutating copied components changed schema")
						}
					}
				})
			}
		}
	}
}

//nolint:gocognit // The two valid unsupported exits and malformed lexical exits share an attribute boundary.
func TestInlineNillableDiagnosticExits(t *testing.T) {
	for _, policy := range schemaMixedComplexPolicies() {
		wantVersion := XSDVersion11
		if policy.policy == Strict10 {
			wantVersion = XSDVersion10
		}
		for _, version := range schemaMixedComplexVersions() {
			for _, value := range []string{"true", "1", "&#x9;true&#xA;", "&#xA;1&#x9;", "", "00", "False", "maybe"} {
				t.Run(policy.name+"/"+version.name+"/"+value, func(t *testing.T) {
					root := inlineNillableSchema(`<xs:sequence/>`, value, version.version)
					if value == "" {
						root = strings.Replace(root, `<xs:element name="root">`, `<xs:element name="root" nillable="">`, 1)
					}
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
					if err == nil {
						t.Fatal("excluded nillable spelling succeeded")
					}
					assertZeroSchema(t, schema)
					d := requireDiagnostic(t, err)
					wantLoc := schemaMixedComplexLoc(t, root, `nillable="`+value+`"`)
					if d.Loc() != wantLoc || len(d.Related()) != 0 {
						t.Fatalf("diagnostic location/related = %s/%v, want %s/none", d.Loc(), d.Related(), wantLoc)
					}
					if value == "true" || value == "1" || value == "&#x9;true&#xA;" || value == "&#xA;1&#x9;" {
						wantSpec := newSchemaSyntaxUnsupportedForVersion(Loc{}, "", wantVersion).SpecRef()
						if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.SpecRef() != wantSpec || d.Unwrap() != nil || !errors.Is(err, ErrUnsupported) {
							t.Fatalf("true nillable diagnostic = %s, want unsupported with %s", d, wantSpec)
						}
						return
					}
					if d.Class() != FailureInvalid || d.Code() != invalidSchemaCompositionCode || d.SpecRef() != "" || errors.Is(err, ErrUnsupported) {
						t.Fatalf("malformed nillable diagnostic = %s, want invalid Boolean", d)
					}
					lexical := requireDiagnostic(t, d.Unwrap())
					if lexical.Class() != FailureInvalid || lexical.Code() != invalidSchemaCompositionCode || lexical.Loc() != wantLoc || lexical.Message() != `attribute "nillable" has an invalid boolean value` || lexical.Unwrap() != nil {
						t.Fatalf("malformed nillable lexical cause = %s", lexical)
					}
				})
			}
		}
	}
}

//nolint:gocognit // Existing validator and generator contracts are compared independently.
func TestInlineNillableFalseKeepsConsumerOutcomes(t *testing.T) {
	for _, policy := range schemaMixedComplexPolicies() {
		var wantValidation, wantGeneration inlineMixedDiagnostic
		for index, value := range []string{"", "false", "0", "&#x9;false&#xA;"} {
			root := inlineNillableSchema(`<xs:sequence><xs:element ref="t:target"/></xs:sequence>`, value, XSDVersion11)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
			if err != nil {
				t.Fatalf("%s nillable=%q parse: %v", policy.name, value, err)
			}
			validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:root"><target>7</target></root>`)))
			validation := inlineMixedError(t, validationErr)
			if validation.class != string(FailureUnsupported) || validation.code != UnsupportedInstanceValidationCode || validation.loc != mustTestLoc(t, "instance.xml", 1, 1) || !validation.unsupported {
				t.Fatalf("%s nillable=%q validator = %#v", policy.name, value, validation)
			}
			output, generationErr := GenerateGo(schema, "generated")
			generation := inlineMixedError(t, generationErr)
			if output != nil || generation.class != string(FailureUnsupported) || generation.code != diagnosticCodegenUnsupported || generation.loc != schemaMixedComplexLoc(t, root, `<xs:complexType name="Base"`) || !generation.unsupported {
				t.Fatalf("%s nillable=%q generator = %d bytes/%#v", policy.name, value, len(output), generation)
			}
			if index == 0 {
				wantValidation, wantGeneration = validation, generation
				continue
			}
			if !reflect.DeepEqual(validation, wantValidation) || !reflect.DeepEqual(generation, wantGeneration) {
				t.Fatalf("%s nillable=%q changed consumers: validation %#v/%#v, generation %#v/%#v", policy.name, value, validation, wantValidation, generation, wantGeneration)
			}
		}
	}
}

type inlineNillableGraphFacts struct {
	documents  []SourceID
	components []ComponentID
	locations  []Loc
	walk       []ComponentID
	node       ComplexTypeID
	references []inlineMixedParticleFact
}

//nolint:dupl,gocognit // Parallel abstract/nillable graph snapshots prove each explicit fact preserves public identities and order.
func inlineNillableGraphSchema(t *testing.T, root string, fixtures map[string]discoveryFixture, policy LanguagePolicy) inlineNillableGraphFacts {
	t.Helper()
	schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, policy)
	if err != nil {
		t.Fatalf("parse composed graph: %v", err)
	}
	var facts inlineNillableGraphFacts
	for _, document := range schema.Documents() {
		facts.documents = append(facts.documents, document.Source())
	}
	for _, component := range schema.Components() {
		facts.components = append(facts.components, component.ID())
		facts.locations = append(facts.locations, component.Loc())
	}
	if err := schema.Walk(func(component Component) error {
		facts.walk = append(facts.walk, component.ID())
		return nil
	}); err != nil {
		t.Fatalf("walk composed graph: %v", err)
	}
	declaration := auxiliaryElement(t, schema, "root")
	if declaration.IsNillable() || declaration.Loc() != schemaMixedComplexLoc(t, root, `<xs:element name="root"`) {
		t.Fatal("composed root lost effective false fact or declaration location")
	}
	definition, ok := declaration.InlineComplexType()
	if !ok {
		t.Fatal("composed root lost inline complex type")
	}
	facts.node, ok = definition.NodeID()
	if !ok || facts.node.IsZero() {
		t.Fatal("composed root lost anonymous identity")
	}
	inlineMixedCollectParticle(&facts.references, definition.Particle())
	if len(facts.references) != 3 || facts.references[1].rangeValue != "0/3" || facts.references[2].rangeValue != "1/1" {
		t.Fatalf("composed root particle bounds = %#v", facts.references)
	}
	for _, reference := range facts.references[1:] {
		if reference.kind != "ref" || reference.target.IsZero() || reference.refLoc.IsZero() {
			t.Fatalf("unresolved composed reference = %#v", reference)
		}
		namespace := "urn:root"
		if reference.name == "Foreign" {
			namespace = "urn:other"
		}
		targets := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, namespace, reference.name))
		if len(targets) != 1 || reference.target != targets[0].ID() {
			t.Fatalf("reference %q target = %v, expected one visible declaration", reference.name, reference.target)
		}
	}
	if !reflect.DeepEqual(facts.walk, facts.components) {
		t.Fatalf("walk order %v differs from components %v", facts.walk, facts.components)
	}
	return facts
}

//nolint:gocognit // Repeat the public graph assertion for nillable across forward and cyclic discovery.
func TestInlineNillableFalseMatchesOmissionInComposedForwardGraph(t *testing.T) {
	for _, policy := range schemaMixedComplexPolicies() {
		for _, version := range schemaMixedComplexVersions() {
			rootFor := func(value string) string {
				return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="` + string(version.version) + `">
<xs:include schemaLocation="child.xsd"/><xs:import namespace="urn:other" schemaLocation="other.xsd"/>
<xs:element name="root"` + inlineNillableAttribute(value) + `>
<xs:complexType><xs:sequence minOccurs="0" maxOccurs="2"><xs:element ref="t:Later" minOccurs="0" maxOccurs="3"/><xs:element ref="o:Foreign"/></xs:sequence></xs:complexType>
</xs:element>
</xs:schema>`
			}
			for _, cycle := range []bool{false, true} {
				name := "acyclic"
				if cycle {
					name = "include cycle"
				}
				t.Run(policy.name+"/"+version.name+"/"+name, func(t *testing.T) {
					fixturesFor := func(value string) map[string]discoveryFixture {
						childInclude := ""
						if cycle {
							childInclude = `<xs:include schemaLocation="root.xsd"/>`
						}
						return map[string]discoveryFixture{
							"root.xsd":  {id: "root.xsd", contents: rootFor(value)},
							"child.xsd": {id: "child.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="` + string(version.version) + `">` + childInclude + `<xs:element name="Later" type="xs:integer"/></xs:schema>`},
							"other.xsd": {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other" version="` + string(version.version) + `"><xs:element name="Foreign" type="xs:integer"/></xs:schema>`},
						}
					}
					omitted := inlineNillableGraphSchema(t, rootFor(""), fixturesFor(""), policy.policy)
					if !reflect.DeepEqual(omitted.documents, []SourceID{"root.xsd", "child.xsd", "other.xsd"}) {
						t.Fatalf("document discovery order = %v", omitted.documents)
					}
					for _, value := range []string{"false", "0", "&#x9;false&#xA;"} {
						actual := inlineNillableGraphSchema(t, rootFor(value), fixturesFor(value), policy.policy)
						if !reflect.DeepEqual(actual, omitted) {
							t.Fatalf("nillable=%q changed graph facts: got %#v, omitted %#v", value, actual, omitted)
						}
					}
				})
			}
		}
	}
}

//nolint:gocognit // The global gate leaves each local shape's established admission or rejection intact.
func TestInlineNillableKeepsLocalElementShapes(t *testing.T) {
	shapes := []struct{ name, element, definitions string }{
		{"direct", `<xs:element name="child" type="xs:integer"`, ""},
		{"named", `<xs:element name="child" type="t:Named"`, `<xs:simpleType name="Named"><xs:restriction base="xs:integer"/></xs:simpleType>`},
		{"inline complex", `<xs:element name="child"`, ""},
		{"inline simple", `<xs:element name="child"`, ""},
		{"ref", `<xs:element ref="t:target"`, `<xs:element name="target" type="xs:integer"/>`},
	}
	for _, policy := range schemaMixedComplexPolicies() {
		for _, version := range schemaMixedComplexVersions() {
			for _, shape := range shapes {
				for _, value := range []string{"false", "0", "true", "1"} {
					t.Run(policy.name+"/"+version.name+"/"+shape.name+"/"+value, func(t *testing.T) {
						childSuffix := `/>`
						if shape.name == "inline complex" {
							childSuffix = `><xs:complexType/></xs:element>`
						}
						if shape.name == "inline simple" {
							childSuffix = `><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:element>`
						}
						child := shape.element + ` nillable="` + value + `"` + childSuffix
						root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" version="` + string(version.version) + `"><xs:complexType name="Owner"><xs:sequence>` + child + `</xs:sequence></xs:complexType>` + shape.definitions + `</xs:schema>`
						schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
						if shape.name == "direct" || shape.name == "named" || shape.name == "inline simple" {
							if err != nil {
								t.Fatalf("established local nillable form rejected: %v", err)
							}
							owners := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, "urn:root", "Owner"))
							if len(owners) != 1 {
								t.Fatalf("Owner components = %d", len(owners))
							}
							owner, ok := owners[0].ComplexTypeDefinition()
							if !ok {
								t.Fatal("Owner has no complex type definition")
							}
							sequence, ok := owner.Particle().(SequenceParticle)
							if !ok || len(sequence.Particles()) != 1 {
								t.Fatalf("local sequence = %T", owner.Particle())
							}
							child, ok := sequence.Particles()[0].(ElementParticle)
							if !ok || child.IsNillable() != (value == "true" || value == "1") || child.Loc() != schemaMixedComplexLoc(t, root, shape.element) {
								t.Fatalf("local %s nillable=%q facts = %#v", shape.name, value, sequence.Particles()[0])
							}
							return
						}
						if err == nil {
							t.Fatal("excluded local shape unexpectedly succeeded")
						}
						assertZeroSchema(t, schema)
						d := requireDiagnostic(t, err)
						wantClass, wantCode, wantLoc, wantSpec := FailureInvalid, invalidSchemaCompositionCode, schemaMixedComplexLoc(t, root, `nillable="`+value+`"`), ""
						if shape.name == "inline complex" {
							wantClass, wantCode = FailureUnsupported, UnsupportedSchemaSyntaxCode
							wantLoc = schemaMixedComplexLoc(t, root, `<xs:complexType/>`)
							wantSpec = newSchemaSyntaxUnsupported(Loc{}, "").SpecRef()
						}
						if d.Class() != wantClass || d.Code() != wantCode || d.Loc() != wantLoc || len(d.Related()) != 0 || d.SpecRef() != wantSpec || errors.Is(err, ErrUnsupported) != (wantClass == FailureUnsupported) {
							t.Fatalf("local %s nillable=%q diagnostic = %s", shape.name, value, d)
						}
					})
				}
			}
		}
	}
}

//nolint:dupl,gocognit // Each global shape keeps its existing nillable fact, parallel to the abstract gate.
func TestInlineNillableKeepsOtherGlobalShapes(t *testing.T) {
	shapes := []struct{ name, element, definitions string }{
		{"direct", `<xs:element name="root" type="xs:integer"`, ""},
		{"named", `<xs:element name="root" type="t:Named"`, `<xs:simpleType name="Named"><xs:restriction base="xs:integer"/></xs:simpleType>`},
		{"inline simple", `<xs:element name="root"`, ""},
	}
	for _, policy := range schemaMixedComplexPolicies() {
		for _, version := range schemaMixedComplexVersions() {
			for _, shape := range shapes {
				for _, value := range []string{"false", "0", "true", "1"} {
					t.Run(policy.name+"/"+version.name+"/"+shape.name+"/"+value, func(t *testing.T) {
						elementSuffix := `/>`
						if shape.name == "inline simple" {
							elementSuffix = `><xs:simpleType><xs:restriction base="xs:integer"/></xs:simpleType></xs:element>`
						}
						element := shape.element + ` nillable="` + value + `"` + elementSuffix
						root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" version="` + string(version.version) + `">` + element + shape.definitions + `</xs:schema>`
						schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
						if err != nil {
							t.Fatalf("global %s nillable=%q: %v", shape.name, value, err)
						}
						declaration := auxiliaryElement(t, schema, "root")
						if declaration.IsNillable() != (value == "true" || value == "1") || declaration.Loc() != schemaMixedComplexLoc(t, root, `<xs:element name="root"`) {
							t.Fatalf("global %s nillable=%q lost effective fact/Loc", shape.name, value)
						}
					})
				}
			}
		}
	}
}

//nolint:gocognit // The nillable gate must retain each broader shape's existing diagnostic.
func TestInlineNillableFalseKeepsBroaderInlineShapesExcluded(t *testing.T) {
	const prefix = `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:root" targetNamespace="urn:root" version="1.1">`
	tests := []struct {
		name, element, trailing, marker, related, code string
		class                                          FailureClass
		spec                                           func(XSDVersion) string
		cause                                          error
	}{
		{
			name: "group reference", element: `<xs:element name="root" nillable="false"><xs:complexType><xs:group ref="t:Fields"/></xs:complexType></xs:element>`,
			trailing: `<xs:group name="Fields"><xs:sequence/></xs:group>`, marker: `ref="t:Fields"`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode,
			spec: func(XSDVersion) string { return newSchemaSyntaxUnsupported(Loc{}, "").SpecRef() },
		},
		{
			name: "zero group reference", element: `<xs:element name="root" nillable="0"><xs:complexType><xs:group ref="t:Fields" minOccurs="0" maxOccurs="0"/></xs:complexType></xs:element>`,
			trailing: `<xs:group name="Fields"><xs:sequence/></xs:group>`, marker: `ref="t:Fields"`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode,
			spec: func(XSDVersion) string { return newSchemaSyntaxUnsupported(Loc{}, "").SpecRef() },
		},
		{
			name: "all", element: `<xs:element name="root" nillable="0"><xs:complexType><xs:all><xs:element name="v" type="xs:integer"/></xs:all></xs:complexType></xs:element>`,
			marker: `<xs:all>`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode,
			spec: func(XSDVersion) string { return newSchemaSyntaxUnsupported(Loc{}, "").SpecRef() },
		},
		{
			name: "grouped extension anonymous owner", element: `<xs:element name="root" nillable="false"><xs:complexType><xs:complexContent><xs:extension base="t:Base"><xs:group ref="t:Fields"/><xs:attribute name="flag" type="xs:boolean"/></xs:extension></xs:complexContent></xs:complexType></xs:element>`,
			trailing: `<xs:group name="Fields"><xs:sequence/></xs:group><xs:complexType name="Base"/>`, marker: `<xs:extension`, related: `<xs:complexType>`, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode, spec: schemaComplexContentExtensionSpecRef, cause: errSchemaGroupedExtensionAnonymousOwner,
		},
		{
			name: "named plus inline", element: `<xs:element name="root" type="t:Base" nillable="false"><xs:complexType/></xs:element>`,
			trailing: `<xs:complexType name="Base"/>`, marker: `<xs:complexType/>`, class: FailureInvalid, code: invalidSchemaCompositionCode,
		},
	}
	for _, policy := range schemaMixedComplexPolicies() {
		wantVersion := XSDVersion11
		if policy.policy == Strict10 {
			wantVersion = XSDVersion10
		}
		for _, test := range tests {
			t.Run(policy.name+"/"+test.name, func(t *testing.T) {
				root := prefix + test.element + test.trailing + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
				if err == nil {
					t.Fatal("broader inline shape unexpectedly succeeded")
				}
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				wantRelated := []Loc(nil)
				if test.related != "" {
					wantRelated = []Loc{schemaMixedComplexLoc(t, root, test.related)}
				}
				if d.Class() != test.class || d.Code() != test.code || d.Loc() != schemaMixedComplexLoc(t, root, test.marker) || !reflect.DeepEqual(d.Related(), wantRelated) {
					t.Fatalf("%s diagnostic = %s", test.name, d)
				}
				if test.cause != nil && !errors.Is(err, test.cause) {
					t.Fatalf("%s lost cause %v: %v", test.name, test.cause, err)
				}
				if test.class == FailureUnsupported {
					if d.SpecRef() != test.spec(wantVersion) || !errors.Is(err, ErrUnsupported) {
						t.Fatalf("%s provenance = %s/%v", test.name, d.SpecRef(), err)
					}
					return
				}
				if d.SpecRef() != "" || errors.Is(err, ErrUnsupported) {
					t.Fatalf("%s acquired unsupported provenance: %s", test.name, d)
				}
			})
		}
	}
}

//nolint:dupl,gocognit // The nillable gate must retain precedence for every alternate syntax/build exit.
func TestInlineNillableFalseKeepsSiblingAndReferencePrecedence(t *testing.T) {
	tests := []struct {
		name, value, body, marker, code string
		class                           FailureClass
		cause                           error
		spec                            func(XSDVersion) string
	}{
		{name: "true before invalid child", value: "true", body: `<xs:sequence><xs:element name="bad" type="xs:integer" abstract="true"/></xs:sequence>`, marker: `abstract="true"/>`, code: invalidSchemaCompositionCode, class: FailureInvalid},
		{name: "true before unsupported group", value: "true", body: `<xs:group ref="t:Missing"/>`, marker: `nillable="true"`, code: UnsupportedSchemaSyntaxCode, class: FailureUnsupported, spec: func(version XSDVersion) string {
			return newSchemaSyntaxUnsupportedForVersion(Loc{}, "", version).SpecRef()
		}},
		{name: "malformed before unsupported group", value: "maybe", body: `<xs:group ref="t:Missing"/>`, marker: `nillable="maybe"`, code: invalidSchemaCompositionCode, class: FailureInvalid},
		{name: "false with invalid child", value: "false", body: `<xs:sequence><xs:element name="bad" type="xs:integer" abstract="true"/></xs:sequence>`, marker: `abstract="true"/>`, code: invalidSchemaCompositionCode, class: FailureInvalid},
		{name: "false with unsupported group", value: "false", body: `<xs:group ref="t:Missing"/>`, marker: `ref="t:Missing"`, code: UnsupportedSchemaSyntaxCode, class: FailureUnsupported, spec: func(XSDVersion) string { return newSchemaSyntaxUnsupported(Loc{}, "").SpecRef() }},
		{name: "false with invalid occurrence", value: "false", body: `<xs:sequence><xs:element ref="t:target" maxOccurs="many"/></xs:sequence>`, marker: `maxOccurs="many"`, code: invalidSchemaCompositionCode, class: FailureInvalid, spec: func(version XSDVersion) string {
			if version == XSDVersion10 {
				return "xsd10-datatypes#nonNegativeInteger"
			}
			return "xsd11-datatypes#nonNegativeInteger"
		}},
		{name: "false with unresolved zero ref", value: "false", body: `<xs:sequence><xs:element ref="t:Missing" minOccurs="0" maxOccurs="0"/></xs:sequence>`, marker: `ref="t:Missing"`, code: diagnosticSchemaElementReferenceUnresolvedCode, class: FailureInvalid, cause: errSchemaElementReferenceUnresolved, spec: schemaElementReferenceSpecRef},
	}
	for _, policy := range schemaMixedComplexPolicies() {
		wantVersion := XSDVersion11
		if policy.policy == Strict10 {
			wantVersion = XSDVersion10
		}
		for _, test := range tests {
			t.Run(policy.name+"/"+test.name, func(t *testing.T) {
				root := inlineNillableSchema(test.body, test.value, XSDVersion11)
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
				if err == nil {
					t.Fatal("sibling or reference exit unexpectedly succeeded")
				}
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				if d.Class() != test.class || d.Code() != test.code || d.Loc() != schemaMixedComplexLoc(t, root, test.marker) || len(d.Related()) != 0 {
					t.Fatalf("%s diagnostic = %s", test.name, d)
				}
				if test.spec != nil && d.SpecRef() != test.spec(wantVersion) {
					t.Fatalf("%s spec = %q, want %q", test.name, d.SpecRef(), test.spec(wantVersion))
				}
				if test.spec == nil && d.SpecRef() != "" {
					t.Fatalf("%s unexpectedly has spec %q", test.name, d.SpecRef())
				}
				if test.cause != nil && !errors.Is(err, test.cause) {
					t.Fatalf("%s lost cause %v: %v", test.name, test.cause, err)
				}
				if errors.Is(err, ErrUnsupported) != (test.class == FailureUnsupported) {
					t.Fatalf("%s unsupported classification = %v", test.name, err)
				}
			})
		}
	}
}

func TestInlineNillableFalseKeepsStrict10PolicyBeforeZeroOccurrence(t *testing.T) {
	root := inlineNillableSchema(`<xs:sequence><xs:element name="v" type="xs:precisionDecimal" minOccurs="0" maxOccurs="0"/></xs:sequence>`, "false", XSDVersion11)
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	if err == nil {
		t.Fatal("Strict10 admitted zero-occurrence precisionDecimal child")
	}
	assertZeroSchema(t, schema)
	d := requireDiagnostic(t, err)
	if d.Class() != FailureUnsupported || d.Code() != diagnosticSchemaPrecisionDecimalVersionCode || d.Loc() != schemaMixedComplexLoc(t, root, `type="xs:precisionDecimal"`) || len(d.Related()) != 0 || d.SpecRef() != "xsd11-datatypes#dt-primitive" || !errors.Is(err, ErrUnsupported) || !errors.Is(err, errLanguagePolicyMismatch) || !errors.Is(err, errSchemaPrecisionDecimalVersion) {
		t.Fatalf("Strict10 zero-occurrence policy diagnostic = %s (%v)", d, err)
	}
}

func TestInlineNillableFalseDoesNotExposeInvisibleGraphReference(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:root" version="1.1">
<xs:element name="root" nillable="false"><xs:complexType><xs:sequence><xs:element ref="o:Foreign"/></xs:sequence></xs:complexType></xs:element>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
		"other.xsd": {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:element name="Foreign" type="xs:integer"/></xs:schema>`},
	}, Compatibility)
	if err == nil {
		t.Fatal("unimported foreign target became visible")
	}
	assertZeroSchema(t, schema)
	d := requireDiagnostic(t, err)
	if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaElementReferenceUnresolvedCode || d.Loc() != schemaMixedComplexLoc(t, root, `ref="o:Foreign"`) || len(d.Related()) != 0 || d.SpecRef() != schemaElementReferenceSpecRef(XSDVersion11) || !errors.Is(err, errSchemaElementReferenceUnresolved) || errors.Is(err, ErrUnsupported) {
		t.Fatalf("invisible reference diagnostic = %s (%v)", d, err)
	}
}
