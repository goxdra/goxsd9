package goxsd9

import (
	"errors"
	"reflect"
	"testing"
)

// TestSchemaShortGlobalAttributeValueConstraintUnlock is also the base/head
// regression fragment for the public attribute-value contract.
//
//nolint:gocognit // Check the complete direct and named public value contract.
func TestSchemaShortGlobalAttributeValueConstraintUnlock(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r">
  <xs:attribute name="directDefault" type="xs:short" default=" +0007 "/>
  <xs:attribute name="directFixed" type="xs:short" fixed="-32768"/>
  <xs:attribute name="upper" type="xs:short" default="32767"/>
  <xs:attribute name="namedFixed" type="r:Small" fixed="+0007"/>
  <xs:simpleType name="Small"><xs:restriction base="xs:short"><xs:minInclusive value="0"/><xs:maxInclusive value="10"/><xs:enumeration value="7"/><xs:totalDigits value="1"/></xs:restriction></xs:simpleType>
</xs:schema>`
	for _, profile := range shortAttributePolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("parse short constraints: %v", err)
			}
			for _, want := range []struct {
				name, typeName, lexical, integer, marker string
				line                                     int
				kind                                     AttributeValueConstraintKind
				isNamed                                  bool
			}{
				{"directDefault", "short", "+0007", "7", `default=" +0007 "`, 2, AttributeValueConstraintDefault, false},
				{"directFixed", "short", "-32768", "-32768", `fixed="-32768"`, 3, AttributeValueConstraintFixed, false},
				{"upper", "short", "32767", "32767", `default="32767"`, 4, AttributeValueConstraintDefault, false},
				{"namedFixed", "Small", "+0007", "7", `fixed="+0007"`, 5, AttributeValueConstraintFixed, true},
			} {
				components := schema.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:r", want.name))
				if len(components) != 1 {
					t.Fatalf("%s components = %d, want one", want.name, len(components))
				}
				declaration, ok := components[0].AttributeDeclaration()
				if !ok {
					t.Fatalf("%s has no attribute declaration", want.name)
				}
				reference, ok := declaration.TypeReference()
				if !ok || reference.Loc() != mustSchemaTokenLoc(t, "root.xsd", root, want.line, `type="`+map[bool]string{true: "r:Small", false: "xs:short"}[want.isNamed]+`"`) {
					t.Fatalf("%s type reference = %v/%t", want.name, reference, ok)
				}
				bounds, ok := reference.IntegerBounds()
				if !ok || len(bounds.Bounds()) != 2 {
					t.Fatalf("%s bounds = %v/%t", want.name, bounds, ok)
				}
				if want.isNamed {
					id, present := reference.ComponentID()
					matches := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:r", "Small"))
					if !reference.IsNamed() || !present || len(matches) != 1 || id != matches[0].ID() || declaration.DeclaredType() != mustTestQName(t, "urn:r", "Small") {
						t.Fatalf("named short identity = %v/%t", id, present)
					}
					got := bounds.Bounds()
					if got[0].Value().Canonical() != "0" || got[1].Value().Canonical() != "10" || got[0].Loc() != elementReferenceTestAttributeLoc(t, root, `value="0"`) || got[1].Loc() != elementReferenceTestAttributeLoc(t, root, `value="10"`) {
						t.Fatalf("named short bounds/provenance = %v", got)
					}
				}
				if !want.isNamed {
					id, present := reference.ComponentID()
					got := bounds.Bounds()
					if !reference.IsBuiltin() || present || !id.IsZero() || declaration.DeclaredType() != mustTestQName(t, testXSDNamespace, "short") || got[0].Value().Canonical() != "-32768" || got[1].Value().Canonical() != "32767" || got[0].Loc() != (Loc{}) || got[1].Loc() != (Loc{}) {
						t.Fatalf("built-in short identity/bounds = %v/%t/%v", id, present, got)
					}
				}
				constraint, ok := declaration.ValueConstraint()
				if !ok || constraint.Kind() != want.kind || constraint.IsDefault() != (want.kind == AttributeValueConstraintDefault) || constraint.IsFixed() != (want.kind == AttributeValueConstraintFixed) || constraint.Lexical() != want.lexical || constraint.Loc() != elementReferenceTestAttributeLoc(t, root, want.marker) {
					t.Fatalf("%s constraint = %v/%t", want.name, constraint, ok)
				}
				integer, ok := constraint.IntegerValue()
				if !ok || integer.Canonical() != want.integer {
					t.Fatalf("%s exact integer = %s/%t", want.name, integer.Canonical(), ok)
				}
				if _, hasDecimal := constraint.DecimalValue(); hasDecimal {
					t.Fatalf("%s unexpectedly exposes decimal", want.name)
				}
				integer.value.SetInt64(123)
				again, ok := declaration.ValueConstraint()
				stored, hasInteger := again.IntegerValue()
				if !ok || !hasInteger || stored.Canonical() != want.integer {
					t.Fatalf("%s changed through copied integer", want.name)
				}
			}
		})
	}
}

//nolint:gocognit // Check repeated graph discovery and both cross-document references.
func TestSchemaShortAttributeConstraintGraphVisibility(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" xmlns:o="urn:o" targetNamespace="urn:r" version="1.0">
  <xs:include schemaLocation="shared.xsd"/><xs:include schemaLocation="shared.xsd"/>
  <xs:import namespace="urn:o" schemaLocation="other.xsd"/>
  <xs:attribute name="included" type="r:Included" default="2"/>
  <xs:attribute name="imported" type="o:Imported" fixed="3"/>
</xs:schema>`
	shared := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" version="1.1"><xs:simpleType name="Included"><xs:restriction base="xs:short"><xs:maxInclusive value="2"/></xs:restriction></xs:simpleType></xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:o" version="1.1"><xs:import namespace="urn:r" schemaLocation="root.xsd"/><xs:simpleType name="Imported"><xs:restriction base="xs:short"><xs:enumeration value="3"/></xs:restriction></xs:simpleType></xs:schema>`
	fixtures := map[string]discoveryFixture{
		"shared.xsd": {id: "shared.xsd", contents: shared},
		"other.xsd":  {id: "other.xsd", contents: other},
		"root.xsd":   {id: "root.xsd", contents: root},
	}
	for _, profile := range shortAttributePolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			first, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			second, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			if len(first.Documents()) != 3 || !reflect.DeepEqual(first.Components(), second.Components()) {
				t.Fatal("repeated include/import changed discovery or component order")
			}
			for _, want := range []struct {
				name, source, lexical, marker string
				kind                          AttributeValueConstraintKind
			}{
				{"included", "shared.xsd", "2", `default="2"`, AttributeValueConstraintDefault},
				{"imported", "other.xsd", "3", `fixed="3"`, AttributeValueConstraintFixed},
			} {
				components := first.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:r", want.name))
				if len(components) != 1 {
					t.Fatalf("%s components = %d", want.name, len(components))
				}
				declaration, _ := components[0].AttributeDeclaration()
				reference, _ := declaration.TypeReference()
				id, ok := declaration.TypeID()
				constraint, hasConstraint := declaration.ValueConstraint()
				value, hasInteger := constraint.IntegerValue()
				if !ok || !reference.IsNamed() || id.Source() != SourceID(want.source) || !hasConstraint || !hasInteger || value.Canonical() != want.lexical || constraint.Kind() != want.kind || constraint.Loc() != elementReferenceTestAttributeLoc(t, root, want.marker) {
					t.Fatalf("%s cross-document facts = %v/%v/%v", want.name, id, reference, constraint)
				}
			}
		})
	}
}

//nolint:gocognit // Check all value conversion exits and their nested causes across policies.
func TestSchemaShortAttributeValueConstraintDiagnostics(t *testing.T) {
	for _, profile := range shortAttributePolicyProfiles() {
		for _, test := range []struct {
			name, typeName, value, declaration, primary, related, innerCode string
			kind                                                            AttributeValueConstraintKind
			cause                                                           error
			conflict                                                        bool
		}{
			{name: "malformed", typeName: "xs:short", value: "7.0", kind: AttributeValueConstraintDefault, innerCode: InvalidIntegerLexicalCode},
			{name: "below intrinsic", typeName: "xs:short", value: "-32769", kind: AttributeValueConstraintFixed, innerCode: BoundValueViolationCode, cause: errBoundValueViolation},
			{name: "above intrinsic", typeName: "xs:short", value: "32768", kind: AttributeValueConstraintDefault, innerCode: BoundValueViolationCode, cause: errBoundValueViolation},
			{name: "named lower", typeName: "r:Narrowed", value: "-1", kind: AttributeValueConstraintFixed, declaration: `<xs:simpleType name="Narrowed"><xs:restriction base="xs:short"><xs:minInclusive value="0"/></xs:restriction></xs:simpleType>`, innerCode: BoundValueViolationCode, cause: errBoundValueViolation, related: `value="0"`},
			{name: "named enumeration", typeName: "r:Enumerated", value: "8", kind: AttributeValueConstraintDefault, declaration: `<xs:simpleType name="Enumerated"><xs:restriction base="xs:short"><xs:enumeration value="7"/></xs:restriction></xs:simpleType>`, innerCode: EnumerationValueViolationCode, cause: errEnumerationValueViolation, related: `<xs:enumeration value="7"`},
			{name: "named digits", typeName: "r:Digits", value: "12", kind: AttributeValueConstraintFixed, declaration: `<xs:simpleType name="Digits"><xs:restriction base="xs:short"><xs:totalDigits value="1"/></xs:restriction></xs:simpleType>`, innerCode: DigitFacetValueViolationCode, cause: errDigitFacetValueViolation, related: `value="1"`},
			{name: "conflicting values", typeName: "xs:short", value: "0", kind: AttributeValueConstraintFixed, conflict: true, primary: `fixed="0"`, related: `default="0"`, cause: errSchemaAttributeValueConstraintConflict},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				version := profile.version
				if profile.policy == Compatibility {
					version = XSDVersion11
				}
				valueText := string(test.kind) + `="` + test.value + `"`
				if test.conflict {
					valueText = `default="0" ` + valueText
				}
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r" version="` + string(profile.version) + `"><xs:attribute name="value" type="` + test.typeName + `" ` + valueText + `/>` + test.declaration + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
					t.Fatal("invalid short value returned a schema")
				}
				diagnostic := requireDiagnostic(t, err)
				primary := valueText
				if test.conflict {
					primary = test.primary
				}
				if diagnostic.Class() != FailureInvalid || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, primary) || !errors.Is(err, test.cause) && test.cause != nil {
					t.Fatalf("diagnostic = %s, want invalid at %s with %v", diagnostic, primary, test.cause)
				}
				if test.conflict {
					if diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.SpecRef() != schemaAttributeValueConstraintSpecRef(version) || !reflect.DeepEqual(diagnostic.Related(), []Loc{elementReferenceTestAttributeLoc(t, root, test.related)}) {
						t.Fatalf("conflict diagnostic = %s/%v", diagnostic, diagnostic.Related())
					}
					return
				}
				if diagnostic.Code() != diagnosticSchemaAttributeValueConstraintCode || diagnostic.SpecRef() != schemaAttributeValueSpecRef(version) || !errors.Is(err, errSchemaAttributeValueConstraintInvalid) {
					t.Fatalf("value diagnostic = %s", diagnostic)
				}
				inner := requireNestedDiagnostic(t, diagnostic)
				if inner.Code() != test.innerCode || inner.Loc() != diagnostic.Loc() {
					t.Fatalf("nested diagnostic = %s", inner)
				}
				if test.related == "" && len(diagnostic.Related()) != 0 {
					t.Fatalf("unexpected related locations = %v", diagnostic.Related())
				}
				if test.related != "" && !schemaLocationListContains(diagnostic.Related(), elementReferenceTestAttributeLoc(t, root, test.related)) {
					t.Fatalf("related locations = %v, want %s", diagnostic.Related(), test.related)
				}
			})
		}
	}
}

