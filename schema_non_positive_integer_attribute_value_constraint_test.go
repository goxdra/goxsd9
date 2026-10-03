package goxsd9

import (
	"errors"
	"reflect"
	"testing"
)

//nolint:gocognit,funlen // One graph observes value, identity, order, provenance, and defensive copies.
func TestSchemaNonPositiveIntegerGlobalAttributeValueConstraints(t *testing.T) {
	const huge = "-1234567890123456789012345678901234567890"
	for _, profile := range nonPositiveIntegerPolicyProfiles() {
		t.Run(profile.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="` + string(profile.version) + `">
  <xs:include schemaLocation="shared.xsd"/>
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:attribute name="huge" type="xs:nonPositiveInteger" default="  ` + huge + `  "/>
  <xs:attribute name="zero" type="xs:nonPositiveInteger" fixed="&#x9;+0&#xA;"/>
  <xs:attribute name="forward" type="r:Forward" default="-0002"/>
  <xs:attribute name="narrowed" type="r:Narrowed" fixed=" -02 "/>
  <xs:attribute name="imported" type="o:Imported" default="-3"/>
  <xs:simpleType name="Narrowed"><xs:restriction base="xs:nonPositiveInteger"><xs:maxInclusive value="-1"/><xs:totalDigits value="1"/><xs:enumeration value="-2"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Forward"><xs:restriction base="r:Narrowed"/></xs:simpleType>
</xs:schema>`
			fixtures := map[string]discoveryFixture{
				"shared.xsd": {id: "shared.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:root"><xs:attribute name="shared" type="xs:nonPositiveInteger" fixed="-0"/></xs:schema>`},
				"other.xsd":  {id: "other.xsd", contents: `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other"><xs:simpleType name="Imported"><xs:restriction base="xs:nonPositiveInteger"><xs:minInclusive value="-3"/></xs:restriction></xs:simpleType></xs:schema>`},
			}
			schema, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil {
				t.Fatal(err)
			}
			repeated, err := discoverTestSchemaWithPolicy(t, root, fixtures, profile.policy)
			if err != nil || !reflect.DeepEqual(schema.Components(), repeated.Components()) {
				t.Fatalf("unstable graph result: %v", err)
			}
			wants := []struct {
				name, namespace, source, typeName, lexical, canonical, marker string
				kind                                                          AttributeValueConstraintKind
			}{
				{"huge", "urn:root", "root.xsd", "nonPositiveInteger", huge, huge, `default="  ` + huge + `  "`, AttributeValueConstraintDefault},
				{"zero", "urn:root", "root.xsd", "nonPositiveInteger", "+0", "0", `fixed="&#x9;+0&#xA;"`, AttributeValueConstraintFixed},
				{"forward", "urn:root", "root.xsd", "Forward", "-0002", "-2", `default="-0002"`, AttributeValueConstraintDefault},
				{"narrowed", "urn:root", "root.xsd", "Narrowed", "-02", "-2", `fixed=" -02 "`, AttributeValueConstraintFixed},
				{"imported", "urn:root", "root.xsd", "Imported", "-3", "-3", `default="-3"`, AttributeValueConstraintDefault},
				{"shared", "urn:root", "shared.xsd", "nonPositiveInteger", "-0", "0", `fixed="-0"`, AttributeValueConstraintFixed},
			}
			var attributes []Component
			for _, component := range schema.Components() {
				if component.Kind() == ComponentKindAttributeDeclaration {
					attributes = append(attributes, component)
				}
			}
			if len(attributes) != len(wants) {
				t.Fatalf("attribute order/count = %v", attributes)
			}
			for i, want := range wants {
				component := attributes[i]
				if component.Name() != mustTestQName(t, want.namespace, want.name) || component.ID().Source() != SourceID(want.source) {
					t.Fatalf("attribute %d = %v", i, component)
				}
				declaration, ok := component.AttributeDeclaration()
				if !ok {
					t.Fatalf("%s has no declaration", want.name)
				}
				constraint, ok := declaration.ValueConstraint()
				loc := schemaBuiltinReferenceAttributeLoc(t, SourceID(want.source), want.marker, root, fixtures)
				if !ok || constraint.Kind() != want.kind || constraint.IsDefault() != (want.kind == AttributeValueConstraintDefault) || constraint.IsFixed() != (want.kind == AttributeValueConstraintFixed) || constraint.Lexical() != want.lexical || constraint.Loc() != loc {
					t.Fatalf("%s constraint = %#v/%t, want lexical %q at %s", want.name, constraint, ok, want.lexical, loc)
				}
				integer, ok := constraint.IntegerValue()
				if !ok || integer.Canonical() != want.canonical {
					t.Fatalf("%s integer = %s/%t", want.name, integer.Canonical(), ok)
				}
				if _, hasDecimal := constraint.DecimalValue(); hasDecimal {
					t.Fatalf("%s exposed a decimal value", want.name)
				}
				integer.value.SetInt64(7)
				constraint.lexical = "changed"
				fresh, _ := declaration.ValueConstraint()
				freshInteger, _ := fresh.IntegerValue()
				if fresh.Lexical() != want.lexical || freshInteger.Canonical() != want.canonical {
					t.Fatalf("%s changed through value copy", want.name)
				}
				reference, _ := declaration.TypeReference()
				bounds, ok := reference.IntegerBounds()
				if !ok || bounds.Version() != profile.version {
					t.Fatalf("%s bounds = %v/%t", want.name, bounds, ok)
				}
				maximum, ok := bounds.MaxInclusive()
				if want.name == "narrowed" || want.name == "forward" {
					if !ok || maximum.Canonical() != "-1" || bounds.Bounds()[0].Loc() != elementReferenceTestAttributeLoc(t, root, `value="-1"`) {
						t.Fatalf("%s effective max = %v/%t", want.name, maximum, ok)
					}
				}
				if want.name == "huge" || want.name == "zero" || want.name == "shared" {
					if !ok || maximum.Canonical() != "0" || !bounds.Bounds()[0].Loc().IsZero() {
						t.Fatalf("%s intrinsic max = %v/%t", want.name, maximum, ok)
					}
				}
				if want.name == "imported" {
					minimum, ok := bounds.MinInclusive()
					if !ok || minimum.Canonical() != "-3" || bounds.Bounds()[0].Loc() != schemaBuiltinReferenceAttributeLoc(t, "other.xsd", `value="-3"`, root, fixtures) {
						t.Fatalf("imported minimum provenance = %v/%t", bounds.Bounds(), ok)
					}
				}
				ordered := bounds.Bounds()
				ordered[0].value.value.SetInt64(99)
				freshReference, _ := declaration.TypeReference()
				freshBounds, _ := freshReference.IntegerBounds()
				if freshBounds.Bounds()[0].Value().Canonical() == "99" {
					t.Fatalf("%s changed through bounds copy", want.name)
				}
			}
			found := schema.FindKind(ComponentKindAttributeDeclaration, mustTestQName(t, "urn:root", "narrowed"))
			declaration, _ := found[0].AttributeDeclaration()
			reference, _ := declaration.TypeReference()
			target := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:root", "Narrowed"))
			id, ok := reference.ComponentID()
			if len(target) != 1 || !ok || id != target[0].ID() || reference.Loc() != elementReferenceTestAttributeLoc(t, root, `type="r:Narrowed"`) {
				t.Fatalf("named reference = %v/%t", id, ok)
			}
			definition, _ := target[0].SimpleTypeDefinition()
			digits := definition.DigitFacets()
			if total, ok := digits.TotalDigits(); !ok || total.Canonical() != "1" {
				t.Fatalf("named totalDigits = %v/%t", total, ok)
			}
			if loc, ok := digits.TotalDigitsLoc(); !ok || loc != elementReferenceTestAttributeLoc(t, root, `value="1"`) {
				t.Fatalf("named totalDigits location = %s/%t", loc, ok)
			}
			if values := definition.IntegerEnumerationFacets().Values(); len(values) != 1 || values[0].Canonical() != "-2" {
				t.Fatalf("named enumeration = %v", values)
			}
		})
	}
}

