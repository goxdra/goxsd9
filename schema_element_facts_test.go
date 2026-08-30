package goxsd9

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit,funlen // Keep the edition, lexical, and scalar-kind matrix together.
func TestSchemaBridgeExposesGlobalElementBooleanFacts(t *testing.T) {
	policies := []struct {
		name    string
		policy  LanguagePolicy
		version XSDVersion
	}{
		{name: "XSD 1.0", policy: Strict10, version: XSDVersion10},
		{name: "XSD 1.1", policy: Strict11, version: XSDVersion11},
	}
	lexicals := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "absent", want: false},
		{name: "true", value: "true", want: true},
		{name: "false", value: "false", want: false},
		{name: "one", value: "1", want: true},
		{name: "zero", value: "0", want: false},
	}
	for _, fact := range []string{"abstract", "nillable"} {
		for _, lexical := range lexicals {
			for _, policy := range policies {
				t.Run(fact+"/"+lexical.name+"/"+policy.name, func(t *testing.T) {
					attribute := ""
					if lexical.value != "" {
						attribute = fmt.Sprintf(` %s=%q`, fact, lexical.value)
					}
					root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(policy.version) + `">
  <xs:element name="directInteger" type="xs:integer"` + attribute + `/>
  <xs:element name="namedInteger" type="r:Integer"` + attribute + `/>
  <xs:element name="directBoolean" type="xs:boolean"` + attribute + `/>
  <xs:element name="namedBoolean" type="r:Boolean"` + attribute + `/>
  <xs:simpleType name="Integer"><xs:restriction base="xs:integer"/></xs:simpleType>
  <xs:simpleType name="Boolean"><xs:restriction base="xs:boolean"/></xs:simpleType>
</xs:schema>`
					schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
					if err != nil {
						t.Fatalf("discoverSchema: %v", err)
					}
					components := schema.Components()
					if got, want := len(components), 6; got != want {
						t.Fatalf("component count = %d, want %d", got, want)
					}
					wantNames := []QName{
						mustTestQName(t, "urn:root", "directInteger"),
						mustTestQName(t, "urn:root", "namedInteger"),
						mustTestQName(t, "urn:root", "directBoolean"),
						mustTestQName(t, "urn:root", "namedBoolean"),
						mustTestQName(t, "urn:root", "Integer"),
						mustTestQName(t, "urn:root", "Boolean"),
					}
					for index, wantName := range wantNames {
						if components[index].Name() != wantName || components[index].ID().Ordinal() != uint64(index+1) {
							t.Fatalf("component %d = %q/%d, want %q/%d", index, components[index].Name(), components[index].ID().Ordinal(), wantName, index+1)
						}
						declaration, ok := components[index].ElementDeclaration()
						if index >= 4 {
							if ok {
								t.Fatalf("component %d unexpectedly has an element view", index)
							}
							continue
						}
						if !ok {
							t.Fatalf("component %d has no element view", index)
						}
						if got := declaration.IsAbstract(); got != (fact == "abstract" && lexical.want) {
							t.Fatalf("component %d IsAbstract() = %t, want %t", index, got, fact == "abstract" && lexical.want)
						}
						if got := declaration.IsNillable(); got != (fact == "nillable" && lexical.want) {
							t.Fatalf("component %d IsNillable() = %t, want %t", index, got, fact == "nillable" && lexical.want)
						}
					}
					before := schema.Components()
					found := schema.FindKind(ComponentKindElementDeclaration, wantNames[0])
					if len(found) != 1 {
						t.Fatalf("FindKind count = %d, want 1", len(found))
					}
					found[0] = Component{}
					returned := schema.Components()
					returned[0] = Component{}
					documents := schema.Documents()
					if len(documents) != 1 {
						t.Fatalf("document count = %d, want 1", len(documents))
					}
					documentComponents := documents[0].Components()
					documentComponents[0] = Component{}
					if !reflect.DeepEqual(before, schema.Components()) {
						t.Fatal("mutating returned component slices changed Schema")
					}
					declaration, ok := schema.Components()[0].ElementDeclaration()
					if !ok || declaration.IsAbstract() != (fact == "abstract" && lexical.want) || declaration.IsNillable() != (fact == "nillable" && lexical.want) {
						t.Fatal("repeated element fact query changed after returned-slice mutation")
					}
				})
			}
		}
	}
}

