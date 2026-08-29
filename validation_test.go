package goxsd9_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/goxdra/goxsd9"
)

const validationTestXSDNamespace = "http://www.w3.org/2001/XMLSchema"

type validationTestFixture struct {
	id       goxsd9.SourceID
	contents string
}

type validationTestResolver struct {
	fixtures map[string]validationTestFixture
}

func (resolver validationTestResolver) Resolve(
	ctx context.Context,
	namespaceURN, schemaLocation string,
) (goxsd9.ResolvedSource, error) {
	fixture, ok := resolver.fixtures[schemaLocation]
	if !ok {
		return goxsd9.ResolvedSource{}, fmt.Errorf("missing validation fixture %q (%q)", namespaceURN, schemaLocation)
	}
	return goxsd9.NewResolvedSource(ctx, fixture.id, io.NopCloser(strings.NewReader(fixture.contents)))
}

type validationTestSource struct {
	data       []byte
	offset     int
	failAt     int
	readErr    error
	closeErr   error
	closed     bool
	closeCalls int
}

func newValidationTestSource(input string) *validationTestSource {
	return &validationTestSource{data: []byte(input), failAt: -1}
}

func (source *validationTestSource) Read(buffer []byte) (int, error) {
	if source.failAt >= 0 && source.offset >= source.failAt {
		return 0, source.readErr
	}
	if source.offset >= len(source.data) {
		return 0, io.EOF
	}
	limit := len(source.data)
	if source.failAt >= 0 && source.failAt < limit {
		limit = source.failAt
	}
	n := copy(buffer, source.data[source.offset:limit])
	source.offset += n
	return n, nil
}

func (source *validationTestSource) Close() error {
	source.closed = true
	source.closeCalls++
	return source.closeErr
}

func validationTestSchema(t *testing.T, root string, fixtures map[string]validationTestFixture) goxsd9.Schema {
	return validationTestSchemaWithPolicy(t, root, fixtures, goxsd9.Compatibility)
}

func validationTestSchemaWithPolicy(t *testing.T, root string, fixtures map[string]validationTestFixture, policy goxsd9.LanguagePolicy) goxsd9.Schema {
	t.Helper()
	rootSource, err := goxsd9.NewResolvedSource(
		context.Background(),
		"root.xsd",
		io.NopCloser(strings.NewReader(root)),
	)
	if err != nil {
		t.Fatalf("NewResolvedSource: %v", err)
	}
	schema, err := goxsd9.ParseSchemaWithPolicy(rootSource, validationTestResolver{fixtures: fixtures}, policy)
	if err != nil {
		t.Fatalf("ParseSchema: %v", err)
	}
	return schema
}

func validationTestDiagnostic(t *testing.T, err error) goxsd9.Diagnostic {
	t.Helper()
	if err == nil {
		t.Fatal("expected a diagnostic")
	}
	var diagnostic goxsd9.Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("error does not contain a Diagnostic: %v", err)
	}
	return diagnostic
}

func validationTestDiagnostics(t *testing.T, err error) []goxsd9.Diagnostic {
	t.Helper()
	if err == nil {
		t.Fatal("expected diagnostics")
	}
	var diagnostics goxsd9.Diagnostics
	if errors.As(err, &diagnostics) {
		return diagnostics.All()
	}
	return []goxsd9.Diagnostic{validationTestDiagnostic(t, err)}
}

//nolint:unparam // Validation fixtures currently use one-line instance locations.
func validationTestLoc(t *testing.T, source goxsd9.SourceID, line, column int) goxsd9.Loc {
	t.Helper()
	loc, err := goxsd9.NewLoc(source, line, column)
	if err != nil {
		t.Fatalf("NewLoc: %v", err)
	}
	return loc
}

func validationTestTextLoc(t *testing.T, input string) goxsd9.Loc {
	t.Helper()
	end := strings.IndexByte(input, '>')
	if end < 0 {
		t.Fatalf("input has no root start-tag end: %q", input)
	}
	return validationTestLoc(t, "instance.xml", 1, end+2)
}

func validationTestHasRelated(locations []goxsd9.Loc, want goxsd9.Loc) bool {
	for _, location := range locations {
		if location == want {
			return true
		}
	}
	return false
}

