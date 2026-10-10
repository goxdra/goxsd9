package goxsd9

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func normalizedStringSequenceRoot(body string) string {
	return `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test">` + body + `</xs:schema>`
}

func normalizedStringSequenceType(t *testing.T, schema Schema, namespace, local string) SequenceParticle {
	t.Helper()
	components := schema.FindKind(ComponentKindComplexTypeDefinition, mustTestQName(t, namespace, local))
	if len(components) != 1 {
		t.Fatalf("complex type %s matches = %d, want one", local, len(components))
	}
	definition, ok := components[0].ComplexTypeDefinition()
	if !ok {
		t.Fatalf("complex type %s has no definition", local)
	}
	sequence, ok := definition.Particle().(SequenceParticle)
	if !ok {
		t.Fatalf("complex type %s particle = %T, want sequence", local, definition.Particle())
	}
	return sequence
}

//nolint:gocognit,funlen // One fixture exercises copied public facts, ownership, ranges, and lexical order.
func TestNormalizedStringDirectSequenceFactsAcrossPolicies(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := normalizedStringSequenceRoot(`
  <xs:complexType name="Owner"><xs:sequence minOccurs="2" maxOccurs="3">
    <xs:element name="qualified" type="xs:normalizedString" form="qualified" minOccurs="0" maxOccurs="18446744073709551616"/>
    <xs:element name="inherited" type="t:Inherited" minOccurs="2" maxOccurs="unbounded"/>
    <xs:element name="forward" type="t:Forward"/>
  </xs:sequence></xs:complexType>
  <xs:simpleType name="Inherited"><xs:restriction base="t:Base"/></xs:simpleType>
  <xs:simpleType name="Base"><xs:restriction base="xs:normalizedString"><xs:enumeration value="a&#xA;b"/><xs:enumeration value=" a  b "/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Forward"><xs:restriction base="xs:normalizedString"><xs:whiteSpace value="collapse"/><xs:enumeration value="a b"/></xs:restriction></xs:simpleType>
`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("ParseSchema: %v", err)
			}
			if schema.LanguagePolicy() != profile.policy {
				t.Fatalf("policy = %q, want %q", schema.LanguagePolicy(), profile.policy)
			}
			sequence := normalizedStringSequenceType(t, schema, "urn:test", "Owner")
			if got := sequence.Occurrences().String(); got != "2/3" {
				t.Fatalf("sequence occurrences = %s", got)
			}
			if got := sequence.Loc(); got != mustSchemaTokenLoc(t, "root.xsd", root, 2, `<xs:sequence`) {
				t.Fatalf("sequence location = %s", got)
			}
			elements := sequence.Elements()
			if len(elements) != 3 || len(sequence.Particles()) != 3 {
				t.Fatalf("ordered child counts = %d/%d, want three", len(elements), len(sequence.Particles()))
			}
			want := []struct {
				name, typeName, occurrence string
				namespace                  string
				line                       int
				named                      bool
			}{
				{"qualified", "normalizedString", "0/18446744073709551616", "urn:test", 3, false},
				{"inherited", "Inherited", "2/unbounded", "", 4, true},
				{"forward", "Forward", "1/1", "", 5, true},
			}
			for index, expected := range want {
				element := elements[index]
				if element.Name() != mustTestQName(t, expected.namespace, expected.name) || element.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, expected.line, `<xs:element`) || element.Occurrences().String() != expected.occurrence {
					t.Fatalf("child %d name/loc/range = %q/%s/%s", index, element.Name(), element.Loc(), element.Occurrences())
				}
				typeName := mustTestQName(t, testXSDNamespace, expected.typeName)
				if expected.named {
					typeName = mustTestQName(t, "urn:test", expected.typeName)
				}
				if element.DeclaredType() != typeName {
					t.Fatalf("child %d declared type = %q, want %q", index, element.DeclaredType(), typeName)
				}
				reference, ok := element.TypeReference()
				if !ok || reference.Name() != typeName || reference.QName() != typeName || reference.Variety() != SimpleTypeVarietyAtomicRestriction || reference.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, expected.line, `type="`) {
					t.Fatalf("child %d reference = %#v/%t", index, reference, ok)
				}
				id, hasID := element.TypeID()
				refID, refHasID := reference.ComponentID()
				if hasID != expected.named || refHasID != expected.named || id != refID {
					t.Fatalf("child %d IDs = %v/%t and %v/%t", index, id, hasID, refID, refHasID)
				}
				if expected.named && id != componentIDForName(t, schema, typeName) {
					t.Fatalf("child %d named ID = %v", index, id)
				}
				whiteSpace, ok := reference.StringWhiteSpaceFacet()
				wantWhiteSpace := "replace"
				if index == 2 {
					wantWhiteSpace = "collapse"
				}
				if !ok || whiteSpace.Value() != wantWhiteSpace {
					t.Fatalf("child %d whitespace = %#v/%t", index, whiteSpace, ok)
				}
				if _, numericBounds := reference.IntegerBounds(); numericBounds {
					t.Fatalf("child %d acquired numeric bounds", index)
				}
			}
			builtIn, _ := elements[0].TypeReference()
			assertNormalizedStringReference(t, builtIn, mustSchemaTokenLoc(t, "root.xsd", root, 3, `type="xs:normalizedString"`))
			inherited, _ := elements[1].TypeReference()
			assertStringEnumerationFacts(t, inherited.StringEnumerationFacets(), profile.version, []string{"a\nb", " a  b "}, []Loc{
				mustSchemaTokenLoc(t, "root.xsd", root, 8, `value="a&#xA;b"`),
				mustSchemaTokenLoc(t, "root.xsd", root, 8, `value=" a  b "`),
			})
			forward, _ := elements[2].TypeReference()
			whiteSpace, ok := forward.StringWhiteSpaceFacet()
			if !ok || whiteSpace.Value() != "collapse" || whiteSpace.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, 9, `value="collapse"`) {
				t.Fatalf("forward whitespace = %#v/%t", whiteSpace, ok)
			}
			assertStringEnumerationFacts(t, forward.StringEnumerationFacets(), profile.version, []string{"a b"}, []Loc{mustSchemaTokenLoc(t, "root.xsd", root, 9, `value="a b"`)})
			values := inherited.StringEnumerationFacets().Values()
			values[0] = "changed"
			locations := inherited.StringEnumerationFacets().Locations()
			locations[0] = Loc{}
			elements[0] = ElementParticle{}
			particles := sequence.Particles()
			particles[0] = nil
			if got := sequence.Elements()[0].Name().Local(); got != "qualified" {
				t.Fatalf("element copy changed schema: %q", got)
			}
			if _, ok := sequence.Elements()[1].TypeReference(); !ok || sequence.Particles()[0] == nil {
				t.Fatal("returned slice changed schema")
			}
			if got := inherited.StringEnumerationFacets().Values()[0]; got != "a\nb" {
				t.Fatalf("facet copy changed schema: %q", got)
			}
			if inherited.StringEnumerationFacets().Locations()[0].IsZero() {
				t.Fatal("facet location copy changed schema")
			}
			repeated, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil || !reflect.DeepEqual(schema.Components(), repeated.Components()) {
				t.Fatalf("repeated schema facts/order changed: %v", err)
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

//nolint:gocognit // Every excluded owner and type shape must fail independently.
func TestNormalizedStringLocalSequenceShapeExclusions(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct {
			name, body, mark string
		}{
			{"choice built-in", `<xs:complexType name="Owner"><xs:choice><xs:element name="value" type="xs:normalizedString"/></xs:choice></xs:complexType>`, `type="xs:normalizedString"`},
			{"choice named", `<xs:complexType name="Owner"><xs:choice><xs:element name="value" type="t:T"/></xs:choice></xs:complexType><xs:simpleType name="T"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`, `type="t:T"`},
			{"all built-in", `<xs:complexType name="Owner"><xs:all><xs:element name="value" type="xs:normalizedString"/></xs:all></xs:complexType>`, `type="xs:normalizedString"`},
			{"all named", `<xs:complexType name="Owner"><xs:all><xs:element name="value" type="t:T"/></xs:all></xs:complexType><xs:simpleType name="T"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`, `type="t:T"`},
			{"extension built-in", `<xs:complexType name="Base"/><xs:complexType name="Owner"><xs:complexContent><xs:extension base="t:Base"><xs:sequence><xs:element name="value" type="xs:normalizedString"/></xs:sequence></xs:extension></xs:complexContent></xs:complexType>`, `type="xs:normalizedString"`},
			{"extension named", `<xs:complexType name="Base"/><xs:complexType name="Owner"><xs:complexContent><xs:extension base="t:Base"><xs:sequence><xs:element name="value" type="t:T"/></xs:sequence></xs:extension></xs:complexContent></xs:complexType><xs:simpleType name="T"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`, `type="t:T"`},
			{"inline owner built-in", `<xs:element name="root"><xs:complexType><xs:sequence><xs:element name="value" type="xs:normalizedString"/></xs:sequence></xs:complexType></xs:element>`, `type="xs:normalizedString"`},
			{"inline owner named", `<xs:element name="root"><xs:complexType><xs:sequence><xs:element name="value" type="t:T"/></xs:sequence></xs:complexType></xs:element><xs:simpleType name="T"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`, `type="t:T"`},
			{"anonymous local built-in base", `<xs:complexType name="Owner"><xs:sequence><xs:element name="value"><xs:simpleType><xs:restriction base="xs:normalizedString"/></xs:simpleType></xs:element></xs:sequence></xs:complexType>`, `<xs:simpleType`},
			{"anonymous local named base", `<xs:complexType name="Owner"><xs:sequence><xs:element name="value"><xs:simpleType><xs:restriction base="t:T"/></xs:simpleType></xs:element></xs:sequence></xs:complexType><xs:simpleType name="T"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`, `<xs:simpleType`},
			{"named list", `<xs:complexType name="Owner"><xs:sequence><xs:element name="value" type="t:T"/></xs:sequence></xs:complexType><xs:simpleType name="T"><xs:list itemType="xs:normalizedString"/></xs:simpleType>`, `type="t:T"`},
			{"named union", `<xs:complexType name="Owner"><xs:sequence><xs:element name="value" type="t:T"/></xs:sequence></xs:complexType><xs:simpleType name="T"><xs:union memberTypes="xs:normalizedString"/></xs:simpleType>`, `type="t:T"`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := normalizedStringSequenceRoot("\n  " + test.body + "\n")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || len(schema.Components()) != 0 || len(schema.Documents()) != 0 || schema.LanguagePolicy() != "" {
					t.Fatalf("excluded shape returned schema or no error: %v", err)
				}
				diagnostic := requireDiagnostic(t, err)
				wantLoc := mustSchemaTokenLoc(t, "root.xsd", root, 2, test.mark)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != schemaSyntaxSpecRefForVersion(profile.version) || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("diagnostic = %v; class=%s code=%s loc=%s spec=%s, want unsupported at %s", err, diagnostic.Class(), diagnostic.Code(), diagnostic.Loc(), diagnostic.SpecRef(), wantLoc)
				}
				if len(diagnostic.Related()) != 0 {
					t.Fatalf("unexpected related locations: %v", diagnostic.Related())
				}
			})
		}
	}
}

