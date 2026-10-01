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

func TestCodegenDirectParticleSourceRejectsNamedChoiceTargetIdentifierCorruption(t *testing.T) {
	schema := codegenDirectChoiceReferenceTestSchema(
		t,
		`<xs:simpleType name="Amount"><xs:restriction base="xs:integer"/></xs:simpleType><xs:element name="item" type="r:Amount"/>`,
		`<xs:element ref="r:item"/>`,
		Compatibility,
	)
	directPlan, err := planCodegenDirectParticles(schema, "generated")
	if err != nil {
		t.Fatalf("planCodegenDirectParticles: %v", err)
	}
	if len(directPlan.owners) != 1 || directPlan.owners[0].choice == nil || len(directPlan.owners[0].choice.alternatives) != 1 {
		t.Fatalf("direct particle plan = %#v, want one choice owner with one alternative", directPlan)
	}
	alternative := &directPlan.owners[0].choice.alternatives[0]
	target, ok := alternative.target.(codegenDirectChoiceNamedTarget)
	if !ok {
		t.Fatalf("choice target = %T, want named target", alternative.target)
	}
	target.componentIdentifier = "Missing"
	alternative.target = target

	output, err := emitCodegenSourceWithDirectParticles(schema, directPlan)
	if output != nil || err == nil {
		t.Fatalf("corrupted direct-particle result = (%q, %v), want nil output and error", output, err)
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInternal || diagnostic.Code() != diagnosticCodegenInvariant {
		t.Fatalf("diagnostic = %s, want internal codegen invariant", diagnostic)
	}
	wantLoc := codegenDirectChoiceReferenceTestParticle(t, schema).Loc()
	if diagnostic.Loc() != wantLoc {
		t.Fatalf("diagnostic location = %s, want reference location %s", diagnostic.Loc(), wantLoc)
	}
	if !errors.Is(err, errCodegenDirectParticlePlan) {
		t.Fatalf("diagnostic lost direct-particle plan cause: %v", err)
	}
}

func TestCodegenDirectParticleSourceRejectsBooleanChoiceTargetCorruption(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *codegenDirectParticlePlan)
	}{
		{
			name: "Boolean encoded as integer family",
			mutate: func(t *testing.T, plan *codegenDirectParticlePlan) {
				target, ok := plan.owners[0].choice.alternatives[0].target.(codegenDirectChoiceBuiltinTarget)
				if !ok {
					t.Fatalf("choice target = %T, want built-in target", plan.owners[0].choice.alternatives[0].target)
				}
				target.family = codegenDirectChoiceScalarInteger
				target.kind = DigitDatatypeInteger
				plan.owners[0].choice.alternatives[0].target = target
			},
		},
		{
			name: "Boolean carries a digit kind",
			mutate: func(t *testing.T, plan *codegenDirectParticlePlan) {
				target, ok := plan.owners[0].choice.alternatives[0].target.(codegenDirectChoiceBuiltinTarget)
				if !ok {
					t.Fatalf("choice target = %T, want built-in target", plan.owners[0].choice.alternatives[0].target)
				}
				target.kind = DigitDatatypeInteger
				plan.owners[0].choice.alternatives[0].target = target
			},
		},
		{
			name: "named Boolean family changes",
			mutate: func(t *testing.T, plan *codegenDirectParticlePlan) {
				target, ok := plan.owners[0].choice.alternatives[1].target.(codegenDirectChoiceNamedTarget)
				if !ok {
					t.Fatalf("choice target = %T, want named target", plan.owners[0].choice.alternatives[1].target)
				}
				target.family = codegenDirectChoiceScalarDecimal
				target.kind = DigitDatatypeDecimal
				plan.owners[0].choice.alternatives[1].target = target
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema, err := discoverTestSchema(t, `<xs:schema xmlns:xs="`+testXSDNamespace+`" xmlns:r="urn:boolean" targetNamespace="urn:boolean">
  <xs:complexType name="Choice"><xs:choice>
    <xs:element name="builtin" type="xs:boolean"/>
    <xs:element name="named" type="r:Flag"/>
  </xs:choice></xs:complexType>
  <xs:simpleType name="Flag"><xs:restriction base="xs:boolean"/></xs:simpleType>
</xs:schema>`, nil)
			if err != nil {
				t.Fatalf("discoverTestSchema: %v", err)
			}
			plan, err := planCodegenDirectParticles(schema, "generated")
			if err != nil {
				t.Fatalf("planCodegenDirectParticles: %v", err)
			}
			test.mutate(t, &plan)
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
