package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func normalizedStringChoice(t *testing.T, schema Schema, namespace string) ChoiceParticle {
	const local = "Owner"
	t.Helper()
	components := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, namespace, local))
	if len(components) != 1 {
		t.Fatalf("%s components = %d", local, len(components))
	}
	definition, ok := components[0].ComplexTypeDefinition()
	if !ok {
		t.Fatalf("%s has no complex definition", local)
	}
	choice, ok := definition.Particle().(ChoiceParticle)
	if !ok {
		t.Fatalf("%s particle = %T", local, definition.Particle())
	}
	return choice
}

func normalizedStringChoiceElement(t *testing.T, particle Particle) ElementParticle {
	t.Helper()
	element, ok := particle.(ElementParticle)
	if !ok {
		t.Fatalf("choice alternative = %T", particle)
	}
	return element
}

//nolint:gocognit,funlen // One public fixture checks lexical order, effective facet copies, ranges, and repeated builds.
func TestNormalizedStringDirectChoiceFactsAcrossPolicies(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := normalizedStringSequenceRoot(`
  <xs:complexType name="Owner"><xs:choice minOccurs="2" maxOccurs="3">
    <xs:element name="qualified" type="xs:normalizedString" form="qualified" minOccurs="0" maxOccurs="18446744073709551616"/>
    <xs:element name="inherited" type="t:Inherited" minOccurs="2" maxOccurs="unbounded"/>
    <xs:element name="neighbor" type="xs:integer"/>
    <xs:element name="forward" type="t:Forward"/>
  </xs:choice></xs:complexType>
  <xs:simpleType name="Inherited"><xs:restriction base="t:Base"/></xs:simpleType>
  <xs:simpleType name="Base"><xs:restriction base="xs:normalizedString"><xs:enumeration value="a&#xA;b"/><xs:enumeration value=" a  b "/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Forward"><xs:restriction base="xs:normalizedString"><xs:whiteSpace value="collapse"/><xs:enumeration value="a b"/></xs:restriction></xs:simpleType>
`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("ParseSchema: %v", err)
			}
			if schema.LanguagePolicy() != profile.policy {
				t.Fatalf("policy = %s", schema.LanguagePolicy())
			}
			choice := normalizedStringChoice(t, schema, "urn:test")
			if choice.Occurrences().String() != "2/3" || choice.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 2, `<xs:choice`) {
				t.Fatalf("choice range/loc = %s/%s", choice.Occurrences(), choice.Loc())
			}
			alternatives := choice.Alternatives()
			if len(alternatives) != 4 {
				t.Fatalf("alternatives = %d", len(alternatives))
			}
			want := []struct {
				name, typ, bounds string
				line              int
				named             bool
			}{
				{"qualified", "normalizedString", "0/18446744073709551616", 3, false},
				{"inherited", "Inherited", "2/unbounded", 4, true},
				{"neighbor", "integer", "1/1", 5, false},
				{"forward", "Forward", "1/1", 6, true},
			}
			for i, expected := range want {
				element := normalizedStringChoiceElement(t, alternatives[i])
				namespace := ""
				if i == 0 {
					namespace = "urn:test"
				}
				if element.Name() != mustTestQName(t, namespace, expected.name) || element.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, expected.line, `<xs:element`) || element.Occurrences().String() != expected.bounds {
					t.Fatalf("alternative %d name/loc/range = %s/%s/%s", i, element.Name(), element.Loc(), element.Occurrences())
				}
				typeNamespace := testXSDNamespace
				if expected.named {
					typeNamespace = "urn:test"
				}
				typeName := mustTestQName(t, typeNamespace, expected.typ)
				reference, ok := element.TypeReference()
				if element.DeclaredType() != typeName || !ok || reference.Name() != typeName || reference.QName() != typeName || reference.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, expected.line, `type="`) {
					t.Fatalf("alternative %d type/reference = %s/%#v/%t", i, element.DeclaredType(), reference, ok)
				}
				id, hasID := element.TypeID()
				refID, refHasID := reference.ComponentID()
				if hasID != expected.named || refHasID != expected.named || id != refID {
					t.Fatalf("alternative %d identity = %v/%t %v/%t", i, id, hasID, refID, refHasID)
				}
				if expected.named && id != componentIDForName(t, schema, typeName) {
					t.Fatalf("alternative %d target = %v", i, id)
				}
			}
			builtIn, _ := normalizedStringChoiceElement(t, alternatives[0]).TypeReference()
			assertNormalizedStringReference(t, builtIn, mustSchemaTokenLoc(t, "root.xsd", root, 3, `type="xs:normalizedString"`))
			inherited, _ := normalizedStringChoiceElement(t, alternatives[1]).TypeReference()
			whiteSpace, ok := inherited.StringWhiteSpaceFacet()
			if !ok || whiteSpace.Value() != "replace" || !whiteSpace.Loc().IsZero() {
				t.Fatalf("inherited whitespace = %#v/%t", whiteSpace, ok)
			}
			assertStringEnumerationFacts(t, inherited.StringEnumerationFacets(), profile.version, []string{"a\nb", " a  b "}, []Loc{
				mustSchemaTokenLoc(t, "root.xsd", root, 9, `value="a&#xA;b"`),
				mustSchemaTokenLoc(t, "root.xsd", root, 9, `value=" a  b "`),
			})
			forward, _ := normalizedStringChoiceElement(t, alternatives[3]).TypeReference()
			whiteSpace, ok = forward.StringWhiteSpaceFacet()
			if !ok || whiteSpace.Value() != "collapse" || whiteSpace.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 10, `value="collapse"`) {
				t.Fatalf("forward whitespace = %#v/%t", whiteSpace, ok)
			}
			assertStringEnumerationFacts(t, forward.StringEnumerationFacets(), profile.version, []string{"a b"}, []Loc{mustSchemaTokenLoc(t, "root.xsd", root, 10, `value="a b"`)})
			values := inherited.StringEnumerationFacets().Values()
			values[0] = "changed"
			locations := inherited.StringEnumerationFacets().Locations()
			locations[0] = Loc{}
			alternatives[0] = nil
			if normalizedStringChoiceElement(t, choice.Alternatives()[0]).Name().Local() != "qualified" || inherited.StringEnumerationFacets().Values()[0] != "a\nb" || inherited.StringEnumerationFacets().Locations()[0].IsZero() {
				t.Fatal("choice/facet copy changed schema")
			}
			again, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil || !reflect.DeepEqual(schema.Components(), again.Components()) {
				t.Fatalf("repeated build changed components: %v", err)
			}
			var walked []QName
			if err := schema.Walk(func(component Component) error { walked = append(walked, component.Name()); return nil }); err != nil {
				t.Fatalf("Walk: %v", err)
			}
			if len(walked) != 4 || walked[0].Local() != "Owner" || walked[1].Local() != "Inherited" || walked[2].Local() != "Base" || walked[3].Local() != "Forward" {
				t.Fatalf("walk order = %v", walked)
			}
		})
	}
}

