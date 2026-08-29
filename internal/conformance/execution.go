package conformance

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/goxdra/goxsd9"
)

// Selector identifies one bounded schema-only catalog selection. Version is
// required and must be exactly 1.0 or 1.1. At least SetPath or CaseName must
// be provided; an empty selector never means the complete suite.
type Selector struct {
	Version   string
	SetPath   string
	GroupName string
	CaseName  string
}

// Selection is an immutable, ordered set of schemaTest cases selected from an
// Inventory. Construct it with Inventory.Select.
type Selection struct {
	selector Selector
	policy   goxsd9.LanguagePolicy
	cases    []catalogCase
}

// Version returns the exact catalog edition selected for the schema run.
func (selection Selection) Version() string {
	return selection.selector.Version
}

// Policy returns the strict parser policy selected from Version.
func (selection Selection) Policy() goxsd9.LanguagePolicy {
	return selection.policy
}

// Selector returns the bounded selector used to construct the selection.
func (selection Selection) Selector() Selector {
	return selection.selector
}

// Len returns the number of ordered schemaTest cases in the selection.
func (selection Selection) Len() int {
	return len(selection.cases)
}

// Outcome identifies the independently classified result of one selected
// schema case.
type Outcome string

const (
	// OutcomePass means parser-established validity matches the catalog
	// expectation.
	OutcomePass Outcome = "pass"
	// OutcomeConformanceFailure means the parser result disagreed with the
	// expected validity.
	OutcomeConformanceFailure Outcome = "conformance-failure"
	// OutcomeUnsupported means the parser reported unfinished specification
	// behavior.
	OutcomeUnsupported Outcome = "unsupported"
	// OutcomeResolutionFailure means a schema source could not be resolved,
	// read, or closed.
	OutcomeResolutionFailure Outcome = "resolution-failure"
	// OutcomeInternalFailure means the parser or execution boundary reported an
	// implementation invariant failure.
	OutcomeInternalFailure Outcome = "internal-failure"
	// OutcomeNotExecuted means catalog metadata or the bounded harness prevented
	// parser execution.
	OutcomeNotExecuted Outcome = "not-executed"
)

// ActualResult identifies what the parser boundary actually did, separately
// from the catalog's expected validity and the classified Outcome.
type ActualResult string

const (
	// ActualValid means ParseSchemaWithPolicy accepted the schema.
	ActualValid ActualResult = "valid"
	// ActualInvalid means ParseSchemaWithPolicy returned an invalid-schema
	// diagnostic.
	ActualInvalid ActualResult = "invalid"
	// ActualUnknown means the parser did not establish schema validity because
	// execution failed with a non-invalid diagnostic or at the resource boundary.
	ActualUnknown ActualResult = "unknown"
	// ActualNotExecuted means no parser call was made for the case.
	ActualNotExecuted ActualResult = "not-executed"
)

// CaseResult contains the ordered catalog facts and execution result for one
// schemaTest. Catalog facts remain separate from parser outcome.
type CaseResult struct {
	setPath          string
	setName          string
	groupName        string
	caseName         string
	origin           catalogOrigin
	version          string
	policy           goxsd9.LanguagePolicy
	status           catalogStatus
	documents        []string
	expected         []string
	expectedValidity string
	usable           bool
	usableReasons    []string
	actual           ActualResult
	actualClass      goxsd9.FailureClass
	outcome          Outcome
	headline         bool
	diagnostics      []goxsd9.Diagnostic
	err              error
	executionReason  string
}

// SetPath returns the pinned catalog test-set path.
func (result CaseResult) SetPath() string {
	return result.setPath
}

// SetName returns the catalog test-set identity.
func (result CaseResult) SetName() string {
	return result.setName
}

// GroupName returns the catalog test-group identity.
func (result CaseResult) GroupName() string {
	return result.groupName
}

// CaseName returns the catalog schema-test identity.
func (result CaseResult) CaseName() string {
	return result.caseName
}

// Origin returns main or auxiliary catalog provenance.
func (result CaseResult) Origin() string {
	return string(result.origin)
}

