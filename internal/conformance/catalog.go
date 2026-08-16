// Package conformance reads the pinned W3C XML Schema test catalogs.
package conformance

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
)

const (
	testSuiteNamespace = "http://www.w3.org/XML/2004/xml-schema-test-suite/"
	xlinkNamespace     = "http://www.w3.org/1999/xlink"
)

// CatalogError identifies a malformed catalog or an unreadable catalog
// resource. Code is stable so callers can classify failures without parsing
// the human-readable message.
type CatalogError struct {
	Code string
	Path string
	Err  error
}

func (e *CatalogError) Error() string {
	if e.Path == "" {
		return fmt.Sprintf("[%s] %v", e.Code, e.Err)
	}
	return fmt.Sprintf("[%s] %s: %v", e.Code, e.Path, e.Err)
}

func (e *CatalogError) Unwrap() error {
	return e.Err
}

// Inventory contains the ordered metadata discovered in the pinned test
// catalogs. Its report is calculated when Write is called.
type Inventory struct {
	setPaths []string
	cases    []catalogCase
}

// Read reads the pinned suite.xml and extra-suite.xml roots from fsys.
// Referenced test sets are read in catalog order and duplicate paths are
// included only once.
func Read(fsys fs.FS) (Inventory, error) {
	if fsys == nil {
		return Inventory{}, catalogError("catalog.read", "", errors.New("nil filesystem"))
	}
	reader := catalogReader{fsys: fsys}
	return reader.read()
}

// ReadDirectory reads the pinned catalogs below root.
func ReadDirectory(root string) (Inventory, error) {
	return Read(os.DirFS(root))
}

// Write prints a stable, human- and machine-readable inventory.
func (inventory Inventory) Write(w io.Writer) error {
	if w == nil {
		return errors.New("write inventory: nil writer")
	}

	rows := inventory.rows()
	queried, disputed, unusable := inventory.summary()
	if _, err := fmt.Fprintln(w, "W3C XML Schema test catalog inventory"); err != nil {
		return fmt.Errorf("write inventory heading: %w", err)
	}
	if _, err := fmt.Fprintf(w, "test-sets: %d\n", len(inventory.setPaths)); err != nil {
		return fmt.Errorf("write test-set count: %w", err)
	}
	if _, err := fmt.Fprintf(w, "cases: %d\n", len(inventory.cases)); err != nil {
		return fmt.Errorf("write case count: %w", err)
	}
	if _, err := fmt.Fprintf(w, "queried: %d\ndisputed: %d\nunusable: %d\n", queried, disputed, unusable); err != nil {
		return fmt.Errorf("write catalog summary: %w", err)
	}
	if _, err := fmt.Fprintln(w, ""); err != nil {
		return fmt.Errorf("write inventory separator: %w", err)
	}
	if _, err := fmt.Fprintln(w, "version kind cases valid invalid other queried disputed-test disputed-spec status-missing unusable headline"); err != nil {
		return fmt.Errorf("write inventory columns: %w", err)
	}
	for _, row := range rows {
		if _, err := fmt.Fprintf(w, "%s %s %d %d %d %d %d %d %d %d %d %d\n",
			row.version, row.kind, row.cases, row.valid, row.invalid, row.other,
			row.queried, row.disputedTest, row.disputedSpec, row.statusMissing,
			row.unusable, row.headline); err != nil {
			return fmt.Errorf("write %s %s inventory row: %w", row.version, row.kind, err)
		}
	}
	if _, err := fmt.Fprintln(w, ""); err != nil {
		return fmt.Errorf("write status heading separator: %w", err)
	}
	if _, err := fmt.Fprintln(w, "# Outcome and status columns are independent; headline excludes queried, disputed, and unusable cases."); err != nil {
		return fmt.Errorf("write inventory note: %w", err)
	}
	return nil
}

type catalogReader struct {
	fsys fs.FS
}

func (reader catalogReader) read() (Inventory, error) {
	inventory := Inventory{}
	seen := make(map[string]struct{})
	for _, root := range []string{"suite.xml", "extra-suite.xml"} {
		suite, err := reader.readSuite(root)
		if err != nil {
			return Inventory{}, err
		}
		for _, ref := range suite.refs {
			setPath, err := resolveReference(root, ref)
			if err != nil {
				return Inventory{}, catalogError("catalog.reference", root, err)
			}
			if _, ok := seen[setPath]; ok {
				continue
			}
			seen[setPath] = struct{}{}
			set, err := reader.readTestSet(setPath, suite.versions)
			if err != nil {
				return Inventory{}, err
			}
			inventory.setPaths = append(inventory.setPaths, setPath)
			inventory.cases = append(inventory.cases, set.cases...)
		}
	}
	return inventory, nil
}

