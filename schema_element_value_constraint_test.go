package goxsd9

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func globalElementValueConstraintSchema(body string) string {
	return "<xs:schema xmlns:xs='" + testXSDNamespace + "' xmlns:r='urn:root' targetNamespace='urn:root'>" + body + "</xs:schema>"
}

//nolint:gocognit // Keep the public element constraint matrix together.
func TestSchemaBridgeRetainsGlobalElementValueConstraints(t *testing.T) {
	root := globalElementValueConstraintSchema(
		"<xs:element name='boolean' type='xs:boolean' default=' 1 '/>" +
			"<xs:element name='integer' type='xs:integer' default=' +00000042 '/>" +
			"<xs:element name='decimal' type='xs:decimal' fixed=' 1.2300 '/>" +
			"<xs:element name='token' type='xs:token' fixed='&#xA; first &#x9;'/>" +
			"<xs:element name='nmtoken' type='xs:NMTOKEN' fixed=' token '/>" +
			"<xs:element name='string' type='xs:string' fixed=' x '/>" +
			"<xs:element name='inline' default=' 7 '><xs:simpleType><xs:restriction base='xs:integer'><xs:minInclusive value='5'/></xs:restriction></xs:simpleType></xs:element>" +
			"<xs:element name='named' type='r:Named' fixed=' second '/>" +
			"<xs:simpleType name='Named'><xs:restriction base='xs:token'><xs:enumeration value='second'/></xs:restriction></xs:simpleType>",
	)
	for _, profile := range []struct {
		name   string
		policy LanguagePolicy
	}{
		{name: "Strict10", policy: Strict10},
		{name: "Strict11", policy: Strict11},
	} {
		t.Run(profile.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, profile.policy)
			if err != nil {
				t.Fatalf("discoverSchema: %v", err)
			}
			tests := []struct {
				name    string
				kind    ValueConstraintKind
				lexical string
			}{
				{name: "boolean", kind: ValueConstraintDefault, lexical: "1"},
				{name: "integer", kind: ValueConstraintDefault, lexical: "+00000042"},
				{name: "decimal", kind: ValueConstraintFixed, lexical: "1.2300"},
				{name: "token", kind: ValueConstraintFixed, lexical: "first"},
				{name: "nmtoken", kind: ValueConstraintFixed, lexical: "token"},
				{name: "string", kind: ValueConstraintFixed, lexical: " x "},
				{name: "inline", kind: ValueConstraintDefault, lexical: "7"},
				{name: "named", kind: ValueConstraintFixed, lexical: "second"},
			}
			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					component := requireElementComponentByName(t, schema, test.name)
					declaration, ok := component.ElementDeclaration()
					if !ok {
						t.Fatalf("element %q has no declaration view", test.name)
					}
					constraint, ok := declaration.ValueConstraint()
					if !ok {
						t.Fatalf("element %q has no value constraint", test.name)
					}
					if constraint.Kind() != test.kind || constraint.Lexical() != test.lexical {
						t.Fatalf("element %q constraint = %q/%q, want %q/%q", test.name, constraint.Kind(), constraint.Lexical(), test.kind, test.lexical)
					}
					repeated, ok := declaration.ValueConstraint()
					if !ok || repeated.Lexical() != constraint.Lexical() || repeated.Loc() != constraint.Loc() {
						t.Fatalf("element %q value constraint changed on repeated query", test.name)
					}
					switch test.name {
					case "boolean":
						value, ok := constraint.BooleanValue()
						if !ok || value.Canonical() != "true" {
							t.Fatalf("boolean value = %v/%t, want true/true", value.Canonical(), ok)
						}
					case "integer", "inline":
						value, ok := constraint.IntegerValue()
						if !ok || value.Canonical() == "" {
							t.Fatalf("integer value = %q/%t", value.Canonical(), ok)
						}
					case "decimal":
						value, ok := constraint.DecimalValue()
						if !ok || value.Canonical() != "1.23" || value.Scale() != 2 {
							t.Fatalf("decimal value = %q scale %d/%t", value.Canonical(), value.Scale(), ok)
						}
					case "token", "nmtoken", "string", "named":
						value, ok := constraint.StringValue()
						if !ok || value != test.lexical {
							t.Fatalf("string value = %q/%t, want %q/true", value, ok, test.lexical)
						}
					}
				})
			}
		})
	}
}

func TestSchemaBridgeRetainsGlobalElementPrecisionDecimalValueConstraint(t *testing.T) {
	root := "<xs:schema xmlns:xs='" + testXSDNamespace + "'><xs:element name='value' type='xs:precisionDecimal' fixed=' -0.00 '/></xs:schema>"
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	declaration, ok := schema.Components()[0].ElementDeclaration()
	if !ok {
		t.Fatal("precisionDecimal element has no declaration view")
	}
	constraint, ok := declaration.ValueConstraint()
	if !ok {
		t.Fatal("precisionDecimal element has no value constraint")
	}
	value, ok := constraint.PrecisionDecimalValue()
	if !ok || !value.IsNegativeZero() || constraint.Lexical() != "-0.00" {
		t.Fatalf("precisionDecimal value = %q/%t negative-zero:%t", constraint.Lexical(), ok, value.IsNegativeZero())
	}
}

