package conformance

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/goxdra/goxsd9"
)

type orderedPrecisionInvalidEvidence struct {
	code   string
	line   int
	column int
}

func TestPinnedOrderedPrecisionDecimalInstanceReplay(t *testing.T) {
	invalidEvidence := []orderedPrecisionInvalidEvidence{
		{"XSD2010", 11, 13},
		{goxsd9.PrecisionDecimalFacetValueViolationCode, 8, 20},
		{goxsd9.PrecisionDecimalFacetValueViolationCode, 8, 17},
		{goxsd9.PrecisionDecimalFacetValueViolationCode, 7, 20},
		{goxsd9.PrecisionDecimalFacetValueViolationCode, 9, 16},
		{goxsd9.PrecisionDecimalFacetValueViolationCode, 7, 24},
		{goxsd9.PrecisionDecimalFacetValueViolationCode, 7, 24},
		{goxsd9.PrecisionDecimalFacetValueViolationCode, 8, 12},
	}
	root := filepath.Join("..", "..", "testdata", "w3c", "xsdtests")
	inventory, err := ReadDirectory(root)
	if err != nil {
		t.Fatalf("ReadDirectory: %v", err)
	}
	plan, err := inventory.PlanAuxiliaryInstances(NewEffectiveExpectationPolicy(nil))
	if err != nil {
		t.Fatalf("PlanAuxiliaryInstances: %v", err)
	}
	selectedPlan := selectOrderedPrecisionDecimalPlan(t, plan)
	resources := &schemaExecutionTrackingFS{base: os.DirFS(root)}
	report, err := selectedPlan.Execute(context.Background(), resources)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	assertOrderedPrecisionDecimalReplay(t, report, resources.opened, invalidEvidence)
}

func selectOrderedPrecisionDecimalPlan(t *testing.T, plan AuxiliaryInstancePlan) AuxiliaryInstancePlan {
	t.Helper()
	selectedPlan := AuxiliaryInstancePlan{policy: plan.Policy(), cases: make([]AuxiliaryInstanceCase, 0, 16)}
	for index, row := range precisionDecimalAuxiliaryInstanceLedger {
		if row.owner != 216 {
			continue
		}
		if index != 50 && (index < 54 || index > 68) {
			t.Fatalf("unexpected owned catalog row %d", index+1)
		}
		planned, ok := plan.Case(index)
		if !ok {
			t.Fatalf("plan row %d missing", index+1)
		}
		selectedPlan.cases = append(selectedPlan.cases, planned)
	}
	if selectedPlan.Len() != 16 || len(selectedPlan.Policy().Overrides()) != 0 {
		t.Fatalf("selected plan has %d rows and %d overrides, want 16 and none", selectedPlan.Len(), len(selectedPlan.Policy().Overrides()))
	}
	return selectedPlan
}

func assertOrderedPrecisionDecimalReplay(t *testing.T, report AuxiliaryInstanceReport, opened []string, invalidEvidence []orderedPrecisionInvalidEvidence) {
	t.Helper()
	if report.Len() != 16 || report.HeadlineCount() != 0 {
		t.Fatalf("report length/headline = %d/%d, want 16/0", report.Len(), report.HeadlineCount())
	}
	selected := 0
	valid := 0
	invalid := 0
	wantOpens := make([]string, 0, 32)
	for index, row := range precisionDecimalAuxiliaryInstanceLedger {
		if row.owner != 216 {
			continue
		}
		selected++
		wantOpens = append(wantOpens, row.schemaPath, row.instancePath)
		validIncrement, invalidIncrement := assertOrderedPrecisionDecimalReportCase(t, report, selected-1, index, row, invalid, invalidEvidence)
		valid += validIncrement
		invalid += invalidIncrement
	}
	if selected != 16 || valid != 8 || invalid != 8 {
		t.Fatalf("selected=%d valid=%d invalid=%d, want 16/8/8", selected, valid, invalid)
	}
	if !equalStrings(opened, wantOpens) {
		t.Fatalf("opened resources = %v, want only assigned schema/instance pairs %v", opened, wantOpens)
	}
}

