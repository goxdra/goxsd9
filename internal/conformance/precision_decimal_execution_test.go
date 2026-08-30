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

	"github.com/goxdra/goxsd9"
)

type precisionDecimalSchemaCase struct {
	setPath   string
	setName   string
	groupName string
	caseName  string
	version   string
	document  string
	expected  string
}

func TestPinnedAuxiliaryPrecisionDecimalSchemaExecutionPlan(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "w3c", "xsdtests")
	resources := &schemaExecutionTrackingFS{base: os.DirFS(root)}
	inventory, err := Read(resources)
	if err != nil {
		t.Fatalf("Read pinned catalog: %v", err)
	}
	resources.reset()

	plan, err := inventory.Plan([]Selector{
		{Version: "1.1", SetPath: precisionDecimalSaxonSetPath},
		{Version: "1.0", SetPath: precisionDecimalSaxonSetPath},
		{Version: "1.1", SetPath: precisionDecimalIBMSetPath},
	})
	if err != nil {
		t.Fatalf("Plan pinned precisionDecimal schemas: %v", err)
	}
	if got, want := plan.Len(), len(pinnedPrecisionDecimalSchemaCases); got != want {
		t.Fatalf("planned schema case count = %d, want %d", got, want)
	}
	if len(resources.opened) != 0 {
		t.Fatalf("Plan opened sources: %v", resources.opened)
	}

	requirements := ExecutionRequirements{AllowedOutcomes: []Outcome{OutcomePass, OutcomeUnsupported}}
	first, err := plan.ExecuteWithRequirements(context.Background(), resources, requirements)
	if err != nil {
		t.Fatalf("Execute pinned precisionDecimal schemas: %v", err)
	}
	assertPinnedPrecisionDecimalSchemaResults(t, first)
	assertNoInstanceResourcesOpened(t, resources.opened)

	var firstOutput, repeatedOutput bytes.Buffer
	writeErr := first.Write(&firstOutput)
	if writeErr != nil {
		t.Fatalf("first Write: %v", writeErr)
	}
	writeErr = first.Write(&repeatedOutput)
	if writeErr != nil {
		t.Fatalf("repeated Write: %v", writeErr)
	}
	if firstOutput.String() != repeatedOutput.String() {
		t.Fatalf("repeated report writes differ")
	}

	resources.reset()
	second, err := plan.ExecuteWithRequirements(context.Background(), resources, requirements)
	if err != nil {
		t.Fatalf("repeat Execute pinned precisionDecimal schemas: %v", err)
	}
	var secondOutput bytes.Buffer
	if err := second.Write(&secondOutput); err != nil {
		t.Fatalf("second Write: %v", err)
	}
	if firstOutput.String() != secondOutput.String() {
		t.Fatalf("repeated executions produced different reports")
	}
	assertNoInstanceResourcesOpened(t, resources.opened)
}

func assertNoInstanceResourcesOpened(t *testing.T, opened []string) {
	t.Helper()
	for _, path := range opened {
		if strings.HasSuffix(path, ".xml") {
			t.Fatalf("instance resource was opened during schema execution: %q", path)
		}
	}
}