func TestValidateInstanceSupportsBuiltInNamedForwardAndCrossDocumentScalars(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="urn:root" xmlns:o="urn:other" targetNamespace="urn:root" version="1.0">
  <xs:import namespace="urn:other" schemaLocation="other.xsd"/>
  <xs:element name="builtInteger" type="xs:integer"/>
  <xs:element name="builtDecimal" type="xs:decimal"/>
  <xs:element name="forwardInteger" type="r:ForwardInteger"/>
  <xs:element name="namedDecimal" type="r:NamedDecimal"/>
  <xs:element name="crossDecimal" type="o:CrossDecimal"/>
  <xs:simpleType name="ForwardInteger"><xs:restriction base="xs:integer"><xs:totalDigits value="5"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="NamedDecimal"><xs:restriction base="xs:decimal"><xs:totalDigits value="4"/><xs:fractionDigits value="2"/></xs:restriction></xs:simpleType>
</xs:schema>`
	other := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" targetNamespace="urn:other" version="1.1">
  <xs:simpleType name="CrossDecimal"><xs:restriction base="xs:decimal"><xs:fractionDigits value="3"/></xs:restriction></xs:simpleType>
</xs:schema>`
	schema := validationTestSchema(t, root, map[string]validationTestFixture{
		"other.xsd": {id: "other.xsd", contents: other},
	})

	cases := []struct {
		name  string
		input string
	}{
		{name: "built integer", input: `<builtInteger xmlns="urn:root">-42</builtInteger>`},
		{name: "built decimal", input: `<builtDecimal xmlns="urn:root">12.50</builtDecimal>`},
		{name: "forward integer", input: `<forwardInteger xmlns="urn:root">12345</forwardInteger>`},
		{name: "named decimal", input: `<namedDecimal xmlns="urn:root">12.30</namedDecimal>`},
		{name: "cross document decimal", input: `<crossDecimal xmlns="urn:root">1.234</crossDecimal>`},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(test.input))); err != nil {
				t.Fatalf("ValidateInstance: %v", err)
			}
		})
	}
}

func TestValidateInstanceExpandsNamespacesAndConcatenatesDecodedText(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" targetNamespace="urn:root">
  <xs:element name="count" type="xs:integer"/>
  <xs:element name="amount" type="xs:decimal"/>
</xs:schema>`
	schema := validationTestSchema(t, root, nil)

	cases := []string{
		`<p:count xmlns:p="urn:root">42</p:count>`,
		`<count xmlns="urn:root">4<!-- ignored --><?note ignored?>2</count>`,
		"<amount xmlns=\"urn:root\"> \n\t1.20\r\n </amount>",
	}
	for _, input := range cases {
		if err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
			t.Fatalf("ValidateInstance(%q): %v", input, err)
		}
	}
}

//nolint:gocognit,funlen // Keep lexical, facet, location, cause, and specification assertions together.
func TestValidateInstanceReportsLexicalAndFacetDiagnosticsWithSchemaEvidence(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="1.0">
	  <xs:element name="count" type="xs:integer"/>
	  <xs:element name="amount" type="r:Amount"/>
  <xs:simpleType name="Amount"><xs:restriction base="xs:decimal"><xs:totalDigits value="3"/><xs:fractionDigits value="2"/></xs:restriction></xs:simpleType>
</xs:schema>`
	schema := validationTestSchemaWithPolicy(t, root, nil, goxsd9.Strict10)

	input := `<count xmlns="urn:root">1.0</count>`
	err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
	diagnostic := validationTestDiagnostic(t, err)
	if got, want := diagnostic.Class(), goxsd9.FailureInvalid; got != want {
		t.Fatalf("Class() = %q, want %q", got, want)
	}
	if got, want := diagnostic.Code(), goxsd9.InvalidIntegerLexicalCode; got != want {
		t.Fatalf("Code() = %q, want %q", got, want)
	}
	if got, want := diagnostic.Loc(), validationTestTextLoc(t, input); got != want {
		t.Fatalf("Loc() = %s, want %s", got, want)
	}
	if got, want := diagnostic.SpecRef(), "xsd11-datatypes#integer"; got != want {
		t.Fatalf("SpecRef() = %q, want %q", got, want)
	}
	countName, err := goxsd9.NewQName("urn:root", "count")
	if err != nil {
		t.Fatalf("NewQName: %v", err)
	}
	count := schema.FindKind(goxsd9.ComponentKindElementDeclaration, countName)
	if len(count) != 1 || !validationTestHasRelated(diagnostic.Related(), count[0].Loc()) {
		t.Fatalf("Related() = %v, want count declaration location %v", diagnostic.Related(), count[0].Loc())
	}
	if diagnostic.Unwrap() != nil {
		t.Fatal("invalid integer lexical diagnostic unexpectedly has a cause")
	}

	failing := `<amount xmlns="urn:root">0.123</amount>`
	err = goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(failing)))
	diagnostic = validationTestDiagnostic(t, err)
	if got, want := diagnostic.Code(), goxsd9.DigitFacetValueViolationCode; got != want {
		t.Fatalf("Code() = %q, want %q", got, want)
	}
	if got, want := diagnostic.Loc(), validationTestTextLoc(t, failing); got != want {
		t.Fatalf("Loc() = %s, want %s", got, want)
	}
	if got, want := diagnostic.SpecRef(), "xsd10-datatypes#cvc-fractionDigits-valid"; got != want {
		t.Fatalf("SpecRef() = %q, want %q", got, want)
	}
	amountName, err := goxsd9.NewQName("urn:root", "amount")
	if err != nil {
		t.Fatalf("NewQName: %v", err)
	}
	amount := schema.FindKind(goxsd9.ComponentKindElementDeclaration, amountName)
	if len(amount) != 1 {
		t.Fatalf("amount declarations = %d, want 1", len(amount))
	}
	typeName, err := goxsd9.NewQName("urn:root", "Amount")
	if err != nil {
		t.Fatalf("NewQName: %v", err)
	}
	types := schema.FindKind(goxsd9.ComponentKindSimpleTypeDefinition, typeName)
	if len(types) != 1 {
		t.Fatalf("Amount definitions = %d, want 1", len(types))
	}
	definition, ok := types[0].SimpleTypeDefinition()
	if !ok {
		t.Fatal("Amount definition has no simple type view")
	}
	fractionLoc, ok := definition.DigitFacets().FractionDigitsLoc()
	if !ok {
		t.Fatal("Amount definition has no fractionDigits location")
	}
	if got, want := diagnostic.Related(), []goxsd9.Loc{amount[0].Loc(), types[0].Loc(), fractionLoc}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Related() = %v, want %v", got, want)
	}
	if diagnostic.Unwrap() == nil {
		t.Fatal("facet violation lost its cause")
	}
}

