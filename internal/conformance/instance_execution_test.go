package conformance

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/goxdra/goxsd9"
)

//nolint:gocognit,funlen // Keep exact pinned row, expectation, and report assertions together.
func TestAuxiliaryInstancePlanAndReplayPinnedCorpus(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "w3c", "xsdtests")
	resources := &schemaExecutionTrackingFS{base: os.DirFS(root)}
	inventory, err := Read(resources)
	if err != nil {
		t.Fatalf("Read pinned catalog: %v", err)
	}
	resources.reset()
	inventoryBefore := inventoryOutput(t, inventory)

	overrideKey := InstanceKey{
		SetPath:   precisionDecimalSaxonSetPath,
		GroupName: "pdecimal006",
		CaseName:  "pdecimal006.n2.xml",
		Version:   auxiliaryInstanceVersion11,
	}
	overrides := []EffectiveExpectationOverride{{
		Key:               overrideKey,
		SourceValidity:    "invalid",
		EffectiveValidity: "valid",
	}}
	policy := NewEffectiveExpectationPolicy(overrides)
	overrides[0].EffectiveValidity = "invalid"
	if got := policy.EffectiveExpectation(overrideKey, "invalid"); got != "valid" {
		t.Fatalf("policy changed after source slice mutation: effective validity = %q, want valid", got)
	}
	returnedOverrides := policy.Overrides()
	returnedOverrides[0].EffectiveValidity = "invalid"
	if got := policy.EffectiveExpectation(overrideKey, "invalid"); got != "valid" {
		t.Fatalf("policy changed after Overrides mutation: effective validity = %q, want valid", got)
	}

	plan, err := inventory.PlanAuxiliaryInstances(policy)
	if err != nil {
		t.Fatalf("PlanAuxiliaryInstances: %v", err)
	}
	if len(resources.opened) != 0 {
		t.Fatalf("planning opened resources: %v", resources.opened)
	}
	if got, want := plan.Len(), 69; got != want {
		t.Fatalf("planned instance count = %d, want %d", got, want)
	}
	if got := plan.Policy().Overrides(); len(got) != 1 || got[0].Key != overrideKey ||
		got[0].SourceValidity != "invalid" || got[0].EffectiveValidity != "valid" {
		t.Fatalf("planned policy = %#v, want the one Saxon override", got)
	}

	planCase, ok := plan.Case(0)
	if !ok {
		t.Fatal("plan.Case(0) missing")
	}
	planSchemaPaths := planCase.SchemaPaths()
	planSchemaPaths[0] = "mutated.xsd"
	unchangedPlanCase, ok := plan.Case(0)
	if !ok || unchangedPlanCase.SchemaPath() == "mutated.xsd" {
		t.Fatal("plan case schema paths are not immutable copies")
	}

	saxonCount := 0
	ibmCount := 0
	sourceValidCount := 0
	sourceInvalidCount := 0
	effectiveValidCount := 0
	effectiveInvalidCount := 0
	for index, want := range precisionDecimalAuxiliaryInstanceLedger {
		planned, caseOK := plan.Case(index)
		if !caseOK {
			t.Fatalf("plan.Case(%d) missing", index)
		}
		if planned.SetPath() != want.setPath || planned.GroupName() != want.groupName ||
			planned.SchemaPath() != want.schemaPath || planned.InstancePath() != want.instancePath {
			t.Fatalf("plan case %d = %s/%s schema=%q instance=%q, want %s/%s schema=%q instance=%q",
				index+1, planned.SetPath(), planned.GroupName(), planned.SchemaPath(), planned.InstancePath(),
				want.setPath, want.groupName, want.schemaPath, want.instancePath)
		}
		if planned.Origin() != string(originAuxiliary) || planned.Status() != string(statusAccepted) ||
			planned.Version() != auxiliaryInstanceVersion11 || planned.Policy() != goxsd9.Strict11 {
			t.Fatalf("plan case %d provenance/version/policy = %q/%q/%q/%q, want auxiliary/accepted/1.1/Strict11",
				index+1, planned.Origin(), planned.Status(), planned.Version(), planned.Policy())
		}
		if planned.SourceExpectedValidity() != want.outcome {
			t.Fatalf("plan case %d source expected = %q, want %q", index+1, planned.SourceExpectedValidity(), want.outcome)
		}
		if planned.Key() != overrideKey && planned.EffectiveExpectedValidity() != planned.SourceExpectedValidity() {
			t.Fatalf("plan case %d non-overridden expectation = %q/%q, want source/effective equality",
				index+1, planned.SourceExpectedValidity(), planned.EffectiveExpectedValidity())
		}
		if planned.EffectiveExpectedValidity() == "valid" {
			effectiveValidCount++
		}
		if planned.EffectiveExpectedValidity() == "invalid" {
			effectiveInvalidCount++
		}
		if want.outcome == "valid" {
			sourceValidCount++
		}
		if want.outcome == "invalid" {
			sourceInvalidCount++
		}
		switch planned.SetPath() {
		case precisionDecimalSaxonSetPath:
			saxonCount++
		case precisionDecimalIBMSetPath:
			ibmCount++
		default:
			t.Fatalf("plan case %d has unexpected set path %q", index+1, planned.SetPath())
		}
	}
	if saxonCount != 50 || ibmCount != 19 {
		t.Fatalf("planned owners = %d Saxon, %d IBM; want 50, 19", saxonCount, ibmCount)
	}
	if sourceValidCount != 24 || sourceInvalidCount != 45 {
		t.Fatalf("planned source expectations = %d valid, %d invalid; want 24, 45", sourceValidCount, sourceInvalidCount)
	}
	if effectiveValidCount != 25 || effectiveInvalidCount != 44 {
		t.Fatalf("planned effective expectations = %d valid, %d invalid; want 25, 44", effectiveValidCount, effectiveInvalidCount)
	}

	overridden, ok := findAuxiliaryPlanCase(plan, overrideKey)
	if !ok || overridden.EffectiveExpectedValidity() != "valid" || overridden.SourceExpectedValidity() != "invalid" {
		t.Fatalf("Saxon pdecimal006.n2 effective result = %#v, want invalid -> valid", overridden)
	}
	ibmKey := InstanceKey{
		SetPath:   precisionDecimalIBMSetPath,
		GroupName: "d3_3_4v14",
		CaseName:  "d3_3_4v14i",
		Version:   auxiliaryInstanceVersion11,
	}
	ibmCase, ok := findAuxiliaryPlanCase(plan, ibmKey)
	if !ok || ibmCase.SourceExpectedValidity() != "valid" || ibmCase.EffectiveExpectedValidity() != "valid" {
		t.Fatalf("IBM d3_3_4v14 expectations = %#v, want valid/valid without override", ibmCase)
	}

	first, err := plan.Execute(context.Background(), resources)
	if err != nil {
		t.Fatalf("first auxiliary execution: %v", err)
	}
	if got, want := first.Len(), 69; got != want {
		t.Fatalf("first report case count = %d, want %d", got, want)
	}
	if got := first.HeadlineCount(); got != 0 {
		t.Fatalf("auxiliary headline count = %d, want 0", got)
	}
	assertAuxiliaryReplayStages(t, first)
	assertAuxiliaryInstanceOpenOrder(t, resources.opened, first)

	var firstOutput, repeatedOutput bytes.Buffer
	writeErr := first.Write(&firstOutput)
	if writeErr != nil {
		t.Fatalf("first auxiliary report Write: %v", writeErr)
	}
	writeErr = first.Write(&repeatedOutput)
	if writeErr != nil {
		t.Fatalf("repeated auxiliary report Write: %v", writeErr)
	}
	if firstOutput.String() != repeatedOutput.String() {
		t.Fatal("repeated auxiliary report writes differ")
	}

	resources.reset()
	second, err := plan.Execute(context.Background(), resources)
	if err != nil {
		t.Fatalf("repeated auxiliary execution: %v", err)
	}
	var secondOutput bytes.Buffer
	if err := second.Write(&secondOutput); err != nil {
		t.Fatalf("repeated auxiliary report Write: %v", err)
	}
	if firstOutput.String() != secondOutput.String() {
		t.Fatal("repeated auxiliary executions produced different reports")
	}
	assertAuxiliaryInstanceOpenOrder(t, resources.opened, second)

	inventoryAfter := inventoryOutput(t, inventory)
	if inventoryBefore != inventoryAfter {
		t.Fatal("auxiliary planning/execution changed the headline inventory")
	}
	assertOutputContains(t, inventoryAfter, []string{
		"auxiliary 1.1 instance 69 24 45 0 0 69 0 0 0 0 0 0 0\n",
	})
}