//nolint:gocognit // Every alternate value exit keeps its classified cause and locations.
func TestSchemaNonPositiveIntegerGlobalAttributeValueDiagnostics(t *testing.T) {
	for _, profile := range nonPositiveIntegerPolicyProfiles() {
		for _, test := range []struct {
			name, typeName, declaration, value, primary, related, innerCode string
			kind                                                            AttributeValueConstraintKind
			cause                                                           error
			conflict                                                        bool
		}{
			{"positive", "xs:nonPositiveInteger", "", "+1", `default="+1"`, "", BoundValueViolationCode, AttributeValueConstraintDefault, errBoundValueViolation, false},
			{"unsigned positive", "xs:nonPositiveInteger", "", "1", `fixed="1"`, "", BoundValueViolationCode, AttributeValueConstraintFixed, errBoundValueViolation, false},
			{"malformed", "xs:nonPositiveInteger", "", "--1", `fixed="--1"`, "", InvalidIntegerLexicalCode, AttributeValueConstraintFixed, nil, false},
			{"named maximum", "r:Bounded", `<xs:simpleType name="Bounded"><xs:restriction base="xs:nonPositiveInteger"><xs:maxInclusive value="-2"/></xs:restriction></xs:simpleType>`, "-1", `fixed="-1"`, `value="-2"`, BoundValueViolationCode, AttributeValueConstraintFixed, errBoundValueViolation, false},
			{"named digits", "r:Digits", `<xs:simpleType name="Digits"><xs:restriction base="xs:nonPositiveInteger"><xs:totalDigits value="1"/></xs:restriction></xs:simpleType>`, "-12", `default="-12"`, `value="1"`, DigitFacetValueViolationCode, AttributeValueConstraintDefault, errDigitFacetValueViolation, false},
			{"named enumeration", "r:Enum", `<xs:simpleType name="Enum"><xs:restriction base="xs:nonPositiveInteger"><xs:enumeration value="-2"/></xs:restriction></xs:simpleType>`, "-3", `fixed="-3"`, `<xs:enumeration value="-2"`, EnumerationValueViolationCode, AttributeValueConstraintFixed, errEnumerationValueViolation, false},
			{"conflict", "xs:nonPositiveInteger", "", "0", `fixed="0"`, `default="-1"`, "", AttributeValueConstraintFixed, errSchemaAttributeValueConstraintConflict, true},
		} {
			t.Run(profile.name+"/"+test.name, func(t *testing.T) {
				valueText := string(test.kind) + `="` + test.value + `"`
				if test.conflict {
					valueText = `default="-1" ` + valueText
				}
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:r" targetNamespace="urn:r" version="` + string(profile.version) + `"><xs:attribute name="a" type="` + test.typeName + `" ` + valueText + `/>` + test.declaration + `</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
				if err == nil || schema.storage != nil || len(schema.Components()) != 0 || len(schema.Documents()) != 0 {
					t.Fatalf("invalid value published schema: %v", err)
				}
				diagnostic := requireDiagnostic(t, err)
				if diagnostic.Class() != FailureInvalid || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, test.primary) || test.cause != nil && !errors.Is(err, test.cause) {
					t.Fatalf("diagnostic = %v, want invalid at %s with %v", diagnostic, test.primary, test.cause)
				}
				if test.related != "" && !schemaLocationListContains(diagnostic.Related(), elementReferenceTestAttributeLoc(t, root, test.related)) {
					t.Fatalf("related = %v, want %s", diagnostic.Related(), test.related)
				}
				if test.related == "" && len(diagnostic.Related()) != 0 {
					t.Fatalf("unexpected related locations = %v", diagnostic.Related())
				}
				if test.conflict {
					if diagnostic.Code() != invalidSchemaCompositionCode || diagnostic.SpecRef() != schemaAttributeValueConstraintSpecRef(profile.version) {
						t.Fatalf("conflict = %v", diagnostic)
					}
					return
				}
				if diagnostic.Code() != diagnosticSchemaAttributeValueConstraintCode || diagnostic.SpecRef() != schemaAttributeValueSpecRef(profile.version) || !errors.Is(err, errSchemaAttributeValueConstraintInvalid) {
					t.Fatalf("value diagnostic = %v", diagnostic)
				}
				inner := requireNestedDiagnostic(t, diagnostic)
				if inner.Code() != test.innerCode || inner.Loc() != diagnostic.Loc() {
					t.Fatalf("nested diagnostic = %v", inner)
				}
			})
		}
	}
}
