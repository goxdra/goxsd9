package goxsd9

import (
	"errors"
	"testing"
)

func TestCodegenDirectSequencePlanRejectsCorruptionAtSourceBoundary(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*codegenDirectParticlePlan)
	}{
		{
			name: "missing field",
			mutate: func(plan *codegenDirectParticlePlan) {
				plan.owners[0].sequence.fields = plan.owners[0].sequence.fields[:1]
			},
		},
		{
			name: "extra field",
			mutate: func(plan *codegenDirectParticlePlan) {
				plan.owners[0].sequence.fields = append(plan.owners[0].sequence.fields, plan.owners[0].sequence.fields[0])
			},
		},
		{
			name: "reordered fields",
			mutate: func(plan *codegenDirectParticlePlan) {
				plan.owners[0].sequence.fields[0], plan.owners[0].sequence.fields[1] = plan.owners[0].sequence.fields[1], plan.owners[0].sequence.fields[0]
			},
		},
		{
			name: "stale target",
			mutate: func(plan *codegenDirectParticlePlan) {
				plan.owners[0].sequence.fields[0].target.scalarKind = codegenSourceScalarDecimal
			},
		},
		{
			name: "typed nil sequence owner",
			mutate: func(plan *codegenDirectParticlePlan) {
				var sequence *codegenDirectSequenceOwner
				plan.owners[0].sequence = sequence
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema := codegenDirectSequenceTestSchema(t)
			plan, err := planCodegenDirectParticles(schema, "generated")
			if err != nil {
				t.Fatalf("planCodegenDirectParticles: %v", err)
			}
			test.mutate(&plan)
			output, err := emitCodegenSourceWithDirectParticles(schema, plan)
			assertCodegenDirectSequenceInternalFailure(t, output, err, errCodegenDirectParticlePlan)
		})
	}
}

func TestCodegenDirectSequenceSourceRejectsCorruptionAtRenderBoundary(t *testing.T) {
	tests := []struct {
		name   string
		cause  error
		mutate func(Schema, *codegenSourcePlan)
	}{
		{
			name: "missing field",
			mutate: func(_ Schema, plan *codegenSourcePlan) {
				plan.declarations[0].sequence.fields = plan.declarations[0].sequence.fields[:1]
			},
		},
		{
			name: "extra field",
			mutate: func(_ Schema, plan *codegenSourcePlan) {
				plan.declarations[0].sequence.fields = append(plan.declarations[0].sequence.fields, plan.declarations[0].sequence.fields[0])
			},
		},
		{
			name: "reordered fields",
			mutate: func(_ Schema, plan *codegenSourcePlan) {
				plan.declarations[0].sequence.fields[0], plan.declarations[0].sequence.fields[1] = plan.declarations[0].sequence.fields[1], plan.declarations[0].sequence.fields[0]
			},
		},
		{
			name: "stale field type",
			mutate: func(_ Schema, plan *codegenSourcePlan) {
				plan.declarations[0].sequence.fields[0].fieldType = "bool"
			},
		},
		{
			name: "typed nil sequence",
			mutate: func(_ Schema, plan *codegenSourcePlan) {
				var sequence *codegenSourceSequence
				plan.declarations[0].sequence = sequence
			},
		},
		{
			name:  "stale schema particle facts",
			cause: errCodegenDirectSequenceParticle,
			mutate: func(schema Schema, _ *codegenSourcePlan) {
				codegenDirectSequenceTestBody(schema).particle = SequenceParticle{}
			},
		},
		{
			name: "reordered schema particles",
			mutate: func(schema Schema, _ *codegenSourcePlan) {
				body := codegenDirectSequenceTestBody(schema)
				sequence, ok := body.particle.(SequenceParticle)
				if !ok {
					t.Fatalf("particle = %T, want SequenceParticle", body.particle)
				}
				sequence.facts.particles[0], sequence.facts.particles[1] = sequence.facts.particles[1], sequence.facts.particles[0]
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema := codegenDirectSequenceTestSchema(t)
			directPlan, err := planCodegenDirectParticles(schema, "generated")
			if err != nil {
				t.Fatalf("planCodegenDirectParticles: %v", err)
			}
			plan, err := planCodegenSourceWithDirectParticles(schema, directPlan)
			if err != nil {
				t.Fatalf("planCodegenSourceWithDirectParticles: %v", err)
			}
			test.mutate(schema, &plan)
			output, err := renderCodegenSource(plan, schema)
			cause := test.cause
			if cause == nil {
				cause = errCodegenSchemaInvariant
			}
			assertCodegenDirectSequenceInternalFailure(t, output, err, cause)
		})
	}
}

