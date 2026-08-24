package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/goxdra/goxsd9"
)

const (
	maxSchemaSourceBytes   = int64(16 << 20)
	maxSchemaTotalBytes    = int64(64 << 20)
	maxResolverCalls       = 256
	maxInstanceSourceBytes = int64(16 << 20)
)

type schemaBudget struct {
	totalBytes    int64
	resolverCalls int
}

type sourceContextKey struct{}

type sourceContext struct {
	root      string
	directory string
}

type schemaPlan struct {
	rootID      goxsd9.SourceID
	rootPath    string
	schemaRoot  string
	rootDir     string
	rootIsStdin bool
}

func prepareSchemaPlan(options parseOptions) (schemaPlan, error) {
	if options.schema == "-" {
		return prepareStdinPlan(options.schemaRoot)
	}
	if hasURIScheme(options.schema) {
		return schemaPlan{}, newCLIError(cliPathPolicyCode, cliPathPolicyKind, "schema/-", "schema operand is a URI, not a local path", nil)
	}

	absolute, err := filepath.Abs(options.schema)
	if err != nil {
		return schemaPlan{}, newCLIError(cliInternalCode, cliInternalKind, "schema/-", "failed to make schema operand absolute", err)
	}
	absolute = filepath.Clean(absolute)

	if options.schemaRootSet {
		return prepareExplicitFilePlan(options.schemaRoot, absolute)
	}

	return prepareDefaultFilePlan(absolute)
}

func prepareStdinPlan(rootPath string) (schemaPlan, error) {
	if hasURIScheme(rootPath) {
		return schemaPlan{}, newCLIError(cliPathPolicyCode, cliPathPolicyKind, "schema/stdin", "schema root is a URI, not a local path", nil)
	}
	root, err := canonicalDirectory(rootPath)
	if err != nil {
		return schemaPlan{}, newCLIError(cliResourceCode, cliResourceKind, "schema/stdin", "failed to use schema root", err)
	}
	return schemaPlan{
		rootID:      "schema/stdin",
		schemaRoot:  root,
		rootDir:     root,
		rootIsStdin: true,
	}, nil
}

func prepareExplicitFilePlan(rootPath, absolute string) (schemaPlan, error) {
	if hasURIScheme(rootPath) {
		return schemaPlan{}, newCLIError(cliPathPolicyCode, cliPathPolicyKind, "schema/-", "schema root is a URI, not a local path", nil)
	}
	root, err := canonicalDirectory(rootPath)
	if err != nil {
		return schemaPlan{}, newCLIError(cliResourceCode, cliResourceKind, "schema/-", "failed to use schema root", err)
	}
	return prepareFilePlan(root, absolute)
}

func prepareDefaultFilePlan(absolute string) (schemaPlan, error) {
	parent := filepath.Dir(absolute)
	root, err := canonicalDirectory(parent)
	if err != nil {
		return schemaPlan{}, newSourcePathError(parent, absolute, roleIDForPath(parent, absolute), err)
	}
	return prepareFilePlan(root, absolute)
}

func prepareFilePlan(root, absolute string) (schemaPlan, error) {
	roleID := roleIDForPath(root, absolute)
	canonical, err := canonicalFile(absolute)
	if err != nil {
		return schemaPlan{}, newSourcePathError(root, absolute, roleID, err)
	}
	if !pathWithin(root, canonical) {
		return schemaPlan{}, newCLIError(cliPathPolicyCode, cliPathPolicyKind, roleID, "schema source escapes schema root", nil)
	}
	resolvedID := roleIDForPath(root, canonical)
	if err := sourceSizeLimit(canonical, resolvedID); err != nil {
		var limitErr *sourceLimitError
		if errors.As(err, &limitErr) {
			return schemaPlan{}, newCLIError(cliLimitCode, cliLimitKind, resolvedID, limitErr.Error(), limitErr)
		}
		return schemaPlan{}, newCLIError(cliResourceCode, cliResourceKind, resolvedID, "failed to inspect schema source size", err)
	}
	return schemaPlan{
		rootID:     resolvedID,
		rootPath:   canonical,
		schemaRoot: root,
		rootDir:    filepath.Dir(canonical),
	}, nil
}