// Version returns the strict catalog edition used for this case.
func (result CaseResult) Version() string {
	return result.version
}

// Policy returns the strict parser policy used for this case.
func (result CaseResult) Policy() goxsd9.LanguagePolicy {
	return result.policy
}

// Status returns the catalog's current status without deriving it from the
// execution outcome.
func (result CaseResult) Status() string {
	return string(result.status)
}

// Documents returns the ordered schema-document paths listed by the catalog.
func (result CaseResult) Documents() []string {
	return append([]string(nil), result.documents...)
}

// ExpectedValidity returns the one expected validity applicable to Version,
// or an empty string when the catalog has no unique applicable expectation.
func (result CaseResult) ExpectedValidity() string {
	return result.expectedValidity
}

// ExpectedValidities returns all applicable expected validity values in
// catalog order. Multiple values remain observable instead of being collapsed.
func (result CaseResult) ExpectedValidities() []string {
	return append([]string(nil), result.expected...)
}

// Usable reports whether the catalog metadata permits execution.
func (result CaseResult) Usable() bool {
	return result.usable
}

// UsabilityReasons returns catalog reasons that prevented execution.
func (result CaseResult) UsabilityReasons() []string {
	return append([]string(nil), result.usableReasons...)
}

// Actual returns parser-established validity independently of expected
// validity and Outcome.
func (result CaseResult) Actual() ActualResult {
	return result.actual
}

// ActualClass returns the parser or execution failure class, when one exists.
// It does not derive a validity value from the failure class.
func (result CaseResult) ActualClass() goxsd9.FailureClass {
	return result.actualClass
}

// Outcome returns the independently classified execution result.
func (result CaseResult) Outcome() Outcome {
	return result.outcome
}

// HeadlineEligible reports catalog eligibility for headline evidence. It does
// not turn an execution failure into a pass and is false for auxiliary,
// disputed, queried, or unusable cases.
func (result CaseResult) HeadlineEligible() bool {
	return result.headline
}

// ExecutionReason returns a stable reason for a case that was not executed.
func (result CaseResult) ExecutionReason() string {
	return result.executionReason
}

// Diagnostics returns parser diagnostics in their original processing order.
func (result CaseResult) Diagnostics() []goxsd9.Diagnostic {
	return append([]goxsd9.Diagnostic(nil), result.diagnostics...)
}

// Cause returns the parser or pinned-resource error, preserving its unwrap
// chain. It is nil when the parser accepted the schema or the case was not
// executed because it was unusable.
func (result CaseResult) Cause() error {
	return result.err
}

// Report is the deterministic result of one bounded schema execution.
type Report struct {
	selector Selector
	policy   goxsd9.LanguagePolicy
	cases    []CaseResult
}

// Version returns the exact catalog edition used by the report.
func (report Report) Version() string {
	return report.selector.Version
}

// Policy returns the strict parser policy used by the report.
func (report Report) Policy() goxsd9.LanguagePolicy {
	return report.policy
}

// Selector returns the bounded selector used by the report.
func (report Report) Selector() Selector {
	return report.selector
}

// Len returns the number of ordered case results.
func (report Report) Len() int {
	return len(report.cases)
}

// Cases returns a copy of the ordered case results.
func (report Report) Cases() []CaseResult {
	result := make([]CaseResult, 0, len(report.cases))
	for _, caseResult := range report.cases {
		result = append(result, cloneCaseResult(caseResult))
	}
	return result
}

// Case returns the case result at index and reports whether it exists.
func (report Report) Case(index int) (CaseResult, bool) {
	if index < 0 || index >= len(report.cases) {
		return CaseResult{}, false
	}
	return cloneCaseResult(report.cases[index]), true
}

// HeadlineCount counts cases eligible for headline evidence without considering
// whether their execution outcome passed.
func (report Report) HeadlineCount() int {
	count := 0
	for _, caseResult := range report.cases {
		if caseResult.headline {
			count++
		}
	}
	return count
}

