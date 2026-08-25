package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/goxdra/goxsd9"
)

const (
	maxGeneratedOutputBytes  = int64(16 << 20)
	invalidPackageDiagnostic = "GOXSD9026"
)

func runGenerate(options generateOptions, stdin io.Reader, stdout, stderr io.Writer) int {
	plan, err := prepareSchemaPlan(options.commandOptions)
	if err != nil {
		return reportError(stderr, "generate", options.diagnostics, "generate", err)
	}

	budget := &schemaBudget{}
	root, err := plan.openRoot(stdin, budget)
	if err != nil {
		return reportError(stderr, "generate", options.diagnostics, "generate", err)
	}

	schema, err := goxsd9.ParseSchema(root, plan.resolver(budget))
	if err != nil {
		return reportError(stderr, "generate", options.diagnostics, "generate", err)
	}

	generated, err := goxsd9.GenerateGo(schema, options.packageName)
	if err != nil {
		if message, ok := invalidPackageMessage(err); ok {
			return reportUsage(stderr, "generate", message, options.diagnostics, usageSourceID(options.schema))
		}
		return reportError(stderr, "generate", options.diagnostics, "generate", err)
	}

	outputSourceID := generatedOutputSourceID(options)
	if int64(len(generated)) > maxGeneratedOutputBytes {
		cause := newGeneratedOutputLimitError()
		diagnostic := newCLIError(cliLimitCode, cliLimitKind, outputSourceID, cause.Error(), cause)
		return reportError(stderr, "generate", options.diagnostics, "output", diagnostic)
	}

	if !options.outputSet || options.output == "-" {
		writeErr := writeBytes(stdout, generated)
		if writeErr != nil {
			diagnostic := newCLIError(cliOutputCode, cliOutputKind, "output/stdout", "failed to write generated Go to stdout", writeErr)
			return reportError(stderr, "generate", options.diagnostics, "output", diagnostic)
		}
		return 0
	}

	destination, err := prepareOutputDestination(options.output)
	if err != nil {
		return reportError(stderr, "generate", options.diagnostics, "output", err)
	}
	if err := writeGeneratedFile(destination, generated, options.force); err != nil {
		return reportError(stderr, "generate", options.diagnostics, "output", err)
	}
	return 0
}

func invalidPackageMessage(err error) (string, bool) {
	var diagnostic goxsd9.Diagnostic
	if errors.As(err, &diagnostic) && diagnostic.Class() == goxsd9.FailureInvalid && diagnostic.Code() == invalidPackageDiagnostic {
		return diagnostic.Message(), true
	}
	var diagnosticPointer *goxsd9.Diagnostic
	if errors.As(err, &diagnosticPointer) && diagnosticPointer != nil && diagnosticPointer.Class() == goxsd9.FailureInvalid && diagnosticPointer.Code() == invalidPackageDiagnostic {
		return diagnosticPointer.Message(), true
	}
	return "", false
}

type outputDestination struct {
	path     string
	sourceID goxsd9.SourceID
}

func prepareOutputDestination(operand string) (outputDestination, error) {
	sourceID := generatedOutputSourceIDForOperand(operand)
	cwd, err := os.Getwd()
	if err != nil {
		return outputDestination{}, newCLIError(cliOutputCode, cliOutputKind, sourceID, "failed to determine output directory", err)
	}
	path := operand
	if !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
	return outputDestination{path: filepath.Clean(path), sourceID: sourceID}, nil
}

func generatedOutputSourceID(options generateOptions) goxsd9.SourceID {
	if !options.outputSet || options.output == "-" {
		return "output/stdout"
	}
	return generatedOutputSourceIDForOperand(options.output)
}

func generatedOutputSourceIDForOperand(operand string) goxsd9.SourceID {
	if operand == "" || operand == "-" {
		return "output/stdout"
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "output/-"
	}
	path := operand
	if !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
	relative, err := filepath.Rel(cwd, filepath.Clean(path))
	if err != nil || relative == "." || relative == "" {
		return "output/-"
	}
	return goxsd9.SourceID("output/" + filepath.ToSlash(relative))
}

type outputTempFile interface {
	io.Writer
	Name() string
	Close() error
}

type outputFileOps interface {
	lstat(path string) (os.FileInfo, error)
	createTemp(directory, pattern string) (outputTempFile, error)
	rename(oldPath, newPath string) error
	remove(path string) error
}

