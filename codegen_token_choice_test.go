package goxsd9

import (
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"strings"
	"testing"
)

//nolint:gocognit,funlen // Keep the token graph and its observable output together.
func TestGenerateGoDirectTokenChoiceGraph(t *testing.T) {
	rootTemplate := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:token-root" xmlns:o="urn:token-other" targetNamespace="urn:token-root"%s>
  <xs:include schemaLocation="chameleon.xsd"/>
  <xs:import namespace="urn:token-other" schemaLocation="other.xsd"/>
  <xs:element name="choice-root" type="r:Choice"/>
  <xs:complexType name="Choice"><xs:choice>
    <xs:element name="line-item" type="xs:token"/>
    <xs:element name="LINE_ITEM" type="r:DerivedToken"/>
    <xs:element name="forward" type="r:ForwardToken"/>
    <xs:element name="included" type="r:IncludedToken"/>
    <xs:element name="imported" type="o:ImportedToken"/>
  </xs:choice></xs:complexType>
  <xs:simpleType name="DerivedToken"><xs:restriction base="r:BaseToken"/></xs:simpleType>
  <xs:simpleType name="BaseToken"><xs:restriction base="xs:token"/></xs:simpleType>
  <xs:simpleType name="ForwardToken"><xs:restriction base="r:ForwardBase"/></xs:simpleType>
  <xs:simpleType name="ForwardBase"><xs:restriction base="xs:token"/></xs:simpleType>
</xs:schema>`
	fixtures := map[string]discoveryFixture{
		"chameleon.xsd": {id: "chameleon.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:simpleType name="IncludedToken"><xs:restriction base="xs:token"/></xs:simpleType></xs:schema>`},
		"other.xsd":     {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:token-other"><xs:simpleType name="ImportedToken"><xs:restriction base="xs:token"/></xs:simpleType></xs:schema>`},
	}
	profiles := []struct {
		name, version string
		policy        LanguagePolicy
	}{
		{"Compatibility", ` version="1.0"`, Compatibility},
		{"Strict10", ` version="1.0"`, Strict10},
		{"Strict11", ` version="1.1"`, Strict11},
	}
	var baseline []byte
	for _, profile := range profiles {
		t.Run(profile.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, fmt.Sprintf(rootTemplate, profile.version), fixtures, profile.policy)
			if err != nil {
				t.Fatalf("parse token graph: %v", err)
			}
			plan, err := planCodegenDirectChoices(schema, "generated")
			if err != nil {
				t.Fatalf("plan token choices: %v", err)
			}
			if len(plan.owners) != 1 || len(plan.owners[0].alternatives) != 5 {
				t.Fatalf("choice plan = %#v, want five ordered alternatives", plan.owners)
			}
			for index, alternative := range plan.owners[0].alternatives {
				family, ok := codegenDirectChoiceTargetFamily(alternative.target)
				if !ok || family != codegenDirectChoiceScalarToken || !equalCodegenPath(alternative.path, []uint32{1, uint32(index + 1)}) || alternative.loc.IsZero() {
					t.Fatalf("alternative %d = %#v, want located token at lexical path", index, alternative)
				}
			}
			first, err := GenerateGo(schema, "generated")
			if err != nil {
				t.Fatalf("GenerateGo: %v", err)
			}
			second, err := GenerateGo(schema, "generated")
			if err != nil || !bytes.Equal(first, second) {
				t.Fatalf("repeated generation = %v, equal %t", err, bytes.Equal(first, second))
			}
			if baseline != nil && !bytes.Equal(first, baseline) {
				t.Fatalf("policy changed token source:\n%s", first)
			}
			baseline = append([]byte(nil), first...)
			formatted, err := format.Source(first)
			if err != nil || !bytes.Equal(first, formatted) {
				t.Fatalf("generated source is not formatted: %v\n%s", err, first)
			}
			source := string(first)
			if strings.Contains(source, "import ") {
				t.Fatalf("token-only output imported runtime:\n%s", first)
			}
			for _, fragment := range []string{
				"type Choice interface {\n\tisChoice()\n}",
				"type ChoiceRoot struct {\n\tValue Choice\n}",
				"type LineItem struct {\n\tLineItem string\n}",
				"type LineItem2 struct {\n\tLineItem2 DerivedToken\n}",
				"type Forward struct {\n\tForward ForwardToken\n}",
				"type Included struct {\n\tIncluded IncludedToken\n}",
				"type Imported struct {\n\tImported ImportedToken\n}",
			} {
				if !strings.Contains(source, fragment) {
					t.Fatalf("generated token source misses %q:\n%s", fragment, first)
				}
			}
			for _, name := range []string{"Choice", "ChoiceRoot", "LineItem", "LineItem2", "Forward", "Included", "Imported", "DerivedToken", "BaseToken", "ForwardToken", "ForwardBase", "IncludedToken", "ImportedToken"} {
				if strings.Count(source, "type "+name+" ") != 1 {
					t.Fatalf("declaration %s occurs %d times", name, strings.Count(source, "type "+name+" "))
				}
			}
			compileGeneratedCode(t, first)
			consumer := []byte(`
func useTokenChoice(value Choice) {
	switch value := value.(type) {
	case LineItem:
		var _ string = value.LineItem
	case LineItem2:
		var _ DerivedToken = value.LineItem2
	case Forward:
		var _ ForwardToken = value.Forward
	case Included:
		var _ IncludedToken = value.Included
	case Imported:
		var _ ImportedToken = value.Imported
	}
}
var _ Choice = LineItem{}
var _ ChoiceRoot = ChoiceRoot{Value: LineItem{}}
`)
			compileGeneratedCode(t, append(append([]byte(nil), first...), consumer...))
		})
	}
}