// Select validates a bounded selector, selects schemaTest cases in catalog
// discovery order, and chooses its strict parser policy before execution.
func (inventory Inventory) Select(selector Selector) (Selection, error) {
	if err := selector.validate(); err != nil {
		return Selection{}, catalogError("catalog.selector", selector.SetPath, err)
	}
	policy, err := LanguagePolicyForVersions([]string{selector.Version})
	if err != nil {
		return Selection{}, err
	}

	selected := make([]catalogCase, 0)
	for _, caseValue := range inventory.cases {
		if caseValue.kind != schemaKind || !selector.matches(caseValue) || !caseApplies(caseValue, selector.Version) {
			continue
		}
		selected = append(selected, cloneCatalogCase(caseValue))
	}
	if len(selected) == 0 {
		return Selection{}, catalogError("catalog.selector", selector.SetPath,
			fmt.Errorf("no applicable schemaTest matches set=%q group=%q case=%q for version %q",
				selector.SetPath, selector.GroupName, selector.CaseName, selector.Version))
	}
	if selector.CaseName != "" && len(selected) != 1 {
		return Selection{}, catalogError("catalog.selector", selector.SetPath,
			fmt.Errorf("schemaTest selector is ambiguous for case %q", selector.CaseName))
	}
	return Selection{selector: selector, policy: policy, cases: selected}, nil
}

func (selector Selector) validate() error {
	if selector.Version == "" {
		return errors.New("schema selector requires one exact version")
	}
	if err := validateSelectorText("version", selector.Version); err != nil {
		return err
	}
	if selector.SetPath == "" && selector.CaseName == "" {
		return errors.New("schema selector requires SetPath or CaseName; full-suite selection is not allowed")
	}
	if selector.GroupName != "" && selector.SetPath == "" {
		return errors.New("schema selector GroupName requires SetPath")
	}
	if err := validateSelectorPath(selector.SetPath); err != nil {
		return err
	}
	if err := validateOptionalSelectorText("GroupName", selector.GroupName); err != nil {
		return err
	}
	if err := validateOptionalSelectorText("CaseName", selector.CaseName); err != nil {
		return err
	}
	return nil
}

func validateSelectorText(name, value string) error {
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("schema selector %s %q has surrounding whitespace", name, value)
	}
	return nil
}

func validateOptionalSelectorText(name, value string) error {
	if value == "" {
		return nil
	}
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("schema selector %s is empty", name)
	}
	return validateSelectorText(name, value)
}

func validateSelectorPath(path string) error {
	if path == "" {
		return nil
	}
	if !fs.ValidPath(path) || path == "." {
		return fmt.Errorf("schema selector SetPath %q is not a catalog path", path)
	}
	return nil
}

func (selector Selector) matches(caseValue catalogCase) bool {
	if selector.SetPath != "" && caseValue.setPath != selector.SetPath {
		return false
	}
	if selector.GroupName != "" && caseValue.groupName != selector.GroupName {
		return false
	}
	if selector.CaseName != "" && caseValue.name != selector.CaseName {
		return false
	}
	return true
}

// Execute resolves and parses the selected schema graphs solely through fsys.
// It processes cases sequentially and returns a report containing per-case
// failures; selector, context, and filesystem contract errors return no report.
func (selection Selection) Execute(ctx context.Context, fsys fs.FS) (Report, error) {
	if ctx == nil {
		return Report{}, executionErrorFor("execution.context", "", errors.New("nil execution context"))
	}
	if fsys == nil {
		return Report{}, executionErrorFor("execution.filesystem", "", errors.New("nil pinned filesystem"))
	}
	if selection.policy == "" || len(selection.cases) == 0 {
		return Report{}, catalogError("catalog.selector", selection.selector.SetPath,
			errors.New("cannot execute an empty schema selection"))
	}

	report := Report{
		selector: selection.selector,
		policy:   selection.policy,
		cases:    make([]CaseResult, 0, len(selection.cases)),
	}
	resolver := pinnedResolver{fsys: fsys}
	for _, caseValue := range selection.cases {
		report.cases = append(report.cases, executeCase(ctx, fsys, resolver, selection.policy, selection.selector.Version, caseValue))
	}
	return report, nil
}

