package goxsd9

import (
	"errors"
	"go/format"
	"reflect"
	"strings"
	"testing"
)

//nolint:gocognit // Keep the compact alternative corpus and record assertions together.
func TestPlanCodegenDirectChoicesBuildsEmptyOneAndMultipleAlternatives(t *testing.T) {
	tests := []struct {
		name           string
		choice         string
		wantAlternates int
	}{
		{name: "empty", choice: "<xs:choice/>", wantAlternates: 0},
		{name: "one", choice: `<xs:choice><xs:element name="amount" type="xs:decimal"/></xs:choice>`, wantAlternates: 1},
		{name: "multiple", choice: `<xs:choice><xs:element name="whole" type="xs:integer"/><xs:element name="amount" type="xs:decimal"/></xs:choice>`, wantAlternates: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" targetNamespace="urn:test"><xs:complexType name="Choice">` + test.choice + `</xs:complexType></xs:schema>`
			schema, err := discoverTestSchema(t, root, nil)
			if err != nil {
				t.Fatalf("discoverTestSchema: %v", err)
			}
			plan, err := planCodegenDirectChoices(schema, "generated")
			if err != nil {
				t.Fatalf("planCodegenDirectChoices: %v", err)
			}
			if len(plan.owners) != 1 {
				t.Fatalf("owner count = %d, want 1", len(plan.owners))
			}
			owner := plan.owners[0]
			if owner.id != schema.Components()[0].ID() || owner.name != schema.Components()[0].Name() || owner.loc != schema.Components()[0].Loc() {
				t.Fatalf("owner identity record = %#v, want schema component identity", owner)
			}
			if owner.identifier != "Choice" || owner.choiceLoc.IsZero() {
				t.Fatalf("owner generated facts = (%q, %s), want Choice and a choice location", owner.identifier, owner.choiceLoc)
			}
			if len(owner.alternatives) != test.wantAlternates {
				t.Fatalf("alternative count = %d, want %d", len(owner.alternatives), test.wantAlternates)
			}
			for index, alternative := range owner.alternatives {
				wantPath := []uint32{1, uint32(index + 1)}
				if !equalCodegenPath(alternative.path, wantPath) {
					t.Fatalf("alternative %d path = %v, want %v", index, alternative.path, wantPath)
				}
				if alternative.loc.IsZero() || alternative.fieldIdentifier == "" || alternative.variantIdentifier == "" {
					t.Fatalf("alternative %d generated/source facts = %#v, want locations and names", index, alternative)
				}
				if _, ok := alternative.target.(codegenDirectChoiceBuiltinTarget); !ok {
					t.Fatalf("alternative %d target = %T, want built-in target", index, alternative.target)
				}
			}
		})
	}
}

