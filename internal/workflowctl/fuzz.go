package workflowctl

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	fuzzWorkerCount   = 1
	fuzzProcessMargin = 30 * time.Second

	fuzzInvalidInputCode  = "WFZ1001"
	fuzzRootCode          = "WFZ1002"
	fuzzSandboxCode       = "WFZ1003"
	fuzzTimeoutCode       = "WFZ1004"
	fuzzProcessStartCode  = "WFZ1005"
	fuzzProcessExitCode   = "WFZ1006"
	fuzzProcessSignalCode = "WFZ1007"
	fuzzCleanupCode       = "WFZ1008"
	fuzzEvidenceCode      = "WFZ1009"
	fuzzOutputCode        = "WFZ1010"
	fuzzCanceledCode      = "WFZ1011"
)

func fuzzGoEnvironment() []string {
	return []string{
		"GOENV=off",
		"GOTOOLCHAIN=local",
		"GOPROXY=off",
		"GOSUMDB=off",
		"GOVCS=*:off",
		"GOWORK=off",
	}
}

type fuzzFlagValue struct {
	set   bool
	value string
}

func (value *fuzzFlagValue) Set(input string) error {
	if value.set {
		return errors.New("flag may be specified only once")
	}
	value.set = true
	value.value = input
	return nil
}

func (value *fuzzFlagValue) String() string {
	return value.value
}

type fuzzRun struct {
	packageName string
	target      string
	duration    time.Duration
	workers     int
	offline     bool
}

type fuzzSandbox struct {
	root         string
	source       string
	cache        string
	tmp          string
	moduleCache  string
	corpusBefore []string
}

type fuzzDiagnostic struct {
	code    string
	message string
	cause   error
}

func (diagnostic *fuzzDiagnostic) Error() string {
	return diagnostic.code + ": " + diagnostic.message
}

func (diagnostic *fuzzDiagnostic) Unwrap() error {
	return diagnostic.cause
}

type fuzzProcessOutcome struct {
	result     string
	diagnostic error
}