//nolint:gocognit,funlen // Keep exact catalog facts, execution classifications, and counts together.
func assertPinnedPrecisionDecimalSchemaResults(t *testing.T, report PlanReport) {
	t.Helper()
	if got, want := report.Len(), len(pinnedPrecisionDecimalSchemaCases); got != want {
		t.Fatalf("report case count = %d, want %d", got, want)
	}
	if got := report.HeadlineCount(); got != 0 {
		t.Fatalf("headline count = %d, want 0", got)
	}

	saxonCases := 0
	ibmCases := 0
	saxonValid := 0
	saxonInvalid := 0
	ibmValid := 0
	ibmInvalid := 0
	passCount := 0
	unsupportedCount := 0
	for index, want := range pinnedPrecisionDecimalSchemaCases {
		result, ok := report.Case(index)
		if !ok {
			t.Fatalf("Case(%d) missing", index)
		}
		if result.SetPath() != want.setPath || result.SetName() != want.setName ||
			result.GroupName() != want.groupName || result.CaseName() != want.caseName ||
			result.Version() != want.version || result.Policy() != precisionDecimalPolicy(want.version) {
			t.Fatalf("case %d identity = %q/%q/%q/%q version=%q policy=%q, want %q/%q/%q/%q version=%q policy=%q",
				index+1, result.SetPath(), result.SetName(), result.GroupName(), result.CaseName(), result.Version(), result.Policy(),
				want.setPath, want.setName, want.groupName, want.caseName, want.version, precisionDecimalPolicy(want.version))
		}
		if got, expected := result.Documents(), []string{want.document}; !equalStrings(got, expected) {
			t.Fatalf("case %d documents = %v, want %v", index+1, got, expected)
		}
		if result.Origin() != string(originAuxiliary) || result.Status() != string(statusAccepted) || !result.Usable() || result.HeadlineEligible() {
			t.Fatalf("case %d catalog facts = origin %q status %q usable %t headline %t, want auxiliary accepted true false",
				index+1, result.Origin(), result.Status(), result.Usable(), result.HeadlineEligible())
		}
		if result.ExpectedValidity() != want.expected || !equalStrings(result.ExpectedValidities(), []string{want.expected}) {
			t.Fatalf("case %d expected validity = %q/%v, want %q/[%s]", index+1,
				result.ExpectedValidity(), result.ExpectedValidities(), want.expected, want.expected)
		}

		switch want.setPath {
		case precisionDecimalSaxonSetPath:
			saxonCases++
			if want.expected == "valid" {
				saxonValid++
			}
			if want.expected == "invalid" {
				saxonInvalid++
			}
		case precisionDecimalIBMSetPath:
			ibmCases++
			if want.expected == "valid" {
				ibmValid++
			}
			if want.expected == "invalid" {
				ibmInvalid++
			}
		default:
			t.Fatalf("case %d has unexpected set path %q", index+1, want.setPath)
		}

		switch result.Outcome() {
		case OutcomePass:
			passCount++
			if result.Actual() != actualForExpected(want.expected) {
				t.Fatalf("case %d pass actual = %q, want %q", index+1, result.Actual(), actualForExpected(want.expected))
			}
			if result.Actual() == ActualInvalid && result.ActualClass() != goxsd9.FailureInvalid {
				t.Fatalf("case %d invalid pass class = %q, want invalid", index+1, result.ActualClass())
			}
			if result.Actual() == ActualValid && result.ActualClass() != "" {
				t.Fatalf("case %d valid pass class = %q, want empty", index+1, result.ActualClass())
			}
		case OutcomeUnsupported:
			unsupportedCount++
			if result.Actual() != ActualUnknown || result.ActualClass() != goxsd9.FailureUnsupported || !errors.Is(result.Cause(), goxsd9.ErrUnsupported) {
				t.Fatalf("case %d unsupported facts = actual %q class %q cause %v, want unknown/unsupported/ErrUnsupported",
					index+1, result.Actual(), result.ActualClass(), result.Cause())
			}
		case OutcomeConformanceFailure, OutcomeResolutionFailure, OutcomeInternalFailure, OutcomeNotExecuted:
			t.Fatalf("case %d has required failure outcome %q, actual=%q class=%q cause=%v",
				index+1, result.Outcome(), result.Actual(), result.ActualClass(), result.Cause())
		default:
			t.Fatalf("case %d has unknown outcome %q", index+1, result.Outcome())
		}
	}

	if saxonCases != 21 || saxonValid != 12 || saxonInvalid != 9 {
		t.Fatalf("Saxon catalog facts = %d cases, %d valid, %d invalid; want 21, 12, 9", saxonCases, saxonValid, saxonInvalid)
	}
	if ibmCases != 33 || ibmValid != 13 || ibmInvalid != 20 {
		t.Fatalf("IBM catalog facts = %d cases, %d valid, %d invalid; want 33, 13, 20", ibmCases, ibmValid, ibmInvalid)
	}
	if passCount != 1 || unsupportedCount != 53 {
		t.Fatalf("pinned execution outcomes = %d pass, %d unsupported; want 1, 53", passCount, unsupportedCount)
	}
}

func precisionDecimalPolicy(version string) goxsd9.LanguagePolicy {
	if version == "1.0" {
		return goxsd9.Strict10
	}
	return goxsd9.Strict11
}

