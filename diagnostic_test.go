package goxsd9

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestDiagnosticCodesAreUnique(t *testing.T) {
	definitions, err := discoverDiagnosticCodeDefinitions(".")
	if err != nil {
		t.Fatalf("discover diagnostic codes: %v", err)
	}
	if duplicates := duplicateDiagnosticCodes(definitions); len(duplicates) != 0 {
		t.Fatalf("duplicate diagnostic codes:\n%s", strings.Join(duplicates, "\n"))
	}
}

type diagnosticCodeDefinition struct {
	code   string
	file   string
	line   int
	column int
	name   string
}

var diagnosticCodePattern = regexp.MustCompile(`^(?:GOXSD|XSD)[0-9]{4}$`)

func discoverDiagnosticCodeDefinitions(root string) ([]diagnosticCodeDefinition, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read diagnostic code root: %w", err)
	}
	fileSet := token.NewFileSet()
	definitions := make([]diagnosticCodeDefinition, 0)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(root, name)
		fileDefinitions, fileErr := discoverDiagnosticCodesInFile(fileSet, path)
		if fileErr != nil {
			return nil, fileErr
		}
		definitions = append(definitions, fileDefinitions...)
	}
	sort.Slice(definitions, func(left, right int) bool {
		first, second := definitions[left], definitions[right]
		if first.code != second.code {
			return first.code < second.code
		}
		if first.file != second.file {
			return first.file < second.file
		}
		if first.line != second.line {
			return first.line < second.line
		}
		if first.column != second.column {
			return first.column < second.column
		}
		return first.name < second.name
	})
	return definitions, nil
}

