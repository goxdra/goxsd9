package conformance

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/goxdra/goxsd9"
)

//nolint:gocognit // Assert every assigned catalog row's independent replay facts and located outcome.
func TestPinnedOrderedPrecisionDecimalInstanceReplay(t *testing.T) {
	invalidEvidence := []struct {
		code   string
		line   int
		column int
	}{
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
	report, err := plan.Execute(context.Background(), os.DirFS(root))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	selected := 0
	valid := 0
	invalid := 0
	for index, row := range precisionDecimalAuxiliaryInstanceLedger {
		if row.owner != 216 {
			continue
		}
		selected++
		result, ok := report.Case(index)
		if !ok {
			t.Fatalf("row %d missing", index+1)
		}
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
			invalid++
		}
		if row.outcome == "valid" {
			valid++
		}
		if result.SourceExpectedValidity() != row.outcome || result.EffectiveExpectedValidity() != row.outcome ||
			stage.Actual() != want || !result.SourceMatch() || !result.EffectiveMatch() ||
			stage.Outcome() != OutcomePass || result.SchemaStage().Actual() != ActualValid {
			t.Errorf("row %d %s: source=%s effective=%s actual=%s match=%t/%t stage=%s schema=%s diagnostics=%v cause=%v",
				index+1, row.instancePath, result.SourceExpectedValidity(), result.EffectiveExpectedValidity(), stage.Actual(), result.SourceMatch(), result.EffectiveMatch(), stage.Outcome(), result.SchemaStage().Actual(), stage.Diagnostics(), stage.Cause())
		}
		if want == ActualInvalid && stage.ActualClass() != goxsd9.FailureInvalid {
			t.Errorf("row %d class=%s, want invalid", index+1, stage.ActualClass())
		}
		if want != ActualInvalid {
			if len(stage.Diagnostics()) != 0 || stage.Cause() != nil {
				t.Errorf("row %d valid stage has diagnostics=%v cause=%v", index+1, stage.Diagnostics(), stage.Cause())
			}
			continue
		}
		evidence := invalidEvidence[invalid-1]
		diagnostics := stage.Diagnostics()
		if len(diagnostics) == 0 {
			t.Errorf("row %d invalid stage has no diagnostics", index+1)
			continue
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
	if selected != 16 || valid != 8 || invalid != 8 {
		t.Fatalf("selected=%d valid=%d invalid=%d, want 16/8/8", selected, valid, invalid)
	}
}