//nolint:gocognit // ValidateInstance and GenerateGo are independent public consumers.
func TestNormalizedStringDirectSequenceConsumersRejectNonzero(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct{ name, typeName, definition, baseMarker string }{
			{"built-in", "xs:normalizedString", "", ""},
			{"named", "t:T", `<xs:simpleType name="T"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`, `base="xs:normalizedString"`},
			{"inherited", "t:Child", `<xs:simpleType name="Child"><xs:restriction base="t:Base"/></xs:simpleType><xs:simpleType name="Base"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`, `base="t:Base"`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := normalizedStringSequenceRoot("\n  <xs:complexType name=\"Owner\"><xs:sequence><xs:element name=\"value\" type=\"" + test.typeName + "\"/></xs:sequence></xs:complexType>\n  <xs:element name=\"root\" type=\"t:Owner\"/>\n  " + test.definition + "\n")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("ParseSchema: %v", err)
				}
				generated, err := GenerateGo(schema, "generated")
				if err == nil || generated != nil {
					t.Fatalf("GenerateGo = %q/%v, want nil and unsupported", generated, err)
				}
				generation := requireDiagnostic(t, err)
				wantChildLoc := mustSchemaTokenLoc(t, "root.xsd", root, 2, `<xs:element name="value"`)
				wantRelated := []Loc(nil)
				if test.name != "built-in" {
					wantRelated = []Loc{
						mustSchemaTokenLoc(t, "root.xsd", root, 4, `<xs:simpleType`),
						mustSchemaTokenLoc(t, "root.xsd", root, 4, test.baseMarker),
					}
				}
				wantSpec := "xsd11-structures#element-sequence"
				if profile.version == XSDVersion10 {
					wantSpec = "xsd10-structures#element-sequence"
				}
				if test.name != "built-in" {
					wantSpec = schemaSimpleTypeSpecRef(profile.version)
				}
				if generation.Class() != FailureUnsupported || generation.Code() != diagnosticCodegenUnsupported || generation.Loc() != wantChildLoc || generation.SpecRef() != wantSpec || !reflect.DeepEqual(generation.Related(), wantRelated) || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("generation diagnostic = %v; related=%v", err, generation.Related())
				}
				err = ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:test"><value xmlns="">a b</value></root>`)))
				if err == nil {
					t.Fatal("ValidateInstance admitted normalizedString local")
				}
				validation := requireDiagnostic(t, err)
				wantValidationRelated := []Loc{
					mustSchemaTokenLoc(t, "root.xsd", root, 3, `<xs:element name="root"`),
					mustSchemaTokenLoc(t, "root.xsd", root, 2, `<xs:complexType`),
					mustSchemaTokenLoc(t, "root.xsd", root, 2, `<xs:sequence`),
					wantChildLoc,
				}
				if test.name != "built-in" {
					wantValidationRelated = append(wantValidationRelated, mustSchemaTokenLoc(t, "root.xsd", root, 4, `<xs:simpleType`))
				}
				if validation.Class() != FailureUnsupported || validation.Code() != UnsupportedInstanceValidationCode || validation.Loc() != mustTestLoc(t, "instance.xml", 1, 1) || validation.SpecRef() != instanceValidationSpecRef(profile.version) || !reflect.DeepEqual(validation.Related(), wantValidationRelated) || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("validation diagnostic = %v; related=%v", err, validation.Related())
				}
			})
		}
	}
}