func discoverDiagnosticCodesInFile(fileSet *token.FileSet, path string) ([]diagnosticCodeDefinition, error) {
	file, err := parser.ParseFile(fileSet, path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	definitions := make([]diagnosticCodeDefinition, 0)
	ast.Inspect(file, func(node ast.Node) bool {
		declaration, ok := node.(*ast.GenDecl)
		if !ok || declaration.Tok != token.CONST {
			return true
		}
		for _, specification := range declaration.Specs {
			valueSpec, ok := specification.(*ast.ValueSpec)
			if !ok {
				continue
			}
			definitions = append(definitions, diagnosticCodesInValueSpec(fileSet, path, valueSpec)...)
		}
		return true
	})
	return definitions, nil
}

func diagnosticCodesInValueSpec(fileSet *token.FileSet, path string, valueSpec *ast.ValueSpec) []diagnosticCodeDefinition {
	definitions := make([]diagnosticCodeDefinition, 0, len(valueSpec.Values))
	for index, value := range valueSpec.Values {
		literal, ok := value.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING || index >= len(valueSpec.Names) {
			continue
		}
		code, err := strconv.Unquote(literal.Value)
		if err != nil || !diagnosticCodePattern.MatchString(code) {
			continue
		}
		position := fileSet.Position(literal.Pos())
		definitions = append(definitions, diagnosticCodeDefinition{
			code: code, file: filepath.ToSlash(path), line: position.Line,
			column: position.Column, name: valueSpec.Names[index].Name,
		})
	}
	return definitions
}

func duplicateDiagnosticCodes(definitions []diagnosticCodeDefinition) []string {
	duplicates := make([]string, 0)
	for index := 0; index < len(definitions); {
		end := index + 1
		for end < len(definitions) && definitions[end].code == definitions[index].code {
			end++
		}
		if end-index > 1 {
			parts := make([]string, 0, end-index)
			for _, definition := range definitions[index:end] {
				parts = append(parts, fmt.Sprintf("%s (%s:%d:%d)", definition.name, definition.file,
					definition.line, definition.column))
			}
			duplicates = append(duplicates, fmt.Sprintf("%s: %s", definitions[index].code, strings.Join(parts, ", ")))
		}
		index = end
	}
	return duplicates
}

func TestDiagnosticCodeDiscoveryCatchesUnlistedDuplicate(t *testing.T) {
	root := t.TempDir()
	fixture := `package fixture

const (
	knownCode = "XSD9999"
	newUnlistedCode = "XSD9999"
)
`
	if err := os.WriteFile(filepath.Join(root, "fixture.go"), []byte(fixture), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	definitions, err := discoverDiagnosticCodeDefinitions(root)
	if err != nil {
		t.Fatalf("discover fixture codes: %v", err)
	}
	duplicates := duplicateDiagnosticCodes(definitions)
	if len(duplicates) != 1 || !strings.Contains(duplicates[0], "newUnlistedCode") {
		t.Fatalf("fixture duplicates = %#v, want newly introduced unlisted literal", duplicates)
	}
}

func TestUnsupportedDiagnostic(t *testing.T) {
	loc, err := NewLoc("schema.xsd", 3, 7)
	if err != nil {
		t.Fatalf("NewLoc: %v", err)
	}
	feature, ok := LookupUnsupportedFeature("xsd.assertion")
	if !ok {
		t.Fatal("xsd.assertion is not registered")
	}
	diagnostic := newUnsupported(feature, "XSD1001", loc, "assertions are not implemented")

	if !errors.Is(diagnostic, ErrUnsupported) {
		t.Fatal("unsupported diagnostic does not match ErrUnsupported")
	}
	if got, want := diagnostic.Error(), "schema.xsd:3:7: XSD1001: assertions are not implemented"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
	if got, want := diagnostic.Feature(), FeatureID("xsd.assertion"); got != want {
		t.Fatalf("Feature() = %q, want %q", got, want)
	}
	if got, want := diagnostic.SpecRef(), "xsd11-structures#cAssertions"; got != want {
		t.Fatalf("SpecRef() = %q, want %q", got, want)
	}
}

func TestUnsupportedDiagnosticRejectsUnregisteredFeature(t *testing.T) {
	diagnostic := newUnsupported(UnsupportedFeature{}, "XSD1001", Loc{}, "assertions are not implemented")
	if diagnostic.Class() != FailureInternal {
		t.Fatalf("Class() = %q, want %q", diagnostic.Class(), FailureInternal)
	}
	if diagnostic.Feature() != "" {
		t.Fatalf("Feature() = %q, want an empty ID", diagnostic.Feature())
	}
}

func TestGenericUnsupportedDiagnosticBecomesInternal(t *testing.T) {
	diagnostic := newDiagnostic(FailureUnsupported, "XSD1001", Loc{}, "placeholder", nil)
	if diagnostic.Class() != FailureInternal {
		t.Fatalf("Class() = %q, want %q", diagnostic.Class(), FailureInternal)
	}
	if diagnostic.Feature() != "" {
		t.Fatalf("Feature() = %q, want an empty ID", diagnostic.Feature())
	}
}

func TestDiagnosticsPreserveOrderAndOwnership(t *testing.T) {
	first := newDiagnostic(FailureInvalid, "XSD0001", Loc{}, "first", nil)
	second := newDiagnostic(FailureResolution, "XSD0002", Loc{}, "second", errors.New("offline"))
	input := []Diagnostic{first, second}
	diagnostics := makeDiagnostics(input)
	input[0] = second

	firstItem, ok := diagnostics.At(0)
	if !ok {
		t.Fatal("At(0) did not find a diagnostic")
	}
	if got, want := firstItem.Code(), "XSD0001"; got != want {
		t.Fatalf("At(0).Code() = %q, want %q", got, want)
	}
	all := diagnostics.All()
	all[0] = second
	firstItem, ok = diagnostics.At(0)
	if !ok {
		t.Fatal("At(0) did not find a diagnostic after copy mutation")
	}
	if got, want := firstItem.Code(), "XSD0001"; got != want {
		t.Fatalf("At(0).Code() after copy mutation = %q, want %q", got, want)
	}
	if !errors.Is(diagnostics, second.Unwrap()) {
		t.Fatal("aggregate does not expose nested cause")
	}
}