//nolint:gocognit // Keep excluded attribute shapes under the same policy matrix.
func TestSchemaShortAttributeConstraintExcludedShapes(t *testing.T) {
	for _, profile := range shortAttributePolicyProfiles() {
		for _, test := range []struct {
			name, body, marker string
			cause              error
		}{
			{"local direct", `<xs:complexType name="C"><xs:attribute name="a" type="xs:short" default="1"/></xs:complexType>`, `default="1"`, errSchemaAttributeUseUnsupported},
			{"local named", `<xs:complexType name="C"><xs:attribute name="a" type="r:Short" fixed="1"/></xs:complexType><xs:simpleType name="Short"><xs:restriction base="xs:short"/></xs:simpleType>`, `fixed="1"`, errSchemaAttributeUseUnsupported},
			{"global inline", `<xs:attribute name="a" default="1"><xs:simpleType><xs:restriction base="xs:short"/></xs:simpleType></xs:attribute>`, `<xs:simpleType>`, ErrUnsupported},
			{"local inline", `<xs:complexType name="C"><xs:attribute name="a" fixed="1"><xs:simpleType><xs:restriction base="xs:short"/></xs:simpleType></xs:attribute></xs:complexType>`, `fixed="1"`, errSchemaAttributeUseUnsupported},
			{"local ref", `<xs:attribute name="a" type="xs:short" default="1"/><xs:complexType name="C"><xs:attribute ref="r:a"/></xs:complexType>`, `ref="r:a"`, errSchemaAttributeReferenceUnsupported},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r" version="` + string(profile.version) + `">` + test.body + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
					t.Fatal("excluded short attribute returned a schema")
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != UnsupportedSchemaSyntaxCode || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, test.marker) || diagnostic.SpecRef() == "" || !errors.Is(err, ErrUnsupported) || !errors.Is(err, test.cause) {
					t.Fatalf("excluded shape diagnostic = %s, want unsupported at %s with %v", diagnostic, test.marker, test.cause)
				}
				if test.name == "local ref" && !reflect.DeepEqual(diagnostic.Related(), []Loc{elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="a"`)}) {
					t.Fatalf("local ref related = %v", diagnostic.Related())
				}
			})
		}
	}
}

func TestSchemaShortNamedConstrainedAttributeGenerationExcluded(t *testing.T) {
	for _, profile := range shortAttributePolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r"><xs:attribute name="a" type="r:Short" fixed="1"/><xs:simpleType name="Short"><xs:restriction base="xs:short"/></xs:simpleType></xs:schema>`
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			output, err := GenerateGo(schema, "generated")
			if err == nil || output != nil {
				t.Fatal("named short attribute generated output")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != diagnosticCodegenUnsupported || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, `<xs:attribute name="a"`) || !errors.Is(err, errCodegenUnsupported) || !errors.Is(err, ErrUnsupported) {
				t.Fatalf("named short generation diagnostic = %s", diagnostic)
			}
		})
	}
}

func TestSchemaShortConstraintKeepsTypeBoundaryPrecedence(t *testing.T) {
	for _, profile := range shortAttributePolicyProfiles() {
		for _, test := range []struct {
			name, typeName, declaration, marker, code string
			cause                                     error
			class                                     FailureClass
		}{
			{"unresolved type", "r:Missing", "", `type="r:Missing"`, diagnosticSchemaAttributeTypeUnresolvedCode, errSchemaAttributeTypeUnresolved, FailureInvalid},
			{"unsupported named facet", "r:Pattern", `<xs:simpleType name="Pattern"><xs:restriction base="xs:short"><xs:pattern value="[0-9]+"/></xs:restriction></xs:simpleType>`, `<xs:pattern`, UnsupportedDatatypeFacetCode, ErrUnsupported, FailureUnsupported},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r"><xs:attribute name="a" type="` + test.typeName + `" default="not-an-integer"/>` + test.declaration + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
					t.Fatal("type failure returned a schema")
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, test.marker) || !errors.Is(err, test.cause) {
					t.Fatalf("type boundary diagnostic = %s", diagnostic)
				}
			})
		}
	}
}