//nolint:gocognit // Keep constraint diagnostic precedence together.
func TestSchemaBridgeRejectsInvalidGlobalElementValueConstraints(t *testing.T) {
	tests := []struct {
		name        string
		root        string
		policy      LanguagePolicy
		class       FailureClass
		code        string
		cause       error
		spec        string
		unsupported bool
	}{
		{
			name: "invalid lexical", root: globalElementValueConstraintSchema("<xs:element name='value' type='xs:integer' default='bad'/>"),
			policy: Strict10, class: FailureInvalid, code: diagnosticSchemaElementValueConstraintCode,
			cause: errSchemaElementValueConstraintInvalid, spec: schemaElementValueXSD10SpecRef,
		},
		{
			name: "facet violation", root: globalElementValueConstraintSchema("<xs:element name='value' type='r:Bounded' default='2'/><xs:simpleType name='Bounded'><xs:restriction base='xs:integer'><xs:minInclusive value='3'/></xs:restriction></xs:simpleType>"),
			policy: Strict11, class: FailureInvalid, code: diagnosticSchemaElementValueConstraintCode,
			cause: errSchemaElementValueConstraintInvalid, spec: schemaElementValueXSD11SpecRef,
		},
		{
			name: "complex type", root: globalElementValueConstraintSchema("<xs:element name='value' type='r:Complex' default='1'/><xs:complexType name='Complex'><xs:sequence/></xs:complexType>"),
			policy: Strict11, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode,
			cause: errSchemaElementValueConstraintUnsupported, spec: schemaElementValueConstraintXSD11SpecRef, unsupported: true,
		},
		{
			name: "excluded ID type", root: globalElementValueConstraintSchema("<xs:element name='value' type='xs:ID' default='id'/>"),
			policy: Strict10, class: FailureUnsupported, code: UnsupportedSchemaSyntaxCode,
			cause: errSchemaElementValueConstraintUnsupported, spec: schemaElementValueConstraintXSD10SpecRef, unsupported: true,
		},
		{
			name: "conflicting facts", root: globalElementValueConstraintSchema("<xs:element name='value' type='xs:integer' default='1' fixed='2'/>"),
			policy: Strict10, class: FailureInvalid, code: invalidSchemaCompositionCode,
			cause: errSchemaElementValueConstraintConflict, spec: schemaElementValueConstraintXSD10SpecRef,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, test.root, nil, test.policy)
			if err == nil || schema.storage != nil || len(schema.Components()) != 0 {
				t.Fatalf("discoverSchema accepted constraint or returned schema: %v", err)
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != test.class || diagnostic.Code() != test.code || diagnostic.SpecRef() != test.spec {
				t.Fatalf("diagnostic = %s/%s/%q/%q, want %s/%s/%s", diagnostic, diagnostic.Class(), diagnostic.Code(), diagnostic.SpecRef(), test.class, test.code, test.spec)
			}
			if !errors.Is(err, test.cause) {
				t.Fatalf("diagnostic lost cause %v: %v", test.cause, err)
			}
			if test.unsupported && !errors.Is(err, ErrUnsupported) {
				t.Fatalf("diagnostic lost unsupported cause: %v", err)
			}
		})
	}
}

func TestGlobalElementValueConstraintsRemainConsumerUnsupported(t *testing.T) {
	root := globalElementValueConstraintSchema("<xs:element name='value' type='xs:integer' default='1'/>")
	schema, err := discoverTestSchemaWithPolicy(t, root, nil, Strict11)
	if err != nil {
		t.Fatalf("discoverSchema: %v", err)
	}
	validationErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader("<value xmlns='urn:root'>1</value>")))
	if validationErr == nil || !errors.Is(validationErr, errInstanceElementValueConstraint) {
		t.Fatalf("ValidateInstance error = %v, want explicit element-value-constraint unsupported", validationErr)
	}
	validationDiagnostic := requireDiagnostic(t, validationErr)
	if validationDiagnostic.Class() != FailureUnsupported || validationDiagnostic.Code() != UnsupportedInstanceValidationCode {
		t.Fatalf("validation diagnostic = %s, want instance-validation unsupported", validationDiagnostic)
	}
	source, generationErr := GenerateGo(schema, "generated")
	if generationErr == nil || len(source) != 0 || !errors.Is(generationErr, errCodegenUnsupported) {
		t.Fatalf("GenerateGo result = %q/%v, want explicit unsupported", source, generationErr)
	}
	generationDiagnostic := requireDiagnostic(t, generationErr)
	if generationDiagnostic.Class() != FailureUnsupported || generationDiagnostic.Code() != diagnosticCodegenUnsupported {
		t.Fatalf("codegen diagnostic = %s, want codegen unsupported", generationDiagnostic)
	}
}

func requireElementComponentByName(t *testing.T, schema Schema, name string) Component {
	t.Helper()
	components := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:root", name))
	if len(components) != 1 {
		t.Fatalf("element %q matches = %d, want one", name, len(components))
	}
	return components[0]
}
