package goxsd9

import (
	"errors"
	"testing"
)

//nolint:gocognit // Keep the ordered public constraint contract together.
func TestSchemaBridgeRetainsGlobalAttributeValueConstraints(t *testing.T) {
	integerLexical := "+00000000000000000000000000000000000000042"
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root">
  <xs:attribute name="defaultInteger" type="xs:integer" default=" ` + integerLexical + ` "/>
  <xs:attribute name="fixedDecimal" type="r:Decimal" fixed=" 1.2300 "/>
  <xs:attribute name="forwardInteger" type="r:Integer" default="42"/>
  <xs:attribute name="unconstrained" type="xs:integer"/>
  <xs:simpleType name="Integer"><xs:restriction base="xs:integer"><xs:minInclusive value="10"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="DecimalBase"><xs:restriction base="xs:decimal"><xs:totalDigits value="6"/><xs:fractionDigits value="2"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Decimal"><xs:restriction base="r:DecimalBase"><xs:minInclusive value="1.2"/></xs:restriction></xs:simpleType>
</xs:schema>`
	for _, policy := range []struct {
		name   string
		policy LanguagePolicy
	}{
		{name: "Strict10", policy: Strict10},
		{name: "Strict11", policy: Strict11},
	} {
		t.Run(policy.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}
			components := schema.Components()
			if len(components) != 7 {
				t.Fatalf("component count = %d, want 7", len(components))
			}

			defaultInteger := requireAttributeValueConstraint(t, components[0])
			if !defaultInteger.IsDefault() || defaultInteger.IsFixed() || defaultInteger.Kind() != AttributeValueConstraintDefault {
				t.Fatalf("default integer kind = %q, want default", defaultInteger.Kind())
			}
			if defaultInteger.Lexical() != integerLexical {
				t.Fatalf("default integer lexical = %q, want %q", defaultInteger.Lexical(), integerLexical)
			}
			if defaultInteger.Loc() != elementReferenceTestAttributeLoc(t, root, "default=") {
				t.Fatalf("default integer location = %s, want default attribute", defaultInteger.Loc())
			}
			integerValue, ok := defaultInteger.IntegerValue()
			if !ok || integerValue.Canonical() != "42" {
				t.Fatalf("default integer value = %q/%t, want 42/true", integerValue.Canonical(), ok)
			}
			if _, hasDecimal := defaultInteger.DecimalValue(); hasDecimal {
				t.Fatal("default integer unexpectedly has a decimal value")
			}

			fixedDecimal := requireAttributeValueConstraint(t, components[1])
			if !fixedDecimal.IsFixed() || fixedDecimal.IsDefault() || fixedDecimal.Kind() != AttributeValueConstraintFixed {
				t.Fatalf("fixed decimal kind = %q, want fixed", fixedDecimal.Kind())
			}
			if fixedDecimal.Lexical() != "1.2300" {
				t.Fatalf("fixed decimal lexical = %q, want 1.2300", fixedDecimal.Lexical())
			}
			if fixedDecimal.Loc() != elementReferenceTestAttributeLoc(t, root, "fixed=") {
				t.Fatalf("fixed decimal location = %s, want fixed attribute", fixedDecimal.Loc())
			}
			decimalValue, ok := fixedDecimal.DecimalValue()
			if !ok || decimalValue.Canonical() != "1.23" || decimalValue.Scale() != 2 {
				t.Fatalf("fixed decimal value = %q scale %d/%t, want 1.23 scale 2/true", decimalValue.Canonical(), decimalValue.Scale(), ok)
			}
			if _, hasInteger := fixedDecimal.IntegerValue(); hasInteger {
				t.Fatal("fixed decimal unexpectedly has an integer value")
			}

			forwardInteger := requireAttributeValueConstraint(t, components[2])
			forwardValue, ok := forwardInteger.IntegerValue()
			if !ok || forwardValue.Canonical() != "42" {
				t.Fatalf("forward integer value = %q/%t, want 42/true", forwardValue.Canonical(), ok)
			}
			unconstrained, ok := components[3].Attribute()
			if !ok {
				t.Fatal("unconstrained attribute has no attribute view")
			}
			if _, ok := unconstrained.ValueConstraint(); ok {
				t.Fatal("unconstrained attribute unexpectedly has a value constraint")
			}
		})
	}
}

func TestSchemaBridgeGlobalAttributeValueConstraintUsesImportedType(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:o="urn:other" targetNamespace="urn:root">
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:attribute name="amount" type="o:Amount" fixed="100.00"/>
</xs:schema>`
	other := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:other">
  <xs:simpleType name="Amount"><xs:restriction base="xs:decimal"><xs:totalDigits value="5"/></xs:restriction></xs:simpleType>
</xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, map[string]discoveryFixture{
		"other.xsd": {id: "other.xsd", contents: other},
	}, Strict11)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	declaration, ok := schema.Components()[0].Attribute()
	if !ok {
		t.Fatal("imported-type attribute has no attribute view")
	}
	constraint, ok := declaration.ValueConstraint()
	if !ok {
		t.Fatal("imported-type attribute has no value constraint")
	}
	value, ok := constraint.DecimalValue()
	if !ok || value.Canonical() != "100.0" {
		t.Fatalf("imported decimal value = %q/%t, want 100.0/true", value.Canonical(), ok)
	}
	if constraint.Lexical() != "100.00" {
		t.Fatalf("imported decimal lexical = %q, want 100.00", constraint.Lexical())
	}
}

