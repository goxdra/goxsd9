package goxsd9_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/goxdra/goxsd9"
)

const (
	directChoiceNamespace      = "urn:direct-choice"
	directChoiceRootFixture    = "examples/direct-choice/root.xsd"
	directChoiceValidFixture   = "examples/direct-choice/valid.xml"
	directChoiceInvalidFixture = "examples/direct-choice/invalid.xml"
	directChoiceRootSource     = goxsd9.SourceID("caller:root")
	directChoiceValidSource    = goxsd9.SourceID("examples/direct-choice/valid.xml")
	directChoiceInvalidSource  = goxsd9.SourceID("examples/direct-choice/invalid.xml")
)

type directChoiceContextKey struct{}

type directChoiceImport struct {
	namespace string
	location  string
	sourceID  goxsd9.SourceID
	fixture   string
}

var directChoiceImports = [...]directChoiceImport{
	{
		namespace: "urn:direct-choice:integer",
		location:  "integer.xsd",
		sourceID:  goxsd9.SourceID("caller:integer"),
		fixture:   "examples/direct-choice/integer.xsd",
	},
	{
		namespace: "urn:direct-choice:decimal",
		location:  "decimal.xsd",
		sourceID:  goxsd9.SourceID("caller:decimal"),
		fixture:   "examples/direct-choice/decimal.xsd",
	},
}

type directChoiceCall struct {
	namespace string
	location  string
}

type directChoiceResolver struct {
	calls []directChoiceCall
}

func (resolver *directChoiceResolver) Resolve(
	ctx context.Context,
	namespaceURN, schemaLocation string,
) (goxsd9.ResolvedSource, error) {
	resolver.calls = append(resolver.calls, directChoiceCall{
		namespace: namespaceURN,
		location:  schemaLocation,
	})
	if ctx == nil {
		return goxsd9.ResolvedSource{}, errors.New("resolver received a nil context")
	}
	if ctx.Value(directChoiceContextKey{}) != "root" {
		return goxsd9.ResolvedSource{}, errors.New("resolver received the wrong caller context")
	}

	index := len(resolver.calls) - 1
	if index >= len(directChoiceImports) {
		return goxsd9.ResolvedSource{}, fmt.Errorf("unexpected resolver call %d: %q (%q)", index+1, namespaceURN, schemaLocation)
	}
	expected := directChoiceImports[index]
	if namespaceURN != expected.namespace || schemaLocation != expected.location {
		return goxsd9.ResolvedSource{}, fmt.Errorf("resolver call %d = %q (%q), want %q (%q)", index+1, namespaceURN, schemaLocation, expected.namespace, expected.location)
	}
	contents, err := os.ReadFile(expected.fixture)
	if err != nil {
		return goxsd9.ResolvedSource{}, fmt.Errorf("read imported fixture %s: %w", expected.fixture, err)
	}
	return newDirectChoiceSource(ctx, expected.sourceID, contents)
}

// Example_directChoice exercises only the documented direct scalar-choice slice.
func Example_directChoice() {
	if err := runDirectChoiceExample(); err != nil {
		if printErr := writeDirectChoiceOutput("direct choice example failed:", err); printErr != nil {
			return
		}
		return
	}
	if err := writeDirectChoiceOutput("direct choice: resolver order, immutable alternatives, validation, deterministic external generation"); err != nil {
		return
	}
	// Output:
	// direct choice: resolver order, immutable alternatives, validation, deterministic external generation
}

