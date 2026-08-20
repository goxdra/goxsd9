package goxsd9

import (
	"errors"
	"reflect"
	"testing"
)

func TestSchemaPreservesDiscoveryAndLexicalWalkOrder(t *testing.T) {
	first := mustTestQName(t, "urn:first", "shared")
	second := mustTestQName(t, "urn:second", "shared")
	last := mustTestQName(t, "urn:second", "last")
	firstRootLoc := mustTestLoc(t, "first.xsd", 1, 1)
	secondRootLoc := mustTestLoc(t, "second.xsd", 1, 1)
	firstLoc := mustTestLoc(t, "first.xsd", 2, 3)
	secondLoc := mustTestLoc(t, "second.xsd", 4, 5)

	schema := mustTestSchema(t, []schemaDocumentInput{
		{
			source:          "first.xsd",
			rootLoc:         firstRootLoc,
			targetNamespace: "urn:first",
			declarations: []schemaComponentInput{
				{kind: ComponentKindElementDeclaration, name: first, loc: firstLoc},
			},
		},
		{
			source:          "second.xsd",
			rootLoc:         secondRootLoc,
			targetNamespace: "urn:second",
			declarations: []schemaComponentInput{
				{kind: ComponentKindElementDeclaration, name: second, loc: secondLoc},
				{kind: ComponentKindComplexTypeDefinition, name: last, loc: secondLoc},
			},
		},
	})
	components := schema.Components()
	if got, want := len(components), 3; got != want {
		t.Fatalf("Components() length = %d, want %d", got, want)
	}
	if got, want := components[0].Document(), SourceID("first.xsd"); got != want {
		t.Fatalf("component 0 document = %q, want %q", got, want)
	}
	if got, want := components[0].Loc(), firstLoc; got != want {
		t.Fatalf("component 0 location = %v, want %v", got, want)
	}
	if got, want := components[1].Name(), second; got != want {
		t.Fatalf("component 1 name = %q, want %q", got, want)
	}
	if got, want := components[1].ID().Ordinal(), uint64(1); got != want {
		t.Fatalf("component 1 ordinal = %d, want %d", got, want)
	}
	if got, want := components[2].ID().Ordinal(), uint64(2); got != want {
		t.Fatalf("component 2 ordinal = %d, want %d", got, want)
	}

	var walked []ComponentID
	if err := schema.Walk(func(component Component) error {
		walked = append(walked, component.ID())
		return nil
	}); err != nil {
		t.Fatalf("Walk: %v", err)
	}
	wantWalk := []ComponentID{components[0].ID(), components[1].ID(), components[2].ID()}
	if !reflect.DeepEqual(walked, wantWalk) {
		t.Fatalf("walk order = %#v, want %#v", walked, wantWalk)
	}

	found := schema.Find(first)
	if len(found) != 1 || found[0].ID() != components[0].ID() {
		t.Fatalf("Find(first) = %#v, want first component", found)
	}
	if got := schema.FindKind(ComponentKindComplexTypeDefinition, last); len(got) != 1 || got[0].ID() != components[2].ID() {
		t.Fatalf("FindKind(last) = %#v, want last component", got)
	}
}

func TestSchemaQueryAndCollectionsOwnTheirReturnedValues(t *testing.T) {
	name, err := NewQName("urn:test", "item")
	if err != nil {
		t.Fatalf("NewQName: %v", err)
	}
	rootLoc := mustTestLoc(t, "schema.xsd", 1, 1)
	mutableRootLoc := mustTestLoc(t, "mutable-input.xsd", 1, 1)
	schema, err := newSchema([]schemaDocumentInput{
		{
			source:  "schema.xsd",
			rootLoc: rootLoc,
			declarations: []schemaComponentInput{
				{kind: ComponentKindElementDeclaration, name: name},
				{kind: ComponentKindAttributeDeclaration, name: name},
			},
		},
	})
	if err != nil {
		t.Fatalf("newSchema: %v", err)
	}
	inputs := []schemaDocumentInput{{
		source:  "mutable-input.xsd",
		rootLoc: mutableRootLoc,
		declarations: []schemaComponentInput{{
			kind: ComponentKindElementDeclaration,
			name: name,
		}},
	}}
	inputSchema, err := newSchema(inputs)
	if err != nil {
		t.Fatalf("newSchema(input): %v", err)
	}
	inputs[0].declarations[0] = schemaComponentInput{}
	if got := inputSchema.Components()[0].Name(); got != name {
		t.Fatalf("input mutation changed completed schema name to %q", got)
	}

	components := schema.Components()
	components[0] = Component{}
	if got := schema.Components()[0].Kind(); got != ComponentKindElementDeclaration {
		t.Fatalf("Components() mutation changed schema kind to %q", got)
	}

	documents := schema.Documents()
	documentComponents := documents[0].Components()
	documentComponents[0] = Component{}
	if got := schema.Documents()[0].Components()[0].Kind(); got != ComponentKindElementDeclaration {
		t.Fatalf("document component mutation changed schema kind to %q", got)
	}

	for iteration := 0; iteration < 5; iteration++ {
		found := schema.Find(name)
		found[0] = Component{}
		if got := schema.Find(name)[0].Kind(); got != ComponentKindElementDeclaration {
			t.Fatalf("Find() mutation changed schema kind to %q", got)
		}
	}

	component, ok := schema.Lookup(schema.Components()[0].ID())
	if !ok {
		t.Fatal("Lookup did not find component")
	}
	if got, want := component.Loc(), (Loc{}); got != want {
		t.Fatalf("Lookup location = %v, want zero location", got)
	}
	if _, ok := schema.Lookup(ComponentID{source: "missing.xsd", ordinal: 1}); ok {
		t.Fatal("Lookup found an unknown component")
	}
}