//nolint:gocognit // Graph visibility and lexical provenance share one discovered fixture.
func TestNormalizedStringDirectChoiceGraphProvenance(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" elementFormDefault="qualified" version="` + string(profile.version) + `">
  <xs:include schemaLocation="included.xsd"/>
  <xs:include schemaLocation="included.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:complexType name="Owner"><xs:choice><xs:element name="included" type="r:Included"/><xs:element name="chameleon" type="r:Chameleon"/><xs:element name="imported" type="o:Imported"/><xs:element name="forward" type="r:Forward"/></xs:choice></xs:complexType>
  <xs:simpleType name="Forward"><xs:restriction base="r:Included"/></xs:simpleType>
</xs:schema>`
			fixtures := map[string]discoveryFixture{
				"root.xsd":      {id: "root.xsd", contents: root},
				"included.xsd":  {id: "included.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:simpleType name="Included"><xs:restriction base="xs:normalizedString"><xs:enumeration value="a&#xA;b"/></xs:restriction></xs:simpleType></xs:schema>`},
				"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="Chameleon"><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:schema>`},
				"other.xsd":     {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Imported"><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:schema>`},
			}
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("ParseSchema graph: %v", err)
			}
			docs := schema.Documents()
			for i, source := range []SourceID{"root.xsd", "included.xsd", "chameleon.xsd", "other.xsd"} {
				if len(docs) != 4 || docs[i].Source() != source {
					t.Fatalf("document order = %#v", docs)
				}
			}
			terms := normalizedStringChoice(t, schema, "urn:root").Alternatives()
			if len(terms) != 4 {
				t.Fatalf("terms = %d", len(terms))
			}
			for i, expected := range []struct {
				local, lexical, typeNamespace, typeLocal string
				source                                   SourceID
			}{
				{"included", "r:Included", "urn:root", "Included", "included.xsd"},
				{"chameleon", "r:Chameleon", "urn:root", "Chameleon", "chameleon.xsd"},
				{"imported", "o:Imported", "urn:other", "Imported", "other.xsd"},
				{"forward", "r:Forward", "urn:root", "Forward", "root.xsd"},
			} {
				element := normalizedStringChoiceElement(t, terms[i])
				ref, ok := element.TypeReference()
				name := mustTestQName(t, expected.typeNamespace, expected.typeLocal)
				id, hasID := element.TypeID()
				if element.Name() != mustTestQName(t, "urn:root", expected.local) || element.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 6, `<xs:element name="`+expected.local+`"`) || element.DeclaredType() != name || !ok || ref.QName() != name || ref.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 6, `type="`+expected.lexical+`"`) || !hasID || id.Source() != expected.source || id != componentIDForName(t, schema, name) {
					t.Fatalf("term %d facts = %#v/%#v/%v", i, element, ref, id)
				}
			}
			included, _ := normalizedStringChoiceElement(t, terms[0]).TypeReference()
			assertStringEnumerationFacts(t, included.StringEnumerationFacets(), profile.version, []string{"a\nb"}, []Loc{schemaBuiltinReferenceAttributeLoc(t, "included.xsd", `value="a&#xA;b"`, root, fixtures)})
			for _, relation := range []string{"unimported", "indirect-import"} {
				hiddenRoot, hiddenFixtures := typeVisibilityTestHiddenGraph(t, string(profile.version), relation, false, true, true, "")
				foreign := hiddenFixtures["foreign.xsd"]
				foreign.contents = strings.Replace(foreign.contents, `base="xs:integer"`, `base="xs:normalizedString"`, 1)
				hiddenFixtures["foreign.xsd"] = foreign
				hiddenSchema, hiddenErr := discoverTestSchemaWithPolicy(t, hiddenRoot, hiddenFixtures, profile.policy)
				if hiddenErr == nil {
					t.Fatal("invisible type admitted")
				}
				assertZeroSchema(t, hiddenSchema)
				diagnostic := requireDiagnostic(t, hiddenErr)
				if diagnostic.Code() != diagnosticSchemaElementTypeUnresolvedCode || diagnostic.Loc() != schemaBuiltinReferenceAttributeLoc(t, "root.xsd", `type="f:Hidden"`, hiddenRoot, hiddenFixtures) || diagnostic.SpecRef() != schemaElementTypeSpecRef(profile.version) || !errors.Is(hiddenErr, errSchemaElementTypeUnresolved) {
					t.Fatalf("%s invisible type diagnostic = %v", relation, hiddenErr)
				}
			}
		})
	}
}