func runDirectChoiceExample() (err error) {
	rootContents, err := os.ReadFile(directChoiceRootFixture)
	if err != nil {
		return fmt.Errorf("read root fixture: %w", err)
	}
	rootContext := context.WithValue(context.Background(), directChoiceContextKey{}, "root")
	root, err := newDirectChoiceSource(rootContext, directChoiceRootSource, rootContents)
	if err != nil {
		return fmt.Errorf("create root source: %w", err)
	}
	resolver := &directChoiceResolver{}
	schema, err := goxsd9.ParseSchema(root, resolver)
	if err != nil {
		return fmt.Errorf("ParseSchema: %w", err)
	}
	if verifyErr := verifyDirectChoiceResolver(resolver, schema); verifyErr != nil {
		return verifyErr
	}
	_, queryErr := findDirectChoice(schema)
	if queryErr != nil {
		return queryErr
	}

	validContents, err := os.ReadFile(directChoiceValidFixture)
	if err != nil {
		return fmt.Errorf("read valid fixture: %w", err)
	}
	if validateErr := goxsd9.ValidateInstance(schema, directChoiceValidSource, io.NopCloser(bytes.NewReader(validContents))); validateErr != nil {
		return fmt.Errorf("ValidateInstance valid: %w", validateErr)
	}

	invalidContents, err := os.ReadFile(directChoiceInvalidFixture)
	if err != nil {
		return fmt.Errorf("read invalid fixture: %w", err)
	}
	invalidErr := goxsd9.ValidateInstance(schema, directChoiceInvalidSource, io.NopCloser(bytes.NewReader(invalidContents)))
	if diagnosticErr := verifyDirectChoiceInvalidDiagnostic(invalidErr, invalidContents); diagnosticErr != nil {
		return diagnosticErr
	}

	first, err := goxsd9.GenerateGo(schema, "generated")
	if err != nil {
		return fmt.Errorf("GenerateGo: %w", err)
	}
	second, err := goxsd9.GenerateGo(schema, "generated")
	if err != nil {
		return fmt.Errorf("GenerateGo second: %w", err)
	}
	if !bytes.Equal(first, second) {
		return errors.New("repeated GenerateGo output differs")
	}
	if compileErr := compileDirectChoiceSource(first); compileErr != nil {
		return compileErr
	}
	return nil
}

func newDirectChoiceSource(ctx context.Context, sourceID goxsd9.SourceID, contents []byte) (goxsd9.ResolvedSource, error) {
	reader := io.NopCloser(bytes.NewReader(contents))
	source, err := goxsd9.NewResolvedSource(ctx, sourceID, reader)
	if err == nil {
		return source, nil
	}
	if closeErr := reader.Close(); closeErr != nil {
		return goxsd9.ResolvedSource{}, errors.Join(err, fmt.Errorf("close source after construction failure: %w", closeErr))
	}
	return goxsd9.ResolvedSource{}, err
}

func verifyDirectChoiceResolver(resolver *directChoiceResolver, schema goxsd9.Schema) error {
	if len(resolver.calls) != len(directChoiceImports) {
		return fmt.Errorf("resolver call count = %d, want %d", len(resolver.calls), len(directChoiceImports))
	}
	for index, call := range resolver.calls {
		expected := directChoiceImports[index]
		if call.namespace != expected.namespace || call.location != expected.location {
			return fmt.Errorf("resolver call %d = %q (%q), want %q (%q)", index+1, call.namespace, call.location, expected.namespace, expected.location)
		}
	}
	documents := schema.Documents()
	wantSources := []goxsd9.SourceID{directChoiceRootSource, directChoiceImports[0].sourceID, directChoiceImports[1].sourceID}
	if len(documents) != len(wantSources) {
		return fmt.Errorf("schema document count = %d, want %d", len(documents), len(wantSources))
	}
	for index, document := range documents {
		if document.Source() != wantSources[index] {
			return fmt.Errorf("schema document %d source = %q, want %q", index+1, document.Source(), wantSources[index])
		}
	}
	return nil
}

func findDirectChoice(schema goxsd9.Schema) (goxsd9.ChoiceParticle, error) {
	if err := verifyDirectChoiceDeclaration(schema); err != nil {
		return goxsd9.ChoiceParticle{}, err
	}
	choice, err := queryDirectChoice(schema)
	if err != nil {
		return goxsd9.ChoiceParticle{}, err
	}
	if err := verifyDirectChoiceAlternatives(choice); err != nil {
		return goxsd9.ChoiceParticle{}, err
	}
	if err := verifyDirectChoiceWalk(schema); err != nil {
		return goxsd9.ChoiceParticle{}, err
	}
	return choice, nil
}