//nolint:gocognit // Built-in and named zero terms must leave the same consumer projection.
func TestNormalizedStringDirectSequenceZeroTermOmitsBeforeConsumers(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct{ name, typeName, definition string }{
			{"built-in", "xs:normalizedString", ""},
			{"named", "t:T", `<xs:simpleType name="T"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := normalizedStringSequenceRoot("\n  <xs:complexType name=\"Owner\"><xs:sequence><xs:element name=\"omitted\" type=\"" + test.typeName + "\" minOccurs=\"0\" maxOccurs=\"0\"/><xs:element name=\"count\" type=\"xs:integer\"/></xs:sequence></xs:complexType>\n  <xs:element name=\"root\" type=\"t:Owner\"/>" + test.definition + "\n")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err != nil {
					t.Fatalf("ParseSchema: %v", err)
				}
				children := normalizedStringSequenceType(t, schema, "urn:test", "Owner").Elements()
				if len(children) != 1 || children[0].Name().Local() != "count" {
					t.Fatalf("0/0 omission = %#v, want only count", children)
				}
				validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:test"><count xmlns="">1</count></root>`)))
				if validationErr != nil {
					t.Fatalf("ValidateInstance after omission: %v", validationErr)
				}
				generated, err := GenerateGo(schema, "generated")
				if test.name == "named" {
					if err == nil || generated != nil {
						t.Fatalf("named global type GenerateGo = %q/%v, want nil unsupported", generated, err)
					}
					diagnostic := requireDiagnostic(t, err)
					wantLoc := mustSchemaTokenLoc(t, "root.xsd", root, 3, `<xs:simpleType`)
					wantRelated := []Loc{mustSchemaTokenLoc(t, "root.xsd", root, 3, `base="xs:normalizedString"`)}
					if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != diagnosticCodegenUnsupported || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != schemaSimpleTypeSpecRef(profile.version) || !reflect.DeepEqual(diagnostic.Related(), wantRelated) || !errors.Is(err, ErrUnsupported) {
						t.Fatalf("named global type diagnostic = %v; related=%v", err, diagnostic.Related())
					}
					return
				}
				if err != nil || len(generated) == 0 {
					t.Fatalf("GenerateGo after omission = %q/%v", generated, err)
				}
			})
		}
	}
}