func TestSchemaDuplicateNamesRemainInWalkOrder(t *testing.T) {
	name, err := NewQName("urn:test", "item")
	if err != nil {
		t.Fatalf("NewQName: %v", err)
	}
	oneRootLoc := mustTestLoc(t, "one.xsd", 1, 1)
	twoRootLoc := mustTestLoc(t, "two.xsd", 1, 1)
	schema, err := newSchema([]schemaDocumentInput{
		{
			source:  "one.xsd",
			rootLoc: oneRootLoc,
			declarations: []schemaComponentInput{
				{kind: ComponentKindElementDeclaration, name: name},
			},
		},
		{
			source:  "two.xsd",
			rootLoc: twoRootLoc,
			declarations: []schemaComponentInput{
				{kind: ComponentKindElementDeclaration, name: name},
			},
		},
	})
	if err != nil {
		t.Fatalf("newSchema: %v", err)
	}

	found := schema.Find(name)
	if got, want := len(found), 2; got != want {
		t.Fatalf("Find() length = %d, want %d", got, want)
	}
	if got, want := found[0].Document(), SourceID("one.xsd"); got != want {
		t.Fatalf("Find()[0] document = %q, want %q", got, want)
	}
	if got, want := found[1].Document(), SourceID("two.xsd"); got != want {
		t.Fatalf("Find()[1] document = %q, want %q", got, want)
	}
}

func TestSchemaRejectsInvalidConstructionAndPreservesWalkErrors(t *testing.T) {
	name, err := NewQName("urn:test", "item")
	if err != nil {
		t.Fatalf("NewQName: %v", err)
	}
	sameRootLoc := mustTestLoc(t, "same", 1, 1)
	schemaRootLoc := mustTestLoc(t, "schema", 1, 1)
	for _, test := range []struct {
		name  string
		input []schemaDocumentInput
		code  string
	}{
		{name: "empty source", input: []schemaDocumentInput{{source: ""}}, code: diagnosticSchemaEmptySourceCode},
		{name: "repeated source", input: []schemaDocumentInput{{source: "same", rootLoc: sameRootLoc}, {source: "same"}}, code: diagnosticSchemaRepeatedSourceCode},
		{name: "empty kind", input: []schemaDocumentInput{{source: "schema", rootLoc: schemaRootLoc, declarations: []schemaComponentInput{{name: name}}}}, code: diagnosticSchemaEmptyKindCode},
		{name: "empty name", input: []schemaDocumentInput{{source: "schema", rootLoc: schemaRootLoc, declarations: []schemaComponentInput{{kind: ComponentKindElementDeclaration}}}}, code: diagnosticSchemaEmptyNameCode},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, constructionErr := newSchema(test.input)
			if constructionErr == nil {
				t.Fatal("newSchema succeeded for invalid input")
			}
			var diagnostic Diagnostic
			if !errors.As(constructionErr, &diagnostic) {
				t.Fatalf("error %T is not a Diagnostic: %v", constructionErr, constructionErr)
			}
			if got := diagnostic.Code(); got != test.code {
				t.Fatalf("diagnostic code = %q, want %q", got, test.code)
			}
		})
	}

	schema, err := newSchema([]schemaDocumentInput{{
		source:  "schema",
		rootLoc: schemaRootLoc,
		declarations: []schemaComponentInput{{
			kind: ComponentKindElementDeclaration,
			name: name,
		}},
	}})
	if err != nil {
		t.Fatalf("newSchema: %v", err)
	}
	visitorErr := errors.New("stop")
	err = schema.Walk(func(Component) error { return visitorErr })
	if !errors.Is(err, visitorErr) {
		t.Fatalf("Walk error does not preserve visitor error: %v", err)
	}
	if err := schema.Walk(nil); err == nil {
		t.Fatal("Walk accepted a nil visitor")
	}
}