func verifyDirectChoiceDeclaration(schema goxsd9.Schema) error {
	rootName, err := goxsd9.NewQName(directChoiceNamespace, "reading")
	if err != nil {
		return fmt.Errorf("NewQName reading: %w", err)
	}
	declarations := schema.FindKind(goxsd9.ComponentKindElementDeclaration, rootName)
	if len(declarations) != 1 {
		return fmt.Errorf("reading declarations = %d, want 1", len(declarations))
	}
	return nil
}

func queryDirectChoice(schema goxsd9.Schema) (goxsd9.ChoiceParticle, error) {
	complexName, err := goxsd9.NewQName(directChoiceNamespace, "Reading")
	if err != nil {
		return goxsd9.ChoiceParticle{}, fmt.Errorf("NewQName Reading: %w", err)
	}
	complexTypes := schema.FindKind(goxsd9.ComponentKindComplexTypeDefinition, complexName)
	if len(complexTypes) != 1 {
		return goxsd9.ChoiceParticle{}, fmt.Errorf("Reading definitions = %d, want 1", len(complexTypes))
	}
	definition, ok := complexTypes[0].ComplexTypeDefinition()
	if !ok {
		return goxsd9.ChoiceParticle{}, errors.New("Reading has no public complex type view")
	}
	choice, ok := definition.Particle().(goxsd9.ChoiceParticle)
	if !ok {
		return goxsd9.ChoiceParticle{}, errors.New("Reading particle is not a public ChoiceParticle")
	}
	return choice, nil
}

func verifyDirectChoiceAlternatives(choice goxsd9.ChoiceParticle) error {
	if choice.MinOccurs() != 1 || choice.MaxOccurs() != 1 {
		return fmt.Errorf("choice bounds = %d/%d, want 1/1", choice.MinOccurs(), choice.MaxOccurs())
	}
	alternatives := choice.Alternatives()
	if len(alternatives) != 2 {
		return fmt.Errorf("choice alternative count = %d, want 2", len(alternatives))
	}
	alternatives[0] = nil
	stableAlternatives := choice.Alternatives()
	wantAlternatives := []struct {
		name     string
		typeName string
	}{
		{name: "count", typeName: "integer"},
		{name: "amount", typeName: "decimal"},
	}
	for index, want := range wantAlternatives {
		if index >= len(stableAlternatives) {
			return fmt.Errorf("choice alternative %d disappeared after slice mutation", index)
		}
		element, ok := stableAlternatives[index].(goxsd9.ElementParticle)
		if !ok {
			return fmt.Errorf("alternative %d type = %T, want ElementParticle", index, stableAlternatives[index])
		}
		if element.Name().Namespace() != "" || element.Name().Local() != want.name {
			return fmt.Errorf("alternative %d name = %q, want unqualified %q", index, element.Name(), want.name)
		}
		declaredType := element.DeclaredType()
		if declaredType.Namespace() != "http://www.w3.org/2001/XMLSchema" || declaredType.Local() != want.typeName {
			return fmt.Errorf("alternative %d declared type = %q, want xs:%s", index, declaredType, want.typeName)
		}
	}
	return nil
}

func verifyDirectChoiceWalk(schema goxsd9.Schema) error {
	var walked []goxsd9.Component
	if err := schema.Walk(func(component goxsd9.Component) error {
		walked = append(walked, component)
		return nil
	}); err != nil {
		return fmt.Errorf("Walk: %w", err)
	}
	if len(walked) != 2 || walked[0].Name().Local() != "Reading" || walked[1].Name().Local() != "reading" {
		return errors.New("public Walk did not preserve root lexical declaration order")
	}
	return nil
}