//nolint:gocognit,funlen // Graph sources carry distinct visibility, form defaults, and authored locations.
func TestNormalizedStringDirectSequenceGraphProvenance(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" elementFormDefault="qualified" version="` + string(profile.version) + `">
  <xs:include schemaLocation="included.xsd"/>
  <xs:include schemaLocation="included.xsd"/>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:complexType name="RootOwner"><xs:sequence>
    <xs:element name="builtIn" type="xs:normalizedString"/>
    <xs:element name="included" type="r:Included"/>
    <xs:element name="chameleon" type="r:Chameleon"/>
    <xs:element name="imported" type="o:Imported"/>
    <xs:element name="forward" type="r:Forward"/>
  </xs:sequence></xs:complexType>
  <xs:simpleType name="Forward"><xs:restriction base="r:Included"/></xs:simpleType>
</xs:schema>`
			fixtures := map[string]discoveryFixture{
				"root.xsd": {id: "root.xsd", contents: root},
				"included.xsd": {id: "included.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" elementFormDefault="unqualified">
  <xs:include schemaLocation="root.xsd"/>
  <xs:simpleType name="Included"><xs:restriction base="xs:normalizedString"><xs:enumeration value="a&#xA;b"/></xs:restriction></xs:simpleType>
  <xs:complexType name="IncludedOwner"><xs:sequence><xs:element name="local" type="r:Included"/></xs:sequence></xs:complexType>
</xs:schema>`},
				"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" elementFormDefault="unqualified">
  <xs:simpleType name="Chameleon"><xs:restriction base="xs:normalizedString"/></xs:simpleType>
  <xs:complexType name="ChameleonOwner"><xs:sequence><xs:element name="local" type="r:Chameleon"/></xs:sequence></xs:complexType>