//nolint:gocognit // Keep forward and cross-document identity assertions together.
func TestSchemaBridgePreservesGlobalElementFactsAcrossForwardAndCrossDocumentTypes(t *testing.T) {
	for _, policy := range []struct {
		name    string
		value   LanguagePolicy
		version XSDVersion
	}{
		{name: "XSD 1.0", value: Strict10, version: XSDVersion10},
		{name: "XSD 1.1", value: Strict11, version: XSDVersion11},
	} {
		t.Run(policy.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="` + string(policy.version) + `">
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:element name="forward" type="r:Forward" abstract="true"/>
  <xs:element name="cross" type="o:Cross" nillable="1"/>
  <xs:simpleType name="Forward"><xs:restriction base="xs:integer"/></xs:simpleType>
</xs:schema>`
			other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other" version="` + string(policy.version) + `">
  <xs:simpleType name="Cross"><xs:restriction base="xs:boolean"/></xs:simpleType>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
				"other.xsd": {id: "other.xsd", contents: other},
			}, policy.value)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}
			components := schema.Components()
			if got, want := len(components), 4; got != want {
				t.Fatalf("component count = %d, want %d", got, want)
			}
			wantNames := []QName{
				mustTestQName(t, "urn:root", "forward"),
				mustTestQName(t, "urn:root", "cross"),
				mustTestQName(t, "urn:root", "Forward"),
				mustTestQName(t, "urn:other", "Cross"),
			}
			for index, want := range wantNames {
				if components[index].Name() != want {
					t.Fatalf("component %d name = %q, want %q", index, components[index].Name(), want)
				}
			}
			if components[0].ID().Ordinal() != 1 || components[1].ID().Ordinal() != 2 || components[2].ID().Ordinal() != 3 || components[3].ID().Ordinal() != 1 {
				t.Fatalf("component ordinals = %d/%d/%d/%d, want 1/2/3/1", components[0].ID().Ordinal(), components[1].ID().Ordinal(), components[2].ID().Ordinal(), components[3].ID().Ordinal())
			}
			forward, ok := components[0].ElementDeclaration()
			if !ok || !forward.IsAbstract() || forward.IsNillable() {
				t.Fatal("forward declaration facts are incorrect")
			}
			cross, ok := components[1].ElementDeclaration()
			if !ok || cross.IsAbstract() || !cross.IsNillable() {
				t.Fatal("cross-document declaration facts are incorrect")
			}
			if typeID, ok := forward.TypeID(); !ok || typeID != components[2].ID() {
				t.Fatalf("forward type ID = %v/%t, want %v/true", typeID, ok, components[2].ID())
			}
			if typeID, ok := cross.TypeID(); !ok || typeID != components[3].ID() {
				t.Fatalf("cross-document type ID = %v/%t, want %v/true", typeID, ok, components[3].ID())
			}
		})
	}
}

//nolint:gocognit // Keep edition and attribute-location assertions together.
func TestSchemaBridgeRejectsMalformedGlobalElementFactsAtAttribute(t *testing.T) {
	for _, policy := range []struct {
		name    string
		value   LanguagePolicy
		version XSDVersion
	}{
		{name: "XSD 1.0", value: Strict10, version: XSDVersion10},
		{name: "XSD 1.1", value: Strict11, version: XSDVersion11},
	} {
		for _, fact := range []string{"abstract", "nillable"} {
			t.Run(policy.name+"/"+fact, func(t *testing.T) {
				line := `  <xs:element name="item" type="xs:integer" ` + fact + `="maybe"/>`
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="` + string(policy.version) + `">` + "\n" + line + "\n</xs:schema>"
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.value)
				if err == nil {
					t.Fatal("discoverSchema accepted a malformed global element boolean")
				}
				if schema.storage != nil || len(schema.Components()) != 0 {
					t.Fatal("discoverSchema returned a partial schema")
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInvalid || diagnostic.Code() != invalidSchemaCompositionCode {
					t.Fatalf("diagnostic = %s, want invalid/%s", diagnostic, invalidSchemaCompositionCode)
				}
				column := strings.Index(line, fact) + 1
				if diagnostic.Loc() != mustTestLoc(t, "root.xsd", 2, column) {
					t.Fatalf("diagnostic location = %s, want attribute location root.xsd:2:%d", diagnostic.Loc(), column)
				}
			})
		}
	}
}