func executeCase(ctx context.Context, fsys fs.FS, resolver pinnedResolver, policy goxsd9.LanguagePolicy,
	version string, caseValue catalogCase,
) CaseResult {
	expected, expectedValidity, usable, reasons := caseFacts(caseValue, version)
	result := CaseResult{
		setPath:          caseValue.setPath,
		setName:          caseValue.setName,
		groupName:        caseValue.groupName,
		caseName:         caseValue.name,
		origin:           caseValue.origin,
		version:          version,
		policy:           policy,
		status:           caseValue.status,
		documents:        append([]string(nil), caseValue.documents...),
		expected:         expected,
		expectedValidity: expectedValidity,
		usable:           usable,
		usableReasons:    reasons,
		actual:           ActualNotExecuted,
		outcome:          OutcomeNotExecuted,
		headline:         usable && caseValue.isHeadline(version, expectedValidity),
	}
	if !usable {
		return result
	}
	if len(caseValue.documents) == 0 {
		result.executionReason = "schema-document-missing"
		return result
	}

	root, graphResolver, sourceClass, sourceErr := openSchemaGraph(ctx, fsys, resolver, caseValue.documents)
	if sourceErr != nil {
		result.actual = ActualUnknown
		result.actualClass = sourceClass
		result.outcome = executionFailureOutcome(sourceClass)
		result.err = sourceErr
		return result
	}

	_, parseErr := goxsd9.ParseSchemaWithPolicy(root, graphResolver, policy)
	if parseErr == nil {
		result.actual = ActualValid
		result.outcome = compareExpectedValidity(expectedValidity, result.actual)
		return result
	}

	result.err = parseErr
	result.diagnostics = parserDiagnostics(parseErr)
	result.actual, result.actualClass, result.outcome = classifyParserFailure(result.diagnostics, expectedValidity)
	return result
}

const (
	multiDocumentRootPath = "__goxsd9_conformance_multi_document_root__.xsd"
	xsdNamespaceURI       = "http://www.w3.org/2001/XMLSchema"
)

type preparedSchemaDocuments struct {
	documents []preparedSchemaDocument
	sources   map[string]preparedSchemaSource
	failures  map[string]error
}

type preparedSchemaDocument struct {
	path                   string
	targetNamespace        string
	targetNamespacePresent bool
}

type preparedSchemaSource struct {
	data                   []byte
	readErr                error
	closeErr               error
	targetNamespace        string
	targetNamespacePresent bool
}

func (source preparedSchemaSource) reader() io.ReadCloser {
	return &preparedSchemaReader{
		Reader:   bytes.NewReader(source.data),
		readErr:  source.readErr,
		closeErr: source.closeErr,
	}
}

type preparedSchemaReader struct {
	*bytes.Reader
	readErr           error
	closeErr          error
	readErrorReturned bool
	closed            bool
}

func (reader *preparedSchemaReader) Read(buffer []byte) (int, error) {
	read, err := reader.Reader.Read(buffer)
	if err != io.EOF || reader.readErr == nil || reader.readErrorReturned {
		return read, err
	}
	reader.readErrorReturned = true
	return read, reader.readErr
}

func (reader *preparedSchemaReader) Close() error {
	if reader.closed {
		return nil
	}
	reader.closed = true
	return reader.closeErr
}