//nolint:gocognit // Keep the catalog provenance filter matrix together.
func TestAuxiliaryInstancePlanFiltersNonReplayProvenance(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "w3c", "xsdtests")
	inventory, err := ReadDirectory(root)
	if err != nil {
		t.Fatalf("ReadDirectory: %v", err)
	}
	key := InstanceKey{
		SetPath:   precisionDecimalSaxonSetPath,
		GroupName: "pdecimal006",
		CaseName:  "pdecimal006.n2.xml",
		Version:   auxiliaryInstanceVersion11,
	}
	tests := []struct {
		name   string
		mutate func(*catalogCase)
	}{
		{name: "non-auxiliary", mutate: func(caseValue *catalogCase) { caseValue.origin = originMain }},
		{name: "queried", mutate: func(caseValue *catalogCase) { caseValue.status = statusQueried }},
		{name: "submitted", mutate: func(caseValue *catalogCase) { caseValue.status = statusSubmitted }},
		{name: "disputed", mutate: func(caseValue *catalogCase) { caseValue.status = statusDisputedTest }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mutated := cloneAuxiliaryTestInventory(inventory)
			for index := range mutated.cases {
				caseValue := &mutated.cases[index]
				if caseValue.kind != instanceKind || caseValue.setPath != key.SetPath ||
					caseValue.groupName != key.GroupName || caseValue.name != key.CaseName {
					continue
				}
				test.mutate(caseValue)
				break
			}
			plan, err := mutated.PlanAuxiliaryInstances(NewEffectiveExpectationPolicy(nil))
			if err != nil {
				t.Fatalf("PlanAuxiliaryInstances: %v", err)
			}
			if got, want := plan.Len(), 68; got != want {
				t.Fatalf("filtered plan count = %d, want %d", got, want)
			}
			if _, ok := findAuxiliaryPlanCase(plan, key); ok {
				t.Fatalf("filtered %s row remained in plan", test.name)
			}
		})
	}
}