func (a app) runFuzz(args []string) error {
	flags := flag.NewFlagSet("fuzz", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var packageFlag fuzzFlagValue
	var targetFlag fuzzFlagValue
	var durationFlag fuzzFlagValue
	flags.Var(&packageFlag, "package", "one explicit Go package")
	flags.Var(&targetFlag, "target", "one FuzzXxx target")
	flags.Var(&durationFlag, "duration", "positive fuzz duration")
	if err := flags.Parse(args); err != nil {
		return usageError("fuzz: %v", err)
	}
	if flags.NArg() != 0 || !packageFlag.set || !targetFlag.set || !durationFlag.set {
		return usageError("usage: workflowctl fuzz --package PACKAGE --target FUZZ_TARGET --duration DURATION")
	}
	run, err := newFuzzRun(packageFlag.value, targetFlag.value, durationFlag.value)
	if err != nil {
		return usageError("fuzz: %w", err)
	}
	root, err := a.root()
	if err != nil {
		return newFuzzDiagnostic(fuzzRootCode, err, "find repository root")
	}
	run, err = validateFuzzRun(root, run)
	if err != nil {
		return usageError("fuzz: %w", err)
	}
	return a.executeFuzzRun(root, run)
}

func newFuzzRun(packageName, target, durationText string) (fuzzRun, error) {
	if err := validateFuzzPackageName(packageName); err != nil {
		return fuzzRun{}, newFuzzDiagnostic(fuzzInvalidInputCode, nil, "%v", err)
	}
	if err := validateFuzzTargetName(target); err != nil {
		return fuzzRun{}, newFuzzDiagnostic(fuzzInvalidInputCode, nil, "%v", err)
	}
	duration, err := time.ParseDuration(durationText)
	if err != nil {
		return fuzzRun{}, newFuzzDiagnostic(fuzzInvalidInputCode, err, "invalid fuzz duration %q", durationText)
	}
	if duration <= 0 {
		return fuzzRun{}, newFuzzDiagnostic(fuzzInvalidInputCode, nil,
			"fuzz duration %q must be positive and bounded", durationText)
	}
	if duration > time.Duration(1<<63-1)-fuzzProcessMargin {
		return fuzzRun{}, newFuzzDiagnostic(fuzzInvalidInputCode, nil,
			"fuzz duration %q is too large for the fixed process margin", durationText)
	}
	return fuzzRun{
		packageName: packageName,
		target:      target,
		duration:    duration,
		workers:     fuzzWorkerCount,
		offline:     true,
	}, nil
}

func validateFuzzPackageName(packageName string) error {
	if packageName == "" {
		return errors.New("package must be non-empty")
	}
	if strings.TrimSpace(packageName) != packageName {
		return errors.New("package must not contain surrounding whitespace")
	}
	if strings.Contains(packageName, "...") {
		return fmt.Errorf("package %q must name exactly one package, not a wildcard", packageName)
	}
	if strings.HasPrefix(packageName, "-") {
		return fmt.Errorf("package %q must not begin with '-'; argument injection is not allowed", packageName)
	}
	if err := validateFuzzPathCharacters(packageName); err != nil {
		return fmt.Errorf("invalid package %q: %w", packageName, err)
	}
	if packageName == "." || packageName == "./" {
		return nil
	}
	return validateFuzzPackageComponents(packageName)
}

func validateFuzzPackageComponents(packageName string) error {
	if strings.Contains(packageName, "//") {
		return fmt.Errorf("package %q has an empty path component", packageName)
	}
	for index, component := range strings.Split(packageName, "/") {
		if component == "" {
			return fmt.Errorf("package %q has an empty path component", packageName)
		}
		if component == ".." {
			return fmt.Errorf("package %q must stay inside the module", packageName)
		}
		if component == "." && index != 0 {
			return fmt.Errorf("package %q has a non-canonical path", packageName)
		}
	}
	return nil
}

func validateFuzzPathCharacters(value string) error {
	for index := 0; index < len(value); index++ {
		character := value[index]
		if isFuzzASCIIAlpha(character) || isFuzzASCIIDigit(character) || strings.ContainsRune("._/-", rune(character)) {
			continue
		}
		return fmt.Errorf("character %q at byte %d is not allowed", character, index)
	}
	return nil
}

func validateFuzzTargetName(target string) error {
	if target == "" {
		return errors.New("target must be non-empty")
	}
	if strings.TrimSpace(target) != target {
		return errors.New("target must not contain surrounding whitespace")
	}
	if !strings.HasPrefix(target, "Fuzz") || len(target) == len("Fuzz") {
		return fmt.Errorf("target %q must be a FuzzXxx function name", target)
	}
	if !isFuzzASCIIAlpha(target[0]) && target[0] != '_' {
		return fmt.Errorf("target %q is not a Go identifier", target)
	}
	for index := 0; index < len(target); index++ {
		character := target[index]
		if isFuzzASCIIAlpha(character) || isFuzzASCIIDigit(character) || character == '_' {
			continue
		}
		return fmt.Errorf("target %q contains invalid identifier byte %q", target, character)
	}
	return nil
}

func isFuzzASCIIAlpha(character byte) bool {
	return character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z'
}

func isFuzzASCIIDigit(character byte) bool {
	return character >= '0' && character <= '9'
}

func validateFuzzRun(root string, run fuzzRun) (fuzzRun, error) {
	relativeDir, err := fuzzPackageRelativeDir(root, run.packageName)
	if err != nil {
		return fuzzRun{}, newFuzzDiagnostic(fuzzInvalidInputCode, err, "resolve fuzz package %q", run.packageName)
	}
	packageDir := filepath.Join(root, filepath.FromSlash(relativeDir))
	info, err := os.Stat(packageDir)
	if err != nil {
		return fuzzRun{}, newFuzzDiagnostic(fuzzInvalidInputCode, err, "inspect fuzz package %q", run.packageName)
	}
	if !info.IsDir() {
		return fuzzRun{}, newFuzzDiagnostic(fuzzInvalidInputCode, nil,
			"fuzz package %q is not a directory", run.packageName)
	}
	if err := findFuzzTarget(packageDir, run.target); err != nil {
		return fuzzRun{}, newFuzzDiagnostic(fuzzInvalidInputCode, err,
			"validate fuzz target %q in package %q", run.target, run.packageName)
	}
	return run, nil
}

func fuzzPackageRelativeDir(root, packageName string) (string, error) {
	if err := validateFuzzPackageName(packageName); err != nil {
		return "", err
	}
	if packageName == "." || packageName == "./" {
		return ".", nil
	}
	if strings.HasPrefix(packageName, "./") {
		return strings.TrimPrefix(packageName, "./"), nil
	}
	modulePath, err := readFuzzModulePath(root)
	if err != nil {
		return "", err
	}
	if packageName == modulePath {
		return ".", nil
	}
	prefix := modulePath + "/"
	if strings.HasPrefix(packageName, prefix) {
		return strings.TrimPrefix(packageName, prefix), nil
	}
	return "", fmt.Errorf("package %q is not a module-relative package", packageName)
}

func readFuzzModulePath(root string) (string, error) {
	// #nosec G304 -- root is the repository root selected by git.
	content, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("read module definition: %w", err)
	}
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "module" {
			continue
		}
		return fields[1], nil
	}
	return "", errors.New("module definition has no module path")
}