type suiteDocument struct {
	versions []string
	refs     []string
}

type testSetDocument struct {
	name  string
	cases []catalogCase
}

type catalogCase struct {
	setPath        string
	setName        string
	groupName      string
	name           string
	kind           caseKind
	parentVersions []string
	documents      []string
	expectations   []expectation
	status         catalogStatus
	usableReasons  []string
}

type expectation struct {
	validity string
	versions []string
	explicit bool
}

type caseKind string

const (
	schemaKind   caseKind = "schema"
	instanceKind caseKind = "instance"
)

type catalogStatus string

const (
	statusMissing      catalogStatus = "status-missing"
	statusSubmitted    catalogStatus = "submitted"
	statusAccepted     catalogStatus = "accepted"
	statusStable       catalogStatus = "stable"
	statusQueried      catalogStatus = "queried"
	statusDisputedTest catalogStatus = "disputed-test"
	statusDisputedSpec catalogStatus = "disputed-spec"
)

func (reader catalogReader) readSuite(documentPath string) (suiteDocument, error) {
	file, err := reader.fsys.Open(documentPath)
	if err != nil {
		return suiteDocument{}, catalogError("catalog.read", documentPath, err)
	}
	parser := xmlParser{decoder: xml.NewDecoder(file), path: documentPath, fsys: reader.fsys}
	parsed, parseErr := parser.parseSuite()
	closeErr := file.Close()
	if parseErr != nil {
		if closeErr != nil {
			return suiteDocument{}, errors.Join(parseErr,
				catalogError("catalog.read", documentPath, fmt.Errorf("close catalog: %w", closeErr)))
		}
		return suiteDocument{}, parseErr
	}
	if closeErr != nil {
		return suiteDocument{}, catalogError("catalog.read", documentPath, fmt.Errorf("close catalog: %w", closeErr))
	}
	return parsed, nil
}

func (reader catalogReader) readTestSet(documentPath string, inheritedVersions []string) (testSetDocument, error) {
	file, err := reader.fsys.Open(documentPath)
	if err != nil {
		return testSetDocument{}, catalogError("catalog.read", documentPath, err)
	}
	parser := xmlParser{decoder: xml.NewDecoder(file), path: documentPath, fsys: reader.fsys}
	parsed, parseErr := parser.parseTestSet(inheritedVersions)
	closeErr := file.Close()
	if parseErr != nil {
		if closeErr != nil {
			return testSetDocument{}, errors.Join(parseErr,
				catalogError("catalog.read", documentPath, fmt.Errorf("close catalog: %w", closeErr)))
		}
		return testSetDocument{}, parseErr
	}
	if closeErr != nil {
		return testSetDocument{}, catalogError("catalog.read", documentPath, fmt.Errorf("close catalog: %w", closeErr))
	}
	return parsed, nil
}

type xmlParser struct {
	decoder *xml.Decoder
	path    string
	fsys    fs.FS
}

func (parser *xmlParser) parseSuite() (suiteDocument, error) {
	start, err := parser.startElement()
	if err != nil {
		return suiteDocument{}, err
	}
	if !isElement(start, "testSuite") {
		return suiteDocument{}, parser.structure("root element is %s, want testSuite", elementName(start.Name))
	}
	if err := requireAttribute(start, "name"); err != nil {
		return suiteDocument{}, parser.structure("testSuite: %v", err)
	}
	if err := requireAttribute(start, "releaseDate"); err != nil {
		return suiteDocument{}, parser.structure("testSuite: %v", err)
	}
	if err := requireAttribute(start, "schemaVersion"); err != nil {
		return suiteDocument{}, parser.structure("testSuite: %v", err)
	}
	suite := suiteDocument{versions: attributeTokens(start, "version")}
	state := suiteState{}
	for {
		token, err := parser.token()
		if err != nil {
			return suiteDocument{}, err
		}
		done, err := parser.suiteToken(token, start, &suite, &state)
		if err != nil {
			return suiteDocument{}, err
		}
		if !done {
			continue
		}
		if err := parser.finishDocument(); err != nil {
			return suiteDocument{}, err
		}
		return suite, nil
	}
}

type suiteState struct {
	refsStarted bool
}