//nolint:gocognit // Shape exclusions are separate schema admission boundaries.
func TestNormalizedStringDirectChoiceShapeExclusions(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct{ name, body, marker string }{
			{"anonymous built-in", `<xs:complexType name="Owner"><xs:choice><xs:element name="value"><xs:simpleType><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:element></xs:choice></xs:complexType>`, `<xs:simpleType`},
			{"anonymous named", `<xs:complexType name="Owner"><xs:choice><xs:element name="value"><xs:simpleType><xs:restriction base="t:T"/></xs:simpleType></xs:element></xs:choice></xs:complexType><xs:simpleType name="T"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`, `<xs:simpleType`},
			{"inline owner built-in", `<xs:element name="root"><xs:complexType><xs:choice><xs:element name="value" type="xs:normalizedString"/></xs:choice></xs:complexType></xs:element>`, `type="xs:normalizedString"`},
			{"inline owner named", `<xs:element name="root"><xs:complexType><xs:choice><xs:element name="value" type="t:T"/></xs:choice></xs:complexType></xs:element><xs:simpleType name="T"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`, `type="t:T"`},
			{"extension built-in", `<xs:complexType name="Base"/><xs:complexType name="Owner"><xs:complexContent><xs:extension base="t:Base"><xs:choice><xs:element name="value" type="xs:normalizedString"/></xs:choice></xs:extension></xs:complexContent></xs:complexType>`, `type="xs:normalizedString"`},
			{"extension named", `<xs:complexType name="Base"/><xs:complexType name="Owner"><xs:complexContent><xs:extension base="t:Base"><xs:choice><xs:element name="value" type="t:T"/></xs:choice></xs:extension></xs:complexContent></xs:complexType><xs:simpleType name="T"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`, `type="t:T"`},
			{"named list", `<xs:complexType name="Owner"><xs:choice><xs:element name="value" type="t:T"/></xs:choice></xs:complexType><xs:simpleType name="T"><xs:list itemType="xs:normalizedString"/></xs:simpleType>`, `type="t:T"`},
			{"named union", `<xs:complexType name="Owner"><xs:choice><xs:element name="value" type="t:T"/></xs:choice></xs:complexType><xs:simpleType name="T"><xs:union memberTypes="xs:normalizedString"/></xs:simpleType>`, `type="t:T"`},
			{"attribute inline", `<xs:complexType name="Owner"><xs:choice><xs:element name="count" type="xs:integer"/></xs:choice><xs:attribute name="value"><xs:simpleType><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:attribute></xs:complexType>`, `<xs:simpleType`},
			{"attribute ref", `<xs:attribute name="value" type="xs:normalizedString"/><xs:complexType name="Owner"><xs:choice><xs:element name="count" type="xs:integer"/></xs:choice><xs:attribute ref="t:value"/></xs:complexType>`, `ref="t:value"`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := normalizedStringSequenceRoot("\n  " + test.body + "\n")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil {
					t.Fatal("excluded shape admitted")
				}
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				wantLoc := mustSchemaTokenLoc(t, "root.xsd", root, 2, test.marker)
				wantSpec := schemaSyntaxSpecRefForVersion(profile.version)
				if strings.HasPrefix(test.name, "attribute ") {
					wantSpec = schemaAttributeTypeSpecRef(profile.version)
				}
				if test.name == "attribute ref" {
					wantSpec = schemaAttributeUseSpecRef(profile.version)
				}
				if d.Class() != FailureUnsupported || d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != wantLoc || d.SpecRef() != wantSpec || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("shape diagnostic = %v; related=%v", err, d.Related())
				}
			})
		}
	}
}