func TestValidateInstanceUsesExactDigitBoundariesAndTrailingZeroValueSpace(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root">
	  <xs:element name="amount" type="r:Amount"/>
  <xs:simpleType name="Amount"><xs:restriction base="xs:decimal"><xs:totalDigits value="3"/><xs:fractionDigits value="2"/></xs:restriction></xs:simpleType>
</xs:schema>`
	schema := validationTestSchema(t, root, nil)
	for _, value := range []string{"1.20", "1.2300", "123.00"} {
		input := `<amount xmlns="urn:root">` + value + `</amount>`
		if err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
			t.Fatalf("ValidateInstance(%q): %v", value, err)
		}
	}
	for _, test := range []struct {
		value string
		code  string
	}{
		{value: "1234", code: goxsd9.DigitFacetValueViolationCode},
		{value: "0.123", code: goxsd9.DigitFacetValueViolationCode},
	} {
		input := `<amount xmlns="urn:root">` + test.value + `</amount>`
		err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
		diagnostic := validationTestDiagnostic(t, err)
		if got, want := diagnostic.Code(), test.code; got != want {
			t.Fatalf("value %q Code() = %q, want %q", test.value, got, want)
		}
	}
}

func TestValidateInstanceAppliesNamedAndBuiltInDecimalVersionRules(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="1.0">
	  <xs:element name="built" type="xs:decimal"/>
	  <xs:element name="named" type="r:Named"/>
  <xs:simpleType name="Named"><xs:restriction base="xs:decimal"/></xs:simpleType>
</xs:schema>`
	schema := validationTestSchemaWithPolicy(t, root, nil, goxsd9.Strict10)
	for _, input := range []string{
		`<built xmlns="urn:root">.5</built>`,
		`<built xmlns="urn:root">1.</built>`,
	} {
		if err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
			t.Fatalf("built-in ValidateInstance(%q): %v", input, err)
		}
	}
	for _, input := range []string{
		`<named xmlns="urn:root">.5</named>`,
		`<named xmlns="urn:root">1.</named>`,
	} {
		err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
		diagnostic := validationTestDiagnostic(t, err)
		if got, want := diagnostic.Code(), goxsd9.InvalidDecimalLexicalCode; got != want {
			t.Fatalf("named value %q Code() = %q, want %q", input, got, want)
		}
		if got, want := diagnostic.SpecRef(), "xsd10-datatypes#decimal"; got != want {
			t.Fatalf("named value %q SpecRef() = %q, want %q", input, got, want)
		}
	}
}