func newSourcePathError(root, path string, roleID goxsd9.SourceID, err error) error {
	var limitErr *sourceLimitError
	if errors.As(err, &limitErr) {
		return newCLIError(cliLimitCode, cliLimitKind, roleID, limitErr.Error(), limitErr)
	}
	if !pathWithin(root, path) {
		return newCLIError(cliPathPolicyCode, cliPathPolicyKind, roleID, "schema source escapes schema root", err)
	}
	return newCLIError(cliResourceCode, cliResourceKind, roleID, "failed to acquire schema source", err)
}

func canonicalDirectory(path string) (string, error) {
	if path == "" {
		return "", errors.New("schema root is empty")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("make schema root absolute: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("canonicalize schema root: %w", err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", fmt.Errorf("inspect schema root: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("schema root is not a directory")
	}
	if info.Mode().Perm()&0o500 != 0o500 {
		return "", errors.New("schema root is not readable and searchable")
	}
	return filepath.Clean(canonical), nil
}

func canonicalFile(path string) (string, error) {
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("canonicalize schema source: %w", err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", fmt.Errorf("inspect schema source: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("schema source is not a regular file")
	}
	if info.Mode().Perm()&0o444 == 0 {
		return "", errors.New("schema source is not readable")
	}
	return filepath.Clean(canonical), nil
}

func sourceSizeLimit(path string, sourceID goxsd9.SourceID) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("inspect schema source size: %w", err)
	}
	if info.Size() > maxSchemaSourceBytes {
		return newSourceLimitError(sourceID, "source exceeds the 16 MiB per-source limit")
	}
	return nil
}

func checkFileBudget(path string, sourceID goxsd9.SourceID, budget *schemaBudget) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("inspect schema source size: %w", err)
	}
	if info.Size() > maxSchemaSourceBytes {
		return 0, newSourceLimitError(sourceID, "source exceeds the 16 MiB per-source limit")
	}
	if info.Size() > maxSchemaTotalBytes-budget.totalBytes {
		return 0, newSourceLimitError(sourceID, "schema graph exceeds the 64 MiB total limit")
	}
	return info.Size(), nil
}