func actualForExpected(expected string) ActualResult {
	if expected == "invalid" {
		return ActualInvalid
	}
	return ActualValid
}

type schemaExecutionTrackingFS struct {
	base   fs.FS
	opened []string
}

func (fsys *schemaExecutionTrackingFS) Open(name string) (fs.File, error) {
	fsys.opened = append(fsys.opened, name)
	return fsys.base.Open(name)
}

func (fsys *schemaExecutionTrackingFS) Stat(name string) (fs.FileInfo, error) {
	return fs.Stat(fsys.base, name)
}

func (fsys *schemaExecutionTrackingFS) reset() {
	fsys.opened = nil
}

var pinnedPrecisionDecimalSchemaCases = []precisionDecimalSchemaCase{
	{precisionDecimalSaxonSetPath, "PDecimal", "pdecimal001", "pdecimal001.xsd", "1.1", "saxonData/PDecimal/pdecimal001.xsd", "valid"},
	{precisionDecimalSaxonSetPath, "PDecimal", "pdecimal001a", "pdecimal001.xsd", "1.0", "saxonData/PDecimal/pdecimal001.xsd", "invalid"},
	{precisionDecimalSaxonSetPath, "PDecimal", "pdecimal002", "pdecimal002.xsd", "1.1", "saxonData/PDecimal/pdecimal002.xsd", "valid"},
	{precisionDecimalSaxonSetPath, "PDecimal", "pdecimal003", "pdecimal003.xsd", "1.1", "saxonData/PDecimal/pdecimal003.xsd", "valid"},
	{precisionDecimalSaxonSetPath, "PDecimal", "pdecimal004", "pdecimal004.xsd", "1.1", "saxonData/PDecimal/pdecimal004.xsd", "valid"},
	{precisionDecimalSaxonSetPath, "PDecimal", "pdecimal005", "pdecimal005.xsd", "1.1", "saxonData/PDecimal/pdecimal005.xsd", "valid"},
	{precisionDecimalSaxonSetPath, "PDecimal", "pdecimal006", "pdecimal006.xsd", "1.1", "saxonData/PDecimal/pdecimal006.xsd", "valid"},
	{precisionDecimalSaxonSetPath, "PDecimal", "pdecimal007", "pdecimal007.xsd", "1.1", "saxonData/PDecimal/pdecimal007.xsd", "valid"},
	{precisionDecimalSaxonSetPath, "PDecimal", "pdecimal008", "pdecimal008.xsd", "1.1", "saxonData/PDecimal/pdecimal008.xsd", "valid"},
	{precisionDecimalSaxonSetPath, "PDecimal", "pdecimal009", "pdecimal009.xsd", "1.1", "saxonData/PDecimal/pdecimal009.n.xsd", "invalid"},
	{precisionDecimalSaxonSetPath, "PDecimal", "pdecimal010", "pdecimal010.xsd", "1.1", "saxonData/PDecimal/pdecimal010.xsd", "valid"},
	{precisionDecimalSaxonSetPath, "PDecimal", "pdecimal011", "pdecimal011.xsd", "1.1", "saxonData/PDecimal/pdecimal011.n.xsd", "invalid"},
	{precisionDecimalSaxonSetPath, "PDecimal", "pdecimal012", "pdecimal012.xsd", "1.1", "saxonData/PDecimal/pdecimal012.n.xsd", "invalid"},
	{precisionDecimalSaxonSetPath, "PDecimal", "pdecimal013", "pdecimal013.xsd", "1.1", "saxonData/PDecimal/pdecimal013.n.xsd", "invalid"},
	{precisionDecimalSaxonSetPath, "PDecimal", "pdecimal014", "pdecimal014.xsd", "1.1", "saxonData/PDecimal/pdecimal014.n.xsd", "invalid"},
	{precisionDecimalSaxonSetPath, "PDecimal", "pdecimal015", "pdecimal015.xsd", "1.1", "saxonData/PDecimal/pdecimal015.n.xsd", "invalid"},
	{precisionDecimalSaxonSetPath, "PDecimal", "pdecimal016", "pdecimal016.xsd", "1.1", "saxonData/PDecimal/pdecimal016.xsd", "valid"},
	{precisionDecimalSaxonSetPath, "PDecimal", "pdecimal017", "pdecimal017.xsd", "1.1", "saxonData/PDecimal/pdecimal017.n.xsd", "invalid"},
	{precisionDecimalSaxonSetPath, "PDecimal", "pdecimal018", "pdecimal018.xsd", "1.1", "saxonData/PDecimal/pdecimal018.n.xsd", "invalid"},
	{precisionDecimalSaxonSetPath, "PDecimal", "pdecimal019", "pdecimal019.xsd", "1.1", "saxonData/PDecimal/pdecimal019.xsd", "valid"},
	{precisionDecimalSaxonSetPath, "PDecimal", "pdecimal020", "pdecimal020.xsd", "1.1", "saxonData/PDecimal/pdecimal020.xsd", "valid"},
	{precisionDecimalIBMSetPath, "PrecisionDecimalTests", "d3_3_4v14", "d3_3_4v14s", "1.1", "ibmData/valid/D3_3_4/d3_3_4v14.xsd", "valid"},
	{precisionDecimalIBMSetPath, "PrecisionDecimalTests", "d3_3_4v15", "d3_3_4v15s", "1.1", "ibmData/valid/D3_3_4/d3_3_4v15.xsd", "valid"},
	{precisionDecimalIBMSetPath, "PrecisionDecimalTests", "d3_3_4v16", "d3_3_4v16s", "1.1", "ibmData/valid/D3_3_4/d3_3_4v16.xsd", "valid"},
	{precisionDecimalIBMSetPath, "PrecisionDecimalTests", "d3_3_4v17", "d3_3_4v17s", "1.1", "ibmData/valid/D3_3_4/d3_3_4v17.xsd", "valid"},
	{precisionDecimalIBMSetPath, "PrecisionDecimalTests", "d3_3_4v18", "d3_3_4v18s", "1.1", "ibmData/valid/D3_3_4/d3_3_4v18.xsd", "valid"},
	{precisionDecimalIBMSetPath, "PrecisionDecimalTests", "d3_3_4v19", "d3_3_4v19s", "1.1", "ibmData/valid/D3_3_4/d3_3_4v19.xsd", "valid"},
	{precisionDecimalIBMSetPath, "PrecisionDecimalTests", "d3_3_4v20", "d3_3_4v20s", "1.1", "ibmData/valid/D3_3_4/d3_3_4v20.xsd", "valid"},
	{precisionDecimalIBMSetPath, "PrecisionDecimalTests", "d3_3_4v21", "d3_3_4v21s", "1.1", "ibmData/valid/D3_3_4/d3_3_4v21.xsd", "valid"},
	{precisionDecimalIBMSetPath, "PrecisionDecimalTests", "d3_3_4v22", "d3_3_4v22s", "1.1", "ibmData/valid/D3_3_4/d3_3_4v22.xsd", "valid"},
	{precisionDecimalIBMSetPath, "PrecisionDecimalTests", "d3_3_4v23", "d3_3_4v32s", "1.1", "ibmData/valid/D3_3_4/d3_3_4v23.xsd", "valid"},
	{precisionDecimalIBMSetPath, "PrecisionDecimalTests", "d3_3_4v24", "d3_3_4v24s", "1.1", "ibmData/valid/D3_3_4/d3_3_4v24.xsd", "valid"},
	{precisionDecimalIBMSetPath, "PrecisionDecimalTests", "d3_3_4si09", "d3_3_4si09s", "1.1", "ibmData/schema_invalid/D3_3_4/d3_3_4si09.xsd", "invalid"},
	{precisionDecimalIBMSetPath, "PrecisionDecimalTests", "d3_3_4si10", "d3_3_4si10s", "1.1", "ibmData/schema_invalid/D3_3_4/d3_3_4si10.xsd", "invalid"},
	{precisionDecimalIBMSetPath, "PrecisionDecimalTests", "d3_3_4si11", "d3_3_4si11s", "1.1", "ibmData/schema_invalid/D3_3_4/d3_3_4si11.xsd", "invalid"},
	{precisionDecimalIBMSetPath, "PrecisionDecimalTests", "d3_3_4si12", "d3_3_4si12s", "1.1", "ibmData/schema_invalid/D3_3_4/d3_3_4si12.xsd", "invalid"},
	{precisionDecimalIBMSetPath, "PrecisionDecimalTests", "d3_3_4si13", "d3_3_4si13s", "1.1", "ibmData/schema_invalid/D3_3_4/d3_3_4si13.xsd", "invalid"},
	{precisionDecimalIBMSetPath, "PrecisionDecimalTests", "d3_3_4si14", "d3_3_4si14s", "1.1", "ibmData/schema_invalid/D3_3_4/d3_3_4si14.xsd", "invalid"},
	{precisionDecimalIBMSetPath, "PrecisionDecimalTests", "d3_3_4si15", "d3_3_4si15s", "1.1", "ibmData/schema_invalid/D3_3_4/d3_3_4si15.xsd", "invalid"},
	{precisionDecimalIBMSetPath, "PrecisionDecimalTests", "d3_3_4si16", "d3_3_4si16s", "1.1", "ibmData/schema_invalid/D3_3_4/d3_3_4si16.xsd", "invalid"},
	{precisionDecimalIBMSetPath, "PrecisionDecimalTests", "d3_3_4si17", "d3_3_4si17s", "1.1", "ibmData/schema_invalid/D3_3_4/d3_3_4si17.xsd", "invalid"},
	{precisionDecimalIBMSetPath, "PrecisionDecimalTests", "d3_3_4si18", "d3_3_4si18s", "1.1", "ibmData/schema_invalid/D3_3_4/d3_3_4si18.xsd", "invalid"},
	{precisionDecimalIBMSetPath, "PrecisionDecimalTests", "d3_3_4si19", "d3_3_4si19s", "1.1", "ibmData/schema_invalid/D3_3_4/d3_3_4si19.xsd", "invalid"},
	{precisionDecimalIBMSetPath, "PrecisionDecimalTests", "d3_3_4si20", "d3_3_4si20s", "1.1", "ibmData/schema_invalid/D3_3_4/d3_3_4si20.xsd", "invalid"},
	{precisionDecimalIBMSetPath, "PrecisionDecimalTests", "d3_3_4si21", "d3_3_4si21s", "1.1", "ibmData/schema_invalid/D3_3_4/d3_3_4si21.xsd", "invalid"},
	{precisionDecimalIBMSetPath, "PrecisionDecimalTests", "d3_3_4si22", "d3_3_4si22s", "1.1", "ibmData/schema_invalid/D3_3_4/d3_3_4si22.xsd", "invalid"},
	{precisionDecimalIBMSetPath, "PrecisionDecimalTests", "d3_3_4si23", "d3_3_4si23s", "1.1", "ibmData/schema_invalid/D3_3_4/d3_3_4si23.xsd", "invalid"},
	{precisionDecimalIBMSetPath, "PrecisionDecimalTests", "d3_3_4si24", "d3_3_4si24s", "1.1", "ibmData/schema_invalid/D3_3_4/d3_3_4si24.xsd", "invalid"},
	{precisionDecimalIBMSetPath, "PrecisionDecimalTests", "d3_3_4si25", "d3_3_4si25s", "1.1", "ibmData/schema_invalid/D3_3_4/d3_3_4si25.xsd", "invalid"},
	{precisionDecimalIBMSetPath, "PrecisionDecimalTests", "d3_3_4si26", "d3_3_4si26s", "1.1", "ibmData/schema_invalid/D3_3_4/d3_3_4si26.xsd", "invalid"},
	{precisionDecimalIBMSetPath, "PrecisionDecimalTests", "d3_3_4si27", "d3_3_4si27s", "1.1", "ibmData/schema_invalid/D3_3_4/d3_3_4si27.xsd", "invalid"},
	{precisionDecimalIBMSetPath, "PrecisionDecimalTests", "d3_3_4si28", "d3_3_4si28s", "1.1", "ibmData/schema_invalid/D3_3_4/d3_3_4si28.xsd", "invalid"},
	{precisionDecimalIBMSetPath, "PrecisionDecimalTests", "d3_3_4ii01", "d3_3_4ii01s", "1.1", "ibmData/instance_invalid/D3_3_4/d3_3_4ii01.xsd", "valid"},
	{precisionDecimalIBMSetPath, "PrecisionDecimalTests", "d3_3_4ii02", "d3_3_4ii02s", "1.1", "ibmData/instance_invalid/D3_3_4/d3_3_4ii02.xsd", "valid"},
}