func findFuzzTarget(packageDir, target string) error {
	entries, err := os.ReadDir(packageDir)
	if err != nil {
		return fmt.Errorf("read package source: %w", err)
	}
	fset := token.NewFileSet()
	found := 0
	valid := 0
	for _, entry := range entries {
		if !isFuzzTestFile(entry) {
			continue
		}
		fileFound, fileValid, fileErr := fuzzTargetFileMatches(packageDir, entry, fset, target)
		if fileErr != nil {
			return fileErr
		}
		found += fileFound
		valid += fileValid
	}
	if found == 0 {
		return fmt.Errorf("target %q is absent", target)
	}
	if found != 1 || valid != 1 {
		return fmt.Errorf("target %q is ambiguous or has an invalid fuzz signature", target)
	}
	return nil
}

func isFuzzTestFile(entry fs.DirEntry) bool {
	return !entry.IsDir() && strings.HasSuffix(entry.Name(), "_test.go")
}

func fuzzTargetFileMatches(packageDir string, entry fs.DirEntry, fset *token.FileSet, target string) (int, int, error) {
	filePath := filepath.Join(packageDir, entry.Name())
	// #nosec G304 -- entry was read directly from the selected package directory.
	file, err := parser.ParseFile(fset, filePath, nil, parser.SkipObjectResolution)
	if err != nil {
		return 0, 0, fmt.Errorf("parse %s: %w", entry.Name(), err)
	}
	testingNames, err := fuzzTestingPackageNames(file)
	if err != nil {
		return 0, 0, fmt.Errorf("inspect %s imports: %w", entry.Name(), err)
	}
	found := 0
	valid := 0
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != target {
			continue
		}
		found++
		if isFuzzTargetFunction(function, testingNames) {
			valid++
		}
	}
	return found, valid, nil
}

func fuzzTestingPackageNames(file *ast.File) ([]string, error) {
	names := make([]string, 0, 1)
	for _, importSpec := range file.Imports {
		importPath, err := strconv.Unquote(importSpec.Path.Value)
		if err != nil {
			return nil, fmt.Errorf("unquote import path: %w", err)
		}
		if importPath != "testing" {
			continue
		}
		if importSpec.Name == nil {
			names = append(names, "testing")
			continue
		}
		if importSpec.Name.Name != "_" && importSpec.Name.Name != "." {
			names = append(names, importSpec.Name.Name)
		}
	}
	return names, nil
}