func TestValidateInstanceRejectsUnknownSchemaRoot(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" targetNamespace="urn:root">
  <xs:include schemaLocation="child.xsd"/>
  <xs:element name="known" type="xs:integer"/>
</xs:schema>`
	child := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" targetNamespace="urn:root">
  <xs:element name="child" type="xs:integer"/>
</xs:schema>`
	schema := validationTestSchema(t, root, map[string]validationTestFixture{
		"child.xsd": {id: "child.xsd", contents: child},
	})

	unknown := `<unknown xmlns="urn:root">1</unknown>`
	err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(unknown)))
	diagnostic := validationTestDiagnostic(t, err)
	if got, want := diagnostic.Code(), goxsd9.UnknownInstanceSchemaRootCode; got != want {
		t.Fatalf("unknown root Code() = %q, want %q", got, want)
	}
	if got, want := diagnostic.Loc(), validationTestLoc(t, "instance.xml", 1, 1); got != want {
		t.Fatalf("unknown root Loc() = %s, want %s", got, want)
	}
	if got, want := diagnostic.SpecRef(), "xsd11-structures#cvc-elt"; got != want {
		t.Fatalf("unknown root SpecRef() = %q, want %q", got, want)
	}
	if diagnostic.Unwrap() == nil {
		t.Fatal("unknown root diagnostic lost its cause")
	}
}

//nolint:gocognit // Keep each unsupported scalar structure's diagnostic contract together.
func TestValidateInstanceReportsUnsupportedScalarStructures(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" targetNamespace="urn:root">
  <xs:element name="value" type="xs:integer"/>
  <xs:element name="untype"/>
</xs:schema>`
	schema := validationTestSchema(t, root, nil)
	valueName, err := goxsd9.NewQName("urn:root", "value")
	if err != nil {
		t.Fatalf("NewQName: %v", err)
	}
	declarations := schema.FindKind(goxsd9.ComponentKindElementDeclaration, valueName)
	if len(declarations) != 1 {
		t.Fatalf("value declarations = %d, want 1", len(declarations))
	}

	cases := []string{
		`<value xmlns="urn:root" extra="1">1</value>`,
		`<value xmlns="urn:root" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:type="xs:integer">1</value>`,
		`<value xmlns="urn:root" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:nil="true"/>`,
		`<value xmlns="urn:root"><child/>1</value>`,
	}
	for _, input := range cases {
		err = goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input)))
		diagnostic := validationTestDiagnostic(t, err)
		if got, want := diagnostic.Class(), goxsd9.FailureUnsupported; got != want {
			t.Fatalf("input %q Class() = %q, want %q", input, got, want)
		}
		if got, want := diagnostic.Code(), goxsd9.UnsupportedInstanceValidationCode; got != want {
			t.Fatalf("input %q Code() = %q, want %q", input, got, want)
		}
		if got, want := diagnostic.Feature(), goxsd9.FeatureInstanceValidation; got != want {
			t.Fatalf("input %q Feature() = %q, want %q", input, got, want)
		}
		if got, want := diagnostic.SpecRef(), "xsd11-structures#cvc-elt"; got != want {
			t.Fatalf("input %q SpecRef() = %q, want %q", input, got, want)
		}
		if !validationTestHasRelated(diagnostic.Related(), declarations[0].Loc()) {
			t.Fatalf("input %q Related() = %v, want declaration location %v", input, diagnostic.Related(), declarations[0].Loc())
		}
		if !errors.Is(err, goxsd9.ErrUnsupported) {
			t.Fatalf("input %q does not match ErrUnsupported", input)
		}
	}

	noType := `<untype xmlns="urn:root"/>`
	err = goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(noType)))
	diagnostic := validationTestDiagnostic(t, err)
	if got, want := diagnostic.Class(), goxsd9.FailureUnsupported; got != want {
		t.Fatalf("no type Class() = %q, want %q", got, want)
	}
	if got, want := diagnostic.Code(), goxsd9.UnsupportedInstanceValidationCode; got != want {
		t.Fatalf("no type Code() = %q, want %q", got, want)
	}
}

//nolint:gocognit // Keep stream lifecycle, cause, and diagnostic ordering assertions together.
func TestValidateInstancePreservesStreamLifecycleCausesAndOrder(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" targetNamespace="urn:root"><xs:element name="value" type="xs:integer"/></xs:schema>`
	schema := validationTestSchema(t, root, nil)
	validInput := `<value xmlns="urn:root">42</value>`
	validReader := newValidationTestSource(validInput)
	if err := goxsd9.ValidateInstance(schema, "instance.xml", validReader); err != nil {
		t.Fatalf("ValidateInstance(valid): %v", err)
	}
	if !validReader.closed || validReader.closeCalls != 1 || validReader.offset != len(validReader.data) {
		t.Fatalf("valid lifecycle = closed %t, close calls %d, offset %d, want closed once and offset %d", validReader.closed, validReader.closeCalls, validReader.offset, len(validReader.data))
	}

	readErr := errors.New("instance validation read failed")
	closeErr := errors.New("instance validation close failed")
	failingReader := newValidationTestSource(validInput)
	failingReader.failAt = len(failingReader.data)
	failingReader.readErr = readErr
	failingReader.closeErr = closeErr
	err := goxsd9.ValidateInstance(schema, "instance.xml", failingReader)
	if err == nil || !errors.Is(err, readErr) || !errors.Is(err, closeErr) {
		t.Fatalf("read/close error = %v, want both causes", err)
	}
	diagnostics := validationTestDiagnostics(t, err)
	if len(diagnostics) != 2 {
		t.Fatalf("read/close diagnostics = %d, want 2: %v", len(diagnostics), err)
	}
	if diagnostics[0].Code() != goxsd9.SourceReadCode || diagnostics[1].Code() != goxsd9.SourceCloseCode {
		t.Fatalf("read/close diagnostic order = %#v, want read then close", diagnostics)
	}
	if !failingReader.closed || failingReader.closeCalls != 1 || failingReader.offset != len(failingReader.data) {
		t.Fatalf("failing lifecycle = closed %t, close calls %d, offset %d, want closed once and drained", failingReader.closed, failingReader.closeCalls, failingReader.offset)
	}

	closeOnlyErr := errors.New("instance validation close only failed")
	closeOnlyReader := newValidationTestSource(validInput)
	closeOnlyReader.closeErr = closeOnlyErr
	err = goxsd9.ValidateInstance(schema, "instance.xml", closeOnlyReader)
	if err == nil || !errors.Is(err, closeOnlyErr) {
		t.Fatalf("close-only error = %v, want close cause", err)
	}
	diagnostics = validationTestDiagnostics(t, err)
	if len(diagnostics) != 1 || diagnostics[0].Code() != goxsd9.SourceCloseCode {
		t.Fatalf("close-only diagnostics = %#v, want one SourceClose diagnostic", diagnostics)
	}
	if !closeOnlyReader.closed || closeOnlyReader.closeCalls != 1 || closeOnlyReader.offset != len(closeOnlyReader.data) {
		t.Fatalf("close-only lifecycle = closed %t, close calls %d, offset %d", closeOnlyReader.closed, closeOnlyReader.closeCalls, closeOnlyReader.offset)
	}

	invalidReader := newValidationTestSource(`<value xmlns="urn:root">1.0</value>`)
	err = goxsd9.ValidateInstance(schema, "instance.xml", invalidReader)
	if err == nil || !invalidReader.closed || invalidReader.closeCalls != 1 || invalidReader.offset != len(invalidReader.data) {
		t.Fatalf("invalid lifecycle = err %v, closed %t, close calls %d, offset %d", err, invalidReader.closed, invalidReader.closeCalls, invalidReader.offset)
	}
}

