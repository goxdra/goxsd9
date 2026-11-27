package goxsd9

import (
	"errors"
	"io"
	"strings"
	"testing"
)

//nolint:gocognit // Choice and sequence references share the public failure contract.
func TestUntypedGlobalReferenceConsumersRejectBeforeTypeDispatch(t *testing.T) {
	for _, policy := range untypedGlobalPolicies() {
		for _, model := range []string{"choice", "sequence"} {
			t.Run(policy.name+"/"+model, func(t *testing.T) {
				root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root">
  <xs:complexType name="Container"><xs:` + model + `><xs:element ref="r:target"/></xs:` + model + `></xs:complexType>
  <xs:element name="container" type="r:Container"/>
  <xs:element name="target" abstract="true" nillable="1"/>
</xs:schema>`
				schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy.policy)
				if err != nil {
					t.Fatalf("parse: %v", err)
				}
				target, ok := schema.FindKind(ComponentKindElementDeclaration, mustTestQName(t, "urn:root", "target"))[0].ElementDeclaration()
				if !ok || !target.IsUntyped() {
					t.Fatal("missing referenced untyped declaration")
				}
				instance := `<container xmlns="urn:root"><target/></container>`
				instanceErr := ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(instance)))
				validation := requireDiagnostic(t, instanceErr)
				version := instanceSchemaValidationVersion(schema)
				wantInstanceSpec := requireDiagnostic(t, newInstanceValidationUnsupported(Loc{}, "", nil, version, errInstanceNoDeclaredType)).SpecRef()
				if validation.Class() != FailureUnsupported || validation.Code() != UnsupportedInstanceValidationCode || validation.Loc() != mustTestLoc(t, "instance.xml", 1, 1) || validation.SpecRef() != wantInstanceSpec || !errors.Is(instanceErr, errInstanceNoDeclaredType) {
					t.Fatalf("validation = %s", validation)
				}
				if len(validation.Related()) == 0 || validation.Related()[len(validation.Related())-1] != target.Loc() {
					t.Fatalf("validation related = %v, want target location", validation.Related())
				}
				output, generationErr := GenerateGo(schema, "generated")
				generation := requireDiagnostic(t, generationErr)
				wantRefLoc := schemaMixedComplexLoc(t, root, `ref="r:target"`)
				wantGenerationSpec := codegenDirectChoiceSpecReference(version, codegenDirectChoiceElementChoiceReference)
				if model == "sequence" {
					wantGenerationSpec = codegenDirectSequenceSpecReference(version, codegenDirectSequenceElementReference)
				}
				if output != nil || generation.Class() != FailureUnsupported || generation.Code() != diagnosticCodegenUnsupported || generation.Loc() != wantRefLoc || generation.SpecRef() != wantGenerationSpec || !errors.Is(generationErr, errCodegenUnsupported) {
					t.Fatalf("generation = %q/%s, want ref Loc %s", output, generation, wantRefLoc)
				}
				containsTarget := false
				for _, related := range generation.Related() {
					if related == target.Loc() {
						containsTarget = true
					}
				}
				if !containsTarget {
					t.Fatalf("generation related = %v, want target location", generation.Related())
				}
			})
		}
	}
}