func (parser *xmlParser) suiteToken(token xml.Token, start xml.StartElement, suite *suiteDocument,
	state *suiteState,
) (bool, error) {
	switch value := token.(type) {
	case xml.StartElement:
		return false, parser.suiteChild(value, suite, state)
	case xml.EndElement:
		if value.Name != start.Name {
			return false, parser.structure("unexpected closing element %s", elementName(value.Name))
		}
		return true, nil
	case xml.CharData:
		if !isWhitespace(value) {
			return false, parser.structure("non-whitespace text in testSuite")
		}
	case xml.Comment, xml.ProcInst, xml.Directive:
	default:
		return false, parser.structure("unexpected token in testSuite")
	}
	return false, nil
}

func (parser *xmlParser) suiteChild(start xml.StartElement, suite *suiteDocument, state *suiteState) error {
	if isElement(start, "annotation") {
		if state.refsStarted {
			return parser.structure("testSuite places annotation after testSetRef")
		}
		return parser.skip()
	}
	if !isElement(start, "testSetRef") {
		return parser.structure("unexpected testSuite child %s", elementName(start.Name))
	}
	ref, err := parser.reference(start)
	if err != nil {
		return err
	}
	suite.refs = append(suite.refs, ref)
	state.refsStarted = true
	return nil
}

func (parser *xmlParser) parseTestSet(inheritedVersions []string) (testSetDocument, error) {
	start, err := parser.startElement()
	if err != nil {
		return testSetDocument{}, err
	}
	if !isElement(start, "testSet") {
		return testSetDocument{}, parser.structure("root element is %s, want testSet", elementName(start.Name))
	}
	name, err := requiredAttribute(start, "name")
	if err != nil {
		return testSetDocument{}, parser.structure("testSet: %v", err)
	}
	if err := requireAttribute(start, "contributor"); err != nil {
		return testSetDocument{}, parser.structure("testSet: %v", err)
	}
	versions := chooseVersions(start, inheritedVersions)
	set := testSetDocument{name: name}
	seenGroups := make(map[string]struct{})
	state := testSetState{}
	for {
		token, err := parser.token()
		if err != nil {
			return testSetDocument{}, err
		}
		done, err := parser.testSetToken(token, start, &set, seenGroups, versions, &state)
		if err != nil {
			return testSetDocument{}, err
		}
		if !done {
			continue
		}
		if err := parser.finishDocument(); err != nil {
			return testSetDocument{}, err
		}
		return set, nil
	}
}

type testSetState struct {
	groupsStarted bool
}

func (parser *xmlParser) testSetToken(token xml.Token, start xml.StartElement, set *testSetDocument,
	seenGroups map[string]struct{}, versions []string, state *testSetState,
) (bool, error) {
	switch value := token.(type) {
	case xml.StartElement:
		return false, parser.testSetChild(value, set, seenGroups, versions, state)
	case xml.EndElement:
		if value.Name != start.Name {
			return false, parser.structure("unexpected closing element %s", elementName(value.Name))
		}
		return true, nil
	case xml.CharData:
		if !isWhitespace(value) {
			return false, parser.structure("non-whitespace text in testSet")
		}
	case xml.Comment, xml.ProcInst, xml.Directive:
	default:
		return false, parser.structure("unexpected token in testSet")
	}
	return false, nil
}

func (parser *xmlParser) testSetChild(start xml.StartElement, set *testSetDocument,
	seenGroups map[string]struct{}, versions []string, state *testSetState,
) error {
	if isElement(start, "annotation") {
		if state.groupsStarted {
			return parser.structure("testSet places annotation after testGroup")
		}
		return parser.skip()
	}
	if !isElement(start, "testGroup") {
		return parser.structure("unexpected testSet child %s", elementName(start.Name))
	}
	group, err := parser.parseTestGroup(start, parser.path, set.name, versions)
	if err != nil {
		return err
	}
	if _, ok := seenGroups[group.name]; ok {
		return parser.structure("testSet repeats testGroup %q", group.name)
	}
	seenGroups[group.name] = struct{}{}
	set.cases = append(set.cases, group.cases...)
	state.groupsStarted = true
	return nil
}

type testGroupDocument struct {
	name  string
	cases []catalogCase
}

func (parser *xmlParser) parseTestGroup(start xml.StartElement, setPath, setName string,
	inheritedVersions []string,
) (testGroupDocument, error) {
	name, err := requiredAttribute(start, "name")
	if err != nil {
		return testGroupDocument{}, parser.structure("testGroup: %v", err)
	}
	versions := chooseVersions(start, inheritedVersions)
	group := testGroupDocument{name: name}
	state := testGroupState{seenTests: make(map[string]struct{})}
	for {
		token, err := parser.token()
		if err != nil {
			return testGroupDocument{}, err
		}
		done, err := parser.testGroupToken(token, start, &group, setPath, setName, versions, &state)
		if err != nil {
			return testGroupDocument{}, err
		}
		if done {
			return group, nil
		}
	}
}