func TestValidateInstanceRejectsBoundaryInputsAndDoesNotMutateSchema(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" targetNamespace="urn:root"><xs:element name="value" type="xs:integer"/></xs:schema>`
	schema := validationTestSchema(t, root, nil)
	if diagnostic := validationTestDiagnostic(t, goxsd9.ValidateInstance(schema, "instance.xml", nil)); diagnostic.Code() != goxsd9.InvalidInstanceReaderCode {
		t.Fatalf("nil reader Code() = %q, want %q", diagnostic.Code(), goxsd9.InvalidInstanceReaderCode)
	}

	zeroSchemaReader := newValidationTestSource(`<value xmlns="urn:root">1</value>`)
	err := goxsd9.ValidateInstance(goxsd9.Schema{}, "instance.xml", zeroSchemaReader)
	diagnostic := validationTestDiagnostic(t, err)
	if got, want := diagnostic.Code(), goxsd9.InvalidInstanceSchemaCode; got != want {
		t.Fatalf("zero schema Code() = %q, want %q", got, want)
	}
	if !zeroSchemaReader.closed || zeroSchemaReader.closeCalls != 1 {
		t.Fatalf("zero schema reader lifecycle = closed %t, close calls %d", zeroSchemaReader.closed, zeroSchemaReader.closeCalls)
	}

	emptySourceReader := newValidationTestSource(`<value xmlns="urn:root">1</value>`)
	err = goxsd9.ValidateInstance(schema, "", emptySourceReader)
	diagnostic = validationTestDiagnostic(t, err)
	if got, want := diagnostic.Code(), goxsd9.InvalidInstanceSourceCode; got != want {
		t.Fatalf("empty source Code() = %q, want %q", got, want)
	}
	if !emptySourceReader.closed || emptySourceReader.closeCalls != 1 {
		t.Fatalf("empty source reader lifecycle = closed %t, close calls %d", emptySourceReader.closed, emptySourceReader.closeCalls)
	}

	before := schema.Components()
	firstInput := `<value xmlns="urn:root">1.0</value>`
	firstErr := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(firstInput)))
	secondErr := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(firstInput)))
	first := validationTestDiagnostic(t, firstErr)
	second := validationTestDiagnostic(t, secondErr)
	if first.Code() != second.Code() || first.Loc() != second.Loc() || !reflect.DeepEqual(first.Related(), second.Related()) || first.SpecRef() != second.SpecRef() || first.Error() != second.Error() {
		t.Fatalf("repeated diagnostics differ: first %v, second %v", firstErr, secondErr)
	}
	if !reflect.DeepEqual(before, schema.Components()) {
		t.Fatal("validation mutated the completed schema")
	}
}