type normalizedStringZeroBoundaryCase struct {
	name, child, tail, marker, code, spec string
	related                               []string
	class                                 FailureClass
	cause                                 error
}

func normalizedStringZeroBoundaryCases(profile tokenPolicyProfile) []normalizedStringZeroBoundaryCase {
	return []normalizedStringZeroBoundaryCase{
		{"unresolved", `<xs:element name="value" type="t:Missing" minOccurs="0" maxOccurs="0"/>`, "", `type="t:Missing"`, diagnosticSchemaElementTypeUnresolvedCode, schemaElementTypeSpecRef(profile.version), nil, FailureInvalid, errSchemaElementTypeUnresolved},
		{"wrong kind", `<xs:element name="value" type="t:T" minOccurs="0" maxOccurs="0"/>`, `<xs:element name="T" type="xs:normalizedString"/>`, `type="t:T"`, diagnosticSchemaElementTypeWrongKindCode, schemaElementTypeSpecRef(profile.version), []string{`<xs:element name="T"`}, FailureInvalid, errSchemaElementTypeWrongKind},
		{"invalid range", `<xs:element name="value" type="xs:normalizedString" minOccurs="2" maxOccurs="1"/>`, "", `<xs:element name="value"`, invalidSchemaCompositionCode, schemaParticleCorrectSpecRef(profile.version), []string{`minOccurs="2"`, `maxOccurs="1"`}, FailureInvalid, errParticleOccurrenceMinimumExceedsMaximum},
		{"invalid occurrence", `<xs:element name="value" type="xs:normalizedString" minOccurs="0" maxOccurs="many"/>`, "", `maxOccurs="many"`, invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(profile.version), nil, FailureInvalid, nil},
		{"malformed QName", `<xs:element name="value" type="t:bad:name" minOccurs="0" maxOccurs="0"/>`, "", `type="t:bad:name"`, invalidSchemaConditionalCode, "", nil, FailureInvalid, nil},
		{"invalid whitespace", `<xs:element name="value" type="t:T" minOccurs="0" maxOccurs="0"/>`, `<xs:simpleType name="T"><xs:restriction base="xs:normalizedString"><xs:whiteSpace value="preserve"/></xs:restriction></xs:simpleType>`, `value="preserve"`, InvalidStringWhiteSpaceRestrictionCode, stringWhiteSpaceSpecRef(profile.version), nil, FailureInvalid, errInvalidStringWhiteSpaceRestriction},
		{"unsupported facet", `<xs:element name="value" type="t:T" minOccurs="0" maxOccurs="0"/>`, `<xs:simpleType name="T"><xs:restriction base="xs:normalizedString"><xs:length value="2"/></xs:restriction></xs:simpleType>`, `<xs:length`, UnsupportedDatatypeFacetCode, tokenDiagnosticSpecRef(profile.version, "decimal"), nil, FailureUnsupported, ErrUnsupported},
		{"cyclic type", `<xs:element name="value" type="t:A" minOccurs="0" maxOccurs="0"/>`, `<xs:simpleType name="A"><xs:restriction base="t:B"/></xs:simpleType><xs:simpleType name="B"><xs:restriction base="t:A"/></xs:simpleType><xs:simpleType name="Seed"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`, `base="t:B"`, diagnosticSchemaSimpleTypeCycleCode, schemaSimpleTypeSpecRef(profile.version), []string{`base="t:A"`}, FailureInvalid, errSchemaSimpleTypeBaseCycle},
		{"name and ref", `<xs:element name="value" ref="t:T" minOccurs="0" maxOccurs="0"/>`, `<xs:element name="T" type="xs:normalizedString"/>`, `ref="t:T"`, invalidSchemaCompositionCode, "", nil, FailureInvalid, nil},
	}
}