</xs:schema>`},
				"other.xsd": {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:other" elementFormDefault="unqualified">
  <xs:simpleType name="Imported"><xs:restriction base="xs:normalizedString"/></xs:simpleType>
  <xs:complexType name="ImportedOwner"><xs:sequence><xs:element name="local" type="o:Imported"/></xs:sequence></xs:complexType>
</xs:schema>`},
			}
			first, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatalf("ParseSchema graph: %v", err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil || !reflect.DeepEqual(first.Components(), second.Components()) {
				t.Fatalf("repeated graph changed facts/order: %v", err)
			}
			documents := first.Documents()
			if len(documents) != 4 {
				t.Fatalf("documents = %d, want root, included, chameleon, imported", len(documents))
			}
			for index, want := range []SourceID{"root.xsd", "included.xsd", "chameleon.xsd", "other.xsd"} {
				if documents[index].Source() != want {
					t.Fatalf("document %d = %s, want %s", index, documents[index].Source(), want)
				}
			}
			rootChildren := normalizedStringSequenceType(t, first, "urn:root", "RootOwner").Elements()
			if len(rootChildren) != 5 {
				t.Fatalf("root children = %d, want five", len(rootChildren))
			}
			for index, want := range []struct {
				local, typeLocal, typeNamespace, typeLexical string
				targetSource                                 SourceID
			}{
				{"builtIn", "normalizedString", testXSDNamespace, "xs:normalizedString", ""},
				{"included", "Included", "urn:root", "r:Included", "included.xsd"},
				{"chameleon", "Chameleon", "urn:root", "r:Chameleon", "chameleon.xsd"},
				{"imported", "Imported", "urn:other", "o:Imported", "other.xsd"},
				{"forward", "Forward", "urn:root", "r:Forward", "root.xsd"},
			} {
				child := rootChildren[index]
				wantType := mustTestQName(t, want.typeNamespace, want.typeLocal)
				if child.Name() != mustTestQName(t, "urn:root", want.local) || child.DeclaredType() != wantType || child.Loc() != schemaBuiltinReferenceAttributeLoc(t, "root.xsd", `<xs:element name="`+want.local+`"`, root, fixtures) {
					t.Fatalf("root child %d facts = %q/%q/%s", index, child.Name(), child.DeclaredType(), child.Loc())
				}
				reference, ok := child.TypeReference()
				if !ok || reference.QName() != wantType || reference.Loc() != schemaBuiltinReferenceAttributeLoc(t, "root.xsd", `type="`+want.typeLexical+`"`, root, fixtures) {
					t.Fatalf("root child %d reference = %#v/%t", index, reference, ok)
				}
				id, hasID := child.TypeID()
				if want.targetSource == "" {
					if hasID || !id.IsZero() || !reference.IsBuiltin() {
						t.Fatalf("built-in child ID = %v/%t", id, hasID)
					}
					continue
				}
				if !hasID || !reference.IsNamed() || id.Source() != want.targetSource || id != componentIDForName(t, first, wantType) {
					t.Fatalf("root child %d ID = %v/%t", index, id, hasID)
				}
			}
			includedRef, _ := rootChildren[1].TypeReference()
			assertStringEnumerationFacts(t, includedRef.StringEnumerationFacets(), profile.version, []string{"a\nb"}, []Loc{schemaBuiltinReferenceAttributeLoc(t, "included.xsd", `value="a&#xA;b"`, root, fixtures)})
			for _, owner := range []struct {
				namespace, local, source string
			}{
				{"urn:root", "IncludedOwner", "included.xsd"},
				{"urn:root", "ChameleonOwner", "chameleon.xsd"},
				{"urn:other", "ImportedOwner", "other.xsd"},
			} {
				children := normalizedStringSequenceType(t, first, owner.namespace, owner.local).Elements()
				if len(children) != 1 || children[0].Name() != mustTestQName(t, "", "local") || children[0].Loc().Source() != SourceID(owner.source) {
					t.Fatalf("%s local form/provenance = %#v", owner.local, children)
				}
			}
		})
	}
}