func isFuzzTargetFunction(function *ast.FuncDecl, testingNames []string) bool {
	if function.Recv != nil || function.Type.Results != nil || function.Type.Params == nil {
		return false
	}
	if len(function.Type.Params.List) != 1 || len(function.Type.Params.List[0].Names) > 1 {
		return false
	}
	star, ok := function.Type.Params.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	selector, ok := star.X.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "F" {
		return false
	}
	packageName, ok := selector.X.(*ast.Ident)
	if !ok {
		return false
	}
	for _, testingName := range testingNames {
		if packageName.Name == testingName {
			return true
		}
	}
	return false
}

func (run fuzzRun) fuzzArguments() []string {
	return []string{
		"test",
		run.packageName,
		"-run=^$",
		"-fuzz=" + run.fuzzPattern(),
		"-fuzztime=" + run.duration.String(),
		"-parallel=1",
		"-p=1",
	}
}

func (run fuzzRun) fuzzPattern() string {
	return "^" + run.target + "$"
}

func (run fuzzRun) ordinaryReplayCommand(corpusName string) string {
	pattern := run.fuzzPattern()
	if corpusName != "" {
		pattern = "^" + run.target + "/" + corpusName + "$"
	}
	return "go test " + run.packageName + " -count=1 -run=" + shellQuote(pattern)
}

func (run fuzzRun) fuzzReplayCommand() string {
	return "go test " + run.packageName + " -run=" + shellQuote("^$") +
		" -fuzz=" + shellQuote(run.fuzzPattern()) +
		" -fuzztime=" + run.duration.String() + " -parallel=1 -p=1"
}