func assertOrderedPrecisionDecimalReportCase(t *testing.T, report AuxiliaryInstanceReport, selectedIndex, ledgerIndex int, row precisionDecimalInstanceLedgerRow, invalidIndex int, invalidEvidence []orderedPrecisionInvalidEvidence) (int, int) {
	t.Helper()
	result, ok := report.Case(selectedIndex)
	if !ok {
		t.Fatalf("row %d missing", ledgerIndex+1)
	}
	if row.outcome == "invalid" {
		if invalidIndex >= len(invalidEvidence) {
			t.Fatalf("row %d invalid evidence missing", ledgerIndex+1)
		}
		assertOrderedPrecisionDecimalRow(t, ledgerIndex, row, result, invalidEvidence[invalidIndex])
		return 0, 1
	}
	if row.outcome != "valid" {
		t.Fatalf("row %d has unknown source outcome %q", ledgerIndex+1, row.outcome)
	}
	assertOrderedPrecisionDecimalRow(t, ledgerIndex, row, result, orderedPrecisionInvalidEvidence{})
	return 1, 0
}

func assertOrderedPrecisionDecimalRow(t *testing.T, index int, row precisionDecimalInstanceLedgerRow, result AuxiliaryInstanceResult, evidence orderedPrecisionInvalidEvidence) {
	t.Helper()
	stage := result.InstanceStage()
	if result.SetPath() != precisionDecimalIBMSetPath || result.GroupName() != row.groupName ||
		result.InstancePath() != row.instancePath || result.SchemaPath() != row.schemaPath ||
		result.Version() != "1.1" || result.Policy() != goxsd9.Strict11 ||
		result.Origin() != "auxiliary" || result.Status() != "accepted" || result.HeadlineEligible() {
		t.Errorf("row %d lost ordered catalog, policy, or provenance facts: %#v", index+1, result)
	}
	want := ActualValid
	if row.outcome == "invalid" {
		want = ActualInvalid
	}
	if result.SourceExpectedValidity() != row.outcome || result.EffectiveExpectedValidity() != row.outcome ||
		stage.Actual() != want || !result.SourceMatch() || !result.EffectiveMatch() ||
		stage.Outcome() != OutcomePass || result.SchemaStage().Actual() != ActualValid {
		t.Errorf("row %d %s: source=%s effective=%s actual=%s match=%t/%t stage=%s schema=%s diagnostics=%v cause=%v",
			index+1, row.instancePath, result.SourceExpectedValidity(), result.EffectiveExpectedValidity(), stage.Actual(), result.SourceMatch(), result.EffectiveMatch(), stage.Outcome(), result.SchemaStage().Actual(), stage.Diagnostics(), stage.Cause())
	}
	if want != ActualInvalid {
		if len(stage.Diagnostics()) != 0 || stage.Cause() != nil {
			t.Errorf("row %d valid stage has diagnostics=%v cause=%v", index+1, stage.Diagnostics(), stage.Cause())
		}
		return
	}
	assertOrderedPrecisionDecimalInvalid(t, index, row, result, evidence)
}

func assertOrderedPrecisionDecimalInvalid(t *testing.T, index int, row precisionDecimalInstanceLedgerRow, result AuxiliaryInstanceResult, evidence orderedPrecisionInvalidEvidence) {
	t.Helper()
	stage := result.InstanceStage()
	if stage.ActualClass() != goxsd9.FailureInvalid {
		t.Errorf("row %d class=%s, want invalid", index+1, stage.ActualClass())
	}
	diagnostics := stage.Diagnostics()
	if len(diagnostics) == 0 {
		t.Errorf("row %d invalid stage has no diagnostics", index+1)
		return
	}
	primary := diagnostics[0]
	if primary.Code() != evidence.code || primary.Class() != goxsd9.FailureInvalid ||
		primary.Loc().Source() != goxsd9.SourceID(row.instancePath) || primary.Loc().Line() != evidence.line || primary.Loc().Column() != evidence.column ||
		len(primary.Related()) == 0 || primary.Related()[0].Source() != goxsd9.SourceID(row.schemaPath) || primary.SpecRef() == "" {
		t.Errorf("row %d diagnostic = %v, want code=%s at %s:%d:%d with related schema and specification", index+1, primary, evidence.code, row.instancePath, evidence.line, evidence.column)
	}
	if stage.Cause() == nil || result.SchemaStage().Cause() != nil {
		t.Errorf("row %d stage causes = instance %v schema %v, want instance only", index+1, stage.Cause(), result.SchemaStage().Cause())
	}
}