//nolint:gocognit,funlen // Keep the complete fail-before-execute validation matrix together.
func TestAuxiliaryInstancePlanRejectsInvalidReplayInputsBeforeExecution(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "w3c", "xsdtests")
	resources := &schemaExecutionTrackingFS{base: os.DirFS(root)}
	inventory, err := Read(resources)
	if err != nil {
		t.Fatalf("Read pinned catalog: %v", err)
	}
	resources.reset()

	key := InstanceKey{
		SetPath:   precisionDecimalSaxonSetPath,
		GroupName: "pdecimal006",
		CaseName:  "pdecimal006.n2.xml",
		Version:   auxiliaryInstanceVersion11,
	}
	validOverride := EffectiveExpectationOverride{
		Key:               key,
		SourceValidity:    "invalid",
		EffectiveValidity: "valid",
	}
	withInstanceMutation := func(mutate func(*catalogCase)) Inventory {
		cloned := cloneAuxiliaryTestInventory(inventory)
		for index := range cloned.cases {
			caseValue := &cloned.cases[index]
			if caseValue.kind != instanceKind || caseValue.setPath != key.SetPath ||
				caseValue.groupName != key.GroupName || caseValue.name != key.CaseName {
				continue
			}
			mutate(caseValue)
			break
		}
		return cloned
	}
	withSchemaMutation := func(mutate func(*catalogCase)) Inventory {
		cloned := cloneAuxiliaryTestInventory(inventory)
		for index := range cloned.cases {
			caseValue := &cloned.cases[index]
			if caseValue.kind != schemaKind || caseValue.setPath != key.SetPath ||
				caseValue.groupName != key.GroupName {
				continue
			}
			mutate(caseValue)
			break
		}
		return cloned
	}
	missingSchema := cloneAuxiliaryTestInventory(inventory)
	filteredCases := make([]catalogCase, 0, len(missingSchema.cases)-1)
	for _, caseValue := range missingSchema.cases {
		if caseValue.kind == schemaKind && caseValue.setPath == key.SetPath && caseValue.groupName == key.GroupName {
			continue
		}
		filteredCases = append(filteredCases, caseValue)
	}
	missingSchema.cases = filteredCases

	tests := []struct {
		name      string
		inventory Inventory
		policy    EffectiveExpectationPolicy
	}{
		{
			name:      "malformed effective validity",
			inventory: inventory,
			policy: NewEffectiveExpectationPolicy([]EffectiveExpectationOverride{{
				Key: key, SourceValidity: "invalid", EffectiveValidity: "maybe",
			}}),
		},
		{
			name:      "malformed key",
			inventory: inventory,
			policy: NewEffectiveExpectationPolicy([]EffectiveExpectationOverride{{
				Key:            InstanceKey{GroupName: key.GroupName, CaseName: key.CaseName, Version: key.Version},
				SourceValidity: "invalid", EffectiveValidity: "valid",
			}}),
		},
		{
			name:      "duplicate key",
			inventory: inventory,
			policy:    NewEffectiveExpectationPolicy([]EffectiveExpectationOverride{validOverride, validOverride}),
		},
		{
			name:      "unknown key",
			inventory: inventory,
			policy: NewEffectiveExpectationPolicy([]EffectiveExpectationOverride{{
				Key:            InstanceKey{SetPath: key.SetPath, GroupName: "unknown", CaseName: "unknown.xml", Version: key.Version},
				SourceValidity: "invalid", EffectiveValidity: "valid",
			}}),
		},
		{
			name:      "source validity drift",
			inventory: inventory,
			policy: NewEffectiveExpectationPolicy([]EffectiveExpectationOverride{{
				Key: key, SourceValidity: "valid", EffectiveValidity: "valid",
			}}),
		},
		{
			name:      "wrong XSD version",
			inventory: inventory,
			policy: NewEffectiveExpectationPolicy([]EffectiveExpectationOverride{{
				Key:            InstanceKey{SetPath: key.SetPath, GroupName: key.GroupName, CaseName: key.CaseName, Version: auxiliaryInstanceVersion10},
				SourceValidity: "invalid", EffectiveValidity: "valid",
			}}),
		},
		{
			name: "non-auxiliary provenance",
			inventory: withInstanceMutation(func(caseValue *catalogCase) {
				caseValue.origin = originMain
			}),
			policy: NewEffectiveExpectationPolicy([]EffectiveExpectationOverride{validOverride}),
		},
		{
			name: "queried provenance",
			inventory: withInstanceMutation(func(caseValue *catalogCase) {
				caseValue.status = statusQueried
			}),
			policy: NewEffectiveExpectationPolicy([]EffectiveExpectationOverride{validOverride}),
		},
		{
			name: "unaccepted provenance",
			inventory: withInstanceMutation(func(caseValue *catalogCase) {
				caseValue.status = statusSubmitted
			}),
			policy: NewEffectiveExpectationPolicy([]EffectiveExpectationOverride{validOverride}),
		},
		{
			name:      "missing paired schema",
			inventory: missingSchema,
			policy:    NewEffectiveExpectationPolicy(nil),
		},
		{
			name: "non-valid paired schema",
			inventory: withSchemaMutation(func(caseValue *catalogCase) {
				caseValue.expectations = []expectation{{validity: "invalid"}}
			}),
			policy: NewEffectiveExpectationPolicy(nil),
		},
		{
			name: "paired schema wrong XSD version",
			inventory: withSchemaMutation(func(caseValue *catalogCase) {
				caseValue.parentVersions = []string{auxiliaryInstanceVersion10}
			}),
			policy: NewEffectiveExpectationPolicy(nil),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resources.reset()
			plan, err := test.inventory.PlanAuxiliaryInstances(test.policy)
			if err == nil {
				t.Fatal("PlanAuxiliaryInstances accepted invalid replay input")
			}
			if plan.Len() != 0 || plan.cases != nil {
				t.Fatalf("invalid replay produced a partial plan: %#v", plan)
			}
			var catalogErr *CatalogError
			if !errors.As(err, &catalogErr) || catalogErr.Code != instancePlanErrorCode {
				t.Fatalf("plan error = %v, want CatalogError code %q", err, instancePlanErrorCode)
			}

			report, executeErr := plan.Execute(context.Background(), resources)
			if executeErr == nil {
				t.Fatal("empty invalid plan unexpectedly executed")
			}
			if report.Len() != 0 || report.cases != nil {
				t.Fatalf("invalid replay produced a partial report: %#v", report)
			}
			if len(resources.opened) != 0 {
				t.Fatalf("invalid replay opened execution resources: %v", resources.opened)
			}
		})
	}
}