func TestSchemaRejectsMissingMalformedOrMismatchedRootLocation(t *testing.T) {
	mismatchedRootLoc := mustTestLoc(t, "other", 2, 3)
	zeroLineRootLoc := Loc{source: "schema", line: 0, column: 1}
	negativeLineRootLoc := Loc{source: "schema", line: -1, column: 1}
	zeroColumnRootLoc := Loc{source: "schema", line: 1, column: 0}
	negativeColumnRootLoc := Loc{source: "schema", line: 1, column: -1}
	for _, test := range []struct {
		name    string
		input   schemaDocumentInput
		wantLoc Loc
	}{
		{name: "missing", input: schemaDocumentInput{source: "schema"}},
		{name: "zero line", input: schemaDocumentInput{source: "schema", rootLoc: zeroLineRootLoc}, wantLoc: zeroLineRootLoc},
		{name: "negative line", input: schemaDocumentInput{source: "schema", rootLoc: negativeLineRootLoc}, wantLoc: negativeLineRootLoc},
		{name: "zero column", input: schemaDocumentInput{source: "schema", rootLoc: zeroColumnRootLoc}, wantLoc: zeroColumnRootLoc},
		{name: "negative column", input: schemaDocumentInput{source: "schema", rootLoc: negativeColumnRootLoc}, wantLoc: negativeColumnRootLoc},
		{name: "mismatched source", input: schemaDocumentInput{source: "schema", rootLoc: mismatchedRootLoc}, wantLoc: mismatchedRootLoc},
	} {
		t.Run(test.name, func(t *testing.T) {
			assertSchemaRootLocationRejected(t, test.input, test.wantLoc)
		})
	}
}

func assertSchemaRootLocationRejected(t *testing.T, input schemaDocumentInput, wantLoc Loc) {
	t.Helper()
	schema, err := newSchema([]schemaDocumentInput{input})
	if err == nil {
		t.Fatal("newSchema accepted an invalid root location")
	}
	if schema.storage != nil || len(schema.Documents()) != 0 || len(schema.Components()) != 0 {
		t.Fatal("newSchema returned a partial schema after an invariant failure")
	}
	var diagnostic Diagnostic
	if !errors.As(err, &diagnostic) {
		t.Fatalf("error %T is not a Diagnostic: %v", err, err)
	}
	if diagnostic.Class() != FailureInternal || diagnostic.Code() != diagnosticSchemaBridgeInvariantCode {
		t.Fatalf("diagnostic = %s, want internal %s invariant", diagnostic, diagnosticSchemaBridgeInvariantCode)
	}
	if diagnostic.Loc() != wantLoc {
		t.Fatalf("diagnostic location = %s, want %s", diagnostic.Loc(), wantLoc)
	}
}

func mustTestQName(t *testing.T, namespace, local string) QName {
	t.Helper()
	name, err := NewQName(namespace, local)
	if err != nil {
		t.Fatalf("NewQName: %v", err)
	}
	return name
}

func mustTestLoc(t *testing.T, source SourceID, line, column int) Loc {
	t.Helper()
	loc, err := NewLoc(source, line, column)
	if err != nil {
		t.Fatalf("NewLoc: %v", err)
	}
	return loc
}

func mustTestSchema(t *testing.T, inputs []schemaDocumentInput) Schema {
	t.Helper()
	schema, err := newSchema(inputs)
	if err != nil {
		t.Fatalf("newSchema: %v", err)
	}
	return schema
}

func TestQNameRequiresLocalNameAndFormatsExpandedNames(t *testing.T) {
	if _, err := NewQName("urn:test", ""); err == nil {
		t.Fatal("NewQName accepted an empty local name")
	}
	name, err := NewQName("urn:test", "item")
	if err != nil {
		t.Fatalf("NewQName: %v", err)
	}
	if got, want := name.String(), "{urn:test}item"; got != want {
		t.Fatalf("QName.String() = %q, want %q", got, want)
	}
	if got, want := name.Namespace(), "urn:test"; got != want {
		t.Fatalf("QName.Namespace() = %q, want %q", got, want)
	}
	if got, want := name.Local(), "item"; got != want {
		t.Fatalf("QName.Local() = %q, want %q", got, want)
	}
	if name.IsZero() {
		t.Fatal("non-zero QName reports IsZero")
	}
}