func TestGenerateGoDirectTokenChoiceRejectsMixedFamilies(t *testing.T) {
	for _, other := range []string{"boolean", "integer", "decimal", "NMTOKEN"} {
		t.Run(other, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="Choice"><xs:choice><xs:element name="token" type="xs:token"/><xs:element name="other" type="xs:` + other + `"/></xs:choice></xs:complexType></xs:schema>`
			schema, err := discoverTestSchema(t, root, nil)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			output, err := GenerateGo(schema, "generated")
			if output != nil || err == nil {
				t.Fatalf("mixed result = (%q, %v), want nil/error", output, err)
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != diagnosticCodegenUnsupported || diagnostic.Loc().IsZero() || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("mixed diagnostic = %v", err)
			}
		})
	}
}

//nolint:gocognit // Keep the policy and exclusion matrix at one consumer boundary.
func TestGenerateGoDirectTokenChoiceExclusionsStayLocated(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		line        int
		column      int
		particleRef bool
	}{
		{"NMTOKEN", `<xs:element name="first" type="xs:NMTOKEN"/>`, 4, 5, false},
		{"named NMTOKEN", `<xs:element name="first" type="r:NamedNMTOKEN"/>`, 4, 5, false},
		{"mixed token and Boolean", `<xs:element name="first" type="xs:token"/>
    <xs:element name="second" type="xs:boolean"/>`, 5, 5, false},
		{"mixed Boolean and token", `<xs:element name="first" type="xs:boolean"/>
    <xs:element name="second" type="xs:token"/>`, 5, 5, false},
		{"mixed token and integer", `<xs:element name="first" type="xs:token"/>
    <xs:element name="second" type="xs:integer"/>`, 5, 5, false},
		{"mixed token and decimal", `<xs:element name="first" type="xs:token"/>
    <xs:element name="second" type="xs:decimal"/>`, 5, 5, false},
		{"optional child", `<xs:element name="first" type="xs:token" minOccurs="0"/>`, 4, 5, true},
		{"repeated child", `<xs:element name="first" type="xs:token" maxOccurs="2"/>`, 4, 5, true},
		{"element reference", `<xs:element ref="r:global"/>`, 4, 17, false},
		{"named element reference", `<xs:element ref="r:globalNamed"/>`, 4, 17, false},
	}
	for _, policy := range []LanguagePolicy{Strict10, Strict11} {
		for _, test := range tests {
			t.Run(string(policy)+"/"+test.name, func(t *testing.T) {
				version := "1.0"
				wantSpec := codegenDirectChoiceXSD10ElementChoiceSpecRef
				if policy == Strict11 {
					version = "1.1"
					wantSpec = codegenDirectChoiceXSD11ElementChoiceSpecRef
				}
				if test.particleRef {
					wantSpec = codegenDirectChoiceXSD10ParticleDetailsSpecRef
					if policy == Strict11 {
						wantSpec = codegenDirectChoiceXSD11ParticleDetailsSpecRef
					}
				}
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:token" targetNamespace="urn:token" version="` + version + `">
  <xs:complexType name="Choice">
  <xs:choice>
    ` + test.body + `
  </xs:choice></xs:complexType>
  <xs:element name="global" type="xs:token"/>
  <xs:element name="globalNamed" type="r:NamedToken"/>
  <xs:simpleType name="NamedToken"><xs:restriction base="xs:token"/></xs:simpleType>
  <xs:simpleType name="NamedNMTOKEN"><xs:restriction base="xs:NMTOKEN"/></xs:simpleType>
</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
				if err != nil {
					t.Fatalf("parse excluded shape: %v", err)
				}
				output, err := GenerateGo(schema, "generated")
				if output != nil || err == nil {
					t.Fatalf("excluded shape = (%q, %v), want nil/error", output, err)
				}
				diagnostic := requireDiagnostic(t, err)
				wantLoc := mustTestLoc(t, "root.xsd", test.line, test.column)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != diagnosticCodegenUnsupported || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != wantSpec || !errors.Is(err, ErrUnsupported) {
					t.Fatalf("diagnostic = %s (spec %s), want unsupported %s at %s", diagnostic, diagnostic.SpecRef(), wantSpec, wantLoc)
				}
			})
		}
	}
}

func TestDirectTokenChoiceSchemaExclusionsStayLocated(t *testing.T) {
	for _, test := range []struct {
		name, body string
		column     int
	}{
		{"inline token", `<xs:element name="first"><xs:simpleType><xs:restriction base="xs:token"/></xs:simpleType></xs:element>`, 30},
		{"nested sequence", `<xs:sequence><xs:element name="first" type="xs:token"/></xs:sequence>`, 5},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:complexType name="Choice"><xs:choice>
    ` + test.body + `
  </xs:choice></xs:complexType></xs:schema>`
			schema, err := discoverTestSchema(t, root, nil)
			if err == nil || len(schema.Components()) != 0 {
				t.Fatalf("excluded parse = (%#v, %v), want no schema and error", schema, err)
			}
			diagnostic := requireDiagnostic(t, err)
			wantLoc := mustTestLoc(t, "root.xsd", 2, test.column)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != "XSD3003" || diagnostic.Loc() != wantLoc {
				t.Fatalf("diagnostic = %s, want XSD3003 at %s", diagnostic, wantLoc)
			}
		})
	}
}