type testGroupState struct {
	seenTests            map[string]struct{}
	documentationStarted bool
	schemaSeen           bool
	instanceSeen         bool
}

func (parser *xmlParser) testGroupToken(token xml.Token, start xml.StartElement, group *testGroupDocument,
	setPath, setName string, versions []string, state *testGroupState,
) (bool, error) {
	switch value := token.(type) {
	case xml.StartElement:
		return false, parser.testGroupChild(value, group, setPath, setName, versions, state)
	case xml.EndElement:
		if value.Name != start.Name {
			return false, parser.structure("unexpected closing element %s", elementName(value.Name))
		}
		return true, nil
	case xml.CharData:
		if !isWhitespace(value) {
			return false, parser.structure("non-whitespace text in testGroup")
		}
	case xml.Comment, xml.ProcInst, xml.Directive:
	default:
		return false, parser.structure("unexpected token in testGroup")
	}
	return false, nil
}

func (parser *xmlParser) testGroupChild(start xml.StartElement, group *testGroupDocument,
	setPath, setName string, versions []string, state *testGroupState,
) error {
	if isElement(start, "annotation") {
		return parser.groupAnnotation(group, state)
	}
	if isElement(start, "documentationReference") {
		return parser.groupDocumentationReference(group, state)
	}
	if isElement(start, "schemaTest") {
		return parser.groupSchemaTest(start, group, setPath, setName, versions, state)
	}
	if isElement(start, "instanceTest") {
		return parser.groupInstanceTest(start, group, setPath, setName, versions, state)
	}
	return parser.structure("unexpected testGroup child %s", elementName(start.Name))
}

func (parser *xmlParser) groupAnnotation(group *testGroupDocument, state *testGroupState) error {
	if state.documentationStarted || state.schemaSeen || state.instanceSeen {
		return parser.structure("testGroup %q places annotation after test metadata", group.name)
	}
	return parser.skip()
}

func (parser *xmlParser) groupDocumentationReference(group *testGroupDocument, state *testGroupState) error {
	if state.schemaSeen || state.instanceSeen {
		return parser.structure("testGroup %q places documentationReference after test", group.name)
	}
	state.documentationStarted = true
	return parser.skip()
}

func (parser *xmlParser) groupSchemaTest(start xml.StartElement, group *testGroupDocument,
	setPath, setName string, versions []string, state *testGroupState,
) error {
	if state.schemaSeen || state.instanceSeen {
		return parser.structure("testGroup %q repeats schemaTest", group.name)
	}
	caseValue, err := parser.parseCase(start, setPath, setName, group.name, schemaKind, versions)
	if err != nil {
		return err
	}
	state.schemaSeen = true
	return parser.addGroupCase(group, caseValue, state.seenTests)
}

func (parser *xmlParser) groupInstanceTest(start xml.StartElement, group *testGroupDocument,
	setPath, setName string, versions []string, state *testGroupState,
) error {
	state.instanceSeen = true
	caseValue, err := parser.parseCase(start, setPath, setName, group.name, instanceKind, versions)
	if err != nil {
		return err
	}
	return parser.addGroupCase(group, caseValue, state.seenTests)
}

func (parser *xmlParser) addGroupCase(group *testGroupDocument, caseValue catalogCase,
	seenTests map[string]struct{},
) error {
	if _, ok := seenTests[caseValue.name]; ok {
		return parser.structure("testGroup %q repeats test %q", group.name, caseValue.name)
	}
	seenTests[caseValue.name] = struct{}{}
	group.cases = append(group.cases, caseValue)
	return nil
}

func (parser *xmlParser) parseCase(start xml.StartElement, setPath, setName, groupName string,
	kind caseKind, inheritedVersions []string,
) (catalogCase, error) {
	name, err := requiredAttribute(start, "name")
	if err != nil {
		return catalogCase{}, parser.structure("%s test: %v", kind, err)
	}
	caseValue := catalogCase{
		setPath:        setPath,
		setName:        setName,
		groupName:      groupName,
		name:           name,
		kind:           kind,
		parentVersions: chooseVersions(start, inheritedVersions),
		status:         statusMissing,
	}
	state := caseState{}
	for {
		token, err := parser.token()
		if err != nil {
			return catalogCase{}, err
		}
		done, err := parser.caseToken(token, start, &caseValue, name, kind, setPath, &state)
		if err != nil {
			return catalogCase{}, err
		}
		if done {
			if err := state.finish(parser, &caseValue, name, kind); err != nil {
				return catalogCase{}, err
			}
			return caseValue, nil
		}
	}
}