//nolint:gocognit // These exits must remain visible even when the choice term is 0/0.
func TestNormalizedStringDirectChoiceZeroBoundaryDiagnostics(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range normalizedStringZeroBoundaryCases(profile) {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := normalizedStringSequenceRoot("\n  <xs:complexType name=\"Owner\"><xs:choice>" + test.child + "</xs:choice></xs:complexType>" + test.tail + "\n")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil {
					t.Fatal("bad zero term admitted")
				}
				assertZeroSchema(t, schema)
				d := requireDiagnostic(t, err)
				var related []Loc
				for _, marker := range test.related {
					related = append(related, mustSchemaTokenLoc(t, "root.xsd", root, 2, marker))
				}
				wantLoc := mustSchemaTokenLoc(t, "root.xsd", root, 2, test.marker)
				if d.Class() != test.class || d.Code() != test.code || d.Loc() != wantLoc || d.SpecRef() != test.spec || !reflect.DeepEqual(d.Related(), related) {
					t.Fatalf("exit diagnostic = %v; related=%v", err, d.Related())
				}
				if test.cause != nil && !errors.Is(err, test.cause) {
					t.Fatalf("lost cause %v: %v", test.cause, err)
				}
				if test.name == "invalid occurrence" {
					var lexical Diagnostic
					if !errors.As(errors.Unwrap(d), &lexical) || lexical.Code() != InvalidIntegerLexicalCode || lexical.Loc() != wantLoc {
						t.Fatalf("occurrence lexical cause = %v", errors.Unwrap(d))
					}
				}
			})
		}
	}
}

//nolint:gocognit // Policy and omission meet at the same direct-choice boundary.
func TestNormalizedStringDirectChoiceZeroOmissionAndPolicy(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := normalizedStringSequenceRoot(`
  <xs:complexType name="Owner"><xs:choice><xs:element name="omitted" type="xs:normalizedString" minOccurs="0" maxOccurs="0"/><xs:element name="neighbor" type="xs:integer"/></xs:choice></xs:complexType>
  <xs:element name="root" type="t:Owner"/>
`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("ParseSchema: %v", err)
			}
			terms := normalizedStringChoice(t, schema, "urn:test").Alternatives()
			if len(terms) != 1 || normalizedStringChoiceElement(t, terms[0]).Name().Local() != "neighbor" {
				t.Fatalf("zero omission = %#v", terms)
			}
			generated, err := GenerateGo(schema, "generated")
			if err != nil || len(generated) == 0 {
				t.Fatalf("GenerateGo after omission = %q/%v", generated, err)
			}
			validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:test"><neighbor xmlns="">1</neighbor></root>`)))
			if validationErr != nil {
				t.Fatalf("ValidateInstance after omission: %v", validationErr)
			}
			namespaceRoot := normalizedStringSequenceRoot(`
  <xs:complexType name="Owner"><xs:choice><xs:element name="local" type="xs:normalizedString" targetNamespace="urn:test"/><xs:element name="omitted" type="xs:normalizedString" targetNamespace="urn:test" minOccurs="0" maxOccurs="0"/></xs:choice></xs:complexType>
`)
			namespaced, err := discoverTestSchemaWithPolicy(t, namespaceRoot, nil, profile.policy)
			if profile.policy == Strict10 {
				if err == nil {
					t.Fatal("Strict10 admitted local targetNamespace")
				}
				assertZeroSchema(t, namespaced)
				d := requireDiagnostic(t, err)
				if d.Code() != UnsupportedSchemaSyntaxCode || d.Loc() != mustSchemaTokenLoc(t, "root.xsd", namespaceRoot, 2, `targetNamespace="urn:test"`) || d.SpecRef() != schemaSyntaxSpecRefForVersion(XSDVersion11) || !errors.Is(err, errLanguagePolicyMismatch) {
					t.Fatalf("policy diagnostic = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("local targetNamespace: %v", err)
			}
			terms = normalizedStringChoice(t, namespaced, "urn:test").Alternatives()
			if len(terms) != 1 || normalizedStringChoiceElement(t, terms[0]).Name() != mustTestQName(t, "urn:test", "local") {
				t.Fatalf("targetNamespace/0 omission = %#v", terms)
			}
		})
	}
}

