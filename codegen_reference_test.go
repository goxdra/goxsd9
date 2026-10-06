package goxsd9

import (
	"errors"
	"testing"
)

type codegenReferenceTestSchemaFunc func(*testing.T, string, string, LanguagePolicy) Schema
type codegenReferenceTestParticleFunc func(*testing.T, Schema) ElementReferenceParticle

func assertCodegenDirectReferenceFailClosed(
	t *testing.T,
	schemaFunc codegenReferenceTestSchemaFunc,
	particleFunc codegenReferenceTestParticleFunc,
	invariant error,
) {
	t.Helper()
	tests := []struct {
		name   string
		mutate func(*testing.T, Schema)
	}{
		{
			name: "zero target ID",
			mutate: func(t *testing.T, schema Schema) {
				particleFunc(t, schema).facts.targetID = ComponentID{}
			},
		},
		{
			name: "invalid target source",
			mutate: func(t *testing.T, schema Schema) {
				particleFunc(t, schema).facts.targetID = ComponentID{ordinal: 1}
			},
		},
		{
			name: "invalid target ordinal",
			mutate: func(t *testing.T, schema Schema) {
				particleFunc(t, schema).facts.targetID = ComponentID{source: "missing.xsd"}
			},
		},
		{
			name: "missing nonzero target",
			mutate: func(t *testing.T, schema Schema) {
				particleFunc(t, schema).facts.targetID = ComponentID{source: "missing.xsd", ordinal: 1}
			},
		},
		{
			name: "wrong target kind",
			mutate: func(t *testing.T, schema Schema) {
				component := schema.FindKind(ComponentKindSimpleTypeDefinition, mustTestQName(t, "urn:reference-root", "notElement"))[0]
				particleFunc(t, schema).facts.targetID = component.ID()
			},
		},
		{
			name: "target name mismatch",
			mutate: func(t *testing.T, schema Schema) {
				component := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:reference-root", "other"))[0]
				particleFunc(t, schema).facts.targetID = component.ID()
			},
		},
		{
			name: "corrupt reference name",
			mutate: func(t *testing.T, schema Schema) {
				particleFunc(t, schema).facts.name = QName{}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema := schemaFunc(
				t,
				`<xs:element name="item" type="xs:integer"/><xs:element name="other" type="xs:integer"/><xs:simpleType name="notElement"><xs:restriction base="xs:integer"/></xs:simpleType>`,
				`<xs:element ref="r:item"/>`,
				Compatibility,
			)
			reference := particleFunc(t, schema)
			wantLoc := reference.RefLoc()
			test.mutate(t, schema)
			output, err := GenerateGo(schema, "generated")
			if output != nil || err == nil {
				t.Fatalf("GenerateGo result = (%q, %v), want nil output and an internal failure", output, err)
			}
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Class() != FailureInternal || diagnostic.Code() != diagnosticCodegenInvariant {
				t.Fatalf("diagnostic = %s, want internal codegen invariant", diagnostic)
			}
			if diagnostic.Loc() != wantLoc {
				t.Fatalf("diagnostic location = %s, want ref location %s", diagnostic.Loc(), wantLoc)
			}
			if !errors.Is(err, invariant) {
				t.Fatalf("diagnostic lost target invariant cause: %v", err)
			}
		})
	}
}