func TestPlanCodegenDirectChoicesPreservesTargetsOrderAndIDs(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test">
  <xs:complexType name="Choice"><xs:choice>
    <xs:element name="first" type="xs:integer"/>
    <xs:element name="second" type="t:Amount"/>
    <xs:element name="third" type="xs:decimal"/>
  </xs:choice></xs:complexType>
  <xs:simpleType name="Amount"><xs:restriction base="xs:decimal"/></xs:simpleType>
</xs:schema>`
	schema, err := discoverTestSchema(t, root, nil)
	if err != nil {
		t.Fatalf("discoverTestSchema: %v", err)
	}
	plan, err := planCodegenDirectChoices(schema, "generated")
	if err != nil {
		t.Fatalf("planCodegenDirectChoices: %v", err)
	}
	owner := plan.owners[0]
	if got, want := len(owner.alternatives), 3; got != want {
		t.Fatalf("alternative count = %d, want %d", got, want)
	}
	wantNames := []string{"first", "second", "third"}
	for index, alternative := range owner.alternatives {
		if got, want := alternative.name.Local(), wantNames[index]; got != want {
			t.Fatalf("alternative %d name = %q, want %q", index, got, want)
		}
	}
	first, ok := owner.alternatives[0].target.(codegenDirectChoiceBuiltinTarget)
	if !ok || first.declaredType.Local() != "integer" || first.kind != DigitDatatypeInteger {
		t.Fatalf("first target = %#v, want built-in integer", owner.alternatives[0].target)
	}
	named, ok := owner.alternatives[1].target.(codegenDirectChoiceNamedTarget)
	if !ok {
		t.Fatalf("second target = %T, want named target", owner.alternatives[1].target)
	}
	if named.declaredType.Local() != "Amount" || named.kind != DigitDatatypeDecimal || named.id != schema.Components()[1].ID() || named.componentIdentifier != "Amount" {
		t.Fatalf("named target = %#v, want Amount identity and generated name", named)
	}
	third, ok := owner.alternatives[2].target.(codegenDirectChoiceBuiltinTarget)
	if !ok || third.declaredType.Local() != "decimal" || third.kind != DigitDatatypeDecimal {
		t.Fatalf("third target = %#v, want built-in decimal", owner.alternatives[2].target)
	}
}

func TestPlanCodegenDirectChoicesIsDeterministicAcrossXSDPolicies(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + testXSDNamespace + `" xmlns:t="urn:test" targetNamespace="urn:test">
  <xs:complexType name="Choice"><xs:choice><xs:element name="value" type="t:Amount"/></xs:choice></xs:complexType>
  <xs:simpleType name="Amount"><xs:restriction base="xs:integer"/></xs:simpleType>
</xs:schema>`
	for _, policy := range []LanguagePolicy{Strict10, Strict11} {
		t.Run(string(policy), func(t *testing.T) {
			schema, err := discoverTestSchemaWithPolicy(t, root, nil, policy)
			if err != nil {
				t.Fatalf("discoverTestSchemaWithPolicy: %v", err)
			}
			first, err := planCodegenDirectChoices(schema, "generated")
			if err != nil {
				t.Fatalf("first plan: %v", err)
			}
			second, err := planCodegenDirectChoices(schema, "generated")
			if err != nil {
				t.Fatalf("second plan: %v", err)
			}
			if !reflect.DeepEqual(first, second) {
				t.Fatalf("repeated plan differs:\nfirst: %#v\nsecond: %#v", first, second)
			}
		})
	}
}

//nolint:gocognit // Keep ordered component, field, variant, and import assertions together.
func TestPlanCodegenDirectChoicesAllocatesAllOrderedNamingScopes(t *testing.T) {
	schema := codegenDirectChoiceCollisionSchema(t)
	plan, err := planCodegenDirectChoices(schema, "generated")
	if err != nil {
		t.Fatalf("planCodegenDirectChoices: %v", err)
	}
	owner := plan.owners[0]
	if owner.identifier != "Runtime" || plan.runtimeAlias != "Runtime2" {
		t.Fatalf("runtime collision names = (%q, %q), want Runtime and Runtime2", owner.identifier, plan.runtimeAlias)
	}
	wantFields := []string{"LineItem", "LineItem2", "Shared"}
	wantVariants := []string{"LineItem", "LineItem2", "Shared3"}
	for index, alternative := range owner.alternatives {
		if alternative.fieldIdentifier != wantFields[index] || alternative.variantIdentifier != wantVariants[index] {
			t.Fatalf("alternative %d names = (%q, %q), want (%q, %q)", index, alternative.fieldIdentifier, alternative.variantIdentifier, wantFields[index], wantVariants[index])
		}
	}
	target, ok := owner.alternatives[2].target.(codegenDirectChoiceNamedTarget)
	if !ok {
		t.Fatalf("cross-document target = %T, want named target", owner.alternatives[2].target)
	}
	if target.id != schema.Components()[5].ID() || target.componentIdentifier != "Shared2" {
		t.Fatalf("cross-document target = %#v, want exact ID and namespace collision name Shared2", target)
	}
	if got := schema.Components()[3].Name().Local(); got != "type" {
		t.Fatalf("keyword fixture component = %q, want type", got)
	}
	// The component and case-fold reservations are consumed before variants.
	names, err := newCodegenNaming(codegenDirectChoiceNamingInput("generated", schema, []codegenDirectChoiceCollectedOwner{
		{id: owner.id, name: owner.name, loc: owner.loc, choiceLoc: owner.choiceLoc},
	}))
	if err != nil {
		t.Fatalf("newCodegenNaming empty scoped input: %v", err)
	}
	if got, ok := names.componentName(schema.Components()[3].ID()); !ok || got != "XType" {
		t.Fatalf("keyword component name = %q, %t, want XType", got, ok)
	}
	if got, ok := names.componentName(schema.Components()[4].ID()); !ok || got != "XType2" {
		t.Fatalf("case-fold component name = %q, %t, want XType2", got, ok)
	}
}

func TestCodegenDirectChoiceSourceConsumesCompleteNamingState(t *testing.T) {
	schema := codegenDirectChoiceCollisionSchema(t)
	choicePlan, err := planCodegenDirectChoices(schema, "generated")
	if err != nil {
		t.Fatalf("planCodegenDirectChoices: %v", err)
	}
	source, err := emitCodegenSourceWithDirectChoices(schema, choicePlan)
	if err != nil {
		t.Fatalf("emitCodegenSourceWithDirectChoices: %v", err)
	}
	formatted, err := format.Source(source)
	if err != nil {
		t.Fatalf("format generated direct-choice source: %v\n%s", err, source)
	}
	if string(source) != string(formatted) {
		t.Fatalf("direct-choice source is not go/format output:\n%s", source)
	}
	for _, fragment := range []string{
		`import Runtime2 "github.com/goxdra/goxsd9"`,
		"type Runtime interface {\n\tisRuntime()\n}",
		"type LineItem struct {\n\tLineItem Runtime2.StrictInteger\n}",
		"type LineItem2 struct {\n\tLineItem2 Runtime2.StrictDecimal\n}",
		"type Shared3 struct {\n\tShared Shared2\n}",
		"type Shared2 struct {\n\tValue Runtime2.StrictInteger\n}",
		"type XType struct {\n\tValue Runtime2.StrictInteger\n}",
		"func (Shared3) isRuntime() {}",
	} {
		if !strings.Contains(string(source), fragment) {
			t.Fatalf("direct-choice source is missing %q:\n%s", fragment, source)
		}
	}
	compileGeneratedCode(t, source)
}

func TestCodegenDirectChoiceSourceRejectsMalformedPlanWithoutOutput(t *testing.T) {
	schema := codegenDirectChoiceFailureSchema(t)
	choicePlan, err := planCodegenDirectChoices(schema, "generated")
	if err != nil {
		t.Fatalf("planCodegenDirectChoices: %v", err)
	}
	choicePlan.owners[0].alternatives[0].fieldIdentifier = ""
	output, err := emitCodegenSourceWithDirectChoices(schema, choicePlan)
	if output != nil || err == nil {
		t.Fatalf("malformed direct-choice plan result = (%q, %v), want nil output and error", output, err)
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureInternal || diagnostic.Code() != diagnosticCodegenInvariant {
		t.Fatalf("diagnostic = %s, want internal codegen invariant", diagnostic)
	}
	wantLoc := codegenDirectChoiceTestElement(codegenDirectChoiceTestChoice(schema)).Loc()
	if diagnostic.Loc() != wantLoc {
		t.Fatalf("diagnostic location = %s, want alternative location %s", diagnostic.Loc(), wantLoc)
	}
	if !errors.Is(err, errCodegenDirectChoicePlan) {
		t.Fatalf("malformed plan error lost direct-choice plan cause: %v", err)
	}
}

func TestGenerateGoDirectChoicePreservesUnresolvedTargetDiagnostic(t *testing.T) {
	schema := codegenDirectChoiceFailureSchema(t)
	choice := codegenDirectChoiceTestChoice(schema)
	element := codegenDirectChoiceTestElement(choice)
	element.facts.declaredType = QName{namespace: "urn:missing", local: "Missing"}
	element.facts.typeID = ComponentID{source: "missing.xsd", ordinal: 1}
	element.facts.hasTypeID = true

	output, err := GenerateGo(schema, "generated")
	if output != nil || err == nil {
		t.Fatalf("unresolved direct-choice result = (%q, %v), want nil output and error", output, err)
	}
	diagnostic := requireDiagnostic(t, err)
	if diagnostic.Class() != FailureResolution || diagnostic.Code() != diagnosticCodegenSchemaInvalid {
		t.Fatalf("diagnostic = %s, want direct-choice resolution diagnostic", diagnostic)
	}
	if diagnostic.Loc() != element.Loc() {
		t.Fatalf("diagnostic location = %s, want element location %s", diagnostic.Loc(), element.Loc())
	}
	if !errors.Is(err, errCodegenDirectChoiceResolve) {
		t.Fatalf("resolution diagnostic lost direct-choice cause: %v", err)
	}
}

func TestPlanCodegenDirectChoicesOwnPathsAndClones(t *testing.T) {
	schema, err := discoverTestSchema(t, `<xs:schema xmlns:xs="`+testXSDNamespace+`"><xs:complexType name="Choice"><xs:choice><xs:element name="first" type="xs:integer"/></xs:choice></xs:complexType></xs:schema>`, nil)
	if err != nil {
		t.Fatalf("discoverTestSchema: %v", err)
	}
	plan, err := planCodegenDirectChoices(schema, "generated")
	if err != nil {
		t.Fatalf("planCodegenDirectChoices: %v", err)
	}
	owners := plan.ownerRecords()
	owners[0].alternatives[0].path[0] = 99
	owners[0].alternatives[0].name = QName{}
	if got := plan.owners[0].alternatives[0].path; !equalCodegenPath(got, []uint32{1, 1}) {
		t.Fatalf("plan path after accessor mutation = %v, want /1/1", got)
	}
	if plan.owners[0].alternatives[0].name.IsZero() {
		t.Fatal("plan name changed after accessor mutation")
	}
	clone := plan.clone()
	clone.owners[0].alternatives[0].path[1] = 77
	if got := plan.owners[0].alternatives[0].path; !equalCodegenPath(got, []uint32{1, 1}) {
		t.Fatalf("plan path after clone mutation = %v, want /1/1", got)
	}
}

//nolint:gocognit // Keep the zero-plan diagnostic corpus and shared assertions together.
func TestPlanCodegenDirectChoicesReturnsZeroPlanWithLocatedCauses(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(Schema)
		wantClass  FailureClass
		wantCode   string
		wantCause  error
		wantSpec   string
		wantSource SourceID
	}{
		{
			name: "unsupported bounds",
			mutate: func(schema Schema) {
				choice := codegenDirectChoiceTestChoice(schema)
				choice.facts.minOccurs = 0
			},
			wantClass: FailureUnsupported,
			wantCode:  diagnosticCodegenUnsupported,
			wantCause: errCodegenUnsupported,
			wantSpec:  codegenDirectChoiceXSD11ParticleDetailsSpecRef,
		},
		{
			name: "missing target identity",
			mutate: func(schema Schema) {
				choice := codegenDirectChoiceTestChoice(schema)
				element := codegenDirectChoiceTestElement(choice)
				element.facts.declaredType = QName{namespace: "urn:missing", local: "Missing"}
				element.facts.typeID = ComponentID{source: "missing.xsd", ordinal: 1}
				element.facts.hasTypeID = true
			},
			wantClass: FailureResolution,
			wantCode:  diagnosticCodegenSchemaInvalid,
			wantCause: errCodegenDirectChoiceResolve,
		},
		{
			name: "naming location",
			mutate: func(schema Schema) {
				choice := codegenDirectChoiceTestChoice(schema)
				element := codegenDirectChoiceTestElement(choice)
				element.facts.name = QName{local: "---"}
			},
			wantClass: FailureInvalid,
			wantCode:  invalidCodegenNameCode,
			wantCause: errInvalidCodegenName,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema := codegenDirectChoiceFailureSchema(t)
			test.mutate(schema)
			plan, err := planCodegenDirectChoices(schema, "generated")
			if err == nil {
				t.Fatal("planCodegenDirectChoices unexpectedly succeeded")
			}
			if !reflect.DeepEqual(plan, codegenDirectChoicePlan{}) {
				t.Fatalf("failure plan = %#v, want zero plan", plan)
			}
			var diagnostic Diagnostic
			if !errors.As(err, &diagnostic) {
				t.Fatalf("error %T is not a Diagnostic: %v", err, err)
			}
			if diagnostic.Class() != test.wantClass || diagnostic.Code() != test.wantCode {
				t.Fatalf("diagnostic = (%q,%q), want (%q,%q)", diagnostic.Class(), diagnostic.Code(), test.wantClass, test.wantCode)
			}
			if diagnostic.Loc().IsZero() || diagnostic.Loc().Source() != "choice.xsd" {
				t.Fatalf("diagnostic location = %s, want choice.xsd location", diagnostic.Loc())
			}
			if test.wantSpec != "" && diagnostic.SpecRef() != test.wantSpec {
				t.Fatalf("diagnostic spec ref = %q, want %q", diagnostic.SpecRef(), test.wantSpec)
			}
			if !errors.Is(err, test.wantCause) {
				t.Fatalf("diagnostic lost cause %v: %v", test.wantCause, err)
			}
			if test.wantClass == FailureUnsupported && !errors.Is(err, ErrUnsupported) {
				t.Fatalf("unsupported diagnostic lost ErrUnsupported: %v", err)
			}
		})
	}
}

func TestPlanCodegenDirectChoicesRejectsMalformedFacts(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(Schema)
		cause  error
	}{
		{
			name: "invalid target source",
			mutate: func(schema Schema) {
				choice := codegenDirectChoiceTestChoice(schema)
				element := codegenDirectChoiceTestElement(choice)
				element.facts.declaredType = QName{namespace: "urn:missing", local: "Missing"}
				element.facts.typeID = ComponentID{ordinal: 1}
				element.facts.hasTypeID = true
			},
			cause: errCodegenDirectChoiceTarget,
		},
		{
			name: "invalid target ordinal",
			mutate: func(schema Schema) {
				choice := codegenDirectChoiceTestChoice(schema)
				element := codegenDirectChoiceTestElement(choice)
				element.facts.declaredType = QName{namespace: "urn:missing", local: "Missing"}
				element.facts.typeID = ComponentID{source: "missing.xsd"}
				element.facts.hasTypeID = true
			},
			cause: errCodegenDirectChoiceTarget,
		},
		{
			name: "nil top-level choice",
			mutate: func(schema Schema) {
				component := schema.Components()[0]
				component.complexType.particle = (*ChoiceParticle)(nil)
			},
			cause: errCodegenDirectChoiceParticle,
		},
		{
			name: "nil top-level element",
			mutate: func(schema Schema) {
				component := schema.Components()[0]
				component.complexType.particle = (*ElementParticle)(nil)
			},
			cause: errCodegenDirectChoiceParticle,
		},
		{
			name: "nil choice alternative",
			mutate: func(schema Schema) {
				choice := codegenDirectChoiceTestChoice(schema)
				choice.facts.alternatives[0] = (*ChoiceParticle)(nil)
			},
			cause: errCodegenDirectChoiceParticle,
		},
		{
			name: "nil element alternative",
			mutate: func(schema Schema) {
				choice := codegenDirectChoiceTestChoice(schema)
				choice.facts.alternatives[0] = (*ElementParticle)(nil)
			},
			cause: errCodegenDirectChoiceParticle,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema := codegenDirectChoiceFailureSchema(t)
			test.mutate(schema)
			assertCodegenDirectChoiceInternalFailure(t, schema, test.cause)
		})
	}
}

func TestPlanCodegenDirectChoicesClassifiesMissingComplexFactsAsInternal(t *testing.T) {
	schema := mustTestSchema(t, []schemaDocumentInput{{
		source:  "choice.xsd",
		rootLoc: mustTestLoc(t, "choice.xsd", 1, 1),
		declarations: []schemaComponentInput{{
			kind: ComponentKindComplexTypeDefinition,
			name: mustTestQName(t, "urn:choice", "Choice"),
			loc:  mustTestLoc(t, "choice.xsd", 2, 3),
		}},
	}})
	assertCodegenDirectChoiceInternalFailure(t, schema, errCodegenDirectChoiceParticle)
}

func TestPlanCodegenDirectChoicesClassifiesMissingParticleAsUnsupported(t *testing.T) {
	schema := codegenDirectChoiceFailureSchema(t)
	component := schema.Components()[0]
	component.complexType.particle = nil

	plan, err := planCodegenDirectChoices(schema, "generated")
	if err == nil {
		t.Fatal("planCodegenDirectChoices accepted a complex type without a particle")
	}
	if !reflect.DeepEqual(plan, codegenDirectChoicePlan{}) {
		t.Fatalf("failure plan = %#v, want zero plan", plan)
	}
	var diagnostic Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("error %T is not a Diagnostic: %v", err, err)
	}
	if diagnostic.Class() != FailureUnsupported || diagnostic.Code() != diagnosticCodegenUnsupported {
		t.Fatalf("diagnostic = (%q,%q), want unsupported codegen diagnostic (%q,%q)", diagnostic.Class(), diagnostic.Code(), FailureUnsupported, diagnosticCodegenUnsupported)
	}
	if diagnostic.Feature() != FeatureCodegen || diagnostic.SpecRef() != codegenDirectChoiceXSD11ParticlesSpecRef {
		t.Fatalf("diagnostic feature/specification reference = %q/%q, want %q/%q", diagnostic.Feature(), diagnostic.SpecRef(), FeatureCodegen, codegenDirectChoiceXSD11ParticlesSpecRef)
	}
	if diagnostic.Loc() != component.Loc() {
		t.Fatalf("diagnostic location = %s, want component location %s", diagnostic.Loc(), component.Loc())
	}
	if !errors.Is(err, ErrUnsupported) || !errors.Is(err, errCodegenUnsupported) {
		t.Fatalf("unsupported diagnostic lost its classification causes: %v", err)
	}
}

func TestPlanCodegenDirectChoicesRejectsNilParticleAlternative(t *testing.T) {
	schema := codegenDirectChoiceFailureSchema(t)
	choice := codegenDirectChoiceTestChoice(schema)
	choice.facts.alternatives[0] = nil
	wantLoc := choice.Loc()

	plan, err := planCodegenDirectChoices(schema, "generated")
	if err == nil {
		t.Fatal("planCodegenDirectChoices accepted a nil particle alternative")
	}
	if !reflect.DeepEqual(plan, codegenDirectChoicePlan{}) {
		t.Fatalf("failure plan = %#v, want zero plan", plan)
	}
	var diagnostic Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("error %T is not a Diagnostic: %v", err, err)
	}
	if diagnostic.Class() != FailureInternal || diagnostic.Code() != diagnosticCodegenInvariant {
		t.Fatalf("diagnostic = (%q,%q), want (%q,%q)", diagnostic.Class(), diagnostic.Code(), FailureInternal, diagnosticCodegenInvariant)
	}
	if diagnostic.Loc() != wantLoc {
		t.Fatalf("diagnostic location = %s, want choice location %s", diagnostic.Loc(), wantLoc)
	}
	if !errors.Is(err, errCodegenDirectChoiceParticle) {
		t.Fatalf("diagnostic lost nil particle cause: %v", err)
	}
}

func assertCodegenDirectChoiceInternalFailure(t *testing.T, schema Schema, cause error) {
	t.Helper()
	plan, err := planCodegenDirectChoices(schema, "generated")
	if err == nil {
		t.Fatal("planCodegenDirectChoices unexpectedly succeeded")
	}
	if !reflect.DeepEqual(plan, codegenDirectChoicePlan{}) {
		t.Fatalf("failure plan = %#v, want zero plan", plan)
	}
	var diagnostic Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("error %T is not a Diagnostic: %v", err, err)
	}
	if diagnostic.Class() != FailureInternal || diagnostic.Code() != diagnosticCodegenInvariant {
		t.Fatalf("diagnostic = (%q,%q), want (%q,%q)", diagnostic.Class(), diagnostic.Code(), FailureInternal, diagnosticCodegenInvariant)
	}
	if diagnostic.Loc().IsZero() || diagnostic.Loc().Source() != "choice.xsd" {
		t.Fatalf("diagnostic location = %s, want choice.xsd location", diagnostic.Loc())
	}
	if !errors.Is(err, cause) {
		t.Fatalf("diagnostic lost cause %v: %v", cause, err)
	}
}

func TestPlanCodegenDirectChoicesRejectsIncompleteElementParticle(t *testing.T) {
	schema := codegenDirectChoiceFailureSchema(t)
	choice := codegenDirectChoiceTestChoice(schema)
	choice.facts.alternatives[0] = ElementParticle{}
	plan, err := planCodegenDirectChoices(schema, "generated")
	if err == nil {
		t.Fatal("planCodegenDirectChoices accepted incomplete element particle")
	}
	if !reflect.DeepEqual(plan, codegenDirectChoicePlan{}) {
		t.Fatalf("failure plan = %#v, want zero plan", plan)
	}
	var diagnostic Diagnostic
	if !errors.As(err, &diagnostic) || diagnostic.Class() != FailureInternal || diagnostic.Code() != diagnosticCodegenInvariant {
		t.Fatalf("diagnostic = %v, want internal direct-choice invariant", err)
	}
	if !errors.Is(err, errCodegenDirectChoiceParticle) {
		t.Fatalf("diagnostic lost incomplete particle cause: %v", err)
	}
}

func codegenDirectChoiceTestChoice(schema Schema) ChoiceParticle {
	choice, ok := schema.Components()[0].complexType.particle.(ChoiceParticle)
	if !ok {
		panic("test fixture did not build a ChoiceParticle")
	}
	return choice
}

func codegenDirectChoiceTestElement(choice ChoiceParticle) ElementParticle {
	element, ok := choice.facts.alternatives[0].(ElementParticle)
	if !ok {
		panic("test fixture did not build an ElementParticle")
	}
	return element
}

func codegenDirectChoiceCollisionSchema(t *testing.T) Schema {
	t.Helper()
	choiceLoc := mustTestLoc(t, "owner.xsd", 4, 5)
	return mustTestSchema(t, []schemaDocumentInput{
		{
			source:          "owner.xsd",
			rootLoc:         mustTestLoc(t, "owner.xsd", 1, 1),
			targetNamespace: "urn:owner",
			declarations: []schemaComponentInput{
				{
					kind: ComponentKindComplexTypeDefinition,
					name: mustTestQName(t, "urn:owner", "runtime"),
					loc:  mustTestLoc(t, "owner.xsd", 2, 3),
					complexType: &schemaComplexTypeInput{particle: &schemaChoiceParticleInput{
						loc:       choiceLoc,
						minOccurs: 1,
						maxOccurs: 1,
						alternatives: []schemaElementParticleInput{
							codegenDirectChoiceElementInput(t, "line-item", mustTestQName(t, testXSDNamespace, "integer"), mustTestLoc(t, "owner.xsd", 5, 7)),
							codegenDirectChoiceElementInput(t, "LINE_ITEM", mustTestQName(t, testXSDNamespace, "decimal"), mustTestLoc(t, "owner.xsd", 6, 7)),
							codegenDirectChoiceElementInput(t, "shared", mustTestQName(t, "urn:other", "shared"), mustTestLoc(t, "owner.xsd", 7, 7)),
						},
					}},
				},
				{kind: ComponentKindSimpleTypeDefinition, name: mustTestQName(t, "urn:one", "shared"), loc: mustTestLoc(t, "owner.xsd", 8, 3), simpleType: codegenDirectChoiceSimpleTypeInput(t, "owner.xsd", 8, mustTestQName(t, testXSDNamespace, "integer"))},
				{kind: ComponentKindSimpleTypeDefinition, name: mustTestQName(t, "urn:owner", "choice"), loc: mustTestLoc(t, "owner.xsd", 9, 3), simpleType: codegenDirectChoiceSimpleTypeInput(t, "owner.xsd", 9, mustTestQName(t, testXSDNamespace, "integer"))},
				{kind: ComponentKindSimpleTypeDefinition, name: mustTestQName(t, "urn:owner", "type"), loc: mustTestLoc(t, "owner.xsd", 10, 3), simpleType: codegenDirectChoiceSimpleTypeInput(t, "owner.xsd", 10, mustTestQName(t, testXSDNamespace, "integer"))},
				{kind: ComponentKindSimpleTypeDefinition, name: mustTestQName(t, "urn:owner", "TYPE"), loc: mustTestLoc(t, "owner.xsd", 11, 3), simpleType: codegenDirectChoiceSimpleTypeInput(t, "owner.xsd", 11, mustTestQName(t, testXSDNamespace, "decimal"))},
			},
		},
		{
			source:          "other.xsd",
			rootLoc:         mustTestLoc(t, "other.xsd", 1, 1),
			targetNamespace: "urn:other",
			declarations:    []schemaComponentInput{{kind: ComponentKindSimpleTypeDefinition, name: mustTestQName(t, "urn:other", "shared"), loc: mustTestLoc(t, "other.xsd", 2, 3), simpleType: codegenDirectChoiceSimpleTypeInput(t, "other.xsd", 2, mustTestQName(t, testXSDNamespace, "integer"))}},
		},
	})
}

func codegenDirectChoiceFailureSchema(t *testing.T) Schema {
	t.Helper()
	return mustTestSchema(t, []schemaDocumentInput{{
		source:  "choice.xsd",
		rootLoc: mustTestLoc(t, "choice.xsd", 1, 1),
		declarations: []schemaComponentInput{{
			kind: ComponentKindComplexTypeDefinition,
			name: mustTestQName(t, "urn:choice", "Choice"),
			loc:  mustTestLoc(t, "choice.xsd", 2, 3),
			complexType: &schemaComplexTypeInput{particle: &schemaChoiceParticleInput{
				loc:          mustTestLoc(t, "choice.xsd", 3, 5),
				minOccurs:    1,
				maxOccurs:    1,
				alternatives: []schemaElementParticleInput{codegenDirectChoiceElementInput(t, "value", mustTestQName(t, testXSDNamespace, "integer"), mustTestLoc(t, "choice.xsd", 4, 7))},
			}},
		}},
	}})
}

func codegenDirectChoiceElementInput(t *testing.T, local string, declaredType QName, loc Loc) schemaElementParticleInput {
	t.Helper()
	return schemaElementParticleInput{
		loc:  loc,
		name: mustTestQName(t, "", local),
		typeInput: &schemaElementInput{
			declaredType: declaredType,
			typeLoc:      loc,
		},
	}
}

func codegenDirectChoiceSimpleTypeInput(t *testing.T, source SourceID, line int, base QName) *schemaSimpleTypeInput {
	t.Helper()
	return &schemaSimpleTypeInput{
		base:    base,
		baseLoc: mustTestLoc(t, source, line, 27),
	}
}