func (run fuzzRun) failureReplayCommand(corpusName string) string {
	if corpusName != "" {
		return run.ordinaryReplayCommand(corpusName)
	}
	return "no deterministic corpus replay available; rerun fuzz: " + run.fuzzReplayCommand()
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func (a app) executeFuzzRun(root string, run fuzzRun) error {
	sandbox, err := a.prepareFuzzSandbox(root, run)
	if err != nil {
		if sandbox.root != "" {
			cleanupErr := a.cleanupFuzzSandbox(sandbox.root)
			if cleanupErr != nil {
				return errors.Join(err, cleanupErr)
			}
		}
		return err
	}
	output, processErr, processContextErr := a.runFuzzCommand(sandbox, run)
	if processErr != nil {
		return a.finishFuzzFailure(sandbox, run, processContextErr, processErr, output)
	}
	return a.finishFuzzSuccess(sandbox, run)
}

func (a app) finishFuzzFailure(sandbox fuzzSandbox, run fuzzRun, contextErr error, processErr error,
	output string,
) error {
	outcome := classifyFuzzProcess(contextErr, processErr, output, run)
	corpusName, evidenceErr := fuzzReplayCorpusSince(sandbox.source, run.target, output, sandbox.corpusBefore)
	replay := run.failureReplayCommand(corpusName)
	evidencePath := sandbox.source
	result := outcome.diagnostic
	if evidenceErr != nil {
		result = errors.Join(result, newFuzzDiagnostic(fuzzEvidenceCode, evidenceErr, "inspect retained fuzz evidence"))
	}
	if reportErr := a.writeFuzzReport(run, outcome.result, replay, evidencePath); reportErr != nil {
		result = errors.Join(result, reportErr)
	}
	return result
}

func (a app) finishFuzzSuccess(sandbox fuzzSandbox, run fuzzRun) error {
	cleanupErr := a.cleanupFuzzSandbox(sandbox.root)
	if cleanupErr != nil {
		replay := run.ordinaryReplayCommand("")
		reportErr := a.writeFuzzReport(run, "cleanup-failure", replay, sandbox.source)
		if reportErr != nil {
			return errors.Join(cleanupErr, reportErr)
		}
		return cleanupErr
	}
	if err := a.writeFuzzReport(run, "success", run.ordinaryReplayCommand(""), ""); err != nil {
		return err
	}
	return nil
}

func (a app) prepareFuzzSandbox(root string, run fuzzRun) (fuzzSandbox, error) {
	makeTempDir := a.fuzzMakeTempDir
	if makeTempDir == nil {
		temporaryDirectoryErr := validateFuzzSandboxRoot(root, os.TempDir())
		if temporaryDirectoryErr != nil {
			return fuzzSandbox{}, newFuzzDiagnostic(fuzzSandboxCode, temporaryDirectoryErr, "validate fuzz temporary directory")
		}
		makeTempDir = func(pattern string) (string, error) {
			return os.MkdirTemp("", pattern)
		}
	}
	runRoot, err := makeTempDir("goxsd9-fuzz-")
	if err != nil {
		return fuzzSandbox{}, newFuzzDiagnostic(fuzzSandboxCode, err, "create fuzz sandbox")
	}
	sandboxLocationErr := validateFuzzSandboxRoot(root, runRoot)
	if sandboxLocationErr != nil {
		setupErr := newFuzzDiagnostic(fuzzSandboxCode, sandboxLocationErr, "validate fuzz sandbox location")
		cleanupErr := a.cleanupRejectedFuzzSandbox(root, runRoot)
		if cleanupErr != nil {
			return fuzzSandbox{}, errors.Join(setupErr, cleanupErr)
		}
		return fuzzSandbox{}, setupErr
	}
	sandbox := fuzzSandbox{
		root:        runRoot,
		source:      filepath.Join(runRoot, "source"),
		cache:       filepath.Join(runRoot, "cache"),
		tmp:         filepath.Join(runRoot, "tmp"),
		moduleCache: filepath.Join(runRoot, "module-cache"),
	}
	sourceDirectoryErr := os.MkdirAll(sandbox.source, 0o700)
	if sourceDirectoryErr != nil {
		return sandbox, newFuzzDiagnostic(fuzzSandboxCode, sourceDirectoryErr, "create fuzz source directory")
	}
	cacheDirectoryErr := os.MkdirAll(sandbox.cache, 0o700)
	if cacheDirectoryErr != nil {
		return sandbox, newFuzzDiagnostic(fuzzSandboxCode, cacheDirectoryErr, "create fuzz cache directory")
	}
	temporaryDirectoryErr := os.MkdirAll(sandbox.tmp, 0o700)
	if temporaryDirectoryErr != nil {
		return sandbox, newFuzzDiagnostic(fuzzSandboxCode, temporaryDirectoryErr, "create fuzz temporary directory")
	}
	moduleCacheDirectoryErr := os.MkdirAll(sandbox.moduleCache, 0o700)
	if moduleCacheDirectoryErr != nil {
		return sandbox, newFuzzDiagnostic(fuzzSandboxCode, moduleCacheDirectoryErr, "create fuzz module cache directory")
	}
	copyWorktree := a.fuzzCopyWorktree
	if copyWorktree == nil {
		copyWorktree = copyFuzzWorktree
	}
	copyErr := copyWorktree(root, sandbox.source)
	if copyErr != nil {
		return sandbox, newFuzzDiagnostic(fuzzSandboxCode, copyErr, "copy repository into fuzz sandbox")
	}
	corpusBefore, err := fuzzCorpusNames(filepath.Join(sandbox.source, "testdata", "fuzz", run.target))
	if err != nil {
		return sandbox, newFuzzDiagnostic(fuzzSandboxCode, err, "snapshot checked-in fuzz corpus")
	}
	sandbox.corpusBefore = corpusBefore
	return sandbox, nil
}

func validateFuzzSandboxRoot(repositoryRoot, sandboxRoot string) error {
	if !filepath.IsAbs(sandboxRoot) {
		return fmt.Errorf("fuzz sandbox path %q is not absolute", sandboxRoot)
	}
	repositoryPath, err := fuzzResolvedAbsolutePath(repositoryRoot)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	sandboxPath, err := fuzzResolvedAbsolutePath(sandboxRoot)
	if err != nil {
		return fmt.Errorf("resolve fuzz sandbox path: %w", err)
	}
	inside, err := fuzzPathSameOrBelow(repositoryPath, sandboxPath)
	if err != nil {
		return fmt.Errorf("compare repository and fuzz sandbox paths: %w", err)
	}
	if inside {
		return fmt.Errorf("fuzz sandbox path %q is equal to or inside repository root %q", sandboxRoot, repositoryRoot)
	}
	return nil
}

func fuzzResolvedAbsolutePath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func fuzzPathSameOrBelow(base, candidate string) (bool, error) {
	relative, err := filepath.Rel(base, candidate)
	if err != nil {
		return false, err
	}
	if relative == "." {
		return true, nil
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return false, nil
	}
	return !filepath.IsAbs(relative), nil
}

func (a app) cleanupRejectedFuzzSandbox(repositoryRoot, sandboxRoot string) error {
	if !filepath.IsAbs(sandboxRoot) {
		return nil
	}
	repositoryPath, repositoryPathErr := fuzzResolvedAbsolutePath(repositoryRoot)
	if repositoryPathErr != nil {
		return newFuzzDiagnostic(fuzzCleanupCode, repositoryPathErr, "resolve repository root for fuzz cleanup")
	}
	sandboxPath, sandboxPathErr := fuzzResolvedAbsolutePath(sandboxRoot)
	if errors.Is(sandboxPathErr, os.ErrNotExist) {
		return nil
	}
	if sandboxPathErr != nil {
		return newFuzzDiagnostic(fuzzCleanupCode, sandboxPathErr, "resolve fuzz sandbox path for cleanup")
	}
	inside, compareErr := fuzzPathSameOrBelow(repositoryPath, sandboxPath)
	if compareErr != nil {
		return newFuzzDiagnostic(fuzzCleanupCode, compareErr, "compare fuzz cleanup paths")
	}
	if !inside || sandboxPath == repositoryPath {
		return nil
	}
	entries, readErr := os.ReadDir(sandboxRoot)
	if errors.Is(readErr, os.ErrNotExist) || len(entries) != 0 {
		return nil
	}
	if readErr != nil {
		return newFuzzDiagnostic(fuzzCleanupCode, readErr, "inspect rejected fuzz sandbox")
	}
	return a.cleanupFuzzSandbox(sandboxRoot)
}

func (a app) cleanupFuzzSandbox(directory string) error {
	removeAll := a.fuzzRemoveAll
	if removeAll == nil {
		removeAll = os.RemoveAll
	}
	if err := removeAll(directory); err != nil {
		return newFuzzDiagnostic(fuzzCleanupCode, err, "remove fuzz sandbox %q", directory)
	}
	return nil
}

func (a app) runFuzzCommand(sandbox fuzzSandbox, run fuzzRun) (string, error, error) {
	parent := a.ctx
	if parent == nil {
		parent = context.Background()
	}
	// The fixed 30-second margin covers setup and lets go test report after -fuzztime expires.
	commandContext, cancel := context.WithTimeout(parent, fuzzProcessDuration(run.duration))
	defer cancel()
	environment := fuzzGoEnvironment()
	environment = append(environment,
		"GOCACHE="+sandbox.cache,
		"GOTMPDIR="+sandbox.tmp,
		"GOMODCACHE="+sandbox.moduleCache,
	)
	output, err := a.commandOutputWithContextAndEnv(commandContext, sandbox.source, environment, nil,
		"go", run.fuzzArguments()...)
	contextErr := commandContext.Err()
	if err == nil && contextErr != nil {
		return output, contextErr, contextErr
	}
	return output, err, contextErr
}

func fuzzProcessDuration(duration time.Duration) time.Duration {
	return duration + fuzzProcessMargin
}

func classifyFuzzProcess(contextErr error, processErr error, output string, run fuzzRun) fuzzProcessOutcome {
	if errors.Is(contextErr, context.DeadlineExceeded) || errors.Is(processErr, context.DeadlineExceeded) {
		return fuzzProcessOutcome{
			result: "timeout",
			diagnostic: newFuzzDiagnostic(fuzzTimeoutCode, fuzzProcessCause(processErr, output),
				"go test exceeded the %s fuzz bound", run.duration),
		}
	}
	if errors.Is(contextErr, context.Canceled) {
		return fuzzProcessOutcome{
			result: "canceled",
			diagnostic: newFuzzDiagnostic(fuzzCanceledCode, fuzzProcessCause(processErr, output),
				"go test fuzz campaign was canceled"),
		}
	}
	var exitErr *exec.ExitError
	if errors.As(processErr, &exitErr) {
		if exitErr.ExitCode() < 0 {
			return fuzzProcessOutcome{
				result: "process-signaled",
				diagnostic: newFuzzDiagnostic(fuzzProcessSignalCode, fuzzProcessCause(processErr, output),
					"go test was terminated by a signal"),
			}
		}
		return fuzzProcessOutcome{
			result: fmt.Sprintf("process-exit-%d", exitErr.ExitCode()),
			diagnostic: newFuzzDiagnostic(fuzzProcessExitCode, fuzzProcessCause(processErr, output),
				"go test exited with status %d", exitErr.ExitCode()),
		}
	}
	if isFuzzProcessStartError(processErr) {
		return fuzzProcessOutcome{
			result: "process-start-failure",
			diagnostic: newFuzzDiagnostic(fuzzProcessStartCode, fuzzProcessCause(processErr, output),
				"start go test fuzz campaign"),
		}
	}
	return fuzzProcessOutcome{
		result: "process-failure",
		diagnostic: newFuzzDiagnostic(fuzzProcessExitCode, fuzzProcessCause(processErr, output),
			"go test fuzz campaign failed"),
	}
}

func fuzzProcessCause(processErr error, output string) error {
	if processErr == nil {
		processErr = errors.New("go test returned no result")
	}
	if output == "" {
		return processErr
	}
	return fmt.Errorf("%w: child output:\n%s", processErr, output)
}

func isFuzzProcessStartError(err error) bool {
	var commandErr *exec.Error
	if errors.As(err, &commandErr) {
		return true
	}
	var pathErr *os.PathError
	return errors.As(err, &pathErr)
}

func newFuzzDiagnostic(code string, cause error, format string, args ...any) error {
	message := fmt.Sprintf(format, args...)
	if cause != nil {
		message += ": " + cause.Error()
	}
	return &fuzzDiagnostic{code: code, message: message, cause: cause}
}

func (a app) writeFuzzReport(run fuzzRun, result, replay, evidence string) error {
	output := a.stdout
	if output == nil {
		output = io.Discard
	}
	lines := []string{
		"package: " + run.packageName,
		"target: " + run.target,
		fmt.Sprintf("duration: %s", run.duration),
		fmt.Sprintf("workers: %d", run.workers),
		fmt.Sprintf("offline: %t", run.offline),
		"result: " + result,
		"replay command: " + replay,
	}
	if evidence != "" {
		lines = append(lines, "evidence path: "+evidence)
	}
	for _, line := range lines {
		if err := writeLine(output, "%s", line); err != nil {
			return newFuzzDiagnostic(fuzzOutputCode, err, "write fuzz report")
		}
	}
	return nil
}

func fuzzReplayCorpusSince(source, target, output string, before []string) (string, error) {
	corpusDirectory := filepath.Join(source, "testdata", "fuzz", target)
	after, err := fuzzCorpusNames(corpusDirectory)
	if err != nil {
		return "", err
	}
	outputName := fuzzCorpusNameFromOutput(output, target)
	if outputName == "" || !fuzzCorpusContains(after, outputName) || fuzzCorpusContains(before, outputName) {
		return "", nil
	}
	return outputName, nil
}

func fuzzCorpusContains(names []string, candidate string) bool {
	for _, name := range names {
		if name == candidate {
			return true
		}
	}
	return false
}

func fuzzCorpusNames(directory string) ([]string, error) {
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", directory, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return nil, fmt.Errorf("inspect fuzz corpus %s: %w", entry.Name(), infoErr)
		}
		if info.Mode().IsRegular() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func fuzzCorpusNameFromOutput(output, target string) string {
	marker := "testdata/fuzz/" + target + "/"
	normalized := strings.ReplaceAll(output, "\\", "/")
	for _, line := range strings.Split(normalized, "\n") {
		index := strings.Index(line, marker)
		if index < 0 {
			continue
		}
		candidate := strings.TrimSpace(line[index+len(marker):])
		fields := strings.Fields(candidate)
		if len(fields) == 0 {
			continue
		}
		candidate = strings.Trim(fields[0], "\"'`")
		candidate = strings.TrimRight(candidate, ".,:;)]}")
		if candidate != "" && !strings.Contains(candidate, "/") {
			return candidate
		}
	}
	return ""
}

func copyFuzzWorktree(source, destination string) error {
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("stat source worktree: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("source worktree %q is not a directory", source)
	}
	if mkdirErr := os.MkdirAll(destination, 0o700); mkdirErr != nil {
		return fmt.Errorf("create copied worktree: %w", mkdirErr)
	}
	err = filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		return copyFuzzEntry(source, destination, path, entry, walkErr)
	})
	if err != nil {
		return fmt.Errorf("walk source worktree: %w", err)
	}
	return nil
}