type caseState struct {
	seenDocument bool
	seenExpected bool
	seenCurrent  bool
	phase        casePhase
}

type casePhase uint8

const (
	caseDocuments casePhase = iota
	caseExpected
	caseCurrent
	casePrior
)

func (parser *xmlParser) caseToken(token xml.Token, start xml.StartElement, caseValue *catalogCase,
	name string, kind caseKind, setPath string, state *caseState,
) (bool, error) {
	switch value := token.(type) {
	case xml.StartElement:
		return false, parser.caseChild(value, caseValue, name, kind, setPath, state)
	case xml.EndElement:
		if value.Name != start.Name {
			return false, parser.structure("unexpected closing element %s", elementName(value.Name))
		}
		return true, nil
	case xml.CharData:
		if !isWhitespace(value) {
			return false, parser.structure("non-whitespace text in %sTest", kind)
		}
	case xml.Comment, xml.ProcInst, xml.Directive:
	default:
		return false, parser.structure("unexpected token in %sTest", kind)
	}
	return false, nil
}

func (parser *xmlParser) caseChild(start xml.StartElement, caseValue *catalogCase, name string,
	kind caseKind, setPath string, state *caseState,
) error {
	if isElement(start, "annotation") {
		if state.phase != caseDocuments || state.seenDocument {
			return parser.structure("%sTest %q places annotation after documents", kind, name)
		}
		return parser.skip()
	}
	if isElement(start, "schemaDocument") {
		if kind != schemaKind {
			return parser.structure("instanceTest %q has schemaDocument", name)
		}
		return parser.caseDocument(start, caseValue, name, kind, setPath, state)
	}
	if isElement(start, "instanceDocument") {
		if kind != instanceKind {
			return parser.structure("schemaTest %q has instanceDocument", name)
		}
		return parser.caseDocument(start, caseValue, name, kind, setPath, state)
	}
	if isElement(start, "expected") {
		return parser.caseExpected(start, caseValue, name, kind, state)
	}
	if isElement(start, "current") {
		return parser.caseCurrent(start, caseValue, name, kind, state)
	}
	if isElement(start, "prior") {
		return parser.casePrior(start, name, kind, state)
	}
	return parser.structure("unexpected %sTest child %s", kind, elementName(start.Name))
}

func (parser *xmlParser) caseDocument(start xml.StartElement, caseValue *catalogCase, name string,
	kind caseKind, setPath string, state *caseState,
) error {
	if state.phase != caseDocuments || (kind == instanceKind && state.seenDocument) {
		return parser.structure("%sTest %q has an invalid document position", kind, name)
	}
	document, err := parser.reference(start)
	if err != nil {
		return err
	}
	resolved, err := resolveReference(setPath, document)
	if err != nil {
		return parser.referenceError(err)
	}
	caseValue.documents = append(caseValue.documents, resolved)
	reason, err := parser.dataReason(resolved)
	if err != nil {
		return err
	}
	if reason != "" {
		caseValue.usableReasons = append(caseValue.usableReasons, reason)
	}
	state.seenDocument = true
	return nil
}

func (parser *xmlParser) caseExpected(start xml.StartElement, caseValue *catalogCase, name string,
	kind caseKind, state *caseState,
) error {
	if !state.seenDocument || state.phase > caseExpected {
		return parser.structure("%sTest %q has expected before document", kind, name)
	}
	parsed, err := parser.expected(start)
	if err != nil {
		return err
	}
	caseValue.expectations = append(caseValue.expectations, parsed)
	state.seenExpected = true
	state.phase = caseExpected
	return nil
}

func (parser *xmlParser) caseCurrent(start xml.StartElement, caseValue *catalogCase, name string,
	kind caseKind, state *caseState,
) error {
	if !state.seenDocument || state.phase > caseCurrent || state.seenCurrent {
		return parser.structure("%sTest %q has invalid current position", kind, name)
	}
	status, err := parser.current(start)
	if err != nil {
		return err
	}
	caseValue.status = status
	state.seenCurrent = true
	state.phase = caseCurrent
	return nil
}

func (parser *xmlParser) casePrior(start xml.StartElement, name string, kind caseKind, state *caseState) error {
	if !state.seenDocument {
		return parser.structure("%sTest %q has prior before document", kind, name)
	}
	if err := parser.statusHistory(start); err != nil {
		return err
	}
	state.phase = casePrior
	return nil
}