func openSchemaGraph(ctx context.Context, fsys fs.FS, resolver pinnedResolver, documents []string) (
	goxsd9.ResolvedSource, pinnedResolver, goxsd9.FailureClass, error,
) {
	if len(documents) == 1 {
		root, class, err := openSingleSchemaSource(ctx, fsys, documents[0])
		return root, resolver, class, err
	}

	prepared := prepareSchemaDocuments(fsys, documents)
	rootContents, err := multiDocumentRootContents(prepared.documents)
	if err != nil {
		return goxsd9.ResolvedSource{}, resolver, goxsd9.FailureInternal,
			executionErrorFor("execution.source", multiDocumentRootPath, err)
	}
	rootReader := io.NopCloser(strings.NewReader(rootContents))
	rootContext := context.WithValue(ctx, pinnedSourceContextKey{}, multiDocumentRootPath)
	root, err := goxsd9.NewResolvedSource(rootContext, goxsd9.SourceID(multiDocumentRootPath), rootReader)
	if err != nil {
		closeErr := rootReader.Close()
		if closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close schema source after construction failure: %w", closeErr))
		}
		return goxsd9.ResolvedSource{}, resolver, goxsd9.FailureInternal,
			executionErrorFor("execution.source", multiDocumentRootPath, err)
	}
	resolver.prepared = prepared.sources
	resolver.failures = prepared.failures
	return root, resolver, "", nil
}

func openSingleSchemaSource(ctx context.Context, fsys fs.FS, rootPath string) (goxsd9.ResolvedSource, goxsd9.FailureClass, error) {
	file, err := fsys.Open(rootPath)
	if err != nil {
		return goxsd9.ResolvedSource{}, goxsd9.FailureResolution, executionErrorFor("execution.source", rootPath, err)
	}
	if file == nil {
		return goxsd9.ResolvedSource{}, goxsd9.FailureInternal,
			executionErrorFor("execution.source", rootPath, errors.New("pinned filesystem returned a nil schema file"))
	}

	rootContext := context.WithValue(ctx, pinnedSourceContextKey{}, rootPath)
	root, err := goxsd9.NewResolvedSource(rootContext, goxsd9.SourceID(rootPath), file)
	if err == nil {
		return root, "", nil
	}
	closeErr := file.Close()
	if closeErr != nil {
		err = errors.Join(err, fmt.Errorf("close schema source after construction failure: %w", closeErr))
	}
	return goxsd9.ResolvedSource{}, goxsd9.FailureInternal, executionErrorFor("execution.source", rootPath, err)
}

func executionFailureOutcome(class goxsd9.FailureClass) Outcome {
	switch class {
	case goxsd9.FailureResolution:
		return OutcomeResolutionFailure
	case goxsd9.FailureInternal:
		return OutcomeInternalFailure
	case goxsd9.FailureInvalid, goxsd9.FailureUnsupported:
		return OutcomeInternalFailure
	}
	return OutcomeInternalFailure
}

func prepareSchemaDocuments(fsys fs.FS, paths []string) preparedSchemaDocuments {
	prepared := preparedSchemaDocuments{
		documents: make([]preparedSchemaDocument, 0, len(paths)),
		sources:   make(map[string]preparedSchemaSource, len(paths)),
		failures:  make(map[string]error),
	}
	for index, documentPath := range paths {
		if source, ok := prepared.sources[documentPath]; ok {
			prepared.documents = append(prepared.documents, preparedSchemaDocument{
				path:                   documentPath,
				targetNamespace:        source.targetNamespace,
				targetNamespacePresent: source.targetNamespacePresent,
			})
			continue
		}
		source, err := readPreparedSchemaSource(fsys, documentPath)
		if err != nil {
			prepared.failures[documentPath] = err
			prepared.documents = append(prepared.documents, preparedSchemaDocument{path: documentPath})
			appendUnpreparedSchemaDocuments(&prepared, paths[index+1:])
			break
		}
		source.targetNamespace, source.targetNamespacePresent = schemaTargetNamespace(source.data)
		prepared.sources[documentPath] = source
		prepared.documents = append(prepared.documents, preparedSchemaDocument{
			path:                   documentPath,
			targetNamespace:        source.targetNamespace,
			targetNamespacePresent: source.targetNamespacePresent,
		})
		if source.readErr != nil || source.closeErr != nil {
			appendUnpreparedSchemaDocuments(&prepared, paths[index+1:])
			break
		}
	}
	return prepared
}

func appendUnpreparedSchemaDocuments(prepared *preparedSchemaDocuments, paths []string) {
	for _, documentPath := range paths {
		prepared.documents = append(prepared.documents, preparedSchemaDocument{path: documentPath})
	}
}