func pathWithin(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil || filepath.IsAbs(relative) {
		return false
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

func roleIDForPath(root, path string) goxsd9.SourceID {
	relative, err := filepath.Rel(root, path)
	if err != nil || !pathWithin(root, path) || relative == "." || relative == "" {
		return "schema/-"
	}
	return goxsd9.SourceID("schema/" + filepath.ToSlash(relative))
}

func (plan schemaPlan) openRoot(input io.Reader, budget *schemaBudget) (goxsd9.ResolvedSource, error) {
	var reader io.ReadCloser
	reserved := false
	if plan.rootIsStdin {
		if input == nil {
			return goxsd9.ResolvedSource{}, newCLIError(cliInternalCode, cliInternalKind, plan.rootID, "stdin reader is nil", nil)
		}
		reader = newInputReadCloser(input)
	}
	if plan.rootIsStdin {
		limited := newLimitedSource(reader, plan.rootID, budget, reserved)
		return plan.newRootSource(limited)
	}
	size, err := checkFileBudget(plan.rootPath, plan.rootID, budget)
	if err != nil {
		var limitErr *sourceLimitError
		if errors.As(err, &limitErr) {
			return goxsd9.ResolvedSource{}, newCLIError(cliLimitCode, cliLimitKind, plan.rootID, limitErr.Error(), limitErr)
		}
		return goxsd9.ResolvedSource{}, newCLIError(cliResourceCode, cliResourceKind, plan.rootID, "failed to inspect schema source size", err)
	}
	file, err := os.Open(plan.rootPath)
	if err != nil {
		return goxsd9.ResolvedSource{}, newCLIError(cliResourceCode, cliResourceKind, plan.rootID, "failed to open schema source", err)
	}
	budget.totalBytes += size
	limited := newLimitedSource(file, plan.rootID, budget, true)
	return plan.newRootSource(limited)
}

func (plan schemaPlan) newRootSource(limited *limitedSource) (goxsd9.ResolvedSource, error) {
	ctx := context.WithValue(context.Background(), sourceContextKey{}, sourceContext{
		root:      plan.schemaRoot,
		directory: plan.rootDir,
	})
	source, err := goxsd9.NewResolvedSource(ctx, plan.rootID, limited)
	if err == nil {
		return source, nil
	}
	closeErr := limited.Close()
	if closeErr != nil {
		err = fmt.Errorf("%w; failed to close schema source: %w", err, closeErr)
	}
	return goxsd9.ResolvedSource{}, newCLIError(cliInternalCode, cliInternalKind, plan.rootID, "failed to create schema source", err)
}

func (plan schemaPlan) resolver(budget *schemaBudget) goxsd9.Resolver {
	return &fileResolver{root: plan.schemaRoot, budget: budget}
}

type fileResolver struct {
	root   string
	budget *schemaBudget
}

func (resolver *fileResolver) Resolve(ctx context.Context, _ string, schemaLocation string) (goxsd9.ResolvedSource, error) {
	resolver.budget.resolverCalls++
	contextState, err := resolverContext(ctx)
	if err != nil {
		return goxsd9.ResolvedSource{}, err
	}
	roleID := roleIDForLocation(contextState, schemaLocation)
	if resolver.budget.resolverCalls > maxResolverCalls {
		return goxsd9.ResolvedSource{}, newCLIError(cliLimitCode, cliLimitKind, roleID, "resolver call limit exceeded", newResolverLimitError())
	}
	candidate, err := localSchemaPath(contextState, schemaLocation)
	if err != nil {
		return goxsd9.ResolvedSource{}, err
	}
	canonical, err := canonicalFile(candidate)
	if err != nil {
		return goxsd9.ResolvedSource{}, newCLIError(cliResourceCode, cliResourceKind, roleID, "failed to acquire referenced schema source", err)
	}
	if !pathWithin(resolver.root, canonical) {
		return goxsd9.ResolvedSource{}, newCLIError(cliPathPolicyCode, cliPathPolicyKind, roleID, "schema source escapes schema root", nil)
	}
	resolvedID := roleIDForPath(resolver.root, canonical)
	size, err := checkFileBudget(canonical, resolvedID, resolver.budget)
	if err != nil {
		var limitErr *sourceLimitError
		if errors.As(err, &limitErr) {
			return goxsd9.ResolvedSource{}, newCLIError(cliLimitCode, cliLimitKind, resolvedID, limitErr.Error(), limitErr)
		}
		return goxsd9.ResolvedSource{}, newCLIError(cliResourceCode, cliResourceKind, resolvedID, "failed to inspect referenced schema size", err)
	}
	return resolver.openSource(ctx, canonical, resolvedID, size)
}

func resolverContext(ctx context.Context) (sourceContext, error) {
	if ctx == nil {
		return sourceContext{}, newCLIError(cliInternalCode, cliInternalKind, "schema/-", "schema resolver context is nil", nil)
	}
	contextState, ok := ctx.Value(sourceContextKey{}).(sourceContext)
	if !ok || contextState.root == "" || contextState.directory == "" {
		return sourceContext{}, newCLIError(cliInternalCode, cliInternalKind, "schema/-", "schema resolver context is missing", nil)
	}
	return contextState, nil
}

func localSchemaPath(state sourceContext, location string) (string, error) {
	if location == "" {
		return "", newCLIError(cliResourceCode, cliResourceKind, "schema/-", "import has no local schema location", nil)
	}
	if hasURIScheme(location) || filepath.IsAbs(location) || filepath.VolumeName(location) != "" {
		return "", newCLIError(cliPathPolicyCode, cliPathPolicyKind, roleIDForLocation(state, location), "schema location is not a bare local relative path", nil)
	}
	candidate := filepath.Clean(filepath.Join(state.directory, filepath.FromSlash(location)))
	if !pathWithin(state.root, candidate) {
		return "", newCLIError(cliPathPolicyCode, cliPathPolicyKind, roleIDForLocation(state, location), "schema location escapes schema root", nil)
	}
	return candidate, nil
}

func (resolver *fileResolver) openSource(ctx context.Context, canonical string, sourceID goxsd9.SourceID, size int64) (goxsd9.ResolvedSource, error) {
	// canonical was produced by EvalSymlinks and checked beneath the root.
	file, err := os.Open(canonical) //nolint:gosec // the canonical path is the CLI's containment-checked source
	if err != nil {
		return goxsd9.ResolvedSource{}, newCLIError(cliResourceCode, cliResourceKind, sourceID, "failed to open referenced schema source", err)
	}
	resolver.budget.totalBytes += size
	reader := newLimitedSource(file, sourceID, resolver.budget, true)
	childContext := context.WithValue(ctx, sourceContextKey{}, sourceContext{
		root:      resolver.root,
		directory: filepath.Dir(canonical),
	})
	source, err := goxsd9.NewResolvedSource(childContext, sourceID, reader)
	if err == nil {
		return source, nil
	}
	closeErr := reader.Close()
	if closeErr != nil {
		err = fmt.Errorf("%w; failed to close referenced schema source: %w", err, closeErr)
	}
	return goxsd9.ResolvedSource{}, newCLIError(cliInternalCode, cliInternalKind, sourceID, "failed to create referenced schema source", err)
}

func roleIDForLocation(state sourceContext, location string) goxsd9.SourceID {
	if location == "" || hasURIScheme(location) || filepath.IsAbs(location) || filepath.VolumeName(location) != "" {
		return "schema/-"
	}
	candidate := filepath.Clean(filepath.Join(state.directory, filepath.FromSlash(location)))
	return roleIDForPath(state.root, candidate)
}

func hasURIScheme(value string) bool {
	colon := strings.IndexByte(value, ':')
	if colon <= 0 {
		return false
	}
	separator := strings.IndexAny(value, "/\\")
	if separator >= 0 && separator < colon {
		return false
	}
	if !isASCIILetter(value[0]) {
		return false
	}
	for index := 1; index < colon; index++ {
		character := value[index]
		if !isASCIILetter(character) && (character < '0' || character > '9') && character != '+' && character != '-' && character != '.' {
			return false
		}
	}
	return true
}

func isASCIILetter(character byte) bool {
	return (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z')
}

type instancePlan struct {
	sourceID  goxsd9.SourceID
	path      string
	fromStdin bool
}

func prepareInstancePlan(operand string) (instancePlan, error) {
	if operand == "-" {
		return instancePlan{sourceID: "instance/stdin", fromStdin: true}, nil
	}
	if hasURIScheme(operand) {
		return instancePlan{}, newCLIError(cliPathPolicyCode, cliPathPolicyKind, "instance/-", "instance operand is a URI, not a local path", nil)
	}

	invocationDirectory, err := os.Getwd()
	if err != nil {
		return instancePlan{}, newCLIError(cliResourceCode, cliResourceKind, "instance/-", "failed to determine invocation directory", err)
	}
	path := operand
	if !filepath.IsAbs(path) {
		path = filepath.Join(invocationDirectory, path)
	}
	path = filepath.Clean(path)
	return instancePlan{
		sourceID: instanceIDForPath(invocationDirectory, path),
		path:     path,
	}, nil
}

func instanceIDForPath(root, path string) goxsd9.SourceID {
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == "." || relative == "" {
		return "instance/-"
	}
	return goxsd9.SourceID("instance/" + filepath.ToSlash(relative))
}

func (plan instancePlan) open(input io.Reader) (io.ReadCloser, error) {
	if plan.fromStdin {
		if input == nil {
			return nil, newCLIError(cliInternalCode, cliInternalKind, plan.sourceID, "stdin reader is nil", nil)
		}
		return newInputReadCloser(input), nil
	}

	info, err := os.Stat(plan.path)
	if err != nil {
		return nil, newCLIError(cliResourceCode, cliResourceKind, plan.sourceID, "failed to inspect instance source", err)
	}
	if !info.Mode().IsRegular() {
		return nil, newCLIError(cliResourceCode, cliResourceKind, plan.sourceID, "instance source is not a regular file", nil)
	}
	if info.Mode().Perm()&0o444 == 0 {
		return nil, newCLIError(cliResourceCode, cliResourceKind, plan.sourceID, "instance source is not readable", nil)
	}
	if info.Size() > maxInstanceSourceBytes {
		return nil, newCLIError(cliLimitCode, cliLimitKind, plan.sourceID, "instance source exceeds the 16 MiB per-source limit", newSourceLimitError(plan.sourceID, "instance source exceeds the 16 MiB per-source limit"))
	}

	file, err := os.Open(plan.path)
	if err != nil {
		return nil, newCLIError(cliResourceCode, cliResourceKind, plan.sourceID, "failed to open instance source", err)
	}
	return file, nil
}

type inputReadCloser struct {
	reader io.Reader
	closer io.Closer
	closed bool
}

func newInputReadCloser(reader io.Reader) *inputReadCloser {
	closer, ok := reader.(io.Closer)
	if !ok {
		closer = nil
	}
	return &inputReadCloser{reader: reader, closer: closer}
}

func (reader *inputReadCloser) Read(buffer []byte) (int, error) {
	if reader.closed {
		return 0, os.ErrClosed
	}
	return reader.reader.Read(buffer)
}

func (reader *inputReadCloser) Close() error {
	if reader.closed {
		return nil
	}
	reader.closed = true
	if reader.closer == nil {
		return nil
	}
	return reader.closer.Close()
}

type instanceSource struct {
	reader   io.Reader
	closer   io.Closer
	sourceID goxsd9.SourceID
	used     int64
	closed   bool
}

func newInstanceSource(source io.ReadCloser, sourceID goxsd9.SourceID) *instanceSource {
	return &instanceSource{reader: source, closer: source, sourceID: sourceID}
}

func (reader *instanceSource) Read(buffer []byte) (int, error) {
	if reader.closed {
		return 0, os.ErrClosed
	}
	if len(buffer) == 0 {
		return 0, nil
	}
	remaining := maxInstanceSourceBytes - reader.used
	if remaining <= 0 {
		return reader.probeLimit()
	}
	allowed := int64(len(buffer))
	if allowed > remaining {
		allowed = remaining
	}
	count, err := reader.reader.Read(buffer[:int(allowed)])
	if count > 0 {
		reader.used += int64(count)
	}
	if err != nil {
		return count, err
	}
	return count, nil
}

func (reader *instanceSource) probeLimit() (int, error) {
	var probe [1]byte
	count, err := reader.reader.Read(probe[:])
	if count > 0 {
		return 0, newSourceLimitError(reader.sourceID, "instance source exceeds the 16 MiB per-source limit")
	}
	if err != nil {
		return 0, err
	}
	return 0, nil
}

func (reader *instanceSource) Close() error {
	if reader.closed {
		return nil
	}
	reader.closed = true
	return reader.closer.Close()
}

type limitedSource struct {
	reader    io.Reader
	closer    io.Closer
	sourceID  goxsd9.SourceID
	budget    *schemaBudget
	usedBytes int64
	closed    bool
	reserved  bool
}

func newLimitedSource(source io.ReadCloser, sourceID goxsd9.SourceID, budget *schemaBudget, reserved bool) *limitedSource {
	return &limitedSource{reader: source, closer: source, sourceID: sourceID, budget: budget, reserved: reserved}
}

func (reader *limitedSource) Read(buffer []byte) (int, error) {
	if reader.closed {
		return 0, os.ErrClosed
	}
	if len(buffer) == 0 {
		return 0, nil
	}
	remainingSource := maxSchemaSourceBytes - reader.usedBytes
	remainingTotal := maxSchemaTotalBytes
	if !reader.reserved {
		remainingTotal = maxSchemaTotalBytes - reader.budget.totalBytes
	}
	if remainingSource <= 0 || remainingTotal <= 0 {
		return reader.probeLimit()
	}
	allowed := int64(len(buffer))
	if allowed > remainingSource {
		allowed = remainingSource
	}
	if allowed > remainingTotal {
		allowed = remainingTotal
	}
	count, err := reader.reader.Read(buffer[:int(allowed)])
	if count > 0 {
		reader.usedBytes += int64(count)
		if !reader.reserved {
			reader.budget.totalBytes += int64(count)
		}
	}
	if err != nil {
		return count, err
	}
	return count, nil
}

func (reader *limitedSource) probeLimit() (int, error) {
	var probe [1]byte
	count, err := reader.reader.Read(probe[:])
	if count > 0 {
		return 0, newSourceLimitError(reader.sourceID, reader.limitMessage())
	}
	if err != nil {
		return 0, err
	}
	return 0, nil
}

func (reader *limitedSource) limitMessage() string {
	if reader.usedBytes >= maxSchemaSourceBytes {
		return "source exceeds the 16 MiB per-source limit"
	}
	return "schema graph exceeds the 64 MiB total limit"
}

func (reader *limitedSource) Close() error {
	if reader.closed {
		return nil
	}
	reader.closed = true
	return reader.closer.Close()
}

type sourceLimitError struct {
	sourceID goxsd9.SourceID
	message  string
}

func newSourceLimitError(sourceID goxsd9.SourceID, message string) *sourceLimitError {
	return &sourceLimitError{sourceID: sourceID, message: message}
}

func (err *sourceLimitError) Error() string {
	return err.message
}

type resolverLimitError struct{}

func newResolverLimitError() *resolverLimitError {
	return &resolverLimitError{}
}

func (*resolverLimitError) Error() string {
	return "schema resolver call limit exceeded"
}