//nolint:gocognit // Direct, named, inherited, and reference alternatives have independent consumer exits.
func TestNormalizedStringDirectChoiceConsumersRejectSurvivingTerms(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct {
			name, child, tail, generationMarker string
			generationRelated                   []string
		}{
			{"direct", `<xs:element name="value" type="xs:normalizedString"/>`, "", `<xs:element name="value"`, nil},
			{"named", `<xs:element name="value" type="t:T"/>`, `<xs:simpleType name="T"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`, `<xs:element name="value"`, []string{`<xs:simpleType name="T"`, `base="xs:normalizedString"`}},
			{"inherited", `<xs:element name="value" type="t:Child"/>`, `<xs:simpleType name="Child"><xs:restriction base="t:Base"/></xs:simpleType><xs:simpleType name="Base"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`, `<xs:element name="value"`, []string{`<xs:simpleType name="Child"`, `base="t:Base"`}},
			{"ref", `<xs:element ref="t:item"/>`, `<xs:element name="item" type="xs:normalizedString"/>`, `ref="t:item"`, []string{`<xs:element ref="t:item"`, `<xs:element name="item"`}},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := normalizedStringSequenceRoot("\n  <xs:complexType name=\"Owner\"><xs:choice>" + test.child + "</xs:choice></xs:complexType>\n  <xs:element name=\"root\" type=\"t:Owner\"/>\n  " + test.tail + "\n")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("ParseSchema: %v", err)
				}
				terms := normalizedStringChoice(t, schema, "urn:test").Alternatives()
				if len(terms) != 1 {
					t.Fatalf("alternatives = %d", len(terms))
				}
				if test.name == "ref" {
					ref, ok := terms[0].(ElementReferenceParticle)
					targets := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:test", "item"))
					if !ok || len(targets) != 1 || ref.RefLoc() != mustSchemaTokenLoc(t, "root.xsd", root, 2, `ref="t:item"`) || ref.TargetID() != targets[0].ID() {
						t.Fatalf("ref facts = %#v/%t", ref, ok)
					}
				}
				if test.name != "ref" {
					normalizedStringChoiceElement(t, terms[0])
				}
				output, err := GenerateGo(schema, "generated")
				if err == nil || output != nil {
					t.Fatalf("GenerateGo = %q/%v", output, err)
				}
				generation := requireDiagnostic(t, err)
				wantGenerationLoc := mustSchemaTokenLoc(t, "root.xsd", root, 2, test.generationMarker)
				var wantGenerationRelated []Loc
				for _, marker := range test.generationRelated {
					line := 4
					if test.name == "ref" && strings.HasPrefix(marker, `<xs:element ref=`) {
						line = 2
					}
					wantGenerationRelated = append(wantGenerationRelated, mustSchemaTokenLoc(t, "root.xsd", root, line, marker))
				}
				wantGenerationSpec := "xsd11-structures#element-choice"
				if profile.version == XSDVersion10 {
					wantGenerationSpec = "xsd10-structures#element-choice"
				}
				if test.name == "named" || test.name == "inherited" {
					wantGenerationSpec = schemaSimpleTypeSpecRef(profile.version)
				}
				if generation.Class() != FailureUnsupported || generation.Code() != diagnosticCodegenUnsupported || generation.Loc() != wantGenerationLoc || generation.SpecRef() != wantGenerationSpec || !reflect.DeepEqual(generation.Related(), wantGenerationRelated) || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("generation = %v; related=%v", err, generation.Related())
				}
				instance := `<root xmlns="urn:test"><value xmlns="">a b</value></root>`
				if test.name == "ref" {
					instance = `<root xmlns="urn:test"><item>a b</item></root>`
				}
				err = ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(instance)))
				if err == nil {
					t.Fatal("ValidateInstance admitted normalizedString choice")
				}
				validation := requireDiagnostic(t, err)
				wantValidationRelated := []Loc{
					mustSchemaTokenLoc(t, "root.xsd", root, 3, `<xs:element name="root"`),
					mustSchemaTokenLoc(t, "root.xsd", root, 2, `<xs:complexType`),
					mustSchemaTokenLoc(t, "root.xsd", root, 2, `<xs:choice`),
					mustSchemaTokenLoc(t, "root.xsd", root, 2, `<xs:element`),
				}
				if test.name == "named" || test.name == "inherited" {
					wantValidationRelated = append(wantValidationRelated, mustSchemaTokenLoc(t, "root.xsd", root, 4, `<xs:simpleType`))
				}
				if test.name == "ref" {
					wantValidationRelated = append(wantValidationRelated, mustSchemaTokenLoc(t, "root.xsd", root, 4, `<xs:element name="item"`))
				}
				wantValidationLoc := mustSchemaTokenLoc(t, "root.xsd", root, 2, `<xs:element`)
				if test.name == "ref" {
					wantValidationLoc = mustTestLoc(t, "instance.xml", 1, 1)
				}
				if validation.Class() != FailureUnsupported || validation.Code() != UnsupportedInstanceValidationCode || validation.Loc() != wantValidationLoc || validation.SpecRef() != instanceValidationSpecRef(profile.version) || !reflect.DeepEqual(validation.Related(), wantValidationRelated) || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("validation = %v; related=%v", err, validation.Related())
				}
			})
		}
	}
}

