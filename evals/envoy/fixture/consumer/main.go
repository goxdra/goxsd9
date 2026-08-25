package main

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
	schemaSourceID  goxsd9.SourceID = "schema.xsd"
	validSourceID   goxsd9.SourceID = "valid.xml"
	invalidSourceID goxsd9.SourceID = "invalid.xml"
)

func main() {
	err := run()
	if err == nil {
		return
	}
	if _, writeErr := fmt.Fprintln(os.Stderr, "envoy consumer:", err); writeErr != nil {
		os.Exit(1)
	}
	os.Exit(1)
}

func run() error {
	schemaBytes, err := os.ReadFile("schema.xsd")
	if err != nil {
		return fmt.Errorf("read schema.xsd: %w", err)
	}
	root, err := goxsd9.NewResolvedSource(
		context.Background(),
		schemaSourceID,
		io.NopCloser(bytes.NewReader(schemaBytes)),
	)
	if err != nil {
		return fmt.Errorf("NewResolvedSource: %w", err)
	}
	schema, err := goxsd9.ParseSchema(root, nil)
	if err != nil {
		return fmt.Errorf("ParseSchema: %w", err)
	}

	validBytes, err := os.ReadFile("valid.xml")
	if err != nil {
		return fmt.Errorf("read valid.xml: %w", err)
	}
	if err := goxsd9.ValidateInstance(schema, validSourceID, io.NopCloser(bytes.NewReader(validBytes))); err != nil {
		return fmt.Errorf("ValidateInstance valid.xml: %w", err)
	}

	invalidBytes, err := os.ReadFile("invalid.xml")
	if err != nil {
		return fmt.Errorf("read invalid.xml: %w", err)
	}
	invalidErr := goxsd9.ValidateInstance(schema, invalidSourceID, io.NopCloser(bytes.NewReader(invalidBytes)))
	if invalidErr == nil {
		return errors.New("ValidateInstance invalid.xml unexpectedly succeeded")
	}
	var diagnostic goxsd9.Diagnostic
	if !errors.As(invalidErr, &diagnostic) {
		return fmt.Errorf("ValidateInstance invalid.xml returned no Diagnostic: %w", invalidErr)
	}
	if diagnostic.Class() != goxsd9.FailureInvalid {
		return fmt.Errorf("invalid diagnostic class = %q", diagnostic.Class())
	}
	if diagnostic.Code() != goxsd9.InvalidIntegerLexicalCode {
		return fmt.Errorf("invalid diagnostic code = %q", diagnostic.Code())
	}
	textOffset := bytes.IndexByte(invalidBytes, '>')
	if textOffset < 0 {
		return errors.New("invalid.xml has no root start-tag end")
	}
	wantLoc, err := goxsd9.NewLoc(invalidSourceID, 1, textOffset+2)
	if err != nil {
		return fmt.Errorf("invalid diagnostic location: %w", err)
	}
	if diagnostic.Loc() != wantLoc {
		return fmt.Errorf("invalid diagnostic location = %s, want %s", diagnostic.Loc(), wantLoc)
	}
	related := diagnostic.Related()
	if len(related) == 0 {
		return errors.New("invalid diagnostic has no related schema evidence")
	}
	relatedSchema := ""
	for _, location := range related {
		if location.Source() == schemaSourceID {
			relatedSchema = location.String()
			break
		}
	}
	if relatedSchema == "" {
		return errors.New("invalid diagnostic has no related schema location")
	}
	if diagnostic.SpecRef() != "xsd11-datatypes#integer" {
		return fmt.Errorf("invalid diagnostic specification reference = %q", diagnostic.SpecRef())
	}

	firstGenerated, err := goxsd9.GenerateGo(schema, "generated")
	if err != nil {
		return fmt.Errorf("GenerateGo: %w", err)
	}
	secondGenerated, err := goxsd9.GenerateGo(schema, "generated")
	if err != nil {
		return fmt.Errorf("GenerateGo second: %w", err)
	}
	if !bytes.Equal(firstGenerated, secondGenerated) {
		return errors.New("GenerateGo returned different bytes on repeat")
	}
	if len(firstGenerated) == 0 {
		return errors.New("GenerateGo returned empty source")
	}
	if err := compileGeneratedSource(firstGenerated); err != nil {
		return fmt.Errorf("compile generated source: %w", err)
	}

	lines := []string{
		"step=parse result=pass",
		"step=validate-valid result=pass",
		fmt.Sprintf("step=validate-invalid result=pass class=%s code=%s loc=%s related=%s spec_ref=%s", diagnostic.Class(), diagnostic.Code(), diagnostic.Loc(), relatedSchema, diagnostic.SpecRef()),
		fmt.Sprintf("step=generate-repeat result=pass bytes=%d", len(firstGenerated)),
		"step=compile-generated result=pass",
	}
	if _, err := fmt.Fprintln(os.Stdout, strings.Join(lines, "\n")); err != nil {
		return fmt.Errorf("write consumer result: %w", err)
	}
	return nil
}

func compileGeneratedSource(source []byte) error {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get consumer directory: %w", err)
	}
	moduleRoot, err := filepath.Abs(filepath.Join(workingDirectory, "..", "..", "..", ".."))
	if err != nil {
		return fmt.Errorf("find public module root: %w", err)
	}
	temporary, err := os.MkdirTemp("", "goxsd9-envoy-generated-")
	if err != nil {
		return fmt.Errorf("create temporary generated consumer: %w", err)
	}
	cleanup := func(cause error) error {
		removeErr := os.RemoveAll(temporary)
		if cause == nil {
			if removeErr != nil {
				return fmt.Errorf("remove temporary generated consumer: %w", removeErr)
			}
			return nil
		}
		if removeErr != nil {
			return errors.Join(cause, fmt.Errorf("remove temporary generated consumer: %w", removeErr))
		}
		return cause
	}

	goMod := fmt.Sprintf("module envoy-generated-consumer\n\ngo 1.26.0\n\nrequire github.com/goxdra/goxsd9 v0.0.0\n\nreplace github.com/goxdra/goxsd9 => %s\n", moduleRoot)
	if err := os.WriteFile(filepath.Join(temporary, "go.mod"), []byte(goMod), 0o600); err != nil {
		return cleanup(fmt.Errorf("write generated consumer go.mod: %w", err))
	}
	if err := os.WriteFile(filepath.Join(temporary, "generated.go"), source, 0o600); err != nil {
		return cleanup(fmt.Errorf("write exact generated.go: %w", err))
	}

	command := exec.CommandContext(context.Background(), "go", "test", "./...")
	command.Dir = temporary
	environment := make([]string, 0, len(os.Environ())+1)
	for _, value := range os.Environ() {
		if strings.HasPrefix(value, "GOWORK=") {
			continue
		}
		environment = append(environment, value)
	}
	command.Env = append(environment, "GOWORK=off")
	output, err := command.CombinedOutput()
	if err != nil {
		return cleanup(fmt.Errorf("go test generated consumer: %w: %s", err, output))
	}
	return cleanup(nil)
}