//nolint:gocognit,funlen // Keep stage, order, diagnostic, and deterministic-output assertions together.
func TestAuxiliaryInstanceReplaySeparatesStagesAndExecutesSequentially(t *testing.T) {
	inventory := auxiliaryReplayFixtureInventory()
	overrideKey := InstanceKey{
		SetPath:   "fixture.testSet",
		GroupName: "good",
		CaseName:  "override.xml",
		Version:   auxiliaryInstanceVersion11,
	}
	policy := NewEffectiveExpectationPolicy([]EffectiveExpectationOverride{{
		Key:               overrideKey,
		SourceValidity:    "invalid",
		EffectiveValidity: "valid",
	}})
	plan, err := inventory.PlanAuxiliaryInstances(policy)
	if err != nil {
		t.Fatalf("PlanAuxiliaryInstances fixture: %v", err)
	}
	if got, want := plan.Len(), 3; got != want {
		t.Fatalf("fixture plan count = %d, want %d", got, want)
	}

	resources := &auxiliaryReplayTrackingFS{files: fstest.MapFS{
		"good.xsd":       &fstest.MapFile{Data: []byte(auxiliaryReplayGoodSchema)},
		"bad.xml":        &fstest.MapFile{Data: []byte(`<root>not-an-integer</root>`)},
		"override.xml":   &fstest.MapFile{Data: []byte(`<root>42</root>`)},
		"bad-schema.xsd": &fstest.MapFile{Data: []byte(`<not-schema/>`)},
		"skipped.xml":    &fstest.MapFile{Data: []byte(`<root>42</root>`)},
	}}
	report, err := plan.Execute(context.Background(), resources)
	if err != nil {
		t.Fatalf("Execute fixture auxiliary plan: %v", err)
	}
	if got, want := resources.opened, []string{"good.xsd", "bad.xml", "good.xsd", "override.xml", "bad-schema.xsd"}; !equalStrings(got, want) {
		t.Fatalf("sequential resource order = %v, want %v", got, want)
	}

	results := report.Cases()
	if len(results) != 3 {
		t.Fatalf("fixture report cases = %d, want 3", len(results))
	}
	first := results[0]
	if first.SchemaStage().Actual() != ActualValid || first.SchemaStage().Outcome() != OutcomePass {
		t.Fatalf("invalid-instance schema stage = %q/%q, want valid/pass", first.SchemaStage().Actual(), first.SchemaStage().Outcome())
	}
	if first.InstanceStage().Actual() != ActualInvalid || first.InstanceStage().ActualClass() != goxsd9.FailureInvalid ||
		first.InstanceStage().Outcome() != OutcomePass || !first.SourceMatch() || !first.EffectiveMatch() {
		t.Fatalf("invalid-instance result = actual %q class %q outcome %q source=%t effective=%t, want invalid/invalid/pass/true/true",
			first.InstanceStage().Actual(), first.InstanceStage().ActualClass(), first.InstanceStage().Outcome(), first.SourceMatch(), first.EffectiveMatch())
	}
	firstDiagnostics := first.InstanceStage().Diagnostics()
	if len(firstDiagnostics) == 0 || firstDiagnostics[0].Code() != goxsd9.InvalidIntegerLexicalCode ||
		firstDiagnostics[0].Loc().Source() != goxsd9.SourceID("bad.xml") {
		t.Fatalf("invalid-instance diagnostics = %#v, want located %s", firstDiagnostics, goxsd9.InvalidIntegerLexicalCode)
	}
	if first.InstanceStage().Cause() == nil {
		t.Fatal("invalid-instance stage discarded its diagnostic cause")
	}

	second := results[1]
	if second.InstancePath() != "override.xml" || second.SourceExpectedValidity() != "invalid" ||
		second.EffectiveExpectedValidity() != "valid" || second.InstanceStage().Actual() != ActualValid ||
		second.InstanceStage().Outcome() != OutcomePass || second.SourceMatch() || !second.EffectiveMatch() {
		t.Fatalf("effective override result = path %q source=%q effective=%q actual=%q outcome=%q source=%t effective-match=%t",
			second.InstancePath(), second.SourceExpectedValidity(), second.EffectiveExpectedValidity(), second.InstanceStage().Actual(),
			second.InstanceStage().Outcome(), second.SourceMatch(), second.EffectiveMatch())
	}
	if second.InstanceStage().Cause() != nil || len(second.InstanceStage().Diagnostics()) != 0 {
		t.Fatal("valid overridden instance retained an unexpected failure")
	}

	third := results[2]
	if third.SchemaStage().Actual() != ActualInvalid || third.SchemaStage().ActualClass() != goxsd9.FailureInvalid ||
		third.SchemaStage().Outcome() != OutcomeConformanceFailure || third.SchemaStage().Cause() == nil {
		t.Fatalf("schema-failure stage = actual %q class %q outcome %q cause=%v, want invalid/invalid/conformance-failure/cause",
			third.SchemaStage().Actual(), third.SchemaStage().ActualClass(), third.SchemaStage().Outcome(), third.SchemaStage().Cause())
	}
	if diagnostics := third.SchemaStage().Diagnostics(); len(diagnostics) == 0 ||
		diagnostics[0].Loc().Source() != goxsd9.SourceID("bad-schema.xsd") {
		t.Fatalf("schema-failure diagnostics = %#v, want bad-schema.xsd location", diagnostics)
	}
	if third.InstanceStage().Executed() || third.InstanceStage().Actual() != ActualNotExecuted ||
		third.InstanceStage().Outcome() != OutcomeNotExecuted || third.InstanceStage().ExecutionReason() != "schema-stage-failed" ||
		third.SourceMatch() || third.EffectiveMatch() {
		t.Fatalf("schema-failure instance stage = %#v, want not-executed with stable reason", third.InstanceStage())
	}
	if report.HeadlineCount() != 0 {
		t.Fatalf("fixture auxiliary headline count = %d, want 0", report.HeadlineCount())
	}

	var firstOutput, secondOutput bytes.Buffer
	if err := report.Write(&firstOutput); err != nil {
		t.Fatalf("fixture report Write: %v", err)
	}
	if err := report.Write(&secondOutput); err != nil {
		t.Fatalf("fixture repeated report Write: %v", err)
	}
	if firstOutput.String() != secondOutput.String() {
		t.Fatal("fixture repeated report writes differ")
	}
	for _, want := range []string{"schema-diagnostic", "instance-diagnostic", "source-match=true", "effective-match=true", "bad-schema.xsd", "bad.xml"} {
		if !strings.Contains(firstOutput.String(), want) {
			t.Fatalf("fixture report missing %q:\n%s", want, firstOutput.String())
		}
	}
}