func readPreparedSchemaSource(fsys fs.FS, documentPath string) (preparedSchemaSource, error) {
	file, err := fsys.Open(documentPath)
	if err != nil {
		return preparedSchemaSource{}, fmt.Errorf("open pinned schema resource %q: %w", documentPath, err)
	}
	if file == nil {
		return preparedSchemaSource{}, fmt.Errorf("open pinned schema resource %q: nil file", documentPath)
	}
	data, readErr := io.ReadAll(file)
	closeErr := file.Close()
	return preparedSchemaSource{data: data, readErr: readErr, closeErr: closeErr}, nil
}

func schemaTargetNamespace(data []byte) (string, bool) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, err := decoder.Token()
		if err != nil {
			return "", false
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Space != xsdNamespaceURI || start.Name.Local != "schema" {
			return "", false
		}
		for _, attribute := range start.Attr {
			if attribute.Name.Space == "" && attribute.Name.Local == "targetNamespace" {
				return attribute.Value, true
			}
		}
		return "", true
	}
}

func multiDocumentRootContents(documents []preparedSchemaDocument) (string, error) {
	var builder strings.Builder
	builder.WriteString(`<xs:schema xmlns:xs="` + xsdNamespaceURI + `">`)
	for _, document := range documents {
		if document.targetNamespacePresent && strings.TrimSpace(document.targetNamespace) != "" {
			builder.WriteString(`<xs:import namespace="`)
			if err := xml.EscapeText(&builder, []byte(document.targetNamespace)); err != nil {
				return "", fmt.Errorf("escape target namespace %q: %w", document.targetNamespace, err)
			}
			builder.WriteString(`" schemaLocation="`)
		}
		if !document.targetNamespacePresent || strings.TrimSpace(document.targetNamespace) == "" {
			builder.WriteString(`<xs:include schemaLocation="`)
		}
		if err := xml.EscapeText(&builder, []byte(document.path)); err != nil {
			return "", fmt.Errorf("escape schema document path %q: %w", document.path, err)
		}
		builder.WriteString(`"/>`)
	}
	builder.WriteString(`</xs:schema>`)
	return builder.String(), nil
}

func compareExpectedValidity(expected string, actual ActualResult) Outcome {
	if (expected == "valid" && actual == ActualValid) || (expected == "invalid" && actual == ActualInvalid) {
		return OutcomePass
	}
	return OutcomeConformanceFailure
}

func caseFacts(caseValue catalogCase, version string) ([]string, string, bool, []string) {
	expected := make([]string, 0, len(caseValue.expectations))
	for _, expectation := range caseValue.expectations {
		if expectedApplies(expectation, caseValue.parentVersions, version) {
			expected = append(expected, expectation.validity)
		}
	}
	reasons := append([]string(nil), caseValue.usableReasons...)
	if len(expected) == 0 && len(caseValue.expectations) != 0 {
		addReason(&reasons, "expected-version-mismatch")
	}
	if len(expected) > 1 {
		addReason(&reasons, "expected-ambiguous")
	}
	if len(expected) == 1 && !isKnownOutcome(expected[0]) {
		addReason(&reasons, "outcome-unsupported")
	}
	validity := ""
	if len(expected) == 1 {
		validity = expected[0]
	}
	return expected, validity, len(reasons) == 0, reasons
}

func addReason(reasons *[]string, reason string) {
	for _, current := range *reasons {
		if current == reason {
			return
		}
	}
	*reasons = append(*reasons, reason)
}

func classifyParserFailure(diagnostics []goxsd9.Diagnostic, expectedValidity string) (ActualResult, goxsd9.FailureClass, Outcome) {
	if len(diagnostics) == 0 {
		return ActualUnknown, goxsd9.FailureInternal, OutcomeInternalFailure
	}
	class := goxsd9.FailureInvalid
	for _, diagnostic := range diagnostics {
		if diagnostic.Class() == goxsd9.FailureInvalid {
			continue
		}
		class = diagnostic.Class()
		break
	}
	switch class {
	case goxsd9.FailureUnsupported:
		return ActualUnknown, class, OutcomeUnsupported
	case goxsd9.FailureResolution:
		return ActualUnknown, class, OutcomeResolutionFailure
	case goxsd9.FailureInvalid:
		return ActualInvalid, class, compareExpectedValidity(expectedValidity, ActualInvalid)
	case goxsd9.FailureInternal:
		return ActualUnknown, class, OutcomeInternalFailure
	default:
		return ActualUnknown, goxsd9.FailureInternal, OutcomeInternalFailure
	}
}