func copyFuzzEntry(source, destination, path string, entry fs.DirEntry, walkErr error) error {
	if walkErr != nil {
		return walkErr
	}
	relative, err := filepath.Rel(source, path)
	if err != nil {
		return err
	}
	if relative == "." {
		return nil
	}
	if fuzzCopyExcluded(relative) {
		if entry.IsDir() {
			return filepath.SkipDir
		}
		return nil
	}
	return copyFuzzPath(path, filepath.Join(destination, relative), relative, entry)
}

func copyFuzzPath(source, destination, relative string, entry fs.DirEntry) error {
	if entry.IsDir() {
		entryInfo, err := entry.Info()
		if err != nil {
			return err
		}
		return os.MkdirAll(destination, entryInfo.Mode().Perm())
	}
	if entry.Type()&os.ModeSymlink != 0 {
		return fmt.Errorf("source worktree contains unsupported symlink %s", relative)
	}
	entryInfo, err := entry.Info()
	if err != nil {
		return err
	}
	if !entryInfo.Mode().IsRegular() {
		return fmt.Errorf("source worktree contains non-regular file %s", relative)
	}
	return copyFuzzFile(source, destination, entryInfo.Mode().Perm())
}

func fuzzCopyExcluded(relative string) bool {
	// Keep module source and checked-in corpora while excluding VCS, ignored, and external-suite state.
	relative = filepath.ToSlash(relative)
	for _, excluded := range []string{".git", ".cache", "bin", "testdata/w3c/xsdtests"} {
		if relative == excluded || strings.HasPrefix(relative, excluded+"/") {
			return true
		}
	}
	return false
}

func copyFuzzFile(source, destination string, mode fs.FileMode) error {
	// #nosec G304 -- both paths are produced by the selected worktree walk.
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	// #nosec G304 -- both paths are produced by the selected worktree walk.
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		inputCloseErr := input.Close()
		if inputCloseErr != nil {
			return errors.Join(err, inputCloseErr)
		}
		return err
	}
	_, copyErr := io.Copy(output, input)
	inputCloseErr := input.Close()
	outputCloseErr := output.Close()
	return errors.Join(copyErr, inputCloseErr, outputCloseErr)
}