func TestGenerateGoDirectTokenChoiceRejectsNonDefaultOuterRange(t *testing.T) {
	for _, policy := range []LanguagePolicy{Strict10, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			version := "1.0"
			wantSpec := codegenDirectChoiceXSD10ParticleDetailsSpecRef
			if policy == Strict11 {
				version = "1.1"
				wantSpec = codegenDirectChoiceXSD11ParticleDetailsSpecRef
			}
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" version="` + version + `">
  <xs:complexType name="Choice">
    <xs:choice minOccurs="0"><xs:element name="value" type="xs:token"/></xs:choice>
  </xs:complexType>
</xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			output, err := GenerateGo(schema, "generated")
			if output != nil || err == nil {
				t.Fatalf("nondefault choice = (%q, %v), want nil/error", output, err)
			}
			diagnostic := requireDiagnostic(t, err)
			wantLoc := mustTestLoc(t, "root.xsd", 3, 5)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != diagnosticCodegenUnsupported || diagnostic.Loc() != wantLoc || diagnostic.SpecRef() != wantSpec || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("diagnostic = %s (spec %s), want unsupported %s at %s", diagnostic, diagnostic.SpecRef(), wantSpec, wantLoc)
			}
		})
	}
}

//nolint:gocognit // Keep token-family plan corruption cases together.
func TestGenerateGoDirectTokenChoiceRejectsCorruptPlans(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:token" targetNamespace="urn:token">
  <xs:complexType name="Choice"><xs:choice>
    <xs:element name="builtin" type="xs:token"/>
    <xs:element name="named" type="r:Named"/>
  </xs:choice></xs:complexType>
  <xs:simpleType name="Named"><xs:restriction base="xs:token"/></xs:simpleType>
</xs:schema>`
	tests := []struct {
		name   string
		index  int
		mutate func(*testing.T, *codegenDirectChoicePlan)
	}{
		{"built-in family", 0, func(t *testing.T, plan *codegenDirectChoicePlan) {
			target, ok := plan.owners[0].alternatives[0].target.(codegenDirectChoiceBuiltinTarget)
			if !ok {
				t.Fatalf("target = %T, want built-in", plan.owners[0].alternatives[0].target)
			}
			target.family = codegenDirectChoiceScalarBoolean
			plan.owners[0].alternatives[0].target = target
		}},
		{"built-in digit kind", 0, func(t *testing.T, plan *codegenDirectChoicePlan) {
			target, ok := plan.owners[0].alternatives[0].target.(codegenDirectChoiceBuiltinTarget)
			if !ok {
				t.Fatalf("target = %T, want built-in", plan.owners[0].alternatives[0].target)
			}
			target.kind = DigitDatatypeInteger
			plan.owners[0].alternatives[0].target = target
		}},
		{"named family", 1, func(t *testing.T, plan *codegenDirectChoicePlan) {
			target, ok := plan.owners[0].alternatives[1].target.(codegenDirectChoiceNamedTarget)
			if !ok {
				t.Fatalf("target = %T, want named", plan.owners[0].alternatives[1].target)
			}
			target.family = codegenDirectChoiceScalarInteger
			target.kind = DigitDatatypeInteger
			plan.owners[0].alternatives[1].target = target
		}},
		{"named identity", 1, func(t *testing.T, plan *codegenDirectChoicePlan) {
			target, ok := plan.owners[0].alternatives[1].target.(codegenDirectChoiceNamedTarget)
			if !ok {
				t.Fatalf("target = %T, want named", plan.owners[0].alternatives[1].target)
			}
			target.id = ComponentID{source: "missing.xsd", ordinal: 1}
			plan.owners[0].alternatives[1].target = target
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchema(t, root, nil)
			if err != nil {
				t.Fatalf("parse token choice: %v", err)
			}
			plan, err := planCodegenDirectChoices(schema, "generated")
			if err != nil {
				t.Fatalf("plan token choice: %v", err)
			}
			wantLoc := plan.owners[0].alternatives[test.index].loc
			test.mutate(t, &plan)
			output, err := emitCodegenSourceWithDirectChoices(schema, plan)
			if output != nil || err == nil {
				t.Fatalf("corrupt plan = (%q, %v), want nil/error", output, err)
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInternal || diagnostic.Code() != diagnosticCodegenInvariant || diagnostic.Loc() != wantLoc || !errors.Is(err, errCodegenDirectChoicePlan) {
				t.Fatalf("corruption diagnostic = %v, want internal at %s with plan cause", err, wantLoc)
			}
		})
	}
}