//nolint:gocognit // Alternate exits at the local term boundary must precede zero omission.
func TestNormalizedStringDirectSequenceZeroBoundaryDiagnostics(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, test := range []struct {
			name, child, tail, mark, code, spec string
			related                             []string
			class                               FailureClass
			cause                               error
		}{
			{"unresolved", `<xs:element name="value" type="t:Missing" minOccurs="0" maxOccurs="0"/>`, "", `type="t:Missing"`, diagnosticSchemaElementTypeUnresolvedCode, schemaElementTypeSpecRef(profile.version), nil, FailureInvalid, errSchemaElementTypeUnresolved},
			{"wrong kind", `<xs:element name="value" type="t:T" minOccurs="0" maxOccurs="0"/>`, `<xs:element name="T" type="xs:normalizedString"/>`, `type="t:T"`, diagnosticSchemaElementTypeWrongKindCode, schemaElementTypeSpecRef(profile.version), []string{`<xs:element name="T"`}, FailureInvalid, errSchemaElementTypeWrongKind},
			{"invalid range", `<xs:element name="value" type="xs:normalizedString" minOccurs="2" maxOccurs="1"/>`, "", `<xs:element name="value"`, invalidSchemaCompositionCode, schemaParticleCorrectSpecRef(profile.version), []string{`minOccurs="2"`, `maxOccurs="1"`}, FailureInvalid, errParticleOccurrenceMinimumExceedsMaximum},
			{"invalid occurrence lexical", `<xs:element name="value" type="xs:normalizedString" minOccurs="0" maxOccurs="many"/>`, "", `maxOccurs="many"`, invalidSchemaCompositionCode, schemaParticleOccurrenceDatatypeSpecRef(profile.version), nil, FailureInvalid, nil},
			{"malformed type QName", `<xs:element name="value" type="t:bad:name" minOccurs="0" maxOccurs="0"/>`, "", `type="t:bad:name"`, invalidSchemaConditionalCode, "", nil, FailureInvalid, nil},
			{"invalid whitespace", `<xs:element name="value" type="t:T" minOccurs="0" maxOccurs="0"/>`, `<xs:simpleType name="T"><xs:restriction base="xs:normalizedString"><xs:whiteSpace value="preserve"/></xs:restriction></xs:simpleType>`, `value="preserve"`, InvalidStringWhiteSpaceRestrictionCode, stringWhiteSpaceSpecRef(profile.version), nil, FailureInvalid, errInvalidStringWhiteSpaceRestriction},
			{"unsupported facet", `<xs:element name="value" type="t:T" minOccurs="0" maxOccurs="0"/>`, `<xs:simpleType name="T"><xs:restriction base="xs:normalizedString"><xs:length value="2"/></xs:restriction></xs:simpleType>`, `<xs:length`, UnsupportedDatatypeFacetCode, tokenDiagnosticSpecRef(profile.version, "decimal"), nil, FailureUnsupported, ErrUnsupported},
			{"cyclic type", `<xs:element name="value" type="t:A" minOccurs="0" maxOccurs="0"/>`, `<xs:simpleType name="A"><xs:restriction base="t:B"/></xs:simpleType><xs:simpleType name="B"><xs:restriction base="t:A"/></xs:simpleType><xs:simpleType name="Seed"><xs:restriction base="xs:normalizedString"/></xs:simpleType>`, `base="t:B"`, diagnosticSchemaSimpleTypeCycleCode, schemaSimpleTypeSpecRef(profile.version), []string{`base="t:A"`}, FailureInvalid, errSchemaSimpleTypeBaseCycle},
			{"name and ref", `<xs:element name="value" ref="t:T" minOccurs="0" maxOccurs="0"/>`, `<xs:element name="T" type="xs:normalizedString"/>`, `ref="t:T"`, invalidSchemaCompositionCode, "", nil, FailureInvalid, nil},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := normalizedStringSequenceRoot("\n  <xs:complexType name=\"Owner\"><xs:sequence>" + test.child + "</xs:sequence></xs:complexType>" + test.tail + "\n")
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil {
					t.Fatal("bad zero term returned no error")
				}
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantLoc := mustSchemaTokenLoc(t, "root.xsd", root, 2, test.mark)
				var wantRelated []Loc
				for _, marker := range test.related {
					wantRelated = append(wantRelated, mustSchemaTokenLoc(t, "root.xsd", root, 2, marker))
				}
				if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != test.spec || !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
					t.Fatalf("%s diagnostic = %v; related=%v spec=%s, want %s at %s related=%v spec=%s", test.name, err, diagnostic.Related(), diagnostic.SpecRef(), test.code, wantLoc, wantRelated, test.spec)
				}
				if test.cause != nil && !errors.Is(err, test.cause) {
					t.Fatalf("%s lost cause %v: %v", test.name, test.cause, err)
				}
			})
		}
	}
}