func TestNormalizedStringDirectChoiceAmbiguousDeclarationAndZeroOuter(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			duplicate := normalizedStringSequenceRoot(`
  <xs:complexType name="Owner"><xs:choice><xs:element name="value" type="t:T"/></xs:choice></xs:complexType>
  <xs:simpleType name="T"><xs:restriction base="xs:normalizedString"/></xs:simpleType>
  <xs:simpleType name="T"><xs:restriction base="xs:normalizedString"/></xs:simpleType>
`)
			schema, err := discoverTestSchemaWithPolicy(t, duplicate, nil, profile.policy)
			if err == nil {
				t.Fatal("duplicate target admitted")
			}
			assertZeroSchema(t, schema)
			d := requireDiagnostic(t, err)
			if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaGlobalDuplicateCode || d.Loc() != mustSchemaTokenLoc(t, "root.xsd", duplicate, 4, `<xs:simpleType`) || d.SpecRef() != schemaGlobalDuplicateSpecRef(profile.version) || !reflect.DeepEqual(d.Related(), []Loc{mustSchemaTokenLoc(t, "root.xsd", duplicate, 3, `<xs:simpleType`)}) || !errors.Is(err, errSchemaGlobalDeclarationDuplicate) {
				t.Fatalf("duplicate diagnostic = %v; related=%v", err, d.Related())
			}
			outer := normalizedStringSequenceRoot(`
  <xs:complexType name="Owner"><xs:choice minOccurs="0" maxOccurs="0"><xs:element name="value" type="t:Missing"/></xs:choice></xs:complexType>
`)
			schema, err = discoverTestSchemaWithPolicy(t, outer, nil, profile.policy)
			if err == nil {
				t.Fatal("zero outer choice hid unresolved child")
			}
			assertZeroSchema(t, schema)
			d = requireDiagnostic(t, err)
			if d.Class() != FailureInvalid || d.Code() != diagnosticSchemaElementTypeUnresolvedCode || d.Loc() != mustSchemaTokenLoc(t, "root.xsd", outer, 2, `type="t:Missing"`) || d.SpecRef() != schemaElementTypeSpecRef(profile.version) || len(d.Related()) != 0 || !errors.Is(err, errSchemaElementTypeUnresolved) {
				t.Fatalf("zero outer diagnostic = %v", err)
			}
		})
	}
}