func TestCodegenDirectSequenceSourceUsesExpectedFieldLocationForCorruption(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*codegenSourcePlan, *testing.T)
	}{
		{
			name: "zero field location",
			mutate: func(plan *codegenSourcePlan, _ *testing.T) {
				plan.declarations[0].sequence.fields[0].loc = Loc{}
			},
		},
		{
			name: "stale field location",
			mutate: func(plan *codegenSourcePlan, t *testing.T) {
				plan.declarations[0].sequence.fields[0].loc = mustTestLoc(t, "stale.xsd", 17, 19)
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			schema := codegenDirectSequenceTestSchema(t)
			directPlan, err := planCodegenDirectParticles(schema, "generated")
			if err != nil {
				t.Fatalf("planCodegenDirectParticles: %v", err)
			}
			plan, err := planCodegenSourceWithDirectParticles(schema, directPlan)
			if err != nil {
				t.Fatalf("planCodegenSourceWithDirectParticles: %v", err)
			}
			wantLoc := plan.declarations[0].sequence.fields[0].loc
			test.mutate(&plan, t)

			output, err := renderCodegenSource(plan, schema)
			assertCodegenDirectSequenceInternalFailure(t, output, err, errCodegenSchemaInvariant)
			diagnostic := requireDiagnostic(t, err)
			if diagnostic.Loc() != wantLoc {
				t.Fatalf("diagnostic location = %s, want recollected field location %s", diagnostic.Loc(), wantLoc)
			}
		})
	}
}

func TestCodegenDirectSequenceSourceRejectsContradictoryParticleMode(t *testing.T) {
	schema := codegenDirectSequenceTestSchema(t)
	directPlan, err := planCodegenDirectParticles(schema, "generated")
	if err != nil {
		t.Fatalf("planCodegenDirectParticles: %v", err)
	}
	plan, err := planCodegenSourceWithDirectParticles(schema, directPlan)
	if err != nil {
		t.Fatalf("planCodegenSourceWithDirectParticles: %v", err)
	}
	plan.directParticles = false
	plan.directChoices = true

	output, err := renderCodegenSource(plan, schema)
	assertCodegenDirectSequenceInternalFailure(t, output, err, errCodegenSchemaInvariant)
}

func codegenDirectSequenceTestSchema(t *testing.T) Schema {
	t.Helper()
	schema, err := discoverTestSchema(t, `<xs:schema xmlns:xs="`+testXSDNamespace+`" targetNamespace="urn:sequence">
  <xs:complexType name="Record"><xs:sequence>
    <xs:element name="first" type="xs:integer"/>
    <xs:element name="second" type="xs:decimal"/>
  </xs:sequence></xs:complexType>
</xs:schema>`, nil)
	if err != nil {
		t.Fatalf("discoverTestSchema: %v", err)
	}
	return schema
}

func codegenDirectSequenceTestBody(schema Schema) *schemaComplexTypeDirectBodyComponent {
	body, ok := schema.Components()[0].complexType.body.(*schemaComplexTypeDirectBodyComponent)
	if !ok || body == nil {
		panic("test fixture did not build a direct complex type body")
	}
	return body
}

func assertCodegenDirectSequenceInternalFailure(t *testing.T, output []byte, err error, cause error) {
	t.Helper()
	if output != nil || err == nil {
		t.Fatalf("corrupted direct-sequence result = (%q, %v), want nil output and error", output, err)
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInternal || diagnostic.Code() != diagnosticCodegenInvariant {
		t.Fatalf("diagnostic = %s, want internal codegen invariant", diagnostic)
	}
	if diagnostic.Loc().IsZero() || diagnostic.Loc().Source() != "root.xsd" {
		t.Fatalf("diagnostic location = %s, want a located root.xsd diagnostic", diagnostic.Loc())
	}
	if !errors.Is(err, cause) {
		t.Fatalf("corruption error = %v, want cause %v", err, cause)
	}
}