func parserDiagnostics(err error) []goxsd9.Diagnostic {
	var aggregate goxsd9.Diagnostics
	if errors.As(err, &aggregate) {
		return aggregate.All()
	}
	var aggregatePointer *goxsd9.Diagnostics
	if errors.As(err, &aggregatePointer) && aggregatePointer != nil {
		return aggregatePointer.All()
	}
	var diagnostic goxsd9.Diagnostic
	if errors.As(err, &diagnostic) {
		return []goxsd9.Diagnostic{diagnostic}
	}
	var diagnosticPointer *goxsd9.Diagnostic
	if errors.As(err, &diagnosticPointer) && diagnosticPointer != nil {
		return []goxsd9.Diagnostic{*diagnosticPointer}
	}
	return nil
}

type pinnedSourceContextKey struct{}

type pinnedResolver struct {
	fsys     fs.FS
	prepared map[string]preparedSchemaSource
	failures map[string]error
}

func (resolver pinnedResolver) Resolve(ctx context.Context, _ string, schemaLocation string) (goxsd9.ResolvedSource, error) {
	basePath, ok := ctx.Value(pinnedSourceContextKey{}).(string)
	if !ok || basePath == "" {
		return goxsd9.ResolvedSource{}, errors.New("pinned resolver context has no source path")
	}
	resolvedPath, err := resolveReference(basePath, schemaLocation)
	if err != nil {
		return goxsd9.ResolvedSource{}, fmt.Errorf("resolve pinned schema location %q: %w", schemaLocation, err)
	}
	if failure, ok := resolver.failures[resolvedPath]; ok {
		return goxsd9.ResolvedSource{}, failure
	}
	childContext := context.WithValue(ctx, pinnedSourceContextKey{}, resolvedPath)
	if prepared, ok := resolver.prepared[resolvedPath]; ok {
		return newPinnedResolvedSource(childContext, resolvedPath, prepared.reader())
	}
	file, err := resolver.fsys.Open(resolvedPath)
	if err != nil {
		return goxsd9.ResolvedSource{}, fmt.Errorf("open pinned schema resource %q: %w", resolvedPath, err)
	}
	if file == nil {
		return goxsd9.ResolvedSource{}, fmt.Errorf("open pinned schema resource %q: nil file", resolvedPath)
	}
	return newPinnedResolvedSource(childContext, resolvedPath, file)
}

func newPinnedResolvedSource(ctx context.Context, path string, reader io.ReadCloser) (goxsd9.ResolvedSource, error) {
	source, err := goxsd9.NewResolvedSource(ctx, goxsd9.SourceID(path), reader)
	if err == nil {
		return source, nil
	}
	closeErr := reader.Close()
	if closeErr != nil {
		return goxsd9.ResolvedSource{}, errors.Join(err, fmt.Errorf("close pinned schema resource %q: %w", path, closeErr))
	}
	return goxsd9.ResolvedSource{}, err
}

type executionError struct {
	code string
	path string
	err  error
}

func (err *executionError) Error() string {
	if err.path == "" {
		return fmt.Sprintf("[%s] %v", err.code, err.err)
	}
	return fmt.Sprintf("[%s] %s: %v", err.code, err.path, err.err)
}

func (err *executionError) Unwrap() error {
	return err.err
}

func executionErrorFor(code, resourcePath string, cause error) error {
	return &executionError{code: code, path: resourcePath, err: cause}
}