//nolint:gocognit // Both public consumers must reject the second ordered alternative.
func TestNormalizedStringDirectChoiceNeighborConsumerOrder(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := normalizedStringSequenceRoot(`
  <xs:complexType name="Owner"><xs:choice><xs:element name="count" type="xs:integer"/><xs:element name="text" type="xs:normalizedString"/></xs:choice></xs:complexType>
  <xs:element name="root" type="t:Owner"/>
`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("ParseSchema: %v", err)
			}
			terms := normalizedStringChoice(t, schema, "urn:test").Alternatives()
			if len(terms) != 2 || normalizedStringChoiceElement(t, terms[0]).Name().Local() != "count" || normalizedStringChoiceElement(t, terms[1]).Name().Local() != "text" {
				t.Fatalf("neighbor order = %#v", terms)
			}
			output, err := GenerateGo(schema, "generated")
			if err == nil || output != nil {
				t.Fatalf("GenerateGo = %q/%v", output, err)
			}
			d := requireDiagnostic(t, err)
			want := mustSchemaTokenLoc(t, "root.xsd", root, 2, `<xs:element name="text"`)
			spec := "xsd11-structures#element-choice"
			if profile.version == XSDVersion10 {
				spec = "xsd10-structures#element-choice"
			}
			if d.Class() != FailureUnsupported || d.Code() != diagnosticCodegenUnsupported || d.Loc() != want || d.SpecRef() != spec || len(d.Related()) != 0 || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("mixed generation = %v", err)
			}
			err = ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:test"><count xmlns="">1</count></root>`)))
			if err == nil {
				t.Fatal("validator skipped unsupported second alternative")
			}
			d = requireDiagnostic(t, err)
			wantRelated := []Loc{
				mustSchemaTokenLoc(t, "root.xsd", root, 3, `<xs:element name="root"`),
				mustSchemaTokenLoc(t, "root.xsd", root, 2, `<xs:complexType`),
				mustSchemaTokenLoc(t, "root.xsd", root, 2, `<xs:choice`),
				want,
			}
			if d.Class() != FailureUnsupported || d.Code() != UnsupportedInstanceValidationCode || d.Loc() != want || d.SpecRef() != instanceValidationSpecRef(profile.version) || !reflect.DeepEqual(d.Related(), wantRelated) || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("mixed validation = %v; related=%v", err, d.Related())
			}
		})
	}
}

//nolint:gocognit // The omission and both consumer outcomes are one public boundary.
func TestNormalizedStringDirectChoiceNamedZeroTermOmits(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := normalizedStringSequenceRoot(`
  <xs:complexType name="Owner"><xs:choice><xs:element name="omitted" type="t:Text" minOccurs="0" maxOccurs="0"/><xs:element name="neighbor" type="xs:integer"/></xs:choice></xs:complexType>
  <xs:element name="root" type="t:Owner"/>
  <xs:simpleType name="Text"><xs:restriction base="xs:normalizedString"><xs:enumeration value="a&#xA;b"/></xs:restriction></xs:simpleType>
`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("ParseSchema: %v", err)
			}
			terms := normalizedStringChoice(t, schema, "urn:test").Alternatives()
			if len(terms) != 1 || normalizedStringChoiceElement(t, terms[0]).Name().Local() != "neighbor" {
				t.Fatalf("named zero term = %#v", terms)
			}
			textName := mustTestQName(t, "urn:test", "Text")
			if len(schema.FindKind(ComponentKindSimpleTypeDefinition, textName)) != 1 {
				t.Fatal("named type lost after omission")
			}
			validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:test"><neighbor xmlns="">1</neighbor></root>`)))
			if validationErr != nil {
				t.Fatalf("ValidateInstance after omission: %v", validationErr)
			}
			output, err := GenerateGo(schema, "generated")
			if err == nil || output != nil {
				t.Fatalf("GenerateGo named global = %q/%v", output, err)
			}
			d := requireDiagnostic(t, err)
			wantRelated := []Loc{mustSchemaTokenLoc(t, "root.xsd", root, 4, `base="xs:normalizedString"`)}
			if d.Class() != FailureUnsupported || d.Code() != diagnosticCodegenUnsupported || d.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 4, `<xs:simpleType`) || d.SpecRef() != schemaSimpleTypeSpecRef(profile.version) || !reflect.DeepEqual(d.Related(), wantRelated) || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("named global generation = %v; related=%v", err, d.Related())
			}
		})
	}
}