func findAuxiliaryPlanCase(plan AuxiliaryInstancePlan, key InstanceKey) (AuxiliaryInstanceCase, bool) {
	for index := 0; index < plan.Len(); index++ {
		caseValue, ok := plan.Case(index)
		if !ok || caseValue.Key() != key {
			continue
		}
		return caseValue, true
	}
	return AuxiliaryInstanceCase{}, false
}

//nolint:gocognit // Keep cross-stage invariants and expectation comparisons together.
func assertAuxiliaryReplayStages(t *testing.T, report AuxiliaryInstanceReport) {
	t.Helper()
	for index := 0; index < report.Len(); index++ {
		result, ok := report.Case(index)
		if !ok {
			t.Fatalf("report.Case(%d) missing", index)
		}
		schemaStage := result.SchemaStage()
		instanceStage := result.InstanceStage()
		if schemaStage.Actual() == ActualValid {
			if !instanceStage.Executed() {
				t.Fatalf("report case %d skipped instance after valid schema", index+1)
			}
			if instanceStage.Actual() != ActualValid && instanceStage.Actual() != ActualInvalid && instanceStage.Actual() != ActualUnknown {
				t.Fatalf("report case %d has unknown executed instance actual %q", index+1, instanceStage.Actual())
			}
		}
		if schemaStage.Actual() != ActualValid && instanceStage.Executed() {
			t.Fatalf("report case %d executed instance after schema actual %q", index+1, schemaStage.Actual())
		}
		if schemaStage.Actual() == ActualValid && schemaStage.Cause() != nil {
			t.Fatalf("report case %d valid schema retained cause %v", index+1, schemaStage.Cause())
		}
		if instanceStage.Actual() == ActualValid && instanceStage.Cause() != nil {
			t.Fatalf("report case %d valid instance retained cause %v", index+1, instanceStage.Cause())
		}
		if schemaStage.Actual() != ActualValid && schemaStage.Cause() == nil {
			t.Fatalf("report case %d schema failure discarded its cause", index+1)
		}
		if instanceStage.Actual() == ActualInvalid && instanceStage.ActualClass() != goxsd9.FailureInvalid {
			t.Fatalf("report case %d invalid instance class = %q, want invalid", index+1, instanceStage.ActualClass())
		}
		if instanceStage.Actual() == ActualUnknown && instanceStage.ActualClass() == "" {
			t.Fatalf("report case %d unknown instance has no failure class", index+1)
		}
		wantSourceMatch := validityMatches(result.SourceExpectedValidity(), instanceStage.Actual())
		wantEffectiveMatch := validityMatches(result.EffectiveExpectedValidity(), instanceStage.Actual())
		if result.SourceMatch() != wantSourceMatch || result.EffectiveMatch() != wantEffectiveMatch {
			t.Fatalf("report case %d expectation matches = %t/%t, want %t/%t", index+1,
				result.SourceMatch(), result.EffectiveMatch(), wantSourceMatch, wantEffectiveMatch)
		}
	}
}

