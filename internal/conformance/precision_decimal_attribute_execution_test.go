package conformance

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/goxdra/goxsd9"
)

// This replay preserves source and effective expectations as separate facts.
//
//nolint:gocognit // Every selected row retains source, effective, actual, and match evidence.
func TestAuxiliaryPrecisionDecimalAttributePacket(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "w3c", "xsdtests")
	inventory, err := ReadDirectory(root)
	if err != nil {
		t.Fatalf("ReadDirectory: %v", err)
	}
	key := InstanceKey{SetPath: precisionDecimalSaxonSetPath, GroupName: "pdecimal006", CaseName: "pdecimal006.n2.xml", Version: auxiliaryInstanceVersion11}
	policy := NewEffectiveExpectationPolicy([]EffectiveExpectationOverride{{Key: key, SourceValidity: "invalid", EffectiveValidity: "valid"}})
	plan, err := inventory.PlanAuxiliaryInstances(policy)
	if err != nil {
		t.Fatalf("PlanAuxiliaryInstances: %v", err)
	}
	selected := AuxiliaryInstancePlan{policy: plan.policy}
	for index, row := range plan.cases {
		if index < 46 || index == 51 {
			want := precisionDecimalAuxiliaryInstanceLedger[index]
			if row.SetPath() != want.setPath || row.GroupName() != want.groupName || row.InstancePath() != want.instancePath || row.SourceExpectedValidity() != want.outcome {
				t.Fatalf("selected row %d drifted from pinned ledger: %#v", index+1, row)
			}
			selected.cases = append(selected.cases, row)
		}
	}
	if len(selected.cases) != 47 {
		t.Fatalf("selected %d rows, want 47", len(selected.cases))
	}
	report, err := selected.Execute(context.Background(), os.DirFS(root))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var sourceValid, sourceInvalid, effectiveValid, effectiveInvalid int
	for index, row := range report.cases {
		if row.SourceExpectedValidity() == "valid" {
			sourceValid++
		}
		if row.SourceExpectedValidity() == "invalid" {
			sourceInvalid++
		}
		if row.EffectiveExpectedValidity() == "valid" {
			effectiveValid++
		}
		if row.EffectiveExpectedValidity() == "invalid" {
			effectiveInvalid++
		}
		if row.SchemaStage().Actual() != ActualValid || row.InstanceStage().Outcome() != OutcomePass || !row.EffectiveMatch() {
			t.Errorf("row %d %s: schema=%s instance=%s actual=%s class=%s diagnostic=%v", index+1, row.CaseName(), row.SchemaStage().Actual(), row.InstanceStage().Outcome(), row.InstanceStage().Actual(), row.InstanceStage().ActualClass(), row.InstanceStage().Cause())
		}
		if index != 26 && !row.SourceMatch() {
			t.Errorf("row %d lost source match: source=%s effective=%s actual=%s", index+1, row.SourceExpectedValidity(), row.EffectiveExpectedValidity(), row.InstanceStage().Actual())
		}
		if row.InstanceStage().Actual() != actualForExpected(row.EffectiveExpectedValidity()) {
			t.Errorf("row %d actual=%s, effective expectation=%s", index+1, row.InstanceStage().Actual(), row.EffectiveExpectedValidity())
		}
		if row.InstanceStage().Actual() == ActualInvalid {
			for _, diagnostic := range row.InstanceStage().Diagnostics() {
				if diagnostic.Code() == "" || diagnostic.Loc().IsZero() || diagnostic.Class() != goxsd9.FailureInvalid {
					t.Errorf("row %d diagnostic = %s, want located invalid code", index+1, diagnostic)
				}
			}
		}
		if index == 26 && (row.SourceMatch() || !row.EffectiveMatch() || row.InstanceStage().Actual() != ActualValid) {
			t.Errorf("row 27 source/effective match = %t/%t actual=%s", row.SourceMatch(), row.EffectiveMatch(), row.InstanceStage().Actual())
		}
	}
	if sourceValid != 12 || sourceInvalid != 35 || effectiveValid != 13 || effectiveInvalid != 34 {
		t.Fatalf("source %d/%d effective %d/%d, want 12/35 13/34", sourceValid, sourceInvalid, effectiveValid, effectiveInvalid)
	}
}