type osOutputFileOps struct{}

func (osOutputFileOps) lstat(path string) (os.FileInfo, error) {
	return os.Lstat(path)
}

func (osOutputFileOps) createTemp(directory, pattern string) (outputTempFile, error) {
	return os.CreateTemp(directory, pattern)
}

func (osOutputFileOps) rename(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

func (osOutputFileOps) remove(path string) error {
	return os.Remove(path)
}

func writeGeneratedFile(destination outputDestination, generated []byte, force bool) error {
	return writeGeneratedFileWithOps(destination, generated, force, osOutputFileOps{})
}

func writeGeneratedFileWithOps(destination outputDestination, generated []byte, force bool, ops outputFileOps) error {
	if ops == nil {
		return newCLIError(cliOutputCode, cliOutputKind, destination.sourceID, "output file operations are unavailable", errors.New("nil output file operations"))
	}
	if err := inspectOutputDestination(destination, force, ops); err != nil {
		return err
	}

	temporary, err := ops.createTemp(filepath.Dir(destination.path), "."+filepath.Base(destination.path)+".goxsd9-")
	if err != nil {
		return newCLIError(cliOutputCode, cliOutputKind, destination.sourceID, "failed to create temporary output", err)
	}
	temporaryPath := temporary.Name()
	if temporaryPath == "" {
		closeErr := temporary.Close()
		cause := errors.Join(errors.New("temporary output has no path"), closeErr)
		return newCLIError(cliOutputCode, cliOutputKind, destination.sourceID, "failed to create temporary output", cause)
	}

	writeErr := writeAll(temporary, generated)
	closeErr := temporary.Close()
	if writeErr != nil || closeErr != nil {
		cause := errors.Join(writeErr, closeErr)
		cleanupErr := ops.remove(temporaryPath)
		cause = errors.Join(cause, cleanupErr)
		return newCLIError(cliOutputCode, cliOutputKind, destination.sourceID, "failed to write temporary output", cause)
	}

	if err := inspectOutputDestination(destination, force, ops); err != nil {
		cleanupErr := ops.remove(temporaryPath)
		return joinOutputCleanup(err, cleanupErr)
	}
	if err := ops.rename(temporaryPath, destination.path); err != nil {
		cleanupErr := ops.remove(temporaryPath)
		cause := errors.Join(err, cleanupErr)
		return newCLIError(cliOutputCode, cliOutputKind, destination.sourceID, "failed to install generated output", cause)
	}
	return nil
}

func inspectOutputDestination(destination outputDestination, force bool, ops outputFileOps) error {
	info, err := ops.lstat(destination.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return newCLIError(cliOutputCode, cliOutputKind, destination.sourceID, "failed to inspect output destination", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return newCLIError(cliOutputCode, cliOutputKind, destination.sourceID, "output destination is a symlink", errors.New("symlink destination is not replaceable"))
	}
	if !info.Mode().IsRegular() {
		return newCLIError(cliOutputCode, cliOutputKind, destination.sourceID, "output destination is not a regular file", errors.New("non-regular destination is not replaceable"))
	}
	if !force {
		return newCLIError(cliOutputCode, cliOutputKind, destination.sourceID, "output destination already exists; use --force to replace it", os.ErrExist)
	}
	return nil
}

func joinOutputCleanup(err, cleanupErr error) error {
	if cleanupErr == nil {
		return err
	}
	return errors.Join(err, fmt.Errorf("clean temporary output: %w", cleanupErr))
}

func writeAll(writer io.Writer, data []byte) error {
	if writer == nil {
		return errors.New("output writer is nil")
	}
	for offset := 0; offset < len(data); {
		count, err := writer.Write(data[offset:])
		if count < 0 || count > len(data)-offset {
			return errors.New("output writer returned an invalid byte count")
		}
		if err != nil {
			return err
		}
		if count == 0 {
			return io.ErrShortWrite
		}
		offset += count
	}
	return nil
}

func writeBytes(writer io.Writer, data []byte) error {
	return writeAll(writer, data)
}

type generatedOutputLimitError struct{}

func newGeneratedOutputLimitError() *generatedOutputLimitError {
	return &generatedOutputLimitError{}
}

func (*generatedOutputLimitError) Error() string {
	return "generated output exceeds the 16 MiB limit"
}