func assertAuxiliaryInstanceOpenOrder(t *testing.T, opened []string, report AuxiliaryInstanceReport) {
	t.Helper()
	want := make([]string, 0, report.Len())
	knownInstances := make(map[string]bool, report.Len())
	for index := 0; index < report.Len(); index++ {
		result, ok := report.Case(index)
		if !ok {
			t.Fatalf("report.Case(%d) missing while checking opens", index)
		}
		knownInstances[result.InstancePath()] = true
		if result.SchemaStage().Actual() == ActualValid {
			want = append(want, result.InstancePath())
		}
	}
	got := make([]string, 0, len(want))
	for _, path := range opened {
		if knownInstances[path] {
			got = append(got, path)
		}
	}
	if !equalStrings(got, want) {
		t.Fatalf("opened instance order = %v, want %v; all opens=%v", got, want, opened)
	}
}

func cloneAuxiliaryTestInventory(inventory Inventory) Inventory {
	cloned := Inventory{
		setPaths: append([]string(nil), inventory.setPaths...),
		cases:    make([]catalogCase, 0, len(inventory.cases)),
	}
	for _, caseValue := range inventory.cases {
		cloned.cases = append(cloned.cases, cloneCatalogCase(caseValue))
	}
	return cloned
}

func auxiliaryReplayFixtureInventory() Inventory {
	return Inventory{
		setPaths: []string{"fixture.testSet"},
		cases: []catalogCase{
			{
				setPath: "fixture.testSet", setName: "fixture", groupName: "good", name: "good.xsd",
				kind: schemaKind, origin: originAuxiliary, parentVersions: []string{auxiliaryInstanceVersion11},
				documents: []string{"good.xsd"}, expectations: []expectation{{validity: "valid"}}, status: statusAccepted,
			},
			{
				setPath: "fixture.testSet", setName: "fixture", groupName: "good", name: "invalid.xml",
				kind: instanceKind, origin: originAuxiliary, parentVersions: []string{auxiliaryInstanceVersion11},
				documents: []string{"bad.xml"}, expectations: []expectation{{validity: "invalid"}}, status: statusAccepted,
			},
			{
				setPath: "fixture.testSet", setName: "fixture", groupName: "good", name: "override.xml",
				kind: instanceKind, origin: originAuxiliary, parentVersions: []string{auxiliaryInstanceVersion11},
				documents: []string{"override.xml"}, expectations: []expectation{{validity: "invalid"}}, status: statusAccepted,
			},
			{
				setPath: "fixture.testSet", setName: "fixture", groupName: "bad", name: "bad-schema.xsd",
				kind: schemaKind, origin: originAuxiliary, parentVersions: []string{auxiliaryInstanceVersion11},
				documents: []string{"bad-schema.xsd"}, expectations: []expectation{{validity: "valid"}}, status: statusAccepted,
			},
			{
				setPath: "fixture.testSet", setName: "fixture", groupName: "bad", name: "skipped.xml",
				kind: instanceKind, origin: originAuxiliary, parentVersions: []string{auxiliaryInstanceVersion11},
				documents: []string{"skipped.xml"}, expectations: []expectation{{validity: "invalid"}}, status: statusAccepted,
			},
		},
	}
}

const auxiliaryReplayGoodSchema = `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema">
  <xs:element name="root" type="xs:integer"/>
</xs:schema>`

type auxiliaryReplayTrackingFS struct {
	files  fstest.MapFS
	opened []string
}

func (fsys *auxiliaryReplayTrackingFS) Open(name string) (fs.File, error) {
	fsys.opened = append(fsys.opened, name)
	return fsys.files.Open(name)
}

func (fsys *auxiliaryReplayTrackingFS) Stat(name string) (fs.FileInfo, error) {
	return fs.Stat(fsys.files, name)
}