//nolint:gocognit // Keep valid exclusions and their diagnostic boundary together.
func TestSchemaBridgeKeepsElementFactExclusionsUnsupported(t *testing.T) {
	cases := []struct {
		name    string
		root    string
		policy  LanguagePolicy
		version XSDVersion
	}{
		{
			name:    "untyped abstract",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="item" abstract="true"/></xs:schema>`,
			version: XSDVersion11,
		},
		{
			name:    "anonymous nillable",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="item" nillable="true"><xs:complexType><xs:choice/></xs:complexType></xs:element></xs:schema>`,
			version: XSDVersion11,
		},
		{
			name:    "local particle nillable",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="Record"><xs:choice><xs:element name="item" type="xs:integer" nillable="true"/></xs:choice></xs:complexType></xs:schema>`,
			version: XSDVersion11,
		},
		{
			name:    "local reference",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root"><xs:complexType name="Record"><xs:choice><xs:element ref="r:item"/></xs:choice></xs:complexType></xs:schema>`,
			version: XSDVersion11,
		},
		{
			name:    "value constraint",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="item" type="xs:integer" abstract="true" default="1"/></xs:schema>`,
			version: XSDVersion11,
		},
		{
			name:    "substitution",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root"><xs:element name="item" type="xs:integer" nillable="true" substitutionGroup="r:head"/></xs:schema>`,
			version: XSDVersion11,
		},
		{
			name:    "block",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="item" type="xs:integer" abstract="true" block="extension"/></xs:schema>`,
			version: XSDVersion11,
		},
		{
			name:    "final",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:element name="item" type="xs:integer" nillable="true" final="extension"/></xs:schema>`,
			version: XSDVersion11,
		},
		{
			name:    "alternative",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="1.1"><xs:element name="item" type="xs:integer" abstract="true"><xs:alternative type="xs:integer"/></xs:element></xs:schema>`,
			policy:  Strict11,
			version: XSDVersion11,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			policy := test.policy
			if policy == "" {
				policy = Compatibility
			}
			schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, policy)
			if err == nil {
				t.Fatal("discoverSchema accepted an excluded element shape")
			}
			if schema.storage != nil || len(schema.Components()) != 0 {
				t.Fatal("discoverSchema returned a partial schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Feature() != FeatureSchemaSyntax || diagnostic.Code() != UnsupportedSchemaSyntaxCode {
				t.Fatalf("diagnostic = %s, want registered schema-syntax unsupported", diagnostic)
			}
			if diagnostic.Loc().IsZero() || diagnostic.Loc().Source() != "root.xsd" || diagnostic.SpecRef() == "" {
				t.Fatalf("diagnostic evidence = %s/%q, want located specification-backed diagnostic", diagnostic.Loc(), diagnostic.SpecRef())
			}
			if !errors.Is(err, ErrUnsupported) {
				t.Fatalf("diagnostic does not match ErrUnsupported: %v", err)
			}
		})
	}
}

//nolint:gocognit // Keep scalar, choice, and precedence validation checks together.
func TestValidateInstanceRejectsNonDefaultGlobalElementFactsBeforeDispatch(t *testing.T) {
	for _, policy := range []struct {
		name    string
		value   LanguagePolicy
		version XSDVersion
	}{
		{name: "XSD 1.0", value: Strict10, version: XSDVersion10},
		{name: "XSD 1.1", value: Strict11, version: XSDVersion11},
	} {
		t.Run(policy.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="` + string(policy.version) + `">
  <xs:element name="abstractRoot" type="xs:integer" abstract="true"/>
  <xs:element name="nillableRoot" type="xs:integer" nillable="true"/>
  <xs:element name="bothRoot" type="xs:integer" nillable="true" abstract="true"/>
  <xs:element name="ordinaryRoot" type="xs:integer" abstract="false" nillable="0"/>
  <xs:complexType name="Choice"><xs:choice><xs:element name="value" type="xs:integer"/></xs:choice></xs:complexType>
  <xs:element name="choiceRoot" type="r:Choice" abstract="true"/>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.value)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}
			cases := []struct {
				name  string
				input string
				fact  string
			}{
				{name: "abstract scalar", input: `<abstractRoot xmlns="urn:root">1</abstractRoot>`, fact: "abstract=true"},
				{name: "nillable scalar", input: `<nillableRoot xmlns="urn:root">1</nillableRoot>`, fact: "nillable=true"},
				{name: "both precedence", input: `<bothRoot xmlns="urn:root">1</bothRoot>`, fact: "abstract=true"},
				{name: "abstract choice", input: `<choiceRoot xmlns="urn:root"><value xmlns="">1</value></choiceRoot>`, fact: "abstract=true"},
			}
			for _, test := range cases {
				t.Run(test.name, func(t *testing.T) {
					err := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(test.input)))
					if err == nil {
						t.Fatal("ValidateInstance accepted a non-default global element fact")
					}
					diagnostic := requireDiagnostic(t, err)
					if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedInstanceValidationCode || diagnostic.Feature() != FeatureInstanceValidation {
						t.Fatalf("diagnostic = %s, want registered instance-validation unsupported", diagnostic)
					}
					wantSpec := "xsd11-structures#cvc-elt"
					if policy.value == Strict10 {
						wantSpec = "xsd10-structures#cvc-elt"
					}
					if diagnostic.SpecRef() != wantSpec {
						t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), wantSpec)
					}
					if diagnostic.Loc() != mustTestLoc(t, "instance.xml", 1, 1) {
						t.Fatalf("diagnostic location = %s, want instance root", diagnostic.Loc())
					}
					name := strings.TrimSuffix(strings.TrimPrefix(strings.Fields(test.input)[0], "<"), ">")
					name = strings.Split(name, " ")[0]
					declarations := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:root", name))
					if len(declarations) != 1 || !containsLoc(diagnostic.Related(), declarations[0].Loc()) {
						t.Fatalf("diagnostic related = %v, want declaration location", diagnostic.Related())
					}
					if !strings.Contains(diagnostic.Message(), test.fact) {
						t.Fatalf("diagnostic message = %q, want %q", diagnostic.Message(), test.fact)
					}
					if !errors.Is(err, ErrUnsupported) {
						t.Fatalf("diagnostic does not match ErrUnsupported: %v", err)
					}
					second := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(test.input)))
					secondDiagnostic := requireDiagnostic(t, second)
					if diagnostic.Error() != secondDiagnostic.Error() || !reflect.DeepEqual(diagnostic.Related(), secondDiagnostic.Related()) {
						t.Fatal("repeated fact diagnostics differ")
					}
				})
			}
			if err := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(`<ordinaryRoot xmlns="urn:root">1</ordinaryRoot>`))); err != nil {
				t.Fatalf("ValidateInstance ordinary false/zero facts: %v", err)
			}
		})
	}
}

func containsLoc(locations []Loc, want Loc) bool {
	for _, location := range locations {
		if location == want {
			return true
		}
	}
	return false
}

//nolint:gocognit // Keep generation precedence and diagnostic assertions together.
func TestGenerateGoRejectsNonDefaultGlobalElementFactsBeforePlanning(t *testing.T) {
	for _, policy := range []struct {
		name    string
		value   LanguagePolicy
		version XSDVersion
	}{
		{name: "XSD 1.0", value: Strict10, version: XSDVersion10},
		{name: "XSD 1.1", value: Strict11, version: XSDVersion11},
	} {
		for _, test := range []struct {
			name       string
			attributes string
			wantFact   string
		}{
			{name: "abstract", attributes: ` abstract="true"`, wantFact: "abstract=true"},
			{name: "nillable", attributes: ` nillable="true"`, wantFact: "nillable=true"},
			{name: "both abstract precedence", attributes: ` nillable="true" abstract="true"`, wantFact: "abstract=true"},
		} {
			t.Run(policy.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="` + string(policy.version) + `">
  <xs:element name="item" type="xs:integer"` + test.attributes + `/>
  <xs:complexType name="Unsupported"><xs:sequence><xs:element name="child" type="xs:integer"/></xs:sequence></xs:complexType>
</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.value)
				if err != nil {
					t.Fatalf("discoverSchema: %v", err)
				}
				output, err := GenerateGo(schema, "generated")
				if output != nil || err == nil {
					t.Fatalf("GenerateGo result = (%q, %v), want no output and a fact diagnostic", output, err)
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != diagnosticCodegenUnsupported || diagnostic.Feature() != FeatureCodegen {
					t.Fatalf("diagnostic = %s, want registered codegen unsupported", diagnostic)
				}
				wantSpec := schemaElementTypeXSD11SpecRef
				if policy.value == Strict10 {
					wantSpec = schemaElementTypeXSD10SpecRef
				}
				if diagnostic.SpecRef() != wantSpec {
					t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), wantSpec)
				}
				if diagnostic.Loc() != schema.Components()[0].Loc() {
					t.Fatalf("diagnostic location = %s, want declaration location %s", diagnostic.Loc(), schema.Components()[0].Loc())
				}
				if !strings.Contains(diagnostic.Message(), test.wantFact) {
					t.Fatalf("diagnostic message = %q, want %q", diagnostic.Message(), test.wantFact)
				}
				if !errors.Is(err, ErrUnsupported) || !errors.Is(err, errCodegenUnsupported) {
					t.Fatalf("diagnostic lost unsupported cause: %v", err)
				}
			})
		}
	}
}