func TestValidateInstanceEnforcesAllNamedIntegerAndDecimalBounds(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root">
  <xs:element name="intMinInclusive" type="r:IntMinInclusive"/>
  <xs:element name="intMinExclusive" type="r:IntMinExclusive"/>
  <xs:element name="intMaxInclusive" type="r:IntMaxInclusive"/>
  <xs:element name="intMaxExclusive" type="r:IntMaxExclusive"/>
  <xs:element name="decimalMinInclusive" type="r:DecimalMinInclusive"/>
  <xs:element name="decimalMinExclusive" type="r:DecimalMinExclusive"/>
  <xs:element name="decimalMaxInclusive" type="r:DecimalMaxInclusive"/>
  <xs:element name="decimalMaxExclusive" type="r:DecimalMaxExclusive"/>
  <xs:element name="intRange" type="r:IntRange"/>
  <xs:element name="decimalRange" type="r:DecimalRange"/>
  <xs:simpleType name="IntMinInclusive"><xs:restriction base="xs:integer"><xs:minInclusive value="10"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="IntMinExclusive"><xs:restriction base="xs:integer"><xs:minExclusive value="10"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="IntMaxInclusive"><xs:restriction base="xs:integer"><xs:maxInclusive value="10"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="IntMaxExclusive"><xs:restriction base="xs:integer"><xs:maxExclusive value="10"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="DecimalMinInclusive"><xs:restriction base="xs:decimal"><xs:minInclusive value="10.00"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="DecimalMinExclusive"><xs:restriction base="xs:decimal"><xs:minExclusive value="10.00"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="DecimalMaxInclusive"><xs:restriction base="xs:decimal"><xs:maxInclusive value="10.00"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="DecimalMaxExclusive"><xs:restriction base="xs:decimal"><xs:maxExclusive value="10.00"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="IntRange"><xs:restriction base="xs:integer"><xs:minInclusive value="10"/><xs:maxExclusive value="20"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="DecimalRange"><xs:restriction base="xs:decimal"><xs:minExclusive value="1.20"/><xs:maxInclusive value="2.30"/></xs:restriction></xs:simpleType>
</xs:schema>`
	schema := validationTestSchema(t, root, nil)

	valid := []struct {
		name  string
		value string
	}{
		{name: "intMinInclusive", value: "10"},
		{name: "intMinExclusive", value: "11"},
		{name: "intMaxInclusive", value: "10"},
		{name: "intMaxExclusive", value: "9"},
		{name: "decimalMinInclusive", value: "10.000"},
		{name: "decimalMinExclusive", value: "10.01"},
		{name: "decimalMaxInclusive", value: "10.000"},
		{name: "decimalMaxExclusive", value: "9.99"},
		{name: "intRange", value: "10"},
		{name: "intRange", value: "19"},
		{name: "decimalRange", value: "1.2001"},
		{name: "decimalRange", value: "2.300"},
	}
	for _, test := range valid {
		t.Run("valid/"+test.name+"/"+test.value, func(t *testing.T) {
			input := `<` + test.name + ` xmlns="urn:root">` + test.value + `</` + test.name + `>`
			if err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
				t.Fatalf("ValidateInstance(%q): %v", input, err)
			}
		})
	}

	invalid := []struct {
		name  string
		value string
	}{
		{name: "intMinInclusive", value: "9"},
		{name: "intMinExclusive", value: "10"},
		{name: "intMaxInclusive", value: "11"},
		{name: "intMaxExclusive", value: "10"},
		{name: "decimalMinInclusive", value: "9.99"},
		{name: "decimalMinExclusive", value: "10.00"},
		{name: "decimalMaxInclusive", value: "10.01"},
		{name: "decimalMaxExclusive", value: "10.00"},
		{name: "intRange", value: "20"},
		{name: "decimalRange", value: "1.20"},
		{name: "decimalRange", value: "2.31"},
	}
	for _, test := range invalid {
		t.Run("invalid/"+test.name+"/"+test.value, func(t *testing.T) {
			input := `<` + test.name + ` xmlns="urn:root">` + test.value + `</` + test.name + `>`
			diagnostic := validationTestDiagnostic(t, goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
			if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != goxsd9.BoundValueViolationCode {
				t.Fatalf("diagnostic = %s, want invalid bound-value violation", diagnostic)
			}
			if diagnostic.Loc() != validationTestTextLoc(t, input) {
				t.Fatalf("Loc() = %s, want candidate text location", diagnostic.Loc())
			}
			if diagnostic.Unwrap() == nil {
				t.Fatal("bound diagnostic lost its cause")
			}
		})
	}
}

//nolint:gocognit // Keep exact values, diagnostics, and deterministic validation checks together.
func TestValidateInstancePreservesBoundDiagnosticsAndExactNumericValues(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root" version="1.1">
  <xs:element name="amount" type="r:Amount"/>
  <xs:element name="hugeInteger" type="r:HugeInteger"/>
  <xs:element name="zero" type="r:Zero"/>
  <xs:simpleType name="Amount"><xs:restriction base="xs:decimal"><xs:minInclusive value="123456789012345678901234567890.120000"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="HugeInteger"><xs:restriction base="xs:integer"><xs:minInclusive value="999999999999999999999999999999999999"/></xs:restriction></xs:simpleType>
  <xs:simpleType name="Zero"><xs:restriction base="xs:decimal"><xs:minInclusive value="-0.00"/><xs:maxInclusive value="0.000"/></xs:restriction></xs:simpleType>
</xs:schema>`
	schema := validationTestSchema(t, root, nil)

	for _, input := range []string{
		`<amount xmlns="urn:root">123456789012345678901234567890.12</amount>`,
		`<hugeInteger xmlns="urn:root">+0999999999999999999999999999999999999</hugeInteger>`,
		`<zero xmlns="urn:root">-0.000000</zero>`,
	} {
		if err := goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))); err != nil {
			t.Fatalf("ValidateInstance(%q): %v", input, err)
		}
	}

	input := `<amount xmlns="urn:root">123456789012345678901234567890.119999</amount>`
	diagnostic := validationTestDiagnostic(t, goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
	if diagnostic.Code() != goxsd9.BoundValueViolationCode || diagnostic.SpecRef() != "xsd11-datatypes#cvc-minInclusive-valid" {
		t.Fatalf("diagnostic = %s (%q), want XSD 1.1 minInclusive value violation", diagnostic, diagnostic.SpecRef())
	}
	if diagnostic.Loc() != validationTestTextLoc(t, input) {
		t.Fatalf("Loc() = %s, want candidate text location", diagnostic.Loc())
	}
	amountName := validationTestQName(t, "amount")
	amountDeclarations := schema.FindKind(goxsd9.ComponentKindElementDeclaration, amountName)
	amountTypeName := validationTestQName(t, "Amount")
	amountTypes := schema.FindKind(goxsd9.ComponentKindSimpleTypeDefinition, amountTypeName)
	if len(amountDeclarations) != 1 || len(amountTypes) != 1 {
		t.Fatalf("amount evidence declarations/types = %d/%d, want one each", len(amountDeclarations), len(amountTypes))
	}
	amountDefinition, ok := amountTypes[0].SimpleTypeDefinition()
	if !ok {
		t.Fatal("Amount simple type view is missing")
	}
	amountBounds, present := amountDefinition.DecimalBounds()
	if !present || len(amountBounds.Bounds()) != 1 {
		t.Fatalf("Amount bounds = %#v/%t, want one bound", amountBounds.Bounds(), present)
	}
	wantRelated := []goxsd9.Loc{amountDeclarations[0].Loc(), amountTypes[0].Loc(), amountBounds.Bounds()[0].Loc()}
	if !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
		t.Fatalf("Related() = %v, want %v", diagnostic.Related(), wantRelated)
	}
	if diagnostic.Unwrap() == nil {
		t.Fatal("bound diagnostic does not expose its preserved cause")
	}

	before := schema.Components()
	first := validationTestDiagnostic(t, goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
	second := validationTestDiagnostic(t, goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
	if first.Code() != second.Code() || first.Loc() != second.Loc() || !reflect.DeepEqual(first.Related(), second.Related()) || first.SpecRef() != second.SpecRef() || first.Error() != second.Error() {
		t.Fatalf("repeated bound diagnostics differ: first %v, second %v", first, second)
	}
	if !reflect.DeepEqual(before, schema.Components()) {
		t.Fatal("bound validation mutated the completed schema")
	}
}

func TestValidateInstanceEnforcesBoundsInDirectScalarChoices(t *testing.T) {
	root := `<xs:schema xmlns:xs="` + validationTestXSDNamespace + `" xmlns:r="urn:root" targetNamespace="urn:root">
  <xs:element name="choiceRoot" type="r:Choice"/>
  <xs:complexType name="Choice"><xs:choice><xs:element name="bounded" type="r:Bounded"/></xs:choice></xs:complexType>
  <xs:simpleType name="Bounded"><xs:restriction base="xs:integer"><xs:minExclusive value="10"/></xs:restriction></xs:simpleType>
</xs:schema>`
	schema := validationTestSchema(t, root, nil)
	input := `<choiceRoot xmlns="urn:root"><bounded xmlns="">10</bounded></choiceRoot>`
	diagnostic := validationTestDiagnostic(t, goxsd9.ValidateInstance(schema, "instance.xml", io.NopCloser(strings.NewReader(input))))
	if diagnostic.Class() != goxsd9.FailureInvalid || diagnostic.Code() != goxsd9.BoundValueViolationCode {
		t.Fatalf("diagnostic = %s, want invalid bound-value violation", diagnostic)
	}
	if diagnostic.Loc() != validationChoiceElementTextLoc(t, input, "bounded") {
		t.Fatalf("Loc() = %s, want selected scalar text location", diagnostic.Loc())
	}
	if diagnostic.SpecRef() != "xsd11-datatypes#cvc-minExclusive-valid" {
		t.Fatalf("SpecRef() = %q, want xsd11 minExclusive value reference", diagnostic.SpecRef())
	}
	choiceName := validationTestQName(t, "Choice")
	choiceComponents := schema.FindKind(goxsd9.ComponentKindComplexTypeDefinition, choiceName)
	boundedName := validationTestQName(t, "Bounded")
	boundedTypes := schema.FindKind(goxsd9.ComponentKindSimpleTypeDefinition, boundedName)
	if len(choiceComponents) != 1 || len(boundedTypes) != 1 {
		t.Fatalf("choice evidence complex/types = %d/%d, want one each", len(choiceComponents), len(boundedTypes))
	}
	choiceDefinition, ok := choiceComponents[0].ComplexTypeDefinition()
	if !ok {
		t.Fatal("Choice complex type view is missing")
	}
	choice, ok := choiceDefinition.Particle().(goxsd9.ChoiceParticle)
	if !ok || len(choice.Alternatives()) != 1 {
		t.Fatal("Choice particle evidence is incomplete")
	}
	boundedElement, ok := choice.Alternatives()[0].(goxsd9.ElementParticle)
	if !ok {
		t.Fatal("bounded choice alternative is not an element")
	}
	boundedDefinition, ok := boundedTypes[0].SimpleTypeDefinition()
	if !ok {
		t.Fatal("Bounded simple type view is missing")
	}
	bounds, present := boundedDefinition.IntegerBounds()
	if !present || len(bounds.Bounds()) != 1 {
		t.Fatal("Bounded integer bounds are missing")
	}
	wantRelated := []goxsd9.Loc{
		schema.FindKind(goxsd9.ComponentKindElementDeclaration, validationTestQName(t, "choiceRoot"))[0].Loc(),
		choiceComponents[0].Loc(),
		choice.Loc(),
		boundedElement.Loc(),
		boundedTypes[0].Loc(),
		bounds.Bounds()[0].Loc(),
	}
	if !reflect.DeepEqual(diagnostic.Related(), wantRelated) {
		t.Fatalf("Related() = %v, want %v", diagnostic.Related(), wantRelated)
	}
}

func validationTestQName(t *testing.T, local string) goxsd9.QName {
	t.Helper()
	name, err := goxsd9.NewQName("urn:root", local)
	if err != nil {
		t.Fatalf("NewQName: %v", err)
	}
	return name
}