func TestSchemaBridgeGlobalAttributeValueConstraintViewsAreDefensive(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="value" type="xs:integer" default="123456789012345678901234567890"/></xs:schema>`
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	declaration, ok := schema.Components()[0].Attribute()
	if !ok {
		t.Fatal("attribute has no attribute view")
	}
	first, ok := declaration.ValueConstraint()
	if !ok {
		t.Fatal("attribute has no value constraint")
	}
	firstInteger, ok := first.IntegerValue()
	if !ok {
		t.Fatal("value constraint has no integer value")
	}
	firstInteger.value.SetInt64(7)
	second, ok := declaration.ValueConstraint()
	if !ok {
		t.Fatal("attribute lost value constraint after view mutation")
	}
	secondInteger, ok := second.IntegerValue()
	if !ok || secondInteger.Canonical() != "123456789012345678901234567890" {
		t.Fatalf("stored integer after view mutation = %q/%t, want original/true", secondInteger.Canonical(), ok)
	}

	secondInteger.value.SetInt64(8)
	thirdInteger, ok := second.IntegerValue()
	if !ok || thirdInteger.Canonical() != "123456789012345678901234567890" {
		t.Fatalf("constraint integer after accessor mutation = %q/%t, want original/true", thirdInteger.Canonical(), ok)
	}
}

//nolint:gocognit,funlen // Keep value-constraint diagnostic precedence together.
func TestSchemaBridgeGlobalAttributeValueConstraintDiagnostics(t *testing.T) {
	tests := []struct {
		name        string
		root        string
		policy      LanguagePolicy
		class       FailureClass
		code        string
		specRef     string
		primary     string
		cause       error
		innerCode   string
		related     string
		unsupported bool
	}{
		{
			name:      "empty integer lexical",
			root:      `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="value" type="xs:integer" default=""/></xs:schema>`,
			policy:    Strict10,
			class:     FailureInvalid,
			code:      diagnosticSchemaAttributeValueConstraintCode,
			specRef:   schemaAttributeValueXSD10SpecRef,
			primary:   "default=",
			cause:     errSchemaAttributeValueConstraintInvalid,
			innerCode: InvalidIntegerLexicalCode,
		},
		{
			name:      "inherited bound",
			root:      `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root"><xs:attribute name="value" type="r:AtLeastTen" fixed="9"/><xs:simpleType name="AtLeastTen"><xs:restriction base="xs:integer"><xs:minInclusive value="10"/></xs:restriction></xs:simpleType></xs:schema>`,
			policy:    Strict11,
			class:     FailureInvalid,
			code:      diagnosticSchemaAttributeValueConstraintCode,
			specRef:   schemaAttributeValueXSD11SpecRef,
			primary:   "fixed=",
			cause:     errSchemaAttributeValueConstraintInvalid,
			innerCode: BoundValueViolationCode,
			related:   "value=\"10\"",
		},
		{
			name:        "unsupported string type",
			root:        `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="value" type="xs:string" default="text"/></xs:schema>`,
			policy:      Strict11,
			class:       FailureUnsupported,
			code:        UnsupportedSchemaSyntaxCode,
			specRef:     schemaAttributeTypeXSD11SpecRef,
			primary:     "type=",
			cause:       errSchemaAttributeTypeUnsupported,
			unsupported: true,
		},
		{
			name:    "unsupported token value",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="value" type="xs:token" default="text"/></xs:schema>`,
			policy:  Strict11,
			class:   FailureUnsupported,
			code:    UnsupportedSchemaSyntaxCode,
			specRef: schemaAttributeValueConstraintXSD11SpecRef,
			primary: "default=",
			cause:   errSchemaAttributeValueConstraintUnsupported,
		},
		{
			name:    "missing type",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="value" default="1"/></xs:schema>`,
			policy:  Strict11,
			class:   FailureUnsupported,
			code:    UnsupportedSchemaSyntaxCode,
			specRef: schemaAttributeValueConstraintXSD11SpecRef,
			primary: "default=",
			cause:   errSchemaAttributeValueConstraintUnsupported,
		},
		{
			name:    "default and fixed",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="value" type="xs:integer" default="1" fixed="1"/></xs:schema>`,
			policy:  Strict10,
			class:   FailureInvalid,
			code:    invalidSchemaCompositionCode,
			specRef: schemaAttributeValueConstraintXSD10SpecRef,
			primary: "fixed=",
			cause:   errSchemaAttributeValueConstraintConflict,
		},
		{
			name:    "type resolution precedes lexical conversion",
			root:    `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:m="urn:missing"><xs:attribute name="value" type="m:Missing" default="not-an-integer"/></xs:schema>`,
			policy:  Strict11,
			class:   FailureInvalid,
			code:    diagnosticSchemaAttributeTypeUnresolvedCode,
			specRef: schemaAttributeTypeXSD11SpecRef,
			primary: "type=",
			cause:   errSchemaAttributeTypeUnresolved,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, test.policy)
			if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
				t.Fatal("invalid or unsupported value constraint returned a schema")
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.SpecRef() != test.specRef {
				t.Fatalf("diagnostic = %s/%q/%q, want %s/%s/%s", diagnostic, diagnostic.Class(), diagnostic.SpecRef(), test.class, test.code, test.specRef)
			}
			if diagnostic.Loc() != elementReferenceTestAttributeLoc(t, test.root, test.primary) {
				t.Fatalf("diagnostic location = %s, want %s", diagnostic.Loc(), elementReferenceTestAttributeLoc(t, test.root, test.primary))
			}
			if test.cause != nil && !errors.Is(err, test.cause) {
				t.Fatalf("diagnostic lost cause %v: %v", test.cause, err)
			}
			if test.unsupported && (!errors.Is(err, ErrUnsupported) || !errors.Is(err, errSchemaAttributeTypeUnsupported)) {
				t.Fatalf("unsupported diagnostic lost unsupported causes: %v", err)
			}
			if test.innerCode != "" {
				inner := requireNestedDiagnostic(t, diagnostic)
				if inner.Code() != test.innerCode || inner.Loc() != diagnostic.Loc() {
					t.Fatalf("nested diagnostic = %s, want %s at %s", inner, test.innerCode, diagnostic.Loc())
				}
			}
			if test.related != "" && !schemaLocationListContains(diagnostic.Related(), elementReferenceTestAttributeLoc(t, test.root, test.related)) {
				t.Fatalf("diagnostic related locations = %v, want %s", diagnostic.Related(), test.related)
			}
		})
	}
}

func TestSchemaBridgeGlobalAttributeDecimalConstraintHonorsVersionPolicy(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `"><xs:attribute name="value" type="xs:decimal" default=".5"/></xs:schema>`
	strict10, err := discoverTestSchemaWithPolicy(t, root, nil, Strict10)
	if err == nil || strict10.storage != nil {
		t.Fatal("Strict10 accepted XSD 1.1 decimal lexical form or returned a schema")
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Code() != diagnosticSchemaAttributeValueConstraintCode || diagnostic.SpecRef() != schemaAttributeValueXSD10SpecRef || diagnostic.Loc() != elementReferenceTestAttributeLoc(t, root, "default=") {
		t.Fatalf("Strict10 diagnostic = %s/%q, want value-constraint diagnostic at default", diagnostic, diagnostic.SpecRef())
	}

	strict11, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("Strict11 rejected decimal lexical form: %v", err)
	}
	constraint := requireAttributeValueConstraint(t, strict11.Components()[0])
	value, ok := constraint.DecimalValue()
	if !ok || value.Canonical() != "0.5" || constraint.Lexical() != ".5" {
		t.Fatalf("Strict11 decimal constraint = %q/%q/%t, want 0.5/.5/true", value.Canonical(), constraint.Lexical(), ok)
	}
}

func requireAttributeValueConstraint(t *testing.T, component Component) AttributeValueConstraint {
	t.Helper()
	declaration, ok := component.Attribute()
	if !ok {
		t.Fatalf("component %q has no attribute view", component.Name())
	}
	constraint, ok := declaration.ValueConstraint()
	if !ok {
		t.Fatalf("attribute %q has no value constraint", declaration.Name())
	}
	return constraint
}

func requireNestedDiagnostic(t *testing.T, diagnostic Diagnostic) Diagnostic {
	t.Helper()
	var nested Diagnostic
	if !errors.As(diagnostic.Unwrap(), &nested) {
		t.Fatalf("diagnostic %s has no nested diagnostic", diagnostic)
	}
	return nested
}