func cloneCatalogCase(caseValue catalogCase) catalogCase {
	clone := caseValue
	clone.parentVersions = append([]string(nil), caseValue.parentVersions...)
	clone.documents = append([]string(nil), caseValue.documents...)
	clone.expectations = append([]expectation(nil), caseValue.expectations...)
	clone.usableReasons = append([]string(nil), caseValue.usableReasons...)
	return clone
}

func cloneCaseResult(result CaseResult) CaseResult {
	clone := result
	clone.documents = append([]string(nil), result.documents...)
	clone.expected = append([]string(nil), result.expected...)
	clone.usableReasons = append([]string(nil), result.usableReasons...)
	clone.diagnostics = append([]goxsd9.Diagnostic(nil), result.diagnostics...)
	return clone
}

// Write prints a deterministic report in selection order. It never reports a
// catalog row as a parser pass solely because its expected validity was valid.
func (report Report) Write(w io.Writer) error {
	if w == nil {
		return errors.New("write execution report: nil writer")
	}
	if _, err := fmt.Fprintln(w, "W3C XML Schema bounded schema execution"); err != nil {
		return fmt.Errorf("write execution heading: %w", err)
	}
	if _, err := fmt.Fprintf(w, "version: %s\npolicy: %s\n", report.selector.Version, report.policy); err != nil {
		return fmt.Errorf("write execution policy: %w", err)
	}
	if _, err := fmt.Fprintf(w, "selection: set=%q group=%q case=%q\n", report.selector.SetPath, report.selector.GroupName, report.selector.CaseName); err != nil {
		return fmt.Errorf("write execution selection: %w", err)
	}
	if _, err := fmt.Fprintf(w, "cases: %d\nheadline-eligible: %d\n\n", len(report.cases), report.HeadlineCount()); err != nil {
		return fmt.Errorf("write execution summary: %w", err)
	}
	for index, result := range report.cases {
		if err := writeCaseResult(w, index+1, result); err != nil {
			return err
		}
	}
	return nil
}

func writeCaseResult(w io.Writer, index int, result CaseResult) error {
	reasons := strings.Join(result.usableReasons, ",")
	if reasons == "" {
		reasons = "-"
	}
	expected := strings.Join(result.expected, ",")
	if expected == "" {
		expected = "-"
	}
	actualClass := string(result.actualClass)
	if actualClass == "" {
		actualClass = "-"
	}
	if _, err := fmt.Fprintf(w,
		"case %d set=%q set-name=%q group=%q name=%q origin=%s version=%s policy=%s status=%s expected=%s expected-validity=%q usable=%t reasons=%q actual=%s actual-class=%s outcome=%s headline=%t execution-reason=%q documents=%d\n",
		index, result.setPath, result.setName, result.groupName, result.caseName, result.origin,
		result.version, result.policy, result.status, expected, result.expectedValidity, result.usable,
		reasons, result.actual, actualClass, result.outcome, result.headline, result.executionReason, len(result.documents)); err != nil {
		return fmt.Errorf("write case %d: %w", index, err)
	}
	for documentIndex, document := range result.documents {
		if _, err := fmt.Fprintf(w, "document case=%d index=%d path=%q\n", index, documentIndex+1, document); err != nil {
			return fmt.Errorf("write case %d document %d: %w", index, documentIndex+1, err)
		}
	}
	for diagnosticIndex, diagnostic := range result.diagnostics {
		cause := ""
		if diagnostic.Unwrap() != nil {
			cause = diagnostic.Unwrap().Error()
		}
		if _, err := fmt.Fprintf(w,
			"diagnostic case=%d index=%d class=%s code=%q loc=%q spec=%q message=%q cause=%q\n",
			index, diagnosticIndex+1, diagnostic.Class(), diagnostic.Code(), diagnostic.Loc().String(),
			diagnostic.SpecRef(), diagnostic.Message(), cause); err != nil {
			return fmt.Errorf("write case %d diagnostic %d: %w", index, diagnosticIndex+1, err)
		}
	}
	if result.err != nil {
		if _, err := fmt.Fprintf(w, "error case=%d value=%q\n", index, result.err.Error()); err != nil {
			return fmt.Errorf("write case %d error: %w", index, err)
		}
	}
	return nil
}