func TestNormalizedStringZeroOuterSequenceStillResolvesChildren(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := normalizedStringSequenceRoot(`
  <xs:complexType name="Owner"><xs:sequence minOccurs="0" maxOccurs="0"><xs:element name="value" type="t:Missing"/></xs:sequence></xs:complexType>
`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err == nil {
				t.Fatal("zero outer sequence hid an unresolved child type")
			}
			assertZeroSchema(t, schema)
			diagnostic := requireDiagnostic(t, err)
			wantLoc := mustSchemaTokenLoc(t, "root.xsd", root, 2, `type="t:Missing"`)
			if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaElementTypeUnresolvedCode || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != schemaElementTypeSpecRef(profile.version) || len(diagnostic.Related()) != 0 || !errors.Is(err, errSchemaElementTypeUnresolved) {
				t.Fatalf("zero outer diagnostic = %v; related=%v", err, diagnostic.Related())
			}
		})
	}
}

func TestNormalizedStringDirectSequenceHiddenTypesStayUnresolved(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		for _, relation := range []string{"unimported", "indirect-import"} {
			t.Run(profile.name+"/"+relation, func(t *testing.T) {
				root, fixtures := typeVisibilityTestHiddenGraph(t, string(profile.version), relation, false, true, true, "")
				root = strings.Replace(root, `<xs:choice><xs:element name="item" type="f:Hidden"/></xs:choice>`, `<xs:sequence><xs:element name="item" type="f:Hidden"/></xs:sequence>`, 1)
				foreign := fixtures["foreign.xsd"]
				foreign.contents = strings.Replace(foreign.contents, `base="xs:integer"`, `base="xs:normalizedString"`, 1)
				fixtures["foreign.xsd"] = foreign
				schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
				if err == nil {
					t.Fatal("invisible named type returned no error")
				}
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantLoc := schemaBuiltinReferenceAttributeLoc(t, "root.xsd", `type="f:Hidden"`, root, fixtures)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != diagnosticSchemaElementTypeUnresolvedCode || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != schemaElementTypeSpecRef(profile.version) || len(diagnostic.Related()) != 0 || !errors.Is(err, errSchemaElementTypeUnresolved) {
					t.Fatalf("hidden type diagnostic = %v; related=%v", err, diagnostic.Related())
				}
			})
		}
	}
}