func (state caseState) finish(parser *xmlParser, caseValue *catalogCase, name string, kind caseKind) error {
	if !state.seenDocument {
		return parser.structure("%sTest %q has no document", kind, name)
	}
	if len(caseValue.expectations) == 0 {
		caseValue.usableReasons = append(caseValue.usableReasons, "expected-missing")
	}
	if !state.seenCurrent {
		caseValue.usableReasons = append(caseValue.usableReasons, "status-missing")
	}
	caseValue.addOutcomeReasons()
	return nil
}

func (caseValue *catalogCase) addOutcomeReasons() {
	for _, expected := range caseValue.expectations {
		if !isKnownOutcome(expected.validity) {
			caseValue.usableReasons = append(caseValue.usableReasons, "outcome-unsupported")
		}
	}
}

func (parser *xmlParser) expected(start xml.StartElement) (expectation, error) {
	validity, err := requiredAttribute(start, "validity")
	if err != nil {
		return expectation{}, parser.structure("expected: %v", err)
	}
	explicitVersions := attributeTokens(start, "version")
	expectationValue := expectation{validity: validity, versions: explicitVersions, explicit: len(explicitVersions) != 0}
	if err := parser.emptyMetadata(start, "expected"); err != nil {
		return expectation{}, err
	}
	return expectationValue, nil
}

func (parser *xmlParser) current(start xml.StartElement) (catalogStatus, error) {
	status, err := requiredAttribute(start, "status")
	if err != nil {
		return "", parser.structure("current: %v", err)
	}
	if err := requireAttribute(start, "date"); err != nil {
		return "", parser.structure("current: %v", err)
	}
	parsed := catalogStatus(status)
	if !isKnownStatus(parsed) {
		return "", parser.structure("current has unknown status %q", status)
	}
	if err := parser.metadataWithAnnotations(start, "current"); err != nil {
		return "", err
	}
	return parsed, nil
}

func (parser *xmlParser) statusHistory(start xml.StartElement) error {
	status, err := requiredAttribute(start, "status")
	if err != nil {
		return parser.structure("prior: %v", err)
	}
	if !isKnownStatus(catalogStatus(status)) {
		return parser.structure("prior has unknown status %q", status)
	}
	if err := requireAttribute(start, "date"); err != nil {
		return parser.structure("prior: %v", err)
	}
	if err := parser.metadataWithAnnotations(start, "prior"); err != nil {
		return err
	}
	return nil
}

func (parser *xmlParser) reference(start xml.StartElement) (string, error) {
	href, err := attribute(start, xlinkNamespace, "href")
	if err != nil {
		return "", parser.structure("%s: %v", elementName(start.Name), err)
	}
	if err := parser.metadataWithAnnotations(start, elementName(start.Name)); err != nil {
		return "", err
	}
	return href, nil
}

func (parser *xmlParser) emptyMetadata(start xml.StartElement, name string) error {
	for {
		token, err := parser.token()
		if err != nil {
			return err
		}
		switch value := token.(type) {
		case xml.EndElement:
			if value.Name != start.Name {
				return parser.structure("unexpected closing element %s in %s", elementName(value.Name), name)
			}
			return nil
		case xml.CharData:
			if !isWhitespace(value) {
				return parser.structure("non-whitespace text in %s", name)
			}
		case xml.Comment, xml.ProcInst, xml.Directive:
		case xml.StartElement:
			return parser.structure("unexpected %s child %s", name, elementName(value.Name))
		default:
			return parser.structure("unexpected token in %s", name)
		}
	}
}

func (parser *xmlParser) metadataWithAnnotations(start xml.StartElement, name string) error {
	for {
		token, err := parser.token()
		if err != nil {
			return err
		}
		done, err := parser.metadataToken(token, start, name)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
	}
}

func (parser *xmlParser) metadataToken(token xml.Token, start xml.StartElement, name string) (bool, error) {
	switch value := token.(type) {
	case xml.StartElement:
		if !isElement(value, "annotation") {
			return false, parser.structure("unexpected %s child %s", name, elementName(value.Name))
		}
		return false, parser.skip()
	case xml.EndElement:
		if value.Name != start.Name {
			return false, parser.structure("unexpected closing element %s in %s", elementName(value.Name), name)
		}
		return true, nil
	case xml.CharData:
		if !isWhitespace(value) {
			return false, parser.structure("non-whitespace text in %s", name)
		}
	case xml.Comment, xml.ProcInst, xml.Directive:
	default:
		return false, parser.structure("unexpected token in %s", name)
	}
	return false, nil
}