func verifyDirectChoiceInvalidDiagnostic(err error, contents []byte) error {
	if err == nil {
		return errors.New("ValidateInstance invalid unexpectedly succeeded")
	}
	var diagnostic goxsd9.Diagnostic
	if !errors.As(err, &diagnostic) {
		return fmt.Errorf("invalid validation error is not a public Diagnostic: %w", err)
	}
	wantLoc, locErr := directChoiceTextLoc(directChoiceInvalidSource, contents, "not-an-integer")
	if locErr != nil {
		return locErr
	}
	if diagnostic.Class() != goxsd9.FailureInvalid {
		return fmt.Errorf("invalid diagnostic class = %q, want %q", diagnostic.Class(), goxsd9.FailureInvalid)
	}
	if diagnostic.Code() != goxsd9.InvalidIntegerLexicalCode {
		return fmt.Errorf("invalid diagnostic code = %q, want %q", diagnostic.Code(), goxsd9.InvalidIntegerLexicalCode)
	}
	if diagnostic.Loc() != wantLoc {
		return fmt.Errorf("invalid diagnostic location = %s, want %s", diagnostic.Loc(), wantLoc)
	}
	if diagnostic.SpecRef() != "xsd11-datatypes#integer" {
		return fmt.Errorf("invalid diagnostic specification reference = %q, want %q", diagnostic.SpecRef(), "xsd11-datatypes#integer")
	}
	return nil
}

func directChoiceTextLoc(sourceID goxsd9.SourceID, contents []byte, marker string) (goxsd9.Loc, error) {
	text := string(contents)
	index := strings.Index(text, marker)
	if index < 0 {
		return goxsd9.Loc{}, fmt.Errorf("invalid fixture has no %q text", marker)
	}
	line, column := 1, 1
	for _, character := range text[:index] {
		if character == '\n' {
			line++
			column = 1
			continue
		}
		column++
	}
	loc, err := goxsd9.NewLoc(sourceID, line, column)
	if err != nil {
		return goxsd9.Loc{}, fmt.Errorf("NewLoc invalid instance text: %w", err)
	}
	return loc, nil
}

func compileDirectChoiceSource(source []byte) (err error) {
	moduleRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get module root: %w", err)
	}
	temporary, err := os.MkdirTemp("", "goxsd9-direct-choice-example-")
	if err != nil {
		return fmt.Errorf("create temporary consumer module: %w", err)
	}
	defer func() {
		if removeErr := os.RemoveAll(temporary); removeErr != nil {
			cleanupErr := fmt.Errorf("remove temporary consumer module: %w", removeErr)
			if err == nil {
				err = cleanupErr
				return
			}
			err = errors.Join(err, cleanupErr)
		}
	}()

	goMod := fmt.Sprintf("module direct-choice-example.test\n\ngo 1.26.0\n\nrequire github.com/goxdra/goxsd9 v0.0.0\n\nreplace github.com/goxdra/goxsd9 => %s\n", moduleRoot)
	if writeErr := os.WriteFile(filepath.Join(temporary, "go.mod"), []byte(goMod), 0o600); writeErr != nil {
		return fmt.Errorf("write consumer go.mod: %w", writeErr)
	}
	// #nosec G703 -- temporary is created and owned by this test; the joined path is not user-controlled.
	if writeErr := os.WriteFile(filepath.Join(temporary, "generated.go"), source, 0o600); writeErr != nil {
		return fmt.Errorf("write exact generated source: %w", writeErr)
	}
	consumerDirectory := filepath.Join(temporary, "consumer")
	if mkdirErr := os.Mkdir(consumerDirectory, 0o700); mkdirErr != nil {
		return fmt.Errorf("create external consumer directory: %w", mkdirErr)
	}
	consumer := `package consumer

import generated "direct-choice-example.test"

var _ generated.Reading = generated.Count{}
var _ generated.Reading = generated.Amount{}

func useChoice(choice generated.Reading) {
	switch alternative := choice.(type) {
	case generated.Count:
		_ = alternative
	case generated.Amount:
		_ = alternative
	}
}
`
	if writeErr := os.WriteFile(filepath.Join(consumerDirectory, "consumer.go"), []byte(consumer), 0o600); writeErr != nil {
		return fmt.Errorf("write external consumer: %w", writeErr)
	}
	command := exec.CommandContext(context.Background(), "go", "test", "./...")
	command.Dir = temporary
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go test external consumer: %w\n%s", err, output)
	}
	return nil
}

func writeDirectChoiceOutput(args ...any) error {
	_, err := fmt.Println(args...)
	return err
}