//nolint:gocognit // A global element ref is a separate query shape and both consumers reject it.
func TestNormalizedStringDirectSequenceReferenceConsumers(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := normalizedStringSequenceRoot(`
  <xs:complexType name="Owner"><xs:sequence><xs:element ref="t:item"/></xs:sequence></xs:complexType>
  <xs:element name="root" type="t:Owner"/>
  <xs:element name="item" type="xs:normalizedString"/>
`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("ParseSchema: %v", err)
			}
			sequence := normalizedStringSequenceType(t, schema, "urn:test", "Owner")
			terms := sequence.Particles()
			if len(terms) != 1 || len(sequence.Elements()) != 0 {
				t.Fatalf("reference terms/elements = %d/%d", len(terms), len(sequence.Elements()))
			}
			reference, ok := terms[0].(ElementReferenceParticle)
			wantRefLoc := mustSchemaTokenLoc(t, "root.xsd", root, 2, `ref="t:item"`)
			targets := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:test", "item"))
			if len(targets) != 1 {
				t.Fatalf("target elements = %d, want one", len(targets))
			}
			if !ok || reference.Name() != mustTestQName(t, "urn:test", "item") || reference.RefLoc() != wantRefLoc || reference.Occurrences().String() != "1/1" || reference.TargetID() != targets[0].ID() {
				t.Fatalf("reference facts = %#v/%t", reference, ok)
			}
			output, err := GenerateGo(schema, "generated")
			if err == nil || output != nil {
				t.Fatalf("GenerateGo = %q/%v, want nil unsupported", output, err)
			}
			generation := requireDiagnostic(t, err)
			wantGenerationRelated := []Loc{
				mustSchemaTokenLoc(t, "root.xsd", root, 2, `<xs:element`),
				mustSchemaTokenLoc(t, "root.xsd", root, 4, `<xs:element`),
			}
			wantSpec := "xsd11-structures#element-sequence"
			if profile.version == XSDVersion10 {
				wantSpec = "xsd10-structures#element-sequence"
			}
			if generation.Class() != FailureUnsupported || generation.Code() != diagnosticCodegenUnsupported || generation.Loc() != wantRefLoc || generation.SpecRef() != wantSpec || !reflect.DeepEqual(generation.Related(), wantGenerationRelated) || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("generation diagnostic = %v; related=%v", err, generation.Related())
			}
			err = ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<root xmlns="urn:test"><item>a b</item></root>`)))
			if err == nil {
				t.Fatal("ValidateInstance admitted normalizedString ref")
			}
			validation := requireDiagnostic(t, err)
			wantValidationRelated := []Loc{
				mustSchemaTokenLoc(t, "root.xsd", root, 3, `<xs:element`),
				mustSchemaTokenLoc(t, "root.xsd", root, 2, `<xs:complexType`),
				mustSchemaTokenLoc(t, "root.xsd", root, 2, `<xs:sequence`),
				mustSchemaTokenLoc(t, "root.xsd", root, 2, `<xs:element`),
				wantRefLoc,
			}
			if validation.Class() != FailureUnsupported || validation.Code() != UnsupportedInstanceValidationCode || validation.Loc() != mustTestLoc(t, "instance.xml", 1, 1) || validation.SpecRef() != instanceValidationSpecRef(profile.version) || !reflect.DeepEqual(validation.Related(), wantValidationRelated) || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("validation diagnostic = %v; related=%v", err, validation.Related())
			}
		})
	}
}

//nolint:gocognit // Keep edition admission and zero-term omission at one policy boundary.
func TestNormalizedStringDirectSequenceTargetNamespacePolicy(t *testing.T) {
	for _, profile := range tokenPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := normalizedStringSequenceRoot(`
  <xs:complexType name="Owner"><xs:sequence><xs:element name="qualified" type="xs:normalizedString" targetNamespace="urn:test"/><xs:element name="omitted" type="xs:normalizedString" targetNamespace="urn:test" minOccurs="0" maxOccurs="0"/></xs:sequence></xs:complexType>
`)
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if profile.policy == Strict10 {
				if err == nil {
					t.Fatal("Strict10 accepted a local targetNamespace")
				}
				assertZeroSchema(t, schema)
				diagnostic := requireDiagnostic(t, err)
				wantLoc := mustSchemaTokenLoc(t, "root.xsd", root, 2, `targetNamespace="urn:test"`)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != schemaSyntaxSpecRefForVersion(XSDVersion11) || len(diagnostic.Related()) != 0 || !errors.Is(err, errLanguagePolicyMismatch) {
					t.Fatalf("policy diagnostic = %v; related=%v", err, diagnostic.Related())
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseSchema: %v", err)
			}
			children := normalizedStringSequenceType(t, schema, "urn:test", "Owner").Elements()
			if len(children) != 1 || children[0].Name() != mustTestQName(t, "urn:test", "qualified") {
				t.Fatalf("targetNamespace/0/0 facts = %#v", children)
			}
		})
	}
}