func (parser *xmlParser) skip() error {
	if err := parser.decoder.Skip(); err != nil {
		return parser.decode(err)
	}
	return nil
}

func (parser *xmlParser) startElement() (xml.StartElement, error) {
	for {
		token, err := parser.token()
		if err != nil {
			return xml.StartElement{}, err
		}
		if start, ok := token.(xml.StartElement); ok {
			return start, nil
		}
		switch value := token.(type) {
		case xml.CharData:
			if !isWhitespace(value) {
				return xml.StartElement{}, parser.structure("non-whitespace text before root")
			}
		case xml.Comment, xml.ProcInst, xml.Directive:
		default:
			return xml.StartElement{}, parser.structure("unexpected token before root")
		}
	}
}

func (parser *xmlParser) token() (xml.Token, error) {
	token, err := parser.decoder.Token()
	if err != nil {
		return nil, parser.decode(err)
	}
	return token, nil
}

func (parser *xmlParser) finishDocument() error {
	for {
		token, err := parser.token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		switch value := token.(type) {
		case xml.StartElement:
			return parser.structure("multiple root elements; found %s", elementName(value.Name))
		case xml.EndElement:
			return parser.structure("unexpected closing element %s after root", elementName(value.Name))
		case xml.CharData:
			if !isWhitespace(value) {
				return parser.structure("non-whitespace text after root")
			}
		case xml.Comment, xml.ProcInst, xml.Directive:
		default:
			return parser.structure("unexpected token after root")
		}
	}
}

func (parser *xmlParser) dataReason(documentPath string) (string, error) {
	info, err := fs.Stat(parser.fsys, documentPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "data-missing", nil
		}
		return "", catalogError("catalog.read", documentPath, err)
	}
	if !info.Mode().IsRegular() {
		return "data-not-regular", nil
	}
	return "", nil
}

func (parser *xmlParser) decode(err error) error {
	return catalogError("catalog.decode", parser.path, err)
}

func (parser *xmlParser) structure(format string, args ...any) error {
	return catalogError("catalog.structure", parser.path, fmt.Errorf(format, args...))
}

func (parser *xmlParser) referenceError(err error) error {
	return catalogError("catalog.reference", parser.path, err)
}

func catalogError(code, documentPath string, err error) error {
	return &CatalogError{Code: code, Path: documentPath, Err: err}
}

func (inventory Inventory) rows() []inventoryRow {
	rows := make([]inventoryRow, 0, 4)
	for _, version := range []string{"1.0", "1.1"} {
		for _, kind := range []caseKind{schemaKind, instanceKind} {
			rows = append(rows, inventory.row(version, kind))
		}
	}
	return rows
}

func (inventory Inventory) row(version string, kind caseKind) inventoryRow {
	row := inventoryRow{version: version, kind: kind}
	for _, caseValue := range inventory.cases {
		if caseValue.kind != kind || !caseApplies(caseValue, version) {
			continue
		}
		row.addCase(caseValue, version)
	}
	return row
}

func (inventory Inventory) summary() (queried, disputed, unusable int) {
	for _, caseValue := range inventory.cases {
		switch caseValue.status {
		case statusQueried:
			queried++
		case statusDisputedTest, statusDisputedSpec:
			disputed++
		case statusMissing, statusSubmitted, statusAccepted, statusStable:
		}
		if caseValue.isGloballyUnusable() {
			unusable++
		}
	}
	return queried, disputed, unusable
}

type inventoryRow struct {
	version       string
	kind          caseKind
	cases         int
	valid         int
	invalid       int
	other         int
	queried       int
	disputedTest  int
	disputedSpec  int
	statusMissing int
	unusable      int
	headline      int
}

func (row *inventoryRow) addStatus(status catalogStatus) {
	switch status {
	case statusSubmitted, statusAccepted, statusStable:
	case statusQueried:
		row.queried++
	case statusDisputedTest:
		row.disputedTest++
	case statusDisputedSpec:
		row.disputedSpec++
	case statusMissing:
		row.statusMissing++
	}
}

func (row *inventoryRow) addCase(caseValue catalogCase, version string) {
	row.cases++
	row.addStatus(caseValue.status)
	if caseValue.isUnusable(version) {
		row.unusable++
	}
	outcome, ok := caseValue.outcome(version)
	if !ok {
		row.other++
		return
	}
	row.addOutcome(outcome)
	if caseValue.isHeadline(version, outcome) {
		row.headline++
	}
}

func (row *inventoryRow) addOutcome(outcome string) {
	switch outcome {
	case "valid":
		row.valid++
	case "invalid":
		row.invalid++
	default:
		row.other++
	}
}

func (caseValue catalogCase) outcome(version string) (string, bool) {
	var outcome string
	for _, expected := range caseValue.expectations {
		if !expectedApplies(expected, caseValue.parentVersions, version) {
			continue
		}
		if outcome != "" {
			return "", false
		}
		outcome = expected.validity
	}
	if outcome == "" {
		return "", false
	}
	return outcome, true
}

func (caseValue catalogCase) isUnusable(version string) bool {
	if len(caseValue.usableReasons) != 0 {
		return true
	}
	_, ok := caseValue.outcome(version)
	return !ok
}

func (caseValue catalogCase) isGloballyUnusable() bool {
	if len(caseValue.usableReasons) != 0 {
		return true
	}
	for _, version := range []string{"1.0", "1.1"} {
		if caseApplies(caseValue, version) && caseValue.isUnusable(version) {
			return true
		}
	}
	return !caseApplies(caseValue, "1.0") && !caseApplies(caseValue, "1.1")
}

func (caseValue catalogCase) isHeadline(version, outcome string) bool {
	if caseValue.status == statusQueried || caseValue.status == statusDisputedTest ||
		caseValue.status == statusDisputedSpec || caseValue.status == statusMissing {
		return false
	}
	if caseValue.isUnusable(version) {
		return false
	}
	return outcome == "valid" || outcome == "invalid"
}

func caseApplies(caseValue catalogCase, version string) bool {
	return parentApplies(caseValue.parentVersions, version)
}

func expectedApplies(expected expectation, parentVersions []string, version string) bool {
	if !parentApplies(parentVersions, version) {
		return false
	}
	if !expected.explicit {
		return true
	}
	return len(expected.versions) == 1 && expected.versions[0] == version
}

func parentApplies(parentVersions []string, version string) bool {
	if len(parentVersions) == 0 {
		return true
	}
	for _, token := range parentVersions {
		if token == version || (token != "1.0" && token != "1.1") {
			return true
		}
	}
	return false
}

func chooseVersions(start xml.StartElement, inherited []string) []string {
	versions := attributeTokens(start, "version")
	if len(versions) != 0 {
		return versions
	}
	return inherited
}

func attributeTokens(start xml.StartElement, local string) []string {
	value, ok := optionalAttribute(start, local)
	if !ok {
		return nil
	}
	return strings.Fields(value)
}

func requiredAttribute(start xml.StartElement, local string) (string, error) {
	value, ok := optionalAttribute(start, local)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("missing %s attribute", local)
	}
	return value, nil
}

func requireAttribute(start xml.StartElement, local string) error {
	_, err := requiredAttribute(start, local)
	return err
}

func attribute(start xml.StartElement, namespace, local string) (string, error) {
	for _, value := range start.Attr {
		if value.Name.Space == namespace && value.Name.Local == local {
			if strings.TrimSpace(value.Value) == "" {
				return "", fmt.Errorf("empty %s attribute", local)
			}
			return value.Value, nil
		}
	}
	return "", fmt.Errorf("missing %s attribute", local)
}

func optionalAttribute(start xml.StartElement, local string) (string, bool) {
	for _, value := range start.Attr {
		if value.Name.Space == "" && value.Name.Local == local {
			return value.Value, true
		}
	}
	return "", false
}

func resolveReference(base, href string) (string, error) {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "/") || strings.Contains(href, "://") ||
		strings.ContainsAny(href, "?#") || strings.Contains(href, "\\") {
		return "", fmt.Errorf("invalid local reference %q", href)
	}
	resolved := path.Clean(path.Join(path.Dir(base), href))
	if resolved == "." || resolved == ".." || strings.HasPrefix(resolved, "../") {
		return "", fmt.Errorf("reference %q escapes catalog root", href)
	}
	if !fs.ValidPath(resolved) {
		return "", fmt.Errorf("invalid local reference %q", href)
	}
	return resolved, nil
}

func isElement(start xml.StartElement, local string) bool {
	return start.Name.Space == testSuiteNamespace && start.Name.Local == local
}

func elementName(name xml.Name) string {
	return name.Local
}

func isWhitespace(data []byte) bool {
	return strings.TrimSpace(string(data)) == ""
}

func isKnownOutcome(value string) bool {
	return value == "valid" || value == "invalid"
}

func isKnownStatus(value catalogStatus) bool {
	switch value {
	case statusSubmitted, statusAccepted, statusStable, statusQueried, statusDisputedTest, statusDisputedSpec:
		return true
	case statusMissing:
		return false
	default:
		return false
	}
}
